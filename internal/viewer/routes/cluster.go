package routes

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/petervdpas/goop2/internal/cluster"
	"github.com/petervdpas/goop2/internal/group"
)

// RegisterCluster adds cluster compute HTTP API endpoints.
func RegisterCluster(mux *http.ServeMux, cm *cluster.Manager, grpMgr *group.Manager, selfID string) {
	// GET /api/cluster/status — role, groupID, stats
	handleGet(mux, "/api/cluster/status", func(w http.ResponseWriter, r *http.Request) {
		role := cm.Role()
		resp := map[string]any{
			"role":     role,
			"group_id": cm.GroupID(),
		}
		if role == "host" {
			resp["stats"] = cm.GetStats()
		}
		writeJSON(w, resp)
	})

	// POST /api/cluster/create — create group (app_type="cluster") + cm.CreateCluster
	handlePost(mux, "/api/cluster/create", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			req.Name = "Cluster"
		}
		id := generateGroupID()
		if err := grpMgr.CreateGroup(id, req.Name, "cluster", 0, true); err != nil {
			http.Error(w, fmt.Sprintf("create group: %v", err), http.StatusInternalServerError)
			return
		}
		if err := cm.CreateCluster(id); err != nil {
			http.Error(w, fmt.Sprintf("create cluster: %v", err), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"status": "created", "group_id": id})
	})

	// POST /api/cluster/join — join remote cluster group as worker
	handlePost(mux, "/api/cluster/join", func(w http.ResponseWriter, r *http.Request, req struct {
		HostPeerID string `json:"host_peer_id"`
		GroupID    string `json:"group_id"`
	}) {
		if req.HostPeerID == "" || req.GroupID == "" {
			http.Error(w, "missing host_peer_id or group_id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := grpMgr.JoinRemoteGroup(ctx, req.HostPeerID, req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("join group: %v", err), http.StatusBadGateway)
			return
		}
		if err := cm.JoinCluster(req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("join cluster: %v", err), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"status": "joined", "group_id": req.GroupID})
	})

	// POST /api/cluster/leave — leave cluster
	handlePostAction(mux, "/api/cluster/leave", func(w http.ResponseWriter, r *http.Request) {
		cm.LeaveCluster()
		writeJSON(w, map[string]any{"status": "ok"})
	})

	// POST /api/cluster/submit — submit job (host only)
	handlePost(mux, "/api/cluster/submit", func(w http.ResponseWriter, r *http.Request, req struct {
		Type     string         `json:"type"`
		Payload  map[string]any `json:"payload,omitempty"`
		Priority int            `json:"priority"`
		TimeoutS int            `json:"timeout_s"`
		MaxRetry int            `json:"max_retry"`
	}) {
		if req.Type == "" {
			http.Error(w, "missing job type", http.StatusBadRequest)
			return
		}
		job := cluster.Job{
			Type:     req.Type,
			Payload:  req.Payload,
			Priority: req.Priority,
			TimeoutS: req.TimeoutS,
			MaxRetry: req.MaxRetry,
		}
		id, err := cm.SubmitJob(job)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"status": "submitted", "job_id": id})
	})

	// POST /api/cluster/cancel — cancel job (host only)
	handlePost(mux, "/api/cluster/cancel", func(w http.ResponseWriter, r *http.Request, req struct {
		JobID string `json:"job_id"`
	}) {
		if req.JobID == "" {
			http.Error(w, "missing job_id", http.StatusBadRequest)
			return
		}
		if err := cm.CancelJob(req.JobID); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"status": "cancelled", "job_id": req.JobID})
	})

	// GET /api/cluster/jobs — list all jobs (host only)
	handleGet(mux, "/api/cluster/jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs := cm.GetJobs()
		if jobs == nil {
			jobs = []cluster.JobState{}
		}
		writeJSON(w, jobs)
	})

	// GET /api/cluster/workers — list all workers (host only)
	handleGet(mux, "/api/cluster/workers", func(w http.ResponseWriter, r *http.Request) {
		workers := cm.GetWorkers()
		if workers == nil {
			workers = []cluster.WorkerInfo{}
		}
		writeJSON(w, workers)
	})

	// GET /api/cluster/stats — queue stats (host only)
	handleGet(mux, "/api/cluster/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, cm.GetStats())
	})
}
