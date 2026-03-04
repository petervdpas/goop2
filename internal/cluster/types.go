package cluster

import "time"

// SendFunc sends a message to a specific peer on a topic.
// Implemented by the MQ adapter in internal/app.
type SendFunc func(peerID, topic string, payload any) error

// SubscribeFunc registers a callback for cluster messages.
// Returns an unsubscribe function.
type SubscribeFunc func(fn func(from, topic string, payload any)) func()

// JobStatus represents the lifecycle state of a job.
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusAssigned  JobStatus = "assigned"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

// WorkerStatus represents the state of a worker node.
type WorkerStatus string

const (
	WorkerIdle    WorkerStatus = "idle"
	WorkerBusy    WorkerStatus = "busy"
	WorkerOffline WorkerStatus = "offline"
)

// Job describes a unit of work to be dispatched to a worker.
type Job struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Payload  map[string]any `json:"payload,omitempty"`
	Priority int            `json:"priority"`
	TimeoutS int            `json:"timeout_s"`
	MaxRetry int            `json:"max_retry"`
}

// JobState tracks the full lifecycle of a job.
type JobState struct {
	Job       Job            `json:"job"`
	Status    JobStatus      `json:"status"`
	WorkerID  string         `json:"worker_id,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	Retries   int            `json:"retries"`
	CreatedAt time.Time      `json:"created_at"`
	StartedAt time.Time      `json:"started_at,omitzero"`
	DoneAt    time.Time      `json:"done_at,omitzero"`
	ElapsedMs int64          `json:"elapsed_ms,omitempty"`
}

// WorkerInfo describes a worker visible to the scheduler.
type WorkerInfo struct {
	PeerID      string       `json:"peer_id"`
	Status      WorkerStatus `json:"status"`
	Capacity    int          `json:"capacity"`
	RunningJobs int          `json:"running_jobs"`
	LastSeen    time.Time    `json:"last_seen"`
}

// QueueStats is a snapshot of queue counters.
type QueueStats struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Workers   int `json:"workers"`
}

// GroupEvent mirrors group.Event fields without importing the group package.
type GroupEvent struct {
	Type    string `json:"type"`
	Group   string `json:"group"`
	From    string `json:"from,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

// ClusterMessage is decoded from MQ topic payloads for job protocol messages.
type ClusterMessage struct {
	Type    string `json:"type"`    // e.g. "job:assign", "job:result"
	GroupID string `json:"group"`   // cluster group ID
	From    string `json:"from"`    // sender peer ID
	Payload any    `json:"payload"` // type-specific data
}
