package store

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgFilter is one WHERE-clause fragment.
// SQLExpr uses literal "$?" markers; build phase rewrites them to $1, $2, ...
type pgFilter struct {
	SQLExpr string
	Args    []any
}

// pgSort is one ORDER BY entry.
type pgSort struct {
	Column string
	Asc    bool
}

// pgQuery is the PostgreSQL backend for TypedQuery[T].
// State-tracking only until a terminal method (List/Get/Count/...) executes the SQL.
type pgQuery[T any] struct {
	ctx     context.Context
	exec    pgxExecutor     // non-nil on the Query(tx) path; nil on the QueryDB path
	pool    *pgxpool.Pool   // non-nil on the QueryDB path; nil on the Query(tx) path
	schema  string          // component schema (e.g. "queue") set on the QueryDB path
	handler *pgTypeHandler[T]

	filters   []pgFilter
	filterFns []func(T) bool
	sorts     []pgSort
	limit     int
	gatherDst *[]T
	gatherIDs any

	err error
}

// newPgQuery constructs a query that runs inside an already-opened transaction.
// The transaction's search_path must already be set by the caller (applySearchPath).
func newPgQuery[T any](ctx context.Context, exec pgxExecutor) *pgQuery[T] {
	return &pgQuery[T]{
		ctx:     ctx,
		exec:    exec,
		handler: lookupPgHandler[T](),
	}
}

// setErr records the first error and is a no-op on subsequent calls.
func (q *pgQuery[T]) setErr(err error) {
	if q.err == nil {
		q.err = err
	}
}

// columnFor maps a Go struct field name to its SQL column. Unknown fields
// poison the query so it surfaces at execution time, not silently.
func (q *pgQuery[T]) columnFor(field string) string {
	if col, ok := q.handler.FieldToColumn[field]; ok {
		return col
	}
	q.setErr(fmt.Errorf("pg: type %s has no field %q", q.handler.Table, field))
	return field
}

// addFilter appends a filter using "$?" placeholders that get renumbered later.
func (q *pgQuery[T]) addFilter(expr string, args ...any) {
	q.filters = append(q.filters, pgFilter{SQLExpr: expr, Args: args})
}

func (q *pgQuery[T]) FilterEqual(field string, values ...any) *pgQuery[T] {
	col := q.columnFor(field)
	switch len(values) {
	case 0:
		q.setErr(fmt.Errorf("pg: FilterEqual %q requires at least one value", field))
	case 1:
		q.addFilter(col+" = $?", values[0])
	default:
		q.addFilter(col+" = ANY($?)", values)
	}
	return q
}

func (q *pgQuery[T]) FilterNotEqual(field string, values ...any) *pgQuery[T] {
	col := q.columnFor(field)
	switch len(values) {
	case 0:
		q.setErr(fmt.Errorf("pg: FilterNotEqual %q requires at least one value", field))
	case 1:
		q.addFilter(col+" <> $?", values[0])
	default:
		q.addFilter("NOT ("+col+" = ANY($?))", values)
	}
	return q
}

func (q *pgQuery[T]) FilterNonzero(v T) *pgQuery[T] {
	for col, val := range q.handler.NonzeroFields(v) {
		q.addFilter(col+" = $?", val)
	}
	return q
}

func (q *pgQuery[T]) FilterFn(fn func(T) bool) *pgQuery[T] {
	q.filterFns = append(q.filterFns, fn)
	return q
}

func (q *pgQuery[T]) FilterGreater(field string, value any) *pgQuery[T] {
	q.addFilter(q.columnFor(field)+" > $?", value)
	return q
}

func (q *pgQuery[T]) FilterGreaterEqual(field string, value any) *pgQuery[T] {
	q.addFilter(q.columnFor(field)+" >= $?", value)
	return q
}

func (q *pgQuery[T]) FilterLess(field string, value any) *pgQuery[T] {
	q.addFilter(q.columnFor(field)+" < $?", value)
	return q
}

