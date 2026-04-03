package chat

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/mq"
)

const (
	topicPrefix = mq.TopicChatRoomPrefix // "chat.room:"

	subtopicMsg     = "msg"
	subtopicHistory = "history"
	subtopicMembers = "members"

	sendTimeout = 4 * time.Second
)

type chatMsg struct {
	Action   string    `json:"action"`
	Message  *Message  `json:"message,omitempty"`
	Messages []Message `json:"messages,omitempty"`
	Members  []Member  `json:"members,omitempty"`
}

func topic(groupID, sub string) string {
	return topicPrefix + groupID + ":" + sub
}

func parseTopic(t string) (groupID, sub string, ok bool) {
	rest := strings.TrimPrefix(t, topicPrefix)
	if rest == t {
		return "", "", false
	}
	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

func (m *Manager) sendToPeer(peerID, groupID, sub string, msg chatMsg) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	var payload any
	_ = json.Unmarshal(data, &payload)

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	if _, err := m.mq.Send(ctx, peerID, topic(groupID, sub), payload); err != nil {
		log.Printf("CHAT: send to %s failed: %v", peerID[:8], err)
	}
}

func (m *Manager) broadcastToRoom(groupID, sub string, msg chatMsg, excludePeer string) {
	members := m.grp.HostedGroupMembers(groupID)
	for _, mi := range members {
		if mi.PeerID == m.selfID || mi.PeerID == excludePeer {
			continue
		}
		m.sendToPeer(mi.PeerID, groupID, sub, msg)
	}
	m.publishLocal(groupID, sub, msg)
}

func (m *Manager) publishLocal(groupID, sub string, msg chatMsg) {
	if m.mq == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	var payload any
	_ = json.Unmarshal(data, &payload)
	m.mq.PublishLocal(topic(groupID, sub), "", payload)
}

func (m *Manager) handleIncoming(from, t string, payload any) {
	groupID, sub, ok := parseTopic(t)
	if !ok {
		return
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var msg chatMsg
	if json.Unmarshal(b, &msg) != nil {
		return
	}

	switch sub {
	case subtopicMsg:
		if msg.Message == nil {
			return
		}
		m.mu.RLock()
		rs, exists := m.rooms[groupID]
		m.mu.RUnlock()
		if !exists {
			return
		}

		rs.mu.Lock()
		rs.history.Add(*msg.Message)
		rs.mu.Unlock()

		m.broadcastToRoom(groupID, subtopicMsg, msg, from)
	}
}
