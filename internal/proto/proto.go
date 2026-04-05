
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

	// libp2p stream protocol ID for fetching peer avatars
	AvatarProtoID = "/goop/avatar/1.0.0"

	// libp2p stream protocol ID for group document sharing
	DocsProtoID = "/goop/docs/1.0.0"

	// libp2p stream protocol ID for listening room audio streaming
	ListenProtoID = "/goop/listen/1.0.0"

	// libp2p stream protocol ID for the message queue transport
	MQProtoID = "/goop/mq/1.0.0"

)

const (
	TypeOnline  = "online"
	TypeUpdate  = "update"
	TypeOffline = "offline"
	TypePunch   = "punch"
)

type PresenceMsg struct {
	Type            string   `json:"type"` // online|update|offline|punch
	PeerID          string   `json:"peerId"`
	Content         string   `json:"content,omitempty"`
	Email           string   `json:"email,omitempty"`
	AvatarHash      string   `json:"avatarHash,omitempty"`
	VideoDisabled   bool     `json:"videoDisabled,omitempty"`   // Peer has video/audio calls disabled
	ActiveTemplate  string   `json:"activeTemplate,omitempty"`  // Currently applied template dir name
	Target            string   `json:"target,omitempty"`            // Punch hint: the peer ID this message is addressed to
	Addrs             []string `json:"addrs,omitempty"`             // Multiaddresses for WAN connectivity
	VerificationToken string   `json:"verificationToken,omitempty"` // Set by client, validated + stripped by server
	PublicKey            string   `json:"publicKey,omitempty"`            // NaCl public key for peer-to-peer encryption
	EncryptionSupported  bool     `json:"encryptionSupported,omitempty"` // Peer supports E2E encrypted protocols
	GoopClientVersion   string   `json:"goopClientVersion,omitempty"`
	TS                   int64    `json:"ts"`
	Verified          bool     `json:"verified,omitempty"` // Set by rendezvous server (email verified)
}

func NowMillis() int64 { return time.Now().UnixMilli() }
