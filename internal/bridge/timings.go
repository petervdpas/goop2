package bridge

import "time"

const (
	HTTPClientTimeout    = 10 * time.Second
	WSHandshakeTimeout   = 10 * time.Second
	WSReadDeadline       = 45 * time.Second
	WSWriteDeadline      = 10 * time.Second
	WSPingInterval       = 15 * time.Second
	WSReconnectBackoff   = 500 * time.Millisecond
)
