package store

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
)

// pgFieldSpec maps one Go struct field to one SQL column. Field names use
// dotted paths to address embedded / nested structs ("MailboxCounts.Total").
// Only scalar paths are supported here; for arbitrary slice / map columns
// (JSONB, BYTEA-encoded), supply a custom Codec.
type pgFieldSpec struct {
	Field  string // Go struct field path
	Column string // SQL column name
	PK     bool   // True for the primary key column (exactly one per type)

	// Codec optionally wraps Scan/Value for non-trivial mappings.
	// Nil means: use the field's native Go type (works for scalars,
	// time.Time, *time.Time, *bool, []string, []int64, []byte).
	Codec pgFieldCodec
}

// pgFieldCodec lets a field plug in custom encode/decode. Encode returns the
// value to pass to pgx as a query parameter; Decode receives a pointer-to
// scan target and the field's reflect.Value to write into.
type pgFieldCodec interface {
	Encode(field reflect.Value) any
	NewScanTarget() any
	WriteScanned(target any, field reflect.Value)
}

// pgQuoteIdent double-quotes a PostgreSQL identifier so reserved keywords and
// mixed-case names are always safe. Any embedded double-quote is escaped by doubling.
func pgQuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// fieldByPath resolves a dotted struct path against rv. Used both for read
// (Encode) and for obtaining an addressable target (Scan). Panics — like
// other reflect mistakes — when the path is wrong, since it's a coding error.
func fieldByPath(rv reflect.Value, path string) reflect.Value {
	for _, name := range strings.Split(path, ".") {
		rv = rv.FieldByName(name)
		if !rv.IsValid() {
			panic("pg: field path resolves to nothing: " + path)
		}
	}
	return rv
}

