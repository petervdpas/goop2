// Package call manages native WebRTC call sessions using Pion.
// It is designed to be maximally standalone — it imports only stdlib.
// Coupling to the rest of goop2 is via the Signaler interface only.
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

	mu           sync.RWMutex
	sessions     map[string]*Session
	pendingCalls map[string]*IncomingCall // channels where call-request fired but not yet accepted/rejected

	// Subscription-based incoming call notifications.
	// Each /api/call/events SSE connection subscribes one channel and unsubscribes
	// on disconnect — prevents the handler-slice leak of the callback approach.
	incomingMu        sync.RWMutex
	incomingListeners map[chan *IncomingCall]struct{}

	done chan struct{}
}

// New creates a new call Manager attached to sig and starts listening for
// signaling messages immediately.
func New(sig Signaler, selfID string) *Manager {
	m := &Manager{
		sig:               sig,
		selfID:            selfID,
		sessions:          make(map[string]*Session),
		pendingCalls:      make(map[string]*IncomingCall),
		incomingListeners: make(map[chan *IncomingCall]struct{}),
		done:              make(chan struct{}),
	}
	go m.dispatchLoop()
	return m
}

// SubscribeIncoming returns a channel that receives incoming call notifications.
// Any call-requests that arrived before this subscription is registered are
// replayed immediately so the browser never misses a call due to SSE timing.
// Call UnsubscribeIncoming when done (e.g. on SSE client disconnect) to avoid leaks.
func (m *Manager) SubscribeIncoming() chan *IncomingCall {
	ch := make(chan *IncomingCall, 8)
	m.incomingMu.Lock()
	m.incomingListeners[ch] = struct{}{}
	m.incomingMu.Unlock()

	// Replay any call-requests that arrived before this SSE connected.
	m.mu.RLock()
	for _, ic := range m.pendingCalls {
		select {
		case ch <- ic:
		default:
		}
	}
	m.mu.RUnlock()

	return ch
}

// UnsubscribeIncoming removes the subscription and closes the channel.
func (m *Manager) UnsubscribeIncoming(ch chan *IncomingCall) {
	m.incomingMu.Lock()
	if _, ok := m.incomingListeners[ch]; ok {
		delete(m.incomingListeners, ch)
		close(ch)
	}
	m.incomingMu.Unlock()
}

// StartCall creates a new outbound call session on channelID to remotePeer.
func (m *Manager) StartCall(ctx context.Context, channelID, remotePeer string) (*Session, error) {
	sess := newSession(channelID, remotePeer, m.sig, true)
	m.mu.Lock()
	m.sessions[channelID] = sess
	m.mu.Unlock()
	log.Printf("CALL: started %s → %s", channelID, remotePeer)
	return sess, nil
}

// AcceptCall creates a session for an incoming call and sends call-ack to the caller.
func (m *Manager) AcceptCall(ctx context.Context, channelID, remotePeer string) (*Session, error) {
	sess := newSession(channelID, remotePeer, m.sig, false)
	m.mu.Lock()
	m.sessions[channelID] = sess
	delete(m.pendingCalls, channelID)
	m.mu.Unlock()
	// Notify the caller that we accepted — they can proceed with SDP exchange.
	_ = m.sig.Send(channelID, map[string]any{"type": "call-ack"})
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

// AllSessions returns a snapshot of all active sessions (for the debug endpoint).
func (m *Manager) AllSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

// removeSession removes a session (and any pending call-request record) from tracking.
func (m *Manager) removeSession(channelID string) {
	m.mu.Lock()
	delete(m.sessions, channelID)
	delete(m.pendingCalls, channelID)
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
	m.pendingCalls = make(map[string]*IncomingCall)
	m.mu.Unlock()
	for _, s := range sessions {
		s.Hangup()
	}

	m.incomingMu.Lock()
	for ch := range m.incomingListeners {
		close(ch)
	}
	m.incomingListeners = nil
	m.incomingMu.Unlock()
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

// dispatch routes one signaling envelope to the appropriate session or fires
// incoming-call listeners for new call-request messages.
func (m *Manager) dispatch(env *Envelope) {
	payload, ok := env.Payload.(map[string]any)
	if !ok {
		return
	}
	msgType, _ := payload["type"].(string)

	if msgType == "call-request" {
		// De-duplicate: an old browser client sends call-request twice per call
		// (once synthesized from the group invite, once explicitly). Only fire
		// listeners the first time for a given channel.
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

		m.mu.Lock()
		_, alreadyPending := m.pendingCalls[env.Channel]
		_, alreadyActive := m.sessions[env.Channel]
		if alreadyPending || alreadyActive {
			m.mu.Unlock()
			log.Printf("CALL: duplicate call-request on channel %s — ignored", env.Channel)
			return
		}
		m.pendingCalls[env.Channel] = ic
		m.mu.Unlock()
		m.incomingMu.RLock()
		for ch := range m.incomingListeners {
			select {
			case ch <- ic:
			default:
				log.Printf("CALL: incoming listener channel full, dropping")
			}
		}
		m.incomingMu.RUnlock()
		return
	}

	// Route other signals (call-ack, offer, answer, ice-candidate, hangup) to existing session.
	m.mu.RLock()
	sess, ok := m.sessions[env.Channel]
	m.mu.RUnlock()
	if ok {
		sess.handleSignal(msgType, payload)
	}
}
