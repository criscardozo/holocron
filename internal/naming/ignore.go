package naming

import (
	"context"
	"fmt"
	"strings"
)

// Some folders are not badly named media, they are simply not media. A tool's
// own directory sitting next to the films, or the bookkeeping exFAT and macOS
// leave behind, will never satisfy "Título (Año)" and flagging them forever
// trains people to skim past the list — which is the failure mode of any
// warning that is usually wrong.
//
// Two mechanisms, because there are two kinds. Hidden directories are handled
// without asking: nothing whose name starts with a dot is a film, so there is
// no judgement to make. Everything else is a judgement, so it is the user's:
// one button per row, and a way back.

// Hidden reports whether a directory name is one that is never media.
//
// Dot-prefixed covers .claude and the rest of a tool's working state, plus
// .Spotlight-V100 and .Trashes, which this library grows because it lives on
// exFAT and gets touched by a Mac. "System Volume Information" is the same
// thing from Windows and is not hidden by its name, so it is named here.
func Hidden(name string) bool {
	return strings.HasPrefix(name, ".") || name == "System Volume Information"
}

// Ignore marks a folder as one to leave alone. Idempotent: pressing the button
// twice is not an error, and neither is ignoring something already ignored by
// another tab.
func (s *Service) Ignore(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty path")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO naming_ignores (path) VALUES (?) ON CONFLICT(path) DO NOTHING`, path)
	if err != nil {
		return fmt.Errorf("ignore %q: %w", path, err)
	}
	return nil
}

// Unignore takes a folder off the list, so it can be flagged again.
func (s *Service) Unignore(ctx context.Context, path string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM naming_ignores WHERE path = ?`, path); err != nil {
		return fmt.Errorf("unignore %q: %w", path, err)
	}
	return nil
}

// IgnoredPaths lists what is being left alone, so the decision is visible and
// reversible. An ignore list nobody can see is a bug that looks like a feature.
func (s *Service) IgnoredPaths(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path FROM naming_ignores ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("list ignores: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan ignore: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ignoredSet is the lookup used while scanning and planning.
func (s *Service) ignoredSet(ctx context.Context) (map[string]bool, error) {
	paths, err := s.IgnoredPaths(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	return set, nil
}
