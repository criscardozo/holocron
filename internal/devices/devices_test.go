package devices

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDir(t *testing.T, toml string, photos ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, photoDir), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, p := range photos {
		if err := os.WriteFile(filepath.Join(dir, photoDir, p), []byte("img:"+p), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const sample = `
# a comment
[[dispositivo]]
nombre      = "Router"
grupo       = "Otros"
dispositivo = "Router TP-Link"   # trailing comment
ip          = "192.0.2.1"
mac         = "02-00-5E-00-53-AF"

[[dispositivo]]
nombre      = "Osaka"
grupo       = "Cristian"
dispositivo = "iPhone 17"

[[dispositivo]]
nombre      = "Caacupé"
dispositivo = "Aspiradora"

[[dispositivo]]
nombre      = "Tandil"
grupo       = "Abuela"

[[dispositivo]]
nombre      = "Ginebra"
grupo       = "General"
foto_ciudad = "otra.jpg"
`

func TestLoadGroupsInTheirOrder(t *testing.T) {
	t.Parallel()
	groups, err := NewStore(writeDir(t, sample)).Load()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	// The known groups first, an unknown one after them, Otros last whatever
	// its place in the file.
	if got, want := strings.Join(names, ","), "General,Cristian,Abuela,Otros"; got != want {
		t.Fatalf("groups = %s, want %s", got, want)
	}
	if !groups[3].Plain || groups[0].Plain {
		t.Error("only Otros is the plain list")
	}
	// No group means General, and file order holds inside a group.
	if got := groups[0].Devices[0].Name + "," + groups[0].Devices[1].Name; got != "Caacupé,Ginebra" {
		t.Errorf("General = %s, want Caacupé,Ginebra", got)
	}
	if got := groups[3].Devices[0].MAC; got != "02:00:5e:00:53:af" {
		t.Errorf("MAC = %q, want it lower case with colons", got)
	}
}

func TestPhotosAreFoundByName(t *testing.T) {
	t.Parallel()
	dir := writeDir(t, sample, "caacupe.jpg", "caacupe-equipo.png", "osaka.webp", "ginebra.jpg", "notes.txt")
	groups, err := NewStore(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Device{}
	for _, g := range groups {
		for _, d := range g.Devices {
			byName[d.Name] = d
		}
	}
	// The accent goes, the extension can be any of the known ones.
	if d := byName["Caacupé"]; d.CityPhoto != "caacupe.jpg" || d.DevicePhoto != "caacupe-equipo.png" {
		t.Errorf("Caacupé photos = %q, %q", d.CityPhoto, d.DevicePhoto)
	}
	if d := byName["Osaka"]; d.CityPhoto != "osaka.webp" || d.DevicePhoto != "" {
		t.Errorf("Osaka photos = %q, %q", d.CityPhoto, d.DevicePhoto)
	}
	// A photo the file names and that is not there is no photo, rather than
	// falling back to the default: the file said which one it wanted.
	if d := byName["Ginebra"]; d.CityPhoto != "" {
		t.Errorf("Ginebra city photo = %q, want none (otra.jpg is missing)", d.CityPhoto)
	}
}

func TestABrokenFileSaysWhere(t *testing.T) {
	t.Parallel()
	_, err := NewStore(writeDir(t, "[[dispositivo]]\nnombre = \"Osaka\ngrupo = \"x\"\n")).Load()
	if err == nil || !strings.Contains(err.Error(), "line") {
		t.Fatalf("err = %v, want one that names the line", err)
	}
}

func TestReadPhotoStaysInItsDirectory(t *testing.T) {
	t.Parallel()
	dir := writeDir(t, sample, "osaka.jpg")
	if err := os.WriteFile(filepath.Join(dir, "secret.jpg"), []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(dir)

	p, err := s.ReadPhoto("osaka.jpg")
	if err != nil || string(p.Data) != "img:osaka.jpg" {
		t.Fatalf("ReadPhoto = %q, %v", p.Data, err)
	}
	for _, name := range []string{"../secret.jpg", "..", ".hidden.jpg", "../" + FileName, "osaka.txt", ""} {
		if _, err := s.ReadPhoto(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadPhoto(%q) = %v, want not found", name, err)
		}
	}
	if _, err := NewStore("").ReadPhoto("osaka.jpg"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("unconfigured ReadPhoto = %v", err)
	}
}

func TestSlug(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"Caacupé": "caacupe", "LasVegas": "lasvegas", "Las Vegas": "lasvegas",
		"Alimentador de Merli": "alimentadordemerli", "São Paulo": "saopaulo",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
