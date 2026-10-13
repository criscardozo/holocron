package jellyfin

import (
	"context"
	"net/http"
	"strings"
)

// Library is one of the server's libraries ("virtual folder" in the API): its
// name as the user sees it and the directories it reads.
type Library struct {
	Name      string   `json:"Name"`
	Locations []string `json:"Locations"`
}

// Libraries lists the server's libraries. Items do not say which library they
// belong to, so this is how a path is placed in one.
func (c *Client) Libraries(ctx context.Context) ([]Library, error) {
	var out []Library
	if err := c.do(ctx, http.MethodGet, "/Library/VirtualFolders", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Holds reports whether path lives under one of the library's directories.
// A whole path component must match: /mnt/biblioteca/Anime does not hold
// /mnt/biblioteca/Animales.
func (l Library) Holds(path string) bool {
	for _, loc := range l.Locations {
		loc = strings.TrimRight(loc, "/")
		if loc == "" {
			continue
		}
		if path == loc || strings.HasPrefix(path, loc+"/") {
			return true
		}
	}
	return false
}

// AudioLanguageUnknown reports whether the item has audio and none of it says
// what language it is in. For such an item "no Spanish subtitles" is not a
// finding: the audio may well be Spanish already.
func (it Item) AudioLanguageUnknown() bool {
	seen := false
	for _, s := range it.streams() {
		if !strings.EqualFold(s.Type, "Audio") {
			continue
		}
		if !hasNoLanguage(s.Language) {
			return false
		}
		seen = true
	}
	return seen
}
