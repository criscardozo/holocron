// Package disks describes the server's disks: what each holds, whether it is
// mounted, whether it sleeps, and how its health reads.
//
// On Ginebra the library spans two USB disks joined by bind mounts: Películas
// on Disco3, the rest on Disco4, mounted over one tree. Which library lives on
// which disk is derived, never configured, because folders keep moving; and
// because it is derived, a change in it is worth a word: a bind that failed to
// mount looks exactly like a library that moved, except that the files under
// it are an old copy.
package disks

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/scanner"
	"github.com/cristian/holocron/internal/services"
	"github.com/cristian/holocron/internal/settings"
)

// Roots of the kernel's views, swapped in tests.
var (
	procRoot = "/proc"
	sysRoot  = "/sys"
)

// Spec is one configured disk.
type Spec struct {
	Name, Mount, Role, Scan string
}

// Disk is one disk as it reads now.
type Disk struct {
	Spec
	// Checked says the mount was checked: true for configured disks. Without
	// HOLOCRON_DISKS the disks are the watched folders, which need not be
	// mount points, and "not mounted" would be a false alarm.
	Checked bool
	// FolderID is the watched folder that carries its scans.
	FolderID int64
	Mounted  bool
	// Device is the whole disk ("sdb", "nvme0n1"), to match SMART by.
	Device string
	Model  string
	// Total, Used and Free come from statfs, which does not wake a disk.
	Total, Used, Free uint64
	StatOK            bool
	// Smart is the last SMART reading, nil when there is none. SmartAt is
	// when the server last looked, which is when "asleep" was true: a disk
	// woken since still reads as asleep until the next look.
	Smart   *services.SmartDisk
	SmartAt time.Time
	// Nested are other disks mounted directly inside the folder this one is
	// scanned from: on Ginebra, Disco3's Películas inside Disco4.
	Nested []Nested
	// Baseline is the reallocated-sector count first seen for this disk.
	Baseline *int64
	// Libraries are the media folders on this disk, by label.
	Libraries []string
}

// UsedPercent is how full the disk is the way df says it: used over what
// could hold data (used plus available), rounded up, so a disk is never shown
// emptier than df shows it.
func (d Disk) UsedPercent() int {
	room := d.Used + d.Free
	if !d.StatOK || room == 0 {
		return 0
	}
	return int((d.Used*100 + room - 1) / room)
}

// ReallocUp says the disk has more reallocated sectors than when first seen.
func (d Disk) ReallocUp() bool {
	return d.Smart != nil && d.Smart.Reallocated != nil && d.Baseline != nil && *d.Smart.Reallocated > *d.Baseline
}

// Nested is a folder that is another disk's mount.
type Nested struct {
	Name, Path string
	// Disk is the configured disk it belongs to, "" when it is none of them.
	Disk string
}

// Move is a library found on another disk than the last time.
type Move struct {
	Library, From, To string
}

// Report is everything the Disk page and the start page need.
type Report struct {
	Disks []Disk
	Moves []Move
}

// Watcher reads the disks and remembers what is worth comparing against.
type Watcher struct {
	specs     []Spec
	folders   *folders.Store
	settings  *settings.Store
	smartFile string
	log       *slog.Logger
}

// NewWatcher builds a watcher over the configured disks.
func NewWatcher(specs []Spec, fs *folders.Store, st *settings.Store, smartFile string, log *slog.Logger) *Watcher {
	return &Watcher{specs: specs, folders: fs, settings: st, smartFile: smartFile, log: log}
}

const (
	layoutKey   = "disks.layout"
	baselineKey = "disks.realloc."
)

