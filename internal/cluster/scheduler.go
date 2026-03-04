package cluster

import (
	"context"
	"log"
	"sync"
	"time"
)

const schedulerTick = 100 * time.Millisecond

// Scheduler matches pending jobs to idle workers.
type Scheduler struct {
	queue *Queue
	send  SendFunc

	mu      sync.Mutex
	workers map[string]*WorkerInfo
	robin   int // round-robin index
}

// NewScheduler creates a scheduler bound to a queue and send function.
func NewScheduler(queue *Queue, send SendFunc) *Scheduler {
	return &Scheduler{
		queue:   queue,
		send:    send,
		workers: make(map[string]*WorkerInfo),
	}
}

// AddWorker registers a worker.
func (s *Scheduler) AddWorker(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[peerID]; exists {
		s.workers[peerID].Status = WorkerIdle
		s.workers[peerID].LastSeen = time.Now()
		return
	}
	s.workers[peerID] = &WorkerInfo{
		PeerID:   peerID,
		Status:   WorkerIdle,
		Capacity: 1,
		LastSeen: time.Now(),
	}
}

// RemoveWorker unregisters a worker.
func (s *Scheduler) RemoveWorker(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workers, peerID)
}

// UpdateWorkerStatus updates a worker's status and last-seen timestamp.
func (s *Scheduler) UpdateWorkerStatus(peerID string, status WorkerStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.workers[peerID]
	if !ok {
		return
	}
	w.Status = status
	w.LastSeen = time.Now()
	if status == WorkerIdle {
		w.RunningJobs = 0
	}
}

// Workers returns a snapshot of all workers.
func (s *Scheduler) Workers() []WorkerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]WorkerInfo, 0, len(s.workers))
	for _, w := range s.workers {
		out = append(out, *w)
	}
	return out
}

// Run is the scheduler main loop. It polls the queue and dispatches jobs
// to idle workers. Blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context, groupID string) {
	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatch(groupID)
		}
	}
}

func (s *Scheduler) dispatch(groupID string) {
	for {
		job := s.queue.NextPending()
		if job == nil {
			return
		}

		worker := s.pickWorker()
		if worker == "" {
			return // no idle workers
		}

		s.queue.Assign(job.ID, worker)

		s.mu.Lock()
		if w, ok := s.workers[worker]; ok {
			w.Status = WorkerBusy
			w.RunningJobs++
		}
		s.mu.Unlock()

		topic := "cluster:" + groupID + ":job:assign"
		if err := s.send(worker, topic, map[string]any{
			"job_id":    job.ID,
			"type":      job.Type,
			"payload":   job.Payload,
			"timeout_s": job.TimeoutS,
		}); err != nil {
			log.Printf("CLUSTER: failed to send job %s to %s: %v", job.ID, worker, err)
			s.queue.Fail(job.ID, "send failed: "+err.Error())
			s.mu.Lock()
			if w, ok := s.workers[worker]; ok {
				w.Status = WorkerIdle
				w.RunningJobs--
			}
			s.mu.Unlock()
		}
	}
}

func (s *Scheduler) pickWorker() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(s.workers))
	for id, w := range s.workers {
		if w.Status == WorkerIdle && w.RunningJobs < w.Capacity {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	pick := ids[s.robin%len(ids)]
	s.robin++
	return pick
}
