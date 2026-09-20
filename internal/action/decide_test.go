package action

import (
	"testing"

	"menata.app/internal/data"
)

func step(id string, seq float64, decision string) *data.Record {
	return &data.Record{
		ID: id,
		Values: map[string]any{
			FieldStepSequence: seq,
			FieldStepDecision: decision,
		},
	}
}

func TestCanDecide_parallelAlwaysUnlocked(t *testing.T) {
	s2 := step("rec_2", 2, DecisionPending)
	siblings := []*data.Record{step("rec_1", 1, DecisionPending), s2}

	if !CanDecide("parallel", s2, siblings) {
		t.Error("CanDecide(parallel) = false, want true: parallel mode never locks a step")
	}
}

func TestCanDecide_sequentialLockedByEarlierPendingStep(t *testing.T) {
	s2 := step("rec_2", 2, DecisionPending)
	siblings := []*data.Record{step("rec_1", 1, DecisionPending), s2}

	if CanDecide(ModeSequential, s2, siblings) {
		t.Error("CanDecide(sequential) = true, want false: step 1 is still pending")
	}
}

func TestCanDecide_sequentialUnlockedOnceEarlierStepsDecided(t *testing.T) {
	s2 := step("rec_2", 2, DecisionPending)
	siblings := []*data.Record{step("rec_1", 1, DecisionApproved), s2}

	if !CanDecide(ModeSequential, s2, siblings) {
		t.Error("CanDecide(sequential) = false, want true: step 1 is already approved")
	}
}

func TestCanDecide_sequentialIgnoresLaterPendingSteps(t *testing.T) {
	s2 := step("rec_2", 2, DecisionPending)
	siblings := []*data.Record{step("rec_1", 1, DecisionApproved), s2, step("rec_3", 3, DecisionPending)}

	if !CanDecide(ModeSequential, s2, siblings) {
		t.Error("CanDecide(sequential) = false, want true: only earlier (lower-sequence) steps should lock this one")
	}
}

func TestCanDecide_firstStepNeverLocked(t *testing.T) {
	s1 := step("rec_1", 1, DecisionPending)
	siblings := []*data.Record{s1, step("rec_2", 2, DecisionPending)}

	if !CanDecide(ModeSequential, s1, siblings) {
		t.Error("CanDecide(sequential) = false, want true: step 1 has no earlier sibling to lock it")
	}
}

func TestDocumentReference(t *testing.T) {
	cases := []struct {
		sortOrder int64
		want      string
	}{
		{1, "DOC-0001"},
		{91, "DOC-0091"},
		{0, "DOC-0000"},
		{10000, "DOC-10000"},
	}
	for _, c := range cases {
		if got := DocumentReference(c.sortOrder); got != c.want {
			t.Errorf("DocumentReference(%d) = %q, want %q", c.sortOrder, got, c.want)
		}
	}
}
