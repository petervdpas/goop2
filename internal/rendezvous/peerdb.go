package rendezvous

import (
	"database/sql"
	"log"
	"sync"

	_ "modernc.org/sqlite"
)

// peerDB provides optional SQLite-backed peer persistence for multi-instance
// rendezvous deployments. When multiple instances share the same database file,
// each instance can see peers registered by the others.
type peerDB struct {
	db *sql.DB
	mu sync.Mutex
}

// openPeerDB opens (or creates) a SQLite database for peer persistence.
func openPeerDB(path string) (*peerDB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// WAL mode for concurrent access from multiple processes sharing the file.
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS peers (
		peer_id        TEXT PRIMARY KEY,
		type           TEXT NOT NULL DEFAULT '',
		content        TEXT DEFAULT '',
		email          TEXT DEFAULT '',
		avatar_hash    TEXT DEFAULT '',
		ts             INTEGER DEFAULT 0,
		last_seen      INTEGER DEFAULT 0,
		bytes_sent     INTEGER DEFAULT 0,
		bytes_received INTEGER DEFAULT 0
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &peerDB{db: db}, nil
}

// upsert writes a peer row to SQLite.
func (p *peerDB) upsert(row peerRow) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.db.Exec(`INSERT INTO peers (peer_id, type, content, email, avatar_hash, ts, last_seen, bytes_sent, bytes_received)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET
			type=excluded.type,
			content=excluded.content,
			email=excluded.email,
			avatar_hash=excluded.avatar_hash,
			ts=excluded.ts,
			last_seen=excluded.last_seen,
			bytes_sent=excluded.bytes_sent,
			bytes_received=excluded.bytes_received`,
		row.PeerID, row.Type, row.Content, row.Email, row.AvatarHash,
		row.TS, row.LastSeen, row.BytesSent, row.BytesReceived)
	if err != nil {
		log.Printf("peerdb: upsert error: %v", err)
	}
}

// remove deletes a peer from SQLite.
func (p *peerDB) remove(peerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = p.db.Exec(`DELETE FROM peers WHERE peer_id = ?`, peerID)
}

// cleanupStale removes peers older than the given threshold (unix millis).
func (p *peerDB) cleanupStale(thresholdMillis int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = p.db.Exec(`DELETE FROM peers WHERE last_seen < ?`, thresholdMillis)
}

// loadAll returns all peers from SQLite.
func (p *peerDB) loadAll() ([]peerRow, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	rows, err := p.db.Query(`SELECT peer_id, type, content, email, avatar_hash, ts, last_seen, bytes_sent, bytes_received FROM peers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []peerRow
	for rows.Next() {
		var r peerRow
		if err := rows.Scan(&r.PeerID, &r.Type, &r.Content, &r.Email, &r.AvatarHash,
			&r.TS, &r.LastSeen, &r.BytesSent, &r.BytesReceived); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// close closes the database.
func (p *peerDB) close() error {
	return p.db.Close()
}
