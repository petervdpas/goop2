package routes

import (
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// RegisterOpenRoute wires GET /open?url=... to open the system browser.
func RegisterOpenRoute(mux *http.ServeMux) {
	mux.HandleFunc("/open", openExternalHandler)
}

func openExternalHandler(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		http.Error(w, "scheme not allowed", http.StatusBadRequest)
		return
	}

	if err := openSystemBrowser(raw); err != nil {
		http.Error(w, fmt.Sprintf("failed to open browser: %v", err), http.StatusInternalServerError)
		return
	}

	// Go back where we came from (or /self if no referer)
	back := r.Referer()
	if back == "" {
		back = "/self"
	}
	http.Redirect(w, r, back, http.StatusFound)
}

func openSystemBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		// linux, bsd
		cmd = exec.Command("xdg-open", url)
	}

	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
