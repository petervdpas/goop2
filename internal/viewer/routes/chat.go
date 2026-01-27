package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"goop/internal/chat"
)

// RegisterChat adds chat-related HTTP endpoints
func RegisterChat(mux *http.ServeMux, chatMgr *chat.Manager) {
	// Send a direct message
	mux.HandleFunc("/api/chat/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			To      string `json:"to"`
			Content string `json:"content"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "sent",
			"to":     req.To,
		})
	})

	// Get all messages
	mux.HandleFunc("/api/chat/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		peerID := r.URL.Query().Get("peer")

		var messages []*chat.Message
		if peerID != "" {
			// Get conversation with specific peer
			messages = chatMgr.GetConversation(peerID)
		} else {
			// Get all messages
			messages = chatMgr.GetMessages()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(messages)
	})

	// SSE endpoint for live chat updates
	mux.HandleFunc("/api/chat/events", func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

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

				// Send message as SSE event
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
