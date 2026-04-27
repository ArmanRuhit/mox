package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mjl-/mox/config"
	"github.com/mjl-/mox/mox-"
)

// pgPoolMu guards the singleton pool. The pool is created lazily by
// InitPool on first call (typically from store.Init or queue.Init when
// PostgreSQL is configured) and torn down by ClosePool at shutdown.
var (
	pgPoolMu sync.Mutex
	pgPool   *pgxpool.Pool
)

// InitPool builds the shared pgx connection pool from cfg. Calling more
// than once without an intervening ClosePool returns an error: a singleton
// keeps the connect-counts visible per-process, and there is no use case
// for swapping pools at runtime.
//
// The pool is configured with cfg.MaxConns (default 10) and cfg.MinConns
// (default 0), and a startup AcquireAndPing verifies the DSN before
// returning. The DSN is taken from cfg.DSN if non-empty, otherwise read
// from cfg.DSNFile relative to the mox config dir.
func InitPool(ctx context.Context, cfg *config.PostgreSQLConfig) (*pgxpool.Pool, error) {
	pgPoolMu.Lock()
	defer pgPoolMu.Unlock()

	if pgPool != nil {
		return nil, errors.New("pg pool already initialised")
	}

	dsn, err := resolveDSN(cfg)
	if err != nil {
		return nil, err
	}

	pcfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pg dsn: %w", err)
	}

	if cfg.MaxConns > 0 {
		pcfg.MaxConns = cfg.MaxConns
	} else {
		pcfg.MaxConns = 10
	}
	if cfg.MinConns > 0 {
		pcfg.MinConns = cfg.MinConns
	}

	// Without a healthy starting search_path, queries before the first
	// SET LOCAL would resolve against "public" — fine in normal use, but
	// a footgun for one-shot pool.QueryRow paths in QueryDB. Pinning to
	// "public" makes the implicit fallback explicit.
	pcfg.ConnConfig.RuntimeParams["search_path"] = "public"

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("build pg pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pg: %w", err)
	}

	pgPool = pool
	return pool, nil
}

// Pool returns the initialised pool, or nil if PostgreSQL isn't configured.
// Callers in the bstore branch must check for nil before dereferencing.
func Pool() *pgxpool.Pool {
	pgPoolMu.Lock()
	defer pgPoolMu.Unlock()
	return pgPool
}

// ClosePool tears down the pool. Safe to call when no pool was ever
// opened.
func ClosePool() {
	pgPoolMu.Lock()
	defer pgPoolMu.Unlock()
	if pgPool == nil {
		return
	}
	pgPool.Close()
	pgPool = nil
}

func resolveDSN(cfg *config.PostgreSQLConfig) (string, error) {
	switch {
	case cfg.DSN != "" && cfg.DSNFile != "":
		return "", errors.New("PostgreSQL: DSN and DSNFile are mutually exclusive")
	case cfg.DSN != "":
		return cfg.DSN, nil
	case cfg.DSNFile != "":
		path := mox.ConfigDirPath(cfg.DSNFile)
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read DSNFile %q: %w", path, err)
		}
		return strings.TrimSpace(string(raw)), nil
	default:
		return "", errors.New("PostgreSQL: neither DSN nor DSNFile set")
	}
}
