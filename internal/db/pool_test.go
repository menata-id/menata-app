package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestConnect_statementTimeoutCancelsALongStatement is K20: with a timeout the server cancels the statement
// (57014); with none the same statement runs to completion.
func TestConnect_statementTimeoutCancelsALongStatement(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()

	bounded, err := Connect(ctx, url, nil, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer bounded.Close()
	_, err = bounded.Exec(ctx, "SELECT pg_sleep(1)")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
		t.Errorf("a 1s statement under a 200ms statement_timeout: err = %v, want SQLSTATE 57014", err)
	}

	unbounded, err := Connect(ctx, url, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unbounded.Close()
	if _, err := unbounded.Exec(ctx, "SELECT pg_sleep(0.3)"); err != nil {
		t.Errorf("with no timeout the statement should complete: %v", err)
	}
}
