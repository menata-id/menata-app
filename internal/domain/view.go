package domain

// LayoutKind is a generic structural composition primitive a Machine's Experience uses
// (006-runtime-model.md "View", 007-composable-runtime-architecture.md §12.2). The set is
// closed and extended deliberately, not inferred (007 §14's static-registry seam).
type LayoutKind string

const (
	LayoutTable LayoutKind = "table"
	LayoutBoard LayoutKind = "board"
)

// KnownLayouts is the closed set of layouts the runtime currently understands.
var KnownLayouts = map[LayoutKind]bool{
	LayoutTable: true,
	LayoutBoard: true,
}

// View is a Machine's Experience declaration (006-runtime-model.md "View"). The zero value
// renders as LayoutTable -- a Machine needs no view: block at all to get the default.
type View struct {
	Layout LayoutKind
	// GroupBy is a Field ID on the same Machine; meaningful only for LayoutBoard.
	GroupBy string
	// SLAField is a date Field ID on the same Machine; when set, that Field renders as an
	// OVERDUE / "N day(s) left" badge instead of a plain date (ROADMAP.md Phase 13). Empty
	// means no Field on this Machine gets SLA treatment.
	SLAField string
}

// EffectiveLayout returns v's Layout, defaulting to LayoutTable for the zero value.
func (v View) EffectiveLayout() LayoutKind {
	if v.Layout == "" {
		return LayoutTable
	}
	return v.Layout
}
