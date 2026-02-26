
package p2p

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

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
	relayv2client "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

func init() {
	// Silence noisy libp2p subsystems.  These produce high-frequency
	// info/debug output (relay reservations, autorelay ticks, dial retries)
	// that floods stderr — especially visible in Wails/Windows devtools.
	logging.SetLogLevel("swarm2", "error")
	logging.SetLogLevel("relay", "error")
	logging.SetLogLevel("autorelay", "error")
	logging.SetLogLevel("autonat", "error")
	logging.SetLogLevel("net/identify", "error")
	logging.SetLogLevel("basichost", "error")
	logging.SetLogLevel("connmgr", "error")
	logging.SetLogLevel("dht", "error")
	logging.SetLogLevel("pubsub", "error")
	logging.SetLogLevel("mdns", "error")
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

	// Relay timing (from rendezvous server config).
	relayCleanupDelay   time.Duration
	relayPollDeadline   time.Duration
	relayConnectTimeout time.Duration
	relayRecoveryGrace  time.Duration

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

	// Diagnostic ring buffer for relay operations.
	diagMu   sync.Mutex
	diagLogs []string
	diagMax  int

	// Guards relay recovery operations so recoverRelay and
	// forceRelayRecovery don't run concurrently and sabotage each other.
	relayRecoveryMu sync.Mutex

	// Node start time for uptime reporting.
	startTime time.Time

	// pulseFn calls the rendezvous server to pulse a target peer,
	// triggering it to refresh its relay reservation.
	pulseFn func(ctx context.Context, peerID string) error

}

type mdnsNotifee struct {
	h  host.Host
	sw *swarm.Swarm
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	// Clear dial backoff so the fresh LAN address wins immediately over any
	// stale cached state from a previous failed dial.
	if n.sw != nil {
		n.sw.Backoff().Clear(pi.ID)
	}
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
					autorelay.WithBackoff(5*time.Second),
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

	var mdnsSw *swarm.Swarm
	if s, ok := h.Network().(*swarm.Swarm); ok {
		mdnsSw = s
	}
	md := mdns.NewMdnsService(h, proto.MdnsTag, &mdnsNotifee{h: h, sw: mdnsSw})
	if err := md.Start(); err != nil {
		_ = h.Close()
		return nil, err
	}

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
		diagLogs:           make([]string, 0, 200),
		diagMax:            200,
		startTime:          time.Now(),
	}

	// Store relay peer info for recovery after connection drops.
	if relayInfo != nil {
		if ri, err := relayInfoToAddrInfo(relayInfo); err == nil {
			n.relayPeer = ri
		}
		// Extract timing from server-pushed config (0 = use default).
		n.relayCleanupDelay = durOrDefault(relayInfo.CleanupDelaySec, 3*time.Second)
		n.relayPollDeadline = durOrDefault(relayInfo.PollDeadlineSec, 25*time.Second)
		n.relayConnectTimeout = durOrDefault(relayInfo.ConnectTimeoutSec, 15*time.Second)
		n.relayRecoveryGrace = durOrDefault(relayInfo.RecoveryGraceSec, 5*time.Second)
	}

	// Diagnostic protocol — the rendezvous server queries this via
	// the relay host connection to get relay health info from any peer.
	h.SetStreamHandler(protocol.ID("/goop/diag/1.0.0"), func(s network.Stream) {
		defer s.Close()
		snap := n.DiagSnapshot()
		_ = json.NewEncoder(s).Encode(snap)
	})

	// Relay-refresh protocol — the rendezvous server sends this to
	// tell a peer to refresh its relay reservation. Triggered when
	// another peer can't reach this one through the relay.
	h.SetStreamHandler(protocol.ID("/goop/relay-refresh/1.0.0"), func(s network.Stream) {
		defer s.Close()
		n.diag("relay-refresh: received pulse from rendezvous")

		// If we already have a circuit address, just report success.
		if n.hasCircuitAddr() {
			n.diag("relay-refresh: already have circuit address")
			_ = json.NewEncoder(s).Encode(map[string]any{"ok": true, "has_circuit": true})
			return
		}

		// Light-touch nudge: clear backoff + refresh peerstore so AutoRelay
		// can retry without us killing the connection that carries this stream.
		n.nudgeRelay()

		// Respond immediately with current state.
		hasCircuit := n.hasCircuitAddr()
		n.diag("relay-refresh: nudged, has_circuit=%v — scheduling background recovery", hasCircuit)
		_ = json.NewEncoder(s).Encode(map[string]any{
			"ok":          hasCircuit,
			"has_circuit": hasCircuit,
			"recovering":  !hasCircuit,
		})

		// If the nudge wasn't enough, schedule a full recovery in the
		// background AFTER we've responded (so we don't kill this stream).
		if !hasCircuit {
			go func() {
				// Give AutoRelay a moment to react to the nudge.
				time.Sleep(n.relayRecoveryGrace)
				if n.hasCircuitAddr() {
					n.diag("relay-refresh: autorelay recovered after nudge")
					return
				}
				n.diag("relay-refresh: nudge insufficient, running full recovery")
				n.ensureRelayReservation(context.Background())
			}()
		}
	})

	return n, nil
}

