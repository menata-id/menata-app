package rendering

import (
	"menata.app/internal/data"
	"menata.app/internal/experience"
)

// TaskRow is one Task ready to render in a Calendar cell: the record itself plus its Project's display
// name, resolved ahead of render time the same way ActivityEntry resolves its actor.
type TaskRow struct {
	Task        *data.Record
	ProjectName string
	// Title and Status are this Task's own declared display shape (mch_task's card_fields, 007 §7.6),
	// resolved by composition.ProjectedByRole -- the Page renders them in its own markup and never
	// picks a Field off the record itself (007 §4.4).
	Title  string
	Status string
	// SLA and DueText are the date Field in the `date` role, **already resolved** -- a badge with its
	// label and tone, and the stored value as text for the lists that show a plain date instead.
	//
	// They replaced `Due any` on 2026-10-03. That field's own comment said "slaBadgePill parses it",
	// which named the defect accurately: a Page was parsing a date and evaluating a business rule
	// against `time.Now()`. Composition resolves both now, with the `now` it already takes. Which
	// Field they come from is still the declaration's answer (composition.FieldForRole).
	SLA     experience.SLABadge
	DueText string
	// Complete, Date and Tags are what a Calendar cell draws beyond the shared shape above: the completion
	// circle (nil when the Machine declares no `completion:`), the date pill resolved exactly as a board
	// card's, and its tag chips.
	Complete *CardComplete
	Date     experience.CardDate
	Tags     []CardTag
	// Assignee is the display name of the Field in the `person` role, "" when nobody is assigned.
	Assignee string
}

// taskPatchURL is the Task's own record route: where its title links and where its circle writes.
func taskPatchURL(t TaskRow) string {
	return "/machines/" + t.Task.MachineID + "/records/" + t.Task.ID
}
