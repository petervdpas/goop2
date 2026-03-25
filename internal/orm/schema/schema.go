package schema

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Row is the dynamic entity type — Go's equivalent of ExpandoObject.
type Row map[string]any

// Table describes a database table schema in a portable JSON format.
type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

// Column describes a single column in a table.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Key      bool   `json:"key,omitempty"`
	Required bool   `json:"required,omitempty"`
	Auto     bool   `json:"auto,omitempty"`
	Default  any    `json:"default,omitempty"`
}

// ParseTable parses a Table from JSON bytes.
func ParseTable(data []byte) (*Table, error) {
	var t Table
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("schema: parse failed: %w", err)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// JSON returns the table schema as indented JSON.
func (t *Table) JSON() ([]byte, error) {
	return json.MarshalIndent(t, "", "  ")
}

// Validate checks that the table schema is well-formed.
func (t *Table) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("schema: table name is required")
	}
	if len(t.Columns) == 0 {
		return fmt.Errorf("schema: table %q has no columns", t.Name)
	}
	hasKey := false
	seen := make(map[string]bool)
	for _, c := range t.Columns {
		if c.Name == "" {
			return fmt.Errorf("schema: table %q has a column with no name", t.Name)
		}
		if seen[c.Name] {
			return fmt.Errorf("schema: table %q has duplicate column %q", t.Name, c.Name)
		}
		seen[c.Name] = true
		if !validType(c.Type) {
			return fmt.Errorf("schema: table %q column %q has invalid type %q", t.Name, c.Name, c.Type)
		}
		if c.Key {
			hasKey = true
		}
	}
	if !hasKey {
		return fmt.Errorf("schema: table %q has no key column", t.Name)
	}
	return nil
}

// DDL returns the CREATE TABLE statement for this schema.
func (t *Table) DDL() string {
	var cols []string
	var keys []string
	for _, c := range t.Columns {
		def := c.Name + " " + sqlType(c.Type)
		if c.Key {
			keys = append(keys, c.Name)
		}
		if c.Required || c.Key {
			def += " NOT NULL"
		}
		if c.Default != nil && !c.Key {
			def += fmt.Sprintf(" DEFAULT %s", sqlDefault(c.Default))
		}
		cols = append(cols, def)
	}
	if len(keys) == 1 {
		for i, c := range t.Columns {
			if c.Key {
				cols[i] = c.Name + " " + sqlType(c.Type) + " PRIMARY KEY"
				if c.Type == "integer" && c.Auto {
					cols[i] += " AUTOINCREMENT"
				} else if c.Type == "integer" {
					cols[i] += " NOT NULL"
				}
				break
			}
		}
	} else {
		cols = append(cols, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(keys, ", ")))
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n  %s\n);", t.Name, strings.Join(cols, ",\n  "))
}

// ColumnNames returns all column names in order.
func (t *Table) ColumnNames() []string {
	names := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		names[i] = c.Name
	}
	return names
}

// KeyColumnNames returns the primary key column names.
func (t *Table) KeyColumnNames() []string {
	var keys []string
	for _, c := range t.Columns {
		if c.Key {
			keys = append(keys, c.Name)
		}
	}
	return keys
}

func validType(t string) bool {
	switch strings.ToLower(t) {
	case "text", "integer", "real", "blob", "guid", "datetime", "date", "time":
		return true
	}
	return false
}

// SQLType converts a portable schema type to its SQLite column type.
func SQLType(t string) string { return sqlType(t) }

func sqlType(t string) string {
	switch strings.ToLower(t) {
	case "text", "guid":
		return "TEXT"
	case "datetime":
		return "DATETIME"
	case "date":
		return "DATE"
	case "time":
		return "TIME"
	case "integer":
		return "INTEGER"
	case "real":
		return "REAL"
	case "blob":
		return "BLOB"
	default:
		return "TEXT"
	}
}

// GenerateGUID returns a new UUID v4 string.
func GenerateGUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NowUTC returns the current UTC time as an RFC3339 string.
func NowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// NowDate returns the current UTC date as YYYY-MM-DD.
func NowDate() string {
	return time.Now().UTC().Format("2006-01-02")
}

// NowTime returns the current UTC time as HH:MM:SS.
func NowTime() string {
	return time.Now().UTC().Format("15:04:05")
}

func sqlDefault(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(val, "'", "''"))
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("'%v'", v)
	}
}
