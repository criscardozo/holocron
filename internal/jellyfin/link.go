package jellyfin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/settings"
)

// codeTTL is how long a pending code is considered usable. Jellyfin expires
// them on its own; this keeps a stale one from lingering in memory.
const codeTTL = 10 * time.Minute

// State is where a Quick Connect attempt stands.
type State string

const (
	StateIdle    State = "idle"
	StatePending State = "pending"
	StateLinked  State = "linked"
	StateExpired State = "expired"
)

var (
	// ErrNoLinkInProgress is returned when checking without having started.
	ErrNoLinkInProgress = errors.New("no jellyfin link in progress")
	// ErrNoServerURL means the address has not been set yet. Unlike Plex there
	// is no cloud service to discover the server through, so the URL comes
	// first and the code second.
	ErrNoServerURL = errors.New("set the jellyfin address first")
)

// Status is the snapshot the UI renders while linking.
type Status struct {
	State State
	Code  string
	// User and Admin describe who authorised, once linked. Admin matters:
	// asking Jellyfin to write metadata requires an administrator.
	User  string
	Admin bool
}

// LinkService drives Quick Connect and persists its result. The pending secret
// stays in memory only: it is short-lived and worthless once expired.
type LinkService struct {
	settings *settings.Store

	mu      sync.Mutex
	secret  string
	code    string
	started time.Time
	linked  bool
	user    string
	admin   bool

	// newClient is swapped in tests.
	newClient func(base, token, device string) *Client
}

// NewLinkService creates a LinkService.
func NewLinkService(st *settings.Store) *LinkService {
	return &LinkService{
		settings: st,
		newClient: func(base, token, device string) *Client {
			return New(base, token, device, "")
		},
	}
}

// deviceID returns this install's persisted identifier, creating it on first
// use so Jellyfin keeps recognising the same device.
func (s *LinkService) deviceID(ctx context.Context) (string, error) {
	if id := s.settings.GetDefault(ctx, settings.KeyJellyfinDeviceID, ""); id != "" {
		return id, nil
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate device id: %w", err)
	}
	id := "holocron-" + hex.EncodeToString(b)
	if err := s.settings.Set(ctx, settings.KeyJellyfinDeviceID, id); err != nil {
		return "", err
	}
	return id, nil
}

func (s *LinkService) client(ctx context.Context) (*Client, error) {
	base := s.settings.GetDefault(ctx, settings.KeyJellyfinURL, "")
	if base == "" {
		return nil, ErrNoServerURL
	}
	device, err := s.deviceID(ctx)
	if err != nil {
		return nil, err
	}
	return s.newClient(base, "", device), nil
}

// Start requests a code for the user to approve in Jellyfin.
func (s *LinkService) Start(ctx context.Context) (Status, error) {
	c, err := s.client(ctx)
	if err != nil {
		return Status{}, err
	}
	pending, err := c.InitiateQuickConnect(ctx)
	if err != nil {
		return Status{}, err
	}

	s.mu.Lock()
	s.secret, s.code, s.started = pending.Secret, pending.Code, time.Now()
	s.linked, s.user, s.admin = false, "", false
	s.mu.Unlock()

	return Status{State: StatePending, Code: pending.Code}, nil
}

// Check asks Jellyfin once whether the code was approved, and on success stores
// the token.
func (s *LinkService) Check(ctx context.Context) (Status, error) {
	s.mu.Lock()
	secret, code, started, linked, user, admin := s.secret, s.code, s.started, s.linked, s.user, s.admin
	s.mu.Unlock()

	if linked {
		return Status{State: StateLinked, User: user, Admin: admin}, nil
	}
	if secret == "" {
		return Status{State: StateIdle}, ErrNoLinkInProgress
	}
	if time.Since(started) > codeTTL {
		s.reset()
		return Status{State: StateExpired}, nil
	}

	c, err := s.client(ctx)
	if err != nil {
		return Status{}, err
	}
	approved, err := c.QuickConnectStatus(ctx, secret)
	if err != nil {
		return Status{}, err
	}
	if !approved {
		return Status{State: StatePending, Code: code}, nil
	}

	auth, err := c.RedeemQuickConnect(ctx, secret)
	if err != nil {
		return Status{}, err
	}
	// Persist immediately: abandoning the page after approving must not throw
	// away a successful authorisation.
	for key, value := range map[string]string{
		settings.KeyJellyfinToken:  auth.Token,
		settings.KeyJellyfinUserID: auth.UserID,
		settings.KeyJellyfinUser:   auth.User,
		settings.KeyJellyfinAdmin:  strconv.FormatBool(auth.Admin),
	} {
		if err := s.settings.Set(ctx, key, value); err != nil {
			return Status{}, err
		}
	}

	s.mu.Lock()
	s.linked, s.user, s.admin = true, auth.User, auth.Admin
	s.mu.Unlock()

	return Status{State: StateLinked, User: auth.User, Admin: auth.Admin}, nil
}

// Cancel abandons an in-progress link.
func (s *LinkService) Cancel() { s.reset() }

// Pending reports whether a code is outstanding, so the settings page can show
// it again after a reload.
func (s *LinkService) Pending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.secret != "" && !s.linked && time.Since(s.started) <= codeTTL
}

// Unlink forgets the stored credentials.
func (s *LinkService) Unlink(ctx context.Context) error {
	s.reset()
	for _, key := range []string{
		settings.KeyJellyfinToken, settings.KeyJellyfinUserID,
		settings.KeyJellyfinUser, settings.KeyJellyfinAdmin,
	} {
		if err := s.settings.Set(ctx, key, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *LinkService) reset() {
	s.mu.Lock()
	s.secret, s.code, s.started, s.linked, s.user, s.admin = "", "", time.Time{}, false, "", false
	s.mu.Unlock()
}
