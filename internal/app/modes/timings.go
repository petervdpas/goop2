package modes

import "time"

const (
	PeerKeyFetchTimeout       = 2 * time.Second  // fetch peer's public key from rendezvous
	EncryptionRegisterTimeout = 3 * time.Second  // register public key with rendezvous
	DefaultRelayRefresh       = 90 * time.Second // periodic relay reservation health check
	PruneCheckInterval        = 1 * time.Second  // peer table prune tick
	ConfigRereadInterval      = 300              // re-read config every N prune ticks (5 min at 1s)
	MQCallSignalTimeout       = 2 * time.Second  // MQ send for call signaling messages
)
