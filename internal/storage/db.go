package storage

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var safeIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validIdent checks that a SQL identifier (table/column name) is safe.
func validIdent(s string) bool {
	return len(s) > 0 && len(s) <= 64 && safeIdentRe.MatchString(s)
}

// DB wraps a SQLite database for a peer
type DB struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// Open opens or creates a SQLite database in the given directory
func Open(configDir string) (*DB, error) {
	dbPath := filepath.Join(configDir, "data.db")

	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable foreign keys and WAL mode for better concurrency
	if _, err := db.Exec(`
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure database: %w", err)
	}

	// Create internal metadata table
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS _meta (
			key   TEXT PRIMARY KEY,
			value TEXT
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create meta table: %w", err)
	}

	// Create tables registry
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS _tables (
			name           TEXT PRIMARY KEY,
			schema         TEXT NOT NULL,
			insert_policy  TEXT DEFAULT 'owner',
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create tables registry: %w", err)
	}

	// Migration: add insert_policy column if missing (existing databases)
	db.Exec(`ALTER TABLE _tables ADD COLUMN insert_policy TEXT DEFAULT 'owner'`)

	// Create groups table
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS _groups (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			app_type    TEXT DEFAULT '',
			max_members INTEGER DEFAULT 0,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create groups table: %w", err)
	}

	// Migration: add host_joined column if missing (existing databases)
	db.Exec(`ALTER TABLE _groups ADD COLUMN host_joined INTEGER DEFAULT 0`)
	// Migration: add volatile column if missing (existing databases)
	db.Exec(`ALTER TABLE _groups ADD COLUMN volatile INTEGER DEFAULT 0`)

	// Create group subscriptions table
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS _group_subscriptions (
			host_peer_id  TEXT NOT NULL,
			group_id      TEXT NOT NULL,
			group_name    TEXT DEFAULT '',
			app_type      TEXT DEFAULT '',
			role          TEXT DEFAULT 'member',
			subscribed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (host_peer_id, group_id)
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create group subscriptions table: %w", err)
	}

	// Migration: add max_members to subscriptions if missing (existing databases)
	db.Exec(`ALTER TABLE _group_subscriptions ADD COLUMN max_members INTEGER DEFAULT 0`)
	// Migration: add volatile to subscriptions if missing (existing databases)
	db.Exec(`ALTER TABLE _group_subscriptions ADD COLUMN volatile INTEGER DEFAULT 0`)

	// Create group members table — persists the last known member list per group
	// so peers can browse each other's files even when the host is offline.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS _group_members (
			group_id TEXT NOT NULL,
			peer_id  TEXT NOT NULL,
			PRIMARY KEY (group_id, peer_id)
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create group members table: %w", err)
	}

	return &DB{db: db, path: dbPath}, nil
}

// Close closes the database
func (d *DB) Close() error {
	return d.db.Close()
}

// Path returns the database file path
func (d *DB) Path() string {
	return d.path
}

// Exec executes a query without returning rows
func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.db.Exec(query, args...)
}

// Query executes a query that returns rows
func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.db.Query(query, args...)
}

// QueryRow executes a query that returns a single row
func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.db.QueryRow(query, args...)
}

// CreateTable creates a new user table with automatic owner tracking
func (d *DB) CreateTable(name string, columns []ColumnDef) error {
	if !validIdent(name) {
		return fmt.Errorf("invalid table name: %s", name)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Build column definitions
	colSQL := ""
	for i, col := range columns {
		if !validIdent(col.Name) {
			return fmt.Errorf("invalid column name: %s", col.Name)
		}
		if i > 0 {
			colSQL += ", "
		}
		colSQL += fmt.Sprintf("%s %s", col.Name, col.Type)
		if col.NotNull {
			colSQL += " NOT NULL"
		}
		if col.Default != "" {
			colSQL += " DEFAULT " + col.Default
		}
	}

	// Add system columns
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			_id INTEGER PRIMARY KEY AUTOINCREMENT,
			_owner TEXT NOT NULL,
			_owner_email TEXT DEFAULT '',
			_created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			_updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			%s
		)
	`, name, colSQL)

	if _, err := d.db.Exec(createSQL); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	// Register in tables registry
	if _, err := d.db.Exec(`
		INSERT OR REPLACE INTO _tables (name, schema) VALUES (?, ?)
	`, name, createSQL); err != nil {
		return fmt.Errorf("register table: %w", err)
	}

	return nil
}

