package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

func (m *Manager) handleMQMessage(from, groupID, msgType string, payload any) {
	m.mu.RLock()
	hg := m.groups[groupID]
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	m.pendingJoinsMu.Lock()
	pendingCh := m.pendingJoins[groupID]
	m.pendingJoinsMu.Unlock()

	switch {
	case hg != nil:
		m.handleHostMessage(from, hg, groupID, msgType, payload)
	case pendingCh != nil && msgType == TypeWelcome:
		m.handleWelcomeForPendingJoin(groupID, payload, pendingCh)
	case pendingCh != nil && msgType == TypeError:
		m.handleErrorForPendingJoin(payload, pendingCh)
	case cc != nil:
		m.handleMemberMessage(from, cc, groupID, msgType, payload)
	default:
		log.Printf("GROUP: Received %s for unknown/pending group %s (from %s)", msgType, groupID, shortID(from))
	}
}

func (m *Manager) handleHostMessage(from string, hg *hostedGroup, groupID, msgType string, payload any) {
	switch msgType {
	case TypeJoin:
		hg.mu.Lock()
		currentCount := len(hg.members)
		if hg.hostJoined {
			currentCount++
		}
		if hg.info.MaxMembers > 0 && currentCount >= hg.info.MaxMembers {
			hg.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), SendTimeout)
			defer cancel()
			_, _ = m.mq.Send(ctx, from, "group:"+groupID+":"+TypeError,
				Message{Type: TypeError, Group: groupID, Payload: ErrorPayload{Code: "full", Message: "group is full"}})
			return
		}
		hg.members[from] = &memberMeta{peerID: from, joinedAt: nowMillis()}
		memberList := hg.memberList(m.selfID)
		appType := hg.info.AppType
		volatile := hg.info.Volatile
		name := hg.info.Name
		maxMembers := hg.info.MaxMembers
		hg.mu.Unlock()

		log.Printf("GROUP: %s joined group %s", shortID(from), groupID)

		ctx, cancel := context.WithTimeout(context.Background(), BroadcastTimeout)
		_, _ = m.mq.Send(ctx, from, "group:"+groupID+":"+TypeWelcome, WelcomePayload{
			GroupName:  name,
			AppType:    appType,
			MaxMembers: maxMembers,
			Volatile:   volatile,
			Members:    memberList,
		})
		cancel()

		m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: memberList}, from)
		m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, Payload: MembersPayload{Members: memberList}})

		if !volatile && len(memberList) > 0 {
			peerIDs := make([]string, len(memberList))
			for i, mi := range memberList {
				peerIDs[i] = mi.PeerID
			}
			_ = m.db.UpsertGroupMembers(groupID, peerIDs)
		}

		m.notifyListeners(&Event{Type: TypeJoin, Group: groupID, From: from})

		if h := m.handlerForType(appType); h != nil {
			h.OnJoin(groupID, from, false)
		}

	case TypeLeave:
		hg.mu.Lock()
		delete(hg.members, from)
		members := hg.memberList(m.selfID)
		volatile := hg.info.Volatile
		appType := hg.info.AppType
		hg.mu.Unlock()

		log.Printf("GROUP: %s left group %s", shortID(from), groupID)

		m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: members}, "")
		m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, From: from, Payload: MembersPayload{Members: members}})

		if !volatile {
			ids := make([]string, len(members))
			for i, mi := range members {
				ids[i] = mi.PeerID
			}
			_ = m.db.UpsertGroupMembers(groupID, ids)
		}

		m.notifyListeners(&Event{Type: TypeLeave, Group: groupID, From: from})

		if h := m.handlerForType(appType); h != nil {
			h.OnLeave(groupID, from, false)
		}

	case TypePong:
		log.Printf("GROUP: Pong from %s in group %s", shortID(from), groupID)

	case TypeMsg, TypeState:
		m.broadcastToGroup(hg, groupID, msgType, payload, from)
		m.notifyListeners(&Event{Type: msgType, Group: groupID, From: from, Payload: payload})
	}
}

func (m *Manager) handleMemberMessage(from string, cc *clientConn, groupID, msgType string, payload any) {
	switch msgType {
	case TypeMembers:
		if rawPayload, ok := payload.(map[string]any); ok {
			if b, err := json.Marshal(rawPayload); err == nil {
				var mp MembersPayload
				if json.Unmarshal(b, &mp) == nil {
					cc.membersMu.Lock()
					cc.members = mp.Members
					cc.membersMu.Unlock()
					if !cc.volatile {
						peerIDs := make([]string, len(mp.Members))
						for i, mi := range mp.Members {
							peerIDs[i] = mi.PeerID
						}
						_ = m.db.UpsertGroupMembers(groupID, peerIDs)
					}
				}
			}
		}
		m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, From: from, Payload: payload})

	case TypeClose:
		appType := cc.appType
		m.mu.Lock()
		if m.activeConns[groupID] == cc {
			delete(m.activeConns, groupID)
		}
		m.mu.Unlock()
		m.db.RemoveSubscription(cc.hostPeerID, groupID) //nolint:errcheck
		m.notifyListeners(&Event{Type: TypeClose, Group: groupID})
		if h := m.handlerForType(appType); h != nil {
			h.OnClose(groupID)
		}
		log.Printf("GROUP: Group %s closed by host", groupID)

	case TypePing:
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), SendTimeout)
			defer cancel()
			_, _ = m.mq.Send(ctx, from, "group:"+groupID+":"+TypePong, Message{Type: TypePong, Group: groupID})
		}()

	case TypeMeta:
		if rawPayload, ok := payload.(map[string]any); ok {
			if b, err := json.Marshal(rawPayload); err == nil {
				var mp MetaPayload
				if json.Unmarshal(b, &mp) == nil && mp.GroupName != "" {
					_ = m.db.AddSubscription(cc.hostPeerID, groupID, mp.GroupName, mp.AppType, mp.MaxMembers, cc.volatile, "member")
				}
			}
		}
		m.notifyListeners(&Event{Type: TypeMeta, Group: groupID, Payload: payload})

	case TypeMsg, TypeState, TypeError:
		m.notifyListeners(&Event{Type: msgType, Group: groupID, From: from, Payload: payload})
	}
}

func (m *Manager) handleWelcomeForPendingJoin(_ string, payload any, ch chan joinResult) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var wp WelcomePayload
	if err := json.Unmarshal(b, &wp); err != nil {
		return
	}
	select {
	case ch <- joinResult{welcome: wp}:
	default:
	}
}

func (m *Manager) handleErrorForPendingJoin(payload any, ch chan joinResult) {
	b, _ := json.Marshal(payload)
	var ep ErrorPayload
	_ = json.Unmarshal(b, &ep)
	msg := ep.Message
	if msg == "" {
		msg = "join rejected by host"
	}
	select {
	case ch <- joinResult{err: fmt.Errorf("%s", msg)}:
	default:
	}
}
