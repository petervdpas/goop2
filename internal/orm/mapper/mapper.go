package mapper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

type FieldMapping struct {
	Target    string   `json:"target"`
	Sources   []string `json:"sources,omitempty"`
	Transform string   `json:"transform,omitempty"`
	Args      []any    `json:"args,omitempty"`
	Constant  any      `json:"constant,omitempty"`
}

type Mapping struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	SourceTable string         `json:"source_table,omitempty"`
	TargetTable string         `json:"target_table,omitempty"`
	Fields      []FieldMapping `json:"fields"`
}

func (m *Mapping) Apply(row schema.Row) (schema.Row, error) {
	out := make(schema.Row, len(m.Fields))
	for _, f := range m.Fields {
		val, err := resolveField(f, row)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Target, err)
		}
		out[f.Target] = val
	}
	return out, nil
}

func (m *Mapping) ApplyMany(rows []schema.Row) ([]schema.Row, error) {
	results := make([]schema.Row, 0, len(rows))
	for i, row := range rows {
		out, err := m.Apply(row)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i, err)
		}
		results = append(results, out)
	}
	return results, nil
}

func resolveField(f FieldMapping, row schema.Row) (any, error) {
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

func (m *Mapping) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("mapping name is required")
	}
	if len(m.Fields) == 0 {
		return fmt.Errorf("mapping must have at least one field")
	}
	seen := make(map[string]bool, len(m.Fields))
	for i, f := range m.Fields {
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

func LoadFile(path string) (*Mapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Mapping
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &m, nil
}

func SaveFile(path string, m *Mapping) error {
	if err := m.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func LoadDir(dir string) ([]*Mapping, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var mappings []*Mapping
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		m, err := LoadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		mappings = append(mappings, m)
	}
	return mappings, nil
}

func Load(dir, name string) (*Mapping, error) {
	path := filepath.Join(dir, name+".json")
	return LoadFile(path)
}

func Save(dir string, m *Mapping) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, m.Name+".json")
	return SaveFile(path, m)
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
