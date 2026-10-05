package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cristian/holocron/internal/settings"
)

// TestNoQuickConnectOverTheServersKey. On Ginebra the server hands Holocron
// Jellyfin's key. Quick Connect used to start anyway, the approval to go
// through, and storing the token to fail — so the app waited for an approval
// it had already been given. There is nothing to link, and the API says so.
func TestNoQuickConnectOverTheServersKey(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.deps.Settings.Manage(settings.Credentials{settings.CredJellyfin: "server-key"})
	token, err := ts.deps.APIToken.Generate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	auth := map[string]string{"Authorization": "Bearer " + token}

	start := ts.post(t, "/api/v1/jellyfin/link", nil, auth)
	if start.Status != http.StatusConflict {
		t.Errorf("start: status %d, want 409; body %s", start.Status, start.Body)
	}

	status := ts.get(t, "/api/v1/jellyfin/link", auth)
	var got struct {
		State   string `json:"state"`
		Managed bool   `json:"managed"`
		Admin   bool   `json:"admin"`
	}
	if err := json.Unmarshal([]byte(status.Body), &got); err != nil {
		t.Fatalf("status body %q: %v", status.Body, err)
	}
	if got.State != "linked" || !got.Managed || !got.Admin {
		t.Errorf("status = %+v, want linked by the server's key", got)
	}
}
