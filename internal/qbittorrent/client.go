// Package qbittorrent is a client for the qBittorrent WebUI API
// (https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)).
// It authenticates with a session cookie and covers listing, add-by-magnet and
// the pause/resume/delete actions.
package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/netaddr"
)

// maxResponse caps how much of a qBittorrent response is read into memory.
// A large library still fits comfortably; a runaway one cannot exhaust the Pi.
const maxResponse = 8 << 20

// Torrent is one entry from /torrents/info.
type Torrent struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	State     string  `json:"state"`
	Progress  float64 `json:"progress"` // 0..1
	Size      int64   `json:"size"`
	DlSpeed   int64   `json:"dlspeed"`
	UpSpeed   int64   `json:"upspeed"`
	NumSeeds  int     `json:"num_seeds"`
	NumLeechs int     `json:"num_leechs"`
	Category  string  `json:"category"`
}

// Client talks to a single qBittorrent WebUI.
type Client struct {
	base string
	user string
	pass string
	hc   *http.Client

	mu       sync.Mutex
	loggedIn bool
}

// New builds a Client for baseURL (e.g. "http://127.0.0.1:8080") with WebUI
// credentials. A cookie jar holds the session.
func New(baseURL, user, pass string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookie jar: %w", err)
	}
	return &Client{
		base: netaddr.Repair(baseURL),
		user: user,
		pass: pass,
		hc:   &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}, nil
}

func (c *Client) login(ctx context.Context) error {
	form := url.Values{"username": {c.user}, "password": {c.pass}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build login: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// qBittorrent's CSRF protection requires a matching Referer.
	req.Header.Set("Referer", c.base)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Ok.") {
		return fmt.Errorf("qbittorrent login failed: %s", strings.TrimSpace(string(body)))
	}
	c.loggedIn = true
	return nil
}

func (c *Client) ensureLogin(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loggedIn {
		return nil
	}
	return c.login(ctx)
}

// post sends a form-encoded POST, logging in first and retrying once on a 403
// (expired session).
func (c *Client) post(ctx context.Context, path string, form url.Values) error {
	if err := c.ensureLogin(ctx); err != nil {
		return err
	}
	do := func() (int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.base+path, strings.NewReader(form.Encode()))
		if err != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", c.base)
		resp, err := c.hc.Do(req)
		if err != nil {
			return 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return resp.StatusCode, nil
	}

	code, err := do()
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	if code == http.StatusForbidden {
		c.mu.Lock()
		c.loggedIn = false
		c.mu.Unlock()
		if err := c.ensureLogin(ctx); err != nil {
			return err
		}
		if code, err = do(); err != nil {
			return fmt.Errorf("post %s: %w", path, err)
		}
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("post %s returned %d", path, code)
	}
	return nil
}

// getJSON performs an authenticated GET and decodes the JSON body into out.
// The session cookie is reused across calls, so an expired one is refreshed and
// the request retried once rather than surfacing as an error.
func (c *Client) getJSON(ctx context.Context, path, what string, out any) error {
	if err := c.ensureLogin(ctx); err != nil {
		return err
	}

	body, status, err := c.doGet(ctx, path)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if status == http.StatusForbidden {
		c.mu.Lock()
		c.loggedIn = false
		c.mu.Unlock()
		if err := c.ensureLogin(ctx); err != nil {
			return err
		}
		if body, status, err = c.doGet(ctx, path); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		if status == http.StatusForbidden {
			return errors.New("qbittorrent rejected the session after re-login")
		}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%s returned %d", what, status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", what, err)
	}
	return nil
}

// doGet performs one GET and returns the body and status.
func (c *Client) doGet(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Referer", c.base)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// Torrents lists all torrents.
func (c *Client) Torrents(ctx context.Context) ([]Torrent, error) {
	var out []Torrent
	if err := c.getJSON(ctx, "/api/v2/torrents/info", "list torrents", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Category is a category configured in qBittorrent.
type Category struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

// Categories lists the categories configured in qBittorrent, sorted by name so
// the UI order is stable (the API returns an unordered object).
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	var byName map[string]Category
	if err := c.getJSON(ctx, "/api/v2/torrents/categories", "list categories", &byName); err != nil {
		return nil, err
	}
	out := make([]Category, 0, len(byName))
	for key, cat := range byName {
		// Older builds leave "name" empty and only use the object key.
		if cat.Name == "" {
			cat.Name = key
		}
		out = append(out, cat)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Pause pauses a torrent by hash.
func (c *Client) Pause(ctx context.Context, hash string) error {
	return c.post(ctx, "/api/v2/torrents/pause", url.Values{"hashes": {hash}})
}

// Resume resumes a torrent by hash.
func (c *Client) Resume(ctx context.Context, hash string) error {
	return c.post(ctx, "/api/v2/torrents/resume", url.Values{"hashes": {hash}})
}

// Delete removes a torrent. When withData is true its files are deleted too.
func (c *Client) Delete(ctx context.Context, hash string, withData bool) error {
	return c.post(ctx, "/api/v2/torrents/delete", url.Values{
		"hashes":      {hash},
		"deleteFiles": {boolStr(withData)},
	})
}

// AddMagnet queues a magnet link, optionally into a category.
func (c *Client) AddMagnet(ctx context.Context, magnet, category string) error {
	form := url.Values{"urls": {magnet}}
	if category != "" {
		form.Set("category", category)
	}
	return c.post(ctx, "/api/v2/torrents/add", form)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
