package rendering

// RelationOption is one selectable target record for a relation field's <select>: the target
// record's id, and a human label derived from its first Field (006-runtime-model.md "Relation").
type RelationOption struct {
	ID    string
	Label string
}

// RelationOptions maps a target Machine ID to its available records, for every relation field on
// the Machine currently being rendered.
type RelationOptions map[string][]RelationOption

// GroupOptions is every Workspace Group a `group` Field may name (CAP-F24, Fase 6c-1).
//
// A flat slice, not a map keyed by target the way RelationOptions is, because there is nothing to
// key it by: a Group is not a Machine, so every group Field in a Workspace draws from the same one
// list. That is also why this is a separate type rather than another entry in RelationOptions --
// that map's key *is* a Machine id, and a Group id parked in it would type-check, render, and
// quietly mean something the rest of the runtime does not believe.
type GroupOptions []RelationOption

// GroupLabel resolves a stored Group id to its name, the group-side counterpart of RelationLabel.
// Falls back to the raw id for the same reason that one does: a Group deleted after a record named
// it should show something traceable rather than a blank.
func GroupLabel(groups GroupOptions, id string) string {
	if id == "" {
		return ""
	}
	for _, g := range groups {
		if g.ID == id {
			return g.Label
		}
	}
	return id
}

// CarryField is one hidden input a form must echo back so the generic update route does not erase
// it: a Field name and its current value, resolved by composition.carryForward.
//
// It carries no type and no role on purpose. These are not a projection -- they are a verbatim
// round-trip forced by the update route rewriting a whole record, and dressing them up as semantic
// fields would claim a meaning they do not have.
type CarryField struct {
	Name  string
	Value string
}
