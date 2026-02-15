package viewer

import (
	"context"
	"net/http"
	"path"
	"strings"

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
				errDetail := err.Error()
				// HTML-escape the error detail for safe rendering
				errDetail = strings.ReplaceAll(errDetail, "&", "&amp;")
				errDetail = strings.ReplaceAll(errDetail, "<", "&lt;")
				errDetail = strings.ReplaceAll(errDetail, ">", "&gt;")
				errDetail = strings.ReplaceAll(errDetail, "\n", "<br>")
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusBadGateway)
				w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Peer Unreachable</title>
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#1a1a2e;color:#e0e0e0}
.box{text-align:center;max-width:600px}.box h1{font-size:1.4em;margin-bottom:8px}.box p{color:#999;margin:8px 0 20px}
.diag{text-align:left;font-size:11px;font-family:monospace;background:rgba(0,0,0,.3);padding:12px;border-radius:8px;margin:16px 0;color:#aaa;word-break:break-all;line-height:1.6}
a{display:inline-block;padding:10px 24px;border-radius:999px;background:rgba(108,140,255,.18);border:1px solid rgba(108,140,255,.35);color:#6c8cff;text-decoration:none}
a:hover{background:rgba(108,140,255,.28)}
.diag-details{margin:16px 0}.diag-summary{font-size:12px;color:#6c8cff;cursor:pointer;user-select:none}.diag-summary:hover{color:#8da8ff}
.copy-btn{margin-top:8px;padding:4px 12px;font-size:11px;border:1px solid rgba(108,140,255,.35);background:transparent;color:#6c8cff;border-radius:4px;cursor:pointer}
</style></head>
<body><div class="box"><h1>Peer Unreachable</h1><p>The peer is online but its relay connection may have dropped.</p>
<details class="diag-details"><summary class="diag-summary">Diagnostic info</summary><div class="diag" id="diag">` + errDetail + `</div>
<button class="copy-btn" onclick="navigator.clipboard.writeText(document.getElementById('diag').innerText).then(function(){this.textContent='Copied!'}.bind(this))">Copy</button></details>
<br><a href="javascript:location.reload()">Retry</a></div></body></html>`))
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
