package cluster

import (
	"context"
	"log"
	"sync"
	"time"
)

// Worker runs jobs on the member side.
type Worker struct {
	send    SendFunc
	groupID string

	mu      sync.Mutex
	status  WorkerStatus
	running map[string]context.CancelFunc // jobID → cancel
}

// NewWorker creates a worker that reports results via send.
func NewWorker(send SendFunc, groupID string) *Worker {
	return &Worker{
		send:    send,
		groupID: groupID,
		status:  WorkerIdle,
		running: make(map[string]context.CancelFunc),
	}
}

// HandleJob runs a job in a goroutine with timeout, then sends the result back to the host.
func (w *Worker) HandleJob(hostPeerID string, job Job) {
	w.mu.Lock()
	w.status = WorkerBusy
	ctx, cancel := context.WithCancel(context.Background())
	if job.TimeoutS > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(job.TimeoutS)*time.Second)
	}
	w.running[job.ID] = cancel
	w.mu.Unlock()

	// Ack receipt
	topic := "cluster:" + w.groupID + ":job:ack"
	_ = w.send(hostPeerID, topic, map[string]any{"job_id": job.ID})

	go func() {
		defer cancel()
		defer func() {
			w.mu.Lock()
			delete(w.running, job.ID)
			if len(w.running) == 0 {
				w.status = WorkerIdle
			}
			w.mu.Unlock()
		}()

		result, err := w.executeJob(ctx, job)

		resultTopic := "cluster:" + w.groupID + ":job:result"
		if err != nil {
			_ = w.send(hostPeerID, resultTopic, map[string]any{
				"job_id": job.ID,
				"status": "failed",
				"error":  err.Error(),
			})
			return
		}
		_ = w.send(hostPeerID, resultTopic, map[string]any{
			"job_id": job.ID,
			"status": "completed",
			"result": result,
		})
	}()
}

// Cancel cancels a running job.
func (w *Worker) Cancel(jobID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if cancel, ok := w.running[jobID]; ok {
		cancel()
		delete(w.running, jobID)
		if len(w.running) == 0 {
			w.status = WorkerIdle
		}
	}
}

// Status returns the current worker status.
func (w *Worker) Status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

// RunningCount returns the number of currently running jobs.
func (w *Worker) RunningCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.running)
}

// Close cancels all running jobs.
func (w *Worker) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for id, cancel := range w.running {
		cancel()
		delete(w.running, id)
	}
	w.status = WorkerOffline
}

// executeJob is the v1 in-process job executor. Later phases will add a
// Processor interface for external binaries/plugins.
func (w *Worker) executeJob(ctx context.Context, job Job) (map[string]any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Millisecond):
		// v1: no-op processor, returns job type as acknowledgment
	}
	log.Printf("CLUSTER: worker executed job %s (type=%s)", job.ID, job.Type)
	return map[string]any{"type": job.Type, "status": "ok"}, nil
}
