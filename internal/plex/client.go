package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	headerToken          = "X-Plex-Token"
	headerContainerStart = "X-Plex-Container-Start"
	headerContainerSize  = "X-Plex-Container-Size"

	maxAttempts = 3
	pageSize    = 50
)

// Client talks to a single Plex Media Server.
type Client struct {
	baseURL   string // always ends with "/"
	token     string
	hc        *http.Client
	retryWait time.Duration
}

// New builds a Client for baseURL (e.g. "http://192.168.1.10:32400") with the
// given X-Plex-Token. The base URL is normalised to end with "/".
func New(baseURL, token string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &Client{
		baseURL:   baseURL,
		token:     token,
		hc:        &http.Client{Timeout: 30 * time.Second},
		retryWait: 500 * time.Millisecond,
	}
}

func (c *Client) get(ctx context.Context, path string, headers map[string]string, out any) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var retryable bool
		retryable, err = c.getOnce(ctx, path, headers, out)
		if err == nil {
			return nil
		}
		if !retryable || attempt == maxAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * c.retryWait):
		}
	}
	return err
}

func (c *Client) getOnce(ctx context.Context, path string, headers map[string]string, out any) (retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return false, fmt.Errorf("building request for %q: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(headerToken, c.token)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return true, fmt.Errorf("requesting %q: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return retryableStatus(resp.StatusCode),
			fmt.Errorf("plex API %q returned %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return false, fmt.Errorf("decoding response from %q: %w", path, err)
	}
	return false, nil
}

func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// Libraries returns every library section on the server.
func (c *Client) Libraries(ctx context.Context) ([]Library, error) {
	var r containerResponse
	if err := c.get(ctx, "library/sections", nil, &r); err != nil {
		return nil, err
	}
	return r.MediaContainer.Directory, nil
}

// LibraryItems returns one page of items from a library. Plex pages via request
// headers rather than query parameters.
func (c *Client) LibraryItems(ctx context.Context, libraryID string, start, size int) ([]Metadata, error) {
	headers := map[string]string{
		headerContainerStart: strconv.Itoa(start),
		headerContainerSize:  strconv.Itoa(size),
	}
	var r containerResponse
	if err := c.get(ctx, "library/sections/"+libraryID+"/all", headers, &r); err != nil {
		return nil, err
	}
	return r.MediaContainer.Metadata, nil
}

// AllLibraryItems pages through an entire library and returns every item.
func (c *Client) AllLibraryItems(ctx context.Context, libraryID string) ([]Metadata, error) {
	var all []Metadata
	for start := 0; ; start += pageSize {
		page, err := c.LibraryItems(ctx, libraryID, start, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			return all, nil
		}
	}
}
