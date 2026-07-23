// Package widgets defines the dashboard widget contract and a registry. Each
// feature contributes a Widget; the dashboard renders their cards and exposes a
// per-widget refresh endpoint.
package widgets

import (
	"context"

	"github.com/a-h/templ"
)

// Widget is a self-contained dashboard panel.
type Widget interface {
	// ID is the stable slug used in the refresh URL (/widgets/{id}).
	ID() string
	// Title is the human-facing heading.
	Title() string
	// Card renders the full card (shell + body) as of now.
	Card(ctx context.Context) templ.Component
}

// Registry holds the registered widgets in display order.
type Registry struct {
	order []Widget
	byID  map[string]Widget
}

// NewRegistry builds a registry from the given widgets, preserving order.
func NewRegistry(ws ...Widget) *Registry {
	r := &Registry{byID: make(map[string]Widget, len(ws))}
	for _, w := range ws {
		r.order = append(r.order, w)
		r.byID[w.ID()] = w
	}
	return r
}

// Get returns the widget with the given id.
func (r *Registry) Get(id string) (Widget, bool) {
	w, ok := r.byID[id]
	return w, ok
}

// Cards renders every widget's card in display order.
func (r *Registry) Cards(ctx context.Context) []templ.Component {
	cards := make([]templ.Component, 0, len(r.order))
	for _, w := range r.order {
		cards = append(cards, w.Card(ctx))
	}
	return cards
}
