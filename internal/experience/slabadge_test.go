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

// TestResolveSLABadgeDetailIsDayScale holds the card footer's sentence (S1a, board 07): days, never
// hours, because a due date is a bare calendar date. Urgent is the line between "label above the
// title" and "footer only" -- today and overdue are urgent, tomorrow is not.
func TestResolveSLABadgeDetailIsDayScale(t *testing.T) {
	now := time.Date(2026, 10, 3, 14, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		due        string
		wantDetail string
		wantUrgent bool
	}{
		{"2026-09-30", "SLA breached · 3 days", true},
		{"2026-10-02", "SLA breached · 1 day", true},
		{"2026-10-03", "Due by the end of today", true},
		{"2026-10-04", "1 day remaining", false},
		{"2026-10-09", "6 days remaining", false},
	} {
		got := ResolveSLABadge(tc.due, now)
		if got.Detail != tc.wantDetail || got.Urgent != tc.wantUrgent {
			t.Errorf("ResolveSLABadge(%s) = {Detail %q, Urgent %v}, want {%q, %v}", tc.due, got.Detail, got.Urgent, tc.wantDetail, tc.wantUrgent)
		}
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

// The three states a card's date pill has, and the one rule that orders them: a finished card is never
// overdue. `now` is injected, so each row is deterministic.
func TestResolveCardDate(t *testing.T) {
	now := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		value     any
		done      bool
		wantLabel string
		wantTone  domain.BadgeTone
	}{
		{"ahead, this year", "2026-10-12", false, "12 Oct", domain.ToneNeutral},
		{"due today is not overdue", "2026-10-05", false, "5 Oct", domain.ToneNeutral},
		{"past", "2026-10-01", false, "1 Oct", domain.ToneBad},
		{"past but finished", "2026-10-01", true, "1 Oct", domain.ToneGood},
		{"finished before its date", "2026-10-12", true, "12 Oct", domain.ToneGood},
		{"another year keeps the year", "2027-01-03", false, "3 Jan 2027", domain.ToneNeutral},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveCardDate(tc.value, tc.done, now)
			if !got.Present || got.Label != tc.wantLabel || got.Tone != tc.wantTone || got.Done != tc.done {
				t.Errorf("ResolveCardDate(%v, %v) = %+v, want label %q tone %q", tc.value, tc.done, got, tc.wantLabel, tc.wantTone)
			}
		})
	}
	for _, v := range []any{nil, "", "not a date"} {
		if got := ResolveCardDate(v, true, now); got.Present {
			t.Errorf("ResolveCardDate(%v) = %+v, want nothing drawn", v, got)
		}
	}
}
