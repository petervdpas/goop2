package routes

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/petervdpas/goop2/internal/cluster"
	"github.com/petervdpas/goop2/internal/group"
)

func RegisterCluster(mux *http.ServeMux, cm *cluster.Manager, grpMgr *group.Manager, selfID string) {
	handleGet(mux, "/api/cluster/status", func(w http.ResponseWriter, r *http.Request) {
		role := cm.Role()
		resp := map[string]any{
			"role":     role,
			"group_id": cm.GroupID(),
		}
		switch role {
		case "host":
			resp["stats"] = cm.GetStats()
		case "worker":
			resp["binary_path"] = cm.BinaryPath()
		}
		writeJSON(w, resp)
	})

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

	handlePostAction(mux, "/api/cluster/leave", func(w http.ResponseWriter, r *http.Request) {
		cm.LeaveCluster()
		writeJSON(w, map[string]any{"status": "ok"})
	})

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

	handleGet(mux, "/api/cluster/jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs := cm.GetJobs()
		if jobs == nil {
			jobs = []cluster.JobState{}
		}
		writeJSON(w, jobs)
	})

	handleGet(mux, "/api/cluster/workers", func(w http.ResponseWriter, r *http.Request) {
		workers := cm.GetWorkers()
		if workers == nil {
			workers = []cluster.WorkerInfo{}
		}
		writeJSON(w, workers)
	})

	handleGet(mux, "/api/cluster/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, cm.GetStats())
	})

	// ── Worker API ──────────────────────────────────────────────────────────

	handlePost(mux, "/api/cluster/binary", func(w http.ResponseWriter, r *http.Request, req struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}) {
		if req.Path == "" {
			http.Error(w, "missing binary path", http.StatusBadRequest)
			return
		}
		if err := cm.SetBinary(req.Path, req.Mode); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"status": "ok", "path": req.Path, "mode": req.Mode})
	})
}
