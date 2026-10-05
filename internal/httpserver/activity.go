package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/activity"
	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/seerr"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleActivityPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.ActivityPage(activityView(s.deps.Activity.Current(r.Context()), time.Now())))
}

func (s *Server) handleActivityEvents(w http.ResponseWriter, r *http.Request) {
	streamLive(s, w, r, s.deps.Activity, "activity", func(v activity.Snapshot) templ.Component {
		return templates.ActivityLive(activityView(v, time.Now()))
	})
}

func activityView(a activity.Snapshot, now time.Time) templates.ActivityView {
	v := templates.ActivityView{
		Idle: a.Idle, Errors: a.Errors,
		Down: rate(float64(a.DownSpeed)), Up: rate(float64(a.UpSpeed)),
		HasJellyfin: a.HasJellyfin, HasTorrents: a.HasTorrents, HasArr: len(a.Apps) > 0,
	}
	for _, ss := range a.Playing {
		v.Playing = append(v.Playing, playingView(ss))
	}
	for _, d := range a.Downloads {
		v.Downloads = append(v.Downloads, downloadView(d, now))
	}
	for _, it := range a.Recent {
		r := templates.ActRecent{Title: it.Name, When: ago(now.Sub(it.DateCreated))}
		if it.Type == jellyfin.TypeEpisode && it.SeriesName != "" {
			r.Title = it.SeriesName
			r.Subtitle = episodeLabel(it.ParentIndexNumber, it.IndexNumber) + " " + it.Name
		} else if it.ProductionYear > 0 {
			r.Subtitle = fmt.Sprintf("(%d)", it.ProductionYear)
		}
		v.Recent = append(v.Recent, r)
	}
	libraryView(&v, a.Library, now)
	refineRequests(&v, a, now)
	return v
}

// refineRequests replaces Seerr's "processing" with what is actually going on.
//
// For Seerr, media status 3 means it accepted the request and handed it to
// Radarr or Sonarr — not that anything is downloading. Read literally, a film
// that has not even come out yet showed as "descargando", and anyone who went
// to look found an empty queue. Holocron has the queue and the calendar in the
// same reading, so it can say which of the three it really is: in the queue,
// waiting for a release date, or watched and not found yet. (Reported by the
// Ginebra session, comparing the screen against the APIs.)
func refineRequests(v *templates.ActivityView, a activity.Snapshot, now time.Time) {
	for i, r := range a.Library.Requests {
		if i >= len(v.Requests) || r.MediaStatus != seerr.MediaProcessing || r.Status == seerr.RequestDeclined {
			continue
		}
		matches := func(tmdb, tvdb int) bool {
			if r.Type == "tv" {
				return r.TvdbID != 0 && tvdb == r.TvdbID
			}
			return r.TmdbID != 0 && tmdb == r.TmdbID
		}
		state := "buscando"
		for _, d := range a.Downloads {
			if matches(d.TmdbID, d.TvdbID) {
				state = "descargando " + pct(d.Progress*100)
				break
			}
		}
		if state == "buscando" {
			for _, u := range a.Library.Upcoming {
				if matches(u.TmdbID, u.TvdbID) && u.When.After(now) {
					state = "sale " + until(u.When, now)
					if u.Kind == "cines" {
						state = "en cines " + until(u.When, now)
					}
					break
				}
			}
		}
		v.Requests[i].State = state
	}
}

