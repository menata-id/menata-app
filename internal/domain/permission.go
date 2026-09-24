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

// ActionCreate governs the generic record-creation routes (POST .../records and the JSON twin).
//
// It did not exist until the authorization review of 2026-09-21, and the reason it did not is
// recorded above and in internal/web's own create handler: "record creation has no existing
// record to match an actor field against yet, so it deliberately has no Action here." That was a
// correct reading of the *record-scoped* shape and it stopped being the whole story when the role
// arm landed (CAP-P01) -- a rule about who may act at all needs no record to read. Creation was
// consequently the one write path in this runtime with no authorization check of any kind, on
// every Machine, and the audit found that before a case did.
//
// ActorField keeps a meaning here, and only the one it can have: at creation the values being
// checked are the ones *being submitted*, so `actor_field: fld_owner` reads "you may only create
// a record that names you". That is what closes the signature-forgery hole (metadata/signature.
// yaml) -- and it needed no new key, because "the actor must be this field's value" is already
// exactly what ActorField says; only the record it is read from is different.
const ActionCreate = "create"

// KnownActions is the closed set of Action names a Permission may govern. Like KnownFieldTypes,
// this is a deliberate static seam (007 §14), not a name inferred from metadata -- a Permission
// naming an Action the runtime does not have would silently protect nothing.
var KnownActions = map[string]bool{
	ActionDecide: true,
	ActionCreate: true,
	ActionEdit:   true,
	ActionDelete: true,
}

// WorkspaceRoleAdmin is the one Workspace-level role a Permission may require. Workspace roles are
// a closed, runtime-owned pair (admin/member) rather than a declared vocabulary the way an
// Application's own roles: are -- they describe administering the Workspace itself, which every
// Workspace has identically, so there is nothing per-Workspace to declare.
const WorkspaceRoleAdmin = "admin"

// WorkspaceRoleMember is the other half of that pair. It names no Permission -- KnownWorkspaceRoles
// deliberately excludes it, see below -- but chrome tests against it directly, and that is a
// different question with a different answer.
//
// The distinction is worth the constant: a screen deciding whether to *offer* an admin destination
// asks `!= member`, not `== admin`, because the shared admin credential holds no membership row at
// all and its role reads as "" while still being able to open the destination. Hiding the link from
// it would hide a link that works. Two readers rely on that exact split (workspacehome.templ's
// "Manage members" link, appshell.templ's Workspace menu), and a bare "member" literal in both was
// how the second one nearly got `== admin` instead.
const WorkspaceRoleMember = "member"

// KnownWorkspaceRoles is the closed set a Permission's WorkspaceRole may name. "member" is
// deliberately absent: requiring it would be a Permission that every member satisfies and every
// admin fails, which is never a rule anyone means.
var KnownWorkspaceRoles = map[string]bool{
	WorkspaceRoleAdmin: true,
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
	// Roles are Application roles, any one of which the actor must hold for this Permission to
	// pass (ROADMAP.md Case 03 Fase 7). Empty means this Permission states nothing about roles,
	// which is every Permission written before Fase 7 -- so adding the arm changed no existing
	// answer, the same adoption path DynamicActor took.
	//
	// This is `menata-runtime`'s CAP-P01 (role-based event permission, ✅, conformance T11/T12)
	// implemented rather than re-decided. Two properties are upstream's, not invented here:
	//
	//   - The roles are read from the *Application that claims this Permission's Machine*
	//     (Machine.ApplicationID), because a role word only means something inside one
	//     Application's declared vocabulary (Application.Roles) -- "approver" in Document Approval
	//     and "approver" in some future Procurement are different grants, and a Workspace-wide
	//     role namespace would silently merge them.
	//   - Several roles on one Permission are alternatives (hold ANY one), while several
	//     *Permissions* on one Action stay requirements (pass ALL) -- the existing AllowsAction
	//     contract, unchanged. So `roles: [approver, reviewer]` beside an `actor_field` reads
	//     "someone holding either role, AND the person this record names", which is upstream's own
	//     one-Permission-per-{role, owner_field}-pair shape written as one row instead of several.
	//
	// The actor's own side of this is domain.Actor.Roles: their *effective* roles
	// (data.EffectiveRoles -- direct assignment ∪ every role their Groups hold there), so a role
	// granted through a Group gates identically to one granted directly, which is CAP-O07's rule
	// and the same late resolution DynamicActor's group arm already relies on.
	Roles []string
	// WorkspaceRole, when set, requires the actor to hold that Workspace role -- today only
	// WorkspaceRoleAdmin. Empty on every Permission that says nothing about it, which is most.
	//
	// It is a *separate* key from Roles rather than a reserved word inside it, because the two
	// name different things and merging them would be the drift this runtime keeps catching: an
	// Application's roles: vocabulary is declared per Application and means whatever that
	// Application says, while admin/member is runtime-owned and identical everywhere. A single
	// list would make `roles: [admin, approver]` read as one alternation across two namespaces,
	// and an Application could shadow the Workspace's own word by declaring "admin" itself.
	//
	// It ANDs with Roles and the record-scoped arms, like every other arm: a Permission naming
	// both a Workspace role and an Application role requires both. It is deliberately NOT a
	// bypass -- holding admin satisfies a Permission that *asks* for admin, and changes nothing
	// about any Permission that does not. See internal/conformance's own test for why that is a
	// decision rather than an omission: an administrator who can approve a document they are not
	// an approver of is the audit hole an approval flow exists to close.
	WorkspaceRole string
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
	// Roles is this Actor's effective Application roles, keyed by Application id -- exactly what
	// data.EffectiveRoles returns (direct assignment ∪ every role their Groups hold there), so the
	// union rule stays expressed in the one pure function that already owns it rather than being
	// recomputed, or worse re-derived in SQL, here.
	//
	// Resolved once per request by the transport layer, like Groups, and nil for an unidentified
	// caller and for any caller not yet taught to resolve it -- both fail closed, since a
	// role-bearing Permission asks "does this actor hold one of these" and a nil map holds none.
	Roles map[string][]string
	// WorkspaceRole is this Actor's role in the Workspace the request is scoped to ("admin",
	// "member", or "" for an identity with no membership row). Read by a Permission declaring
	// WorkspaceRole; "" satisfies none, which is the fail-closed direction.
	WorkspaceRole string
}

// InGroup reports whether this Actor belongs to groupID. Nil-safe: the zero Actor is in nothing.
func (a Actor) InGroup(groupID string) bool { return a.Groups[groupID] }

// HasRole reports whether this Actor holds role in applicationID. Nil-safe, and deliberately
// false for an empty applicationID: a Machine no Application claims has no role vocabulary to
// name, so a role-bearing Permission on one would be unanswerable rather than universally true.
func (a Actor) HasRole(applicationID, role string) bool {
	if applicationID == "" {
		return false
	}
	for _, held := range a.Roles[applicationID] {
		if held == role {
			return true
		}
	}
	return false
}

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
