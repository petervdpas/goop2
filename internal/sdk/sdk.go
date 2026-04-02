// Package sdk serves the public JavaScript and CSS SDK for site/template authors.
// Files are available at /sdk/goop-*.js and /sdk/goop-*.css.
package sdk

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/js"
)

//go:embed *.js *.css
var rawFS embed.FS

var minified map[string][]byte

func init() {
	m := minify.New()
	m.AddFunc("application/javascript", js.Minify)
	m.AddFunc("text/css", css.Minify)

	minified = make(map[string][]byte)

	_ = fs.WalkDir(rawFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		var mime string
		switch ext {
		case ".js":
			mime = "application/javascript"
		case ".css":
			mime = "text/css"
		default:
			return nil
		}
		raw, err := rawFS.ReadFile(path)
		if err != nil {
			return nil
		}
		out, err := m.Bytes(mime, raw)
		if err != nil {
			log.Printf("sdk: minify warning: %s: %v (using original)", path, err)
			minified[path] = raw
			return nil
		}
		minified[path] = out
		return nil
	})
}

// Handler returns an http.Handler that serves the SDK JS and CSS files.
// Mount it at /sdk/ with a StripPrefix.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if data, ok := minified[path]; ok {
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".js":
				w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			case ".css":
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
			}
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	})
}
