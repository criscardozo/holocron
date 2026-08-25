package jellyfin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Item types Holocron cares about.
const (
	TypeMovie  = "Movie"
	TypeSeries = "Series"
	typeBoxSet = "BoxSet"
)

// fields asked for on every listing. MediaSources carries the per-file streams,
// which is where subtitles live.
const itemFields = "Path,ProviderIds,MediaSources,MediaStreams,ProductionYear"

// MediaStream is one track of a file: video, audio or subtitle.
type MediaStream struct {
	Index        int    `json:"Index"`
	Type         string `json:"Type"`
	Codec        string `json:"Codec"`
	Language     string `json:"Language"`
	IsExternal   bool   `json:"IsExternal"`
	IsForced     bool   `json:"IsForced"`
	Path         string `json:"Path"`
	DisplayTitle string `json:"DisplayTitle"`
}

// MediaSource is one file backing an item. An item can have several: the same
// title in 1080p and 4K, or in two containers.
type MediaSource struct {
	Path      string        `json:"Path"`
	Container string        `json:"Container"`
	Size      int64         `json:"Size"`
	Streams   []MediaStream `json:"MediaStreams"`
}

// Item is a movie or a series.
type Item struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
	Type string `json:"Type"`
	Path string `json:"Path"`
	Year int    `json:"ProductionYear"`
	// ProviderIDs is a map rather than a struct: Jellyfin returns keys like
	// "official website", with a space, alongside Imdb/Tmdb/Tvdb.
	ProviderIDs map[string]string `json:"ProviderIds"`
	Sources     []MediaSource     `json:"MediaSources"`
	Streams     []MediaStream     `json:"MediaStreams"`
}

type itemsResponse struct {
	Items            []Item `json:"Items"`
	TotalRecordCount int    `json:"TotalRecordCount"`
}

// Items returns every movie and series in the library.
//
// This is deliberately two queries, not one. A recursive listing filtered to
// Movie and Series omits any movie that belongs to a collection — on the
// library this was built against, 46 of 355 films were invisible that way, with
// no error to hint at it. So collections are listed too and walked for their
// children, deduplicating by id.
func (c *Client) Items(ctx context.Context, userID string) ([]Item, error) {
	direct, err := c.listItems(ctx, userID, url.Values{
		"Recursive":        {"true"},
		"IncludeItemTypes": {TypeMovie + "," + TypeSeries},
		"Fields":           {itemFields},
	})
	if err != nil {
		return nil, err
	}

	out := make([]Item, 0, len(direct))
	seen := make(map[string]bool, len(direct))
	for _, it := range direct {
		// The server returns collections even when they are not requested, so
		// the type is filtered here rather than trusted.
		if it.Type != TypeMovie && it.Type != TypeSeries {
			continue
		}
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}

	boxSets, err := c.listItems(ctx, userID, url.Values{
		"Recursive":        {"true"},
		"IncludeItemTypes": {typeBoxSet},
		"Fields":           {"Path"},
	})
	if err != nil {
		return nil, err
	}
	for _, bs := range boxSets {
		if bs.Type != typeBoxSet {
			continue
		}
		children, err := c.listItems(ctx, userID, url.Values{
			"parentId": {bs.ID},
			"Fields":   {itemFields},
		})
		if err != nil {
			return nil, fmt.Errorf("list collection %q: %w", bs.Name, err)
		}
		for _, it := range children {
			if it.Type != TypeMovie && it.Type != TypeSeries {
				continue
			}
			if seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			out = append(out, it)
		}
	}
	return out, nil
}

