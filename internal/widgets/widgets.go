// Package widgets defines the start page's tiles and a registry. Each feature
// contributes a Widget that says, in one number, how its area is doing.
package widgets

import (
	"context"
	"sync"

	"github.com/cristian/holocron/web/templates"
)

// Widget is one area on the start page.
type Widget interface {
	// ID is a stable slug, for tests and logs.
	ID() string
	// Tile reads the area's state as of now.
	Tile(ctx context.Context) templates.Tile
}

// Registry holds the registered widgets in display order.
type Registry struct {
	order []Widget
}

// NewRegistry builds a registry from the given widgets, preserving order.
func NewRegistry(ws ...Widget) *Registry {
	return &Registry{order: ws}
}

// Tiles reads every widget at once and returns their tiles in display order.
// In parallel because each asks a different service, and the page waits for
// the slowest rather than for the sum.
func (r *Registry) Tiles(ctx context.Context) []templates.Tile {
	out := make([]templates.Tile, len(r.order))
	var wg sync.WaitGroup
	for i, w := range r.order {
		wg.Go(func() { out[i] = w.Tile(ctx) })
	}
	wg.Wait()
	return out
}
