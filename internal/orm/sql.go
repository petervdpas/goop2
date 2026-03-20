package orm

import (
	"fmt"
	"strings"
)

func BuildInsertSQL(table string, columns []string) string {
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		table, strings.Join(columns, ", "), placeholders(len(columns)))
}

func BuildUpdateSQL(table string, columns, keyColumns []string) string {
	setCols := make([]string, 0, len(columns))
	for _, col := range columns {
		if contains(keyColumns, col) {
			continue
		}
		setCols = append(setCols, fmt.Sprintf("%s = ?", col))
	}
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s;",
		table, strings.Join(setCols, ", "), whereClause(keyColumns))
}

func BuildUpsertSQL(table string, columns, keyColumns, updateColumns []string) string {
	if len(updateColumns) == 0 {
		updateColumns = columnsExcluding(columns, keyColumns)
	}
	assignments := make([]string, 0, len(updateColumns))
	for _, col := range updateColumns {
		if contains(keyColumns, col) {
			continue
		}
		assignments = append(assignments, fmt.Sprintf("%s = excluded.%s", col, col))
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(%s) DO UPDATE SET %s;",
		table, strings.Join(columns, ", "), placeholders(len(columns)),
		strings.Join(keyColumns, ", "), strings.Join(assignments, ", "))
}

func BuildSelectByKeySQL(table string, columns, keyColumns []string) string {
	return fmt.Sprintf("SELECT %s FROM %s WHERE %s;",
		strings.Join(columns, ", "), table, whereClause(keyColumns))
}

func BuildSelectAllSQL(table string, columns []string) string {
	return fmt.Sprintf("SELECT %s FROM %s;", strings.Join(columns, ", "), table)
}

func BuildDeleteByKeySQL(table string, keyColumns []string) string {
	return fmt.Sprintf("DELETE FROM %s WHERE %s;", table, whereClause(keyColumns))
}

func BuildExistsByKeySQL(table string, keyColumns []string) string {
	return fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s);", table, whereClause(keyColumns))
}

func BuildCountSQL(table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s;", table)
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func whereClause(columns []string) string {
	parts := make([]string, len(columns))
	for i, col := range columns {
		parts[i] = fmt.Sprintf("%s = ?", col)
	}
	return strings.Join(parts, " AND ")
}

func contains(columns []string, candidate string) bool {
	for _, col := range columns {
		if col == candidate {
			return true
		}
	}
	return false
}

func columnsExcluding(columns, excluded []string) []string {
	result := make([]string, 0, len(columns))
	for _, col := range columns {
		if !contains(excluded, col) {
			result = append(result, col)
		}
	}
	return result
}
