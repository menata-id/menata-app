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
//   - A change to an installed Application is its **whole desired state**, not a list of edits
//     (2026-09-30): the model is shown the Application as it is and returns it as it should be,
//     and PlanUpdate computes what differs -- Terraform's plan, applied to one Application. What a
//     plan may do is one policy in one place (plan.go): add anything, change display text and
//     authorization roles, never remove or retype what already holds records or grants access.
//     Everything the generated shape cannot express (datasets, views, workflow bindings, other
//     Services) is invisible to the model and preserved untouched, because the writer edits only
//     the keys it knows.
//
// See menata-app-document's audits/2026-09-25-kajian-new-application-ai.md for the fuller design
// reasoning this package implements.
package aiassist

// GeneratedChange is one AI-proposed metadata change, in the constrained shape the Gemini
// conversation is grounded to (see prompt.go) -- deliberately narrower than domain.Application/
// domain.Machine's own full shape, since it may only ever describe what's already composable.
type GeneratedChange struct {
	// Kind is "new_application" or "update_application" -- the two modes the owner asked to be
	// unified in one conversation rather than built as separate features.
	Kind string `json:"kind"`
	// TargetAppID names the installed Application being updated; set only for "update_application".
	TargetAppID string `json:"target_app_id,omitempty"`
	// Application is the whole Application: a new one, or an installed one as it should be after
	// the update. For an update, anything left out is read as a removal (PlanUpdate).
	Application *GeneratedApplication `json:"application,omitempty"`
	// ConfirmedRemovals are the removals the owner confirmed at review (PlanItem.Confirm). It is never part
	// of what the model returns or sees (json:"-"): a removal the model could confirm for itself is not
	// confirmed.
	ConfirmedRemovals []string `json:"-"`
}

