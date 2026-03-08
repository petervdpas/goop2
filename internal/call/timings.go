package call

import "time"

const (
	ICEGatherTimeout    = 10 * time.Second
	AudioCheckInterval  = 2 * time.Second
	VideoCheckInterval  = 5 * time.Second
	SelfViewInterval    = 5 * time.Second
)
