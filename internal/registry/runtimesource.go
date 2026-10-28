package registry

import "sort"

// RuntimeSource is one kind of record the runtime itself holds, as opposed to the Workspace's own: its members, its
// groups. A Machine that declares `runtime: <id>` is not stored as records; its rows are provided by the runtime for the
// viewer and request, and a page reads them through the Datasets, Projection and Components every Machine has
// (007 §7.1: a DataSource may be "a runtime service source where explicitly supported", and this is where it is).
//
// It is a dispatch seam in the sense `Services` is: the closed set and what a source promises live here, the provider
// that reads the database stays in `internal/composition`, and `TestRuntimeSourcesAndProvidersAgree` is the only thing
// binding the two.
type RuntimeSource struct {
	ID string
	// Provides are the Field ids a Machine declaring this source must declare, because the provider fills them. A Machine
	// may declare more Fields (they are empty); it may not omit one, or a page reading its role would read nothing.
	Provides []string
	// Doc says what the source holds and who may read it. Reading is gated by the page that binds it, never by the source.
	Doc string
	// RecordRoute is the screen one record of this source opens, with {id} where the record's id goes ("" when none). A
	// runtime record has no generic /machines/<id>/records/<rid> page, so `from: {href: record}` on a page of the
	// runtime's own is this.
	RecordRoute string
	// Params are the request parameters the provider reads to narrow its rows (a search box's `q`). Nothing else of the
	// request reaches a provider.
	Params []string
	// Provider says where the reader lives when it is not `internal/composition`: "web" for a source that needs the process's
	// own configuration (the backup directory), which composition cannot see. Empty is composition.
	Provider string
}

