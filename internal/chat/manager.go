package chat

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Manager owns the chat domain: message persistence (both directions),
// Lua command dispatch, and HTTP endpoints for history.
type Manager struct {
	selfID string
	store  Store
	mq     MQ

	luaMu sync.RWMutex
	lua   LuaDispatcher
}

func New(selfID string, store Store, mq MQ) *Manager {
	return &Manager{selfID: selfID, store: store, mq: mq}
}

// SetLuaDispatcher wires the Lua engine for "!" command handling.
// Safe to call after Start(); the dispatcher is checked under a read-lock.
func (m *Manager) SetLuaDispatcher(d LuaDispatcher) {
	m.luaMu.Lock()
	m.lua = d
	m.luaMu.Unlock()
}

// Start subscribes to inbound direct chat messages on the MQ bus.
// Broadcast messages are ephemeral (fire-and-forget) and not persisted.
func (m *Manager) Start() {
	m.mq.SubscribeTopic("chat", func(from, topic string, payload any) {
		if topic != "chat" {
			return
		}
		m.handleDirect(from, payload)
	})
}

func (m *Manager) handleDirect(from string, payload any) {
	content := extractContent(payload)
	if from == "" || content == "" {
		return
	}

	if err := m.store.StoreChatMessage(from, from, content, time.Now().UnixMilli()); err != nil {
		log.Printf("CHAT: persist incoming from %s failed: %v", from, err)
	}

	if strings.HasPrefix(content, "!") {
		m.luaMu.RLock()
		lua := m.lua
		m.luaMu.RUnlock()
		if lua != nil {
			lua.DispatchCommand(context.Background(), from, content, func(ctx context.Context, toPeerID, msg string) error {
				_, err := m.mq.Send(ctx, toPeerID, "chat", map[string]any{"content": msg})
				return err
			})
		}
	}
}

// PersistOutbound stores a message sent by the local user.
// Called from the MQ send handler after successful delivery.
func (m *Manager) PersistOutbound(peerID, content string) {
	if content == "" {
		return
	}
	if err := m.store.StoreChatMessage(peerID, m.selfID, content, time.Now().UnixMilli()); err != nil {
		log.Printf("CHAT: persist outbound to %s failed: %v", peerID, err)
	}
}

// RegisterHTTP registers the chat history endpoints on the given mux.
func (m *Manager) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/api/chat/history", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer_id")
		if peerID == "" {
			http.Error(w, "missing peer_id", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			msgs, err := m.store.GetChatHistory(peerID, 200)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, msgs)
		case http.MethodDelete:
			if err := m.store.ClearChatHistory(peerID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func extractContent(payload any) string {
	m, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	c, _ := m["content"].(string)
	return c
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
