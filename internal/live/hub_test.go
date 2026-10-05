package live

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestTheLoopRunsOnlyWhileWatched is the house rule: a screen nobody has open
// must not keep sampling.
func TestTheLoopRunsOnlyWhileWatched(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	h := NewHub(5*time.Millisecond, func(context.Context) int { return int(calls.Add(1)) })

	if h.Running() {
		t.Fatal("running before anyone subscribed")
	}
	ch, stop := h.Subscribe()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no reading arrived")
	}
	stop()
	stop() // a handler's defer and a vanished client can both call it

	deadline := time.Now().Add(2 * time.Second)
	for h.Running() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if h.Running() {
		t.Fatal("the loop kept running with no subscribers")
	}
	before := calls.Load()
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != before {
		t.Error("sampling continued after the loop stopped")
	}
}

// TestSubscribersShareOneReading: ten tabs must cost what one does.
func TestSubscribersShareOneReading(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	h := NewHub(20*time.Millisecond, func(context.Context) int { return int(calls.Add(1)) })
	var stops []func()
	var chans []<-chan int
	for i := 0; i < 10; i++ {
		ch, stop := h.Subscribe()
		chans = append(chans, ch)
		stops = append(stops, stop)
	}
	for _, ch := range chans {
		<-ch
	}
	for _, s := range stops {
		s()
	}
	if n := calls.Load(); n > 3 {
		t.Errorf("sampled %d times for ten subscribers to one reading", n)
	}
}
