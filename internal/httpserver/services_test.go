package httpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/services"
)

func i64(v int64) *int64 { return &v }

// TestASleepingDiskIsNotAnError: the SMART timer reads with -n standby so it
// never wakes the SMR disk, and a disk that was asleep simply has no reading.
// Shown as an error, it would be a permanent red mark for doing the right
// thing.
func TestASleepingDiskIsNotAnError(t *testing.T) {
	t.Parallel()
	v := servicesView(services.Snapshot{Smart: &services.Smart{
		Generated: time.Now().Add(-3 * time.Hour),
		Disks: []services.SmartDisk{
			{Disk: "/dev/sdb", Asleep: true},
			{Disk: "/dev/nvme0n1", Model: "KINGSTON", Health: "ok", TempC: i64(34), WearPct: i64(7)},
			{Disk: "/dev/sdc", Model: "WD 3 TB", Health: "ok", Reallocated: i64(217)},
		},
	}}, true, time.Now())
	if len(v.Disks) != 3 {
		t.Fatalf("disks = %+v", v.Disks)
	}
	if d := v.Disks[0]; !d.Asleep || d.Bad {
		t.Errorf("asleep disk = %+v", d)
	}
	if d := v.Disks[1]; d.Bad || d.Health != "sano" {
		t.Errorf("healthy disk = %+v", d)
	}
	// The old WD with 217 reallocated sectors: SMART still says "ok", and that
	// is exactly why the count has to be said separately.
	if d := v.Disks[2]; !d.Bad || len(d.Warn) != 1 || !strings.Contains(d.Warn[0], "217") {
		t.Errorf("worn disk = %+v", d)
	}
}

// TestAFinishedOneshotIsNotDown: a timer's service is inactive with
// Result=success between runs, which is a job done.
func TestAFinishedOneshotIsNotDown(t *testing.T) {
	t.Parallel()
	now := time.Now()
	v := servicesView(services.Snapshot{
		Units: []services.Unit{
			{Name: "jellyfin", Active: "active", Sub: "running"},
			{Name: "mnt-biblioteca", Active: "active", Sub: "mounted"},
			{Name: "bazarr", Active: "failed", Result: "exit-code"},
		},
		Timers: []services.Timer{
			{Name: "ginebra-respaldo-config", Last: now.Add(-time.Hour), Next: now.Add(5 * 24 * time.Hour), Result: "success"},
			{Name: "ginebra-posters", Result: "exit-code"},
		},
	}, true, now)
	if v.Down != 1 || v.Units[1].State != "montado" || v.Units[2].State != "falló" {
		t.Errorf("units = %+v (down %d)", v.Units, v.Down)
	}
	if v.Timers[0].Failed || v.Timers[0].Result != "bien" {
		t.Errorf("a successful oneshot reads as %+v", v.Timers[0])
	}
	if !v.Timers[1].Failed {
		t.Errorf("a failed job must say so: %+v", v.Timers[1])
	}
}

func TestTheServicesPageExplainsWhenNothingIsConfigured(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	body := ts.get(t, "/services", nil).Body
	if !strings.Contains(body, "HOLOCRON_WATCH_UNITS") || !strings.Contains(body, `sse-connect="/events/services"`) {
		t.Errorf("services page = %q", body)
	}
}
