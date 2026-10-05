package hardware

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeHost writes a /proc and /sys shaped like Ginebra's: two cores, an
// ethernet link, an NVMe and a USB disk with a partition, zram, and a battery.
type fakeHost struct{ proc, sys string }

func newFakeHost(t *testing.T) fakeHost {
	t.Helper()
	root := t.TempDir()
	h := fakeHost{proc: filepath.Join(root, "proc"), sys: filepath.Join(root, "sys")}
	files := map[string]string{
		"sys/class/net/enp1s0/operstate":                       "up\n",
		"sys/class/net/enp1s0/speed":                           "1000\n",
		"sys/class/net/lo/operstate":                           "unknown\n",
		"sys/class/net/lo/statistics/rx_bytes":                 "999\n",
		"sys/class/net/lo/statistics/tx_bytes":                 "999\n",
		"sys/block/nvme0n1/device/model":                       "WDC PC SN530\n",
		"sys/block/sda/device/model":                           "EZRX-00D8PB0\n",
		"sys/block/sda/device/vendor":                          "WDC WD30\n",
		"sys/block/zram0/mm_stat":                              "4000000 1000000 1200000 0 0 0 0 0\n",
		"sys/block/zram0/disksize":                             "4294967296\n",
		"sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq": "3900000\n",
		"sys/devices/system/cpu/cpu1/cpufreq/scaling_cur_freq": "800000\n",
		"sys/class/hwmon/hwmon2/name":                          "coretemp\n",
		"sys/class/hwmon/hwmon2/temp1_label":                   "Package id 0\n",
		"sys/class/hwmon/hwmon2/temp1_input":                   "51000\n",
		"sys/class/thermal/thermal_zone0/type":                 "acpitz\n",
		"sys/class/thermal/thermal_zone0/temp":                 "27800\n",
		"sys/class/power_supply/BAT1/type":                     "Battery\n",
		"sys/class/power_supply/BAT1/capacity":                 "80\n",
		"sys/class/power_supply/BAT1/status":                   "Charging\n",
		"sys/class/power_supply/BAT1/charge_full":              "2106000\n",
		"sys/class/power_supply/BAT1/charge_full_design":       "3220000\n",
		"sys/class/power_supply/BAT1/charge_now":               "1684800\n",
		"sys/class/power_supply/BAT1/current_now":              "842400\n",
		"sys/class/power_supply/ACAD/type":                     "Mains\n",
		"sys/class/power_supply/ACAD/online":                   "1\n",
		"proc/meminfo":                                         "MemTotal: 20328448 kB\nMemFree: 1000 kB\nMemAvailable: 17051648 kB\nSwapTotal: 8388608 kB\nSwapFree: 8388608 kB\n",
		"proc/loadavg":                                         "0.27 0.32 0.26 1/300 1234\n",
		"proc/uptime":                                          "9480.5 70000.0\n",
	}
	for rel, body := range files {
		writeFile(t, filepath.Join(root, rel), body)
	}
	h.tick(t, 0)
	return h
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tick advances every counter as if n intervals of activity happened: each
// core 50 % busy, 1 MB in and 0.5 MB out on the link, 2048 sectors read from
// the NVMe with 500 ms busy, and nothing on the sleeping USB disk.
func (h fakeHost) tick(t *testing.T, n uint64) {
	t.Helper()
	busy, idle := 100+50*n, 100+50*n
	stat := "cpu  " + u(2*busy) + " 0 0 " + u(2*idle) + " 0 0 0 0 0 0\n" +
		"cpu0 " + u(busy) + " 0 0 " + u(idle) + " 0 0 0 0 0 0\n" +
		"cpu1 " + u(busy) + " 0 0 " + u(idle) + " 0 0 0 0 0 0\n"
	writeFile(t, filepath.Join(h.proc, "stat"), stat)
	writeFile(t, filepath.Join(h.sys, "class/net/enp1s0/statistics/rx_bytes"), u(1_000_000*n)+"\n")
	writeFile(t, filepath.Join(h.sys, "class/net/enp1s0/statistics/tx_bytes"), u(500_000*n)+"\n")
	disk := "259 0 nvme0n1 0 0 " + u(2048*n) + " 0 0 0 0 0 0 " + u(500*n) + " 0\n" +
		"8 0 sda 0 0 0 0 0 0 0 0 0 0 0\n" +
		"8 1 sda1 0 0 0 0 0 0 0 0 0 0 0\n" +
		"252 0 zram0 0 0 99 0 0 0 99 0 0 0 0\n"
	writeFile(t, filepath.Join(h.proc, "diskstats"), disk)
}

func u(v uint64) string { return strconv.FormatUint(v, 10) }

// Not parallel: these tests swap the package-level roots.
func TestSampleReportsRatesBetweenTwoReadings(t *testing.T) {
	h := newFakeHost(t)
	oldP, oldS := procRoot, sysRoot
	procRoot, sysRoot = h.proc, h.sys
	t.Cleanup(func() { procRoot, sysRoot = oldP, oldS })

	c := NewCollector()
	t0 := time.Unix(1_000_000, 0)
	first := c.Sample(t0)
	if first.HasCPU {
		t.Error("the first reading has nothing to compare with and must not claim a CPU figure")
	}

	h.tick(t, 1)
	s := c.Sample(t0.Add(time.Second))

	if !s.HasCPU || s.CPUPct != 50 {
		t.Errorf("CPU = %v (%v), want 50", s.CPUPct, s.HasCPU)
	}
	if len(s.Cores) != 2 || s.Cores[0].Name != "cpu0" || s.Cores[0].BusyPct != 50 || s.Cores[0].MHz != 3900 {
		t.Errorf("cores = %+v", s.Cores)
	}
	if !s.HasTemp || s.TempC != 51 {
		t.Errorf("temp = %v, want the coretemp package, not acpitz", s.TempC)
	}

	if len(s.Links) != 1 || s.Links[0].Name != "enp1s0" {
		t.Fatalf("links = %+v; loopback must not count as the network", s.Links)
	}
	if l := s.Links[0]; l.RxBps != 1_000_000 || l.TxBps != 500_000 || l.SpeedMbps != 1000 {
		t.Errorf("link = %+v", l)
	}

	// Whole disks only: sda1 would double-count sda, and zram is memory.
	var names []string
	for _, d := range s.Disks {
		names = append(names, d.Name)
	}
	if strings.Join(names, ",") != "nvme0n1,sda" {
		t.Fatalf("disks = %v", names)
	}
	if d := s.Disks[0]; d.ReadBps != 2048*512 || d.BusyPct != 50 || d.Model != "WDC PC SN530" {
		t.Errorf("nvme = %+v", d)
	}
	if d := s.Disks[1]; d.ReadBps != 0 || d.BusyPct != 0 {
		t.Errorf("an idle disk must read as idle: %+v", d)
	}
	// A USB bridge splits the model across vendor and model; alone, the
	// second half is a string nobody recognises.
	if got := s.Disks[1].Model; got != "WDC WD30EZRX-00D8PB0" {
		t.Errorf("USB disk model = %q", got)
	}

	if s.Memory.Total != 20328448*1024 || s.Memory.ZramData != 4000000 || s.Memory.ZramStored != 1200000 {
		t.Errorf("memory = %+v", s.Memory)
	}
	b := s.Battery
	if !b.Present || b.Percent != 80 || !b.OnAC || b.Discharging() {
		t.Errorf("battery = %+v", b)
	}
	if b.HealthPct < 65 || b.HealthPct > 66 {
		t.Errorf("health = %.1f, want about 65 (2106 of 3220 mAh)", b.HealthPct)
	}
	if len(s.History.CPU) != 1 || len(s.History.Rx) != 1 {
		t.Errorf("history = %+v", s.History)
	}
}

// TestPowerCutIsReported is the UPS case: mains gone, running on the battery,
// with an estimate of how long is left.
func TestPowerCutIsReported(t *testing.T) {
	h := newFakeHost(t)
	oldP, oldS := procRoot, sysRoot
	procRoot, sysRoot = h.proc, h.sys
	t.Cleanup(func() { procRoot, sysRoot = oldP, oldS })

	writeFile(t, filepath.Join(h.sys, "class/power_supply/ACAD/online"), "0\n")
	writeFile(t, filepath.Join(h.sys, "class/power_supply/BAT1/status"), "Discharging\n")

	b := readBattery()
	if !b.Discharging() {
		t.Fatalf("battery = %+v, want it to say the power is out", b)
	}
	// charge_now 1684800 µAh at 842400 µA is two hours.
	if b.Left != 2*time.Hour {
		t.Errorf("time left = %s, want 2h", b.Left)
	}
}
