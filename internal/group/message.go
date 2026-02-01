package group

import "time"

// Message type constants for the group protocol wire format.
const (
	TypeJoin    = "join"
	TypeWelcome = "welcome"
	TypeMembers = "members"
	TypeMsg     = "msg"
	TypeState   = "state"
	TypeLeave   = "leave"
	TypeClose   = "close"
	TypeError   = "error"
)

// Message is the JSON wire format for group protocol messages.
// Messages are newline-delimited JSON on the stream.
type Message struct {
	Type    string      `json:"type"`
	Group   string      `json:"group"`
	From    string      `json:"from,omitempty"`
	Payload any `json:"payload,omitempty"`
}

// WelcomePayload is sent to a new member after joining.
type WelcomePayload struct {
	GroupName string                 `json:"group_name,omitempty"`
	AppType   string                 `json:"app_type,omitempty"`
	Members   []MemberInfo           `json:"members"`
	State     map[string]any `json:"state,omitempty"`
}

// MembersPayload is broadcast when membership changes.
type MembersPayload struct {
	Members []MemberInfo `json:"members"`
}

// MemberInfo describes a group member.
type MemberInfo struct {
	PeerID   string `json:"peer_id"`
	JoinedAt int64  `json:"joined_at"`
}

// ErrorPayload is sent when an error occurs.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GroupInfo describes a hosted group.
type GroupInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AppType    string `json:"app_type"`
	MaxMembers int    `json:"max_members"`
	CreatedAt  string `json:"created_at"`
}

// Subscription describes a client-side subscription to a remote group.
type Subscription struct {
	HostPeerID   string `json:"host_peer_id"`
	GroupID      string `json:"group_id"`
	GroupName    string `json:"group_name"`
	AppType      string `json:"app_type"`
	Role         string `json:"role"`
	SubscribedAt string `json:"subscribed_at"`
}

func nowMillis() int64 { return time.Now().UnixMilli() }
