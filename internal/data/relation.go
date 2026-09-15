package data

import (
	"context"
	"errors"
	"fmt"

	"menata.app/internal/domain"
)

// ValidateRelations checks that every reference field's value (Relation or Person alike, per
// domain.Field.IsReference) refers to a record that actually exists in its target Machine.
// Unlike ValidateRecord, this needs the Store, so it stays a separate call -- a shape check (is
// this a record id?) and an existence check (does that record exist?) are different concerns
// run at different points.
func ValidateRelations(ctx context.Context, store *Store, m *domain.Machine, values map[string]any) error {
	var issues []string

	for _, f := range m.Fields {
		if !f.IsReference() {
			continue
		}
		id, ok := values[f.ID].(string)
		if !ok || id == "" {
			continue // absence/shape already reported by ValidateRecord
		}

		_, err := store.GetRecord(ctx, f.RelatedMachine, id)
		if errors.Is(err, ErrRecordNotFound) {
			issues = append(issues, fmt.Sprintf("field %q: no %s record %q", f.ID, f.RelatedMachine, id))
			continue
		}
		if err != nil {
			return err
		}
	}

	if len(issues) > 0 {
		return &RecordValidationError{Issues: issues}
	}
	return nil
}
