package storage

import "fmt"

// GroupRow represents a row from the _groups table.
type GroupRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AppType    string `json:"app_type"`
	MaxMembers int    `json:"max_members"`
	CreatedAt  string `json:"created_at"`
}

// SubscriptionRow represents a row from the _group_subscriptions table.
type SubscriptionRow struct {
	HostPeerID   string `json:"host_peer_id"`
	GroupID      string `json:"group_id"`
	GroupName    string `json:"group_name"`
	AppType      string `json:"app_type"`
	Role         string `json:"role"`
	SubscribedAt string `json:"subscribed_at"`
}

// CreateGroup inserts a new group into _groups.
func (d *DB) CreateGroup(id, name, appType string, maxMembers int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(
		`INSERT INTO _groups (id, name, app_type, max_members) VALUES (?, ?, ?, ?)`,
		id, name, appType, maxMembers,
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

	rows, err := d.db.Query(`SELECT id, name, app_type, max_members, created_at FROM _groups ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []GroupRow
	for rows.Next() {
		var g GroupRow
		if err := rows.Scan(&g.ID, &g.Name, &g.AppType, &g.MaxMembers, &g.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetGroup returns a single group by ID.
func (d *DB) GetGroup(id string) (GroupRow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var g GroupRow
	err := d.db.QueryRow(
		`SELECT id, name, app_type, max_members, created_at FROM _groups WHERE id = ?`, id,
	).Scan(&g.ID, &g.Name, &g.AppType, &g.MaxMembers, &g.CreatedAt)
	if err != nil {
		return g, fmt.Errorf("get group: %w", err)
	}
	return g, nil
}

// DeleteGroup removes a group from _groups.
func (d *DB) DeleteGroup(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`DELETE FROM _groups WHERE id = ?`, id)
	return err
}

// AddSubscription records a subscription to a remote group.
func (d *DB) AddSubscription(hostPeerID, groupID, groupName, appType, role string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO _group_subscriptions (host_peer_id, group_id, group_name, app_type, role) VALUES (?, ?, ?, ?, ?)`,
		hostPeerID, groupID, groupName, appType, role,
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

// ListSubscriptions returns all subscription records.
func (d *DB) ListSubscriptions() ([]SubscriptionRow, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(
		`SELECT host_peer_id, group_id, group_name, app_type, role, subscribed_at FROM _group_subscriptions ORDER BY subscribed_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []SubscriptionRow
	for rows.Next() {
		var s SubscriptionRow
		if err := rows.Scan(&s.HostPeerID, &s.GroupID, &s.GroupName, &s.AppType, &s.Role, &s.SubscribedAt); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}
