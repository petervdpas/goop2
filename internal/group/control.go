package group

import (
	"encoding/json"
	"fmt"
)

// SendControl sends a typed control message to all members of a group.
// The payload is wrapped in the standard envelope: {"group_type": groupType, groupType: msg}.
// Automatically uses SendToGroupAsHost or SendToGroup depending on whether
// this peer hosts the group.
func (m *Manager) SendControl(groupID, groupType string, msg any) error {
	payload := map[string]any{
		"group_type": groupType,
		groupType:    msg,
	}

	m.mu.RLock()
	_, isHost := m.groups[groupID]
	m.mu.RUnlock()

	if isHost {
		return m.SendToGroupAsHost(groupID, payload)
	}
	return m.SendToGroup(groupID, payload)
}

// ExtractControl extracts a typed control message from a group "msg" event payload.
// The groupType key is used to find the nested control data in the envelope.
// Returns the raw JSON bytes of the control message, or an error.
func ExtractControl(payload any, groupType string) (json.RawMessage, bool) {
	mp, ok := payload.(map[string]any)
	if !ok {
		return nil, false
	}
	raw, ok := mp[groupType]
	if !ok {
		return nil, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	return data, true
}

// ParseControl extracts and unmarshals a typed control message from a group event payload.
// Usage: var ctrl MyControlMsg; if group.ParseControl(evt.Payload, "listen", &ctrl) { ... }
func ParseControl(payload any, groupType string, dest any) bool {
	data, ok := ExtractControl(payload, groupType)
	if !ok {
		return false
	}
	return json.Unmarshal(data, dest) == nil
}

// ParseMembers extracts the member peer IDs from a "members" event payload,
// excluding the given selfID. Returns the list and whether new members joined
// compared to the previous set.
func ParseMembers(payload any, selfID string, previous []string) (members []string, hasNew bool) {
	mp, ok := payload.(map[string]any)
	if !ok {
		return nil, false
	}
	rawMembers, ok := mp["members"].([]any)
	if !ok {
		return nil, false
	}

	oldSet := make(map[string]bool, len(previous))
	for _, pid := range previous {
		oldSet[pid] = true
	}

	members = make([]string, 0, len(rawMembers))
	for _, member := range rawMembers {
		if mi, ok := member.(map[string]any); ok {
			if pid, ok := mi["peer_id"].(string); ok && pid != selfID {
				members = append(members, pid)
				if !oldSet[pid] {
					hasNew = true
				}
			}
		}
	}
	return members, hasNew
}

// StateStore provides JSON persistence for group type state.
// Each group type can use this to save/load its state across restarts.
type StateStore struct {
	dir string
}

// NewStateStore creates a state store that persists files in the given directory.
func NewStateStore(dir string) *StateStore {
	if dir == "" {
		return nil
	}
	return &StateStore{dir: dir}
}

// Save persists state as JSON for the given key (typically group_type or group-specific).
func (s *StateStore) Save(key string, v any) error {
	if s == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return writeFile(s.dir, key+".json", data)
}

// Load reads persisted state for the given key into dest.
// Returns false if the file doesn't exist or can't be parsed.
func (s *StateStore) Load(key string, dest any) bool {
	if s == nil {
		return false
	}
	data, err := readFile(s.dir, key+".json")
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dest) == nil
}
