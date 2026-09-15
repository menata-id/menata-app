package domain

// FieldType is the semantic type of a Field, per 006-runtime-model.md "Field": metadata should
// prefer a semantic type over a renderer-specific widget.
type FieldType string

const (
	FieldTypeText     FieldType = "text"
	FieldTypeNumber   FieldType = "number"
	FieldTypeBoolean  FieldType = "boolean"
	FieldTypeDate     FieldType = "date"
	FieldTypeStatus   FieldType = "status"
	FieldTypePerson   FieldType = "person"
	FieldTypeMoney    FieldType = "money"
	FieldTypeRelation FieldType = "relation"
)

// KnownFieldTypes is the closed set of field types the runtime currently understands. New types
// are added here deliberately, not inferred, per 007 §14's static-registry seam.
var KnownFieldTypes = map[FieldType]bool{
	FieldTypeText:     true,
	FieldTypeNumber:   true,
	FieldTypeBoolean:  true,
	FieldTypeDate:     true,
	FieldTypeStatus:   true,
	FieldTypePerson:   true,
	FieldTypeMoney:    true,
	FieldTypeRelation: true,
}

// Field describes a semantic attribute of a Machine (006-runtime-model.md "Field").
type Field struct {
	ID       string
	Name     string
	Type     FieldType
	Required bool
	// Options enumerates valid values for FieldTypeStatus.
	Options []string
	// RelatedMachine is the target Machine ID for FieldTypeRelation; meaningless otherwise
	// (006-runtime-model.md "Relation": grounded in existing Machine/reference semantics).
	RelatedMachine string
}

// Machine is the primary runtime realization unit for a business capability
// (006-runtime-model.md "Machine").
type Machine struct {
	ID     string
	Name   string
	Fields []Field
}
