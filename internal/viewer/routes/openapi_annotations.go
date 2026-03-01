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

// ── MQ ───────────────────────────────────────────────────────────────────────

// swagMQSend is a documentation stub for POST /api/mq/send.
//
//	@Summary	Send an MQ message to a peer
//	@Description	Delivers the payload to the remote peer over the MQ P2P protocol.\nBlocks until the peer sends a transport ACK (up to 30 s).\nTopic convention: call:{channelId}, group:{groupId}:{type}, group.invite, chat, chat.broadcast
//	@Tags		mq
//	@Accept		json
//	@Produce	json
//	@Param		body	body		mqSendRequest	true	"Send request"
//	@Success	200		{object}	mqSendResponse
//	@Failure	400		{string}	string	"missing peer_id or topic"
//	@Failure	504		{string}	string	"peer unreachable within 30 s"
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
//	@Success	200	{object}	map[string]any
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

// ── Groups ───────────────────────────────────────────────────────────────────

// swagGroupsList is a documentation stub for GET /api/groups.
//
//	@Summary	List hosted groups with live member data
//	@Tags		groups
//	@Produce	json
//	@Success	200	{array}	map[string]any
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
//	@Success	200	{object}	map[string]any
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
//	@Success	200		{object}	map[string]any
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
//	@Success	200		{object}	map[string]any
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
//	@Success	200	{object}	map[string]any
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
//	@Param		body	body		map[string]any	true	"{ peer_id, favorite }"
//	@Success	200		{object}	statusOK
//	@Router		/api/peers/favorite [post]
func swagPeersFavorite() {}

// swagPeerContent is a documentation stub for GET /api/peer/content.
//
//	@Summary	Fetch a remote peer's site content (HTML string)
//	@Tags		peers
//	@Produce	json
//	@Param		id	query		string	true	"Peer ID"
//	@Success	200	{object}	map[string]string
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
//	@Success	200	{object}	map[string]any
//	@Router		/api/services/health [get]
func swagServicesHealth() {}

// swagServicesCheck is a documentation stub for GET /api/services/check.
//
//	@Summary	Check a single service URL (pre-save validation)
//	@Tags		settings
//	@Produce	json
//	@Param		url		query		string	true	"Service base URL"
//	@Param		type	query		string	false	"Service type: registration, credits, email, templates"
//	@Success	200		{object}	map[string]any
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
//	@Success	200			{object}	map[string]any
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
//	@Success	200			{object}	map[string]any
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
//	@Success	200		{object}	map[string]any
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
//	@Success	200		{object}	map[string]any
//	@Router		/api/data/update [post]
func swagDataUpdate() {}

// swagDataDelete is a documentation stub for POST /api/data/delete.
//
//	@Summary	Delete rows from a table
//	@Tags		data
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dataDeleteRequest	true	"Delete request"
//	@Success	200		{object}	map[string]any
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
//	@Param		file	formData	file	true	"Avatar image"
//	@Success	200		{object}	map[string]any
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
