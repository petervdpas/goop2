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
	pendingCalls map[string]struct{} // channels where call-request fired but not yet accepted/rejected

	done chan struct{}
}

// New creates a new call Manager attached to sig and starts listening for
// signaling messages immediately.
func New(sig Signaler, selfID string) *Manager {
	m := &Manager{
		sig:          sig,
		selfID:       selfID,
		sessions:     make(map[string]*Session),
		pendingCalls: make(map[string]struct{}),
		done:         make(chan struct{}),
	}
	go m.dispatchLoop()
	return m
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
	// If the channel is already closed (caller hung up before we accepted), log
	// the error so it's visible in the Logs → Video tab, then clean up immediately.
	if err := m.sig.Send(channelID, map[string]any{"type": "call-ack"}); err != nil {
		log.Printf("CALL: accepted %s but call-ack send failed (%v) — channel likely closed, aborting", channelID, err)
		m.removeSession(channelID)
		sess.Hangup()
		return nil, err
	}
	log.Printf("CALL: accepted %s from %s — call-ack sent, waiting for SDP offer", channelID, remotePeer)
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
	m.pendingCalls = make(map[string]struct{})
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

// dispatch routes one signaling envelope to the appropriate session.
// call-request is de-duplicated and tracked via pendingCalls; the browser
// receives it directly from the MQ SSE stream via Goop.mq.subscribe('call:*').
func (m *Manager) dispatch(env *Envelope) {
	payload, ok := env.Payload.(map[string]any)
	if !ok {
		return
	}
	msgType, _ := payload["type"].(string)

	if msgType == "call-request" {
		// De-duplicate: only register the first call-request for a given channel.
		m.mu.Lock()
		_, alreadyPending := m.pendingCalls[env.Channel]
		_, alreadyActive := m.sessions[env.Channel]
		if alreadyPending || alreadyActive {
			m.mu.Unlock()
			log.Printf("CALL: duplicate call-request on channel %s — ignored", env.Channel)
			return
		}
		m.pendingCalls[env.Channel] = struct{}{}
		m.mu.Unlock()
		log.Printf("CALL: incoming call-request on channel %s from %s", env.Channel, env.From)
		return
	}

	// Route other signals (call-ack, offer, answer, ice-candidate, hangup) to existing session.
	m.mu.RLock()
	sess, ok := m.sessions[env.Channel]
	m.mu.RUnlock()
	if !ok {
		// Only warn for meaningful signal types — ice-candidate noise would flood logs.
		if msgType == "call-ack" || msgType == "call-offer" || msgType == "call-answer" {
			log.Printf("CALL: received %q on %s but no session found — caller may not have called /api/call/start", msgType, env.Channel)
		}
		return
	}
	sess.handleSignal(msgType, payload)
}
