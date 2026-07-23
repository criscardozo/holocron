// Package opensubtitles is a small client for the OpenSubtitles REST API
// (https://opensubtitles.stoplight.io/docs/opensubtitles-api). It covers the
// search, login and download endpoints Holocron needs.
package opensubtitles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.opensubtitles.com/api/v1"
	userAgent      = "Holocron/0.1"
)

// Client talks to the OpenSubtitles API.
type Client struct {
	apiKey  string
	baseURL string
	hc      *http.Client
	token   string // set after Login; optional for search, required for download
}

// New builds a Client with the given API key.
func New(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		hc:      &http.Client{Timeout: 30 * time.Second},
	}
}

// WithBaseURL overrides the API base URL (used in tests).
func (c *Client) WithBaseURL(u string) *Client {
	c.baseURL = strings.TrimRight(u, "/")
	return c
}

// Subtitle is one search result (flattened to a single downloadable file).
type Subtitle struct {
	FileID   int
	FileName string
	Language string
	Release  string
	Title    string
	Year     int
}

// Login exchanges username/password for a bearer token used by Download. It is
// optional for Search.
func (c *Client) Login(ctx context.Context, username, password string) error {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	var out struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, http.MethodPost, "/login", bytes.NewReader(body), &out); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if out.Token == "" {
		return fmt.Errorf("login: empty token")
	}
	c.token = out.Token
	return nil
}

// Search finds subtitles for a title/year in the given language (e.g. "es").
func (c *Client) Search(ctx context.Context, query string, year int, language string) ([]Subtitle, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("languages", language)
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}

	var out struct {
		Data []struct {
			Attributes struct {
				Language string `json:"language"`
				Release  string `json:"release"`
				Files    []struct {
					FileID   int    `json:"file_id"`
					FileName string `json:"file_name"`
				} `json:"files"`
				FeatureDetails struct {
					Title string `json:"title"`
					Year  int    `json:"year"`
				} `json:"feature_details"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/subtitles?"+q.Encode(), nil, &out); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	var subs []Subtitle
	for _, d := range out.Data {
		for _, f := range d.Attributes.Files {
			subs = append(subs, Subtitle{
				FileID:   f.FileID,
				FileName: f.FileName,
				Language: d.Attributes.Language,
				Release:  d.Attributes.Release,
				Title:    d.Attributes.FeatureDetails.Title,
				Year:     d.Attributes.FeatureDetails.Year,
			})
		}
	}
	return subs, nil
}

// Download resolves a file_id to a temporary link and fetches its contents.
func (c *Client) Download(ctx context.Context, fileID int) (content []byte, filename string, err error) {
	body, _ := json.Marshal(map[string]int{"file_id": fileID})
	var out struct {
		Link     string `json:"link"`
		FileName string `json:"file_name"`
	}
	if err := c.do(ctx, http.MethodPost, "/download", bytes.NewReader(body), &out); err != nil {
		return nil, "", fmt.Errorf("request download link: %w", err)
	}
	if out.Link == "" {
		return nil, "", fmt.Errorf("download: empty link")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, out.Link, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch subtitle: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("fetch subtitle: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5 MiB cap
	if err != nil {
		return nil, "", fmt.Errorf("read subtitle: %w", err)
	}
	return data, out.FileName, nil
}

// do performs a JSON request with the required headers and decodes into out.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("api %s returned %s: %s", path, resp.Status, strings.TrimSpace(string(snippet)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
