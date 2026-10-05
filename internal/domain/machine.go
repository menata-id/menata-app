package domain

import (
	"regexp"

	"menata.app/internal/expression"
)

// FieldType is the semantic type of a Field, per 006-runtime-model.md "Field": metadata should
// prefer a semantic type over a renderer-specific widget.
type FieldType string

const (
	FieldTypeText FieldType = "text"
	// FieldTypeLongText is free text that keeps its line breaks: a multi-line input, and a value drawn
	// with its newlines intact (Case 19 PM02's Description). Stored as a plain string, so it validates
	// and parses exactly like FieldTypeText -- only the control and the detail rendering differ.
	FieldTypeLongText FieldType = "long_text"
	FieldTypeNumber   FieldType = "number"
	FieldTypeBoolean  FieldType = "boolean"
	FieldTypeDate     FieldType = "date"
	FieldTypeStatus   FieldType = "status"
	FieldTypePerson   FieldType = "person"
	FieldTypeMoney    FieldType = "money"
	FieldTypeRelation FieldType = "relation"
	FieldTypeFile     FieldType = "file"
	// FieldTypeGroup holds a Workspace Group's id (CAP-F24, Fase 6c-1) -- reference *sugar* over
	// the platform workspace_groups table, the same posture FieldTypePerson takes over mch_user,
	// except that a Group is not a Machine and so this has no RelatedMachine at all.
	//
	// That absence is the whole point and it is load-bearing: IsReference() must stay FALSE for
	// this type. Its contract is "relations[f.RelatedMachine] is that Machine's record list"
	// (internal/composition.Loader.RelationOptions, fieldInput), and a Group has no Machine, no
	// Fields, and a label that lives in a platform column rather than in a record. A group Field's
	// options come from the parallel rendering.GroupOptions instead.
	//
	// Declaring a thin mch_group was the alternative, and capabilities.md named this exact
	// question as the one "CAP-F24's approver_group is what will force". It is answered no: a
	// Machine mirroring workspace_groups would be a second source of truth for Group identity that
	// the Fase 4 admin screens do not write to. Upstream reached the same answer and shipped the
	// same shape (menata-runtime's model.FieldTypeGroup: "Unlike reference, never needs
	// Options.TargetMachine -- a Group isn't a Machine").
	FieldTypeGroup FieldType = "group"
)

// FieldTypeSpec is what a Field type *is* -- not how any plane implements it.
//
// **Deliberately a Domain fact rather than a registry entry, and 001-007 is why.** 004 puts Field in the
// Domain Plane; 007 §14 scopes internal/registry to "discovering how a component type is *implemented*"
// (contract → validator → resolver → renderer). Nothing here decides what to run: `metadata` still
// validates a declaration, `data` still coerces and checks a value, `rendering` still picks a control --
// and §14 explicitly permits each of those as "a Go map or compiler-checked switch", objecting only to
// "business-specific switch statements scattered across handlers". Measured 2026-09-30: the three that
// remain are in metadata/parse.go, data/validate.go and rendering/controls.templ. **None is a handler**,
// each is one resolution point for its own plane's concern, and each already has a `default`.
//
// So this replaced `map[FieldType]bool` without moving a single branch. What it removes is a *prose copy*:
// internal/aiassist described these ten types in a hand-written paragraph, and its own comment called
// keeping that in sync "a review discipline ... not a new kind of drift risk". It had drifted.
type FieldTypeSpec struct {
	// Label is how the type is named to a human or to a model -- one short phrase, no trailing period.
	Label string
	// NeedsOptions is true when a declaration is incomplete without `options:` (validated in
	// internal/metadata).
	NeedsOptions bool
	// ReferencesMachine is true when a value of this type is another record's id. Person is included:
	// Normalize binds it to mch_user, which is an inference (001 #6), not a hand-written target.
	ReferencesMachine bool
}

