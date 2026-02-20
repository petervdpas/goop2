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

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/storage"
)

// RegisterGroups adds group-related HTTP API endpoints.
func RegisterGroups(mux *http.ServeMux, grpMgr *group.Manager, selfID string, peerName func(id string) string, peerReachable func(id string) bool) {
	// Create a hosted group / list hosted groups
	mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				Name       string `json:"name"`
				AppType    string `json:"app_type"`
				MaxMembers int    `json:"max_members"`
				Volatile   bool   `json:"volatile"`
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
			if err := grpMgr.CreateGroup(id, req.Name, req.AppType, req.MaxMembers, req.Volatile); err != nil {
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

			type memberWithName struct {
				group.MemberInfo
				Name string `json:"name,omitempty"`
			}
			type groupWithMembers struct {
				storage.GroupRow
				MemberCount int             `json:"member_count"`
				Members     []memberWithName `json:"members"`
				HostInGroup bool             `json:"host_in_group"`
			}
			result := make([]groupWithMembers, len(groups))
			for i, g := range groups {
				raw := grpMgr.HostedGroupMembers(g.ID)
				named := make([]memberWithName, 0, len(raw))
				for _, m := range raw {
					named = append(named, memberWithName{MemberInfo: m, Name: peerName(m.PeerID)})
				}
				result[i] = groupWithMembers{
					GroupRow:    g,
					MemberCount: len(named),
					Members:     named,
					HostInGroup: grpMgr.HostInGroup(g.ID),
				}
			}

			writeJSON(w, result)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Host joins own group
	handlePost(mux, "/api/groups/join-own", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
	}) {
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
	handlePost(mux, "/api/groups/leave-own", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
	}) {
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
	handlePost(mux, "/api/groups/close", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
	}) {
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
	handleGet(mux, "/api/groups/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		subs, err := grpMgr.ListSubscriptions()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list subscriptions: %v", err), http.StatusInternalServerError)
			return
		}

		type subWithCount struct {
			storage.SubscriptionRow
			HostName      string `json:"host_name"`
			HostReachable bool   `json:"host_reachable"`
			MemberCount   int    `json:"member_count"`
		}
		enriched := make([]subWithCount, len(subs))
		for i, s := range subs {
			enriched[i] = subWithCount{
				SubscriptionRow: s,
				HostName:        peerName(s.HostPeerID),
				HostReachable:   peerReachable(s.HostPeerID),
				MemberCount:     len(grpMgr.StoredGroupMembers(s.GroupID)),
			}
		}

		writeJSON(w, map[string]any{
			"subscriptions":  enriched,
			"active_groups":  grpMgr.ActiveGroups(),
		})
	})

	// Join a remote group
	handlePost(mux, "/api/groups/join", func(w http.ResponseWriter, r *http.Request, req struct {
		HostPeerID string `json:"host_peer_id"`
		GroupID    string `json:"group_id"`
	}) {
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
	handlePost(mux, "/api/groups/invite", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
		PeerID  string `json:"peer_id"`
	}) {
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
	handlePost(mux, "/api/groups/rejoin", func(w http.ResponseWriter, r *http.Request, req struct {
		HostPeerID string `json:"host_peer_id"`
		GroupID    string `json:"group_id"`
	}) {
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
	handlePost(mux, "/api/groups/subscriptions/remove", func(w http.ResponseWriter, r *http.Request, req struct {
		HostPeerID string `json:"host_peer_id"`
		GroupID    string `json:"group_id"`
	}) {
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

	// Leave a specific group
	handlePost(mux, "/api/groups/leave", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}
		if err := grpMgr.LeaveGroup(req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to leave group: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "left"})
	})

	// POST /api/groups/kick — remove a member from a hosted group
	handlePost(mux, "/api/groups/kick", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
		PeerID  string `json:"peer_id"`
	}) {
		if req.GroupID == "" || req.PeerID == "" {
			http.Error(w, "missing group_id or peer_id", http.StatusBadRequest)
			return
		}
		if err := grpMgr.KickMember(req.GroupID, req.PeerID); err != nil {
			http.Error(w, fmt.Sprintf("kick failed: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "kicked"})
	})

	// POST /api/groups/max-members — update max member limit for a hosted group
	handlePost(mux, "/api/groups/max-members", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID    string `json:"group_id"`
		MaxMembers int    `json:"max_members"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "missing group_id", http.StatusBadRequest)
			return
		}
		if err := grpMgr.SetMaxMembers(req.GroupID, req.MaxMembers); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// POST /api/groups/meta — update name and/or max_members for a hosted group
	handlePost(mux, "/api/groups/meta", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID    string `json:"group_id"`
		Name       string `json:"name"`
		MaxMembers int    `json:"max_members"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "missing group_id", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		if err := grpMgr.UpdateGroupMeta(req.GroupID, req.Name, req.MaxMembers); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// Send message to current group (client-side) or hosted group (host-side)
	handlePost(mux, "/api/groups/send", func(w http.ResponseWriter, r *http.Request, req struct {
		Payload any    `json:"payload"`
		GroupID string `json:"group_id"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "Missing group_id", http.StatusBadRequest)
			return
		}
		if grpMgr.IsGroupHost(req.GroupID) {
			if err := grpMgr.SendToGroupAsHost(req.GroupID, req.Payload); err != nil {
				http.Error(w, fmt.Sprintf("Failed to send: %v", err), http.StatusInternalServerError)
				return
			}
		} else {
			if err := grpMgr.SendToGroup(req.GroupID, req.Payload); err != nil {
				http.Error(w, fmt.Sprintf("Failed to send: %v", err), http.StatusInternalServerError)
				return
			}
		}

		writeJSON(w, map[string]string{"status": "sent"})
	})

	// SSE endpoint for group events
	handleGet(mux, "/api/groups/events", func(w http.ResponseWriter, r *http.Request) {
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
				// Internal protocol messages; never forward to browser.
				if evt.Type == group.TypePing || evt.Type == group.TypePong || evt.Type == group.TypeMeta {
					continue
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
