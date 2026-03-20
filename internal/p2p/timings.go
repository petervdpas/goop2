package p2p

import "time"

// Relay recovery timings.
const (
	RelayWaitPoll       = 250 * time.Millisecond // poll interval when waiting for circuit address
	RelayCleanupDelay   = 500 * time.Millisecond  // wait for old reservation to clear before reconnect
	RelayPollDeadline   = 5 * time.Second        // max wait for reservation after reconnect
	RelayConnectTimeout = 3 * time.Second        // dial timeout to relay server
	RelayRecoveryGrace  = 2 * time.Second        // let autorelay self-recover before intervening
	RelayReserveTimeout = 3 * time.Second        // explicit reservation request timeout
	AutoRelayBackoff    = 500 * time.Millisecond  // autorelay retry backoff
)

// Peer connection timings.
const (
	ProbeTimeout     = 2 * time.Second  // reachability probe per peer
	ProbeCooldown    = 500 * time.Millisecond // min interval between failed probes to same peer
	AddrTTLMin       = 2 * time.Minute  // floor for peerstore address TTL
	PeerstoreAddrTTL = 10 * time.Minute // TTL for injected relay addresses
	DirectAddrTTL        = 20 * time.Second // presence-based direct address TTL
	SiteDialRetryBackoff = 3 * time.Second   // per-attempt backoff multiplier for site fetch retries
	SiteRelayRetryTotal  = 45 * time.Second  // total budget for relay-based site fetch retries
	SiteRelayAttemptTimeout = 15 * time.Second // per-attempt timeout for relay site fetch
	DataLuaCallTimeout   = 30 * time.Second  // Lua function call via data protocol
)

// RelayRetryDelays defines the backoff between relay recovery attempts.
var RelayRetryDelays = []time.Duration{0, 1 * time.Second, 3 * time.Second, 6 * time.Second}
