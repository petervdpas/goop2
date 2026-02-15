
package p2p

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
	"os"
	"path/filepath"
	"strings"

	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/docs"
	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/util"

	logging "github.com/ipfs/go-log/v2"
	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

func init() {
	// Silence noisy libp2p subsystems — dial failures and backoff errors
	// go to stderr by default and pollute terminal output.
	logging.SetLogLevel("swarm2", "error")
	logging.SetLogLevel("relay", "info")
	logging.SetLogLevel("autorelay", "info")
	logging.SetLogLevel("autonat", "warn")
}

type Node struct {
	Host  host.Host
	ps    *pubsub.PubSub
	topic *pubsub.Topic
	sub   *pubsub.Subscription

	selfContent        func() string
	selfEmail          func() string
	selfVideoDisabled  func() bool
	selfActiveTemplate func() string
	peers              *state.PeerTable

	// Presence TTL for direct peer addresses; circuit addresses use 10x this.
	presenceTTL time.Duration

	// Relay peer info for recovery after connection drops.
	relayPeer *peer.AddrInfo

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

func New(ctx context.Context, listenPort int, keyFile string, peers *state.PeerTable, selfContent, selfEmail func() string, selfVideoDisabled func() bool, selfActiveTemplate func() string, relayInfo *rendezvous.RelayInfo, presenceTTL time.Duration) (*Node, error) {
	priv, isNew, err := loadOrCreateKey(keyFile)
	if err != nil {
		return nil, err
	}
	if isNew {
		log.Printf("Generated new identity key: %s", keyFile)
	} else {
		log.Printf("Loaded identity key: %s", keyFile)
	}

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", listenPort)),
	}

	// When a relay is available, enable circuit relay transport, hole-punching,
	// and auto-relay so the peer gets a public relay address.
	if relayInfo != nil {
		ri, err := relayInfoToAddrInfo(relayInfo)
		if err == nil {
			opts = append(opts,
				libp2p.EnableRelay(),
				libp2p.EnableHolePunching(),
				libp2p.EnableAutoRelayWithStaticRelays([]peer.AddrInfo{*ri},
					autorelay.WithBootDelay(0),
					autorelay.WithBackoff(30*time.Second),
				),
				libp2p.ForceReachabilityPrivate(),
			)
			log.Printf("relay: enabled (relay peer %s, %d addrs)", ri.ID, len(ri.Addrs))
		} else {
			log.Printf("relay: invalid relay info, skipping: %v", err)
		}
	}

	h, err := libp2p.New(opts...)
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
		Host:               h,
		ps:                 ps,
		topic:              topic,
		sub:                sub,
		selfContent:        selfContent,
		selfEmail:          selfEmail,
		selfVideoDisabled:  selfVideoDisabled,
		selfActiveTemplate: selfActiveTemplate,
		peers:              peers,
		presenceTTL:        presenceTTL,
	}

	// Store relay peer info for recovery after connection drops.
	if relayInfo != nil {
		if ri, err := relayInfoToAddrInfo(relayInfo); err == nil {
			n.relayPeer = ri
		}
	}

	return n, nil
}

// SetLuaDispatcher sets the Lua engine for lua-call/lua-list data operations.
func (n *Node) SetLuaDispatcher(d LuaDispatcher) {
	n.luaDispatcher = d
}

// RescanLuaFunctions tells the Lua engine to re-read its functions directory.
// This is a no-op if no dispatcher is set.
func (n *Node) RescanLuaFunctions() {
	if n.luaDispatcher != nil {
		n.luaDispatcher.RescanFunctions()
	}
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
		msg.ActiveTemplate = n.selfActiveTemplate()
		msg.Addrs = n.wanAddrs()
	}

	b, _ := json.Marshal(msg)
	_ = n.topic.Publish(ctx, b)
}

// wanAddrs returns the host's multiaddresses filtered to exclude loopback
// and link-local addresses. Circuit relay addresses (p2p-circuit) are always
// included since they represent a public relay path.
func (n *Node) wanAddrs() []string {
	var out []string
	for _, a := range n.Host.Addrs() {
		// Always include circuit relay addresses — they're public relay paths.
		if isCircuitAddr(a) {
			out = append(out, a.String())
			continue
		}
		ip, err := manet.ToIP(a)
		if err != nil {
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		out = append(out, a.String())
	}
	return out
}

// isCircuitAddr returns true if the multiaddr contains a /p2p-circuit component.
func isCircuitAddr(a ma.Multiaddr) bool {
	for _, p := range a.Protocols() {
		if p.Code == ma.P_CIRCUIT {
			return true
		}
	}
	return false
}

// WaitForRelay polls the host's addresses for a /p2p-circuit address.
// Returns true if a circuit address appeared before the timeout, false otherwise.
// This ensures the first publish includes relay addresses.
func (n *Node) WaitForRelay(ctx context.Context, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	log.Printf("relay: waiting for circuit address...")
	for {
		for _, a := range n.Host.Addrs() {
			if isCircuitAddr(a) {
				log.Printf("relay: circuit address obtained: %s", a)
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			log.Printf("relay: timeout waiting for circuit address (%s)", timeout)
			return false
		case <-ticker.C:
		}
	}
}

// hasCircuitAddr returns true if the host currently has any /p2p-circuit address.
func (n *Node) hasCircuitAddr() bool {
	for _, a := range n.Host.Addrs() {
		if isCircuitAddr(a) {
			return true
		}
	}
	return false
}

// SubscribeAddressChanges watches for libp2p address changes and calls onChange
// when circuit relay addresses appear or disappear. This handles late relay
// connections and relay recovery without requiring a restart.
//
// When the circuit address is lost, it actively helps autorelay recover by
// clearing swarm dial backoff, refreshing the relay's peerstore addresses,
// and reconnecting. Without this, autorelay can silently fail to reconnect
// because its reservation-refresh failure path doesn't trigger reconnection.
func (n *Node) SubscribeAddressChanges(ctx context.Context, onChange func()) {
	sub, err := n.Host.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		log.Printf("relay: failed to subscribe to address changes: %v", err)
		return
	}

	hadCircuit := n.hasCircuitAddr()

	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case <-sub.Out():
				hasCircuit := n.hasCircuitAddr()
				if hasCircuit != hadCircuit {
					if hasCircuit {
						log.Printf("relay: circuit address appeared, re-publishing")
					} else {
						log.Printf("relay: circuit address lost, recovering...")
						n.recoverRelay(ctx)
					}
					hadCircuit = hasCircuit
					onChange()
				}
			}
		}
	}()
}

