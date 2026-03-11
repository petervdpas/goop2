package cluster

import "time"

type SendFunc func(peerID, topic string, payload any) error

type SubscribeFunc func(fn func(from, topic string, payload any)) func()

type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusAssigned  JobStatus = "assigned"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

type WorkerStatus string

const (
	WorkerJoined   WorkerStatus = "joined"
	WorkerVerified WorkerStatus = "verified"
	WorkerIdle     WorkerStatus = "idle"
	WorkerBusy     WorkerStatus = "busy"
	WorkerOffline  WorkerStatus = "offline"
)

type Job struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Payload  map[string]any `json:"payload,omitempty"`
	Priority int            `json:"priority"`
	TimeoutS int            `json:"timeout_s"`
	MaxRetry int            `json:"max_retry"`
}

type JobState struct {
	Job         Job            `json:"job"`
	Status      JobStatus      `json:"status"`
	WorkerID    string         `json:"worker_id,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	Progress    int            `json:"progress,omitempty"`
	ProgressMsg string         `json:"progress_msg,omitempty"`
	Retries     int            `json:"retries"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   time.Time      `json:"started_at,omitzero"`
	DoneAt      time.Time      `json:"done_at,omitzero"`
	ElapsedMs   int64          `json:"elapsed_ms,omitempty"`
}

type WorkerInfo struct {
	PeerID      string       `json:"peer_id"`
	Status      WorkerStatus `json:"status"`
	BinaryPath  string       `json:"binary_path,omitempty"`
	BinaryMode  string       `json:"binary_mode,omitempty"`
	Verified    bool         `json:"verified"`
	JobTypes    []string     `json:"job_types,omitempty"`
	Capacity    int          `json:"capacity"`
	RunningJobs int          `json:"running_jobs"`
	LastSeen    time.Time    `json:"last_seen"`
}

type QueueStats struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Workers   int `json:"workers"`
}

type GroupEvent struct {
	Type    string `json:"type"`
	Group   string `json:"group"`
	From    string `json:"from,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

type ClusterMessage struct {
	Type    string `json:"type"`
	GroupID string `json:"group"`
	From    string `json:"from"`
	Payload any    `json:"payload"`
}
