
package proto

import "time"

const (
	PresenceTopic = "goop.presence.v1"
	MdnsTag       = "goop-mdns"

	// libp2p stream protocol ID used to fetch a peer's current content (single line)
	ContentProtoID = "/goop/content/1.0.0"

	// libp2p stream protocol ID used to fetch files from a peer's site folder
	SiteProtoID = "/goop/site/1.0.0"

	// libp2p stream protocol ID for remote data operations
	DataProtoID = "/goop/data/1.0.0"

	// libp2p stream protocol ID for host-relayed groups
	GroupProtoID = "/goop/group/1.0.0"

	// libp2p stream protocol ID for group invitations
	GroupInviteProtoID = "/goop/group-invite/1.0.0"

	// libp2p stream protocol ID for fetching peer avatars
	AvatarProtoID = "/goop/avatar/1.0.0"

	// libp2p stream protocol ID for group document sharing
	DocsProtoID = "/goop/docs/1.0.0"
)

const (
	TypeOnline  = "online"
	TypeUpdate  = "update"
	TypeOffline = "offline"
)

type PresenceMsg struct {
	Type            string   `json:"type"` // online|update|offline
	PeerID          string   `json:"peerId"`
	Content         string   `json:"content,omitempty"`
	Email           string   `json:"email,omitempty"`
	AvatarHash      string   `json:"avatarHash,omitempty"`
	VideoDisabled   bool     `json:"videoDisabled,omitempty"`   // Peer has video/audio calls disabled
	ActiveTemplate  string   `json:"activeTemplate,omitempty"`  // Currently applied template dir name
	Addrs           []string `json:"addrs,omitempty"`           // Multiaddresses for WAN connectivity
	TS              int64    `json:"ts"`
	Verified        bool     `json:"verified,omitempty"` // Set by rendezvous server (email verified)
}

func NowMillis() int64 { return time.Now().UnixMilli() }
