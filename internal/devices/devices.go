// Package devices reads the house's devices: a TOML file the household edits
// by hand, and the photos beside it. Each device is named after a city, and
// its card shows that city and the device itself.
//
// The file and its photos live on the server, not in the repository: they hold
// the addresses and hardware ids of everything at home, and the repository is
// public. Everything is read through an os.Root on that directory, so a name in
// the file cannot reach anything outside it.
package devices

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"
)

// FileName is the devices file inside the directory.
const FileName = "dispositivos.toml"

// photoDir holds the photos, beside the file.
const photoDir = "fotos"

// GroupOthers is the group that shows as a plain list at the end: the
// network's plumbing (routers, a cat feeder) has no city and needs no photo.
const GroupOthers = "Otros"

// groupOrder is the order of the groups that have cards. Any other group
// follows in the order it first appears in the file; Otros always goes last.
var groupOrder = []string{"General", "Cristian", "Natalia"}

// photoExts are the photo formats looked for, in order.
var photoExts = []string{".jpg", ".jpeg", ".png", ".webp"}

// ErrNotConfigured means no devices directory was given.
var ErrNotConfigured = errors.New("devices directory not configured")

// Device is one entry of the file, with its photos resolved.
type Device struct {
	Name  string
	Group string
	Model string
	IP    string
	MAC   string
	// CityPhoto and DevicePhoto are file names inside the photo directory,
	// or "" when there is none.
	CityPhoto   string
	DevicePhoto string
}

// Group is a heading on the page and its devices, in file order.
type Group struct {
	Name    string
	Devices []Device
	// Plain is set for Otros, which shows as a list without photos.
	Plain bool
}

// file is the TOML as written. Field names are the Spanish keys Cristian
// edits; the struct is unexported so they stay a detail of this package.
type file struct {
	Dispositivo []struct {
		Nombre      string `toml:"nombre"`
		Grupo       string `toml:"grupo"`
		Dispositivo string `toml:"dispositivo"`
		IP          string `toml:"ip"`
		MAC         string `toml:"mac"`
		FotoCiudad  string `toml:"foto_ciudad"`
		FotoEquipo  string `toml:"foto_equipo"`
	} `toml:"dispositivo"`
}

// FileError is a mistake in the file, with the line it is on: the person
// who edits it by hand needs to know where to look.
type FileError struct {
	Line int
	err  error
}

func (e *FileError) Error() string {
	return fmt.Sprintf("parse %s, line %d: %v", FileName, e.Line, e.err)
}

func (e *FileError) Unwrap() error { return e.err }

// Store reads the devices directory.
type Store struct {
	dir string
}

// NewStore returns a Store over dir. An empty dir is allowed: the page then
// says how to set it up.
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Configured reports whether a directory was given.
func (s *Store) Configured() bool { return s != nil && s.dir != "" }

// Load reads the file and groups the devices. It is read on every call: the
// file is small, and an edit shows on the next page load without a restart.
func (s *Store) Load() ([]Group, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, fmt.Errorf("open devices dir: %w", err)
	}
	defer func() { _ = root.Close() }()

	raw, err := root.ReadFile(FileName)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", FileName, err)
	}
	devs, err := parse(raw)
	if err != nil {
		return nil, err
	}
	photos := listPhotos(root)
	for i := range devs {
		devs[i].CityPhoto = resolve(photos, devs[i].CityPhoto, slug(devs[i].Name))
		devs[i].DevicePhoto = resolve(photos, devs[i].DevicePhoto, slug(devs[i].Name)+"-equipo")
	}
	return group(devs), nil
}

// parse decodes the file. The photo fields carry what the file says, to be
// resolved against the directory afterwards.
func parse(raw []byte) ([]Device, error) {
	var f file
	if _, err := toml.Decode(string(raw), &f); err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return nil, &FileError{Line: pe.Position.Line, err: err}
		}
		return nil, fmt.Errorf("parse %s: %w", FileName, err)
	}
	out := make([]Device, 0, len(f.Dispositivo))
	for _, d := range f.Dispositivo {
		name := strings.TrimSpace(d.Nombre)
		if name == "" {
			continue
		}
		g := strings.TrimSpace(d.Grupo)
		if g == "" {
			g = groupOrder[0]
		}
		out = append(out, Device{
			Name:        name,
			Group:       g,
			Model:       strings.TrimSpace(d.Dispositivo),
			IP:          strings.TrimSpace(d.IP),
			MAC:         normaliseMAC(d.MAC),
			CityPhoto:   strings.TrimSpace(d.FotoCiudad),
			DevicePhoto: strings.TrimSpace(d.FotoEquipo),
		})
	}
	return out, nil
}

