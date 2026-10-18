package metadata

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// machineDoc, fieldDoc, and constraintDoc are the YAML serialization of a Machine
// (004-runtime-metadata.md "Serialization Independence" -- YAML is one possible representation,
// not the model itself).
type machineDoc struct {
	ID          string          `yaml:"id"`
	Name        string          `yaml:"name"`
	Fields      []fieldDoc      `yaml:"fields"`
	Constraints []constraintDoc `yaml:"constraints"`
	// Ensure declares aggregate bounds over this Machine's own records (domain.AggregateBound).
	Ensure []ensureDoc `yaml:"ensure"`
	// FrozenWhen freezes a record while a related record holds a value (domain.FrozenWhen).
	FrozenWhen *frozenWhenDoc `yaml:"frozen_when"`
	// DeletableUnless: see domain.Machine.DeletableUnless.
	DeletableUnless []deleteGuardDoc `yaml:"deletable_unless"`
	Events          []eventDoc       `yaml:"events"`
	Permissions     []permissionDoc  `yaml:"permissions"`
	Transitions     []transitionDoc  `yaml:"transitions"`
	// Actions are what a named Action writes beyond the submitted values -- see domain.ActionEffect.
	Actions    []actionEffectDoc `yaml:"actions"`
	Datasets   []datasetDoc      `yaml:"datasets"`
	Sequencing *sequencingDoc    `yaml:"sequencing"`
	// SignaturePlacement/SignatureStore -- which Fields hold a signature and where it sits. See
	// domain.SignaturePlacement for why this is a declaration rather than a derivation.
	SignaturePlacement *signaturePlacementDoc `yaml:"signature_placement"`
	SignatureStore     *signatureStoreDoc     `yaml:"signature_store"`
	// FlowTemplate/FlowTemplateStep -- the Fields of a saved approval flow (CAP-V28).
	FlowTemplate     *flowTemplateDoc     `yaml:"flow_template"`
	FlowTemplateStep *flowTemplateStepDoc `yaml:"flow_template_step"`
	// MemberRemovalBlocks -- see domain.MemberRemovalBlock's own doc comment for why this is not a
	// second Constraint shape.
	MemberRemovalBlocks []memberRemovalBlockDoc `yaml:"blocks_member_removal"`

	// Machine-level display: how these records look wherever they appear, read on screens that
	// select no View at all (the detail page, composition's bespoke cards).
	SLAField string `yaml:"sla_field"`
	// Completion -- see domain.Completion.
	Completion *completionDoc `yaml:"completion"`
	// StatusCategories -- see domain.StatusCategories.
	StatusCategories *statusCategoriesDoc `yaml:"status_categories"`
	// CardTags -- see domain.CardTags.
	CardTags   *cardTagsDoc   `yaml:"card_tags"`
	CardFields []cardFieldDoc `yaml:"card_fields"`
	// Views are the declared arrangements of those records.
	Views []viewDoc `yaml:"views"`
	// AppendOnly -- see domain.Machine.AppendOnly.
	AppendOnly bool `yaml:"append_only"`
	// Runtime -- see domain.Machine.Runtime.
	Runtime string `yaml:"runtime"`
}

// cardTagsDoc is the YAML serialization of a domain.CardTags.
type cardTagsDoc struct {
	Machine string `yaml:"machine"`
	Via     string `yaml:"via"`
	Tag     string `yaml:"tag"`
}

// completionDoc is the YAML serialization of a domain.Completion.
type completionDoc struct {
	Field string `yaml:"field"`
	Done  string `yaml:"done"`
}

// statusCategoriesDoc is the YAML serialization of a domain.StatusCategories.
type statusCategoriesDoc struct {
	Field  string            `yaml:"field"`
	Values map[string]string `yaml:"values"`
}

// transitionDoc is the YAML serialization of a domain.Transition (Case 03 Fase 7).
//
// `action` is omitted for an edge the runtime performs on its own -- see domain.Transition.Action
// for why such an edge is still written down rather than left out.
type transitionDoc struct {
	ID     string `yaml:"id"`
	Name   string `yaml:"name"`
	Field  string `yaml:"field"`
	From   string `yaml:"from"`
	To     string `yaml:"to"`
	Action string `yaml:"action"`
	// Roles and ActorField gate who may take the edge (domain.Transition.Roles/ActorField).
	Roles      []string `yaml:"roles"`
	ActorField string   `yaml:"actor_field"`
	// Sets writes Fields from the clock when the edge is taken (domain.Transition.Sets).
	Sets map[string]string `yaml:"sets"`
	// Generates creates a schedule of records when the edge is taken (domain.GenerateSchedule).
	Generates *generatesDoc `yaml:"generates"`
	// Balanced holds the edge until the lines pointing at the record add up (domain.BalanceRule).
	Balanced *balancedDoc `yaml:"balanced"`
}

type balancedDoc struct {
	Of     string `yaml:"of"`
	Via    string `yaml:"via"`
	Debit  string `yaml:"debit"`
	Credit string `yaml:"credit"`
	Reason string `yaml:"reason"`
}

