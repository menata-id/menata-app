package data

import (
	"context"
	"sort"
	"sync"
)

// ReadLog counts the reads one request issued, per target. It makes the throwaway probe Phase 6's
// fourth test used a permanent part of the runtime (ROADMAP.md Phase 18 Step 3): the cost of a
// page is then a number anyone can read, and the next forcing condition announces itself instead
// of waiting to be guessed at.
//
// This also serves 001 Principle #6, which does not merely prefer inference but requires that
// "when inference materially affects data access, composition, authorization, rendering, or
// execution planning, the runtime should be able to expose the resolved result through
// diagnostics or equivalent tooling" -- what a page actually read is the observable half of that.
type ReadLog struct {
	mu       sync.Mutex
	total    int
	byTarget map[string]int
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
