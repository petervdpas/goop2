// Package routes — swaggo annotation stubs.
// Each function below is a documentation stub only; the real handler logic lives
// in the anonymous closures passed to handlePost/handleGet/mux.HandleFunc.
// Run `go generate` from the project root to regenerate ./docs/.
package routes

// ── Request / Response types ─────────────────────────────────────────────────

// mqSendRequest is the body for POST /api/mq/send.
type mqSendRequest struct {
	PeerID  string `json:"peer_id"  example:"12D3KooWXxx..."`
	Topic   string `json:"topic"    example:"call:nc-abc123"`
	Payload any    `json:"payload"`
	MsgID   string `json:"msg_id,omitempty" example:"uuid-optional"`
}

// mqSendResponse is the success body for POST /api/mq/send.
type mqSendResponse struct {
	MsgID  string `json:"msg_id"  example:"a1b2c3d4-..."`
	Status string `json:"status"  example:"delivered"`
}

// mqAckRequest is the body for POST /api/mq/ack.
type mqAckRequest struct {
	MsgID      string `json:"msg_id"                   example:"a1b2c3d4-..."`
	FromPeerID string `json:"from_peer_id,omitempty"   example:"12D3KooWXxx..."`
}

// callModeResponse is the body for GET /api/call/mode.
type callModeResponse struct {
	Mode     string `json:"mode"     example:"native"`
	Platform string `json:"platform" example:"linux"`
	First    bool   `json:"first"`
}

// callSessionStatus mirrors call.SessionStatus.
type callSessionStatus struct {
	ChannelID  string `json:"channel_id"`
	RemotePeer string `json:"remote_peer"`
	IsOrigin   bool   `json:"is_origin"`
	PCState    string `json:"pc_state"`
	AudioOn    bool   `json:"audio_on"`
	VideoOn    bool   `json:"video_on"`
	Hung       bool   `json:"hung"`
}

// callStartRequest is the body for POST /api/call/start and /api/call/accept.
type callStartRequest struct {
	ChannelID  string `json:"channel_id"  example:"nc-abc123"`
	RemotePeer string `json:"remote_peer" example:"12D3KooWXxx..."`
}

// callStartResponse is the body returned by /api/call/start and /api/call/accept.
type callStartResponse struct {
	Status    string `json:"status"     example:"started"`
	ChannelID string `json:"channel_id" example:"nc-abc123"`
}

// callChannelRequest is the body for /api/call/hangup, toggle-audio, toggle-video.
type callChannelRequest struct {
	ChannelID string `json:"channel_id" example:"nc-abc123"`
}

// callMuteResponse is the body for /api/call/toggle-audio.
type callMuteResponse struct {
	Muted bool `json:"muted"`
}

// callVideoResponse is the body for /api/call/toggle-video.
type callVideoResponse struct {
	Disabled bool `json:"disabled"`
}

// loopbackOfferRequest is the body for POST /api/call/loopback/{channel}/offer.
type loopbackOfferRequest struct {
	SDP string `json:"sdp"`
}

// loopbackOfferResponse is the body for POST /api/call/loopback/{channel}/offer.
type loopbackOfferResponse struct {
	SDP string `json:"sdp,omitempty"`
}

// loopbackICERequest is the body for POST /api/call/loopback/{channel}/ice.
type loopbackICERequest struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid"`
	SDPMLineIndex int    `json:"sdpMLineIndex"`
}

// groupCreateRequest is the body for POST /api/groups.
type groupCreateRequest struct {
	Name       string `json:"name"        example:"My Group"`
	AppType    string `json:"app_type,omitempty"`
	MaxMembers int    `json:"max_members,omitempty"`
	Volatile   bool   `json:"volatile,omitempty"`
}

// groupCreateResponse is the response for POST /api/groups.
type groupCreateResponse struct {
	Status string `json:"status" example:"created"`
	ID     string `json:"id"     example:"a1b2c3d4e5f6a1b2"`
}

// groupIDRequest is the body for single-group-id endpoints.
type groupIDRequest struct {
	GroupID string `json:"group_id" example:"a1b2c3d4e5f6a1b2"`
}

// groupPeerRequest is the body for invite / kick.
type groupPeerRequest struct {
	GroupID string `json:"group_id" example:"a1b2c3d4e5f6a1b2"`
	PeerID  string `json:"peer_id"  example:"12D3KooWXxx..."`
}

// groupHostJoinRequest is the body for join / rejoin / subscriptions/remove.
type groupHostJoinRequest struct {
	HostPeerID string `json:"host_peer_id" example:"12D3KooWXxx..."`
	GroupID    string `json:"group_id"     example:"a1b2c3d4e5f6a1b2"`
}

// groupMaxMembersRequest is the body for /api/groups/max-members.
type groupMaxMembersRequest struct {
	GroupID    string `json:"group_id"    example:"a1b2c3d4e5f6a1b2"`
	MaxMembers int    `json:"max_members" example:"20"`
}

// groupMetaRequest is the body for /api/groups/meta.
type groupMetaRequest struct {
	GroupID    string `json:"group_id"              example:"a1b2c3d4e5f6a1b2"`
	Name       string `json:"name"                  example:"New Name"`
	MaxMembers int    `json:"max_members,omitempty" example:"20"`
}

// groupSendRequest is the body for /api/groups/send.
type groupSendRequest struct {
	GroupID string `json:"group_id" example:"a1b2c3d4e5f6a1b2"`
	Payload any    `json:"payload"`
}

// listenGroup is the listen group state returned by create and state endpoints.
type listenGroup struct {
	ID         string          `json:"id"                    example:"listen-a1b2c3d4e5f6"`
	Name       string          `json:"name"                  example:"My Station"`
	Role       string          `json:"role"                  example:"host"`
	Track      *listenTrack    `json:"track,omitempty"`
	PlayState  *listenPlayState `json:"play_state,omitempty"`
	Listeners  []string        `json:"listeners,omitempty"`
	Queue      []string        `json:"queue,omitempty"`
	QueueTypes []string        `json:"queue_types,omitempty"`
	QueueIndex int             `json:"queue_index"`
	QueueTotal int             `json:"queue_total"`
}

// listenTrack describes the currently loaded audio track.
type listenTrack struct {
	Name     string  `json:"name"      example:"song.mp3"`
	Duration float64 `json:"duration"  example:"245.3"`
	Bitrate  int     `json:"bitrate"   example:"320000"`
	Format   string  `json:"format"    example:"mp3"`
	IsStream bool    `json:"is_stream" example:"false"`
}

// listenPlayState describes the current playback position.
type listenPlayState struct {
	Playing   bool    `json:"playing"    example:"true"`
	Position  float64 `json:"position"   example:"42.5"`
	UpdatedAt int64   `json:"updated_at" example:"1709136000000"`
}

// listenStateResponse is the body for GET /api/listen/state.
type listenStateResponse struct {
	Group         *listenGroup      `json:"group"`
	ListenerNames map[string]string `json:"listener_names,omitempty"`
}

// listenCreateRequest is the body for POST /api/listen/create.
type listenCreateRequest struct {
	Name string `json:"name,omitempty" example:"My Listen Group"`
}

// listenLoadRequest is the body for POST /api/listen/load.
type listenLoadRequest struct {
	FilePath  string   `json:"file_path,omitempty"`
	FilePaths []string `json:"file_paths,omitempty"`
}

// listenQueueAddRequest is the body for POST /api/listen/queue/add.
type listenQueueAddRequest struct {
	FilePaths []string `json:"file_paths"`
}

// listenControlRequest is the body for POST /api/listen/control.
type listenControlRequest struct {
	Action   string  `json:"action"             example:"play"`
	Position float64 `json:"position,omitempty" example:"42.5"`
	Index    int     `json:"index,omitempty"    example:"2"`
}

// listenJoinRequest is the body for POST /api/listen/join.
type listenJoinRequest struct {
	HostPeerID string `json:"host_peer_id" example:"12D3KooWXxx..."`
	GroupID    string `json:"group_id"     example:"a1b2c3d4e5f6a1b2"`
}

// quickSettingsRequest is the body for POST /api/settings/quick.
type quickSettingsRequest struct {
	Label              *string `json:"label,omitempty"`
	Email              *string `json:"email,omitempty"`
	VerificationToken  *string `json:"verification_token,omitempty"`
	Theme              *string `json:"theme,omitempty"`
	PreferredCam       *string `json:"preferred_cam,omitempty"`
	PreferredMic       *string `json:"preferred_mic,omitempty"`
	VideoDisabled      *bool   `json:"video_disabled,omitempty"`
	HideUnverified     *bool   `json:"hide_unverified,omitempty"`
	OpenSitesExternal  *bool   `json:"open_sites_external,omitempty"`
	UseServices        *bool   `json:"use_services,omitempty"`
}

// quickSettingsResponse is the body for GET /api/settings/quick/get.
type quickSettingsResponse struct {
	Label             string `json:"label"`
	Email             string `json:"email"`
	VerificationToken string `json:"verification_token"`
	Theme             string `json:"theme"`
	PreferredCam      string `json:"preferred_cam"`
	PreferredMic      string `json:"preferred_mic"`
	VideoDisabled     bool   `json:"video_disabled"`
	HideUnverified    bool   `json:"hide_unverified"`
	OpenSitesExternal bool   `json:"open_sites_external"`
}

