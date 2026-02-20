package storage

import (
	"encoding/json"
	"sort"
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
	Favorite       bool
}

// UpsertCachedPeer stores or fully replaces the cached state for a peer in _peer_cache.
// If the peer is in _favorites (marked as favorite), also updates their metadata there so data is preserved.
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

	// Update _peer_cache
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
	if err != nil {
		return err
	}

	// If peer is in favorites, also update their metadata there
	_, _ = d.db.Exec(`
		UPDATE _favorites SET
			content         = ?,
			email           = ?,
			avatar_hash     = ?,
			video_disabled  = ?,
			active_template = ?,
			verified        = ?,
			addrs           = CASE WHEN ? = '[]' THEN _favorites.addrs ELSE ? END,
			last_seen       = CURRENT_TIMESTAMP
		WHERE peer_id = ?`,
		p.Content, p.Email, p.AvatarHash, vd, p.ActiveTemplate, ver, string(addrs), string(addrs), p.PeerID,
	)

	return nil
}

// GetCachedPeer returns the last known state for a peer, or false if unknown.
// Prefers _peer_cache (current data if online), falls back to _favorites if peer is offline/pruned.
// Favorite flag is only set if peer is in _favorites.
func (d *DB) GetCachedPeer(peerID string) (CachedPeer, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var p CachedPeer
	var vd, ver int
	var addrsJSON string
	var lastSeen string
	isFavorite := false

	// Try _peer_cache first (most current if online)
	err := d.db.QueryRow(`
		SELECT peer_id, content, email, avatar_hash, video_disabled,
		       active_template, verified, addrs, last_seen
		FROM _peer_cache WHERE peer_id = ?`, peerID).
		Scan(&p.PeerID, &p.Content, &p.Email, &p.AvatarHash, &vd,
			&p.ActiveTemplate, &ver, &addrsJSON, &lastSeen)

	if err != nil {
		// Fall back to _favorites if peer is offline/pruned
		err = d.db.QueryRow(`
			SELECT peer_id, content, email, avatar_hash, video_disabled,
			       active_template, verified, addrs, last_seen
			FROM _favorites WHERE peer_id = ?`, peerID).
			Scan(&p.PeerID, &p.Content, &p.Email, &p.AvatarHash, &vd,
				&p.ActiveTemplate, &ver, &addrsJSON, &lastSeen)
		if err != nil {
			return CachedPeer{}, false
		}
		isFavorite = true // Came from _favorites, so it's favorite
	} else {
		// Found in _peer_cache, check if also in _favorites
		var favCount int
		d.db.QueryRow(`SELECT 1 FROM _favorites WHERE peer_id = ?`, peerID).Scan(&favCount)
		isFavorite = favCount == 1
	}

	p.VideoDisabled = vd != 0
	p.Verified = ver != 0
	p.Favorite = isFavorite
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

// ListCachedPeers returns all known peers: online peers from _peer_cache + offline favorites from _favorites.
// Prefers _peer_cache data (most current) but includes offline favorites to preserve their data.
func (d *DB) ListCachedPeers() ([]CachedPeer, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Get all peers from _peer_cache
	rows, err := d.db.Query(`
		SELECT peer_id, content, email, avatar_hash, video_disabled,
		       active_template, verified, addrs, last_seen
		FROM _peer_cache ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Load all favorites for efficient lookup
	favRows, err := d.db.Query(`SELECT peer_id FROM _favorites`)
	if err != nil {
		rows.Close()
		return nil, err
	}
	favorites := make(map[string]bool)
	for favRows.Next() {
		var peerID string
		if err := favRows.Scan(&peerID); err != nil {
			favRows.Close()
			rows.Close()
			return nil, err
		}
		favorites[peerID] = true
	}
	favRows.Close()

	var peers []CachedPeer
	seenPeers := make(map[string]bool)

	// Process all online peers from _peer_cache
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
		p.Favorite = favorites[p.PeerID]
		json.Unmarshal([]byte(addrsJSON), &p.Addrs)
		p.LastSeen, _ = time.Parse("2006-01-02 15:04:05", lastSeen)
		peers = append(peers, p)
		seenPeers[p.PeerID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Add any offline favorites not in _peer_cache
	for favID := range favorites {
		if seenPeers[favID] {
			continue // Already included from _peer_cache
		}
		var p CachedPeer
		var vd, ver int
		var addrsJSON, lastSeen string
		err := d.db.QueryRow(`
			SELECT peer_id, content, email, avatar_hash, video_disabled,
			       active_template, verified, addrs, last_seen
			FROM _favorites WHERE peer_id = ?`, favID).
			Scan(&p.PeerID, &p.Content, &p.Email, &p.AvatarHash, &vd,
				&p.ActiveTemplate, &ver, &addrsJSON, &lastSeen)
		if err != nil {
			continue // Skip if can't read
		}
		p.VideoDisabled = vd != 0
		p.Verified = ver != 0
		p.Favorite = true
		json.Unmarshal([]byte(addrsJSON), &p.Addrs)
		p.LastSeen, _ = time.Parse("2006-01-02 15:04:05", lastSeen)
		peers = append(peers, p)
	}

	// Sort by last_seen DESC
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].LastSeen.After(peers[j].LastSeen)
	})

	return peers, nil
}

// DeleteCachedPeer removes a peer from the cache entirely.
// Only used when a peer is permanently forgotten (not just offline).
func (d *DB) DeleteCachedPeer(peerID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`DELETE FROM _peer_cache WHERE peer_id = ?`, peerID)
	return err
}

// SetFavorite marks a peer as favorite (or unfavorite).
// When favoriting, copies full peer metadata from _peer_cache to _favorites so data is preserved if peer goes offline.
// When unfavoriting, removes from _favorites.
func (d *DB) SetFavorite(peerID string, favorite bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if favorite {
		// Copy peer metadata from _peer_cache to _favorites (upsert)
		_, err := d.db.Exec(`
			INSERT INTO _favorites (peer_id, content, email, avatar_hash, video_disabled, active_template, verified, addrs, last_seen)
			SELECT peer_id, content, email, avatar_hash, video_disabled, active_template, verified, addrs, last_seen
			FROM _peer_cache WHERE peer_id = ?
			ON CONFLICT(peer_id) DO UPDATE SET
				content         = excluded.content,
				email           = excluded.email,
				avatar_hash     = excluded.avatar_hash,
				video_disabled  = excluded.video_disabled,
				active_template = excluded.active_template,
				verified        = excluded.verified,
				addrs           = excluded.addrs,
				last_seen       = excluded.last_seen`, peerID)
		return err
	} else {
		// Remove from favorites
		_, err := d.db.Exec(`DELETE FROM _favorites WHERE peer_id = ?`, peerID)
		return err
	}
}

// UpdateFavoriteIfExists updates a favorite peer's metadata if they exist in _favorites.
// Called when a peer comes online and their metadata updates.
func (d *DB) UpdateFavoriteIfExists(peerID string, content, email, avatarHash string, videoDisabled, verified bool, addrs string, lastSeen time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	vd := 0
	if videoDisabled {
		vd = 1
	}
	ver := 0
	if verified {
		ver = 1
	}
	_, err := d.db.Exec(`
		UPDATE _favorites SET
			content         = ?,
			email           = ?,
			avatar_hash     = ?,
			video_disabled  = ?,
			active_template = ?,
			verified        = ?,
			addrs           = ?,
			last_seen       = ?
		WHERE peer_id = ?`,
		content, email, avatarHash, vd, "", ver, addrs, lastSeen, peerID)
	return err
}
