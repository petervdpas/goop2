
package routes

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"path"
	"runtime"
	"strings"

	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func baseVM(title, active, contentTmpl string, d Deps) viewmodels.BaseVM {
	debug := false
	theme := "dark"
	verificationToken := ""

	// Reload config from disk to get latest theme/debug settings
	if d.CfgPath != "" {
		if cfg, err := config.Load(d.CfgPath); err == nil {
			debug = cfg.Viewer.Debug
			theme = cfg.Viewer.Theme
			verificationToken = cfg.Profile.VerificationToken
		}
	}

	if theme != "light" && theme != "dark" {
		theme = "dark"
	}
	selfID := ""
	if d.Node != nil {
		selfID = d.Node.ID()
	}

	return viewmodels.BaseVM{
		Title:                 title,
		Active:                active,
		ContentTmpl:           contentTmpl,
		SelfName:              safeCall(d.SelfLabel),
		SelfEmail:             safeCall(d.SelfEmail),
		SelfVerificationToken: verificationToken,
		SelfID:                selfID,
		BaseURL:               d.BaseURL,
		Debug:                 debug,
		Theme:                 theme,
		RendezvousOnly:        d.RendezvousOnly,
		RendezvousURL:         d.RendezvousURL,
		BridgeURL:             d.BridgeURL,
		WhichOS:               runtime.GOOS,
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

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".svg": true, ".ico": true, ".bmp": true,
}

func isImageExt(p string) bool {
	return imageExts[strings.ToLower(path.Ext(p))]
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

// validatePOSTRequest performs common POST request validation:
// - checks HTTP method is POST
// - verifies request is from localhost
// - parses form data
// - validates CSRF token
// Returns error if any check fails (error already sent to client).
func validatePOSTRequest(w http.ResponseWriter, r *http.Request, csrf string) error {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return http.ErrNotSupported
	}
	if !isLocalRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return http.ErrNotSupported
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return err
	}
	if r.PostForm.Get("csrf") != csrf {
		http.Error(w, "bad csrf", http.StatusForbidden)
		return http.ErrNotSupported
	}
	return nil
}

// getTrimmedFormValue returns a trimmed form value for the given key.
func getTrimmedFormValue(form http.Header, key string) string {
	return strings.TrimSpace(form.Get(key))
}

// getTrimmedPostFormValue returns a trimmed POST form value for the given key.
func getTrimmedPostFormValue(form map[string][]string, key string) string {
	values := form[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

// requireContentStore checks if content store is configured and sends error if not.
// Returns true if store is configured, false otherwise.
func requireContentStore(w http.ResponseWriter, store any) bool {
	if store == nil {
		http.Error(w, "content store not configured", http.StatusInternalServerError)
		return false
	}
	return true
}

// isValidTheme returns true for allowed theme values.
func isValidTheme(s string) bool {
	return s == "light" || s == "dark"
}

// formBool parses an HTML checkbox/toggle form value as a bool.
// Truthy values: "on", "1", "true", "yes" (case-insensitive).
func formBool(form map[string][]string, key string) bool {
	switch strings.ToLower(getTrimmedPostFormValue(form, key)) {
	case "on", "1", "true", "yes":
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// decodeJSON decodes a JSON request body into v and sends a 400 on failure.
// Returns nil on success; callers should return early on non-nil error.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return err
	}
	return nil
}

// requireMethod checks the HTTP method and sends 405 if it doesn't match.
// Returns true if the method is correct.
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// requireLocal checks that the request originates from localhost and sends
// 403 if it doesn't. Returns true if the request is local.
func requireLocal(w http.ResponseWriter, r *http.Request) bool {
	if !isLocalRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// fetchServiceHealth checks /healthz and optionally fetches extra fields
// from a status endpoint. Returns {"url", "ok", ...extracted fields}.
func fetchServiceHealth(client *http.Client, baseURL, statusPath string, fields []string) map[string]interface{} {
	base := strings.TrimRight(baseURL, "/")
	result := map[string]interface{}{
		"url": baseURL,
		"ok":  false,
	}
	if baseURL == "" {
		return result
	}

	resp, err := client.Get(base + "/healthz")
	if err != nil {
		return result
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result
	}
	result["ok"] = true

	if statusPath != "" {
		if r2, err := client.Get(base + statusPath); err == nil {
			var status map[string]interface{}
			json.NewDecoder(r2.Body).Decode(&status)
			r2.Body.Close()
			for _, f := range fields {
				if v, ok := status[f]; ok {
					result[f] = v
				}
			}
		}
	}
	return result
}

func sseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}
