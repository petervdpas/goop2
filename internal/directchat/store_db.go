package directchat

import "github.com/petervdpas/goop2/internal/storage"

// DBStore adapts *storage.DB to the chat.Store interface.
type DBStore struct{ db *storage.DB }

func NewDBStore(db *storage.DB) *DBStore { return &DBStore{db: db} }

func (s *DBStore) StoreChatMessage(peerID, fromID, content string, ts int64) error {
	return s.db.StoreChatMessage(peerID, fromID, content, ts)
}

func (s *DBStore) GetChatHistory(peerID string, limit int) ([]Message, error) {
	rows, err := s.db.GetChatHistory(peerID, limit)
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, len(rows))
	for i, r := range rows {
		msgs[i] = Message{From: r.From, Content: r.Content, Timestamp: r.Timestamp}
	}
	return msgs, nil
}

func (s *DBStore) ClearChatHistory(peerID string) error {
	return s.db.ClearChatHistory(peerID)
}
