package composition

import (
	"math"
	"testing"
)

// weekStep adds `by` to the request's offset before clamping, so a huge `by`
// wraps around and lands on the opposite end. decodePageStep accepts any Atoi-able integer.
func WeekStepDoesNotOverflow(t *testing.T) {
	if got := windowStep("week", map[string]string{"week": "5"}, math.MaxInt); got != "520" {
		t.Errorf("week 5 stepped by MaxInt = %q, want 520 (the clamp)", got)
	}
	if got := windowStep("week", map[string]string{"week": "-5"}, math.MinInt); got != "-520" {
		t.Errorf("week -5 stepped by MinInt = %q, want -520 (the clamp)", got)
	}
}

// A month step clamps as a week step does -- before the sum, so a huge `by` cannot wrap -- but to its own bound, and "" is this month. A parameter
// that is not an offset is not stepped at all, and the range falls back to the week for any value that is not the month.
func TestWindowStepClampsMonthsToTheirOwnBoundAndRangeFallsBackToTheWeek(t *testing.T) {
	if got := windowStep("month", map[string]string{"month": "5"}, math.MaxInt); got != "120" {
		t.Errorf("a huge step forward from month 5 = %q, want the 120-month bound", got)
	}
	if got := windowStep("month", map[string]string{"month": "-5"}, math.MinInt); got != "-120" {
		t.Errorf("a huge step back from month -5 = %q, want -120", got)
	}
	if got := windowStep("month", map[string]string{"month": "1"}, -1); got != "" {
		t.Errorf("a step back to this month = %q, want the plain route", got)
	}
	if got := windowStep("month", map[string]string{"week": "7"}, 1); got != "1" {
		t.Errorf("a month step reads the month, not the week: %q, want 1", got)
	}
	if got := windowStep("range", map[string]string{}, 1); got != "" {
		t.Errorf("range is not an offset: %q, want none", got)
	}
	for in, want := range map[string]string{"": "week", "week": "week", "month": "month", "weekly": "week", "MONTH": "week"} {
		if got := requestRange(map[string]string{"range": in}); got != want {
			t.Errorf("requestRange(%q) = %q, want %q", in, got, want)
		}
	}
}
