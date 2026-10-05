package httpserver

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/activity"
	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/bazarr"
	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/seerr"
	"github.com/cristian/holocron/web/templates"
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

// TestRequestStateAnswersCanIWatchItYet. A request and its media each have a
// status, and the media's is the one that answers the question — a request
// marked approved can be days from available.
func TestRequestStateAnswersCanIWatchItYet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		req, media int
		want       string
		done       bool
	}{
		{seerr.RequestCompleted, seerr.MediaAvailable, "disponible", true},
		{seerr.RequestApproved, seerr.MediaProcessing, "buscando", false},
		{seerr.RequestCompleted, seerr.MediaPartiallyAvailable, "disponible en parte", false},
		{seerr.RequestPending, seerr.MediaUnknown, "esperando aprobación", false},
		{seerr.RequestDeclined, seerr.MediaUnknown, "rechazado", false},
	}
	for _, c := range cases {
		got, done, _ := requestState(seerr.Request{Status: c.req, MediaStatus: c.media})
		if got != c.want || done != c.done {
			t.Errorf("request %d / media %d = %q (%v), want %q (%v)", c.req, c.media, got, done, c.want, c.done)
		}
	}
}

// TestALostBazarrLinkIsAWarning: Bazarr hears about new files over a live link
// to each *arr, and when it drops new downloads stop getting subtitles with
// nothing saying so.
func TestALostBazarrLinkIsAWarning(t *testing.T) {
	t.Parallel()
	var v templates.ActivityView
	libraryView(&v, activity.Library{
		At:   time.Now(),
		Subs: &bazarr.Badges{Movies: 11, Episodes: 499, RadarrSignalR: "LIVE", SonarrSignalR: "CONNECTING"},
	}, time.Now())
	var warned, counted bool
	for _, a := range v.Attention {
		if strings.Contains(a.Text, "Sonarr") && a.Warn {
			warned = true
		}
		if strings.Contains(a.Text, "11 películas") && strings.Contains(a.Text, "499 episodios") {
			counted = true
		}
	}
	if !warned || !counted {
		t.Errorf("attention = %+v", v.Attention)
	}
}

// TestProcessingSaysWhatIsReallyHappening is the case the Ginebra session
// caught: Seerr's "processing" only means it was handed to Radarr. Read
// literally it said "descargando" for a film still a week from cinemas, with
// nothing in any queue.
func TestProcessingSaysWhatIsReallyHappening(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	a := activity.Snapshot{
		Downloads: []activity.Download{{App: arr.Radarr, TmdbID: 1, Progress: 0.4}},
		Library: activity.Library{
			At: now,
			Requests: []seerr.Request{
				{Title: "En la cola", Type: "movie", TmdbID: 1, Status: seerr.RequestApproved, MediaStatus: seerr.MediaProcessing},
				{Title: "Street Fighter", Type: "movie", TmdbID: 2, Status: seerr.RequestApproved, MediaStatus: seerr.MediaProcessing},
				{Title: "Ni rastro", Type: "movie", TmdbID: 3, Status: seerr.RequestApproved, MediaStatus: seerr.MediaProcessing},
			},
			Upcoming: []arr.Upcoming{{TmdbID: 2, When: now.Add(8 * 24 * time.Hour), Kind: "cines"}},
		},
	}
	v := activityView(a, now)
	want := []string{"descargando 40 %", "en cines en 8 días", "buscando"}
	for i, w := range want {
		if v.Requests[i].State != w {
			t.Errorf("%s = %q, want %q", v.Requests[i].Title, v.Requests[i].State, w)
		}
	}
}
