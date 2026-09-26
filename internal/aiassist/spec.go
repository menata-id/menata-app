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
	Roles    []string           `json:"roles"`
	Machines []GeneratedMachine `json:"machines"`
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
// must name another Machine in the same GeneratedChange (a generated Application cannot reference
// a Machine outside itself -- see validate.go).
type GeneratedField struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Required       bool     `json:"required"`
	Options        []string `json:"options,omitempty"`
	RelatedMachine string   `json:"related_machine,omitempty"`
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

// MetadataAddition is one purely-additive change to an Application already installed
// (GeneratedChange.Kind == "extend_application") -- exactly the row hot-reload-safety.md's own
// §3.3/§7.2 classification table already calls safe without a data-compatibility check: a new
// option on an existing status Field, a new role, or a new navigation item. Never a Field, a
// Machine, a Permission or a Transition on an existing Machine -- those interact with whatever
// data and behavior that Machine already has in ways this first increment does not attempt to
// reason about safely.
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