// diag stores a relay diagnostic message in the ring buffer.
// The rendezvous server can query this buffer via the /goop/diag/1.0.0 stream.
// These are NOT written to stderr — relay diagnostics are high-frequency
// internal events; use the admin panel or /api/diag to inspect them.
func (n *Node) diag(format string, args ...any) {
	ts := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] "+format, append([]any{ts}, args...)...)

	n.diagMu.Lock()
	n.diagLogs = append(n.diagLogs, entry)
	if len(n.diagLogs) > n.diagMax {
		n.diagLogs = n.diagLogs[len(n.diagLogs)-n.diagMax:]
	}
	n.diagMu.Unlock()
}

// DiagSnapshot returns a diagnostic report for this peer, queried by the
// rendezvous server via the /goop/diag/1.0.0 stream protocol.
func (n *Node) DiagSnapshot() map[string]any {
	now := time.Now()

	// ── Host addresses ──
	var addrs []string
	hasCircuit := false
	for _, a := range n.Host.Addrs() {
		s := a.String()
		addrs = append(addrs, s)
		if isCircuitAddr(a) {
			hasCircuit = true
		}
	}

	// ── Listen addresses (local bind addresses) ──
	var listenAddrs []string
	for _, a := range n.Host.Network().ListenAddresses() {
		listenAddrs = append(listenAddrs, a.String())
	}

	// ── Relay config & connection details ──
	relayConns := 0
	var relayConfig map[string]any
	var relayConnDetails []map[string]any
	var relayPeerstoreAddrs []string
	if n.relayPeer != nil {
		// Configured relay info
		var cfgAddrs []string
		for _, a := range n.relayPeer.Addrs {
			cfgAddrs = append(cfgAddrs, a.String())
		}
		relayConfig = map[string]any{
			"peer_id": n.relayPeer.ID.String(),
			"addrs":   cfgAddrs,
		}

		// Peerstore addresses for relay (these expire — key debugging info)
		for _, a := range n.Host.Peerstore().Addrs(n.relayPeer.ID) {
			relayPeerstoreAddrs = append(relayPeerstoreAddrs, a.String())
		}

		// Actual connections to relay
		conns := n.Host.Network().ConnsToPeer(n.relayPeer.ID)
		relayConns = len(conns)
		for _, c := range conns {
			age := now.Sub(c.Stat().Opened)
			detail := map[string]any{
				"addr":    c.RemoteMultiaddr().String(),
				"dir":     dirString(c.Stat().Direction),
				"age":     age.Truncate(time.Second).String(),
				"streams": len(c.GetStreams()),
			}
			relayConnDetails = append(relayConnDetails, detail)
		}
	}

	// ── All connected peers with connection details ──
	var connectedPeerDetails []map[string]any
	for _, pid := range n.Host.Network().Peers() {
		conns := n.Host.Network().ConnsToPeer(pid)
		for _, c := range conns {
			age := now.Sub(c.Stat().Opened)
			detail := map[string]any{
				"peer_id": pid.String(),
				"addr":    c.RemoteMultiaddr().String(),
				"dir":     dirString(c.Stat().Direction),
				"age":     age.Truncate(time.Second).String(),
				"streams": len(c.GetStreams()),
			}
			// Mark if this is the relay peer
			if n.relayPeer != nil && pid == n.relayPeer.ID {
				detail["is_relay"] = true
			}
			connectedPeerDetails = append(connectedPeerDetails, detail)
		}
	}

	// ── Uptime ──
	uptime := now.Sub(n.startTime)

	// ── Site status ──
	hasSite := n.siteRoot != ""

	// ── Registered protocol handlers ──
	var protocols []string
	for _, p := range n.Host.Mux().Protocols() {
		protocols = append(protocols, string(p))
	}

	// ── Relay timing (what this peer is using) ──
	var relayTiming map[string]any
	if n.relayPeer != nil {
		relayTiming = map[string]any{
			"cleanup_delay":   n.relayCleanupDelay.String(),
			"poll_deadline":   n.relayPollDeadline.String(),
			"connect_timeout": n.relayConnectTimeout.String(),
			"recovery_grace":  n.relayRecoveryGrace.String(),
		}
	}

	// ── Recent diag logs ──
	n.diagMu.Lock()
	logs := make([]string, len(n.diagLogs))
	copy(logs, n.diagLogs)
	n.diagMu.Unlock()

	// ── OS / runtime info ──
	hostname, _ := os.Hostname()

	result := map[string]any{
		"peer_id":         n.Host.ID().String(),
		"addrs":           addrs,
		"listen_addrs":    listenAddrs,
		"has_circuit":     hasCircuit,
		"relay_conns":     relayConns,
		"connected_peers": len(n.Host.Network().Peers()),
		"uptime":          uptime.Truncate(time.Second).String(),
		"started":         n.startTime.Format("2006-01-02 15:04:05"),
		"presence_ttl":    n.presenceTTL.String(),
		"has_site":        hasSite,
		"hostname":        hostname,
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"go_version":      runtime.Version(),
		"num_goroutine":   runtime.NumGoroutine(),
		"logs":            logs,
	}

	if len(protocols) > 0 {
		result["protocols"] = protocols
	}
	if relayTiming != nil {
		result["relay_timing"] = relayTiming
	}
	if relayConfig != nil {
		result["relay_config"] = relayConfig
	}
	if len(relayConnDetails) > 0 {
		result["relay_conn_details"] = relayConnDetails
	}
	if len(relayPeerstoreAddrs) > 0 {
		result["relay_peerstore_addrs"] = relayPeerstoreAddrs
	} else if n.relayPeer != nil {
		// Explicitly mark as empty — means addresses expired!
		result["relay_peerstore_addrs"] = []string{}
	}
	if len(connectedPeerDetails) > 0 {
		result["connected_peer_details"] = connectedPeerDetails
	}

	return result
}

