package data

import (
	"fmt"
	"strings"

	"menata.app/internal/domain"
)

// RecordValidationError aggregates every problem found in one record's values, mirroring
// metadata.ValidationError so authors see every problem at once.
type RecordValidationError struct {
	Issues []string
}

func (e *RecordValidationError) Error() string {
	return fmt.Sprintf("invalid record:\n  - %s", strings.Join(e.Issues, "\n  - "))
}

// ValidateRecord checks record values against a Machine's Field declarations: every value must
// name a known Field, every required Field must be present, and a status Field's value must be
// one of its declared options. This is the Data Plane's own boundary -- a write must not reach
// physical storage carrying values the Domain Plane doesn't recognize
// (005-runtime-lifecycle.md "Security Ordering" / Phase 3 discipline applied to data writes).
func ValidateRecord(m *domain.Machine, values map[string]any) error {
	var issues []string

	fieldsByID := make(map[string]domain.Field, len(m.Fields))
	for _, f := range m.Fields {
		fieldsByID[f.ID] = f
	}

	for key := range values {
		if _, ok := fieldsByID[key]; !ok {
			issues = append(issues, fmt.Sprintf("unknown field %q", key))
		}
	}

	for _, f := range m.Fields {
		v, present := values[f.ID]
		empty := !present || v == nil || v == ""

		if f.Required && empty {
			issues = append(issues, fmt.Sprintf("field %q is required", f.ID))
			continue
		}
		if empty {
			continue
		}

		switch f.Type {
		case domain.FieldTypeStatus:
			s, ok := v.(string)
			if !ok || !contains(f.Options, s) {
				issues = append(issues, fmt.Sprintf("field %q: value %v is not one of %v", f.ID, v, f.Options))
			}
		case domain.FieldTypeNumber:
			switch v.(type) {
			case float64, float32, int, int64:
			default:
				issues = append(issues, fmt.Sprintf("field %q: value %v is not a number", f.ID, v))
			}
		case domain.FieldTypeRelation, domain.FieldTypePerson:
			if s, ok := v.(string); !ok || s == "" {
				issues = append(issues, fmt.Sprintf("field %q: value must be a record id", f.ID))
			}
		case domain.FieldTypeFile:
			if s, ok := v.(string); !ok || s == "" {
				issues = append(issues, fmt.Sprintf("field %q: value must be a storage key (an upload)", f.ID))
			}
		}
	}

	if len(issues) > 0 {
		return &RecordValidationError{Issues: issues}
	}
	return nil
}

func contains(options []string, v string) bool {
	for _, o := range options {
		if o == v {
			return true
		}
	}
	return false
}
