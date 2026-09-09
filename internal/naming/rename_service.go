package naming

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/jobs"
)

// PlannedFolder is one folder's plan together with the configured media folder
// it lives under, which is what confines every operation on it.
type PlannedFolder struct {
	// Root is the configured media folder, absolute.
	Root string
	// Label is what the user called that media folder.
	Label string
	Plan  Plan
}

// Key identifies a plan across the preview-then-apply round trip. The absolute
// path, because it is the one thing that is unambiguous when several media
// folders are configured.
func (p PlannedFolder) Key() string { return filepath.Join(p.Root, p.Plan.Folder) }

// Summary is the outcome of applying a batch.
type Summary struct {
	Folders int
	Files   int
	Failed  []Skip
}

// PlanMovies works out what would change in every configured movie folder,
// without touching anything.
//
// Movies only, deliberately. A TV folder's episodes carry season and episode
// numbers that this parser knows nothing about, and renaming them by the same
// rules would turn a working library into a pile of files named after the show.
func (s *Service) PlanMovies(ctx context.Context) ([]PlannedFolder, error) {
	roots, err := s.folders.List(ctx, folders.PurposeMovies)
	if err != nil {
		return nil, fmt.Errorf("list movie folders: %w", err)
	}

	ignored, err := s.ignoredSet(ctx)
	if err != nil {
		return nil, err
	}

	var out []PlannedFolder
	for _, f := range roots {
		root, err := os.OpenRoot(f.Path)
		if err != nil {
			// A media folder can be temporarily unmounted; that is not a
			// reason to fail the whole preview.
			continue
		}
		entries, err := readDirIn(root, ".")
		if err != nil {
			_ = root.Close()
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || Hidden(e.Name()) {
				continue
			}
			if ignored[filepath.Join(f.Path, e.Name())] {
				continue
			}
			if err := ctx.Err(); err != nil {
				_ = root.Close()
				return out, err
			}
			p, err := PlanFolder(root, e.Name())
			if err != nil {
				continue
			}
			if p.Empty() {
				continue
			}
			out = append(out, PlannedFolder{Root: f.Path, Label: f.Label, Plan: p})
		}
		_ = root.Close()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Plan.Folder < out[j].Plan.Folder })
	return out, nil
}

// ErrNotUnderAMediaFolder means the path asked for is not inside anything the
// user configured. Refused rather than resolved: this is the check that stops a
// crafted request renaming something elsewhere on the disk.
var ErrNotUnderAMediaFolder = errors.New("that folder is not inside a configured media folder")

// ApplyFolders renames the folders named by keys, which are absolute paths from
// a previous preview.
//
// The plan is recomputed here rather than carried over from the preview. The
// disk can have changed in between — a media server is writing to it — and
// acting on a stale plan is how a rename lands on a name that now belongs to
// something else. The preview is advisory; this is the decision.
func (s *Service) ApplyFolders(ctx context.Context, keys []string) (Summary, error) {
	roots, err := s.folders.List(ctx, folders.PurposeMovies)
	if err != nil {
		return Summary{}, fmt.Errorf("list movie folders: %w", err)
	}

	ignored, err := s.ignoredSet(ctx)
	if err != nil {
		return Summary{}, err
	}

	var sum Summary
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return sum, err
		}
		rootPath, rel, ok := locate(roots, key)
		if !ok {
			sum.Failed = append(sum.Failed, Skip{
				Name:   filepath.Base(key),
				Reason: ErrNotUnderAMediaFolder.Error(),
			})
			continue
		}
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			sum.Failed = append(sum.Failed, Skip{Name: rel, Reason: "no se pudo abrir la carpeta de medios"})
			continue
		}
		p, err := PlanFolder(root, rel)
		if err != nil {
			sum.Failed = append(sum.Failed, Skip{Name: rel, Reason: "no se pudo leer la carpeta"})
			_ = root.Close()
			continue
		}
		if p.Blocked != "" || p.Empty() {
			_ = root.Close()
			continue
		}
		// Checked again here, not only when listing. The preview and the apply
		// are separate requests, and an ignore added in between has to win —
		// otherwise the one case the button exists for is the one it misses.
		if ignored[filepath.Join(rootPath, rel)] {
			_ = root.Close()
			continue
		}
		res, err := Apply(root, p)
		_ = root.Close()
		if err != nil {
			sum.Failed = append(sum.Failed, Skip{Name: rel, Reason: err.Error()})
			continue
		}
		sum.Files += res.FilesRenamed
		if res.FolderRenamed {
			sum.Folders++
		}
		sum.Failed = append(sum.Failed, res.Failed...)
	}
	return sum, nil
}

