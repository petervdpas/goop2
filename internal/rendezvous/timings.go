package rendezvous

import "time"

// Client-side rendezvous timings — connecting to rendezvous server(s).
const (
	HTTPClientTimeout    = 2 * time.Second  // REST calls to rendezvous
	WSHandshakeTimeout   = 2 * time.Second  // WebSocket upgrade handshake
	WSWriteDeadline      = 2 * time.Second  // writing a WS frame
	WSReadDeadline       = 15 * time.Second // must be > 2× WSPingInterval
	WSPingInterval       = 5 * time.Second  // WS keepalive ping
	SSEReconnectBackoff  = 3 * time.Second  // SSE reconnect delay
	WSReconnectBackoff   = 3 * time.Second  // WS reconnect delay
	WSProbeTimeout       = 2 * time.Second  // WS health probe
	WSProbeFirstInterval = 10 * time.Second // first probe after connect
	WSProbeNextInterval  = 25 * time.Second // subsequent probe interval
)
