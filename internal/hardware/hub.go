package hardware

import (
	"sync"
	"time"
)

// Hub samples while somebody is watching and stops when nobody is.
//
// The house rule on Ginebra is that Jellyfin owns the CPU and everything else
// reads lightly. A sampler running in the background all day for a screen that
// is open a few minutes a week would break that for nothing. So the loop
// starts with the first subscriber, every subscriber shares the same reading
// (ten tabs cost what one does), and the loop exits with the last.
type Hub struct {
	interval  time.Duration
	collector *Collector

	mu      sync.Mutex
	subs    map[chan Snapshot]struct{}
	running bool
	last    Snapshot
	hasLast bool
}

// NewHub creates a Hub that samples every interval while it has subscribers.
func NewHub(interval time.Duration) *Hub {
	return &Hub{interval: interval, collector: NewCollector(), subs: map[chan Snapshot]struct{}{}}
}

// Interval is how often a subscriber can expect a reading.
func (h *Hub) Interval() time.Duration { return h.interval }

// Subscribe returns a channel of readings and a function to stop. The channel
// holds one reading: a slow client gets the latest one rather than a backlog,
// and never holds up anybody else.
func (h *Hub) Subscribe() (<-chan Snapshot, func()) {
	ch := make(chan Snapshot, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if h.hasLast {
		ch <- h.last
	}
	if !h.running {
		h.running = true
		go h.loop()
	}
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
		})
	}
}

// Current is a reading for a page being rendered for the first time. It reuses
// the live loop's last reading when there is a fresh one, so opening the page
// while another tab is watching costs nothing extra.
func (h *Hub) Current() Snapshot {
	h.mu.Lock()
	if h.hasLast && time.Since(h.last.At) < 2*h.interval {
		s := h.last
		h.mu.Unlock()
		return s
	}
	h.mu.Unlock()
	return h.collector.Sample(time.Now())
}

// Watching reports how many subscribers there are, for tests and logs.
func (h *Hub) Watching() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

func (h *Hub) loop() {
	// Prime the rates: the first reading has nothing to compare with.
	h.collector.Sample(time.Now())
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for range t.C {
		h.mu.Lock()
		if len(h.subs) == 0 {
			h.running = false
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()

		s := h.collector.Sample(time.Now())

		h.mu.Lock()
		h.last, h.hasLast = s, true
		for ch := range h.subs {
			select {
			case <-ch: // drop the stale one nobody took
			default:
			}
			ch <- s
		}
		h.mu.Unlock()
	}
}
