package viewer

import "net/http"

// noCache disables all browser caching.
// This is REQUIRED for the editor and peer-local sites
// to behave like live files.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strong no-cache for dev / live-edit behavior
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// Prevent conditional caching
		w.Header().Del("ETag")
		w.Header().Del("Last-Modified")

		next.ServeHTTP(w, r)
	})
}
