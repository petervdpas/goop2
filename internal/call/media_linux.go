//go:build linux

package call

import (
	"log"

	"github.com/pion/interceptor"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/webrtc/v4"
)

// initMediaPC creates the ExternalPC with VP8+Opus codecs and attempts to
// capture local camera/mic via pion/mediadevices (V4L2 + malgo on Linux).
// Returns the PC, a cleanup func for local media (may be nil), and any error.
// logFn, if non-nil, is called with (level, msg) for hardware errors that
// should appear in the browser's Video log tab via MQ. May be nil.
func initMediaPC(channelID string, logFn func(level, msg string)) (*webrtc.PeerConnection, func(), error) {
	// ── Codec selector ───────────────────────────────────────────────────────

	vpxParams, err := vpx.NewVP8Params()
	if err != nil {
		return nil, nil, err
	}
	vpxParams.BitRate = 1_500_000 // 1.5 Mbps

	opusParams, err := opus.NewParams()
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
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
		return nil, nil, err
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
			constraints.Video = func(_ *mediadevices.MediaTrackConstraints) {}
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

		for _, track := range stream.GetTracks() {
			track.OnEnded(func(err error) {
				if err != nil {
					log.Printf("CALL [%s]: local track ended: %v", channelID, err)
				}
			})
			if _, err := pc.AddTrack(track); err != nil {
				log.Printf("CALL [%s]: AddTrack error: %v", channelID, err)
			}
		}

		log.Printf("CALL [%s]: local media captured (%s) — %d tracks", channelID, a.label, len(stream.GetTracks()))
		closeFn := func() {
			for _, t := range stream.GetTracks() {
				t.Close()
			}
		}
		return pc, closeFn, nil
	}

	// All attempts failed — fall back to receive-only so the call can still
	// receive remote media even without local camera/mic.
	msg := "all media capture attempts failed — proceeding receive-only"
	log.Printf("CALL [%s]: %s", channelID, msg)
	if logFn != nil {
		logFn("warn", msg)
	}
	addRecvOnlyTransceivers(channelID, pc)
	return pc, nil, nil
}
