package routes

import (
	"fmt"
	"net/http"

	"github.com/petervdpas/goop2/internal/group_types/chat"
	"github.com/petervdpas/goop2/internal/state"
)

// RegisterChatRooms adds chat room HTTP API endpoints.
func RegisterChatRooms(mux *http.ServeMux, cm *chat.Manager, _ func(string) state.PeerIdentityPayload) {
	selfID := cm.SelfID()

	// POST /api/chat/rooms/create
	handlePost(mux, "/api/chat/rooms/create", func(w http.ResponseWriter, r *http.Request, req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Context    string `json:"context"`
		MaxMembers  int    `json:"max_members"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		room, err := cm.CreateRoom(req.Name, req.Description, req.Context, req.MaxMembers)
		if err != nil {
			http.Error(w, fmt.Sprintf("create failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, room)
	})

	// POST /api/chat/rooms/close
	handlePost(mux, "/api/chat/rooms/close", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "group_id required", http.StatusBadRequest)
			return
		}
		if err := cm.CloseRoom(req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("close failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "closed"})
	})

	// POST /api/chat/rooms/join
	handlePost(mux, "/api/chat/rooms/join", func(w http.ResponseWriter, r *http.Request, req struct {
		HostPeerID string `json:"host_peer_id"`
		GroupID    string `json:"group_id"`
	}) {
		if req.HostPeerID == "" || req.GroupID == "" {
			http.Error(w, "host_peer_id and group_id required", http.StatusBadRequest)
			return
		}
		if err := cm.JoinRoom(r.Context(), req.HostPeerID, req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("join failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "joined"})
	})

	// POST /api/chat/rooms/leave
	handlePost(mux, "/api/chat/rooms/leave", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "group_id required", http.StatusBadRequest)
			return
		}
		if err := cm.LeaveRoom(req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("leave failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "left"})
	})

	// POST /api/chat/rooms/send
	handlePost(mux, "/api/chat/rooms/send", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
		Text    string `json:"text"`
	}) {
		if req.GroupID == "" || req.Text == "" {
			http.Error(w, "group_id and text required", http.StatusBadRequest)
			return
		}
		if err := cm.SendMessage(req.GroupID, selfID, req.Text); err != nil {
			http.Error(w, fmt.Sprintf("send failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "sent"})
	})

	// GET /api/chat/rooms/state?group_id=...
	handleGet(mux, "/api/chat/rooms/state", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("group_id")
		if groupID == "" {
			http.Error(w, "group_id required", http.StatusBadRequest)
			return
		}
		room, msgs, err := cm.GetState(groupID)
		if err != nil {
			http.Error(w, fmt.Sprintf("state failed: %v", err), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{
			"room":     room,
			"messages": msgs,
		})
	})
}
