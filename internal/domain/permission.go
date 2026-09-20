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
// Phase 16 (ROADMAP.md) shipped exactly one shape: record-scoped -- the acting identity must be
// the value of ActorField on the record being acted upon. Its own doc comment said "a second,
// differently-shaped rule generalizes this when it is actually needed, the same discipline
// Constraint (Phase 4) and Action (Phase 12) followed." Fase 6c-1 is that need: DynamicActor
// below is the second shape.
type Permission struct {
	ID string
	// Action is the Action this Permission governs, from KnownActions.
	Action string
	// ActorField names a reference Field on the same Machine whose value the acting identity
	// must match -- in practice a `person` Field, since that is what resolves to a real mch_user.
	//
	// It stays the fallback when DynamicActor is set: see that field's own comment.
	ActorField string
	// DynamicActor lets each *record* choose, at write time, which kind of actor gates it --
	// a named person or a Workspace Group's membership (CAP-F24, Fase 6c-1). Nil for a Permission
	// that only ever gates on ActorField, which is every Permission but one today.
	//
	// The per-record part is what is new. A person gate (ActorField) and group membership
	// (data.EffectiveRoles, Fase 4) both already existed separately; what nothing could express
	// was a step deciding for itself, at submission, which of the two applies -- board 08's own
	// User/Group toggle, and board 09's note "Approvers are supplied by Groups".
	DynamicActor *DynamicActorGate
}

// Actor is who is acting, as far as a Permission is concerned: an identity, and the Workspace
// Groups that identity belongs to.
//
// One value rather than two parameters, because the two halves are only ever meaningful together
// and a half-threaded actor fails silently: passing the id but forgetting the groups makes a
// Group-gated record look forbidden rather than erroring. Bundling them means every call site
// that has an Actor has a whole one (CAP-F24, Fase 6c-1).
//
// Groups is resolved once per request by the transport layer and is nil for an unidentified
// caller, for an identity in no Group, and -- deliberately -- for any caller that has not been
// taught to resolve it. All three fail closed.
type Actor struct {
	ID     string
	Groups map[string]bool
}

// InGroup reports whether this Actor belongs to groupID. Nil-safe: the zero Actor is in nothing.
func (a Actor) InGroup(groupID string) bool { return a.Groups[groupID] }

// DynamicActorGate names the three Fields that together let one record pick its own actor kind:
// a status Field holding User or Group, and the two candidate Fields it selects between.
//
// Three flat named fields rather than one nested block, mirroring ActorField's own shape on this
// same struct -- the same reasoning upstream recorded for the three flat columns it added to its
// permissions table ("consistent with this table's established convention"), and it keeps
// load-time validation able to check each one by name.
//
// Both may be declared at once, and that is the adoption path rather than an oddity: resolution
// tries this gate first and falls back to ActorField whenever a record's own ActorTypeField is
// unset or holds something unrecognized. Every Approval Step already in the database is exactly
// that case, so prm_decide_own_step could adopt this without a migration, a backfill, or a single
// invalidated test.
type DynamicActorGate struct {
	// ActorTypeField is a status Field whose value selects the arm: ActorKindUser or
	// ActorKindGroup. Any other value (including empty) means "this record does not use the
	// dynamic gate" and falls through to ActorField.
	ActorTypeField string
	// ActorUserField is a person Field naming one identity, used when ActorTypeField is User.
	ActorUserField string
	// ActorGroupField is a group Field naming one Workspace Group, used when ActorTypeField is
	// Group. Membership is resolved late -- at the moment someone tries to act, not at
	// submission -- which is what board 09's own note promises: "Changing group membership is
	// managed separately."
	ActorGroupField string
}

// ActorKindUser and ActorKindGroup are the two values a DynamicActorGate's ActorTypeField may
// hold. They are capitalized because they are the option values metadata declares, not internal
// identifiers -- a status Field's options are shown to a person choosing between them.
const (
	ActorKindUser  = "User"
	ActorKindGroup = "Group"
)

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
