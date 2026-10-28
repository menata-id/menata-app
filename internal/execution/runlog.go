package execution

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"menata.app/internal/data"
)

// A rule's Service is best effort -- it never fails the write it follows -- which is why a rule that does nothing and a rule that is broken look the same from outside. runRecorder is how
// a Service says what happened without changing that: the failures it met, and the reasons it had nothing to do, ride on the context and runService writes them as one history row
// (data.EventRun) after the Service returns.
type runRecorder struct {
	mu       sync.Mutex
	failures []string
	quiet    []string
	did      []string
}

type recorderKey struct{}

func withRecorder(ctx context.Context) (context.Context, *runRecorder) {
	rec := &runRecorder{}
	return context.WithValue(ctx, recorderKey{}, rec), rec
}

func recorderOf(ctx context.Context) *runRecorder {
	rec, _ := ctx.Value(recorderKey{}).(*runRecorder)
	return rec
}

// fail logs a failure as the log always did and, when a rule is running, records it as the rule's.
func fail(ctx context.Context, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	if rec := recorderOf(ctx); rec != nil {
		rec.mu.Lock()
		rec.failures = append(rec.failures, msg)
		rec.mu.Unlock()
	}
}

// nothingToDo records why a rule that ran had no one or nothing to act on.
func nothingToDo(ctx context.Context, format string, args ...any) {
	if rec := recorderOf(ctx); rec != nil {
		rec.mu.Lock()
		rec.quiet = append(rec.quiet, fmt.Sprintf(format, args...))
		rec.mu.Unlock()
	}
}

// did records what a rule did, in a few words.
func did(ctx context.Context, format string, args ...any) {
	if rec := recorderOf(ctx); rec != nil {
		rec.mu.Lock()
		rec.did = append(rec.did, fmt.Sprintf(format, args...))
		rec.mu.Unlock()
	}
}

// outcome is the one word and the line a run is remembered by: failed if anything failed, quiet if it only had nothing to do, otherwise ran.
func (r *runRecorder) outcome() (string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case len(r.failures) > 0:
		return data.EventFailedOutcome, strings.Join(r.failures, "; ")
	case len(r.did) == 0 && len(r.quiet) > 0:
		return data.EventQuietOutcome, strings.Join(r.quiet, "; ")
	}
	return data.EventRanOutcome, strings.Join(r.did, "; ")
}

// remember writes a run to the history, best effort like everything about a rule.
func remember(ctx context.Context, in serviceInput, rec *runRecorder) {
	if in.svc.Store == nil || in.machine == nil || in.record == nil {
		return
	}
	outcome, detail := rec.outcome()
	if err := in.svc.Store.RecordEventRun(ctx, in.machine.ID, in.event.ID, in.record.ID, outcome, detail); err != nil {
		log.Printf("event run history (%s %s): %v", in.machine.ID, in.event.ID, err)
	}
}
