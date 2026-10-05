package activity

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/bazarr"
	"github.com/cristian/holocron/internal/seerr"
)

// The slow lane: things that change a few times a day — requests, the release
// calendar, what is missing, the indexers' health. Read every slowEvery, not on
// every five-second reading, which would be the polling the house rule is
// about: Jellyfin owns the CPU, and nobody needs the Prowlarr indexer list
// refreshed while they watch a download bar.
const slowEvery = 5 * time.Minute

// calendarDays is how far ahead "upcoming" looks.
const calendarDays = 30

// requestLimit is how many recent requests are shown.
const requestLimit = 8

// SlowSources are the clients the slow lane reads. Any of them may be nil when
// the service is not configured.
type SlowSources struct {
	Seerr    *seerr.Client
	Bazarr   *bazarr.Client
	Prowlarr *arr.Client
	Arrs     []*arr.Client // Radarr and Sonarr
}

// Library is the slow lane's reading.
type Library struct {
	At       time.Time
	Requests []seerr.Request
	Counts   *seerr.Counts
	Upcoming []arr.Upcoming
	// Missing is what each *arr is still looking for, by app.
	Missing map[arr.App]int
	// Subs is Bazarr's tally of what lacks subtitles; nil without Bazarr.
	Subs *bazarr.Badges
	// Indexers in Prowlarr: how many there are and which are failing.
	Indexers        int
	IndexersEnabled int
	Failing         []string
	// Health is what the *arrs themselves report as wrong.
	Health []string
	Errors []string
}

// Slow reads the slow lane, reusing the last reading while it is fresh.
type Slow struct {
	src SlowSources

	mu   sync.Mutex
	last Library
	at   time.Time
}

// NewSlow creates the slow lane.
func NewSlow(src SlowSources) *Slow { return &Slow{src: src} }

// Read returns the slow lane, refreshing it if it is older than slowEvery.
func (s *Slow) Read(ctx context.Context, now time.Time) Library {
	s.mu.Lock()
	if !s.at.IsZero() && now.Sub(s.at) < slowEvery {
		l := s.last
		s.mu.Unlock()
		return l
	}
	s.mu.Unlock()

	l := s.read(ctx, now)

	s.mu.Lock()
	s.last, s.at = l, now
	s.mu.Unlock()
	return l
}

func (s *Slow) read(ctx context.Context, now time.Time) Library {
	l := Library{At: now, Missing: map[arr.App]int{}}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	fail := func(label string) {
		mu.Lock()
		l.Errors = append(l.Errors, label)
		mu.Unlock()
	}
	run := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}

	if c := s.src.Seerr; c != nil {
		run(func() {
			reqs, err := c.Recent(ctx, requestLimit)
			counts, err2 := c.Count(ctx)
			if err != nil || err2 != nil {
				fail("Seerr")
				return
			}
			mu.Lock()
			l.Requests, l.Counts = reqs, &counts
			mu.Unlock()
		})
	}
	if c := s.src.Bazarr; c != nil {
		run(func() {
			b, err := c.Badges(ctx)
			if err != nil {
				fail("Bazarr")
				return
			}
			mu.Lock()
			l.Subs = &b
			mu.Unlock()
		})
	}
	if c := s.src.Prowlarr; c != nil {
		run(func() {
			idx, err := c.Indexers(ctx)
			st, err2 := c.IndexerStatuses(ctx)
			health, err3 := c.Health(ctx)
			if err != nil || err2 != nil || err3 != nil {
				fail("Prowlarr")
				return
			}
			names := map[int]string{}
			enabled := 0
			for _, i := range idx {
				names[i.ID] = i.Name
				if i.Enable {
					enabled++
				}
			}
			var failing []string
			for _, st := range st {
				// A status record with a backoff in the future is an indexer
				// Prowlarr has stopped asking; one in the past has recovered.
				if st.DisabledTill != nil && st.DisabledTill.After(now) {
					if n := names[st.IndexerID]; n != "" {
						failing = append(failing, n)
					}
				}
			}
			sort.Strings(failing)
			mu.Lock()
			l.Indexers, l.IndexersEnabled, l.Failing = len(idx), enabled, failing
			for _, h := range health {
				if h.Type == "warning" || h.Type == "error" {
					l.Health = append(l.Health, "Prowlarr: "+h.Message)
				}
			}
			mu.Unlock()
		})
	}
	for _, c := range s.src.Arrs {
		c := c
		run(func() {
			up, err := c.Calendar(ctx, now, now.AddDate(0, 0, calendarDays))
			missing, err2 := c.MissingCount(ctx)
			health, err3 := c.Health(ctx)
			if err != nil || err2 != nil || err3 != nil {
				fail(c.App().Label())
				return
			}
			mu.Lock()
			for _, u := range up {
				if !u.HasFile {
					l.Upcoming = append(l.Upcoming, u)
				}
			}
			l.Missing[c.App()] = missing
			for _, h := range health {
				if h.Type == "warning" || h.Type == "error" {
					l.Health = append(l.Health, c.App().Label()+": "+h.Message)
				}
			}
			mu.Unlock()
		})
	}
	wg.Wait()

	sort.Slice(l.Upcoming, func(i, j int) bool { return l.Upcoming[i].When.Before(l.Upcoming[j].When) })
	sort.Strings(l.Errors)
	sort.Strings(l.Health)
	return l
}
