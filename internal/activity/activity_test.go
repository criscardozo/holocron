package activity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/qbittorrent"
)

func arrServer(t *testing.T, fixture string, status int) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile("../arr/testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDownloadsAreJoinedToTheirTorrent: the *arr says what a download is for,
// the torrent says how fast it is going, and they meet on the hash — upper case
// on one side, lower on the other.
func TestDownloadsAreJoinedToTheirTorrent(t *testing.T) {
	t.Parallel()
	radarr := arrServer(t, "radarr_queue.json", http.StatusOK)
	src := Sources{
		Torrents: func(context.Context) ([]qbittorrent.Torrent, error) {
			return []qbittorrent.Torrent{
				{Hash: "ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12", Name: "Dune.2021.1080p", Progress: 0.8, DlSpeed: 5_000_000, State: "downloading"},
				{Hash: "ffff", Name: "ubuntu-24.04.iso", Progress: 0.1, DlSpeed: 1_000_000, State: "downloading"},
			}, nil
		},
		Queues: []*arr.Client{arr.New(arr.Radarr, radarr.URL, "k")},
	}
	snap := NewSampler(func(context.Context) Sources { return src }).Sample(t.Context())

	if len(snap.Downloads) != 2 {
		t.Fatalf("downloads = %+v", snap.Downloads)
	}
	var dune, manual *Download
	for i := range snap.Downloads {
		switch snap.Downloads[i].App {
		case arr.Radarr:
			dune = &snap.Downloads[i]
		case "":
			manual = &snap.Downloads[i]
		}
	}
	if dune == nil || !dune.HasTorrent || dune.Speed != 5_000_000 || dune.Subject != "Dune (2021)" {
		t.Errorf("the Radarr item was not joined to its torrent: %+v", dune)
	}
	if dune != nil && dune.Progress != 0.8 {
		t.Errorf("progress = %v, want the torrent's own figure, which is current", dune.Progress)
	}
	if manual == nil || manual.Subject != "ubuntu-24.04.iso" {
		t.Errorf("a torrent no *arr claims must still be listed: %+v", manual)
	}
	if snap.DownSpeed != 6_000_000 {
		t.Errorf("total speed = %d", snap.DownSpeed)
	}
}

// TestOneSourceFailingDoesNotBlankTheOthers, and the failure is named:
// "nothing downloading" and "Sonarr is down" look identical as an empty list.
func TestOneSourceFailingDoesNotBlankTheOthers(t *testing.T) {
	t.Parallel()
	radarr := arrServer(t, "radarr_queue.json", http.StatusOK)
	sonarr := arrServer(t, "sonarr_queue.json", http.StatusInternalServerError)
	src := Sources{
		Sessions: func(context.Context) ([]jellyfin.Session, error) { return nil, errors.New("down") },
		Queues: []*arr.Client{
			arr.New(arr.Radarr, radarr.URL, "k"),
			arr.New(arr.Sonarr, sonarr.URL, "k"),
		},
	}
	snap := NewSampler(func(context.Context) Sources { return src }).Sample(t.Context())
	if len(snap.Downloads) != 1 {
		t.Errorf("the working source's downloads were lost: %+v", snap.Downloads)
	}
	if len(snap.Errors) != 2 || snap.Errors[0] != "Jellyfin" || snap.Errors[1] != "Sonarr" {
		t.Errorf("errors = %v, want Jellyfin and Sonarr named", snap.Errors)
	}
}

// TestProblemsComeFirst: a blocked import is the one thing on this list that
// needs a person, so it does not sit below a healthy download.
func TestProblemsComeFirst(t *testing.T) {
	t.Parallel()
	sonarr := arrServer(t, "sonarr_queue.json", http.StatusOK)
	radarr := arrServer(t, "radarr_queue.json", http.StatusOK)
	src := Sources{Queues: []*arr.Client{arr.New(arr.Radarr, radarr.URL, "k"), arr.New(arr.Sonarr, sonarr.URL, "k")}}
	snap := NewSampler(func(context.Context) Sources { return src }).Sample(t.Context())
	if len(snap.Downloads) == 0 || snap.Downloads[0].State != "importBlocked" {
		t.Errorf("first = %+v, want the blocked import", snap.Downloads)
	}
}

// TestRecentlyAddedIsNotPolled: it changes a few times a day, so asking
// Jellyfin for it on every reading would be exactly the polling to avoid.
func TestRecentlyAddedIsNotPolled(t *testing.T) {
	t.Parallel()
	calls := 0
	src := Sources{Recent: func(context.Context, int) ([]jellyfin.Added, error) {
		calls++
		return []jellyfin.Added{{Name: "Dune"}}, nil
	}}
	s := NewSampler(func(context.Context) Sources { return src })
	for i := 0; i < 5; i++ {
		s.Sample(t.Context())
	}
	if calls != 1 {
		t.Errorf("asked Jellyfin %d times in five readings, want once", calls)
	}
}
