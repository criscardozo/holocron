package trailers

import (
	"io/fs"
	"os"
	"path"
	"strings"
)

// Jellyfin recognises a trailer by the "-trailer" suffix before the extension,
// and that is what this library already uses. Worth stating because it is not
// what one might assume: the convention is "<name>-trailer.mp4" beside the
// film, not "<name>.trailer.mp4" and not a trailers/ subfolder. Measured on the
// real library rather than taken from the documentation.
const trailerSuffix = "-trailer"

// Missing is a film with no trailer file.
type Missing struct {
	// Folder is the film's folder name inside the media folder.
	Folder string
	// Title and Year come from parsing the folder name; Year is 0 when it has
	// none, which makes the search vaguer but does not stop it.
	Title string
	Year  int
}

// HasTrailer reports whether any file in the folder is a trailer.
func HasTrailer(root *os.Root, dir string) (bool, error) {
	entries, err := fs.ReadDir(root.FS(), dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		stem := strings.TrimSuffix(name, path.Ext(name))
		if strings.HasSuffix(strings.ToLower(stem), trailerSuffix) {
			return true, nil
		}
	}
	return false, nil
}
