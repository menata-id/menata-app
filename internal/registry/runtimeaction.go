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
	// Field, with Kind ActionInputRecordText, is the field of the record whose text the input starts as (a file's content).
	Field string
}

// The two dynamic input kinds an action may ask.
const (
	ActionInputMembers  = "members"
	ActionInputAppRoles = "app_roles"
	// ActionInputWorkspaceRole is a select of the Workspace's two roles, starting as the member's own.
	ActionInputWorkspaceRole = "workspace_role"
	// ActionInputMemberAppRole is one select of the roles of the Application a member-in-an-Application record belongs to,
	// starting as the role that member holds there directly. The record id says both (`<hub id>:<user record id>`).
	ActionInputMemberAppRole = "member_app_role"
	// ActionInputTemplateRoles is one select of the roles a library template declares, starting as its first, so the admin who
	// installs an Application holds a role in it at once (without one they would be locked out of what they just installed).
	// A template that declares none asks nothing.
	ActionInputTemplateRoles = "template_roles"
	// ActionInputRecordText is a code box that starts as the text one field of the record holds (a file's content), so a Form can
	// edit what a source provides without the page naming the field's value.
	ActionInputRecordText = "record_text"
	// ActionInputRecordLine is a line of text that starts as what one field of the record holds (a Field's name); ActionInputRecordFlag is a
	// tick that starts as that field's "true". Both read `Field` of the record, so a Form edits what a source provides without the page naming a value.
	ActionInputRecordLine = "record_line"
	ActionInputRecordFlag = "record_flag"
	// ActionInputRecordLines is a box of lines, one thing per line, that starts as the lines one field of the record holds (a Field's options).
	ActionInputRecordLines = "record_lines"
	// ActionInputFieldType is a select of the types of Field a person can add by choosing one (domain.AddableFieldTypes); ActionInputLines is an empty box of
	// lines and ActionInputFlag an unticked box, for a Form that makes something new rather than editing a record's own values.
	ActionInputFieldType = "field_type"
	ActionInputLines     = "lines"
	ActionInputFlag      = "flag"
	// ActionInputPartRoles is the roles the Application of a builder part declares as ticked choices, starting as the roles the part's `fld_roles` holds.
	ActionInputPartRoles = "part_roles"
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
	"deactivate_member":      {ID: "deactivate_member", Source: "members", Admin: true, Doc: "gates a member out of this Workspace; refused while an approval would be left undecidable"},
	"reactivate_member":      {ID: "reactivate_member", Source: "members", Admin: true, Doc: "lets a deactivated member open this Workspace again"},
	"deactivate_application": {ID: "deactivate_application", Source: "workspace_applications", Admin: true, Doc: "switches an Application off in this Workspace (`inactive: true` in its file): it leaves the menu and Home and every route it owns answers 404, while its Machines and records stay; saved first, load-verified, and reversible by reactivate_application"},
	"reactivate_application": {ID: "reactivate_application", Source: "workspace_applications", Admin: true, Doc: "switches an Application back on: removes `inactive: true` from its file, and it reappears in the menu and Home with its data as it was"},
	"undo_publish":           {ID: "undo_publish", Source: "workspace_applications", Admin: true, Doc: "takes back an Application the assistant published within the last ten minutes: its files, its Machines' files and its roles go, saved first, and the proposal is open for review again; refused once anyone has added a record to it (switch it off instead)"},
	"reload_workspace":       {ID: "reload_workspace", Admin: true, Doc: "re-reads this Workspace's files from disk; a file that does not load changes nothing live"},
	"restore_snapshot":       {ID: "restore_snapshot", Source: "snapshots", Admin: true, Doc: "puts the Workspace's installation back to a saved one and reloads it; saved first itself, so pressing it by mistake is undone"},
	"archive_workspace":      {ID: "archive_workspace", Admin: true, Doc: "makes the Workspace read-only and hides it from its members until an admin restores it"},
	"set_member_app_role": {ID: "set_member_app_role", Source: "application_members", Admin: true, Inputs: []ActionInput{{Name: "role", Label: "Role", Kind: ActionInputMemberAppRole}},
		Doc: "sets one member's direct role in one Application and nothing else: not their Workspace role, not their roles elsewhere"},
	"install_application": {ID: "install_application", Source: "templates", Admin: true, Inputs: []ActionInput{{Name: "role", Label: "Your role here", Kind: ActionInputTemplateRoles}},
		Doc: "installs a library template into this Workspace as that Workspace's own copy, reloads it, and grants the installer the role they chose"},
	"review_file": {ID: "review_file", Source: "workspace_files", Admin: true, Inputs: []ActionInput{{Name: "yaml", Label: "New content", Required: true, Kind: ActionInputRecordText, Field: "fld_yaml"}},
		Doc: "keeps the pasted content as the viewer's own draft of that file and shows what the loader says about it; writes nothing to the Workspace"},
	"apply_file":         {ID: "apply_file", Source: "file_reviews", Admin: true, Doc: "replaces the file with the viewer's draft of it (validated on a staged copy, snapshotted first, load-verified), then reloads the Workspace"},
	"discard_file_draft": {ID: "discard_file_draft", Source: "workspace_files", Admin: true, Doc: "drops the viewer's draft of a file"},
	"set_group_members": {ID: "set_group_members", Source: "groups", Admin: true, Inputs: []ActionInput{{Name: "member", Label: "Tick who belongs to this group", Kind: ActionInputMembers}},
		Doc: "replaces a Group's membership with exactly the members ticked"},
	"edit_field": {ID: "edit_field", Source: "builder_nodes", Admin: true, Inputs: []ActionInput{
		{Name: "name", Label: "Name", Required: true, Kind: ActionInputRecordLine, Field: "fld_name"},
		{Name: "required", Label: "Must be filled in", Kind: ActionInputRecordFlag, Field: "fld_required"},
		{Name: "editable_by", Label: "Who may change it (none ticked: anyone who may edit the record)", Kind: ActionInputPartRoles, Field: "fld_editable_by"},
		{Name: "options", Label: "Choices, one per line; after a bar the colour a status choice is drawn in (start, active, attention, done, stopped)", Kind: ActionInputRecordLines, Field: "fld_options"}},
		Doc: "changes the name of a Field, whether it is required and, for a Field that offers choices, which choices (added and removed as the same operations a conversation uses), as the same builder operations the assistant's changes use (reviewed against the installation's version, snapshotted, load-verified); about a part of the builder tree that is a Field"},
	"add_field": {ID: "add_field", Source: "builder_nodes", Admin: true, Inputs: []ActionInput{
		{Name: "name", Label: "Name", Required: true},
		{Name: "type", Label: "Type", Required: true, Kind: ActionInputFieldType},
		{Name: "options", Label: "Choices, one per line (for a status); after a bar its colour: start, active, attention, done or stopped", Kind: ActionInputLines},
		{Name: "required", Label: "Must be filled in", Kind: ActionInputFlag}},
		Doc: "adds a Field to the Machine the builder part names, as the add_field operation a conversation uses (its id is made from the name, the form, the lists and the boards offer it at once); about a part of the builder tree that is a Machine"},
	"set_permission_roles": {ID: "set_permission_roles", Source: "builder_nodes", Admin: true, Inputs: []ActionInput{
		{Name: "roles", Label: "Who may do this", Required: true, Kind: ActionInputPartRoles, Field: "fld_roles"}},
		Doc: "sets which of the Application's roles may do what one Permission of a Machine governs, as the set_permission_roles operation a conversation uses; at least one role stays, since a Permission with none would open the action to everyone; about a part of the builder tree that is a Permission"},
	"ask_assistant_about": {ID: "ask_assistant_about", Source: "builder_nodes", Admin: true, Inputs: []ActionInput{{Name: "wish", Label: "What should change?", Required: true}},
		Doc: "opens the assistant's conversation with what the person wants written about one part of the builder tree, naming the part (its kind, its place and its path -- structure only, never a record's contents); writes nothing, the assistant proposes and the person reviews before anything is applied"},
	"publish_draft": {ID: "publish_draft", Source: "builder_drafts", Admin: true, Doc: "writes the changes the viewer staged in the Application Builder for one Application together, through the builder's one write path (snapshot first, load-verified, the Workspace reloaded), and drops the draft; if the plan refuses them now, nothing is written and the draft stays"},
	"discard_draft": {ID: "discard_draft", Source: "builder_drafts", Admin: true, Doc: "drops the changes the viewer staged in the Application Builder for one Application; nothing was written"},
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
