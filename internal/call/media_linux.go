//go:build linux

package call

import (
	"log"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v4"
)

// vp8SelfView wraps a mediadevices VP8 EncodedReadCloser as a SelfViewSource.
type vp8SelfView struct{ r mediadevices.EncodedReadCloser }

func (s *vp8SelfView) ReadFrame() ([]byte, func(), error) {
	buf, rel, err := s.r.Read()
	if err != nil {
		return nil, nil, err
	}
	data := make([]byte, len(buf.Data))
	copy(data, buf.Data)
	return data, rel, nil
}

func (s *vp8SelfView) Close() error { return s.r.Close() }

// initMediaPC creates the ExternalPC with VP8+Opus codecs and attempts to
// capture local camera/mic via pion/mediadevices (V4L2 + malgo on Linux).
// Returns the PC, a cleanup func for local media (may be nil), a SelfViewSource
// for browser self-preview (non-nil when video capture succeeded), and any error.
// logFn, if non-nil, is called with (level, msg) for hardware errors that
// should appear in the browser's Video log tab via MQ. May be nil.
func initMediaPC(channelID string, logFn func(level, msg string)) (*webrtc.PeerConnection, func(), SelfViewSource, error) {
	// ── Codec selector ───────────────────────────────────────────────────────

	vpxParams, err := vpx.NewVP8Params()
	if err != nil {
		return nil, nil, nil, err
	}
	vpxParams.BitRate = 1_500_000 // 1.5 Mbps

	opusParams, err := opus.NewParams()
	if err != nil {
		return nil, nil, nil, err
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
		return nil, nil, nil, err
	}

	// Use generous ICE timeouts so a brief relay/NAT hiccup does not immediately
	// terminate the call.  The default disconnectedTimeout is 5 s — far too short
	// for relay paths that can have short outages during re-keying or failover.
	// 30 s gives ICE time to recover without the user noticing a freeze.
	se := webrtc.SettingEngine{}
	se.SetICETimeouts(30*time.Second, 120*time.Second, 2*time.Second)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
		webrtc.WithSettingEngine(se),
	)

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// ── Enumerate available media devices (diagnostics) ──────────────────────

	devices := mediadevices.EnumerateDevices()
	if len(devices) == 0 {
		msg := "no media devices found by pion/mediadevices"
		log.Printf("CALL [%s]: %s", channelID, msg)
		if logFn != nil {
			logFn("warn", msg)
		}
	} else {
		for _, d := range devices {
			log.Printf("CALL [%s]: media device — kind=%v label=%q", channelID, d.Kind, d.Label)
		}
	}

	// ── Capture local media with graceful fallback ───────────────────────────
	//
	// GetUserMedia fails as a unit if either track (video OR audio) can't be
	// opened.  Try video+audio first, then video-only, then audio-only so that
	// a missing/busy microphone doesn't prevent the camera from working and
	// vice versa.

	type attempt struct {
		video bool
		audio bool
		label string
	}
	for _, a := range []attempt{
		{true, true, "video+audio"},
		{true, false, "video-only"},
		{false, true, "audio-only"},
	} {
		constraints := mediadevices.MediaStreamConstraints{Codec: codecSelector}
		if a.video {
			constraints.Video = func(c *mediadevices.MediaTrackConstraints) {
				// Exclude MJPEG — some cameras expose an MJPEG V4L2 node that
				// produces malformed JPEG frames, which poisons the VP8 encoder
				// and causes SetRemoteDescription to fail.  Raw formats only.
				c.FrameFormat = prop.FrameFormatOneOf{
					frame.FormatYUYV,
					frame.FormatI420,
					frame.FormatI444,
					frame.FormatRGBA,
				}
				// Cap at 640×480 — higher resolutions increase VP8 encoding
				// latency and can cause WebKitGTK MSE to stall on large frames.
				c.Width = prop.IntRanged{Max: 640}
				c.Height = prop.IntRanged{Max: 480}
			}
		}
		if a.audio {
			constraints.Audio = func(_ *mediadevices.MediaTrackConstraints) {}
		}

		stream, err := mediadevices.GetUserMedia(constraints)
		if err != nil {
			msg := "GetUserMedia (" + a.label + ") failed: " + err.Error()
			log.Printf("CALL [%s]: %s", channelID, msg)
			if logFn != nil {
				logFn("warn", msg)
			}
			continue
		}

		tracks := stream.GetTracks()
		var selfSrc SelfViewSource
		brokenVideo := false
		for _, track := range tracks {
			track.OnEnded(func(err error) {
				if err != nil {
					log.Printf("CALL [%s]: local track ended: %v", channelID, err)
				}
			})
			if _, err := pc.AddTrack(track); err != nil {
				log.Printf("CALL [%s]: AddTrack error: %v", channelID, err)
			}
			// Create an independent VP8 reader for browser self-view.
			// pion/mediadevices broadcasts raw frames to multiple consumers;
			// this encoder runs in parallel to the one Pion uses for RTP.
			if track.Kind() == webrtc.RTPCodecTypeVideo {
				r, err := track.NewEncodedReader(webrtc.MimeTypeVP8)
				if err == nil {
					selfSrc = &vp8SelfView{r: r}
					log.Printf("CALL [%s]: self-view VP8 reader ready", channelID)
				} else {
					// Broken video encoder (e.g. malformed MJPEG from camera).
					// Close all tracks and fall through to the next attempt —
					// a poisoned VP8 encoder would cause SetRemoteDescription to
					// fail and break SDP negotiation entirely.
					msg := "video track broken, skipping attempt (" + a.label + "): " + err.Error()
					log.Printf("CALL [%s]: %s", channelID, msg)
					if logFn != nil {
						logFn("warn", msg)
					}
					brokenVideo = true
				}
			}
		}
		if brokenVideo {
			for _, t := range tracks {
				t.Close()
			}
			continue
		}

		log.Printf("CALL [%s]: local media captured (%s) — %d tracks", channelID, a.label, len(tracks))
		closeFn := func() {
			for _, t := range tracks {
				t.Close()
			}
		}
		return pc, closeFn, selfSrc, nil
	}

	// All attempts failed — fall back to receive-only so the call can still
	// receive remote media even without local camera/mic.
	msg := "all media capture attempts failed — proceeding receive-only"
	log.Printf("CALL [%s]: %s", channelID, msg)
	if logFn != nil {
		logFn("warn", msg)
	}
	addRecvOnlyTransceivers(channelID, pc)
	return pc, nil, nil, nil
}
