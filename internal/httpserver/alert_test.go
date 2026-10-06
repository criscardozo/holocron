package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/hardware"
	"github.com/cristian/holocron/internal/live"
	"github.com/cristian/holocron/internal/services"
)

// TestAnAlarmIsSaidOnce. Readings arrive every couple of seconds; a screen
// reader told the same alarm with each would repeat it until the tab closed.
// The alert event goes out when its text changes, and only then.
func TestAnAlarmIsSaidOnce(t *testing.T) {
	t.Parallel()
	hub := live.NewHub(5*time.Millisecond, func(context.Context) int { return 1 })
	s := &Server{log: slog.New(slog.DiscardHandler)}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/events/x", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	streamLive(s, rec, req, hub, "x", func(int) templ.Component {
		return templ.Raw("<p>lectura</p>")
	}, func(int) string { return "Se cortó la luz & algo más" })

	body := rec.Body.String()
	if n := strings.Count(body, "event: x\n"); n < 3 {
		t.Fatalf("only %d readings in the window; the test proves nothing", n)
	}
	if n := strings.Count(body, "event: x-alert\n"); n != 1 {
		t.Errorf("the alarm went out %d times, want once", n)
	}
	if !strings.Contains(body, "data: Se cortó la luz &amp; algo más") {
		t.Errorf("the alert text was not escaped as HTML:\n%s", body)
	}
}

func TestWhatTheScreenReaderHears(t *testing.T) {
	t.Parallel()
	if got := hardwareAlert(hardware.Snapshot{Battery: hardware.Battery{Present: true, OnAC: true, Percent: 100}}); got != "" {
		t.Errorf("on mains: %q, want nothing", got)
	}
	cut := hardwareAlert(hardware.Snapshot{Battery: hardware.Battery{Present: true, Percent: 98, Left: 3*time.Hour + 14*time.Minute}})
	if !strings.Contains(cut, "Se cortó la luz") || !strings.Contains(cut, "98 %") || !strings.Contains(cut, "Quedan unos") {
		t.Errorf("power cut: %q", cut)
	}

	sn := services.Snapshot{Units: []services.Unit{
		{Name: "jellyfin", Active: "active", Sub: "running"},
		{Name: "sonarr", Active: "failed", Sub: "failed"},
	}}
	if got := servicesAlert(sn); got != "1 servicio caído: sonarr." {
		t.Errorf("services: %q", got)
	}
}

// TestWatchersAreLogged. Measuring what the machine costs with nobody looking
// needs a way to know nobody is: the stream logs its subscriber count when
// someone arrives and again when they leave.
func TestWatchersAreLogged(t *testing.T) {
	t.Parallel()
	var logs strings.Builder
	s := &Server{log: slog.New(slog.NewTextHandler(&logs, nil))}
	hub := live.NewHub(5*time.Millisecond, func(context.Context) int { return 1 })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/events/x", nil).WithContext(ctx)

	streamLive(s, httptest.NewRecorder(), req, hub, "x", func(int) templ.Component {
		return templ.Raw("<p>lectura</p>")
	}, nil)

	got := logs.String()
	if !strings.Contains(got, `msg="live subscribers" stream=x count=1`) ||
		!strings.Contains(got, `msg="live subscribers" stream=x count=0`) {
		t.Errorf("logs = %q, want count=1 on arrival and count=0 on leaving", got)
	}
}
