package storage

import (
	"fmt"
)


// GroupRow represents a row from the _groups table.
type GroupRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AppType    string `json:"app_type"`
	MaxMembers int    `json:"max_members"`
	Volatile   bool   `json:"volatile"`
	HostJoined bool   `json:"host_joined"`
	CreatedAt  string `json:"created_at"`
}

// SubscriptionRow represents a row from the _group_subscriptions table.
type SubscriptionRow struct {
	HostPeerID   string `json:"host_peer_id"`
	GroupID      string `json:"group_id"`
	GroupName    string `json:"group_name"`
	AppType      string `json:"app_type"`
	MaxMembers   int    `json:"max_members"`
	Volatile     bool   `json:"volatile"`
	Role         string `json:"role"`
	SubscribedAt string `json:"subscribed_at"`
}

// CreateGroup inserts a new group into _groups.
func (d *DB) CreateGroup(id, name, appType string, maxMembers int, volatile bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	v := 0
	if volatile {
		v = 1
	}
	_, err := d.db.Exec(
		`INSERT INTO _groups (id, name, app_type, max_members, volatile) VALUES (?, ?, ?, ?, ?)`,
		id, name, appType, maxMembers, v,
	)
	if err != nil {
		return fmt.Errorf("create group: %w", err)
	}
	return nil
}

// ListGroups returns all groups from _groups.
func (d *DB) ListGroups() ([]GroupRow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`SELECT id, name, app_type, max_members, COALESCE(volatile,0), host_joined, created_at FROM _groups ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []GroupRow
	for rows.Next() {
		var g GroupRow
		var vol int
		if err := rows.Scan(&g.ID, &g.Name, &g.AppType, &g.MaxMembers, &vol, &g.HostJoined, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.Volatile = vol != 0
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetGroup returns a single group by ID.
func (d *DB) GetGroup(id string) (GroupRow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var g GroupRow
	var vol int
	err := d.db.QueryRow(
		`SELECT id, name, app_type, max_members, COALESCE(volatile,0), host_joined, created_at FROM _groups WHERE id = ?`, id,
	).Scan(&g.ID, &g.Name, &g.AppType, &g.MaxMembers, &vol, &g.HostJoined, &g.CreatedAt)
	if err != nil {
		return g, fmt.Errorf("get group: %w", err)
	}
	g.Volatile = vol != 0
	return g, nil
}

// DeleteGroup removes a group from _groups.
func (d *DB) DeleteGroup(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`DELETE FROM _groups WHERE id = ?`, id)
	return err
}

// SetMaxMembers updates the max_members limit for a group.
func (d *DB) SetMaxMembers(groupID string, max int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE _groups SET max_members = ? WHERE id = ?`, max, groupID)
	return err
}

// UpdateGroup updates the name and max_members of a hosted group.
func (d *DB) UpdateGroup(groupID, name string, maxMembers int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE _groups SET name = ?, max_members = ? WHERE id = ?`, name, maxMembers, groupID)
	return err
}

// SetHostJoined updates the host_joined flag for a group.
func (d *DB) SetHostJoined(groupID string, joined bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	v := 0
	if joined {
		v = 1
	}
	_, err := d.db.Exec(`UPDATE _groups SET host_joined = ? WHERE id = ?`, v, groupID)
	return err
}

// AddSubscription records a subscription to a remote group.
func (d *DB) AddSubscription(hostPeerID, groupID, groupName, appType string, maxMembers int, volatile bool, role string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	v := 0
	if volatile {
		v = 1
	}
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO _group_subscriptions (host_peer_id, group_id, group_name, app_type, max_members, volatile, role) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		hostPeerID, groupID, groupName, appType, maxMembers, v, role,
	)
	if err != nil {
		return fmt.Errorf("add subscription: %w", err)
	}
	return nil
}

// RemoveSubscription removes a subscription record.
func (d *DB) RemoveSubscription(hostPeerID, groupID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(
		`DELETE FROM _group_subscriptions WHERE host_peer_id = ? AND group_id = ?`,
		hostPeerID, groupID,
	)
	return err
}

// UpsertGroupMembers replaces the stored member list for a group.
// Called whenever a TypeMembers or TypeWelcome is received so the list
// persists across disconnections and restarts.
func (d *DB) UpsertGroupMembers(groupID string, peerIDs []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM _group_members WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	for _, pid := range peerIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO _group_members (group_id, peer_id) VALUES (?, ?)`, groupID, pid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListGroupMembers returns the persisted peer IDs for a group.
func (d *DB) ListGroupMembers(groupID string) ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`SELECT peer_id FROM _group_members WHERE group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		peers = append(peers, pid)
	}
	return peers, rows.Err()
}

// DeleteGroupMembers removes all stored members for a group.
func (d *DB) DeleteGroupMembers(groupID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`DELETE FROM _group_members WHERE group_id = ?`, groupID)
	return err
}

// ListSubscriptions returns all subscription records.
func (d *DB) ListSubscriptions() ([]SubscriptionRow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(
		`SELECT host_peer_id, group_id, group_name, app_type, COALESCE(max_members,0), COALESCE(volatile,0), role, subscribed_at FROM _group_subscriptions ORDER BY subscribed_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []SubscriptionRow
	for rows.Next() {
		var s SubscriptionRow
		var vol int
		if err := rows.Scan(&s.HostPeerID, &s.GroupID, &s.GroupName, &s.AppType, &s.MaxMembers, &vol, &s.Role, &s.SubscribedAt); err != nil {
			return nil, err
		}
		s.Volatile = vol != 0
		subs = append(subs, s)
	}
	return subs, rows.Err()
}
