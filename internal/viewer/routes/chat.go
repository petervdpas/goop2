package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/petervdpas/goop2/internal/chat"
	"github.com/petervdpas/goop2/internal/state"
)

// RegisterChat adds chat-related HTTP endpoints
func RegisterChat(mux *http.ServeMux, chatMgr *chat.Manager, peers *state.PeerTable) {
	// Send a direct message
	handlePost(mux, "/api/chat/send", func(w http.ResponseWriter, r *http.Request, req struct {
		To      string `json:"to"`
		Content string `json:"content"`
	}) {
		if req.To == "" || req.Content == "" {
			http.Error(w, "Missing to or content", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := chatMgr.SendDirect(ctx, req.To, req.Content); err != nil {
			log.Printf("Failed to send chat message: %v", err)
			http.Error(w, fmt.Sprintf("Failed to send: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"status": "sent",
			"to":     req.To,
		})
	})

	// Send a broadcast message
	handlePost(mux, "/api/chat/broadcast", func(w http.ResponseWriter, r *http.Request, req struct {
		Content string `json:"content"`
	}) {
		if req.Content == "" {
			http.Error(w, "Missing content", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := chatMgr.SendBroadcast(ctx, req.Content, peers.IDs()); err != nil {
			log.Printf("Failed to send broadcast: %v", err)
			http.Error(w, fmt.Sprintf("Failed to broadcast: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"status": "sent",
			"type":   "broadcast",
		})
	})

	// Get broadcast messages
	handleGet(mux, "/api/chat/broadcasts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, chatMgr.GetBroadcasts())
	})

	// Get all messages
	handleGet(mux, "/api/chat/messages", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer")

		var messages []*chat.Message
		if peerID != "" {
			messages = chatMgr.GetConversation(peerID)
		} else {
			messages = chatMgr.GetMessages()
		}

		writeJSON(w, messages)
	})

	// SSE endpoint for live chat updates
	handleGet(mux, "/api/chat/events", func(w http.ResponseWriter, r *http.Request) {
		sseHeaders(w)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Subscribe to chat updates
		msgChan := chatMgr.Subscribe()
		defer chatMgr.Unsubscribe(msgChan)

		// Send initial connection message
		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgChan:
				if !ok {
					return
				}

				data, err := json.Marshal(msg)
				if err != nil {
					log.Printf("Failed to marshal chat message: %v", err)
					continue
				}

				fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	})
}
