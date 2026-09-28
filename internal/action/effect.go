package action

import (
	"fmt"

	"menata.app/internal/domain"
)

// EffectInput is everything a declared write can draw on, gathered by the caller because gathering it
// is I/O and this package performs none (the same posture behavior/experience already take: a pure
// decision over already-fetched values).
//
// ActorName is the display name resolved for ActorID at this moment, and the snapshot semantics matter:
// mch_approval_step's fld_decided_by_name is printed onto a signed PDF, so it records the name as it
// stood when the decision was made rather than following that person's later changes.
type EffectInput struct {
	// Submitted is the value the request carried for a WriteFromSubmitted field. One value, not a map,
	// because a declared effect names which Field it lands on -- the request says *what*, the
	// declaration says *where*.
	Submitted string
	ActorID   string
	ActorName string
}

// ApplyEffect writes onto values whatever m declares actionName writes (domain.ActionEffect), and
// reports the Fields it set so a caller can log or assert on them.
//
// This is Stage B's whole point: the two lines that used to sit in internal/web --
// `step.Values[FieldStepDecision] = decision` and `[FieldStepDecidedByName] = deciderName(...)` -- named
// one Machine's Field ids, so the approval engine could only ever write records shaped exactly like
// Document Approval's own. What lands here instead is "apply what this Machine says this Action writes",
// which works for a Machine whose decision Field is called something else entirely.
//
// **The status move is not one of these writes.** It comes from the declared Transition (see
// domain.Machine.ActionField/ActionTargets) because a Transition already says field, from, to and which
// Action performs it; declaring it again here would be two sources for one answer. ApplyStatusMove below
// is that half.
//
// A Machine declaring no effect for actionName is not an error -- `edit` and `delete` write only what
// was submitted -- so this returns no fields and no error for that case.
func ApplyEffect(m *domain.Machine, actionName string, values map[string]any, in EffectInput) ([]string, error) {
	effect, ok := m.EffectFor(actionName)
	if !ok {
		return nil, nil
	}
	written := make([]string, 0, len(effect.Writes))
	for _, wr := range effect.Writes {
		value, err := resolveWrite(wr, in)
		if err != nil {
			return nil, fmt.Errorf("machine %s, action %s, field %s: %w", m.ID, actionName, wr.Field, err)
		}
		values[wr.Field] = value
		written = append(written, wr.Field)
	}
	return written, nil
}

// ApplyStatusMove writes the value actionName moves its declared status Field to, and reports which
// Field that was.
//
// The Field and the legal values both come from the Machine's own Transitions, which is what lets a
// route stop naming either: internal/web's own submittedDecision checked the submitted value against
// the literals "approved"/"rejected" until 2026-09-28, and those two strings are exactly the `to:` values
// of the edges declaring `action: decide`.
//
// Returns ("", nil) when this Machine declares no edge for the Action -- an Action that moves no status
// (`revise` on a Machine whose status is derived, where the value is declared as a literal write
// instead). Refuses a value no declared edge names, which is the check that used to be a hardcoded pair.
func ApplyStatusMove(m *domain.Machine, actionName string, values map[string]any, to string) (string, error) {
	field := m.ActionField(actionName)
	if field == "" {
		return "", nil
	}
	targets := m.ActionTargets(actionName, field)
	for _, target := range targets {
		if target == to {
			values[field] = to
			return field, nil
		}
	}
	return "", fmt.Errorf("%s must move %s to one of %v, got %q", actionName, field, targets, to)
}

func resolveWrite(wr domain.FieldWrite, in EffectInput) (string, error) {
	if wr.Value != "" {
		return wr.Value, nil
	}
	switch wr.From {
	case domain.WriteFromSubmitted:
		return in.Submitted, nil
	case domain.WriteFromActor:
		return in.ActorID, nil
	case domain.WriteFromActorName:
		return in.ActorName, nil
	default:
		// Unreachable through the loader (metadata.validateActionEffects refuses an unknown source and a
		// write with neither from: nor value:), and still an error rather than an empty write: a source
		// this function does not know would otherwise store "" and look like it had worked.
		return "", fmt.Errorf("no value: declared and %q is not a source this runtime resolves", wr.From)
	}
}
