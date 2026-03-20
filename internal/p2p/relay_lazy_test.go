package p2p

import (
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// newTestNode creates a minimal Node with a real libp2p host for testing.
// The relay peer is set up with a fake address so relay injection logic can run.
func newTestNode(t *testing.T) (*Node, peer.ID) {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })

	// Create a fake relay peer (just need an ID and address for injection logic).
	relayH, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relayH.Close() })

	relayAddr := relayH.Addrs()[0]
	relayInfo := &peer.AddrInfo{
		ID:    relayH.ID(),
		Addrs: []ma.Multiaddr{relayAddr},
	}

	// Create a target peer (not running, just for address testing).
	targetH, err := libp2p.New(libp2p.NoListenAddrs)
	if err != nil {
		t.Fatal(err)
	}
	targetID := targetH.ID()
	_ = targetH.Close()

	n := &Node{
		Host:        h,
		relayPeer:   relayInfo,
		presenceTTL: 20 * time.Second,
	}

	return n, targetID
}

// circuitAddrsInPeerstore returns all circuit relay addresses for pid in the peerstore.
func circuitAddrsInPeerstore(n *Node, pid peer.ID) []ma.Multiaddr {
	var result []ma.Multiaddr
	for _, a := range n.Host.Peerstore().Addrs(pid) {
		if isCircuitAddr(a) {
			result = append(result, a)
		}
	}
	return result
}

// directAddrsInPeerstore returns all non-circuit addresses for pid in the peerstore.
func directAddrsInPeerstore(n *Node, pid peer.ID) []ma.Multiaddr {
	var result []ma.Multiaddr
	for _, a := range n.Host.Peerstore().Addrs(pid) {
		if !isCircuitAddr(a) {
			result = append(result, a)
		}
	}
	return result
}

func TestAddPeerAddrs_DirectOnly_NoCircuitInjected(t *testing.T) {
	n, targetID := newTestNode(t)

	directAddr := "/ip4/192.168.1.100/tcp/4001"
	n.AddPeerAddrs(targetID.String(), []string{directAddr})

	// Direct address should be in peerstore.
	if len(directAddrsInPeerstore(n, targetID)) == 0 {
		t.Fatal("expected direct addresses in peerstore")
	}

	// No circuit addresses should be injected — lazy relay means we don't
	// fabricate relay addresses on heartbeat.
	time.Sleep(50 * time.Millisecond) // let any goroutines settle
	if got := circuitAddrsInPeerstore(n, targetID); len(got) > 0 {
		t.Fatalf("expected no circuit addresses injected, got %d: %v", len(got), got)
	}
}

func TestAddPeerAddrs_AlreadyConnected_NoDial(t *testing.T) {
	n, _ := newTestNode(t)

	// Create a peer we're actually connected to.
	peerH, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { peerH.Close() })

	// Connect directly.
	if err := n.Host.Connect(t.Context(), peer.AddrInfo{
		ID:    peerH.ID(),
		Addrs: peerH.Addrs(),
	}); err != nil {
		t.Fatal(err)
	}

	// Clear peerstore circuit addresses (if any from connection setup).
	for _, a := range circuitAddrsInPeerstore(n, peerH.ID()) {
		n.Host.Peerstore().ClearAddrs(peerH.ID())
		_ = a // just clearing
	}

	// AddPeerAddrs for a connected peer should NOT inject circuit addresses.
	n.AddPeerAddrs(peerH.ID().String(), []string{peerH.Addrs()[0].String()})

	time.Sleep(50 * time.Millisecond)
	if got := circuitAddrsInPeerstore(n, peerH.ID()); len(got) > 0 {
		t.Fatalf("expected no circuit addresses for connected peer, got %d", len(got))
	}
}

func TestAddPeerAddrs_WithPublishedCircuit_NotStoredOnHeartbeat(t *testing.T) {
	n, targetID := newTestNode(t)

	directAddr := "/ip4/192.168.1.100/tcp/4001"
	circuitAddr := "/ip4/10.0.0.1/tcp/4001/p2p/" + n.relayPeer.ID.String() + "/p2p-circuit"

	n.AddPeerAddrs(targetID.String(), []string{directAddr, circuitAddr})

	// Direct address should be in peerstore.
	if len(directAddrsInPeerstore(n, targetID)) == 0 {
		t.Fatal("expected direct addresses in peerstore")
	}

	// Circuit addresses must NOT be in peerstore after heartbeat.
	// Host.Connect dials ALL peerstore addresses — storing circuit here
	// would cause every direct dial to also attempt a relay circuit.
	time.Sleep(50 * time.Millisecond)
	if got := circuitAddrsInPeerstore(n, targetID); len(got) > 0 {
		t.Fatalf("circuit addresses should not be stored on heartbeat (defeats direct-first), got %d", len(got))
	}
}

