package testpeer

import (
	"context"
	"strings"
	"sync"

	"github.com/petervdpas/goop2/internal/mq"
)

// compile-time check: *MQAdapter implements mq.Transport
var _ mq.Transport = (*MQAdapter)(nil)

// topicSubscription holds one topic-prefix callback.
type topicSubscription struct {
	prefix string
	fn     func(from, topic string, payload any)
}

// MQAdapter implements mq.Transport by routing messages through a TestBus.
// Each TestPeer gets its own MQAdapter — it's the peer's "MQ Manager" in tests.
type MQAdapter struct {
	bus    *TestBus
	selfID string

	subMu sync.RWMutex
	subs  []topicSubscription
}

// NewMQAdapter creates an adapter for the given peer and registers it on the bus.
func NewMQAdapter(bus *TestBus, peerID string) *MQAdapter {
	a := &MQAdapter{
		bus:    bus,
		selfID: peerID,
	}
	bus.register(peerID, a)
	return a
}

// Send routes a message through the TestBus to the target peer.
// The target peer's topic subscribers are called synchronously.
func (a *MQAdapter) Send(_ context.Context, peerID, topic string, payload any) (string, error) {
	return a.bus.route(a.selfID, peerID, topic, payload)
}

// SubscribeTopic registers a callback for messages whose topic starts with prefix.
// Returns an unsubscribe function.
func (a *MQAdapter) SubscribeTopic(prefix string, fn func(from, topic string, payload any)) func() {
	sub := topicSubscription{prefix: prefix, fn: fn}

	a.subMu.Lock()
	a.subs = append(a.subs, sub)
	idx := len(a.subs) - 1
	a.subMu.Unlock()

	return func() {
		a.subMu.Lock()
		defer a.subMu.Unlock()
		if idx < len(a.subs) {
			a.subs[idx] = a.subs[len(a.subs)-1]
			a.subs = a.subs[:len(a.subs)-1]
		}
	}
}

// PublishLocal delivers a message to this peer's own topic subscribers.
// Used for local-only events (peer:announce, group membership, etc.).
func (a *MQAdapter) PublishLocal(topic, from string, payload any) {
	if from == "" {
		from = a.selfID
	}
	a.dispatch(from, topic, payload)
}

// deliver is called by the TestBus when a message arrives from another peer.
// It dispatches to matching topic subscribers.
func (a *MQAdapter) deliver(from, topic string, payload any) {
	a.dispatch(from, topic, payload)
}

// dispatch calls all topic subscribers whose prefix matches the topic.
func (a *MQAdapter) dispatch(from, topic string, payload any) {
	a.subMu.RLock()
	matches := make([]func(string, string, any), 0, len(a.subs))
	for _, sub := range a.subs {
		if strings.HasPrefix(topic, sub.prefix) {
			matches = append(matches, sub.fn)
		}
	}
	a.subMu.RUnlock()

	for _, fn := range matches {
		fn(from, topic, payload)
	}
}

// Close removes this adapter from the bus.
func (a *MQAdapter) Close() {
	a.bus.unregister(a.selfID)
}
