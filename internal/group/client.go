package group

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/storage"

	"github.com/libp2p/go-libp2p/core/peer"
)

// JoinRemoteGroup sends a join request to a remote host and waits for a welcome.
func (m *Manager) JoinRemoteGroup(ctx context.Context, hostPeerID, groupID string) error {
	// Auto-leave any existing connection to this same group (re-join scenario).
	m.mu.Lock()
	old := m.activeConns[groupID]
	if old != nil {
		delete(m.activeConns, groupID)
	}
	m.mu.Unlock()

	if old != nil {
		leaveCtx, leaveCancel := context.WithTimeout(context.Background(), SendTimeout)
		_, _ = m.mq.Send(leaveCtx, old.hostPeerID, "group:"+groupID+":"+TypeLeave, Message{Type: TypeLeave, Group: groupID})
		leaveCancel()
		m.db.RemoveSubscription(old.hostPeerID, old.groupID) //nolint:errcheck
		_ = m.db.DeleteGroupMembers(old.groupID)
	}

	// Best-effort connect to ensure the peer is reachable
	pid, err := peer.Decode(hostPeerID)
	if err != nil {
		return fmt.Errorf("invalid host peer ID: %w", err)
	}
	_ = m.host.Connect(ctx, peer.AddrInfo{ID: pid})

	// Register pending result channel before sending join
	resultCh := make(chan joinResult, 1)
	m.pendingJoinsMu.Lock()
	m.pendingJoins[groupID] = resultCh
	m.pendingJoinsMu.Unlock()

	defer func() {
		m.pendingJoinsMu.Lock()
		delete(m.pendingJoins, groupID)
		m.pendingJoinsMu.Unlock()
	}()

	// Set a timeout for the entire join handshake
	joinCtx, joinCancel := context.WithTimeout(ctx, JoinTimeout)
	defer joinCancel()

	// Send join
	if _, err := m.mq.Send(joinCtx, hostPeerID, "group:"+groupID+":"+TypeJoin, Message{Type: TypeJoin, Group: groupID}); err != nil {
		return fmt.Errorf("join send failed: %w", err)
	}

	// Wait for welcome or error (delivered via MQ subscription)
	var r joinResult
	select {
	case r = <-resultCh:
	case <-joinCtx.Done():
		return fmt.Errorf("timed out waiting for welcome from %s", shortID(hostPeerID))
	}
	if r.err != nil {
		return r.err
	}
	wp := r.welcome

	cc := &clientConn{
		hostPeerID: hostPeerID,
		groupID:    groupID,
		appType:    wp.AppType,
		volatile:   wp.Volatile,
		members:    wp.Members,
	}

	m.mu.Lock()
	m.activeConns[groupID] = cc
	m.mu.Unlock()

	// Persist member list for stable groups
	if !wp.Volatile && len(wp.Members) > 0 {
		peerIDs := make([]string, len(wp.Members))
		for i, mi := range wp.Members {
			peerIDs[i] = mi.PeerID
		}
		_ = m.db.UpsertGroupMembers(groupID, peerIDs)
	}

	// Volatile game groups: wipe stale subscriptions of the same type
	if wp.Volatile {
		if subs, err := m.db.ListSubscriptions(); err == nil {
			for _, s := range subs {
				if s.AppType == wp.AppType && s.GroupID != groupID {
					_ = m.db.RemoveSubscription(s.HostPeerID, s.GroupID)
					_ = m.db.DeleteGroupMembers(s.GroupID)
				}
			}
		}
	}

	// Store subscription with full metadata
	m.db.AddSubscription(hostPeerID, groupID, wp.GroupName, wp.AppType, wp.MaxMembers, wp.Volatile, "member") //nolint:errcheck

	if h := m.handlerForType(wp.AppType); h != nil {
		if err := h.OnJoin(groupID, m.selfID, &wp); err != nil {
			m.mu.Lock()
			delete(m.activeConns, groupID)
			m.mu.Unlock()
			return err
		}
	}

	m.notifyListeners(&Event{Type: TypeWelcome, Group: groupID, From: hostPeerID, Payload: map[string]any{
		"group_name":  wp.GroupName,
		"app_type":    wp.AppType,
		"max_members": wp.MaxMembers,
		"volatile":    wp.Volatile,
		"members":     wp.Members,
	}})

	log.Printf("GROUP: Joined group %s on host %s", groupID, shortID(hostPeerID))
	return nil
}

