// Package activity answers "what is happening right now": who is watching what
// in Jellyfin and how, what Radarr and Sonarr are downloading, and how fast.
//
// Four sources, read in parallel and each allowed to fail on its own. A Sonarr
// that does not answer must not blank the screen while Jellyfin is fine, and
// the failure is shown by name, because "nothing downloading" and "Sonarr is
// down" look identical as an empty list.
package activity

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/qbittorrent"
)

// Sources is what the sampler reads from. Interfaces, so the screen can be
// tested without four servers.
type Sources struct {
	Sessions func(context.Context) ([]jellyfin.Session, error)
	Recent   func(context.Context, int) ([]jellyfin.Added, error)
	Torrents func(context.Context) ([]qbittorrent.Torrent, error)
	// Queues are the *arr clients that are configured. Absent ones are simply
	// not there, which is different from one that fails.
	Queues []*arr.Client
	// Slow is the lane for what changes a few times a day. Nil means none.
	Slow *Slow
}

// Download is one thing being downloaded, joined across the *arr that asked
// for it and the torrent doing it.
type Download struct {
	// App is "radarr", "sonarr", or "" for a torrent no *arr knows about —
	// added by hand, from the app.
	App      arr.App
	Subject  string
	Release  string
	Progress float64
	Size     float64
	State    string // the *arr's view: downloading, importBlocked…
	Health   string // ok | warning | error
	Messages []string
	ETA      time.Time
	// From the torrent, when there is one.
	Speed        int64
	TorrentState string
	HasTorrent   bool
}

// Snapshot is one reading.
type Snapshot struct {
	At        time.Time
	Playing   []jellyfin.Session
	Idle      int // connected clients not playing anything
	Downloads []Download
	Recent    []jellyfin.Added
	DownSpeed int64
	UpSpeed   int64
	// Errors names the sources that did not answer, by label.
	Errors []string
	// Configured says which sources exist at all, so an empty list can say
	// whether there is nothing or nobody to ask.
	HasJellyfin, HasTorrents bool
	Apps                     []arr.App

	// Library is the slow lane: requests, the calendar, what is missing.
	Library Library
}

// recentEvery is how often "recently added" is refreshed. It changes a few
// times a day; asking Jellyfin for it every few seconds would be the kind of
// polling the house rule is about.
const recentEvery = 5 * time.Minute

// recentLimit is how many recent items are shown.
const recentLimit = 12

// Sampler reads a Snapshot. It keeps the recently-added list between readings.
//
// Sources are asked for on every reading rather than fixed at start-up:
// Jellyfin can be linked or unlinked from the settings page while the server
// runs, and the screen should follow without a restart.
type Sampler struct {
	sources func(context.Context) Sources

	mu         sync.Mutex
	recent     []jellyfin.Added
	recentAt   time.Time
	recentFail bool
}

// NewSampler creates a Sampler.
func NewSampler(sources func(context.Context) Sources) *Sampler {
	return &Sampler{sources: sources}
}

// Sample reads every source once.
func (s *Sampler) Sample(ctx context.Context) Snapshot {
	src := s.sources(ctx)
	snap := Snapshot{
		At:          time.Now(),
		HasJellyfin: src.Sessions != nil,
		HasTorrents: src.Torrents != nil,
	}
	for _, q := range src.Queues {
		snap.Apps = append(snap.Apps, q.App())
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		sessions []jellyfin.Session
		torrents []qbittorrent.Torrent
		queue    []arr.QueueItem
	)
	fail := func(label string) {
		mu.Lock()
		snap.Errors = append(snap.Errors, label)
		mu.Unlock()
	}

	if src.Sessions != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := src.Sessions(ctx)
			if err != nil {
				fail("Jellyfin")
				return
			}
			mu.Lock()
			sessions = v
			mu.Unlock()
		}()
	}
	if src.Torrents != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := src.Torrents(ctx)
			if err != nil {
				fail("qBittorrent")
				return
			}
			mu.Lock()
			torrents = v
			mu.Unlock()
		}()
	}
	for _, c := range src.Queues {
		wg.Add(1)
		go func(c *arr.Client) {
			defer wg.Done()
			v, err := c.Queue(ctx)
			if err != nil {
				fail(c.App().Label())
				return
			}
			mu.Lock()
			queue = append(queue, v...)
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	sort.Strings(snap.Errors)

	for _, ss := range sessions {
		if ss.Playing() != "" {
			snap.Playing = append(snap.Playing, ss)
		} else {
			snap.Idle++
		}
	}
	snap.Downloads, snap.DownSpeed, snap.UpSpeed = join(queue, torrents)
	snap.Recent = s.recentList(ctx, src.Recent)
	if src.Slow != nil {
		snap.Library = src.Slow.Read(ctx, snap.At)
	}
	return snap
}

func (s *Sampler) recentList(ctx context.Context, fetch func(context.Context, int) ([]jellyfin.Added, error)) []jellyfin.Added {
	if fetch == nil {
		return nil
	}
	s.mu.Lock()
	if time.Since(s.recentAt) < recentEvery && !s.recentFail {
		r := s.recent
		s.mu.Unlock()
		return r
	}
	s.mu.Unlock()

	r, err := fetch(ctx, recentLimit)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recentAt = time.Now()
	s.recentFail = err != nil
	if err == nil {
		s.recent = r
	}
	return s.recent
}

// join matches each queue item to its torrent by hash, and lists the torrents
// no *arr claims as downloads of their own. The *arr's downloadId is the
// torrent hash in upper case; qBittorrent reports it in lower.
func join(queue []arr.QueueItem, torrents []qbittorrent.Torrent) ([]Download, int64, int64) {
	byHash := make(map[string]qbittorrent.Torrent, len(torrents))
	var down, up int64
	for _, t := range torrents {
		byHash[strings.ToUpper(t.Hash)] = t
		down += t.DlSpeed
		up += t.UpSpeed
	}
	claimed := map[string]bool{}

	var out []Download
	for _, q := range queue {
		d := Download{
			App: q.App, Subject: q.Subject, Release: q.Release,
			Progress: q.Progress(), Size: q.Size, State: q.State, Health: q.Health,
			Messages: q.Messages, ETA: q.ETA,
		}
		if t, ok := byHash[q.DownloadID]; ok && q.DownloadID != "" {
			claimed[q.DownloadID] = true
			d.HasTorrent = true
			d.Speed = t.DlSpeed
			d.TorrentState = t.State
			// The torrent knows its progress to the byte; the *arr's figure
			// lags a polling interval behind it.
			d.Progress = t.Progress
		}
		out = append(out, d)
	}
	for _, t := range torrents {
		if claimed[strings.ToUpper(t.Hash)] {
			continue
		}
		out = append(out, Download{
			Subject: t.Name, Release: t.Name, Progress: t.Progress, Size: float64(t.Size),
			Speed: t.DlSpeed, TorrentState: t.State, HasTorrent: true,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Problems first, then what is moving, then the rest.
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return out[i].Subject < out[j].Subject
	})
	return out, down, up
}

func rank(d Download) int {
	switch {
	case d.Health == "error" || d.Health == "warning" || strings.HasPrefix(d.State, "importBlocked") || strings.HasPrefix(d.State, "failed"):
		return 0
	case d.Speed > 0:
		return 1
	default:
		return 2
	}
}