const (
	KindNewApplication    = "new_application"
	KindUpdateApplication = "update_application"
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

// GeneratedMenuItem is one menu entry: a label and the Machine whose generic list page
// (/machines/{id}) it opens -- the only route this package can name, since it is the one that has a
// real handler whatever the Application declares.
//
// ID identifies an item that already exists, in an update: an existing item keeps its id, route and
// every other declared key, and only its label and position can change -- which is also how an
// item opening a runtime screen the model cannot name (an inbox, a settings hub) is carried through
// an update unchanged. A new item leaves ID empty and names MachineID; the writer assigns its id.
type GeneratedMenuItem struct {
	ID        string `json:"id,omitempty"`
	Label     string `json:"label"`
	MachineID string `json:"machine_id,omitempty"`
	// List makes the entry a declared page listing this Machine's records instead of the Machine's own
	// generic page (W5-a). Only for a new item: an existing one is never given a list by an update.
	List *GeneratedList `json:"list,omitempty"`
	// ByStatus makes the entry a page of one tile per status, each opening a second page that lists the
	// records in that status (W6). Only for a new item; the writer adds the second page to the menu.
	ByStatus *GeneratedByStatus `json:"by_status,omitempty"`
	// Queue makes the entry the work-queue page of this Machine: figures, then what is open in sections, each
	// row carrying the buttons that move it along (queue.go). Only for a new item.
	Queue *GeneratedQueue `json:"queue,omitempty"`
	// detail marks the second page a ByStatus entry expands into; set by the writer, never by the model.
	detail *GeneratedByStatus
}

// GeneratedByStatus is the status filter: tiles counting the Machine's records per option of StatusField,
// and the page a tile opens (its menu label is DetailLabel). Like a list it only filters what a page shows.
type GeneratedByStatus struct {
	StatusField string `json:"status_field"`
	DetailLabel string `json:"detail_label"`
	// Limit is how many records the detail page shows at most, 1..200.
	Limit int `json:"limit"`
}

// GeneratedList is the list a menu entry shows: which of the Machine's records, how many and in what
// order. "mine" lists the records whose person Field is the viewer; "all" lists every record. Either is a
// filter on what the list shows and not an authorization: the Machine's own page still lists every record
// (who may read which record is not built, so a request for that is a runtime gap, not a scope).
type GeneratedList struct {
	Scope string `json:"scope"`
	// PersonField is a person Field of the Machine; required for "mine", empty for "all".
	PersonField string `json:"person_field,omitempty"`
	// SortField is a Field of the Machine, or created_at / updated_at; empty means created_at.
	SortField string `json:"sort_field,omitempty"`
	// Direction is "asc" or "desc"; empty means desc (newest first).
	Direction string `json:"direction,omitempty"`
	// Limit is how many records the list shows at most, 1..200.
	Limit int `json:"limit"`
	// Columns, when set, makes the page a Table of these Fields, in this order, the first one the link to the
	// record; empty keeps the plain list of titles (D2, plan 2026-10-25). SearchFields and FilterField belong to a
	// table and need Columns.
	Columns []string `json:"columns,omitempty"`
	// SearchFields are text Fields a search box above the table looks in.
	SearchFields []string `json:"search_fields,omitempty"`
	// FilterField is a status Field the table is filtered by, with chips that count each option.
	FilterField string `json:"filter_field,omitempty"`
}

// The scopes of a generated list.
const (
	ListScopeMine = "mine"
	ListScopeAll  = "all"
)

// GeneratedMachine is one Machine the Application exposes. Deliberately narrower than
// domain.Machine: no Datasets, Views, Constraints, CardFields or Sequencing in this first
// increment -- every generated Machine gets the runtime's own implicit default (one table view,
// no aggregate, unrestricted transitions unless declared) rather than the assistant guessing at a
// shape with no forcing case yet. A second real request that needs one of those is this package's
// own trigger to grow it, the same discipline the rest of this codebase holds everywhere else.
type GeneratedMachine struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Fields []GeneratedField `json:"fields"`
	// Completion names the status Field and the option that means a record is done (domain.Completion),
	// which gives the record a mark-done / reopen action. Roadmap stage 1, W2.
	Completion *GeneratedCompletion `json:"completion,omitempty"`
	// CardFields is what a card shows (domain.CardField): which Field plays which role on a board or cards
	// view. Views is how the Machine's records are arranged; the first is the default (W4).
	CardFields []GeneratedCardField `json:"card_fields,omitempty"`
	// StatusCategories assigns one of five closed categories to each option of one status Field
	// (domain.StatusCategories): a status badge is then drawn in its category's tone.
	StatusCategories *GeneratedStatusCategories `json:"status_categories,omitempty"`
	Views            []GeneratedView            `json:"views,omitempty"`
	Permissions      []GeneratedPermission      `json:"permissions"`
	Transitions      []GeneratedTransition      `json:"transitions"`
	Events           []GeneratedEvent           `json:"events"`
	// Ensure bounds a total over this Machine's own records ("stock may not go below zero"): see GeneratedEnsure.
	Ensure []GeneratedEnsure `json:"ensure,omitempty"`
	// FrozenWhen makes a record read-only while the record one of its relation Fields points at holds a status
	// ("a voucher of a closed period"); see GeneratedFrozenWhen.
	FrozenWhen *GeneratedFrozenWhen `json:"frozen_when,omitempty"`
}