// dirString converts a network.Direction to a human-readable string.
func dirString(d network.Direction) string {
	switch d {
	case network.DirInbound:
		return "inbound"
	case network.DirOutbound:
		return "outbound"
	default:
		return "unknown"
	}
}

// SetPeerProtocols pre-populates the peerstore with a cached protocol list for a peer.
// Called on startup to restore protocol knowledge from the DB across restarts.
// Skips peers with an empty protocol list (nothing to seed).
func (n *Node) SetPeerProtocols(peerID string, protocols []string) {
	if len(protocols) == 0 {
		return
	}
	pid, err := peer.Decode(peerID)
	if err != nil {
		return
	}
	protos := make([]protocol.ID, len(protocols))
	for i, p := range protocols {
		protos[i] = protocol.ID(p)
	}
	_ = n.Host.Peerstore().SetProtocols(pid, protos...)
}

// SubscribeIdentify registers a callback that fires whenever a peer's supported
// protocol list is learned via the libp2p Identify exchange. Used to persist
// protocol lists to the DB so mq.Send() can skip unsupported peers after restart.
func (n *Node) SubscribeIdentify(ctx context.Context, fn func(peerID string, protocols []string)) {
	sub, err := n.Host.EventBus().Subscribe(new(event.EvtPeerIdentificationCompleted))
	if err != nil {
		log.Printf("identify: failed to subscribe: %v", err)
		return
	}
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-sub.Out():
				if !ok {
					return
				}
				e := evt.(event.EvtPeerIdentificationCompleted)
				protos, err := n.Host.Peerstore().GetProtocols(e.Peer)
				if err != nil || len(protos) == 0 {
					continue
				}
				strs := make([]string, len(protos))
				for i, p := range protos {
					strs[i] = string(p)
				}
				fn(e.Peer.String(), strs)
			}
		}
	}()
}

