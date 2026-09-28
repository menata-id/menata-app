package domain

// ActionEffect declares what one named Action writes onto the record it acts on, beyond the value the
// request already carries (006-runtime-model.md §Domain Model names Approve and Reject as Actions;
// ROADMAP.md "Document Approval: closing the last three layers", Stage B).
//
// It closes the audit's Gap A. `decide` existed here as a *name* long before this: KnownActions carried
// it, a Permission gated it (prm_decide_own_step), Transitions said which moves it performs -- and what
// it actually *wrote* was two lines of Go naming fld_decision and fld_decided_by_name. So a Machine an
// Application bound to an engine had to carry those exact Field ids or the binding engaged over records
// the engine could not write, which is what kept the workflow binding from being useful to anything but
// Document Approval's own copy.
//
// **What an effect does not say.** The status move itself is not here, because a Transition already
// declares it: mch_approval_step's trn_step_approve says field fld_decision, from pending, to approved,
// action decide. Repeating that as a write would be two sources for one answer (001 Principle #8), so
// the runtime derives the moved Field and the legal target values from the declared edges and an effect
// carries only the companions -- here the decider's name. Where a Machine deliberately declares no
// person-performed edge for a Field (mch_document's fld_status is derived from its steps, and
// internal/conformance.TestDocumentStatusIsDerivedNotSettable holds that literally), an effect may write
// a declared literal instead: that is reachable only from that Action's own route and its own
// Permission, never from the generic update route, which is the property that gate exists to protect.
type ActionEffect struct {
	// Action is which of KnownActions this effect belongs to. One effect per Action per Machine.
	Action string
	// Writes are the Fields it sets, in declaration order.
	Writes []FieldWrite
}

// FieldWrite is one Field an Action sets, and where the value comes from: either a source the runtime
// resolves (From, one of KnownWriteSources) or a literal declared here (Value). Exactly one of the two.
type FieldWrite struct {
	// Field is a Field on the Machine declaring this effect (validated at load).
	Field string
	// From names a runtime-resolved source, or "" when Value carries a literal.
	From string
	// Value is a literal, for the case where the value is part of the declaration rather than of the
	// request -- `revise` moving a Document back to draft. Validated against the Field's own options
	// when it declares any, so a literal here can never be a value no screen can render.
	Value string
}

const (
	// WriteFromSubmitted is the value the request carried for this Field.
	WriteFromSubmitted = "submitted"
	// WriteFromActor is the acting identity's own record id -- what mch_document's fld_submitted_by
	// holds, stamped from the session rather than accepted from a form.
	WriteFromActor = "actor"
	// WriteFromActorName is the acting identity's display name, as it stands at the moment of the
	// Action: a **snapshot**, deliberately not a reference (mch_approval_step's fld_decided_by_name --
	// a live lookup would rewrite the name printed on an already-signed PDF whenever that person later
	// changed theirs).
	WriteFromActorName = "actor_name"
)

// KnownWriteSources is the closed set a FieldWrite's From may name -- the same static-seam discipline
// KnownActions/KnownServices/KnownWorkflowEngines follow (007 §14), and for the same reason: a source
// the runtime cannot resolve would write an empty value and look like it worked.
//
// `now` is deliberately absent. The audit's own sketch named it as a third source, and nothing in this
// repo writes a Field from it -- there is no fld_decided_at. Adding it would be generalizing on zero
// cases, which is the one thing this repo's Method forbids; the day a Machine declares such a Field is
// the day it earns a line here.
var KnownWriteSources = map[string]bool{
	WriteFromSubmitted: true,
	WriteFromActor:     true,
	WriteFromActorName: true,
}

// EffectFor returns the effect m declares for actionName, if any. A Machine declaring none for an
// Action it otherwise permits is normal: `edit` and `delete` write nothing beyond the submitted values.
func (m *Machine) EffectFor(actionName string) (ActionEffect, bool) {
	for _, e := range m.ActionEffects {
		if e.Action == actionName {
			return e, true
		}
	}
	return ActionEffect{}, false
}

// ActionTargets is the set of values actionName may move field to, read off the declared Transitions --
// which is how a route stops hardcoding the pair of values it accepts.
//
// internal/web's own submittedDecision checked against the literals "approved"/"rejected" until
// 2026-09-28; those two strings are exactly the `to:` values of the two edges naming `action: decide`,
// so the check is a derivation rather than a list. Empty means this Machine declares no edge for that
// Action on that Field, and the caller has nothing to accept.
func (m *Machine) ActionTargets(actionName, field string) []string {
	var out []string
	for _, t := range m.Transitions {
		if t.Action == actionName && t.Field == field {
			out = append(out, t.To)
		}
	}
	return out
}

// ActionField is the Field actionName moves on this Machine, derived from the declared Transitions.
// Empty when no edge names that Action, and empty when two edges name it on *different* Fields -- an
// ambiguity metadata validation refuses (validateActionEffects), so reaching that here would mean
// something loaded that should not have.
func (m *Machine) ActionField(actionName string) string {
	field := ""
	for _, t := range m.Transitions {
		if t.Action != actionName {
			continue
		}
		if field != "" && field != t.Field {
			return ""
		}
		field = t.Field
	}
	return field
}
