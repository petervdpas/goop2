package directchat

import "context"

// Message represents one stored chat message.
type Message struct {
	From      string `json:"from"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// Store abstracts chat message persistence.
type Store interface {
	StoreChatMessage(peerID, fromID, content string, ts int64) error
	GetChatHistory(peerID string, limit int) ([]Message, error)
	ClearChatHistory(peerID string) error
}

// MQ abstracts the message queue transport layer.
type MQ interface {
	SubscribeTopic(prefix string, fn func(from, topic string, payload any)) func()
	Send(ctx context.Context, peerID, topic string, payload any) (string, error)
}

// LuaDispatcher handles "!" chat commands via the Lua scripting engine.
type LuaDispatcher interface {
	DispatchCommand(ctx context.Context, fromPeerID, content string, reply func(ctx context.Context, toPeerID, msg string) error)
}