// SetPulseFn sets the callback that FetchSiteFile uses to ask the rendezvous
// server to pulse a target peer (tell it to refresh its relay reservation).
func (n *Node) SetPulseFn(fn func(ctx context.Context, peerID string) error) {
	n.pulseFn = fn
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
		msg.Addrs = n.WanAddrs()
	}

	b, _ := json.Marshal(msg)
	_ = n.topic.Publish(ctx, b)
}

// WanAddrs returns the host's multiaddresses filtered to exclude loopback
// and link-local addresses. Circuit relay addresses (p2p-circuit) are always
// included since they represent a public relay path.
func (n *Node) WanAddrs() []string {
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

// refreshRelay is the single core relay recovery function. It closes existing
// relay connections, clears dial backoff, refreshes peerstore addresses,
// reconnects, and waits for AutoRelay to re-obtain a circuit reservation.
//
// Caller MUST hold relayRecoveryMu.
func (n *Node) refreshRelay(ctx context.Context, label string) bool {
	conns := n.Host.Network().ConnsToPeer(n.relayPeer.ID)
	if len(conns) > 0 {
		n.diag("relay [%s]: closing %d relay connections", label, len(conns))
		for _, c := range conns {
			_ = c.Close()
		}
		// Let the relay server clean up the old reservation before we
		// reconnect. Relay v2 enforces MaxReservationsPerPeer=1; if we
		// reconnect too fast the old slot is still occupied and the new
		// reservation is rejected.
		select {
		case <-time.After(n.relayCleanupDelay):
		case <-ctx.Done():
			return false
		}
	} else {
		n.diag("relay [%s]: no relay connections, reconnecting", label)
	}

	if sw, ok := n.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(n.relayPeer.ID)
	}
	n.Host.Peerstore().AddAddrs(n.relayPeer.ID, n.relayPeer.Addrs, 10*time.Minute)

	connCtx, cancel := context.WithTimeout(ctx, n.relayConnectTimeout)
	defer cancel()
	if err := n.Host.Connect(connCtx, *n.relayPeer); err != nil {
		n.diag("relay [%s]: connect failed: %v", label, err)
		return false
	}

	// Try a direct reservation request — this is what AutoRelay does
	// internally, but we do it explicitly to (a) get the exact error if
	// it fails, and (b) kick-start the reservation without waiting for
	// AutoRelay's backoff timer.
	resCtx, resCancel := context.WithTimeout(ctx, 15*time.Second)
	rsvp, resErr := relayv2client.Reserve(resCtx, n.Host, *n.relayPeer)
	resCancel()
	if resErr != nil {
		n.diag("relay [%s]: direct Reserve failed: %v", label, resErr)
	} else {
		n.diag("relay [%s]: direct Reserve OK, expires %s, %d addrs",
			label, rsvp.Expiration.Format("15:04:05"), len(rsvp.Addrs))
	}

	deadline := time.After(n.relayPollDeadline)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			n.diag("relay [%s]: reservation NOT restored after %s", label, n.relayPollDeadline)
			return false
		case <-tick.C:
			if n.hasCircuitAddr() {
				n.diag("relay [%s]: reservation confirmed", label)
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
}

// recoverRelay is called when the circuit address is lost (from
// SubscribeAddressChanges). Gives AutoRelay a grace period to self-recover first.
func (n *Node) recoverRelay(ctx context.Context) {
	if n.relayPeer == nil {
		return
	}

	select {
	case <-time.After(n.relayRecoveryGrace):
	case <-ctx.Done():
		return
	}

	if n.hasCircuitAddr() {
		n.diag("relay: autorelay recovered on its own")
		return
	}

	if !n.relayRecoveryMu.TryLock() {
		n.diag("relay: recovery already in progress, skipping")
		return
	}
	defer n.relayRecoveryMu.Unlock()
	n.refreshRelay(ctx, "recover")
}

// StartRelayRefresh periodically checks the relay reservation and forces
// a refresh only when the circuit address is missing.
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
				if n.hasCircuitAddr() {
					continue // reservation healthy, nothing to do
				}
				n.diag("relay: periodic check — no circuit address, refreshing")
				n.ensureRelayReservation(ctx)
			}
		}
	}()
}

