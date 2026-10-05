// Package live runs a sampler only while somebody is watching its results.
//
// Every live screen in Holocron works this way. On Ginebra Jellyfin owns the
// CPU and everything else reads lightly, so a sampler for a screen that is open
// a few minutes a week must cost nothing the rest of the time. The loop starts
// with the first subscriber, every subscriber shares the same reading (ten tabs
// cost what one does), and the loop ends with the last.
//
// Measured on Ginebra with the hardware screen: 0.00 % of a core with no tab
// open, 0.40 % with one.
package live

import (
	"context"
	"sync"
	"time"
)

// Hub samples every interval while it has subscribers.
type Hub[T any] struct {
	interval time.Duration
	sample   func(context.Context) T
	// freshFor is how long the last reading still counts as current for a
	// page being rendered for the first time.
	freshFor time.Duration

	mu      sync.Mutex
	subs    map[chan T]struct{}
	running bool
	last    T
	lastAt  time.Time
	hasLast bool
}

// NewHub creates a Hub. sample is called from the hub's own goroutine, never
// concurrently with itself, and should bound its own work with the context.
func NewHub[T any](interval time.Duration, sample func(context.Context) T) *Hub[T] {
	return &Hub[T]{
		interval: interval, sample: sample, freshFor: 2 * interval,
		subs: map[chan T]struct{}{},
	}
}

// Subscribe returns a channel of readings and a function to stop. The channel
// holds one reading: a slow client gets the latest rather than a backlog, and
// never holds up anyone else. Calling stop more than once is harmless.
func (h *Hub[T]) Subscribe() (<-chan T, func()) {
	ch := make(chan T, 1)
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
// the live loop's last reading when it is fresh, so opening the page while
// another tab watches costs nothing extra.
func (h *Hub[T]) Current(ctx context.Context) T {
	h.mu.Lock()
	if h.hasLast && time.Since(h.lastAt) < h.freshFor {
		v := h.last
		h.mu.Unlock()
		return v
	}
	h.mu.Unlock()
	return h.sample(ctx)
}

// Watching reports how many subscribers there are.
func (h *Hub[T]) Watching() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Running reports whether the sampling loop is alive.
func (h *Hub[T]) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running
}

func (h *Hub[T]) loop() {
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		h.mu.Lock()
		if len(h.subs) == 0 {
			h.running = false
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), h.interval*4)
		v := h.sample(ctx)
		cancel()

		h.mu.Lock()
		h.last, h.lastAt, h.hasLast = v, time.Now(), true
		for ch := range h.subs {
			select {
			case <-ch: // drop the stale reading nobody took
			default:
			}
			ch <- v
		}
		h.mu.Unlock()

		<-t.C
	}
}
