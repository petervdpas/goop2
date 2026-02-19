package storage

import (
	"encoding/json"
	"time"
)

// CachedPeer is the persistent record of a remote peer's last known state.
// It is written whenever a presence pulse is received and never cleared
// just because the peer goes offline — only the originator can change it.
type CachedPeer struct {
	PeerID         string
	Content        string
	Email          string
	AvatarHash     string
	VideoDisabled  bool
	ActiveTemplate string
	Verified       bool
	Addrs          []string
	LastSeen       time.Time
}

// UpsertCachedPeer stores or fully replaces the cached state for a peer.
func (d *DB) UpsertCachedPeer(p CachedPeer) error {
	addrs, _ := json.Marshal(p.Addrs)
	vd := 0
	if p.VideoDisabled {
		vd = 1
	}
	ver := 0
	if p.Verified {
		ver = 1
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`
		INSERT INTO _peer_cache
			(peer_id, content, email, avatar_hash, video_disabled, active_template, verified, addrs, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(peer_id) DO UPDATE SET
			content         = excluded.content,
			email           = excluded.email,
			avatar_hash     = excluded.avatar_hash,
			video_disabled  = excluded.video_disabled,
			active_template = excluded.active_template,
			verified        = excluded.verified,
			addrs           = CASE WHEN excluded.addrs = '[]' THEN _peer_cache.addrs ELSE excluded.addrs END,
			last_seen       = CURRENT_TIMESTAMP`,
		p.PeerID, p.Content, p.Email, p.AvatarHash, vd, p.ActiveTemplate, ver, string(addrs),
	)
	return err
}

// GetCachedPeer returns the last known state for a peer, or false if unknown.
func (d *DB) GetCachedPeer(peerID string) (CachedPeer, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var p CachedPeer
	var vd, ver int
	var addrsJSON string
	var lastSeen string
	err := d.db.QueryRow(`
		SELECT peer_id, content, email, avatar_hash, video_disabled,
		       active_template, verified, addrs, last_seen
		FROM _peer_cache WHERE peer_id = ?`, peerID).
		Scan(&p.PeerID, &p.Content, &p.Email, &p.AvatarHash, &vd,
			&p.ActiveTemplate, &ver, &addrsJSON, &lastSeen)
	if err != nil {
		return CachedPeer{}, false
	}
	p.VideoDisabled = vd != 0
	p.Verified = ver != 0
	json.Unmarshal([]byte(addrsJSON), &p.Addrs)
	p.LastSeen, _ = time.Parse("2006-01-02 15:04:05", lastSeen)
	return p, true
}

// GetPeerName returns just the content/name for a peer ID, or "" if unknown.
func (d *DB) GetPeerName(peerID string) string {
	p, ok := d.GetCachedPeer(peerID)
	if !ok {
		return ""
	}
	return p.Content
}

// ListCachedPeers returns all cached peers.
func (d *DB) ListCachedPeers() ([]CachedPeer, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.db.Query(`
		SELECT peer_id, content, email, avatar_hash, video_disabled,
		       active_template, verified, addrs, last_seen
		FROM _peer_cache ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var peers []CachedPeer
	for rows.Next() {
		var p CachedPeer
		var vd, ver int
		var addrsJSON, lastSeen string
		if err := rows.Scan(&p.PeerID, &p.Content, &p.Email, &p.AvatarHash, &vd,
			&p.ActiveTemplate, &ver, &addrsJSON, &lastSeen); err != nil {
			return nil, err
		}
		p.VideoDisabled = vd != 0
		p.Verified = ver != 0
		json.Unmarshal([]byte(addrsJSON), &p.Addrs)
		p.LastSeen, _ = time.Parse("2006-01-02 15:04:05", lastSeen)
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

// DeleteCachedPeer removes a peer from the cache entirely.
// Only used when a peer is permanently forgotten (not just offline).
func (d *DB) DeleteCachedPeer(peerID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`DELETE FROM _peer_cache WHERE peer_id = ?`, peerID)
	return err
}
