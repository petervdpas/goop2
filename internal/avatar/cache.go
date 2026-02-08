package avatar

import (
	"os"
	"path/filepath"
	"sync"
)

// Cache stores remote peer avatars on disk, keyed by peerID + hash.
type Cache struct {
	mu  sync.RWMutex
	dir string // {peerDir}/cache/avatars
}

// NewCache creates an avatar cache in {peerDir}/cache/avatars.
func NewCache(peerDir string) *Cache {
	dir := filepath.Join(peerDir, "cache", "avatars")
	_ = os.MkdirAll(dir, 0755)
	return &Cache{dir: dir}
}

func (c *Cache) filePath(peerID string) string {
	return filepath.Join(c.dir, peerID+".png")
}

func (c *Cache) hashPath(peerID string) string {
	return filepath.Join(c.dir, peerID+".hash")
}

// Get returns the cached avatar for a peer, or nil if not cached or hash mismatch.
func (c *Cache) Get(peerID, hash string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if hash == "" {
		return nil, nil
	}

	stored, err := os.ReadFile(c.hashPath(peerID))
	if err != nil || string(stored) != hash {
		return nil, nil
	}

	data, err := os.ReadFile(c.filePath(peerID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

// Put stores a peer's avatar and its hash.
func (c *Cache) Put(peerID, hash string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.WriteFile(c.filePath(peerID), data, 0644); err != nil {
		return err
	}
	return os.WriteFile(c.hashPath(peerID), []byte(hash), 0644)
}

// HasHash returns true if the cached hash matches for this peer.
func (c *Cache) HasHash(peerID, hash string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	stored, err := os.ReadFile(c.hashPath(peerID))
	if err != nil {
		return false
	}
	return string(stored) == hash
}
