package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema/*.sql
var schemaFS embed.FS

// schemaFilePattern matches files like "account_001.sql": <component>_<version>.sql.
// Component is lowercase letters; version is one or more digits.
var schemaFilePattern = regexp.MustCompile(`^([a-z]+)_(\d+)\.sql$`)

// schemaVersionTableSQL is created idempotently before reading applied
// versions. The schema files themselves also include this CREATE so that
// loading just one file (e.g. via psql) still works, but creating it from
// the runner removes a chicken-and-egg case from loadAppliedVersions.
const schemaVersionTableSQL = `CREATE TABLE IF NOT EXISTS schema_version (
    component TEXT NOT NULL,
    version INTEGER NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (component, version)
)`

type schemaFile struct {
	component string
	version   int
	body      string
}

// loadSchemaFiles returns all embedded migration files for component, sorted
// ascending by version. Empty result means "no files match" — the caller
// decides whether that's an error.
func loadSchemaFiles(component string) ([]schemaFile, error) {
	entries, err := fs.ReadDir(schemaFS, "schema")
	if err != nil {
		return nil, fmt.Errorf("read embedded schema dir: %w", err)
	}
	var out []schemaFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := schemaFilePattern.FindStringSubmatch(e.Name())
		if m == nil || m[1] != component {
			continue
		}
		ver, err := strconv.Atoi(m[2])
		if err != nil {
			return nil, fmt.Errorf("parse version in %s: %w", e.Name(), err)
		}
		body, err := fs.ReadFile(schemaFS, "schema/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", e.Name(), err)
		}
		out = append(out, schemaFile{component, ver, string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// EnsureSchema applies any missing migrations for component against the
// named PG schema. Idempotent: already-recorded versions in schema_version
// are skipped. Each migration runs in its own transaction; the
// schema_version row is inserted in the same transaction so a partial
// failure rolls back cleanly.
//
// schemaName is the un-quoted PG schema (e.g. "auth", "queue",
// "account_alice"). The schema is CREATE-IF-NOT-EXISTS-ed and search_path
// is set on the held connection before any migration body runs.
//
// component must match the prefix of files in schema/ (e.g. "account",
// "auth", "queue"). Components without any matching file return an error
// to flag a wiring mistake — silently doing nothing would mask the bug.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool, schemaName, component string) error {
	files, err := loadSchemaFiles(component)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no embedded migrations for component %q", component)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, schemaQuoteIdent(schemaName))); err != nil {
		return fmt.Errorf("create schema %q: %w", schemaName, err)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(`SET search_path = %s`, schemaQuoteIdent(schemaName))); err != nil {
		return fmt.Errorf("set search_path %q: %w", schemaName, err)
	}
	if _, err := conn.Exec(ctx, schemaVersionTableSQL); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	applied, err := loadAppliedVersions(ctx, conn, component)
	if err != nil {
		return err
	}

	for _, f := range files {
		if applied[f.version] {
			continue
		}
		if err := applyMigration(ctx, conn, f); err != nil {
			return fmt.Errorf("apply %s_%03d: %w", f.component, f.version, err)
		}
	}
	return nil
}

func loadAppliedVersions(ctx context.Context, conn *pgxpool.Conn, component string) (map[int]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_version WHERE component = $1`, component)
	if err != nil {
		return nil, fmt.Errorf("read schema_version: %w", err)
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		out[v] = true
	}
	return out, rows.Err()
}

func applyMigration(ctx context.Context, conn *pgxpool.Conn, f schemaFile) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// File body is parameter-free, so pgx executes it via the simple
	// protocol — multi-statement support is built-in. The body's own
	// CREATE TABLE IF NOT EXISTS schema_version is harmless on re-run.
	if _, err := tx.Exec(ctx, f.body); err != nil {
		return fmt.Errorf("exec body: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_version (component, version) VALUES ($1, $2)`,
		f.component, f.version,
	); err != nil {
		return fmt.Errorf("record schema_version: %w", err)
	}
	return tx.Commit(ctx)
}
