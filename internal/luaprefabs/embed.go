// internal/luaprefabs/embed.go

package luaprefabs

import (
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"strings"
)

//go:embed all:help all:starter all:dice all:examples
var prefabFS embed.FS

// PrefabMeta holds prefab metadata from manifest.json.
type PrefabMeta struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Dir         string   `json:"dir"`
	ScriptNames []string `json:"-"` // e.g. ["help", "ping"] — populated by List()
}

// List returns metadata for all available prefabs.
func List() ([]PrefabMeta, error) {
	entries, err := prefabFS.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var out []PrefabMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := readManifest(e.Name())
		if err != nil {
			continue // skip broken prefabs
		}
		m.Dir = e.Name()
		m.ScriptNames = scriptNames(e.Name())
		out = append(out, m)
	}
	return out, nil
}

// scriptNames returns the list of script names (without .lua) in a prefab.
func scriptNames(dir string) []string {
	var names []string
	fs.WalkDir(prefabFS, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base := strings.TrimPrefix(p, dir+"/")
		if strings.HasSuffix(base, ".lua") {
			names = append(names, strings.TrimSuffix(base, ".lua"))
		}
		return nil
	})
	return names
}

// Scripts returns all .lua files in a prefab directory.
// Returns a map of filename → file content.
func Scripts(dir string) (map[string][]byte, error) {
	out := make(map[string][]byte)

	err := fs.WalkDir(prefabFS, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base := strings.TrimPrefix(p, dir+"/")
		// skip manifest
		if base == "manifest.json" {
			return nil
		}
		// only .lua files
		if !strings.HasSuffix(base, ".lua") {
			return nil
		}
		data, err := prefabFS.ReadFile(p)
		if err != nil {
			return err
		}
		out[base] = data
		return nil
	})

	return out, err
}

// GetMeta returns the manifest metadata for a specific prefab directory.
func GetMeta(dir string) (PrefabMeta, error) {
	return readManifest(dir)
}

func readManifest(dir string) (PrefabMeta, error) {
	var m PrefabMeta
	b, err := prefabFS.ReadFile(path.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}