func TestAddPeerAddrs_CircuitOnly_UsesRelayFallback(t *testing.T) {
	n, targetID := newTestNode(t)

	circuitAddr := "/ip4/10.0.0.1/tcp/4001/p2p/" + n.relayPeer.ID.String() + "/p2p-circuit"

	// Peer published only circuit addresses (no direct) — relay-only peer.
	n.AddPeerAddrs(targetID.String(), []string{circuitAddr})

	// No direct addresses — should trigger connectViaRelay which stores them.
	time.Sleep(100 * time.Millisecond)
	if got := circuitAddrsInPeerstore(n, targetID); len(got) == 0 {
		t.Fatal("relay-only peer should get circuit addresses stored via connectViaRelay")
	}
}

func TestConnectViaRelay_WithCircuit_NoInjection(t *testing.T) {
	n, targetID := newTestNode(t)

	// Provide circuit addresses — connectViaRelay should use them without injecting.
	circuitAddr, _ := ma.NewMultiaddr(
		"/ip4/10.0.0.1/tcp/4001/p2p/" + n.relayPeer.ID.String() + "/p2p-circuit",
	)
	n.Host.Peerstore().AddAddrs(targetID, []ma.Multiaddr{circuitAddr}, time.Minute)

	before := len(circuitAddrsInPeerstore(n, targetID))
	n.connectViaRelay(targetID, []ma.Multiaddr{circuitAddr})

	// Should not have injected additional circuit addresses.
	after := len(circuitAddrsInPeerstore(n, targetID))
	if after > before {
		t.Fatalf("expected no new circuit addresses injected, before=%d after=%d", before, after)
	}
}

func TestConnectViaRelay_NoCircuit_InjectsRelay(t *testing.T) {
	n, targetID := newTestNode(t)

	// No circuit addresses — should inject relay addresses.
	n.connectViaRelay(targetID, nil)

	time.Sleep(50 * time.Millisecond)
	if got := circuitAddrsInPeerstore(n, targetID); len(got) == 0 {
		t.Fatal("expected relay addresses to be injected when no circuit addresses available")
	}
}

func TestConnectViaRelay_NoRelayPeer_Noop(t *testing.T) {
	n, targetID := newTestNode(t)
	n.relayPeer = nil // no relay configured

	n.connectViaRelay(targetID, nil)

	time.Sleep(50 * time.Millisecond)
	if got := circuitAddrsInPeerstore(n, targetID); len(got) > 0 {
		t.Fatalf("expected no circuit addresses without relay peer, got %d", len(got))
	}
}

// --- Bridge topology tests ---
//
// Simulate two LANs connected by a relay (the bridge).
// Peers on the same LAN can connect directly. Peers across LANs
// need the relay only when direct connection fails.

type bridgeTopology struct {
	relay *peer.AddrInfo
	lanA  []*Node // LAN A peers
	lanB  []*Node // LAN B peers
}

func newBridgeTopology(t *testing.T) *bridgeTopology {
	t.Helper()

	relayH, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relayH.Close() })

	relayInfo := &peer.AddrInfo{
		ID:    relayH.ID(),
		Addrs: relayH.Addrs(),
	}

	makeNode := func() *Node {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { h.Close() })
		return &Node{
			Host:        h,
			relayPeer:   relayInfo,
			presenceTTL: 20 * time.Second,
		}
	}

	return &bridgeTopology{
		relay: relayInfo,
		lanA:  []*Node{makeNode(), makeNode()},
		lanB:  []*Node{makeNode(), makeNode()},
	}
}

func TestBridge_SameLAN_NoRelayUsed(t *testing.T) {
	topo := newBridgeTopology(t)

	eggman := topo.lanA[0]
	roadwarrior := topo.lanA[1]

	// Same-LAN peers connect directly (mDNS in production, Host.Connect here).
	if err := eggman.Host.Connect(t.Context(), peer.AddrInfo{
		ID:    roadwarrior.Host.ID(),
		Addrs: roadwarrior.Host.Addrs(),
	}); err != nil {
		t.Fatal(err)
	}

	// Heartbeat arrives with direct (non-loopback) addresses — no circuit published.
	eggman.AddPeerAddrs(
		roadwarrior.Host.ID().String(),
		[]string{"/ip4/192.168.178.10/tcp/4001"},
	)

	time.Sleep(100 * time.Millisecond)

	// Connected same-LAN peers should never get circuit addresses.
	if got := circuitAddrsInPeerstore(eggman, roadwarrior.Host.ID()); len(got) > 0 {
		t.Fatalf("same-LAN peers should have no circuit addresses, got %d", len(got))
	}
}

