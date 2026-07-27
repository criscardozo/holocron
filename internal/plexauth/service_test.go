package plexauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/settings"
)

func newTestService(t *testing.T, f *fakePlexTV) (*Service, *settings.Store) {
	t.Helper()
	database, err := db.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := settings.NewStore(database)
	svc := NewService(store)
	base := f.server(t).URL + "/"
	svc.newClient = func(clientID string) *Client { return newClientWithBase(clientID, base) }
	return svc, store
}

func TestLinkFlowStoresTokenAndDiscoversServers(t *testing.T) {
	t.Parallel()
	f := &fakePlexTV{token: "plex-token", resources: `[
		{"name":"Pi","provides":"server","connections":[
			{"uri":"http://192.168.1.20:32400","local":true,"relay":false}
		]}
	]`}
	svc, store := newTestService(t, f)
	ctx := context.Background()

	start, err := svc.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if start.State != StatePending || start.Code != "ABCD" {
		t.Fatalf("unexpected start: %+v", start)
	}
	if !svc.Pending() {
		t.Error("Pending should be true right after Start")
	}

	// Not authorised yet.
	status, err := svc.Check(ctx)
	if err != nil {
		t.Fatalf("Check while pending: %v", err)
	}
	if status.State != StatePending {
		t.Fatalf("state = %q, want pending", status.State)
	}
	if _, ok, _ := store.Get(ctx, settings.KeyPlexToken); ok {
		t.Fatal("the token must not be stored before authorisation")
	}

	// The user authorises at plex.tv.
	f.authorised = true
	status, err = svc.Check(ctx)
	if err != nil {
		t.Fatalf("Check after authorisation: %v", err)
	}
	if status.State != StateLinked {
		t.Fatalf("state = %q, want linked", status.State)
	}
	if len(status.Servers) != 1 || status.Servers[0].BaseURL != "http://192.168.1.20:32400" {
		t.Fatalf("unexpected servers: %+v", status.Servers)
	}

	token, ok, err := store.Get(ctx, settings.KeyPlexToken)
	if err != nil || !ok {
		t.Fatalf("token not stored: ok=%v err=%v", ok, err)
	}
	if token != "plex-token" {
		t.Errorf("stored token = %q, want plex-token", token)
	}

	// Picking a server fills in the address and ends the flow.
	if err := svc.SelectServer(ctx, status.Servers[0].BaseURL); err != nil {
		t.Fatalf("SelectServer: %v", err)
	}
	url, ok, _ := store.Get(ctx, settings.KeyPlexURL)
	if !ok || url != "http://192.168.1.20:32400" {
		t.Errorf("stored url = %q (ok=%v)", url, ok)
	}
	if svc.Pending() {
		t.Error("Pending should be false once the flow finished")
	}
}

func TestCheckWithoutStarting(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t, &fakePlexTV{})

	if _, err := svc.Check(context.Background()); !errors.Is(err, ErrNoLinkInProgress) {
		t.Errorf("Check error = %v, want ErrNoLinkInProgress", err)
	}
}

func TestExpiredPINIsReported(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t, &fakePlexTV{token: "t"})
	ctx := context.Background()

	if _, err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Pretend the code was issued long ago.
	svc.mu.Lock()
	svc.started = time.Now().Add(-2 * pinTTL)
	svc.mu.Unlock()

	status, err := svc.Check(ctx)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status.State != StateExpired {
		t.Errorf("state = %q, want expired", status.State)
	}
	if svc.Pending() {
		t.Error("an expired link should not read as pending")
	}
}

// Losing discovery must not lose the token: the link itself succeeded.
func TestLinkSurvivesFailedServerDiscovery(t *testing.T) {
	t.Parallel()
	f := &fakePlexTV{token: "plex-token", resources: `not json`, authorised: true}
	svc, store := newTestService(t, f)
	ctx := context.Background()

	if _, err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status, err := svc.Check(ctx)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status.State != StateLinked {
		t.Fatalf("state = %q, want linked despite discovery failing", status.State)
	}
	if len(status.Servers) != 0 {
		t.Errorf("expected no servers, got %+v", status.Servers)
	}
	if token, ok, _ := store.Get(ctx, settings.KeyPlexToken); !ok || token != "plex-token" {
		t.Error("the token should have been stored even though discovery failed")
	}
}

func TestClientIDIsGeneratedOnceAndReused(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t, &fakePlexTV{})
	ctx := context.Background()

	first, err := svc.clientID(ctx)
	if err != nil {
		t.Fatalf("clientID: %v", err)
	}
	second, err := svc.clientID(ctx)
	if err != nil {
		t.Fatalf("clientID again: %v", err)
	}
	if first != second {
		t.Error("the client id must be stable across calls")
	}
	stored, ok, _ := store.Get(ctx, settings.KeyPlexClientID)
	if !ok || stored != first {
		t.Errorf("client id not persisted: %q (ok=%v)", stored, ok)
	}
}

func TestCancelClearsTheFlow(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t, &fakePlexTV{})
	ctx := context.Background()

	if _, err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	svc.Cancel()
	if svc.Pending() {
		t.Error("Pending should be false after Cancel")
	}
	if _, err := svc.Check(ctx); !errors.Is(err, ErrNoLinkInProgress) {
		t.Errorf("Check after Cancel = %v, want ErrNoLinkInProgress", err)
	}
}
