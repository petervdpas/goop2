package rendezvous

import "time"

// Client-side rendezvous timings — connecting to rendezvous server(s).
const (
	DNSResolveTimeout    = 10 * time.Second // DNS resolution (allows failover between nameservers)
	DNSCacheTTL          = 5 * time.Minute  // how long a cached DNS result stays valid
	HTTPClientTimeout    = 5 * time.Second  // REST calls to rendezvous (excl. DNS)
	WSHandshakeTimeout   = 5 * time.Second  // WebSocket upgrade handshake (excl. DNS)
	WSWriteDeadline      = 2 * time.Second  // writing a WS frame
	WSReadDeadline       = 15 * time.Second // must be > 2× WSPingInterval
	WSPingInterval       = 5 * time.Second  // WS keepalive ping
	SSEReconnectBackoff  = 500 * time.Millisecond // SSE reconnect delay
	WSReconnectBackoff   = 500 * time.Millisecond // WS reconnect delay
	WSProbeTimeout       = 2 * time.Second  // WS health probe
	WSProbeFirstInterval = 3 * time.Second  // first probe after connect
	WSProbeNextInterval  = 10 * time.Second // subsequent probe interval
)
