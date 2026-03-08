package rendezvous

import "time"

const (
	PeerLogInterval       = 60 * time.Second
	WSHeartbeatInterval   = 25 * time.Second
	ReadHeaderTimeout     = 5 * time.Second
	StatusCacheTTL        = 30 * time.Second
	HealthCheckTimeout    = 2 * time.Second
	PulseTimeout          = 3 * time.Second
	PunchCheckInterval    = 5 * time.Second
	PunchCutoffAge        = 5 * time.Minute
	RelayStatusInterval   = 3 * time.Second
	PresenceClientTimeout = 5 * time.Second
	WSBackoff             = 250 * time.Millisecond
	RelayDuration         = 30 * time.Minute
)
