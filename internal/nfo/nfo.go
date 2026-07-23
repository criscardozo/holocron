// Package nfo generates Kodi/Jellyfin-style .nfo metadata files for movies and
// shows, using the metadata Plex has already resolved. Plex itself does not
// maintain .nfo files; these help other tools and act as a portable record.
package nfo

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

// Item is the metadata used to render a .nfo file.
type Item struct {
	Title          string
	Year           int
	PlexGUID       string            // plex://movie/<id>
	IDs            map[string]string // e.g. {"imdb": "tt123", "tmdb": "603"}
	HasSpanishSubs bool
}

type uniqueID struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type subtitle struct {
	Language string `xml:"language,attr"`
	Value    string `xml:",chardata"` // "yes" or "no"
}

type document struct {
	XMLName   xml.Name
	Title     string     `xml:"title"`
	Year      int        `xml:"year,omitempty"`
	UniqueIDs []uniqueID `xml:"uniqueid"`
	Subtitle  subtitle   `xml:"subtitle"`
}

// MovieXML renders the movie.nfo document for an item.
func MovieXML(it Item) ([]byte, error) { return marshal("movie", it) }

// ShowXML renders the tvshow.nfo document for an item.
func ShowXML(it Item) ([]byte, error) { return marshal("tvshow", it) }

func marshal(root string, it Item) ([]byte, error) {
	doc := document{
		XMLName:  xml.Name{Local: root},
		Title:    it.Title,
		Year:     it.Year,
		Subtitle: subtitle{Language: "spa", Value: yesNo(it.HasSpanishSubs)},
	}
	if it.PlexGUID != "" {
		doc.UniqueIDs = append(doc.UniqueIDs, uniqueID{Type: "plex", Value: it.PlexGUID})
	}
	for typ, val := range it.IDs {
		if val != "" {
			doc.UniqueIDs = append(doc.UniqueIDs, uniqueID{Type: typ, Value: val})
		}
	}
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s nfo: %w", root, err)
	}
	return append([]byte(xml.Header), append(body, '\n')...), nil
}

// WriteMovie writes movie.nfo into folder and returns the file path.
func WriteMovie(folder string, it Item) (string, error) {
	return write(folder, "movie.nfo", MovieXML, it)
}

// WriteShow writes tvshow.nfo into folder and returns the file path.
func WriteShow(folder string, it Item) (string, error) {
	return write(folder, "tvshow.nfo", ShowXML, it)
}

func write(folder, name string, render func(Item) ([]byte, error), it Item) (string, error) {
	info, err := os.Stat(folder)
	if err != nil {
		return "", fmt.Errorf("stat folder: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", folder)
	}
	data, err := render(it)
	if err != nil {
		return "", err
	}
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write nfo: %w", err)
	}
	return path, nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
