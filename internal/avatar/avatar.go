// internal/avatar/avatar.go
package avatar

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store manages the local avatar file and provides hash-based cache invalidation.
type Store struct {
	mu      sync.RWMutex
	peerDir string
	hash    string // cached hash of current avatar (empty = no avatar)
}

// NewStore creates an avatar store rooted at peerDir.
// It computes the initial hash if avatar.png exists.
func NewStore(peerDir string) *Store {
	s := &Store{peerDir: peerDir}
	s.hash = s.computeHash()
	return s
}

func (s *Store) avatarPath() string {
	return filepath.Join(s.peerDir, "avatar.png")
}

// Hash returns the current avatar hash (16 hex chars), or "" if no avatar.
func (s *Store) Hash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hash
}

// Read returns the avatar bytes, or nil if no avatar exists.
func (s *Store) Read() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.avatarPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

// Write stores a new avatar and updates the cached hash.
func (s *Store) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(s.avatarPath(), data, 0644); err != nil {
		return err
	}
	s.hash = hashBytes(data)
	return nil
}

// Delete removes the avatar file and clears the cached hash.
func (s *Store) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.avatarPath())
	if os.IsNotExist(err) {
		err = nil
	}
	s.hash = ""
	return err
}

// PeerDir returns the peer directory path (used by p2p avatar handler).
func (s *Store) PeerDir() string {
	return s.peerDir
}

func (s *Store) computeHash() string {
	data, err := os.ReadFile(s.avatarPath())
	if err != nil {
		return ""
	}
	return hashBytes(data)
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8]) // 16 hex chars
}

// InitialsSVG generates a deterministic initials-based SVG avatar.
// label is the display name, email is used as fallback for color hashing.
func InitialsSVG(label, email string) []byte {
	initials := extractInitials(label)
	color := deterministicColor(label + email)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">
  <rect width="256" height="256" rx="128" fill="%s"/>
  <text x="128" y="128" dy=".35em" text-anchor="middle"
        font-family="sans-serif" font-size="100" font-weight="600" fill="#fff">%s</text>
</svg>`, color, initials)
	return []byte(svg)
}

func extractInitials(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "?"
	}
	parts := strings.Fields(label)
	if len(parts) >= 2 {
		return strings.ToUpper(string([]rune(parts[0])[:1]) + string([]rune(parts[1])[:1]))
	}
	r := []rune(parts[0])
	if len(r) >= 2 {
		return strings.ToUpper(string(r[:2]))
	}
	return strings.ToUpper(string(r[:1]))
}

var palette = []string{
	"#e74c3c", "#e67e22", "#f1c40f", "#2ecc71", "#1abc9c",
	"#3498db", "#9b59b6", "#e91e63", "#00bcd4", "#ff5722",
	"#607d8b", "#795548", "#8bc34a", "#673ab7",
}

func deterministicColor(s string) string {
	h := sha256.Sum256([]byte(s))
	idx := int(h[0]) % len(palette)
	return palette[idx]
}
