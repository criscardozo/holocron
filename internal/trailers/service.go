package trailers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/naming"
	"github.com/cristian/holocron/internal/scanner"
)

// Job kinds.
const (
	KindScan  = "trailers-scan"
	KindFetch = "trailers-fetch"
)

// diskFloor is how much room has to be left before a download starts.
//
// Not a guess at how big the trailers are — they average 28 MB, so the whole
// batch is about 5 GB. It is a floor under the disk itself. This library sits
// on a volume that was at 97% and losing 25 GB a day while this was written, so
// a feature that writes to it needs to stop on its own rather than rely on
// somebody watching the number.
const diskFloor = 20 << 30 // 20 GiB

// ErrLowOnSpace means the disk is too full to be writing trailers to.
var ErrLowOnSpace = errors.New("not enough free space")

// Film is one film that has no trailer.
type Film struct {
	// Dir is the absolute folder, built from the configured media folder and a
	// directory entry — never from anything a request supplied.
	Dir    string
	Folder string
	Label  string
	Title  string
	Year   int
}

// Tool is everything the service needs from yt-dlp. An interface so the
// download path can be tested without a network and without the binary, which
// matters more here than usual: the binary is the part most likely to be
// missing or broken on the machine this runs on.
type Tool interface {
	Available() bool
	Path() string
	Version(ctx context.Context) (string, error)
	Search(ctx context.Context, query string, n int) ([]Candidate, error)
	Download(ctx context.Context, url, dir, stem string, floor int) error
}

// Service finds films without a trailer and fetches them.
type Service struct {
	folders *folders.Store
	jobs    *jobs.Manager
	runner  Tool

	mu      sync.Mutex
	missing []Film
	// results records what happened per folder in the last fetch, so the
	// screen can say which films were skipped and why rather than only how
	// many worked.
	results []Result
}

// Result is the outcome for one film.
type Result struct {
	Folder string
	// Trailer is the video that was taken, empty when none was.
	Trailer string
	Reason  string
	Err     string
}

// NewService creates a Service.
func NewService(fs *folders.Store, jm *jobs.Manager, r Tool) *Service {
	return &Service{folders: fs, jobs: jm, runner: r}
}

// ToolInfo reports what is known about yt-dlp, for the screen to explain
// itself: "not installed", "too old" and "YouTube changed again" have three
// different fixes.
func (s *Service) ToolInfo(ctx context.Context) (path, version string, stale bool) {
	if !s.runner.Available() {
		return "", "", false
	}
	v, err := s.runner.Version(ctx)
	if err != nil {
		return s.runner.Path(), "", false
	}
	return s.runner.Path(), v, Stale(v, time.Now())
}

// Missing returns the last scan.
func (s *Service) Missing() []Film {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Film, len(s.missing))
	copy(out, s.missing)
	return out
}

// Results returns the outcome of the last fetch.
func (s *Service) Results() []Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Result, len(s.results))
	copy(out, s.results)
	return out
}

// Scanning and Fetching report what is running.
func (s *Service) Scanning() bool { return s.jobs.IsRunning(KindScan) }
func (s *Service) Fetching() bool { return s.jobs.IsRunning(KindFetch) }

// LastScanJob and LastFetchJob expose the job records for the status fragment.
func (s *Service) LastScanJob() (jobs.Job, bool)  { return s.jobs.Latest(KindScan) }
func (s *Service) LastFetchJob() (jobs.Job, bool) { return s.jobs.Latest(KindFetch) }

// StartScan looks for films without a trailer.
func (s *Service) StartScan(ctx context.Context) error {
	_, err := s.jobs.Start(KindScan, func(ctx context.Context, p *jobs.Progress) (string, error) {
		restore := jobs.Deprioritise()
		defer restore()

		films, err := s.scan(ctx)
		if err != nil {
			return "", err
		}
		s.mu.Lock()
		s.missing = films
		s.mu.Unlock()

		if len(films) == 0 {
			return "Todas las películas tienen trailer.", nil
		}
		var noYear int
		for _, f := range films {
			if f.Year == 0 {
				noYear++
			}
		}
		msg := fmt.Sprintf("%d películas sin trailer", len(films))
		if noYear > 0 {
			msg += fmt.Sprintf("; %d sin año en el nombre, así que la búsqueda va a ser más vaga", noYear)
		}
		return msg, nil
	})
	return err
}

// scan walks the configured movie folders.
func (s *Service) scan(ctx context.Context) ([]Film, error) {
	roots, err := s.folders.List(ctx, folders.PurposeMovies)
	if err != nil {
		return nil, fmt.Errorf("list movie folders: %w", err)
	}

	var out []Film
	for _, f := range roots {
		root, err := os.OpenRoot(f.Path)
		if err != nil {
			continue
		}
		entries, err := readDir(root)
		if err != nil {
			_ = root.Close()
			continue
		}
		for _, e := range entries {
			// Hidden directories are never films, and listing them here would
			// offer to download a trailer for .Spotlight-V100. Same rule as
			// the naming scan, from the same place, so the two screens cannot
			// disagree about what counts as a film.
			if !e.IsDir() || naming.Hidden(e.Name()) {
				continue
			}
			if err := ctx.Err(); err != nil {
				_ = root.Close()
				return out, err
			}
			has, err := HasTrailer(root, e.Name())
			if err != nil || has {
				continue
			}
			title, year, _ := naming.Parse(e.Name())
			if title == "" {
				title = e.Name()
			}
			out = append(out, Film{
				Dir:    filepath.Join(f.Path, e.Name()),
				Folder: e.Name(),
				Label:  f.Label,
				Title:  title,
				Year:   year,
			})
		}
		_ = root.Close()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Folder < out[j].Folder })
	return out, nil
}

