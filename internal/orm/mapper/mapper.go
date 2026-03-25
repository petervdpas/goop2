package mapper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

type FieldTransform struct {
	Target    string   `json:"target"`
	Sources   []string `json:"sources,omitempty"`
	Transform string   `json:"transform,omitempty"`
	Args      []any    `json:"args,omitempty"`
	Constant  any      `json:"constant,omitempty"`
}

type DataEndpoint struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
}

type Transformation struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Source      DataEndpoint     `json:"source"`
	Target      DataEndpoint     `json:"target"`
	Fields      []FieldTransform `json:"fields"`
}

func (t *Transformation) Apply(row schema.Row) (schema.Row, error) {
	out := make(schema.Row, len(t.Fields))
	for _, f := range t.Fields {
		val, err := resolveField(f, row)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Target, err)
		}
		out[f.Target] = val
	}
	return out, nil
}

func (t *Transformation) ApplyMany(rows []schema.Row) ([]schema.Row, error) {
	results := make([]schema.Row, 0, len(rows))
	for i, row := range rows {
		out, err := t.Apply(row)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i, err)
		}
		results = append(results, out)
	}
	return results, nil
}

func resolveField(f FieldTransform, row schema.Row) (any, error) {
	if f.Constant != nil {
		return f.Constant, nil
	}

	var values []any
	for _, src := range f.Sources {
		values = append(values, row[src])
	}

	if f.Transform == "" {
		if len(values) == 1 {
			return values[0], nil
		}
		if len(values) == 0 {
			return nil, nil
		}
		return values, nil
	}

	fn, ok := transforms[f.Transform]
	if !ok {
		return nil, fmt.Errorf("unknown transform %q", f.Transform)
	}
	return fn(values, f.Args)
}

func (t *Transformation) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("transformation name is required")
	}
	if len(t.Fields) == 0 {
		return fmt.Errorf("transformation must have at least one field")
	}
	seen := make(map[string]bool, len(t.Fields))
	for i, f := range t.Fields {
		if strings.TrimSpace(f.Target) == "" {
			return fmt.Errorf("field %d: target is required", i)
		}
		if seen[f.Target] {
			return fmt.Errorf("duplicate target field %q", f.Target)
		}
		seen[f.Target] = true
		if f.Constant == nil && len(f.Sources) == 0 && f.Transform != "now" && f.Transform != "guid" && f.Transform != "datetime" && f.Transform != "date" && f.Transform != "time" {
			return fmt.Errorf("field %q: needs sources, constant, or a generating transform", f.Target)
		}
		if f.Transform != "" {
			if _, ok := transforms[f.Transform]; !ok {
				return fmt.Errorf("field %q: unknown transform %q", f.Target, f.Transform)
			}
		}
	}
	return nil
}

func LoadFile(path string) (*Transformation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Transformation
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &t, nil
}

func SaveFile(path string, t *Transformation) error {
	if err := t.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func LoadDir(dir string) ([]*Transformation, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var results []*Transformation
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		t, err := LoadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		results = append(results, t)
	}
	return results, nil
}

func Load(dir, name string) (*Transformation, error) {
	path := filepath.Join(dir, name+".json")
	return LoadFile(path)
}

func Save(dir string, t *Transformation) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, t.Name+".json")
	return SaveFile(path, t)
}

func Delete(dir, name string) error {
	path := filepath.Join(dir, name+".json")
	return os.Remove(path)
}

func TransformNames() []string {
	names := make([]string, 0, len(transforms))
	for k := range transforms {
		names = append(names, k)
	}
	return names
}
