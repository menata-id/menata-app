package domain

// ChildCollection names a Machine and the Field on it that references some other Machine's
// records back -- the reverse direction of Relation (006-runtime-model.md "Relation").
type ChildCollection struct {
	Machine *Machine
	Field   Field
}

// FindChildCollections returns every (Machine, Field) pair among machines whose Field
// references targetMachineID, used to show e.g. a Project's own Tasks embedded on the Project's
// detail page (ROADMAP.md Phase 9). A Field counts regardless of its semantic Type -- Relation
// and Person alike, per IsReference -- so a User's detail page gets "Tasks assigned to me" for
// free from the same mechanism.
func FindChildCollections(machines []*Machine, targetMachineID string) []ChildCollection {
	var refs []ChildCollection
	for _, m := range machines {
		for _, f := range m.Fields {
			if f.IsReference() && f.RelatedMachine == targetMachineID {
				refs = append(refs, ChildCollection{Machine: m, Field: f})
			}
		}
	}
	return refs
}
