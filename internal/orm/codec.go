package orm

// Codec maps a Go type T to a database table.
type Codec[T any] interface {
	TableName() string
	Columns() []string
	KeyColumns() []string

	New() T
	Values(entity T) []any
	KeyValues(entity T) []any
	ScanTargets(entity *T) []any
}

// SchemaProvider is optionally implemented by codecs that define their own DDL.
type SchemaProvider interface {
	Schema() []string
}

// UpsertProvider is optionally implemented by codecs that specify which
// columns to update on conflict.
type UpsertProvider interface {
	UpsertColumns() []string
}
