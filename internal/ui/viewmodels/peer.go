// internal/ui/viewmodels/peer.go

package viewmodels

type PeerContentVM struct {
	BaseVM
	PeerID     string
	Content    string
	PeerEmail  string
	AvatarHash string
}