// KnownFieldTypes is the closed set of field types the runtime currently understands, each with what it
// is. New types are added here **and** wherever their plane implements them; an unrecognized type is a
// load-time error rather than a silent skip (capability-lifecycle.md §4 rule 3, "Unknown = explicit"),
// which internal/metadata/validate.go enforces by reading this map.
var KnownFieldTypes = map[FieldType]FieldTypeSpec{
	FieldTypeText:     {Label: "text"},
	FieldTypeLongText: {Label: "long text (multi-line)"},
	FieldTypeNumber:   {Label: "number"},
	FieldTypeBoolean:  {Label: "boolean"},
	FieldTypeDate:     {Label: "date"},
	FieldTypeStatus:   {Label: "status (with options)", NeedsOptions: true},
	FieldTypePerson:   {Label: "person (a user reference)", ReferencesMachine: true},
	FieldTypeMoney:    {Label: "money"},
	FieldTypeRelation: {Label: "relation (references another machine)", ReferencesMachine: true},
	FieldTypeFile:     {Label: "file"},
	FieldTypeGroup:    {Label: "group"},
}

// FieldTypeLabels lists every type's label in a stable order, which is what a generated description needs
// -- a map range would reorder the sentence between builds, and 007 §4.6 states determinism as a MUST.
func FieldTypeLabels() []string {
	order := []FieldType{FieldTypeText, FieldTypeLongText, FieldTypeNumber, FieldTypeBoolean, FieldTypeDate, FieldTypeStatus,
		FieldTypePerson, FieldTypeMoney, FieldTypeRelation, FieldTypeFile, FieldTypeGroup}
	out := make([]string, 0, len(order))
	for _, t := range order {
		out = append(out, KnownFieldTypes[t].Label)
	}
	return out
}

// UserMachineID is the implicit relation target for every FieldTypePerson field
// (development-history.md Phase 7). Person is a semantic type in its own right, not a spelling of Relation
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
	// Compute makes this a computed Field: its value is derived from other Fields of the same record
	// every time the record is written through the generic create/edit routes, and it is never an
	// input (007 §4.4: Composition resolves the shape; a person does not type a total). Nil for an
	// ordinary Field. Declared as `compute: {op: sum, fields: [...]}`; see FieldCompute.
	Compute *FieldCompute
}

// ComputeOp is one operation a computed Field may declare. A closed registry, like KnownAggregates:
// an operation is added here, with its evaluation below, when a real case needs it.
type ComputeOp string

const (
	// ComputeSum adds number Fields. The first case: a water-usage report totalling three meters.
	ComputeSum ComputeOp = "sum"
)

// KnownComputeOps is the closed set of operations a computed Field may declare.
var KnownComputeOps = map[ComputeOp]bool{ComputeSum: true}

// FieldCompute is how a computed Field derives its value: Op applied to Fields, in order. Operands
// are number Fields of the same Machine that are not computed themselves, which is what makes the
// evaluation order-free and cycle-free (internal/metadata validates both at load).
type FieldCompute struct {
	Op     ComputeOp
	Fields []string
}

