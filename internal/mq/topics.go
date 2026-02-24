package mq

import "time"

// ── Topic constants ───────────────────────────────────────────────────────────
// Single source of truth for all MQ topic strings used across the codebase.
// Mirrored in internal/ui/assets/js/mq/topics.js — keep both in sync.
const (
	// Peer lifecycle — published locally by run.go PeerTable bridge.
	TopicPeerAnnounce = "peer:announce"
	TopicPeerGone     = "peer:gone"

	// Call signaling — P2P between peers; hangup also published locally by routes/call.go.
	TopicCallPrefix = "call:" // + channelID

	// Call loopback ICE — Go → browser only (Phase 4).
	// Published by PublishLoopbackICE when Go's LocalPC generates ICE candidates.
	// Browser subscribes via Goop.mq.onLoopbackICE(channelId, fn).
	// Topic format: "call:loopback:" + channelID
	TopicCallLoopbackPrefix = "call:loopback:" // + channelID

	// Group protocol — P2P; group.invite is not scoped to a group ID.
	TopicGroupPrefix = "group:"  // + groupID + ":" + type
	TopicGroupInvite = "group.invite"

	// Listen state — published locally by listen.Manager.
	TopicListenPrefix = "listen:" // + groupID + ":state"

	// Chat — direct P2P and broadcast to all peers.
	TopicChat          = "chat"
	TopicChatBroadcast = "chat.broadcast"

	// Internal MQ event log — published locally by mq.logMQEvent.
	TopicLogMQ = "log:mq"
)

// ── Call signal type constants ─────────────────────────────────────────────────
// Value of the "type" field inside all call:* message payloads.
// Used in both browser mode (video-call.js) and native mode (call-native.js / Go/Pion).
const (
	CallTypeRequest    = "call-request"  // caller → callee: initiate a call
	CallTypeAck        = "call-ack"      // callee → caller: call accepted, SDP exchange starts
	CallTypeOffer      = "call-offer"    // caller → callee: SDP offer (after ack)
	CallTypeAnswer     = "call-answer"   // callee → caller: SDP answer
	CallTypeICE        = "ice-candidate" // either → other: trickle ICE candidate
	CallTypeHangup     = "call-hangup"   // either side: end the call
	CallTypeLoopbackICE = "loopback-ice" // Go → browser: LocalPC ICE candidate (Phase 4)
)

// ── Payload structs ───────────────────────────────────────────────────────────

// ── Call signal payloads ── topic: "call:{channelID}" ─────────────────────────
//
// All call signals share the topic "call:{channelID}" and are routed by the
// "type" field. Both browser (video-call.js) and native (call-native.js/Go Pion)
// modes use these same payload shapes.
//
// P2P signaling sequence:
//
//   caller                          callee
//   ──────────────────────────────────────────────────────────────
//   sendCallRequest ────────────────► (incoming call modal)
//                   ◄──────────────── sendCallAck  (on accept)
//   sendCallOffer   ────────────────►
//                   ◄──────────────── sendCallAnswer
//   sendCallICE ◄──────────────────► sendCallICE  (trickle, both ways)
//   sendCallHangup  ────────────────► (or either side, any time)
//
// Native-mode note: after the initial call-request/ack, Go/Pion handles the
// SDP offer/answer and peer-to-peer ICE exchange internally. The browser
// receives the media stream via WebSocket (/api/call/media/{channel}) using
// WebM/MSE (Phase 4). Browser ↔ local-Go loopback ICE uses the separate
// "call:loopback:{channelID}" topic (Phase 4).

// CallRequestPayload is sent by the caller to invite the remote peer.
type CallRequestPayload struct {
	Type        string `json:"type"`                  // CallTypeRequest
	Constraints any    `json:"constraints,omitempty"` // browser-mode media constraints
}

// CallAckPayload is sent by the callee after accepting the call.
// Receipt triggers the caller to create and send the SDP offer.
type CallAckPayload struct {
	Type string `json:"type"` // CallTypeAck
}

// CallOfferPayload carries the SDP offer from the caller to the callee.
type CallOfferPayload struct {
	Type string `json:"type"` // CallTypeOffer
	SDP  string `json:"sdp"`
}

