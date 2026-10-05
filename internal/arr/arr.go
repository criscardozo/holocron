// Package arr talks to Radarr and Sonarr, which share an API (v3) and differ
// only in what a queue item is about: a film, or an episode of a series.
//
// Field names come from each project's own openapi.json (Radarr develop,
// Sonarr develop), not from memory. Holocron only reads: it never grabs,
// removes or reorders anything, because those decisions belong to the *arr
// that owns the download.
package arr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// App says which of the two this client is.
type App string

const (
	Radarr App = "radarr"
	Sonarr App = "sonarr"
)

// Label is the name shown on screen.
func (a App) Label() string {
	if a == Sonarr {
		return "Sonarr"
	}
	return "Radarr"
}

// ErrUnauthorized means the API key was refused.
var ErrUnauthorized = errors.New("the API key was refused")

// Client is one *arr instance.
type Client struct {
	app  App
	base string
	key  string
	http *http.Client
}

// New creates a client. base is the instance's address, such as
// http://127.0.0.1:7878.
func New(app App, base, key string) *Client {
	return &Client{
		app: app, base: strings.TrimRight(base, "/"), key: key,
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// App reports which application this is.
func (c *Client) App() App { return c.app }

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	// The key goes in a header, never the query string, where it would end up
	// in every access log between here and the service.
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req) //#nosec G704 -- base is loopback from configuration, never from a request
	if err != nil {
		return fmt.Errorf("%s %s: %w", c.app, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%s: %w", c.app, ErrUnauthorized)
	case resp.StatusCode >= 300:
		return fmt.Errorf("%s %s: HTTP %d", c.app, path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode: %w", c.app, path, err)
	}
	return nil
}

// QueueItem is one download the *arr is tracking.
type QueueItem struct {
	App App
	// Subject is what the download is for, in words: "Dune (2021)", or
	// "The Bear · T2E04".
	Subject string
	// Release is the release name, as the indexer named it.
	Release string
	// DownloadID is the download client's id: for qBittorrent, the torrent
	// hash, upper case. It is what joins this item to its torrent.
	DownloadID string
	Status     string // queued | paused | downloading | completed | failed | warning | delay | …
	State      string // downloading | importBlocked | importPending | importing | imported | failedPending | failed | ignored
	Health     string // ok | warning | error
	Size       float64
	SizeLeft   float64
	ETA        time.Time
	Messages   []string
	Error      string
}

// Progress is the fraction downloaded, 0..1.
func (q QueueItem) Progress() float64 {
	if q.Size <= 0 {
		return 0
	}
	p := 1 - q.SizeLeft/q.Size
	switch {
	case p < 0:
		return 0
	case p > 1:
		return 1
	}
	return p
}

type queuePage struct {
	TotalRecords int             `json:"totalRecords"`
	Records      []queueResource `json:"records"`
}

type queueResource struct {
	Title                   string    `json:"title"`
	DownloadID              string    `json:"downloadId"`
	Status                  string    `json:"status"`
	TrackedDownloadState    string    `json:"trackedDownloadState"`
	TrackedDownloadStatus   string    `json:"trackedDownloadStatus"`
	Size                    float64   `json:"size"`
	SizeLeft                float64   `json:"sizeleft"`
	EstimatedCompletionTime time.Time `json:"estimatedCompletionTime"`
	ErrorMessage            string    `json:"errorMessage"`
	StatusMessages          []struct {
		Title    string   `json:"title"`
		Messages []string `json:"messages"`
	} `json:"statusMessages"`
	Movie *struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
	} `json:"movie"`
	Series *struct {
		Title string `json:"title"`
	} `json:"series"`
	Episode *struct {
		SeasonNumber  int    `json:"seasonNumber"`
		EpisodeNumber int    `json:"episodeNumber"`
		Title         string `json:"title"`
	} `json:"episode"`
}

// queueLimit bounds one read. A home queue rarely holds more than a handful;
// a runaway one should not turn a status screen into a bulk export.
const queueLimit = 100

// Queue lists what the *arr is downloading or about to import.
func (c *Client) Queue(ctx context.Context) ([]QueueItem, error) {
	q := url.Values{"pageSize": {strconv.Itoa(queueLimit)}}
	if c.app == Sonarr {
		q.Set("includeSeries", "true")
		q.Set("includeEpisode", "true")
	} else {
		q.Set("includeMovie", "true")
	}
	var page queuePage
	if err := c.get(ctx, "/api/v3/queue", q, &page); err != nil {
		return nil, err
	}
	out := make([]QueueItem, 0, len(page.Records))
	for _, r := range page.Records {
		it := QueueItem{
			App: c.app, Release: r.Title, DownloadID: strings.ToUpper(r.DownloadID),
			Status: r.Status, State: r.TrackedDownloadState, Health: r.TrackedDownloadStatus,
			Size: r.Size, SizeLeft: r.SizeLeft, ETA: r.EstimatedCompletionTime, Error: r.ErrorMessage,
		}
		for _, m := range r.StatusMessages {
			it.Messages = append(it.Messages, m.Messages...)
		}
		it.Subject = subject(r)
		out = append(out, it)
	}
	return out, nil
}

func subject(r queueResource) string {
	switch {
	case r.Movie != nil && r.Movie.Title != "":
		if r.Movie.Year > 0 {
			return fmt.Sprintf("%s (%d)", r.Movie.Title, r.Movie.Year)
		}
		return r.Movie.Title
	case r.Series != nil && r.Series.Title != "":
		s := r.Series.Title
		if r.Episode != nil {
			s += fmt.Sprintf(" · T%dE%02d", r.Episode.SeasonNumber, r.Episode.EpisodeNumber)
			if r.Episode.Title != "" {
				s += " — " + r.Episode.Title
			}
		}
		return s
	}
	return r.Title
}

// HealthIssue is one entry from the *arr's own health check.
type HealthIssue struct {
	Source  string `json:"source"`
	Type    string `json:"type"` // ok | notice | warning | error
	Message string `json:"message"`
}

// Health returns what the *arr itself reports as wrong.
func (c *Client) Health(ctx context.Context) ([]HealthIssue, error) {
	var out []HealthIssue
	if err := c.get(ctx, "/api/v3/health", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
