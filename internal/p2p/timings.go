package p2p

import "time"

// Relay recovery timings.
const (
	RelayWaitPoll       = 500 * time.Millisecond // poll interval when waiting for circuit address
	RelayCleanupDelay   = 3 * time.Second        // wait for old reservation to clear before reconnect
	RelayPollDeadline   = 10 * time.Second       // max wait for reservation after reconnect
	RelayConnectTimeout = 5 * time.Second        // dial timeout to relay server
	RelayRecoveryGrace  = 5 * time.Second        // let autorelay self-recover before intervening
	RelayReserveTimeout = 5 * time.Second        // explicit reservation request timeout
	AutoRelayBackoff    = 3 * time.Second        // autorelay retry backoff
)

// Peer connection timings.
const (
	ProbeTimeout     = 3 * time.Second  // reachability probe per peer
	ProbeCooldown    = 3 * time.Second  // min interval between failed probes to same peer
	AddrTTLMin       = 2 * time.Minute  // floor for peerstore address TTL
	PeerstoreAddrTTL = 10 * time.Minute // TTL for injected relay addresses
	DirectAddrTTL    = 20 * time.Second // presence-based direct address TTL
)

// RelayRetryDelays defines the backoff between relay recovery attempts.
var RelayRetryDelays = []time.Duration{0, 3 * time.Second, 6 * time.Second, 12 * time.Second}
