package routes

import (
	"net/http"
	"time"

	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/storage"
)

// RegisterChat wires the chat history endpoints and subscribes to incoming
// chat messages so they are persisted to the DB.
//
//	GET    /api/chat/history?peer_id=X  — last 200 messages with a peer
//	DELETE /api/chat/history?peer_id=X  — clear history with a peer
func RegisterChat(mux *http.ServeMux, db *storage.DB, mqMgr *mq.Manager, selfID string) {
	if db == nil || mqMgr == nil {
		return
	}

	// Persist incoming direct chat messages.
	mqMgr.SubscribeTopic("chat", func(from, _ string, payload any) {
		content := extractChatContent(payload)
		if from != "" && content != "" {
			_ = db.StoreChatMessage(from, from, content, time.Now().UnixMilli())
		}
	})

	// GET /api/chat/history?peer_id=X
	handleGet(mux, "/api/chat/history", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer_id")
		if peerID == "" {
			http.Error(w, "missing peer_id", http.StatusBadRequest)
			return
		}
		msgs, err := db.GetChatHistory(peerID, 200)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, msgs)
	})

	// DELETE /api/chat/history?peer_id=X
	mux.HandleFunc("/api/chat/history", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		peerID := r.URL.Query().Get("peer_id")
		if peerID == "" {
			http.Error(w, "missing peer_id", http.StatusBadRequest)
			return
		}
		if err := db.ClearChatHistory(peerID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})
}

func extractChatContent(payload any) string {
	if m, ok := payload.(map[string]any); ok {
		if c, ok := m["content"].(string); ok {
			return c
		}
	}
	return ""
}
