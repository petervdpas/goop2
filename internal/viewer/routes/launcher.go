// internal/viewer/routes/launcher.go

package routes

import (
	"net/http"
	"strings"

	"goop/internal/ui/render"
	"goop/internal/ui/viewmodels"
)

// LauncherController is implemented by your Wails App (or a thin adapter around it).
// Keep it small and UI-centric.
type LauncherController interface {
	ListPeers() ([]string, error)
	StartPeer(peerName string) error

	// ViewerURL should return something like "http://127.0.0.1:7777" (no trailing slash).
	// If your viewer is already mounted at /app, the launcher will redirect there.
	GetViewerURL() string
}

// RegisterLauncher registers the launcher at "/" on the given mux.
func RegisterLauncher(mux *http.ServeMux, c LauncherController) {
	// GET /  -> render launcher
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		peers, err := c.ListPeers()

		vm := viewmodels.LauncherVM{
			BaseVM: viewmodels.BaseVM{
				Title:       "Goop",
				Active:      "",
				ContentTmpl: "page.launcher",
				BaseURL:     "", // launcher lives at root
			},
			Peers: peers,
		}
		if err != nil {
			vm.Error = err.Error()
		}

		render.Render(w, vm)
	})

	// POST /start  -> start selected peer, then redirect to viewer UI
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		peer := strings.TrimSpace(r.PostForm.Get("peer"))
		if peer == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		if err := c.StartPeer(peer); err != nil {
			// Re-render launcher with error
			peers, _ := c.ListPeers()
			vm := viewmodels.LauncherVM{
				BaseVM: viewmodels.BaseVM{
					Title:       "Goop",
					Active:      "",
					ContentTmpl: "page.launcher",
					BaseURL:     "",
				},
				Peers: peers,
				Error: err.Error(),
			}
			render.Render(w, vm)
			return
		}

		// Redirect into the viewer UI.
		// Assumption: viewer UI is mounted under /app (as per your refactor direction).
		v := strings.TrimRight(strings.TrimSpace(c.GetViewerURL()), "/")
		if v == "" {
			// fallback: if viewer URL unknown, just go back to root
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		http.Redirect(w, r, v+"/app/peers", http.StatusFound)
	})
}
