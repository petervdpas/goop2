package chat

import "time"

// MessageType represents the type of chat message
type MessageType string

const (
	MessageTypeDirect    MessageType = "direct"    // 1-to-1 message
	MessageTypeBroadcast MessageType = "broadcast" // public broadcast to all peers
)

// Message represents a chat message between peers
type Message struct {
	ID        string      `json:"id"`        // unique message ID
	From      string      `json:"from"`      // sender peer ID
	To        string      `json:"to"`        // recipient peer ID (empty for broadcast)
	Type      MessageType `json:"type"`      // message type
	Content   string      `json:"content"`   // message content
	Timestamp int64       `json:"timestamp"` // unix timestamp in milliseconds
}

// NewMessage creates a new direct message
func NewMessage(from, to, content string) *Message {
	return &Message{
		ID:        generateID(),
		From:      from,
		To:        to,
		Type:      MessageTypeDirect,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
	}
}

// NewBroadcast creates a new broadcast message
func NewBroadcast(from, content string) *Message {
	return &Message{
		ID:        generateID(),
		From:      from,
		Type:      MessageTypeBroadcast,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
	}
}


// generateID creates a unique message ID
func generateID() string {
	// Simple timestamp-based ID for now
	// Could be improved with UUID or nanoid
	return time.Now().Format("20060102150405.000000")
}
