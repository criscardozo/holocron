package widgets

import (
	"testing"

	"github.com/cristian/holocron/internal/disks"
)

// TestTheDiskTileShowsEveryLibraryDisk: on Ginebra the library is on two
// disks, and the tile showed only the fuller one.
func TestTheDiskTileShowsEveryLibraryDisk(t *testing.T) {
	t.Parallel()
	const tb = uint64(1) << 40
	rep := disks.Report{Disks: []disks.Disk{
		{Spec: disks.Spec{Name: "Disco4", Role: "biblioteca"}, Mounted: true, StatOK: true, Total: 4 * tb, Used: 3 * tb, Free: 1 * tb},
		{Spec: disks.Spec{Name: "Disco3", Role: "biblioteca"}, Mounted: true, StatOK: true, Total: 3 * tb, Used: 1 * tb, Free: 2 * tb},
		{Spec: disks.Spec{Name: "Sistema", Role: "descargas"}, Mounted: true, StatOK: true, Total: tb / 4, Used: tb / 8, Free: tb / 8},
	}}
	rows := libraryRows(rep)
	if len(rows) != 2 || rows[0].Label != "Disco4" || rows[0].Value != "75 %" || rows[1].Label != "Disco3" || rows[1].Value != "33 %" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Sub != "1.0 TiB libres" {
		t.Errorf("free = %q", rows[0].Sub)
	}
	if one := libraryRows(disks.Report{Disks: rep.Disks[:1]}); one != nil {
		t.Errorf("one disk keeps the plain value, got rows %+v", one)
	}
}
