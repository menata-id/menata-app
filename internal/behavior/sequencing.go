package behavior

import (
	"fmt"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// CanAct reports whether record is unlocked for action yet, given its parent and every sibling
// sharing that parent. Pure -- no I/O, same posture as CheckConstraints, MatchedEvents and
// RollupValue; the caller supplies the records, not a database.
//
// Generalizes what was internal/action's own CanDecide, hardcoded to one Machine pair's field ids.
// The rule was always generic: in any mode other than the declared sequential one every record is
// actionable, and in sequential mode a record is locked while any sibling earlier in the order is
// still open. Only the binding -- which Field orders, which Field holds state, which value means
// open, where the mode lives -- was specific, and that now comes from domain.Sequencing.
//
// The parent record is taken whole rather than the mode read out of it by each caller, so the
// mode's own field binding stays inside the declaration that names it instead of being repeated
// at four call sites.
//
// A Machine declaring no sequencing has every record always actionable, which is why the nil case
// returns true rather than being an error: ordering is opt-in.
func CanAct(seq *domain.Sequencing, parent, record *data.Record, siblings []*data.Record) bool {
	if seq == nil || parent == nil || fmt.Sprint(parent.Values[seq.ModeField]) != seq.SequentialValue {
		return true
	}
	myOrder := orderValue(seq, record)
	for _, s := range siblings {
		if s.ID == record.ID {
			continue
		}
		if orderValue(seq, s) < myOrder && fmt.Sprint(s.Values[seq.StateField]) == seq.OpenValue {
			return false
		}
	}
	return true
}

// orderValue reads a record's own position in the order. A record whose OrderField holds no number
// sorts as zero -- earliest -- so a missing value never silently unlocks a record that should be
// waiting behind others.
func orderValue(seq *domain.Sequencing, r *data.Record) float64 {
	v, _ := r.Values[seq.OrderField].(float64)
	return v
}

// SequencingMode reads the parent's declared mode value, nil-safe. Exposed because a screen may
// want to *show* the mode ("Sequential"/"Parallel") beside the steps it governs, and reading it
// through the declaration keeps that one field binding from being retyped at the display site.
func SequencingMode(seq *domain.Sequencing, parent *data.Record) string {
	if seq == nil || parent == nil {
		return ""
	}
	return fmt.Sprint(parent.Values[seq.ModeField])
}
