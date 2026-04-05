package state

import "time"

// PeerIdentityPayload is THE single identity struct for a peer.
// Used everywhere: MQ wire format (peer:announce, identity.response),
// resolver return type, and conversion target from SeenPeer/CachedPeer.
// One struct — never duplicate these fields into another type.
type PeerIdentityPayload struct {
	PeerID              string    `json:"peerID"`
	Content             string    `json:"content"`
	Email               string    `json:"email,omitempty"`
	AvatarHash          string    `json:"avatarHash,omitempty"`
	VideoDisabled       bool      `json:"videoDisabled,omitempty"`
	ActiveTemplate      string    `json:"activeTemplate,omitempty"`
	PublicKey           string    `json:"publicKey,omitempty"`
	EncryptionSupported bool      `json:"encryptionSupported,omitempty"`
	Verified            bool      `json:"verified,omitempty"`
	GoopClientVersion   string    `json:"goopClientVersion,omitempty"`
	Reachable           bool      `json:"reachable"`
	Offline             bool      `json:"offline,omitempty"`
	LastSeen            int64     `json:"lastSeen,omitempty"`
	Favorite            bool      `json:"favorite,omitempty"`
	Known               bool      `json:"-"` // resolver flag — not sent over wire
	LastSeenTime        time.Time `json:"-"` // internal only
}

// Name returns the display name (Content field).
func (p PeerIdentityPayload) Name() string {
	return p.Content
}

// FromSeenPeer converts a SeenPeer to a PeerIdentityPayload.
func FromSeenPeer(sp SeenPeer) PeerIdentityPayload {
	return PeerIdentityPayload{
		Content:             sp.Content,
		Email:               sp.Email,
		AvatarHash:          sp.AvatarHash,
		Reachable:           sp.Reachable,
		Verified:            sp.Verified,
		GoopClientVersion:   sp.GoopClientVersion,
		PublicKey:           sp.PublicKey,
		EncryptionSupported: sp.EncryptionSupported,
		ActiveTemplate:      sp.ActiveTemplate,
		VideoDisabled:       sp.VideoDisabled,
		Offline:             !sp.OfflineSince.IsZero(),
		LastSeen:            sp.LastSeen.UnixMilli(),
		Favorite:            sp.Favorite,
		Known:               true,
	}
}
