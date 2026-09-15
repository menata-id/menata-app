package metadata

import (
	"fmt"
	"regexp"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// machineIDPattern, fieldIDPattern, and constraintIDPattern enforce 004-runtime-metadata.md
// "Stable Identity": identity must survive label/presentation/implementation changes, so it is
// validated independently of Name.
var (
	machineIDPattern    = regexp.MustCompile(`^mch_[a-z][a-z0-9_]*$`)
	fieldIDPattern      = regexp.MustCompile(`^fld_[a-z][a-z0-9_]*$`)
	constraintIDPattern = regexp.MustCompile(`^cst_[a-z][a-z0-9_]*$`)
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
	fieldsByID := make(map[string]domain.Field, len(m.Fields))
	for _, f := range m.Fields {
		if !fieldIDPattern.MatchString(f.ID) {
			issues = append(issues, fmt.Sprintf("field id %q must match %s", f.ID, fieldIDPattern.String()))
			continue
		}
		if seen[f.ID] {
			issues = append(issues, fmt.Sprintf("field id %q is declared more than once", f.ID))
		}
		seen[f.ID] = true
		fieldsByID[f.ID] = f

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

	for _, c := range m.Constraints {
		issues = append(issues, validateConstraint(m, c, fieldsByID)...)
	}

	if m.View.Layout != "" && !domain.KnownLayouts[m.View.Layout] {
		issues = append(issues, fmt.Sprintf("machine %q: view.layout %q is not a known layout", m.ID, m.View.Layout))
	}
	if m.View.EffectiveLayout() == domain.LayoutBoard {
		if _, ok := fieldsByID[m.View.GroupBy]; !ok {
			issues = append(issues, fmt.Sprintf("machine %q: view.group_by %q is not a field of this machine", m.ID, m.View.GroupBy))
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateConstraint checks one Constraint's own shape: everything a single Machine file can
// verify on its own. Cross-Machine checks (does the related Machine/field actually exist) happen
// once the whole Application is loaded (application.go's validateConstraintTargets), the same
// split already used for relation fields.
func validateConstraint(m *domain.Machine, c domain.Constraint, fieldsByID map[string]domain.Field) []string {
	var issues []string

	if !constraintIDPattern.MatchString(c.ID) {
		issues = append(issues, fmt.Sprintf("constraint id %q must match %s", c.ID, constraintIDPattern.String()))
	}

	onField, onExists := fieldsByID[c.On]
	if !onExists {
		issues = append(issues, fmt.Sprintf("constraint %q: on %q is not a field of machine %q", c.ID, c.On, m.ID))
	} else if onField.Type == domain.FieldTypeStatus && !contains(onField.Options, c.WhenEquals) {
		issues = append(issues, fmt.Sprintf("constraint %q: when_equals %q is not one of field %q's options %v", c.ID, c.WhenEquals, c.On, onField.Options))
	}
	if c.WhenEquals == "" {
		issues = append(issues, fmt.Sprintf("constraint %q: when_equals is required", c.ID))
	}

	if !machineIDPattern.MatchString(c.BlockIf.RelatedMachine) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.related_machine %q must match %s", c.ID, c.BlockIf.RelatedMachine, machineIDPattern.String()))
	}
	if !fieldIDPattern.MatchString(c.BlockIf.RelatedField) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.related_field %q must match %s", c.ID, c.BlockIf.RelatedField, fieldIDPattern.String()))
	}
	if !fieldIDPattern.MatchString(c.BlockIf.Condition.Field) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.field %q must match %s", c.ID, c.BlockIf.Condition.Field, fieldIDPattern.String()))
	}
	if !expression.KnownOps[c.BlockIf.Condition.Op] {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.op %q is not a known operator", c.ID, c.BlockIf.Condition.Op))
	}
	if c.BlockIf.Condition.Value == "" {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.value is required", c.ID))
	}

	return issues
}

func contains(options []string, v string) bool {
	for _, o := range options {
		if o == v {
			return true
		}
	}
	return false
}
