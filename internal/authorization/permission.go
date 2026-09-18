package authorization

import (
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
// Semantics:
//   - A Machine declaring no Permission for action leaves it unrestricted (metadata describes
//     exceptions, not defaults -- 001 Principle #6).
//   - Every declared Permission for action must pass; they are requirements, not alternatives.
//   - An empty actorID never satisfies a Permission -- an unidentified caller is not an actor.
//   - A record whose actor field is empty or is not a record id satisfies nothing: an
//     unassigned record is actionable by no one, rather than by everyone.
func AllowsAction(m *domain.Machine, action string, values map[string]any, actorID string) bool {
	for _, p := range m.PermissionsFor(action) {
		if actorID == "" {
			return false
		}
		assigned, ok := values[p.ActorField].(string)
		if !ok || assigned == "" || assigned != actorID {
			return false
		}
	}
	return true
}
