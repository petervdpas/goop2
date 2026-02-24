package call

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
)

// Session represents one active call between two peers.
// Platform-specific media capture (camera/mic) is in media_linux.go /
// media_other.go via the initMediaPC() function they each provide.
//
// Phase 3: ExternalPC exchanges media with the remote peer over a standard
// WebRTC PeerConnection.  Local capture works on Linux (V4L2 + malgo);
// on other platforms the PC is receive-only until Phase 5 adds more drivers.
//
// Phase 4: Remote tracks are relayed to the browser via a WebM stream served
// over a WebSocket at /api/call/media/{channel}.  The browser's MSE API
// (Media Source Extensions) feeds the stream to a <video> element, giving us
// remote video without requiring RTCPeerConnection in the webview.
type Session struct {
	channelID  string
	remotePeer string
	sig        Signaler
	isCaller   bool // true = created by StartCall; false = created by AcceptCall

	mu         sync.Mutex
	audioOn    bool
	videoOn    bool
	hung       bool
	hangupCh   chan struct{}
	mediaClose func() // closes local media tracks; nil when no local media

	// ExternalPC is the Pion PeerConnection to the remote peer.
	externalPC *webrtc.PeerConnection

	// pcState tracks the most recent PeerConnectionState for /api/call/debug.
	pcState webrtc.PeerConnectionState

	// remoteDescSet is true once SetRemoteDescription has been called.
	// ICE candidates that arrive earlier are buffered in pendingICE.
	remoteDescSet bool
	pendingICE    []webrtc.ICECandidateInit

	// mediaReady is closed when initExternalPC completes (success or failure).
	// createAndSendOffer and handleOffer wait on it before touching the PC.
	mediaReady chan struct{}

	// webm manages the live WebM stream sent to the browser via WebSocket.
	// Browser uses MSE to display the received VP8/Opus stream.
	webm *webmSession
}

// SessionStatus is the snapshot returned by /api/call/debug.
type SessionStatus struct {
	ChannelID  string `json:"channel_id"`
	RemotePeer string `json:"remote_peer"`
	IsCaller   bool   `json:"is_caller"`
	PCState    string `json:"pc_state"`
	AudioOn    bool   `json:"audio_on"`
	VideoOn    bool   `json:"video_on"`
	Hung       bool   `json:"hung"`
}

// Status returns a snapshot of the session for the debug endpoint.
func (s *Session) Status() SessionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionStatus{
		ChannelID:  s.channelID,
		RemotePeer: s.remotePeer,
		IsCaller:   s.isCaller,
		PCState:    s.pcState.String(),
		AudioOn:    s.audioOn,
		VideoOn:    s.videoOn,
		Hung:       s.hung,
	}
}

// newSession creates a Session and kicks off background PC + media initialisation.
func newSession(channelID, remotePeer string, sig Signaler, isCaller bool) *Session {
	s := &Session{
		channelID:  channelID,
		remotePeer: remotePeer,
		sig:        sig,
		isCaller:   isCaller,
		audioOn:    true,
		videoOn:    true,
		hangupCh:   make(chan struct{}),
		mediaReady: make(chan struct{}),
		webm:       newWebmSession(),
	}
	go s.initExternalPC()

	// Callee-side watchdog: if no call-offer arrives within 10 s after the
	// session is created (i.e. after call-ack was sent), log a clear warning.
	// This fires only when something is wrong with the signaling chain so the
	// problem appears in the Video log tab without requiring Eggman's full
	// Go stdout to be captured.
	if !isCaller {
		go func() {
			select {
			case <-s.hangupCh:
				return // normal path — call ended before 10 s
			case <-time.After(10 * time.Second):
				// Check if the PC has made any progress (received an offer → pcState set)
				s.mu.Lock()
				started := s.remoteDescSet
				s.mu.Unlock()
				if !started {
					log.Printf("CALL [%s]: WARNING — no call-offer received from %s within 10 s; check caller signaling", channelID, remotePeer)
				}
			}
		}()
	}

	return s
}

// SubscribeMedia returns a channel that receives binary WebM messages
// (init segment first, then clusters) for Phase 4 browser display via MSE.
// The caller must invoke the returned cancel function when done.
func (s *Session) SubscribeMedia() (<-chan []byte, func()) {
	return s.webm.subscribeMedia()
}

// HangupCh returns a channel closed when the call ends (either peer hung up).
// The /api/call/session/{channel}/events SSE selects on this.
func (s *Session) HangupCh() <-chan struct{} { return s.hangupCh }

