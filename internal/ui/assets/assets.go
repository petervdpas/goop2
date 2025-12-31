// internal/ui/assets.go

package assets

import (
	"embed"
	"net/http"
)

// Embed everything we serve under /assets/.
//
//go:embed app.css app.js vendor/**
var fs embed.FS

func Handler() http.Handler {
	return http.FileServer(http.FS(fs))
}
