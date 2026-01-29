// internal/storage/db.go
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

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
			name        TEXT PRIMARY KEY,
			schema      TEXT NOT NULL,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create tables registry: %w", err)
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
func (d *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.db.Exec(query, args...)
}

// Query executes a query that returns rows
func (d *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.db.Query(query, args...)
}

// QueryRow executes a query that returns a single row
func (d *DB) QueryRow(query string, args ...interface{}) *sql.Row {
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
			_created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
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

	rows, err := d.db.Query(`SELECT name, created_at FROM _tables ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
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
func (d *DB) Insert(table string, ownerID string, data map[string]interface{}) (int64, error) {
	if !validIdent(table) {
		return 0, fmt.Errorf("invalid table name: %s", table)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	cols := "_owner"
	placeholders := "?"
	args := []interface{}{ownerID}

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
func (d *DB) UpdateRow(table string, rowID int64, data map[string]interface{}) error {
	if !validIdent(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	setClauses := ""
	args := []interface{}{}
	i := 0
	for col, val := range data {
		if !validIdent(col) {
			return fmt.Errorf("invalid column name: %s", col)
		}
		if i > 0 {
			setClauses += ", "
		}
		setClauses += fmt.Sprintf("%s = ?", col)
		args = append(args, val)
		i++
	}
	args = append(args, rowID)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE _id = ?", table, setClauses)
	_, err := d.db.Exec(query, args...)
	return err
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

// Select queries rows from a table
func (d *DB) Select(table string, columns []string, where string, args ...interface{}) ([]map[string]interface{}, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	colStr := "*"
	if len(columns) > 0 {
		colStr = ""
		for i, c := range columns {
			if i > 0 {
				colStr += ", "
			}
			colStr += c
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s", colStr, table)
	if where != "" {
		query += " WHERE " + where
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(colNames))
		valuePtrs := make([]interface{}, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range colNames {
			// Convert []byte to string so JSON encoding works correctly
			// (otherwise []byte becomes base64-encoded)
			if b, ok := values[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = values[i]
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
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}
