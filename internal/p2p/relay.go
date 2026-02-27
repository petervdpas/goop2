package p2p

// relay.go — circuit relay lifecycle: detection, recovery, peer address injection.
//
// All relay-related Node methods live here so node.go stays focused on
// host construction, presence, and peer management.

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	relayv2client "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/petervdpas/goop2/internal/rendezvous"
)

// isCircuitAddr returns true if the multiaddr contains a /p2p-circuit component.
func isCircuitAddr(a ma.Multiaddr) bool {
	for _, p := range a.Protocols() {
		if p.Code == ma.P_CIRCUIT {
			return true
		}
	}
	return false
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

// SubscribeAddressChanges watches for libp2p address changes and calls onChange
// when circuit relay addresses appear or disappear. This handles late relay
// connections and relay recovery without requiring a restart.
//
// onCircuit(hasCircuit bool) is called whenever the circuit state flips — useful
// for pushing relay status notifications to the browser. May be nil.
//
// When the circuit address is lost, it actively helps autorelay recover by
// clearing swarm dial backoff, refreshing the relay's peerstore addresses,
// and reconnecting. Without this, autorelay can silently fail to reconnect
// because its reservation-refresh failure path doesn't trigger reconnection.
func (n *Node) SubscribeAddressChanges(ctx context.Context, onChange func(), onCircuit func(bool)) {
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
					if onCircuit != nil {
						onCircuit(hasCircuit)
					}
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
	start := time.Now()
	log.Printf("relay [%s]: starting recovery", label)
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
			log.Printf("relay [%s]: recovery FAILED after %s", label, time.Since(start).Truncate(time.Millisecond))
			return false
		case <-tick.C:
			if n.hasCircuitAddr() {
				n.diag("relay [%s]: reservation confirmed", label)
				log.Printf("relay [%s]: recovered in %s", label, time.Since(start).Truncate(time.Millisecond))
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

	if n.refreshRelay(ctx, "recover") {
		return
	}
	// First attempt failed — relay server may still be recovering.
	// Wait 30s and try once more before giving up.
	n.diag("relay: first recovery failed, retrying in 30s")
	log.Printf("relay: first recovery failed, retrying in 30s")
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
		return
	}
	if !n.hasCircuitAddr() {
		n.refreshRelay(ctx, "recover-retry")
	}
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
func (n *Node) addRelayAddrForPeer(pid peer.ID) {
	n.injectRelayAddrs(pid, true)
}

// injectRelayAddrs adds circuit relay addresses for pid to the peerstore.
// Skips injection if the peer already has a direct (non-relay) connection —
// the direct path is always preferable to routing through the relay.
func (n *Node) injectRelayAddrs(pid peer.ID, logIt bool) {
	if n.relayPeer == nil {
		return
	}
	// Skip relay injection if already directly connected.
	// Direct path (LAN/WAN) is always better than relay; injecting relay
	// addresses causes unnecessary relay usage and disrupts call quality.
	for _, c := range n.Host.Network().ConnsToPeer(pid) {
		if !isCircuitAddr(c.RemoteMultiaddr()) {
			return
		}
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
			base = ma.StringCast(strings.TrimSuffix(raddr.String(), p2pSuffix))
		}
		circuitAddr := base.Encapsulate(circuitSuffix)
		if logIt {
			n.diag("relay: injecting circuit addr for %s: %s", pid.ShortString(), circuitAddr)
		}
		n.Host.Peerstore().AddAddr(pid, circuitAddr, 10*time.Minute)
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
