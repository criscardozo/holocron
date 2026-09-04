// Package jobs runs long-running work (disk scans, .nfo generation, subtitle
// searches) off the request path. Each job reports progress that the UI polls
// via HTMX. At most one job of a given kind runs at a time so a low-powered
// device is never overwhelmed.
//
// This foundation is intentionally in-memory: job state does not survive a
// restart. When a feature needs durable history it can persist results to the
// jobs table (see internal/db migrations).
package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Status is the lifecycle state of a job.
type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusError   Status = "error"
)

// Job is an immutable snapshot of a job's state. Retrieve fresh copies via
// Manager.Get; do not hold references across time.
type Job struct {
	ID       string
	Kind     string
	Status   Status
	Progress int // 0..100
	Err      string
	// Cause is the error itself, kept alongside its message so a caller can
	// tell one failure from another with errors.Is. Without it the only thing
	// a handler has is a string, and every failure reads the same to the user.
	Cause      error
	Result     string
	StartedAt  time.Time
	FinishedAt time.Time
}

// Progress lets a running job report how far along it is.
type Progress struct {
	job *jobState
}

// Set updates the job's progress, clamped to 0..100.
func (p *Progress) Set(percent int) {
	if p == nil || p.job == nil {
		return
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	p.job.mu.Lock()
	p.job.progress = percent
	p.job.mu.Unlock()
}

// Func is the body of a job. It should honour ctx cancellation and report
// progress. The returned string is stored as the job result.
type Func func(ctx context.Context, p *Progress) (result string, err error)

type jobState struct {
	mu         sync.Mutex
	id         string
	kind       string
	status     Status
	progress   int
	err        string
	cause      error
	result     string
	startedAt  time.Time
	finishedAt time.Time
}

func (s *jobState) snapshot() Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Job{
		ID:         s.id,
		Kind:       s.kind,
		Status:     s.status,
		Progress:   s.progress,
		Err:        s.err,
		Cause:      s.cause,
		Result:     s.result,
		StartedAt:  s.startedAt,
		FinishedAt: s.finishedAt,
	}
}

// ErrKindBusy is returned by Start when a job of the same kind is already
// running.
var ErrKindBusy = fmt.Errorf("a job of this kind is already running")

// maxHistory caps how many finished jobs are kept per kind (besides the most
// recent one, which is always retained). Without a cap the id map would grow
// for the lifetime of the process.
const maxHistory = 20

// Manager tracks jobs and enforces one-per-kind concurrency.
type Manager struct {
	mu         sync.Mutex
	byID       map[string]*jobState
	running    map[string]*jobState // kind -> running job
	lastByKind map[string]string    // kind -> most recent job id
	history    map[string][]string  // kind -> finished job ids, oldest first
	now        func() time.Time
	seq        int

	// baseCtx is the parent of every job context, so Shutdown can cancel all
	// running work. wg tracks running jobs so Shutdown can wait for them.
	baseCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewManager creates an empty job manager.
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		byID:       make(map[string]*jobState),
		running:    make(map[string]*jobState),
		lastByKind: make(map[string]string),
		history:    make(map[string][]string),
		now:        time.Now,
		baseCtx:    ctx,
		cancel:     cancel,
	}
}

// Shutdown cancels every running job and waits for them to return, or until
// ctx is done. Jobs are deliberately detached from the request that started
// them, so this is the only thing that stops them.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.cancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Start launches fn as a job of the given kind. It returns ErrKindBusy if a job
// of that kind is already running. The job runs in its own goroutine; Start
// returns immediately with the initial job snapshot.
func (m *Manager) Start(kind string, fn Func) (Job, error) {
	m.mu.Lock()
	if _, busy := m.running[kind]; busy {
		m.mu.Unlock()
		return Job{}, ErrKindBusy
	}
	m.seq++
	st := &jobState{
		id:        fmt.Sprintf("%s-%d", kind, m.seq),
		kind:      kind,
		status:    StatusRunning,
		startedAt: m.now(),
	}
	m.byID[st.id] = st
	m.running[kind] = st
	m.lastByKind[kind] = st.id
	m.wg.Add(1)
	m.mu.Unlock()

	go m.run(st, fn)

	return st.snapshot(), nil
}

func (m *Manager) run(st *jobState, fn Func) {
	var result string
	var err error

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
		m.finish(st, result, err)
		m.wg.Done()
	}()

	// Jobs outlive the request that started them, so they hang off the
	// manager's context instead: cancelled only by Shutdown.
	result, err = fn(m.baseCtx, &Progress{job: st})
}

// finish records the outcome and frees the kind in a single critical section.
// Doing it in two steps left a window where a job already reported "done" while
// Start still rejected the next one as busy — so pressing the button again the
// moment a scan finished did nothing.
func (m *Manager) finish(st *jobState, result string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	st.mu.Lock()
	st.finishedAt = m.now()
	if err != nil {
		st.status = StatusError
		st.err = err.Error()
		st.cause = err
	} else {
		st.status = StatusDone
		st.progress = 100
		st.result = result
	}
	st.mu.Unlock()

	delete(m.running, st.kind)
	m.retire(st)
}

// retire records a finished job in its kind's history and drops the oldest
// entries beyond maxHistory. The most recent job of a kind is never dropped:
// Latest must keep working. Callers must hold m.mu.
func (m *Manager) retire(st *jobState) {
	hist := append(m.history[st.kind], st.id)
	for len(hist) > maxHistory {
		oldest := hist[0]
		hist = hist[1:]
		if oldest != m.lastByKind[st.kind] {
			delete(m.byID, oldest)
		}
	}
	m.history[st.kind] = hist
}

// Get returns a snapshot of the job with the given id, or false if unknown.
func (m *Manager) Get(id string) (Job, bool) {
	m.mu.Lock()
	st, ok := m.byID[id]
	m.mu.Unlock()
	if !ok {
		return Job{}, false
	}
	return st.snapshot(), true
}

// IsRunning reports whether a job of the given kind is currently running.
func (m *Manager) IsRunning(kind string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.running[kind]
	return ok
}

// Latest returns a snapshot of the most recent job of the given kind (running
// or finished), or false if none has ever run.
func (m *Manager) Latest(kind string) (Job, bool) {
	m.mu.Lock()
	id, ok := m.lastByKind[kind]
	m.mu.Unlock()
	if !ok {
		return Job{}, false
	}
	return m.Get(id)
}
