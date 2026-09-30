// Package aiassist implements the AI Metadata Assistant (Flow 2 gap study Tahap 8): a conversation,
// backed by the Gemini API, that proposes purely-additive Runtime Metadata -- a brand-new
// Application, or an addition to one already installed -- reviewed by a Workspace admin before it
// is written to disk and reloaded live.
//
// It deliberately cannot propose everything a hand-written *.yaml file can. Two boundaries, both
// load-bearing rather than incidental:
//
//   - Only what is already fully composable today: Machines, Fields, role-based Permissions,
//     status Transitions moved through the generic edit route, on-create Events, plain
//     append-don't-rewrite additions to an existing Application's own options/roles/navigation.
//     Never a dedicated Approve/Reject-with-signatures workflow -- internal/action's own doc
//     comment says that engine is hardcoded to mch_document/mch_approval_step, not generic, and a
//     generated Application that promised one would be exactly the "generator that emits YAML
//     that then needs per-Application code to work" ROADMAP.md warns against.
//   - Only additive changes: a new file, or an appended list entry. Never a rewrite, a removal, or
//     a retargeted relation -- the one property that makes it safe to reload live without the
//     general hot-reload-safety.md compatibility gate (menata-app-document's own design, which
//     this package deliberately implements only the narrow additive slice of).
//
// See menata-app-document's audits/2026-09-25-kajian-new-application-ai.md for the fuller design
// reasoning this package implements.
package aiassist

// GeneratedChange is one AI-proposed metadata change, in the constrained shape the Gemini
// conversation is grounded to (see prompt.go) -- deliberately narrower than domain.Application/
// domain.Machine's own full shape, since it may only ever describe what's already composable.
type GeneratedChange struct {
	// Kind is "new_application" or "extend_application" -- the two modes the owner asked to be
	// unified in one conversation rather than built as separate features.
	Kind string `json:"kind"`
	// TargetAppID names the Application being extended; set only when Kind is "extend_application".
	TargetAppID string `json:"target_app_id,omitempty"`

	Application *GeneratedApplication `json:"application,omitempty"`
	Additions   []MetadataAddition    `json:"additions,omitempty"`
}

const (
	KindNewApplication    = "new_application"
	KindExtendApplication = "extend_application"
)

// GeneratedApplication is a whole new Application: its own card face, role vocabulary, and the
// Machines it exposes. Set only when GeneratedChange.Kind is "new_application".
type GeneratedApplication struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	// Roles is this Application's own role vocabulary (domain.Application.Roles) -- e.g.
	// ["Employee", "Supervisor", "HR admin"] for the mockup's own Leave Requests example.
	Roles []string `json:"roles"`
	// PublisherRole is which of Roles the person publishing this conversation will hold
	// themselves, once it goes live -- must be one of Roles (validate.go). Without this, nobody
	// held any role in a brand-new Application the moment it was created, including its own
	// creator: publishing redirected straight into a 403 ("you have no role in <name>"), found
	// 2026-09-27 chasing a real conversation that hit exactly that. This runtime's own
	// workspace_member_app_roles table holds one *direct* role per person per Application (its
	// primary key is workspace/user/application, no room for a second row) -- a second role would
	// need a Group, which this first increment does not create -- so the conversation must ask
	// which one role the person wants, rather than the code guessing or granting all of them.
	PublisherRole string             `json:"publisher_role"`
	Machines      []GeneratedMachine `json:"machines"`
	// Navigation is this Application's menu, in the order the person wants it: which of Machines
	// get a menu entry and what each entry says. The first entry is also where the Application's
	// Workspace Home card opens. The conversation must ask for it (prompt.go, rule 6); nothing
	// derives it from Machines, because which records deserve a menu entry is the person's
	// decision. Until 2026-09-30 the writer made one entry for the first Machine and nothing else,
	// so a waste-reporting Application reached its branch list and not its waste report.
	Navigation []GeneratedMenuItem `json:"navigation"`
}

// GeneratedMenuItem is one menu entry of a new Application: a label and the Machine whose generic
// list page (/machines/{id}) it opens -- the only route this package can name, for the reason
// GeneratedNavItem gives.
type GeneratedMenuItem struct {
	Label     string `json:"label"`
	MachineID string `json:"machine_id"`
}

// GeneratedMachine is one Machine the Application exposes. Deliberately narrower than
// domain.Machine: no Datasets, Views, Constraints, CardFields or Sequencing in this first
// increment -- every generated Machine gets the runtime's own implicit default (one table view,
// no aggregate, unrestricted transitions unless declared) rather than the assistant guessing at a
// shape with no forcing case yet. A second real request that needs one of those is this package's
// own trigger to grow it, the same discipline the rest of this codebase holds everywhere else.
type GeneratedMachine struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Fields      []GeneratedField      `json:"fields"`
	Permissions []GeneratedPermission `json:"permissions"`
	Transitions []GeneratedTransition `json:"transitions"`
	Events      []GeneratedEvent      `json:"events"`
}

