// Package hardware samples the machine Holocron runs on, in more detail than
// the dashboard's summary: every core, the memory tiers, the network link, the
// disks' throughput and the battery that doubles as a UPS.
//
// Everything comes from /proc and /sys, readable without privileges. Two
// things are deliberately absent. SMART needs root and would wake a sleeping
// SMR disk, so a timer on the server writes it to a file instead (see
// internal/ginebra). And GPU utilisation needs CAP_PERFMON; whether Jellyfin
// is transcoding in hardware is answered by Jellyfin's own sessions, which is
// the question that actually matters.
//
// Nothing here touches a disk: /proc/diskstats is kernel bookkeeping, so
// reading it does not spin up a drive that has gone to sleep.
package hardware

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Roots are where /proc and /sys are read from. Variables so a test can point
// them at a fake tree.
var (
	procRoot = "/proc"
	sysRoot  = "/sys"
)

// counters is one raw reading. Rates are the difference between two of them.
type counters struct {
	at    time.Time
	cpus  map[string]cpuTimes // "cpu" is the total, "cpu0".. the cores
	nets  map[string]netCount
	disks map[string]diskCount
}

type cpuTimes struct{ idle, total uint64 }

type netCount struct{ rx, tx uint64 }

type diskCount struct {
	readSectors, writeSectors uint64
	ioMillis                  uint64
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path) //#nosec G304 -- fixed /proc and /sys paths under the package roots
}

func readTrim(path string) (string, bool) {
	b, err := readFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

func readUint(path string) (uint64, bool) {
	s, ok := readTrim(path)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 10, 64)
	return v, err == nil
}

func readInt(path string) (int64, bool) {
	s, ok := readTrim(path)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

// readCounters takes one raw reading of everything that is a rate.
func readCounters(now time.Time) counters {
	return counters{
		at:    now,
		cpus:  readCPUTimes(),
		nets:  readNetCounts(),
		disks: readDiskCounts(),
	}
}

func readCPUTimes() map[string]cpuTimes {
	b, err := readFile(filepath.Join(procRoot, "stat"))
	if err != nil {
		return nil
	}
	out := map[string]cpuTimes{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		var total uint64
		var vals []uint64
		for _, x := range f[1:] {
			v, err := strconv.ParseUint(x, 10, 64)
			if err != nil {
				break
			}
			vals = append(vals, v)
		}
		if len(vals) < 4 {
			continue
		}
		// guest and guest_nice are already counted inside user and nice.
		for i, v := range vals {
			if i >= 8 {
				break
			}
			total += v
		}
		idle := vals[3]
		if len(vals) > 4 {
			idle += vals[4] // iowait is idle time too
		}
		out[f[0]] = cpuTimes{idle: idle, total: total}
	}
	return out
}

// ignoredIface names interfaces whose traffic is not "the network": loopback,
// and the virtual ones a container runtime or a VPN adds alongside the real
// link. Tailscale is kept — it is how the house is reached from outside.
func ignoredIface(name string) bool {
	return name == "lo" || strings.HasPrefix(name, "veth") ||
		strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-")
}

func readNetCounts() map[string]netCount {
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "class/net/*"))
	out := map[string]netCount{}
	for _, d := range dirs {
		name := filepath.Base(d)
		if ignoredIface(name) {
			continue
		}
		rx, ok1 := readUint(filepath.Join(d, "statistics/rx_bytes"))
		tx, ok2 := readUint(filepath.Join(d, "statistics/tx_bytes"))
		if ok1 && ok2 {
			out[name] = netCount{rx: rx, tx: tx}
		}
	}
	return out
}

// wholeDisk reports whether a /proc/diskstats name is a physical disk rather
// than a partition or a virtual device. Partitions would double-count, and
// zram is memory, not storage.
func wholeDisk(name string) bool {
	switch {
	case strings.HasPrefix(name, "loop"), strings.HasPrefix(name, "ram"),
		strings.HasPrefix(name, "zram"), strings.HasPrefix(name, "dm-"),
		strings.HasPrefix(name, "sr"):
		return false
	}
	_, err := os.Stat(filepath.Join(sysRoot, "block", name))
	return err == nil
}

func readDiskCounts() map[string]diskCount {
	b, err := readFile(filepath.Join(procRoot, "diskstats"))
	if err != nil {
		return nil
	}
	out := map[string]diskCount{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 14 || !wholeDisk(f[2]) {
			continue
		}
		rs, _ := strconv.ParseUint(f[5], 10, 64)
		ws, _ := strconv.ParseUint(f[9], 10, 64)
		io, _ := strconv.ParseUint(f[12], 10, 64)
		out[f[2]] = diskCount{readSectors: rs, writeSectors: ws, ioMillis: io}
	}
	return out
}