func (c *Client) listItems(ctx context.Context, userID string, q url.Values) ([]Item, error) {
	if userID != "" {
		q.Set("userId", userID)
	}
	var resp itemsResponse
	if err := c.do(ctx, http.MethodGet, "/Items?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// OnDisk reports whether the item actually has a file. Jellyfin lists episodes
// it knows about from metadata providers but has never seen on disk; acting on
// those means chasing files that do not exist.
func (it Item) OnDisk() bool { return strings.TrimSpace(it.Path) != "" }

// Folder is the directory holding the item, which is what Holocron records.
// Series already point at their directory; movies point at a file.
func (it Item) Folder() string {
	if it.Type == TypeSeries {
		return it.Path
	}
	if i := strings.LastIndex(it.Path, "/"); i > 0 {
		return it.Path[:i]
	}
	return ""
}

// FilePaths lists every file backing the item, deduplicated. item.Path names
// only one of them, so multi-version titles (the same film in two resolutions)
// would otherwise be half-processed. Deduplication is by full path: two
// versions can share a basename and differ only in container.
func (it Item) FilePaths() []string {
	seen := make(map[string]bool, len(it.Sources)+1)
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, src := range it.Sources {
		add(src.Path)
	}
	add(it.Path)
	return out
}

// streams returns every stream of the item, from all of its files.
func (it Item) streams() []MediaStream {
	out := make([]MediaStream, 0, len(it.Streams))
	out = append(out, it.Streams...)
	for _, src := range it.Sources {
		out = append(out, src.Streams...)
	}
	return out
}

// Language classification follows the criteria measured against the real
// library, so Holocron and the tuning scripts report the same numbers. If the
// two disagree on what "has Spanish subtitles" means, neither is believable.

// spanishExact are the tags treated as Spanish outright.
var spanishExact = map[string]bool{
	"spa": true, "es": true, "esp": true, "lat": true,
	"es-419": true, "es-es": true, "es-mx": true,
	"castellano": true, "latino": true,
}

// noLanguage are the tags that mean "unknown", not "absent". "mis" is the ISO
// code for multiple languages and shows up in multi-audio releases.
var noLanguage = map[string]bool{
	"": true, "und": true, "unknown": true, "none": true, "mis": true,
}

// isSpanish reports whether a language tag names Spanish. Besides the exact
// list it accepts anything containing "spa" or "esp", which is what covers
// tags like "spa-419", "Spanish" and "esp-latino".
func isSpanish(lang string) bool {
	l := strings.ToLower(strings.TrimSpace(lang))
	if noLanguage[l] {
		return false
	}
	return spanishExact[l] || strings.Contains(l, "spa") || strings.Contains(l, "esp")
}

// hasNoLanguage reports whether a tag carries no usable language.
func hasNoLanguage(lang string) bool {
	return noLanguage[strings.ToLower(strings.TrimSpace(lang))]
}

// SubtitleState is what Holocron can say about an item's Spanish subtitles.
type SubtitleState int

const (
	// SubsMissing means nothing covers Spanish for this item.
	SubsMissing SubtitleState = iota
	// SubsPresent means a Spanish subtitle track exists, embedded or external.
	SubsPresent
	// SubsUnknownLanguage means there is a subtitle whose language Jellyfin
	// could not determine — typically an .srt with no language tag in its
	// filename. Reported apart rather than guessed: next to an Argentine film
	// it is almost certainly Spanish, next to an anime almost certainly not.
	SubsUnknownLanguage
	// SubsNotNeeded means the item already has Spanish audio.
	SubsNotNeeded
)

// subtitleStreams and audioStreams of every file backing the item. Subtitles
// attach to one MediaSource, so a title whose 4K version has them and whose
// 1080p version does not is still covered — the union is what matters.
func (it Item) matching(kind string, pred func(MediaStream) bool) bool {
	for _, s := range it.streams() {
		if strings.EqualFold(s.Type, kind) && pred(s) {
			return true
		}
	}
	return false
}

// SpanishSubtitleState classifies the item. The order is deliberate: an item
// with Spanish audio needs no subtitles at all, and counting it as "missing"
// is noise rather than a finding.
func (it Item) SpanishSubtitleState() SubtitleState {
	if it.matching("Audio", func(s MediaStream) bool { return isSpanish(s.Language) }) {
		return SubsNotNeeded
	}
	if it.matching("Subtitle", func(s MediaStream) bool { return isSpanish(s.Language) }) {
		return SubsPresent
	}
	if it.matching("Subtitle", func(s MediaStream) bool { return hasNoLanguage(s.Language) }) {
		return SubsUnknownLanguage
	}
	return SubsMissing
}

// HasSpanishSubtitles is the boolean the inventory stores: true when Spanish is
// covered or unnecessary. An unlabelled subtitle does not count as covered.
func (it Item) HasSpanishSubtitles() bool {
	switch it.SpanishSubtitleState() {
	case SubsPresent, SubsNotNeeded:
		return true
	default:
		return false
	}
}

// SubtitlePaths lists external subtitle files, so a report can name them.
func (it Item) SubtitlePaths() []string {
	var out []string
	for _, s := range it.streams() {
		if strings.EqualFold(s.Type, "Subtitle") && s.IsExternal && s.Path != "" {
			out = append(out, s.Path)
		}
	}
	return out
}

// CleanDisplayTitle strips control characters. Real libraries contain stream
// titles with a form feed embedded, which would corrupt the page.
func CleanDisplayTitle(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
