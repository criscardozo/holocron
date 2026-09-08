package naming

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// Renaming a media folder is the most destructive thing Holocron does: it
// changes files the user cannot easily reconstruct, in bulk, on a disk shared
// with a media server. Everything here is built around that.
//
// Two rules shape the design.
//
// Nothing is renamed alone. A subtitle only works while its name matches the
// video's, and the same goes for the artwork Jellyfin picks up by filename. So
// the unit of work is the folder: either the folder and every file that keyed
// off its name move together, or nothing in it moves. Renaming the folder and
// leaving "Movie.1080p.es.srt" behind would silently cost the subtitles.
//
// Nothing is overwritten, ever. Every rename whose destination already exists
// is skipped and reported. A media library is exactly where a collision means
// two different films, not a stale copy.

// Extensions that carry the stem of the video they belong to. Anything else in
// the folder is left alone: "poster.jpg" and "fanart.jpg" are already the names
// Jellyfin looks for, and rewriting them would break what works.
var stemmedExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true, ".mov": true,
	".wmv": true, ".mpg": true, ".mpeg": true, ".ts": true, ".m2ts": true,
	".webm": true, ".flv": true, ".iso": true, ".img": true,
	".srt": true, ".ssa": true, ".ass": true, ".sub": true, ".idx": true,
	".vtt": true, ".sup": true, ".smi": true,
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".tbn": true,
	".nfo": true,
}

// videoExts are the ones that make a file the thing the folder is about, which
// is what lets a folder whose files disagree with its name still be planned.
var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true, ".mov": true,
	".wmv": true, ".mpg": true, ".mpeg": true, ".ts": true, ".m2ts": true,
	".webm": true, ".flv": true, ".iso": true, ".img": true,
}

// Rename is one path changing name, relative to the media root.
type Rename struct {
	From string
	To   string
}

// Skip is something deliberately left alone, with the reason. Reported rather
// than hidden: a bulk operation that quietly does less than it says is worse
// than one that refuses.
type Skip struct {
	Name   string
	Reason string
}

// Plan is everything that would happen to one folder. Produced without
// touching anything, so it can be shown before it is run.
type Plan struct {
	// Folder is the current folder name, relative to the media root.
	Folder string
	// NewFolder is what it would become. Equal to Folder when only the files
	// inside are wrong.
	NewFolder string
	// Files are renames inside the folder, named relative to the folder.
	Files []Rename
	// Skipped is what was left alone and why.
	Skipped []Skip
	// Blocked is set when the folder cannot be planned at all; the plan is
	// then empty and this says what a person would have to decide.
	Blocked string
}

// Empty reports whether running this plan would change nothing.
func (p Plan) Empty() bool {
	return p.Blocked == "" && len(p.Files) == 0 && p.NewFolder == p.Folder
}

// ErrNoYear means the folder name carries no year, so the correct name cannot
// be derived from it. Holocron does not guess: the year is what tells two
// films with the same title apart, and inventing one sends the scraper to the
// wrong film with a confident-looking name.
var ErrNoYear = errors.New("the folder name has no year in it")

// PlanFolder works out what folder would have to change, without changing
// anything. dir is relative to root, which confines every path this touches.
func PlanFolder(root *os.Root, dir string) (Plan, error) {
	p := Plan{Folder: dir, NewFolder: dir}

	ok, expected := Validate(dir)
	switch {
	case !ok && strings.HasSuffix(expected, "(Año)"):
		p.Blocked = "no tiene año en el nombre, así que hay que decidirlo a mano"
		return p, nil
	case !ok:
		p.NewFolder = expected
	}

	entries, err := readDirIn(root, dir)
	if err != nil {
		return p, fmt.Errorf("read %s: %w", dir, err)
	}

	// The stems a file might be keyed off: the folder's own name, and the name
	// of any video in it. Longest first, so "Movie.2019.1080p" wins over
	// "Movie.2019" when both would match.
	stems := []string{dir}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if videoExts[strings.ToLower(path.Ext(e.Name()))] {
			stems = append(stems, strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
		}
	}
	sort.SliceStable(stems, func(i, j int) bool { return len(stems[i]) > len(stems[j]) })

	newStem := p.NewFolder
	taken := make(map[string]bool, len(entries))
	for _, e := range entries {
		taken[e.Name()] = true
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !stemmedExts[strings.ToLower(path.Ext(name))] {
			continue
		}
		stem := matchStem(name, stems)
		if stem == "" {
			// Not keyed off the folder or the video: "poster.jpg" and friends.
			// Already the name Jellyfin looks for; renaming would break it.
			continue
		}
		want := newStem + name[len(stem):]
		if want == name {
			continue
		}
		if taken[want] {
			p.Skipped = append(p.Skipped, Skip{
				Name:   name,
				Reason: "ya existe «" + want + "» y no se pisa nada",
			})
			continue
		}
		taken[want] = true
		p.Files = append(p.Files, Rename{From: name, To: want})
	}
	return p, nil
}

