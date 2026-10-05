package hardware

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Core is one logical CPU.
type Core struct {
	Name    string
	BusyPct float64
	MHz     float64
}

// Snapshot is everything the hardware screen shows at one instant.
type Snapshot struct {
	At       time.Time
	CPUPct   float64
	HasCPU   bool
	Cores    []Core
	TempC    float64
	HasTemp  bool
	Load     Load
	HasLoad  bool
	Memory   Memory
	HasMem   bool
	Links    []Link
	Disks    []Disk
	Battery  Battery
	Uptime   time.Duration
	HasUp    bool
	History  History
	Interval time.Duration
}

// History is the recent past of the numbers worth drawing as a line.
type History struct {
	CPU  []float64
	Temp []float64
	Rx   []float64
	Tx   []float64
}

// historyLen at the sampling interval is how far back the lines go: 90
// samples every 2 s is three minutes, enough to see a transcode start.
const historyLen = 90

// Collector turns raw counters into rates. It holds the previous reading,
// because a rate needs two, and the history the lines are drawn from.
type Collector struct {
	mu   sync.Mutex
	prev *counters
	hist History
}

// NewCollector creates a Collector.
func NewCollector() *Collector { return &Collector{} }

// Sample takes a reading. The first call has no previous reading to compare
// with, so its rates are zero and HasCPU is false; every later call reports
// the change since the one before.
func (c *Collector) Sample(now time.Time) Snapshot {
	cur := readCounters(now)

	c.mu.Lock()
	prev := c.prev
	c.prev = &cur
	c.mu.Unlock()

	s := Snapshot{At: now}
	s.Memory, s.HasMem = readMemory()
	s.Load, s.HasLoad = readLoad()
	s.TempC, s.HasTemp = packageTempC()
	s.Uptime, s.HasUp = readUptime()
	s.Battery = readBattery()
	s.Links = readLinks()

	mhz := coreMHz()
	if prev != nil {
		s.Interval = cur.at.Sub(prev.at)
		secs := s.Interval.Seconds()
		if t, ok := busy(prev.cpus["cpu"], cur.cpus["cpu"]); ok {
			s.CPUPct, s.HasCPU = t, true
		}
		for name, ct := range cur.cpus {
			if name == "cpu" {
				continue
			}
			pct, _ := busy(prev.cpus[name], ct)
			s.Cores = append(s.Cores, Core{Name: name, BusyPct: pct, MHz: mhz[name]})
		}
		for i := range s.Links {
			p, okp := prev.nets[s.Links[i].Name]
			n, okn := cur.nets[s.Links[i].Name]
			if okp && okn && secs > 0 && n.rx >= p.rx && n.tx >= p.tx {
				s.Links[i].RxBps = float64(n.rx-p.rx) / secs
				s.Links[i].TxBps = float64(n.tx-p.tx) / secs
			}
		}
		for name, d := range cur.disks {
			p, ok := prev.disks[name]
			if !ok || secs <= 0 || d.readSectors < p.readSectors || d.writeSectors < p.writeSectors {
				continue
			}
			busyPct := float64(d.ioMillis-p.ioMillis) / (secs * 10)
			if busyPct > 100 {
				busyPct = 100
			}
			s.Disks = append(s.Disks, Disk{
				Name:     name,
				Model:    diskModel(name),
				ReadBps:  float64(d.readSectors-p.readSectors) * 512 / secs,
				WriteBps: float64(d.writeSectors-p.writeSectors) * 512 / secs,
				BusyPct:  busyPct,
			})
		}
	} else {
		for name := range cur.cpus {
			if name != "cpu" {
				s.Cores = append(s.Cores, Core{Name: name, MHz: mhz[name]})
			}
		}
		for name := range cur.disks {
			s.Disks = append(s.Disks, Disk{Name: name, Model: diskModel(name)})
		}
	}
	sort.Slice(s.Cores, func(i, j int) bool { return coreIndex(s.Cores[i].Name) < coreIndex(s.Cores[j].Name) })
	sort.Slice(s.Disks, func(i, j int) bool { return s.Disks[i].Name < s.Disks[j].Name })

	c.mu.Lock()
	if s.HasCPU {
		c.hist.CPU = push(c.hist.CPU, s.CPUPct)
	}
	if s.HasTemp {
		c.hist.Temp = push(c.hist.Temp, s.TempC)
	}
	var rx, tx float64
	for _, l := range s.Links {
		if l.Name == "tailscale0" {
			continue // already counted on the physical link it rides
		}
		rx += l.RxBps
		tx += l.TxBps
	}
	if prev != nil {
		c.hist.Rx = push(c.hist.Rx, rx)
		c.hist.Tx = push(c.hist.Tx, tx)
	}
	s.History = History{
		CPU: clone(c.hist.CPU), Temp: clone(c.hist.Temp),
		Rx: clone(c.hist.Rx), Tx: clone(c.hist.Tx),
	}
	c.mu.Unlock()
	return s
}

func busy(a, b cpuTimes) (float64, bool) {
	if b.total <= a.total || b.idle < a.idle {
		return 0, false
	}
	dt := float64(b.total - a.total)
	di := float64(b.idle - a.idle)
	if di > dt {
		di = dt
	}
	return (dt - di) / dt * 100, true
}

func coreIndex(name string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(name, "cpu"))
	if err != nil {
		return 1 << 30
	}
	return n
}

func push(xs []float64, v float64) []float64 {
	xs = append(xs, v)
	if len(xs) > historyLen {
		xs = xs[len(xs)-historyLen:]
	}
	return xs
}

func clone(xs []float64) []float64 { return append([]float64(nil), xs...) }
