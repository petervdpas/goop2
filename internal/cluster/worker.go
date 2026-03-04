package cluster

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// PendingJob is a job waiting for an external executor to claim it.
type PendingJob struct {
	Job        Job       `json:"job"`
	HostPeerID string    `json:"host_peer_id"`
	ReceivedAt time.Time `json:"received_at"`
}

// Worker runs jobs on the member side. Jobs arrive via MQ and are parked
// for an external executor (browser, script, container) to claim via the
// local HTTP API. If no executor claims the job within ClaimTimeoutS, the
// built-in no-op handler runs as a fallback.
type Worker struct {
	send    SendFunc
	groupID string

	mu       sync.Mutex
	status   WorkerStatus
	pending  map[string]*PendingJob           // jobID → waiting for executor
	accepted map[string]*PendingJob           // jobID → claimed by executor
	running  map[string]func()                // jobID → cancel (for fallback only)
}

// NewWorker creates a worker that reports results via send.
func NewWorker(send SendFunc, groupID string) *Worker {
	return &Worker{
		send:     send,
		groupID:  groupID,
		status:   WorkerIdle,
		pending:  make(map[string]*PendingJob),
		accepted: make(map[string]*PendingJob),
		running:  make(map[string]func()),
	}
}

// HandleJob parks an incoming job for an external executor.
// Called by handleClusterMessage when a job:assign arrives from the host.
func (w *Worker) HandleJob(hostPeerID string, job Job) {
	w.mu.Lock()
	w.status = WorkerBusy
	w.pending[job.ID] = &PendingJob{
		Job:        job,
		HostPeerID: hostPeerID,
		ReceivedAt: time.Now(),
	}
	w.mu.Unlock()

	// Ack receipt to host
	topic := "cluster:" + w.groupID + ":job:ack"
	_ = w.send(hostPeerID, topic, map[string]any{"job_id": job.ID})

	log.Printf("CLUSTER: job %s (type=%s) parked for executor", job.ID, job.Type)
}

// PendingJobs returns all jobs waiting to be claimed by an executor.
func (w *Worker) PendingJobs() []PendingJob {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]PendingJob, 0, len(w.pending))
	for _, pj := range w.pending {
		out = append(out, *pj)
	}
	return out
}

// AcceptedJobs returns all jobs currently claimed by an executor.
func (w *Worker) AcceptedJobs() []PendingJob {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]PendingJob, 0, len(w.accepted))
	for _, pj := range w.accepted {
		out = append(out, *pj)
	}
	return out
}

// AcceptJob moves a job from pending to accepted. Returns the job and host peer ID.
func (w *Worker) AcceptJob(jobID string) (PendingJob, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	pj, ok := w.pending[jobID]
	if !ok {
		// Check if already accepted
		if _, accepted := w.accepted[jobID]; accepted {
			return PendingJob{}, fmt.Errorf("job %s already accepted", jobID)
		}
		return PendingJob{}, fmt.Errorf("job %s not found in pending", jobID)
	}

	w.accepted[jobID] = pj
	delete(w.pending, jobID)
	return *pj, nil
}

// ReportProgress sends a progress update to the host for an accepted job.
func (w *Worker) ReportProgress(jobID string, percent int, message string, stats map[string]any) error {
	w.mu.Lock()
	pj, ok := w.accepted[jobID]
	if !ok {
		w.mu.Unlock()
		return fmt.Errorf("job %s not accepted", jobID)
	}
	hostPeerID := pj.HostPeerID
	w.mu.Unlock()

	topic := "cluster:" + w.groupID + ":job:progress"
	return w.send(hostPeerID, topic, map[string]any{
		"job_id":  jobID,
		"percent": percent,
		"message": message,
		"stats":   stats,
	})
}

// ReportResult sends a completion or failure result to the host for an accepted job.
// The job is removed from the accepted set.
func (w *Worker) ReportResult(jobID string, succeeded bool, result map[string]any, errMsg string) error {
	w.mu.Lock()
	pj, ok := w.accepted[jobID]
	if !ok {
		w.mu.Unlock()
		return fmt.Errorf("job %s not accepted", jobID)
	}
	hostPeerID := pj.HostPeerID
	delete(w.accepted, jobID)
	if len(w.pending) == 0 && len(w.accepted) == 0 {
		w.status = WorkerIdle
	}
	w.mu.Unlock()

	topic := "cluster:" + w.groupID + ":job:result"
	payload := map[string]any{"job_id": jobID}
	if succeeded {
		payload["status"] = "completed"
		payload["result"] = result
	} else {
		payload["status"] = "failed"
		payload["error"] = errMsg
	}
	return w.send(hostPeerID, topic, payload)
}

// Heartbeat sends worker stats to the host.
func (w *Worker) Heartbeat(stats map[string]any) error {
	w.mu.Lock()
	// Find any host peer ID from pending or accepted jobs
	var hostPeerID string
	for _, pj := range w.accepted {
		hostPeerID = pj.HostPeerID
		break
	}
	if hostPeerID == "" {
		for _, pj := range w.pending {
			hostPeerID = pj.HostPeerID
			break
		}
	}
	w.mu.Unlock()

	if hostPeerID == "" {
		return fmt.Errorf("no host peer known")
	}

	topic := "cluster:" + w.groupID + ":worker:status"
	payload := map[string]any{"status": string(w.status)}
	if stats != nil {
		payload["stats"] = stats
	}
	return w.send(hostPeerID, topic, payload)
}

// Cancel cancels a running/pending/accepted job.
func (w *Worker) Cancel(jobID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.pending, jobID)
	delete(w.accepted, jobID)
	if cancel, ok := w.running[jobID]; ok {
		cancel()
		delete(w.running, jobID)
	}
	if len(w.pending) == 0 && len(w.accepted) == 0 && len(w.running) == 0 {
		w.status = WorkerIdle
	}
}

// Status returns the current worker status.
func (w *Worker) Status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

// RunningCount returns the number of jobs in any active state.
func (w *Worker) RunningCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pending) + len(w.accepted) + len(w.running)
}

// Close cancels all jobs.
func (w *Worker) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for id, cancel := range w.running {
		cancel()
		delete(w.running, id)
	}
	w.pending = make(map[string]*PendingJob)
	w.accepted = make(map[string]*PendingJob)
	w.status = WorkerOffline
}
