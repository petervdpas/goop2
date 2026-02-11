package rendezvous

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
)

// LocalTemplateStore loads templates from a directory on disk and serves them
// directly from the rendezvous server — no microservice needed.
// All templates are free (no pricing, no credits, no registration gating).
type LocalTemplateStore struct {
	mu        sync.RWMutex
	templates map[string]localTpl // dir name -> cached template
}

type localTpl struct {
	meta  StoreMeta
	files map[string][]byte // relative path -> content
}

// NewLocalTemplateStore creates a store by loading templates from a single
// directory on disk. Each subdirectory with a manifest.json becomes a template.
// Returns nil if the directory doesn't exist or contains no templates.
func NewLocalTemplateStore(dir string) *LocalTemplateStore {
	ts := &LocalTemplateStore{
		templates: make(map[string]localTpl),
	}
	ts.loadDisk(dir)
	if len(ts.templates) == 0 {
		return nil
	}
	return ts
}

// loadDisk reads template subdirectories from dir.
func (ts *LocalTemplateStore) loadDisk(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("local templates: cannot read %s: %v", dir, err)
		}
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		tplDir := filepath.Join(dir, e.Name())
		manifestPath := filepath.Join(tplDir, "manifest.json")

		b, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var meta StoreMeta
		if err := json.Unmarshal(b, &meta); err != nil {
			log.Printf("local templates: bad manifest in %s: %v", e.Name(), err)
			continue
		}
		meta.Dir = e.Name()
		meta.Source = "store"

		files := make(map[string][]byte)
		filepath.WalkDir(tplDir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(tplDir, p)
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			files[rel] = data
			return nil
		})

		ts.templates[e.Name()] = localTpl{meta: meta, files: files}
		log.Printf("local templates: loaded %q from %s (%d files)", e.Name(), dir, len(files))
	}
}

// List returns metadata for all cached templates.
func (ts *LocalTemplateStore) List() []StoreMeta {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	out := make([]StoreMeta, 0, len(ts.templates))
	for _, t := range ts.templates {
		out = append(out, t.meta)
	}
	return out
}

// GetManifest returns metadata for a single template.
func (ts *LocalTemplateStore) GetManifest(dir string) (StoreMeta, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	t, ok := ts.templates[dir]
	if !ok {
		return StoreMeta{}, false
	}
	return t.meta, true
}

// WriteBundle writes a tar.gz archive of the template to w.
func (ts *LocalTemplateStore) WriteBundle(w io.Writer, dir string) error {
	ts.mu.RLock()
	t, ok := ts.templates[dir]
	ts.mu.RUnlock()

	if !ok {
		return os.ErrNotExist
	}

	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)

	for rel, data := range t.files {
		hdr := &tar.Header{
			Name: path.Join(dir, filepath.ToSlash(rel)),
			Mode: 0o644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			tw.Close()
			gw.Close()
			return err
		}
		if _, err := tw.Write(data); err != nil {
			tw.Close()
			gw.Close()
			return err
		}
	}

	if err := tw.Close(); err != nil {
		gw.Close()
		return err
	}
	return gw.Close()
}

// Count returns the number of loaded templates.
func (ts *LocalTemplateStore) Count() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.templates)
}
