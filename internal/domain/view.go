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

// CardFieldRole is the semantic role a projected Field plays on a composed card -- 007 §7.6
// (Projection): the card's renderer picks markup by role, not by the Field's own storage type, so
// the same role works whether the underlying Field happens to be a plain string or a relation.
// Closed set, extended deliberately, not inferred (007 §14's static-registry seam), the same
// discipline KnownFieldTypes/KnownActions/KnownServices already follow.
type CardFieldRole string

const (
	CardFieldRoleTitle  CardFieldRole = "title"
	CardFieldRolePerson CardFieldRole = "person"
	CardFieldRoleMoney  CardFieldRole = "money"
	CardFieldRoleStatus CardFieldRole = "status"
	CardFieldRoleDate   CardFieldRole = "date"
)

// KnownCardFieldRoles is the closed set of roles view.card_fields may declare.
var KnownCardFieldRoles = map[CardFieldRole]bool{
	CardFieldRoleTitle:  true,
	CardFieldRolePerson: true,
	CardFieldRoleMoney:  true,
	CardFieldRoleStatus: true,
	CardFieldRoleDate:   true,
}

// CardField names one of this Machine's own Fields to project onto a composed card (Approval
// Inbox's pendingApprovalCard today; any other composed screen later, per the same admission-gate
// reasoning HomeRoute/PrimaryNavGroup already established for metadata-derived values), plus the
// semantic role it should render as. This is the composable-runtime kajian's Fase 1 pilot: a
// composed card's own field list becomes metadata, not a hardcoded struct + .templ edit.
type CardField struct {
	Field string
	Role  CardFieldRole
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
	// CardFields is the ordered list of this Machine's own Fields a composed card should project,
	// each with its semantic role. Empty (the default for every Machine today) means no composed
	// card renders anything beyond what it already hardcodes -- this is purely additive.
	CardFields []CardField
}

// EffectiveLayout returns v's Layout, defaulting to LayoutTable for the zero value.
func (v View) EffectiveLayout() LayoutKind {
	if v.Layout == "" {
		return LayoutTable
	}
	return v.Layout
}
