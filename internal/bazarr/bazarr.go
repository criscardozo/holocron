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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/badges", nil)
	if err != nil {
		return b, err
	}
	req.Header.Set("X-API-KEY", c.key)
	resp, err := c.http.Do(req) //#nosec G704 -- base is loopback from configuration, never from a request
	if err != nil {
		return b, fmt.Errorf("bazarr: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return b, fmt.Errorf("bazarr: %w", ErrUnauthorized)
	case resp.StatusCode >= 300:
		return b, fmt.Errorf("bazarr: HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&b)
	return b, err
}
