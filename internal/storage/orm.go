package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/petervdpas/goop2/internal/orm"
	"github.com/petervdpas/goop2/internal/orm/schema"
)

// CreateTableORM creates a table from a JSON schema definition and stores
// the schema in _orm_schemas. The table gets the same system columns as
// classic tables (_id, _owner, _owner_email, _created_at, _updated_at).
func (d *DB) CreateTableORM(tbl *schema.Table) error {
	if !validIdent(tbl.Name) {
		return fmt.Errorf("invalid table name: %s", tbl.Name)
	}
	for _, col := range tbl.Columns {
		if !validIdent(col.Name) {
			return fmt.Errorf("invalid column name: %s", col.Name)
		}
	}

	columns := make([]ColumnDef, len(tbl.Columns))
	for i, col := range tbl.Columns {
		columns[i] = ColumnDef{
			Name:    col.Name,
			Type:    col.Type,
			NotNull: col.Required || col.Key,
		}
		if col.Default != nil {
			columns[i].Default = sqlDefault(col.Default)
		}
	}

	if err := d.CreateTable(tbl.Name, columns); err != nil {
		return err
	}

	schemaJSON, err := json.Marshal(tbl)
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.db.Exec(
		`INSERT OR REPLACE INTO _orm_schemas (table_name, schema_json) VALUES (?, ?)`,
		tbl.Name, string(schemaJSON),
	); err != nil {
		return fmt.Errorf("store orm schema: %w", err)
	}
	return nil
}

// GetSchema returns the stored JSON schema for an ORM-managed table.
// Returns nil if the table is classic (no stored schema).
func (d *DB) GetSchema(tableName string) (*schema.Table, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var schemaJSON string
	err := d.db.QueryRow(
		`SELECT schema_json FROM _orm_schemas WHERE table_name = ?`, tableName,
	).Scan(&schemaJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var tbl schema.Table
	if err := json.Unmarshal([]byte(schemaJSON), &tbl); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	return &tbl, nil
}

// IsORM returns true if the table was created via the ORM schema path.
func (d *DB) IsORM(tableName string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var n int
	d.db.QueryRow(
		`SELECT COUNT(*) FROM _orm_schemas WHERE table_name = ?`, tableName,
	).Scan(&n)
	return n > 0
}

// DeleteSchemaORM removes the stored schema when an ORM table is deleted.
func (d *DB) DeleteSchemaORM(tableName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.db.Exec(`DELETE FROM _orm_schemas WHERE table_name = ?`, tableName)
}

// RenameSchemaORM updates the stored schema when a table is renamed.
func (d *DB) RenameSchemaORM(oldName, newName string) {
	tbl, err := d.GetSchema(oldName)
	if err != nil || tbl == nil {
		return
	}
	tbl.Name = newName
	schemaJSON, err := json.Marshal(tbl)
	if err != nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.db.Exec(`DELETE FROM _orm_schemas WHERE table_name = ?`, oldName)
	d.db.Exec(`INSERT INTO _orm_schemas (table_name, schema_json) VALUES (?, ?)`, newName, string(schemaJSON))
}

// ValidateInsert checks that the data map matches the ORM schema types.
// Returns nil if the table is classic (no schema) or all values are valid.
func (d *DB) ValidateInsert(tableName string, data map[string]any) error {
	tbl, err := d.GetSchema(tableName)
	if err != nil || tbl == nil {
		return nil
	}

	for _, col := range tbl.Columns {
		val, exists := data[col.Name]
		if !exists || val == nil {
			if col.Required && col.Default == nil {
				return fmt.Errorf("column %q is required", col.Name)
			}
			continue
		}
		if err := validateType(col.Name, col.Type, val); err != nil {
			return err
		}
	}
	return nil
}

// OrmRepository returns a typed ORM repository for the given table.
// Returns nil if the table has no stored schema.
func (d *DB) OrmRepository(tableName string) (*orm.Repository[schema.Row], error) {
	tbl, err := d.GetSchema(tableName)
	if err != nil {
		return nil, err
	}
	if tbl == nil {
		return nil, nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return orm.NewRepository[schema.Row](d.db, schema.NewCodec(tbl)), nil
}

// ExportSchema exports the schema of an existing table (ORM or classic) as
// a portable JSON schema. For ORM tables, returns the stored schema. For
// classic tables, reads the schema from PRAGMA table_info.
func (d *DB) ExportSchema(ctx context.Context, tableName string) (*schema.Table, error) {
	tbl, err := d.GetSchema(tableName)
	if err != nil {
		return nil, err
	}
	if tbl != nil {
		return tbl, nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return schema.ExportTable(ctx, d.db, tableName)
}

func validateType(name, colType string, val any) error {
	switch colType {
	case "integer":
		switch val.(type) {
		case int, int64, float64, json.Number:
			return nil
		}
		return fmt.Errorf("column %q expects integer, got %T", name, val)
	case "real":
		switch val.(type) {
		case float64, int, int64, json.Number:
			return nil
		}
		return fmt.Errorf("column %q expects real, got %T", name, val)
	case "text":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("column %q expects text, got %T", name, val)
		}
	case "blob":
		switch val.(type) {
		case []byte, string:
			return nil
		}
		return fmt.Errorf("column %q expects blob, got %T", name, val)
	}
	return nil
}

func sqlDefault(v any) string {
	switch val := v.(type) {
	case string:
		return "'" + val + "'"
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case bool:
		if val {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("'%v'", v)
	}
}
