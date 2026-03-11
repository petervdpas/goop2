package cluster

import (
	"fmt"
	"sync"
	"time"

	"crypto/rand"
	"encoding/hex"
)

type Queue struct {
	mu   sync.Mutex
	jobs map[string]*JobState
}

func NewQueue() *Queue {
	return &Queue{jobs: make(map[string]*JobState)}
}

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

func (q *Queue) UpdateProgress(jobID string, pct int, msg string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	js, ok := q.jobs[jobID]
	if !ok {
		return
	}
	js.Progress = pct
	js.ProgressMsg = msg
}

func (q *Queue) State() []JobState {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]JobState, 0, len(q.jobs))
	for _, js := range q.jobs {
		out = append(out, *js)
	}
	return out
}

func (q *Queue) Get(jobID string) (JobState, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	js, ok := q.jobs[jobID]
	if !ok {
		return JobState{}, false
	}
	return *js, true
}

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

func (q *Queue) WorkerJobIDs(workerID string) []string {
	q.mu.Lock()
	defer q.mu.Unlock()

	var ids []string
	for _, js := range q.jobs {
		if js.WorkerID == workerID && (js.Status == StatusAssigned || js.Status == StatusRunning) {
			ids = append(ids, js.Job.ID)
		}
	}
	return ids
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
