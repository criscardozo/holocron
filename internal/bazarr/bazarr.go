// Package bazarr reads Bazarr's own tally of what is missing subtitles.
//
// Holocron no longer searches subtitles itself — Bazarr does that for Radarr
// and Sonarr, automatically — so all that is left here is to say how many are
// still missing, which is what /api/badges is for: the same counts Bazarr shows
// on its own menu.
package bazarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnauthorized means the API key was refused.
var ErrUnauthorized = errors.New("the API key was refused")

// Client talks to one Bazarr.
type Client struct {
	base, key string
	http      *http.Client
}

// New creates a client for the Bazarr at base, such as http://127.0.0.1:6767.
func New(base, key string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), key: key, http: &http.Client{Timeout: 10 * time.Second}}
}

// Badges is the count of what is missing subtitles, as Bazarr's menu shows it.
type Badges struct {
	Episodes  int `json:"episodes"`
	Movies    int `json:"movies"`
	Providers int `json:"providers"` // providers currently throttled or failing
	// The live links Bazarr keeps to Sonarr and Radarr. "LIVE" when connected;
	// anything else means Bazarr is not hearing about new files.
	SonarrSignalR string `json:"sonarr_signalr"`
	RadarrSignalR string `json:"radarr_signalr"`
}

// Badges reads the counts.
func (c *Client) Badges(ctx context.Context) (Badges, error) {
	var b Badges
	err := c.get(ctx, "/api/badges", &b)
	return b, err
}

// Version is the running Bazarr's version.
func (c *Client) Version(ctx context.Context) (string, error) {
	var st struct {
		Data struct {
			Version string `json:"bazarr_version"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/api/system/status", &st); err != nil {
		return "", err
	}
	return st.Data.Version, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.key)
	resp, err := c.http.Do(req) //#nosec G704 -- base is loopback from configuration, never from a request
	if err != nil {
		return fmt.Errorf("bazarr: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("bazarr: %w", ErrUnauthorized)
	case resp.StatusCode >= 300:
		return fmt.Errorf("bazarr %s: HTTP %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("bazarr %s: decode: %w", path, err)
	}
	return nil
}
