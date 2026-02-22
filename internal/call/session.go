package call

import (
	"log"
	"sync"
)

// Session represents one active call between two peers.
// Phase 1: stub implementation — no Pion PeerConnections yet.
// Phase 3 adds ExternalPC (real P2P call via Pion + mediadevices).
// Phase 4 adds LocalPC (localhost loopback so browser gets a real MediaStream).
type Session struct {
	channelID  string
	remotePeer string
	sig        Signaler

	mu      sync.Mutex
	audioOn bool
	videoOn bool
	hung    bool
}

func newSession(channelID, remotePeer string, sig Signaler) *Session {
	return &Session{
		channelID:  channelID,
		remotePeer: remotePeer,
		sig:        sig,
		audioOn:    true,
		videoOn:    true,
	}
}

// ToggleAudio flips local audio on/off. Returns the new muted state (true = muted).
func (s *Session) ToggleAudio() bool {
	s.mu.Lock()
	s.audioOn = !s.audioOn
	muted := !s.audioOn
	s.mu.Unlock()
	log.Printf("CALL [%s]: audio muted=%v", s.channelID, muted)
	return muted
}

// ToggleVideo flips local video on/off. Returns the new disabled state (true = disabled).
func (s *Session) ToggleVideo() bool {
	s.mu.Lock()
	s.videoOn = !s.videoOn
	disabled := !s.videoOn
	s.mu.Unlock()
	log.Printf("CALL [%s]: video disabled=%v", s.channelID, disabled)
	return disabled
}

// Hangup tears down this session and sends a hangup signal to the remote peer.
// Idempotent — safe to call multiple times.
func (s *Session) Hangup() {
	s.mu.Lock()
	if s.hung {
		s.mu.Unlock()
		return
	}
	s.hung = true
	s.mu.Unlock()

	_ = s.sig.Send(s.channelID, map[string]any{"type": "call-hangup"})
	log.Printf("CALL [%s]: hangup sent to %s", s.channelID, s.remotePeer)
}

// handleSignal processes an inbound signaling message from the remote peer.
// Phase 1: logs only.
// Phase 3+: routes offer/answer/ice-candidate to ExternalPC.
func (s *Session) handleSignal(msgType string, _ map[string]any) {
	log.Printf("CALL [%s]: signal %q from %s (Pion not wired yet — Phase 3)", s.channelID, msgType, s.remotePeer)
}
