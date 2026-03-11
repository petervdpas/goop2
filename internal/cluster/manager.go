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

	mu        sync.Mutex
	role      role
	groupID   string
	cancel    context.CancelFunc
	queue     *Queue
	scheduler *Scheduler
	worker    *Worker
	unsub     func()
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

func (m *Manager) HandleGroupEvent(evt *GroupEvent) {
	m.handleGroupEvent(evt)
}

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
	_ = send("", topic, map[string]any{
		"path": path,
		"mode": mode,
	})

	return nil
}

func (m *Manager) BinaryPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.worker == nil {
		return ""
	}
	return m.worker.BinaryPath()
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
