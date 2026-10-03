package experience

import (
	"testing"
	"time"

	"menata.app/internal/domain"
)

// TestResolveSLABadge covers the resolution that moved out of two `.templ` files on 2026-10-03.
//
// **It is the only verification this change has, and that is a measured statement rather than laziness.**
// The migration was checked against a worktree baseline over thirteen screens and all thirteen came back
// token-identical -- because **no record in the dev database has a non-empty `fld_due_date`**, so the badge
// has never rendered in this environment and a live diff could not have shown the markup change at all. The
// two "Due today" strings on those pages are a summary-tile label and a filter chip. Saying so is the point:
// thirteen identical screens would otherwise read as proof of something it cannot prove.
func TestResolveSLABadge(t *testing.T) {
	now := time.Date(2026, 10, 3, 14, 30, 0, 0, time.UTC)

	for _, tc := range []struct {
		name        string
		value       any
		wantPresent bool
		wantLabel   string
		wantTone    domain.BadgeTone
	}{
		{name: "overdue is a bad tone", value: "2026-10-01", wantPresent: true, wantLabel: "OVERDUE", wantTone: domain.ToneBad},
		{name: "today is not overdue", value: "2026-10-03", wantPresent: true, wantLabel: "Due today", wantTone: domain.ToneNeutral},
		{name: "tomorrow is singular", value: "2026-10-04", wantPresent: true, wantLabel: "1 day left", wantTone: domain.ToneNeutral},
		{name: "further out is plural", value: "2026-10-09", wantPresent: true, wantLabel: "6 days left", wantTone: domain.ToneNeutral},

		// The three absent cases. A Field declared in the date role is not required to hold a value, and a
		// screen must render nothing rather than break -- which is the one behaviour of the deleted
		// `slaBadgePill`/`parseSLA` pair worth keeping.
		{name: "nil renders nothing", value: nil},
		{name: "empty renders nothing", value: ""},
		{name: "unparseable renders nothing", value: "next tuesday"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSLABadge(tc.value, now)
			if got.Present != tc.wantPresent {
				t.Fatalf("ResolveSLABadge(%v).Present = %v, want %v", tc.value, got.Present, tc.wantPresent)
			}
			if !tc.wantPresent {
				return
			}
			if got.Label != tc.wantLabel || got.Tone != tc.wantTone {
				t.Errorf("ResolveSLABadge(%v) = {%q, %q}, want {%q, %q}", tc.value, got.Label, got.Tone, tc.wantLabel, tc.wantTone)
			}
		})
	}
}

// TestResolveSLABadgeIsDeterministic is the §4.6 half, asserted rather than assumed.
//
// The whole reason this function exists is that the two renderers it replaced called `EvaluateSLA(due,
// time.Now())`, so the same record produced "1 day left" and later "OVERDUE" with no input having changed.
// With `now` as a parameter that cannot happen -- and a future refactor that reaches for the clock again
// fails here as well as in `conformance.TestRenderingDoesNotReadTheClock`.
func TestResolveSLABadgeIsDeterministic(t *testing.T) {
	due := "2026-10-04"
	first := ResolveSLABadge(due, time.Date(2026, 10, 3, 0, 0, 1, 0, time.UTC))
	last := ResolveSLABadge(due, time.Date(2026, 10, 3, 23, 59, 59, 0, time.UTC))
	if first != last {
		t.Fatalf("the same due date resolved differently within one day: %+v then %+v -- the day truncation is not holding", first, last)
	}
	crossMidnight := ResolveSLABadge(due, time.Date(2026, 10, 4, 0, 0, 1, 0, time.UTC))
	if crossMidnight == first {
		t.Fatal("the same due date resolved identically across midnight -- `now` is not reaching the comparison, so the parameter is decorative")
	}
}
