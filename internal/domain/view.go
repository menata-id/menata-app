package domain

// ViewKind is how one declared View arranges a Machine's records (006-runtime-model.md "View",
// 007 §12.2). The set is closed and extended deliberately, not inferred (007 §14's
// static-registry seam) -- upstream's own registry carries ten View types; these are the three
// that already exist in this runtime's code, and a fourth arrives when a screen earns it.
type ViewKind string

const (
	ViewTable ViewKind = "table"
	ViewBoard ViewKind = "board"
	// ViewCards renders each record through its Machine's own card_fields (Projection, 007 §7.6).
	// It is the first consumer of that primitive whose output varies per record -- see
	// KnownCardFieldRoles below, and internal/metadata's requirement that a cards View's Machine
	// actually declare card_fields.
	ViewCards ViewKind = "cards"
)

// KnownViewKinds is the closed set of View types the runtime currently understands.
var KnownViewKinds = map[ViewKind]bool{
	ViewTable: true,
	ViewBoard: true,
	ViewCards: true,
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

// KnownCardFieldRoles is the closed set of roles card_fields may declare.
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

// View is one declared arrangement of a Machine's records (006-runtime-model.md "View"). A
// Machine may declare several, each addressable by id, which is the whole point: before this,
// `view:` was a single anonymous block and a second arrangement of the same records could not be
// expressed at all.
//
// A View carries only what differs *between* arrangements. What describes the records themselves
// wherever they appear -- which Field gets the SLA badge, which Fields a card projects -- lives on
// the Machine instead, because those are read where no View is selected: the record detail page
// has no arrangement to pick, and internal/composition reads card_fields for a bespoke card
// outside any View at all. Putting them here would have forced an arbitrary choice at both sites.
type View struct {
	ID string
	// Name is what a viewer reads when choosing between arrangements. Required, and deliberately
	// not derived from Type: "table"/"cards" is the runtime engine's own vocabulary, and a label
	// the user reads is the Application author's to write -- the same separation every other
	// user-facing string in metadata already follows (Machine.Name, Field.Name).
	Name string
	Type ViewKind
	// GroupBy is a Field ID on the same Machine; meaningful only for ViewBoard, where its values
	// become the board's columns.
	GroupBy string
}

// EffectiveType returns v's Type, defaulting to ViewTable for the zero value -- so a Machine that
// declares no views: at all still renders exactly as it did before views existed.
func (v View) EffectiveType() ViewKind {
	if v.Type == "" {
		return ViewTable
	}
	return v.Type
}