// clientLogRequest is the body for POST /api/logs/client.
type clientLogRequest struct {
	Level   string `json:"level"   example:"warn"`
	Source  string `json:"source"  example:"call"`
	Message string `json:"message" example:"getUserMedia failed"`
}

// docsDeleteRequest is the body for POST /api/docs/delete.
type docsDeleteRequest struct {
	GroupID  string `json:"group_id"  example:"a1b2c3d4e5f6a1b2"`
	Filename string `json:"filename"  example:"notes.pdf"`
}

// dataTableRequest is the body for single-table endpoints.
type dataTableRequest struct {
	Table string `json:"table" example:"my_table"`
}

// dataTableCreateRequest is the body for POST /api/data/tables/create.
type dataTableCreateRequest struct {
	Name    string `json:"name"              example:"my_table"`
	Columns []any  `json:"columns,omitempty"`
}

// dataInsertRequest is the body for POST /api/data/insert.
type dataInsertRequest struct {
	Table string         `json:"table" example:"my_table"`
	Row   map[string]any `json:"row"`
}

// dataQueryRequest is the body for POST /api/data/query.
type dataQueryRequest struct {
	Table  string         `json:"table"            example:"my_table"`
	Filter map[string]any `json:"filter,omitempty"`
	Limit  int            `json:"limit,omitempty"  example:"100"`
}

// dataUpdateRequest is the body for POST /api/data/update.
type dataUpdateRequest struct {
	Table  string         `json:"table"  example:"my_table"`
	Filter map[string]any `json:"filter"`
	Set    map[string]any `json:"set"`
}

// dataDeleteRequest is the body for POST /api/data/delete.
type dataDeleteRequest struct {
	Table  string         `json:"table"  example:"my_table"`
	Filter map[string]any `json:"filter"`
}

// dataColumnRequest is the body for add-column / drop-column.
type dataColumnRequest struct {
	Table  string `json:"table"  example:"my_table"`
	Column any    `json:"column"`
}

// dataPolicyRequest is the body for POST /api/data/tables/set-policy.
type dataPolicyRequest struct {
	Table  string `json:"table"  example:"my_table"`
	Policy string `json:"policy" example:"group"`
}

// dataRenameRequest is the body for POST /api/data/tables/rename.
type dataRenameRequest struct {
	Table   string `json:"table"    example:"my_table"`
	NewName string `json:"new_name" example:"new_name"`
}

// statusOK is the generic {"status":"ok"} response body.
type statusOK struct {
	Status string `json:"status" example:"ok"`
}

// ── Groups response types ────────────────────────────────────────────────────

// groupMemberInfo describes a member in a hosted group listing.
type groupMemberInfo struct {
	PeerID   string `json:"peer_id"   example:"12D3KooWXxx..."`
	JoinedAt int64  `json:"joined_at" example:"1709136000000"`
	Name     string `json:"name"      example:"Roadwarrior"`
}

// hostedGroupInfo is one item in the GET /api/groups response.
type hostedGroupInfo struct {
	ID           string            `json:"id"             example:"a1b2c3d4e5f6a1b2"`
	Name         string            `json:"name"           example:"My Group"`
	AppType      string            `json:"app_type"       example:"listen"`
	MaxMembers   int               `json:"max_members"    example:"20"`
	Volatile     bool              `json:"volatile"`
	HostJoined   bool              `json:"host_joined"`
	CreatedAt    string            `json:"created_at"     example:"2026-03-08T12:00:00Z"`
	MemberCount  int               `json:"member_count"   example:"3"`
	Members      []groupMemberInfo `json:"members"`
	HostInGroup  bool              `json:"host_in_group"`
}

// subscriptionInfo is one item in the subscriptions list.
type subscriptionInfo struct {
	HostPeerID    string `json:"host_peer_id"    example:"12D3KooWXxx..."`
	GroupID       string `json:"group_id"        example:"a1b2c3d4e5f6a1b2"`
	GroupName     string `json:"group_name"      example:"My Group"`
	AppType       string `json:"app_type"        example:"listen"`
	MaxMembers    int    `json:"max_members"     example:"20"`
	Volatile      bool   `json:"volatile"`
	Role          string `json:"role"            example:"member"`
	SubscribedAt  string `json:"subscribed_at"   example:"2026-03-08T12:00:00Z"`
	HostName      string `json:"host_name"       example:"Eggman"`
	HostReachable bool   `json:"host_reachable"`
	MemberCount   int    `json:"member_count"    example:"3"`
}

// subscriptionsResponse is the body for GET /api/groups/subscriptions.
type subscriptionsResponse struct {
	Subscriptions []subscriptionInfo `json:"subscriptions"`
	ActiveGroups  []string           `json:"active_groups"`
}

// ── Call response types ──────────────────────────────────────────────────────

// callDebugResponse is the body for GET /api/call/debug.
type callDebugResponse struct {
	SessionCount int                 `json:"session_count" example:"2"`
	Sessions     []callSessionStatus `json:"sessions"`
}

// ── Cluster response types ───────────────────────────────────────────────────

// clusterJob describes a submitted job.
type clusterJob struct {
	ID       string         `json:"id"                example:"j-abc123"`
	Type     string         `json:"type"              example:"render"`
	Payload  map[string]any `json:"payload,omitempty"`
	Priority int            `json:"priority"          example:"5"`
	TimeoutS int            `json:"timeout_s"         example:"300"`
	MaxRetry int            `json:"max_retry"         example:"2"`
}

