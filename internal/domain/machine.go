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
// Supports two shapes, mutually exclusive (internal/metadata.validateEvent enforces exactly one):
// field-change (On names a Field on this Machine; the Event fires after a successful update whose
// new value for that Field differs from the old one, optionally narrowed to one target value --
// WhenEquals empty means "any change") and creation (OnCreate true; the Event fires once, when
// the record is first created -- On/WhenEquals are meaningless here, since there is no prior value
// to compare against). The creation shape generalizes what was internal/web's own hardcoded
// logRecordCreated switch (record-created Activity logging on mch_document/mch_task/mch_project --
// workflow-behavior-decomposition-criteria.md's own B1-B5 worked example, a second/third real case
// proven three times over before this was built, never assumed ahead of it). Schedule/time-based
// triggers (the shape SLA-breach detection would still want) remain the one deferred shape,
// waiting on their own second real case.
type Event struct {
	ID         string
	On         string
	WhenEquals string
	OnCreate   bool
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
	// Rollup is ServiceRollupParentStatus's own configuration, nil for every other Service.
	Rollup *Rollup
}

// Rollup derives a parent record's own status from the values its children currently hold: the
// declarative form of a parent-rollup cascade. Declared on the *child* Machine, because that is
// where the relation to the parent already lives, so every field it names is checkable against
// one Machine file plus the Machine that relation already points at.
//
// Three outcomes, in priority order: AnyValue wins the moment one child holds it (a single
// rejection decides the parent immediately, without waiting for the rest), AllValue applies only
// once every child holds it, and Default covers everything else including having no children yet.
//
// That ordering is the capability's substance, not an implementation detail -- it is what
// distinguishes a rollup from a plain count, and it matches the shape this replaces
// (internal/action's own DocumentStatus, deleted with this change) exactly.
type Rollup struct {
	// ParentField is the reference Field on this Machine pointing at the parent record.
	ParentField string
	// TargetField is the Field on the *parent* Machine this rollup writes.
	TargetField string
	// AnyValue, held by at least one child, sets the parent to AnySet.
	AnyValue string
	AnySet   string
	// AllValue, held by every child, sets the parent to AllSet.
	AllValue string
	AllSet   string
	// Default is what the parent becomes when neither rule fires, including when the parent has
	// no children at all.
	Default string
}

const (
	// ServiceLogActivity appends an mch_activity record (internal/web's logActivity).
	ServiceLogActivity = "log_activity"
	// ServiceRollupParentStatus writes a parent record's status from its children's own values
	// (behavior.RollupValue decides, internal/web performs the read and write).
	ServiceRollupParentStatus = "rollup_parent_status"
)

// KnownServices is the closed set of Service names a Service.Name may name, the same static-seam
// discipline KnownActions already established for Action.
var KnownServices = map[string]bool{
	ServiceLogActivity:        true,
	ServiceRollupParentStatus: true,
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
	Datasets    []Dataset
	// Sequencing is set only by a Machine whose records are acted on in order; nil means every
	// record is always actionable.
	Sequencing *Sequencing
	View       View
}

// DatasetByID returns the Dataset with the given id, if m declares one. Composed screens look
// their Dataset up by id rather than by position, so reordering the datasets: block in YAML is
// never a behavioral change.
func (m *Machine) DatasetByID(id string) (Dataset, bool) {
	for _, ds := range m.Datasets {
		if ds.ID == id {
			return ds, true
		}
	}
	return Dataset{}, false
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
