package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/directchat"
)

// RegisterChat delegates chat HTTP endpoints to the chat.Manager.
func RegisterChat(mux *http.ServeMux, chatMgr *directchat.Manager) {
	if chatMgr == nil {
		return
	}
	chatMgr.RegisterHTTP(mux)
}
