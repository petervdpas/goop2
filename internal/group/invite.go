package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

type inviteMsg struct {
	GroupID    string `json:"group_id"`
	GroupName  string `json:"group_name"`
	HostPeerID string `json:"host_peer_id"`
	AppType    string `json:"app_type"`
	Volatile   bool   `json:"volatile"`
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
		GroupID:    groupID,
		GroupName:  hg.info.Name,
		HostPeerID: m.selfID,
		AppType:    hg.info.AppType,
		Volatile:   hg.info.Volatile,
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
				if s.AppType == inv.AppType && s.GroupID != inv.GroupID {
					_ = m.db.RemoveSubscription(s.HostPeerID, s.GroupID)
					_ = m.db.DeleteGroupMembers(s.GroupID)
				}
			}
		}
	}

	_ = m.db.AddSubscription(inv.HostPeerID, inv.GroupID, inv.GroupName, inv.AppType, 0, inv.Volatile, "member", m.db.GetPeerName(inv.HostPeerID))

	evt := &Event{
		Type:  "invite",
		Group: inv.GroupID,
		From:  inv.HostPeerID,
		Payload: map[string]any{
			"group_id":   inv.GroupID,
			"group_name": inv.GroupName,
			"host":       inv.HostPeerID,
			"app_type":   inv.AppType,
		},
	}
	m.mq.PublishLocal("group.invite", "", evt)

	// Auto-join for app types that require it
	if inv.AppType == "realtime" || inv.AppType == "template" || inv.AppType == "files" {
		go func() {
			if err := m.JoinRemoteGroup(context.Background(), inv.HostPeerID, inv.GroupID); err != nil {
				log.Printf("GROUP: Auto-join %s group %s failed: %v", inv.AppType, inv.GroupID, err)
				m.notifyListeners(&Event{Type: TypeError, Group: inv.GroupID, Payload: map[string]any{
					"code":    "join_failed",
					"message": err.Error(),
				}})
			}
		}()
	}
}
