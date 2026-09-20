package domain

// Sequencing declares that a Machine's records are acted on in order: a record is locked while an
// earlier sibling is still open. Declared on the Machine whose records are ordered (the child),
// because that is where both the ordering Field and the relation to the parent already live.
//
// The rule this replaces was never Machine-specific -- internal/action's own CanDecide read
// "a sibling with a lower order value is still pending" generically. What was hardcoded was the
// binding: which Field orders the siblings, which Field holds their state, and which values mean
// "still open" and "ordering applies". Those six names are what this declares.
//
// ModeField is read from the *parent* record, not this one, because whether ordering applies at
// all is a property of the whole process rather than of any single step: one Document runs
// sequentially and the next runs in parallel, using the same Machines. A record whose parent's
// ModeField holds anything other than SequentialValue is never locked.
type Sequencing struct {
	// ParentField is the reference Field on this Machine naming the record whose children are
	// ordered together. Siblings are the records sharing its value.
	ParentField string
	// ModeField is the Field on the *parent* Machine deciding whether ordering applies.
	ModeField string
	// SequentialValue is the one ModeField value that turns ordering on.
	SequentialValue string
	// OrderField is the number Field ordering the siblings. Lower values act first.
	OrderField string
	// StateField is the Field a sibling's own progress is read from.
	StateField string
	// OpenValue is the StateField value meaning "not decided yet" -- a sibling holding it, with a
	// lower OrderField, is what locks a record.
	OpenValue string
}