type generatesDoc struct {
	Machine      string `yaml:"machine"`
	CountField   string `yaml:"count_field"`
	LinkField    string `yaml:"link_field"`
	DueField     string `yaml:"due_field"`
	DueEveryDays int    `yaml:"due_every_days"`
	AmountField  string `yaml:"amount_field"`
	AmountFrom   string `yaml:"amount_from"`
}

// actionEffectDoc/fieldWriteDoc are the YAML serialization of a domain.ActionEffect (Stage B,
// 2026-09-28).
type actionEffectDoc struct {
	Action string          `yaml:"action"`
	Writes []fieldWriteDoc `yaml:"writes"`
}

type fieldWriteDoc struct {
	Field string `yaml:"field"`
	From  string `yaml:"from"`
	Value string `yaml:"value"`
}

// sequencingDoc is the YAML serialization of a domain.Sequencing.
type sequencingDoc struct {
	ParentField     string `yaml:"parent_field"`
	ModeField       string `yaml:"mode_field"`
	SequentialValue string `yaml:"sequential_value"`
	OrderField      string `yaml:"order_field"`
	StateField      string `yaml:"state_field"`
	OpenValue       string `yaml:"open_value"`
}

// signaturePlacementDoc and signatureStoreDoc are the YAML serialization of a
// domain.SignaturePlacement and domain.SignatureStore (Stage D, 2026-09-28).
type signaturePlacementDoc struct {
	ImageField string `yaml:"image_field"`
	PageField  string `yaml:"page_field"`
	XField     string `yaml:"x_field"`
	YField     string `yaml:"y_field"`
	WidthField string `yaml:"width_field"`
}

type signatureStoreDoc struct {
	OwnerField string `yaml:"owner_field"`
	ImageField string `yaml:"image_field"`
}

// flowTemplateDoc and flowTemplateStepDoc are the YAML serialization of a domain.FlowTemplate and
// domain.FlowTemplateStep (Stage E2, 2026-09-29).
type flowTemplateDoc struct {
	KeyField  string `yaml:"key_field"`
	ModeField string `yaml:"mode_field"`
}

type flowTemplateStepDoc struct {
	TemplateField   string `yaml:"template_field"`
	OrderField      string `yaml:"order_field"`
	ActorField      string `yaml:"actor_field"`
	ActorTypeField  string `yaml:"actor_type_field"`
	ActorGroupField string `yaml:"actor_group_field"`
}

// datasetDoc is the YAML serialization of a domain.Dataset (007 §7.2-§7.4). measures[].where
// reuses comparisonDoc, the same shape constraintDoc's block_if.condition already declares, so a
// filter reads identically wherever it appears in metadata.
type datasetDoc struct {
	ID        string       `yaml:"id"`
	Dimension string       `yaml:"dimension"`
	Measures  []measureDoc `yaml:"measures"`
	// Select/Sort/Limit are the record-selection half (007 §7.7-§7.9), added 2026-09-29. A Dataset
	// declares either these or dimension/measures, never both -- validateDataset refuses the mix.
	Select string `yaml:"select"`
	// ViewerScoped is domain.Dataset.ViewerScoped: this Dataset lists the viewer's own records.
	ViewerScoped bool          `yaml:"viewer_scoped"`
	Relations    []relationDoc `yaml:"relations"`
	Where        *predicateDoc `yaml:"where"`
	Sort         []sortDoc     `yaml:"sort"`
	Limit        int           `yaml:"limit"`
}

// relationDoc is the YAML serialization of a domain.Relation (007 §7.5).
type relationDoc struct {
	ID      string        `yaml:"id"`
	Machine string        `yaml:"machine"`
	Via     string        `yaml:"via"`
	Where   *predicateDoc `yaml:"where"`
}

// predicateDoc is the YAML serialization of an expression.Predicate. It accepts both shapes on
// purpose: a bare field/op/value, which is what every filter in this repo already writes and what
// 007 §7.7 keeps valid as syntax sugar, or an `all:` list when more than one must hold, with an `any:` list beside it when one of several must.
type predicateDoc struct {
	filterDoc `yaml:",inline"`
	All       []filterDoc `yaml:"all"`
	Any       []filterDoc `yaml:"any"`
}

// filterDoc is one comparison of a `where:`, which -- unlike a constraint's condition -- may be `optional: true`:
// expression.Comparison.Optional, valid only on a records Dataset comparing against a `$parameters.<name>`.
type filterDoc struct {
	comparisonDoc `yaml:",inline"`
	Optional      bool   `yaml:"optional"`
	IfParam       string `yaml:"if_param"`
}

// predicate lowers the doc into the one representation, so a single comparison and a one-element
// `all:` are indistinguishable downstream -- the "compile to the common expression representation"
// half of §7.7.
func (d *predicateDoc) predicate() *expression.Predicate {
	if d == nil {
		return nil
	}
	lower := func(docs []filterDoc) []expression.Comparison {
		var out []expression.Comparison
		for _, c := range docs {
			out = append(out, expression.Comparison{Field: c.Field, Op: expression.Op(c.Op), Value: c.Value, Optional: c.Optional, IfParam: c.IfParam})
		}
		return out
	}
	out := &expression.Predicate{All: lower(d.All), Any: lower(d.Any)}
	if len(out.All) == 0 && (d.Field != "" || d.Op != "" || d.Value != "" || d.Optional || d.IfParam != "") {
		out.All = lower([]filterDoc{d.filterDoc})
	}
	if len(out.All) == 0 && len(out.Any) == 0 {
		return nil
	}
	return out
}

