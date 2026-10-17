package registry

import "sort"

// RuntimeAction is one operation the runtime performs on its own state (its groups, members, installation) that is not a
// write of a Machine's record: create a group, remove one. A page of the runtime's own names it with
// `binding: {write: action, action: <id>}` on a Form or a Button, and the runtime's route performs it.
//
// It is a dispatch seam like `Services`: the closed set, what each action asks and who may do it live here; the executor
// that touches the database lives in `internal/web`, and `TestRuntimeActionsAndRunnersAgree` is the only thing binding
// the two. A page never says how an action is done, only which one, so what the action may do is bounded by this list
// and not by what an author writes.
type RuntimeAction struct {
	ID string
	// Inputs are what a Form for this action asks, in order. A Button action asks nothing.
	Inputs []ActionInput
	// Source, when set, makes the action about one record of that runtime source (registry.RuntimeSources): it is valid only
	// on a Button inside a Collection of that source's Machine, and acts on that record. Empty means the action is not about
	// a record (create one).
	Source string
	// Admin says only a Workspace admin may perform it. Every action is, today; the field is here so a member-level
	// action is a statement and not an absence.
	Admin bool
	Doc   string
}

// ActionInput is one thing a Form action asks of the person.
type ActionInput struct {
	Name     string
	Label    string
	Required bool
	// Kind says what the input is when it is more than a line of text. "members" is the Workspace's members as a set of
	// ticked choices, starting as the record's own (a Group's members); "app_roles" expands to one select per Application
	// that declares roles, each starting as the record's role there. Both are decided per request by the runtime, so the page
	// writes neither an option nor a name.
	Kind string
}

// The two dynamic input kinds an action may ask.
const (
	ActionInputMembers  = "members"
	ActionInputAppRoles = "app_roles"
	// ActionInputWorkspaceRole is a select of the Workspace's two roles, starting as the member's own.
	ActionInputWorkspaceRole = "workspace_role"
)

// RuntimeActions is the closed set a binding's `action:` may name.
var RuntimeActions = map[string]RuntimeAction{
	"create_group": {ID: "create_group", Inputs: []ActionInput{{Name: "name", Label: "Group name", Required: true}}, Admin: true, Doc: "adds a Group to the Workspace"},
	"delete_group": {ID: "delete_group", Source: "groups", Admin: true, Doc: "removes a Group and what it granted"},
	"invite_member": {ID: "invite_member", Admin: true, Inputs: []ActionInput{{Name: "email", Label: "Email", Required: true}, {Name: "roles", Label: "Application roles", Kind: ActionInputAppRoles}},
		Doc: "records an invitation with the roles it will become and emails its link; the invitee states their own name when they accept"},
	"revoke_invite": {ID: "revoke_invite", Source: "invites", Admin: true, Doc: "withdraws an invitation nobody has accepted; its emailed link stops working"},
	"set_member_access": {ID: "set_member_access", Source: "members", Admin: true, Inputs: []ActionInput{
		{Name: "workspace_role", Label: "Workspace role", Required: true, Kind: ActionInputWorkspaceRole}, {Name: "roles", Label: "Application access", Kind: ActionInputAppRoles}},
		Doc: "sets a member's Workspace role and their direct role in each Application, from that Application's own declared roles"},
	"deactivate_member": {ID: "deactivate_member", Source: "members", Admin: true, Doc: "gates a member out of this Workspace; refused while an approval would be left undecidable"},
	"reactivate_member": {ID: "reactivate_member", Source: "members", Admin: true, Doc: "lets a deactivated member open this Workspace again"},
	"reload_workspace":  {ID: "reload_workspace", Admin: true, Doc: "re-reads this Workspace's files from disk; a file that does not load changes nothing live"},
	"restore_snapshot":  {ID: "restore_snapshot", Source: "snapshots", Admin: true, Doc: "puts the Workspace's installation back to a saved one and reloads it; saved first itself, so pressing it by mistake is undone"},
	"archive_workspace": {ID: "archive_workspace", Admin: true, Doc: "makes the Workspace read-only and hides it from its members until an admin restores it"},
	"set_group_members": {ID: "set_group_members", Source: "groups", Admin: true, Inputs: []ActionInput{{Name: "member", Label: "Tick who belongs to this group", Kind: ActionInputMembers}},
		Doc: "replaces a Group's membership with exactly the members ticked"},
	"set_group_roles": {ID: "set_group_roles", Source: "groups", Admin: true, Inputs: []ActionInput{{Name: "roles", Label: "Application roles", Kind: ActionInputAppRoles}},
		Doc: "sets the role a Group grants in each Application, from that Application's own declared roles"},
}

// RuntimeActionIDs lists the set, sorted, for messages.
func RuntimeActionIDs() []string {
	ids := make([]string, 0, len(RuntimeActions))
	for id := range RuntimeActions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
