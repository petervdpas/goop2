package listen

import "time"

// Listen group type timings.
const (
	StreamPollInterval  = 500 * time.Millisecond // pause/stop check during audio streaming
	ListenJoinTimeout   = 15 * time.Second       // join/rejoin remote listen group
	ListenStreamTimeout = 15 * time.Second       // open audio stream to host
)
