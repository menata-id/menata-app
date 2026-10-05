package data

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type queryStartKey struct{}

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
//
// It also stamps the start time on the returned context, which pgx hands back to TraceQueryEnd, so
// the ReadLog can total how long the request spent waiting on the database. A context with no log
// is left untouched: nothing would read the stamp.
func (t *QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	log := readLogFrom(ctx)
	if log == nil {
		return ctx
	}
	log.countQuery(classifyStatement(data.SQL))
	return context.WithValue(ctx, queryStartKey{}, time.Now())
}

// TraceQueryEnd adds this statement's elapsed time to the request's ReadLog. Counting still happens
// at the start rather than here on purpose: a query that fails or whose rows are never drained still
// cost a round trip, and a diagnostic that only counts successes would under-report exactly when a
// page is in trouble. Duration is the opposite case -- it is only knowable at the end, so a statement
// whose End never fires (a leaked Rows) is counted but adds no time, and DBTime is a lower bound.
func (t *QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	started, ok := ctx.Value(queryStartKey{}).(time.Time)
	if !ok {
		return
	}
	readLogFrom(ctx).addDBTime(time.Since(started))
}

// statementKind is what the tracer can say about a statement from its first keyword alone.
type statementKind int

const (
	// kindRead is everything else, SELECT included: a read must name itself through record(), so
	// one that does not is the `unnamed` the diagnostic exists to expose.
	kindRead statementKind = iota
	kindWrite
	kindControl
)

// classifyStatement separates writes and transaction control from reads, by first keyword.
//
// Classifying is not naming, and that is why this is safe where the comment on countQuery refuses
// to name a read after its SQL: the first keyword answers only "is this a read", a question it
// answers exactly, and gives no label. It exists because `reads` counts only reads, so every POST
// that wrote was reported `unnamed` -- 28 of the 28 unnamed lines in the 2026-10-05 log window
// were writes -- and a marker that fires on every healthy write is one a reader learns to ignore.
// No statement in this tree starts with WITH; one that did would classify as a read and surface as
// unnamed, which is the failure direction that is visible rather than silent.
func classifyStatement(sql string) statementKind {
	sql = strings.TrimSpace(sql)
	if i := strings.IndexAny(sql, " \t\r\n"); i >= 0 {
		sql = sql[:i]
	}
	switch strings.ToUpper(sql) {
	case "INSERT", "UPDATE", "DELETE":
		return kindWrite
	case "BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE":
		return kindControl
	}
	return kindRead
}