// ToggleAudio flips local audio on/off. Returns new muted state (true = muted).
func (s *Session) ToggleAudio() bool {
	s.mu.Lock()
	s.audioOn = !s.audioOn
	muted := !s.audioOn
	s.mu.Unlock()
	log.Printf("CALL [%s]: audio muted=%v", s.channelID, muted)
	// TODO Phase 5: mute the Pion audio track in-place
	return muted
}

// ToggleVideo flips local video on/off. Returns new disabled state (true = disabled).
func (s *Session) ToggleVideo() bool {
	s.mu.Lock()
	s.videoOn = !s.videoOn
	disabled := !s.videoOn
	s.mu.Unlock()
	log.Printf("CALL [%s]: video disabled=%v", s.channelID, disabled)
	// TODO Phase 5: disable the Pion video track in-place
	return disabled
}

// Hangup tears down this session and signals the remote peer. Idempotent.
func (s *Session) Hangup() {
	s.mu.Lock()
	if s.hung {
		s.mu.Unlock()
		return
	}
	s.hung = true
	close(s.hangupCh)
	s.mu.Unlock()

	s.cleanup()
	_ = s.sig.Send(s.channelID, map[string]any{"type": "call-hangup"})
	log.Printf("CALL [%s]: hangup sent to %s", s.channelID, s.remotePeer)
}

// cleanup closes ExternalPC and releases local media tracks.
func (s *Session) cleanup() {
	s.mu.Lock()
	pc := s.externalPC
	closeFn := s.mediaClose
	s.externalPC = nil
	s.mediaClose = nil
	s.mu.Unlock()

	if closeFn != nil {
		closeFn()
	}
	if pc != nil {
		_ = pc.Close()
	}
}

// ── ExternalPC initialisation ──────────────────────────────────────────────────

// initExternalPC builds the Pion PeerConnection using the platform-specific
// initMediaPC() function (media_linux.go / media_other.go), wires up common
// callbacks, and closes s.mediaReady when done.
func (s *Session) initExternalPC() {
	defer close(s.mediaReady)

	pc, closeFn, err := initMediaPC(s.channelID)
	if err != nil {
		log.Printf("CALL [%s]: PeerConnection create error: %v", s.channelID, err)
		return
	}

	s.mu.Lock()
	s.externalPC = pc
	s.mediaClose = closeFn
	s.mu.Unlock()

	// ── PC callbacks ─────────────────────────────────────────────────────────

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // gathering complete
		}
		init := c.ToJSON()
		sdpMid := ""
		if init.SDPMid != nil {
			sdpMid = *init.SDPMid
		}
		sdpMLineIndex := uint16(0)
		if init.SDPMLineIndex != nil {
			sdpMLineIndex = *init.SDPMLineIndex
		}
		_ = s.sig.Send(s.channelID, map[string]any{
			"type": "ice-candidate",
			"candidate": map[string]any{
				"candidate":     init.Candidate,
				"sdpMid":        sdpMid,
				"sdpMLineIndex": sdpMLineIndex,
			},
		})
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.mu.Lock()
		s.pcState = state
		s.mu.Unlock()
		log.Printf("CALL [%s]: PC state → %s", s.channelID, state)
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateDisconnected {
			s.Hangup()
		}
	})

	// Phase 4: stream remote tracks to the browser via WebM/MSE.
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("CALL [%s]: remote track — kind=%s codec=%s ssrc=%d",
			s.channelID, track.Kind(), track.Codec().MimeType, track.SSRC())
		switch track.Kind() {
		case webrtc.RTPCodecTypeVideo:
			go s.streamVideoTrack(track)
		case webrtc.RTPCodecTypeAudio:
			s.webm.enableAudio()
			go s.streamAudioTrack(track)
		}
	})
}

