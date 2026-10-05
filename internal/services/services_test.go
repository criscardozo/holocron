package services

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// The testdata files are real output from Ginebra (systemd 257), captured
// before this was written, because the two commands disagree about time:
// `show` prints a timer's last trigger in local wall time even with
// --timestamp=unix and leaves its next elapse empty, while `list-timers -o
// json` gives both in microseconds.
func fakeReader(t *testing.T, units ...string) *Reader {
	t.Helper()
	timers, err := os.ReadFile("testdata/list-timers.json")
	if err != nil {
		t.Fatal(err)
	}
	show, err := os.ReadFile("testdata/show.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := NewReader(Config{
		Units: units, TimerPrefix: "ginebra-",
		Mounts:    []string{"/mnt/biblioteca", "/mnt/disco3"},
		SmartFile: "testdata/smart.json",
	})
	r.mountinfo = "testdata/mountinfo"
	r.run = func(_ context.Context, args ...string) ([]byte, error) {
		switch args[0] {
		case "list-timers":
			return timers, nil
		case "show":
			if !contains(args, "--timestamp=unix") {
				t.Error("show without --timestamp=unix prints ambiguous local times")
			}
			return show, nil
		}
		t.Fatalf("unexpected systemctl %v", args)
		return nil, nil
	}
	return r
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func TestReadsUnitsTimersMountsAndSmartFromRealOutput(t *testing.T) {
	t.Parallel()
	s := fakeReader(t, "jellyfin", "qbittorrent", "caddy", "no-existe").Read(t.Context())

	if len(s.Units) != 4 {
		t.Fatalf("units = %+v", s.Units)
	}
	if u := s.Units[0]; u.Name != "jellyfin" || !u.OK() || u.Sub != "running" || u.Since.IsZero() {
		t.Errorf("jellyfin = %+v", u)
	}
	// A unit the server does not have is reported as such, not dropped:
	// a typo in the watch list would otherwise look like one fewer problem.
	if u := s.Units[3]; u.Name != "no-existe" || u.OK() {
		t.Errorf("missing unit = %+v", u)
	}

	if len(s.Timers) != 4 {
		t.Fatalf("timers = %+v", s.Timers)
	}
	for _, tm := range s.Timers {
		if tm.Next.IsZero() {
			t.Errorf("%s has no next run; it must come from list-timers, not show", tm.Name)
		}
		if tm.Result != "success" {
			t.Errorf("%s result = %q, want the activated service's result", tm.Name, tm.Result)
		}
	}
	if s.Timers[0].Name != "ginebra-posters" {
		t.Errorf("timers not sorted: %s first", s.Timers[0].Name)
	}

	if len(s.Mounts) != 2 || !s.Mounts[0].Mounted || s.Mounts[1].Mounted {
		t.Errorf("mounts = %+v", s.Mounts)
	}

	if s.Smart == nil || len(s.Smart.Disks) != 4 {
		t.Fatalf("smart = %+v", s.Smart)
	}
	var asleep int
	for _, d := range s.Smart.Disks {
		if d.Asleep {
			asleep++
		}
	}
	if asleep != 2 {
		t.Errorf("asleep = %d, want the two USB disks", asleep)
	}
	if len(s.Errors) != 0 {
		t.Errorf("errors = %v", s.Errors)
	}
}

func TestAFailedTimerServiceIsReported(t *testing.T) {
	t.Parallel()
	r := fakeReader(t)
	show, _ := os.ReadFile("testdata/show.txt")
	failed := strings.Replace(string(show),
		"Id=ginebra-respaldo-config.service\nActiveState=inactive", "Id=ginebra-respaldo-config.service\nActiveState=failed", 1)
	failed = strings.Replace(failed, "ActiveState=failed\nSubState=dead\nActiveEnterTimestamp=", "ActiveState=failed\nSubState=failed\nActiveEnterTimestamp=", 1)
	orig := r.run
	r.run = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "show" {
			return []byte(failed), nil
		}
		return orig(ctx, args...)
	}
	for _, tm := range r.Read(t.Context()).Timers {
		if tm.Name == "ginebra-respaldo-config" && tm.Result != "failed" {
			t.Errorf("a backup whose service failed reads as %q", tm.Result)
		}
	}
}

func TestParseUnix(t *testing.T) {
	t.Parallel()
	if got := parseUnix("@1791195293"); !got.Equal(time.Unix(1791195293, 0)) {
		t.Errorf("parseUnix = %v", got)
	}
	for _, v := range []string{"", "n/a", "@0"} {
		if !parseUnix(v).IsZero() {
			t.Errorf("parseUnix(%q) should be never", v)
		}
	}
}

func TestMountUnitsReadAsTheirFolder(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]string{
		"mnt-biblioteca.mount": "biblioteca (montaje)",
		"mnt-disco4.mount":     "disco4 (montaje)",
		"jellyfin.service":     "jellyfin",
	} {
		if got := displayName(id); got != want {
			t.Errorf("displayName(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestReadsTheDriftReport reads a capture of Ginebra's deriva.json, and treats
// a missing file as a server that does not check itself rather than an error.
func TestReadsTheDriftReport(t *testing.T) {
	t.Parallel()
	r := fakeReader(t)
	r.cfg.DriftFile = "testdata/deriva.json"
	s := r.Read(t.Context())
	if s.Drift == nil || len(s.Drift.Differ) != 1 {
		t.Fatalf("drift = %+v", s.Drift)
	}
	if d := s.Drift.Differ[0]; d.File != "/usr/local/bin/ginebra-trampa" || d.Problem != "no está instalado" {
		t.Errorf("difference = %+v", d)
	}
	if s.Drift.Generated.IsZero() || s.Drift.Commit == "" {
		t.Errorf("drift header = %+v", s.Drift)
	}

	r.cfg.DriftFile = "testdata/no-such-file.json"
	s = r.Read(t.Context())
	if s.Drift != nil {
		t.Error("a missing report produced a drift card")
	}
	for _, e := range s.Errors {
		if strings.Contains(e, "deriva") {
			t.Errorf("a missing report was an error: %q", e)
		}
	}
}
