package viewer

import (
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/util"
)

func proxyPeerSite(v Viewer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/p/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		peerID := parts[0]

		reqPath := "/index.html"
		if len(parts) == 2 && parts[1] != "" {
			reqPath = "/" + parts[1]
		}

		// Shared headers for all peer-served pages
		setPeerSiteHeaders := func(w http.ResponseWriter) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy",
				"default-src 'none'; "+
					"style-src 'self'; "+
					"script-src 'self'; "+
					"img-src 'self' data:; "+
					"font-src 'self' data:; "+
					"connect-src 'self'; "+
					"base-uri 'none'; "+
					"frame-ancestors 'none'",
			)
		}

		// ✅ Self short-circuit: serve local staged content
		if v.Node != nil && peerID == v.Node.ID() {
			if v.Content == nil {
				http.Error(w, "content store not configured", http.StatusInternalServerError)
				return
			}

			rel := strings.TrimPrefix(reqPath, "/")
			rel = path.Clean(rel)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "." || rel == "" {
				rel = "index.html"
			}

			ctx, cancel := context.WithTimeout(r.Context(), util.DefaultFetchTimeout)
			defer cancel()

			data, _, err := v.Content.Read(ctx, rel)
			if err != nil {
				http.NotFound(w, r)
				return
			}

			setPeerSiteHeaders(w)
			w.Header().Set("Content-Type", contentTypeForPath(rel, data))
			_, _ = w.Write(data)
			return
		}

		// 🌍 Remote peer proxy (untrusted content)
		ctx, cancel := context.WithTimeout(r.Context(), 2*util.DefaultFetchTimeout)
		defer cancel()

		mt, data, err := v.Node.FetchSiteFile(ctx, peerID, reqPath)
		if err != nil {
			msg := strings.ToLower(err.Error())
			switch {
			case strings.Contains(msg, "not found"):
				http.NotFound(w, r)
			case strings.Contains(msg, "forbidden"):
				http.Error(w, "forbidden", http.StatusForbidden)
			default:
				w.WriteHeader(http.StatusBadGateway)
				render.RenderStandalone(w, "page.error_unreachable", struct{ Detail string }{
					Detail: err.Error(),
				})
			}
			return
		}

		setPeerSiteHeaders(w)

		if mt == "" || strings.HasPrefix(mt, "text/plain") || strings.HasPrefix(mt, "application/octet-stream") {
			mt = contentTypeForPath(strings.TrimPrefix(reqPath, "/"), data)
		}
		w.Header().Set("Content-Type", mt)

		_, _ = w.Write(data)
	}
}
