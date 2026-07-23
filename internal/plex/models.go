// Package plex is a small read-only client for the Plex Media Server HTTP API,
// covering only the endpoints Holocron needs. Ported from the sibling
// plexmatch-generator project.
package plex

// containerResponse mirrors the "MediaContainer" envelope Plex wraps every
// response in. Both Directory (libraries) and Metadata (items) decode from the
// same shape so a single type serves every endpoint.
type containerResponse struct {
	MediaContainer struct {
		Directory []Library  `json:"Directory"`
		Metadata  []Metadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

// Library is a single Plex library section (movies, shows, music, ...).
type Library struct {
	ID    string `json:"key"`
	Type  string `json:"type"` // "movie", "show" or "artist"
	Title string `json:"title"`
}

// Metadata is a media item or one of its children. Not every field is populated
// for every endpoint; unused ones stay at their zero value.
type Metadata struct {
	RatingKey    string     `json:"ratingKey"`
	Type         string     `json:"type"` // "movie", "show", "season", "episode"
	Title        string     `json:"title"`
	Year         int        `json:"year"`
	Guid         string     `json:"guid"` // primary GUID, e.g. plex://movie/<id>
	Index        int        `json:"index"`
	ShowOrdering string     `json:"showOrdering"`
	Location     []Location `json:"Location"` // folder paths (shows/music)
	Media        []Media    `json:"Media"`    // file parts (movies/episodes)

	// Plex also returns a "Guid" array of alternate IDs (imdb/tmdb/tvdb). It
	// MUST be captured here: without an exact-tag field for "Guid",
	// encoding/json's case-insensitive matching unmarshals that array into the
	// "guid" string above and fails.
	AltGUIDs []AltGUID `json:"Guid"`
}

// AltGUID is an alternate identifier (e.g. imdb://tt..., tmdb://...).
type AltGUID struct {
	ID string `json:"id"`
}

// Location is a folder path as Plex knows it.
type Location struct {
	Path string `json:"path"`
}

// Media groups the file parts of an item.
type Media struct {
	Part []Part `json:"Part"`
}

// Part is a single media file.
type Part struct {
	File string `json:"file"`
}