// GeneratedField mirrors domain.Field's own composable surface. Type is restricted at validation
// time to domain.KnownFieldTypes; RelatedMachine is meaningful only when Type is "relation" and
// must name a Machine in the same GeneratedChange or one already in this Workspace.
type GeneratedField struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Required       bool     `json:"required"`
	Options        []string `json:"options,omitempty"`
	RelatedMachine string   `json:"related_machine,omitempty"`
	// Compute makes a number Field computed (domain.FieldCompute): never entered, derived on save.
	Compute *GeneratedCompute `json:"compute,omitempty"`
}

// GeneratedCompute mirrors domain.FieldCompute.
type GeneratedCompute struct {
	Op     string   `json:"op"`
	Fields []string `json:"fields"`
}

// GeneratedPermission mirrors the one shape a generated Machine may declare: role-based, on the
// three record-scoped Actions a plain CRUD screen already performs (create/edit/delete). Never
// "decide" or "revise" -- both are hardcoded-workflow Actions with no generic route to reach them
// from, so a Permission naming either would gate a route that does not exist for this Machine.
type GeneratedPermission struct {
	ID     string   `json:"id"`
	Action string   `json:"action"`
	Roles  []string `json:"roles"`
}

// GeneratedTransition mirrors domain.Transition, with Action fixed to "edit" -- a generated
// Machine's status model always moves through the generic edit form, since "decide" does not exist
// for it. This is the honest shape of "approval" in a generated Application: a role-gated status
// field, not an Approve/Reject workflow (see this package's own doc comment).
type GeneratedTransition struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// GeneratedEvent mirrors the one Service shape safe to generate unattended: log_activity, on
// creation or on a field reaching one value. rollup_parent_status is not offered -- it names a
// parent/child relationship shape that would need cross-Machine validation this first increment
// does not build.
type GeneratedEvent struct {
	ID         string `json:"id"`
	On         string `json:"on,omitempty"`
	WhenEquals string `json:"when_equals,omitempty"`
	OnCreate   bool   `json:"on_create,omitempty"`
	Summary    string `json:"summary"`
}

// MetadataAddition is one change to an Application already installed (GeneratedChange.Kind ==
// "extend_application"). Two kinds, deliberately separated:
//
//   - **Additions**, which leave everything that exists untouched: a new option on a status Field, a
//     new role, a new Machine (which has no records yet, so no data can disagree with it), a new
//     navigation item. These are hot-reload-safety.md §3.3/§7.2's "safe without a data check" row.
//   - **Presentation changes**, which change a value but only one that no record, Permission or id
//     depends on: the Application's name, a menu item's label, the menu's order. An Application's id
//     and a navigation item's id stay what they were -- ids are what everything else references
//     (CLAUDE.md: "a name is never an identity"), which is exactly why the name is safe to change.
//
// Still never: removing anything, or changing an existing Machine's Fields, Permissions or
// Transitions. Those interact with the records and behaviour the Machine already has. Until
// 2026-09-30 only the first two additions existed (and a navigation item validated but was never
// written), so a request to add a water-usage report beside an installed waste-report Application,
// with a menu for it, was refused.
type MetadataAddition struct {
	// MachineID/NewOption: append NewOption to that Machine's own status Field's declared options
	// (FieldID names which one). Both must already exist; NewOption must not already be declared.
	MachineID string `json:"machine_id,omitempty"`
	FieldID   string `json:"field_id,omitempty"`
	NewOption string `json:"new_option,omitempty"`
	// NewRole: append a role to the target Application's own roles: vocabulary.
	NewRole string `json:"new_role,omitempty"`
	// NewNavItem: append a navigation entry to the target Application's own navigation -- title/
	// description only; Route/route-bearing fields are never generated (a nav item must point at a
	// real handler, which this package cannot create).
	NewNavItem *GeneratedNavItem `json:"new_nav_item,omitempty"`
	// NewMachine: add a whole Machine to the target Application. Its relation Fields may point at
	// any Machine already in this Workspace, including one another Application owns -- Machines are
	// the Workspace's, not an Application's (domain.Workspace).
	NewMachine *GeneratedMachine `json:"new_machine,omitempty"`
	// RenameApplication: the target Application's new display name. Its id does not change.
	RenameApplication string `json:"rename_application,omitempty"`
	// RelabelNavItem: a new label for one of the target Application's navigation items, by id.
	RelabelNavItem *NavRelabel `json:"relabel_nav_item,omitempty"`
	// ReorderNavigation: every navigation item id of the target Application, including any this same
	// change adds, in the new order.
	ReorderNavigation []string `json:"reorder_navigation,omitempty"`
}

// NavRelabel names one existing navigation item and the label it should carry.
type NavRelabel struct {
	NavID string `json:"nav_id"`
	Label string `json:"label"`
}

// GeneratedNavItem is deliberately narrow: it may only redescribe a destination that already
// exists (a Machine's own generic list page), never invent a route. See writer.go.
type GeneratedNavItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// MachineID is the existing Machine this item links to (its generic /machines/{id} page) --
	// the only route shape this package is willing to generate, since it is the one guaranteed to
	// already have a real handler regardless of what the Application declares.
	MachineID string `json:"machine_id"`
	Icon      string `json:"icon,omitempty"`
}
