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
	playing.NowPlaying = &struct {
		Name              string `json:"Name"`
		SeriesName        string `json:"SeriesName"`
		Type              string `json:"Type"`
		ProductionYear    int    `json:"ProductionYear"`
		IndexNumber       *int   `json:"IndexNumber"`
		ParentIndexNumber *int   `json:"ParentIndexNumber"`
		RunTimeTicks      int64  `json:"RunTimeTicks"`
	}{Name: "Honeydew", SeriesName: "The Bear", Type: "Episode", IndexNumber: i(4), ParentIndexNumber: i(2), RunTimeTicks: 18_000_000_000}
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
