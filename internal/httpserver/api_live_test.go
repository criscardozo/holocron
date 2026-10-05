package httpserver

import (
	"encoding/json"
	"testing"

	"github.com/cristian/holocron/internal/settings"
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

func TestTheAppGetsTheStartPage(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	if _, err := ts.db.Exec(`INSERT INTO media_items (path, type, title, year, server_item_id, has_subs_es, provider_ids)
		VALUES ('/m/Dune (2021)', 'movie', 'Dune', 2021, ?, 1, '{}')`, testPosterID); err != nil {
		t.Fatal(err)
	}
	token, err := ts.deps.APIToken.Generate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	auth := map[string]string{"Authorization": "Bearer " + token}

	var home struct {
		Machine string `json:"machine"`
		Tiles   []struct {
			Href, Title, Value string
		} `json:"tiles"`
		Mural []string `json:"mural"`
	}
	if err := json.Unmarshal([]byte(ts.get(t, "/api/v1/home", auth).Body), &home); err != nil {
		t.Fatal(err)
	}
	if home.Machine == "" || len(home.Tiles) != 8 || home.Tiles[0].Href != "/activity" {
		t.Errorf("home = %+v", home)
	}
	if len(home.Mural) != 1 || home.Mural[0] != "/art/jf/"+testPosterID {
		t.Errorf("mural = %v, want the one poster once", home.Mural)
	}

	// Linked, so the inventory is served.
	ts.deps.Settings.Manage(settings.Credentials{settings.CredJellyfin: "server-key"})
	var media struct {
		Items []struct{ Art string } `json:"items"`
	}
	if err := json.Unmarshal([]byte(ts.get(t, "/api/v1/media", auth).Body), &media); err != nil {
		t.Fatal(err)
	}
	if len(media.Items) != 1 || media.Items[0].Art != "/art/jf/"+testPosterID {
		t.Errorf("media items = %+v", media.Items)
	}
}
