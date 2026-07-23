package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientFlow(t *testing.T) {
	t.Parallel()
	var addedMagnet string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("SID"); err != nil {
			t.Error("missing session cookie on info")
		}
		_, _ = w.Write([]byte(`[{"hash":"h1","name":"Ubuntu ISO","state":"downloading",
			"progress":0.42,"size":1000,"dlspeed":2048,"upspeed":128,"num_seeds":5,"num_leechs":2}]`))
	})
	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		addedMagnet = r.PostFormValue("urls")
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/pause", func(w http.ResponseWriter, _ *http.Request) {})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	c, err := New(srv.URL, "admin", "pass")
	if err != nil {
		t.Fatal(err)
	}

	list, err := c.Torrents(ctx)
	if err != nil {
		t.Fatalf("Torrents: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Ubuntu ISO" || list[0].DlSpeed != 2048 {
		t.Fatalf("unexpected torrents: %+v", list)
	}

	if err := c.AddMagnet(ctx, "magnet:?xt=urn:btih:demo", ""); err != nil {
		t.Fatalf("AddMagnet: %v", err)
	}
	if addedMagnet != "magnet:?xt=urn:btih:demo" {
		t.Errorf("added magnet = %q", addedMagnet)
	}

	if err := c.Pause(ctx, "h1"); err != nil {
		t.Errorf("Pause: %v", err)
	}
}
