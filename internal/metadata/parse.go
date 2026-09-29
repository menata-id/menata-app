package metadata

import (
	"fmt"
	"strconv"

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
	Events      []eventDoc      `yaml:"events"`
	Permissions []permissionDoc `yaml:"permissions"`
	Transitions []transitionDoc `yaml:"transitions"`
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
	SLAField   string         `yaml:"sla_field"`
	CardFields []cardFieldDoc `yaml:"card_fields"`
	// Views are the declared arrangements of those records.
	Views []viewDoc `yaml:"views"`
	// AppendOnly -- see domain.Machine.AppendOnly.
	AppendOnly bool `yaml:"append_only"`
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
	NameField       string `yaml:"name_field"`
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
	Select    string        `yaml:"select"`
	Relations []relationDoc `yaml:"relations"`
	Where     *predicateDoc `yaml:"where"`
	Sort      []sortDoc     `yaml:"sort"`
	Limit     int           `yaml:"limit"`
}

// relationDoc is the YAML serialization of a domain.Relation (007 §7.5).
type relationDoc struct {
	ID      string `yaml:"id"`
	Machine string `yaml:"machine"`
	Via     string `yaml:"via"`
}

// predicateDoc is the YAML serialization of an expression.Predicate. It accepts both shapes on
// purpose: a bare field/op/value, which is what every filter in this repo already writes and what
// 007 §7.7 keeps valid as syntax sugar, or an `all:` list when more than one must hold.
type predicateDoc struct {
	comparisonDoc `yaml:",inline"`
	All           []comparisonDoc `yaml:"all"`
}

// predicate lowers the doc into the one representation, so a single comparison and a one-element
// `all:` are indistinguishable downstream -- the "compile to the common expression representation"
// half of §7.7.
func (d *predicateDoc) predicate() *expression.Predicate {
	if d == nil {
		return nil
	}
	if len(d.All) > 0 {
		out := &expression.Predicate{}
		for _, c := range d.All {
			out.All = append(out.All, expression.Comparison{Field: c.Field, Op: expression.Op(c.Op), Value: c.Value})
		}
		return out
	}
	if d.Field == "" && d.Op == "" && d.Value == "" {
		return nil
	}
	return &expression.Predicate{All: []expression.Comparison{
		{Field: d.Field, Op: expression.Op(d.Op), Value: d.Value},
	}}
}

// sortDoc is the YAML serialization of a domain.SortKey. `direction` rather than a bool, because
// `direction: desc` is what an author writes and `descending: true` is what a struct holds.
type sortDoc struct {
	Field     string `yaml:"field"`
	Direction string `yaml:"direction"`
}

type measureDoc struct {
	ID        string         `yaml:"id"`
	Aggregate string         `yaml:"aggregate"`
	Field     string         `yaml:"field"`
	Where     *comparisonDoc `yaml:"where"`
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
}

type constraintDoc struct {
	ID         string `yaml:"id"`
	On         string `yaml:"on"`
	WhenEquals string `yaml:"when_equals"`
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
		RecipientField string `yaml:"recipient_field"`
		PreferenceKey  string `yaml:"preference_key"`

		// composite_signed_document's own one new key (domain.Composite) -- parent_field and
		// target_field above are shared with a rollup, same question, same answer.
		SourceField string `yaml:"source_field"`
	} `yaml:"then"`
}

// rollupRuleDoc is one arm of a rollup: the child value to look for, and what the parent becomes
// when it is found.
type rollupRuleDoc struct {
	Value string `yaml:"value"`
	Set   string `yaml:"set"`
}

// permissionDoc is the YAML serialization of a Permission (ROADMAP.md Phase 16).
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
		m.Fields = append(m.Fields, domain.Field{
			ID:             fd.ID,
			Name:           fd.Name,
			Type:           fieldType,
			Required:       fd.Required,
			Options:        fd.Options,
			RelatedMachine: relatedMachine,
			Default:        def,
		})
	}
	for _, cd := range doc.Constraints {
		m.Constraints = append(m.Constraints, domain.Constraint{
			ID:         cd.ID,
			On:         cd.On,
			WhenEquals: cd.WhenEquals,
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
				RecipientField: ed.Then.RecipientField,
				PreferenceKey:  ed.Then.PreferenceKey,
			}
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
			Select: dd.Select, Limit: dd.Limit, Where: dd.Where.predicate(),
		}
		for _, rd := range dd.Relations {
			ds.Relations = append(ds.Relations, domain.Relation{ID: rd.ID, Machine: rd.Machine, Via: rd.Via})
		}
		for _, sd := range dd.Sort {
			// The declared direction is carried through unreduced so validateDataset can reject an
			// unknown one; collapsing it here would make `direction: descending` sort ascending in
			// silence.
			ds.Sort = append(ds.Sort, domain.SortKey{Field: sd.Field, Direction: sd.Direction})
		}
		for _, md := range dd.Measures {
			ms := domain.Measure{
				ID:        md.ID,
				Aggregate: domain.AggregateKind(md.Aggregate),
				Field:     md.Field,
			}
			if md.Where != nil {
				ms.Where = &expression.Comparison{
					Field: md.Where.Field,
					Op:    expression.Op(md.Where.Op),
					Value: md.Where.Value,
				}
			}
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
			NameField:       doc.FlowTemplateStep.NameField,
			ActorField:      doc.FlowTemplateStep.ActorField,
			ActorTypeField:  doc.FlowTemplateStep.ActorTypeField,
			ActorGroupField: doc.FlowTemplateStep.ActorGroupField,
		}
	}

	m.SLAField = doc.SLAField
	m.AppendOnly = doc.AppendOnly
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

// coerceDefault converts a Field's raw YAML default string into the same in-memory type
// data.ValuesFromForm produces for that FieldType, so internal/data.ApplyDefaults never needs to
// re-parse it. Errors here fail metadata loading at startup (005-runtime-lifecycle.md Phase 3-4:
// invalid metadata must not enter execution) rather than silently producing a wrong default.
func coerceDefault(fieldType domain.FieldType, raw string) (any, error) {
	switch fieldType {
	case domain.FieldTypeNumber:
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
