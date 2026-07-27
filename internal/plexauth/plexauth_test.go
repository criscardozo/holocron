package plexauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePlexTV stands in for plex.tv. authorised flips the PIN from pending to
// granted, which is what the polling flow is built around.
type fakePlexTV struct {
	authorised bool
	token      string
	resources  string
	sawHeaders http.Header
}

func (f *fakePlexTV) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("POST /pins", func(w http.ResponseWriter, r *http.Request) {
		f.sawHeaders = r.Header.Clone()
		_ = json.NewEncoder(w).Encode(PIN{ID: 42, Code: "ABCD"})
	})

	mux.HandleFunc("GET /pins/42", func(w http.ResponseWriter, _ *http.Request) {
		out := PIN{ID: 42, Code: "ABCD"}
		if f.authorised {
			out.AuthToken = f.token
		}
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != f.token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	mux.HandleFunc("GET /resources", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != f.token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(f.resources))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T, f *fakePlexTV) *Client {
	t.Helper()
	return newClientWithBase("client-123", f.server(t).URL+"/")
}

func TestCreatePINSendsIdentifyingHeaders(t *testing.T) {
	t.Parallel()
	f := &fakePlexTV{}
	c := newTestClient(t, f)

	pin, err := c.CreatePIN(context.Background())
	if err != nil {
		t.Fatalf("CreatePIN: %v", err)
	}
	if pin.ID != 42 || pin.Code != "ABCD" {
		t.Fatalf("unexpected pin: %+v", pin)
	}
	// Plex uses these to name the device in the account's authorised list.
	if got := f.sawHeaders.Get("X-Plex-Product"); got != product {
		t.Errorf("X-Plex-Product = %q, want %q", got, product)
	}
	if got := f.sawHeaders.Get("X-Plex-Client-Identifier"); got != "client-123" {
		t.Errorf("X-Plex-Client-Identifier = %q, want client-123", got)
	}
}

func TestCheckPINIsEmptyUntilAuthorised(t *testing.T) {
	t.Parallel()
	f := &fakePlexTV{token: "secret-token"}
	c := newTestClient(t, f)
	ctx := context.Background()

	pin, err := c.CreatePIN(ctx)
	if err != nil {
		t.Fatalf("CreatePIN: %v", err)
	}

	token, err := c.CheckPIN(ctx, pin)
	if err != nil {
		t.Fatalf("CheckPIN before authorisation: %v", err)
	}
	if token != "" {
		t.Fatalf("expected no token before authorisation, got %q", token)
	}

	f.authorised = true
	token, err = c.CheckPIN(ctx, pin)
	if err != nil {
		t.Fatalf("CheckPIN after authorisation: %v", err)
	}
	if token != "secret-token" {
		t.Errorf("token = %q, want secret-token", token)
	}
}

func TestAuthURLCarriesCodeAndClient(t *testing.T) {
	t.Parallel()
	c := newClientWithBase("client-123", "https://example.invalid/")

	got := c.AuthURL(PIN{ID: 1, Code: "WXYZ"})
	for _, want := range []string{"https://app.plex.tv/auth#?", "code=WXYZ", "clientID=client-123", "Holocron"} {
		if !strings.Contains(got, want) {
			t.Errorf("AuthURL = %q, missing %q", got, want)
		}
	}
}

func TestValidateToken(t *testing.T) {
	t.Parallel()
	f := &fakePlexTV{token: "good"}
	c := newTestClient(t, f)
	ctx := context.Background()

	ok, err := c.ValidateToken(ctx, "good")
	if err != nil || !ok {
		t.Errorf("ValidateToken(good) = %v, %v; want true, nil", ok, err)
	}
	ok, err = c.ValidateToken(ctx, "bad")
	if err != nil {
		t.Errorf("ValidateToken(bad) returned an error: %v", err)
	}
	if ok {
		t.Error("ValidateToken(bad) = true, want false")
	}
}

func TestDiscoverServersPrefersALocalConnection(t *testing.T) {
	t.Parallel()
	f := &fakePlexTV{token: "t", resources: `[
		{"name":"Pi","provides":"server","connections":[
			{"uri":"https://relay.plex.direct","local":false,"relay":true},
			{"uri":"https://remote.example:32400","local":false,"relay":false},
			{"uri":"http://192.168.1.20:32400","local":true,"relay":false}
		]},
		{"name":"Un cliente","provides":"client","connections":[
			{"uri":"http://192.168.1.99:32500","local":true,"relay":false}
		]}
	]`}
	c := newTestClient(t, f)

	servers, err := c.DiscoverServers(context.Background(), "t")
	if err != nil {
		t.Fatalf("DiscoverServers: %v", err)
	}
	// Only media servers, and the LAN address wins: this runs on the same
	// network as the Pi.
	if len(servers) != 1 {
		t.Fatalf("got %d servers, want 1: %+v", len(servers), servers)
	}
	if servers[0].Name != "Pi" || servers[0].BaseURL != "http://192.168.1.20:32400" {
		t.Errorf("unexpected server: %+v", servers[0])
	}
}

func TestDiscoverServersFallsBackWhenThereIsNoLocalConnection(t *testing.T) {
	t.Parallel()
	f := &fakePlexTV{token: "t", resources: `[
		{"name":"Remoto","provides":"server","connections":[
			{"uri":"https://relay.plex.direct","local":false,"relay":true},
			{"uri":"https://remote.example:32400","local":false,"relay":false}
		]},
		{"name":"Solo relay","provides":"server","connections":[
			{"uri":"https://only-relay.plex.direct","local":false,"relay":true}
		]}
	]`}
	c := newTestClient(t, f)

	servers, err := c.DiscoverServers(context.Background(), "t")
	if err != nil {
		t.Fatalf("DiscoverServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(servers))
	}
	if servers[0].BaseURL != "https://remote.example:32400" {
		t.Errorf("non-relay should win over relay, got %q", servers[0].BaseURL)
	}
	if servers[1].BaseURL != "https://only-relay.plex.direct" {
		t.Errorf("a relay-only server should still be listed, got %q", servers[1].BaseURL)
	}
}

func TestNewClientIDIsRandom(t *testing.T) {
	t.Parallel()
	a, err := NewClientID()
	if err != nil {
		t.Fatalf("NewClientID: %v", err)
	}
	b, err := NewClientID()
	if err != nil {
		t.Fatalf("NewClientID: %v", err)
	}
	if a == b {
		t.Fatal("two generated client ids are identical")
	}
	if len(a) != 32 {
		t.Errorf("client id length = %d, want 32 hex chars", len(a))
	}
}
