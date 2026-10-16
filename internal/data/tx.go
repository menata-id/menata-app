package data

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type txKey struct{}

// querier is what a record statement needs from either the pool or a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// q answers the transaction the context carries, or the pool when it carries none. The record methods a guarded write
// uses (get, list-by, create, update, delete) read through it, so a write run under InTx sees its own earlier rows and
// commits or rolls back as one.
func (s *Store) q(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return s.pool
}

// InTx runs fn in one transaction: fn's ctx carries it, so the record methods called with that ctx join it. A nil error
// commits, any error rolls back. Called inside a transaction already, fn simply joins it (the outer call owns the
// commit), so a helper may ask for atomicity without knowing whether its caller already has it.
func (s *Store) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Lock takes a transaction-scoped advisory lock named by key, waiting for any other transaction holding it. It exists
// to serialise the read-then-write of a rule that judges a total (a limit over a group, a balance over lines): two
// writers to the same group queue here, and the second reads what the first committed. Locks are released at commit or
// rollback. It must be called inside InTx; outside one it is an error, since a lock the statement would release at once
// holds nothing.
func (s *Store) Lock(ctx context.Context, key string) error {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	if !ok {
		return fmt.Errorf("data: Lock(%q) outside a transaction", key)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return fmt.Errorf("lock %q: %w", key, err)
	}
	return nil
}