// Memory is RAM plus the two places Linux puts what does not fit.
type Memory struct {
	Total, Available uint64
	SwapTotal        uint64
	SwapUsed         uint64
	// Zram is compressed RAM used as swap: Data is what was stored, Stored is
	// what it costs after compression. Zero when there is no zram device.
	ZramData, ZramStored uint64
	ZramSize             uint64
}

// Used is RAM in use by programs, not counting what the kernel would hand
// back on demand.
func (m Memory) Used() uint64 {
	if m.Available > m.Total {
		return 0
	}
	return m.Total - m.Available
}

func readMemory() (Memory, bool) {
	b, err := readFile(filepath.Join(procRoot, "meminfo"))
	if err != nil {
		return Memory{}, false
	}
	kv := map[string]uint64{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		v, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		kv[k] = v * 1024
	}
	m := Memory{
		Total:     kv["MemTotal"],
		Available: kv["MemAvailable"],
		SwapTotal: kv["SwapTotal"],
	}
	if kv["SwapTotal"] >= kv["SwapFree"] {
		m.SwapUsed = kv["SwapTotal"] - kv["SwapFree"]
	}
	// mm_stat: orig_data_size compr_data_size mem_used_total ...
	if s, ok := readTrim(filepath.Join(sysRoot, "block/zram0/mm_stat")); ok {
		f := strings.Fields(s)
		if len(f) >= 3 {
			m.ZramData, _ = strconv.ParseUint(f[0], 10, 64)
			m.ZramStored, _ = strconv.ParseUint(f[2], 10, 64)
		}
		m.ZramSize, _ = readUint(filepath.Join(sysRoot, "block/zram0/disksize"))
	}
	return m, m.Total > 0
}

// Load is the run-queue average over 1, 5 and 15 minutes.
type Load struct{ One, Five, Fifteen float64 }

func readLoad() (Load, bool) {
	s, ok := readTrim(filepath.Join(procRoot, "loadavg"))
	if !ok {
		return Load{}, false
	}
	f := strings.Fields(s)
	if len(f) < 3 {
		return Load{}, false
	}
	var l Load
	var err1, err2, err3 error
	l.One, err1 = strconv.ParseFloat(f[0], 64)
	l.Five, err2 = strconv.ParseFloat(f[1], 64)
	l.Fifteen, err3 = strconv.ParseFloat(f[2], 64)
	return l, err1 == nil && err2 == nil && err3 == nil
}

func readUptime() (time.Duration, bool) {
	s, ok := readTrim(filepath.Join(procRoot, "uptime"))
	if !ok {
		return 0, false
	}
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	sec, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(sec * float64(time.Second)), true
}

// coreMHz reads each core's current frequency, keyed like /proc/stat.
func coreMHz() map[string]float64 {
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "devices/system/cpu/cpu[0-9]*"))
	out := map[string]float64{}
	for _, d := range dirs {
		if khz, ok := readUint(filepath.Join(d, "cpufreq/scaling_cur_freq")); ok {
			out[filepath.Base(d)] = float64(khz) / 1000
		}
	}
	return out
}

// packageTempC is the CPU package temperature: hwmon coretemp's "Package id",
// then a thermal zone of type x86_pkg_temp, then zone 0 as on the Pi. Same
// order as internal/system, for the same reason: on a laptop zone 0 is often
// the motherboard, which reads well below what throttles.
func packageTempC() (float64, bool) {
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "class/hwmon/hwmon*"))
	for _, d := range dirs {
		if name, _ := readTrim(filepath.Join(d, "name")); name != "coretemp" {
			continue
		}
		labels, _ := filepath.Glob(filepath.Join(d, "temp*_label"))
		sort.Strings(labels)
		for _, l := range labels {
			if lab, _ := readTrim(l); strings.HasPrefix(lab, "Package id") {
				if v, ok := readInt(strings.TrimSuffix(l, "_label") + "_input"); ok {
					return float64(v) / 1000, true
				}
			}
		}
	}
	zones, _ := filepath.Glob(filepath.Join(sysRoot, "class/thermal/thermal_zone*"))
	sort.Strings(zones)
	for _, z := range zones {
		if t, _ := readTrim(filepath.Join(z, "type")); t == "x86_pkg_temp" {
			if v, ok := readInt(filepath.Join(z, "temp")); ok {
				return float64(v) / 1000, true
			}
		}
	}
	if v, ok := readInt(filepath.Join(sysRoot, "class/thermal/thermal_zone0/temp")); ok {
		return float64(v) / 1000, true
	}
	return 0, false
}

