package cluster

import (
	"context"
	"log"
	"sync"
	"time"
)


type Dispatcher struct {
	queue *Queue
	send  SendFunc

	mu      sync.Mutex
	workers map[string]*WorkerInfo
	robin   int
}

func NewDispatcher(queue *Queue, send SendFunc) *Dispatcher {
	return &Dispatcher{
		queue:   queue,
		send:    send,
		workers: make(map[string]*WorkerInfo),
	}
}

func (s *Dispatcher) AddWorker(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[peerID]; exists {
		s.workers[peerID].LastSeen = time.Now()
		return
	}
	s.workers[peerID] = &WorkerInfo{
		PeerID:   peerID,
		Status:   WorkerJoined,
		Capacity: 1,
		LastSeen: time.Now(),
	}
}

func (s *Dispatcher) RemoveWorker(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workers, peerID)
}

func (s *Dispatcher) SetWorkerVerified(peerID string, ok bool, types []string, capacity int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, exists := s.workers[peerID]
	if !exists {
		return
	}
	w.Verified = ok
	w.JobTypes = types
	if capacity > 0 {
		w.Capacity = capacity
	}
	if ok {
		w.Status = WorkerIdle
	} else {
		w.Status = WorkerJoined
	}
	w.LastSeen = time.Now()
	log.Printf("CLUSTER: worker %s verified=%v types=%v capacity=%d", peerID, ok, types, w.Capacity)
}

func (s *Dispatcher) SetWorkerBinary(peerID, path, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, exists := s.workers[peerID]
	if !exists {
		return
	}
	w.BinaryPath = path
	w.BinaryMode = mode
	w.LastSeen = time.Now()
}

func (s *Dispatcher) UpdateWorkerStatus(peerID string, status WorkerStatus) {
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

func (s *Dispatcher) Workers() []WorkerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]WorkerInfo, 0, len(s.workers))
	for _, w := range s.workers {
		out = append(out, *w)
	}
	return out
}

func (s *Dispatcher) Run(ctx context.Context, groupID string) {
	ticker := time.NewTicker(DispatchTick)
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

func (s *Dispatcher) dispatch(groupID string) {
	for {
		job := s.queue.NextPending()
		if job == nil {
			return
		}

		worker := s.pickWorkerForType(job.Type)
		if worker == "" {
			return
		}

		s.queue.Assign(job.ID, worker)
		log.Printf("CLUSTER: dispatching job %s (type=%s) → worker %s", job.ID, job.Type, worker)

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

func (s *Dispatcher) pickWorkerForType(jobType string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(s.workers))
	for id, w := range s.workers {
		if !w.Verified || w.Status == WorkerPaused || w.RunningJobs >= w.Capacity {
			continue
		}
		if len(w.JobTypes) == 0 || sliceContains(w.JobTypes, jobType) {
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

func sliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
