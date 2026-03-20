package group

import "time"

// Group protocol timings.
const (
	PingInterval       = 60 * time.Second // host → member heartbeat
	SendTimeout        = 5 * time.Second  // single MQ send (leave, close, kick, pong)
	BroadcastTimeout   = 5 * time.Second  // MQ send for welcome, broadcast, ping
	JoinTimeout        = 5 * time.Second  // full join handshake (send + wait for welcome)
	ReconnectTimeout   = 5 * time.Second  // reconnect attempt per subscription
	DiscoveryWait      = 3 * time.Second  // wait for mDNS/rendezvous before reconnecting
	ClusterSendTimeout = 3 * time.Second  // cluster MQ send (tighter for job scheduling)
)
