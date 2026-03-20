package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("orm: record not found")
var ErrInvalidKeyValueCount = errors.New("orm: invalid key value count")

// DBTX is the common interface satisfied by *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Repository provides typed CRUD operations for entity T.
type Repository[T any] struct {
	db    DBTX
	codec Codec[T]
}

func NewRepository[T any](db DBTX, codec Codec[T]) *Repository[T] {
	return &Repository[T]{db: db, codec: codec}
}

func (r *Repository[T]) TableName() string {
	return r.codec.TableName()
}

func (r *Repository[T]) CreateSchema(ctx context.Context) error {
	sp, ok := r.codec.(SchemaProvider)
	if !ok {
		return nil
	}
	for _, stmt := range sp.Schema() {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create schema for %s failed: %w", r.codec.TableName(), err)
		}
	}
	return nil
}

func (r *Repository[T]) Insert(ctx context.Context, entity T) error {
	q := BuildInsertSQL(r.codec.TableName(), r.codec.Columns())
	if _, err := r.db.ExecContext(ctx, q, r.codec.Values(entity)...); err != nil {
		return fmt.Errorf("insert into %s failed: %w", r.codec.TableName(), err)
	}
	return nil
}

func (r *Repository[T]) Update(ctx context.Context, entity T) error {
	q := BuildUpdateSQL(r.codec.TableName(), r.codec.Columns(), r.codec.KeyColumns())

	values := r.codec.Values(entity)
	nonKeyArgs := make([]any, 0, len(values))
	for i, col := range r.codec.Columns() {
		if contains(r.codec.KeyColumns(), col) {
			continue
		}
		nonKeyArgs = append(nonKeyArgs, values[i])
	}
	args := append(nonKeyArgs, r.codec.KeyValues(entity)...)

	result, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update %s failed: %w", r.codec.TableName(), err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository[T]) Upsert(ctx context.Context, entity T) error {
	var updateCols []string
	if p, ok := r.codec.(UpsertProvider); ok {
		updateCols = p.UpsertColumns()
	}
	q := BuildUpsertSQL(r.codec.TableName(), r.codec.Columns(), r.codec.KeyColumns(), updateCols)
	if _, err := r.db.ExecContext(ctx, q, r.codec.Values(entity)...); err != nil {
		return fmt.Errorf("upsert %s failed: %w", r.codec.TableName(), err)
	}
	return nil
}

func (r *Repository[T]) GetByKey(ctx context.Context, keyValues ...any) (T, error) {
	var zero T
	if len(keyValues) != len(r.codec.KeyColumns()) {
		return zero, ErrInvalidKeyValueCount
	}
	entity := r.codec.New()
	q := BuildSelectByKeySQL(r.codec.TableName(), r.codec.Columns(), r.codec.KeyColumns())
	if err := r.db.QueryRowContext(ctx, q, keyValues...).Scan(r.codec.ScanTargets(&entity)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return zero, ErrNotFound
		}
		return zero, fmt.Errorf("get by key from %s failed: %w", r.codec.TableName(), err)
	}
	return entity, nil
}

func (r *Repository[T]) ListAll(ctx context.Context) ([]T, error) {
	q := BuildSelectAllSQL(r.codec.TableName(), r.codec.Columns())
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all from %s failed: %w", r.codec.TableName(), err)
	}
	defer rows.Close()

	var result []T
	for rows.Next() {
		entity := r.codec.New()
		if err := rows.Scan(r.codec.ScanTargets(&entity)...); err != nil {
			return nil, fmt.Errorf("scan row from %s failed: %w", r.codec.TableName(), err)
		}
		result = append(result, entity)
	}
	return result, rows.Err()
}

func (r *Repository[T]) DeleteByKey(ctx context.Context, keyValues ...any) error {
	if len(keyValues) != len(r.codec.KeyColumns()) {
		return ErrInvalidKeyValueCount
	}
	q := BuildDeleteByKeySQL(r.codec.TableName(), r.codec.KeyColumns())
	result, err := r.db.ExecContext(ctx, q, keyValues...)
	if err != nil {
		return fmt.Errorf("delete from %s failed: %w", r.codec.TableName(), err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository[T]) ExistsByKey(ctx context.Context, keyValues ...any) (bool, error) {
	if len(keyValues) != len(r.codec.KeyColumns()) {
		return false, ErrInvalidKeyValueCount
	}
	q := BuildExistsByKeySQL(r.codec.TableName(), r.codec.KeyColumns())
	var exists bool
	if err := r.db.QueryRowContext(ctx, q, keyValues...).Scan(&exists); err != nil {
		return false, fmt.Errorf("exists in %s failed: %w", r.codec.TableName(), err)
	}
	return exists, nil
}

func (r *Repository[T]) Count(ctx context.Context) (int64, error) {
	q := BuildCountSQL(r.codec.TableName())
	var count int64
	if err := r.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("count from %s failed: %w", r.codec.TableName(), err)
	}
	return count, nil
}
