package httpserver

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/trailers"
)

// TestTrailersExplainsWhenTheToolIsMissing. yt-dlp is not installed on the
// machine the tests run on, which is the same state a fresh Pi is in — and the
// page has to say that plainly rather than fail with something generic.
func TestTrailersExplainsWhenTheToolIsMissing(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	body := ts.get(t, "/trailers", nil).Body
	if !strings.Contains(body, "yt-dlp no está instalado") {
		t.Errorf("the page does not explain the missing tool: %q", body)
	}
	// And it still says what it can do without it.
	if !strings.Contains(body, "qué películas no tienen trailer") {
		t.Error("it should still offer to list the films")
	}
}

// TestFetchingWithoutTheToolIsRefusedClearly rather than starting a job that
// fails 181 times.
func TestFetchingWithoutTheToolIsRefusedClearly(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	resp := ts.post(t, "/trailers/fetch", url.Values{"film": {"/mnt/x/Dune (2021)"}}, nil)
	if !strings.Contains(resp.Body, "yt-dlp no está instalado") {
		t.Errorf("expected a clear refusal, got %q", resp.Body)
	}
}

// TestFetchingNothingIsRefused covers the empty submit.
func TestFetchingNothingIsRefused(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	resp := ts.post(t, "/trailers/fetch", url.Values{}, nil)
	if !strings.Contains(resp.Body, "No tildaste ninguna película") {
		t.Errorf("expected the empty-selection notice, got %q", resp.Body)
	}
}

// absentYTDLP is the tool as a fresh Pi has it: not there. Used instead of the
// real lookup so the tests do not depend on whether the machine running them
// happens to have yt-dlp installed — this one does, the Pi did not, and a test
// that flips between the two is worse than no test.
type absentYTDLP struct{}

func (absentYTDLP) Available() bool { return false }
func (absentYTDLP) Path() string    { return "" }
func (absentYTDLP) Version(context.Context) (string, error) {
	return "", trailers.ErrNoYTDLP
}
func (absentYTDLP) Search(context.Context, string, int) ([]trailers.Candidate, error) {
	return nil, trailers.ErrNoYTDLP
}
func (absentYTDLP) Download(context.Context, string, string, string) error {
	return trailers.ErrNoYTDLP
}
