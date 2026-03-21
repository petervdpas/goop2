package bridge

import "time"

// Bridge client timings — WebSocket connection to the bridge server.
const (
	DNSResolveTimeout  = 10 * time.Second      // DNS resolution (allows failover between nameservers)
	DNSCacheTTL        = 5 * time.Minute       // how long a cached DNS result stays valid
	HTTPClientTimeout  = 5 * time.Second       // REST calls to bridge server
	WSHandshakeTimeout = 5 * time.Second       // WS upgrade handshake
	WSReadDeadline     = 45 * time.Second      // must be > 2× WSPingInterval
	WSWriteDeadline    = 3 * time.Second       // writing a WS frame
	WSPingInterval     = 15 * time.Second      // keepalive ping frequency
	WSReconnectBackoff    = 500 * time.Millisecond // initial backoff on WS disconnect
	WSReconnectMaxBackoff = 5 * time.Second        // max backoff cap on WS disconnect
)
