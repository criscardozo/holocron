package naming

import (
	"regexp"
	"strconv"
	"strings"
)

// Turning "Blade.Runner.2049.2017.2160p.WEB-DL.x265-GRP" into
// "Blade Runner 2049 (2017)" is the part of this feature that has to be right.
// The suggestion used to be shown next to the folder and read by a person, who
// would notice it was nonsense. Now it drives a bulk rename, and a plausible
// wrong answer is worse than an obviously wrong one: nobody checks a name that
// looks fine, and the file is already renamed by the time the scraper matches
// the wrong film.

// releaseTag matches the words that describe the file rather than the film.
// Only ever removed from the end of a title, never from the middle, so a film
// actually called "Blade" keeps its name.
var releaseTag = map[string]bool{
	"1080p": true, "720p": true, "480p": true, "2160p": true, "4k": true,
	"uhd": true, "hd": true, "sd": true, "bluray": true, "blu-ray": true,
	"brrip": true, "bdrip": true, "bdremux": true, "webrip": true,
	"web-dl": true, "webdl": true, "web": true, "hdtv": true, "dvdrip": true,
	"dvd": true, "remux": true, "hdrip": true, "camrip": true, "cam": true,
	"ts": true, "tc": true, "r5": true, "hc": true,
	"x264": true, "x265": true, "h264": true, "h265": true, "avc": true,
	"hevc": true, "xvid": true, "divx": true, "av1": true,
	"aac": true, "ac3": true, "eac3": true, "dts": true, "dts-hd": true,
	"truehd": true, "atmos": true, "flac": true, "mp3": true, "opus": true,
	"5": true, "1": true, "7": true, "2": true, "0": true,
	"hdr": true, "hdr10": true, "dv": true, "dovi": true, "sdr": true,
	"10bit": true, "8bit": true, "12bit": true,
	"multi": true, "dual": true, "latino": true, "castellano": true,
	"spanish": true, "english": true, "subs": true, "subbed": true,
	"sub": true, "esp": true, "eng": true, "vose": true, "vo": true,
	"proper": true, "repack": true, "internal": true, "limited": true,
	"remastered": true, "imax": true, "open": true, "matinee": true,
}

var (
	// A year on its own, possibly wrapped, as a whole token.
	yearToken = regexp.MustCompile(`^[(\[]?((?:19|20)\d{2})[)\]]?$`)
	// Edition tags are Jellyfin's own syntax and must survive untouched.
	editionRe = regexp.MustCompile(`\{[^}]+\}`)
	// A trailing release-group suffix: "-GRP" or "-RARBG" at the very end.
	groupRe = regexp.MustCompile(`-[A-Za-z0-9]{2,}$`)
)

// Parse pulls a title and a year out of a raw folder or file name. ok is false
// when there is no year to be found, which is the case Holocron refuses to
// guess at rather than invent.
//
// The title comes back without any edition tag. Jellyfin puts those after the
// year — "Dune (2021) {edition-IMAX}" — so the caller assembles them in that
// order; carrying them inside the title would build a name Jellyfin does not
// recognise as carrying an edition at all.
func Parse(raw string) (title string, year int, ok bool) {
	s := editionRe.ReplaceAllString(strings.TrimSpace(raw), " ")

	s = separatorsToSpaces(s)
	fields := strings.Fields(s)

	// The last year-looking token, not the first. Titles that contain a year
	// put it before the release year — "Blade Runner 2049 2017", "2012 2009",
	// "1917 2019" — and taking the first gets every one of them wrong.
	idx, y := -1, 0
	for i, f := range fields {
		if m := yearToken.FindStringSubmatch(f); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			// A title made only of a year is still a title: "2012" alone has
			// nothing after it, so it cannot also be the release year.
			if i == 0 && len(fields) == 1 {
				continue
			}
			idx, y = i, n
		}
	}
	if idx < 0 {
		return cleanTitle(fields), 0, false
	}

	title = cleanTitle(fields[:idx])
	if title == "" {
		// Everything before the year was junk, so the year token probably was
		// the title: "2012" in a folder called "2012 1080p BluRay".
		return cleanTitle(fields[idx : idx+1]), 0, false
	}
	return title, y, true
}

// Editions returns the {edition-...} tags in raw, in the order they appear.
func Editions(raw string) []string { return editionRe.FindAllString(raw, -1) }

// separatorsToSpaces turns scene punctuation into spaces, but only when the
// name looks like scene punctuation rather than prose. "Mr. Nobody (2009)" has
// one dot and three spaces and must keep its full stop; "Mr.Nobody.2009" has
// two dots and no spaces and must lose them.
func separatorsToSpaces(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if strings.Count(s, ".") > strings.Count(s, " ") {
		s = strings.ReplaceAll(s, ".", " ")
	}
	return s
}

// cleanTitle drops trailing describe-the-file words and tidies the punctuation
// a scene name leaves behind.
func cleanTitle(fields []string) string {
	out := make([]string, len(fields))
	copy(out, fields)

	// Only from the end: a film called "Dual" or "Limited" keeps its name as
	// long as something follows it.
	for len(out) > 1 {
		last := strings.ToLower(strings.Trim(out[len(out)-1], "()[]-.,"))
		if releaseTag[last] {
			out = out[:len(out)-1]
			continue
		}
		break
	}
	title := strings.Join(out, " ")
	title = groupRe.ReplaceAllString(title, "")
	title = strings.Trim(title, " .-_[]()")
	return strings.Join(strings.Fields(title), " ")
}
