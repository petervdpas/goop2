// internal/ui/assets.go
package assets

import (
	"embed"
	"net/http"
)

// Embed everything we serve under /assets/.
//
// IMPORTANT:
// - app.css is now a manifest that @imports ./css/*.css
// - so we must embed css/** as well, otherwise those imports 404.
//
//go:embed app.css app.js css/** vendor/**
var fs embed.FS

func Handler() http.Handler {
	return http.FileServer(http.FS(fs))
}
