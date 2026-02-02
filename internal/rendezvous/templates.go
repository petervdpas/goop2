// internal/rendezvous/templates.go
package rendezvous

import (
	"archive/tar"
	"compress/gzip"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed all:storetemplates/corkboard all:storetemplates/quiz all:storetemplates/photobook
var defaultStoreFS embed.FS

// StoreMeta holds metadata for a store template.
// Mirrors sitetemplates.TemplateMeta to avoid importing the embed-heavy package.
type StoreMeta struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Icon        string                 `json:"icon"`
	Dir         string                 `json:"dir"`
	Source      string                 `json:"source"`
	Tables      map[string]TablePolicy `json:"tables,omitempty"`
}

// TablePolicy holds per-table configuration from a template manifest.
type TablePolicy struct {
	InsertPolicy string `json:"insert_policy"`
}

// TemplateStore loads templates from embedded defaults and an optional disk
// directory, caches all files in memory.
type TemplateStore struct {
	mu        sync.RWMutex
	templates map[string]storeTpl // dir name → cached template
}

type storeTpl struct {
	meta  StoreMeta
	files map[string][]byte // relative path → content
}

// NewTemplateStore creates a store pre-loaded with the embedded default
// templates. If dir is non-empty and exists, templates from disk are added
// (disk wins on name collision).
func NewTemplateStore(dir string) *TemplateStore {
	ts := &TemplateStore{
		templates: make(map[string]storeTpl),
	}

	// Load embedded defaults
	ts.loadEmbedded()

	// Overlay disk templates (if any)
	if dir != "" {
		ts.loadDisk(dir)
	}

	if len(ts.templates) == 0 {
		return nil
	}
	return ts
}

// loadEmbedded reads templates from the compiled-in storetemplates/ FS.
func (ts *TemplateStore) loadEmbedded() {
	entries, err := defaultStoreFS.ReadDir("storetemplates")
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		dirName := e.Name()
		prefix := path.Join("storetemplates", dirName)

		b, err := defaultStoreFS.ReadFile(path.Join(prefix, "manifest.json"))
		if err != nil {
			continue
		}

		var meta StoreMeta
		if err := json.Unmarshal(b, &meta); err != nil {
			continue
		}
		meta.Dir = dirName
		meta.Source = "store"

		files := make(map[string][]byte)
		fs.WalkDir(defaultStoreFS, prefix, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel := strings.TrimPrefix(p, prefix+"/")
			data, err := defaultStoreFS.ReadFile(p)
			if err != nil {
				return nil
			}
			files[rel] = data
			return nil
		})

		ts.templates[dirName] = storeTpl{meta: meta, files: files}
		log.Printf("template store: loaded embedded %q (%d files)", dirName, len(files))
	}
}

// loadDisk reads templates from a directory on disk. Overwrites any embedded
// template with the same dir name.
func (ts *TemplateStore) loadDisk(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("template store: cannot read %s: %v", dir, err)
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
			log.Printf("template store: bad manifest in %s: %v", e.Name(), err)
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

		ts.templates[e.Name()] = storeTpl{meta: meta, files: files}
		log.Printf("template store: loaded disk %q (%d files)", e.Name(), len(files))
	}
}

// List returns metadata for all cached templates.
func (ts *TemplateStore) List() []StoreMeta {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	out := make([]StoreMeta, 0, len(ts.templates))
	for _, t := range ts.templates {
		out = append(out, t.meta)
	}
	return out
}

// GetManifest returns metadata for a single template.
func (ts *TemplateStore) GetManifest(dir string) (StoreMeta, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	t, ok := ts.templates[dir]
	if !ok {
		return StoreMeta{}, false
	}
	return t.meta, true
}

// WriteBundle writes a tar.gz archive of the template to w.
func (ts *TemplateStore) WriteBundle(w io.Writer, dir string) error {
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
			Name: filepath.Join(dir, rel),
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
