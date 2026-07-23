package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func waitFor(t *testing.T, m *Manager, id string, want Status) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := m.Get(id); ok && j.Status == want {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach status %q in time", id, want)
	return Job{}
}

func TestManagerRunsToCompletion(t *testing.T) {
	t.Parallel()
	m := NewManager()

	job, err := m.Start("demo", func(_ context.Context, p *Progress) (string, error) {
		p.Set(50)
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	done := waitFor(t, m, job.ID, StatusDone)
	if done.Result != "ok" {
		t.Errorf("result = %q, want %q", done.Result, "ok")
	}
	if done.Progress != 100 {
		t.Errorf("progress = %d, want 100", done.Progress)
	}
}

func TestManagerCapturesError(t *testing.T) {
	t.Parallel()
	m := NewManager()

	job, err := m.Start("failer", func(_ context.Context, _ *Progress) (string, error) {
		return "", errors.New("boom")
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	got := waitFor(t, m, job.ID, StatusError)
	if got.Err != "boom" {
		t.Errorf("err = %q, want %q", got.Err, "boom")
	}
}

func TestManagerRejectsConcurrentSameKind(t *testing.T) {
	t.Parallel()
	m := NewManager()

	release := make(chan struct{})
	first, err := m.Start("busy", func(_ context.Context, _ *Progress) (string, error) {
		<-release
		return "", nil
	})
	if err != nil {
		t.Fatalf("first Start returned error: %v", err)
	}

	if _, err := m.Start("busy", func(_ context.Context, _ *Progress) (string, error) {
		return "", nil
	}); !errors.Is(err, ErrKindBusy) {
		t.Errorf("second Start error = %v, want ErrKindBusy", err)
	}

	close(release)
	waitFor(t, m, first.ID, StatusDone)
}
