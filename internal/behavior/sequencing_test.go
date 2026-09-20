package behavior

import (
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// approvalSequencing mirrors metadata/approval_step.yaml's own declaration. These cases moved here
// from internal/action's CanDecide tests when the rule stopped being hardcoded to one Machine
// pair -- same coverage, now exercising the declared binding rather than baked-in field ids.
func approvalSequencing() *domain.Sequencing {
	return &domain.Sequencing{
		ParentField:     "fld_document",
		ModeField:       "fld_mode",
		SequentialValue: "sequential",
		OrderField:      "fld_sequence",
		StateField:      "fld_decision",
		OpenValue:       "pending",
	}
}

func parentInMode(mode string) *data.Record {
	return &data.Record{ID: "doc_1", Values: map[string]any{"fld_mode": mode}}
}

func stepRec(id string, order float64, decision string) *data.Record {
	return &data.Record{ID: id, Values: map[string]any{"fld_sequence": order, "fld_decision": decision}}
}

func TestCanAct(t *testing.T) {
	s1 := stepRec("rec_1", 1, "pending")
	s2 := stepRec("rec_2", 2, "pending")
	s1Approved := stepRec("rec_1", 1, "approved")
	s3 := stepRec("rec_3", 3, "pending")

	for _, tc := range []struct {
		name     string
		mode     string
		record   *data.Record
		siblings []*data.Record
		want     bool
	}{
		{"parallel never locks", "parallel", s2, []*data.Record{s1, s2}, true},
		{"sequential locked by an earlier open sibling", "sequential", s2, []*data.Record{s1, s2}, false},
		{"sequential unlocked once the earlier one is decided", "sequential", s2, []*data.Record{s1Approved, s2}, true},
		{"later open siblings never lock", "sequential", s2, []*data.Record{s1Approved, s2, s3}, true},
		{"first in order is never locked", "sequential", s1, []*data.Record{s1, s2}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanAct(approvalSequencing(), parentInMode(tc.mode), tc.record, tc.siblings); got != tc.want {
				t.Errorf("CanAct(%s) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

// TestCanAct_undeclaredIsAlwaysActionable pins the opt-in contract: ordering is something a Machine
// declares, so a Machine that declares none must never lock anything -- which is what every
// Machine other than Approval Step relies on.
func TestCanAct_undeclaredIsAlwaysActionable(t *testing.T) {
	s2 := stepRec("rec_2", 2, "pending")
	siblings := []*data.Record{stepRec("rec_1", 1, "pending"), s2}

	if !CanAct(nil, parentInMode("sequential"), s2, siblings) {
		t.Error("CanAct(nil sequencing) = false, want true: a Machine declaring no ordering never locks a record")
	}
	if !CanAct(approvalSequencing(), nil, s2, siblings) {
		t.Error("CanAct(nil parent) = false, want true: with no parent there is no mode saying ordering applies")
	}
}

func TestSequencingMode(t *testing.T) {
	if got := SequencingMode(approvalSequencing(), parentInMode("parallel")); got != "parallel" {
		t.Errorf("SequencingMode() = %q, want %q", got, "parallel")
	}
	if got := SequencingMode(nil, parentInMode("parallel")); got != "" {
		t.Errorf("SequencingMode(nil) = %q, want empty", got)
	}
}