// sortDoc is the YAML serialization of a domain.SortKey. `direction` rather than a bool, because
// `direction: desc` is what an author writes and `descending: true` is what a struct holds.
type sortDoc struct {
	Field     string `yaml:"field"`
	Direction string `yaml:"direction"`
}

type measureDoc struct {
	ID          string        `yaml:"id"`
	Aggregate   string        `yaml:"aggregate"`
	Field       string        `yaml:"field"`
	Where       *predicateDoc `yaml:"where"`
	Numerator   string        `yaml:"numerator"`
	Denominator string        `yaml:"denominator"`
}

// comparisonDoc is the YAML serialization of an expression.Comparison.
type comparisonDoc struct {
	Field string `yaml:"field"`
	Op    string `yaml:"op"`
	Value string `yaml:"value"`
}

// memberRemovalBlockDoc is the YAML serialization of a domain.MemberRemovalBlock. Condition
// reuses comparisonDoc, the same shape constraintDoc's own block_if.condition and datasetDoc's
// own measures[].where already declare, rather than a fourth copy of the identical three fields.
type memberRemovalBlockDoc struct {
	ID         string         `yaml:"id"`
	ActorField string         `yaml:"actor_field"`
	Condition  *comparisonDoc `yaml:"condition"`
	Reason     string         `yaml:"reason"`
}

type viewDoc struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	GroupBy string `yaml:"group_by"`
}

// cardFieldDoc is the YAML serialization of a domain.CardField (007 §7.6 Projection pilot).
type cardFieldDoc struct {
	Field string `yaml:"field"`
	Role  string `yaml:"role"`
}

type fieldDoc struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Required bool     `yaml:"required"`
	Options  []string `yaml:"options"`
	// Machine is the target Machine ID, meaningful only when type is "relation".
	Machine string `yaml:"machine"`
	// Default is always written as a YAML string, even for a number or boolean Field (quote it:
	// `default: "5"`, `default: "true"`) -- Parse coerces it to this Field's real storage type,
	// the same convention constraintDoc's own Condition.Value already uses. Empty means no
	// default was declared; there is no way to declare an empty-string default deliberately.
	Default string `yaml:"default"`
	// Compute declares a computed Field -- see domain.FieldCompute.
	Compute *computeDoc `yaml:"compute"`
	// Stamp declares a stamped Field -- see domain.FieldStamp.
	Stamp string `yaml:"stamp"`
	// Numbering declares a number the runtime assigns -- see domain.FieldNumbering.
	Numbering *numberingDoc `yaml:"numbering"`
	// RequiredWhen and DefaultFrom: see domain.Field.
	RequiredWhen *requiredWhenDoc `yaml:"required_when"`
	DefaultFrom  *defaultFromDoc  `yaml:"default_from"`
	// LockedWhen, SetOnce and OnParentDelete: see domain.Field.
	// Currency: see domain.Field.Currency.
	Currency string `yaml:"currency"`
	// Categories: see domain.Field.Categories.
	Categories     map[string]string `yaml:"categories"`
	LockedWhen     *requiredWhenDoc  `yaml:"locked_when"`
	SetOnce        bool              `yaml:"set_once"`
	OnParentDelete string            `yaml:"on_parent_delete"`
}

// deleteGuardDoc is one `deletable_unless:` entry: see domain.DeleteGuard.
type deleteGuardDoc struct {
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
	Reason string `yaml:"reason"`
}

type requiredWhenDoc struct {
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
}

type defaultFromDoc struct {
	Field string         `yaml:"field"`
	Days  map[string]int `yaml:"days"`
}

type computeDoc struct {
	Op     string   `yaml:"op"`
	Fields []string `yaml:"fields"`
	// expr is the tree shape (`{subtract: [fld_gross, {multiply: [fld_gross, 0.02]}]}`), read by UnmarshalYAML; nil for
	// the flat `{op, fields}`.
	expr *domain.ComputeExpr
}

// UnmarshalYAML reads either shape of `compute:`: the flat `{op: subtract, fields: [...]}`, or one operation named as
// the key with its operands as the value. An operand is a Field id, a number, `{parent: fld_via.fld_x}`, or another
// operation; `sum_children`/`count_children` take `{machine, via, field, where: {field, equals}}`.
func (c *computeDoc) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: compute: is a mapping -- {op, fields} or one operation such as {subtract: [fld_a, fld_b]}", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i].Value; k == "op" || k == "fields" {
			type flat struct {
				Op     string   `yaml:"op"`
				Fields []string `yaml:"fields"`
			}
			var f flat
			if err := n.Decode(&f); err != nil {
				return err
			}
			c.Op, c.Fields = f.Op, f.Fields
			return nil
		}
	}
	e, err := parseComputeExpr(n)
	if err != nil {
		return err
	}
	c.Op, c.expr = string(e.Op), &e
	return nil
}

