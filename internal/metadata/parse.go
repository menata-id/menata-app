package metadata

import (
	"fmt"

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
	Permissions []permissionDoc `yaml:"permissions"`
	View        *viewDoc        `yaml:"view"`
}

type viewDoc struct {
	Layout   string `yaml:"layout"`
	GroupBy  string `yaml:"group_by"`
	SLAField string `yaml:"sla_field"`
}

type fieldDoc struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Required bool     `yaml:"required"`
	Options  []string `yaml:"options"`
	// Machine is the target Machine ID, meaningful only when type is "relation".
	Machine string `yaml:"machine"`
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
		m.Fields = append(m.Fields, domain.Field{
			ID:             fd.ID,
			Name:           fd.Name,
			Type:           fieldType,
			Required:       fd.Required,
			Options:        fd.Options,
			RelatedMachine: relatedMachine,
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
	for _, pd := range doc.Permissions {
		m.Permissions = append(m.Permissions, domain.Permission{
			ID:         pd.ID,
			Action:     pd.Action,
			ActorField: pd.ActorField,
		})
	}
	if doc.View != nil {
		m.View = domain.View{
			Layout:   domain.LayoutKind(doc.View.Layout),
			GroupBy:  doc.View.GroupBy,
			SLAField: doc.View.SLAField,
		}
	}
	return m, nil
}
