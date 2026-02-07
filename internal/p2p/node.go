// internal/p2p/node.go

package p2p

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"goop/internal/avatar"
	"goop/internal/docs"
	"goop/internal/proto"
	"goop/internal/state"
	"goop/internal/storage"
	"goop/internal/util"

	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

type Node struct {
	Host  host.Host
	ps    *pubsub.PubSub
	topic *pubsub.Topic
	sub   *pubsub.Subscription

	selfContent       func() string
	selfEmail         func() string
	selfVideoDisabled func() bool
	peers             *state.PeerTable

	// Set by EnableSite in site.go
	siteRoot string

	// Set by EnableData in data.go
	db *storage.DB

	// Set by SetLuaDispatcher
	luaDispatcher LuaDispatcher

	// Set by EnableAvatar in avatar.go
	avatarStore *avatar.Store

	// Set by EnableDocs in docs.go
	docsStore    *docs.Store
	groupChecker GroupChecker
}

type mdnsNotifee struct {
	h host.Host
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	ctx, cancel := context.WithTimeout(context.Background(), util.DefaultConnectTimeout)
	defer cancel()
	_ = n.h.Connect(ctx, pi)
}

// loadOrCreateKey loads a persistent identity key from disk,
// or generates a new Ed25519 key and saves it on first run.
func loadOrCreateKey(keyFile string) (crypto.PrivKey, bool, error) {
	data, err := os.ReadFile(keyFile)
	if err == nil {
		priv, err := crypto.UnmarshalPrivateKey(data)
		if err == nil {
			return priv, false, nil
		}
		log.Printf("WARNING: corrupt identity key at %s: %v (generating new key)", keyFile, err)
	}

	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		return nil, false, err
	}

	raw, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, false, fmt.Errorf("marshal identity key: %w", err)
	}

	if dir := filepath.Dir(keyFile); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, false, fmt.Errorf("create key directory: %w", err)
		}
	}

	if err := os.WriteFile(keyFile, raw, 0600); err != nil {
		return nil, false, fmt.Errorf("save identity key: %w", err)
	}

	return priv, true, nil
}

func New(ctx context.Context, listenPort int, keyFile string, peers *state.PeerTable, selfContent, selfEmail func() string, selfVideoDisabled func() bool) (*Node, error) {
	priv, isNew, err := loadOrCreateKey(keyFile)
	if err != nil {
		return nil, err
	}
	if isNew {
		log.Printf("Generated new identity key: %s", keyFile)
	} else {
		log.Printf("Loaded identity key: %s", keyFile)
	}

	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", listenPort)),
	)
	if err != nil {
		return nil, err
	}

	// Every node is a server: serve content over stream protocol
	h.SetStreamHandler(protocol.ID(proto.ContentProtoID), func(s network.Stream) {
		defer s.Close()
		content := selfContent()
		_, _ = s.Write([]byte(content + "\n"))
	})

	// LAN discovery via mDNS (API matches your version)
	md := mdns.NewMdnsService(h, proto.MdnsTag, &mdnsNotifee{h: h})
	if err := md.Start(); err != nil {
		_ = h.Close()
		return nil, err
	}
	_ = md

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		_ = h.Close()
		return nil, err
	}

	topic, err := ps.Join(proto.PresenceTopic)
	if err != nil {
		_ = h.Close()
		return nil, err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		_ = h.Close()
		return nil, err
	}

	n := &Node{
		Host:              h,
		ps:                ps,
		topic:             topic,
		sub:               sub,
		selfContent:       selfContent,
		selfEmail:         selfEmail,
		selfVideoDisabled: selfVideoDisabled,
		peers:             peers,
	}

	return n, nil
}

// SetLuaDispatcher sets the Lua engine for lua-call/lua-list data operations.
func (n *Node) SetLuaDispatcher(d LuaDispatcher) {
	n.luaDispatcher = d
}

func (n *Node) Close() error {
	return n.Host.Close()
}

func (n *Node) ID() string {
	return n.Host.ID().String()
}

func (n *Node) Publish(ctx context.Context, typ string) {
	msg := proto.PresenceMsg{
		Type:    typ,
		PeerID:  n.ID(),
		Content: "",
		TS:      proto.NowMillis(),
	}
	if typ == proto.TypeOnline || typ == proto.TypeUpdate {
		msg.Content = n.selfContent()
		msg.Email = n.selfEmail()
		msg.AvatarHash = n.AvatarHash()
		msg.VideoDisabled = n.selfVideoDisabled()
	}

	b, _ := json.Marshal(msg)
	_ = n.topic.Publish(ctx, b)
}

func (n *Node) RunPresenceLoop(ctx context.Context, onEvent func(msg proto.PresenceMsg)) {
	go func() {
		for {
			m, err := n.sub.Next(ctx)
			if err != nil {
				return
			}

			var pm proto.PresenceMsg
			if err := json.Unmarshal(m.Data, &pm); err != nil {
				continue
			}
			if pm.PeerID == "" || pm.Type == "" {
				continue
			}
			if pm.PeerID == n.ID() {
				continue
			}

			switch pm.Type {
			case proto.TypeOnline, proto.TypeUpdate:
				n.peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled)
			case proto.TypeOffline:
				n.peers.Remove(pm.PeerID)
			}

			if onEvent != nil {
				onEvent(pm)
			}
		}
	}()
}

// FetchContent fetches the peer's content using the libp2p stream protocol.
func (n *Node) FetchContent(ctx context.Context, peerID string) (string, error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return "", err
	}

	// Best effort connect (mDNS usually already connected)
	_ = n.Host.Connect(ctx, peer.AddrInfo{ID: pid})

	s, err := n.Host.NewStream(ctx, pid, protocol.ID(proto.ContentProtoID))
	if err != nil {
		return "", err
	}
	defer s.Close()

	rd := bufio.NewReader(s)
	line, _ := rd.ReadString('\n')
	line = strings.TrimSpace(line)
	return line, nil
}
