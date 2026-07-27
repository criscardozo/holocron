package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/cristian/holocron/internal/apitoken"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/library"
	"github.com/cristian/holocron/internal/plexauth"
	"github.com/cristian/holocron/internal/scanner"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/internal/torrents"
)

// The JSON API backs the iOS app. It is versioned under /api/v1 so the app can
// keep working while the HTML UI evolves, and it is the only part of the server
// that requires authentication (see requireAPIToken).

// apiRoutes registers the JSON API on mux.
func (s *Server) apiRoutes(mux *http.ServeMux) {
	api := http.NewServeMux()

	api.HandleFunc("GET /v1/system", s.apiSystem)

	api.HandleFunc("GET /v1/disk", s.apiDiskList)
	api.HandleFunc("GET /v1/disk/{id}", s.apiDiskDetail)
	api.HandleFunc("GET /v1/disk/{id}/browse", s.apiDiskBrowse)
	api.HandleFunc("POST /v1/disk/{id}/scan", s.apiDiskScan)

	api.HandleFunc("GET /v1/naming", s.apiNaming)
	api.HandleFunc("POST /v1/naming/scan", s.apiNamingScan)

	api.HandleFunc("POST /v1/plex/link", s.apiPlexLinkStart)
	api.HandleFunc("GET /v1/plex/link", s.apiPlexLinkStatus)
	api.HandleFunc("POST /v1/plex/link/server", s.apiPlexLinkServer)

	api.HandleFunc("GET /v1/media", s.apiMedia)
	api.HandleFunc("POST /v1/media/sync", s.apiMediaSync)
	api.HandleFunc("POST /v1/media/nfo", s.apiMediaNFO)

	api.HandleFunc("GET /v1/subtitles", s.apiSubtitles)
	api.HandleFunc("GET /v1/subtitles/search", s.apiSubtitleSearch)
	api.HandleFunc("POST /v1/subtitles/download", s.apiSubtitleDownload)

	api.HandleFunc("GET /v1/torrents", s.apiTorrents)
	api.HandleFunc("POST /v1/torrents", s.apiTorrentAdd)
	api.HandleFunc("POST /v1/torrents/{hash}/{action}", s.apiTorrentAction)

	mux.Handle("/api/", http.StripPrefix("/api", s.requireAPIToken(api)))
}

// requireAPIToken rejects API requests without a valid bearer token. Unlike the
// HTML UI (trusted LAN, no auth by design), the API is reachable from a phone
// that roams outside the LAN, so it is always authenticated.
func (s *Server) requireAPIToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := apitoken.BearerToken(r.Header.Get("Authorization"))
		if token == "" {
			s.apiError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		if err := s.deps.APIToken.Verify(r.Context(), token); err != nil {
			if errors.Is(err, apitoken.ErrNoToken) {
				s.apiError(w, http.StatusServiceUnavailable,
					"no API token configured; generate one in Ajustes")
				return
			}
			s.apiError(w, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON encodes v as the response body.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status and part of the body are already on the wire.
		s.log.Error("encode api response", "error", err)
	}
}

// apiError returns a generic, machine-readable error. Detail stays in the log.
func (s *Server) apiError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}

// apiFailure logs the cause and returns a generic 500.
func (s *Server) apiFailure(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("api handler", "path", r.URL.Path, "error", err)
	s.apiError(w, http.StatusInternalServerError, "internal error")
}

// ── system ──────────────────────────────────────────────────────────────

type apiSystemResponse struct {
	CPUPercent  *float64 `json:"cpuPercent"`
	MemUsed     *uint64  `json:"memUsedBytes"`
	MemTotal    *uint64  `json:"memTotalBytes"`
	MemPercent  *float64 `json:"memPercent"`
	TempCelsius *float64 `json:"tempCelsius"`
	UptimeSecs  *int64   `json:"uptimeSeconds"`
	Load1       *float64 `json:"load1"`
	Hostname    string   `json:"hostname"`
}

