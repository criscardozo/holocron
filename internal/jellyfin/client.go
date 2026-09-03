// Package jellyfin is a client for the Jellyfin server API. It replaces the
// Plex client: this HTPC migrated to Jellyfin, so the Plex integration had
// stopped doing anything at all.
//
// Two things it gets from Jellyfin that Plex never offered: the real filesystem
// path of every item, and the subtitle streams of each file — including
// embedded ones, and including the path of external .srt files. That last part
// replaces a directory walk per title, which on a 5400 rpm USB disk was the
// heaviest thing Holocron did.
package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cristian/holocron/internal/netaddr"
)

// maxJSONBody caps how much of a response is read into memory. The full library
// listing of a few thousand items fits well inside this.
const maxJSONBody = 64 << 20

// product and version identify Holocron in Jellyfin's device list.
const product = "Holocron"

// Client talks to one Jellyfin server.
type Client struct {
	base     string
	token    string
	deviceID string
	version  string
	hc       *http.Client
}

// New builds a client. token may be empty for the Quick Connect handshake,
// which authenticates the device rather than using an existing token. deviceID
// must be stable across restarts so Jellyfin keeps one entry per install.
//
// The address is normalised here as well as on the way in, so an install that
// already stored a bare host:port starts working on upgrade instead of waiting
// for someone to re-save the form.
func New(baseURL, token, deviceID, version string) *Client {
	return &Client{
		base:     netaddr.Repair(baseURL),
		token:    token,
		deviceID: deviceID,
		version:  version,
		hc:       &http.Client{Timeout: 60 * time.Second},
	}
}

// authHeader is required on every request, with or without a token: Jellyfin
// rejects calls that do not identify the client, including the Quick Connect
// handshake.
func (c *Client) authHeader() string {
	h := fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="%s"`,
		product, product, c.deviceID, c.version)
	if c.token != "" {
		h += fmt.Sprintf(`, Token="%s"`, c.token)
	}
	return h
}

func (c *Client) do(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.authHeader())

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &StatusError{
			Status:  resp.StatusCode,
			Path:    path,
			Message: strings.TrimSpace(string(snippet)),
		}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxJSONBody))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBody)).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// StatusError is a non-2xx response. Callers check Status to tell "you are not
// allowed" apart from "the server is unhappy", which matters for the refresh
// endpoint: it needs an administrator.
type StatusError struct {
	Status  int
	Path    string
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("jellyfin %s returned %d: %s", e.Path, e.Status, e.Message)
}

// Forbidden reports whether the server refused for lack of permission.
func (e *StatusError) Forbidden() bool { return e.Status == http.StatusForbidden }

// ServerInfo is the handshake used to verify a configured connection.
type ServerInfo struct {
	Name    string `json:"ServerName"`
	Version string `json:"Version"`
	ID      string `json:"Id"`
}

// Info returns the server's identity. Requires a token, so reaching it proves
// the whole configuration works, not just the address.
func (c *Client) Info(ctx context.Context) (ServerInfo, error) {
	var out ServerInfo
	if err := c.do(ctx, http.MethodGet, "/System/Info", &out); err != nil {
		return ServerInfo{}, err
	}
	return out, nil
}

// PublicInfo answers without a token. That is the point: it tells "the address
// is wrong" apart from "you have not linked yet", which are the two ways the
// setup fails and used to look identical.
func (c *Client) PublicInfo(ctx context.Context) (ServerInfo, error) {
	var out ServerInfo
	if err := c.do(ctx, http.MethodGet, "/System/Info/Public", &out); err != nil {
		return ServerInfo{}, err
	}
	return out, nil
}

// RefreshItem asks Jellyfin to re-read one item's metadata. With the library's
// "save metadata as NFO" setting on, this is what makes Jellyfin write a
// missing .nfo — Holocron never writes those files itself, so the two cannot
// fight over them.
//
// FullRefresh is deliberate: a Default refresh was measured not to write the
// file at all. It does reach out to the metadata provider, so callers must
// throttle and must warn the user.
func (c *Client) RefreshItem(ctx context.Context, itemID string) error {
	q := url.Values{
		"metadataRefreshMode": {"FullRefresh"},
		"imageRefreshMode":    {"None"},
		"replaceAllMetadata":  {"false"},
		"replaceAllImages":    {"false"},
	}
	return c.do(ctx, http.MethodPost,
		"/Items/"+url.PathEscape(itemID)+"/Refresh?"+q.Encode(), nil)
}
