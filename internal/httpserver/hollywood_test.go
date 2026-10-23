package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHollywoodPage(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	// No launcher address: the page is the settings and the way in, with no
	// claim about whether it answers.
	body := ts.get(t, "/hollywood", nil).Body
	for _, want := range []string{"Configuración del launcher", `href="https://hollywood.`, "YouTube", "Seerr"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(body, "no responde") || strings.Contains(body, ">responde<") {
		t.Error("without an address the page must not say whether it answers")
	}
}

func TestLauncherAnswers(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(up.Close)
	if !launcherAnswers(t.Context(), up.URL) {
		t.Error("a launcher that answers must read as up")
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	t.Cleanup(down.Close)
	if launcherAnswers(t.Context(), down.URL) {
		t.Error("a 502 is not an answer")
	}
	if launcherAnswers(t.Context(), "http://127.0.0.1:1/") {
		t.Error("nothing listening is not an answer")
	}
}