func TestBridge_CrossLAN_DirectReachable_NoRelay(t *testing.T) {
	topo := newBridgeTopology(t)

	eggman := topo.lanA[0]
	hellfire := topo.lanB[0]

	// Cross-LAN but directly reachable (e.g. VPN tunnel).
	if err := eggman.Host.Connect(t.Context(), peer.AddrInfo{
		ID:    hellfire.Host.ID(),
		Addrs: hellfire.Host.Addrs(),
	}); err != nil {
		t.Fatal(err)
	}

	// Heartbeat arrives with direct addresses only (Hellfire's relay reservation
	// may have expired — no circuit address published).
	eggman.AddPeerAddrs(
		hellfire.Host.ID().String(),
		[]string{"/ip4/84.86.207.172/tcp/59901"},
	)

	time.Sleep(100 * time.Millisecond)

	// Directly-connected peer should not get circuit addresses injected.
	if got := circuitAddrsInPeerstore(eggman, hellfire.Host.ID()); len(got) > 0 {
		t.Fatalf("directly-reachable cross-LAN peers should have no circuit addresses, got %d", len(got))
	}
}

func TestBridge_CrossLAN_DirectReachable_RepeatedHeartbeats(t *testing.T) {
	topo := newBridgeTopology(t)

	eggman := topo.lanA[0]
	hellfire := topo.lanB[0]

	// Establish direct connection (VPN).
	if err := eggman.Host.Connect(t.Context(), peer.AddrInfo{
		ID:    hellfire.Host.ID(),
		Addrs: hellfire.Host.Addrs(),
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate 10 heartbeats — each WITHOUT circuit addresses
	// (Hellfire's reservation keeps cycling).
	for range 10 {
		eggman.AddPeerAddrs(
			hellfire.Host.ID().String(),
			[]string{"/ip4/84.86.207.172/tcp/59901"},
		)
	}

	time.Sleep(100 * time.Millisecond)

	// After repeated heartbeats, still no circuit addresses injected.
	if got := circuitAddrsInPeerstore(eggman, hellfire.Host.ID()); len(got) > 0 {
		t.Fatalf("repeated heartbeats should not inject circuit addresses, got %d after 10 heartbeats", len(got))
	}
}

func TestBridge_CrossLAN_Unreachable_FallsBackToRelay(t *testing.T) {
	topo := newBridgeTopology(t)

	eggman := topo.lanA[0]

	// Create an unreachable peer (closed immediately — simulates NAT-blocked peer).
	unreachableH, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	unreachableID := unreachableH.ID()
	unreachableH.Close() // not listening anymore — direct dial will fail

	// Eggman discovers unreachable peer with a fake WAN address (will fail to connect).
	eggman.AddPeerAddrs(unreachableID.String(), []string{"/ip4/203.0.113.1/tcp/4001"})

	// Wait for direct dial to fail and relay fallback to kick in.
	time.Sleep(4 * time.Second)

	// Relay addresses should have been injected as fallback.
	if got := circuitAddrsInPeerstore(eggman, unreachableID); len(got) == 0 {
		t.Fatal("expected relay addresses injected after direct dial failure")
	}

	_ = topo // keep topology alive
}

func TestBridge_ConnectedPeer_HeartbeatWithoutCircuit_NoInjection(t *testing.T) {
	topo := newBridgeTopology(t)

	eggman := topo.lanA[0]
	hellfire := topo.lanB[0]

	// Establish direct connection first.
	if err := eggman.Host.Connect(t.Context(), peer.AddrInfo{
		ID:    hellfire.Host.ID(),
		Addrs: hellfire.Host.Addrs(),
	}); err != nil {
		t.Fatal(err)
	}

	// Now a heartbeat arrives from Hellfire WITHOUT circuit addresses.
	// (Hellfire's relay reservation momentarily expired.)
	// Old behavior: would inject fabricated relay addresses.
	// New behavior: peer is connected, no injection needed.
	eggman.AddPeerAddrs(
		hellfire.Host.ID().String(),
		addrStrings(hellfire.Host.Addrs()),
	)

	time.Sleep(100 * time.Millisecond)

	if got := circuitAddrsInPeerstore(eggman, hellfire.Host.ID()); len(got) > 0 {
		t.Fatalf("connected peer should not get circuit addresses injected on heartbeat, got %d", len(got))
	}
}

func addrStrings(addrs []ma.Multiaddr) []string {
	s := make([]string, len(addrs))
	for i, a := range addrs {
		s[i] = a.String()
	}
	return s
}