// SendToGroup sends a message through the client connection for the given group.
func (m *Manager) SendToGroup(groupID string, payload any) error {
	m.mu.RLock()
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	if cc == nil {
		return fmt.Errorf("not connected to group %s", groupID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), BroadcastTimeout)
	defer cancel()
	_, err := m.mq.Send(ctx, cc.hostPeerID, "group:"+groupID+":"+TypeMsg, payload)
	return err
}

// LeaveGroup disconnects from the specified remote group.
func (m *Manager) LeaveGroup(groupID string) error {
	m.mu.Lock()
	cc := m.activeConns[groupID]
	if cc != nil {
		delete(m.activeConns, groupID)
	}
	m.mu.Unlock()

	if cc == nil {
		return fmt.Errorf("not connected to group %s", groupID)
	}

	appType := cc.appType

	ctx, cancel := context.WithTimeout(context.Background(), SendTimeout)
	defer cancel()
	_, _ = m.mq.Send(ctx, cc.hostPeerID, "group:"+groupID+":"+TypeLeave, Message{Type: TypeLeave, Group: groupID})

	m.db.RemoveSubscription(cc.hostPeerID, cc.groupID) //nolint:errcheck
	_ = m.db.DeleteGroupMembers(cc.groupID)
	m.notifyListeners(&Event{Type: TypeLeave, Group: cc.groupID})

	if h := m.handlerForType(appType); h != nil {
		h.OnLeave(groupID, m.selfID)
	}

	log.Printf("GROUP: Left group %s", cc.groupID)
	return nil
}

// ActiveGroup returns the host peer ID for an active client connection to the given group.
func (m *Manager) ActiveGroup(groupID string) (hostPeerID string, connected bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if cc, ok := m.activeConns[groupID]; ok {
		return cc.hostPeerID, true
	}
	return "", false
}

// ActiveGroups returns info about all active client-side group connections.
func (m *Manager) ActiveGroups() []ActiveGroupInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ActiveGroupInfo, 0, len(m.activeConns))
	for _, cc := range m.activeConns {
		result = append(result, ActiveGroupInfo{
			HostPeerID: cc.hostPeerID,
			GroupID:    cc.groupID,
			AppType:    cc.appType,
		})
	}
	return result
}

// IsGroupConnected returns true if we have an active client connection to the given group.
func (m *Manager) IsGroupConnected(groupID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.activeConns[groupID]
	return ok
}

// ClientGroupMembers returns the last known member list for a group we joined as a client.
func (m *Manager) ClientGroupMembers(groupID string) []MemberInfo {
	m.mu.RLock()
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	if cc == nil {
		return nil
	}

	cc.membersMu.RLock()
	defer cc.membersMu.RUnlock()
	return cc.members
}

// ListSubscriptions returns all stored group subscriptions.
func (m *Manager) ListSubscriptions() ([]storage.SubscriptionRow, error) {
	return m.db.ListSubscriptions()
}

// RejoinSubscription attempts to reconnect to a previously subscribed group.
func (m *Manager) RejoinSubscription(ctx context.Context, hostPeerID, groupID string) error {
	pid, err := peer.Decode(hostPeerID)
	if err != nil {
		return fmt.Errorf("invalid host peer ID: %w", err)
	}
	_ = m.host.Connect(ctx, peer.AddrInfo{ID: pid})

	return m.JoinRemoteGroup(ctx, hostPeerID, groupID)
}

// RemoveSubscription removes a stale subscription from the database.
func (m *Manager) RemoveSubscription(hostPeerID, groupID string) error {
	return m.db.RemoveSubscription(hostPeerID, groupID)
}

func (m *Manager) reconnectSubscriptions() {
	time.Sleep(DiscoveryWait)

	subs, err := m.db.ListSubscriptions()
	if err != nil || len(subs) == 0 {
		return
	}

	for _, sub := range subs {
		m.mu.RLock()
		_, alreadyConnected := m.activeConns[sub.GroupID]
		m.mu.RUnlock()
		if alreadyConnected {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), ReconnectTimeout)
		err := m.RejoinSubscription(ctx, sub.HostPeerID, sub.GroupID)
		cancel()

		if err != nil {
			msg := err.Error()
			if i := strings.Index(msg, "\n"); i > 0 {
				msg = msg[:i]
			}
			log.Printf("GROUP: Auto-reconnect to %s failed: %s", sub.GroupID, msg)
		} else {
			log.Printf("GROUP: Auto-reconnected to group %s on host %s", sub.GroupID, shortID(sub.HostPeerID))
		}
	}
}