// StartFetch downloads a trailer for each of the named folders.
//
// The names are matched against the last scan rather than resolved as paths.
// That is the containment check: a folder Holocron did not itself find by
// reading a configured media folder is not a folder it will write into, so
// there is no path arithmetic between a request and a download.
func (s *Service) StartFetch(ctx context.Context, folderNames []string) error {
	if !s.runner.Available() {
		return ErrNoYTDLP
	}
	want := make(map[string]bool, len(folderNames))
	for _, n := range folderNames {
		want[n] = true
	}
	var films []Film
	for _, f := range s.Missing() {
		if want[f.Dir] {
			films = append(films, f)
		}
	}
	if len(films) == 0 {
		return errors.New("nothing selected")
	}

	_, err := s.jobs.Start(KindFetch, func(ctx context.Context, p *jobs.Progress) (string, error) {
		restore := jobs.Deprioritise()
		defer restore()
		return s.fetch(ctx, films, p)
	})
	return err
}

func (s *Service) fetch(ctx context.Context, films []Film, p *jobs.Progress) (string, error) {
	var results []Result
	var got, failed int

	for i, f := range films {
		if err := ctx.Err(); err != nil {
			break
		}
		p.Set(i * 100 / len(films))

		// Checked before every single download, not once at the start. The
		// batch takes a long time and something else is writing to this disk.
		if free, err := scanner.AvailableBytes(f.Dir); err == nil && free < diskFloor {
			results = append(results, Result{
				Folder: f.Folder,
				Err:    "se paró: quedan menos de 20 GB libres en el disco",
			})
			break
		}

		cands, err := Find(ctx, s.runner, f.Title, f.Year)
		if err != nil {
			failed++
			results = append(results, Result{Folder: f.Folder, Err: describe(err)})
			if errors.Is(err, ErrExtractorBroken) || errors.Is(err, ErrNoYTDLP) {
				// The next film will fail the same way. Stopping keeps the
				// message honest instead of repeating it 180 times.
				break
			}
			continue
		}

		// Candidates are tried in order because the search cannot see
		// everything that disqualifies one: a video below the resolution
		// floor, or one YouTube has since removed, only announces itself when
		// the download is attempted. Neither says anything about the next.
		res, stop := s.tryCandidates(ctx, f, cands)
		results = append(results, res)
		if res.Err == "" {
			got++
		} else {
			failed++
		}
		if stop {
			break
		}
	}

	s.mu.Lock()
	s.results = results
	s.mu.Unlock()

	// Re-scan so the list stops offering what is already done.
	if films, err := s.scan(ctx); err == nil {
		s.mu.Lock()
		s.missing = films
		s.mu.Unlock()
	}

	msg := fmt.Sprintf("%d trailers bajados", got)
	if failed > 0 {
		msg += fmt.Sprintf("; %d sin resultado", failed)
	}
	return msg, nil
}

// tryCandidates downloads the first candidate that works. stop is true when
// the failure is one that will repeat for every remaining film, in which case
// the batch is over rather than 180 identical lines long.
func (s *Service) tryCandidates(ctx context.Context, f Film, cands []Found) (Result, bool) {
	var last error
	// Every candidate at the preferred resolution before any of them at the
	// hard floor. The other order would take a 360p upload of the right
	// trailer over a 1080p one further down the list, which is the case this
	// exists for.
	for _, floor := range []int{preferHeight, minHeight} {
		for _, c := range cands {
			if err := ctx.Err(); err != nil {
				return Result{Folder: f.Folder, Err: describe(err)}, true
			}
			err := s.runner.Download(ctx, c.Candidate.URL(), f.Dir, Stem(f.Folder), floor)
			switch {
			case err == nil:
				return Result{Folder: f.Folder, Trailer: c.Candidate.Title, Reason: c.Reason}, false
			case errors.Is(err, ErrExtractorBroken), errors.Is(err, ErrNoYTDLP):
				return Result{Folder: f.Folder, Err: describe(err)}, true
			}
			last = err
		}
	}
	return Result{Folder: f.Folder, Err: describe(last)}, false
}

// describe turns an error into something worth showing. The three failure modes
// have three different fixes, and an unexplained one sends the user reading
// Holocron's logs for a problem that is not Holocron's.
func describe(err error) string {
	switch {
	case errors.Is(err, ErrNoYTDLP):
		return "yt-dlp no está instalado en la Pi"
	case errors.Is(err, ErrExtractorBroken):
		return "yt-dlp no pudo leer YouTube — casi seguro la versión quedó vieja"
	case errors.Is(err, ErrNotFound):
		return "no se encontró nada que sea el trailer de esta película"
	case errors.Is(err, ErrTooLowRes):
		return "todos los candidatos están por debajo de 360p, no vale la pena bajarlos"
	case errors.Is(err, ErrGone):
		return "los videos encontrados ya no están en YouTube"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "se canceló"
	default:
		return "falló la descarga"
	}
}

// readDir lists the top level of root.
func readDir(root *os.Root) ([]os.DirEntry, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return f.ReadDir(-1)
}