func (q *pgQuery[T]) FilterLessEqual(field string, value any) *pgQuery[T] {
	q.addFilter(q.columnFor(field)+" <= $?", value)
	return q
}

func (q *pgQuery[T]) FilterIn(field string, value any) *pgQuery[T] {
	q.addFilter(q.columnFor(field)+" = ANY($?)", value)
	return q
}

func (q *pgQuery[T]) FilterID(id any) *pgQuery[T] {
	q.addFilter(q.handler.PKColumn+" = $?", id)
	return q
}

func (q *pgQuery[T]) FilterIDs(ids any) *pgQuery[T] {
	q.addFilter(q.handler.PKColumn+" = ANY($?)", ids)
	return q
}

func (q *pgQuery[T]) SortAsc(fields ...string) *pgQuery[T] {
	for _, f := range fields {
		q.sorts = append(q.sorts, pgSort{Column: q.columnFor(f), Asc: true})
	}
	return q
}

func (q *pgQuery[T]) SortDesc(fields ...string) *pgQuery[T] {
	for _, f := range fields {
		q.sorts = append(q.sorts, pgSort{Column: q.columnFor(f), Asc: false})
	}
	return q
}

func (q *pgQuery[T]) Limit(n int) *pgQuery[T] {
	q.limit = n
	return q
}

func (q *pgQuery[T]) Gather(dst *[]T) *pgQuery[T] {
	q.gatherDst = dst
	return q
}

func (q *pgQuery[T]) GatherIDs(ids any) *pgQuery[T] {
	q.gatherIDs = ids
	return q
}

// renumberPlaceholders replaces "$?" in expr with "$N", "$N+1", ...
// returning the rewritten string and the next available index.
func renumberPlaceholders(expr string, startIdx int) (string, int) {
	if !strings.Contains(expr, "$?") {
		return expr, startIdx
	}
	var b strings.Builder
	b.Grow(len(expr))
	n := startIdx
	for i := 0; i < len(expr); i++ {
		if expr[i] == '$' && i+1 < len(expr) && expr[i+1] == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			i++
			continue
		}
		b.WriteByte(expr[i])
	}
	return b.String(), n
}

// composeWhere joins all filters into "WHERE a AND b AND ..." with sequential
// placeholders, returning the SQL fragment, the flat args slice, and the next
// placeholder index (so subsequent SET clauses can continue numbering).
func (q *pgQuery[T]) composeWhere(startIdx int) (string, []any, int) {
	if len(q.filters) == 0 {
		return "", nil, startIdx
	}
	parts := make([]string, 0, len(q.filters))
	args := make([]any, 0, len(q.filters))
	idx := startIdx
	for _, f := range q.filters {
		expr, next := renumberPlaceholders(f.SQLExpr, idx)
		parts = append(parts, expr)
		args = append(args, f.Args...)
		idx = next
	}
	return " WHERE " + strings.Join(parts, " AND "), args, idx
}

func (q *pgQuery[T]) composeOrderBy() string {
	if len(q.sorts) == 0 {
		return ""
	}
	parts := make([]string, len(q.sorts))
	for i, s := range q.sorts {
		dir := "ASC"
		if !s.Asc {
			dir = "DESC"
		}
		parts[i] = s.Column + " " + dir
	}
	return " ORDER BY " + strings.Join(parts, ", ")
}

func (q *pgQuery[T]) composeLimit() string {
	if q.limit <= 0 {
		return ""
	}
	return fmt.Sprintf(" LIMIT %d", q.limit)
}

// buildSelect assembles "SELECT cols FROM table WHERE ... ORDER BY ... LIMIT ...".
// When filterFns are present, ORDER BY/LIMIT are still applied SQL-side; the
// post-fetch Go filter trims further. Limit semantics are intentionally a
// best-effort match: combining FilterFn with Limit may return fewer than N rows.
func (q *pgQuery[T]) buildSelect(columns string) (string, []any) {
	where, args, _ := q.composeWhere(0)
	return "SELECT " + columns + " FROM " + q.handler.Table + where + q.composeOrderBy() + q.composeLimit(), args
}

