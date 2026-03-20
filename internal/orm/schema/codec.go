package schema

import (
	"strings"
)

// Codec implements orm.Codec[Row] from a Table schema definition.
type Codec struct {
	table *Table
}

// NewCodec creates a dynamic codec from a table schema.
func NewCodec(t *Table) *Codec {
	return &Codec{table: t}
}

func (c *Codec) TableName() string    { return c.table.Name }
func (c *Codec) Columns() []string    { return c.table.ColumnNames() }
func (c *Codec) KeyColumns() []string { return c.table.KeyColumnNames() }
func (c *Codec) New() Row             { return make(Row) }

func (c *Codec) Schema() []string {
	return []string{c.table.DDL()}
}

func (c *Codec) Values(entity Row) []any {
	vals := make([]any, len(c.table.Columns))
	for i, col := range c.table.Columns {
		vals[i] = entity[col.Name]
	}
	return vals
}

func (c *Codec) KeyValues(entity Row) []any {
	keys := c.table.KeyColumnNames()
	vals := make([]any, len(keys))
	for i, k := range keys {
		vals[i] = entity[k]
	}
	return vals
}

func (c *Codec) ScanTargets(entity *Row) []any {
	if *entity == nil {
		*entity = make(Row)
	}
	targets := make([]any, len(c.table.Columns))
	for i, col := range c.table.Columns {
		switch strings.ToLower(col.Type) {
		case "integer":
			targets[i] = &intScanner{row: *entity, key: col.Name}
		case "real":
			targets[i] = &floatScanner{row: *entity, key: col.Name}
		case "blob":
			targets[i] = &blobScanner{row: *entity, key: col.Name}
		default:
			targets[i] = &textScanner{row: *entity, key: col.Name}
		}
	}
	return targets
}

func (c *Codec) UpsertColumns() []string {
	var cols []string
	for _, col := range c.table.Columns {
		if !col.Key {
			cols = append(cols, col.Name)
		}
	}
	return cols
}
