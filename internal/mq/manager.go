package mq

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/petervdpas/goop2/internal/proto"
)

const (
	// inboxCap is the maximum number of messages buffered per peer before
	// the browser SSE listener connects and drains the buffer.
	inboxCap = 200

	// ackTimeout is how long Send() waits for a transport ACK from the remote
	// peer before returning an error to the caller.
	ackTimeout = 10 * time.Second
)

// Manager owns the MQ P2P handler, per-peer in-memory inbox, and SSE listeners.
type Manager struct {
	host   host.Host
	selfID string

	seq int64 // atomic monotonic counter for outbound messages

	// Per-peer in-memory inbox: messages arrived before browser SSE connected.
	inboxMu sync.Mutex
	inbox   map[string][]inboxEntry // peerID → buffered messages

	// Pending ACK channels: msg ID → channel closed/sent when ACK arrives.
	ackMu   sync.Mutex
	pending map[string]chan struct{}

	// SSE listeners: browser connects one channel per /api/mq/events connection.
	listenerMu sync.RWMutex
	listeners  map[chan mqEvent]struct{}

	// Topic subscribers (for call.Signaler adapter).
	topicMu   sync.RWMutex
	topicSubs []topicSub
}

type topicSub struct {
	prefix string
	fn     func(from, topic string, payload any)
}

// inboxEntry pairs a buffered message with the peer that sent it.
type inboxEntry struct {
	Msg  MQMsg
	From string
}

// New creates a new MQ Manager and registers the /goop/mq/1.0.0 stream handler.
func New(h host.Host) *Manager {
	m := &Manager{
		host:      h,
		selfID:    h.ID().String(),
		inbox:     make(map[string][]inboxEntry),
		pending:   make(map[string]chan struct{}),
		listeners: make(map[chan mqEvent]struct{}),
	}
	h.SetStreamHandler(protocol.ID(proto.MQProtoID), m.handleIncoming)
	log.Printf("MQ: registered handler for %s", proto.MQProtoID)
	return m
}

// peerSupportsMQ returns false only when the peerstore has a non-empty protocol
// list for the peer and /goop/mq/1.0.0 is absent from that list.
// If the protocol list is unknown (empty or error), we optimistically return true
// so a live connection attempt is still made.
func (m *Manager) peerSupportsMQ(pid peer.ID) bool {
	protos, err := m.host.Peerstore().GetProtocols(pid)
	if err != nil || len(protos) == 0 {
		return true // unknown — optimistically try
	}
	for _, p := range protos {
		if p == protocol.ID(proto.MQProtoID) {
			return true
		}
	}
	return false
}

// Send opens (or reuses) a stream to peerID, writes a message with the given
// topic and payload, and waits up to ackTimeout for a transport ACK.
// Returns the message ID and nil on success, or an error if the send or ACK fails.
func (m *Manager) Send(ctx context.Context, peerID, topic string, payload any) (string, error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return "", fmt.Errorf("mq: invalid peer id %q: %w", peerID, err)
	}

	// Fast-fail if we know from the peerstore that this peer doesn't support MQ.
	// This avoids a dial attempt + timeout for old clients.
	if !m.peerSupportsMQ(pid) {
		return "", fmt.Errorf("protocols not supported: [%s]", proto.MQProtoID)
	}

	msgID := uuid.NewString()
	seq := atomic.AddInt64(&m.seq, 1)

	msg := MQMsg{
		Type:    MsgTypeMsg,
		ID:      msgID,
		Seq:     seq,
		Topic:   topic,
		Payload: payload,
	}

	// Register ACK channel before opening the stream so we don't miss it.
	ackCh := make(chan struct{}, 1)
	m.ackMu.Lock()
	m.pending[msgID] = ackCh
	m.ackMu.Unlock()

	defer func() {
		m.ackMu.Lock()
		delete(m.pending, msgID)
		m.ackMu.Unlock()
	}()

	// Open a new stream (libp2p reuses the underlying muxed connection).
	dialCtx, cancel := context.WithTimeout(ctx, ackTimeout)
	defer cancel()

	stream, err := m.host.NewStream(dialCtx, pid, protocol.ID(proto.MQProtoID))
	if err != nil {
		go m.logMQEvent("error", topic, peerID, "unreachable", "")
		return "", fmt.Errorf("mq: open stream to %s: %w", peerID, err)
	}
	defer stream.Close()

	// Write the message as newline-delimited JSON.
	enc := json.NewEncoder(stream)
	if err := enc.Encode(msg); err != nil {
		return "", fmt.Errorf("mq: encode msg: %w", err)
	}

	// Read the transport ACK from the stream (remote writes it back synchronously).
	var ack MQAck
	dec := json.NewDecoder(bufio.NewReader(stream))
	_ = stream.SetReadDeadline(time.Now().Add(ackTimeout))
	if err := dec.Decode(&ack); err != nil {
		return "", fmt.Errorf("mq: waiting for ack from %s: %w", peerID, err)
	}
	if ack.ID != msgID {
		return "", fmt.Errorf("mq: ack id mismatch (got %s, want %s)", ack.ID, msgID)
	}

	// Also signal the in-process pending channel (used by SubscribeTopic callers).
	select {
	case ackCh <- struct{}{}:
	default:
	}

	log.Printf("MQ: sent msg %s (topic=%s) to %s", msgID[:8], topic, peerID[:8])
	go m.logMQEvent("send", topic, peerID, "", connVia(stream))
	return msgID, nil
}

