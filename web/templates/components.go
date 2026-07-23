package templates

import (
	"context"
	"io"
	"strconv"

	"github.com/a-h/templ"
)

// intToStr formats an int64 id for use in form values and URLs.
func intToStr(v int64) string { return strconv.FormatInt(v, 10) }

// clampPct constrains a percentage to the 0..100 range for use in bar widths.
func clampPct(p int) int {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	default:
		return p
	}
}

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
