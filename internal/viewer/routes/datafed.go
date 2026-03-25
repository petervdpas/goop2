package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/group_types/datafed"
	"github.com/petervdpas/goop2/internal/orm/schema"
)

func RegisterDataFed(mux *http.ServeMux, mgr *datafed.Manager) {
	if mgr == nil {
		return
	}

	handleGet(mux, "/api/datafed/groups", func(w http.ResponseWriter, r *http.Request) {
		ids := mgr.AllGroups()
		type groupInfo struct {
			GroupID       string                          `json:"group_id"`
			Contributions map[string][]string             `json:"contributions"`
		}
		result := make([]groupInfo, 0, len(ids))
		for _, id := range ids {
			contribs := mgr.GroupContributions(id)
			tableMap := make(map[string][]string, len(contribs))
			for peerID, c := range contribs {
				names := make([]string, len(c.Tables))
				for i, t := range c.Tables {
					names[i] = t.Name
				}
				tableMap[peerID] = names
			}
			result = append(result, groupInfo{
				GroupID:       id,
				Contributions: tableMap,
			})
		}
		writeJSON(w, result)
	})

	handlePost(mux, "/api/datafed/offer", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID       string               `json:"group_id"`
		Tables        []string             `json:"tables"`
		Relationships []datafed.Relationship `json:"relationships"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "group_id required", http.StatusBadRequest)
			return
		}
		if len(req.Tables) == 0 {
			http.Error(w, "at least one table required", http.StatusBadRequest)
			return
		}
		tables := mgr.ContextTablesForNames(req.Tables)
		if len(tables) == 0 {
			http.Error(w, "no matching context tables found", http.StatusBadRequest)
			return
		}
		mgr.OfferTables(req.GroupID, tables, req.Relationships)
		writeJSON(w, map[string]any{
			"status": "offered",
			"tables": len(tables),
		})
	})

	handlePost(mux, "/api/datafed/withdraw", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "group_id required", http.StatusBadRequest)
			return
		}
		mgr.WithdrawTables(req.GroupID)
		writeJSON(w, map[string]string{"status": "withdrawn"})
	})

	handlePost(mux, "/api/datafed/contributions", func(w http.ResponseWriter, r *http.Request, req struct {
		GroupID string `json:"group_id"`
	}) {
		if req.GroupID == "" {
			http.Error(w, "group_id required", http.StatusBadRequest)
			return
		}
		contribs := mgr.GroupContributions(req.GroupID)
		type peerContrib struct {
			PeerID        string               `json:"peer_id"`
			Tables        []schema.Table        `json:"tables"`
			Relationships []datafed.Relationship `json:"relationships"`
		}
		result := make([]peerContrib, 0, len(contribs))
		for _, c := range contribs {
			result = append(result, peerContrib{
				PeerID:        c.PeerID,
				Tables:        c.Tables,
				Relationships: c.Relationships,
			})
		}
		writeJSON(w, result)
	})
}
