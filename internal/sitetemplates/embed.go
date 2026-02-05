// internal/sitetemplates/embed.go

package sitetemplates

import (
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"strings"
)

//go:embed all:blog all:enquete all:clubhouse all:tictactoe
var templateFS embed.FS

// TablePolicy holds per-table configuration from a template manifest.
type TablePolicy struct {
	InsertPolicy string `json:"insert_policy"` // "owner", "email", "open", "public"
}

// TemplateMeta holds template metadata from manifest.json
type TemplateMeta struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Icon        string                 `json:"icon"`
	Dir         string                 `json:"dir"`    // directory name (e.g. "corkboard")
	Tables      map[string]TablePolicy `json:"tables"` // table name → policy
}

// List returns metadata for all available templates.
func List() ([]TemplateMeta, error) {
	entries, err := templateFS.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var out []TemplateMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := readManifest(e.Name())
		if err != nil {
			continue // skip broken templates
		}
		m.Dir = e.Name()
		out = append(out, m)
	}
	return out, nil
}

// Schema returns the SQL schema for a template.
func Schema(dir string) (string, error) {
	b, err := templateFS.ReadFile(path.Join(dir, "schema.sql"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SiteFiles returns all site files (non-manifest, non-schema) for a template.
// Returns a map of relative path → file content.
func SiteFiles(dir string) (map[string][]byte, error) {
	out := make(map[string][]byte)

	err := fs.WalkDir(templateFS, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base := strings.TrimPrefix(p, dir+"/")
		// skip manifest and schema
		if base == "manifest.json" || base == "schema.sql" {
			return nil
		}
		data, err := templateFS.ReadFile(p)
		if err != nil {
			return err
		}
		out[base] = data
		return nil
	})

	return out, err
}

// GetMeta returns the manifest metadata for a specific template directory.
func GetMeta(dir string) (TemplateMeta, error) {
	return readManifest(dir)
}

func readManifest(dir string) (TemplateMeta, error) {
	var m TemplateMeta
	b, err := templateFS.ReadFile(path.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}
