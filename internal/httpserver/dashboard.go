package httpserver

import (
	"context"
	"fmt"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/scanner"
	"github.com/cristian/holocron/web/templates"
)

// attentionChips builds the dashboard "Atención" strip: one chip per thing that
// needs action (invalid folder names, media missing Spanish subtitles, disks
// that are nearly full). All lookups are best-effort — a failing service just
// omits its chip rather than breaking the dashboard.
func (s *Server) attentionChips(ctx context.Context) []templates.AttnChip {
	var chips []templates.AttnChip

	if n, err := s.deps.Naming.Count(ctx); err == nil && n > 0 {
		chips = append(chips, templates.AttnChip{
			Label: fmt.Sprintf("%d nombres inválidos", n),
			Href:  "/naming",
			Icon:  "alert",
		})
	}

	if n, err := s.deps.Subtitles.MissingCount(ctx); err == nil && n > 0 {
		chips = append(chips, templates.AttnChip{
			Label: fmt.Sprintf("%d sin subtítulos", n),
			Href:  "/subtitles",
			Icon:  "cap",
		})
	}

	if list, err := s.deps.Folders.List(ctx, folders.PurposeDisk); err == nil {
		for _, f := range list {
			total, used, _, _, err := scanner.FilesystemStat(f.Path)
			if err != nil || total == 0 {
				continue
			}
			if pct := int(float64(used) / float64(total) * 100); pct >= 90 {
				chips = append(chips, templates.AttnChip{
					Label: fmt.Sprintf("%s %d%%", f.Label, pct),
					Href:  "/disk",
					Icon:  "drive",
				})
			}
		}
	}

	return chips
}