// libraryView fills in the slow lane: requests, upcoming releases, and what is
// worth a look.
func libraryView(v *templates.ActivityView, l activity.Library, now time.Time) {
	if l.At.IsZero() {
		return
	}
	v.LibAge = "actualizado " + ago(now.Sub(l.At))
	if now.Sub(l.At) < time.Minute {
		v.LibAge = "actualizado recién"
	}
	if l.Counts != nil {
		v.HasSeerr = true
		v.ReqSummary = templates.Plural(l.Counts.Total, "pedido", "pedidos")
		if l.Counts.Pending > 0 {
			v.ReqSummary += " · " + templates.Plural(l.Counts.Pending, "esperando aprobación", "esperando aprobación")
		}
	}
	for _, r := range l.Requests {
		ar := templates.ActRequest{Title: r.Title, By: r.By, When: ago(now.Sub(r.CreatedAt)), Kind: "película"}
		if r.Type == "tv" {
			ar.Kind = "serie"
		}
		if r.Year > 0 {
			ar.Title = fmt.Sprintf("%s (%d)", r.Title, r.Year)
		}
		ar.State, ar.Done, ar.Stuck = requestState(r)
		v.Requests = append(v.Requests, ar)
	}
	for _, u := range l.Upcoming {
		v.Upcoming = append(v.Upcoming, templates.ActUpcoming{
			Subject: u.Subject, Kind: u.Kind, App: u.App.Label(), When: until(u.When, now),
		})
	}

	if n := l.Missing[arr.Radarr]; n > 0 {
		v.Attention = append(v.Attention, templates.ActAttention{Text: "Radarr sigue buscando " + templates.Plural(n, "película", "películas")})
	}
	if n := l.Missing[arr.Sonarr]; n > 0 {
		v.Attention = append(v.Attention, templates.ActAttention{Text: "Sonarr sigue buscando " + templates.Plural(n, "episodio", "episodios")})
	}
	if b := l.Subs; b != nil {
		if b.Movies > 0 || b.Episodes > 0 {
			v.Attention = append(v.Attention, templates.ActAttention{
				Text: "Sin subtítulos según Bazarr (sólo lo que gestionan Radarr y Sonarr): " + templates.Plural(b.Movies, "película", "películas") +
					" y " + templates.Plural(b.Episodes, "episodio", "episodios"),
			})
		}
		// Bazarr hears about new files over a live link to each *arr. When
		// it drops, new downloads stop getting subtitles and nothing says so.
		for app, state := range map[string]string{"Radarr": b.RadarrSignalR, "Sonarr": b.SonarrSignalR} {
			if state != "" && state != "LIVE" {
				v.Attention = append(v.Attention, templates.ActAttention{Text: "Bazarr perdió la conexión con " + app + ": los archivos nuevos no van a recibir subtítulos", Warn: true})
			}
		}
		if b.Providers > 0 {
			v.Attention = append(v.Attention, templates.ActAttention{Text: templates.Plural(b.Providers, "proveedor de subtítulos con problemas", "proveedores de subtítulos con problemas"), Warn: true})
		}
	}
	if l.Indexers > 0 {
		if len(l.Failing) > 0 {
			v.Attention = append(v.Attention, templates.ActAttention{
				Text: "Indexadores fallando en Prowlarr: " + strings.Join(l.Failing, ", "), Warn: true,
			})
		} else {
			v.Attention = append(v.Attention, templates.ActAttention{
				Text: fmt.Sprintf("Prowlarr: %d de %d indexadores activos, ninguno fallando", l.IndexersEnabled, l.Indexers),
			})
		}
	}
	for _, h := range l.Health {
		v.Attention = append(v.Attention, templates.ActAttention{Text: h, Warn: true})
	}
	for _, e := range l.Errors {
		v.Attention = append(v.Attention, templates.ActAttention{Text: e + " no responde", Warn: true})
	}
}

// requestState says where a Seerr request is. The request and the media each
// have a status, and the media's is the one that answers "can I watch it yet".
func requestState(r seerr.Request) (state string, done, stuck bool) {
	switch r.Status {
	case seerr.RequestPending:
		return "esperando aprobación", false, false
	case seerr.RequestDeclined:
		return "rechazado", false, true
	case seerr.RequestFailed:
		return "falló", false, true
	}
	switch r.MediaStatus {
	case seerr.MediaAvailable:
		return "disponible", true, false
	case seerr.MediaPartiallyAvailable:
		return "disponible en parte", false, false
	case seerr.MediaProcessing:
		// Refined against the queue and the calendar in refineRequests.
		return "buscando", false, false
	case seerr.MediaPending:
		return "buscando", false, false
	case seerr.MediaBlocklisted:
		return "en lista negra", false, true
	case seerr.MediaDeleted:
		return "borrado", false, true
	}
	if r.Status == seerr.RequestCompleted {
		return "completado", true, false
	}
	return "aprobado", false, false
}

func until(t, now time.Time) string {
	d := t.Sub(now)
	switch {
	case d < 0:
		return "ya salió"
	case d < 24*time.Hour:
		return "hoy"
	case d < 48*time.Hour:
		return "mañana"
	default:
		return fmt.Sprintf("en %d días", int(d.Hours()/24))
	}
}

// ticksPerSecond: Jellyfin counts time in 100-nanosecond ticks.
const ticksPerSecond = 10_000_000

func playingView(ss jellyfin.Session) templates.ActPlaying {
	p := templates.ActPlaying{Who: ss.UserName, Client: ss.Client}
	if ss.DeviceName != "" {
		if p.Who != "" {
			p.Who += " · "
		}
		p.Who += ss.DeviceName
	}
	if ss.Client != "" && ss.Client != ss.DeviceName {
		p.Who += " (" + ss.Client + ")"
	}
	item := ss.NowPlaying
	if item != nil {
		p.Title = item.Name
		if item.SeriesName != "" {
			p.Title = item.SeriesName
			p.Subtitle = strings.TrimSpace(episodeLabel(item.ParentIndexNumber, item.IndexNumber) + " " + item.Name)
		} else if item.ProductionYear > 0 {
			p.Subtitle = fmt.Sprintf("(%d)", item.ProductionYear)
		}
	}
	var pos, total int64
	if ss.PlayState != nil {
		pos = ss.PlayState.PositionTicks
		p.Paused = ss.PlayState.IsPaused
	}
	if item != nil {
		total = item.RunTimeTicks
	}
	if total > 0 {
		p.Width = width(float64(pos) * 100 / float64(total))
		p.Position = clock(pos) + " de " + clock(total)
	} else if pos > 0 {
		p.Position = clock(pos)
	}

	method := ""
	if ss.PlayState != nil {
		method = ss.PlayState.PlayMethod
	}
	switch method {
	case "DirectPlay":
		p.Method = "directo"
	case "DirectStream":
		p.Method = "directo (remux)"
	case "Transcode":
		p.Method = "transcodificando"
		p.Transcode = true
	default:
		p.Method = "—"
	}
	if t := ss.Transcoding; t != nil {
		if !t.IsVideoDirect {
			p.Transcode = true
			switch strings.ToLower(t.HardwareAccelerationType) {
			case "", "none":
				// The case worth flagging on Ginebra: Quick Sync is there,
				// and a CPU transcode on four cores is what makes the next
				// stream stutter.
				p.Hardware, p.OnCPU = "CPU", true
			case "qsv":
				p.Hardware = "Quick Sync"
			default:
				p.Hardware = strings.ToUpper(t.HardwareAccelerationType)
			}
		}
		var parts []string
		if t.Height > 0 {
			parts = append(parts, fmt.Sprintf("%dp", t.Height))
		}
		if t.VideoCodec != "" {
			parts = append(parts, t.VideoCodec)
		}
		if t.Bitrate > 0 {
			parts = append(parts, fmt.Sprintf("%.1f Mb/s", float64(t.Bitrate)/1e6))
		}
		p.Detail = strings.Join(parts, " · ")
		for _, r := range t.TranscodeReasons {
			p.Reasons = append(p.Reasons, transcodeReason(r))
		}
	}
	return p
}

