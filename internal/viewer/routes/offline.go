
package routes

import (
	"context"
	"net/http"

	"github.com/petervdpas/goop2/internal/proto"
)

func registerOfflineRoutes(mux *http.ServeMux, d Deps) {
	handlePostAction(mux, "/offline", func(w http.ResponseWriter, r *http.Request) {
		d.Node.Publish(context.Background(), proto.TypeOffline)
		http.Redirect(w, r, "/peers", http.StatusFound)
	})
}
