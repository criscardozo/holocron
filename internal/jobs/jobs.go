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
	ID         string
	Kind       string
	Status     Status
	Progress   int // 0..100
	Err        string
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
		Result:     s.result,
		StartedAt:  s.startedAt,
		FinishedAt: s.finishedAt,
	}
}

// ErrKindBusy is returned by Start when a job of the same kind is already
// running.
var ErrKindBusy = fmt.Errorf("a job of this kind is already running")

// Manager tracks jobs and enforces one-per-kind concurrency.
type Manager struct {
	mu         sync.Mutex
	byID       map[string]*jobState
	running    map[string]*jobState // kind -> running job
	lastByKind map[string]string    // kind -> most recent job id
	now        func() time.Time
	seq        int
}

// NewManager creates an empty job manager.
func NewManager() *Manager {
	return &Manager{
		byID:       make(map[string]*jobState),
		running:    make(map[string]*jobState),
		lastByKind: make(map[string]string),
		now:        time.Now,
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
	m.mu.Unlock()

	go m.run(st, fn)

	return st.snapshot(), nil
}

func (m *Manager) run(st *jobState, fn Func) {
	defer func() {
		if r := recover(); r != nil {
			st.mu.Lock()
			st.status = StatusError
			st.err = fmt.Sprintf("panic: %v", r)
			st.finishedAt = m.now()
			st.mu.Unlock()
		}
		m.mu.Lock()
		delete(m.running, st.kind)
		m.mu.Unlock()
	}()

	result, err := fn(context.Background(), &Progress{job: st})

	st.mu.Lock()
	st.finishedAt = m.now()
	if err != nil {
		st.status = StatusError
		st.err = err.Error()
	} else {
		st.status = StatusDone
		st.progress = 100
		st.result = result
	}
	st.mu.Unlock()
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
