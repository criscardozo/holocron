// Package artwork serves posters to the web UI from a cache on disk.
//
// The browser never talks to Jellyfin or TMDb: the CSP allows images only from
// Holocron itself, and Jellyfin wants a token that stays on the server. So the
// server fetches each poster once, scaled down, keeps it under the data
// directory, and serves it from there afterwards — the mural and the poster
// rows cost Jellyfin nothing after the first look.
package artwork

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Kinds of artwork, which are also the first segment of the URL.
const (
	KindJellyfin = "jf"   // a Jellyfin item id
	KindTMDb     = "tmdb" // a TMDb file name, for what is not in the library yet
)

// Width is the poster width asked of the source. Rows show them at about
// 140 px, so this covers a 2x screen.
const Width = 300

// maxAge is how long a cached poster is trusted. Jellyfin's can change after a
// metadata refresh; past this the next request fetches it again, and serves
// the old one if that fails.
const maxAge = 30 * 24 * time.Hour

// missTTL is how long a poster that does not exist is remembered as missing,
// so a mural with one bad id does not ask for it on every page.
const missTTL = time.Hour

// maxFetches caps concurrent requests to the sources. A first visit to the
// media grid asks for dozens of posters at once.
const maxFetches = 4

// maxBody caps a TMDb response read into memory.
const maxBody = 4 << 20

var (
	jellyfinID = regexp.MustCompile(`^[0-9a-f]{32}$`)
	tmdbFile   = regexp.MustCompile(`^[A-Za-z0-9_-]{4,64}\.(jpg|png)$`)
)

// ErrNotFound means there is no such poster, or the id is not one this
// package will ask for.
var ErrNotFound = errors.New("artwork: not found")

// JellyfinFunc fetches one item's poster at the given width.
type JellyfinFunc func(ctx context.Context, id string, width int) ([]byte, string, error)

// Image is a poster ready to serve.
type Image struct {
	Data []byte
	Type string
}

// Store fetches and caches posters.
type Store struct {
	dir      string
	jellyfin JellyfinFunc
	http     *http.Client
	tmdbBase string
	now      func() time.Time
	log      *slog.Logger

	slots chan struct{}

	mu      sync.Mutex
	pending map[string]*call
	misses  map[string]time.Time
}

type call struct {
	done chan struct{}
	img  Image
	err  error
}

// New returns a Store that caches under dir, creating it if needed.
func New(dir string, jf JellyfinFunc, log *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("artwork dir: %w", err)
	}
	return &Store{
		dir:      dir,
		jellyfin: jf,
		http:     &http.Client{Timeout: 15 * time.Second},
		tmdbBase: "https://image.tmdb.org/t/p/w300/",
		now:      time.Now,
		log:      log,
		slots:    make(chan struct{}, maxFetches),
		pending:  map[string]*call{},
		misses:   map[string]time.Time{},
	}, nil
}

// Valid reports whether id is something this package will fetch for kind.
// Everything else is refused before it gets near a file name or a URL.
func Valid(kind, id string) bool {
	switch kind {
	case KindJellyfin:
		return jellyfinID.MatchString(id)
	case KindTMDb:
		return tmdbFile.MatchString(id)
	}
	return false
}

// URL is the path the web UI uses for a poster, or "" when there is none.
func URL(kind, id string) string {
	if !Valid(kind, id) {
		return ""
	}
	return "/art/" + kind + "/" + id
}

// TMDbURL is URL for a TMDb poster path as Seerr reports it, "/abc.jpg".
func TMDbURL(path string) string {
	return URL(KindTMDb, strings.TrimPrefix(path, "/"))
}

