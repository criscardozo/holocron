// Package system reads Raspberry Pi health metrics from the Linux /proc and
// /sys pseudo-filesystems. Every read is best-effort: on a non-Linux host (for
// example a developer's Mac) the files are absent and the corresponding fields
// stay zero, which the UI renders as "—". All reads are cheap file reads, so
// the package compiles and runs on any platform.
package system

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Stats is a point-in-time snapshot of host health.
type Stats struct {
	CPUPercent float64       // busy CPU across all cores, 0..100
	HasCPU     bool          // whether CPUPercent could be read
	MemTotal   uint64        // total RAM in bytes
	MemUsed    uint64        // used RAM in bytes
	MemPercent float64       // used RAM, 0..100
	TempC      float64       // SoC temperature in Celsius
	HasTemp    bool          // whether TempC could be read
	Uptime     time.Duration // host uptime
	HasUptime  bool          // whether Uptime could be read
	Load1      float64       // 1-minute load average
	HasLoad    bool          // whether Load1 could be read
}

// Read collects a health snapshot. Failures in individual metrics are silently
// left as zero values with their Has* flag unset; Read never returns an error.
func Read() Stats {
	var s Stats

	if pct, err := cpuPercent(200 * time.Millisecond); err == nil {
		s.CPUPercent, s.HasCPU = pct, true
	}
	if total, used, err := memory(); err == nil {
		s.MemTotal, s.MemUsed = total, used
		if total > 0 {
			s.MemPercent = float64(used) / float64(total) * 100
		}
	}
	if c, err := tempC(); err == nil {
		s.TempC, s.HasTemp = c, true
	}
	if up, err := uptime(); err == nil {
		s.Uptime, s.HasUptime = up, true
	}
	if l, err := load1(); err == nil {
		s.Load1, s.HasLoad = l, true
	}

	return s
}

// cpuPercent samples /proc/stat twice, sleeping between samples, and returns the
// busy percentage over the interval.
func cpuPercent(interval time.Duration) (float64, error) {
	idle1, total1, err := cpuSample()
	if err != nil {
		return 0, err
	}
	time.Sleep(interval)
	idle2, total2, err := cpuSample()
	if err != nil {
		return 0, err
	}
	dt := float64(total2 - total1)
	di := float64(idle2 - idle1)
	if dt <= 0 {
		return 0, nil
	}
	return (1 - di/dt) * 100, nil
}

// cpuSample parses the aggregate "cpu" line of /proc/stat, returning idle and
// total jiffies. idle includes iowait.
func cpuSample() (idle, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, fmt.Errorf("open /proc/stat: %w", err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, fmt.Errorf("empty /proc/stat")
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("unexpected /proc/stat format")
	}
	for i, f := range fields[1:] {
		v, convErr := strconv.ParseUint(f, 10, 64)
		if convErr != nil {
			continue
		}
		total += v
		if i == 3 || i == 4 { // idle, iowait
			idle += v
		}
	}
	return idle, total, nil
}

// memory reads MemTotal and MemAvailable from /proc/meminfo (kB) and returns
// total and used bytes.
func memory() (total, used uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer func() { _ = f.Close() }()

	var memTotal, memAvailable uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, convErr := strconv.ParseUint(fields[1], 10, 64)
		if convErr != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotal = v * 1024
		case "MemAvailable:":
			memAvailable = v * 1024
		}
	}
	if memTotal == 0 {
		return 0, 0, fmt.Errorf("MemTotal not found")
	}
	if memAvailable > memTotal {
		memAvailable = memTotal
	}
	return memTotal, memTotal - memAvailable, nil
}

// tempC reads the SoC temperature from the first thermal zone (milli-Celsius).
func tempC() (float64, error) {
	b, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, fmt.Errorf("read thermal zone: %w", err)
	}
	milli, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse temp: %w", err)
	}
	return milli / 1000, nil
}

// uptime reads the first field of /proc/uptime (seconds).
func uptime() (time.Duration, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("read /proc/uptime: %w", err)
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty /proc/uptime")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse uptime: %w", err)
	}
	return time.Duration(secs) * time.Second, nil
}

// load1 reads the 1-minute load average from /proc/loadavg.
func load1() (float64, error) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("read /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty /proc/loadavg")
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse loadavg: %w", err)
	}
	return v, nil
}
