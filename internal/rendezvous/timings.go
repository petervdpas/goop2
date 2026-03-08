package rendezvous

import "time"

const (
	HTTPClientTimeout    = 2 * time.Second
	WSHandshakeTimeout   = 2 * time.Second
	WSWriteDeadline      = 2 * time.Second
	WSReadDeadline       = 15 * time.Second
	WSPingInterval       = 5 * time.Second
	SSEReconnectBackoff  = 3 * time.Second
	WSReconnectBackoff   = 3 * time.Second
	WSProbeTimeout       = 2 * time.Second
	WSProbeFirstInterval = 10 * time.Second
	WSProbeNextInterval  = 25 * time.Second
)