// Read inspects every disk. Nothing here wakes a sleeping disk: statfs,
// mountinfo, sysfs and the SMART file the server writes are all answered
// without touching the platters.
func (w *Watcher) Read(ctx context.Context) Report {
	mounts := readMounts(filepath.Join(procRoot, "self", "mountinfo"))
	var smart *services.Smart
	if w.smartFile != "" {
		if sm, err := services.ReadSmart(w.smartFile); err == nil {
			smart = sm
		}
	}
	scanFolders := map[string]int64{}
	if list, err := w.folders.List(ctx, folders.PurposeDisk); err == nil {
		for _, f := range list {
			scanFolders[f.Label] = f.ID
		}
	}

	// Without a mount table there is nothing to check against: "not
	// mounted" would be a claim made without looking.
	specs, checked := w.specs, len(mounts) > 0
	if len(specs) == 0 {
		checked = false
		if list, err := w.folders.List(ctx, folders.PurposeDisk); err == nil {
			for _, f := range list {
				specs = append(specs, Spec{Name: f.Label, Mount: f.Path, Scan: f.Path})
			}
		}
	}

	var rep Report
	devOf := map[uint64]int{} // st_dev of a mounted disk → index in rep.Disks
	for _, sp := range specs {
		d := Disk{Spec: sp, FolderID: scanFolders[sp.Name], Checked: checked}
		m, ok := mounts[sp.Mount]
		d.Mounted = ok || !checked
		if ok {
			d.Device = blockDevice(m.majMin)
		}
		// Free is what can be written, as df's «Avail»: on the system disk
		// the 5 % reserved for root is not free to anything Holocron serves,
		// and counting it made the card ten gigabytes and two points off df.
		if total, used, _, avail, err := scanner.FilesystemStat(sp.Mount); err == nil {
			d.Total, d.Used, d.Free, d.StatOK = total, used, avail, true
		}
		if smart != nil && d.Device != "" {
			for i := range smart.Disks {
				if filepath.Base(smart.Disks[i].Disk) == d.Device {
					sd := smart.Disks[i]
					d.Smart, d.SmartAt = &sd, smart.Generated
					d.Model = sd.Model
				}
			}
		}
		// SMART's model is whole; sysfs's is what the USB bridge reports, cut
		// to 16 characters ("EZRX-00D8PB0"). Sysfs only when SMART has none,
		// as for a bridge that passes no SMART at all.
		if d.Model == "" && d.Device != "" {
			d.Model = readTrim(filepath.Join(sysRoot, "block", d.Device, "device", "model"))
		}
		w.baseline(ctx, &d)
		if d.Mounted {
			if dev, ok := statDev(sp.Mount); ok {
				devOf[dev] = len(rep.Disks)
			}
		}
		rep.Disks = append(rep.Disks, d)
	}

	// Other disks mounted right inside each one, from the mount table alone:
	// listing the folder would wake a sleeping disk.
	diskByMajMin := map[string]string{}
	for _, d := range rep.Disks {
		if m, ok := mounts[d.Mount]; ok {
			diskByMajMin[m.majMin] = d.Name
		}
	}
	for i := range rep.Disks {
		d := &rep.Disks[i]
		own := mounts[d.Mount].majMin
		for point, m := range mounts {
			if filepath.Dir(point) != filepath.Clean(d.Scan) || point == d.Scan || m.majMin == own {
				continue
			}
			d.Nested = append(d.Nested, Nested{Name: filepath.Base(point), Path: point, Disk: diskByMajMin[m.majMin]})
		}
		sort.Slice(d.Nested, func(a, b int) bool { return d.Nested[a].Name < d.Nested[b].Name })
	}

	// Which disk holds each library: the one whose filesystem the folder sits
	// on. A bind mount keeps the device of its source, so Películas, bound
	// from Disco3 into Disco4's tree, reads as Disco3.
	now := map[string]string{}
	for _, purpose := range []string{folders.PurposeMovies, folders.PurposeTV} {
		list, err := w.folders.List(ctx, purpose)
		if err != nil {
			continue
		}
		for _, f := range list {
			dev, ok := statDev(f.Path)
			if !ok {
				continue
			}
			if i, ok := devOf[dev]; ok {
				rep.Disks[i].Libraries = append(rep.Disks[i].Libraries, f.Label)
				now[f.Label] = rep.Disks[i].Name
			}
		}
	}
	for i := range rep.Disks {
		sort.Strings(rep.Disks[i].Libraries)
	}
	rep.Moves = w.compareLayout(ctx, now)
	return rep
}

// baseline loads the reallocated-sector count first seen for this disk, and
// records it the first time there is a fresh reading. A disk asleep keeps the
// value read before it slept; that is not a new reading and sets nothing.
func (w *Watcher) baseline(ctx context.Context, d *Disk) {
	sd := d.Smart
	if sd == nil || sd.Serial == "" || sd.Reallocated == nil {
		return
	}
	key := baselineKey + sd.Serial
	if v, err := strconv.ParseInt(w.settings.GetDefault(ctx, key, ""), 10, 64); err == nil {
		d.Baseline = &v
		return
	}
	if sd.Asleep {
		return
	}
	v := *sd.Reallocated
	if err := w.settings.Set(ctx, key, strconv.FormatInt(v, 10)); err == nil {
		d.Baseline = &v
	}
}

