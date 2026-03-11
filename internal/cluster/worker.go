package cluster

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type activeJob struct {
	job        Job
	hostPeerID string
	startedAt  time.Time
	cancel     func()
}

type Worker struct {
	send    SendFunc
	groupID string

	mu         sync.Mutex
	status     WorkerStatus
	binaryPath string
	binaryMode string
	verified   bool
	jobs       map[string]*activeJob
}

func NewWorker(send SendFunc, groupID string) *Worker {
	return &Worker{
		send:    send,
		groupID: groupID,
		status:  WorkerJoined,
		jobs:    make(map[string]*activeJob),
	}
}

func (w *Worker) SetBinary(path, mode string) error {
	if path == "" {
		return fmt.Errorf("binary path is empty")
	}
	if mode == "" {
		mode = "oneshot"
	}
	if mode != "oneshot" && mode != "daemon" {
		return fmt.Errorf("invalid mode %q (must be oneshot or daemon)", mode)
	}

	w.mu.Lock()
	w.binaryPath = path
	w.binaryMode = mode
	w.verified = false
	w.status = WorkerJoined
	w.mu.Unlock()

	log.Printf("CLUSTER: binary set to %s (mode=%s)", path, mode)
	return nil
}

func (w *Worker) BinaryPath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.binaryPath
}

func (w *Worker) BinaryMode() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.binaryMode
}

func (w *Worker) SetVerified(ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.verified = ok
	if ok {
		w.status = WorkerIdle
	} else {
		w.status = WorkerJoined
	}
}

func (w *Worker) Verified() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.verified
}

// HandleJob is called when a job:assign arrives from the host.
// TODO(phase3): run the configured binary with the job on stdin.
// For now, ACKs receipt and tracks the job internally.
func (w *Worker) HandleJob(hostPeerID string, job Job) {
	w.mu.Lock()
	w.status = WorkerBusy
	w.jobs[job.ID] = &activeJob{
		job:        job,
		hostPeerID: hostPeerID,
		startedAt:  time.Now(),
	}
	w.mu.Unlock()

	topic := "cluster:" + w.groupID + ":job:ack"
	_ = w.send(hostPeerID, topic, map[string]any{"job_id": job.ID})

	log.Printf("CLUSTER: job %s (type=%s) received", job.ID, job.Type)
}

func (w *Worker) Cancel(jobID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if aj, ok := w.jobs[jobID]; ok {
		if aj.cancel != nil {
			aj.cancel()
		}
		delete(w.jobs, jobID)
	}
	if len(w.jobs) == 0 {
		w.status = WorkerIdle
	}
}

func (w *Worker) Status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *Worker) RunningCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.jobs)
}

func (w *Worker) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for id, aj := range w.jobs {
		if aj.cancel != nil {
			aj.cancel()
		}
		delete(w.jobs, id)
	}
	w.status = WorkerOffline
}
