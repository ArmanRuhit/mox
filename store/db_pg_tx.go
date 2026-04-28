package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgDB is the PostgreSQL backend for DB. It wraps a pgx connection pool and
// a "search_path" string (typically a single PG schema name like
// "account_alice" or "auth") so all queries target the right tables.
//
// Construction is the responsibility of pool wiring (Step 11) — this file
// only implements the DB/Tx surface against an already-built pool.
type PgDB struct {
	pool       *pgxpool.Pool
	searchPath string
}

// NewPgDB wraps a pool with a schema search_path. The caller owns the pool;
// PgDB.Close closes it.
func NewPgDB(pool *pgxpool.Pool, schema string) *PgDB {
	return &PgDB{pool: pool, searchPath: schema}
}

// Pool exposes the underlying pgxpool.Pool. For tooling only (migrations,
// pg_dump invocation), not for application code.
func (d *PgDB) Pool() *pgxpool.Pool { return d.pool }

func (d *PgDB) Close() error {
	d.pool.Close()
	return nil
}

// PgTx implements Tx against a pgx.Tx. The conn is held until Commit/Rollback,
// at which point it is released back to the pool.
type PgTx struct {
	ctx  context.Context
	conn *pgxpool.Conn
	tx   pgx.Tx
	db   *PgDB
}

func (t *PgTx) DBBackend() any { return t.tx }

// schemaQuoteIdent quotes a PG identifier safely. pgxpool's SET LOCAL doesn't
// accept parameters for identifier values, so we have to interpolate; the
// account name is operator-controlled, but quote-double the identifier
// regardless to defend against future input paths.
func schemaQuoteIdent(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			out = append(out, '"', '"')
			continue
		}
		out = append(out, s[i])
	}
	out = append(out, '"')
	return string(out)
}

// applySearchPath sets search_path for the duration of the current transaction.
// Called once per Begin; queries inside the transaction inherit it.
func applySearchPath(ctx context.Context, tx pgx.Tx, schema string) error {
	if schema == "" {
		return nil
	}
	_, err := tx.Exec(ctx, "SET LOCAL search_path = "+schemaQuoteIdent(schema))
	return err
}

// Begin starts a new transaction. The caller MUST eventually call Commit or
// Rollback to release the underlying pool connection.
func (d *PgDB) Begin(ctx context.Context, writable bool) (Tx, error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pg acquire: %w", err)
	}
	mode := pgx.ReadWrite
	if !writable {
		mode = pgx.ReadOnly
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: mode})
	if err != nil {
		conn.Release()
		return nil, fmt.Errorf("pg begin: %w", err)
	}
	if err := applySearchPath(ctx, tx, d.searchPath); err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		return nil, fmt.Errorf("pg set search_path: %w", err)
	}
	return &PgTx{ctx: ctx, conn: conn, tx: tx, db: d}, nil
}

func (t *PgTx) Commit() error {
	defer t.conn.Release()
	return mapPgErr(t.tx.Commit(t.ctx))
}

func (t *PgTx) Rollback() error {
	defer t.conn.Release()
	err := t.tx.Rollback(t.ctx)
	// Rolling back an already-committed/rolled-back tx returns ErrTxClosed; treat as no-op.
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return mapPgErr(err)
}

// Read runs fn inside a read-only transaction. Always commits (read-only;
// nothing to roll back) unless fn returns an error.
func (d *PgDB) Read(ctx context.Context, fn func(tx Tx) error) error {
	return d.runInTx(ctx, false, fn)
}

// Write runs fn inside a writable transaction, committing on nil error
// and rolling back on any error or panic.
func (d *PgDB) Write(ctx context.Context, fn func(tx Tx) error) error {
	return d.runInTx(ctx, true, fn)
}

func (d *PgDB) runInTx(ctx context.Context, writable bool, fn func(tx Tx) error) (rerr error) {
	tx, err := d.Begin(ctx, writable)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// One-shot CRUD on PgDB opens its own transaction. Used by callers that don't
// need to coordinate multiple operations. Falls through to PgTx.
func (d *PgDB) Get(ctx context.Context, v any) error {
	return d.Read(ctx, func(tx Tx) error { return tx.Get(v) })
}
func (d *PgDB) Insert(ctx context.Context, v any) error {
	return d.Write(ctx, func(tx Tx) error { return tx.Insert(v) })
}
func (d *PgDB) Update(ctx context.Context, v any) error {
	return d.Write(ctx, func(tx Tx) error { return tx.Update(v) })
}
func (d *PgDB) Delete(ctx context.Context, v any) error {
	return d.Write(ctx, func(tx Tx) error { return tx.Delete(v) })
}

// Get/Insert/Update/Delete on PgTx dispatch into the per-type handler registry
// populated by Step 9 (pg_types.go). Each helper resolves the handler by the
// runtime type of v (which must be a pointer to a registered struct), then
// runs the handler-supplied SQL through the open pgx.Tx.
//
// Until Step 9 lands, these will panic from lookupPgHandler — that's the
// designed behaviour, not a bug.

func (t *PgTx) Get(v any) error {
	return runPgTxOp(v, "Get", func(h pgTxOps) error { return h.get(t.ctx, t.tx, v) })
}
func (t *PgTx) Insert(v any) error {
	return runPgTxOp(v, "Insert", func(h pgTxOps) error { return h.insert(t.ctx, t.tx, v) })
}
func (t *PgTx) Update(v any) error {
	return runPgTxOp(v, "Update", func(h pgTxOps) error { return h.update(t.ctx, t.tx, v) })
}
func (t *PgTx) Delete(v any) error {
	return runPgTxOp(v, "Delete", func(h pgTxOps) error { return h.delete(t.ctx, t.tx, v) })
}

// pgTxOps and pgHandlerOps adapt the generic pgTypeHandler[T] callbacks to a
// non-generic interface usable from runtime-typed CRUD entrypoints. The lookup
// goes through pgHandlerLookup (registered by registerPgType in Step 9).
type pgTxOps struct {
	get    func(ctx context.Context, tx pgxExecutor, v any) error
	insert func(ctx context.Context, tx pgxExecutor, v any) error
	update func(ctx context.Context, tx pgxExecutor, v any) error
	delete func(ctx context.Context, tx pgxExecutor, v any) error
}

// pgRuntimeLookup returns a pgTxOps adapter for the dynamic type of v. v must
// be a pointer to a registered struct type. Step 9 fills the registry; until
// then this panics with the same message lookupPgHandler[T] would.
//
// runPgTxOp wraps pgRuntimeLookup with PG error mapping.
func runPgTxOp(v any, opName string, fn func(pgTxOps) error) error {
	ops, err := pgRuntimeLookup(v)
	if err != nil {
		return fmt.Errorf("pg %s: %w", opName, err)
	}
	return mapPgErr(fn(ops))
}

// mapPgErr translates pgx / PG SQLSTATE errors to the package-level sentinels
// (ErrUnique, ErrAbsent) so callers can use errors.Is uniformly across
// backends. Non-PG errors pass through unchanged.
func mapPgErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAbsent
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", ErrUnique, pgErr.Message)
		case "02000": // no_data
			return ErrAbsent
		}
	}
	return err
}
