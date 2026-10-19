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
	// ViewStepper renders a sequential done/current/waiting progress indicator over a *different*
	// Machine's records that reference this View's own Machine as their parent -- the exact
	// relationship a Sequencing declaration already names (Machine.Sequencing.ParentField). This
	// is `menata-runtime`'s CAP-V20 (capability-registry.md), "a View composing other Views"
	// (ROADMAP.md Planned): the first View type that arranges a parent record's children rather
	// than a Machine's own records. Requires the Machine to declare `sequencing:` --
	// internal/metadata.validateView refuses one that doesn't, mirroring the existing
	// ViewCards/card_fields requirement -- since there is no ordering/decision concept to render
	// otherwise. Composed by internal/composition.Loader.ChildSections' caller
	// (internal/rendering/detail.templ), not by a top-level ?view= switch: a Machine using this
	// type today (mch_approval_step) has no navigable list page of its own for one to select on.
	ViewStepper ViewKind = "stepper"
)

// KnownViewKinds is the closed set of View types the runtime currently understands.
var KnownViewKinds = map[ViewKind]bool{
	ViewTable:   true,
	ViewBoard:   true,
	ViewCards:   true,
	ViewStepper: true,
}

// CardFieldRole is the semantic role a projected Field plays on a composed card -- 007 §7.6
// (Projection): the card's renderer picks markup by role, not by the Field's own storage type, so
// the same role works whether the underlying Field happens to be a plain string or a relation.
// Closed set, extended deliberately, not inferred (007 §14's static-registry seam), the same
// discipline KnownFieldTypes/KnownActions and registry.Services already follow.
type CardFieldRole string

const (
	CardFieldRoleTitle  CardFieldRole = "title"
	CardFieldRolePerson CardFieldRole = "person"
	CardFieldRoleMoney  CardFieldRole = "money"
	CardFieldRoleStatus CardFieldRole = "status"
	CardFieldRoleDate   CardFieldRole = "date"
	// CardFieldRoleColor is a Field holding one entry of KnownTagColors. It is read on a Machine
	// whose records are the tags another Machine's card_tags: draws (a label's own colour).
	CardFieldRoleColor CardFieldRole = "color"
	// CardFieldRoleFile is a Field of type file holding a stored upload. A child Machine declaring it is
	// drawn on its parent's detail page as an attachments list, one row per record.
	CardFieldRoleFile CardFieldRole = "file"
	// CardFieldRoleComment is a long_text Field holding what a comment says. A child Machine declaring it is
	// drawn in its parent's "Comments and activity" feed, one entry per record, beside the parent's own events.
	// Who wrote it is not a second role: it is the Machine's person Field that declares `stamp: current_user`,
	// which validation requires.
	CardFieldRoleComment CardFieldRole = "comment"
	// CardFieldRoleContainer is a relation Field naming the record this one sits inside -- a Task's Project. A
	// page draws that record's own title (its Machine's `title` role, or its first Field when it declares none),
	// resolved for only the records the page lists. A person Field is not a container: people are the `person`
	// role, labelled through the Workspace's members.
	CardFieldRoleContainer CardFieldRole = "container"
	// CardFieldRoleLink is a text Field holding the in-app route a record leads to when it is opened (a notification's
	// destination). A page takes it as a link's address with `from: {href: link}`; the runtime hands the page the value
	// only when it is a path inside the app (`SafeInAppPath`), so a stored value can never send a viewer elsewhere.
	CardFieldRoleLink CardFieldRole = "link"
	// CardFieldRoleSubtitle and CardFieldRoleDetail are the second and third lines a record says about itself under its
	// title: a count, a role, an address. Plain text of any Field; what they hold is the Machine's own words, so a page
	// draws them without knowing what they are.
	CardFieldRoleSubtitle CardFieldRole = "subtitle"
	CardFieldRoleDetail   CardFieldRole = "detail"
	// CardFieldRoleSummary is a longer plain sentence a record says about itself (what a member can actually do, once their
	// Groups are counted), drawn as a paragraph beneath the lines above.
	CardFieldRoleSummary CardFieldRole = "summary"
)

