
package routes

import (
	"context"
	"net/http"
	"time"
)

func registerCreditsUIRoutes(mux *http.ServeMux, d Deps) {
	// GET /api/my-balance — returns this peer's credit balance from the
	// credits service (proxied via the rendezvous server).
	mux.HandleFunc("/api/my-balance", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if !requireLocal(w, r) {
			return
		}

		if len(d.RVClients) == 0 {
			writeJSON(w, map[string]any{"credits_active": false})
			return
		}

		selfID := ""
		if d.Node != nil {
			selfID = d.Node.ID()
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		for _, c := range d.RVClients {
			result, err := c.FetchBalance(ctx, selfID)
			if err == nil {
				writeJSON(w, result)
				return
			}
		}

		writeJSON(w, map[string]any{"credits_active": false})
	})
}