// RuntimeSources is the closed set a Machine's `runtime:` may name.
var RuntimeSources = map[string]RuntimeSource{
	"members": {ID: "members", Provides: []string{"fld_id", "fld_title"}, RecordRoute: "/workspace-members/{id}/edit", Params: []string{"q"},
		Doc: "the Workspace's members, each with the roles they hold, the Groups they belong to and whether they are active; `q` narrows by name or email; for the Workspace's admins"},
	"invites": {ID: "invites", Provides: []string{"fld_id", "fld_title"}, Params: []string{"q"},
		Doc: "the invitations nobody has accepted yet, each with the roles it will become; `q` narrows by email; for the Workspace's admins"},
	"settings_applications": {ID: "settings_applications", Provides: []string{"fld_id", "fld_title"},
		Doc: "the Applications of this Workspace that declare roles and a settings hub, one record each, by the hub's navigation id: the Application's name, what its members screen says, how many active members have access, and the way back to its hub; for the Workspace's admins"},
	"application_members": {ID: "application_members", Provides: []string{"fld_id", "fld_hub", "fld_title"},
		Doc: "every member of the Workspace once for each Application that declares roles, keyed by the Application's settings hub id: who they are, the roles a Group adds, and the one role they hold directly there; for the Workspace's admins"},
	"applications": {ID: "applications", Provides: []string{"fld_id", "fld_title"}, Provider: "web",
		Doc: "the Applications the viewer can open in this Workspace, each with their role there, where it opens and its pending count or summary count; for any member (it is the viewer's own). A Workspace's admins also get the Applications the assistant has generated and nobody has published, as `Draft` rows with where to review each"},
	"workspace_applications": {ID: "workspace_applications", Provides: []string{"fld_id", "fld_title"}, Provider: "web", RecordRoute: "/builder/{id}", Params: []string{"as"},
		Doc: "every Application installed in this Workspace, switched off or not, one record each: its name, what it holds and whether it is Active or Inactive; the rows the Workspace settings page switches on and off; for the Workspace's admins"},
	"builder_nodes": {ID: "builder_nodes", Provides: []string{"fld_id", "fld_app", "fld_title"}, Provider: "web", Params: []string{"node", "tab", "rec"},
		Doc: "the parts of every Application of this Workspace the builder lists, one record each, keyed by the Application's id and the part's place in its metadata: its Machines with their Fields, its screens, its menu and the workflow of each Machine that has statuses, each with the kind it is and the address that selects it; the part `node` names also carries the metadata file that declares it when `tab` is code, and a Field its name and whether it is required (what the Edit tab starts from); read only; for the Workspace's admins"},
	"builder_drafts": {ID: "builder_drafts", Provides: []string{"fld_id", "fld_app", "fld_title"}, Provider: "web",
		Doc: "the changes the viewer has staged in the Application Builder and not published yet, one record for each Application they hold a draft of: how many, and what the plan says each would do; read only; for the Workspace's admins"},
	"builder_steps": {ID: "builder_steps", Provides: []string{"fld_id", "fld_node", "fld_title"}, Provider: "web", Params: []string{"node", "try", "as"},
		Doc: "the statuses of the workflow the part `node` names (a Machine's lifecycle), one record each in the order the Machine declares them: the status, the meaning it is given (start, active, attention, done, stopped), how many records stand in it now, the edges that leave it with who may take each, and what is wrong with it when something is (a dead end, a status nothing leads to); with `try` (a status) and `as` (a role) it also says where a person standing in that status as that role could go, which writes nothing; read only; for the Workspace's admins"},
	"builder_runs": {ID: "builder_runs", Provides: []string{"fld_id", "fld_node", "fld_title"}, Provider: "web", Params: []string{"node"},
		Doc: "the last runs of the rule the part `node` names (an Event of a Machine), newest first: when, on which record, whether it ran, had nothing to act on (quiet) or failed, and a line saying what; this is what tells a rule that is quiet from one that is broken; read only; for the Workspace's admins"},
	"builder_roles": {ID: "builder_roles", Provides: []string{"fld_id", "fld_app", "fld_title"}, Provider: "web", Params: []string{"node", "tab", "as", "try"},
		Doc: "the ways the builder's preview of each Application can be looked at, one record each: as the admin who is building it, then as someone who holds only each role the Application declares; each says whether the request chose it and carries the builder's address with that choice; read only; for the Workspace's admins"},
	"application_suggestions": {ID: "application_suggestions", Provides: []string{"fld_id", "fld_title"},
		Doc: "the Workspace's own example prompts for the assistant (`suggested_applications:` in its manifest), each with where it starts a conversation"},
	"current_workspace": {ID: "current_workspace", Provides: []string{"fld_id", "fld_title"}, Provider: "web",
		Doc: "the Workspace the request is in, with the viewer's role in it"},
	"templates": {ID: "templates", Provides: []string{"fld_id", "fld_title"}, Provider: "web",
		Doc: "the ready-made applications of the template library, each with what installing it into this Workspace would do (renames, references added, or why it cannot be installed); for the Workspace's admins"},
	"workspace_files": {ID: "workspace_files", Provides: []string{"fld_id", "fld_path"}, Provider: "web", Params: []string{"path"},
		Doc: "the YAML files this Workspace owns, each with the content an editor starts as (the viewer's own unsent draft when they have one, else the file's current content); for the Workspace's admins"},
	"file_reviews": {ID: "file_reviews", Provides: []string{"fld_id", "fld_path"}, Provider: "web", Params: []string{"path"},
		Doc: "the loader's verdict on the viewer's own draft of a file, one record per file they have a draft of: whether it loads, why not, and whether applying it would change anything; for the Workspace's admins"},
	"file_diffs": {ID: "file_diffs", Provides: []string{"fld_id", "fld_path", "fld_title"}, Provider: "web", Params: []string{"path"},
		Doc: "what applying the viewer's own draft would change, one record per file the writer would touch, keyed by the file they are reviewing; for the Workspace's admins"},
	"snapshots": {ID: "snapshots", Provides: []string{"fld_id", "fld_title"}, Provider: "web",
		Doc: "the Workspace's saved installations, newest first, each with when it was taken and how large; for the Workspace's admins"},
	"groups": {ID: "groups", Provides: []string{"fld_name"}, RecordRoute: "/workspace-groups/{id}", Doc: "the Workspace's Groups, each with its member count and the role it grants per Application; for the Workspace's admins"},
}

// RuntimeSourceIDs lists the set, sorted, for messages.
func RuntimeSourceIDs() []string {
	ids := make([]string, 0, len(RuntimeSources))
	for id := range RuntimeSources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