// group orders the devices into their groups: the known ones first, then the
// rest as they appear, then Otros.
func group(devs []Device) []Group {
	byName := map[string]*Group{}
	var order []string
	for _, d := range devs {
		g, ok := byName[d.Group]
		if !ok {
			g = &Group{Name: d.Group, Plain: strings.EqualFold(d.Group, GroupOthers)}
			byName[d.Group] = g
			order = append(order, d.Group)
		}
		g.Devices = append(g.Devices, d)
	}

	out := make([]Group, 0, len(order))
	taken := map[string]bool{}
	for _, name := range groupOrder {
		if g, ok := byName[name]; ok {
			out = append(out, *g)
			taken[name] = true
		}
	}
	var plain []Group
	for _, name := range order {
		if taken[name] {
			continue
		}
		if byName[name].Plain {
			plain = append(plain, *byName[name])
			continue
		}
		out = append(out, *byName[name])
	}
	return append(out, plain...)
}

// normaliseMAC writes a MAC address one way, whatever way it was typed:
// «02-00-5E-00-53-01» and «02:00:5e:00:53:01» are the same card.
func normaliseMAC(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "-", ":"))
}

// listPhotos is the photo directory's file names. A missing directory is just
// no photos.
func listPhotos(root *os.Root) map[string]bool {
	out := map[string]bool{}
	entries, err := fs.ReadDir(root.FS(), photoDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.Type().IsRegular() && IsPhoto(e.Name()) {
			out[e.Name()] = true
		}
	}
	return out
}

// resolve picks the photo for a device: the one the file names, if it
// exists, or else base with any of the known extensions.
func resolve(photos map[string]bool, named, base string) string {
	if named != "" {
		if photos[named] {
			return named
		}
		return ""
	}
	for _, ext := range photoExts {
		if photos[base+ext] {
			return base + ext
		}
	}
	return ""
}

// IsPhoto reports whether name is a plain file name with a photo extension:
// no directories, no dot files.
func IsPhoto(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return false
	}
	ext := strings.ToLower(path.Ext(name))
	for _, e := range photoExts {
		if ext == e {
			return true
		}
	}
	return false
}

// unaccent folds the accented letters a Spanish or Portuguese city name can
// carry. A table rather than Unicode normalisation, which would be one more
// dependency for a dozen letters.
var unaccent = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u", "â", "a", "ê", "e",
	"î", "i", "ô", "o", "û", "u", "ã", "a", "õ", "o", "ç", "c", "ä", "a",
	"ë", "e", "ï", "i", "ö", "o",
)

// slug is the photo name for a device: lower case, no accents, no spaces.
// «Caacupé» looks for caacupe.jpg, «Las Vegas» for lasvegas.jpg.
func slug(name string) string {
	var b strings.Builder
	for _, r := range unaccent.Replace(strings.ToLower(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Photo is a photo's bytes and when it last changed, for serving.
type Photo struct {
	Data    []byte
	ModTime time.Time
}

// ReadPhoto reads a photo by file name. The name must be a plain photo file
// name; the os.Root keeps it inside the photo directory either way. Photos
// are a few hundred KB, so reading one whole is simpler than streaming it.
func (s *Store) ReadPhoto(name string) (Photo, error) {
	if !s.Configured() {
		return Photo{}, ErrNotConfigured
	}
	if !IsPhoto(name) {
		return Photo{}, fs.ErrNotExist
	}
	root, err := os.OpenRoot(path.Join(s.dir, photoDir))
	if err != nil {
		return Photo{}, fmt.Errorf("open photo dir: %w", err)
	}
	defer func() { _ = root.Close() }()

	info, err := root.Stat(name)
	if err != nil {
		return Photo{}, fmt.Errorf("stat photo: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Photo{}, fs.ErrNotExist
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return Photo{}, fmt.Errorf("read photo: %w", err)
	}
	return Photo{Data: data, ModTime: info.ModTime()}, nil
}
