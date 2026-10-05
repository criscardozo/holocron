package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cristian/holocron/internal/db"
)

func TestCredentialsFromSystemdAreReadOnlySettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"jellyfin":    "abc123",
		"qbittorrent": "admin\nsecreto\n",
		"radarr":      "rk",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	creds, err := LoadCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	st := NewStore(database)
	st.Manage(creds)
	ctx := t.Context()

	for key, want := range map[string]string{
		KeyJellyfinToken: "abc123",
		KeyQbitUser:      "admin",
		KeyQbitPass:      "secreto",
		// Loopback by default, where Ginebra runs them.
		KeyJellyfinURL: "http://127.0.0.1:8096",
		KeyQbitURL:     "http://127.0.0.1:8080",
	} {
		if got := st.GetDefault(ctx, key, ""); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if v, ok := st.Credential(CredRadarr); !ok || v != "rk" {
		t.Errorf("radarr credential = %q, %v", v, ok)
	}
	// The form cannot overwrite what the server provides.
	if err := st.Set(ctx, KeyJellyfinToken, "otro"); !errors.Is(err, ErrManaged) {
		t.Errorf("Set on a managed key: %v, want ErrManaged", err)
	}
	// But a hand-saved address still wins over the loopback default.
	if err := st.Set(ctx, KeyJellyfinURL, "http://10.0.0.5:8096"); err != nil {
		t.Fatal(err)
	}
	if got := st.GetDefault(ctx, KeyJellyfinURL, ""); got != "http://10.0.0.5:8096" {
		t.Errorf("saved URL lost to the default: %q", got)
	}
}

// Outside Ginebra there is no credentials directory, and that must be a
// normal start rather than an error.
func TestNoCredentialsDirectoryIsFine(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{"", filepath.Join(t.TempDir(), "no-existe")} {
		c, err := LoadCredentials(dir)
		if err != nil || len(c) != 0 {
			t.Errorf("LoadCredentials(%q) = %v, %v", dir, c, err)
		}
	}
}
