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
