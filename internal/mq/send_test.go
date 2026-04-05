package mq

import (
	"context"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
)

func newTestHost(t *testing.T) *Manager {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return New(h)
}

func connectManagers(t *testing.T, a, b *Manager) {
	t.Helper()
	info := peer.AddrInfo{ID: b.host.ID(), Addrs: b.host.Addrs()}
	if err := a.host.Connect(context.Background(), info); err != nil {
		t.Fatal(err)
	}
}

func TestSend_DeliveredAndAcked(t *testing.T) {
	sender := newTestHost(t)
	receiver := newTestHost(t)
	connectManagers(t, sender, receiver)

	ch, cancel := receiver.Subscribe()
	defer cancel()

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()

	msgID, err := sender.Send(ctx, receiver.host.ID().String(), "chat", map[string]string{"text": "hello"})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if msgID == "" {
		t.Fatal("Send returned empty message ID")
	}

	select {
	case evt := <-ch:
		if evt.Type != "message" {
			t.Fatalf("expected message event, got %s", evt.Type)
		}
		if evt.Msg.Topic != "chat" {
			t.Fatalf("expected topic chat, got %s", evt.Msg.Topic)
		}
		if evt.Msg.ID != msgID {
			t.Fatalf("message ID mismatch: sent %s, received %s", msgID, evt.Msg.ID)
		}
		if evt.From != sender.host.ID().String() {
			t.Fatalf("expected from %s, got %s", sender.host.ID().String(), evt.From)
		}
		payload, ok := evt.Msg.Payload.(map[string]any)
		if !ok {
			t.Fatalf("payload not map[string]any: %T", evt.Msg.Payload)
		}
		if payload["text"] != "hello" {
			t.Fatalf("expected text=hello, got %v", payload["text"])
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}

func TestSend_TopicSubscriberReceives(t *testing.T) {
	sender := newTestHost(t)
	receiver := newTestHost(t)
	connectManagers(t, sender, receiver)

	received := make(chan string, 1)
	receiver.SubscribeTopic("test:", func(from, topic string, payload any) {
		received <- topic
	})

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()

	_, err := sender.Send(ctx, receiver.host.ID().String(), "test:foo", nil)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case topic := <-received:
		if topic != "test:foo" {
			t.Fatalf("expected topic test:foo, got %s", topic)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for topic subscriber")
	}
}

func drainUntilTopic(ch <-chan mqEvent, topic string, timeout <-chan time.Time) (mqEvent, bool) {
	for {
		select {
		case evt := <-ch:
			if evt.Msg != nil && evt.Msg.Topic == topic {
				return evt, true
			}
		case <-timeout:
			return mqEvent{}, false
		}
	}
}

func TestSend_Bidirectional(t *testing.T) {
	peerA := newTestHost(t)
	peerB := newTestHost(t)
	connectManagers(t, peerA, peerB)

	chA, cancelA := peerA.Subscribe()
	defer cancelA()
	chB, cancelB := peerB.Subscribe()
	defer cancelB()

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()

	_, err := peerA.Send(ctx, peerB.host.ID().String(), "chat", "from-A")
	if err != nil {
		t.Fatalf("A→B Send failed: %v", err)
	}

	_, err = peerB.Send(ctx, peerA.host.ID().String(), "chat", "from-B")
	if err != nil {
		t.Fatalf("B→A Send failed: %v", err)
	}

	deadline := time.After(5 * time.Second)

	evt, ok := drainUntilTopic(chB, "chat", deadline)
	if !ok {
		t.Fatal("timed out waiting for A→B message")
	}
	if evt.Msg.Payload != "from-A" {
		t.Fatalf("B expected payload from-A, got %v", evt.Msg.Payload)
	}

	evt, ok = drainUntilTopic(chA, "chat", deadline)
	if !ok {
		t.Fatal("timed out waiting for B→A message")
	}
	if evt.Msg.Payload != "from-B" {
		t.Fatalf("A expected payload from-B, got %v", evt.Msg.Payload)
	}
}

func TestSend_InvalidPeerID(t *testing.T) {
	sender := newTestHost(t)

	ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()

	_, err := sender.Send(ctx, "not-a-peer-id", "chat", nil)
	if err == nil {
		t.Fatal("Send to invalid peer ID should fail")
	}
}

func TestSend_UnreachablePeer(t *testing.T) {
	sender := newTestHost(t)
	unreachable := newTestHost(t)

	ctx, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()

	_, err := sender.Send(ctx, unreachable.host.ID().String(), "chat", nil)
	if err == nil {
		t.Fatal("Send to unconnected peer should fail")
	}
}

func TestSend_MultipleMessages_SequenceIncreases(t *testing.T) {
	sender := newTestHost(t)
	receiver := newTestHost(t)
	connectManagers(t, sender, receiver)

	ch, cancel := receiver.Subscribe()
	defer cancel()

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()

	for i := range 3 {
		_, err := sender.Send(ctx, receiver.host.ID().String(), "chat", i)
		if err != nil {
			t.Fatalf("Send %d failed: %v", i, err)
		}
	}

	var seqs []int64
	deadline := time.After(5 * time.Second)
	for range 3 {
		evt, ok := drainUntilTopic(ch, "chat", deadline)
		if !ok {
			t.Fatal("timed out waiting for messages")
		}
		seqs = append(seqs, evt.Msg.Seq)
	}

	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("sequence not monotonically increasing: %v", seqs)
		}
	}
}

func TestSend_InboxBuffering_NoListener(t *testing.T) {
	sender := newTestHost(t)
	receiver := newTestHost(t)
	connectManagers(t, sender, receiver)

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()

	_, err := sender.Send(ctx, receiver.host.ID().String(), "chat", "buffered")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	receiver.inboxMu.Lock()
	total := 0
	for _, entries := range receiver.inbox {
		total += len(entries)
	}
	receiver.inboxMu.Unlock()

	if total != 1 {
		t.Fatalf("expected 1 buffered message, got %d", total)
	}

	ch, cancel := receiver.Subscribe()
	defer cancel()

	select {
	case evt := <-ch:
		if evt.Msg.Payload != "buffered" {
			t.Fatalf("expected payload buffered, got %v", evt.Msg.Payload)
		}
	default:
		t.Fatal("subscribe should replay buffered message")
	}
}
