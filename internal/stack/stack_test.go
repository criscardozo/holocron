package stack

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestTheDomainIsTheOneTheRequestCameTo(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"holocron.merli.store":      "merli.store",
		"merli.store":               "merli.store",
		"Holocron.Merli.Store:443":  "merli.store",
		"holocron.example.org:8090": "example.org",
	}
	for host, want := range cases {
		if got := Domain(host); got != want {
			t.Errorf("Domain(%q) = %q, want %q", host, got, want)
		}
	}
	qb := Apps[len(Apps)-1]
	if qb.Key != "qbittorrent" || qb.URL("merli.store") != "https://qb.merli.store/" {
		t.Errorf("qBittorrent answers on qb.: %+v → %s", qb, qb.URL("merli.store"))
	}
}

func TestEveryAppIsComplete(t *testing.T) {
	t.Parallel()
	featured := 0
	for _, a := range Apps {
		if a.Key == "" || a.Name == "" || a.Short == "" || a.Purpose == "" || a.Sub == "" || a.Port == 0 || a.Logo == "" || a.Unit == "" {
			t.Errorf("incomplete: %+v", a)
		}
		if a.Featured {
			featured++
		}
	}
	if featured != 2 {
		t.Errorf("%d featured apps, want Jellyfin and Seerr", featured)
	}
}

// TestVersionsAreReadOnceAndKept. Asking seven services on every visit would
// be the polling the house avoids, and one that stops answering should not
// make its version disappear from the page.
func TestVersionsAreReadOnceAndKept(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	failing := atomic.Bool{}
	v := NewVersions(map[string]VersionFunc{
		"radarr": func(context.Context) (string, error) {
			calls.Add(1)
			if failing.Load() {
				return "", errors.New("down")
			}
			return "6.4.4.10685", nil
		},
		"qbittorrent": func(context.Context) (string, error) { return "v5.1.2", nil },
	})
	got := v.Get(t.Context())
	if got["radarr"] != "6.4.4.10685" || got["qbittorrent"] != "5.1.2" {
		t.Errorf("versions = %v", got)
	}
	v.Get(t.Context())
	if calls.Load() != 1 {
		t.Errorf("asked %d times within the cache window, want 1", calls.Load())
	}

	failing.Store(true)
	v.mu.Lock()
	v.at = v.at.Add(-2 * versionsTTL)
	v.mu.Unlock()
	if got := v.Get(t.Context()); got["radarr"] != "6.4.4.10685" {
		t.Errorf("a failed read lost the version: %v", got)
	}
}
