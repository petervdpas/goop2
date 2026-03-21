package p2p

import "time"

const (
	YamuxKeepAlive         = 5 * time.Second
	RelayWaitPoll          = 250 * time.Millisecond
	RelayCleanupDelay      = 500 * time.Millisecond
	RelayPollDeadline      = 5 * time.Second
	RelayConnectTimeout    = 3 * time.Second
	RelayRecoveryGrace     = 2 * time.Second
	RelayReserveTimeout    = 3 * time.Second
	AutoRelayBackoff       = 500 * time.Millisecond
	ProbeTimeout           = 2 * time.Second
	ProbeCooldown          = 500 * time.Millisecond
	AddrTTLMin             = 2 * time.Minute
	PeerstoreAddrTTL       = 10 * time.Minute
	DirectAddrTTL          = 20 * time.Second
	SiteDialRetryBackoff   = 1 * time.Second
	SiteRelayRetryTotal    = 15 * time.Second
	SiteRelayAttemptTimeout = 5 * time.Second
	DataLuaCallTimeout     = 30 * time.Second
)

// RelayRetryDelays defines the backoff between relay recovery attempts.
var RelayRetryDelays = []time.Duration{0, 1 * time.Second, 3 * time.Second, 6 * time.Second}
