package store

import (
	"context"
	"iter"

	"github.com/mjl-/bstore"
)

// DB abstracts the database (bstore or PostgreSQL)
type DB interface {
	Read(ctx context.Context, fn func (tx Tx) error) error
	Write(ctx context.Context, fn func (tx Tx) error) error
	Begin(ctx context.Context, wriable bool) (Tx, error)
	Close() error
	Get(ctx context.Context, v any) error
	Insert(ctx context.Context, v any) error
	Update(ctx context.Context, v any) error
	Delete(ctx context.Context, v any) error
}

// Tx is an in-progress database transaction.
// Obtained via DB.Read/Write callbacks or DB.Begin
type Tx interface {
	Get(v any) error
	Insert(v any) error
	Update(v any) error
	Delete(v any) error
	Rollback() error
	Commit() error
	// DBBackend returns the underlying object (*bstore.Tx or pgx.Tx)
	// Used by Query[T] to dispatch to the correct query builder.
	DBBackend() any
}

var (
	ErrAbsent = bstore.ErrAbsent
	ErrUnique = bstore.ErrUnique
	StopForEach = bstore.StopForEach
)

// TypedQuery is a generic, backend-agnostic query builder.
// Obtained via Query[T](tx) or QueryDB[T](ctx, db)
// bq is non-nil for bstore backend; pg field added later for PostgreSQL.
type TypedQuery[T any] struct {
	bq *bstore.Query[T]
}

func (q *TypedQuery[T]) FilterEqual(field string, values... any) *TypedQuery[T] {
	if q.bq != nil {
		q.bq = q.bq.FilterEqual(field, values...)
	}
	return q
}

func (q *TypedQuery[T]) FilterNotEqual(field string, values ...any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterNotEqual(field, values...)
	}
	return q
}

func (q *TypedQuery[T]) FilterNonzero(v T) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterNonzero(v)
	}
	return q
}

func (q *TypedQuery[T]) FilterFn(fn func(T) bool) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterFn(fn)
	}
	return q
}

func (q *TypedQuery[T]) FilterGreater(field string, value any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterGreater(field, value)
	}
	return q
}

func (q *TypedQuery[T]) FilterGreaterEqual(field string, value any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterGreaterEqual(field, value)
	}
	return q
}

func (q *TypedQuery[T]) FilterLess(field string, value any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterLess(field, value)
	}
	return q
}

func (q *TypedQuery[T]) FilterLessEqual(field string, value any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterLessEqual(field, value)
	}
	return q
}

func (q *TypedQuery[T]) FilterIn(field string, value any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterIn(field, value)
	}
	return q
}

func (q *TypedQuery[T]) FilterID(id any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterID(id)
	}
	return q
}

func (q *TypedQuery[T]) FilterIDs(ids any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.FilterIDs(ids)
	}
	return q
}

func (q *TypedQuery[T]) SortAsc(fields ...string) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.SortAsc(fields...)
	}
	return q
}

func (q *TypedQuery[T]) SortDesc(fields ...string) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.SortDesc(fields...)
	}
	return q
}

func (q *TypedQuery[T]) Limit(n int) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.Limit(n)
	}
	return q
}

func (q *TypedQuery[T]) Gather(dst *[]T) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.Gather(dst)
	}
	return q
}

func (q *TypedQuery[T]) GatherIDs(ids any) *TypedQuery[T] {
	if q.bq != nil {
			q.bq = q.bq.GatherIDs(ids)
	}
	return q
}

func (q *TypedQuery[T]) List() ([]T, error) {
	if q.bq != nil {
			return q.bq.List()
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) Get() (T, error) {
	if q.bq != nil {
			return q.bq.Get()
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) Count() (int, error) {
	if q.bq != nil {
			return q.bq.Count()
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) Exists() (bool, error) {
	if q.bq != nil {
			return q.bq.Exists()
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) ForEach(fn func(T) error) error {
	if q.bq != nil {
			return q.bq.ForEach(fn)
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) Delete() (int, error) {
	if q.bq != nil {
			return q.bq.Delete()
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) UpdateFields(fields map[string]any) (int, error) {
	if q.bq != nil {
			return q.bq.UpdateFields(fields)
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) UpdateField(field string, value any) (int, error) {
	if q.bq != nil {
			return q.bq.UpdateField(field, value)
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) UpdateNonzero(v T) (int, error) {
	if q.bq != nil {
			return q.bq.UpdateNonzero(v)
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) IDs(idsptr any) error {
	if q.bq != nil {
			return q.bq.IDs(idsptr)
	}
	panic("TypedQuery: no backend set")
}

func (q *TypedQuery[T]) Err() error {
	if q.bq != nil {
			return q.bq.Err()
	}
	return nil
}

func (q *TypedQuery[T]) All() iter.Seq2[T, error] {
	if q.bq != nil {
		return q.bq.All()
	}
	panic("TypedQuery: no backend set")
}