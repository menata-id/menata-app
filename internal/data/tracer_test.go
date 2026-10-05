package data

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestClassifyStatement(t *testing.T) {
	for sql, want := range map[string]statementKind{
		"\n\t\tSELECT 1":                       kindRead,
		"select 1":                             kindRead,
		"\n\t\tSELECT id FROM t FOR UPDATE":    kindRead,
		"\n\t\tINSERT INTO t VALUES (1)":       kindWrite,
		"update t SET a = 1":                   kindWrite,
		"DELETE FROM t":                        kindWrite,
		"begin":                                kindControl,
		"commit":                               kindControl,
		"ROLLBACK":                             kindControl,
		"WITH x AS (SELECT 1) SELECT * FROM x": kindRead,
		"":                                     kindRead,
	} {
		if got := classifyStatement(sql); got != want {
			t.Errorf("classifyStatement(%q) = %v, want %v", sql, got, want)
		}
	}
}

// TestUnnamedCountsOnlyReadsThatDidNotNameThemselves holds the accounting the diagnostic's
// `unnamed` marker rests on: a write and a transaction's BEGIN/COMMIT are accounted for, an
// unnamed read is not. Before writes were classified every POST that wrote read as unnamed.
func TestUnnamedCountsOnlyReadsThatDidNotNameThemselves(t *testing.T) {
	ctx, log := WithReadLog(context.Background())
	tr := NewQueryTracer()
	issue := func(sql string) {
		tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: sql})
	}

	issue("begin")
	issue("INSERT INTO t VALUES (1)")
	issue("UPDATE t SET a = 1")
	issue("commit")
	if log.Unnamed() != 0 || log.Writes() != 2 || log.Queries() != 4 {
		t.Fatalf("a write transaction: unnamed=%d writes=%d queries=%d, want 0/2/4", log.Unnamed(), log.Writes(), log.Queries())
	}

	issue("SELECT 1")
	if log.Unnamed() != 1 {
		t.Errorf("an unnamed SELECT: unnamed=%d, want 1", log.Unnamed())
	}
	log.record("some read")
	if log.Unnamed() != 0 {
		t.Errorf("after the read named itself: unnamed=%d, want 0", log.Unnamed())
	}
}
