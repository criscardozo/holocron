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
	Radarr   App = "radarr"
	Sonarr   App = "sonarr"
	Prowlarr App = "prowlarr"
)

// Label is the name shown on screen.
func (a App) Label() string {
	switch a {
	case Sonarr:
		return "Sonarr"
	case Prowlarr:
		return "Prowlarr"
	default:
		return "Radarr"
	}
}

// api is the path prefix of the app's API. Prowlarr is a generation newer and
// on v1; the other two on v3.
func (a App) api() string {
	if a == Prowlarr {
		return "/api/v1"
	}
	return "/api/v3"
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
	// TmdbID (films) and TvdbID (series) say what the download is for in the
	// terms Seerr uses, which is how a request finds its download.
	TmdbID int
	TvdbID int
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
		Title  string `json:"title"`
		Year   int    `json:"year"`
		TmdbID int    `json:"tmdbId"`
	} `json:"movie"`
	Series *struct {
		Title  string `json:"title"`
		TvdbID int    `json:"tvdbId"`
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
	if err := c.get(ctx, c.app.api()+"/queue", q, &page); err != nil {
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
		if r.Movie != nil {
			it.TmdbID = r.Movie.TmdbID
		}
		if r.Series != nil {
			it.TvdbID = r.Series.TvdbID
		}
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
	if err := c.get(ctx, c.app.api()+"/health", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Upcoming is one release the *arr is waiting for.
type Upcoming struct {
	App     App
	Subject string
	// When is the release that matters for a home library: the digital one
	// for a film (cinemas are no use here), the air date for an episode.
	When    time.Time
	Kind    string // "digital", "físico", "cines" or "emisión"
	HasFile bool
	TmdbID  int
	TvdbID  int
}

// Calendar lists what comes out between from and to.
func (c *Client) Calendar(ctx context.Context, from, to time.Time) ([]Upcoming, error) {
	q := url.Values{
		"start": {from.UTC().Format("2006-01-02")},
		"end":   {to.UTC().Format("2006-01-02")},
	}
	if c.app == Sonarr {
		q.Set("includeSeries", "true")
		var eps []struct {
			SeasonNumber  int       `json:"seasonNumber"`
			EpisodeNumber int       `json:"episodeNumber"`
			Title         string    `json:"title"`
			AirDateUtc    time.Time `json:"airDateUtc"`
			HasFile       bool      `json:"hasFile"`
			Series        *struct {
				Title  string `json:"title"`
				TvdbID int    `json:"tvdbId"`
			} `json:"series"`
		}
		if err := c.get(ctx, c.app.api()+"/calendar", q, &eps); err != nil {
			return nil, err
		}
		out := make([]Upcoming, 0, len(eps))
		for _, e := range eps {
			name, tvdb := e.Title, 0
			if e.Series != nil {
				name = fmt.Sprintf("%s · T%dE%02d", e.Series.Title, e.SeasonNumber, e.EpisodeNumber)
				tvdb = e.Series.TvdbID
			}
			out = append(out, Upcoming{App: c.app, Subject: name, When: e.AirDateUtc, Kind: "emisión", HasFile: e.HasFile, TvdbID: tvdb})
		}
		return out, nil
	}

	var movies []struct {
		Title           string     `json:"title"`
		Year            int        `json:"year"`
		InCinemas       *time.Time `json:"inCinemas"`
		DigitalRelease  *time.Time `json:"digitalRelease"`
		PhysicalRelease *time.Time `json:"physicalRelease"`
		HasFile         bool       `json:"hasFile"`
		TmdbID          int        `json:"tmdbId"`
	}
	if err := c.get(ctx, c.app.api()+"/calendar", q, &movies); err != nil {
		return nil, err
	}
	var out []Upcoming
	for _, m := range movies {
		name := m.Title
		if m.Year > 0 {
			name = fmt.Sprintf("%s (%d)", m.Title, m.Year)
		}
		// Radarr lists a film when any of its dates falls in the window. Take
		// the one in the window that a home library can act on, digital
		// before physical before cinemas.
		for _, cand := range []struct {
			t    *time.Time
			kind string
		}{{m.DigitalRelease, "digital"}, {m.PhysicalRelease, "físico"}, {m.InCinemas, "cines"}} {
			if cand.t != nil && !cand.t.Before(from) && !cand.t.After(to) {
				out = append(out, Upcoming{App: c.app, Subject: name, When: *cand.t, Kind: cand.kind, HasFile: m.HasFile, TmdbID: m.TmdbID})
				break
			}
		}
	}
	return out, nil
}

// MissingCount is how many monitored items the *arr is still looking for.
func (c *Client) MissingCount(ctx context.Context) (int, error) {
	var page struct {
		TotalRecords int `json:"totalRecords"`
	}
	err := c.get(ctx, c.app.api()+"/wanted/missing", url.Values{"pageSize": {"1"}}, &page)
	return page.TotalRecords, err
}

// Indexer is one indexer configured in Prowlarr.
type Indexer struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Enable   bool   `json:"enable"`
	Protocol string `json:"protocol"`
}

// IndexerStatus is Prowlarr's record of an indexer that has been failing.
type IndexerStatus struct {
	IndexerID         int        `json:"indexerId"`
	DisabledTill      *time.Time `json:"disabledTill"`
	MostRecentFailure *time.Time `json:"mostRecentFailure"`
}

// Indexers lists Prowlarr's indexers. Only the four fields above are decoded:
// the rest of each record is the indexer's configuration, which can hold its
// own cookies and keys and has no business leaving Prowlarr.
func (c *Client) Indexers(ctx context.Context) ([]Indexer, error) {
	var out []Indexer
	err := c.get(ctx, c.app.api()+"/indexer", nil, &out)
	return out, err
}

// IndexerStatuses lists the indexers Prowlarr is backing off from.
func (c *Client) IndexerStatuses(ctx context.Context) ([]IndexerStatus, error) {
	var out []IndexerStatus
	err := c.get(ctx, c.app.api()+"/indexerstatus", nil, &out)
	return out, err
}