// handleIncoming is the libp2p stream handler for /goop/mq/1.0.0.
// It reads one MQMsg, sends the transport ACK immediately, then dispatches.
func (m *Manager) handleIncoming(stream network.Stream) {
	defer stream.Close()

	remotePeer := stream.Conn().RemotePeer().String()

	_ = stream.SetReadDeadline(time.Now().Add(30 * time.Second))

	var msg MQMsg
	if err := json.NewDecoder(bufio.NewReader(stream)).Decode(&msg); err != nil {
		log.Printf("MQ: decode error from %s: %v", remotePeer[:8], err)
		return
	}

	// Validate sender.
	if remotePeer != stream.Conn().RemotePeer().String() {
		log.Printf("MQ: peer mismatch, dropping")
		return
	}

	// Send transport ACK immediately — bytes are in the buffer.
	ack := MQAck{Type: MsgTypeAck, ID: msg.ID, Seq: msg.Seq}
	_ = stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewEncoder(stream).Encode(ack); err != nil {
		log.Printf("MQ: ack write error to %s: %v", remotePeer[:8], err)
		// Continue dispatching even if ACK write failed.
	}

	log.Printf("MQ: received msg %s (topic=%s) from %s", msg.ID[:8], msg.Topic, remotePeer[:8])

	// Dispatch to topic subscribers (call.Signaler adapter etc.)
	m.topicMu.RLock()
	for _, sub := range m.topicSubs {
		if strings.HasPrefix(msg.Topic, sub.prefix) {
			go sub.fn(remotePeer, msg.Topic, msg.Payload)
		}
	}
	m.topicMu.RUnlock()

	evt := mqEvent{
		Type: "message",
		Msg:  &msg,
		From: remotePeer,
	}

	// Deliver to SSE listeners, or buffer for later.
	m.listenerMu.RLock()
	n := len(m.listeners)
	for ch := range m.listeners {
		select {
		case ch <- evt:
		default:
			log.Printf("MQ: SSE listener full, dropping msg %s", msg.ID[:8])
		}
	}
	m.listenerMu.RUnlock()

	if n == 0 {
		// No browser connected yet — buffer the message.
		m.inboxMu.Lock()
		buf := m.inbox[remotePeer]
		if len(buf) >= inboxCap {
			buf = buf[1:] // drop oldest
		}
		m.inbox[remotePeer] = append(buf, inboxEntry{Msg: msg, From: remotePeer})
		m.inboxMu.Unlock()
	}

	go m.logMQEvent("recv", msg.Topic, remotePeer, "", connVia(stream))
}

// NotifyDelivered dispatches a "delivered" event to SSE listeners.
// Called by the /api/mq/ack HTTP handler after it sends the p2p ack back.
func (m *Manager) NotifyDelivered(msgID string) {
	evt := mqEvent{Type: "delivered", MsgID: msgID}
	m.listenerMu.RLock()
	for ch := range m.listeners {
		select {
		case ch <- evt:
		default:
		}
	}
	m.listenerMu.RUnlock()
}

