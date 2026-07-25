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

func TestShutdownCancelsRunningJobs(t *testing.T) {
	t.Parallel()
	m := NewManager()

	started := make(chan struct{})
	job, err := m.Start("long", func(ctx context.Context, _ *Progress) (string, error) {
		close(started)
		<-ctx.Done() // only Shutdown can cancel a detached job
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	got, ok := m.Get(job.ID)
	if !ok {
		t.Fatal("job not found after shutdown")
	}
	if got.Status != StatusError {
		t.Errorf("status = %q, want %q (cancelled)", got.Status, StatusError)
	}
}

func TestShutdownTimesOutOnStuckJob(t *testing.T) {
	t.Parallel()
	m := NewManager()

	release := make(chan struct{})
	defer close(release)
	if _, err := m.Start("stuck", func(_ context.Context, _ *Progress) (string, error) {
		<-release // ignores cancellation on purpose
		return "", nil
	}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := m.Shutdown(ctx); err == nil {
		t.Error("expected Shutdown to report a timeout for a stuck job")
	}
}

func TestFinishedJobsArePrunedButLatestSurvives(t *testing.T) {
	t.Parallel()
	m := NewManager()

	var ids []string
	for i := 0; i < maxHistory+5; i++ {
		job, err := m.Start("churn", func(_ context.Context, _ *Progress) (string, error) {
			return "ok", nil
		})
		if err != nil {
			t.Fatalf("Start %d returned error: %v", i, err)
		}
		waitFor(t, m, job.ID, StatusDone)
		ids = append(ids, job.ID)
	}

	if _, ok := m.Get(ids[0]); ok {
		t.Error("oldest job should have been pruned from the id map")
	}
	latest, ok := m.Latest("churn")
	if !ok {
		t.Fatal("Latest lost the most recent job")
	}
	if latest.ID != ids[len(ids)-1] {
		t.Errorf("Latest = %q, want %q", latest.ID, ids[len(ids)-1])
	}
}
