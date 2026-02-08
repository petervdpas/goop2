
package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrOutsideRoot = errors.New("path outside root")
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("conflict")
	ErrImagePath   = errors.New("image files must be placed in the images/ folder")
)

// imageExts lists extensions that are considered image files.
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".svg": true, ".ico": true, ".bmp": true,
}

type Store struct {
	root string // absolute path to peer's editable root (e.g. /.../peerA/site)
}

func NewStore(peerFolder string, siteRel string) (*Store, error) {
	if siteRel == "" {
		siteRel = "site"
	}
	var joined string
	if filepath.IsAbs(siteRel) {
		joined = filepath.Clean(siteRel)
	} else {
		joined = filepath.Join(peerFolder, siteRel)
	}
	root, err := filepath.Abs(joined)
	if err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

type FileInfo struct {
	Path  string // root-relative, forward slashes
	Size  int64
	ETag  string // sha256:<hex>
	Mod   int64  // unix seconds
	IsDir bool
}

func (s *Store) RootAbs() string { return s.root }

func (s *Store) EnsureRoot() error {
	return os.MkdirAll(s.root, 0o755)
}

// Read returns bytes + etag.
func (s *Store) Read(ctx context.Context, rel string) ([]byte, string, error) {
	abs, err := s.cleanAbs(rel)
	if err != nil {
		return nil, "", err
	}

	b, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	return b, etagBytes(b), nil
}

// Write writes atomically. If ifMatch is non-empty, it must match current etag.
// Hardened: refuses to write if any parent component is a file, or if target path is a directory.
func (s *Store) Write(ctx context.Context, rel string, data []byte, ifMatch string) (string, error) {
	abs, err := s.cleanAbs(rel)
	if err != nil {
		return "", err
	}

	// Image files must live under images/
	ext := strings.ToLower(path.Ext(rel))
	clean := strings.TrimPrefix(filepath.ToSlash(rel), "/")
	if imageExts[ext] && !strings.HasPrefix(clean, "images/") {
		return "", ErrImagePath
	}

	// optional optimistic concurrency
	if ifMatch != "" {
		_, curETag, err := s.Read(ctx, rel)
		if err != nil && err != ErrNotFound {
			return "", err
		}
		if err == nil && curETag != ifMatch {
			return "", ErrConflict
		}
		if err == ErrNotFound && ifMatch != "none" {
			return "", ErrConflict
		}
	}

	// If the target exists and is a directory, refuse (file/dir collision)
	if st, err := os.Stat(abs); err == nil && st.IsDir() {
		return "", ErrConflict
	}

	// Ensure parent directory exists, but refuse if any parent component is a file.
	dir := filepath.Dir(abs)
	if err := s.mkdirAllChecked(dir); err != nil {
		return "", err
	}

	// Create temp file in same dir for atomic rename.
	f, err := os.CreateTemp(dir, ".goop-*")
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

	// Re-check symlink resolution now that parents exist.
	if p, err := filepath.EvalSymlinks(tmp); err == nil {
		rootClean := filepath.Clean(s.root)
		rootPrefix := rootClean + string(filepath.Separator)
		if p != rootClean && !strings.HasPrefix(p, rootPrefix) {
			_ = os.Remove(tmp)
			return "", ErrOutsideRoot
		}
	}

	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}

	return etagBytes(data), nil
}

func (s *Store) Delete(ctx context.Context, rel string) error {
	abs, err := s.cleanAbs(rel)
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

// Mkdir creates a directory (and parents) under the root.
// NOTE: relDir must be a DIRECTORY path. If you have "dir" that might be a FILE,
// use NormalizeDir() or MkdirUnder().
func (s *Store) Mkdir(ctx context.Context, relDir string) error {
	abs, err := s.cleanAbs(relDir)
	if err != nil {
		return err
	}
	return s.mkdirAllChecked(abs)
}

// NormalizeDir coerces an incoming "dir" value into a directory path.
// If dir points to a file (e.g. "index.html"), it returns its parent ("").
// Output is root-relative and forward slashes, no leading slash.
func (s *Store) NormalizeDir(ctx context.Context, rel string) (string, error) {
	rel = normalizeRelPath(rel)
	if rel == "" {
		return "", nil
	}

	abs, err := s.cleanAbs(rel)
	if err != nil {
		return "", err
	}

	// If it exists and is a file, parent dir.
	if st, statErr := os.Stat(abs); statErr == nil && !st.IsDir() {
		parent := normalizeRelPath(path.Dir(rel))
		if parent == "." {
			parent = ""
		}
		return parent, nil
	}

	// Heuristic: if basename contains a dot and it DOES NOT exist as a directory, treat as file path => parent.
	// This keeps "pages.v1/" possible if it actually exists as a directory.
	if strings.Contains(path.Base(rel), ".") {
		if st, statErr := os.Stat(abs); statErr != nil {
			parent := normalizeRelPath(path.Dir(rel))
			if parent == "." {
				parent = ""
			}
			return parent, nil
		} else if st != nil && !st.IsDir() {
			parent := normalizeRelPath(path.Dir(rel))
			if parent == "." {
				parent = ""
			}
			return parent, nil
		}
	}

	return rel, nil
}

// MkdirUnder creates folder "name" under "dir".
// dir may accidentally point at a file; we normalize it.
// name must be a single path segment (no slashes).
func (s *Store) MkdirUnder(ctx context.Context, dir string, name string) (string, error) {
	dir, err := s.NormalizeDir(ctx, dir)
	if err != nil {
		return "", err
	}

	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		return "", errors.New("empty folder name")
	}
	if strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return "", errors.New("folder name must not contain slashes")
	}
	if name == "." || name == ".." {
		return "", errors.New("invalid folder name")
	}

	target := normalizeRelPath(path.Join(dir, name))
	if err := s.Mkdir(ctx, target); err != nil {
		return "", err
	}
	return target, nil
}

