package httpserver

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestCrossSitePostsAreRejected pins the hole this middleware was written for.
// Before it existed, this exact request returned 303 and the folder was
// written: the web UI has no session, so a page open in any browser on the LAN
// could drive Holocron.
func TestCrossSitePostsAreRejected(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	form := url.Values{"label": {"pwned"}, "path": {t.TempDir()}, "purpose": {"disk"}}
	resp := ts.post(t, "/settings/folders", form, map[string]string{
		"Origin":  "https://evil.example",
		"Referer": "https://evil.example/x",
	})

	if resp.Status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.Status)
	}
	if n := ts.folderCount(t); n != 0 {
		t.Errorf("%d folder(s) were written by a cross-site POST", n)
	}
	if !strings.Contains(ts.logs.String(), "rejected cross-site request") {
		t.Error("the rejection should be logged: it is either a broken proxy or someone trying")
	}
}

func TestSecFetchSiteDecidesBeforeOrigin(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{
			// What a browser sends for a form on our own page.
			name:    "same-origin passes",
			headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
			want:    http.StatusSeeOther,
		},
		{
			// A direct navigation, e.g. a bookmarked POST target.
			name:    "none passes",
			headers: map[string]string{"Sec-Fetch-Site": "none"},
			want:    http.StatusSeeOther,
		},
		{
			name:    "cross-site is refused",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			want:    http.StatusForbidden,
		},
		{
			name:    "same-site is refused too",
			headers: map[string]string{"Sec-Fetch-Site": "same-site"},
			want:    http.StatusForbidden,
		},
		{
			// Sec-Fetch-Site wins: a browser that sends it is telling the
			// truth, and Origin can legitimately differ behind a proxy.
			name: "sec-fetch-site outranks a foreign Origin",
			headers: map[string]string{
				"Sec-Fetch-Site": "same-origin",
				"Origin":         "https://evil.example",
			},
			want: http.StatusSeeOther,
		},
		{
			// Nothing at all: curl, the installer, a script. Not a browser, so
			// there is no cross-site attack to prevent.
			name:    "a request with neither header passes",
			headers: nil,
			want:    http.StatusSeeOther,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts := newTestServer(t)
			form := url.Values{"label": {"Discos"}, "path": {t.TempDir()}, "purpose": {"disk"}}
			resp := ts.post(t, "/settings/folders", form, tc.headers)
			if resp.Status != tc.want {
				t.Errorf("status = %d, want %d", resp.Status, tc.want)
			}
		})
	}
}

// TestOriginIsComparedByHostOnly matters because of the tunnel: the browser
// speaks https to Cloudflare while the origin server speaks http, so requiring
// the scheme to match would reject every request arriving through it.
func TestOriginIsComparedByHostOnly(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	host := strings.TrimPrefix(ts.URL, "http://")

	for _, origin := range []string{"http://" + host, "https://" + host} {
		form := url.Values{"label": {"Discos"}, "path": {t.TempDir()}, "purpose": {"disk"}}
		resp := ts.post(t, "/settings/folders", form, map[string]string{"Origin": origin})
		if resp.Status == http.StatusForbidden {
			t.Errorf("Origin %q was refused; only the host should be compared", origin)
		}
	}
}

// TestReadsAreNotBlocked keeps the check on the methods that change something.
// A cross-site GET cannot do damage, and blocking it would break a link.
func TestReadsAreNotBlocked(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	resp := ts.get(t, "/", map[string]string{"Sec-Fetch-Site": "cross-site"})
	if resp.Status != http.StatusOK {
		t.Errorf("cross-site GET = %d, want 200", resp.Status)
	}
}

// TestAPIIsExemptFromTheOriginCheck: the API is for the phone, not a browser.
// It authenticates with a bearer token, and an app does not send Origin.
func TestAPIIsExemptFromTheOriginCheck(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	resp := ts.post(t, "/api/v1/media/sync", nil, map[string]string{
		"Sec-Fetch-Site": "cross-site",
	})
	// 401 rather than 403: it got past the origin check and was stopped by the
	// token check, which is the layer that guards the API.
	if resp.Status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (rejected by the token, not the origin)", resp.Status)
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	resp := ts.get(t, "/", nil)

	want := map[string]string{
		"Content-Security-Policy": "default-src 'self'; base-uri 'self'; frame-ancestors 'none'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
	}
	for header, value := range want {
		if got := resp.Header.Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
}

// TestNoInlineStylesAnywhere replaces the manual curl-and-grep that caught this
// repeatedly. The CSP has no 'unsafe-inline', so a style attribute is dropped
// by the browser without any error: a bar simply does not render.
func TestNoInlineStylesAnywhere(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	for _, path := range []string{
		"/", "/disk", "/naming", "/media", "/quality",
		"/subtitles", "/torrents", "/settings",
	} {
		resp := ts.get(t, path, nil)
		if resp.Status != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.Status)
			continue
		}
		if strings.Contains(resp.Body, `style="`) {
			t.Errorf("%s has an inline style, which the CSP silently drops", path)
		}
	}
}
