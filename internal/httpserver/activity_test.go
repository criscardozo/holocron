package httpserver

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/activity"
	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/jellyfin"
)

// session decodes a /Sessions entry shaped by Jellyfin 12.1's SessionInfoDto.
func session(t *testing.T, raw string) jellyfin.Session {
	t.Helper()
	var s jellyfin.Session
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestACPUTranscodeIsFlagged is the case that matters on Ginebra: Quick Sync
// is there, and a transcode that falls back to four CPU cores is what makes
// the next stream stutter.
func TestACPUTranscodeIsFlagged(t *testing.T) {
	t.Parallel()
	qsv := playingView(session(t, `{
		"UserName":"cristian","DeviceName":"Living","Client":"Jellyfin Android TV",
		"NowPlayingItem":{"Name":"Honeydew","SeriesName":"The Bear","Type":"Episode","IndexNumber":4,"ParentIndexNumber":2,"RunTimeTicks":18000000000},
		"PlayState":{"PositionTicks":9000000000,"IsPaused":false,"PlayMethod":"Transcode"},
		"TranscodingInfo":{"IsVideoDirect":false,"VideoCodec":"h264","Height":1080,"Bitrate":8000000,
			"HardwareAccelerationType":"qsv","TranscodeReasons":["VideoBitDepthNotSupported"]}}`))
	if qsv.Hardware != "Quick Sync" || qsv.OnCPU {
		t.Errorf("qsv transcode = %+v", qsv)
	}
	if qsv.Title != "The Bear" || qsv.Subtitle != "T2E04 Honeydew" || qsv.Position != "15:00 de 30:00" || qsv.Width != "50.0" {
		t.Errorf("what is playing = %+v", qsv)
	}
	if len(qsv.Reasons) != 1 || qsv.Reasons[0] != "video de 10 bits" {
		t.Errorf("reasons = %v", qsv.Reasons)
	}

	cpu := playingView(session(t, `{"NowPlayingItem":{"Name":"Dune","ProductionYear":2021},
		"PlayState":{"PlayMethod":"Transcode"},
		"TranscodingInfo":{"IsVideoDirect":false,"HardwareAccelerationType":"none"}}`))
	if cpu.Hardware != "CPU" || !cpu.OnCPU {
		t.Errorf("a CPU transcode must be flagged: %+v", cpu)
	}

	direct := playingView(session(t, `{"NowPlayingItem":{"Name":"Dune"},"PlayState":{"PlayMethod":"DirectPlay"}}`))
	if direct.Transcode || direct.Hardware != "" || direct.Method != "directo" {
		t.Errorf("direct play = %+v", direct)
	}
}

func TestABlockedImportNeedsAPerson(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	d := downloadView(activity.Download{
		App: arr.Sonarr, Subject: "The Bear · T2E04", State: "importBlocked", Health: "warning",
		Progress: 1, Messages: []string{"No se encontraron archivos aptos para importar"},
	}, now)
	if !d.Problem || d.State != "importación trabada" || d.App != "Sonarr" {
		t.Errorf("blocked import = %+v", d)
	}
	moving := downloadView(activity.Download{
		App: arr.Radarr, Subject: "Dune (2021)", State: "downloading", Progress: 0.5,
		Speed: 5_000_000, TorrentState: "downloading", HasTorrent: true, ETA: now.Add(20 * time.Minute),
	}, now)
	if moving.Problem || moving.ETA == "" || !strings.HasPrefix(moving.ETA, "faltan") {
		t.Errorf("moving download = %+v", moving)
	}
}

func TestTheActivityPageRenders(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	body := ts.get(t, "/activity", nil).Body
	for _, want := range []string{`sse-connect="/events/activity"`, "Reproduciendo", "Descargas", "Jellyfin no está vinculado"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page is missing %q", want)
		}
	}
}
