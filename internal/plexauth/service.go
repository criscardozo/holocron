package plexauth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/settings"
)

// pinTTL is how long a started link is considered usable. plex.tv expires PINs
// on its own; this keeps a stale one from lingering in memory forever.
const pinTTL = 10 * time.Minute

// State is where a device-link attempt stands.
type State string

const (
	// StateIdle means no link is in progress.
	StateIdle State = "idle"
	// StatePending means the code was issued and we are waiting for the user
	// to authorise it at plex.tv.
	StatePending State = "pending"
	// StateLinked means the token was obtained and stored.
	StateLinked State = "linked"
	// StateExpired means the code was never authorised in time.
	StateExpired State = "expired"
)

// ErrNoLinkInProgress is returned when checking without having started.
var ErrNoLinkInProgress = errors.New("no plex link in progress")

// Status is the snapshot the UI renders while linking.
type Status struct {
	State   State
	Code    string
	AuthURL string
	// Servers is populated once linked, so the user can pick one instead of
	// typing the server address.
	Servers []Server
}

// Service drives the device-link flow and persists its result. The in-progress
// PIN is deliberately in memory only: it is short-lived and worthless once
// expired, so it does not belong in the database.
type Service struct {
	settings *settings.Store

	mu      sync.Mutex
	pin     PIN
	started time.Time
	linked  bool
	servers []Server

	// newClient is swapped in tests to point at a fake plex.tv.
	newClient func(clientID string) *Client
}

// NewService creates a Service backed by the settings store.
func NewService(st *settings.Store) *Service {
	return &Service{settings: st, newClient: NewClient}
}

// clientID returns this device's persisted identifier, creating it on first
// use so plex.tv keeps recognising the same device.
func (s *Service) clientID(ctx context.Context) (string, error) {
	if id := s.settings.GetDefault(ctx, settings.KeyPlexClientID, ""); id != "" {
		return id, nil
	}
	id, err := NewClientID()
	if err != nil {
		return "", err
	}
	if err := s.settings.Set(ctx, settings.KeyPlexClientID, id); err != nil {
		return "", err
	}
	return id, nil
}

// Start requests a new PIN and returns the code plus the URL to authorise it.
func (s *Service) Start(ctx context.Context) (Status, error) {
	id, err := s.clientID(ctx)
	if err != nil {
		return Status{}, err
	}
	client := s.newClient(id)

	pin, err := client.CreatePIN(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("create plex pin: %w", err)
	}

	s.mu.Lock()
	s.pin, s.started, s.linked, s.servers = pin, time.Now(), false, nil
	s.mu.Unlock()

	return Status{State: StatePending, Code: pin.Code, AuthURL: client.AuthURL(pin)}, nil
}

// Check asks plex.tv once whether the pending PIN was authorised. On success it
// stores the token and discovers the account's servers.
func (s *Service) Check(ctx context.Context) (Status, error) {
	s.mu.Lock()
	pin, started, linked, servers := s.pin, s.started, s.linked, s.servers
	s.mu.Unlock()

	if linked {
		return Status{State: StateLinked, Servers: servers}, nil
	}
	if pin.ID == 0 {
		return Status{State: StateIdle}, ErrNoLinkInProgress
	}

	id, err := s.clientID(ctx)
	if err != nil {
		return Status{}, err
	}
	client := s.newClient(id)

	if time.Since(started) > pinTTL {
		s.reset()
		return Status{State: StateExpired}, nil
	}

	token, err := client.CheckPIN(ctx, pin)
	if err != nil {
		return Status{}, fmt.Errorf("check plex pin: %w", err)
	}
	if token == "" {
		return Status{State: StatePending, Code: pin.Code, AuthURL: client.AuthURL(pin)}, nil
	}

	// Store the token as soon as it exists, so abandoning the server picker
	// does not throw away a successful authorisation.
	if err := s.settings.Set(ctx, settings.KeyPlexToken, token); err != nil {
		return Status{}, err
	}

	// Discovery is a convenience: failing it must not undo the link.
	found, err := client.DiscoverServers(ctx, token)
	if err != nil {
		found = nil
	}

	s.mu.Lock()
	s.linked, s.servers = true, found
	s.mu.Unlock()

	return Status{State: StateLinked, Servers: found}, nil
}

// SelectServer stores the chosen server address and ends the flow.
func (s *Service) SelectServer(ctx context.Context, baseURL string) error {
	if baseURL == "" {
		return errors.New("server address is required")
	}
	if err := s.settings.Set(ctx, settings.KeyPlexURL, baseURL); err != nil {
		return err
	}
	s.reset()
	return nil
}

// Cancel abandons an in-progress link.
func (s *Service) Cancel() { s.reset() }

// Pending reports whether a link is waiting for authorisation, so the settings
// page can render the code again after a reload.
func (s *Service) Pending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pin.ID != 0 && !s.linked && time.Since(s.started) <= pinTTL
}

func (s *Service) reset() {
	s.mu.Lock()
	s.pin, s.started, s.linked, s.servers = PIN{}, time.Time{}, false, nil
	s.mu.Unlock()
}
