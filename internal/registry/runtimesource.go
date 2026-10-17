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
		Doc: "the Applications the viewer can open in this Workspace, each with their role there, where it opens and its pending count or summary count; for any member (it is the viewer's own)"},
	"draft_applications": {ID: "draft_applications", Provides: []string{"fld_id", "fld_title"}, Provider: "web",
		Doc: "the Applications the assistant has generated and nobody has published, each with where to review it; for the Workspace's admins"},
	"application_suggestions": {ID: "application_suggestions", Provides: []string{"fld_id", "fld_title"},
		Doc: "the Workspace's own example prompts for the assistant (`suggested_applications:` in its manifest), each with where it starts a conversation"},
	"current_workspace": {ID: "current_workspace", Provides: []string{"fld_id", "fld_title"}, Provider: "web",
		Doc: "the Workspace the request is in, with the viewer's role in it"},
	"templates": {ID: "templates", Provides: []string{"fld_id", "fld_title"}, Provider: "web",
		Doc: "the ready-made applications of the template library, each with what installing it into this Workspace would do (renames, references added, or why it cannot be installed); for the Workspace's admins"},
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