// nudgeRelay is a non-destructive helper that clears dial backoff and
// refreshes peerstore addresses for the relay peer. This gives AutoRelay
// the best chance to re-obtain a reservation without tearing down the
// existing connection (which would kill any stream running over it).
// If not currently connected to the relay, it also dials.
func (n *Node) nudgeRelay() {
	if n.relayPeer == nil {
		return
	}

	if sw, ok := n.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(n.relayPeer.ID)
	}
	n.Host.Peerstore().AddAddrs(n.relayPeer.ID, n.relayPeer.Addrs, 10*time.Minute)

	conns := n.Host.Network().ConnsToPeer(n.relayPeer.ID)
	if len(conns) == 0 {
		n.diag("relay [nudge]: not connected, dialing relay")
		ctx, cancel := context.WithTimeout(context.Background(), n.relayConnectTimeout)
		defer cancel()
		if err := n.Host.Connect(ctx, *n.relayPeer); err != nil {
			n.diag("relay [nudge]: connect failed: %v", err)
		}
	} else {
		n.diag("relay [nudge]: %d connections exist, cleared backoff + refreshed addrs", len(conns))
	}
}

// ensureRelayReservation tears down the relay connection and verifies that a
// fresh reservation comes back. Called by the periodic timer and by the
// relay-refresh stream handler (pulse from rendezvous).
func (n *Node) ensureRelayReservation(ctx context.Context) {
	if !n.relayRecoveryMu.TryLock() {
		n.diag("relay: refresh skipped — recovery in progress")
		return
	}
	defer n.relayRecoveryMu.Unlock()
	n.refreshRelay(ctx, "refresh")
}

// addRelayAddrForPeer constructs a circuit relay address for a target peer
// and adds it to the peerstore. This allows dialing a peer through the relay
// even if the peer never published a circuit address in its presence.
// Address format: <relay-addr>/p2p/<relay-id>/p2p-circuit
// When logIt is true, entries are written to the diagnostic log.
func (n *Node) addRelayAddrForPeer(pid peer.ID) {
	n.injectRelayAddrs(pid, true)
}

func (n *Node) injectRelayAddrs(pid peer.ID, logIt bool) {
	if n.relayPeer == nil {
		return
	}
	relayIDStr := n.relayPeer.ID.String()
	p2pSuffix := "/p2p/" + relayIDStr
	circuitSuffix := ma.StringCast("/p2p/" + relayIDStr + "/p2p-circuit")

	for _, raddr := range n.relayPeer.Addrs {
		// Strip existing /p2p/<relay-id> suffix to avoid doubling it.
		// Some relay addresses include the peer ID (e.g. from the rendezvous
		// /relay endpoint), others don't.
		base := raddr
		if strings.HasSuffix(raddr.String(), p2pSuffix) {
			// Remove the /p2p/<id> component so we only add it once.
			base = ma.StringCast(strings.TrimSuffix(raddr.String(), p2pSuffix))
		}
		circuitAddr := base.Encapsulate(circuitSuffix)
		if logIt {
			n.diag("relay: injecting circuit addr for %s: %s", pid.ShortString(), circuitAddr)
		}
		n.Host.Peerstore().AddAddr(pid, circuitAddr, 2*time.Minute)
	}
}

