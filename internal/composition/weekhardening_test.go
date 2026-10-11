package composition

import (
	"math"
	"testing"
)

// weekStep adds `by` to the request's offset before clamping, so a huge `by`
// wraps around and lands on the opposite end. decodePageStep accepts any Atoi-able integer.
func WeekStepDoesNotOverflow(t *testing.T) {
	if got := weekStep(map[string]string{"week": "5"}, math.MaxInt); got != "520" {
		t.Errorf("week 5 stepped by MaxInt = %q, want 520 (the clamp)", got)
	}
	if got := weekStep(map[string]string{"week": "-5"}, math.MinInt); got != "-520" {
		t.Errorf("week -5 stepped by MinInt = %q, want -520 (the clamp)", got)
	}
}