func (s *Server) apiSystem(w http.ResponseWriter, _ *http.Request) {
	st := system.Read()
	hostname, _ := os.Hostname() // cosmetic; an empty name is fine
	resp := apiSystemResponse{Hostname: hostname}
	if st.HasCPU {
		resp.CPUPercent = &st.CPUPercent
	}
	if st.MemTotal > 0 {
		resp.MemUsed, resp.MemTotal, resp.MemPercent = &st.MemUsed, &st.MemTotal, &st.MemPercent
	}
	if st.HasTemp {
		resp.TempCelsius = &st.TempC
	}
	if st.HasUptime {
		secs := int64(st.Uptime.Seconds())
		resp.UptimeSecs = &secs
	}
	if st.HasLoad {
		resp.Load1 = &st.Load1
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// ── disk ────────────────────────────────────────────────────────────────

type apiDiskFolder struct {
	ID          int64  `json:"id"`
	Label       string `json:"label"`
	Path        string `json:"path"`
	TotalBytes  uint64 `json:"totalBytes"`
	UsedBytes   uint64 `json:"usedBytes"`
	FreeBytes   uint64 `json:"freeBytes"`
	UsedPercent int    `json:"usedPercent"`
	Available   bool   `json:"available"`
}

func (s *Server) apiDiskList(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Folders.List(r.Context(), folders.PurposeDisk)
	if err != nil {
		s.apiFailure(w, r, err)
		return
	}
	out := make([]apiDiskFolder, 0, len(list))
	for _, f := range list {
		out = append(out, diskFolderPayload(f))
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"folders": out})
}

func diskFolderPayload(f folders.Folder) apiDiskFolder {
	item := apiDiskFolder{ID: f.ID, Label: f.Label, Path: f.Path}
	total, used, free, _, err := scanner.FilesystemStat(f.Path)
	if err != nil {
		return item
	}
	item.Available = true
	item.TotalBytes, item.UsedBytes, item.FreeBytes = total, used, free
	if total > 0 {
		item.UsedPercent = int(float64(used) / float64(total) * 100)
	}
	return item
}

type apiDiskEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes uint64 `json:"bytes"`
	IsDir bool   `json:"isDir"`
}

func (s *Server) apiDiskDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	if !ok {
		s.apiError(w, http.StatusBadRequest, "bad folder id")
		return
	}
	ctx := r.Context()
	folder, err := s.deps.Folders.Get(ctx, id)
	if err != nil {
		s.apiError(w, http.StatusNotFound, "folder not found")
		return
	}

	resp := map[string]any{
		"folder":   diskFolderPayload(folder),
		"scanning": s.deps.Disk.Scanning(id),
		"top":      []apiDiskEntry{},
	}
	if res, scannedAt, ok, err := s.deps.Disk.CachedResult(ctx, id); err == nil && ok {
		top := make([]apiDiskEntry, 0, len(res.TopFolders))
		for _, t := range res.TopFolders {
			top = append(top, apiDiskEntry{Name: t.Name, Path: t.Path, Bytes: t.Bytes, IsDir: true})
		}
		resp["top"] = top
		resp["scannedAt"] = scannedAt
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) apiDiskBrowse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	if !ok {
		s.apiError(w, http.StatusBadRequest, "bad folder id")
		return
	}
	res, err := s.deps.Disk.Browse(r.Context(), id, r.URL.Query().Get("path"))
	if err != nil {
		// A rejected path is the client's fault, not a server failure.
		s.log.Warn("api browse", "folder", id, "error", err)
		s.apiError(w, http.StatusBadRequest, "cannot browse that path")
		return
	}
	entries := make([]apiDiskEntry, 0, len(res.Entries))
	for _, e := range res.Entries {
		entries = append(entries, apiDiskEntry{Name: e.Name, Path: e.Path, Bytes: e.Bytes, IsDir: e.IsDir})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"path":       res.Path,
		"parent":     res.Parent,
		"totalBytes": res.TotalBytes,
		"entries":    entries,
	})
}

func (s *Server) apiDiskScan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	if !ok {
		s.apiError(w, http.StatusBadRequest, "bad folder id")
		return
	}
	if _, err := s.deps.Disk.StartScan(r.Context(), id); err != nil && !errors.Is(err, jobs.ErrKindBusy) {
		s.apiFailure(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, map[string]any{"scanning": true})
}

// ── naming ──────────────────────────────────────────────────────────────

type apiNamingIssue struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Found    string `json:"found"`
	Expected string `json:"expected"`
}

func (s *Server) apiNaming(w http.ResponseWriter, r *http.Request) {
	issues, err := s.deps.Naming.Issues(r.Context())
	if err != nil {
		s.apiFailure(w, r, err)
		return
	}
	out := make([]apiNamingIssue, 0, len(issues))
	for _, is := range issues {
		out = append(out, apiNamingIssue{Path: is.Path, Type: is.Type, Found: is.Found, Expected: is.Expected})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "issues": out})
}

func (s *Server) apiNamingScan(w http.ResponseWriter, r *http.Request) {
	count, err := s.deps.Naming.Scan(r.Context())
	if err != nil {
		s.apiFailure(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"count": count})
}

// ── media ───────────────────────────────────────────────────────────────

