package rendezvous

import "time"

// Server-side rendezvous timings — the rendezvous server process.
const (
	PeerLogInterval       = 60 * time.Second  // log connected peer count
	WSHeartbeatInterval   = 25 * time.Second  // WS entangler heartbeat to peers
	ReadHeaderTimeout     = 5 * time.Second   // HTTP server read header timeout
	StatusCacheTTL        = 30 * time.Second  // cache duration for /status proxied responses
	HealthCheckTimeout    = 2 * time.Second   // health check HTTP client
	PulseTimeout          = 3 * time.Second   // pulse a peer to refresh relay reservation
	PunchCheckInterval    = 5 * time.Second   // hole-punch hint check loop
	PunchCutoffAge        = 5 * time.Minute   // ignore punch hints older than this
	RelayStatusInterval   = 3 * time.Second   // relay status broadcast tick
	PresenceClientTimeout = 5 * time.Second   // HTTP client for remote presence fetch
	WSBackoff             = 250 * time.Millisecond // initial WS reconnect backoff
	RelayDuration         = 30 * time.Minute  // max duration per relayed connection
)
