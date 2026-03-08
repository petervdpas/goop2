package p2p

import "time"

const (
	RelayWaitPoll        = 500 * time.Millisecond
	RelayCleanupDelay    = 3 * time.Second
	RelayPollDeadline    = 10 * time.Second
	RelayConnectTimeout  = 5 * time.Second
	RelayRecoveryGrace   = 5 * time.Second
	RelayReserveTimeout  = 5 * time.Second
	AutoRelayBackoff     = 3 * time.Second
	ProbeTimeout         = 3 * time.Second
	ProbeCooldown        = 3 * time.Second
	AddrTTLMin           = 2 * time.Minute
	PeerstoreAddrTTL     = 10 * time.Minute
	DirectAddrTTL        = 20 * time.Second
)

// RelayRetryDelays defines the backoff between relay recovery attempts.
// First attempt is immediate, then 3s, 6s, 12s.
var RelayRetryDelays = []time.Duration{0, 3 * time.Second, 6 * time.Second, 12 * time.Second}
