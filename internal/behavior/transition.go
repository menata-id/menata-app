package behavior

import (
	"fmt"
	"math"
	"time"

	"menata.app/internal/domain"
)

// TransitionError reports one refused move of a status Field, carrying the Field and both values
// so a caller can say what was actually attempted rather than "invalid".
//
// A distinct type rather than ConstraintError's aggregate shape, because the two answer different
// questions and fail at different times: a Constraint is a *business condition* that may hold
// again later ("this Project still has open Tasks"), while an undeclared transition is a
// *modelling* fact that never changes for a given pair of values. Aggregating them would invite
// presenting "not a declared transition" as something the person could fix by waiting.
type TransitionError struct {
	Field string
	From  string
	To    string
	// Action is the Action a declared edge reserves this move for, or "" when the edge is not
	// declared at all. It is what turns the message from "you may not" into "use Approve/Reject".
	Action string
	// Declared is false when no edge exists for this move at all.
	Declared bool
}

func (e *TransitionError) Error() string {
	if !e.Declared {
		return fmt.Sprintf("%s cannot move from %q to %q -- no such transition is declared", e.Field, e.From, e.To)
	}
	if e.Action == "" {
		return fmt.Sprintf("%s moves from %q to %q by itself -- it is not set directly", e.Field, e.From, e.To)
	}
	return fmt.Sprintf("%s moves from %q to %q through the %s action, not this one", e.Field, e.From, e.To, e.Action)
}

// CheckTransitions evaluates a Machine's declared state model against one proposed write: every
// status Field whose value `next` changes must be moving along an edge the Machine declares, and
// that edge must be reserved for `action` -- the Action the route performing this write realizes
// (006-runtime-model.md's Behavioral Model: an Action is what carries a State change).
//
// Pure, like CheckConstraints/CanAct/RollupValue beside it: the caller fetches the record's
// current values and states which Action it is performing, and gets a decision back.
//
// Three deliberate non-rules, each the difference between an opt-in primitive and a freeze:
//
//   - A Field no Transition mentions is untouched (Machine.GovernsTransitions). Declaring the
//     state model of one Field must not lock every other status Field in the manifest -- metadata
//     describes exceptions, not defaults (001 Principle #6).
//   - A value that is not changing is not a transition, so re-submitting a record's current state
//     is always allowed. This is what keeps the generic update route usable at all: it rewrites a
//     whole record from whatever the form submits (see internal/rendering/signatureplacement's own
//     carry-forward list), so every status Field is present on every write.
//   - Creation is not a transition and never reaches here. A record has no previous value to move
//     *from*, and `from: ""` is not a state any Field declares.
func CheckTransitions(m *domain.Machine, action string, current, next map[string]any) error {
	for _, f := range m.Fields {
		if f.Type != domain.FieldTypeStatus || !m.GovernsTransitions(f.ID) {
			continue
		}
		proposed, present := next[f.ID]
		if !present {
			continue // this write does not mention the Field at all
		}
		from, to := displayValue(current[f.ID]), displayValue(proposed)
		if from == to {
			continue
		}
		t, ok := m.TransitionFor(f.ID, from, to)
		if !ok {
			return &TransitionError{Field: f.ID, From: from, To: to}
		}
		if t.Action != action {
			return &TransitionError{Field: f.ID, From: from, To: to, Action: t.Action, Declared: true}
		}
	}
	return nil
}

// MovedTransitions lists the declared edges a write takes: for each governed status Field that next changes, the
// edge from the current value to the proposed one. An undeclared move is CheckTransitions' to refuse, so it is
// skipped here.
func MovedTransitions(m *domain.Machine, current, next map[string]any) []domain.Transition {
	var out []domain.Transition
	for _, f := range m.Fields {
		if f.Type != domain.FieldTypeStatus || !m.GovernsTransitions(f.ID) {
			continue
		}
		proposed, present := next[f.ID]
		if !present {
			continue
		}
		from, to := displayValue(current[f.ID]), displayValue(proposed)
		if from == to {
			continue
		}
		if t, ok := m.TransitionFor(f.ID, from, to); ok {
			out = append(out, t)
		}
	}
	return out
}