// recoverRelay clears swarm dial backoff for the relay peer, re-adds its
// addresses to the peerstore, and reconnects. This gives autorelay a fresh
// connection so it can re-obtain a reservation.
func (n *Node) recoverRelay(ctx context.Context) {
	if n.relayPeer == nil {
		return
	}

	// Give autorelay a moment to handle it on its own.
	select {
	case <-time.After(5 * time.Second):
	case <-ctx.Done():
		return
	}

	if n.hasCircuitAddr() {
		log.Printf("relay: autorelay recovered on its own")
		return
	}

	// Clear swarm dial backoff so we get a fresh connection attempt.
	if sw, ok := n.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(n.relayPeer.ID)
	}

	// Re-add relay addresses to peerstore (they may have expired).
	n.Host.Peerstore().AddAddrs(n.relayPeer.ID, n.relayPeer.Addrs, 10*time.Minute)

	// Reconnect to relay peer.
	connCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := n.Host.Connect(connCtx, *n.relayPeer); err != nil {
		log.Printf("relay: recovery connect failed: %v", err)
		return
	}

	log.Printf("relay: reconnected to relay peer, waiting for reservation...")
}

// StartRelayRefresh periodically forces a fresh relay connection to prevent
// stale relay state. Without this, the TCP connection to the relay stays alive
// but the reservation/data path silently dies, making the peer unreachable
// through the relay while appearing connected.
func (n *Node) StartRelayRefresh(ctx context.Context, interval time.Duration) {
	if n.relayPeer == nil {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if !n.hasCircuitAddr() {
					// No circuit address — recoverRelay will handle this.
					continue
				}
				// Close existing relay connections to force AutoRelay to refresh.
				conns := n.Host.Network().ConnsToPeer(n.relayPeer.ID)
				if len(conns) == 0 {
					continue
				}
				log.Printf("relay: refreshing relay connection (%d conns)", len(conns))
				for _, c := range conns {
					_ = c.Close()
				}
				// AutoRelay will detect the lost connection and re-reserve.
				// recoverRelay will also kick in via SubscribeAddressChanges if needed.
			}
		}
	}()
}

// relayInfoToAddrInfo converts a RelayInfo (from the rendezvous server) into
// a libp2p peer.AddrInfo suitable for autorelay.
func relayInfoToAddrInfo(ri *rendezvous.RelayInfo) (*peer.AddrInfo, error) {
	pid, err := peer.Decode(ri.PeerID)
	if err != nil {
		return nil, fmt.Errorf("decode relay peer ID: %w", err)
	}
	var addrs []ma.Multiaddr
	for _, s := range ri.Addrs {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		addrs = append(addrs, a)
	}
	return &peer.AddrInfo{ID: pid, Addrs: addrs}, nil
}

// addPeerAddrs parses multiaddr strings and adds them to the peerstore.
// Circuit relay addresses get a longer TTL since they represent a stable
// relay path that outlives individual presence heartbeats.
func (n *Node) addPeerAddrs(peerID string, addrs []string) {
	if len(addrs) == 0 {
		return
	}
	pid, err := peer.Decode(peerID)
	if err != nil {
		return
	}
	var direct, circuit []ma.Multiaddr
	for _, s := range addrs {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		if ip, err := manet.ToIP(a); err == nil {
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
		}
		if isCircuitAddr(a) {
			circuit = append(circuit, a)
		} else {
			direct = append(direct, a)
		}
	}
	ttl := n.presenceTTL
	if ttl <= 0 {
		ttl = 20 * time.Second
	}
	if len(direct) > 0 {
		n.Host.Peerstore().AddAddrs(pid, direct, ttl)
	}
	if len(circuit) > 0 {
		n.Host.Peerstore().AddAddrs(pid, circuit, ttl*10)
	}
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
				n.peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled, pm.ActiveTemplate, true)
				n.addPeerAddrs(pm.PeerID, pm.Addrs)
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