// matchStem returns the longest stem that name starts with, or "". A match must
// end at a separator so that "Movie 2" does not match inside "Movie 20".
func matchStem(name string, stems []string) string {
	for _, s := range stems {
		if s == "" || !strings.HasPrefix(name, s) {
			continue
		}
		rest := name[len(s):]
		if rest == "" || rest[0] == '.' || rest[0] == '-' || rest[0] == ' ' || rest[0] == '_' {
			return s
		}
	}
	return ""
}

// readDirIn lists dir inside root. os.Root has no ReadDir, so it goes through
// the fs.FS view, which stays confined.
func readDirIn(root *os.Root, dir string) ([]fs.DirEntry, error) {
	return fs.ReadDir(root.FS(), dir)
}

// Result is what actually happened, which is not always what was planned: the
// disk can have changed since, and a folder shared with a media server does.
type Result struct {
	Folder        string
	FinalFolder   string
	FilesRenamed  int
	FolderRenamed bool
	// Failed carries what could not be done, one entry per item, so a partial
	// run reports exactly which files are now inconsistent.
	Failed []Skip
}

// Apply runs a plan against the disk.
//
// Files first, folder last. If it stops halfway the contents are still inside a
// folder with the name the user recognises, which is the state that is easiest
// to understand and to finish by hand. Doing it the other way round would leave
// a correctly named folder full of files that no longer match it, looking done
// while the subtitles are broken.
func Apply(root *os.Root, p Plan) (Result, error) {
	res := Result{Folder: p.Folder, FinalFolder: p.Folder}
	if p.Blocked != "" {
		return res, fmt.Errorf("%s: %w", p.Folder, ErrNoYear)
	}

	for _, r := range p.Files {
		from := path.Join(p.Folder, r.From)
		to := path.Join(p.Folder, r.To)
		if err := renameNoClobber(root, from, to); err != nil {
			res.Failed = append(res.Failed, Skip{Name: r.From, Reason: err.Error()})
			continue
		}
		res.FilesRenamed++
	}

	if p.NewFolder != p.Folder {
		// A failure here goes into the result rather than out as an error, and
		// that is the whole point: the files above have already been renamed,
		// so the caller needs the partial outcome to tell the user which folder
		// is now internally consistent but still wearing the old name.
		// Returning an error would throw that away.
		switch err := renameNoClobber(root, p.Folder, p.NewFolder); err {
		case nil:
			res.FolderRenamed = true
			res.FinalFolder = p.NewFolder
		default:
			res.Failed = append(res.Failed, Skip{Name: p.Folder, Reason: err.Error()})
		}
	}
	return res, nil
}

// renameNoClobber refuses to replace an existing path.
//
// This matters more than it looks: rename(2) silently replaces the destination,
// so a plain Rename between two real films with names that normalise to the
// same thing would delete one of them. The Stat first is not airtight — nothing
// stops the destination appearing in between — but the alternative that would
// be, hard-linking and unlinking, needs a filesystem with links, and the
// library lives on exFAT. On a home server with one writer the check holds; the
// honest statement is that it closes the case that happens, not the race.
func renameNoClobber(root *os.Root, from, to string) error {
	if from == to {
		return nil
	}
	if _, err := root.Lstat(to); err == nil {
		return fmt.Errorf("ya existe «%s»", path.Base(to))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no se pudo comprobar «%s»: %w", path.Base(to), err)
	}
	if err := root.Rename(from, to); err != nil {
		return fmt.Errorf("no se pudo renombrar «%s»: %w", path.Base(from), err)
	}
	return nil
}
