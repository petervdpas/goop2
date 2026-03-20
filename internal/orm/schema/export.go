package schema

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ExportTable reads the schema of an existing SQLite table and returns a Table.
func ExportTable(ctx context.Context, db *sql.DB, tableName string) (*Table, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info('%s');", tableName))
	if err != nil {
		return nil, fmt.Errorf("schema: table_info for %q: %w", tableName, err)
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("schema: scan table_info: %w", err)
		}
		col := Column{
			Name:     name,
			Type:     normalizeType(colType),
			Key:      pk > 0,
			Required: notNull != 0 && pk == 0,
		}
		if dflt.Valid {
			col.Default = parseDefault(dflt.String, col.Type)
		}
		columns = append(columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("schema: table %q not found or has no columns", tableName)
	}
	return &Table{Name: tableName, Columns: columns}, nil
}

// ExportAll reads the schema of all user tables in the database.
func ExportAll(ctx context.Context, db *sql.DB) ([]*Table, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;")
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var tables []*Table
	for _, name := range names {
		t, err := ExportTable(ctx, db, name)
		if err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, nil
}

func normalizeType(sqlType string) string {
	upper := strings.ToUpper(sqlType)
	switch {
	case strings.Contains(upper, "INT"):
		return "integer"
	case strings.Contains(upper, "REAL") || strings.Contains(upper, "FLOAT") || strings.Contains(upper, "DOUBLE"):
		return "real"
	case strings.Contains(upper, "BLOB"):
		return "blob"
	default:
		return "text"
	}
}

func parseDefault(raw, colType string) any {
	raw = strings.TrimSpace(raw)
	if strings.EqualFold(raw, "NULL") {
		return nil
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return strings.ReplaceAll(raw[1:len(raw)-1], "''", "'")
	}
	switch colType {
	case "integer":
		var v int64
		if _, err := fmt.Sscanf(raw, "%d", &v); err == nil {
			return v
		}
	case "real":
		var v float64
		if _, err := fmt.Sscanf(raw, "%g", &v); err == nil {
			return v
		}
	}
	return raw
}