// locate finds which configured folder a path belongs to and its name within
// it. It works on cleaned paths and requires a separator at the boundary, so
// "/mnt/Peliculas-viejas" is not treated as living inside "/mnt/Peliculas".
func locate(roots []folders.Folder, key string) (root, rel string, ok bool) {
	key = filepath.Clean(key)
	for _, f := range roots {
		base := filepath.Clean(f.Path)
		if key == base {
			continue // the media folder itself is not a film
		}
		if !strings.HasPrefix(key, base+string(filepath.Separator)) {
			continue
		}
		rel = strings.TrimPrefix(key, base+string(filepath.Separator))
		// One level down only: a film is a folder in the media folder, and
		// anything deeper is a request to rename something this never planned.
		if rel == "" || strings.Contains(rel, string(filepath.Separator)) {
			continue
		}
		return base, rel, true
	}
	return "", "", false
}

// Job kinds. Separate kinds so a preview and an apply cannot be mistaken for
// each other in the job history, which is the record of what touched the disk.
const (
	KindPreview = "naming-preview"
	KindApply   = "naming-apply"
)

// StartPreview works out the plans in the background. Planning reads every
// movie folder — hundreds of directory listings on a USB disk — which is too
// slow to do while a request waits.
func (s *Service) StartPreview(ctx context.Context) error {
	_, err := s.jobs.Start(KindPreview, func(ctx context.Context, p *jobs.Progress) (string, error) {
		restore := jobs.Deprioritise()
		defer restore()

		plans, err := s.PlanMovies(ctx)
		if err != nil {
			return "", err
		}
		s.mu.Lock()
		s.plans = plans
		s.mu.Unlock()

		var files, blocked int
		for _, pf := range plans {
			files += len(pf.Plan.Files)
			if pf.Plan.Blocked != "" {
				blocked++
			}
		}
		if len(plans) == 0 {
			return "No hay nada para renombrar.", nil
		}
		msg := fmt.Sprintf("%d carpetas y %d archivos para renombrar", len(plans)-blocked, files)
		if blocked > 0 {
			msg += fmt.Sprintf("; %d necesitan que decidas el año", blocked)
		}
		return msg, nil
	})
	return err
}

// Previewing reports whether a preview is running.
func (s *Service) Previewing() bool { return s.jobs.IsRunning(KindPreview) }

// Renaming reports whether an apply is running.
func (s *Service) Renaming() bool { return s.jobs.IsRunning(KindApply) }

// Plans returns the last preview.
func (s *Service) Plans() []PlannedFolder {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PlannedFolder, len(s.plans))
	copy(out, s.plans)
	return out
}

// LastPreviewJob returns the last preview job, for the status fragment.
func (s *Service) LastPreviewJob() (jobs.Job, bool) { return s.jobs.Latest(KindPreview) }

// LastApplyJob returns the last apply job.
func (s *Service) LastApplyJob() (jobs.Job, bool) { return s.jobs.Latest(KindApply) }

// StartApply renames the selected folders in the background, then re-plans so
// the screen shows what is left rather than what was true before.
func (s *Service) StartApply(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return errors.New("nothing selected")
	}
	sel := make([]string, len(keys))
	copy(sel, keys)

	_, err := s.jobs.Start(KindApply, func(ctx context.Context, p *jobs.Progress) (string, error) {
		restore := jobs.Deprioritise()
		defer restore()

		sum, err := s.ApplyFolders(ctx, sel)
		if err != nil {
			return "", err
		}

		// Re-plan so the page cannot keep offering work that is already done.
		// Best effort: the rename is what mattered and it already happened, so
		// a failure here must not be reported as the rename failing.
		if plans, err := s.PlanMovies(ctx); err == nil {
			s.mu.Lock()
			s.plans = plans
			s.mu.Unlock()
		}
		if _, err := s.Scan(ctx); err != nil {
			// Same reasoning: the cached issue list is a convenience.
			_ = err
		}

		msg := fmt.Sprintf("%d carpetas y %d archivos renombrados", sum.Folders, sum.Files)
		if n := len(sum.Failed); n > 0 {
			msg += fmt.Sprintf("; %d no se pudieron", n)
		}
		return msg, nil
	})
	return err
}
