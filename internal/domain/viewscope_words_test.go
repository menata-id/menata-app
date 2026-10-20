package domain

import "testing"

// A scope's closing phrase is said in the count's own number: "1 project", not "1 projects" (the review of slice 5).
func TestAScopesPhraseAgreesWithItsCount(t *testing.T) {
	s := ViewScope{
		Chosen: "one of {n} projects in this workspace", ChosenOne: "the only project in this workspace",
		Overall: "{n} projects in this workspace", OverallOne: "{n} project in this workspace",
	}
	for _, tc := range []struct {
		chosen bool
		n      int
		want   string
	}{
		{false, 1, "1 project in this workspace"},
		{false, 0, "0 projects in this workspace"},
		{false, 3, "3 projects in this workspace"},
		{true, 1, "the only project in this workspace"},
		{true, 2, "one of 2 projects in this workspace"},
	} {
		if got := s.Phrase(tc.chosen, tc.n); got != tc.want {
			t.Errorf("Phrase(%v, %d) = %q, want %q", tc.chosen, tc.n, got, tc.want)
		}
	}
	// With no singular wording declared, the plural serves one too, as a PageCount's Other does.
	bare := ViewScope{Chosen: "one of {n}", Overall: "{n} in all"}
	if got := bare.Phrase(false, 1); got != "1 in all" {
		t.Errorf("no singular declared: %q", got)
	}
}
