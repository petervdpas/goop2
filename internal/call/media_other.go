//go:build !linux

package call

import (
	"log"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

// initMediaPC creates a receive-only PeerConnection on non-Linux platforms.
// Camera/mic capture via pion/mediadevices requires platform-specific drivers
// (V4L2/malgo on Linux); on Windows/macOS the browser WebRTC path handles media.
// logFn is unused on non-Linux — no hardware capture is attempted here.
func initMediaPC(channelID string, _ func(level, msg string)) (*webrtc.PeerConnection, func(), error) {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, nil, err
	}

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

	// Add recvonly transceivers so SDP has valid m-lines with ICE credentials.
	addRecvOnlyTransceivers(channelID, pc)

	log.Printf("CALL [%s]: ExternalPC ready (receive-only, no local media on this platform)", channelID)
	return pc, nil, nil
}
