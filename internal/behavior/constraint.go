// Package behavior evaluates Constraints against a proposed transition
// (006-runtime-model.md "Behavioral Model": Event -> Action -> Permission/Constraint -> Service/
// Data operation -> State change). It is a pure evaluator: fetching the related data a
// Constraint needs is the caller's job (internal/data), not this package's -- keeping evaluation
// deterministic and independently testable without a database.
package behavior

import (
	"fmt"
	"strings"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// ConstraintError aggregates every Constraint a transition violates, mirroring
// data.RecordValidationError so callers can present every problem at once.
type ConstraintError struct {
	Issues []string
}

func (e *ConstraintError) Error() string {
	return fmt.Sprintf("constraint violated:\n  - %s", strings.Join(e.Issues, "\n  - "))
}

// CheckConstraints evaluates every Constraint on m that fires for the transition described by
// values (the new field values about to be written to the record identified by recordID),
// against relatedRecords -- every record of each Constraint's block_if.related_machine, keyed by
// that Machine's ID. The caller fetches relatedRecords; CheckConstraints performs no I/O.
func CheckConstraints(m *domain.Machine, recordID string, values map[string]any, relatedRecords map[string][]*data.Record) error {
	var issues []string

	for _, c := range m.Constraints {
		newVal, present := values[c.On]
		if !present || fmt.Sprint(newVal) != c.WhenEquals {
			continue
		}

		for _, r := range relatedRecords[c.BlockIf.RelatedMachine] {
			if fmt.Sprint(r.Values[c.BlockIf.RelatedField]) != recordID {
				continue // this related record doesn't belong to the record being transitioned
			}
			if c.BlockIf.Condition.Evaluate(r.Values) {
				issues = append(issues, fmt.Sprintf(
					"%s: cannot set %s to %q while related %s record %s has %s %s %q",
					c.ID, c.On, c.WhenEquals, c.BlockIf.RelatedMachine, r.ID,
					c.BlockIf.Condition.Field, c.BlockIf.Condition.Op, c.BlockIf.Condition.Value,
				))
			}
		}
	}

	if len(issues) > 0 {
		return &ConstraintError{Issues: issues}
	}
	return nil
}
