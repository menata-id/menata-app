package db

import (
	"context"
	"fmt"

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
func Connect(ctx context.Context, databaseURL string, tracer pgx.QueryTracer) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.ConnConfig.Tracer = tracer

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
