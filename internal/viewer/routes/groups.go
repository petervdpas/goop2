package routes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"goop/internal/group"
	"goop/internal/storage"
)

// RegisterGroups adds group-related HTTP API endpoints.
func RegisterGroups(mux *http.ServeMux, grpMgr *group.Manager, selfID string) {
	// Create a hosted group
	mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				Name       string `json:"name"`
				AppType    string `json:"app_type"`
				MaxMembers int    `json:"max_members"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}
			if req.Name == "" {
				http.Error(w, "Missing name", http.StatusBadRequest)
				return
			}
			id := generateGroupID()
			if err := grpMgr.CreateGroup(id, req.Name, req.AppType, req.MaxMembers); err != nil {
				http.Error(w, fmt.Sprintf("Failed to create group: %v", err), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{
				"status": "created",
				"id":     id,
			})

		case http.MethodGet:
			groups, err := grpMgr.ListHostedGroups()
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to list groups: %v", err), http.StatusInternalServerError)
				return
			}
			if groups == nil {
				groups = []storage.GroupRow{}
			}

			// Enrich with member counts and host-in-group status
			type groupWithMembers struct {
				storage.GroupRow
				MemberCount int                `json:"member_count"`
				Members     []group.MemberInfo `json:"members"`
				HostInGroup bool               `json:"host_in_group"`
			}
			result := make([]groupWithMembers, len(groups))
			for i, g := range groups {
				members := grpMgr.HostedGroupMembers(g.ID)
				if members == nil {
					members = []group.MemberInfo{}
				}
				result[i] = groupWithMembers{
					GroupRow:    g,
					MemberCount: len(members),
					Members:     members,
					HostInGroup: grpMgr.HostInGroup(g.ID),
				}
			}

			writeJSON(w, result)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Host joins own group
	mux.HandleFunc("/api/groups/join-own", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			GroupID string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.GroupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}
		if err := grpMgr.JoinOwnGroup(req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("Failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "joined"})
	})

	// Host leaves own group
	mux.HandleFunc("/api/groups/leave-own", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			GroupID string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.GroupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}
		if err := grpMgr.LeaveOwnGroup(req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("Failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "left"})
	})

	// Close/delete a hosted group
	mux.HandleFunc("/api/groups/close", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			GroupID string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.GroupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}
		if err := grpMgr.CloseGroup(req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to close group: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "closed"})
	})

	// List subscriptions
	mux.HandleFunc("/api/groups/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		subs, err := grpMgr.ListSubscriptions()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list subscriptions: %v", err), http.StatusInternalServerError)
			return
		}

		// Also include active connection info
		hostPeer, groupID, connected := grpMgr.ActiveGroup()
		result := map[string]any{
			"subscriptions": subs,
			"active": map[string]any{
				"connected":    connected,
				"host_peer_id": hostPeer,
				"group_id":     groupID,
			},
		}

		writeJSON(w, result)
	})

	// Join a remote group
	mux.HandleFunc("/api/groups/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			HostPeerID string `json:"host_peer_id"`
			GroupID    string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.HostPeerID == "" || req.GroupID == "" {
			http.Error(w, "Missing host_peer_id or group_id", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := grpMgr.JoinRemoteGroup(ctx, req.HostPeerID, req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to join group: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "joined"})
	})

	// Invite a peer to a hosted group
	mux.HandleFunc("/api/groups/invite", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			GroupID string `json:"group_id"`
			PeerID  string `json:"peer_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.GroupID == "" || req.PeerID == "" {
			http.Error(w, "Missing group_id or peer_id", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := grpMgr.InvitePeer(ctx, req.PeerID, req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to invite peer: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "invited"})
	})

	// Rejoin a subscription (reconnect to a previously joined group)
	mux.HandleFunc("/api/groups/rejoin", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			HostPeerID string `json:"host_peer_id"`
			GroupID    string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.HostPeerID == "" || req.GroupID == "" {
			http.Error(w, "Missing host_peer_id or group_id", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := grpMgr.RejoinSubscription(ctx, req.HostPeerID, req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to rejoin: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "rejoined"})
	})

	// Remove a stale subscription
	mux.HandleFunc("/api/groups/subscriptions/remove", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			HostPeerID string `json:"host_peer_id"`
			GroupID    string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.HostPeerID == "" || req.GroupID == "" {
			http.Error(w, "Missing host_peer_id or group_id", http.StatusBadRequest)
			return
		}

		if err := grpMgr.RemoveSubscription(req.HostPeerID, req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to remove subscription: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "removed"})
	})

	// Leave current group
	mux.HandleFunc("/api/groups/leave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := grpMgr.LeaveGroup(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to leave group: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "left"})
	})

	// Send message to current group (client-side) or hosted group (host-side)
	mux.HandleFunc("/api/groups/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Payload any `json:"payload"`
			GroupID string      `json:"group_id"` // optional: if set, send as host to hosted group
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// If group_id is specified and we host that group, send as host
		if req.GroupID != "" && grpMgr.IsGroupHost(req.GroupID) {
			if err := grpMgr.SendToGroupAsHost(req.GroupID, req.Payload); err != nil {
				http.Error(w, fmt.Sprintf("Failed to send: %v", err), http.StatusInternalServerError)
				return
			}
		} else {
			// Send via active client connection
			if err := grpMgr.SendToGroup(req.Payload); err != nil {
				http.Error(w, fmt.Sprintf("Failed to send: %v", err), http.StatusInternalServerError)
				return
			}
		}

		writeJSON(w, map[string]string{"status": "sent"})
	})

	// SSE endpoint for group events
	mux.HandleFunc("/api/groups/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sseHeaders(w)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		evtChan := grpMgr.Subscribe()
		defer grpMgr.Unsubscribe(evtChan)

		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-evtChan:
				if !ok {
					return
				}
				data, err := json.Marshal(evt)
				if err != nil {
					log.Printf("GROUP: Failed to marshal event: %v", err)
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
				flusher.Flush()
			}
		}
	})
}

// generateGroupID returns a random 8-byte hex string (16 chars).
func generateGroupID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
