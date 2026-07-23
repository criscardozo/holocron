package widgets

import (
	"context"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/naming"
	"github.com/cristian/holocron/web/templates"
)

// NamingWidget shows how many media folders break the "Title (Year)"
// convention. Its refresh button (in the card template) triggers a re-scan.
type NamingWidget struct {
	naming *naming.Service
}

// NewNamingWidget creates a NamingWidget.
func NewNamingWidget(s *naming.Service) NamingWidget { return NamingWidget{naming: s} }

func (NamingWidget) ID() string    { return "naming" }
func (NamingWidget) Title() string { return "Nombres" }

// Card renders the cached count (no scan on dashboard load).
func (w NamingWidget) Card(ctx context.Context) templ.Component {
	view := templates.NamingCardView{HasMediaFolders: w.naming.HasMediaFolders(ctx)}
	if count, err := w.naming.Count(ctx); err == nil {
		view.Count = count
	}
	return templates.NamingCard(view)
}
