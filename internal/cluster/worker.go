package cluster

import (
	"context"
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
	send       SendFunc
	groupID    string
	hostPeerID string

	mu         sync.Mutex
	status     WorkerStatus
	binaryPath string
	binaryMode string
	verified   bool
	jobs       map[string]*activeJob
	runner     *BinaryRunner // Reusable runner for daemon mode
}

func NewWorker(send SendFunc, groupID, hostPeerID string) *Worker {
	return &Worker{
		send:       send,
		groupID:    groupID,
		hostPeerID: hostPeerID,
		status:     WorkerJoined,
		jobs:       make(map[string]*activeJob),
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

	go w.verify()
	return nil
}

func (w *Worker) verify() {
	w.mu.Lock()
	binaryPath := w.binaryPath
	w.mu.Unlock()

	if binaryPath == "" {
		return
	}

	checkJob := Job{
		ID:   "__check__",
		Type: "__check__",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RunOneshot(ctx, binaryPath, checkJob, nil)
	if err != nil {
		log.Printf("CLUSTER: check-job failed for %s: %v", binaryPath, err)
		w.SetVerified(false)
		w.sendVerified(false, nil, 0)
		return
	}

	var types []string
	if raw, _ := result["types"].([]any); raw != nil {
		for _, v := range raw {
			if s, _ := v.(string); s != "" {
				types = append(types, s)
			}
		}
	}
	capacity := 1
	if c, _ := result["capacity"].(float64); c > 0 {
		capacity = int(c)
	}

	w.SetVerified(true)
	log.Printf("CLUSTER: binary verified — types=%v capacity=%d", types, capacity)
	w.sendVerified(true, types, capacity)
}

func (w *Worker) sendVerified(ok bool, types []string, capacity int) {
	topic := "cluster:" + w.groupID + ":worker:verified"
	_ = w.send(w.hostPeerID, topic, map[string]any{
		"ok":       ok,
		"types":    types,
		"capacity": capacity,
	})
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

func (w *Worker) HandleJob(hostPeerID string, job Job) {
	w.mu.Lock()
	binaryPath := w.binaryPath
	if binaryPath == "" {
		w.mu.Unlock()
		log.Printf("CLUSTER: job %s rejected — no binary set", job.ID)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	if job.TimeoutS > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(job.TimeoutS)*time.Second)
	}

	w.status = WorkerBusy
	w.jobs[job.ID] = &activeJob{
		job:        job,
		hostPeerID: hostPeerID,
		startedAt:  time.Now(),
		cancel:     cancel,
	}
	w.mu.Unlock()

	topic := "cluster:" + w.groupID + ":job:ack"
	_ = w.send(hostPeerID, topic, map[string]any{"job_id": job.ID})
	log.Printf("CLUSTER: job %s (type=%s) running binary %s", job.ID, job.Type, binaryPath)

	go w.runJob(ctx, hostPeerID, job, binaryPath)
}

func (w *Worker) runJob(ctx context.Context, hostPeerID string, job Job, binaryPath string) {
	defer func() {
		w.mu.Lock()
		delete(w.jobs, job.ID)
		if len(w.jobs) == 0 {
			w.status = WorkerIdle
		}
		w.mu.Unlock()
	}()

	progressFn := func(pct int, msg string) {
		topic := "cluster:" + w.groupID + ":job:progress"
		_ = w.send(hostPeerID, topic, map[string]any{
			"job_id":  job.ID,
			"percent": pct,
			"message": msg,
		})
	}

	result, err := RunOneshot(ctx, binaryPath, job, progressFn)

	resultTopic := "cluster:" + w.groupID + ":job:result"
	if err != nil {
		log.Printf("CLUSTER: job %s failed: %v", job.ID, err)
		_ = w.send(hostPeerID, resultTopic, map[string]any{
			"job_id": job.ID,
			"status": "failed",
			"error":  err.Error(),
		})
		return
	}

	log.Printf("CLUSTER: job %s completed", job.ID)
	_ = w.send(hostPeerID, resultTopic, map[string]any{
		"job_id": job.ID,
		"status": "completed",
		"result": result,
	})
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