// Link is one network interface's state.
type Link struct {
	Name      string
	Up        bool
	SpeedMbps int // 0 when the driver does not say (Wi-Fi, a VPN)
	RxBps     float64
	TxBps     float64
}

func readLinks() []Link {
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "class/net/*"))
	var out []Link
	for _, d := range dirs {
		name := filepath.Base(d)
		if ignoredIface(name) {
			continue
		}
		state, _ := readTrim(filepath.Join(d, "operstate"))
		l := Link{Name: name, Up: state == "up" || state == "unknown"}
		// speed reads as -1, or errors, on a link that is down or virtual.
		if v, ok := readInt(filepath.Join(d, "speed")); ok && v > 0 {
			l.SpeedMbps = int(v)
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Disk is one physical disk's activity.
type Disk struct {
	Name     string
	Model    string
	ReadBps  float64
	WriteBps float64
	// BusyPct is the share of the interval the disk spent doing I/O.
	BusyPct float64
}

// diskModel is the disk's model as the kernel knows it. USB-to-SATA bridges
// split it: /sys says vendor "WDC WD30" and model "EZRX-00D8PB0", and the model
// alone is a string nobody recognises. Vendors that are only the bridge's
// generic "ATA" are left off.
func diskModel(name string) string {
	m, _ := readTrim(filepath.Join(sysRoot, "block", name, "device/model"))
	v, _ := readTrim(filepath.Join(sysRoot, "block", name, "device/vendor"))
	if v == "" || v == "ATA" || strings.HasPrefix(m, v) {
		return m
	}
	return v + m
}

// Battery is the laptop's battery, which on Ginebra is the UPS.
type Battery struct {
	Present bool
	// Percent is the charge left, 0..100.
	Percent int
	// Status is the kernel's word: Charging, Discharging, Full, Not charging.
	Status string
	// OnAC is whether mains power is connected. False with Present true means
	// the power is out and the machine is running on the battery.
	OnAC bool
	// HealthPct is the full capacity today against the design capacity.
	HealthPct float64
	// Left is the estimated time to empty while discharging; zero otherwise
	// or when the hardware does not report a current draw.
	Left time.Duration
}

// Discharging reports whether the machine is running on its battery.
func (b Battery) Discharging() bool { return b.Present && !b.OnAC }

// ReadBattery reads the battery on its own, for places that only need to know
// whether the power is out — the dashboard, every page load — without paying
// for a full sample.
func ReadBattery() Battery { return readBattery() }

func readBattery() Battery {
	var b Battery
	bats, _ := filepath.Glob(filepath.Join(sysRoot, "class/power_supply/BAT*"))
	sort.Strings(bats)
	if len(bats) > 0 {
		d := bats[0]
		b.Present = true
		if v, ok := readInt(filepath.Join(d, "capacity")); ok {
			b.Percent = int(v)
		}
		b.Status, _ = readTrim(filepath.Join(d, "status"))

		// Some batteries report charge (µAh), others energy (µWh); health
		// and time left work the same way with either pair.
		full, okFull := readUint(filepath.Join(d, "charge_full"))
		design, okDesign := readUint(filepath.Join(d, "charge_full_design"))
		now, okNow := readUint(filepath.Join(d, "charge_now"))
		rate, okRate := readUint(filepath.Join(d, "current_now"))
		if !okFull {
			full, okFull = readUint(filepath.Join(d, "energy_full"))
			design, okDesign = readUint(filepath.Join(d, "energy_full_design"))
			now, okNow = readUint(filepath.Join(d, "energy_now"))
			rate, okRate = readUint(filepath.Join(d, "power_now"))
		}
		if okFull && okDesign && design > 0 {
			b.HealthPct = float64(full) * 100 / float64(design)
		}
		if b.Status == "Discharging" && okNow && okRate && rate > 0 {
			b.Left = time.Duration(float64(now) / float64(rate) * float64(time.Hour))
		}
	}
	// The mains adapter is its own supply. Absent means a desktop, which is
	// always on mains.
	b.OnAC = true
	acs, _ := filepath.Glob(filepath.Join(sysRoot, "class/power_supply/*"))
	for _, d := range acs {
		if t, _ := readTrim(filepath.Join(d, "type")); t != "Mains" {
			continue
		}
		if v, ok := readInt(filepath.Join(d, "online")); ok {
			b.OnAC = v == 1
		}
	}
	return b
}
