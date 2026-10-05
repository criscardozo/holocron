package templates

// ActivityView is "what is happening right now", already formatted.
type ActivityView struct {
	Playing   []ActPlaying
	Idle      int
	Downloads []ActDownload
	Recent    []ActRecent
	Down      string // total download speed
	Up        string
	// Errors names the sources that did not answer. Shown, because an empty
	// list and a dead source look identical otherwise.
	Errors []string

	HasJellyfin bool
	HasTorrents bool
	HasArr      bool

	// The slow lane, refreshed every few minutes.
	Requests   []ActRequest
	ReqSummary string // "21 pedidos · 0 esperando aprobación"
	HasSeerr   bool
	Upcoming   []ActUpcoming
	// Attention is what someone might want to act on: what the *arrs are still
	// looking for, what lacks subtitles, failing indexers, health warnings.
	Attention []ActAttention
	LibAge    string // "hace 3 min"
}

// ActRequest is one Seerr request.
type ActRequest struct {
	Title string
	Kind  string // película | serie
	By    string
	When  string
	State string
	Done  bool
	Stuck bool
}

// ActUpcoming is one release on the calendar.
type ActUpcoming struct {
	Subject string
	When    string
	Kind    string
	App     string
}

// ActAttention is one line of things worth a look.
type ActAttention struct {
	Text string
	Href string // where to go about it, when Holocron has a page for it
	Warn bool
}

// ActPlaying is one session playing something.
type ActPlaying struct {
	Title    string
	Subtitle string // the episode, or the year
	Who      string // user · device
	Client   string
	Paused   bool
	Position string // "1:02:10 de 2:15:00"
	Width    string // progress bar on a 0..100 canvas
	// Method is how the stream reaches the client: directo, remux o
	// transcodificando.
	Method    string
	Transcode bool
	// Hardware is "Quick Sync" when the GPU is doing the work and "CPU" when it
	// is not — the difference between a second stream fitting or not.
	Hardware string
	OnCPU    bool
	Reasons  []string
	Detail   string // "1080p · h264 · 8 Mb/s"
}

// ActDownload is one download, joined across the *arr and the torrent.
type ActDownload struct {
	App      string // "Radarr", "Sonarr", or "" for a torrent added by hand
	Subject  string
	Release  string
	Pct      string
	Width    string
	Speed    string
	ETA      string
	State    string
	Problem  bool
	Messages []string
}

// ActRecent is one item recently added to the library.
type ActRecent struct {
	Title    string
	Subtitle string
	When     string
}
