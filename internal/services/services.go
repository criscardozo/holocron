// Package services reports the state of the machine's systemd units and
// timers, and reads the status files the server writes for things Holocron
// may not do itself, such as SMART.
//
// One `systemctl show` for every unit at once, not one per unit: it is a
// shell-out, and the screen refreshes on a timer. No privileges are needed to
// read unit state over D-Bus, and that holds inside Holocron's hardened unit
// (checked on Ginebra with systemd-run and the same sandbox).
//
// The list of what to watch is configuration, not code. On Ginebra it is the
// same list ginebra-vigia uses, so the screen and the alerting cannot disagree
// about what should be running.
package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Unit is one systemd service's state.
type Unit struct {
	Name   string
	Active string // active | inactive | failed | activating | …
	Sub    string // running | dead | exited | …
	Since  time.Time
	Result string // success | exit-code | signal | …
}

// OK reports whether the unit is where it should be.
func (u Unit) OK() bool { return u.Active == "active" }

// Timer is one systemd timer.
type Timer struct {
	Name   string
	Last   time.Time
	Next   time.Time
	Result string // of the service it last ran
}

// Mount is a path that should be a mount point.
type Mount struct {
	Path    string
	Mounted bool
}

// Snapshot is one reading of everything.
type Snapshot struct {
	At     time.Time
	Units  []Unit
	Timers []Timer
	Mounts []Mount
	Smart  *Smart
	// Errors says what could not be read, in words.
	Errors []string
}

// Config is what to watch.
type Config struct {
	Units       []string // service names, with or without ".service"
	TimerPrefix string   // e.g. "ginebra-": every timer whose name starts with it
	Mounts      []string
	SmartFile   string
}

// Reader reads a Snapshot.
type Reader struct {
	cfg Config
	// run executes systemctl. A field so tests can answer without systemd.
	run func(ctx context.Context, args ...string) ([]byte, error)
	// mountinfo is where mount points are read from.
	mountinfo string
}

// NewReader creates a Reader for cfg.
func NewReader(cfg Config) *Reader {
	return &Reader{cfg: cfg, run: runSystemctl, mountinfo: "/proc/self/mountinfo"}
}

// Configured reports whether there is anything to watch.
func (r *Reader) Configured() bool {
	return len(r.cfg.Units) > 0 || r.cfg.TimerPrefix != "" || len(r.cfg.Mounts) > 0 || r.cfg.SmartFile != ""
}

func runSystemctl(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// A fixed binary and arguments as a slice: unit names come from the
	// server's configuration, and even so nothing goes through a shell.
	out, err := exec.CommandContext(ctx, "systemctl", args...).Output() //#nosec G204 -- fixed binary, configured unit names, no shell
	return out, err
}

// Read takes one reading: one systemctl call for the timers and one for every
// unit's state.
//
// Both shapes were measured on Ginebra (systemd 257) before this was written,
// and they decided it. `show` leaves NextElapseUSecRealtime empty and prints
// LastTriggerUSec in local wall time even with --timestamp=unix, so timers come
// from `list-timers -o json`, which gives both as microseconds since the epoch.
// Unit state comes from `show --timestamp=unix`, which does turn
// ActiveEnterTimestamp into "@seconds".
func (r *Reader) Read(ctx context.Context) Snapshot {
	s := Snapshot{At: time.Now()}

	timers, err := r.timers(ctx)
	if err != nil {
		s.Errors = append(s.Errors, "no se pudo listar los timers")
	}

	var names []string
	for _, u := range r.cfg.Units {
		names = append(names, serviceID(u))
	}
	// The service a timer runs is where its outcome is: a timer that fired on
	// time and a backup that failed both look like "the timer is fine".
	for _, t := range timers {
		if t.activates != "" {
			names = append(names, t.activates)
		}
	}

	state := map[string]map[string]string{}
	if len(names) > 0 {
		out, err := r.run(ctx, append([]string{
			"show", "--no-pager", "--timestamp=unix",
			"-p", "Id,ActiveState,SubState,ActiveEnterTimestamp,Result",
			"--",
		}, names...)...)
		if err != nil && len(out) == 0 {
			s.Errors = append(s.Errors, "systemctl no respondió")
		}
		for _, blk := range parseShow(out) {
			state[blk["Id"]] = blk
		}
	}

	for _, u := range r.cfg.Units {
		id := serviceID(u)
		name := strings.TrimSuffix(id, ".service")
		blk, ok := state[id]
		if !ok {
			s.Units = append(s.Units, Unit{Name: name, Active: "desconocido"})
			continue
		}
		s.Units = append(s.Units, Unit{
			Name: name, Active: blk["ActiveState"], Sub: blk["SubState"],
			Since: parseUnix(blk["ActiveEnterTimestamp"]), Result: blk["Result"],
		})
	}
	for _, t := range timers {
		tm := Timer{Name: strings.TrimSuffix(t.unit, ".timer"), Last: t.last, Next: t.next}
		if blk, ok := state[t.activates]; ok {
			tm.Result = blk["Result"]
			if blk["ActiveState"] == "failed" {
				tm.Result = "failed"
			}
		}
		s.Timers = append(s.Timers, tm)
	}

	mounted, err := mountPoints(r.mountinfo)
	for _, m := range r.cfg.Mounts {
		s.Mounts = append(s.Mounts, Mount{Path: m, Mounted: err == nil && mounted[filepath.Clean(m)]})
	}

	if r.cfg.SmartFile != "" {
		smart, err := ReadSmart(r.cfg.SmartFile)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// Not written yet, or not this machine: no card rather than an error.
		case err != nil:
			s.Errors = append(s.Errors, "no se pudo leer el estado SMART")
		default:
			s.Smart = smart
		}
	}
	sort.Slice(s.Timers, func(i, j int) bool { return s.Timers[i].Name < s.Timers[j].Name })
	return s
}

