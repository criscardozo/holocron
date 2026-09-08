package trailers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Holocron ships as one static binary and assumes no toolchain on the Pi, and
// this is the one place that breaks the rule. Reading YouTube in pure Go is not
// a thing that stays working: the extractors change often enough that yt-dlp
// exists as a full-time project to keep up with them. So the dependency is
// real, external, and will break on its own schedule.
//
// What that buys in design terms: it must be treated as absent by default,
// never as a given. Every entry point checks, and the screen says plainly which
// of "not installed", "too old" or "YouTube changed again" it is, because those
// have three different fixes and an unexplained failure sends someone reading
// Holocron's logs for a problem that is not Holocron's.

// ErrNoYTDLP means the tool is not installed.
var ErrNoYTDLP = errors.New("yt-dlp is not installed")

// ErrExtractorBroken means yt-dlp ran and YouTube refused it, which in practice
// means the installed version has fallen behind.
var ErrExtractorBroken = errors.New("yt-dlp could not read YouTube")

// ErrTooLowRes means the video has nothing above the resolution floor. Its own
// error because it is a property of that candidate rather than a failure: the
// answer is to try the next one, not to give up on the film.
var ErrTooLowRes = errors.New("no acceptable resolution")

// ErrGone means YouTube no longer has the video. Also per-candidate: an old
// trailer that was taken down says nothing about the next result.
var ErrGone = errors.New("the video is not available")

// Two floors, not one, because a single number cannot be right here.
//
// The library already holds trailers at 320x240 and 450x360: they pass every
// filter based on duration and title, and are useless on a television. But a
// hard floor high enough to exclude them would also exclude the genuinely old
// trailers that only ever existed at 360p — which are exactly the older films
// this feature exists for, so it would fail hardest where it is needed most.
//
// So every candidate is offered the preferred floor first, and only if none of
// them clears it does a second pass accept the hard floor. A 1080p upload of
// the same trailer wins over a 360p one even when the 360p one scored better on
// its title, which is the case that made this necessary: searching "Antes de
// amanecer" returns a 360p result above a 1080p one.
const (
	preferHeight = 480
	minHeight    = 360
)

// Runner runs yt-dlp. The binary is resolved once so a PATH change mid-run
// cannot switch which program is being executed.
type Runner struct {
	bin string
	// timeout bounds a single yt-dlp invocation. A search that hangs would
	// otherwise hold a job open forever on a machine with one job per kind.
	timeout time.Duration
}

// NewRunner locates yt-dlp. It returns a Runner even when the tool is missing,
// so callers can report that as a state rather than as a startup failure.
func NewRunner() *Runner {
	bin, err := exec.LookPath("yt-dlp")
	if err != nil {
		bin = ""
	}
	return &Runner{bin: bin, timeout: 5 * time.Minute}
}

// Available reports whether yt-dlp was found.
func (r *Runner) Available() bool { return r.bin != "" }

// Path is where the binary was found, for the screen to show.
func (r *Runner) Path() string { return r.bin }

// Version returns what yt-dlp reports, which is a date like "2026.08.19".
func (r *Runner) Version(ctx context.Context) (string, error) {
	if !r.Available() {
		return "", ErrNoYTDLP
	}
	out, err := r.run(ctx, "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Stale reports whether the installed version is old enough to be the likely
// cause of a failure. Not a hard limit: yt-dlp releases often and being a few
// months behind is normal, but a year behind explains a broken extractor.
func Stale(version string, now time.Time) bool {
	t, err := time.Parse("2006.01.02", strings.TrimSpace(version))
	if err != nil {
		return false // unparseable is not evidence of anything
	}
	return now.Sub(t) > 180*24*time.Hour
}

// Search asks YouTube for n results and returns what came back.
func (r *Runner) Search(ctx context.Context, query string, n int) ([]Candidate, error) {
	if !r.Available() {
		return nil, ErrNoYTDLP
	}
	out, err := r.run(ctx,
		fmt.Sprintf("ytsearch%d:%s", n, query),
		"--skip-download", "--dump-json", "--no-warnings", "--ignore-errors",
		"--flat-playlist", "--no-playlist",
	)
	if err != nil {
		return nil, err
	}

	var cands []Candidate
	sc := bufio.NewScanner(bytes.NewReader(out))
	// yt-dlp emits one JSON object per line and they are large.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var j struct {
			ID       string  `json:"id"`
			Title    string  `json:"title"`
			Channel  string  `json:"channel"`
			Uploader string  `json:"uploader"`
			Duration float64 `json:"duration"`
		}
		if err := json.Unmarshal(sc.Bytes(), &j); err != nil {
			continue // a warning line rather than a result
		}
		if j.ID == "" {
			continue
		}
		ch := j.Channel
		if ch == "" {
			ch = j.Uploader
		}
		cands = append(cands, Candidate{
			ID: j.ID, Title: j.Title, Channel: ch, Duration: int(j.Duration),
		})
	}
	return cands, sc.Err()
}

// Download fetches url into dir as "<stem>-trailer.<ext>".
//
// The name comes from the folder, never from the video's own title. The
// library already carries files called "A MAN CALLED OTTO  Trailer oficial
// Subtitulos Español Latinoamericano-trailer.mp4" from doing it the other way,
// and reintroducing that in the release that cleans names up would be absurd.
func (r *Runner) Download(ctx context.Context, url, dir, stem string, floor int) error {
	if !r.Available() {
		return ErrNoYTDLP
	}
	// The resolution floor lives in the format selector rather than in the
	// candidate filter, because a flat search does not report resolution at
	// all. When nothing clears the floor, yt-dlp answers "Requested format is
	// not available", which becomes ErrTooLowRes and sends the caller to the
	// next candidate.
	h := itoa(floor)
	_, err := r.run(ctx,
		url,
		"-f", "bv*[height>="+h+"][height<=1080]+ba/b[height>="+h+"][height<=1080]/bv*[height>="+h+"]+ba/b[height>="+h+"]",
		"--merge-output-format", "mp4",
		"-o", stem+"-trailer.%(ext)s",
		"-P", dir,
		"--no-playlist", "--no-warnings",
		"--retries", "3", "--fragment-retries", "3",
		// Never leave a half file behind wearing the final name.
		"--no-part-file=false",
	)
	return err
}

// run executes yt-dlp with a fixed binary and arguments passed as a slice, so
// nothing here goes through a shell and a film title cannot become a command.
func (r *Runner) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.bin, args...) //#nosec G204 -- bin is resolved by LookPath, never from input; args are a slice, so no shell
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		switch {
		case strings.Contains(msg, "Requested format is not available"):
			return nil, ErrTooLowRes
		case strings.Contains(msg, "Video unavailable"),
			strings.Contains(msg, "is not available"),
			strings.Contains(msg, "has been removed"),
			strings.Contains(msg, "Private video"):
			return nil, ErrGone
		}
		if strings.Contains(msg, "needs to be reloaded") ||
			strings.Contains(msg, "Unable to extract") ||
			strings.Contains(msg, "Sign in to confirm") ||
			strings.Contains(msg, "nsig extraction failed") {
			return nil, fmt.Errorf("%w: %s", ErrExtractorBroken, firstLine(msg))
		}
		// stdout can still hold usable results when --ignore-errors was set.
		if stdout.Len() > 0 {
			return stdout.Bytes(), nil
		}
		return nil, fmt.Errorf("yt-dlp: %w: %s", err, firstLine(msg))
	}
	return stdout.Bytes(), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
