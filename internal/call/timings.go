package call

import "time"

// Native call stack (Pion WebRTC) timings.
const (
	ICEGatherTimeout   = 5 * time.Second // wait for ICE candidate gathering
	AudioCheckInterval = 2 * time.Second // poll audio track liveness
	VideoCheckInterval = 5 * time.Second // poll video track liveness
	SelfViewInterval   = 5 * time.Second // self-view frame check
)
