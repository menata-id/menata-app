package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a PostgreSQL connection pool and verifies it with a ping.
//
// tracer, when non-nil, is installed on every connection the pool opens, so it observes every
// statement issued through it (data.QueryTracer is the one this app passes, for the per-request
// query diagnostic). It arrives as a parameter rather than being built here because this package
// may not import menata.app/internal at all -- internal/db owns the pool and knows nothing of
// Runtime Metadata semantics (internal/conformance's plane rule for "db") -- so the composition
// root is what pairs the two. A nil tracer is valid and is what a test wanting an uninstrumented
// pool passes.
//
// statementTimeout, when positive, is PostgreSQL's statement_timeout on every connection the pool opens (K20,
// 007 §18.8: the runtime "MUST NOT silently execute unbounded work"). A statement that outlives it is
// cancelled by the server (SQLSTATE 57014) instead of holding a pooled connection until the client gives up.
// It applies to the pool only: migrations run through goose on their own connection and are not bounded by it.
func Connect(ctx context.Context, databaseURL string, tracer pgx.QueryTracer, statementTimeout time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.ConnConfig.Tracer = tracer
	if statementTimeout > 0 {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(statementTimeout.Milliseconds(), 10)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