// buildSimpleHandler constructs a pgTypeHandler from per-field specs using
// reflection for all per-row work. Types with embedded structs need each
// inner field listed with a dotted Field path.
//
// Insert and Update both write *every* non-PK column (no NULL-by-omission
// behaviour). PK is always included on Insert (callers supply explicit IDs;
// auto-generated IDs will be added in a follow-up step alongside sequences).
func buildSimpleHandler[T any](table string, specs []pgFieldSpec) *pgTypeHandler[T] {
	var pkSpec *pgFieldSpec
	for i := range specs {
		if specs[i].PK {
			if pkSpec != nil {
				panic("pg: multiple PK fields for " + table)
			}
			pkSpec = &specs[i]
		}
	}
	if pkSpec == nil {
		panic("pg: no PK field for " + table)
	}

	qTable := pgQuoteIdent(table)
	qPKCol := pgQuoteIdent(pkSpec.Column)

	cols := make([]string, len(specs))
	field2col := make(map[string]string, len(specs))
	for i, s := range specs {
		qcol := pgQuoteIdent(s.Column)
		cols[i] = qcol
		// Register both full path and last segment so callers can use
		// either "Total" or "MailboxCounts.Total".
		field2col[s.Field] = qcol
		if dot := strings.LastIndexByte(s.Field, '.'); dot >= 0 {
			field2col[s.Field[dot+1:]] = qcol
		}
	}

	colList := strings.Join(cols, ", ")
	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qTable, colList, strings.Join(placeholders, ", "))

	// UPDATE: set non-PK cols, key on PK as last placeholder.
	setParts := make([]string, 0, len(cols)-1)
	idx := 0
	for _, s := range specs {
		if s.PK {
			continue
		}
		idx++
		setParts = append(setParts, fmt.Sprintf("%s = $%d", pgQuoteIdent(s.Column), idx))
	}
	updateSQL := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d",
		qTable, strings.Join(setParts, ", "), qPKCol, idx+1)

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", qTable, qPKCol)
	getSQL := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1", colList, qTable, qPKCol)

	encode := func(rv reflect.Value, s pgFieldSpec) any {
		fv := fieldByPath(rv, s.Field)
		if s.Codec != nil {
			return s.Codec.Encode(fv)
		}
		return fv.Interface()
	}

	allValues := func(v *T) []any {
		rv := reflect.ValueOf(v).Elem()
		out := make([]any, len(specs))
		for i, s := range specs {
			out[i] = encode(rv, s)
		}
		return out
	}

	updateValues := func(v *T) []any {
		rv := reflect.ValueOf(v).Elem()
		out := make([]any, 0, len(specs))
		for _, s := range specs {
			if s.PK {
				continue
			}
			out = append(out, encode(rv, s))
		}
		out = append(out, encode(rv, *pkSpec))
		return out
	}

	scanTargets := func(v *T) []any {
		rv := reflect.ValueOf(v).Elem()
		targets := make([]any, len(specs))
		for i, s := range specs {
			if s.Codec != nil {
				targets[i] = s.Codec.NewScanTarget()
				continue
			}
			targets[i] = fieldByPath(rv, s.Field).Addr().Interface()
		}
		return targets
	}

	writeScanned := func(v *T, targets []any) {
		rv := reflect.ValueOf(v).Elem()
		for i, s := range specs {
			if s.Codec == nil {
				continue
			}
			s.Codec.WriteScanned(targets[i], fieldByPath(rv, s.Field))
		}
	}

	scanRow := func(row pgx.Row, dst *T) error {
		targets := scanTargets(dst)
		if err := row.Scan(targets...); err != nil {
			return err
		}
		writeScanned(dst, targets)
		return nil
	}

	scanRows := func(rows pgx.Rows, dst *[]T) error {
		for rows.Next() {
			var v T
			targets := scanTargets(&v)
			if err := rows.Scan(targets...); err != nil {
				return err
			}
			writeScanned(&v, targets)
			*dst = append(*dst, v)
		}
		return nil
	}

	insertFn := func(ctx context.Context, exec pgxExecutor, v *T) error {
		_, err := exec.Exec(ctx, insertSQL, allValues(v)...)
		return err
	}

	updateFn := func(ctx context.Context, exec pgxExecutor, v *T) (int64, error) {
		tag, err := exec.Exec(ctx, updateSQL, updateValues(v)...)
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}

	deleteFn := func(ctx context.Context, exec pgxExecutor, v *T) (int64, error) {
		rv := reflect.ValueOf(v).Elem()
		tag, err := exec.Exec(ctx, deleteSQL, encode(rv, *pkSpec))
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}

	getFn := func(ctx context.Context, exec pgxExecutor, v *T) error {
		rv := reflect.ValueOf(v).Elem()
		row := exec.QueryRow(ctx, getSQL, encode(rv, *pkSpec))
		return scanRow(row, v)
	}

	pkValue := func(v T) any {
		rv := reflect.ValueOf(v)
		return encode(rv, *pkSpec)
	}

	setPK := func(v *T, id any) {
		rv := reflect.ValueOf(v).Elem()
		fv := fieldByPath(rv, pkSpec.Field)
		fv.Set(reflect.ValueOf(id).Convert(fv.Type()))
	}

	nonzero := func(v T) map[string]any {
		rv := reflect.ValueOf(v)
		out := make(map[string]any)
		for _, s := range specs {
			fv := fieldByPath(rv, s.Field)
			if fv.IsZero() {
				continue
			}
			qcol := pgQuoteIdent(s.Column)
			if s.Codec != nil {
				out[qcol] = s.Codec.Encode(fv)
				continue
			}
			out[qcol] = fv.Interface()
		}
		return out
	}

	return &pgTypeHandler[T]{
		Table:         qTable,
		PKColumn:      qPKCol,
		PKField:       pkSpec.Field,
		Columns:       cols,
		FieldToColumn: field2col,
		ScanRow:       scanRow,
		ScanRows:      scanRows,
		Insert:        insertFn,
		Update:        updateFn,
		Delete:        deleteFn,
		GetByPK:       getFn,
		NonzeroFields: nonzero,
		PKValue:       pkValue,
		SetPK:         setPK,
	}
}
