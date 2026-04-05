package mq

import (
	"context"
	"fmt"
)

// Transport is the MQ abstraction used by all managers (group, chat, cluster, etc.).
// It covers the full messaging surface: sending to peers, subscribing to topics,
// and publishing local-only events to the browser SSE stream.
//
// The concrete *Manager satisfies this via libp2p streams.
// For testing, testpeer.MQAdapter provides an in-process implementation.
type Transport interface {
	Send(ctx context.Context, peerID, topic string, payload any) (string, error)
	SubscribeTopic(prefix string, fn func(from, topic string, payload any)) func()
	PublishLocal(topic, from string, payload any)
}

// compile-time check: *Manager implements Transport
var _ Transport = (*Manager)(nil)

// NopTransport is a no-op Transport for unit tests that don't need messaging.
// Send returns an error, SubscribeTopic returns a no-op unsubscribe,
// PublishLocal is silently dropped.
type NopTransport struct{}

func (NopTransport) Send(_ context.Context, peerID, topic string, _ any) (string, error) {
	return "", fmt.Errorf("nop: no MQ transport (send %s to %s)", topic, peerID)
}

func (NopTransport) SubscribeTopic(_ string, _ func(string, string, any)) func() {
	return func() {}
}

func (NopTransport) PublishLocal(_, _ string, _ any) {}
