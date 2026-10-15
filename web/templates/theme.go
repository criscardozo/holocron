package templates

import "context"

// The colour scheme a person picked in Ajustes. Auto follows the system's.
const (
	ThemeAuto  = "auto"
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// ValidTheme reports whether t is one of the schemes.
func ValidTheme(t string) bool { return t == ThemeAuto || t == ThemeLight || t == ThemeDark }

type themeKey struct{}

// WithTheme carries the picked scheme to the layout, as WithMural does the
// posters: no page handler has to pass it along.
func WithTheme(ctx context.Context, theme string) context.Context {
	return context.WithValue(ctx, themeKey{}, theme)
}

func themeFrom(ctx context.Context) string {
	if t, ok := ctx.Value(themeKey{}).(string); ok && ValidTheme(t) {
		return t
	}
	return ThemeAuto
}

// themeOptions are the choices in Ajustes, in display order.
var themeOptions = []struct{ Value, Label, Icon string }{
	{ThemeAuto, "Automático", "gauge"},
	{ThemeLight, "Claro", "sun"},
	{ThemeDark, "Oscuro", "moon"},
}

// boolAttr writes a boolean ARIA attribute, which wants the words.
func boolAttr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
