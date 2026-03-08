package modes

import "time"

const (
	PeerKeyFetchTimeout       = 2 * time.Second
	RelayWaitTimeout          = 8 * time.Second
	EncryptionRegisterTimeout = 3 * time.Second
	DefaultRelayRefresh       = 90 * time.Second
	PruneCheckInterval        = 1 * time.Second
	ConfigRereadInterval      = 300 // in prune ticks (= 5 minutes at 1s tick)
	MQCallSignalTimeout       = 2 * time.Second
	MQClusterSendTimeout      = 3 * time.Second
)
