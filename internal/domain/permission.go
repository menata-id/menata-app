package domain

// ActionDecide is the cross-record Action the runtime realizes beyond plain record writes:
// Approve/Reject on an Approval Step (ROADMAP.md Phase 12, `POST .../decide`).
const ActionDecide = "decide"

// ActionEdit and ActionDelete govern the generic record update/delete routes themselves (PUT/
// DELETE .../records/{id} and their JSON twins), generalizing the same record-scoped shape
// ActionDecide already used to a second and third real case (signature-marker drag/place/width
// PUT-ing to the generic update route with no identity check at all) -- the discipline
// deleteAllowed's own doc comment (internal/web/record.go) already names: "generalize on a
// second real case, never the first". Record *creation* has no existing record to match an
// actor field against yet, so it deliberately has no Action here.
const (
	ActionEdit   = "edit"
	ActionDelete = "delete"
)

// KnownActions is the closed set of Action names a Permission may govern. Like KnownFieldTypes,
// this is a deliberate static seam (007 §14), not a name inferred from metadata -- a Permission
// naming an Action the runtime does not have would silently protect nothing.
var KnownActions = map[string]bool{
	ActionDecide: true,
	ActionEdit:   true,
	ActionDelete: true,
}

// Permission expresses an authorization requirement for performing an Action
// (006-runtime-model.md "Permission"; 004-runtime-metadata.md "Domain Plane"). Permission scope
// is part of execution identity and must be established before any work is performed
// (005-runtime-lifecycle.md "Security Ordering").
//
// Phase 16 (ROADMAP.md) supports exactly one shape: record-scoped -- the acting identity must be
// the value of ActorField on the record being acted upon. Roles, groups, field-level scoping and
// a per-Machine CRUD matrix are deliberately not part of this shape; none is forced while there
// is one shared login credential. A second, differently-shaped rule generalizes this when it is
// actually needed, the same discipline Constraint (Phase 4) and Action (Phase 12) followed.
type Permission struct {
	ID string
	// Action is the Action this Permission governs, from KnownActions.
	Action string
	// ActorField names a reference Field on the same Machine whose value the acting identity
	// must match -- in practice a `person` Field, since that is what resolves to a real mch_user.
	ActorField string
}

// PermissionsFor returns every Permission m declares for the given Action. An empty result means
// the Action is unrestricted on this Machine: metadata describes exceptions, not defaults
// (001-design-principles.md Principle #6).
func (m *Machine) PermissionsFor(action string) []Permission {
	var out []Permission
	for _, p := range m.Permissions {
		if p.Action == action {
			out = append(out, p)
		}
	}
	return out
}
