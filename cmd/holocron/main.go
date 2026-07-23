// Command holocron is the HTPC management dashboard for a Raspberry Pi.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cristian/holocron/internal/config"
	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/diskusage"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/httpserver"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/library"
	"github.com/cristian/holocron/internal/naming"
	"github.com/cristian/holocron/internal/settings"
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
	diskService := diskusage.NewService(database, folderStore, jobManager)
	namingService := naming.NewService(database, folderStore)
	libraryService := library.NewService(database, settingsStore, jobManager)

	registry := widgets.NewRegistry(
		widgets.SystemWidget{},
		widgets.NewDiskWidget(folderStore),
		widgets.NewNamingWidget(namingService),
		widgets.NewMediaWidget(libraryService),
	)

	srv := httpserver.New(httpserver.Deps{
		Log:      logger,
		Widgets:  registry,
		Folders:  folderStore,
		Disk:     diskService,
		Naming:   namingService,
		Settings: settingsStore,
		Library:  libraryService,
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
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