type apiMediaItem struct {
	Path      string `json:"path"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	HasNFO    bool   `json:"hasNfo"`
	HasSubsES bool   `json:"hasSubsEs"`
}

func (s *Server) apiMedia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.deps.Library.Configured(ctx) {
		s.writeJSON(w, http.StatusOK, map[string]any{"configured": false, "items": []apiMediaItem{}})
		return
	}
	stats, err := s.deps.Library.Stats(ctx)
	if err != nil {
		s.apiFailure(w, r, err)
		return
	}
	items, err := s.deps.Library.Items(ctx, apiListLimit)
	if err != nil {
		s.apiFailure(w, r, err)
		return
	}
	out := make([]apiMediaItem, 0, len(items))
	for _, it := range items {
		out = append(out, apiMediaItem{
			Path: it.Path, Title: it.Title, Year: it.Year, Type: it.Type,
			HasNFO: it.HasNFO, HasSubsES: it.HasSubsES,
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"configured":    true,
		"total":         stats.Total,
		"withNfo":       stats.WithNFO,
		"withoutSubsEs": stats.WithoutSubs,
		"syncing":       s.deps.Library.Syncing(),
		"generatingNfo": s.deps.Library.GeneratingNFO(),
		"items":         out,
		"truncated":     len(items) >= apiListLimit && stats.Total > len(items),
	})
}

func (s *Server) apiMediaSync(w http.ResponseWriter, r *http.Request) {
	s.apiStartJob(w, r, func(ctx context.Context) error {
		_, err := s.deps.Library.StartSync(ctx)
		return err
	})
}

func (s *Server) apiMediaNFO(w http.ResponseWriter, r *http.Request) {
	s.apiStartJob(w, r, func(ctx context.Context) error {
		_, err := s.deps.Library.StartGenerateNFO(ctx)
		return err
	})
}

// apiStartJob runs a job starter and maps its outcome to a status code. An
// already-running job of the same kind is success: the caller wanted it running.
func (s *Server) apiStartJob(w http.ResponseWriter, r *http.Request, start func(context.Context) error) {
	err := start(r.Context())
	switch {
	case err == nil, errors.Is(err, jobs.ErrKindBusy):
		s.writeJSON(w, http.StatusAccepted, map[string]any{"started": true})
	case errors.Is(err, library.ErrNotConfigured):
		s.apiError(w, http.StatusPreconditionFailed, "Plex is not configured")
	default:
		s.apiFailure(w, r, err)
	}
}

// ── plex device link ────────────────────────────────────────────────────

// The app drives the same flow as the web UI: start, poll, pick a server.
func (s *Server) apiPlexLinkStart(w http.ResponseWriter, r *http.Request) {
	status, err := s.deps.PlexAuth.Start(r.Context())
	if err != nil {
		s.log.Warn("api plex link start", "error", err)
		s.apiError(w, http.StatusBadGateway, "could not reach plex.tv")
		return
	}
	s.writeJSON(w, http.StatusOK, plexLinkPayload(status))
}

func (s *Server) apiPlexLinkStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.deps.PlexAuth.Check(r.Context())
	switch {
	case errors.Is(err, plexauth.ErrNoLinkInProgress):
		s.writeJSON(w, http.StatusOK, map[string]any{"state": string(plexauth.StateIdle)})
	case err != nil:
		s.log.Warn("api plex link check", "error", err)
		s.apiError(w, http.StatusBadGateway, "could not reach plex.tv")
	default:
		s.writeJSON(w, http.StatusOK, plexLinkPayload(status))
	}
}

func (s *Server) apiPlexLinkServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseURL string `json:"baseUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.apiError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.deps.PlexAuth.SelectServer(r.Context(), body.BaseURL); err != nil {
		s.apiError(w, http.StatusBadRequest, "baseUrl is required")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func plexLinkPayload(status plexauth.Status) map[string]any {
	servers := status.Servers
	if servers == nil {
		servers = []plexauth.Server{}
	}
	return map[string]any{
		"state":   string(status.State),
		"code":    status.Code,
		"authUrl": status.AuthURL,
		"servers": servers,
	}
}

// ── subtitles ───────────────────────────────────────────────────────────

// apiListLimit caps list endpoints so a huge library cannot produce an
// unbounded response on a small device.
const apiListLimit = 500

type apiSubtitleMissing struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Year  int    `json:"year"`
	Type  string `json:"type"`
}

func (s *Server) apiSubtitles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	configured := s.deps.Subtitles.Configured(ctx)
	items, err := s.deps.Subtitles.MissingItems(ctx, apiListLimit)
	if err != nil {
		s.apiFailure(w, r, err)
		return
	}
	total, err := s.deps.Subtitles.MissingCount(ctx)
	if err != nil {
		total = len(items)
	}
	out := make([]apiSubtitleMissing, 0, len(items))
	for _, it := range items {
		out = append(out, apiSubtitleMissing{Path: it.Path, Title: it.Title, Year: it.Year, Type: it.Type})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"configured": configured,
		"missing":    total,
		"items":      out,
		"truncated":  len(items) >= apiListLimit && total > len(items),
	})
}