// parseComputeExpr reads one node of a computed value's tree.
func parseComputeExpr(n *yaml.Node) (domain.ComputeExpr, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		if v, err := strconv.ParseFloat(n.Value, 64); err == nil {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return domain.ComputeExpr{}, fmt.Errorf("line %d: compute: %q is not a number a value can be computed with", n.Line, n.Value)
			}
			return domain.ComputeExpr{Number: &v}, nil
		}
		if n.Value == "" {
			return domain.ComputeExpr{}, fmt.Errorf("line %d: compute: an empty operand", n.Line)
		}
		return domain.ComputeExpr{Field: n.Value}, nil
	case yaml.MappingNode:
		if len(n.Content) != 2 {
			return domain.ComputeExpr{}, fmt.Errorf("line %d: compute: an operation is one key -- its name -- and its operands", n.Line)
		}
		key, val := n.Content[0].Value, n.Content[1]
		switch op := domain.ComputeOp(key); {
		case key == "parent":
			via, field, ok := strings.Cut(val.Value, ".")
			if val.Kind != yaml.ScalarNode || !ok || via == "" || field == "" {
				return domain.ComputeExpr{}, fmt.Errorf("line %d: compute: parent: names the relation and the related record's Field, such as fld_contract.fld_value", val.Line)
			}
			return domain.ComputeExpr{Parent: &domain.ParentRef{Via: via, Field: field}}, nil
		case op == domain.ComputeSumChildren || op == domain.ComputeCountChildren:
			var doc struct {
				Machine string           `yaml:"machine"`
				Via     string           `yaml:"via"`
				Field   string           `yaml:"field"`
				Where   *requiredWhenDoc `yaml:"where"`
			}
			if err := val.Decode(&doc); err != nil {
				return domain.ComputeExpr{}, err
			}
			agg := &domain.ChildAggregate{Machine: doc.Machine, Via: doc.Via, Field: doc.Field}
			if doc.Where != nil {
				agg.Where = &domain.FieldCondition{Field: doc.Where.Field, Equals: doc.Where.Equals}
			}
			return domain.ComputeExpr{Op: op, Children: agg}, nil
		case op == domain.ComputeJoin:
			var doc struct {
				With string      `yaml:"with"`
				Of   []yaml.Node `yaml:"of"`
			}
			if val.Kind != yaml.MappingNode {
				return domain.ComputeExpr{}, fmt.Errorf("line %d: compute: join is {with: \" -- \", of: [fld_a, fld_b]}", val.Line)
			}
			if err := val.Decode(&doc); err != nil {
				return domain.ComputeExpr{}, err
			}
			e := domain.ComputeExpr{Op: op, Sep: doc.With}
			for i := range doc.Of {
				arg, err := parseComputeExpr(&doc.Of[i])
				if err != nil {
					return domain.ComputeExpr{}, err
				}
				e.Args = append(e.Args, arg)
			}
			return e, nil
		case domain.KnownComputeOps[op]:
			if val.Kind != yaml.SequenceNode {
				return domain.ComputeExpr{}, fmt.Errorf("line %d: compute: %s takes a list of operands", val.Line, key)
			}
			e := domain.ComputeExpr{Op: op}
			for _, a := range val.Content {
				arg, err := parseComputeExpr(a)
				if err != nil {
					return domain.ComputeExpr{}, err
				}
				e.Args = append(e.Args, arg)
			}
			return e, nil
		}
		return domain.ComputeExpr{}, fmt.Errorf("line %d: compute: %q is not an operation (%v) or parent:", n.Line, key, sortedComputeOps())
	}
	return domain.ComputeExpr{}, fmt.Errorf("line %d: compute: an operand is a Field id, a number, parent: or an operation", n.Line)
}

type frozenWhenDoc struct {
	Via    string `yaml:"via"`
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
}

type ensureDoc struct {
	ID      string    `yaml:"id"`
	SumOf   string    `yaml:"sum_of"`
	Per     string    `yaml:"per"`
	AtLeast *float64  `yaml:"at_least"`
	AtMost  *boundDoc `yaml:"at_most"`
	Reason  string    `yaml:"reason"`
}

// boundDoc is an `at_most:`: a number, or `{parent: fld_contract.fld_value}` -- a Field of the record the bound's `per:`
// relation points at (plan A1: the instalments of a contract may not add up past its value).
type boundDoc struct {
	value  *float64
	parent *domain.ParentRef
}

func (b *boundDoc) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		v, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return fmt.Errorf("line %d: at_most: is a number or {parent: fld_via.fld_x}", n.Line)
		}
		b.value = &v
		return nil
	}
	var doc struct {
		Parent string `yaml:"parent"`
	}
	if err := n.Decode(&doc); err != nil {
		return err
	}
	via, field, ok := strings.Cut(doc.Parent, ".")
	if !ok || via == "" || field == "" {
		return fmt.Errorf("line %d: at_most: parent: names the relation and the related record's Field, such as fld_contract.fld_value", n.Line)
	}
	b.parent = &domain.ParentRef{Via: via, Field: field}
	return nil
}