// GeneratedFrozenWhen mirrors domain.FrozenWhen: while the record Via (a relation Field of this Machine) points at holds
// Equals in its status Field Field, a record of this Machine is not edited or deleted and none is created under it.
type GeneratedFrozenWhen struct {
	Via    string `json:"via"`
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

// GeneratedEnsure mirrors domain.AggregateBound: the sum of SumOf (a number Field) over the records sharing one value
// of Per, kept at or above AtLeast and/or at or below AtMost. A pointer for each bound, because zero is a real bound.
type GeneratedEnsure struct {
	ID      string   `json:"id"`
	SumOf   string   `json:"sum_of"`
	Per     string   `json:"per"`
	AtLeast *float64 `json:"at_least,omitempty"`
	AtMost  *float64 `json:"at_most,omitempty"`
	Reason  string   `json:"reason,omitempty"`
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
	// Default is the value a new record starts with, always written as text ("5", "true"), as the
	// loader reads it (W2). A status Field's default must be one of its options.
	Default string `json:"default,omitempty"`
	// Stamp makes a person Field filled by the runtime with who created the record: "current_user" (W2).
	Stamp string `json:"stamp,omitempty"`
	// RequiredWhen makes the Field required only while a status Field of the record holds one value ("a banner
	// needs a reference file"); it replaces Required, which must then be false. DefaultFrom fills a date Field on
	// create from a status Field's value plus a number of days ("the priority sets the due date").
	RequiredWhen *GeneratedRequiredWhen `json:"required_when,omitempty"`
	DefaultFrom  *GeneratedDefaultFrom  `json:"default_from,omitempty"`
}

type GeneratedRequiredWhen struct {
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

type GeneratedDefaultFrom struct {
	Field string              `json:"field"`
	Days  []GeneratedDayCount `json:"days"`
}

// GeneratedDayCount is one entry of the days map as a list, because a response schema cannot name free map keys.
type GeneratedDayCount struct {
	Value string `json:"value"`
	Days  int    `json:"days"`
}

// GeneratedView mirrors domain.View for the three arrangements a generated Machine may declare.
type GeneratedView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Type is "board" (one column per option of GroupBy), "table", or "cards" (each record through CardFields).
	Type string `json:"type"`
	// GroupBy is a status Field of this Machine; only for a board.
	GroupBy string `json:"group_by,omitempty"`
}

// The categories a status option may be given (domain.KnownStatusCategories).
var GeneratableStatusCategories = []string{"start", "active", "attention", "done", "stopped"}

// GeneratedStatusCategories mirrors domain.StatusCategories. Values is a list and not a map because the
// output schema has no free-keyed object.
type GeneratedStatusCategories struct {
	Field  string                    `json:"field"`
	Values []GeneratedStatusCategory `json:"values"`
}

// GeneratedStatusCategory is one option and its category.
type GeneratedStatusCategory struct {
	Option   string `json:"option"`
	Category string `json:"category"`
}

// GeneratedCardField mirrors domain.CardField for the five roles that describe a record on a card.
type GeneratedCardField struct {
	Field string `json:"field"`
	Role  string `json:"role"`
}

// The view types and card roles a generated Machine may use. stepper needs a sequencing engine; color, file,
// comment and container serve child Machines (tags, attachments, comments, a parent), which are not generated.
var (
	GeneratableViewTypes = []string{"board", "cards", "table"}
	GeneratableCardRoles = []string{"title", "status", "date", "person", "money"}
)

// GeneratedCompletion mirrors domain.Completion.
type GeneratedCompletion struct {
	Field string `json:"field"`
	Done  string `json:"done"`
}

// GeneratedParentActor mirrors domain.ParentActorGate: the permission is held by the person named in
// Field on the record that this Machine's relation Field Via points at (W2).
type GeneratedParentActor struct {
	Via   string `json:"via"`
	Field string `json:"field"`
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
	// ActorField limits the permission to the person named in this person Field of the record itself
	// ("only the requester edits"); Roles may then be empty (W2).
	ActorField string `json:"actor_field,omitempty"`
	// ParentActor limits it to the person named on a related record ("the owner of the room approves") (W2).
	ParentActor *GeneratedParentActor `json:"parent_actor,omitempty"`
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
	// Roles limits who may take this edge to holders of any one of these Application roles ("only a manager
	// approves"); ActorField limits it to the person named in a person Field of the record ("only the requester
	// reopens"). Both optional; they AND when both are given (a transition's own roles/actor_field).
	Roles      []string `json:"roles,omitempty"`
	ActorField string   `json:"actor_field,omitempty"`
	// Sets are Fields this step writes from the clock when taken ("the completion date is the day it was completed"):
	// each names a date Field and a source, today.
	Sets []GeneratedSet `json:"sets,omitempty"`
	// Balanced holds the step until the records of another machine that point at this one add up (a journal entry is
	// posted only when its lines' debit and credit are equal). Generates makes taking the step create a schedule of
	// records of another machine (disbursing a loan writes its instalments). Both are only on a new machine or added
	// to an existing one, never changed.
	Balanced  *GeneratedBalanced  `json:"balanced,omitempty"`
	Generates *GeneratedGenerates `json:"generates,omitempty"`
}

// GeneratedBalanced mirrors domain.BalanceRule: Of is the machine whose records are totalled, Via a relation field of
// it pointing back at this machine, Debit and Credit two different number fields of it.
type GeneratedBalanced struct {
	Of     string `json:"of"`
	Via    string `json:"via"`
	Debit  string `json:"debit"`
	Credit string `json:"credit"`
	Reason string `json:"reason,omitempty"`
}

// GeneratedGenerates mirrors domain.GenerateSchedule: Count records of Machine, each pointing back through LinkField,
// the i-th due i x DueEveryDays days from the day the step is taken (DueField, a date), each carrying an even share of
// this record's AmountFrom (AmountField, a number); CountField is a number field of this record.
type GeneratedGenerates struct {
	Machine      string `json:"machine"`
	CountField   string `json:"count_field"`
	LinkField    string `json:"link_field"`
	DueField     string `json:"due_field"`
	DueEveryDays int    `json:"due_every_days"`
	AmountField  string `json:"amount_field"`
	AmountFrom   string `json:"amount_from"`
}

// GeneratedSet is one Transition.Sets entry as a list item, because a response schema cannot name free map keys.
type GeneratedSet struct {
	Field  string `json:"field"`
	Source string `json:"source"`
}

// GeneratedEvent mirrors the two Service shapes safe to generate unattended, on creation or on a field
// reaching one value: log_activity (the default, an activity-feed entry) and, since W3 (2026-10-15),
// send_notification (a message to the person named in one of the record's own person fields).
// rollup_parent_status and composite_signed_document are not offered -- they name a parent/child shape
// and a PDF engine that need cross-Machine validation this package does not build.
type GeneratedEvent struct {
	ID         string `json:"id"`
	On         string `json:"on,omitempty"`
	WhenEquals string `json:"when_equals,omitempty"`
	OnCreate   bool   `json:"on_create,omitempty"`
	// Service is "log_activity" (also what an empty value means) or "send_notification".
	Service string `json:"service,omitempty"`
	// Summary is the activity sentence, or the notification's message; {fld_x} placeholders are filled.
	Summary string `json:"summary"`
	// RecipientField and PreferenceKey are send_notification's own: a person Field of this same Machine
	// whose person is told, and which of that person's notification settings also gates the email.
	RecipientField string `json:"recipient_field,omitempty"`
	PreferenceKey  string `json:"preference_key,omitempty"`
	// RecipientRole tells everyone holding one of this Application's roles instead of one person ("every
	// coordinator is told"); RecipientGroupField tells the members of the Group a group Field of the record holds.
	// Exactly one of the three recipients is named.
	RecipientRole       string `json:"recipient_role,omitempty"`
	RecipientGroupField string `json:"recipient_group_field,omitempty"`
}

// The Services a generated Event may name.
const (
	EventServiceLogActivity      = "log_activity"
	EventServiceSendNotification = "send_notification"
)

// EventService is the Service an event names, with "" read as the default.
func (e GeneratedEvent) EventService() string {
	if e.Service == "" {
		return EventServiceLogActivity
	}
	return e.Service
}

// GeneratedQueue is the closed queue-page recipe. The model picks it and names at most a date Field and some
// words; the Datasets, count measures, Metrics, Collections and transition Buttons are derived from the Machine's
// status_categories, completion, transitions and card_fields (queue.go).
type GeneratedQueue struct {
	// DateField, when set, makes a dated queue (overdue, due today, next 7 days, later): a date Field the
	// Machine projects with a card_fields entry of role date, and the Machine must declare completion.
	// Left empty, the sections are the Machine's open statuses (start, active, attention categories).
	DateField string `json:"date_field,omitempty"`
	// Limit is how many records each section shows at most, 1..200.
	Limit int `json:"limit"`
	// Words are the headings and labels, in the person's own language; any left empty reads in English.
	Words GeneratedQueueWords `json:"words,omitempty"`
}

type GeneratedQueueWords struct {
	Overdue      string `json:"overdue,omitempty"`
	DueToday     string `json:"due_today,omitempty"`
	NextWeek     string `json:"next_week,omitempty"`
	Later        string `json:"later,omitempty"`
	RecentlyDone string `json:"recently_done,omitempty"`
	Empty        string `json:"empty,omitempty"`
	All          string `json:"all,omitempty"`
	Add          string `json:"add,omitempty"`
}
