package httpserver

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/hardware"
	"github.com/cristian/holocron/internal/live"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleHardwarePage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.HardwarePage(hardwareView(s.deps.Hardware.Current(r.Context()))))
}

// handleHardwareEvents streams hardware readings. See streamLive.
func (s *Server) handleHardwareEvents(w http.ResponseWriter, r *http.Request) {
	streamLive(s, w, r, s.deps.Hardware, "hardware", func(v hardware.Snapshot) templ.Component {
		return templates.HardwareLive(hardwareView(v))
	}, hardwareAlert)
}

// hardwareAlert is what a screen reader should hear about the machine: only a
// power cut. Everything else on the screen is a reading, not news.
func hardwareAlert(v hardware.Snapshot) string {
	b := v.Battery
	if !b.Discharging() {
		return ""
	}
	msg := fmt.Sprintf("Se cortó la luz: el equipo está andando con la batería, al %d %%.", b.Percent)
	if b.Left > 0 {
		msg += " Quedan unos " + system.HumanDuration(b.Left) + "."
	}
	return msg
}

// streamLive sends a hub's readings as Server-Sent Events, one rendered
// fragment per reading, until the client goes away.
//
// announce, when not nil, names what a screen reader should be told. It goes
// out as its own event, "<event>-alert", and only when its text changes: the
// readings arrive every couple of seconds, and a live region refilled with
// each of them would repeat the same alarm until the tab closed.
//
// The connection outlives the server's 60 s WriteTimeout by design, so the
// deadline is pushed forward before each write instead of being dropped: a
// client that stops reading still gets cut off, just not one that is merely
// watching. Behind Caddy nothing else is needed — reverse_proxy flushes
// text/event-stream as it arrives (measured on Ginebra: 29 events in 60 s).
func streamLive[T any](s *Server, w http.ResponseWriter, r *http.Request, hub *live.Hub[T], event string, render func(T) templ.Component, announce func(T) string) {
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		s.log.Warn("event stream: cannot flush", "event", event, "error", err)
		return
	}

	readings, stop := hub.Subscribe()
	defer stop()

	ctx := r.Context()
	var buf bytes.Buffer
	var said string
	for {
		select {
		case <-ctx.Done():
			return
		case v := <-readings:
			buf.Reset()
			if err := render(v).Render(ctx, &buf); err != nil {
				s.log.Warn("event stream: render", "event", event, "error", err)
				return
			}
			_ = rc.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if err := writeEvent(w, event, buf.Bytes()); err != nil {
				return // the client went away
			}
			if announce != nil {
				if a := announce(v); a != said {
					said = a
					if err := writeEvent(w, event+"-alert", []byte(html.EscapeString(a))); err != nil {
						return
					}
				}
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

// writeEvent frames data as one SSE event. Every line of the payload needs its
// own "data:" prefix, or a fragment with a newline in it ends the event early.
func writeEvent(w http.ResponseWriter, name string, data []byte) error {
	var b strings.Builder
	b.WriteString("event: ")
	b.WriteString(name)
	b.WriteByte('\n')
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		b.WriteString("data: ")
		b.Write(sc.Bytes())
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	_, err := w.Write([]byte(b.String()))
	return err
}

// hardwareView formats a reading for the screen.
func hardwareView(s hardware.Snapshot) templates.HardwareView {
	v := templates.HardwareView{Live: s.HasCPU}
	if s.HasCPU {
		v.CPU = pct(s.CPUPct)
	}
	v.CPUSpark = spark(s.History.CPU, 100, func(m float64) string { return "100 %" })
	for _, c := range s.Cores {
		hc := templates.HWCore{
			Name:  strings.TrimPrefix(c.Name, "cpu"),
			Busy:  pct(c.BusyPct),
			Width: width(c.BusyPct),
			Hot:   c.BusyPct >= 85,
		}
		if c.MHz > 0 {
			hc.MHz = strings.Replace(fmt.Sprintf("%.1f GHz", c.MHz/1000), ".", ",", 1)
		}
		v.Cores = append(v.Cores, hc)
	}
	if s.HasTemp {
		v.Temp = fmt.Sprintf("%.0f °C", s.TempC)
		v.TempHot = s.TempC >= 85
	}
	if s.HasLoad {
		v.Load = decimal(s.Load.One) + " · " + decimal(s.Load.Five) + " · " + decimal(s.Load.Fifteen)
	}
	if s.HasUp {
		v.Uptime = system.HumanDuration(s.Uptime)
	}

	if s.HasMem {
		v.RAM = meter(s.Memory.Used(), s.Memory.Total)
		if s.Memory.SwapTotal > 0 {
			v.HasSwap = true
			v.Swap = meter(s.Memory.SwapUsed, s.Memory.SwapTotal)
		}
		if s.Memory.ZramStored > 0 {
			ratio := float64(s.Memory.ZramData) / float64(s.Memory.ZramStored)
			v.Zram = system.HumanBytes(s.Memory.ZramData) + " guardados en " +
				system.HumanBytes(s.Memory.ZramStored) + " (×" + decimal(ratio) + ")"
		}
	}

	for _, l := range s.Links {
		hl := templates.HWLink{Name: l.Name, Up: l.Up, Rx: rate(l.RxBps), Tx: rate(l.TxBps)}
		switch {
		case l.SpeedMbps >= 1000 && l.SpeedMbps%1000 == 0:
			hl.Speed = strconv.Itoa(l.SpeedMbps/1000) + " Gb/s"
		case l.SpeedMbps > 0:
			hl.Speed = strconv.Itoa(l.SpeedMbps) + " Mb/s"
		}
		v.Links = append(v.Links, hl)
	}
	v.NetSpk = spark(s.History.Rx, 0, rate)

	for _, d := range s.Disks {
		v.Disks = append(v.Disks, templates.HWDisk{
			Name:  d.Name,
			Model: d.Model,
			Read:  rate(d.ReadBps),
			Write: rate(d.WriteBps),
			Busy:  pct(d.BusyPct),
			Width: width(d.BusyPct),
			Idle:  d.ReadBps == 0 && d.WriteBps == 0 && d.BusyPct < 0.5,
		})
	}

	b := s.Battery
	if b.Present {
		v.Battery = templates.HWBattery{
			Present:     true,
			Percent:     strconv.Itoa(b.Percent) + " %",
			Width:       width(float64(b.Percent)),
			Status:      batteryStatus(b),
			Discharging: b.Discharging(),
			Low:         b.Percent <= 20,
		}
		if b.HealthPct > 0 {
			v.Battery.Health = fmt.Sprintf("%.0f %%", b.HealthPct)
		}
		if b.Left > 0 {
			v.Battery.Left = system.HumanDuration(b.Left)
		}
	}
	return v
}

func batteryStatus(b hardware.Battery) string {
	if b.Discharging() {
		return "con la batería (sin luz)"
	}
	switch b.Status {
	case "Charging":
		return "cargando"
	case "Full":
		return "llena"
	case "Not charging":
		return "enchufada, sin cargar"
	case "Discharging":
		return "descargándose"
	default:
		return "enchufada"
	}
}

func pct(p float64) string { return strconv.Itoa(int(math.Round(p))) + " %" }

func decimal(f float64) string { return strings.Replace(fmt.Sprintf("%.2f", f), ".", ",", 1) }

// width is a bar length on a 0..100 SVG canvas.
func width(p float64) string {
	switch {
	case p < 0:
		p = 0
	case p > 100:
		p = 100
	}
	return strconv.FormatFloat(p, 'f', 1, 64)
}

func meter(used, total uint64) templates.HWMeter {
	p := 0.0
	if total > 0 {
		p = float64(used) * 100 / float64(total)
	}
	return templates.HWMeter{
		Used: system.HumanBytes(used), Total: system.HumanBytes(total),
		Pct: pct(p), Width: width(p), High: p >= 90,
	}
}

// rate formats bytes per second.
func rate(bps float64) string {
	if bps < 1 {
		return "0 B/s"
	}
	return system.HumanBytes(uint64(bps)) + "/s"
}

// spark lays values out as an SVG polyline on a 100×24 canvas, newest on the
// right. fixedMax pins the top of the scale (CPU is always out of 100); zero
// scales to the largest value seen, which is what a traffic line needs.
func spark(xs []float64, fixedMax float64, label func(float64) string) templates.Spark {
	if len(xs) < 2 {
		return templates.Spark{}
	}
	maxV := fixedMax
	if maxV <= 0 {
		for _, x := range xs {
			if x > maxV {
				maxV = x
			}
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	var b strings.Builder
	step := 100.0 / float64(len(xs)-1)
	for i, x := range xs {
		y := 23 - math.Min(x/maxV, 1)*22
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.FormatFloat(float64(i)*step, 'f', 1, 64))
		b.WriteByte(',')
		b.WriteString(strconv.FormatFloat(y, 'f', 1, 64))
	}
	return templates.Spark{Points: b.String(), Max: label(maxV)}
}
