package cluster

import "time"

type JobType struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Template    string `json:"template"`
	Help        string `json:"help"`
}

var PredefinedJobTypes = []JobType{
	{
		Name: "calculate", Description: "Compute a result from inputs",
		Template: "{\n  \"expression\": \"\",\n  \"parameters\": {}\n}",
		Help:     "expression: the formula or function to evaluate. parameters: optional key-value pairs passed to the computation.",
	},
	{
		Name: "construct", Description: "Build or assemble output from parts",
		Template: "{\n  \"source\": \"\",\n  \"output\": \"\",\n  \"options\": {}\n}",
		Help:     "source: path or URL to input parts. output: where to write the result. options: format-specific settings.",
	},
	{
		Name: "transform", Description: "Convert input from one form to another",
		Template: "{\n  \"input\": \"\",\n  \"output\": \"\",\n  \"format\": \"\",\n  \"options\": {}\n}",
		Help:     "input: source file or URL. output: destination path. format: target format (e.g. webm, png, csv). options: quality, codec, etc.",
	},
	{
		Name: "search", Description: "Find or query across data",
		Template: "{\n  \"query\": \"\",\n  \"scope\": \"\",\n  \"limit\": 100\n}",
		Help:     "query: the search expression or SQL. scope: dataset or table to search. limit: max number of results to return.",
	},
	{
		Name: "validate", Description: "Check or verify something",
		Template: "{\n  \"target\": \"\",\n  \"rules\": {}\n}",
		Help:     "target: file path or resource to validate. rules: validation criteria as key-value pairs (e.g. schema version, strict mode).",
	},
	{
		Name: "distribute", Description: "Spread data to multiple targets",
		Template: "{\n  \"source\": \"\",\n  \"targets\": []\n}",
		Help:     "source: path or URL of data to distribute. targets: array of peer IDs or endpoint URLs to receive the data.",
	},
	{
		Name: "custom", Description: "User-defined job type",
		Template: "{\n  \"type_name\": \"\",\n  \"payload\": {}\n}",
		Help:     "type_name: your custom type identifier (sent as the job type). payload: free-form data for the worker.",
	},
}

type JobStore interface {
	SaveJob(groupID string, js *JobState) error
	LoadJobs(groupID string) ([]*JobState, error)
	DeleteJobs(groupID string) error
}

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

type JobMode string

const (
	JobOneshot    JobMode = "oneshot"
	JobContinuous JobMode = "continuous"
)

type Job struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Mode     JobMode        `json:"mode"`
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