// Subscribe returns a channel that receives mqEvents and a cancel function.
// On subscribe, all buffered inbox messages (across all peers) are replayed
// immediately so the browser never misses a message.
func (m *Manager) Subscribe() (<-chan mqEvent, func()) {
	ch := make(chan mqEvent, 128)

	m.listenerMu.Lock()
	m.listeners[ch] = struct{}{}
	m.listenerMu.Unlock()

	// Replay buffered inbox.
	m.inboxMu.Lock()
	var buffered []inboxEntry
	for _, entries := range m.inbox {
		buffered = append(buffered, entries...)
	}
	m.inbox = make(map[string][]inboxEntry) // clear after replay
	m.inboxMu.Unlock()

	for i := range buffered {
		select {
		case ch <- mqEvent{Type: "message", Msg: &buffered[i].Msg, From: buffered[i].From}:
		default:
		}
	}

	cancel := func() {
		m.listenerMu.Lock()
		if _, ok := m.listeners[ch]; ok {
			delete(m.listeners, ch)
			close(ch)
		}
		m.listenerMu.Unlock()
	}
	return ch, cancel
}

// PublishLocal injects a message directly into SSE listener channels without
// opening any P2P stream. Use this to push Go-side state changes to the browser
// (e.g. group membership updates, listen state). The ack machinery is bypassed —
// the browser will attempt to ack with from="", which the server silently drops.
func (m *Manager) PublishLocal(topic, from string, payload any) {
	msg := MQMsg{
		Type:    MsgTypeMsg,
		ID:      uuid.NewString(),
		Seq:     atomic.AddInt64(&m.seq, 1),
		Topic:   topic,
		Payload: payload,
	}
	evt := mqEvent{Type: "message", Msg: &msg, From: from}
	m.listenerMu.RLock()
	defer m.listenerMu.RUnlock()
	for ch := range m.listeners {
		select {
		case ch <- evt:
		default:
			log.Printf("MQ: PublishLocal SSE listener full, dropping topic=%s", topic)
		}
	}
}

// connVia returns "relay:<relayID8>" if the stream is routed through a circuit
// relay (with the first 8 chars of the relay peer ID), or "direct" otherwise.
func connVia(s network.Stream) string {
	ma := s.Conn().RemoteMultiaddr().String()
	circuitIdx := strings.Index(ma, "/p2p-circuit")
	if circuitIdx < 0 {
		return "direct"
	}
	// Multiaddr before /p2p-circuit: .../p2p/<relayPeerID>/p2p-circuit
	before := ma[:circuitIdx]
	if p2pIdx := strings.LastIndex(before, "/p2p/"); p2pIdx >= 0 {
		relayID := before[p2pIdx+5:]
		if len(relayID) > 8 {
			relayID = relayID[:8]
		}
		return "relay:" + relayID
	}
	return "relay"
}

// logMQEvent publishes a structured log entry for an MQ message to browser listeners.
// dir is "recv", "send", or "error"; via is "direct", "relay", or "" (unknown/error path).
// Skips log:* and mq.ack topics to prevent noise/recursion.
func (m *Manager) logMQEvent(dir, topic, peerID, errMsg, via string) {
	if strings.HasPrefix(topic, "log:") || topic == "mq.ack" {
		return
	}
	entry := map[string]any{
		"dir":   dir,
		"topic": topic,
		"peer":  peerID,
		"ts":    time.Now().UnixMilli(),
	}
	if errMsg != "" {
		entry["error"] = errMsg
	}
	if via != "" {
		entry["via"] = via
	}
	m.PublishLocal("log:mq", "", entry)
}

// SubscribeTopic registers a callback for messages whose topic has the given prefix.
// Returns an unsubscribe function.
func (m *Manager) SubscribeTopic(prefix string, fn func(from, topic string, payload any)) func() {
	sub := topicSub{prefix: prefix, fn: fn}

	m.topicMu.Lock()
	m.topicSubs = append(m.topicSubs, sub)
	idx := len(m.topicSubs) - 1
	m.topicMu.Unlock()

	return func() {
		m.topicMu.Lock()
		defer m.topicMu.Unlock()
		if idx < len(m.topicSubs) {
			// Replace with last element and shrink.
			m.topicSubs[idx] = m.topicSubs[len(m.topicSubs)-1]
			m.topicSubs = m.topicSubs[:len(m.topicSubs)-1]
		}
	}
}
