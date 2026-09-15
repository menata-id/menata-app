package metadata

import (
	"fmt"
	"regexp"
	"strings"

	"menata.app/internal/domain"
)

// machineIDPattern and fieldIDPattern enforce 004-runtime-metadata.md "Stable Identity":
// identity must survive label/presentation/implementation changes, so it is validated
// independently of Name.
var (
	machineIDPattern = regexp.MustCompile(`^mch_[a-z][a-z0-9_]*$`)
	fieldIDPattern   = regexp.MustCompile(`^fld_[a-z][a-z0-9_]*$`)
)

// ValidationError aggregates every problem found in one metadata document, per 005-runtime-
// lifecycle.md Phase 3: invalid metadata must not enter executable planning, and a metadata
// author should see every problem at once rather than one failure per fix-and-rerun cycle.
type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid metadata:\n  - %s", strings.Join(e.Issues, "\n  - "))
}

// Validate checks a parsed Machine against the Domain Plane's structural rules: stable
// identity, known field types, and no duplicate field identity within the Machine.
func Validate(m *domain.Machine) error {
	var issues []string

	if !machineIDPattern.MatchString(m.ID) {
		issues = append(issues, fmt.Sprintf("machine id %q must match %s", m.ID, machineIDPattern.String()))
	}
	if m.Name == "" {
		issues = append(issues, fmt.Sprintf("machine %q: name is required", m.ID))
	}

	seen := make(map[string]bool, len(m.Fields))
	for _, f := range m.Fields {
		if !fieldIDPattern.MatchString(f.ID) {
			issues = append(issues, fmt.Sprintf("field id %q must match %s", f.ID, fieldIDPattern.String()))
			continue
		}
		if seen[f.ID] {
			issues = append(issues, fmt.Sprintf("field id %q is declared more than once", f.ID))
		}
		seen[f.ID] = true

		if !domain.KnownFieldTypes[f.Type] {
			issues = append(issues, fmt.Sprintf("field %q: unknown type %q", f.ID, f.Type))
		}
		if f.Type == domain.FieldTypeStatus && len(f.Options) == 0 {
			issues = append(issues, fmt.Sprintf("field %q: type status requires at least one option", f.ID))
		}
		if f.Type == domain.FieldTypeRelation && !machineIDPattern.MatchString(f.RelatedMachine) {
			issues = append(issues, fmt.Sprintf("field %q: type relation requires a valid target machine id, got %q", f.ID, f.RelatedMachine))
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}
