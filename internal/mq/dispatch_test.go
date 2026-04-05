package mq

import (
	"strings"
	"testing"
)

// simulateDispatch replicates the dispatch logic from handleIncoming
// without requiring a network.Stream. Returns true if the message
// would be delivered to SSE listeners.
func simulateDispatch(m *Manager, remotePeer string, msg MQMsg) bool {
	// Dispatch to topic subscribers
	m.topicMu.RLock()
	for _, sub := range m.topicSubs {
		if strings.HasPrefix(msg.Topic, sub.prefix) {
			sub.fn(remotePeer, msg.Topic, msg.Payload)
		}
	}
	m.topicMu.RUnlock()

	// Check if topic is suppressed from SSE
	if msg.Topic == TopicGroupInvite || strings.HasPrefix(msg.Topic, TopicGroupPrefix) ||
		strings.HasPrefix(msg.Topic, TopicChatRoomPrefix) ||
		msg.Topic == TopicIdentity || msg.Topic == TopicIdentityResponse {
		return false
	}
	return true
}

func testManager() *Manager {
	return &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
		selfID:    "self",
	}
}

func TestDispatch_GroupInvite_SuppressedFromSSE(t *testing.T) {
	m := testManager()

	subscriberCalled := false
	m.SubscribeTopic("group.invite", func(from, topic string, payload any) {
		subscriberCalled = true
	})

	delivered := simulateDispatch(m, "peer1", MQMsg{
		ID: "msg1", Topic: TopicGroupInvite, Payload: map[string]any{"group_name": "Test"},
	})

	if !subscriberCalled {
		t.Fatal("topic subscriber should have been called")
	}
	if delivered {
		t.Fatal("group.invite should NOT be delivered to SSE (subscriber republishes via PublishLocal)")
	}
}

func TestDispatch_GroupMsg_SuppressedFromSSE(t *testing.T) {
	m := testManager()

	subscriberCalled := false
	m.SubscribeTopic("group:", func(from, topic string, payload any) {
		subscriberCalled = true
	})

	delivered := simulateDispatch(m, "peer1", MQMsg{
		ID: "msg2", Topic: "group:abc123:msg", Payload: map[string]any{"text": "hello"},
	})

	if !subscriberCalled {
		t.Fatal("topic subscriber should have been called")
	}
	if delivered {
		t.Fatal("group:* should NOT be delivered to SSE")
	}
}

func TestDispatch_ChatRoom_SuppressedFromSSE(t *testing.T) {
	m := testManager()

	subscriberCalled := false
	m.SubscribeTopic(TopicChatRoomPrefix, func(from, topic string, payload any) {
		subscriberCalled = true
	})

	delivered := simulateDispatch(m, "peer1", MQMsg{
		ID: "msg3", Topic: "chat.room:abc123:msg", Payload: map[string]any{"text": "hi"},
	})

	if !subscriberCalled {
		t.Fatal("topic subscriber should have been called")
	}
	if delivered {
		t.Fatal("chat.room:* should NOT be delivered to SSE")
	}
}

func TestDispatch_CallSignal_DeliveredToSSE(t *testing.T) {
	m := testManager()

	subscriberCalled := false
	m.SubscribeTopic("call:", func(from, topic string, payload any) {
		subscriberCalled = true
	})

	delivered := simulateDispatch(m, "peer1", MQMsg{
		ID: "msg4", Topic: "call:channel1:offer", Payload: map[string]any{"type": "call-offer"},
	})

	if !subscriberCalled {
		t.Fatal("call subscriber should have been called")
	}
	if !delivered {
		t.Fatal("call:* SHOULD be delivered to SSE (browser needs signaling)")
	}
}

func TestDispatch_Chat_DeliveredToSSE(t *testing.T) {
	m := testManager()

	delivered := simulateDispatch(m, "peer1", MQMsg{
		ID: "msg5", Topic: TopicChat, Payload: map[string]any{"text": "dm"},
	})

	if !delivered {
		t.Fatal("chat (DM) SHOULD be delivered to SSE")
	}
}

func TestDispatch_ChatBroadcast_DeliveredToSSE(t *testing.T) {
	m := testManager()

	delivered := simulateDispatch(m, "peer1", MQMsg{
		ID: "msg6", Topic: TopicChatBroadcast, Payload: map[string]any{"text": "broadcast"},
	})

	if !delivered {
		t.Fatal("chat.broadcast SHOULD be delivered to SSE")
	}
}

func TestDispatch_UnknownTopic_DeliveredToSSE(t *testing.T) {
	m := testManager()

	delivered := simulateDispatch(m, "peer1", MQMsg{
		ID: "msg7", Topic: "custom:something", Payload: "data",
	})

	if !delivered {
		t.Fatal("unknown topics SHOULD be delivered to SSE")
	}
}
