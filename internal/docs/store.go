// internal/docs/store.go
// Shared document storage for group file sharing.
// Each group gets a subdirectory under the shared/ root.
// Files are namespaced by owner peer - only the local peer's files live here.

package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const MaxFileSize = 50 * 1024 * 1024 // 50 MB

var (
	ErrOutsideRoot = errors.New("path outside root")
	ErrTooLarge    = errors.New("file exceeds 50 MB limit")
	ErrNotFound    = errors.New("not found")
	ErrBadName     = errors.New("invalid filename")
)

// DocInfo describes a shared document.
type DocInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"` // unix seconds
	Hash    string `json:"hash"`     // sha256:<hex>
}

// Store manages the local shared documents directory.
type Store struct {
	root string // absolute path, e.g. <peer-dir>/shared
}

// NewStore creates a new document store rooted at <peerDir>/shared.
func NewStore(peerDir string) (*Store, error) {
	root := filepath.Join(peerDir, "shared")
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: abs}, nil
}

// Save writes a file to the group's shared directory.
func (s *Store) Save(groupID, filename string, data []byte) (string, error) {
	if err := validateFilename(filename); err != nil {
		return "", err
	}
	if len(data) > MaxFileSize {
		return "", ErrTooLarge
	}

	dir := s.groupDir(groupID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	abs, err := s.cleanAbs(groupID, filename)
	if err != nil {
		return "", err
	}

	// Atomic write via temp file + rename
	f, err := os.CreateTemp(dir, ".goop-doc-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}

	if _, err := f.Write(data); err != nil {
		cleanup()
		return "", err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}

	return hashBytes(data), nil
}

// Read returns the file contents and hash.
func (s *Store) Read(groupID, filename string) ([]byte, string, error) {
	if err := validateFilename(filename); err != nil {
		return nil, "", err
	}
	abs, err := s.cleanAbs(groupID, filename)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	return data, hashBytes(data), nil
}

// Delete removes a file from the group's shared directory.
func (s *Store) Delete(groupID, filename string) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	abs, err := s.cleanAbs(groupID, filename)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// List returns all files shared in a group.
func (s *Store) List(groupID string) ([]DocInfo, error) {
	dir := s.groupDir(groupID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []DocInfo{}, nil
		}
		return nil, err
	}

	out := make([]DocInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Skip temp files
		if strings.HasPrefix(e.Name(), ".goop-doc-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Compute hash
		abs := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		out = append(out, DocInfo{
			Name:    e.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
			Hash:    hashBytes(data),
		})
	}
	return out, nil
}

// groupDir returns the absolute path of a group's shared directory.
func (s *Store) groupDir(groupID string) string {
	// Sanitize groupID to prevent path traversal
	safe := sanitizeSegment(groupID)
	return filepath.Join(s.root, safe)
}

// cleanAbs resolves the absolute path and verifies it stays within the root.
func (s *Store) cleanAbs(groupID, filename string) (string, error) {
	dir := s.groupDir(groupID)
	safe := sanitizeSegment(filename)
	abs := filepath.Clean(filepath.Join(dir, safe))

	// Verify within root
	rootPrefix := filepath.Clean(s.root) + string(filepath.Separator)
	if !strings.HasPrefix(abs, rootPrefix) {
		return "", ErrOutsideRoot
	}

	// Verify within group dir
	dirPrefix := filepath.Clean(dir) + string(filepath.Separator)
	if !strings.HasPrefix(abs, dirPrefix) {
		return "", ErrOutsideRoot
	}

	return abs, nil
}

// validateFilename checks that a filename is safe.
func validateFilename(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return ErrBadName
	}
	if strings.ContainsAny(name, "/\\") {
		return ErrBadName
	}
	if strings.HasPrefix(name, ".") {
		return ErrBadName
	}
	// Check for filesystem-unsafe chars
	for _, ch := range name {
		if ch < 32 {
			return ErrBadName
		}
	}
	return nil
}

// sanitizeSegment removes dangerous characters from a path segment.
func sanitizeSegment(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "\\", "")
	s = path.Clean(s)
	if s == "." || s == "" {
		return "_"
	}
	return s
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// HasFiles checks if a group has any shared files (used by fs.WalkDir).
func (s *Store) HasFiles(groupID string) bool {
	dir := s.groupDir(groupID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".goop-doc-") {
			return true
		}
	}
	return false
}

// ListGroups returns group IDs that have shared documents.
func (s *Store) ListGroups() ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var groups []string
	for _, e := range entries {
		if e.IsDir() && s.HasFiles(e.Name()) {
			groups = append(groups, e.Name())
		}
	}
	return groups, nil
}
