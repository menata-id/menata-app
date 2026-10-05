package data

import (
	"context"
	"sort"
	"sync"
	"time"
)

// ReadLog counts the reads one request issued, per target. It makes the throwaway probe Phase 6's
// fourth test used a permanent part of the runtime (development-history.md Phase 18 Step 3): the cost of a
// page is then a number anyone can read, and the next forcing condition announces itself instead
// of waiting to be guessed at.
//
// This also serves 001 Principle #6, which does not merely prefer inference but requires that
// "when inference materially affects data access, composition, authorization, rendering, or
// execution planning, the runtime should be able to expose the resolved result through
// diagnostics or equivalent tooling" -- what a page actually read is the observable half of that.
type ReadLog struct {
	mu sync.Mutex
	// queries is every statement the pool actually issued, counted by QueryTracer. total and
	// byTarget are the named half, recorded by the Store methods themselves. The two are kept
	// apart rather than merged because their disagreement is the useful signal: queries above
	// total + writes + control means something issued a read without naming itself.
	queries int
	writes  int
	control int
	dbTime  time.Duration
	// method and path say which request this log belongs to, for a line the tracer writes while
	// the request is still running (SLOW-QUERY), when queryDiagnostics has not printed its own yet.
	method   string
	path     string
	total    int
	byTarget map[string]int
}

// countQuery records one statement issued through the pool (QueryTracer). Unlike record it takes
// no target: the tracer sees SQL text, not the Machine or table a caller meant, and naming a read
// after the first token of its SQL would be a worse label than none. It does take a kind, because
// whether a statement is a write is answerable from that token (classifyStatement).
func (l *ReadLog) countQuery(kind statementKind) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.queries++
	switch kind {
	case kindWrite:
		l.writes++
	case kindControl:
		l.control++
	}
}

// SetRequest names the request this log belongs to. See the method/path fields.
func (l *ReadLog) SetRequest(method, path string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.method, l.path = method, path
}

func (l *ReadLog) request() string {
	if l == nil {
		return "-"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.path == "" {
		return "-"
	}
	return l.method + " " + l.path
}

// Writes is how many INSERT/UPDATE/DELETE statements the request issued. They are not reads and
// never name themselves, so they are accounted for here instead of showing up as Unnamed.
func (l *ReadLog) Writes() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writes
}

// Unnamed is the statements that were neither a named read, a write, nor transaction control: a
// read issued by something that did not call record(). Zero is the healthy value, and on a write
// request it is now as reachable as on a GET.
func (l *ReadLog) Unnamed() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.queries - l.total - l.writes - l.control
}

// addDBTime accumulates one statement's elapsed time (QueryTracer.TraceQueryEnd).
func (l *ReadLog) addDBTime(d time.Duration) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dbTime += d
}

// DBTime is the summed time of every finished statement. It is a *sum*, not a wall-clock span, so it
// can exceed the request's own duration if statements ever overlap, and it excludes a statement
// whose End was never reported. Compared with the request's total it says how much of a slow page
// was the database and how much was everything else -- the one question the count alone cannot.
func (l *ReadLog) DBTime() time.Duration {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dbTime
}

// Queries is how many statements the pool issued for this request -- the driver's own count,
// which no caller can forget to increment. Total is the subset that named itself; where the two
// differ, Queries is the one to trust.
func (l *ReadLog) Queries() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.queries
}

// recordFor names one read that is *about a particular subject* -- a membership, a person's app
// roles, one Group's grants.
//
// It exists because `repeated` counts by target name alone, and for these reads that was a false
// positive rather than a finding: an admin editing another member legitimately reads two
// memberships, theirs and the viewer's, and the diagnostic reported "membership x2" as a repeat.
// Measured directly on 2026-09-28 -- the per-record route sweep still reported repeated=3 after the
// fixture was changed to edit a *different* member, which is what proved the metric wrong rather
// than the screen.
//
// Only reads keyed by a subject take this. A whole-Machine list has no subject to name, and giving
// it a synthetic one would make every such read look distinct and hide the repeats that are real.
func (l *ReadLog) recordFor(target, subject string) {
	if subject == "" {
		l.record(target)
		return
	}
	l.record(target + " " + shortSubject(subject))
}

// shortSubject keeps a diagnostic line readable: ids here are 28 characters and the line already
// carries a dozen targets. The last six are enough to tell two subjects apart, which is the only
// thing this needs to do.
func shortSubject(id string) string {
	if len(id) <= 6 {
		return "(" + id + ")"
	}
	return "(…" + id[len(id)-6:] + ")"
}

// Target names one read: the Machine, plus the field a child-collection read filtered on.
func (l *ReadLog) record(target string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total++
	if l.byTarget == nil {
		l.byTarget = map[string]int{}
	}
	l.byTarget[target]++
}

// Total is how many queries the request issued.
func (l *ReadLog) Total() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}

// Repeated returns how many reads were issued more than once for the same target -- the waste a
// request-scoped memo is supposed to remove. Zero is the healthy value.
func (l *ReadLog) Repeated() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var repeated int
	for _, n := range l.byTarget {
		if n > 1 {
			repeated += n - 1
		}
	}
	return repeated
}

// Breakdown lists each target and its read count, most-read first, for a diagnostic line.
func (l *ReadLog) Breakdown() []TargetCount {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]TargetCount, 0, len(l.byTarget))
	for target, n := range l.byTarget {
		out = append(out, TargetCount{Target: target, Reads: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Reads != out[j].Reads {
			return out[i].Reads > out[j].Reads
		}
		return out[i].Target < out[j].Target
	})
	return out
}

// TargetCount is one row of a ReadLog breakdown.
type TargetCount struct {
	Target string
	Reads  int
}

type readLogKey struct{}

// WithReadLog returns a context that counts every Store read made under it, and the log to read
// afterwards. A request that never calls this is unaffected: recording is a no-op on a nil log,
// so the diagnostic costs nothing where it isn't installed.
func WithReadLog(ctx context.Context) (context.Context, *ReadLog) {
	log := &ReadLog{}
	return context.WithValue(ctx, readLogKey{}, log), log
}

func readLogFrom(ctx context.Context) *ReadLog {
	log, _ := ctx.Value(readLogKey{}).(*ReadLog)
	return log
}