type numberingDoc struct {
	Pattern string            `yaml:"pattern"`
	Codes   map[string]string `yaml:"codes"`
}

type constraintDoc struct {
	ID         string `yaml:"id"`
	On         string `yaml:"on"`
	WhenEquals string `yaml:"when_equals"`
	Reason     string `yaml:"reason"`
	BlockIf    struct {
		RelatedMachine string `yaml:"related_machine"`
		RelatedField   string `yaml:"related_field"`
		Condition      struct {
			Field string `yaml:"field"`
			Op    string `yaml:"op"`
			Value string `yaml:"value"`
		} `yaml:"condition"`
	} `yaml:"block_if"`
}

// eventDoc is the YAML serialization of an Event, mirroring constraintDoc's own shape.
type eventDoc struct {
	ID         string `yaml:"id"`
	On         string `yaml:"on"`
	WhenEquals string `yaml:"when_equals"`
	OnCreate   bool   `yaml:"on_create"`
	// Schedule is the third trigger shape's own block (domain.Schedule) -- mutually exclusive with
	// On/OnCreate above, enforced by validateEvent.
	Schedule *struct {
		DateField   string `yaml:"date_field"`
		When        string `yaml:"when"`
		GuardField  string `yaml:"guard_field"`
		GuardEquals string `yaml:"guard_equals"`
		// GuardNotEquals: the guard's other polarity (domain.Schedule.GuardNotEquals).
		GuardNotEquals string `yaml:"guard_not_equals"`
	} `yaml:"schedule"`
	Then struct {
		Service             string `yaml:"service"`
		Summary             string `yaml:"summary"`
		SummaryOverrideWhen string `yaml:"summary_override_when"`
		SummaryOverride     string `yaml:"summary_override"`

		// rollup_parent_status's own keys (domain.Rollup).
		ParentField string         `yaml:"parent_field"`
		TargetField string         `yaml:"target_field"`
		Any         *rollupRuleDoc `yaml:"any"`
		All         *rollupRuleDoc `yaml:"all"`
		Default     string         `yaml:"default"`

		// send_notification's own keys (domain.Notify).
		RecipientField      string `yaml:"recipient_field"`
		RecipientRole       string `yaml:"recipient_role"`
		RecipientGroupField string `yaml:"recipient_group_field"`
		PreferenceKey       string `yaml:"preference_key"`

		// composite_signed_document's own one new key (domain.Composite) -- parent_field and
		// target_field above are shared with a rollup, same question, same answer.
		SourceField string `yaml:"source_field"`

		// set_field's own key (domain.SetField); target_field above is shared.
		Value string `yaml:"value"`
	} `yaml:"then"`
}

// rollupRuleDoc is one arm of a rollup: the child value to look for, and what the parent becomes
// when it is found.
type rollupRuleDoc struct {
	Value string `yaml:"value"`
	Set   string `yaml:"set"`
}

// permissionDoc is the YAML serialization of a Permission (development-history.md Phase 16).
//
// The three actor_*_field keys are the dynamic actor gate (CAP-F24, Fase 6c-1). They are flat
// siblings of actor_field rather than a nested block, matching how the struct they build models
// them -- see domain.DynamicActorGate. All three together or none: a partially declared gate is
// rejected by validatePermission rather than silently half-applied.
type permissionDoc struct {
	ID         string `yaml:"id"`
	Action     string `yaml:"action"`
	ActorField string `yaml:"actor_field"`
	// WorkspaceRole is the Workspace-level arm (admin), a separate key from roles: because the
	// two name different namespaces -- see domain.Permission.WorkspaceRole.
	WorkspaceRole string `yaml:"workspace_role"`
	// Roles is CAP-P01's own arm (Case 03 Fase 7): role words from the vocabulary declared by the
	// Application that claims this Machine, any one of which satisfies it.
	Roles           []string `yaml:"roles"`
	ActorTypeField  string   `yaml:"actor_type_field"`
	ActorUserField  string   `yaml:"actor_user_field"`
	ActorGroupField string   `yaml:"actor_group_field"`
	// ParentActor is domain.Permission.ParentActor: `parent_actor: {via: <relation field>, field: <person field
	// on the related Machine>}`.
	ParentActor *parentActorDoc `yaml:"parent_actor"`
}

type parentActorDoc struct {
	Via   string `yaml:"via"`
	Field string `yaml:"field"`
}