// streamVideoTrack depacketizes incoming VP8 RTP packets, assembles frames,
// and feeds them to the webmSession for Phase 4 browser display via MSE.
// ReadRTP blocks until a packet arrives or the PC closes (error → return).
func (s *Session) streamVideoTrack(track *webrtc.TrackRemote) {
	codec := track.Codec().MimeType
	log.Printf("CALL [%s]: video streaming started (%s)", s.channelID, codec)

	// Send PLI (Picture Loss Indication) to request a keyframe from the remote
	// VP8 encoder.  Without this the browser may wait 3–10 s for the encoder's
	// natural keyframe interval before the WebM init segment can be generated.
	sendPLI := func() {
		s.mu.Lock()
		pc := s.externalPC
		s.mu.Unlock()
		if pc != nil {
			_ = pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
				MediaSSRC: uint32(track.SSRC()),
			}})
		}
	}
	sendPLI()
	log.Printf("CALL [%s]: PLI sent — requesting initial VP8 keyframe", s.channelID)

	// Keep requesting keyframes every 2 s until the WebM init segment is ready,
	// then stop.  Bounded to 10 s to avoid hammering a stalled remote encoder.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		deadline := time.NewTimer(10 * time.Second)
		defer ticker.Stop()
		defer deadline.Stop()
		for {
			select {
			case <-s.hangupCh:
				return
			case <-deadline.C:
				return
			case <-ticker.C:
				if s.webm.hasInitSeg() {
					return
				}
				sendPLI()
				log.Printf("CALL [%s]: PLI retry — still waiting for VP8 keyframe", s.channelID)
			}
		}
	}()

	var depack codecs.VP8Packet
	var frameAccum []byte
	var pktCount atomic.Int64

	// Log stats periodically; exit on hangup.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.hangupCh:
				log.Printf("CALL [%s]: remote video (%s) done — %d packets total", s.channelID, codec, pktCount.Load())
				return
			case <-ticker.C:
				log.Printf("CALL [%s]: ← remote video (%s) — %d packets", s.channelID, codec, pktCount.Load())
			}
		}
	}()

	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		pktCount.Add(1)

		payload, err := depack.Unmarshal(pkt.Payload)
		if err != nil || len(payload) == 0 {
			continue
		}
		frameAccum = append(frameAccum, payload...)

		if pkt.Marker {
			// Complete VP8 frame — feed to WebM session.
			// VP8 RTP clock is 90 kHz; convert to milliseconds.
			tsMs := int64(pkt.Timestamp) / 90
			keyframe := len(frameAccum) > 0 && (frameAccum[0]&0x01) == 0
			frame := make([]byte, len(frameAccum))
			copy(frame, frameAccum)
			s.webm.handleVideoFrame(tsMs, keyframe, frame)
			frameAccum = frameAccum[:0]
		}
	}
}

// streamAudioTrack reads incoming Opus RTP packets and feeds them to the
// webmSession for Phase 4 browser audio via MSE.
// ReadRTP blocks until a packet arrives or the PC closes (error → return).
func (s *Session) streamAudioTrack(track *webrtc.TrackRemote) {
	codec := track.Codec().MimeType
	log.Printf("CALL [%s]: audio streaming started (%s)", s.channelID, codec)

	var pktCount atomic.Int64

	// Log stats periodically; exit on hangup.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.hangupCh:
				log.Printf("CALL [%s]: remote audio (%s) done — %d packets total", s.channelID, codec, pktCount.Load())
				return
			case <-ticker.C:
				log.Printf("CALL [%s]: ← remote audio (%s) — %d packets", s.channelID, codec, pktCount.Load())
			}
		}
	}()

	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		pktCount.Add(1)

		if len(pkt.Payload) == 0 {
			continue
		}
		// Opus RTP clock is 48 kHz; convert to milliseconds.
		tsMs := int64(pkt.Timestamp) / 48
		// Opus RTP payload is the raw Opus frame — no header to strip.
		frame := make([]byte, len(pkt.Payload))
		copy(frame, pkt.Payload)
		s.webm.handleAudioFrame(tsMs, frame)
	}
}

// ── Signal handlers ────────────────────────────────────────────────────────────

// handleSignal processes an inbound signaling message from the remote peer.
func (s *Session) handleSignal(msgType string, payload map[string]any) {
	switch msgType {
	case "call-ack":
		// Callee accepted → caller creates and sends offer.
		if !s.isCaller {
			log.Printf("CALL [%s]: unexpected call-ack on callee side", s.channelID)
			return
		}
		log.Printf("CALL [%s]: call-ack received — starting SDP offer", s.channelID)
		go s.createAndSendOffer()

	case "call-offer":
		// Caller sent offer → callee creates and sends answer.
		if s.isCaller {
			log.Printf("CALL [%s]: unexpected call-offer on caller side", s.channelID)
			return
		}
		sdp, _ := payload["sdp"].(string)
		if sdp == "" {
			log.Printf("CALL [%s]: call-offer missing SDP", s.channelID)
			return
		}
		log.Printf("CALL [%s]: call-offer received — handling SDP", s.channelID)
		go s.handleOffer(sdp)

	case "call-answer":
		// Callee sent answer → caller sets remote description.
		if !s.isCaller {
			log.Printf("CALL [%s]: unexpected call-answer on callee side", s.channelID)
			return
		}
		sdp, _ := payload["sdp"].(string)
		if sdp == "" {
			log.Printf("CALL [%s]: call-answer missing SDP", s.channelID)
			return
		}
		log.Printf("CALL [%s]: call-answer received — setting remote description", s.channelID)
		go s.handleAnswer(sdp)

	case "ice-candidate":
		raw, _ := payload["candidate"].(map[string]any)
		if raw == nil {
			return
		}
		candidate, _ := raw["candidate"].(string)
		sdpMid, _ := raw["sdpMid"].(string)
		idxFloat, _ := raw["sdpMLineIndex"].(float64)
		idx := uint16(idxFloat)
		s.addICECandidate(webrtc.ICECandidateInit{
			Candidate:     candidate,
			SDPMid:        &sdpMid,
			SDPMLineIndex: &idx,
		})

	case "call-hangup":
		s.mu.Lock()
		alreadyHung := s.hung
		if !s.hung {
			s.hung = true
			close(s.hangupCh)
		}
		s.mu.Unlock()
		if !alreadyHung {
			s.cleanup()
			log.Printf("CALL [%s]: remote hangup from %s", s.channelID, s.remotePeer)
		}

	default:
		log.Printf("CALL [%s]: unknown signal %q from %s", s.channelID, msgType, s.remotePeer)
	}
}

