package call

import (
	"log"

	"github.com/pion/webrtc/v4"
)

// SelfViewSource provides encoded VP8 frames of the local camera for
// self-view display in the browser.  Only non-nil on Linux when camera
// capture succeeded.  ReadFrame blocks until the next frame is ready.
// Close must be called when the session ends.
type SelfViewSource interface {
	ReadFrame() (data []byte, release func(), err error)
	Close() error
}

// addRecvOnlyTransceivers adds recvonly transceivers for video and audio so
// CreateOffer/CreateAnswer always produces valid m-lines with ICE credentials.
func addRecvOnlyTransceivers(channelID string, pc *webrtc.PeerConnection) {
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		log.Printf("CALL [%s]: AddTransceiver(video) error: %v", channelID, err)
	}
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		log.Printf("CALL [%s]: AddTransceiver(audio) error: %v", channelID, err)
	}
}
