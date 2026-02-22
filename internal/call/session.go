package call

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/webrtc/v4"
)

// Session represents one active call between two peers.
// Phase 3: ExternalPC captures camera/mic via pion/mediadevices (V4L2 + malgo) and
// exchanges media with the remote peer over a standard WebRTC PeerConnection.
// Phase 4 will add LocalPC — a localhost loopback connection so the browser webview
// receives a real MediaStream without needing getUserMedia.
type Session struct {
	channelID  string
	remotePeer string
	sig        Signaler
	isCaller   bool // true = created by StartCall; false = created by AcceptCall

	mu      sync.Mutex
	audioOn bool
	videoOn bool
	hung    bool
	hangupCh chan struct{}

	// ExternalPC is the Pion PeerConnection to the remote peer.
	externalPC  *webrtc.PeerConnection
	localStream mediadevices.MediaStream

	// pcState tracks the most recent PeerConnectionState for /api/call/debug.
	pcState webrtc.PeerConnectionState

	// remoteDescSet is true once SetRemoteDescription has been called.
	// ICE candidates that arrive earlier are buffered in pendingICE.
	remoteDescSet bool
	pendingICE    []webrtc.ICECandidateInit

	// mediaReady is closed when initExternalPC completes (success or failure).
	// createAndSendOffer and handleOffer wait on it before touching the PC.
	mediaReady chan struct{}
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
	}
	go s.initExternalPC()
	return s
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

// cleanup closes ExternalPC and releases the local media stream.
func (s *Session) cleanup() {
	s.mu.Lock()
	pc := s.externalPC
	stream := s.localStream
	s.externalPC = nil
	s.localStream = nil
	s.mu.Unlock()

	if stream != nil {
		for _, t := range stream.GetTracks() {
			t.Close()
		}
	}
	if pc != nil {
		_ = pc.Close()
	}
}

// ── ExternalPC initialisation ──────────────────────────────────────────────────

// initExternalPC builds the Pion PeerConnection with VP8+Opus codecs, captures
// local camera and mic via pion/mediadevices, and adds the tracks to the PC.
// Called in a goroutine from newSession; closes s.mediaReady when done.
func (s *Session) initExternalPC() {
	defer close(s.mediaReady)

	// ── Codec selector ───────────────────────────────────────────────────────

	vpxParams, err := vpx.NewVP8Params()
	if err != nil {
		log.Printf("CALL [%s]: VP8 params error: %v", s.channelID, err)
		return
	}
	vpxParams.BitRate = 1_500_000 // 1.5 Mbps

	opusParams, err := opus.NewParams()
	if err != nil {
		log.Printf("CALL [%s]: Opus params error: %v", s.channelID, err)
		return
	}

	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&vpxParams),
		mediadevices.WithAudioEncoders(&opusParams),
	)

	// ── WebRTC API ───────────────────────────────────────────────────────────

	mediaEngine := &webrtc.MediaEngine{}
	codecSelector.Populate(mediaEngine)

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		log.Printf("CALL [%s]: interceptor register error: %v", s.channelID, err)
		return
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
	)

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		log.Printf("CALL [%s]: PeerConnection create error: %v", s.channelID, err)
		return
	}

	s.mu.Lock()
	s.externalPC = pc
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

	// Phase 4: OnTrack will also relay remote tracks to LocalPC.
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("CALL [%s]: remote track — kind=%s codec=%s ssrc=%d",
			s.channelID, track.Kind(), track.Codec().MimeType, track.SSRC())
		go s.drainRemoteTrack(track)
	})

	// ── Capture local media ──────────────────────────────────────────────────

	stream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(_ *mediadevices.MediaTrackConstraints) {},
		Audio: func(_ *mediadevices.MediaTrackConstraints) {},
		Codec: codecSelector,
	})
	if err != nil {
		// Non-fatal: call can still receive remote media; we just won't send any.
		log.Printf("CALL [%s]: GetUserMedia error: %v — proceeding without local media", s.channelID, err)
		// Add recvonly transceivers so CreateOffer/CreateAnswer still produces
		// valid m-lines with ICE credentials. Without at least one m-line, the
		// remote peer's SetRemoteDescription fails with "no ice-ufrag".
		if _, terr := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); terr != nil {
			log.Printf("CALL [%s]: AddTransceiver(video) error: %v", s.channelID, terr)
		}
		if _, terr := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); terr != nil {
			log.Printf("CALL [%s]: AddTransceiver(audio) error: %v", s.channelID, terr)
		}
		return
	}

	s.mu.Lock()
	s.localStream = stream
	s.mu.Unlock()

	for _, track := range stream.GetTracks() {
		track.OnEnded(func(err error) {
			if err != nil {
				log.Printf("CALL [%s]: local track ended: %v", s.channelID, err)
			}
		})
		if _, err := pc.AddTrack(track); err != nil {
			log.Printf("CALL [%s]: AddTrack error: %v", s.channelID, err)
		}
	}

	log.Printf("CALL [%s]: ExternalPC ready — %d local tracks, awaiting signal",
		s.channelID, len(stream.GetTracks()))
}

// drainRemoteTrack reads RTP data from a remote track and logs packet stats every
// 5 seconds so you can confirm media is flowing without needing a UI.
func (s *Session) drainRemoteTrack(track *webrtc.TrackRemote) {
	var count atomic.Uint64

	// Ticker: log stats every 5 s until the call ends.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.hangupCh:
				log.Printf("CALL [%s]: remote %s track done — %d packets total",
					s.channelID, track.Kind(), count.Load())
				return
			case <-ticker.C:
				log.Printf("CALL [%s]: ← remote %s (%s) — %d packets",
					s.channelID, track.Kind(), track.Codec().MimeType, count.Load())
			}
		}
	}()

	buf := make([]byte, 1500)
	for {
		if _, _, err := track.Read(buf); err != nil {
			return
		}
		count.Add(1)
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
	<-s.mediaReady

	s.mu.Lock()
	pc := s.externalPC
	s.mu.Unlock()
	if pc == nil {
		log.Printf("CALL [%s]: handleOffer: no PC available", s.channelID)
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
