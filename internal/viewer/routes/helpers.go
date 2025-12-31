// internal/viewer/routes/helpers.go

package routes

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"path"
	"strings"

	"goop/internal/ui/render"
)

func baseVM(title, active, contentTmpl string, d Deps) render.BaseVM {
	return render.BaseVM{
		Title:       title,
		Active:      active,
		ContentTmpl: contentTmpl,
		SelfName:    safeCall(d.SelfLabel),
		SelfID:      d.Node.ID(),
		BaseURL:     d.BaseURL,
	}
}

func safeCall(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

func newToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func atoiOrNeg(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return -1
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// normalizeRel is for FILE paths (editor open/save). Empty => index.html.
func normalizeRel(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.ReplaceAll(p, `\`, "/")
	p = path.Clean(p)
	if p == "." || p == "" {
		return "index.html"
	}
	return strings.TrimPrefix(p, "/")
}

// normalizeDirRel is for DIRECTORY fields (mkdir/new). Empty => "" (site root).
// If caller accidentally passes a FILE path (e.g. "index.html"), it returns its parent dir ("").
func normalizeDirRel(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.ReplaceAll(p, `\`, "/")
	p = path.Clean(p)

	if p == "." || p == "" {
		return ""
	}

	p = strings.TrimPrefix(p, "/")

	// If it looks like a file path, coerce to parent directory.
	if strings.Contains(path.Base(p), ".") {
		d := path.Dir(p)
		if d == "." || d == "/" {
			return ""
		}
		return strings.TrimPrefix(d, "/")
	}

	return p
}

func dirOf(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.ReplaceAll(p, `\`, "/")
	p = path.Clean(p)

	if p == "." || p == "" {
		return ""
	}

	d := path.Dir(p)
	if d == "." || d == "/" {
		return ""
	}
	return strings.TrimPrefix(d, "/")
}
