package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

type inviteMsg struct {
	GroupID     string `json:"group_id"`
	GroupName   string `json:"group_name"`
	HostPeerID  string `json:"host_peer_id"`
	GroupType   string `json:"group_type"`
	GroupContext string `json:"group_context,omitempty"`
	Volatile    bool   `json:"volatile"`
}

// InvitePeer sends a group invitation to a remote peer via MQ.
func (m *Manager) InvitePeer(ctx context.Context, peerID, groupID string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	hg.mu.RLock()
	inv := inviteMsg{
		GroupID:     groupID,
		GroupName:   hg.info.Name,
		HostPeerID:  m.selfID,
		GroupType:   hg.info.GroupType,
		GroupContext: hg.info.GroupContext,
		Volatile:    hg.info.Volatile,
	}
	hg.mu.RUnlock()

	_, err := m.mq.Send(ctx, peerID, "group.invite", inv)
	if err != nil {
		return fmt.Errorf("invite send failed: %w", err)
	}

	log.Printf("GROUP: Sent invite for group %s to peer %s", groupID, shortID(peerID))
	return nil
}

func (m *Manager) handleInvite(from string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("GROUP: Failed to marshal invite payload: %v", err)
		return
	}
	var inv inviteMsg
	if err := json.Unmarshal(b, &inv); err != nil || inv.GroupID == "" {
		log.Printf("GROUP: Failed to decode invite: %v", err)
		return
	}
	if from != "" {
		inv.HostPeerID = from
	}

	log.Printf("GROUP: Received invite for group %s from host %s", inv.GroupID, shortID(inv.HostPeerID))

	// Volatile game groups: wipe stale subscriptions of the same type
	if inv.Volatile {
		if subs, err := m.db.ListSubscriptions(); err == nil {
			for _, s := range subs {
				if s.GroupType == inv.GroupType && s.GroupID != inv.GroupID {
					_ = m.db.RemoveSubscription(s.HostPeerID, s.GroupID)
					_ = m.db.DeleteGroupMembers(s.GroupID)
				}
			}
		}
	}

	_ = m.db.AddSubscription(inv.HostPeerID, inv.GroupID, inv.GroupName, inv.GroupType, 0, inv.Volatile, "member", m.db.GetPeerName(inv.HostPeerID))

	evt := &Event{
		Type:  "invite",
		Group: inv.GroupID,
		From:  inv.HostPeerID,
		Payload: map[string]any{
			"group_id":      inv.GroupID,
			"group_name":    inv.GroupName,
			"host":          inv.HostPeerID,
			"group_type":    inv.GroupType,
			"group_context": inv.GroupContext,
		},
	}
	m.mq.PublishLocal("group.invite", "", evt)

	// Auto-join for app types that require it
	if inv.GroupType == "realtime" || inv.GroupType == "template" || inv.GroupType == "files" || inv.GroupType == "chat" {
		go func() {
			if err := m.JoinRemoteGroup(context.Background(), inv.HostPeerID, inv.GroupID); err != nil {
				log.Printf("GROUP: Auto-join %s group %s failed: %v", inv.GroupType, inv.GroupID, err)
				m.notifyListeners(&Event{Type: TypeError, Group: inv.GroupID, Payload: map[string]any{
					"code":    "join_failed",
					"message": err.Error(),
				}})
			}
		}()
	}
}
