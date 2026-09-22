package data

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// QueryTracer counts every query issued through the pool against the request's ReadLog, wherever
// in the code it was issued from.
//
// It exists because the hand-placed alternative failed in exactly the way a hand-placed
// instrument fails. ReadLog.record is called from four Machine-level methods and from nowhere
// else, so the identity, Workspace and membership queries every authenticated request issues were
// invisible to the diagnostic that claimed to report "what each request actually read" -- and the
// number it printed instead (`repeated=0`, on most lines) read as a guarantee it had no standing
// to give. A log review on 2026-09-22 found that, not a slow page.
//
// So the total is taken where it cannot be forgotten: pgx calls this for Query, QueryRow and Exec
// on every connection, so a Store method added later is counted whether or not its author knows
// this type exists. record() stays, because a bare total says a page cost fifteen queries without
// saying fifteen of what -- the tracer supplies the truth and record() supplies the names.
//
// The gap between the two is deliberately printed rather than reconciled away (web.queryDiagnostics):
// `queries` above `reads` means something issued a query without naming itself, which is the state
// this whole type is here to stop being invisible.
type QueryTracer struct{}

// NewQueryTracer returns the tracer to hand db.Connect. It is stateless -- every count lands on
// the ReadLog the request's own context carries, so one tracer serves the whole pool and holds no
// cross-request state of its own.
func NewQueryTracer() *QueryTracer { return &QueryTracer{} }

// TraceQueryStart counts one query against whatever ReadLog ctx carries. A context with no log --
// startup, migrations, a background call -- records nothing: readLogFrom returns nil and
// countQuery is a no-op on nil, the same way record() already is.
func (t *QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	readLogFrom(ctx).countQuery()
	return ctx
}

// TraceQueryEnd completes the pgx.QueryTracer interface. Counting happens at the start rather than
// here on purpose: a query that fails or whose rows are never drained still cost a round trip, and
// a diagnostic that only counts successes would under-report exactly when a page is in trouble.
func (t *QueryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
