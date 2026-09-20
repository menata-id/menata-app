package metadata

import (
	"fmt"
	"strconv"

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
	Events      []eventDoc      `yaml:"events"`
	Permissions []permissionDoc `yaml:"permissions"`
	Datasets    []datasetDoc    `yaml:"datasets"`
	Sequencing  *sequencingDoc  `yaml:"sequencing"`
	View        *viewDoc        `yaml:"view"`
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

// datasetDoc is the YAML serialization of a domain.Dataset (007 §7.2-§7.4). measures[].where
// reuses comparisonDoc, the same shape constraintDoc's block_if.condition already declares, so a
// filter reads identically wherever it appears in metadata.
type datasetDoc struct {
	ID        string       `yaml:"id"`
	Dimension string       `yaml:"dimension"`
	Measures  []measureDoc `yaml:"measures"`
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

type viewDoc struct {
	Layout     string         `yaml:"layout"`
	GroupBy    string         `yaml:"group_by"`
	SLAField   string         `yaml:"sla_field"`
	CardFields []cardFieldDoc `yaml:"card_fields"`
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
	Then       struct {
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
	} `yaml:"then"`
}

// rollupRuleDoc is one arm of a rollup: the child value to look for, and what the parent becomes
// when it is found.
type rollupRuleDoc struct {
	Value string `yaml:"value"`
	Set   string `yaml:"set"`
}

// permissionDoc is the YAML serialization of a Permission (ROADMAP.md Phase 16).
type permissionDoc struct {
	ID         string `yaml:"id"`
	Action     string `yaml:"action"`
	ActorField string `yaml:"actor_field"`
}

// Parse decodes Runtime Metadata YAML describing a single Machine. It performs structural
// (schema) parsing only -- semantic validation happens in Validate.
func Parse(data []byte) (*domain.Machine, error) {
	var doc machineDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	m := &domain.Machine{
		ID:   doc.ID,
		Name: doc.Name,
	}
	for _, fd := range doc.Fields {
		fieldType := domain.FieldType(fd.Type)
		relatedMachine := fd.Machine
		if fieldType == domain.FieldTypePerson {
			// Person always references a real mch_user record -- authors never write
			// `machine: mch_user` by hand (ROADMAP.md Phase 7).
			relatedMachine = domain.UserMachineID
		}
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
		m.Events = append(m.Events, domain.Event{
			ID:         ed.ID,
			On:         ed.On,
			WhenEquals: ed.WhenEquals,
			OnCreate:   ed.OnCreate,
			Then:       then,
		})
	}
	for _, pd := range doc.Permissions {
		m.Permissions = append(m.Permissions, domain.Permission{
			ID:         pd.ID,
			Action:     pd.Action,
			ActorField: pd.ActorField,
		})
	}
	for _, dd := range doc.Datasets {
		ds := domain.Dataset{ID: dd.ID, Source: doc.ID, Dimension: dd.Dimension}
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

	if doc.View != nil {
		var cardFields []domain.CardField
		for _, cf := range doc.View.CardFields {
			cardFields = append(cardFields, domain.CardField{
				Field: cf.Field,
				Role:  domain.CardFieldRole(cf.Role),
			})
		}
		m.View = domain.View{
			Layout:     domain.LayoutKind(doc.View.Layout),
			GroupBy:    doc.View.GroupBy,
			SLAField:   doc.View.SLAField,
			CardFields: cardFields,
		}
	}
	return m, nil
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
