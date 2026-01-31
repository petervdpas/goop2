// internal/p2p/avatar.go
package p2p

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"goop/internal/avatar"
	"goop/internal/proto"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// EnableAvatar registers the avatar stream handler and stores the avatar store reference.
func (n *Node) EnableAvatar(store *avatar.Store) {
	n.avatarStore = store
	n.Host.SetStreamHandler(protocol.ID(proto.AvatarProtoID), n.handleAvatarStream)
}

func (n *Node) handleAvatarStream(s network.Stream) {
	defer s.Close()

	if n.avatarStore == nil {
		_, _ = io.WriteString(s, "NONE\n")
		return
	}

	data, err := n.avatarStore.Read()
	if err != nil || data == nil {
		_, _ = io.WriteString(s, "NONE\n")
		return
	}

	_, _ = fmt.Fprintf(s, "OK %d\n", len(data))
	_, _ = s.Write(data)
}

// FetchAvatar fetches a peer's avatar via the p2p avatar protocol.
// Returns the image bytes, or nil if the peer has no avatar.
func (n *Node) FetchAvatar(ctx context.Context, peerID string) ([]byte, error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return nil, err
	}

	_ = n.Host.Connect(ctx, peer.AddrInfo{ID: pid})

	s, err := n.Host.NewStream(ctx, pid, protocol.ID(proto.AvatarProtoID))
	if err != nil {
		return nil, err
	}
	defer s.Close()

	rd := bufio.NewReader(s)
	header, err := rd.ReadString('\n')
	if err != nil {
		return nil, err
	}
	header = strings.TrimSpace(header)

	if header == "NONE" {
		return nil, nil
	}

	if !strings.HasPrefix(header, "OK ") {
		return nil, fmt.Errorf("unexpected response: %q", header)
	}

	sizeStr := strings.TrimPrefix(header, "OK ")
	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return nil, fmt.Errorf("bad size: %w", err)
	}
	if size < 0 || size > 512*1024 {
		return nil, fmt.Errorf("refusing avatar size %d", size)
	}

	data := make([]byte, size)
	_, err = io.ReadFull(rd, data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// AvatarHash returns the current avatar hash (convenience for Publish).
func (n *Node) AvatarHash() string {
	if n.avatarStore == nil {
		return ""
	}
	return n.avatarStore.Hash()
}
