package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	// ChatProtocolID is the libp2p protocol ID for chat
	ChatProtocolID = "/goop/chat/1.0.0"

	// DefaultBufferSize is the default number of messages to keep in memory
	DefaultBufferSize = 100
)

// Manager handles chat operations for a peer
type Manager struct {
	host        host.Host
	mu          sync.RWMutex
	messages    []*Message      // in-memory message buffer
	bufferSize  int             // max messages to keep
	listeners   []chan *Message // SSE listeners
	localPeerID string          // our peer ID
}

// New creates a new chat manager
func New(h host.Host, bufferSize int) *Manager {
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}

	m := &Manager{
		host:        h,
		messages:    make([]*Message, 0, bufferSize),
		bufferSize:  bufferSize,
		listeners:   make([]chan *Message, 0),
		localPeerID: h.ID().String(),
	}

	// Register stream handler
	h.SetStreamHandler(protocol.ID(ChatProtocolID), m.handleStream)

	return m
}

// SendDirect sends a direct message to a specific peer
func (m *Manager) SendDirect(ctx context.Context, toPeerID, content string) error {
	peerID, err := peer.Decode(toPeerID)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	msg := NewMessage(m.localPeerID, toPeerID, content)

	// Open stream to peer
	stream, err := m.host.NewStream(ctx, peerID, protocol.ID(ChatProtocolID))
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Send message as JSON
	if err := json.NewEncoder(stream).Encode(msg); err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	// Store in local buffer (outgoing)
	m.addMessage(msg)

	log.Printf("CHAT: Sent direct message to %s", toPeerID)
	return nil
}

// SendBroadcast sends a message to all connected peers
func (m *Manager) SendBroadcast(ctx context.Context, content string) error {
	msg := NewBroadcast(m.localPeerID, content)

	// Get all connected peers
	peers := m.host.Network().Peers()
	if len(peers) == 0 {
		// Still store locally even if no peers
		m.addMessage(msg)
		log.Printf("CHAT: Broadcast message stored (no peers connected)")
		return nil
	}

	var lastErr error
	sentCount := 0

	for _, peerID := range peers {
		// Open stream to peer
		stream, err := m.host.NewStream(ctx, peerID, protocol.ID(ChatProtocolID))
		if err != nil {
			lastErr = err
			log.Printf("CHAT: Failed to open stream to %s for broadcast: %v", peerID, err)
			continue
		}

		// Send message as JSON
		if err := json.NewEncoder(stream).Encode(msg); err != nil {
			stream.Close()
			lastErr = err
			log.Printf("CHAT: Failed to send broadcast to %s: %v", peerID, err)
			continue
		}

		stream.Close()
		sentCount++
	}

	// Store in local buffer (outgoing)
	m.addMessage(msg)

	log.Printf("CHAT: Broadcast sent to %d/%d peers", sentCount, len(peers))

	if sentCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to send to any peer: %w", lastErr)
	}

	return nil
}

// GetMessages returns all messages in the buffer
func (m *Manager) GetMessages() []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy
	messages := make([]*Message, len(m.messages))
	copy(messages, m.messages)
	return messages
}

// GetConversation returns messages for a specific peer conversation
func (m *Manager) GetConversation(peerID string) []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conversation := make([]*Message, 0)
	for _, msg := range m.messages {
		if msg.Type == MessageTypeDirect &&
			((msg.From == peerID && msg.To == m.localPeerID) ||
				(msg.From == m.localPeerID && msg.To == peerID)) {
			conversation = append(conversation, msg)
		}
	}
	return conversation
}

// GetBroadcasts returns all broadcast messages
func (m *Manager) GetBroadcasts() []*Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	broadcasts := make([]*Message, 0)
	for _, msg := range m.messages {
		if msg.Type == MessageTypeBroadcast {
			broadcasts = append(broadcasts, msg)
		}
	}
	return broadcasts
}

// LocalPeerID returns the local peer ID
func (m *Manager) LocalPeerID() string {
	return m.localPeerID
}

// Subscribe returns a channel that receives new messages
func (m *Manager) Subscribe() <-chan *Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan *Message, 10)
	m.listeners = append(m.listeners, ch)
	return ch
}

// Unsubscribe removes a listener channel
func (m *Manager) Unsubscribe(ch <-chan *Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, listener := range m.listeners {
		if listener == ch {
			close(listener)
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			return
		}
	}
}

// handleStream handles incoming chat streams
func (m *Manager) handleStream(stream network.Stream) {
	defer stream.Close()

	remotePeer := stream.Conn().RemotePeer().String()

	// Read message
	var msg Message
	if err := json.NewDecoder(io.LimitReader(stream, 1024*1024)).Decode(&msg); err != nil {
		log.Printf("CHAT: Failed to decode message from %s: %v", remotePeer, err)
		return
	}

	// Validate sender
	if msg.From != remotePeer {
		log.Printf("CHAT: Message from %s claims to be from %s, rejecting", remotePeer, msg.From)
		return
	}

	// Add timestamp if missing
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}

	// Store message
	m.addMessage(&msg)

	log.Printf("CHAT: Received message from %s: %.50s", msg.From, msg.Content)
}

// addMessage adds a message to the buffer and notifies listeners
func (m *Manager) addMessage(msg *Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Add to buffer
	m.messages = append(m.messages, msg)

	// Trim buffer if needed
	if len(m.messages) > m.bufferSize {
		m.messages = m.messages[len(m.messages)-m.bufferSize:]
	}

	// Notify listeners
	for _, listener := range m.listeners {
		select {
		case listener <- msg:
		default:
			// Listener buffer full, skip
		}
	}
}

// Close shuts down the chat manager
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all listener channels
	for _, listener := range m.listeners {
		close(listener)
	}
	m.listeners = nil

	return nil
}