// DeletePath deletes a file or directory. If recursive is true, directories are removed recursively.
func (s *Store) DeletePath(ctx context.Context, rel string, recursive bool) error {
	abs, err := s.cleanAbs(rel)
	if err != nil {
		return err
	}

	st, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}

	if st.IsDir() {
		if recursive {
			return os.RemoveAll(abs)
		}
		return os.Remove(abs) // fails if not empty (desired)
	}

	if err := os.Remove(abs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// Rename renames/moves a file or folder within the root.
func (s *Store) Rename(ctx context.Context, fromRel, toRel string) error {
	fromAbs, err := s.cleanAbs(fromRel)
	if err != nil {
		return err
	}
	toAbs, err := s.cleanAbs(toRel)
	if err != nil {
		return err
	}

	// Ensure target parent exists; refuse if parent chain contains a file.
	if err := s.mkdirAllChecked(filepath.Dir(toAbs)); err != nil {
		return err
	}

	if err := os.Rename(fromAbs, toAbs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) List(ctx context.Context, relDir string) ([]FileInfo, error) {
	absDir, err := s.cleanAbs(relDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, err
		}

		rel := filepath.ToSlash(filepath.Join(relDir, e.Name()))
		fi := FileInfo{
			Path:  strings.TrimPrefix(rel, "/"),
			Size:  info.Size(),
			Mod:   info.ModTime().Unix(),
			IsDir: info.IsDir(),
		}

		// Only compute ETag for files
		if !fi.IsDir {
			b, err := os.ReadFile(filepath.Join(absDir, e.Name()))
			if err == nil {
				fi.ETag = etagBytes(b)
			}
		}
		out = append(out, fi)
	}
	return out, nil
}

// TreeItem is a single node in a flattened tree listing.
type TreeItem struct {
	Path  string // root-relative, forward slashes, no leading slash
	IsDir bool
	Depth int // 0 = root level under site/
}

// ListTree returns a flattened tree under relDir ("" means root).
func (s *Store) ListTree(ctx context.Context, relDir string) ([]TreeItem, error) {
	absDir, err := s.cleanAbs(relDir)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(absDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	baseRel := normalizeRelPath(relDir)
	baseRel = strings.TrimSuffix(baseRel, "/")

	var out []TreeItem
	err = filepath.WalkDir(absDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == absDir {
			return nil
		}

		relLocal, err := filepath.Rel(absDir, p)
		if err != nil {
			return err
		}
		relLocal = filepath.ToSlash(relLocal)

		var rel string
		if baseRel == "" {
			rel = relLocal
		} else {
			rel = baseRel + "/" + relLocal
		}
		rel = strings.TrimPrefix(rel, "/")

		depth := 0
		if relLocal != "" {
			depth = strings.Count(relLocal, "/")
		}

		out = append(out, TreeItem{
			Path:  rel,
			IsDir: d.IsDir(),
			Depth: depth,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Hierarchical ordering:
	// - parent before children
	// - within same folder, directories before files
	// - lexicographic by segment
	sort.Slice(out, func(i, j int) bool {
		a := out[i]
		b := out[j]

		as := strings.Split(a.Path, "/")
		bs := strings.Split(b.Path, "/")

		n := len(as)
		if len(bs) < n {
			n = len(bs)
		}
		for k := 0; k < n; k++ {
			if as[k] != bs[k] {
				return as[k] < bs[k]
			}
		}

		// One is prefix of the other => parent first
		if len(as) != len(bs) {
			return len(as) < len(bs)
		}

		// Same folder: dirs before files
		if a.IsDir != b.IsDir {
			return a.IsDir
		}

		return a.Path < b.Path
	})

	return out, nil
}

// --- safety boundary ---

func (s *Store) cleanAbs(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "/")
	rel = filepath.FromSlash(rel)

	abs := filepath.Clean(filepath.Join(s.root, rel))

	rootClean := filepath.Clean(s.root)
	rootPrefix := rootClean + string(filepath.Separator)
	if abs != rootClean && !strings.HasPrefix(abs, rootPrefix) {
		return "", ErrOutsideRoot
	}

	// prevent symlink escape on existing paths
	if p, err := filepath.EvalSymlinks(abs); err == nil {
		if p != rootClean && !strings.HasPrefix(p, rootPrefix) {
			return "", ErrOutsideRoot
		}
	}

	return abs, nil
}

// mkdirAllChecked creates directories but refuses if any component in the path is a file.
func (s *Store) mkdirAllChecked(absDir string) error {
	absDir = filepath.Clean(absDir)
	rootClean := filepath.Clean(s.root)

	// Only operate within root
	if absDir != rootClean && !strings.HasPrefix(absDir, rootClean+string(filepath.Separator)) {
		return ErrOutsideRoot
	}

	// Walk from root to absDir; if any existing component is a file => conflict.
	rel, err := filepath.Rel(rootClean, absDir)
	if err != nil {
		return err
	}
	cur := rootClean
	if rel == "." {
		return nil
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == "" {
			continue
		}
		cur = filepath.Join(cur, part)

		if st, err := os.Stat(cur); err == nil {
			if !st.IsDir() {
				return ErrConflict
			}
			continue
		} else if errors.Is(err, os.ErrNotExist) {
			if mkErr := os.Mkdir(cur, 0o755); mkErr != nil && !errors.Is(mkErr, os.ErrExist) {
				return mkErr
			}
			continue
		} else {
			return err
		}
	}
	return nil
}

func normalizeRelPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.ReplaceAll(p, `\`, "/")
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return strings.TrimPrefix(p, "/")
}

func etagBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
