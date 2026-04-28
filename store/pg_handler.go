package store

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgxExecutor abstracts the subset of pgx surface needed by pgQuery.
// Both *pgxpool.Pool, pgx.Tx, and *pgxpool.Conn satisfy it.
type pgxExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// pgTypeHandler holds per-type metadata required by pgQuery and the
// PgTx CRUD methods to build SQL against the table for type T.
// Concrete handlers are registered from pg_types.go (Step 9).
type pgTypeHandler[T any] struct {
	// Table is the unqualified table name (schema set by PgTx via search_path).
	Table string

	// PKColumn is the SQL column for the primary key field.
	PKColumn string

	// PKField is the Go struct field name for the primary key (used by FilterID/IDs).
	PKField string

	// Columns lists every selectable column in the order ScanRow expects.
	Columns []string

	// FieldToColumn maps a Go struct field name (incl. embedded flat fields)
	// to its SQL column name.
	FieldToColumn map[string]string

	// ScanRow populates dst from a row that was selected with Columns.
	ScanRow func(row pgx.Row, dst *T) error

	// ScanRows is the slice variant; convenience for List/ForEach paths.
	// Implementations typically loop calling ScanRow on each row.
	ScanRows func(rows pgx.Rows, dst *[]T) error

	// Insert inserts v into the table. May mutate v (e.g. assign generated ID).
	Insert func(ctx context.Context, exec pgxExecutor, v *T) error

	// Update updates v by primary key. Returns rows affected.
	Update func(ctx context.Context, exec pgxExecutor, v *T) (int64, error)

	// Delete deletes v by primary key. Returns rows affected.
	Delete func(ctx context.Context, exec pgxExecutor, v *T) (int64, error)

	// GetByPK loads a row by the PK already populated on v.
	GetByPK func(ctx context.Context, exec pgxExecutor, v *T) error

	// NonzeroFields returns {column: value} for every non-zero field of v,
	// used by FilterNonzero and UpdateNonzero. Embedded flat fields are
	// expanded; zero-valued nested structs contribute nothing.
	NonzeroFields func(v T) map[string]any

	// PKValue extracts the primary key value from v.
	PKValue func(v T) any

	// SetPK assigns a primary key value back onto v (used after auto-id insert).
	SetPK func(v *T, id any)
}

var (
	pgHandlerRegistry    sync.Map // reflect.Type -> *pgTypeHandler[T]
	pgRuntimeOpsRegistry sync.Map // reflect.Type -> pgTxOps (runtime-typed CRUD adapter)
)

// registerPgType records a handler for type T. Called from pg_types.go init().
// It also installs a non-generic pgTxOps adapter so PgTx CRUD methods (which
// receive `any`) can dispatch by reflect.TypeOf(v).Elem().
func registerPgType[T any](h *pgTypeHandler[T]) {
	var zero T
	typ := reflect.TypeOf(zero)
	pgHandlerRegistry.Store(typ, h)
	pgRuntimeOpsRegistry.Store(typ, pgTxOps{
		get: func(ctx context.Context, exec pgxExecutor, v any) error {
			return h.GetByPK(ctx, exec, v.(*T))
		},
		insert: func(ctx context.Context, exec pgxExecutor, v any) error {
			return h.Insert(ctx, exec, v.(*T))
		},
		update: func(ctx context.Context, exec pgxExecutor, v any) error {
			n, err := h.Update(ctx, exec, v.(*T))
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrAbsent
			}
			return nil
		},
		delete: func(ctx context.Context, exec pgxExecutor, v any) error {
			n, err := h.Delete(ctx, exec, v.(*T))
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrAbsent
			}
			return nil
		},
	})
}

// lookupPgHandler returns the handler for T, panicking if none is registered.
// Panic is appropriate: a missing handler is a programmer error, not a runtime
// condition the caller can recover from.
func lookupPgHandler[T any]() *pgTypeHandler[T] {
	var zero T
	v, ok := pgHandlerRegistry.Load(reflect.TypeOf(zero))
	if !ok {
		panic(fmt.Sprintf("pg: no type handler registered for %T", zero))
	}
	return v.(*pgTypeHandler[T])
}

// pgRuntimeLookup returns the runtime-typed CRUD adapter for the dynamic type
// of v. v must be a pointer to a registered struct type.
func pgRuntimeLookup(v any) (pgTxOps, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return pgTxOps{}, fmt.Errorf("expected non-nil pointer to struct, got %T", v)
	}
	typ := rv.Elem().Type()
	ops, ok := pgRuntimeOpsRegistry.Load(typ)
	if !ok {
		return pgTxOps{}, fmt.Errorf("no type handler registered for %s", typ)
	}
	return ops.(pgTxOps), nil
}
