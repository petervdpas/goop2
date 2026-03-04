package cluster

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
)

type role int

const (
	roleNone role = iota
	roleHost
	roleWorker
)

// Manager is the unified entry point for cluster compute.
// It implements group.Handler (via HandleGroupEvent) for membership events
// and processes job protocol messages from the MQ adapter.
type Manager struct {
	selfID    string
	send      SendFunc
	subscribe SubscribeFunc

	mu        sync.Mutex
	role      role
	groupID   string
	cancel    context.CancelFunc
	queue     *Queue
	scheduler *Scheduler
	worker    *Worker
	unsub     func() // MQ topic unsubscribe
}

// New creates a cluster manager. send and subscribe are provided by the MQ adapter.
func New(selfID string, send SendFunc, subscribe SubscribeFunc) *Manager {
	m := &Manager{
		selfID:    selfID,
		send:      send,
		subscribe: subscribe,
	}
	// Subscribe to cluster topic messages
	m.unsub = subscribe(func(from, topic string, payload any) {
		// topic format: cluster:{groupID}:{msgType}
		// We receive the full topic from the adapter
		parts := strings.SplitN(topic, ":", 3)
		if len(parts) < 3 {
			return
		}
		groupID := parts[1]

		m.mu.Lock()
		activeGroup := m.groupID
		m.mu.Unlock()

		if activeGroup == "" || groupID != activeGroup {
			return
		}

		msgType := parts[2]
		m.handleClusterMessage(from, msgType, payload)
	})
	return m
}

// HandleGroupEvent implements group.Handler. Called by the group manager
// for membership events (join, leave, close) on cluster-type groups.
func (m *Manager) HandleGroupEvent(evt *GroupEvent) {
	m.handleGroupEvent(evt)
}

// CreateCluster sets up this node as the cluster host.
func (m *Manager) CreateCluster(groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleNone {
		return fmt.Errorf("already in a cluster (role=%d, group=%s)", m.role, m.groupID)
	}

	m.role = roleHost
	m.groupID = groupID
	m.queue = NewQueue()
	m.scheduler = NewScheduler(m.queue, m.send)

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.scheduler.Run(ctx, groupID)

	log.Printf("CLUSTER: created cluster %s (host)", groupID)
	return nil
}

// JoinCluster sets up this node as a cluster worker.
func (m *Manager) JoinCluster(groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleNone {
		return fmt.Errorf("already in a cluster (role=%d, group=%s)", m.role, m.groupID)
	}

	m.role = roleWorker
	m.groupID = groupID
	m.worker = NewWorker(m.send, groupID)

	log.Printf("CLUSTER: joined cluster %s (worker)", groupID)
	return nil
}

// LeaveCluster tears down the current cluster role.
func (m *Manager) LeaveCluster() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanup()
}

// SubmitJob adds a job to the queue (host only).
func (m *Manager) SubmitJob(job Job) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost {
		return "", fmt.Errorf("not a cluster host")
	}
	id := m.queue.Submit(job)
	log.Printf("CLUSTER: submitted job %s (type=%s, priority=%d)", id, job.Type, job.Priority)
	return id, nil
}

// CancelJob cancels a job (host only).
func (m *Manager) CancelJob(jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost {
		return fmt.Errorf("not a cluster host")
	}

	// Find worker and send cancel
	js, ok := m.queue.Get(jobID)
	if ok && js.WorkerID != "" && (js.Status == StatusAssigned || js.Status == StatusRunning) {
		topic := "cluster:" + m.groupID + ":job:cancel"
		_ = m.send(js.WorkerID, topic, map[string]any{"job_id": jobID})
	}

	return m.queue.Cancel(jobID)
}

// GetJobs returns all job states (host only).
func (m *Manager) GetJobs() []JobState {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost || m.queue == nil {
		return nil
	}
	return m.queue.State()
}

// GetWorkers returns all worker infos (host only).
func (m *Manager) GetWorkers() []WorkerInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost || m.scheduler == nil {
		return nil
	}
	return m.scheduler.Workers()
}

// GetStats returns queue statistics (host only).
func (m *Manager) GetStats() QueueStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost || m.queue == nil {
		return QueueStats{}
	}
	stats := m.queue.Stats()
	if m.scheduler != nil {
		stats.Workers = len(m.scheduler.Workers())
	}
	return stats
}

// ── Executor API (worker side) ───────────────────────────────────────────────

// PendingJobs returns jobs waiting for an executor to claim (worker only).
func (m *Manager) PendingJobs() []PendingJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleWorker || m.worker == nil {
		return nil
	}
	return m.worker.PendingJobs()
}

// AcceptedJobs returns jobs claimed by an executor (worker only).
func (m *Manager) AcceptedJobs() []PendingJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleWorker || m.worker == nil {
		return nil
	}
	return m.worker.AcceptedJobs()
}

// AcceptJob claims a pending job for execution (worker only).
func (m *Manager) AcceptJob(jobID string) (PendingJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleWorker || m.worker == nil {
		return PendingJob{}, fmt.Errorf("not a cluster worker")
	}
	return m.worker.AcceptJob(jobID)
}

// ReportProgress sends a progress update to the host (worker only).
func (m *Manager) ReportProgress(jobID string, percent int, message string, stats map[string]any) error {
	m.mu.Lock()
	if m.role != roleWorker || m.worker == nil {
		m.mu.Unlock()
		return fmt.Errorf("not a cluster worker")
	}
	w := m.worker
	m.mu.Unlock()

	return w.ReportProgress(jobID, percent, message, stats)
}

// ReportResult sends a job completion or failure to the host (worker only).
func (m *Manager) ReportResult(jobID string, succeeded bool, result map[string]any, errMsg string) error {
	m.mu.Lock()
	if m.role != roleWorker || m.worker == nil {
		m.mu.Unlock()
		return fmt.Errorf("not a cluster worker")
	}
	w := m.worker
	m.mu.Unlock()

	return w.ReportResult(jobID, succeeded, result, errMsg)
}

// WorkerHeartbeat sends worker stats to the host (worker only).
func (m *Manager) WorkerHeartbeat(stats map[string]any) error {
	m.mu.Lock()
	if m.role != roleWorker || m.worker == nil {
		m.mu.Unlock()
		return fmt.Errorf("not a cluster worker")
	}
	w := m.worker
	m.mu.Unlock()

	return w.Heartbeat(stats)
}

// Role returns "host", "worker", or "" for the current role.
func (m *Manager) Role() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.role {
	case roleHost:
		return "host"
	case roleWorker:
		return "worker"
	default:
		return ""
	}
}

// GroupID returns the active cluster group ID.
func (m *Manager) GroupID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.groupID
}

// Close shuts down the manager and releases all resources.
func (m *Manager) Close() {
	if m.unsub != nil {
		m.unsub()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanup()
}

// cleanup tears down the current role. Caller must hold m.mu.
func (m *Manager) cleanup() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.worker != nil {
		m.worker.Close()
		m.worker = nil
	}
	m.queue = nil
	m.scheduler = nil
	m.role = roleNone
	m.groupID = ""
}