// transcodeReason says in words why Jellyfin is converting the stream. Only
// the common ones are translated; the rest keep Jellyfin's own name, which is
// still searchable.
func transcodeReason(r string) string {
	switch r {
	case "ContainerNotSupported":
		return "el cliente no lee el contenedor"
	case "VideoCodecNotSupported":
		return "el cliente no lee el códec de video"
	case "AudioCodecNotSupported":
		return "el cliente no lee el códec de audio"
	case "SubtitleCodecNotSupported":
		return "subtítulos que hay que quemar en la imagen"
	case "VideoBitDepthNotSupported":
		return "video de 10 bits"
	case "VideoRangeTypeNotSupported":
		return "HDR que el cliente no muestra"
	case "AudioChannelsNotSupported":
		return "demasiados canales de audio"
	case "ContainerBitrateExceedsLimit", "VideoBitrateNotSupported":
		return "más bitrate del que permite el cliente"
	case "VideoResolutionNotSupported":
		return "resolución mayor a la del cliente"
	case "DirectPlayError":
		return "falló la reproducción directa"
	default:
		return r
	}
}

func downloadView(d activity.Download, now time.Time) templates.ActDownload {
	out := templates.ActDownload{
		Subject: d.Subject, Release: d.Release,
		Pct: pct(d.Progress * 100), Width: width(d.Progress * 100),
		Messages: d.Messages,
	}
	switch d.App {
	case arr.Radarr, arr.Sonarr:
		out.App = d.App.Label()
	}
	if d.Speed > 0 {
		out.Speed = rate(float64(d.Speed))
	}
	if !d.ETA.IsZero() && d.ETA.After(now) && d.Progress < 1 {
		out.ETA = "faltan " + system.HumanDuration(d.ETA.Sub(now))
	}
	out.State, out.Problem = downloadState(d)
	return out
}

// downloadState says where a download is, in Spanish, and whether it needs a
// person. An import that is blocked is the case that does: the file is on
// disk and nothing will happen until somebody looks.
func downloadState(d activity.Download) (string, bool) {
	switch d.State {
	case "importBlocked":
		return "importación trabada", true
	case "importPending":
		return "esperando para importar", false
	case "importing":
		return "importando", false
	case "imported":
		return "importado", false
	case "failedPending", "failed":
		return "falló", true
	}
	if d.Health == "error" {
		return "con error", true
	}
	if d.Health == "warning" {
		return "con advertencias", true
	}
	switch d.TorrentState {
	case "downloading", "forcedDL":
		return "bajando", false
	case "stalledDL":
		return "sin fuentes", true
	case "metaDL":
		return "buscando metadatos", false
	case "pausedDL", "stoppedDL":
		return "en pausa", false
	case "queuedDL":
		return "en cola", false
	case "uploading", "stalledUP", "pausedUP", "stoppedUP", "queuedUP", "forcedUP":
		return "terminado", false
	case "error", "missingFiles":
		return "error del torrent", true
	}
	if d.State == "downloading" {
		return "bajando", false
	}
	return "en cola", false
}

func episodeLabel(season, episode *int) string {
	if season == nil || episode == nil {
		return ""
	}
	return fmt.Sprintf("T%dE%02d", *season, *episode)
}

func clock(ticks int64) string {
	sec := ticks / ticksPerSecond
	h, m, s := sec/3600, (sec%3600)/60, sec%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func ago(d time.Duration) string {
	switch {
	case d < time.Hour:
		return "recién"
	case d < 24*time.Hour:
		return fmt.Sprintf("hace %d h", int(d.Hours()))
	case d < 48*time.Hour:
		return "ayer"
	default:
		return fmt.Sprintf("hace %d días", int(d.Hours()/24))
	}
}