func serviceID(u string) string {
	if strings.Contains(u, ".") {
		return u
	}
	return u + ".service"
}

type timerRow struct {
	unit, activates string
	next, last      time.Time
}

func (r *Reader) timers(ctx context.Context) ([]timerRow, error) {
	if r.cfg.TimerPrefix == "" {
		return nil, nil
	}
	out, err := r.run(ctx, "list-timers", "--all", "--no-pager", "-o", "json", r.cfg.TimerPrefix+"*")
	if err != nil && len(out) == 0 {
		return nil, err
	}
	var rows []struct {
		Next      int64  `json:"next"`
		Last      int64  `json:"last"`
		Unit      string `json:"unit"`
		Activates string `json:"activates"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &rows); err != nil {
		return nil, fmt.Errorf("decode list-timers: %w", err)
	}
	res := make([]timerRow, 0, len(rows))
	for _, x := range rows {
		res = append(res, timerRow{unit: x.Unit, activates: x.Activates, next: usec(x.Next), last: usec(x.Last)})
	}
	return res, nil
}

func usec(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.UnixMicro(v)
}

// parseUnix reads "@1791195293", what --timestamp=unix prints. Empty or "n/a"
// means never.
func parseUnix(v string) time.Time {
	n, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(v), "@"), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

// parseShow splits `systemctl show` output for several units: key=value lines,
// one blank line between units.
func parseShow(out []byte) []map[string]string {
	var blocks []map[string]string
	cur := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if len(cur) > 0 {
				blocks = append(blocks, cur)
				cur = map[string]string{}
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			cur[k] = v
		}
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	return blocks
}

// mountPoints reads /proc/self/mountinfo. No exec: findmnt would be a second
// shell-out for something the kernel already lists.
func mountPoints(path string) (map[string]bool, error) {
	b, err := os.ReadFile(path) //#nosec G304 -- fixed procfs path
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) > 4 {
			out[unescapeMount(f[4])] = true
		}
	}
	return out, nil
}

// unescapeMount undoes mountinfo's octal escapes (a space is \040).
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Smart is what the server's SMART timer wrote. The file is the server's
// contract (ginebra-smart.timer, every 6 h, with -n standby so it never wakes a
// sleeping disk); Holocron only reads it, and reading SMART itself would need
// root and would spin the disk up.
type Smart struct {
	Generated time.Time   `json:"generado"`
	Disks     []SmartDisk `json:"discos"`
}

// SmartDisk is one disk's last reading. Asleep means it was in standby when
// the timer ran, so there is no reading yet — not an error.
type SmartDisk struct {
	Disk          string     `json:"disco"`
	Model         string     `json:"modelo"`
	Serial        string     `json:"serie"`
	Capacity      int64      `json:"capacidad_bytes"`
	Health        string     `json:"salud"` // ok | falla | desconocida
	Reallocated   *int64     `json:"reasignados"`
	Pending       *int64     `json:"pendientes"`
	Uncorrectable *int64     `json:"incorregibles"`
	CRC           *int64     `json:"crc"`
	WearPct       *int64     `json:"desgaste_pct"`
	TempC         *int64     `json:"temperatura"`
	Hours         *int64     `json:"horas"`
	ReadAt        *time.Time `json:"leido_en"`
	Asleep        bool       `json:"dormido"`
}

// ReadSmart reads the SMART status file.
func ReadSmart(path string) (*Smart, error) {
	b, err := os.ReadFile(path) //#nosec G304 -- path from server configuration
	if err != nil {
		return nil, err
	}
	var s Smart
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &s, nil
}
