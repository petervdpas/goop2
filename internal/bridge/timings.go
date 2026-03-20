package bridge

import "time"

// Bridge client timings — WebSocket connection to the bridge server.
const (
	HTTPClientTimeout  = 3 * time.Second       // REST calls to bridge server
	WSHandshakeTimeout = 3 * time.Second       // WS upgrade handshake
	WSReadDeadline     = 45 * time.Second      // must be > 2× WSPingInterval
	WSWriteDeadline    = 3 * time.Second       // writing a WS frame
	WSPingInterval     = 15 * time.Second      // keepalive ping frequency
	WSReconnectBackoff    = 500 * time.Millisecond // initial backoff on WS disconnect
	WSReconnectMaxBackoff = 5 * time.Second        // max backoff cap on WS disconnect
)
