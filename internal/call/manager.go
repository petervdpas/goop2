// Package call manages native WebRTC call sessions using Pion.
// It is designed to be maximally standalone — it imports only Pion libraries
// and stdlib. Coupling to the rest of goop2 is via the Signaler interface only.
package call

import (
	"context"
	"log"
	"sync"
)

// Manager owns active call sessions and bridges realtime signaling to them.
type Manager struct {
	sig    Signaler
	selfID string

	mu       sync.RWMutex
	sessions map[string]*Session

	incomingMu sync.RWMutex
	incoming   []func(*IncomingCall)

	done chan struct{}
}

// New creates a new call Manager attached to sig and starts listening for
// signaling messages immediately.
func New(sig Signaler, selfID string) *Manager {
	m := &Manager{
		sig:      sig,
		selfID:   selfID,
		sessions: make(map[string]*Session),
		done:     make(chan struct{}),
	}
	go m.dispatchLoop()
	return m
}

// OnIncoming registers a callback that is fired for each incoming call-request.
// Multiple handlers can be registered; each SSE connection in call.go registers one.
func (m *Manager) OnIncoming(fn func(*IncomingCall)) {
	m.incomingMu.Lock()
	m.incoming = append(m.incoming, fn)
	m.incomingMu.Unlock()
}

// StartCall creates a new outbound call session on channelID to remotePeer.
func (m *Manager) StartCall(ctx context.Context, channelID, remotePeer string) (*Session, error) {
	sess := newSession(channelID, remotePeer, m.sig)
	m.mu.Lock()
	m.sessions[channelID] = sess
	m.mu.Unlock()
	log.Printf("CALL: started %s → %s", channelID, remotePeer)
	return sess, nil
}

// AcceptCall creates a session for an incoming call.
func (m *Manager) AcceptCall(ctx context.Context, channelID, remotePeer string) (*Session, error) {
	sess := newSession(channelID, remotePeer, m.sig)
	m.mu.Lock()
	m.sessions[channelID] = sess
	m.mu.Unlock()
	log.Printf("CALL: accepted %s from %s", channelID, remotePeer)
	return sess, nil
}

// GetSession returns the active session for channelID, if any.
func (m *Manager) GetSession(channelID string) (*Session, bool) {
	m.mu.RLock()
	s, ok := m.sessions[channelID]
	m.mu.RUnlock()
	return s, ok
}

// removeSession removes a session from the tracking map.
func (m *Manager) removeSession(channelID string) {
	m.mu.Lock()
	delete(m.sessions, channelID)
	m.mu.Unlock()
}

// Close shuts down the manager and hangs up all active sessions.
func (m *Manager) Close() {
	select {
	case <-m.done:
		return
	default:
		close(m.done)
	}

	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, s := range sessions {
		s.Hangup()
	}
}

// dispatchLoop reads signaling envelopes from the Signaler and routes them.
func (m *Manager) dispatchLoop() {
	ch, cancel := m.sig.Subscribe()
	defer cancel()

	for {
		select {
		case <-m.done:
			return
		case env, ok := <-ch:
			if !ok {
				return
			}
			m.dispatch(env)
		}
	}
}

// dispatch routes one signaling envelope to the appropriate session or
// fires OnIncoming handlers for new call-request messages.
func (m *Manager) dispatch(env *Envelope) {
	payload, ok := env.Payload.(map[string]any)
	if !ok {
		return
	}
	msgType, _ := payload["type"].(string)

	if msgType == "call-request" {
		ic := &IncomingCall{
			ChannelID:  env.Channel,
			RemotePeer: env.From,
			Accept: func(ctx context.Context) (*Session, error) {
				return m.AcceptCall(ctx, env.Channel, env.From)
			},
			Reject: func() {
				_ = m.sig.Send(env.Channel, map[string]any{"type": "call-hangup"})
				m.removeSession(env.Channel)
			},
		}
		m.incomingMu.RLock()
		handlers := make([]func(*IncomingCall), len(m.incoming))
		copy(handlers, m.incoming)
		m.incomingMu.RUnlock()
		for _, fn := range handlers {
			fn(ic)
		}
		return
	}

	// Route other signals (offer, answer, ice-candidate, hangup) to existing session.
	m.mu.RLock()
	sess, ok := m.sessions[env.Channel]
	m.mu.RUnlock()
	if ok {
		sess.handleSignal(msgType, payload)
	}
}