// clusterJobState describes a job with its execution state.
type clusterJobState struct {
	Job         clusterJob     `json:"job"`
	Status      string         `json:"status"               example:"running"`
	WorkerID    string         `json:"worker_id,omitempty"  example:"12D3KooWXxx..."`
	Result      map[string]any `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	Progress    int            `json:"progress,omitempty"    example:"42"`
	ProgressMsg string         `json:"progress_msg,omitempty" example:"rendering frame 84/200"`
	Retries     int            `json:"retries"              example:"0"`
	CreatedAt   string         `json:"created_at"           example:"2026-03-08T12:00:00Z"`
	StartedAt   string         `json:"started_at,omitempty"`
	DoneAt      string         `json:"done_at,omitempty"`
	ElapsedMs   int64          `json:"elapsed_ms,omitempty" example:"1234"`
}

// clusterWorkerInfo describes a cluster worker.
type clusterWorkerInfo struct {
	PeerID      string   `json:"peer_id"                example:"12D3KooWXxx..."`
	Status      string   `json:"status"                 example:"idle"`
	BinaryPath  string   `json:"binary_path,omitempty"  example:"/usr/bin/renderer"`
	BinaryMode  string   `json:"binary_mode,omitempty"  example:"oneshot"`
	Verified    bool     `json:"verified"               example:"true"`
	JobTypes    []string `json:"job_types,omitempty"`
	Capacity    int      `json:"capacity"               example:"4"`
	RunningJobs int      `json:"running_jobs"           example:"1"`
	LastSeen    string   `json:"last_seen"              example:"2026-03-08T12:00:00Z"`
}

// clusterQueueStats describes cluster queue statistics.
type clusterQueueStats struct {
	Pending   int `json:"pending"   example:"3"`
	Running   int `json:"running"   example:"2"`
	Completed int `json:"completed" example:"15"`
	Failed    int `json:"failed"    example:"1"`
	Workers   int `json:"workers"   example:"4"`
}

// ── Peer response types ──────────────────────────────────────────────────────

// peerContentResponse is the body for GET /api/peer/content.
type peerContentResponse struct {
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// peerFavoriteRequest is the body for POST /api/peers/favorite.
type peerFavoriteRequest struct {
	PeerID   string `json:"peer_id"  example:"12D3KooWXxx..."`
	Favorite bool   `json:"favorite" example:"true"`
}

// ── Avatar response types ────────────────────────────────────────────────────

// avatarUploadResponse is the body for POST /api/avatar/upload.
type avatarUploadResponse struct {
	OK   bool   `json:"ok"   example:"true"`
	Hash string `json:"hash" example:"a1b2c3d4"`
}

// ── Data response types ──────────────────────────────────────────────────────

// dataInsertResponse is the body for POST /api/data/insert.
type dataInsertResponse struct {
	Status string `json:"status" example:"inserted"`
	ID     int64  `json:"id"     example:"42"`
}

// ── Services response types ──────────────────────────────────────────────────

// serviceHealthEntry describes the health of one service.
type serviceHealthEntry struct {
	OK     bool   `json:"ok"`
	Status any    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

// servicesHealthResponse is the body for GET /api/services/health.
type servicesHealthResponse struct {
	Registration *serviceHealthEntry `json:"registration,omitempty"`
	Credits      *serviceHealthEntry `json:"credits,omitempty"`
	Email        *serviceHealthEntry `json:"email,omitempty"`
	Templates    *serviceHealthEntry `json:"templates,omitempty"`
	Bridge       *serviceHealthEntry `json:"bridge,omitempty"`
	Encryption   *serviceHealthEntry `json:"encryption,omitempty"`
}

// ── Docs response types ──────────────────────────────────────────────────────

// docFileInfo describes a shared file.
type docFileInfo struct {
	Name    string `json:"name"     example:"notes.pdf"`
	Size    int64  `json:"size"     example:"1048576"`
	ModTime string `json:"mod_time" example:"2026-03-08T12:00:00Z"`
}

// ── Cluster ──────────────────────────────────────────────────────────────────

// clusterStatusResponse is the body for GET /api/cluster/status.
type clusterStatusResponse struct {
	Role         string             `json:"role"                    example:"host"`
	GroupID      string             `json:"group_id"                example:"a1b2c3d4e5f6a1b2"`
	Stats        *clusterQueueStats `json:"stats,omitempty"`
	BinaryPath   string             `json:"binary_path,omitempty"   example:"/usr/bin/renderer"`
	BinaryMode   string             `json:"binary_mode,omitempty"   example:"oneshot"`
	WorkerStatus string             `json:"worker_status,omitempty" example:"idle"`
}

// clusterCreateRequest is the body for POST /api/cluster/create.
type clusterCreateRequest struct {
	Name    string `json:"name"              example:"My Cluster"`
	GroupID string `json:"group_id,omitempty" example:"a1b2c3d4e5f6a1b2"`
}

// clusterCreateResponse is the body for POST /api/cluster/create.
type clusterCreateResponse struct {
	Status  string `json:"status"   example:"created"`
	GroupID string `json:"group_id" example:"a1b2c3d4e5f6a1b2"`
}

// clusterJoinRequest is the body for POST /api/cluster/join.
type clusterJoinRequest struct {
	HostPeerID string `json:"host_peer_id" example:"12D3KooWXxx..."`
	GroupID    string `json:"group_id"     example:"a1b2c3d4e5f6a1b2"`
}

// clusterSubmitRequest is the body for POST /api/cluster/submit.
type clusterSubmitRequest struct {
	Type     string         `json:"type"                example:"calculate"`
	Mode     string         `json:"mode,omitempty"      example:"oneshot"`
	Payload  map[string]any `json:"payload,omitempty"`
	Priority int            `json:"priority,omitempty"  example:"5"`
	TimeoutS int            `json:"timeout_s,omitempty" example:"300"`
	MaxRetry int            `json:"max_retry,omitempty" example:"2"`
}

// clusterSubmitResponse is the body for POST /api/cluster/submit.
type clusterSubmitResponse struct {
	Status string `json:"status" example:"submitted"`
	JobID  string `json:"job_id" example:"j-abc123"`
}

// clusterCancelRequest is the body for POST /api/cluster/cancel.
type clusterCancelRequest struct {
	JobID string `json:"job_id" example:"j-abc123"`
}

// swagClusterStatus is a documentation stub for GET /api/cluster/status.
//
//	@Summary	Current cluster role and group
//	@Description	Returns the current role (host, worker, or empty), the active group ID, and stats if host.
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{object}	clusterStatusResponse
//	@Router		/api/cluster/status [get]
func swagClusterStatus() {}

// swagClusterCreate is a documentation stub for POST /api/cluster/create.
//
//	@Summary	Create or activate a cluster (become host)
//	@Description	Creates a new cluster group and sets this node as host. If group_id is provided, activates an existing group instead of creating a new one.
//	@Tags		cluster
//	@Accept		json
//	@Produce	json
//	@Param		body	body		clusterCreateRequest	true	"Create request"
//	@Success	200		{object}	clusterCreateResponse
//	@Failure	409		{string}	string	"already in a cluster"
//	@Router		/api/cluster/create [post]
func swagClusterCreate() {}

// swagClusterJoin is a documentation stub for POST /api/cluster/join.
//
//	@Summary	Join an existing cluster as worker
//	@Description	Joins a remote cluster group and registers this node as a worker.
//	@Tags		cluster
//	@Accept		json
//	@Produce	json
//	@Param		body	body		clusterJoinRequest	true	"Join request"
//	@Success	200		{object}	statusOK
//	@Failure	400		{string}	string	"missing host_peer_id or group_id"
//	@Failure	409		{string}	string	"already in a cluster"
//	@Failure	502		{string}	string	"failed to join group"
//	@Router		/api/cluster/join [post]
func swagClusterJoin() {}

// swagClusterLeave is a documentation stub for POST /api/cluster/leave.
//
//	@Summary	Close the current cluster
//	@Description	Sends shutdown to all workers, leaves the cluster, and deletes the group.
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{object}	statusOK
//	@Router		/api/cluster/leave [post]
func swagClusterLeave() {}

// swagClusterSubmit is a documentation stub for POST /api/cluster/submit.
//
//	@Summary	Submit a job to the cluster queue (host only)
//	@Tags		cluster
//	@Accept		json
//	@Produce	json
//	@Param		body	body		clusterSubmitRequest	true	"Job submission"
//	@Success	200		{object}	clusterSubmitResponse
//	@Failure	400		{string}	string	"missing job type"
//	@Failure	409		{string}	string	"not a cluster host"
//	@Router		/api/cluster/submit [post]
func swagClusterSubmit() {}

// swagClusterCancel is a documentation stub for POST /api/cluster/cancel.
//
//	@Summary	Cancel a job (host only)
//	@Tags		cluster
//	@Accept		json
//	@Produce	json
//	@Param		body	body		clusterCancelRequest	true	"Cancel request"
//	@Success	200		{object}	statusOK
//	@Failure	400		{string}	string	"missing job_id"
//	@Failure	409		{string}	string	"not a cluster host"
//	@Router		/api/cluster/cancel [post]
func swagClusterCancel() {}

// clusterDeleteRequest is the body for POST /api/cluster/delete.
type clusterDeleteRequest struct {
	JobID string `json:"job_id" example:"j-abc123"`
}

// swagClusterDelete is a documentation stub for POST /api/cluster/delete.
//
//	@Summary	Delete a terminal job from the queue (host only)
//	@Description	Removes a cancelled, completed, or failed job from the queue and database. Active jobs (pending, assigned, running) cannot be deleted — cancel them first.
//	@Tags		cluster
//	@Accept		json
//	@Produce	json
//	@Param		body	body		clusterDeleteRequest	true	"Delete request"
//	@Success	200		{object}	statusOK
//	@Failure	400		{string}	string	"missing job_id"
//	@Failure	409		{string}	string	"job is pending — cancel it first"
//	@Router		/api/cluster/delete [post]
func swagClusterDelete() {}

// swagClusterClear is a documentation stub for POST /api/cluster/clear.
//
//	@Summary	Clear the entire job queue (host only)
//	@Description	Removes all jobs (pending, completed, failed, cancelled) from the queue and database. This is irreversible.
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{object}	statusOK
//	@Failure	409	{string}	string	"not a cluster host"
//	@Router		/api/cluster/clear [post]
func swagClusterClear() {}

// swagClusterJobs is a documentation stub for GET /api/cluster/jobs.
//
//	@Summary	List all jobs in the queue (host only)
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{array}	clusterJobState
//	@Router		/api/cluster/jobs [get]
func swagClusterJobs() {}

// swagClusterWorkers is a documentation stub for GET /api/cluster/workers.
//
//	@Summary	List all workers (host only)
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{array}	clusterWorkerInfo
//	@Router		/api/cluster/workers [get]
func swagClusterWorkers() {}

// swagClusterStats is a documentation stub for GET /api/cluster/stats.
//
//	@Summary	Queue statistics (host only)
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{object}	clusterQueueStats
//	@Router		/api/cluster/stats [get]
func swagClusterStats() {}

// swagClusterTypes is a documentation stub for GET /api/cluster/types.
//
//	@Summary	List predefined job types with payload templates
//	@Description	Returns the 7 predefined job types (calculate, construct, transform, search, validate, distribute, custom) with descriptions, JSON payload templates, and help text.
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{array}	cluster.JobType
//	@Router		/api/cluster/types [get]
func swagClusterTypes() {}

// ── Cluster Worker API ───────────────────────────────────────────────────────

// clusterBinaryRequest is the body for POST /api/cluster/binary.
type clusterBinaryRequest struct {
	Path string `json:"path" example:"/usr/bin/renderer"`
	Mode string `json:"mode" example:"oneshot"`
}

// clusterBinaryResponse is the body for POST /api/cluster/binary.
type clusterBinaryResponse struct {
	Status string `json:"status" example:"ok"`
	Path   string `json:"path"   example:"/usr/bin/renderer"`
	Mode   string `json:"mode"   example:"oneshot"`
}

// swagClusterBinary is a documentation stub for POST /api/cluster/binary.
//
//	@Summary	Set the binary path for this worker (worker only)
//	@Description	Sets the local binary that will execute jobs for this cluster. The binary is a child process that speaks the goop2 cluster JSON protocol over stdin/stdout. Mode can be "oneshot" (started per job) or "daemon" (long-running). Setting the binary resets the verified flag — the host must re-verify via check-job before dispatching work.
//	@Tags		cluster
//	@Accept		json
//	@Produce	json
//	@Param		body	body		clusterBinaryRequest	true	"Binary configuration"
//	@Success	200		{object}	clusterBinaryResponse
//	@Failure	400		{string}	string	"missing binary path"
//	@Failure	409		{string}	string	"not a cluster worker"
//	@Router		/api/cluster/binary [post]
func swagClusterBinary() {}

// swagClusterPause is a documentation stub for POST /api/cluster/pause.
//
//	@Summary	Pause this worker (worker only)
//	@Description	Pauses the worker so the host scheduler skips it when dispatching jobs. The worker stays in the cluster. Use POST /api/cluster/resume to resume.
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{object}	statusOK
//	@Failure	409	{string}	string	"not a cluster worker"
//	@Router		/api/cluster/pause [post]
func swagClusterPause() {}

// swagClusterResume is a documentation stub for POST /api/cluster/resume.
//
//	@Summary	Resume this worker (worker only)
//	@Description	Resumes a paused worker so the host scheduler can dispatch jobs to it again.
//	@Tags		cluster
//	@Produce	json
//	@Success	200	{object}	statusOK
//	@Failure	409	{string}	string	"not a cluster worker"
//	@Router		/api/cluster/resume [post]
func swagClusterResume() {}

// clusterWorkerPeerRequest is the body for POST /api/cluster/worker/pause and /resume.
type clusterWorkerPeerRequest struct {
	PeerID string `json:"peer_id" example:"12D3KooWXxx..."`
}

// swagClusterWorkerPause is a documentation stub for POST /api/cluster/worker/pause.
//
//	@Summary	Pause a remote worker (host only)
//	@Description	Sends a pause command to the specified worker. The worker stops accepting new jobs but remains in the cluster. Existing running jobs are not interrupted.
//	@Tags		cluster
//	@Accept		json
//	@Produce	json
//	@Param		body	body		clusterWorkerPeerRequest	true	"Worker peer ID"
//	@Success	200		{object}	statusOK
//	@Failure	400		{string}	string	"missing peer_id"
//	@Failure	409		{string}	string	"not a cluster host"
//	@Router		/api/cluster/worker/pause [post]
func swagClusterWorkerPause() {}

// swagClusterWorkerResume is a documentation stub for POST /api/cluster/worker/resume.
//
//	@Summary	Resume a remote worker (host only)
//	@Description	Sends a resume command to the specified worker so the scheduler can dispatch jobs to it again.
//	@Tags		cluster
//	@Accept		json
//	@Produce	json
//	@Param		body	body		clusterWorkerPeerRequest	true	"Worker peer ID"
//	@Success	200		{object}	statusOK
//	@Failure	400		{string}	string	"missing peer_id"
//	@Failure	409		{string}	string	"not a cluster host"
//	@Router		/api/cluster/worker/resume [post]
func swagClusterWorkerResume() {}

// ── MQ ───────────────────────────────────────────────────────────────────────

// swagMQSend is a documentation stub for POST /api/mq/send.
//
//	@Summary	Send an MQ message to a peer
//	@Description	Delivers the payload to the remote peer over the MQ P2P protocol.\nBlocks until the peer sends a transport ACK (up to 4 s, one retry).\nTopic convention: call:{channelId}, group:{groupId}:{type}, group.invite, chat, chat.broadcast
//	@Tags		mq
//	@Accept		json
//	@Produce	json
//	@Param		body	body		mqSendRequest	true	"Send request"
//	@Success	200		{object}	mqSendResponse
//	@Failure	400		{string}	string	"missing peer_id or topic"
//	@Failure	504		{string}	string	"peer unreachable"
//	@Router		/api/mq/send [post]
func swagMQSend() {}

// swagMQAck is a documentation stub for POST /api/mq/ack.
//
//	@Summary	Acknowledge a received MQ message
//	@Description	Called by the browser after processing an incoming MQ message.\nRelays an application-level ACK to the sender.\nIf from_peer_id is empty (PublishLocal event) the ACK is silently dropped.
//	@Tags		mq
//	@Accept		json
//	@Produce	json
//	@Param		body	body		mqAckRequest	true	"Ack request"
//	@Success	200		{object}	statusOK
//	@Router		/api/mq/ack [post]
func swagMQAck() {}

// swagMQEvents is a documentation stub for GET /api/mq/events.
//
//	@Summary	SSE stream — incoming MQ messages and delivery receipts
//	@Description	THE single browser event stream. Every incoming P2P MQ message and every PublishLocal event arrives here.\nEvent frames are SSE 'message' events with JSON body: {type, msg:{id,seq,topic,payload}, from}.\nKeep-alive: the stream never closes unless the server shuts down or the browser disconnects.
//	@Tags		mq
//	@Produce	text/event-stream
//	@Success	200	{string}	string	"SSE stream"
//	@Router		/api/mq/events [get]
func swagMQEvents() {}

// ── Call ─────────────────────────────────────────────────────────────────────

// swagCallMode is a documentation stub for GET /api/call/mode.
//
//	@Summary	Query call stack mode (native vs browser)
//	@Description	Returns native (Go/Pion active) or browser (standard WebRTC).\nSafe to call regardless of whether the call feature is enabled.
//	@Tags		call
//	@Produce	json
//	@Success	200	{object}	callModeResponse
//	@Router		/api/call/mode [get]
func swagCallMode() {}

// swagCallActive is a documentation stub for GET /api/call/active.
//
//	@Summary	List active Pion sessions (native mode)
//	@Description	Returns non-hung sessions so the browser can restore the call overlay after page navigation.
//	@Tags		call
//	@Produce	json
//	@Success	200	{array}		callSessionStatus
//	@Router		/api/call/active [get]
func swagCallActive() {}

// swagCallDebug is a documentation stub for GET /api/call/debug.
//
//	@Summary	Debug dump of all active sessions
//	@Tags		call
//	@Produce	json
//	@Success	200	{object}	callDebugResponse
//	@Router		/api/call/debug [get]
func swagCallDebug() {}

// swagCallStart is a documentation stub for POST /api/call/start.
//
//	@Summary	Register a new outbound Pion session (native mode, origin)
//	@Description	Creates the Go-side Pion PeerConnection.\nThe browser must call this before sending call-request via MQ.\nFlow: POST /api/call/start → _sendMQ(call-request) → wait for call-ack → _connectNative()
//	@Tags		call
//	@Accept		json
//	@Produce	json
//	@Param		body	body		callStartRequest	true	"Start request"
//	@Success	200		{object}	callStartResponse
//	@Router		/api/call/start [post]
func swagCallStart() {}

// swagCallAccept is a documentation stub for POST /api/call/accept.
//
//	@Summary	Accept an incoming Pion session (native mode, target)
//	@Description	Creates the Go-side Pion PeerConnection for an incoming call.\nGo sends call-ack via MQ; origin then connects MSE.\nFlow: receive call-request via MQ → POST /api/call/accept → Go sends call-ack → origin's _handleCallAck → _connectNative()
//	@Tags		call
//	@Accept		json
//	@Produce	json
//	@Param		body	body		callStartRequest	true	"Accept request"
//	@Success	200		{object}	callStartResponse
//	@Router		/api/call/accept [post]
func swagCallAccept() {}

// swagCallHangup is a documentation stub for POST /api/call/hangup.
//
//	@Summary	Hang up a Pion session
//	@Tags		call
//	@Accept		json
//	@Produce	json
//	@Param		body	body		callChannelRequest	true	"Hangup request"
//	@Success	200		{object}	statusOK
//	@Router		/api/call/hangup [post]
func swagCallHangup() {}

// swagCallToggleAudio is a documentation stub for POST /api/call/toggle-audio.
//
//	@Summary	Toggle local audio track mute (native mode)
//	@Tags		call
//	@Accept		json
//	@Produce	json
//	@Param		body	body		callChannelRequest	true	"Channel"
//	@Success	200		{object}	callMuteResponse
//	@Router		/api/call/toggle-audio [post]
func swagCallToggleAudio() {}

// swagCallToggleVideo is a documentation stub for POST /api/call/toggle-video.
//
//	@Summary	Toggle local video track (native mode)
//	@Tags		call
//	@Accept		json
//	@Produce	json
//	@Param		body	body		callChannelRequest	true	"Channel"
//	@Success	200		{object}	callVideoResponse
//	@Router		/api/call/toggle-video [post]
func swagCallToggleVideo() {}

// swagCallMedia is a documentation stub for GET /api/call/media/{channel}.
//
//	@Summary	WebSocket — live WebM stream of remote video/audio (native mode)
//	@Description	Binary WebM frames sent over WebSocket. First message is the init segment; subsequent messages are clusters.\nBrowser feeds these to MSE (MediaSource Extensions) for display.
//	@Tags		call
//	@Param		channel	path	string	true	"Channel ID"
//	@Success	101		{string}	string	"WebSocket upgrade"
//	@Router		/api/call/media/{channel} [get]
func swagCallMedia() {}

// swagCallSelf is a documentation stub for GET /api/call/self/{channel}.
//
//	@Summary	WebSocket — self-view WebM stream (local camera, Linux native only)
//	@Description	Binary WebM frames (VP8 video only, no audio track in init segment).\nBrowser feeds to MSE for the local camera inset (PiP).
//	@Tags		call
//	@Param		channel	path	string	true	"Channel ID"
//	@Success	101		{string}	string	"WebSocket upgrade"
//	@Router		/api/call/self/{channel} [get]
func swagCallSelf() {}

// swagCallLoopbackOffer is a documentation stub for POST /api/call/loopback/{channel}/offer.
//
//	@Summary	Send browser SDP offer to Go LocalPC (Phase 4 loopback)
//	@Tags		call
//	@Accept		json
//	@Produce	json
//	@Param		channel	path		string				true	"Channel ID"
//	@Param		body	body		loopbackOfferRequest	true	"SDP offer"
//	@Success	200		{object}	loopbackOfferResponse
//	@Router		/api/call/loopback/{channel}/offer [post]
func swagCallLoopbackOffer() {}

// swagCallLoopbackICE is a documentation stub for POST /api/call/loopback/{channel}/ice.
//
//	@Summary	Send browser ICE candidates to Go LocalPC (Phase 4 loopback)
//	@Tags		call
//	@Accept		json
//	@Produce	json
//	@Param		channel	path	string				true	"Channel ID"
//	@Param		body	body	loopbackICERequest	true	"ICE candidate"
//	@Success	200		{object}	statusOK
//	@Router		/api/call/loopback/{channel}/ice [post]
func swagCallLoopbackICE() {}

// swagCallVideo is a documentation stub for GET /api/call/video/{channel}.
//
//	@Summary	HTTP chunked WebM stream of remote video/audio (Linux native mode)
//	@Description	Replaces the WebSocket+MSE path on Linux. GStreamer's souphttpsrc handles this natively via <video src="http://...">. Uses SubscribeMediaFresh + RequestPLI for clean keyframe-first delivery.
//	@Tags		call
//	@Param		channel	path	string	true	"Channel ID"
//	@Produce	video/webm
//	@Success	200	{string}	string	"Chunked WebM stream"
//	@Router		/api/call/video/{channel} [get]
func swagCallVideo() {}

// swagCallSelfVideo is a documentation stub for GET /api/call/selfvideo/{channel}.
//
//	@Summary	HTTP chunked WebM stream of local self-view (Linux native mode)
//	@Description	Self-view camera stream for the PiP inset. Replays last keyframe for instant display.
//	@Tags		call
//	@Param		channel	path	string	true	"Channel ID"
//	@Produce	video/webm
//	@Success	200	{string}	string	"Chunked WebM stream"
//	@Router		/api/call/selfvideo/{channel} [get]
func swagCallSelfVideo() {}

// ── Groups ───────────────────────────────────────────────────────────────────

// swagGroupsList is a documentation stub for GET /api/groups.
//
//	@Summary	List hosted groups with live member data
//	@Tags		groups
//	@Produce	json
//	@Success	200	{array}	hostedGroupInfo
//	@Router		/api/groups [get]
func swagGroupsList() {}

// swagGroupsCreate is a documentation stub for POST /api/groups.
//
//	@Summary	Create a new hosted group
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupCreateRequest	true	"Create request"
//	@Success	200		{object}	groupCreateResponse
//	@Router		/api/groups [post]
func swagGroupsCreate() {}

// swagGroupsJoinOwn is a documentation stub for POST /api/groups/join-own.
//
//	@Summary	Host joins own group as a member
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupIDRequest	true	"Group ID"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/join-own [post]
func swagGroupsJoinOwn() {}

// swagGroupsLeaveOwn is a documentation stub for POST /api/groups/leave-own.
//
//	@Summary	Host leaves own group
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupIDRequest	true	"Group ID"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/leave-own [post]
func swagGroupsLeaveOwn() {}

// swagGroupsClose is a documentation stub for POST /api/groups/close.
//
//	@Summary	Close and delete a hosted group (broadcasts group:close via MQ to all members)
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupIDRequest	true	"Group ID"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/close [post]
func swagGroupsClose() {}

// swagGroupsSubscriptions is a documentation stub for GET /api/groups/subscriptions.
//
//	@Summary	List group subscriptions (member side) with host reachability
//	@Tags		groups
//	@Produce	json
//	@Success	200	{object}	subscriptionsResponse
//	@Router		/api/groups/subscriptions [get]
func swagGroupsSubscriptions() {}

// swagGroupsJoin is a documentation stub for POST /api/groups/join.
//
//	@Summary	Join a remote group as a member (sends group:join via MQ)
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupHostJoinRequest	true	"Join request"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/join [post]
func swagGroupsJoin() {}

// swagGroupsInvite is a documentation stub for POST /api/groups/invite.
//
//	@Summary	Invite a peer to a hosted group (sends group.invite via MQ)
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupPeerRequest	true	"Invite request"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/invite [post]
func swagGroupsInvite() {}

// swagGroupsRejoin is a documentation stub for POST /api/groups/rejoin.
//
//	@Summary	Rejoin a previously joined group
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupHostJoinRequest	true	"Rejoin request"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/rejoin [post]
func swagGroupsRejoin() {}

// swagGroupsSubscriptionsRemove is a documentation stub for POST /api/groups/subscriptions/remove.
//
//	@Summary	Remove a stale subscription record
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupHostJoinRequest	true	"Remove request"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/subscriptions/remove [post]
func swagGroupsSubscriptionsRemove() {}

// swagGroupsLeave is a documentation stub for POST /api/groups/leave.
//
//	@Summary	Leave a group as a member (sends group:leave via MQ)
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupIDRequest	true	"Group ID"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/leave [post]
func swagGroupsLeave() {}

// swagGroupsKick is a documentation stub for POST /api/groups/kick.
//
//	@Summary	Kick a member from a hosted group
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupPeerRequest	true	"Kick request"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/kick [post]
func swagGroupsKick() {}

// swagGroupsMaxMembers is a documentation stub for POST /api/groups/max-members.
//
//	@Summary	Update max member limit for a hosted group
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupMaxMembersRequest	true	"Max members request"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/max-members [post]
func swagGroupsMaxMembers() {}

// swagGroupsMeta is a documentation stub for POST /api/groups/meta.
//
//	@Summary	Update group name and/or max_members (broadcasts group:meta via MQ)
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupMetaRequest	true	"Meta request"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/meta [post]
func swagGroupsMeta() {}

// swagGroupsSend is a documentation stub for POST /api/groups/send.
//
//	@Summary	Send a payload to a group (host broadcasts, member sends to host)
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Param		body	body		groupSendRequest	true	"Send request"
//	@Success	200		{object}	statusOK
//	@Router		/api/groups/send [post]
func swagGroupsSend() {}

// ── Listen ───────────────────────────────────────────────────────────────────

// swagListenCreate is a documentation stub for POST /api/listen/create.
//
//	@Summary	Host creates a listen group
//	@Tags		listen
//	@Accept		json
//	@Produce	json
//	@Param		body	body		listenCreateRequest	true	"Create request"
//	@Success	200		{object}	listenGroup
//	@Router		/api/listen/create [post]
func swagListenCreate() {}

// swagListenClose is a documentation stub for POST /api/listen/close.
//
//	@Summary	Host closes the listen group
//	@Tags		listen
//	@Produce	json
//	@Success	200	{object}	statusOK
//	@Router		/api/listen/close [post]
func swagListenClose() {}

// swagListenLoad is a documentation stub for POST /api/listen/load.
//
//	@Summary	Load MP3 file(s) as playlist (local access only)
//	@Tags		listen
//	@Accept		json
//	@Produce	json
//	@Param		body	body		listenLoadRequest	true	"Load request"
//	@Success	200		{object}	listenTrack
//	@Router		/api/listen/load [post]
func swagListenLoad() {}

// swagListenQueueAdd is a documentation stub for POST /api/listen/queue/add.
//
//	@Summary	Append files to the playlist (local access only)
//	@Tags		listen
//	@Accept		json
//	@Produce	json
//	@Param		body	body		listenQueueAddRequest	true	"Queue add request"
//	@Success	200		{object}	statusOK
//	@Router		/api/listen/queue/add [post]
func swagListenQueueAdd() {}

// swagListenControl is a documentation stub for POST /api/listen/control.
//
//	@Summary	Playback control — play, pause, seek, next, prev, skip, remove
//	@Tags		listen
//	@Accept		json
//	@Produce	json
//	@Param		body	body		listenControlRequest	true	"Control request"
//	@Success	200		{object}	statusOK
//	@Router		/api/listen/control [post]
func swagListenControl() {}

// swagListenJoin is a documentation stub for POST /api/listen/join.
//
//	@Summary	Listener joins a group
//	@Tags		listen
//	@Accept		json
//	@Produce	json
//	@Param		body	body		listenJoinRequest	true	"Join request"
//	@Success	200		{object}	statusOK
//	@Router		/api/listen/join [post]
func swagListenJoin() {}

// swagListenLeave is a documentation stub for POST /api/listen/leave.
//
//	@Summary	Listener leaves the current group
//	@Tags		listen
//	@Produce	json
//	@Success	200	{object}	statusOK
//	@Router		/api/listen/leave [post]
func swagListenLeave() {}

// swagListenStream is a documentation stub for GET /api/listen/stream.
//
//	@Summary	Live MP3 audio stream
//	@Description	Chunked streaming response (audio/mpeg). Connect an HTML audio element src directly to this URL.
//	@Tags		listen
//	@Produce	audio/mpeg
//	@Success	200	{string}	string	"Chunked audio stream"
//	@Router		/api/listen/stream [get]
func swagListenStream() {}

// swagListenState is a documentation stub for GET /api/listen/state.
//
//	@Summary	Current listen group state
//	@Tags		listen
//	@Produce	json
//	@Success	200	{object}	listenStateResponse
//	@Router		/api/listen/state [get]
func swagListenState() {}

// ── Peers ────────────────────────────────────────────────────────────────────

// swagPeersList is a documentation stub for GET /api/peers.
//
//	@Summary	List all known peers with metadata
//	@Tags		peers
//	@Produce	json
//	@Success	200	{array}	map[string]any
//	@Router		/api/peers [get]
func swagPeersList() {}

// swagSelf is a documentation stub for GET /api/self.
//
//	@Summary	This peer's own identity and metadata
//	@Tags		peers
//	@Produce	json
//	@Success	200	{object}	map[string]any
//	@Router		/api/self [get]
func swagSelf() {}

// swagPeersProbe is a documentation stub for POST /api/peers/probe.
//
//	@Summary	Probe all known peers for reachability
//	@Tags		peers
//	@Produce	json
//	@Success	200	{object}	statusOK
//	@Router		/api/peers/probe [post]
func swagPeersProbe() {}

// topologyPeer describes a peer in the topology response.
type topologyPeer struct {
	ID         string `json:"id"         example:"12D3KooWXxx..."`
	Label      string `json:"label"      example:"Roadwarrior"`
	Reachable  bool   `json:"reachable"  example:"true"`
	Connection string `json:"connection" example:"direct"`
	Addr       string `json:"addr"       example:"/ip4/192.168.1.42/tcp/4001"`
	Age        string `json:"age"        example:"3m24s"`
	Streams    int    `json:"streams"    example:"5"`
}

// topologyNode describes self or relay in the topology response.
type topologyNode struct {
	ID         string `json:"id"                    example:"12D3KooWXxx..."`
	Label      string `json:"label"                 example:"EggMan"`
	HasCircuit bool   `json:"has_circuit,omitempty"  example:"true"`
	Addr       string `json:"addr,omitempty"         example:"/ip4/1.2.3.4/tcp/4001"`
}

// topologyResponse is the full topology payload.
type topologyResponse struct {
	Self  topologyNode    `json:"self"`
	Relay *topologyNode   `json:"relay,omitempty"`
	Peers []topologyPeer  `json:"peers"`
}

// swagTopology is a documentation stub for GET /api/topology.
//
//	@Summary	Network topology graph data
//	@Description	Returns this peer's view of the network: self node, relay node (if configured), and all known peers with connection type (direct/relay/none).
//	@Tags		peers
//	@Produce	json
//	@Success	200	{object}	topologyResponse
//	@Failure	503	{string}	string	"no p2p node"
//	@Router		/api/topology [get]
func swagTopology() {}

// swagPeersFavorite is a documentation stub for POST /api/peers/favorite.
//
//	@Summary	Toggle favorite flag for a peer
//	@Tags		peers
//	@Accept		json
//	@Produce	json
//	@Param		body	body		peerFavoriteRequest	true	"Favorite request"
//	@Success	200		{object}	statusOK
//	@Router		/api/peers/favorite [post]
func swagPeersFavorite() {}

// swagPeerContent is a documentation stub for GET /api/peer/content.
//
//	@Summary	Fetch a remote peer's site content (HTML string)
//	@Tags		peers
//	@Produce	json
//	@Param		id	query		string	true	"Peer ID"
//	@Success	200	{object}	peerContentResponse
//	@Router		/api/peer/content [get]
func swagPeerContent() {}

// ── Settings ─────────────────────────────────────────────────────────────────

// swagSettingsQuick is a documentation stub for POST /api/settings/quick.
//
//	@Summary	Partial settings update — only provided (non-null) fields are written
//	@Tags		settings
//	@Accept		json
//	@Produce	json
//	@Param		body	body		quickSettingsRequest	true	"Partial settings"
//	@Success	200		{object}	statusOK
//	@Router		/api/settings/quick [post]
func swagSettingsQuick() {}

// swagSettingsQuickGet is a documentation stub for GET /api/settings/quick/get.
//
//	@Summary	Read current quick settings (label, email, theme, device prefs, flags)
//	@Tags		settings
//	@Produce	json
//	@Success	200	{object}	quickSettingsResponse
//	@Router		/api/settings/quick/get [get]
func swagSettingsQuickGet() {}

// swagServicesHealth is a documentation stub for GET /api/services/health.
//
//	@Summary	Ping all configured external services
//	@Tags		settings
//	@Produce	json
//	@Success	200	{object}	servicesHealthResponse
//	@Router		/api/services/health [get]
func swagServicesHealth() {}

// swagServicesCheck is a documentation stub for GET /api/services/check.
//
//	@Summary	Check a single service URL (pre-save validation)
//	@Tags		settings
//	@Produce	json
//	@Param		url		query		string	true	"Service base URL"
//	@Param		type	query		string	false	"Service type: registration, credits, email, templates"
//	@Success	200		{object}	serviceHealthEntry
//	@Router		/api/services/check [get]
func swagServicesCheck() {}

// ── Logs ─────────────────────────────────────────────────────────────────────

// swagOpenAPISpec is a documentation stub for GET /api/openapi.json.
//
//	@Summary	This OpenAPI 3.0 spec (generated by swaggo/swag)
//	@Tags		logs
//	@Produce	application/json
//	@Success	200	{object}	map[string]any
//	@Router		/api/openapi.json [get]
func swagOpenAPISpec() {}

// swagLogsSnapshot is a documentation stub for GET /api/logs.
//
//	@Summary	Snapshot of recent Go process log lines
//	@Tags		logs
//	@Produce	json
//	@Success	200	{array}	map[string]string
//	@Router		/api/logs [get]
func swagLogsSnapshot() {}

// swagLogsStream is a documentation stub for GET /api/logs/stream.
//
//	@Summary	SSE stream of new Go process log lines
//	@Tags		logs
//	@Produce	text/event-stream
//	@Success	200	{string}	string	"SSE stream"
//	@Router		/api/logs/stream [get]
func swagLogsStream() {}

// swagLogsClient is a documentation stub for POST /api/logs/client.
//
//	@Summary	Sink for browser-side log messages
//	@Tags		logs
//	@Accept		json
//	@Param		body	body	clientLogRequest	true	"Log entry"
//	@Success	204		"Accepted"
//	@Router		/api/logs/client [post]
func swagLogsClient() {}

// ── Docs ─────────────────────────────────────────────────────────────────────

// swagDocsMy is a documentation stub for GET /api/docs/my.
//
//	@Summary	List my shared files for a group
//	@Tags		docs
//	@Produce	json
//	@Param		group_id	query		string	true	"Group ID"
//	@Success	200			{array}		docFileInfo
//	@Router		/api/docs/my [get]
func swagDocsMy() {}

// swagDocsUpload is a documentation stub for POST /api/docs/upload.
//
//	@Summary	Upload a file to share with the group (multipart)
//	@Tags		docs
//	@Accept		multipart/form-data
//	@Produce	json
//	@Param		group_id	formData	string	true	"Group ID"
//	@Param		file		formData	file	true	"File to upload (max 50 MB)"
//	@Success	200			{object}	statusOK
//	@Router		/api/docs/upload [post]
func swagDocsUpload() {}

// swagDocsDelete is a documentation stub for POST /api/docs/delete.
//
//	@Summary	Delete a shared file (local access only)
//	@Tags		docs
//	@Accept		json
//	@Produce	json
//	@Param		body	body		docsDeleteRequest	true	"Delete request"
//	@Success	200		{object}	statusOK
//	@Router		/api/docs/delete [post]
func swagDocsDelete() {}

// swagDocsBrowse is a documentation stub for GET /api/docs/browse.
//
//	@Summary	Aggregate file lists from all group members (parallel fetch)
//	@Tags		docs
//	@Produce	json
//	@Param		group_id	query		string	true	"Group ID"
//	@Success	200			{object}	map[string]any
//	@Router		/api/docs/browse [get]
func swagDocsBrowse() {}

// docsUploadLocalRequest is the body for POST /api/docs/upload-local.
type docsUploadLocalRequest struct {
	GroupID string `json:"group_id" example:"e933372f2147..."`
	Path    string `json:"path"     example:"/home/user/photo.png"`
}

// swagDocsUploadLocal is a documentation stub for POST /api/docs/upload-local.
//
//	@Summary	Upload a file from a local filesystem path to share with the group
//	@Tags		docs
//	@Accept		json
//	@Produce	json
//	@Param		body	body		docsUploadLocalRequest	true	"Upload request"
//	@Success	200		{object}	statusOK
//	@Failure	400		{string}	string	"Missing group_id or path / Cannot read file"
//	@Router		/api/docs/upload-local [post]
func swagDocsUploadLocal() {}

// swagDocsDownload is a documentation stub for GET /api/docs/download.
//
//	@Summary	Download a file (local store or proxied from remote peer)
//	@Tags		docs
//	@Param		group_id	query	string	true	"Group ID"
//	@Param		file		query	string	true	"Filename"
//	@Param		peer_id		query	string	false	"Peer ID (empty = self)"
//	@Param		inline		query	string	false	"Pass '1' for Content-Disposition: inline"
//	@Success	200			{string}	string	"File content"
//	@Router		/api/docs/download [get]
func swagDocsDownload() {}

// ── Data ─────────────────────────────────────────────────────────────────────

// swagDataTables is a documentation stub for GET /api/data/tables.
//
//	@Summary	List SQLite tables with schema and policies
//	@Tags		data
//	@Produce	json
//	@Success	200	{array}	map[string]any
//	@Router		/api/data/tables [get]
func swagDataTables() {}

// swagDataTablesCreate is a documentation stub for POST /api/data/tables/create.
//
//	@Summary	Create a new SQLite table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataTableCreateRequest	true	"Create request"
//	@Success	200		{object}	statusOK
//	@Router		/api/data/tables/create [post]
func swagDataTablesCreate() {}

// swagDataInsert is a documentation stub for POST /api/data/insert.
//
//	@Summary	Insert a row into a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataInsertRequest	true	"Insert request"
//	@Success	200		{object}	dataInsertResponse
//	@Router		/api/data/insert [post]
func swagDataInsert() {}

// swagDataQuery is a documentation stub for POST /api/data/query.
//
//	@Summary	Query rows from a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataQueryRequest	true	"Query request"
//	@Success	200		{array}		map[string]any
//	@Router		/api/data/query [post]
func swagDataQuery() {}

// swagDataUpdate is a documentation stub for POST /api/data/update.
//
//	@Summary	Update rows in a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataUpdateRequest	true	"Update request"
//	@Success	200		{object}	statusOK
//	@Router		/api/data/update [post]
func swagDataUpdate() {}

// swagDataDelete is a documentation stub for POST /api/data/delete.
//
//	@Summary	Delete rows from a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataDeleteRequest	true	"Delete request"
//	@Success	200		{object}	statusOK
//	@Router		/api/data/delete [post]
func swagDataDelete() {}

// swagDataTablesDescribe is a documentation stub for POST /api/data/tables/describe.
//
//	@Summary	Describe a table's schema and policy
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataTableRequest	true	"Table name"
//	@Success	200		{object}	map[string]any
//	@Router		/api/data/tables/describe [post]
func swagDataTablesDescribe() {}

// swagDataTablesDelete is a documentation stub for POST /api/data/tables/delete.
//
//	@Summary	Drop a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataTableRequest	true	"Table name"
//	@Success	200		{object}	statusOK
//	@Router		/api/data/tables/delete [post]
func swagDataTablesDelete() {}

// swagDataTablesAddColumn is a documentation stub for POST /api/data/tables/add-column.
//
//	@Summary	Add a column to a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataColumnRequest	true	"Add column request"
//	@Success	200		{object}	statusOK
//	@Router		/api/data/tables/add-column [post]
func swagDataTablesAddColumn() {}

// swagDataTablesDropColumn is a documentation stub for POST /api/data/tables/drop-column.
//
//	@Summary	Drop a column from a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataColumnRequest	true	"Drop column request"
//	@Success	200		{object}	statusOK
//	@Router		/api/data/tables/drop-column [post]
func swagDataTablesDropColumn() {}

// swagDataTablesSetPolicy is a documentation stub for POST /api/data/tables/set-policy.
//
//	@Summary	Set sharing policy for a table (private, group, public)
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataPolicyRequest	true	"Policy request"
//	@Success	200		{object}	statusOK
//	@Router		/api/data/tables/set-policy [post]
func swagDataTablesSetPolicy() {}

// swagDataTablesRename is a documentation stub for POST /api/data/tables/rename.
//
//	@Summary	Rename a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataRenameRequest	true	"Rename request"
//	@Success	200		{object}	statusOK
//	@Router		/api/data/tables/rename [post]
func swagDataTablesRename() {}

// ── Avatar ───────────────────────────────────────────────────────────────────

// swagAvatarGet is a documentation stub for GET /api/avatar.
//
//	@Summary	Get own avatar image
//	@Tags		avatar
//	@Produce	image/jpeg,image/png
//	@Success	200	{string}	string	"Image data"
//	@Router		/api/avatar [get]
func swagAvatarGet() {}

// swagAvatarUpload is a documentation stub for POST /api/avatar/upload.
//
//	@Summary	Upload own avatar (multipart image)
//	@Tags		avatar
//	@Accept		multipart/form-data
//	@Produce	json
//	@Param		avatar	formData	file	true	"Avatar image"
//	@Success	200		{object}	avatarUploadResponse
//	@Router		/api/avatar/upload [post]
func swagAvatarUpload() {}

// swagAvatarDelete is a documentation stub for DELETE /api/avatar/delete.
//
//	@Summary	Delete own avatar
//	@Tags		avatar
//	@Produce	json
//	@Success	200	{object}	statusOK
//	@Router		/api/avatar/delete [delete]
func swagAvatarDelete() {}

// swagAvatarPeer is a documentation stub for GET /api/avatar/peer/{id}.
//
//	@Summary	Get a peer's avatar image
//	@Tags		avatar
//	@Produce	image/jpeg,image/png
//	@Param		id	path		string	true	"Peer ID"
//	@Success	200	{string}	string	"Image data"
//	@Router		/api/avatar/peer/{id} [get]
func swagAvatarPeer() {}

// ── Chat ─────────────────────────────────────────────────────────────────────

// swagChatHistory is a documentation stub for GET /api/chat/history.
//
//	@Summary	Get chat history with a peer
//	@Tags		chat
//	@Produce	json
//	@Param		peer_id	query		string				true	"Peer ID"
//	@Success	200		{array}		storage.ChatMessage
//	@Router		/api/chat/history [get]
func swagChatHistory() {}

// swagChatClear is a documentation stub for DELETE /api/chat/history.
//
//	@Summary	Clear chat history with a peer
//	@Tags		chat
//	@Param		peer_id	query		string				true	"Peer ID"
//	@Success	200		{object}	statusOK
//	@Router		/api/chat/history [delete]
func swagChatClear() {}

// ── Site ─────────────────────────────────────────────────────────────────────

// swagSiteContent is a documentation stub for GET /api/site/content.
//
//	@Summary	Fetch a site file's content and ETag
//	@Tags		site
//	@Produce	json
//	@Param		path	query		string	true	"File path relative to site root"
//	@Success	200		{object}	map[string]string
//	@Router		/api/site/content [get]
func swagSiteContent() {}

// ── Lua ──────────────────────────────────────────────────────────────────────

// swagLuaContent is a documentation stub for GET /api/lua/content.
//
//	@Summary	Fetch a Lua script's content
//	@Tags		lua
//	@Produce	json
//	@Param		name	query		string	true	"Script name (without .lua)"
//	@Param		func	query		string	false	"Set to 1 for data functions"
//	@Success	200		{object}	map[string]interface{}
//	@Router		/api/lua/content [get]
func swagLuaContent() {}

// swagLuaPrefabsApply is a documentation stub for POST /api/lua/prefabs/apply.
//
//	@Summary	Install scripts from a prefab pack
//	@Tags		lua
//	@Accept		json
//	@Produce	json
//	@Param		body	body		luaPrefabApplyRequest	true	"Prefab install request"
//	@Success	200		{object}	luaPrefabApplyResponse
//	@Failure	400		{string}	string	"prefab name required / prefab not found / script not found in prefab"
//	@Failure	403		{string}	string	"bad csrf"
//	@Router		/api/lua/prefabs/apply [post]
func swagLuaPrefabsApply() {}

// ── Docs ─────────────────────────────────────────────────────────────────────

// docGroupItem is an entry in the GET /api/docs/groups response.
type docGroupItem struct {
	GroupID   string `json:"group_id"   example:"g-abc123"`
	GroupName string `json:"group_name" example:"Shared Files"`
	Source    string `json:"source"     example:"hosted"`
	Files     []any  `json:"files"`
}

// docGroupsResponse is the body for GET /api/docs/groups.
type docGroupsResponse struct {
	Groups []docGroupItem `json:"groups"`
}

// swagDocsGroups is a documentation stub for GET /api/docs/groups.
//
//	@Summary	List all file-sharing groups with their local files
//	@Description	Returns hosted, subscribed, and local file groups with per-group file lists.
//	@Tags		docs
//	@Produce	json
//	@Success	200	{object}	docGroupsResponse
//	@Router		/api/docs/groups [get]
func swagDocsGroups() {}

// ── Credits ──────────────────────────────────────────────────────────────────

// myBalanceResponse is the body for GET /api/my-balance.
type myBalanceResponse struct {
	CreditsActive bool `json:"credits_active" example:"true"`
	Balance       int  `json:"balance,omitempty" example:"500"`
}

// swagMyBalance is a documentation stub for GET /api/my-balance.
//
//	@Summary	Get this peer's credit balance
//	@Description	Queries the credits service via the rendezvous server. Returns credits_active=false when no service is configured.
//	@Tags		credits
//	@Produce	json
//	@Success	200	{object}	myBalanceResponse
//	@Router		/api/my-balance [get]
func swagMyBalance() {}

// ── Rendezvous ───────────────────────────────────────────────────────────────

// swagRendezvousCheck is a documentation stub for GET /api/rendezvous/check.
//
//	@Summary	Check a rendezvous server's capabilities
//	@Description	Fetches /api/capabilities from the given rendezvous URL and returns the capabilities map.
//	@Tags		rendezvous
//	@Produce	json
//	@Param		url	query		string					true	"Rendezvous server URL"
//	@Success	200	{object}	map[string]interface{}	"Capabilities map from remote server"
//	@Router		/api/rendezvous/check [get]
func swagRendezvousCheck() {}

// ── Site (additional) ────────────────────────────────────────────────────────

// siteFileItem represents an entry from content.ListTree.
type siteFileItem struct {
	Path  string `json:"path"   example:"index.html"`
	IsDir bool   `json:"is_dir" example:"false"`
	Depth int    `json:"depth"  example:"0"`
}

// swagSiteFiles is a documentation stub for GET /api/site/files.
//
//	@Summary	List site files as a flat tree
//	@Tags		site
//	@Produce	json
//	@Success	200	{array}		siteFileItem
//	@Router		/api/site/files [get]
func swagSiteFiles() {}

// siteDeleteRequest is the body for POST /api/site/delete.
type siteDeleteRequest struct {
	Path string `json:"path" example:"css/style.css"`
}

// swagSiteDelete is a documentation stub for POST /api/site/delete.
//
//	@Summary	Delete a file from the site content store
//	@Tags		site
//	@Accept		json
//	@Produce	json
//	@Param		body	body		siteDeleteRequest	true	"File to delete"
//	@Success	200		{object}	statusOK			"status: deleted"
//	@Failure	400		{string}	string				"path required"
//	@Router		/api/site/delete [post]
func swagSiteDelete() {}

// siteUploadResponse is the body for POST /api/site/upload.
type siteUploadResponse struct {
	Status string `json:"status" example:"uploaded"`
	Path   string `json:"path"   example:"img/logo.png"`
	ETag   string `json:"etag"   example:"abc123"`
}

// swagSiteUpload is a documentation stub for POST /api/site/upload.
//
//	@Summary	Upload a file to the site content store
//	@Tags		site
//	@Accept		multipart/form-data
//	@Produce	json
//	@Param		path	formData	string				true	"Destination path relative to site root"
//	@Param		file	formData	file				true	"File to upload"
//	@Success	200		{object}	siteUploadResponse
//	@Failure	400		{string}	string	"path required / file required"
//	@Router		/api/site/upload [post]
func swagSiteUpload() {}

// siteUploadLocalRequest is the body for POST /api/site/upload-local.
type siteUploadLocalRequest struct {
	SrcPath  string `json:"src_path"  example:"/home/user/logo.png"`
	DestPath string `json:"dest_path" example:"images/logo.png"`
}

// swagSiteUploadLocal is a documentation stub for POST /api/site/upload-local.
//
//	@Summary	Upload a file from a local filesystem path to the site content store
//	@Tags		site
//	@Accept		json
//	@Produce	json
//	@Param		body	body		siteUploadLocalRequest	true	"Upload request"
//	@Success	200		{object}	siteUploadResponse
//	@Failure	400		{string}	string	"dest_path and src_path required / cannot read file"
//	@Router		/api/site/upload-local [post]
func swagSiteUploadLocal() {}

// swagSiteExport is a documentation stub for GET /api/site/export.
//
//	@Summary	Download the entire site as a zip archive
//	@Description	Exports manifest, schema, site files, and Lua scripts into a zip. The response is an attachment download.
//	@Tags		site
//	@Produce	application/zip
//	@Success	200	{file}	binary	"Zip archive"
//	@Router		/api/site/export [get]
func swagSiteExport() {}

// siteImportResponse is the body for POST /api/site/import.
type siteImportResponse struct {
	Status string `json:"status" example:"imported"`
}

// swagSiteImport is a documentation stub for POST /api/site/import.
//
//	@Summary	Import a site from a zip archive
//	@Description	Uploads a zip (manifest + schema + site/ + lua/) and applies it, replacing the current site and database.
//	@Tags		site
//	@Accept		multipart/form-data
//	@Produce	json
//	@Param		csrf	formData	string				true	"CSRF token"
//	@Param		file	formData	file				true	"Zip archive to import"
//	@Success	200		{object}	siteImportResponse
//	@Failure	400		{string}	string	"file required / failed to extract zip"
//	@Failure	403		{string}	string	"bad csrf"
//	@Router		/api/site/import [post]
func swagSiteImport() {}

// ── Filesystem ───────────────────────────────────────────────────────────────

// fsBrowseEntry is a single entry in the /api/fs/browse response.
type fsBrowseEntry struct {
	Name  string `json:"name"            example:"Documents"`
	IsDir bool   `json:"is_dir"          example:"true"`
	Size  int64  `json:"size,omitempty"  example:"4096"`
}

// fsBrowseResponse is the response for GET /api/fs/browse.
type fsBrowseResponse struct {
	Dir     string          `json:"dir"     example:"/home/peter"`
	Parent  string          `json:"parent"  example:"/home"`
	Entries []fsBrowseEntry `json:"entries"`
}

// swagFSBrowse is a documentation stub for GET /api/fs/browse.
//
//	@Summary	Browse the local filesystem (directories and files)
//	@Tags		fs
//	@Produce	json
//	@Param		dir	query		string	false	"Directory to list (defaults to home directory)"
//	@Success	200	{object}	fsBrowseResponse
//	@Failure	400	{string}	string	"cannot read directory"
//	@Router		/api/fs/browse [get]
func swagFSBrowse() {}

// ── Templates ────────────────────────────────────────────────────────────────

// luaPrefabApplyRequest is the body for POST /api/lua/prefabs/apply.
type luaPrefabApplyRequest struct {
	Prefab string `json:"prefab" example:"quiz-tools"`
	Script string `json:"script,omitempty" example:"score-tracker"`
	CSRF   string `json:"csrf"   example:"token123"`
}

// luaPrefabApplyResponse is the body for POST /api/lua/prefabs/apply.
type luaPrefabApplyResponse struct {
	Status string `json:"status" example:"installed"`
	Prefab string `json:"prefab" example:"quiz-tools"`
}

// templateApplyRequest is the body for POST /api/templates/apply.
type templateApplyRequest struct {
	Template string `json:"template" example:"corkboard"`
	CSRF     string `json:"csrf"     example:"token123"`
}

// templateApplyResponse is the body for POST /api/templates/apply.
type templateApplyResponse struct {
	Status   string `json:"status"   example:"applied"`
	Template string `json:"template" example:"corkboard"`
}

// swagTemplatesApply is a documentation stub for POST /api/templates/apply.
//
//	@Summary	Apply a built-in template
//	@Description	Resets the site and database, then applies the named built-in template.
//	@Tags		templates
//	@Accept		json
//	@Produce	json
//	@Param		body	body		templateApplyRequest	true	"Template to apply"
//	@Success	200		{object}	templateApplyResponse
//	@Failure	400		{string}	string	"template name required / template not found"
//	@Failure	403		{string}	string	"bad csrf"
//	@Router		/api/templates/apply [post]
func swagTemplatesApply() {}

// templateValidateLocalRequest is the body for POST /api/templates/validate-local.
type templateValidateLocalRequest struct {
	Path string `json:"path" example:"/home/user/my-template"`
}

// templateValidateLocalResponse is the body for POST /api/templates/validate-local.
type templateValidateLocalResponse struct {
	Name        string `json:"name"        example:"My Template"`
	Description string `json:"description" example:"A custom template"`
	Category    string `json:"category"    example:"blog"`
	Icon        string `json:"icon"        example:"pencil"`
}

// swagTemplatesValidateLocal is a documentation stub for POST /api/templates/validate-local.
//
//	@Summary	Validate a local template folder
//	@Description	Reads manifest.json from the given absolute path and returns its metadata for preview.
//	@Tags		templates
//	@Accept		json
//	@Produce	json
//	@Param		body	body		templateValidateLocalRequest		true	"Folder path"
//	@Success	200		{object}	templateValidateLocalResponse
//	@Failure	400		{string}	string	"path required / not a directory / manifest.json not found"
//	@Router		/api/templates/validate-local [post]
func swagTemplatesValidateLocal() {}

// templateApplyLocalRequest is the body for POST /api/templates/apply-local.
type templateApplyLocalRequest struct {
	Path string `json:"path" example:"/home/user/my-template"`
	CSRF string `json:"csrf" example:"token123"`
}

// swagTemplatesApplyLocal is a documentation stub for POST /api/templates/apply-local.
//
//	@Summary	Apply a template from a local folder
//	@Description	Reads all files from the given directory (must contain manifest.json), resets site and database, then applies the template.
//	@Tags		templates
//	@Accept		json
//	@Produce	json
//	@Param		body	body		templateApplyLocalRequest	true	"Folder path and CSRF"
//	@Success	200		{object}	templateApplyResponse
//	@Failure	400		{string}	string	"path required / invalid path / not a directory / manifest.json not found"
//	@Failure	403		{string}	string	"bad csrf"
//	@Router		/api/templates/apply-local [post]
func swagTemplatesApplyLocal() {}

// templateApplyStoreRequest is the body for POST /api/templates/apply-store.
type templateApplyStoreRequest struct {
	Template string `json:"template" example:"kanban"`
	CSRF     string `json:"csrf"     example:"token123"`
}

// templateApplyStoreResponse is the body for POST /api/templates/apply-store.
type templateApplyStoreResponse struct {
	Status   string `json:"status"   example:"applied"`
	Template string `json:"template" example:"kanban"`
	Balance  int    `json:"balance,omitempty" example:"450"`
}

// swagTemplatesApplyStore is a documentation stub for POST /api/templates/apply-store.
//
//	@Summary	Apply a store template (download, spend credits, apply)
//	@Description	Spends credits if required, downloads the template bundle from the rendezvous server, resets site and database, then applies the template.
//	@Tags		templates
//	@Accept		json
//	@Produce	json
//	@Param		body	body		templateApplyStoreRequest	true	"Store template to apply"
//	@Success	200		{object}	templateApplyStoreResponse
//	@Failure	400		{string}	string	"template name required"
//	@Failure	402		{string}	string	"insufficient credits"
//	@Failure	403		{string}	string	"bad csrf"
//	@Failure	502		{string}	string	"failed to download template"
//	@Router		/api/templates/apply-store [post]
func swagTemplatesApplyStore() {}
