package mq

import (
	"testing"
)

func TestTopicSubscribe_PrefixMatch(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}

	var received []string
	m.SubscribeTopic("call:", func(from, topic string, payload any) {
		received = append(received, topic)
	})

	// Simulate dispatch logic from handleIncoming.
	for _, topic := range []string{"call:abc:offer", "call:abc:answer", "chat", "call:xyz"} {
		m.topicMu.RLock()
		for _, sub := range m.topicSubs {
			if len(topic) >= len(sub.prefix) && topic[:len(sub.prefix)] == sub.prefix {
				sub.fn("peer1", topic, nil)
			}
		}
		m.topicMu.RUnlock()
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 call: matches, got %d: %v", len(received), received)
	}
	for _, r := range received {
		if len(r) < 5 || r[:5] != "call:" {
			t.Errorf("unexpected topic %q in results", r)
		}
	}
}

func TestTopicSubscribe_Unsubscribe(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}

	calls := 0
	unsub := m.SubscribeTopic("test:", func(from, topic string, payload any) {
		calls++
	})

	// Fire before unsub.
	m.topicMu.RLock()
	for _, sub := range m.topicSubs {
		if len("test:foo") >= len(sub.prefix) && "test:foo"[:len(sub.prefix)] == sub.prefix {
			sub.fn("peer", "test:foo", nil)
		}
	}
	m.topicMu.RUnlock()

	unsub()

	// Fire after unsub — should not match.
	m.topicMu.RLock()
	for _, sub := range m.topicSubs {
		if len("test:bar") >= len(sub.prefix) && "test:bar"[:len(sub.prefix)] == sub.prefix {
			sub.fn("peer", "test:bar", nil)
		}
	}
	m.topicMu.RUnlock()

	if calls != 1 {
		t.Fatalf("expected 1 call before unsub, got %d", calls)
	}
}

func TestInboxBuffer_Replay(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}

	// Buffer messages before any listener.
	for i := range 5 {
		m.inboxMu.Lock()
		m.inbox["peer1"] = append(m.inbox["peer1"], inboxEntry{
			Msg:  MQMsg{ID: string(rune('A' + i)), Topic: "chat"},
			From: "peer1",
		})
		m.inboxMu.Unlock()
	}

	// Subscribe — should replay buffered messages.
	ch, cancel := m.Subscribe()
	defer cancel()

	count := 0
	for range 10 {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != 5 {
		t.Fatalf("expected 5 replayed messages, got %d", count)
	}

	// Inbox should be cleared after replay.
	m.inboxMu.Lock()
	remaining := len(m.inbox["peer1"])
	m.inboxMu.Unlock()
	if remaining != 0 {
		t.Fatalf("inbox should be empty after replay, got %d", remaining)
	}
}

func TestInboxBuffer_CapOverflow(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}

	// Fill inbox beyond cap.
	m.inboxMu.Lock()
	for i := range inboxCap + 50 {
		buf := m.inbox["peer1"]
		if len(buf) >= inboxCap {
			buf = buf[1:]
		}
		m.inbox["peer1"] = append(buf, inboxEntry{
			Msg:  MQMsg{ID: string(rune(i)), Topic: "chat"},
			From: "peer1",
		})
	}
	m.inboxMu.Unlock()

	m.inboxMu.Lock()
	n := len(m.inbox["peer1"])
	m.inboxMu.Unlock()

	if n > inboxCap {
		t.Fatalf("inbox should be capped at %d, got %d", inboxCap, n)
	}
}

func TestNotifyDelivered_DispatchToListeners(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}

	ch, cancel := m.Subscribe()
	defer cancel()

	m.NotifyDelivered("msg-123")

	select {
	case evt := <-ch:
		if evt.Type != "delivered" {
			t.Fatalf("expected delivered event, got %s", evt.Type)
		}
		if evt.MsgID != "msg-123" {
			t.Fatalf("expected msg_id msg-123, got %s", evt.MsgID)
		}
	default:
		t.Fatal("listener should receive delivered event")
	}
}

func TestPublishLocal_DispatchToListeners(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
		selfID:    "self",
	}

	ch, cancel := m.Subscribe()
	defer cancel()

	m.PublishLocal("peer:announce", "", map[string]string{"name": "TestPeer"})

	select {
	case evt := <-ch:
		if evt.Type != "message" {
			t.Fatalf("expected message event, got %s", evt.Type)
		}
		if evt.Msg.Topic != "peer:announce" {
			t.Fatalf("expected topic peer:announce, got %s", evt.Msg.Topic)
		}
	default:
		t.Fatal("listener should receive PublishLocal event")
	}
}

func TestPublishLocal_NoListeners_NoPanic(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
		selfID:    "self",
	}

	// Should not panic with no listeners.
	m.PublishLocal("test:topic", "", "payload")
}

func TestSubscribe_Cancel(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}

	_, cancel1 := m.Subscribe()
	_, cancel2 := m.Subscribe()

	m.listenerMu.RLock()
	n := len(m.listeners)
	m.listenerMu.RUnlock()
	if n != 2 {
		t.Fatalf("expected 2 listeners, got %d", n)
	}

	cancel1()
	m.listenerMu.RLock()
	n = len(m.listeners)
	m.listenerMu.RUnlock()
	if n != 1 {
		t.Fatalf("expected 1 listener after cancel, got %d", n)
	}

	cancel2()
	m.listenerMu.RLock()
	n = len(m.listeners)
	m.listenerMu.RUnlock()
	if n != 0 {
		t.Fatalf("expected 0 listeners after both cancel, got %d", n)
	}
}

func TestSubscribe_DoubleCancel_NoPanic(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}

	_, cancel := m.Subscribe()
	cancel()
	cancel() // should not panic
}

func TestLogMQEvent_SkipsLogTopics(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
		selfID:    "self",
	}

	ch, cancel := m.Subscribe()
	defer cancel()

	// log:* and mq.ack topics should be silently dropped.
	m.logMQEvent("recv", "log:mq", "peer1", "", "direct", false)
	m.logMQEvent("recv", "mq.ack", "peer1", "", "direct", false)

	select {
	case evt := <-ch:
		t.Fatalf("should not receive log events, got topic=%s", evt.Msg.Topic)
	default:
		// expected — no events
	}
}

func TestInboxBuffer_MultiPeerReplay(t *testing.T) {
	m := &Manager{
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}

	// Buffer from two different peers.
	m.inboxMu.Lock()
	m.inbox["peerA"] = []inboxEntry{
		{Msg: MQMsg{ID: "a1", Topic: "chat"}, From: "peerA"},
		{Msg: MQMsg{ID: "a2", Topic: "chat"}, From: "peerA"},
	}
	m.inbox["peerB"] = []inboxEntry{
		{Msg: MQMsg{ID: "b1", Topic: "group:xyz"}, From: "peerB"},
	}
	m.inboxMu.Unlock()

	ch, cancel := m.Subscribe()
	defer cancel()

	count := 0
	for range 10 {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != 3 {
		t.Fatalf("expected 3 replayed messages from 2 peers, got %d", count)
	}
}
