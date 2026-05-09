package store

import (
	"context"
	"github.com/mjl-/bstore"
)

// BstoreDB wraps *bstore.DB to implement the DB interface
type BstoreDB struct {
	db *bstore.DB
}

func (b *BstoreDB) Close() error { return b.db.Close() }

func (b *BstoreDB) Get(ctx context.Context, v any) error    { return b.db.Get(ctx, v) }
func (b *BstoreDB) Insert(ctx context.Context, v any) error { return b.db.Insert(ctx, v) }
func (b *BstoreDB) Update(ctx context.Context, v any) error { return b.db.Update(ctx, v) }
func (b *BstoreDB) Delete(ctx context.Context, v any) error { return b.db.Delete(ctx, v) }

type BstoreTx struct {
	tx *bstore.Tx
}

func (t *BstoreTx) Get(v any) error    { return t.tx.Get(v) }
func (t *BstoreTx) Insert(v any) error { return t.tx.Insert(v) }
func (t *BstoreTx) Update(v any) error { return t.tx.Update(v) }
func (t *BstoreTx) Delete(v any) error { return t.tx.Delete(v) }
func (t *BstoreTx) Rollback() error    { return t.tx.Rollback() }
func (t *BstoreTx) Commit() error      { return t.tx.Commit() }
func (t *BstoreTx) DBBackend() any     { return t.tx }

func (b *BstoreDB) Begin(ctx context.Context, writable bool) (Tx, error) {
	btx, err := b.db.Begin(ctx, writable)
	if err != nil {
		return nil, err
	}
	return &BstoreTx{tx: btx}, nil
}

func (b *BstoreDB) Write(ctx context.Context, fn func(tx Tx) error) error {
        return b.db.Write(ctx, func(btx *bstore.Tx) error {
                return fn(&BstoreTx{tx: btx})
        })
  }

// NewBstoreDB wraps an opend *bstore.DB as a DB interface
func NewBstoreDB(db *bstore.DB) DB {
	return &BstoreDB{db: db}
}

// RawBstore returns the underlying *bstore.DB
// Only for migration tooling - do not use in application code.
func (b *BstoreDB) RawBstore() *bstore.DB {
	return b.db
}

func (b *BstoreDB) Read(ctx context.Context, fn func(tx Tx) error) error {
	return b.db.Read(ctx, func(btx *bstore.Tx) error {
		return fn(&BstoreTx{tx: btx})
	})
}

func Query[T any](tx Tx) *TypedQuery[T] {
	switch t := tx.(type) {
	case *BstoreTx:
		return &TypedQuery[T]{bq: bstore.QueryTx[T](t.tx)}
	case *PgTx:
		return &TypedQuery[T]{pg: newPgQuery[T](t.ctx, t.tx)}
	default:
		panic("Query: unknown Tx backend")
	}
}

func QueryDB[T any](ctx context.Context, db DB) *TypedQuery[T] {
	switch d := db.(type) {
	case *BstoreDB:
		return &TypedQuery[T]{bq: bstore.QueryDB[T](ctx, d.db)}
	case *PgDB:
		// Uses a short transaction with SET LOCAL search_path so the query
		// targets the correct per-component schema. withSchema opens the tx
		// lazily when a terminal method (List/Count/Delete/…) is called.
		return &TypedQuery[T]{pg: &pgQuery[T]{
			ctx:     ctx,
			pool:    d.pool,
			schema:  d.searchPath,
			handler: lookupPgHandler[T](),
		}}
	default:
		panic("QueryDB: unknown DB backend")
	}
}
