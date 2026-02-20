// Package sdk serves the public JavaScript SDK for site/template authors.
// Files are available at /sdk/goop-*.js and are separate from the viewer UI.
package sdk

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/js"
)

//go:embed *.js
var rawFS embed.FS

var minified map[string][]byte

func init() {
	m := minify.New()
	m.AddFunc("application/javascript", js.Minify)

	minified = make(map[string][]byte)

	_ = fs.WalkDir(rawFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.ToLower(filepath.Ext(path)) != ".js" {
			return nil
		}
		raw, err := rawFS.ReadFile(path)
		if err != nil {
			return nil
		}
		out, err := m.Bytes("application/javascript", raw)
		if err != nil {
			log.Printf("sdk: minify warning: %s: %v (using original)", path, err)
			minified[path] = raw
			return nil
		}
		minified[path] = out
		return nil
	})
}

// Handler returns an http.Handler that serves the SDK JS files.
// Mount it at /sdk/ with a StripPrefix.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if data, ok := minified[path]; ok {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	})
}
