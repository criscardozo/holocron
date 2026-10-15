package httpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/devices"
)

// testDevicesDir is a devices directory with one card, one plain entry and
// one photo.
func testDevicesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	const file = `
[[dispositivo]]
nombre      = "Caacupé"
dispositivo = "ECOVACS DEEBOT T50 Pro Omni"
ip          = "192.0.2.185"
mac         = "02-00-5E-00-53-01"

[[dispositivo]]
nombre      = "Router"
grupo       = "Otros"
dispositivo = "Router TP-Link"
ip          = "192.0.2.1"
`
	if err := os.WriteFile(filepath.Join(dir, devices.FileName), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "fotos"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fotos", "caacupe.jpg"), []byte("\xff\xd8\xffjpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, devices.FileName+".jpg"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDevicesPage(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	r := ts.get(t, "/devices", nil)
	if r.Status != 200 {
		t.Fatalf("status %d", r.Status)
	}
	for _, want := range []string{
		"Caacupé", "ECOVACS DEEBOT T50 Pro Omni", "192.0.2.185",
		// The MAC is folded and written one way, whatever the file said.
		`<details class="dv-mac">`, "02:00:5e:00:53:01",
		`src="/devices/photo/caacupe.jpg"`,
		// No device photo: the empty tile, not a broken image.
		"dv-tile-empty",
		// Otros is a table at the end.
		`class="table dv-table"`, "Router TP-Link",
	} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Index(r.Body, "Router TP-Link") < strings.Index(r.Body, "Caacupé") {
		t.Error("Otros must come after the cards")
	}
}

func TestDevicePhotoStaysInItsFolder(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	if r := ts.get(t, "/devices/photo/caacupe.jpg", nil); r.Status != 200 || r.Body != "\xff\xd8\xffjpeg" {
		t.Fatalf("photo: %d %q", r.Status, r.Body)
	}
	for _, p := range []string{
		"/devices/photo/..%2F" + devices.FileName + ".jpg",
		"/devices/photo/" + devices.FileName,
		"/devices/photo/nope.jpg",
	} {
		if r := ts.get(t, p, nil); r.Status != 404 {
			t.Errorf("%s: status %d, want 404", p, r.Status)
		}
	}
}
