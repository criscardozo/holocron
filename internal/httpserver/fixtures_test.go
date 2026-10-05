package httpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/activity"
	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/bazarr"
	"github.com/cristian/holocron/internal/hardware"
	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/seerr"
	"github.com/cristian/holocron/internal/services"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

// TestWriteIOSFixtures writes the JSON the iOS contract tests decode, for the
// live screens. Opt-in with HOLOCRON_WRITE_FIXTURES=<dir>.
//
// These come from the real encoder over realistic readings — values measured
// on Ginebra — rather than from a capture, because a capture on the
// development Mac has no /proc and no services and would come out empty, which
// tests nothing. What the contract tests need is the exact shape Go sends, and
// that is what this produces.
func TestWriteIOSFixtures(t *testing.T) {
	dir := os.Getenv("HOLOCRON_WRITE_FIXTURES")
	if dir == "" {
		t.Skip("set HOLOCRON_WRITE_FIXTURES to the iOS fixtures directory")
	}
	now := time.Date(2026, 10, 5, 13, 0, 0, 0, time.Local)
	i := func(v int) *int { return &v }
	i64 := func(v int64) *int64 { return &v }

	hw := hardware.Snapshot{
		At: now, HasCPU: true, CPUPct: 7.4, HasTemp: true, TempC: 45, HasLoad: true, HasUp: true,
		Load:   hardware.Load{One: 0.27, Five: 0.32, Fifteen: 0.26},
		Uptime: 2*time.Hour + 38*time.Minute,
		Cores: []hardware.Core{
			{Name: "cpu0", BusyPct: 12, MHz: 3900}, {Name: "cpu1", BusyPct: 3, MHz: 800},
			{Name: "cpu2", BusyPct: 9, MHz: 1600}, {Name: "cpu3", BusyPct: 91, MHz: 3900},
		},
		HasMem: true,
		Memory: hardware.Memory{Total: 20 << 30, Available: 16 << 30, SwapTotal: 8 << 30, SwapUsed: 1 << 20,
			ZramData: 4 << 20, ZramStored: 1 << 20, ZramSize: 4 << 30},
		Links: []hardware.Link{
			{Name: "enp1s0", Up: true, SpeedMbps: 1000, RxBps: 150_000, TxBps: 40_000},
			{Name: "tailscale0", Up: true}, {Name: "wlp2s0"},
		},
		Disks: []hardware.Disk{
			{Name: "nvme0n1", Model: "KINGSTON RBUSNS8154P3256GJ1", ReadBps: 2_000_000, BusyPct: 3},
			{Name: "sdb", Model: "Expansion HDD"},
		},
		Battery: hardware.Battery{Present: true, Percent: 100, Status: "Full", OnAC: true, HealthPct: 65.4},
		History: hardware.History{CPU: []float64{5, 8, 7, 12, 7}, Rx: []float64{1e5, 1.5e5, 1.2e5, 1.5e5}},
	}

	playing := jellyfin.Session{UserName: "cristian", DeviceName: "Living", Client: "Jellyfin Android TV"}
	playing.NowPlaying = &jellyfin.NowPlayingItem{ID: "0b6f0e4a1c2d4e5f8a9b0c1d2e3f4a5b", SeriesID: "f27caa37e5142225cceded48f6553502", Name: "Honeydew", SeriesName: "The Bear", Type: "Episode", IndexNumber: i(4), ParentIndexNumber: i(2), RunTimeTicks: 18_000_000_000}
	playing.PlayState = &struct {
		PositionTicks int64  `json:"PositionTicks"`
		IsPaused      bool   `json:"IsPaused"`
		PlayMethod    string `json:"PlayMethod"`
	}{PositionTicks: 9_000_000_000, PlayMethod: "Transcode"}
	playing.Transcoding = &jellyfin.Transcoding{VideoCodec: "h264", Height: 1080, Bitrate: 8_000_000,
		HardwareAccelerationType: "qsv", TranscodeReasons: []string{"VideoBitDepthNotSupported"}}

	act := activity.Snapshot{
		At: now, Playing: []jellyfin.Session{playing}, Idle: 2,
		HasJellyfin: true, HasTorrents: true, Apps: []arr.App{arr.Radarr, arr.Sonarr},
		DownSpeed: 5_000_000, UpSpeed: 0,
		Downloads: []activity.Download{
			{App: arr.Sonarr, Subject: "The Bear · T2E05", State: "importBlocked", Health: "warning", Progress: 1,
				Messages: []string{"No se encontraron archivos aptos para importar"}},
			{App: arr.Radarr, Subject: "Dune (2021)", State: "downloading", Progress: 0.42, Speed: 5_000_000,
				TorrentState: "downloading", HasTorrent: true, ETA: now.Add(20 * time.Minute)},
		},
		Recent: []jellyfin.Added{{Name: "The Social Dilemma", Type: "Movie", ProductionYear: 2020, DateCreated: now.Add(-3 * time.Hour)}},
		Library: activity.Library{
			At:     now.Add(-2 * time.Minute),
			Counts: &seerr.Counts{Total: 21},
			Requests: []seerr.Request{
				{Title: "Street Fighter", Year: 2026, Type: "movie", By: "cristian", CreatedAt: now.Add(-48 * time.Hour),
					Status: seerr.RequestApproved, MediaStatus: seerr.MediaProcessing, TmdbID: 2},
				{Title: "Deadloch", Year: 2023, Type: "tv", By: "cristian", CreatedAt: now.Add(-72 * time.Hour),
					Status: seerr.RequestCompleted, MediaStatus: seerr.MediaPartiallyAvailable},
			},
			Upcoming: []arr.Upcoming{{App: arr.Radarr, Subject: "Street Fighter (2026)", When: now.Add(8 * 24 * time.Hour), Kind: "cines", TmdbID: 2}},
			Missing:  map[arr.App]int{arr.Radarr: 14, arr.Sonarr: 4},
			Subs:     &bazarr.Badges{Movies: 11, Episodes: 499, RadarrSignalR: "LIVE", SonarrSignalR: "LIVE"},
			Indexers: 5, IndexersEnabled: 5,
		},
	}

	svc := services.Snapshot{
		At: now,
		Units: []services.Unit{
			{Name: "jellyfin", Active: "active", Sub: "running", Since: now.Add(-150 * time.Minute)},
			{Name: "biblioteca (montaje)", Active: "active", Sub: "mounted", Since: now.Add(-150 * time.Minute)},
			{Name: "bazarr", Active: "failed", Result: "exit-code"},
		},
		Timers: []services.Timer{
			{Name: "ginebra-vigia", Last: now.Add(-7 * time.Minute), Next: now.Add(8 * time.Minute), Result: "success"},
			{Name: "ginebra-respaldo-config", Last: now.Add(-26 * time.Hour), Next: now.Add(124 * time.Hour), Result: "success"},
		},
		Smart: &services.Smart{Generated: now.Add(-3 * time.Hour), Disks: []services.SmartDisk{
			{Disk: "/dev/nvme0n1", Model: "KINGSTON", Health: "ok", TempC: i64(34), Hours: i64(1351), WearPct: i64(7), Capacity: 256060514304},
			{Disk: "/dev/sdb", Asleep: true},
			{Disk: "/dev/sdc", Model: "WDC WD30EZRX-00D8PB0", Health: "ok", Reallocated: i64(217)},
		}},
	}

	for name, v := range map[string]any{
		"hardware.json": hardwareView(hw),
		"activity.json": activityView(act, now),
		"services.json": servicesView(svc, true, now),
		"home.json":     fixtureHome(act, svc, now),
	} {
		b, err := json.MarshalIndent(v, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// fixtureHome is the start screen over the same readings, through the same
// functions the start page uses. The registry's tiles are written out with
// values seen on Ginebra, since they read services this test does not have.
func fixtureHome(act activity.Snapshot, svc services.Snapshot, now time.Time) templates.HomeView {
	av := activityView(act, now)
	return templates.HomeView{
		Machine: "Ginebra",
		Status: homeStatus(system.Stats{
			HasCPU: true, CPUPercent: 14, HasTemp: true, TempC: 36,
			MemTotal: 20 << 30, MemUsed: 4 << 30, MemPercent: 20, HasUptime: true, Uptime: 4*time.Hour + 10*time.Minute,
		}, hardware.Battery{Present: true, Percent: 100, Status: "Full", OnAC: true}),
		Attn: []templates.AttnChip{{Label: "Disco4 91%", Href: "/disk", Icon: "drive"}},
		Tiles: []templates.Tile{
			activityTile(av),
			{Href: "/hardware", Icon: "cpu", Tone: "violet", Title: "Hardware", Value: "14 %", Sub: "36 °C · RAM 20 %"},
			servicesTile(servicesView(svc, true, now)),
			{Href: "/disk", Icon: "drive", Tone: "lilac", Title: "Disco", Value: "91 %", Sub: "3.3 TiB de 3.6 TiB · Disco4", Warn: true},
			{Href: "/media", Icon: "film", Tone: "indigo", Title: "Medios", Value: "366", Sub: "325 películas · 41 series"},
			{Href: "/quality", Icon: "gauge", Tone: "amber", Title: "Calidad", Value: "Sin analizar", Sub: "Analizar la biblioteca"},
			{Href: "/naming", Icon: "tag", Tone: "sky", Title: "Nombres", Value: "Todo bien", Sub: "Todo cumple «Título (Año)»"},
			{Href: "/torrents", Icon: "download", Tone: "mint", Title: "Torrents", Value: "—", Sub: "qBittorrent sin configurar", Off: true},
		},
		Recent: av.Recent,
		Mural:  []string{"/art/jf/f27caa37e5142225cceded48f6553502"},
	}
}
