package domain

import "menata.app/internal/expression"

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
	FieldTypeFile     FieldType = "file"
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
	FieldTypeFile:     true,
}

// UserMachineID is the implicit relation target for every FieldTypePerson field
// (ROADMAP.md Phase 7). Person is a semantic type in its own right, not a spelling of Relation
// metadata authors write out by hand -- but underneath, it references the same real mch_user
// records a Relation field would.
const UserMachineID = "mch_user"

// Field describes a semantic attribute of a Machine (006-runtime-model.md "Field").
type Field struct {
	ID       string
	Name     string
	Type     FieldType
	Required bool
	// Options enumerates valid values for FieldTypeStatus.
	Options []string
	// RelatedMachine is the target Machine ID for FieldTypeRelation, and is set automatically to
	// UserMachineID for FieldTypePerson (006-runtime-model.md "Relation": grounded in existing
	// Machine/reference semantics) -- see Parse's own normalization step. Empty for every other
	// type.
	RelatedMachine string
	// Default is the value a new record gets when this field is left unset at create time --
	// 001 Principle #5 ("Convention over Configuration"), the same posture already load-bearing
	// for Layout (a Machine with no view: block defaults to table, Phase 5). Already coerced to
	// this Field's own storage type by Parse (float64 for FieldTypeNumber, bool for
	// FieldTypeBoolean, string otherwise) -- callers never re-parse it. Nil means no default was
	// declared. Applies only at creation (internal/data.ApplyDefaults); an update that clears a
	// field back to empty is never silently re-filled, the same distinction SQL's own DEFAULT
	// makes.
	Default any
}

// IsReference reports whether f's value is a record id referencing another Machine -- true for
// FieldTypeRelation and FieldTypePerson alike, since both are grounded in RelatedMachine once
// normalized. Validation, rendering, and relation-option loading all use this instead of
// switching on Type themselves, so a future reference-shaped type doesn't need to be added in
// five places at once.
func (f Field) IsReference() bool {
	return f.RelatedMachine != ""
}

// Constraint expresses a declarative condition that must hold for a state transition
// (006-runtime-model.md "Constraint"). Constraints are evaluated by the runtime and are not
// arbitrary executable code.
//
// Phase 4 (ROADMAP.md) supports exactly one shape: block a field transition (On becomes
// WhenEquals) while a related Machine has any record matching BlockIf's condition. A second,
// differently-shaped rule generalizes this when it's actually needed, not before.
type Constraint struct {
	ID         string
	On         string
	WhenEquals string
	BlockIf    RelationBlock
}

// RelationBlock names a related Machine, the Field on that Machine that relates back to this
// one, and the Condition a related record must match to block the transition.
type RelationBlock struct {
	RelatedMachine string
	RelatedField   string
	Condition      expression.Comparison
}

// Event identifies something that already happened on a write and may run one declared runtime
// Service in response (006-runtime-model.md "Event"/"Service"; Behavioral Model:
// Event -> Action -> Permission/Constraint -> Service/Data operation -> State change/Event).
//
// Supports exactly one shape, mirroring Constraint's own discipline: On names a Field on this
// Machine; the Event fires after a successful update whose new value for that Field differs from
// the old one, optionally narrowed to one target value (WhenEquals -- empty means "any change").
// Record creation and schedule/time-based triggers (the shapes the other known hardcoded
// "write happened, run a side effect" cases would actually need -- record-created Activity
// logging, SLA-breach detection) are deliberately not built until a second real case needs them:
// generalize on a second real case, never assumed ahead of it.
type Event struct {
	ID         string
	On         string
	WhenEquals string
	Then       Service
}

// Service is one closed, runtime-owned side effect an Event may trigger (006-runtime-model.md:
// "Service implementation belongs to the runtime") -- a static seam (007 §14), the same
// discipline KnownActions/expression.KnownOps already established, not dynamic dispatch or a
// scripting mechanism.
//
// Summary is svc_log_activity's own message template -- {old}, {new}, and any Field id in braces
// are its only placeholders (deliberately not a general templating language, the same minimalism
// expression.Comparison already established for Constraint's own condition vocabulary).
// SummaryOverride/SummaryOverrideWhen express one real case (Task status moving to "done" reads
// "completed", not "moved from X to Y") needing a second, value-specific wording without
// generalizing to arbitrary conditional branching: at most one override, selected only when the
// new value equals SummaryOverrideWhen.
type Service struct {
	Name                string
	Summary             string
	SummaryOverrideWhen string
	SummaryOverride     string
}

// ServiceLogActivity is the one Service KnownServices realizes today: append an mch_activity
// record (internal/web's logActivity).
const ServiceLogActivity = "log_activity"

// KnownServices is the closed set of Service names a Service.Name may name, the same static-seam
// discipline KnownActions already established for Action.
var KnownServices = map[string]bool{
	ServiceLogActivity: true,
}

// Machine is the primary runtime realization unit for a business capability
// (006-runtime-model.md "Machine").
type Machine struct {
	ID          string
	Name        string
	Fields      []Field
	Constraints []Constraint
	Events      []Event
	Permissions []Permission
	View        View
}

// FieldByID returns the Field with the given id, if m declares one.
func (m *Machine) FieldByID(id string) (Field, bool) {
	for _, f := range m.Fields {
		if f.ID == id {
			return f, true
		}
	}
	return Field{}, false
}