// applyFilterFns runs every recorded FilterFn against rows in place,
// retaining only those for which all functions return true.
func (q *pgQuery[T]) applyFilterFns(rows []T) []T {
	if len(q.filterFns) == 0 {
		return rows
	}
	out := rows[:0]
nextRow:
	for _, r := range rows {
		for _, fn := range q.filterFns {
			if !fn(r) {
				continue nextRow
			}
		}
		out = append(out, r)
	}
	return out
}

// withSchema runs fn against an executor that has the correct search_path set.
// On the Query(tx) path (pool == nil) the tx already has search_path and fn is
// called directly. On the QueryDB(pool) path a short transaction is opened,
// SET LOCAL search_path is applied, and the transaction is committed/rolled-back
// after fn returns.
func (q *pgQuery[T]) withSchema(writable bool, fn func(exec pgxExecutor) error) error {
	if q.pool == nil {
		// Already inside a transaction with search_path set.
		return fn(q.exec)
	}
	conn, err := q.pool.Acquire(q.ctx)
	if err != nil {
		return fmt.Errorf("pg acquire: %w", err)
	}
	defer conn.Release()
	mode := pgx.ReadOnly
	if writable {
		mode = pgx.ReadWrite
	}
	tx, err := conn.BeginTx(q.ctx, pgx.TxOptions{AccessMode: mode})
	if err != nil {
		return fmt.Errorf("pg begin: %w", err)
	}
	if err := applySearchPath(q.ctx, tx, q.schema); err != nil {
		tx.Rollback(q.ctx) //nolint:errcheck
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback(q.ctx) //nolint:errcheck
		return err
	}
	return tx.Commit(q.ctx)
}

