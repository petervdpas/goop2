package routes

import "time"

// Viewer HTTP route timings.
const (
	ServiceCheckTimeout  = 3 * time.Second        // health check to microservices
	BridgeCheckTimeout   = 3 * time.Second        // bridge token request
	ProxyTimeout         = 3 * time.Second        // proxy/capability check to rendezvous
	EncryptionTimeout    = 3 * time.Second        // encryption service check
	CallWriteDeadline    = 5 * time.Second        // WebSocket write deadline for call streams
	ListenPollInterval   = 500 * time.Millisecond // listen stream position poll
	ListenHostTimeout    = 3 * time.Second        // listen stream host-gone detection
	DocListFetchTimeout  = 5 * time.Second        // fetch doc list from peer
	DocFileFetchTimeout  = 15 * time.Second       // fetch single doc file from peer
	ClusterJoinTimeout   = 5 * time.Second        // cluster join operation
	MQSendTimeout        = 5 * time.Second        // MQ send via API
	MQAckRelayTimeout    = 3 * time.Second        // MQ ack relay back to sender
	AvatarFetchTimeout   = 5 * time.Second        // fetch avatar from peer
	GroupJoinTimeout     = 5 * time.Second        // group join/invite/rejoin
	TemplateListTimeout  = 3 * time.Second        // template store listing
	TemplateBundleTimeout = 15 * time.Second      // template bundle download
	CreditsBalanceTimeout = 3 * time.Second       // credits balance fetch
)
