package storage

import (
	"encoding/json"
	"fmt"
)


// GroupRow represents a row from the _groups table.
type GroupRow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Owner        string `json:"owner"`
	GroupType    string `json:"group_type"`
	GroupContext  string `json:"group_context"`
	MaxMembers   int    `json:"max_members"`
	DefaultRole  string   `json:"default_role"`
	Roles        []string `json:"roles,omitempty"`
	Volatile     bool     `json:"volatile"`
	HostJoined   bool   `json:"host_joined"`
	CreatedAt    string `json:"created_at"`
}

// SubscriptionRow represents a row from the _group_subscriptions table.
type SubscriptionRow struct {
	HostPeerID   string `json:"host_peer_id"`
	HostName     string `json:"host_name"`
	GroupID      string `json:"group_id"`
	GroupName    string `json:"group_name"`
	GroupType      string `json:"group_type"`
	MaxMembers   int    `json:"max_members"`
	Volatile     bool   `json:"volatile"`
	Role         string `json:"role"`
	SubscribedAt string `json:"subscribed_at"`
}

// CreateGroup inserts a new group into _groups.
func (d *DB) CreateGroup(id, name, owner, groupType, groupContext string, maxMembers int, volatile bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	v := 0
	if volatile {
		v = 1
	}
	_, err := d.db.Exec(
		`INSERT INTO _groups (id, name, owner, group_type, group_context, max_members, volatile) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, owner, groupType, groupContext, maxMembers, v,
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

	rows, err := d.db.Query(`SELECT id, name, COALESCE(owner,''), group_type, COALESCE(group_context,''), max_members, COALESCE(default_role,'viewer'), COALESCE(roles,'[]'), COALESCE(volatile,0), host_joined, created_at FROM _groups ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []GroupRow
	for rows.Next() {
		var g GroupRow
		var vol int
		var rolesJSON string
		if err := rows.Scan(&g.ID, &g.Name, &g.Owner, &g.GroupType, &g.GroupContext, &g.MaxMembers, &g.DefaultRole, &rolesJSON, &vol, &g.HostJoined, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.Volatile = vol != 0
		_ = json.Unmarshal([]byte(rolesJSON), &g.Roles)
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
	var rolesJSON string
	err := d.db.QueryRow(
		`SELECT id, name, COALESCE(owner,''), group_type, COALESCE(group_context,''), max_members, COALESCE(default_role,'viewer'), COALESCE(roles,'[]'), COALESCE(volatile,0), host_joined, created_at FROM _groups WHERE id = ?`, id,
	).Scan(&g.ID, &g.Name, &g.Owner, &g.GroupType, &g.GroupContext, &g.MaxMembers, &g.DefaultRole, &rolesJSON, &vol, &g.HostJoined, &g.CreatedAt)
	if err != nil {
		return g, fmt.Errorf("get group: %w", err)
	}
	g.Volatile = vol != 0
	_ = json.Unmarshal([]byte(rolesJSON), &g.Roles)
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

// SetGroupRoles updates the available roles for a group.
func (d *DB) SetGroupRoles(groupID string, roles []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	b, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`UPDATE _groups SET roles = ? WHERE id = ?`, string(b), groupID)
	return err
}

// SetDefaultRole updates the default_role for a group.
func (d *DB) SetDefaultRole(groupID, role string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE _groups SET default_role = ? WHERE id = ?`, role, groupID)
	return err
}

// SetMemberRole updates the role of a specific member in a group.
func (d *DB) SetMemberRole(groupID, peerID, role string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE _group_members SET role = ? WHERE group_id = ? AND peer_id = ?`, role, groupID, peerID)
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
func (d *DB) AddSubscription(hostPeerID, groupID, groupName, groupType string, maxMembers int, volatile bool, role, hostName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	v := 0
	if volatile {
		v = 1
	}
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO _group_subscriptions (host_peer_id, group_id, group_name, group_type, max_members, volatile, role, host_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		hostPeerID, groupID, groupName, groupType, maxMembers, v, role, hostName,
	)
	if err != nil {
		return fmt.Errorf("add subscription: %w", err)
	}
	return nil
}

// UpdateSubscriptionHostName updates the host_name for all subscriptions to a given host peer.
func (d *DB) UpdateSubscriptionHostName(hostPeerID, hostName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(
		`UPDATE _group_subscriptions SET host_name = ? WHERE host_peer_id = ?`,
		hostName, hostPeerID,
	)
	return err
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

// GroupMember represents a member in a group with their role.
type GroupMember struct {
	PeerID string `json:"peer_id"`
	Role   string `json:"role"`
}

// UpsertGroupMembers replaces the stored member list for a group.
// Called whenever a TypeMembers or TypeWelcome is received so the list
// persists across disconnections and restarts.
func (d *DB) UpsertGroupMembers(groupID string, members []GroupMember) error {
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
	for _, m := range members {
		role := m.Role
		if role == "" {
			role = "viewer"
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO _group_members (group_id, peer_id, role) VALUES (?, ?, ?)`, groupID, m.PeerID, role); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListGroupMembers returns the persisted members for a group.
func (d *DB) ListGroupMembers(groupID string) ([]GroupMember, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`SELECT peer_id, COALESCE(role,'viewer') FROM _group_members WHERE group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []GroupMember
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.PeerID, &m.Role); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
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
		`SELECT host_peer_id, group_id, group_name, group_type, COALESCE(max_members,0), COALESCE(volatile,0), role, subscribed_at, COALESCE(host_name,'') FROM _group_subscriptions ORDER BY subscribed_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []SubscriptionRow
	for rows.Next() {
		var s SubscriptionRow
		var vol int
		if err := rows.Scan(&s.HostPeerID, &s.GroupID, &s.GroupName, &s.GroupType, &s.MaxMembers, &vol, &s.Role, &s.SubscribedAt, &s.HostName); err != nil {
			return nil, err
		}
		s.Volatile = vol != 0
		subs = append(subs, s)
	}
	return subs, rows.Err()
}
