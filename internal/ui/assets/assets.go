package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/js"
)

// Embed everything we serve under /assets/.
//
// IMPORTANT:
// - app.css is now a manifest that @imports ./css/*.css
// - so we must embed css/** as well, otherwise those imports 404.
//
//go:embed app.css app.js css/** js/** vendor/** images/**
var rawFS embed.FS

// minified holds minified CSS/JS content keyed by path (e.g. "app.css").
// Populated once at init time; non-minifiable files are not stored here.
var minified map[string][]byte

func init() {
	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("application/javascript", js.Minify)

	minified = make(map[string][]byte)

	var totalOriginal, totalMinified int

	err := fs.WalkDir(rawFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		// Skip vendor files (already minified, e.g. CodeMirror)
		if strings.HasPrefix(path, "vendor/") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		var mediaType string
		switch ext {
		case ".css":
			mediaType = "text/css"
		case ".js":
			mediaType = "application/javascript"
		default:
			return nil
		}

		raw, err := rawFS.ReadFile(path)
		if err != nil {
			return nil
		}

		out, err := m.Bytes(mediaType, raw)
		if err != nil {
			log.Printf("minify: warning: %s: %v (using original)", path, err)
			return nil
		}

		totalOriginal += len(raw)
		totalMinified += len(out)
		minified[path] = out
		return nil
	})
	if err != nil {
		log.Printf("minify: walk error: %v", err)
	}

	saved := totalOriginal - totalMinified
	log.Printf("minified %d assets, saved %s (%d%% reduction)",
		len(minified),
		fmtBytes(saved),
		percent(saved, totalOriginal))
}

func Handler() http.Handler {
	// Fallback serves images, vendor files, and anything not minified.
	fallback := http.FileServer(http.FS(rawFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip leading slash to match embed paths.
		path := strings.TrimPrefix(r.URL.Path, "/")

		if data, ok := minified[path]; ok {
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".css":
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
			case ".js":
				w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			}
			w.Write(data)
			return
		}

		fallback.ServeHTTP(w, r)
	})
}

func fmtBytes(b int) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%.1f KB", float64(b)/1024)
}

func percent(saved, total int) int {
	if total == 0 {
		return 0
	}
	return saved * 100 / total
}
