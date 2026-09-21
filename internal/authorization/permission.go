package authorization

import (
	"fmt"

	"menata.app/internal/domain"
)

// AllowsAction reports whether actorID may perform action on the record whose field values are
// given (ROADMAP.md Phase 16 -- the Domain Plane's Permission primitive, 006-runtime-model.md
// "Permission").
//
// It is a pure function over already-fetched values, the same posture as behavior.CheckConstraints
// and action.CanDecide: the caller performs the I/O, so authorization itself stays deterministic
// and fully unit-testable. 007 §20 is explicit that permissions remain owned by this package --
// the Data and Experience planes consume its decisions rather than redefining them.
//
// THIS IS THE ONE FUNCTION. Both the handler that accepts a POST and the .templ that decides
// whether to draw the button call it, which is what makes "the button is shown" and "the request
// is allowed" incapable of disagreeing. Upstream engineered that same property deliberately for
// the same capability (one ResolveActorGate, called from the guard and the interpreter); here it
// was already true, and adding a second entry point for the group case would have thrown it away.
//
// domain.Actor carries the identity and its Workspace Groups together (see that type for why one
// value rather than two parameters). The group set is resolved once per request by the transport
// layer (internal/web) rather than through the lazy `func(groupID) map[string]bool` closure
// upstream threads, because this function is called from .templ files and the rendering plane
// performs no I/O (007 §20). That costs nothing here: the only question ever asked is "is *this*
// actor in that group", so the actor's own group set answers every call without a lookup.
//
// Semantics:
//   - A Machine declaring no Permission for action leaves it unrestricted (metadata describes
//     exceptions, not defaults -- 001 Principle #6).
//   - Every declared Permission for action must pass; they are requirements, not alternatives.
//     Within one Permission, several Roles are alternatives -- hold any one (CAP-P01).
//   - An empty actorID never satisfies a Permission -- an unidentified caller is not an actor.
//   - A record whose actor field is empty or is not a record id satisfies nothing: an
//     unassigned record is actionable by no one, rather than by everyone.
func AllowsAction(m *domain.Machine, action string, values map[string]any, actor domain.Actor) bool {
	for _, p := range m.PermissionsFor(action) {
		if actor.ID == "" {
			return false
		}
		if !allowsOne(m.ApplicationID, p, values, actor) {
			return false
		}
	}
	return true
}

// allowsOne resolves a single Permission against one record: the dynamic actor gate when this
// record opts into it, the declared ActorField otherwise.
//
// The fallback is last on purpose, and it is what made this shape adoptable without a migration.
// A record that never set ActorTypeField -- every Approval Step written before Fase 6c-1 -- takes
// exactly the path it took before, reading exactly the Field it read before. So the gate could be
// declared on an existing Permission without rewriting a single stored record, and the existing
// tests kept passing unchanged, which is the evidence that the old behaviour really is preserved
// rather than the claim that it is.
func allowsOne(applicationID string, p domain.Permission, values map[string]any, actor domain.Actor) bool {
	if !holdsOneOf(applicationID, p.Roles, actor) {
		return false
	}
	if p.DynamicActor != nil {
		switch fmt.Sprint(values[p.DynamicActor.ActorTypeField]) {
		case domain.ActorKindUser:
			return isActor(values[p.DynamicActor.ActorUserField], actor.ID)
		case domain.ActorKindGroup:
			groupID, ok := values[p.DynamicActor.ActorGroupField].(string)
			if !ok || groupID == "" {
				// A step set to Group that names no Group is actionable by nobody, the same way
				// an unassigned record is -- not by everybody.
				return false
			}
			return actor.InGroup(groupID)
		}
		// Unset or unrecognized: this record does not use the dynamic gate. Fall through.
	}
	if p.ActorField == "" {
		// A Permission that names no actor Field says nothing about *which* record may be acted
		// on -- only, through Roles above, about who may act at all. Returning true here is what
		// makes a role-only Permission mean what it reads as; falling through to the lookup below
		// would read values[""], find nothing, and deny everyone, which is the shape of an
		// authorization rule that silently protects everything. internal/metadata refuses a
		// Permission that declares no arm at all, so "" here always means a real role-only rule.
		return true
	}
	return isActor(values[p.ActorField], actor.ID)
}

// holdsOneOf is CAP-P01's own arm: the actor must hold at least one of the roles this Permission
// names, in the Application that claims the Machine declaring it.
//
// Declaring no role is "this Permission says nothing about roles", not "no role may" -- the same
// Principle #6 reading PermissionsFor already applies one level up, and the reason every
// Permission written before Fase 7 kept its exact previous answer.
//
// It runs *before* the record-scoped arms rather than after, which is not only ordering: a role
// check reads nothing off the record, so refusing here means a Permission can state a rule about
// who may act at all without the record having to name anybody -- which is what makes a
// role-bearing Permission declarable on a Machine that has no actor Field.
func holdsOneOf(applicationID string, roles []string, actor domain.Actor) bool {
	if len(roles) == 0 {
		return true
	}
	for _, role := range roles {
		if actor.HasRole(applicationID, role) {
			return true
		}
	}
	return false
}

func isActor(assigned any, actorID string) bool {
	s, ok := assigned.(string)
	return ok && s != "" && s == actorID
}