// SafeInAppPath reports whether s is a route inside this app: one slash then something that is not a second slash or
// a backslash (both read by a browser as a host), no scheme, no control characters. A link role's value that fails
// this is treated as no destination at all.
func SafeInAppPath(s string) bool {
	if len(s) < 2 || s[0] != '/' || s[1] == '/' || s[1] == '\\' {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// TagColor is one entry of the closed palette a tag may be drawn in. A Workspace picks a name, never a
// colour value: the palette is shared so a tag reads the same on every board and stays legible on a
// small card (the Board Settings mockup states the rule in those words), and the Tailwind mapping lives
// only in internal/rendering (007 §15.2).
type TagColor string

const (
	TagBlue    TagColor = "blue"
	TagPurple  TagColor = "purple"
	TagAmber   TagColor = "amber"
	TagSlate   TagColor = "slate"
	TagEmerald TagColor = "emerald"
	TagCyan    TagColor = "cyan"
	TagRose    TagColor = "rose"
)

// KnownTagColors is the closed palette, in the order Board Settings lists it.
var KnownTagColors = []TagColor{TagBlue, TagPurple, TagAmber, TagSlate, TagEmerald, TagCyan, TagRose}

// IsTagColor reports whether s names an entry of the palette.
func IsTagColor(s string) bool {
	for _, c := range KnownTagColors {
		if string(c) == s {
			return true
		}
	}
	return false
}

// CardTags declares the tags a record's card shows: the records of a join Machine that point at it,
// and the tag each one points at. It is 007 §7.5's Relation (`machine`, `via`) with the one thing a
// Relation alone does not say -- which of the join's *other* reference Fields is the thing to draw.
//
// Nothing here restates a Field: `via` and `tag` both name reference Fields the join Machine already
// has, and what a tag *looks like* is that tag Machine's own card_fields (a `title` role for its name, a
// `color` role for its colour), so a Workspace's labels are described once, on the label.
type CardTags struct {
	Machine string
	Via     string
	Tag     string
}

// KnownCardFieldRoles is the closed set of roles card_fields may declare.
var KnownCardFieldRoles = map[CardFieldRole]bool{
	CardFieldRoleTitle:     true,
	CardFieldRolePerson:    true,
	CardFieldRoleMoney:     true,
	CardFieldRoleStatus:    true,
	CardFieldRoleDate:      true,
	CardFieldRoleColor:     true,
	CardFieldRoleFile:      true,
	CardFieldRoleComment:   true,
	CardFieldRoleContainer: true,
	CardFieldRoleLink:      true,
	CardFieldRoleSubtitle:  true,
	CardFieldRoleDetail:    true,
	CardFieldRoleSummary:   true,
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
	// ColumnActions are what a board's column menu offers for the column's own record when the board groups by a relation (a List):
	// each is one of KnownColumnActions, done through that Machine's generic record routes and offered only where its Permissions let
	// the viewer. Empty draws no menu. Meaningful only for ViewBoard with a relation group_by (metadata.validateView).
	ColumnActions []ColumnAction
	// AddColumn is the board's tile at the end of its columns, for a place that is not the board where columns are made: it leads to a
	// navigation item and says why. Nil draws no tile. Meaningful only for ViewBoard with a relation group_by.
	AddColumn *AddColumn
}

// HasColumnAction reports whether the View declares action in its column menu.
func (v View) HasColumnAction(a ColumnAction) bool {
	for _, c := range v.ColumnActions {
		if c == a {
			return true
		}
	}
	return false
}

// ColumnAction is one thing a board's column menu can do to the column's record.
type ColumnAction string

const (
	// ColumnRename changes the record's label Field (an edit); ColumnMove moves it one place earlier or later in its Machine's order,
	// which is the order of the columns (an edit); ColumnDelete removes it (a delete, after a question).
	ColumnRename ColumnAction = "rename"
	ColumnMove   ColumnAction = "move"
	ColumnDelete ColumnAction = "delete"
)

// KnownColumnActions is the closed set.
var KnownColumnActions = map[ColumnAction]bool{ColumnRename: true, ColumnMove: true, ColumnDelete: true}

// AddColumn is the words and destination of a board's "add a column" tile: Label names it, Note says what the place is for, and To is the
// id of a navigation item of an Application that installs the Machine (never a typed route, 001 #3).
type AddColumn struct {
	To, Label, Note string
}

// EffectiveType returns v's Type, defaulting to ViewTable for the zero value -- so a Machine that
// declares no views: at all still renders exactly as it did before views existed.
func (v View) EffectiveType() ViewKind {
	if v.Type == "" {
		return ViewTable
	}
	return v.Type
}
