// Package behavior evaluates Constraints against a proposed transition
// (006-runtime-model.md "Behavioral Model": Event -> Action -> Permission/Constraint -> Service/
// Data operation -> State change). It is a pure evaluator: fetching the related data a
// Constraint needs is the caller's job (internal/data), not this package's -- keeping evaluation
// deterministic and independently testable without a database.
package behavior

import (
	"fmt"
	"math"
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

// BoundError names every aggregate bound a write would break.
type BoundError struct {
	Issues []string
}

func (e *BoundError) Error() string {
	return fmt.Sprintf("limit exceeded:\n  - %s", strings.Join(e.Issues, "\n  - "))
}

// CheckBounds evaluates every aggregate bound on m for a write of values as record recordID ("" for a create), against
// siblings: every stored record of m (the caller fetches them, this performs no I/O). The record being written replaces
// its stored self, so an edit is judged as the total it would leave. Only the group the written record belongs to is
// totalled, so a write is never refused for another group's overrun. A record that names no group value, or no number,
// is outside every bound: required-ness is ValidateRecord's question, not this one's.
func CheckBounds(m *domain.Machine, recordID string, values map[string]any, siblings []*data.Record) error {
	var issues []string
	for _, b := range m.Bounds {
		group, ok := boundGroup(values[b.Per])
		if !ok {
			continue
		}
		own, _ := numberOf(values[b.SumOf])
		total := own
		for _, r := range siblings {
			if r.ID == recordID {
				continue
			}
			if g, ok := boundGroup(r.Values[b.Per]); !ok || g != group {
				continue
			}
			n, _ := numberOf(r.Values[b.SumOf])
			total += n
		}
		reason := b.Reason
		// Compared to the millionth, so instalments of 0.1 and 0.2 fit a value of 0.3 (float64 adds them to 0.30000000000000004).
		total = math.Round(total*1e6) / 1e6
		switch {
		case b.AtLeast != nil && total < *b.AtLeast:
			if reason == "" {
				reason = fmt.Sprintf("the total of %s may not go below %v", b.SumOf, *b.AtLeast)
			}
			issues = append(issues, fmt.Sprintf("%s: %s (it would be %v for %s)", b.ID, reason, total, group))
		case b.AtMost != nil && total > *b.AtMost:
			if reason == "" {
				reason = fmt.Sprintf("the total of %s may not exceed %v", b.SumOf, *b.AtMost)
			}
			issues = append(issues, fmt.Sprintf("%s: %s (it would be %v for %s)", b.ID, reason, total, group))
		}
	}
	if len(issues) > 0 {
		return &BoundError{Issues: issues}
	}
	return nil
}

func boundGroup(v any) (string, bool) {
	s := fmt.Sprint(v)
	if v == nil || s == "" || s == "<nil>" {
		return "", false
	}
	return s, true
}

func numberOf(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
