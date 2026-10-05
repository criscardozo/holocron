package httpserver

import (
	"encoding/json"
	"testing"
)

// TestTheLiveScreensAreInTheAPI: the app reads the same formatted views the web
// renders, so the words live in one place.
func TestTheLiveScreensAreInTheAPI(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := tokenFor(t, ts)
	auth := map[string]string{"Authorization": "Bearer " + token}

	for path, keys := range map[string][]string{
		"/api/v1/hardware": {"live", "cores", "ram", "links", "disks", "battery"},
		"/api/v1/activity": {"playing", "downloads", "hasJellyfin", "attention"},
		"/api/v1/services": {"configured", "units", "timers", "disks"},
		"/api/v1/manage":   {"remote", "machine", "actions"},
	} {
		resp := ts.get(t, path, auth)
		if resp.Status != 200 {
			t.Errorf("%s = %d", path, resp.Status)
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(resp.Body), &m); err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Errorf("%s has no %q: %s", path, k, resp.Body)
			}
		}
	}
}
