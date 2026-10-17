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
}

// RuntimeSources is the closed set a Machine's `runtime:` may name.
var RuntimeSources = map[string]RuntimeSource{
	"groups": {ID: "groups", Provides: []string{"fld_name"}, Doc: "the Workspace's Groups, each with its member count and the role it grants per Application; for the Workspace's admins"},
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
