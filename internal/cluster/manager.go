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

type Manager struct {
	selfID    string
	send      SendFunc
	subscribe SubscribeFunc
	db JobStore

	mu             sync.Mutex
	role           role
	groupID        string
	cancel         context.CancelFunc
	queue          *Queue
	scheduler      *Scheduler
	worker         *Worker
	unsub          func()
	savedBinaryPath string
	savedBinaryMode string
}

func New(selfID string, send SendFunc, subscribe SubscribeFunc) *Manager {
	m := &Manager{
		selfID:    selfID,
		send:      send,
		subscribe: subscribe,
	}
	m.unsub = subscribe(func(from, topic string, payload any) {
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

func (m *Manager) SetDB(db JobStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
}

func (m *Manager) SetSavedBinary(path, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedBinaryPath = path
	m.savedBinaryMode = mode
}

func (m *Manager) HandleGroupEvent(evt *GroupEvent) {
	m.handleGroupEvent(evt)
}

func (m *Manager) CreateCluster(groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role == roleHost && m.groupID == groupID {
		return nil
	}
	if m.role != roleNone {
		return fmt.Errorf("already in a cluster (role=%d, group=%s)", m.role, m.groupID)
	}

	m.role = roleHost
	m.groupID = groupID
	m.queue = NewQueue(m.db, groupID)
	m.scheduler = NewScheduler(m.queue, m.send)

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.scheduler.Run(ctx, groupID)

	log.Printf("CLUSTER: created cluster %s (host)", groupID)
	return nil
}

func (m *Manager) JoinCluster(groupID, hostPeerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role == roleWorker && m.groupID == groupID {
		if m.worker != nil {
			go m.worker.Reannounce()
		}
		return nil
	}
	if m.role != roleNone {
		return fmt.Errorf("already in a cluster (role=%d, group=%s)", m.role, m.groupID)
	}

	m.role = roleWorker
	m.groupID = groupID
	m.worker = NewWorker(m.send, groupID, hostPeerID)

	log.Printf("CLUSTER: joined cluster %s (worker, host=%s)", groupID, hostPeerID)

	if m.savedBinaryPath != "" {
		w := m.worker
		path, mode := m.savedBinaryPath, m.savedBinaryMode
		go func() {
			if err := w.SetBinary(path, mode); err == nil {
				topic := "cluster:" + groupID + ":worker:binary"
				_ = m.send(hostPeerID, topic, map[string]any{
					"path": path,
					"mode": mode,
				})
			}
		}()
	}

	return nil
}

func (m *Manager) LeaveCluster() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanup()
}

// ── Host API ────────────────────────────────────────────────────────────────

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

func (m *Manager) CancelJob(jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost {
		return fmt.Errorf("not a cluster host")
	}

	js, ok := m.queue.Get(jobID)
	if ok && js.WorkerID != "" && (js.Status == StatusAssigned || js.Status == StatusRunning) {
		topic := "cluster:" + m.groupID + ":job:cancel"
		_ = m.send(js.WorkerID, topic, map[string]any{"job_id": jobID})
	}

	return m.queue.Cancel(jobID)
}

func (m *Manager) DeleteJob(jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost {
		return fmt.Errorf("not a cluster host")
	}
	return m.queue.Delete(jobID)
}

func (m *Manager) ClearJobs() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost {
		return fmt.Errorf("not a cluster host")
	}
	if m.queue != nil {
		m.queue.Clear()
	}
	if m.db != nil && m.groupID != "" {
		_ = m.db.DeleteJobs(m.groupID)
	}
	log.Printf("CLUSTER: job queue cleared")
	return nil
}

func (m *Manager) GetJobs() []JobState {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost || m.queue == nil {
		return nil
	}
	return m.queue.State()
}

func (m *Manager) GetWorkers() []WorkerInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost || m.scheduler == nil {
		return nil
	}
	return m.scheduler.Workers()
}

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

// ── Worker API ──────────────────────────────────────────────────────────────

func (m *Manager) SetBinary(path, mode string) error {
	m.mu.Lock()
	if m.role != roleWorker || m.worker == nil {
		m.mu.Unlock()
		return fmt.Errorf("not a cluster worker")
	}
	w := m.worker
	groupID := m.groupID
	send := m.send
	m.mu.Unlock()

	if err := w.SetBinary(path, mode); err != nil {
		return err
	}

	// Notify host that we set our binary
	topic := "cluster:" + groupID + ":worker:binary"
	_ = send(w.hostPeerID, topic, map[string]any{
		"path": path,
		"mode": mode,
	})

	return nil
}

func (m *Manager) BinaryPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.worker != nil {
		if p := m.worker.BinaryPath(); p != "" {
			return p
		}
	}
	return m.savedBinaryPath
}

func (m *Manager) BinaryMode() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.worker != nil {
		if p := m.worker.BinaryMode(); p != "" {
			return p
		}
	}
	return m.savedBinaryMode
}

func (m *Manager) PauseWorker() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleWorker || m.worker == nil {
		return fmt.Errorf("not a cluster worker")
	}
	m.worker.Pause()
	return nil
}

func (m *Manager) ResumeWorker() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleWorker || m.worker == nil {
		return fmt.Errorf("not a cluster worker")
	}
	m.worker.Resume()
	return nil
}

func (m *Manager) PauseRemoteWorker(peerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost {
		return fmt.Errorf("not a cluster host")
	}
	m.scheduler.UpdateWorkerStatus(peerID, WorkerPaused)
	topic := "cluster:" + m.groupID + ":worker:pause"
	return m.send(peerID, topic, map[string]any{"action": "pause"})
}

func (m *Manager) ResumeRemoteWorker(peerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.role != roleHost {
		return fmt.Errorf("not a cluster host")
	}
	m.scheduler.UpdateWorkerStatus(peerID, WorkerIdle)
	topic := "cluster:" + m.groupID + ":worker:resume"
	return m.send(peerID, topic, map[string]any{"action": "resume"})
}

func (m *Manager) WorkerStatus() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.worker == nil {
		return ""
	}
	return string(m.worker.Status())
}

// ── Common ──────────────────────────────────────────────────────────────────

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

func (m *Manager) GroupID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.groupID
}

func (m *Manager) Close() {
	if m.unsub != nil {
		m.unsub()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanup()
}

func (m *Manager) cleanup() {
	if m.role == roleHost && m.scheduler != nil && m.groupID != "" {
		topic := "cluster:" + m.groupID + ":shutdown"
		for _, w := range m.scheduler.Workers() {
			_ = m.send(w.PeerID, topic, map[string]any{"reason": "cluster closed"})
		}
		log.Printf("CLUSTER: sent shutdown to %d workers", len(m.scheduler.Workers()))
	}
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
