package rendering

import (
	"context"

	"menata.app/internal/domain"
)

// emptyReferenceNote is the sentence under a required reference control that has nothing to pick: the form cannot be
// saved until a record of the related Machine exists, and without this the only sign is the server's refusal. "" when
// there is something to pick, the Field is optional, or the related Machine is not one this Workspace names (a Person
// Field lists members, whose absence is not a missing record to create).
func emptyReferenceNote(ctx context.Context, f domain.Field, relations RelationOptions, required bool) string {
	if !required || len(relations[f.RelatedMachine]) > 0 {
		return ""
	}
	for _, m := range CurrentWorkspace(ctx).Machines {
		if m.ID == f.RelatedMachine {
			return "No " + m.Name + " exists yet -- add one first, then come back to this form."
		}
	}
	return ""
}