// AcceptRealloc takes the disk's current count as its new normal.
func (w *Watcher) AcceptRealloc(ctx context.Context, serial string) error {
	for _, d := range w.Read(ctx).Disks {
		if d.Smart != nil && d.Smart.Serial == serial && d.Smart.Reallocated != nil {
			return w.settings.Set(ctx, baselineKey+serial, strconv.FormatInt(*d.Smart.Reallocated, 10))
		}
	}
	return fmt.Errorf("no disk with serial %q", serial)
}

// compareLayout reports libraries now on another disk than recorded. The first
// layout ever seen is recorded as it is; after that only "es correcto"
// (AcceptLayout) changes the record, so a failed mount keeps being reported
// until someone looks.
func (w *Watcher) compareLayout(ctx context.Context, now map[string]string) []Move {
	raw := w.settings.GetDefault(ctx, layoutKey, "")
	if raw == "" {
		if len(now) > 0 {
			w.recordLayout(ctx, now)
		}
		return nil
	}
	var was map[string]string
	if err := json.Unmarshal([]byte(raw), &was); err != nil {
		w.recordLayout(ctx, now)
		return nil
	}
	var moves []Move
	for lib, disk := range now {
		if prev, ok := was[lib]; ok && prev != disk {
			moves = append(moves, Move{Library: lib, From: prev, To: disk})
		}
	}
	sort.Slice(moves, func(i, j int) bool { return moves[i].Library < moves[j].Library })
	return moves
}

// AcceptLayout records the current layout as the expected one.
func (w *Watcher) AcceptLayout(ctx context.Context) error {
	now := map[string]string{}
	for _, d := range w.Read(ctx).Disks {
		for _, lib := range d.Libraries {
			now[lib] = d.Name
		}
	}
	return w.saveLayout(ctx, now)
}

// recordLayout saves the layout as a side effect of reading. A failure only
// means the next read tries again, so it is logged rather than returned.
func (w *Watcher) recordLayout(ctx context.Context, layout map[string]string) {
	if err := w.saveLayout(ctx, layout); err != nil {
		w.log.Warn("disk layout not recorded", "error", err)
	}
}

func (w *Watcher) saveLayout(ctx context.Context, layout map[string]string) error {
	b, err := json.Marshal(layout)
	if err != nil {
		return err
	}
	return w.settings.Set(ctx, layoutKey, string(b))
}

// mount is one line of mountinfo that matters here.
type mount struct {
	majMin string // "8:17"
}

// readMounts maps mount points to their device. Later lines win, as they do
// for the kernel: a mount stacked over another hides it.
func readMounts(path string) map[string]mount {
	out := map[string]mount{}
	b, err := os.ReadFile(path) //#nosec G304 -- fixed procfs path
	if err != nil {
		return out
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) > 4 {
			out[unescape(f[4])] = mount{majMin: f[2]}
		}
	}
	return out
}

// unescape undoes mountinfo's octal escapes (a space is \040).
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// blockDevice turns "8:17" into the whole disk it is part of, "sdb": the
// partition's sysfs entry sits inside its disk's. The letter is never
// configured, because a USB disk can come back as another one.
func blockDevice(majMin string) string {
	real, err := filepath.EvalSymlinks(filepath.Join(sysRoot, "dev", "block", majMin))
	if err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(real, "partition")); err == nil {
		return filepath.Base(filepath.Dir(real))
	}
	return filepath.Base(real)
}

func readTrim(path string) string {
	b, err := os.ReadFile(path) //#nosec G304 -- sysfs path built from the kernel's own device name
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// statDev is the device a path's filesystem is on.
func statDev(path string) (uint64, bool) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, false
	}
	return uint64(st.Dev), true //nolint:unconvert // Dev is int32 on darwin, uint64 on linux
}

// DevOf exposes statDev to the drill-down, which marks folders that are
// another disk.
func DevOf(path string) (uint64, bool) { return statDev(path) }
