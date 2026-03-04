package cluster

import (
	"fmt"
	"sync"
	"time"

	"crypto/rand"
	"encoding/hex"
)

// Queue manages jobs on the host side.
type Queue struct {
	mu   sync.Mutex
	jobs map[string]*JobState
}

// NewQueue creates an empty job queue.
func NewQueue() *Queue {
	return &Queue{jobs: make(map[string]*JobState)}
}

// Submit adds a job to the queue and returns its assigned ID.
func (q *Queue) Submit(job Job) string {
	q.mu.Lock()
	defer q.mu.Unlock()

	if job.ID == "" {
		job.ID = generateID()
	}
	q.jobs[job.ID] = &JobState{
		Job:       job,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	return job.ID
}

// Cancel marks a job as cancelled. Returns error if not found or already done.
func (q *Queue) Cancel(jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	js, ok := q.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}
	switch js.Status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return fmt.Errorf("job %s already in terminal state %s", jobID, js.Status)
	}
	js.Status = StatusCancelled
	js.DoneAt = time.Now()
	return nil
}

// NextPending returns the highest-priority pending job, or nil if none.
func (q *Queue) NextPending() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	var best *JobState
	for _, js := range q.jobs {
		if js.Status != StatusPending {
			continue
		}
		if best == nil || js.Job.Priority > best.Job.Priority {
			best = js
		}
	}
	if best == nil {
		return nil
	}
	j := best.Job
	return &j
}

// Assign marks a job as assigned to a worker.
func (q *Queue) Assign(jobID, workerID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	js, ok := q.jobs[jobID]
	if !ok {
		return
	}
	js.Status = StatusAssigned
	js.WorkerID = workerID
	js.StartedAt = time.Now()
}

// MarkRunning transitions a job from assigned to running.
func (q *Queue) MarkRunning(jobID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	js, ok := q.jobs[jobID]
	if !ok {
		return
	}
	if js.Status == StatusAssigned {
		js.Status = StatusRunning
	}
}

// Complete marks a job as successfully completed.
func (q *Queue) Complete(jobID string, result map[string]any) {
	q.mu.Lock()
	defer q.mu.Unlock()

	js, ok := q.jobs[jobID]
	if !ok {
		return
	}
	js.Status = StatusCompleted
	js.Result = result
	js.DoneAt = time.Now()
	if !js.StartedAt.IsZero() {
		js.ElapsedMs = js.DoneAt.Sub(js.StartedAt).Milliseconds()
	}
}

// Fail marks a job as failed. Re-queues if retries remain.
func (q *Queue) Fail(jobID string, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	js, ok := q.jobs[jobID]
	if !ok {
		return
	}
	js.Retries++
	if js.Retries <= js.Job.MaxRetry {
		js.Status = StatusPending
		js.WorkerID = ""
		js.Error = errMsg
		return
	}
	js.Status = StatusFailed
	js.Error = errMsg
	js.DoneAt = time.Now()
	if !js.StartedAt.IsZero() {
		js.ElapsedMs = js.DoneAt.Sub(js.StartedAt).Milliseconds()
	}
}

// State returns a snapshot of all jobs.
func (q *Queue) State() []JobState {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]JobState, 0, len(q.jobs))
	for _, js := range q.jobs {
		out = append(out, *js)
	}
	return out
}

// Get returns a single job state by ID.
func (q *Queue) Get(jobID string) (JobState, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	js, ok := q.jobs[jobID]
	if !ok {
		return JobState{}, false
	}
	return *js, true
}

// Stats returns aggregate counters.
func (q *Queue) Stats() QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()

	var s QueueStats
	for _, js := range q.jobs {
		switch js.Status {
		case StatusPending, StatusAssigned:
			s.Pending++
		case StatusRunning:
			s.Running++
		case StatusCompleted:
			s.Completed++
		case StatusFailed:
			s.Failed++
		}
	}
	return s
}

// WorkerJobID returns the job ID assigned to a worker, if any.
func (q *Queue) WorkerJobID(workerID string) string {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, js := range q.jobs {
		if js.WorkerID == workerID && (js.Status == StatusAssigned || js.Status == StatusRunning) {
			return js.Job.ID
		}
	}
	return ""
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
