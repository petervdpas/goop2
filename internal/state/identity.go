package state

// PeerIdentity is the canonical resolved identity for a peer.
// Built once from PeerTable + DB cache + self info, used by all subsystems.
type PeerIdentity struct {
	Name                string
	Email               string
	AvatarHash          string
	Reachable           bool
	Verified            bool
	GoopClientVersion   string
	PublicKey           string
	EncryptionSupported bool
	ActiveTemplate      string
	VideoDisabled       bool
	Known               bool // true if the peer was found in any source
}

// FromSeenPeer converts a SeenPeer to a PeerIdentity.
func FromSeenPeer(sp SeenPeer) PeerIdentity {
	return PeerIdentity{
		Name:                sp.Content,
		Email:               sp.Email,
		AvatarHash:          sp.AvatarHash,
		Reachable:           sp.Reachable,
		Verified:            sp.Verified,
		GoopClientVersion:   sp.GoopClientVersion,
		PublicKey:           sp.PublicKey,
		EncryptionSupported: sp.EncryptionSupported,
		ActiveTemplate:      sp.ActiveTemplate,
		VideoDisabled:       sp.VideoDisabled,
		Known:               true,
	}
}
