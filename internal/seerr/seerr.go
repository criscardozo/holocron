// Package seerr reads requests from Seerr (formerly Jellyseerr): what has been
// asked for, by whom, and how far it has got.
//
// Status numbers are those of Seerr's own source (server/constants/media.ts),
// not of its published YAML, which lags behind it: the YAML stops request
// statuses at 3 and calls media status 6 "deleted", while the code has FAILED
// and COMPLETED for requests and BLOCKLISTED at 6.
package seerr

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
	"sync"
	"time"
)

// ErrUnauthorized means the API key was refused.
var ErrUnauthorized = errors.New("the API key was refused")

// Client talks to one Seerr.
type Client struct {
	base string
	key  string
	http *http.Client

	// Titles never change for a TMDb id, and a request does not carry one,
	// so each costs a lookup the first time and never again.
	mu     sync.Mutex
	titles map[string]titled
}

type titled struct {
	Title string
	Year  int
}

// New creates a client for the Seerr at base, such as http://127.0.0.1:5055.
func New(base, key string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"), key: key,
		http:   &http.Client{Timeout: 10 * time.Second},
		titles: map[string]titled{},
	}
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req) //#nosec G704 -- base is loopback from configuration, never from a request
	if err != nil {
		return fmt.Errorf("seerr %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("seerr: %w", ErrUnauthorized)
	case resp.StatusCode >= 300:
		return fmt.Errorf("seerr %s: HTTP %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("seerr %s: decode: %w", path, err)
	}
	return nil
}

// Request is one thing somebody asked for.
type Request struct {
	ID        int
	Type      string // movie | tv
	Title     string
	Year      int
	By        string
	CreatedAt time.Time
	// Status is the request's own state: pending approval, approved, declined,
	// failed, completed.
	Status int
	// MediaStatus is where the thing itself is: unknown, pending, processing,
	// partially available, available, blocklisted, deleted.
	MediaStatus int
	Seasons     int
	// TmdbID and TvdbID identify the media, to find it in the *arr queues
	// and calendars.
	TmdbID int
	TvdbID int
}

// Request statuses (MediaRequestStatus in Seerr's source).
const (
	RequestPending   = 1
	RequestApproved  = 2
	RequestDeclined  = 3
	RequestFailed    = 4
	RequestCompleted = 5
)

// Media statuses (MediaStatus in Seerr's source).
const (
	MediaUnknown            = 1
	MediaPending            = 2
	MediaProcessing         = 3
	MediaPartiallyAvailable = 4
	MediaAvailable          = 5
	MediaBlocklisted        = 6
	MediaDeleted            = 7
)

type requestPage struct {
	Results []struct {
		ID          int       `json:"id"`
		Type        string    `json:"type"`
		Status      int       `json:"status"`
		CreatedAt   time.Time `json:"createdAt"`
		SeasonCount int       `json:"seasonCount"`
		Media       struct {
			TmdbID int `json:"tmdbId"`
			TvdbID int `json:"tvdbId"`
			Status int `json:"status"`
		} `json:"media"`
		RequestedBy struct {
			DisplayName string `json:"displayName"`
			Username    string `json:"username"`
		} `json:"requestedBy"`
	} `json:"results"`
}

// Recent returns the latest requests, newest first, with their titles.
//
// Only a display name is taken from the requester. Seerr also sends their
// email address, and that has no business on a status screen.
func (c *Client) Recent(ctx context.Context, take int) ([]Request, error) {
	q := url.Values{"take": {strconv.Itoa(take)}, "sort": {"added"}}
	var page requestPage
	if err := c.get(ctx, "/api/v1/request", q, &page); err != nil {
		return nil, err
	}
	out := make([]Request, 0, len(page.Results))
	for _, r := range page.Results {
		req := Request{
			ID: r.ID, Type: r.Type, Status: r.Status, CreatedAt: r.CreatedAt,
			MediaStatus: r.Media.Status, Seasons: r.SeasonCount,
			TmdbID: r.Media.TmdbID, TvdbID: r.Media.TvdbID,
			By: r.RequestedBy.DisplayName,
		}
		if req.By == "" {
			req.By = r.RequestedBy.Username
		}
		if t, err := c.title(ctx, r.Type, r.Media.TmdbID); err == nil {
			req.Title, req.Year = t.Title, t.Year
		} else {
			req.Title = fmt.Sprintf("TMDb %d", r.Media.TmdbID)
		}
		out = append(out, req)
	}
	return out, nil
}

func (c *Client) title(ctx context.Context, kind string, tmdbID int) (titled, error) {
	if tmdbID == 0 {
		return titled{}, errors.New("no tmdb id")
	}
	key := kind + "/" + strconv.Itoa(tmdbID)
	c.mu.Lock()
	t, ok := c.titles[key]
	c.mu.Unlock()
	if ok {
		return t, nil
	}
	var d struct {
		Title        string `json:"title"`
		Name         string `json:"name"`
		ReleaseDate  string `json:"releaseDate"`
		FirstAirDate string `json:"firstAirDate"`
	}
	path := "/api/v1/movie/"
	if kind == "tv" {
		path = "/api/v1/tv/"
	}
	if err := c.get(ctx, path+strconv.Itoa(tmdbID), nil, &d); err != nil {
		return titled{}, err
	}
	t = titled{Title: d.Title}
	date := d.ReleaseDate
	if kind == "tv" {
		t.Title, date = d.Name, d.FirstAirDate
	}
	if len(date) >= 4 {
		t.Year, _ = strconv.Atoi(date[:4])
	}
	c.mu.Lock()
	c.titles[key] = t
	c.mu.Unlock()
	return t, nil
}

// Counts is Seerr's own tally of requests.
type Counts struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Approved   int `json:"approved"`
	Declined   int `json:"declined"`
	Processing int `json:"processing"`
	Available  int `json:"available"`
	Completed  int `json:"completed"`
}

// Count returns how many requests are in each state.
func (c *Client) Count(ctx context.Context) (Counts, error) {
	var n Counts
	err := c.get(ctx, "/api/v1/request/count", nil, &n)
	return n, err
}
