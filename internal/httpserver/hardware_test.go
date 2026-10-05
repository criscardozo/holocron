package httpserver

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestHardwareStreamsEventsAndStopsWhenTheClientLeaves drives the real route
// through the real middleware: gzip must not buffer it, the logging wrapper
// must let it flush, and closing the tab must stop the sampler — on Ginebra
// Jellyfin owns the CPU and a screen nobody watches should cost nothing.
func TestHardwareStreamsEventsAndStopsWhenTheClientLeaves(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events/hardware", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip") // the middleware must ignore it here
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("the stream was compressed (%q), so it would arrive in bursts or never", enc)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	gotEvent, gotData := false, false
	deadline := time.Now().Add(3 * time.Second)
	for sc.Scan() && time.Now().Before(deadline) {
		line := sc.Text()
		if line == "event: hardware" {
			gotEvent = true
		}
		if gotEvent && strings.HasPrefix(line, "data: ") {
			gotData = true
		}
		if gotEvent && line == "" {
			break // one whole event
		}
	}
	if !gotEvent || !gotData {
		t.Fatalf("no complete event arrived (event=%v data=%v)", gotEvent, gotData)
	}
	if n := ts.deps.Hardware.Watching(); n != 1 {
		t.Errorf("watching = %d while connected, want 1", n)
	}

	cancel()
	_ = resp.Body.Close()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if ts.deps.Hardware.Watching() == 0 {
			return
		}
	}
	t.Error("the subscriber was not released after the client went away")
}

func TestTheHardwarePageRendersBeforeAnyStream(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	body := ts.get(t, "/hardware", nil).Body
	for _, want := range []string{`sse-connect="/events/hardware"`, `sse-swap="hardware"`, "CPU", "Memoria", "htmx-ext-sse.min.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page is missing %q", want)
		}
	}
}

// TestASSEEventKeepsMultilineFragmentsTogether: every line needs its own
// "data:" prefix, or the first newline in the HTML ends the event early.
func TestASSEEventKeepsMultilineFragmentsTogether(t *testing.T) {
	t.Parallel()
	rec := &bufWriter{h: http.Header{}}
	if err := writeEvent(rec, "x", []byte("<div>\n<span>1</span>\n</div>")); err != nil {
		t.Fatal(err)
	}
	want := "event: x\ndata: <div>\ndata: <span>1</span>\ndata: </div>\n\n"
	if rec.b.String() != want {
		t.Errorf("framed as %q", rec.b.String())
	}
}

type bufWriter struct {
	h http.Header
	b strings.Builder
}

func (w *bufWriter) Header() http.Header         { return w.h }
func (w *bufWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
func (w *bufWriter) WriteHeader(int)             {}
