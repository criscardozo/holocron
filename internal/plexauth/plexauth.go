// Package plexauth obtains a Plex authentication token through the plex.tv PIN
// (device-link) flow and discovers the account's media servers, so the user
// never has to dig a X-Plex-Token out of the browser.
//
// It talks to plex.tv, a different host from the media server itself (that one
// is internal/plex). Ported from the sibling plexmatch-generator project; the
// credential store was dropped, since Holocron keeps its settings in SQLite.
package plexauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// product identifies this client to Plex; it shows up in the account's
	// "authorised devices" list.
	product = "Holocron"

	defaultBaseURL = "https://plex.tv/api/v2/"

	// maxBody caps how much of a plex.tv response is read into memory.
	maxBody = 1 << 20
)

// NewClientID returns a random identifier for this device. It is persisted so
// the same Pi keeps being recognised as the same device across restarts.
func NewClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate client identifier: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Client performs the plex.tv authentication and discovery calls.
type Client struct {
	hc       *http.Client
	baseURL  string
	clientID string
}

// NewClient builds a Client for the given persisted client identifier.
func NewClient(clientID string) *Client {
	return newClientWithBase(clientID, defaultBaseURL)
}

// newClientWithBase lets the tests point the client at an httptest server.
func newClientWithBase(clientID, baseURL string) *Client {
	return &Client{
		hc:       &http.Client{Timeout: 30 * time.Second},
		baseURL:  baseURL,
		clientID: clientID,
	}
}

// PIN is a device-link PIN returned by plex.tv.
type PIN struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
	// AuthToken is empty until the user authorises the PIN.
	AuthToken string `json:"authToken"`
}

// CreatePIN requests a fresh PIN to start the device-link flow.
func (c *Client) CreatePIN(ctx context.Context) (PIN, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"pins?strong=true", nil)
	if err != nil {
		return PIN{}, fmt.Errorf("build pin request: %w", err)
	}
	c.setHeaders(req, "")

	var p PIN
	if err := c.do(req, &p); err != nil {
		return PIN{}, err
	}
	return p, nil
}

// AuthURL is the URL the user opens in a browser to authorise the PIN.
func (c *Client) AuthURL(p PIN) string {
	v := url.Values{}
	v.Set("clientID", c.clientID)
	v.Set("code", p.Code)
	v.Set("context[device][product]", product)
	return "https://app.plex.tv/auth#?" + v.Encode()
}

// CheckPIN asks plex.tv once whether the PIN has been authorised. An empty
// token means "not yet": the caller polls. A single check (rather than a
// blocking poll loop) keeps HTTP handlers from hanging for minutes.
func (c *Client) CheckPIN(ctx context.Context, p PIN) (string, error) {
	target := fmt.Sprintf("%spins/%d?code=%s", c.baseURL, p.ID, url.QueryEscape(p.Code))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("build pin poll request: %w", err)
	}
	c.setHeaders(req, "")

	var got PIN
	if err := c.do(req, &got); err != nil {
		return "", err
	}
	return got.AuthToken, nil
}

// ValidateToken reports whether a token is still accepted by plex.tv.
func (c *Client) ValidateToken(ctx context.Context, token string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"user", nil)
	if err != nil {
		return false, fmt.Errorf("build user request: %w", err)
	}
	c.setHeaders(req, token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return false, fmt.Errorf("validate token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusUnauthorized:
		return false, nil
	default:
		return false, fmt.Errorf("plex.tv /user returned %s", resp.Status)
	}
}

// Server is a Plex Media Server discovered for the account, with the best base
// URL to reach it.
type Server struct {
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
}

type resource struct {
	Name        string       `json:"name"`
	Provides    string       `json:"provides"`
	Connections []connection `json:"connections"`
}

type connection struct {
	URI   string `json:"uri"`
	Local bool   `json:"local"`
	Relay bool   `json:"relay"`
}

// DiscoverServers lists the account's media servers and their best base URL,
// so the server address can be filled in instead of typed.
func (c *Client) DiscoverServers(ctx context.Context, token string) ([]Server, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"resources?includeHttps=1&includeRelay=1", nil)
	if err != nil {
		return nil, fmt.Errorf("build resources request: %w", err)
	}
	c.setHeaders(req, token)

	var resources []resource
	if err := c.do(req, &resources); err != nil {
		return nil, err
	}

	servers := []Server{}
	for _, res := range resources {
		if !strings.Contains(res.Provides, "server") {
			continue
		}
		if uri := bestConnection(res.Connections); uri != "" {
			servers = append(servers, Server{Name: res.Name, BaseURL: uri})
		}
	}
	return servers, nil
}

// bestConnection prefers a local, non-relay connection (ideal for a LAN device
// like a Raspberry Pi), then any non-relay connection, then a relay.
func bestConnection(conns []connection) string {
	var nonRelay, relay string
	for _, conn := range conns {
		switch {
		case conn.Relay:
			if relay == "" {
				relay = conn.URI
			}
		case conn.Local:
			return conn.URI
		default:
			if nonRelay == "" {
				nonRelay = conn.URI
			}
		}
	}
	if nonRelay != "" {
		return nonRelay
	}
	return relay
}

func (c *Client) setHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", product)
	req.Header.Set("X-Plex-Client-Identifier", c.clientID)
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request %q: %w", req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("plex.tv %q returned %s: %s",
			req.URL.Path, resp.Status, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return fmt.Errorf("decode response from %q: %w", req.URL.Path, err)
	}
	return nil
}