// Parse decodes Runtime Metadata YAML describing a single Machine. It performs structural
// (schema) parsing only -- semantic validation happens in Validate.
func Parse(data []byte) (*domain.Machine, error) {
	var doc machineDoc
	if err := decodeStrict(data, &doc, "a machine file"); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	m := &domain.Machine{
		ID:   doc.ID,
		Name: doc.Name,
	}
	for _, fd := range doc.Fields {
		fieldType := domain.FieldType(fd.Type)
		// The person -> mch_user binding is not applied here any more: it is an *inference*, and
		// Normalize (normalize.go) is where inferences live so that a Machine built any other way gets
		// them too. 005's Phase 4 is the reasoning; a hand-built fixture failing validation over a
		// correct `person` Field is what found it.
		relatedMachine := fd.Machine
		var def any
		if fd.Default != "" {
			var err error
			def, err = coerceDefault(fieldType, fd.Default)
			if err != nil {
				return nil, fmt.Errorf("field %s: default %q: %w", fd.ID, fd.Default, err)
			}
		}
		var compute *domain.FieldCompute
		if fd.Compute != nil {
			compute = &domain.FieldCompute{Op: domain.ComputeOp(fd.Compute.Op), Fields: fd.Compute.Fields, Expr: fd.Compute.expr}
		}
		var requiredWhen *domain.FieldCondition
		if fd.RequiredWhen != nil {
			requiredWhen = &domain.FieldCondition{Field: fd.RequiredWhen.Field, Equals: fd.RequiredWhen.Equals}
		}
		var defaultFrom *domain.FieldDefaultFrom
		if fd.DefaultFrom != nil {
			defaultFrom = &domain.FieldDefaultFrom{Field: fd.DefaultFrom.Field, Days: fd.DefaultFrom.Days}
		}
		var lockedWhen *domain.FieldCondition
		if fd.LockedWhen != nil {
			lockedWhen = &domain.FieldCondition{Field: fd.LockedWhen.Field, Equals: fd.LockedWhen.Equals}
		}
		var categories map[string]domain.StatusCategory
		for option, c := range fd.Categories {
			if categories == nil {
				categories = map[string]domain.StatusCategory{}
			}
			categories[option] = domain.StatusCategory(c)
		}
		m.Fields = append(m.Fields, domain.Field{
			Currency:       fd.Currency,
			Categories:     categories,
			LockedWhen:     lockedWhen,
			SetOnce:        fd.SetOnce,
			OnParentDelete: domain.ParentDelete(fd.OnParentDelete),
			RequiredWhen:   requiredWhen,
			DefaultFrom:    defaultFrom,
			ID:             fd.ID,
			Name:           fd.Name,
			Type:           fieldType,
			Required:       fd.Required,
			Options:        fd.Options,
			RelatedMachine: relatedMachine,
			Default:        def,
			Compute:        compute,
			Stamp:          domain.FieldStamp(fd.Stamp),
			Numbering:      numberingOf(fd.Numbering),
		})
	}
	for _, cd := range doc.Constraints {
		m.Constraints = append(m.Constraints, domain.Constraint{
			ID:         cd.ID,
			On:         cd.On,
			WhenEquals: cd.WhenEquals,
			Reason:     strings.TrimSpace(cd.Reason),
			BlockIf: domain.RelationBlock{
				RelatedMachine: cd.BlockIf.RelatedMachine,
				RelatedField:   cd.BlockIf.RelatedField,
				Condition: expression.Comparison{
					Field: cd.BlockIf.Condition.Field,
					Op:    expression.Op(cd.BlockIf.Condition.Op),
					Value: cd.BlockIf.Condition.Value,
				},
			},
		})
	}
	if doc.FrozenWhen != nil {
		m.Frozen = &domain.FrozenWhen{Via: doc.FrozenWhen.Via, Field: doc.FrozenWhen.Field, Equals: doc.FrozenWhen.Equals}
	}
	for _, g := range doc.DeletableUnless {
		m.DeletableUnless = append(m.DeletableUnless, domain.DeleteGuard{Field: g.Field, Equals: g.Equals, Reason: g.Reason})
	}
	for _, ed := range doc.Ensure {
		bound := domain.AggregateBound{ID: ed.ID, SumOf: ed.SumOf, Per: ed.Per, AtLeast: ed.AtLeast, Reason: ed.Reason}
		if ed.AtMost != nil {
			bound.AtMost, bound.AtMostParent = ed.AtMost.value, ed.AtMost.parent
		}
		m.Bounds = append(m.Bounds, bound)
	}
	for _, bd := range doc.MemberRemovalBlocks {
		block := domain.MemberRemovalBlock{ID: bd.ID, ActorField: bd.ActorField, Reason: bd.Reason}
		if bd.Condition != nil {
			block.Condition = expression.Comparison{
				Field: bd.Condition.Field,
				Op:    expression.Op(bd.Condition.Op),
				Value: bd.Condition.Value,
			}
		}
		m.MemberRemovalBlocks = append(m.MemberRemovalBlocks, block)
	}
	for _, ed := range doc.Events {
		then := domain.Service{
			Name:                ed.Then.Service,
			Summary:             ed.Then.Summary,
			SummaryOverrideWhen: ed.Then.SummaryOverrideWhen,
			SummaryOverride:     ed.Then.SummaryOverride,
		}
		if ed.Then.Service == domain.ServiceRollupParentStatus {
			r := domain.Rollup{
				ParentField: ed.Then.ParentField,
				TargetField: ed.Then.TargetField,
				Default:     ed.Then.Default,
			}
			if ed.Then.Any != nil {
				r.AnyValue, r.AnySet = ed.Then.Any.Value, ed.Then.Any.Set
			}
			if ed.Then.All != nil {
				r.AllValue, r.AllSet = ed.Then.All.Value, ed.Then.All.Set
			}
			then.Rollup = &r
		}
		if ed.Then.Service == domain.ServiceSendNotification {
			then.Notify = &domain.Notify{
				RecipientField:      ed.Then.RecipientField,
				RecipientRole:       ed.Then.RecipientRole,
				RecipientGroupField: ed.Then.RecipientGroupField,
				PreferenceKey:       ed.Then.PreferenceKey,
			}
		}
		if ed.Then.Service == domain.ServiceSetField {
			then.Set = &domain.SetField{TargetField: ed.Then.TargetField, Value: ed.Then.Value}
		}
		if ed.Then.Service == domain.ServiceCompositeSignedDocument {
			then.Composite = &domain.Composite{
				ParentField: ed.Then.ParentField,
				SourceField: ed.Then.SourceField,
				TargetField: ed.Then.TargetField,
			}
		}
		var schedule *domain.Schedule
		if ed.Schedule != nil {
			schedule = &domain.Schedule{
				DateField:   ed.Schedule.DateField,
				When:        ed.Schedule.When,
				GuardField:  ed.Schedule.GuardField,
				GuardEquals: ed.Schedule.GuardEquals,

				GuardNotEquals: ed.Schedule.GuardNotEquals,
			}
		}
		m.Events = append(m.Events, domain.Event{
			ID:         ed.ID,
			On:         ed.On,
			WhenEquals: ed.WhenEquals,
			OnCreate:   ed.OnCreate,
			Schedule:   schedule,
			Then:       then,
		})
	}
	for _, pd := range doc.Permissions {
		perm := domain.Permission{
			ID:            pd.ID,
			Action:        pd.Action,
			ActorField:    pd.ActorField,
			Roles:         pd.Roles,
			WorkspaceRole: pd.WorkspaceRole,
		}
		// Any one of the three keys builds the gate, so a partial declaration reaches
		// validatePermission as a real gate with an empty field name and is reported -- rather
		// than being dropped here and reading as though no gate were ever written.
		if pd.ActorTypeField != "" || pd.ActorUserField != "" || pd.ActorGroupField != "" {
			perm.DynamicActor = &domain.DynamicActorGate{
				ActorTypeField:  pd.ActorTypeField,
				ActorUserField:  pd.ActorUserField,
				ActorGroupField: pd.ActorGroupField,
			}
		}
		if pd.ParentActor != nil {
			perm.ParentActor = &domain.ParentActorGate{ViaField: pd.ParentActor.Via, ActorField: pd.ParentActor.Field}
		}
		m.Permissions = append(m.Permissions, perm)
	}
	for _, td := range doc.Transitions {
		m.Transitions = append(m.Transitions, domain.Transition{
			ID:     td.ID,
			Name:   td.Name,
			Field:  td.Field,
			From:   td.From,
			To:     td.To,
			Action: td.Action,

			Roles:      td.Roles,
			ActorField: td.ActorField,
			Sets:       td.Sets,
			Generates:  generatesOf(td.Generates),
			Balanced:   balancedOf(td.Balanced),
		})
	}
	for _, ad := range doc.Actions {
		effect := domain.ActionEffect{Action: ad.Action}
		for _, wd := range ad.Writes {
			effect.Writes = append(effect.Writes, domain.FieldWrite{Field: wd.Field, From: wd.From, Value: wd.Value})
		}
		m.ActionEffects = append(m.ActionEffects, effect)
	}
	for _, dd := range doc.Datasets {
		ds := domain.Dataset{
			ID: dd.ID, Source: doc.ID, Dimension: dd.Dimension,
			Select: dd.Select, ViewerScoped: dd.ViewerScoped, Limit: dd.Limit, Where: dd.Where.predicate(),
		}
		for _, rd := range dd.Relations {
			ds.Relations = append(ds.Relations, domain.Relation{ID: rd.ID, Machine: rd.Machine, Via: rd.Via, Where: rd.Where.predicate()})
		}
		for _, sd := range dd.Sort {
			// The declared direction is carried through unreduced so validateDataset can reject an
			// unknown one; collapsing it here would make `direction: descending` sort ascending in
			// silence.
			ds.Sort = append(ds.Sort, domain.SortKey{Field: sd.Field, Direction: sd.Direction})
		}
		for _, md := range dd.Measures {
			ms := domain.Measure{
				ID:          md.ID,
				Aggregate:   domain.AggregateKind(md.Aggregate),
				Field:       md.Field,
				Numerator:   md.Numerator,
				Denominator: md.Denominator,
			}
			ms.Where = md.Where.predicate()
			ds.Measures = append(ds.Measures, ms)
		}
		m.Datasets = append(m.Datasets, ds)
	}

	if doc.Sequencing != nil {
		m.Sequencing = &domain.Sequencing{
			ParentField:     doc.Sequencing.ParentField,
			ModeField:       doc.Sequencing.ModeField,
			SequentialValue: doc.Sequencing.SequentialValue,
			OrderField:      doc.Sequencing.OrderField,
			StateField:      doc.Sequencing.StateField,
			OpenValue:       doc.Sequencing.OpenValue,
		}
	}

	if doc.SignaturePlacement != nil {
		m.SignaturePlacement = &domain.SignaturePlacement{
			ImageField: doc.SignaturePlacement.ImageField,
			PageField:  doc.SignaturePlacement.PageField,
			XField:     doc.SignaturePlacement.XField,
			YField:     doc.SignaturePlacement.YField,
			WidthField: doc.SignaturePlacement.WidthField,
		}
	}
	if doc.SignatureStore != nil {
		m.SignatureStore = &domain.SignatureStore{
			OwnerField: doc.SignatureStore.OwnerField,
			ImageField: doc.SignatureStore.ImageField,
		}
	}
	if doc.FlowTemplate != nil {
		m.FlowTemplate = &domain.FlowTemplate{
			KeyField:  doc.FlowTemplate.KeyField,
			ModeField: doc.FlowTemplate.ModeField,
		}
	}
	if doc.FlowTemplateStep != nil {
		m.FlowTemplateStep = &domain.FlowTemplateStep{
			TemplateField:   doc.FlowTemplateStep.TemplateField,
			OrderField:      doc.FlowTemplateStep.OrderField,
			ActorField:      doc.FlowTemplateStep.ActorField,
			ActorTypeField:  doc.FlowTemplateStep.ActorTypeField,
			ActorGroupField: doc.FlowTemplateStep.ActorGroupField,
		}
	}

	m.SLAField = doc.SLAField
	if doc.CardTags != nil {
		m.CardTags = &domain.CardTags{Machine: doc.CardTags.Machine, Via: doc.CardTags.Via, Tag: doc.CardTags.Tag}
	}
	if doc.Completion != nil {
		m.Completion = &domain.Completion{Field: doc.Completion.Field, Done: doc.Completion.Done}
	}
	if doc.StatusCategories != nil {
		sc := &domain.StatusCategories{Field: doc.StatusCategories.Field, Values: map[string]domain.StatusCategory{}}
		for option, category := range doc.StatusCategories.Values {
			sc.Values[option] = domain.StatusCategory(category)
		}
		m.StatusCategories = sc
	}
	m.AppendOnly = doc.AppendOnly
	m.Runtime = doc.Runtime
	for _, cf := range doc.CardFields {
		m.CardFields = append(m.CardFields, domain.CardField{
			Field: cf.Field,
			Role:  domain.CardFieldRole(cf.Role),
		})
	}
	for _, vd := range doc.Views {
		m.Views = append(m.Views, domain.View{
			ID:      vd.ID,
			Name:    vd.Name,
			Type:    domain.ViewKind(vd.Type),
			GroupBy: vd.GroupBy,
		})
	}

	// Phase 4 (005-runtime-lifecycle.md): the inferences a Machine carries however it was built. Kept as
	// the last step so everything above reads as a faithful transcription of the document, and the
	// conveniences the runtime expands are all in one place.
	return Normalize(m), nil
}

