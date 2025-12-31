package viewer

import (
	"mime"
	"net/http"
	"path"
	"strings"
)

// contentTypeForPath returns a browser-safe Content-Type for common site files.
// It intentionally overrides sniffing for .css/.js to avoid MIME mismatch blocking.
func contentTypeForPath(rel string, data []byte) string {
	ext := strings.ToLower(path.Ext(rel))

	// Hard rules for browser-enforced types
	switch ext {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	}

	// Best-effort for everything else
	if ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			return mt
		}
	}

	return http.DetectContentType(data)
}
