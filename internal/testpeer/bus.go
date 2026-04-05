package testpeer

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BusMessage records a single message routed through the TestBus.
type BusMessage struct {
	From    string
	To      string
	Topic   string
	Payload any
	TS      time.Time
}

// TestBus is an in-process message router that connects multiple TestPeers.
// It replaces libp2p streams for testing — messages are delivered synchronously
// to topic subscribers on the receiving peer's MQAdapter.
type TestBus struct {
	mu      sync.Mutex
	adapters map[string]*MQAdapter // peerID → adapter
	log      []BusMessage
}

// NewTestBus creates a new in-process message bus.
func NewTestBus() *TestBus {
	return &TestBus{
		adapters: make(map[string]*MQAdapter),
	}
}

// register adds an adapter to the bus. Called by NewMQAdapter.
func (b *TestBus) register(peerID string, adapter *MQAdapter) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.adapters[peerID] = adapter
}

// unregister removes an adapter from the bus.
func (b *TestBus) unregister(peerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.adapters, peerID)
}

// route delivers a message from one peer to another, synchronously.
// Called by MQAdapter.Send. Returns a message ID.
func (b *TestBus) route(from, to, topic string, payload any) (string, error) {
	b.mu.Lock()
	target, ok := b.adapters[to]
	b.log = append(b.log, BusMessage{From: from, To: to, Topic: topic, Payload: payload, TS: time.Now()})
	b.mu.Unlock()

	if !ok {
		return "", fmt.Errorf("testbus: peer %q not connected", to)
	}

	msgID := uuid.NewString()
	target.deliver(from, topic, payload)
	return msgID, nil
}

// broadcast delivers a message to all peers except the sender.
// Called by MQAdapter.PublishLocal when broadcasting presence.
func (b *TestBus) broadcast(from, topic string, payload any) {
	b.mu.Lock()
	targets := make([]*MQAdapter, 0, len(b.adapters))
	for id, a := range b.adapters {
		if id != from {
			targets = append(targets, a)
		}
	}
	b.log = append(b.log, BusMessage{From: from, To: "*", Topic: topic, Payload: payload, TS: time.Now()})
	b.mu.Unlock()

	for _, a := range targets {
		a.deliver(from, topic, payload)
	}
}

// Messages returns all recorded messages matching the filter.
// Pass nil to get all messages.
func (b *TestBus) Messages(filter func(BusMessage) bool) []BusMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	if filter == nil {
		cp := make([]BusMessage, len(b.log))
		copy(cp, b.log)
		return cp
	}
	var result []BusMessage
	for _, m := range b.log {
		if filter(m) {
			result = append(result, m)
		}
	}
	return result
}

// MessagesForTopic returns all messages with the given topic prefix.
func (b *TestBus) MessagesForTopic(prefix string) []BusMessage {
	return b.Messages(func(m BusMessage) bool {
		return strings.HasPrefix(m.Topic, prefix)
	})
}

// MessageCount returns the total number of routed messages.
func (b *TestBus) MessageCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.log)
}