// Evaluate derives the value from a record's own values. ok is false when no operand holds a value,
// so a record nobody has filled in yet shows an empty total rather than a made-up zero; an operand
// left empty otherwise counts as zero.
func (c FieldCompute) Evaluate(values map[string]any) (v any, ok bool) {
	switch c.Op {
	case ComputeSum:
		var total float64
		for _, id := range c.Fields {
			if n, isNum := values[id].(float64); isNum {
				total += n
				ok = true
			}
		}
		return total, ok
	}
	return nil, false
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

// MemberRemovalBlock declares that a record naming a person in ActorField, currently matching
// Condition, blocks *that person* from being deactivated out of the Workspace -- a platform
// action (internal/data.Store.DeactivateMember), not a Field transition on this Machine, which is
// why this is its own shape rather than a second Constraint (Constraint.BlockIf blocks a
// transition on a record that exists; here there is no record and no transition being blocked at
// all -- a person is).
//
// Introduced on its first real case (mch_approval_step's own pending-step guard, Flow 2 canvas
// re-audit, ROADMAP.md, 2026-09-27) rather than its second -- a deliberate, named exception to
// this repo's usual "wait for a second real case" discipline: the owner judged the shape general
// on inspection (an actor-Field plus a business-state condition, the same two ingredients
// Constraint.BlockIf and Schedule's own GuardField/GuardEquals already share), not assumed ahead
// of a concrete second need.
type MemberRemovalBlock struct {
	ID string
	// ActorField is a person-type Field on this Machine (validated at load,
	// internal/metadata.validateMemberRemovalBlock) -- the record naming the person being checked.
	ActorField string
	Condition  expression.Comparison
	// Reason is the sentence a blocked deactivation attempt shows -- CLAUDE.md's own posture on
	// every gate that refuses something: say why, don't just refuse.
	Reason string
}

// Event identifies something that already happened -- a write, or the passage of time -- and may
// run one declared runtime Service in response (006-runtime-model.md "Event"/"Service";
// Behavioral Model: Event -> Action -> Permission/Constraint -> Service/Data operation -> State
// change/Event).
//
// Supports three shapes, mutually exclusive (internal/metadata.validateEvent enforces exactly
// one): field-change (On names a Field on this Machine; the Event fires after a successful update
// whose new value for that Field differs from the old one, optionally narrowed to one target
// value -- WhenEquals empty means "any change"), creation (OnCreate true; the Event fires once,
// when the record is first created -- On/WhenEquals are meaningless here, since there is no prior
// value to compare against), and schedule (Schedule non-nil; the Event fires on the passage of
// time rather than on any write -- see Schedule). The creation shape generalizes what was
// internal/web's own hardcoded logRecordCreated switch (record-created Activity logging on
// mch_document/mch_task/mch_project -- workflow-behavior-decomposition-criteria.md's own B1-B5
// worked example, a second/third real case proven three times over before this was built, never
// assumed ahead of it). The schedule shape closes this comment's own long-standing note that it
// was "the one deferred shape, waiting on their own second real case": the SLA-breach reminder
// (ROADMAP.md, Flow 2 canvas re-audit, 2026-09-27) is that second case, replacing
// internal/composition's own logSLABreaches -- a GET-triggered write that stood in for a
// scheduler this runtime had never built.
type Event struct {
	ID         string
	On         string
	WhenEquals string
	OnCreate   bool
	// Schedule is the third trigger shape's own configuration, nil for the other two.
	Schedule *Schedule
	Then     Service
}

// Schedule is a schedule-shaped Event's own configuration: a condition evaluated periodically
// against every record of the declaring Machine, independent of any write (internal/execution's
// RunScheduledEvents is the only caller, on a ticker -- see cmd/server/main.go).
type Schedule struct {
	// DateField is a Field on this Machine, type "date", whose value is compared against now.
	DateField string
	// When is closed vocabulary for the comparison (internal/behavior.MatchedScheduleEvents); only
	// "overdue" (DateField's value is before today, both truncated to midnight UTC, matching
	// experience.EvaluateSLA's own day-granularity convention) is needed today. A string rather
	// than a bool so a second value (e.g. "due_today") is additive later, the same posture
	// Rollup's own three-outcome shape already takes.
	When string
	// GuardField/GuardEquals optionally require another Field on the same record to already hold
	// a given value before the schedule condition counts -- generalizing On/WhenEquals's own
	// field-equality shape rather than inventing a second one, so "overdue AND still in_review" is
	// declarative rather than special-cased to one Machine.
	GuardField  string
	GuardEquals string
}

// ScheduleWhenOverdue is the only value Schedule.When accepts today.
const ScheduleWhenOverdue = "overdue"

// KnownScheduleWhens is the closed set of comparisons a Schedule.When may name, the same
// static-seam discipline registry.Services/KnownNotificationPreferenceKeys already establish.
var KnownScheduleWhens = map[string]bool{
	ScheduleWhenOverdue: true,
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
	// Notify is ServiceSendNotification's own configuration, nil for every other Service.
	Notify *Notify
	// Composite is ServiceCompositeSignedDocument's own configuration, nil for every other Service.
	Composite *Composite
}

// Composite tells ServiceCompositeSignedDocument which records to work on: declared on the *child*
// Machine whose change triggers it (an Approval Step's decision), naming the parent it composites onto
// -- the same shape and the same reasoning as Rollup below, which is why it reuses parent_field and
// target_field rather than inventing two names for one question (001 Principle #8).
//
// What it deliberately does not describe is *how*. The compositing itself is 178 lines of binary PDF
// manipulation (internal/action/composite.go, banner.go) and stays runtime-owned, exactly as 002
// intends ("physical strategies remain runtime-owned"); what became declarable is the **invocation** --
// which decision causes a signed document, and which three Fields it reads and writes. Before this,
// the one thing that made a signed PDF appear was a direct call from flow code, named by no metadata
// anywhere, which is 001 #3 inverted.
type Composite struct {
	// ParentField is the reference Field on this Machine pointing at the record being composited.
	ParentField string
	// SourceField is the file Field on the *parent* holding the document to composite onto -- always
	// the original, never a previous output, so every run recomposites from scratch.
	SourceField string
	// TargetField is the file Field on the parent where the result is stored.
	TargetField string
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
	// ServiceSendNotification writes an mch_notification record and, if the recipient's own
	// preference allows it, sends an email (internal/web's sendNotification, Flow 2 gap study
	// Tahap 6).
	ServiceSendNotification = "send_notification"
	// ServiceCompositeSignedDocument burns every approved step's signature and the approval status
	// banner onto the parent document's PDF and stores the result (development-history.md Stage C, 2026-09-28;
	// the audit's Gap B). See Composite for what is declared and what deliberately stays Go.
	ServiceCompositeSignedDocument = "composite_signed_document"
)

// FieldIDPattern is the shape a Field id must have, from 004-runtime-metadata.md's own naming rule.
//
// Exported from domain rather than kept in the loader because two planes now check it:
// internal/metadata (ten call sites, over Fields, Constraints and member-removal blocks) and
// internal/registry (four, inside the rollup and notify contracts). A regexp copied across a plane
// boundary is 001 #8 over a naming rule that belongs to the thing being named.
//
// It is the whole *regexp.Regexp rather than a bool helper so a caller can print `.String()` in its own
// error message, which is what keeps those messages byte-identical to the ones this move replaced.
//
// **Its two siblings stay in internal/metadata**, and the asymmetry is deliberate rather than an
// oversight: machineIDPattern and constraintIDPattern have no second plane asking about them, and moving
// them "for consistency" would be shape-before-need.
var FieldIDPattern = regexp.MustCompile(`^fld_[a-z][a-z0-9_]*$`)

// ViolatesOptions reports whether f constrains its values to a declared option list and value is not
// on it. A Field's own question, answered by the Field, because two planes now ask it:
// internal/metadata's validators (eight call sites) and internal/registry's per-Service ones. It was an
// unexported two-line helper in metadata until 2026-09-30; copying it across the plane boundary would
// have been 001 #8 over a predicate that is purely a property of the Field.
//
// An empty option list means the Field constrains nothing, so nothing can violate it.
func (f Field) ViolatesOptions(value string) bool {
	if len(f.Options) == 0 {
		return false
	}
	for _, o := range f.Options {
		if o == value {
			return false
		}
	}
	return true
}

// Notify is send_notification's own configuration. RecipientField is a Field on the record the
// Event fired on -- no cross-record resolution built, unlike Rollup's own ParentField indirection:
// all three real notification triggers today (Tahap 6, and the SLA-breach reminder above) read a
// Field on their own record ("assigned to me" reads mch_approval_step's own fld_assignee; "my
// document was decided"/"my document is overdue" both read mch_document's own fld_submitted_by).
// A case that needs a parent lookup is the trigger to add that indirection here, mirroring
// Rollup, not before.
type Notify struct {
	RecipientField string
	// PreferenceKey selects which of the recipient's own notify_* preferences (credentials table)
	// gates whether this ALSO sends an email -- the in-app mch_notification row is written
	// unconditionally either way. One of KnownNotificationPreferenceKeys.
	PreferenceKey string
}

// KnownNotificationPreferenceKeys is the closed set of preference keys a Notify.PreferenceKey may
// name, each corresponding to one boolean column on credentials.
var KnownNotificationPreferenceKeys = map[string]bool{
	"assigned":   true,
	"decided":    true,
	"sla_breach": true,
}

// Machine is the primary runtime realization unit for a business capability
// (006-runtime-model.md "Machine").
type Machine struct {
	ID   string
	Name string
	// ApplicationID is the Application that claims this Machine, resolved once at load time from
	// that Application's own `machines:` list (internal/metadata.stampApplicationIDs) rather than
	// declared here -- the claim already exists in exactly one place and retyping it on the
	// Machine would be a second source of truth (001 Principle #8).
	//
	// Empty for a Machine no Application claims: mch_user and mch_activity are shared, and
	// Workspace.ApplicationForMachine already returns false for them. It is load-bearing for
	// exactly one thing -- a role-bearing Permission names a role from *some* Application's
	// vocabulary (domain.Application.Roles), and this is how AllowsAction knows which, without
	// every caller threading a Workspace through. A Machine with no Application therefore cannot
	// carry a role-bearing Permission, which internal/metadata refuses at load rather than
	// letting it silently deny everyone.
	ApplicationID string
	// WorkflowEngine and WorkflowRole are this Machine's part in its Application's declared
	// workflow binding (Application.Workflow), resolved once at load time from that one
	// declaration (internal/metadata.stampWorkflowRoles) rather than declared here -- the same
	// derive-don't-retype reasoning as ApplicationID above.
	//
	// Both empty for a Machine whose Application binds no engine, or one it binds but gives no
	// role. They are what internal/action's IsDocument/IsStep ask, in place of the Application-id
	// and Machine-id literals those predicates used to match: the engine now wakes for whichever
	// Machines an Application *says* play its roles, under any names.
	WorkflowEngine string
	WorkflowRole   string
	Fields         []Field
	Constraints    []Constraint
	Events         []Event
	// MemberRemovalBlocks declare when a record naming a person blocks deactivating them out of
	// the Workspace (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27) -- see MemberRemovalBlock's
	// own doc comment for why this is not a second Constraint shape.
	MemberRemovalBlocks []MemberRemovalBlock
	Permissions         []Permission
	// Transitions are this Machine's own declared state model: which moves of a status Field
	// exist, and which Action performs each (ROADMAP.md Case 03 Fase 7). Empty means every status
	// Field on this Machine moves freely, the same opt-in posture Sequencing takes.
	Transitions []Transition
	// ActionEffects declare what a named Action writes onto the record beyond the submitted values --
	// see ActionEffect. Empty is the normal case: `edit` and `delete` write only what was submitted.
	ActionEffects []ActionEffect
	Datasets      []Dataset
	// Sequencing is set only by a Machine whose records are acted on in order; nil means every
	// record is always actionable.
	Sequencing *Sequencing
	// SignaturePlacement and SignatureStore declare which Fields hold a signature and where it sits
	// -- see their own doc comments. Nil means this Machine declares neither, which is every Machine
	// but the two an approval Application casts in its `step` and `signature` roles.
	SignaturePlacement *SignaturePlacement
	SignatureStore     *SignatureStore
	// FlowTemplate and FlowTemplateStep declare the Fields of a saved approval flow (CAP-V28) -- see
	// FlowTemplate's own doc comment for why these are declared rather than derived the way a live
	// step's are. Nil for every Machine but the two cast in the `flow_template` roles.
	FlowTemplate     *FlowTemplate
	FlowTemplateStep *FlowTemplateStep
	// SLAField is a date Field ID on this Machine; when set, that Field renders as an OVERDUE /
	// "N day(s) left" badge instead of a plain date (development-history.md Phase 13). Machine-level rather
	// than per-View because the record detail page renders it too, and a detail page selects no
	// View.
	SLAField string
	// Completion declares which Field/value means a record is finished -- see Completion's own doc
	// comment. Nil means this Machine declares no notion of "done", so nothing renders as complete.
	Completion *Completion
	// CardTags declares the tag chips a board card shows -- see CardTags' own doc comment.
	CardTags *CardTags
	// CardFields is the ordered list of this Machine's own Fields a card projects, each with its
	// semantic role (Projection, 007 §7.6). Machine-level for the same reason as SLAField, and
	// because internal/composition reads it for a bespoke card outside any View.
	CardFields []CardField
	// Views are this Machine's declared arrangements of its own records. Empty means one implicit
	// table, which is how every Machine behaved before views: existed.
	Views []View
	// AppendOnly declares that a record of this Machine is never changed or removed once written:
	// every update and delete route refuses, for everyone, including a Workspace admin.
	//
	// A Machine-level property rather than a Permission, because it is not a statement about who.
	// A Permission answers "which actor may"; there is no actor who may edit an audit trail, and
	// expressing that as a Permission would mean inventing a role nobody can hold -- a rule that
	// reads as a grant and denies everyone, which is exactly the shape internal/metadata refuses
	// elsewhere. It is `menata-runtime`'s CAP-R07 (record immutability) narrowed to the one case
	// that exists here; upstream carries the richer state-triggered form ("frozen once posted"),
	// which needs a case this repo does not have yet.
	//
	// mch_activity is the whole of it today: internal/web.logActivity and every declared
	// log_activity Event write it, and until 2026-09-21 any authenticated member could edit or
	// delete those rows through the generic CRUD screens -- an audit trail with no integrity
	// property at all, found by the authorization review rather than by a case.
	AppendOnly bool
}

// Completion names the one Field and the one option of it that mean "this record is finished"
// (007 §7.6 Projection's missing half: card_fields says which Field is the status, nothing said which
// status value is terminal, so internal/composition's P3 hardcoded `done` for Task alone).
//
// Declared, not derived from the Machine's `transitions:`: a Machine with a free status Field (Task
// declares none) has no edge to read, and the *last option* is a convention, not a fact -- a Machine
// may list `archived` after `done`. The same "derive when something already answers it, declare when
// nothing does" rule the Stage D/E2 blocks follow.
//
// Reopening a finished record writes Reopen: the Field's declared default when it has one, else its
// first option. That is derivable from the Field itself, so it is not a second key.
type Completion struct {
	Field string
	Done  string
}

// ReopenValue is what finishing's opposite writes: the completion Field's declared default when it has one,
// else its first option. "" when m declares no completion.
func (m *Machine) ReopenValue() string {
	if m.Completion == nil {
		return ""
	}
	f, ok := m.FieldByID(m.Completion.Field)
	if !ok {
		return ""
	}
	if s, ok := f.Default.(string); ok && s != "" {
		return s
	}
	if len(f.Options) > 0 {
		return f.Options[0]
	}
	return ""
}

// ViewByID returns the View with the given id, if m declares one -- the lookup a screen uses to
// honour ?view=, so an unknown id can be refused rather than silently rendering something else.
func (m *Machine) ViewByID(id string) (View, bool) {
	for _, v := range m.Views {
		if v.ID == id {
			return v, true
		}
	}
	return View{}, false
}

// DefaultView is the arrangement a screen renders when none is asked for: the first declared, or
// a plain table for a Machine that declares none.
func (m *Machine) DefaultView() View {
	if len(m.Views) == 0 {
		return View{Type: ViewTable}
	}
	return m.Views[0]
}

// BoardView returns the first declared board View that groups by a Field -- the arrangement a card's own
// "move" is defined against (the group Field is what a move writes, and a position is a place among the
// cards of one group). False for a Machine with no such View, which is a Machine whose records cannot be
// moved between lists at all.
func (m *Machine) BoardView() (View, bool) {
	for _, v := range m.Views {
		if v.EffectiveType() == ViewBoard && v.GroupBy != "" {
			return v, true
		}
	}
	return View{}, false
}

// StepperView returns the first declared View of type ViewStepper, if m has one -- what a child
// collection's own renderer (internal/rendering/detail.templ) checks to decide whether to compose
// the sequential stepper instead of the generic child-collection table. Never more than one is
// expected in practice (there is exactly one Sequencing per Machine to render), but this returns
// the first rather than erroring on a second, the same permissive posture ViewByID/DefaultView
// already take toward a Machine's own View list.
func (m *Machine) StepperView() (View, bool) {
	for _, v := range m.Views {
		if v.EffectiveType() == ViewStepper {
			return v, true
		}
	}
	return View{}, false
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
// CardFieldFor is the id of the Field this Machine's `card_fields:` assigns to role, or "" when it declares
// none. It lives here rather than in internal/composition so the Execution Plane can ask the same question
// without importing the plane that composes screens.
func (m *Machine) CardFieldFor(role CardFieldRole) string {
	for _, cf := range m.CardFields {
		if cf.Role == role {
			return cf.Field
		}
	}
	return ""
}

func (m *Machine) FieldByID(id string) (Field, bool) {
	for _, f := range m.Fields {
		if f.ID == id {
			return f, true
		}
	}
	return Field{}, false
}