// Get returns the poster, from disk when it can and from the source when it
// has to. Concurrent requests for the same poster share one fetch.
func (s *Store) Get(ctx context.Context, kind, id string) (Image, error) {
	if !Valid(kind, id) {
		return Image{}, ErrNotFound
	}
	name := fileName(kind, id)

	cached, fresh := s.read(name)
	if fresh {
		return cached, nil
	}

	s.mu.Lock()
	if at, ok := s.misses[name]; ok && s.now().Sub(at) < missTTL {
		s.mu.Unlock()
		return Image{}, ErrNotFound
	}
	if c, ok := s.pending[name]; ok {
		s.mu.Unlock()
		return c.wait(ctx)
	}
	c := &call{done: make(chan struct{})}
	s.pending[name] = c
	s.mu.Unlock()

	// Detached from the request: a visitor who navigates away should not
	// cancel a fetch that others are waiting on, and whose result is cached.
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	c.img, c.err = s.fetch(fctx, kind, id)
	cancel()
	if c.err == nil {
		// Written before the fetch is released, so whoever asks next finds
		// it on disk instead of starting another. A poster that fails to
		// cache is still a poster: it is served, and the failure logged.
		if err := s.write(name, c.img.Data); err != nil {
			s.log.Warn("artwork cache write failed", "file", name, "err", err)
		}
	}

	s.mu.Lock()
	delete(s.pending, name)
	if errors.Is(c.err, ErrNotFound) {
		s.misses[name] = s.now()
	}
	s.mu.Unlock()
	close(c.done)

	switch {
	case c.err == nil:
		return c.img, nil
	case cached.Data != nil:
		// Stale beats nothing: the source being down is no reason to lose a
		// poster that was right a month ago.
		return cached, nil
	}
	return Image{}, c.err
}

func (c *call) wait(ctx context.Context) (Image, error) {
	select {
	case <-c.done:
		return c.img, c.err
	case <-ctx.Done():
		return Image{}, ctx.Err()
	}
}

func (s *Store) fetch(ctx context.Context, kind, id string) (Image, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return Image{}, ctx.Err()
	}
	switch kind {
	case KindJellyfin:
		if s.jellyfin == nil {
			return Image{}, ErrNotFound
		}
		data, ct, err := s.jellyfin(ctx, id, Width)
		if err != nil {
			return Image{}, err
		}
		return Image{Data: data, Type: ct}, nil
	default:
		return s.tmdb(ctx, id)
	}
}

func (s *Store) tmdb(ctx context.Context, file string) (Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.tmdbBase+file, nil)
	if err != nil {
		return Image{}, fmt.Errorf("tmdb request: %w", err)
	}
	resp, err := s.http.Do(req) //#nosec G704 -- fixed host; file is checked against tmdbFile
	if err != nil {
		return Image{}, fmt.Errorf("tmdb %s: %w", file, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Image{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return Image{}, fmt.Errorf("tmdb %s: HTTP %d", file, resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return Image{}, fmt.Errorf("tmdb %s: content type %q", file, ct)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return Image{}, fmt.Errorf("tmdb %s: read: %w", file, err)
	}
	if len(data) > maxBody {
		return Image{}, fmt.Errorf("tmdb %s: larger than %d bytes", file, maxBody)
	}
	return Image{Data: data, Type: ct}, nil
}

// fileName is the cache file for a poster. Both id formats are validated to
// a plain name, and the kind prefix keeps the two apart.
func fileName(kind, id string) string {
	if kind == KindJellyfin {
		return kind + "-" + id + ".jpg"
	}
	return kind + "-" + id
}

func typeOf(name string) string {
	if strings.HasSuffix(name, ".png") {
		return "image/png"
	}
	return "image/jpeg"
}

// read returns the cached poster and whether it is still within maxAge.
func (s *Store) read(name string) (Image, bool) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return Image{}, false
	}
	defer func() { _ = root.Close() }()
	info, err := root.Stat(name)
	if err != nil {
		return Image{}, false
	}
	data, err := fs.ReadFile(root.FS(), name)
	if err != nil {
		return Image{}, false
	}
	return Image{Data: data, Type: typeOf(name)}, s.now().Sub(info.ModTime()) < maxAge
}

// write stores a poster atomically, so a reader never sees half a file.
func (s *Store) write(name string, data []byte) error {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	tmp := name + ".tmp"
	if err := root.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	if err := root.Rename(tmp, name); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	return nil
}
