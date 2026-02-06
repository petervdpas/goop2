package rendezvous

import (
	"database/sql"
	"log"
	"sync"
	"time"

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

	// Registrations table for email-based access control
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS registrations (
		email          TEXT PRIMARY KEY,
		token          TEXT NOT NULL,
		created_at     INTEGER NOT NULL,
		verified_at    INTEGER DEFAULT 0,
		metadata       TEXT DEFAULT '{}'
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

// maxLastSeenAndCount returns the maximum last_seen value and peer count.
// Used by syncFromDB to skip full loads when nothing changed.
func (p *peerDB) maxLastSeenAndCount() (int64, int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var maxLS sql.NullInt64
	var count int
	err := p.db.QueryRow(`SELECT MAX(last_seen), COUNT(*) FROM peers`).Scan(&maxLS, &count)
	if err != nil {
		return 0, 0, err
	}
	return maxLS.Int64, count, nil
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

// Registration represents a registered email.
type Registration struct {
	Email      string `json:"email"`
	Token      string `json:"-"`
	CreatedAt  int64  `json:"created_at"`
	VerifiedAt int64  `json:"verified_at"`
	Metadata   string `json:"metadata"`
}

// IsVerified returns true if the registration has been verified.
func (r Registration) IsVerified() bool {
	return r.VerifiedAt > 0
}

// createRegistration creates a new pending registration.
func (p *peerDB) createRegistration(email, token string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := timeNowMillis()
	_, err := p.db.Exec(`INSERT INTO registrations (email, token, created_at) VALUES (?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET token=excluded.token, created_at=excluded.created_at, verified_at=0`,
		email, token, now)
	return err
}

// verifyRegistration marks a registration as verified if the token matches.
func (p *peerDB) verifyRegistration(token string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var email string
	err := p.db.QueryRow(`SELECT email FROM registrations WHERE token = ? AND verified_at = 0`, token).Scan(&email)
	if err != nil {
		return "", false
	}

	now := timeNowMillis()
	_, err = p.db.Exec(`UPDATE registrations SET verified_at = ? WHERE token = ?`, now, token)
	if err != nil {
		return "", false
	}

	return email, true
}

// isEmailVerified checks if an email is verified.
func (p *peerDB) isEmailVerified(email string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	var verifiedAt int64
	err := p.db.QueryRow(`SELECT verified_at FROM registrations WHERE email = ?`, email).Scan(&verifiedAt)
	if err != nil {
		return false
	}
	return verifiedAt > 0
}

// getRegistration returns a registration by email.
func (p *peerDB) getRegistration(email string) (*Registration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var r Registration
	err := p.db.QueryRow(`SELECT email, token, created_at, verified_at, metadata FROM registrations WHERE email = ?`, email).
		Scan(&r.Email, &r.Token, &r.CreatedAt, &r.VerifiedAt, &r.Metadata)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// listRegistrations returns all registrations.
func (p *peerDB) listRegistrations() ([]Registration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	rows, err := p.db.Query(`SELECT email, token, created_at, verified_at, metadata FROM registrations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Registration
	for rows.Next() {
		var r Registration
		if err := rows.Scan(&r.Email, &r.Token, &r.CreatedAt, &r.VerifiedAt, &r.Metadata); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// deleteRegistration removes a registration.
func (p *peerDB) deleteRegistration(email string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.db.Exec(`DELETE FROM registrations WHERE email = ?`, email)
	return err
}

// timeNowMillis returns current time in milliseconds.
func timeNowMillis() int64 {
	return time.Now().UnixMilli()
}
