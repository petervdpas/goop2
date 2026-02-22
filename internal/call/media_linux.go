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
func initMediaPC(channelID string) (*webrtc.PeerConnection, func(), error) {
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

	// ── Capture local media ──────────────────────────────────────────────────

	stream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(_ *mediadevices.MediaTrackConstraints) {},
		Audio: func(_ *mediadevices.MediaTrackConstraints) {},
		Codec: codecSelector,
	})
	if err != nil {
		// Non-fatal: add recvonly transceivers so the SDP offer still has valid
		// m-lines with ICE credentials; call can still receive remote media.
		log.Printf("CALL [%s]: GetUserMedia error: %v — proceeding without local media", channelID, err)
		addRecvOnlyTransceivers(channelID, pc)
		return pc, nil, nil
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

	log.Printf("CALL [%s]: ExternalPC ready — %d local tracks", channelID, len(stream.GetTracks()))

	closeFn := func() {
		for _, t := range stream.GetTracks() {
			t.Close()
		}
	}
	return pc, closeFn, nil
}

