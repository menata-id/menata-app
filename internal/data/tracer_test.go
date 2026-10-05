package data

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"
	"time"

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

// captureLog redirects the standard logger for one test and returns what it received.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	flags := log.Flags()
	log.SetFlags(0)
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(flags) })
	return &buf
}

func runStatement(tr *QueryTracer, ctx context.Context, sql string, args []any, took time.Duration) {
	ctx = tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: sql, Args: args})
	time.Sleep(took)
	tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
}

// TestSlowStatementIsLoggedByTextAndNeverByArguments: the line names the statement and the request,
// and must not carry a bound value -- args are where emails and record content travel.
func TestSlowStatementIsLoggedByTextAndNeverByArguments(t *testing.T) {
	buf := captureLog(t)
	ctx, rl := WithReadLog(context.Background())
	rl.SetRequest("GET", "/approval-inbox")
	tr := &QueryTracer{SlowThreshold: time.Millisecond}

	runStatement(tr, ctx, "\n\t\tSELECT id\n\t\t  FROM records WHERE email = $1", []any{"secret@example.com"}, 5*time.Millisecond)

	line := buf.String()
	for _, want := range []string{"SLOW-QUERY", "GET /approval-inbox", `sql="SELECT id FROM records WHERE email = $1"`} {
		if !strings.Contains(line, want) {
			t.Errorf("slow line lacks %q:\n  %s", want, line)
		}
	}
	if strings.Contains(line, "secret@example.com") {
		t.Errorf("a bound argument reached the log:\n  %s", line)
	}
}

func TestFastOrDisabledStatementsLogNothingSlow(t *testing.T) {
	buf := captureLog(t)
	ctx, _ := WithReadLog(context.Background())

	runStatement(&QueryTracer{SlowThreshold: time.Hour}, ctx, "SELECT 1", nil, time.Millisecond)
	runStatement(&QueryTracer{}, ctx, "SELECT 1", nil, 3*time.Millisecond) // zero threshold = off
	if buf.Len() != 0 {
		t.Errorf("nothing was slow, yet the log has: %s", buf.String())
	}
}

func TestSlowStatementTextIsTruncated(t *testing.T) {
	long := "SELECT " + strings.Repeat("a, ", 200) + "z FROM t"
	if got := squeezeSQL(long); len([]rune(got)) > slowSQLMax+1 {
		t.Errorf("a %d-rune statement logged as %d runes", len(long), len([]rune(got)))
	}
}