type apiSubtitleResult struct {
	FileID   string `json:"fileId"`
	FileName string `json:"fileName"`
	Release  string `json:"release"`
	Language string `json:"language"`
}

func (s *Server) apiSubtitleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	title := strings.TrimSpace(q.Get("title"))
	if title == "" {
		s.apiError(w, http.StatusBadRequest, "title is required")
		return
	}
	year := 0
	if raw := q.Get("year"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			year = parsed
		}
	}
	results, err := s.deps.Subtitles.Search(r.Context(), title, year)
	if err != nil {
		s.log.Warn("api subtitle search", "title", title, "error", err)
		s.apiError(w, http.StatusBadGateway, "OpenSubtitles search failed")
		return
	}
	out := make([]apiSubtitleResult, 0, len(results))
	for _, res := range results {
		out = append(out, apiSubtitleResult{
			FileID:   strconv.Itoa(res.FileID),
			FileName: res.FileName,
			Release:  res.Release,
			Language: res.Language,
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

func (s *Server) apiSubtitleDownload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FileID int    `json:"fileId"`
		Path   string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.apiError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.FileID == 0 || body.Path == "" {
		s.apiError(w, http.StatusBadRequest, "fileId and path are required")
		return
	}
	dest, err := s.deps.Subtitles.Download(r.Context(), body.FileID, body.Path)
	if err != nil {
		// The path is validated against the inventory inside Download.
		s.log.Warn("api subtitle download", "file_id", body.FileID, "error", err)
		s.apiError(w, http.StatusBadRequest, "could not download that subtitle")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"path": dest})
}

// ── torrents ────────────────────────────────────────────────────────────

type apiTorrent struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	State     string  `json:"state"`
	Progress  float64 `json:"progress"`
	SizeBytes int64   `json:"sizeBytes"`
	DlSpeed   int64   `json:"dlSpeed"`
	UpSpeed   int64   `json:"upSpeed"`
	Seeds     int     `json:"seeds"`
	Leechs    int     `json:"leechs"`
	Paused    bool    `json:"paused"`
}

func (s *Server) apiTorrents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.deps.Torrents.Configured(ctx) {
		s.writeJSON(w, http.StatusOK, map[string]any{"configured": false, "torrents": []apiTorrent{}})
		return
	}
	list, err := s.deps.Torrents.List(ctx)
	if err != nil {
		s.log.Warn("api torrents list", "error", err)
		s.apiError(w, http.StatusBadGateway, "cannot reach qBittorrent")
		return
	}
	out := make([]apiTorrent, 0, len(list))
	var dl, up int64
	active := 0
	for _, t := range list {
		state := strings.ToLower(t.State)
		dl += t.DlSpeed
		up += t.UpSpeed
		if t.DlSpeed > 0 || t.UpSpeed > 0 {
			active++
		}
		out = append(out, apiTorrent{
			Hash: t.Hash, Name: t.Name, State: t.State, Progress: t.Progress,
			SizeBytes: t.Size, DlSpeed: t.DlSpeed, UpSpeed: t.UpSpeed,
			Seeds: t.NumSeeds, Leechs: t.NumLeechs,
			Paused: strings.Contains(state, "paused") || strings.Contains(state, "stopped"),
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"total":      len(out),
		"active":     active,
		"dlSpeed":    dl,
		"upSpeed":    up,
		"torrents":   out,
	})
}

func (s *Server) apiTorrentAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Magnet   string `json:"magnet"`
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.apiError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	err := s.deps.Torrents.AddMagnet(r.Context(), body.Magnet, body.Category)
	switch {
	case errors.Is(err, torrents.ErrInvalidMagnet):
		s.apiError(w, http.StatusBadRequest, "not a magnet link")
	case errors.Is(err, torrents.ErrNotConfigured):
		s.apiError(w, http.StatusPreconditionFailed, "qBittorrent is not configured")
	case err != nil:
		s.log.Warn("api add magnet", "error", err)
		s.apiError(w, http.StatusBadGateway, "could not add the magnet")
	default:
		s.writeJSON(w, http.StatusAccepted, map[string]any{"added": true})
	}
}

func (s *Server) apiTorrentAction(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	ctx := r.Context()

	var err error
	switch action := r.PathValue("action"); action {
	case "pause":
		err = s.deps.Torrents.Pause(ctx, hash)
	case "resume":
		err = s.deps.Torrents.Resume(ctx, hash)
	case "delete":
		err = s.deps.Torrents.Delete(ctx, hash)
	default:
		s.apiError(w, http.StatusBadRequest, "unknown action")
		return
	}
	if err != nil {
		s.log.Warn("api torrent action", "hash", hash, "error", err)
		s.apiError(w, http.StatusBadGateway, "the action failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// pathInt64 parses an int64 path parameter.
func pathInt64(r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