// createAndSendOffer waits for media to be ready, then negotiates as the caller.
func (s *Session) createAndSendOffer() {
	log.Printf("CALL [%s]: createAndSendOffer: waiting for media to be ready", s.channelID)
	<-s.mediaReady

	s.mu.Lock()
	pc := s.externalPC
	s.mu.Unlock()
	if pc == nil {
		log.Printf("CALL [%s]: createAndSendOffer: no PC available", s.channelID)
		return
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("CALL [%s]: CreateOffer error: %v", s.channelID, err)
		return
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		log.Printf("CALL [%s]: SetLocalDescription(offer) error: %v", s.channelID, err)
		return
	}
	_ = s.sig.Send(s.channelID, map[string]any{
		"type": "call-offer",
		"sdp":  offer.SDP,
	})
	log.Printf("CALL [%s]: offer sent to %s", s.channelID, s.remotePeer)
}

// handleOffer waits for media, sets the remote offer, and sends back an answer.
func (s *Session) handleOffer(sdp string) {
	log.Printf("CALL [%s]: handleOffer: waiting for media to be ready", s.channelID)
	<-s.mediaReady

	s.mu.Lock()
	pc := s.externalPC
	s.mu.Unlock()
	if pc == nil {
		log.Printf("CALL [%s]: handleOffer: no PC available (media init failed)", s.channelID)
		return
	}

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer, SDP: sdp,
	}); err != nil {
		log.Printf("CALL [%s]: SetRemoteDescription(offer) error: %v", s.channelID, err)
		return
	}
	s.flushPendingICE(pc)

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("CALL [%s]: CreateAnswer error: %v", s.channelID, err)
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Printf("CALL [%s]: SetLocalDescription(answer) error: %v", s.channelID, err)
		return
	}
	_ = s.sig.Send(s.channelID, map[string]any{
		"type": "call-answer",
		"sdp":  answer.SDP,
	})
	log.Printf("CALL [%s]: answer sent to %s", s.channelID, s.remotePeer)
}

// handleAnswer sets the remote description from the callee's SDP answer.
func (s *Session) handleAnswer(sdp string) {
	s.mu.Lock()
	pc := s.externalPC
	s.mu.Unlock()
	if pc == nil {
		log.Printf("CALL [%s]: handleAnswer: no PC available", s.channelID)
		return
	}

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer, SDP: sdp,
	}); err != nil {
		log.Printf("CALL [%s]: SetRemoteDescription(answer) error: %v", s.channelID, err)
		return
	}
	s.flushPendingICE(pc)
	log.Printf("CALL [%s]: remote answer set — ICE connecting to %s", s.channelID, s.remotePeer)
}

// flushPendingICE marks remote desc as set and drains any buffered ICE candidates.
func (s *Session) flushPendingICE(pc *webrtc.PeerConnection) {
	s.mu.Lock()
	s.remoteDescSet = true
	pending := s.pendingICE
	s.pendingICE = nil
	s.mu.Unlock()

	for _, c := range pending {
		if err := pc.AddICECandidate(c); err != nil {
			log.Printf("CALL [%s]: AddICECandidate (buffered) error: %v", s.channelID, err)
		}
	}
}

// addICECandidate adds a remote ICE candidate, buffering it if remote desc isn't set yet.
func (s *Session) addICECandidate(init webrtc.ICECandidateInit) {
	s.mu.Lock()
	pc := s.externalPC
	ready := s.remoteDescSet
	if !ready {
		s.pendingICE = append(s.pendingICE, init)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if pc == nil {
		return
	}
	if err := pc.AddICECandidate(init); err != nil {
		log.Printf("CALL [%s]: AddICECandidate error: %v", s.channelID, err)
	}
}