// ListTables returns all user-created tables
func (d *DB) ListTables() ([]TableInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`SELECT name, COALESCE(insert_policy, 'owner'), created_at FROM _tables ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.InsertPolicy, &t.CreatedAt); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

// GetTableInsertPolicy returns the insert_policy for a table.
// Returns "owner" as default if the table is not found.
func (d *DB) GetTableInsertPolicy(table string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var policy string
	err := d.db.QueryRow(`SELECT COALESCE(insert_policy, 'owner') FROM _tables WHERE name = ?`, table).Scan(&policy)
	if err != nil {
		return "owner", err
	}
	return policy, nil
}

// SetTableInsertPolicy updates the insert_policy for a table.
func (d *DB) SetTableInsertPolicy(table, policy string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE _tables SET insert_policy = ? WHERE name = ?`, policy, table)
	return err
}

// DescribeTable returns column metadata for a table using PRAGMA table_info
func (d *DB) DescribeTable(table string) ([]ColumnInfo, error) {
	if !validIdent(table) {
		return nil, fmt.Errorf("invalid table name: %s", table)
	}
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var c ColumnInfo
		var nn int
		var pk int
		if err := rows.Scan(&c.CID, &c.Name, &c.Type, &nn, &c.Default, &pk); err != nil {
			return nil, err
		}
		c.NotNull = nn != 0
		c.PrimaryKey = pk != 0
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// Insert inserts a row into a table
func (d *DB) Insert(table string, ownerID string, ownerEmail string, data map[string]any) (int64, error) {
	if !validIdent(table) {
		return 0, fmt.Errorf("invalid table name: %s", table)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	cols := "_owner, _owner_email"
	placeholders := "?, ?"
	args := []any{ownerID, ownerEmail}

	for col, val := range data {
		if !validIdent(col) {
			return 0, fmt.Errorf("invalid column name: %s", col)
		}
		cols += ", " + col
		placeholders += ", ?"
		args = append(args, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, cols, placeholders)
	result, err := d.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateRow updates specific columns of a row by _id
func (d *DB) UpdateRow(table string, rowID int64, data map[string]any) error {
	if !validIdent(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	setClauses := "_updated_at = CURRENT_TIMESTAMP"
	args := []any{}
	for col, val := range data {
		if !validIdent(col) {
			return fmt.Errorf("invalid column name: %s", col)
		}
		setClauses += fmt.Sprintf(", %s = ?", col)
		args = append(args, val)
	}
	args = append(args, rowID)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE _id = ?", table, setClauses)
	_, err := d.db.Exec(query, args...)
	return err
}

// UpdateRowOwner updates a row only if it belongs to the given owner.
func (d *DB) UpdateRowOwner(table string, rowID int64, ownerID string, data map[string]any) error {
	if !validIdent(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	setClauses := "_updated_at = CURRENT_TIMESTAMP"
	args := []any{}
	for col, val := range data {
		if !validIdent(col) {
			return fmt.Errorf("invalid column name: %s", col)
		}
		setClauses += fmt.Sprintf(", %s = ?", col)
		args = append(args, val)
	}
	args = append(args, rowID, ownerID)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE _id = ? AND _owner = ?", table, setClauses)
	res, err := d.db.Exec(query, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("row not found or not owned by caller")
	}
	return nil
}

// DeleteRow deletes a row by _id
func (d *DB) DeleteRow(table string, rowID int64) error {
	if !validIdent(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE _id = ?", table), rowID)
	return err
}

// DeleteRowOwner deletes a row only if it belongs to the given owner.
func (d *DB) DeleteRowOwner(table string, rowID int64, ownerID string) error {
	if !validIdent(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	res, err := d.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE _id = ? AND _owner = ?", table), rowID, ownerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("row not found or not owned by caller")
	}
	return nil
}

// AddColumn adds a column to an existing table
func (d *DB) AddColumn(table string, col ColumnDef) error {
	if !validIdent(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	if !validIdent(col.Name) {
		return fmt.Errorf("invalid column name: %s", col.Name)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col.Name, col.Type)
	if col.NotNull {
		stmt += " NOT NULL DEFAULT ''"
	}
	if col.Default != "" {
		stmt += " DEFAULT " + col.Default
	}

	_, err := d.db.Exec(stmt)
	return err
}

// DropColumn removes a column from an existing table
func (d *DB) DropColumn(table, column string) error {
	if !validIdent(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	if !validIdent(column) {
		return fmt.Errorf("invalid column name: %s", column)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column))
	return err
}

// RenameTable renames a table and updates the registry
func (d *DB) RenameTable(oldName, newName string) error {
	if !validIdent(oldName) {
		return fmt.Errorf("invalid table name: %s", oldName)
	}
	if !validIdent(newName) {
		return fmt.Errorf("invalid table name: %s", newName)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, err := d.db.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", oldName, newName)); err != nil {
		return fmt.Errorf("rename table: %w", err)
	}
	if _, err := d.db.Exec("UPDATE _tables SET name = ? WHERE name = ?", newName, oldName); err != nil {
		return fmt.Errorf("update registry: %w", err)
	}
	return nil
}

// DeleteTable drops a table and removes it from the registry
func (d *DB) DeleteTable(table string) error {
	if !validIdent(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, err := d.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
		return fmt.Errorf("drop table: %w", err)
	}
	if _, err := d.db.Exec("DELETE FROM _tables WHERE name = ?", table); err != nil {
		return fmt.Errorf("unregister table: %w", err)
	}
	return nil
}

// SelectOpts holds optional query parameters for Select
type SelectOpts struct {
	Table   string
	Columns []string
	Where   string
	Args    []any
	Limit   int
	Offset  int
}

// Select queries rows from a table
func (d *DB) Select(table string, columns []string, where string, args ...any) ([]map[string]any, error) {
	return d.SelectPaged(SelectOpts{
		Table:   table,
		Columns: columns,
		Where:   where,
		Args:    args,
	})
}

// SelectPaged queries rows with optional LIMIT/OFFSET
func (d *DB) SelectPaged(opts SelectOpts) ([]map[string]any, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	colStr := "*"
	if len(opts.Columns) > 0 {
		colStr = ""
		for i, c := range opts.Columns {
			if i > 0 {
				colStr += ", "
			}
			colStr += c
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s", colStr, opts.Table)
	if opts.Where != "" {
		query += " WHERE " + opts.Where
	}
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	}

	rows, err := d.db.Query(query, opts.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	for rows.Next() {
		values := make([]any, len(colNames))
		valuePtrs := make([]any, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any)
		for i, col := range colNames {
			switch v := values[i].(type) {
			case []byte:
				// Convert []byte to string so JSON encoding works correctly
				// (otherwise []byte becomes base64-encoded)
				row[col] = string(v)
			case time.Time:
				// Normalize time.Time to SQLite-style string so JS gets a
				// consistent format ("2006-01-02 15:04:05") instead of
				// Go's RFC 3339 which already contains "T" and "Z".
				row[col] = v.UTC().Format("2006-01-02 15:04:05")
			default:
				row[col] = v
			}
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

// ColumnDef defines a table column
type ColumnDef struct {
	Name    string `json:"name"`
	Type    string `json:"type"`    // TEXT, INTEGER, REAL, BLOB
	NotNull bool   `json:"not_null"`
	Default string `json:"default"`
}

// ColumnInfo describes a column as returned by PRAGMA table_info
type ColumnInfo struct {
	CID        int     `json:"cid"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	NotNull    bool    `json:"not_null"`
	Default    *string `json:"default"`
	PrimaryKey bool    `json:"pk"`
}

// TableInfo contains table metadata
type TableInfo struct {
	Name         string `json:"name"`
	InsertPolicy string `json:"insert_policy"`
	CreatedAt    string `json:"created_at"`
}

// ── Lua read-only query methods ──

const (
	luaMaxRows        = 1000
	luaMaxResultBytes = 1 * 1024 * 1024 // 1MB
)

// validateReadOnly checks that a SQL query is read-only.
func validateReadOnly(query string) error {
	q := strings.TrimSpace(query)
	upper := strings.ToUpper(q)

	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return fmt.Errorf("only SELECT queries are allowed")
	}

	// Reject multiple statements (allow trailing semicolons)
	trimmed := strings.TrimRight(q, "; \t\n\r")
	if strings.Contains(trimmed, ";") {
		return fmt.Errorf("multiple SQL statements not allowed")
	}

	return nil
}

// LuaQuery executes a read-only parameterized query for Lua scripts.
// Returns at most 1000 rows. Total result size is capped at 1MB serialized JSON.
func (d *DB) LuaQuery(query string, args ...any) ([]map[string]any, error) {
	if err := validateReadOnly(query); err != nil {
		return nil, err
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	totalSize := 0

	for rows.Next() {
		if len(results) >= luaMaxRows {
			break
		}

		values := make([]any, len(colNames))
		valuePtrs := make([]any, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any)
		for i, col := range colNames {
			switch v := values[i].(type) {
			case []byte:
				row[col] = string(v)
			case time.Time:
				row[col] = v.UTC().Format("2006-01-02 15:04:05")
			default:
				row[col] = v
			}
		}

		rowJSON, _ := json.Marshal(row)
		totalSize += len(rowJSON)
		if totalSize > luaMaxResultBytes {
			return nil, fmt.Errorf("result set exceeds 1MB limit")
		}

		results = append(results, row)
	}

	return results, rows.Err()
}

// LuaExec executes a parameterized write statement (INSERT, UPDATE, DELETE)
// for Lua data functions. Returns the number of rows affected.
func (d *DB) LuaExec(stmt string, args ...any) (int64, error) {
	if err := validateWrite(stmt); err != nil {
		return 0, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	res, err := d.db.Exec(stmt, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// validateWrite ensures only INSERT, UPDATE, DELETE statements are allowed.
func validateWrite(stmt string) error {
	q := strings.TrimSpace(stmt)
	upper := strings.ToUpper(q)

	allowed := false
	for _, prefix := range []string{"INSERT", "UPDATE", "DELETE", "REPLACE"} {
		if strings.HasPrefix(upper, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("only INSERT, UPDATE, DELETE statements are allowed")
	}

	// Reject multiple statements
	trimmed := strings.TrimRight(q, "; \t\n\r")
	if strings.Contains(trimmed, ";") {
		return fmt.Errorf("multiple SQL statements not allowed")
	}

	return nil
}

// LuaScalar executes a read-only parameterized query and returns a single value.
func (d *DB) LuaScalar(query string, args ...any) (any, error) {
	if err := validateReadOnly(query); err != nil {
		return nil, err
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	var result any
	err := d.db.QueryRow(query, args...).Scan(&result)
	if err != nil {
		return nil, err
	}

	if b, ok := result.([]byte); ok {
		return string(b), nil
	}
	return result, nil
}

// DumpSQL produces a SQL script (CREATE TABLE + INSERT INTO) for all user tables.
// The output can recreate the full schema and data when executed on a fresh database.
func (d *DB) DumpSQL() (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 1. Get all user table names from _tables registry
	rows, err := d.db.Query(`SELECT name FROM _tables ORDER BY name`)
	if err != nil {
		return "", fmt.Errorf("query _tables: %w", err)
	}
	var tableNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return "", err
		}
		tableNames = append(tableNames, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}

	var buf strings.Builder

	for _, table := range tableNames {
		// 2. PRAGMA table_info → build CREATE TABLE
		infoRows, err := d.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return "", fmt.Errorf("table_info %s: %w", table, err)
		}

		type colMeta struct {
			name     string
			typ      string
			notNull  bool
			dflt     *string
			pk       bool
		}
		var cols []colMeta
		for infoRows.Next() {
			var cid, nn, pk int
			var name, typ string
			var dflt *string
			if err := infoRows.Scan(&cid, &name, &typ, &nn, &dflt, &pk); err != nil {
				infoRows.Close()
				return "", err
			}
			cols = append(cols, colMeta{name: name, typ: typ, notNull: nn != 0, dflt: dflt, pk: pk != 0})
		}
		infoRows.Close()

		buf.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n", table))
		for i, c := range cols {
			buf.WriteString("  ")
			buf.WriteString(c.name)
			if c.typ != "" {
				buf.WriteString(" ")
				buf.WriteString(c.typ)
			}
			if c.pk {
				buf.WriteString(" PRIMARY KEY")
				// Check if it's an AUTOINCREMENT column (INTEGER PRIMARY KEY)
				if strings.EqualFold(c.typ, "INTEGER") {
					buf.WriteString(" AUTOINCREMENT")
				}
			}
			if c.notNull && !c.pk {
				buf.WriteString(" NOT NULL")
			}
			if c.dflt != nil {
				buf.WriteString(" DEFAULT ")
				buf.WriteString(*c.dflt)
			}
			if i < len(cols)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(");\n")

		// 3. SELECT * → build INSERT INTO statements
		colNames := make([]string, len(cols))
		for i, c := range cols {
			colNames[i] = c.name
		}

		dataRows, err := d.db.Query(fmt.Sprintf("SELECT * FROM %s ORDER BY _id", table))
		if err != nil {
			return "", fmt.Errorf("select %s: %w", table, err)
		}

		dataCols, _ := dataRows.Columns()
		for dataRows.Next() {
			values := make([]any, len(dataCols))
			ptrs := make([]any, len(dataCols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := dataRows.Scan(ptrs...); err != nil {
				dataRows.Close()
				return "", err
			}

			buf.WriteString(fmt.Sprintf("INSERT INTO %s (%s) VALUES (", table, strings.Join(dataCols, ", ")))
			for i, v := range values {
				if i > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(sqlEscapeValue(v))
			}
			buf.WriteString(");\n")
		}
		dataRows.Close()

		buf.WriteString("\n")
	}

	return buf.String(), nil
}

// sqlEscapeValue converts a Go value to a SQL literal for use in INSERT statements.
func sqlEscapeValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case string:
		return "'" + strings.ReplaceAll(val, "'", "''") + "'"
	case []byte:
		return "X'" + hex.EncodeToString(val) + "'"
	case time.Time:
		return "'" + val.UTC().Format("2006-01-02 15:04:05") + "'"
	case bool:
		if val {
			return "1"
		}
		return "0"
	default:
		s := fmt.Sprintf("%v", val)
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
}
