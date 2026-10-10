package web

import (
	"context"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// allowsActionResolvingParents is authorization.AllowsAction for a write handler: it reads the parent records
// a Permission's parent_actor arm names (authorization.ParentRefs), through the Workspace-scoped store, and
// asks authorization.AllowsActionWithParents (K21).
//
// The parents are fetched here, in the transport layer, because authorization is a pure function that a
// .templ also calls and the rendering plane performs no I/O. A Machine with no parent arm costs exactly what
// it did before: ParentRefs is empty, nothing is read.
//
// A parent that cannot be read is simply absent, and an absent parent refuses the Permission (it can only
// restrict). The read error is not surfaced as a 500: "I cannot see the Document this step points at" and
// "you are not its submitter" both mean this actor may not.
func allowsActionResolvingParents(ctx context.Context, store *data.Store, m *domain.Machine, action string, values map[string]any, actor domain.Actor) bool {
	refs := authorization.ParentRefs(m, action, values)
	var parents map[string]map[string]any
	if len(refs) > 0 {
		parents = make(map[string]map[string]any, len(refs))
		for _, ref := range refs {
			rec, err := store.GetRecord(ctx, ref.MachineID, ref.RecordID)
			if err != nil || rec == nil {
				continue
			}
			parents[ref.ViaField] = rec.Values
		}
	}
	return authorization.AllowsActionWithParents(m, action, values, actor, parents)
}
