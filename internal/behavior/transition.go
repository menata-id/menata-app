package behavior

import (
	"fmt"

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
