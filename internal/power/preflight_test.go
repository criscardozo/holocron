package power

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeMedia struct {
	playing []string
	tasks   []string
	err     error
}

func (f fakeMedia) NowPlaying(context.Context) ([]string, error) {
	return f.playing, f.err
}

func (f fakeMedia) RunningTasks(context.Context) ([]string, error) {
	return f.tasks, f.err
}

type fakeTorrents struct {
	active int
	err    error
}

func (f fakeTorrents) ActiveTorrents(context.Context) (int, error) {
	return f.active, f.err
}

func TestCheckNamesWhatWouldBeInterrupted(t *testing.T) {
	t.Parallel()
	p := Check(t.Context(),
		fakeMedia{playing: []string{"Chernobyl · 1:23:45"}, tasks: []string{"Escanear biblioteca"}},
		fakeTorrents{active: 3})

	if !p.Busy() || !p.Checked {
		t.Fatalf("busy=%v checked=%v", p.Busy(), p.Checked)
	}
	joined := strings.Join(p.Warnings, " | ")
	// Naming the episode is the point: "alguien está mirando algo" does not
	// help anyone decide.
	for _, want := range []string{"Chernobyl · 1:23:45", "Escanear biblioteca", "3 torrents"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings should mention %q, got %q", want, joined)
		}
	}
}

func TestQuietMachineHasNoWarnings(t *testing.T) {
	t.Parallel()
	p := Check(t.Context(), fakeMedia{}, fakeTorrents{active: 0})
	if p.Busy() {
		t.Errorf("nothing is happening, got %v", p.Warnings)
	}
	if !p.Checked {
		t.Error("the probes answered, so this is a real all-clear")
	}
}

// TestUnreachableProbesDoNotClaimAllClear is the distinction that matters: an
// empty warning list because nothing is happening and an empty one because
// nothing could be asked look identical, and only one of them means it is safe
// to power off.
func TestUnreachableProbesDoNotClaimAllClear(t *testing.T) {
	t.Parallel()
	down := errors.New("connection refused")
	p := Check(t.Context(), fakeMedia{err: down}, fakeTorrents{err: down})

	if p.Busy() {
		t.Error("nothing could be checked, so nothing can be reported as busy")
	}
	if p.Checked {
		t.Error("Checked must stay false, or the screen would imply an all-clear it never got")
	}
}

// TestOneProbeAnsweringStillCounts: qBittorrent being unreachable is not a
// reason to withhold what Jellyfin said.
func TestOneProbeAnsweringStillCounts(t *testing.T) {
	t.Parallel()
	p := Check(t.Context(),
		fakeMedia{playing: []string{"Dune"}},
		fakeTorrents{err: errors.New("no qbittorrent")})

	if !p.Checked || !p.Busy() {
		t.Fatalf("checked=%v busy=%v", p.Checked, p.Busy())
	}
	if len(p.Warnings) != 1 {
		t.Errorf("warnings = %v, want just the one that could be determined", p.Warnings)
	}
}

func TestSingleTorrentReadsNaturally(t *testing.T) {
	t.Parallel()
	p := Check(t.Context(), nil, fakeTorrents{active: 1})
	if got := strings.Join(p.Warnings, ""); !strings.Contains(got, "1 torrent activo") ||
		strings.Contains(got, "torrents activos") {
		t.Errorf("warning = %q, want the singular", got)
	}
}

func TestNilProbesAreSafe(t *testing.T) {
	t.Parallel()
	// Nothing configured at all, which is a real state on a fresh install.
	p := Check(t.Context(), nil, nil)
	if p.Busy() || p.Checked {
		t.Errorf("busy=%v checked=%v; nothing was asked", p.Busy(), p.Checked)
	}
}
