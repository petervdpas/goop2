package chat

// Room represents an active chat room backed by a group.
type Room struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Members     []Member `json:"members,omitempty"`
}

// Member is a participant with a resolved display name.
type Member struct {
	PeerID string `json:"peer_id"`
	Name   string `json:"name,omitempty"`
}

// Message is a single chat message.
type Message struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	FromName  string `json:"from_name"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}
