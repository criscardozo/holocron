package httpserver

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleSubtitlesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	view := templates.SubtitlesPageView{Configured: s.deps.Subtitles.Configured(ctx)}
	if !view.Configured {
		s.render(w, r, templates.SubtitlesPage(view))
		return
	}
	items, err := s.deps.Subtitles.MissingItems(ctx, 200)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for i, it := range items {
		rid := fmt.Sprintf("subres-%d", i)
		q := url.Values{}
		q.Set("title", it.Title)
		q.Set("year", strconv.Itoa(it.Year))
		q.Set("path", it.Path)
		view.Rows = append(view.Rows, templates.SubtitleMissingRow{
			Title:      it.Title,
			Year:       it.Year,
			Type:       it.Type,
			ResultsID:  rid,
			SearchHref: "/subtitles/search?" + q.Encode(),
		})
	}
	s.render(w, r, templates.SubtitlesPage(view))
}

func (s *Server) handleSubtitlesSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	title := q.Get("title")
	year, _ := strconv.Atoi(q.Get("year"))
	path := q.Get("path")

	results, err := s.deps.Subtitles.Search(r.Context(), title, year)
	if err != nil {
		s.log.Warn("subtitle search", "title", title, "error", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="error">No se pudo buscar en OpenSubtitles.</p>`))
		return
	}

	view := templates.SubtitleSearchView{Path: path, None: len(results) == 0}
	for _, res := range results {
		view.Results = append(view.Results, templates.SubtitleResultRow{
			FileID:   strconv.Itoa(res.FileID),
			FileName: res.FileName,
			Release:  res.Release,
			Language: res.Language,
		})
	}
	s.render(w, r, templates.SubtitleResults(view))
}

func (s *Server) handleSubtitlesDownload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	fileID, err := strconv.Atoi(r.PostFormValue("file_id"))
	if err != nil {
		http.Error(w, "bad file_id", http.StatusBadRequest)
		return
	}
	path := r.PostFormValue("path")

	if _, err := s.deps.Subtitles.Download(r.Context(), fileID, path); err != nil {
		s.log.Warn("subtitle download", "file_id", fileID, "error", err)
		s.render(w, r, templates.SubtitleDownloadResult(false, "No se pudo descargar."))
		return
	}
	s.render(w, r, templates.SubtitleDownloadResult(true, "Descargado."))
}
