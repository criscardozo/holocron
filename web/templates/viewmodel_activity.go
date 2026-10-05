package templates

// ActivityView is "what is happening right now", already formatted.
type ActivityView struct {
	Playing   []ActPlaying  `json:"playing"`
	Idle      int           `json:"idle"`
	Downloads []ActDownload `json:"downloads"`
	Recent    []ActRecent   `json:"recent"`
	Down      string        `json:"down"` // total download speed
	Up        string        `json:"up"`
	// Errors names the sources that did not answer. Shown, because an empty
	// list and a dead source look identical otherwise.
	Errors []string `json:"errors"`

	HasJellyfin bool `json:"hasJellyfin"`
	HasTorrents bool `json:"hasTorrents"`
	HasArr      bool `json:"hasArr"`

	// The slow lane, refreshed every few minutes.
	Requests   []ActRequest  `json:"requests"`
	ReqSummary string        `json:"reqSummary"` // "21 pedidos · 0 esperando aprobación"
	HasSeerr   bool          `json:"hasSeerr"`
	Upcoming   []ActUpcoming `json:"upcoming"`
	// Attention is what someone might want to act on: what the *arrs are still
	// looking for, what lacks subtitles, failing indexers, health warnings.
	Attention []ActAttention `json:"attention"`
	LibAge    string         `json:"libAge"` // "hace 3 min"
}

// ActRequest is one Seerr request.
type ActRequest struct {
	Title string `json:"title"`
	Kind  string `json:"kind"` // película | serie
	By    string `json:"by"`
	When  string `json:"when"`
	State string `json:"state"`
	Done  bool   `json:"done"`
	Stuck bool   `json:"stuck"`
}

// ActUpcoming is one release on the calendar.
type ActUpcoming struct {
	Subject string `json:"subject"`
	When    string `json:"when"`
	Kind    string `json:"kind"`
	App     string `json:"app"`
}

// ActAttention is one line of things worth a look.
type ActAttention struct {
	Text string `json:"text"`
	Href string `json:"href"` // where to go about it, when Holocron has a page for it
	Warn bool   `json:"warn"`
}

// ActPlaying is one session playing something.
type ActPlaying struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"` // the episode, or the year
	Who      string `json:"who"`      // user · device
	Client   string `json:"client"`
	Paused   bool   `json:"paused"`
	Position string `json:"position"` // "1:02:10 de 2:15:00"
	Width    string `json:"width"`    // progress bar on a 0..100 canvas
	// Method is how the stream reaches the client: directo, remux o
	// transcodificando.
	Method    string `json:"method"`
	Transcode bool   `json:"transcode"`
	// Hardware is "Quick Sync" when the GPU is doing the work and "CPU" when it
	// is not — the difference between a second stream fitting or not.
	Hardware string   `json:"hardware"`
	OnCPU    bool     `json:"onCPU"`
	Reasons  []string `json:"reasons"`
	Detail   string   `json:"detail"` // "1080p · h264 · 8 Mb/s"
}

// ActDownload is one download, joined across the *arr and the torrent.
type ActDownload struct {
	App      string   `json:"app"` // "Radarr", "Sonarr", or "" for a torrent added by hand
	Subject  string   `json:"subject"`
	Release  string   `json:"release"`
	Pct      string   `json:"pct"`
	Width    string   `json:"width"`
	Speed    string   `json:"speed"`
	ETA      string   `json:"eta"`
	State    string   `json:"state"`
	Problem  bool     `json:"problem"`
	Messages []string `json:"messages"`
}

// ActRecent is one item recently added to the library.
type ActRecent struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	When     string `json:"when"`
}
