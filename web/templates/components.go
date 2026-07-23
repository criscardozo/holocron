package templates

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// Grid renders the dashboard widget cards inside the grid container. The loop
// lives in Go rather than a templ range so the template stays a trivial shell.
func Grid(cards []templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div class="grid">`); err != nil {
			return err
		}
		for _, c := range cards {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
}