// CoerceDefault is coerceDefault for a caller outside this package that holds the same raw string a
// `default:` key carries (the AI assistant's validator, which must judge a generated default exactly
// as the loader will).
func CoerceDefault(fieldType domain.FieldType, raw string) (any, error) {
	return coerceDefault(fieldType, raw)
}

// coerceDefault converts a Field's raw YAML default string into the same in-memory type
// data.ValuesFromForm produces for that FieldType, so internal/data.ApplyDefaults never needs to
// re-parse it. Errors here fail metadata loading at startup (005-runtime-lifecycle.md Phase 3-4:
// invalid metadata must not enter execution) rather than silently producing a wrong default.
func coerceDefault(fieldType domain.FieldType, raw string) (any, error) {
	switch fieldType {
	case domain.FieldTypeNumber, domain.FieldTypeMoney:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("not a number: %w", err)
		}
		return n, nil
	case domain.FieldTypeBoolean:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("not a boolean: %w", err)
		}
		return b, nil
	default:
		return raw, nil
	}
}

func balancedOf(d *balancedDoc) *domain.BalanceRule {
	if d == nil {
		return nil
	}
	return &domain.BalanceRule{Machine: d.Of, Via: d.Via, Debit: d.Debit, Credit: d.Credit, Reason: d.Reason}
}

func generatesOf(d *generatesDoc) *domain.GenerateSchedule {
	if d == nil {
		return nil
	}
	return &domain.GenerateSchedule{Machine: d.Machine, CountField: d.CountField, LinkField: d.LinkField, DueField: d.DueField,
		DueEveryDays: d.DueEveryDays, AmountField: d.AmountField, AmountFrom: d.AmountFrom}
}

func numberingOf(d *numberingDoc) *domain.FieldNumbering {
	if d == nil {
		return nil
	}
	return &domain.FieldNumbering{Pattern: d.Pattern, Codes: d.Codes}
}