// CallAnswerPayload carries the SDP answer from the callee back to the caller.
type CallAnswerPayload struct {
	Type string `json:"type"` // CallTypeAnswer
	SDP  string `json:"sdp"`
}

// CallICECandidateInit is the standard RTCIceCandidateInit shape (W3C WebRTC).
type CallICECandidateInit struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid,omitempty"`
	SDPMLineIndex uint16 `json:"sdpMLineIndex"`
}

// CallICEPayload carries a trickle ICE candidate between peers.
type CallICEPayload struct {
	Type      string               `json:"type"`      // CallTypeICE
	Candidate CallICECandidateInit `json:"candidate"`
}

// CallLoopbackICEPayload is published locally (Go → browser) by PublishLoopbackICE.
// The browser adds this candidate to its loopback RTCPeerConnection (Phase 4).
// Topic: "call:loopback:{channelID}" (TopicCallLoopbackPrefix + channelID)
type CallLoopbackICEPayload struct {
	Type      string               `json:"type"`       // CallTypeLoopbackICE
	ChannelID string               `json:"channel_id"`
	Candidate CallICECandidateInit `json:"candidate"`
}

// ── Peer lifecycle payloads ─────────────────────────────────────────────────────

// PeerAnnouncePayload is the payload for TopicPeerAnnounce.
// Published by run.go whenever the PeerTable emits an "update" event.
type PeerAnnouncePayload struct {
	PeerID         string    `json:"peerID"`
	Content        string    `json:"content"`
	Email          string    `json:"email,omitempty"`
	AvatarHash     string    `json:"avatarHash,omitempty"`
	VideoDisabled  bool      `json:"videoDisabled,omitempty"`
	ActiveTemplate string    `json:"activeTemplate,omitempty"`
	Verified       bool      `json:"verified,omitempty"`
	Reachable      bool      `json:"reachable"`
	Offline        bool      `json:"offline"`
	LastSeen       int64     `json:"lastSeen"` // Unix milliseconds
	Favorite       bool      `json:"favorite,omitempty"`
	// Internal only — not sent to browser, populated from CachedPeer after lookup.
	LastSeenTime time.Time `json:"-"`
}

// PeerGonePayload is the payload for TopicPeerGone.
// Published by run.go when a peer is pruned from the in-memory PeerTable.
type PeerGonePayload struct {
	PeerID string `json:"peerID"`
}

// CallHangupPayload is the payload published locally by routes/call.go
// when a native call session's HangupCh fires.
type CallHangupPayload struct {
	Type      string `json:"type"`        // always "call-hangup"
	ChannelID string `json:"channel_id"`
}

// ── Typed publish helpers ─────────────────────────────────────────────────────

// PublishPeerAnnounce pushes a peer metadata update to the browser via MQ SSE.
func (m *Manager) PublishPeerAnnounce(p PeerAnnouncePayload) {
	m.PublishLocal(TopicPeerAnnounce, "", p)
}

// PublishPeerGone notifies the browser that a peer has been pruned from the table.
func (m *Manager) PublishPeerGone(peerID string) {
	m.PublishLocal(TopicPeerGone, "", PeerGonePayload{PeerID: peerID})
}

// PublishCallHangup notifies the browser that a native call session has ended.
// Called by routes/call.go watchHangup() when sess.HangupCh() fires.
func (m *Manager) PublishCallHangup(channelID string) {
	m.PublishLocal(TopicCallPrefix+channelID, "", CallHangupPayload{
		Type:      CallTypeHangup,
		ChannelID: channelID,
	})
}

// PublishLoopbackICE pushes a Go LocalPC ICE candidate to the browser.
// Called by routes/call.go during Phase 4 loopback negotiation when Go's
// local PeerConnection generates an ICE candidate for the browser's loopback PC.
// Topic: "call:loopback:{channelID}" — browser subscribes via Goop.mq.onLoopbackICE.
func (m *Manager) PublishLoopbackICE(channelID string, candidate CallICECandidateInit) {
	m.PublishLocal(TopicCallLoopbackPrefix+channelID, "", CallLoopbackICEPayload{
		Type:      CallTypeLoopbackICE,
		ChannelID: channelID,
		Candidate: candidate,
	})
}
