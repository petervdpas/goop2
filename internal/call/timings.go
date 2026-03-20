package call

import "time"

// Native call stack (Pion WebRTC) timings.
const (
	ICEGatherTimeout       = 5 * time.Second   // wait for ICE candidate gathering
	ICEDisconnectedTimeout = 30 * time.Second  // ICE disconnected before failure (relay recovery window)
	ICEFailedTimeout       = 120 * time.Second // ICE failed — give up after this
	ICECheckInterval       = 2 * time.Second   // ICE connectivity check interval
	AudioCheckInterval     = 2 * time.Second   // poll audio track liveness
	VideoCheckInterval     = 5 * time.Second   // poll video track liveness
	SelfViewInterval       = 5 * time.Second   // self-view frame check
)
