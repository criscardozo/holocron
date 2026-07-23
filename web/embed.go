// Package web embeds the static assets (HTMX, CSS, favicon) served by the app.
// The templ templates in web/templates compile to Go and are not embedded here.
package web

import "embed"

//go:embed static
var StaticFS embed.FS