// fetchAllWithExec runs the SELECT against exec and returns the scanned rows.
func (q *pgQuery[T]) fetchAllWithExec(exec pgxExecutor) ([]T, error) {
	cols := strings.Join(q.handler.Columns, ", ")
	sql, args := q.buildSelect(cols)
	rows, err := exec.Query(q.ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("pg query %s: %w", q.handler.Table, err)
	}
	defer rows.Close()
	var out []T
	if err := q.handler.ScanRows(rows, &out); err != nil {
		return nil, fmt.Errorf("pg scan %s: %w", q.handler.Table, err)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return q.applyFilterFns(out), nil
}

// fetchAll runs the SELECT, applying search_path wrapping when on the QueryDB path.
func (q *pgQuery[T]) fetchAll() ([]T, error) {
	if q.err != nil {
		return nil, q.err
	}
	var out []T
	err := q.withSchema(false, func(exec pgxExecutor) error {
		var err error
		out, err = q.fetchAllWithExec(exec)
		return err
	})
	return out, err
}

func (q *pgQuery[T]) List() ([]T, error) {
	out, err := q.fetchAll()
	if err != nil {
		return nil, err
	}
	if q.gatherDst != nil {
		*q.gatherDst = out
	}
	if q.gatherIDs != nil {
		if err := q.fillGatherIDs(out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Get returns exactly one row, or ErrAbsent / pgErrMultiple wrapped error.
func (q *pgQuery[T]) Get() (T, error) {
	var zero T
	old := q.limit
	if q.limit == 0 || q.limit > 2 {
		q.limit = 2
	}
	out, err := q.fetchAll()
	q.limit = old
	if err != nil {
		return zero, err
	}
	switch len(out) {
	case 0:
		return zero, ErrAbsent
	case 1:
		return out[0], nil
	default:
		return zero, fmt.Errorf("pg get %s: multiple matches", q.handler.Table)
	}
}

func (q *pgQuery[T]) Count() (int, error) {
	if len(q.filterFns) > 0 {
		out, err := q.fetchAll()
		if err != nil {
			return 0, err
		}
		return len(out), nil
	}
	if q.err != nil {
		return 0, q.err
	}
	var n int
	err := q.withSchema(false, func(exec pgxExecutor) error {
		sql, args := q.buildSelect("COUNT(*)")
		sql = stripOrderBy(sql)
		if err := exec.QueryRow(q.ctx, sql, args...).Scan(&n); err != nil {
			return fmt.Errorf("pg count %s: %w", q.handler.Table, err)
		}
		return nil
	})
	return n, err
}

func (q *pgQuery[T]) Exists() (bool, error) {
	if len(q.filterFns) > 0 {
		out, err := q.fetchAll()
		return len(out) > 0, err
	}
	if q.err != nil {
		return false, q.err
	}
	var exists bool
	err := q.withSchema(false, func(exec pgxExecutor) error {
		old := q.limit
		q.limit = 1
		sql, args := q.buildSelect("1")
		q.limit = old
		var dummy int
		err := exec.QueryRow(q.ctx, sql, args...).Scan(&dummy)
		if errors.Is(err, pgx.ErrNoRows) {
			exists = false
			return nil
		}
		if err != nil {
			return fmt.Errorf("pg exists %s: %w", q.handler.Table, err)
		}
		exists = true
		return nil
	})
	return exists, err
}

func (q *pgQuery[T]) ForEach(fn func(T) error) error {
	out, err := q.fetchAll()
	if err != nil {
		return err
	}
	for _, r := range out {
		if err := fn(r); err != nil {
			if errors.Is(err, StopForEach) {
				return nil
			}
			return err
		}
	}
	return nil
}

// Delete removes matching rows. With FilterFn set, fetches IDs first then deletes by PK.
func (q *pgQuery[T]) Delete() (int, error) {
	if q.err != nil {
		return 0, q.err
	}
	if len(q.filterFns) > 0 {
		ids, err := q.fetchPKs()
		if err != nil {
			return 0, err
		}
		if len(ids) == 0 {
			return 0, nil
		}
		var n int
		err = q.withSchema(true, func(exec pgxExecutor) error {
			tag, err := exec.Exec(q.ctx,
				"DELETE FROM "+q.handler.Table+" WHERE "+q.handler.PKColumn+" = ANY($1)", ids)
			if err != nil {
				return fmt.Errorf("pg delete %s: %w", q.handler.Table, err)
			}
			n = int(tag.RowsAffected())
			return nil
		})
		return n, err
	}
	var n int
	err := q.withSchema(true, func(exec pgxExecutor) error {
		where, args, _ := q.composeWhere(0)
		tag, err := exec.Exec(q.ctx, "DELETE FROM "+q.handler.Table+where, args...)
		if err != nil {
			return fmt.Errorf("pg delete %s: %w", q.handler.Table, err)
		}
		n = int(tag.RowsAffected())
		return nil
	})
	return n, err
}

func (q *pgQuery[T]) UpdateFields(fields map[string]any) (int, error) {
	if q.err != nil {
		return 0, fmt.Errorf("pg update %s: empty fields map", q.handler.Table)
	}
	if len(fields) == 0 {
		return 0, fmt.Errorf("pg update %s: empty fields map", q.handler.Table)
	}
	setParts := make([]string, 0, len(fields))
	setArgs := make([]any, 0, len(fields))
	idx := 0
	for f, v := range fields {
		col := q.columnFor(f)
		if q.err != nil {
			return 0, q.err
		}
		idx++
		setParts = append(setParts, fmt.Sprintf("%s = $%d", col, idx))
		setArgs = append(setArgs, v)
	}
	return q.runUpdate(strings.Join(setParts, ", "), setArgs, idx)
}

func (q *pgQuery[T]) UpdateField(field string, value any) (int, error) {
	return q.UpdateFields(map[string]any{field: value})
}

func (q *pgQuery[T]) UpdateNonzero(v T) (int, error) {
	return q.UpdateFields(q.handler.NonzeroFields(v))
}

// runUpdate emits the UPDATE statement, respecting filterFns by pre-fetching PKs.
func (q *pgQuery[T]) runUpdate(setClause string, setArgs []any, startIdx int) (int, error) {
	if len(q.filterFns) > 0 {
		ids, err := q.fetchPKs()
		if err != nil {
			return 0, err
		}
		if len(ids) == 0 {
			return 0, nil
		}
		var n int
		err = q.withSchema(true, func(exec pgxExecutor) error {
			args := append(setArgs, ids)
			sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ANY($%d)",
				q.handler.Table, setClause, q.handler.PKColumn, startIdx+1)
			tag, err := exec.Exec(q.ctx, sql, args...)
			if err != nil {
				return fmt.Errorf("pg update %s: %w", q.handler.Table, err)
			}
			n = int(tag.RowsAffected())
			return nil
		})
		return n, err
	}
	var n int
	err := q.withSchema(true, func(exec pgxExecutor) error {
		where, whereArgs, _ := q.composeWhere(startIdx)
		args := append(setArgs, whereArgs...)
		sql := "UPDATE " + q.handler.Table + " SET " + setClause + where
		tag, err := exec.Exec(q.ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("pg update %s: %w", q.handler.Table, err)
		}
		n = int(tag.RowsAffected())
		return nil
	})
	return n, err
}

// fetchPKs returns the primary keys for rows matching all filters, including FilterFns.
func (q *pgQuery[T]) fetchPKs() ([]any, error) {
	rows, err := q.fetchAll()
	if err != nil {
		return nil, err
	}
	ids := make([]any, len(rows))
	for i, r := range rows {
		ids[i] = q.handler.PKValue(r)
	}
	return ids, nil
}

// IDs writes the matching primary keys into idsptr (a pointer to a slice).
func (q *pgQuery[T]) IDs(idsptr any) error {
	rows, err := q.fetchAll()
	if err != nil {
		return err
	}
	dst := reflect.ValueOf(idsptr)
	if dst.Kind() != reflect.Pointer || dst.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("pg IDs: expected pointer to slice, got %T", idsptr)
	}
	slice := reflect.MakeSlice(dst.Elem().Type(), len(rows), len(rows))
	for i, r := range rows {
		slice.Index(i).Set(reflect.ValueOf(q.handler.PKValue(r)))
	}
	dst.Elem().Set(slice)
	return nil
}

// fillGatherIDs writes the PKs of the already-fetched rows into the
// user-supplied slice pointer.
func (q *pgQuery[T]) fillGatherIDs(rows []T) error {
	dst := reflect.ValueOf(q.gatherIDs)
	if dst.Kind() != reflect.Pointer || dst.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("pg GatherIDs: expected pointer to slice, got %T", q.gatherIDs)
	}
	slice := reflect.MakeSlice(dst.Elem().Type(), len(rows), len(rows))
	for i, r := range rows {
		slice.Index(i).Set(reflect.ValueOf(q.handler.PKValue(r)))
	}
	dst.Elem().Set(slice)
	return nil
}

func (q *pgQuery[T]) Err() error {
	return q.err
}

// All returns an iter.Seq2 that yields one row at a time. The query is executed
// when iteration begins; pre-fetch is required because FilterFn would otherwise
// have to be evaluated lazily, complicating Stop semantics.
func (q *pgQuery[T]) All() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		out, err := q.fetchAll()
		if err != nil {
			var zero T
			yield(zero, err)
			return
		}
		for _, r := range out {
			if !yield(r, nil) {
				return
			}
		}
	}
}

// stripOrderBy removes a trailing " ORDER BY ..." clause (before any LIMIT)
// from a SELECT statement built by buildSelect. ORDER BY is meaningless for
// COUNT(*) and PG would otherwise still parse the columns.
func stripOrderBy(sql string) string {
	idx := strings.Index(sql, " ORDER BY ")
	if idx < 0 {
		return sql
	}
	limIdx := strings.Index(sql[idx:], " LIMIT ")
	if limIdx < 0 {
		return sql[:idx]
	}
	return sql[:idx] + sql[idx+limIdx:]
}
