// Command holocron is the HTPC management dashboard for a Raspberry Pi.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/cristian/holocron/internal/activity"
	"github.com/cristian/holocron/internal/apitoken"
	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/bazarr"
	"github.com/cristian/holocron/internal/config"
	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/diskusage"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/hardware"
	"github.com/cristian/holocron/internal/httpserver"
	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/library"
	"github.com/cristian/holocron/internal/live"
	"github.com/cristian/holocron/internal/naming"
	"github.com/cristian/holocron/internal/power"
	"github.com/cristian/holocron/internal/quality"
	"github.com/cristian/holocron/internal/seerr"
	"github.com/cristian/holocron/internal/services"
	"github.com/cristian/holocron/internal/settings"
	"github.com/cristian/holocron/internal/torrents"
	"github.com/cristian/holocron/internal/updates"
	"github.com/cristian/holocron/internal/widgets"
)

func main() {
	cfg := config.Load()
	logger := newLogger(cfg.LogLevel)

	if err := run(cfg, logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	jobManager := jobs.NewManager()
	folderStore := folders.NewStore(database)
	settingsStore := settings.NewStore(database)
	if len(cfg.BadFolders) > 0 {
		logger.Warn("ignored malformed HOLOCRON_MEDIA_FOLDERS entries", "entries", strings.Join(cfg.BadFolders, ";"))
	}
	if len(cfg.MediaFolders) > 0 {
		specs := make([]folders.Spec, 0, len(cfg.MediaFolders))
		for _, f := range cfg.MediaFolders {
			specs = append(specs, folders.Spec{Label: f.Label, Purpose: f.Purpose, Path: f.Path})
		}
		if err := folderStore.Manage(ctx, specs); err != nil {
			return fmt.Errorf("managed folders: %w", err)
		}
		logger.Info("folders provided by the server", "count", len(specs))
	}
	power.SetMachineName(cfg.MachineName)
	// On Ginebra, systemd hands the services' keys over with LoadCredential.
	// Elsewhere the directory does not exist and the settings form is used.
	creds, err := settings.LoadCredentials(os.Getenv("CREDENTIALS_DIRECTORY"))
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	settingsStore.Manage(creds)
	if len(creds) > 0 {
		names := make([]string, 0, len(creds))
		for n := range creds {
			names = append(names, n)
		}
		sort.Strings(names)
		logger.Info("credentials provided by the server", "names", strings.Join(names, ","))
	}
	diskService := diskusage.NewService(database, folderStore, jobManager)
	namingService := naming.NewService(database, folderStore)
	libraryService := library.NewService(database, settingsStore, jobManager)
	qualityService := quality.NewService(database, settingsStore, jobManager)
	torrentsService := torrents.NewService(settingsStore)
	apiTokenStore := apitoken.NewStore(settingsStore)
	jellyfinLink := jellyfin.NewLinkService(settingsStore)
	updatesService := updates.NewService(filepath.Dir(cfg.DBPath))
	powerService := power.NewService(filepath.Dir(cfg.DBPath))
	activityHub := newActivityHub(libraryService, torrentsService, settingsStore)
	servicesReader := services.NewReader(services.Config{
		Units: cfg.WatchUnits, TimerPrefix: cfg.WatchTimers, SmartFile: cfg.SmartFile,
	})
	// Unit state changes rarely and is one shell-out for all of them, so 15 s
	// is plenty — and like every live screen, only while somebody watches.
	servicesHub := live.NewHub(15*time.Second, servicesReader.Read)

	registry := widgets.NewRegistry(
		widgets.SystemWidget{},
		widgets.NewDiskWidget(folderStore),
		widgets.NewNamingWidget(namingService),
		widgets.NewMediaWidget(libraryService),
		widgets.NewQualityWidget(qualityService),
		widgets.NewTorrentsWidget(torrentsService),
	)

	srv := httpserver.New(httpserver.Deps{
		Log:                logger,
		Widgets:            registry,
		Folders:            folderStore,
		Disk:               diskService,
		Hardware:           hardware.NewHub(2 * time.Second),
		Activity:           activityHub,
		Services:           servicesHub,
		ServicesConfigured: servicesReader.Configured(),
		Naming:             namingService,
		Settings:           settingsStore,
		Library:            libraryService,
		Quality:            qualityService,
		Torrents:           torrentsService,
		APIToken:           apiTokenStore,
		JellyfinLink:       jellyfinLink,
		Updates:            updatesService,
		Power:              powerService,
	})

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("holocron listening", "addr", cfg.Addr, "db", cfg.DBPath)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelHTTP()
	httpErr := httpSrv.Shutdown(httpCtx)

	// Background jobs are detached from the request that started them, so they
	// must be cancelled explicitly. This runs even when the HTTP shutdown timed
	// out — otherwise a slow client would leave the jobs unsignalled — and gets
	// its own budget rather than whatever the HTTP wait left over.
	jobsCtx, cancelJobs := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelJobs()
	if err := jobManager.Shutdown(jobsCtx); err != nil {
		logger.Warn("background jobs did not finish before shutdown", "error", err)
	}
	return httpErr
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}

// Where Ginebra runs Radarr and Sonarr. Fixed, not configurable from the web,
// for the same reason the Jellyfin address is pinned when the server manages
// its key: an editable address is a way to send the key somewhere else.
const (
	radarrAddr   = "http://127.0.0.1:7878"
	sonarrAddr   = "http://127.0.0.1:8989"
	prowlarrAddr = "http://127.0.0.1:9696"
	seerrAddr    = "http://127.0.0.1:5055"
	bazarrAddr   = "http://127.0.0.1:6767"
)

// newActivityHub builds the live "what is happening" hub. Sessions change
// within seconds and a download's speed too, so it reads every 5 s — but only
// while somebody has the screen open.
func newActivityHub(lib *library.Service, tor *torrents.Service, st *settings.Store) *live.Hub[activity.Snapshot] {
	var queues []*arr.Client
	if key, ok := st.Credential(settings.CredRadarr); ok {
		queues = append(queues, arr.New(arr.Radarr, radarrAddr, key))
	}
	if key, ok := st.Credential(settings.CredSonarr); ok {
		queues = append(queues, arr.New(arr.Sonarr, sonarrAddr, key))
	}
	slowSrc := activity.SlowSources{Arrs: queues}
	if key, ok := st.Credential(settings.CredSeerr); ok {
		slowSrc.Seerr = seerr.New(seerrAddr, key)
	}
	if key, ok := st.Credential(settings.CredBazarr); ok {
		slowSrc.Bazarr = bazarr.New(bazarrAddr, key)
	}
	if key, ok := st.Credential(settings.CredProwlarr); ok {
		slowSrc.Prowlarr = arr.New(arr.Prowlarr, prowlarrAddr, key)
	}
	slow := activity.NewSlow(slowSrc)

	sampler := activity.NewSampler(func(ctx context.Context) activity.Sources {
		src := activity.Sources{Queues: queues, Slow: slow}
		if lib.Configured(ctx) {
			src.Sessions = lib.Sessions
			src.Recent = lib.RecentlyAdded
		}
		if tor.Configured(ctx) {
			src.Torrents = tor.List
		}
		return src
	})
	return live.NewHub(5*time.Second, sampler.Sample)
}
