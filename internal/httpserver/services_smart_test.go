package httpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/services"
)

// TestASleepingDiskKeepsItsLastReading: on Ginebra, Servicios said «dormido,
// sin lectura todavía» for the WD while Disco showed its reading from the day
// before. The file keeps that reading; the row shows it, with its age.
func TestASleepingDiskKeepsItsLastReading(t *testing.T) {
	t.Parallel()
	n := func(v int64) *int64 { return &v }
	read := time.Now().Add(-26 * time.Hour)

	d := diskView(services.SmartDisk{
		Disk: "/dev/sdc", Model: "WDC WD30EZRX", Health: "ok", Asleep: true,
		TempC: n(31), Reallocated: n(217), ReadAt: &read,
	})
	if d.Health != "sano" || !strings.HasPrefix(d.Note, "dormido · leído hace") {
		t.Errorf("asleep with a reading: health %q, note %q", d.Health, d.Note)
	}

	never := diskView(services.SmartDisk{Disk: "/dev/sdb", Asleep: true})
	if never.Health != "" || never.Note != "" {
		t.Errorf("asleep and never read: %+v, want the plain «sin lectura» row", never)
	}
}

// TestHealthOnlyIsNotAsleep: Disco4's USB bridge passes only the overall
// verdict. It used to show as asleep with no reading; it is healthy, with its
// attributes unknown.
func TestHealthOnlyIsNotAsleep(t *testing.T) {
	t.Parallel()
	read := time.Now()
	d := diskView(services.SmartDisk{Disk: "/dev/sdb", Model: "Seagate Expansion HDD", Health: "ok", HealthOnly: true, ReadAt: &read})
	if d.Health != "sano" || d.Bad || d.Asleep {
		t.Fatalf("health-only disk: %+v", d)
	}
	if strings.Join(d.Facts, ",") != "el gabinete sólo da el estado general" {
		t.Errorf("facts = %q", d.Facts)
	}
}