// durOrDefault converts seconds to a duration, falling back to def when sec <= 0.
func durOrDefault(sec int, def time.Duration) time.Duration {
	if sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return def
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

// AddPeerAddrs parses multiaddr strings and adds them to the peerstore.
// Circuit relay addresses get a longer TTL since they represent a stable
// relay path that outlives individual presence heartbeats. When no circuit
// address is present and a relay is configured, a circuit address is
// injected so the peer is reachable through the relay.
func (n *Node) AddPeerAddrs(peerID string, addrs []string) {
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
	// Keep addresses in the peerstore long enough to survive a network
	// switch.  Heartbeats refresh them, so a 2-minute floor is safe and
	// means LAN addresses are still available when probing after switching
	// from WAN→LAN (before mDNS rediscovers).
	addrTTL := ttl
	if addrTTL < 2*time.Minute {
		addrTTL = 2 * time.Minute
	}
	if len(direct) > 0 {
		n.Host.Peerstore().AddAddrs(pid, direct, addrTTL)
	}
	if len(circuit) > 0 {
		n.Host.Peerstore().AddAddrs(pid, circuit, addrTTL*5)
	}

	// If the peer published no circuit address but we have a relay configured,
	// proactively inject a constructed circuit relay address. This ensures we
	// can reach peers that failed to obtain their own relay reservation.
	if len(circuit) == 0 && n.relayPeer != nil && pid != n.relayPeer.ID {
		n.injectRelayAddrs(pid, false)
	}

	// Fresh addresses just arrived (presence heartbeat or mDNS).
	// Clear any accumulated dial backoff so the new addresses win immediately,
	// then kick off a background connect if we are not already connected.
	// This handles both LAN (direct addresses) and WAN (circuit relay addresses).
	if sw, ok := n.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(pid)
	}
	if n.Host.Network().Connectedness(pid) != network.Connected {
		allAddrs := make([]ma.Multiaddr, 0, len(direct)+len(circuit))
		allAddrs = append(allAddrs, direct...)
		allAddrs = append(allAddrs, circuit...)
		if len(allAddrs) > 0 {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), util.DefaultConnectTimeout)
				defer cancel()
				_ = n.Host.Connect(ctx, peer.AddrInfo{ID: pid, Addrs: allAddrs})
			}()
		}
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
				// Preserve the Verified flag set by the rendezvous server — P2P gossip
				// is not an authority on email verification.
				existing, _ := n.peers.Get(pm.PeerID)
				n.peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled, pm.ActiveTemplate, existing.Verified)
				n.AddPeerAddrs(pm.PeerID, pm.Addrs)
			case proto.TypeOffline:
				n.peers.MarkOffline(pm.PeerID)
			}

			if onEvent != nil {
				onEvent(pm)
			}
		}
	}()
}

// ProbePeer tests whether we can open a direct/relay stream to the
// peer.  NewStream is the only reliable check — Connectedness can be
// true from GossipSub overlay connections that don't imply direct
// reachability.  We clear the swarm dial backoff first so that a
// network switch doesn't cause stale "don't retry" entries to block
// fresh dial attempts.
func (n *Node) ProbePeer(ctx context.Context, rawID string) {
	pid, err := peer.Decode(rawID)
	if err != nil {
		return
	}
	if sw, ok := n.Host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(pid)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	s, err := n.Host.NewStream(probeCtx, pid, protocol.ID(proto.ContentProtoID))
	if err != nil {
		// Only log when transitioning from reachable → unreachable.
		if sp, ok := n.peers.Get(rawID); ok && sp.Reachable {
			log.Printf("probe %s: UNREACHABLE err=%v", rawID[:16], err)
		}
		n.peers.SetReachable(rawID, false)
		return
	}
	s.Close()
	// Only log when transitioning from unreachable → reachable.
	if sp, ok := n.peers.Get(rawID); ok && !sp.Reachable {
		log.Printf("probe %s: REACHABLE", rawID[:16])
	}
	n.peers.SetReachable(rawID, true)
}

// SubscribeConnectionEvents watches for new peer connections (e.g. mDNS)
// and re-probes any peer that is currently marked unreachable. onConnect, if
// non-nil, is called with the peer ID for every newly connected peer.
func (n *Node) SubscribeConnectionEvents(ctx context.Context, onConnect func(peerID string)) {
	sub, err := n.Host.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		log.Printf("probe: failed to subscribe to connection events: %v", err)
		return
	}
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-sub.Out():
				e, ok := ev.(event.EvtPeerConnectednessChanged)
				if !ok || e.Connectedness != network.Connected {
					continue
				}
				rawID := e.Peer.String()
				sp, known := n.peers.Get(rawID)
				if known && !sp.Reachable {
					go n.ProbePeer(ctx, rawID)
				}
				if onConnect != nil {
					go onConnect(rawID)
				}
			}
		}
	}()
}

// ProbeAllPeers probes every known peer in parallel and blocks until done.
func (n *Node) ProbeAllPeers(ctx context.Context) {
	ids := n.peers.IDs()
	if len(ids) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, rawID := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			n.ProbePeer(ctx, id)
		}(rawID)
	}
	wg.Wait()
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