// ApplyTransitionSets writes into next the Fields each edge the write takes sets from the clock (domain.Transition.Sets),
// overriding what the form carried: the date a task was completed is the day it was. It changes nothing for a write that
// takes no such edge, so an edit that leaves the status alone cannot rewrite a completion date.
func ApplyTransitionSets(m *domain.Machine, current, next map[string]any, now time.Time) {
	for _, t := range MovedTransitions(m, current, next) {
		for fieldID, source := range t.Sets {
			if source == domain.TransitionSourceToday {
				next[fieldID] = now.Format(domain.DateLayout)
			}
		}
	}
}

// HasTransitionSets reports whether any edge of m writes a Field from the clock, so a caller can skip the read of the
// stored record for the Machines that declare none.
func HasTransitionSets(m *domain.Machine) bool {
	for _, t := range m.Transitions {
		if len(t.Sets) > 0 {
			return true
		}
	}
	return false
}

// displayValue renders a stored value the same way a declared option is written in metadata, so
// "pending" read back out of JSONB compares equal to `from: pending`. Matches the fmt.Sprint
// comparison CheckConstraints and CanAct already use on the same values, deliberately -- three
// evaluators disagreeing about what a stored status *is* would be worse than any of their
// individual choices.
func displayValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// HasGenerates reports whether any edge of m generates records, so a caller can skip the read of the stored record.
func HasGenerates(m *domain.Machine) bool {
	for _, t := range m.Transitions {
		if t.Generates != nil {
			return true
		}
	}
	return false
}

// GeneratedBatch is the records an edge's `generates:` will create, planned but not written.
type GeneratedBatch struct {
	Machine   string
	LinkField string
	Rows      []map[string]any
}

// PlanGenerated plans the records the edge this write takes generates (domain.GenerateSchedule), or nil when the write
// takes no such edge. recordID is the record taking the edge: each row points back at it. The i-th row (from 1) is due i
// times DueEveryDays from now, and each carries an even share of the amount rounded to cents, the last taking the
// remainder so the shares add up exactly. A count that is not a whole number from 1 to MaxGeneratedRecords, or an amount
// that is not a number, is an error: refusing is better than writing a schedule nobody asked for.
func PlanGenerated(m *domain.Machine, recordID string, current, next map[string]any, now time.Time) (*GeneratedBatch, error) {
	for _, t := range MovedTransitions(m, current, next) {
		g := t.Generates
		if g == nil {
			continue
		}
		count, ok := next[g.CountField].(float64)
		if !ok || count != float64(int(count)) || count < 1 || count > domain.MaxGeneratedRecords {
			return nil, fmt.Errorf("%s must be a whole number from 1 to %d to generate the schedule, got %v", g.CountField, domain.MaxGeneratedRecords, next[g.CountField])
		}
		total, ok := next[g.AmountFrom].(float64)
		if !ok {
			return nil, fmt.Errorf("%s must be a number to split into the schedule, got %v", g.AmountFrom, next[g.AmountFrom])
		}
		n := int(count)
		cents := int64(math.Round(total * 100))
		share := cents / int64(n)
		batch := &GeneratedBatch{Machine: g.Machine, LinkField: g.LinkField}
		var given int64
		for i := 1; i <= n; i++ {
			c := share
			if i == n {
				c = cents - given
			}
			given += c
			batch.Rows = append(batch.Rows, map[string]any{
				g.LinkField:   recordID,
				g.DueField:    now.AddDate(0, 0, i*g.DueEveryDays).Format(domain.DateLayout),
				g.AmountField: float64(c) / 100,
			})
		}
		return batch, nil
	}
	return nil, nil
}
