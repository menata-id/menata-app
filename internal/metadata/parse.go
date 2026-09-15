package metadata

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
)

// machineDoc and fieldDoc are the YAML serialization of a Machine (004-runtime-metadata.md
// "Serialization Independence" -- YAML is one possible representation, not the model itself).
type machineDoc struct {
	ID     string     `yaml:"id"`
	Name   string     `yaml:"name"`
	Fields []fieldDoc `yaml:"fields"`
}

type fieldDoc struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Required bool     `yaml:"required"`
	Options  []string `yaml:"options"`
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
		m.Fields = append(m.Fields, domain.Field{
			ID:       fd.ID,
			Name:     fd.Name,
			Type:     domain.FieldType(fd.Type),
			Required: fd.Required,
			Options:  fd.Options,
		})
	}
	return m, nil
}
