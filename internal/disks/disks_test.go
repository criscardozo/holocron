package disks

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/settings"
)

// host fakes the kernel's views for one disk: sdc, partition sdc1 (8:33),
// mounted at mount.
type host struct {
	mount, smart string
	fs           *folders.Store
	st           *settings.Store
}

func newHost(t *testing.T, reallocated string, asleep bool) host {
	t.Helper()
	root := t.TempDir()
	oldP, oldS := procRoot, sysRoot
	procRoot, sysRoot = filepath.Join(root, "proc"), filepath.Join(root, "sys")
	t.Cleanup(func() { procRoot, sysRoot = oldP, oldS })

	mount := filepath.Join(root, "mnt", "disco3")
	write(t, filepath.Join(mount, "Peliculas", "x.mkv"), "x")
	write(t, filepath.Join(procRoot, "self", "mountinfo"),
		"29 1 259:2 / / rw shared:1 - ext4 /dev/nvme0n1p2 rw\n"+
			"40 29 8:33 / "+strings.ReplaceAll(mount, " ", `\040`)+" rw shared:2 - ext4 /dev/sdc1 rw\n")
	part := filepath.Join(sysRoot, "devices", "usb", "block", "sdc", "sdc1")
	write(t, filepath.Join(part, "partition"), "1")
	write(t, filepath.Join(sysRoot, "block", "sdc", "device", "model"), "WDC WD30EZRX-00D8PB0  \n")
	if err := os.MkdirAll(filepath.Join(sysRoot, "dev", "block"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(part, filepath.Join(sysRoot, "dev", "block", "8:33")); err != nil {
		t.Fatal(err)
	}
	smart := filepath.Join(root, "smart.json")
	dormido := "false"
	if asleep {
		dormido = "true"
	}
	write(t, smart, `{"generado":"2026-10-12T10:00:00Z","discos":[{"disco":"/dev/sdc","serie":"WD-123","salud":"ok","reasignados":`+reallocated+`,"dormido":`+dormido+`,"leido_en":"2026-10-10T10:00:00Z"}]}`)

	database, err := db.Open(context.Background(), filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return host{mount: mount, smart: smart, fs: folders.NewStore(database), st: settings.NewStore(database)}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Not parallel: the tests swap the package's kernel roots.

func TestADiskIsFoundBehindItsPartition(t *testing.T) {
	h := newHost(t, "217", false)
	w := NewWatcher([]Spec{{Name: "Disco3", Mount: h.mount, Role: "biblioteca", Scan: h.mount}}, h.fs, h.st, h.smart, slog.New(slog.DiscardHandler))
	d := w.Read(t.Context()).Disks[0]
	if !d.Mounted || d.Device != "sdc" || d.Model != "WDC WD30EZRX-00D8PB0" {
		t.Fatalf("disk = %+v", d)
	}
	if d.Smart == nil || *d.Smart.Reallocated != 217 || d.Baseline == nil || *d.Baseline != 217 {
		t.Errorf("smart %+v, baseline %v", d.Smart, d.Baseline)
	}
	if d.ReallocUp() {
		t.Error("the first reading counts as the baseline, not as a rise")
	}

	notMounted := NewWatcher([]Spec{{Name: "Disco9", Mount: filepath.Join(h.mount, "..", "nada"), Role: "biblioteca"}}, h.fs, h.st, h.smart, slog.New(slog.DiscardHandler))
	if notMounted.Read(t.Context()).Disks[0].Mounted {
		t.Error("a path that is no mount point read as mounted")
	}
}

// TestTheBaselineNeedsAFreshReading. A sleeping disk keeps its last values;
// that is not news, and must not become the normal.
func TestTheBaselineNeedsAFreshReading(t *testing.T) {
	h := newHost(t, "217", true)
	w := NewWatcher([]Spec{{Name: "Disco3", Mount: h.mount, Scan: h.mount}}, h.fs, h.st, h.smart, slog.New(slog.DiscardHandler))
	if d := w.Read(t.Context()).Disks[0]; d.Baseline != nil {
		t.Errorf("baseline set from a sleeping disk's old reading: %v", *d.Baseline)
	}
}

func TestMoreReallocatedSectorsAreReported(t *testing.T) {
	h := newHost(t, "217", false)
	w := NewWatcher([]Spec{{Name: "Disco3", Mount: h.mount, Scan: h.mount}}, h.fs, h.st, h.smart, slog.New(slog.DiscardHandler))
	w.Read(t.Context())
	write(t, h.smart, strings.Replace(mustRead(t, h.smart), `"reasignados":217`, `"reasignados":221`, 1))
	if d := w.Read(t.Context()).Disks[0]; !d.ReallocUp() {
		t.Fatalf("221 over a baseline of 217 not reported: %+v", d.Baseline)
	}
	if err := w.AcceptRealloc(t.Context(), "WD-123"); err != nil {
		t.Fatal(err)
	}
	if d := w.Read(t.Context()).Disks[0]; d.ReallocUp() || *d.Baseline != 221 {
		t.Errorf("after accepting: baseline %v", *d.Baseline)
	}
}

// TestALibraryOnAnotherDiskIsReported. The first layout is the normal one;
// a library found later on another disk is a move to look at, until someone
// says it is correct.
func TestALibraryOnAnotherDiskIsReported(t *testing.T) {
	h := newHost(t, "217", false)
	ctx := t.Context()
	if err := h.fs.Manage(ctx, []folders.Spec{{Label: "Películas", Purpose: folders.PurposeMovies, Path: filepath.Join(h.mount, "Peliculas")}}); err != nil {
		t.Fatal(err)
	}
	first := NewWatcher([]Spec{{Name: "Disco3", Mount: h.mount, Scan: h.mount}}, h.fs, h.st, h.smart, slog.New(slog.DiscardHandler))
	rep := first.Read(ctx)
	if len(rep.Moves) != 0 || len(rep.Disks[0].Libraries) != 1 {
		t.Fatalf("first read: %+v", rep)
	}

	// The same mount now answers as Disco4: as if Películas were on it.
	moved := NewWatcher([]Spec{{Name: "Disco4", Mount: h.mount, Scan: h.mount}}, h.fs, h.st, h.smart, slog.New(slog.DiscardHandler))
	rep = moved.Read(ctx)
	if len(rep.Moves) != 1 || rep.Moves[0] != (Move{Library: "Películas", From: "Disco3", To: "Disco4"}) {
		t.Fatalf("moves = %+v", rep.Moves)
	}
	if err := moved.AcceptLayout(ctx); err != nil {
		t.Fatal(err)
	}
	if rep := moved.Read(ctx); len(rep.Moves) != 0 {
		t.Errorf("still reported after accepting: %+v", rep.Moves)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestSmartsModelWinsAndNestedDisksAreFound. The USB bridge cuts sysfs's model
// to 16 characters; SMART has it whole. And a disk bound inside another is
// found from the mount table, without listing a folder that would wake it.
func TestSmartsModelWinsAndNestedDisksAreFound(t *testing.T) {
	h := newHost(t, "217", false)
	write(t, h.smart, strings.Replace(mustRead(t, h.smart), `"serie":"WD-123"`, `"serie":"WD-123","modelo":"WDC WD30EZRX-00D8PB0"`, 1))
	write(t, filepath.Join(sysRoot, "block", "sdc", "device", "model"), "EZRX-00D8PB0\n")
	disco4 := filepath.Join(filepath.Dir(h.mount), "disco4")
	write(t, filepath.Join(disco4, "Series", "x"), "x")
	write(t, filepath.Join(procRoot, "self", "mountinfo"), mustRead(t, filepath.Join(procRoot, "self", "mountinfo"))+
		"41 29 8:17 / "+disco4+" rw shared:3 - ext4 /dev/sdb1 rw\n"+
		"42 41 8:33 /Peliculas "+filepath.Join(disco4, "Peliculas")+" rw shared:2 - ext4 /dev/sdc1 rw\n")

	w := NewWatcher([]Spec{
		{Name: "Disco4", Mount: disco4, Scan: disco4},
		{Name: "Disco3", Mount: h.mount, Scan: h.mount},
	}, h.fs, h.st, h.smart, slog.New(slog.DiscardHandler))
	rep := w.Read(t.Context())
	if rep.Disks[1].Model != "WDC WD30EZRX-00D8PB0" {
		t.Errorf("model = %q, want SMART's whole one", rep.Disks[1].Model)
	}
	if n := rep.Disks[0].Nested; len(n) != 1 || n[0].Name != "Peliculas" || n[0].Disk != "Disco3" {
		t.Errorf("nested in Disco4 = %+v", n)
	}
	if len(rep.Disks[1].Nested) != 0 {
		t.Errorf("Disco3 has nothing inside: %+v", rep.Disks[1].Nested)
	}
}
