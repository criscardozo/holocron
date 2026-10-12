package httpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/disks"
	"github.com/cristian/holocron/internal/services"
)

// TestASleepingDiskSaysWhenItWasRead. A sleeping disk keeps its last SMART
// values; shown without their age they would read as current.
func TestASleepingDiskSaysWhenItWasRead(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	read := now.Add(-50 * time.Hour)
	d := disks.Disk{
		Spec: disks.Spec{Name: "Disco3", Role: "biblioteca"}, Checked: true, Mounted: true, Device: "sdc",
		Smart:    &services.SmartDisk{Health: "ok", Reallocated: i64(217), TempC: i64(28), Asleep: true, ReadAt: &read},
		Baseline: i64(217),
	}
	c := diskCard(d, false, now)
	if c.Health != "sano" || !c.Asleep || !strings.Contains(c.Realloc, "217 reasignados · leído hace 2 días") || c.TempAge == "" {
		t.Errorf("card = %+v", c)
	}
	if c.ReallocUp {
		t.Error("an unchanged count reported as a rise")
	}
}

// TestNoSmartIsNotAFailure: Disco4's USB bridge passes no SMART commands.
func TestNoSmartIsNotAFailure(t *testing.T) {
	t.Parallel()
	c := diskCard(disks.Disk{Spec: disks.Spec{Name: "Disco4"}, Checked: true, Mounted: true, Device: "sdb"}, false, time.Now())
	if c.Health != "sin datos de salud" || c.HealthTone != "none" {
		t.Errorf("card = %+v", c)
	}
}

// TestTheFilesystemsOwnUseIsNotHidden: ext4's journal and tables show as a
// few GB nobody accounts for; only more than 1 % or 20 GB is worth a row.
func TestTheFilesystemsOwnUseIsNotHidden(t *testing.T) {
	t.Parallel()
	const tb = uint64(1) << 40
	if got := hiddenFloor(3 * tb); got != 3*tb/100 {
		t.Errorf("3 TB disk: floor %d, want 1 %%", got)
	}
	if got := hiddenFloor(1 * tb); got != 20<<30 {
		t.Errorf("1 TB disk: floor %d, want 20 GB", got)
	}
}

func TestTheDiskPageShowsACardPerDisk(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	if _, err := ts.db.Exec(`INSERT INTO watched_folders (label, path, purpose) VALUES ('Disco4', ?, 'disk')`, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	body := ts.get(t, "/disk", nil).Body
	for _, want := range []string{`class="card disk-card disk-card-active"`, "Disco4", "libres de", "Escanear (despierta el disco)"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page is missing %q", want)
		}
	}
	// Without HOLOCRON_DISKS the folders need not be mount points: no
	// "no montado" for them.
	if strings.Contains(body, "no montado") {
		t.Error("a watched folder was reported as not mounted")
	}
}
