package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/internal/torrents"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleTorrentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	view := templates.TorrentsPageView{Configured: s.deps.Torrents.Configured(ctx)}
	if view.Configured {
		view = s.torrentsView(ctx)
		view.Configured = true
	}
	s.render(w, r, templates.TorrentsPage(view))
}

func (s *Server) handleTorrentsList(w http.ResponseWriter, r *http.Request) {
	view := s.torrentsView(r.Context())
	view.Configured = true
	s.render(w, r, templates.TorrentsTable(view))
}

func (s *Server) torrentsView(ctx context.Context) templates.TorrentsPageView {
	view := templates.TorrentsPageView{}
	list, err := s.deps.Torrents.List(ctx)
	if err != nil {
		s.log.Warn("list torrents", "error", err)
		view.Err = "No se pudo conectar con qBittorrent."
		return view
	}
	for _, t := range list {
		state := strings.ToLower(t.State)
		view.Rows = append(view.Rows, templates.TorrentRow{
			Hash:      t.Hash,
			Name:      t.Name,
			State:     t.State,
			Percent:   int(t.Progress * 100),
			SizeHuman: system.HumanBytes(uint64(t.Size)),
			DlHuman:   system.HumanBytes(uint64(t.DlSpeed)) + "/s",
			UpHuman:   system.HumanBytes(uint64(t.UpSpeed)) + "/s",
			Seeds:     t.NumSeeds,
			Leechs:    t.NumLeechs,
			Paused:    strings.Contains(state, "paused") || strings.Contains(state, "stopped"),
		})
	}
	return view
}

func (s *Server) handleTorrentsAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	hash := r.PostFormValue("hash")
	var err error
	switch r.PostFormValue("action") {
	case "pause":
		err = s.deps.Torrents.Pause(ctx, hash)
	case "resume":
		err = s.deps.Torrents.Resume(ctx, hash)
	case "delete":
		err = s.deps.Torrents.Delete(ctx, hash)
	default:
		http.Error(w, "bad action", http.StatusBadRequest)
		return
	}
	if err != nil {
		s.log.Warn("torrent action", "hash", hash, "error", err)
	}

	view := s.torrentsView(ctx)
	view.Configured = true
	s.render(w, r, templates.TorrentsTable(view))
}

func (s *Server) handleTorrentsAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	err := s.deps.Torrents.AddMagnet(r.Context(), r.PostFormValue("magnet"), r.PostFormValue("category"))
	switch {
	case errors.Is(err, torrents.ErrInvalidMagnet):
		s.render(w, r, templates.TorrentAddResult(false, "El texto no es un magnet-link válido."))
	case err != nil:
		s.log.Warn("add magnet", "error", err)
		s.render(w, r, templates.TorrentAddResult(false, "No se pudo agregar."))
	default:
		s.render(w, r, templates.TorrentAddResult(true, "Agregado."))
	}
}
