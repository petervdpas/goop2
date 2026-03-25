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
			Type:    schema.SQLType(col.Type),
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

// LuaSchemaColumn mirrors the column definition from Lua scripts.
type LuaSchemaColumn struct {
	Name     string
	Type     string
	Key      bool
	Required bool
	Auto     bool
	Default  any
	Values   []schema.EnumValue
}

// CreateTableORMFromLua creates an ORM-managed table from Lua-provided columns.
func (d *DB) CreateTableORMFromLua(name string, columns []LuaSchemaColumn) error {
	schemaCols := make([]schema.Column, len(columns))
	for i, c := range columns {
		schemaCols[i] = schema.Column{
			Name:     c.Name,
			Type:     c.Type,
			Key:      c.Key,
			Required: c.Required,
			Auto:     c.Auto,
			Default:  c.Default,
			Values:   c.Values,
		}
	}
	tbl := &schema.Table{Name: name, Columns: schemaCols}
	if err := tbl.Validate(); err != nil {
		return err
	}
	return d.CreateTableORM(tbl)
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
			if col.Auto {
				continue
			}
			if col.Required && col.Default == nil {
				return fmt.Errorf("column %q is required", col.Name)
			}
			continue
		}
		if err := validateType(col.Name, col.Type, val); err != nil {
			return err
		}
		if col.Type == "enum" && len(col.Values) > 0 {
			s, _ := val.(string)
			valid := false
			for _, ev := range col.Values {
				if ev.Key == s {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("column %q: value %q is not a valid enum option", col.Name, s)
			}
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

// OrmInsert validates and inserts a row into an ORM-managed table.
// Adds system columns (_owner, _owner_email) automatically.
// Auto-generates values for guid columns when not provided.
func (d *DB) OrmInsert(tableName, ownerID, ownerEmail string, data map[string]any) (int64, error) {
	tbl, _ := d.GetSchema(tableName)
	if tbl != nil {
		for _, col := range tbl.Columns {
			if !col.Auto {
				continue
			}
			if v, exists := data[col.Name]; !exists || v == nil || v == "" {
				switch col.Type {
				case "guid":
					data[col.Name] = schema.GenerateGUID()
				case "datetime":
					data[col.Name] = schema.NowUTC()
				case "date":
					data[col.Name] = schema.NowDate()
				case "time":
					data[col.Name] = schema.NowTime()
				case "integer":
					var maxVal int64
					d.db.QueryRow(
						"SELECT COALESCE(MAX("+col.Name+"), 0) FROM "+tableName,
					).Scan(&maxVal)
					data[col.Name] = maxVal + 1
				}
			}
		}
	}
	if err := d.ValidateInsert(tableName, data); err != nil {
		return 0, err
	}
	return d.Insert(tableName, ownerID, ownerEmail, data)
}

// OrmGet retrieves a row by _id from an ORM-managed table.
func (d *DB) OrmGet(tableName string, id int64) (map[string]any, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tbl, err := d.GetSchema(tableName)
	if err != nil || tbl == nil {
		return nil, fmt.Errorf("orm: table %q has no schema", tableName)
	}

	allCols := append([]string{"_id", "_owner", "_owner_email", "_created_at", "_updated_at"}, tbl.ColumnNames()...)
	q := fmt.Sprintf("SELECT %s FROM %s WHERE _id = ?", joinCols(allCols), tableName)
	rows, err := d.db.Query(q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("orm: not found")
	}
	cols, _ := rows.Columns()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	result := make(map[string]any, len(cols))
	for i, col := range cols {
		result[col] = vals[i]
	}
	return result, nil
}

// OrmList retrieves all rows from an ORM-managed table.
func (d *DB) OrmList(tableName string, limit int) ([]map[string]any, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tbl, err := d.GetSchema(tableName)
	if err != nil || tbl == nil {
		return nil, fmt.Errorf("orm: table %q has no schema", tableName)
	}

	allCols := append([]string{"_id", "_owner", "_owner_email", "_created_at", "_updated_at"}, tbl.ColumnNames()...)
	q := fmt.Sprintf("SELECT %s FROM %s", joinCols(allCols), tableName)
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := d.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var results []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// OrmUpdate validates and updates a row by _id in an ORM-managed table.
func (d *DB) OrmUpdate(tableName string, id int64, data map[string]any) error {
	if err := d.ValidateInsert(tableName, data); err != nil {
		return err
	}
	return d.UpdateRow(tableName, id, data)
}

// OrmDelete deletes a row by _id from an ORM-managed table.
func (d *DB) OrmDelete(tableName string, id int64) error {
	return d.DeleteRow(tableName, id)
}

func joinCols(cols []string) string {
	result := ""
	for i, c := range cols {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result
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
	case "text", "guid", "datetime", "date", "time", "enum":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("column %q expects %s, got %T", name, colType, val)
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
