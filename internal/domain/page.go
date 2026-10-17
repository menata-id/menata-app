package domain

import "net/url"

// PageNode is one node of a `page:` declaration: what an Application writes in a navigation item to say what
// a screen's body is made of (007 §12.4, §15.1's first stage).
//
// **It is the declaration, not the UI IR.** `internal/ir.UINode` carries resolved properties and no Binding;
// a PageNode carries the author's words, including a Binding that names a Dataset. `ir.Lower` turns one into
// the other, and it lives in `domain` rather than `ir` because `internal/ir` imports `domain` and the reverse
// would be a cycle (001 #17: YAML is a declaration, not the compiler's intermediate form).
//
// Kind is `layout`, `static` or `component` -- the three planes a Page composes from -- and Type is the
// closed vocabulary that Kind owns (`LayoutKind`, `StaticKind`, `ComponentType`). Props are every other
// scalar key on the node, stringified; **which keys a Type accepts is not decided here** but by
// `ir.Validate`'s allowed-property table and the Component's own registered validator, so this type stays
// vocabulary-free and a new Layout property is one table entry rather than a parser change.
type PageNode struct {
	Kind    string
	Type    string
	Props   map[string]string
	Binding *PageBinding
	// From maps a property of this node to the Projection role (`card_fields`, 007 §7.6) it takes from the
	// record the enclosing records-bound Collection is drawing: `{text: title}` means "this node's text is
	// that record's title". Valid only inside such a Collection's item template -- `ir.Lower` refuses it
	// anywhere else, and the property it names may not also be written on the node.
	From map[string]string
	// To is the navigation item a `static: link` points at (`to: nav_approval_inbox`). A link's destination is
	// never a typed route (001 #3, #8): `ir.Lower` resolves it from the Application's own `navigation:`, so
	// renaming a route in one place moves every page that links to it. Valid only on a `link`, which in turn
	// has no other way to name a destination.
	To string
	// ListOf is the Dataset whose Machine a `static: link` opens the generic list page of
	// (`list_of: ds_recent_documents` -> `MachineListRoute(<that Dataset's source>)`). It is the second way a
	// link names a destination, for the screens that have no navigation item: a Machine claimed by no
	// Application's `navigation:` still has a page, and a typed `/machines/<id>` would be a route retyped
	// (001 #3, #8). The Dataset is the declared handle on the Machine, the way a Binding's is, so the same
	// load-time check (it exists in this Workspace) applies. Valid only on a `link`, and never beside `to:`.
	ListOf string
	// Count is the words a `static` node says about how many children of the record a declared Relation
	// (007 §7.5) found: `count: {of: rel_label_cards, one: "used on {n} card", other: "used on {n} cards"}`.
	// The number is never written by the author -- it comes from the Relation the bound Dataset declares --
	// and neither is the node's `text`, which this produces. Valid only on a `static` node inside a
	// records-bound Collection's item template; `ir.Lower` refuses it anywhere else.
	Count *PageCount
	// HintFigure is a Metric's hint worded around another Measure's figure of the same Dataset
	// (`hint_figure: {of: msr_projects, one: "In {n} project", other: "In {n} projects"}`): the number comes from
	// the binding, never typed, for the reason a Metric's value never is. Valid only on a Metric bound with
	// `rows: total`, and instead of a written `hint:`.
	HintFigure *PageCount
	// Param is the query-string parameter a dimension-bound Metric carries its row's Dimension value in when it
	// links (`to: nav_documents_in_status`, `param: status` -> `/pages/nav_documents_in_status?status=<value>`).
	// It is the other end of a destination's `$parameters.<name>`: the page that reads the parameter declares
	// the name in its Dataset, this names it again only where a value is sent, and the loader checks the two
	// agree. Valid only beside `to:` on a Metric bound with rows: dimension; `ir.Lower` refuses it anywhere else.
	Param string
	// When makes the node appear only for the records it holds for (007 §15.2, "conditional visibility"):
	// `when: {role: status, is_not: $done}`. It is a statement about the record's own Projection role, so it
	// is valid only inside a records-bound Collection's item template, and a node it hides is not lowered at
	// all -- its children are not either, which is what makes it a visibility rule and not a CSS class.
	When *PageCondition
	// Each makes the node repeat once for every item of a list the current record owns (`each: tags`): the chips
	// a Machine's `card_tags:` (007 §7.6) attaches to it. The copy takes its `from:` out of that item, whose roles
	// are `title` and `color` and nothing else, and a record with none draws none. Valid only directly in a
	// records-bound Collection's item template over a Machine that declares `card_tags:`; the one value is
	// PageEachTags, a closed set, because a second list would be a second case to measure first.
	Each string
	// Step is the other end of a request-relative parameter a link sends: `step: {parameter: week, by: 1}` on a
	// `static: link` with `to:` points at that destination one week on from the week the request is looking at.
	// The value is never written -- it depends on the request, which a page cannot know -- so `ir.Lower` asks the
	// Resolver for it. Valid only on a link that names its destination with `to:`.
	Step *PageStep
	// Window is the Dataset whose date window a `static` node says in words (`window: ds_tasks_due_week` -> "12 --
	// 18 Oct 2026"): the very bounds a `group: date` Collection over it draws its days from, so the label above a
	// list and the days in it cannot disagree. The node's text is produced, never written. Valid only on a
	// `static` node.
	Window string
	// Columns are a Table's columns, in order (`columns: [{header: Title, from: title, href: record}]`): each names the
	// Projection role its cell shows. Valid only on a `Table` bound with rows: records; `ir.Lower` builds the cells, so a
	// Table holds no children of its own.
	Columns  []PageColumn
	Children []PageNode
}

// PageColumn is one column of a declared Table. Header is the words in the column's header cell, Align the edge its
// text sits on (empty is the start), and Href, when it is PageRecordRole, makes the cell a link to the record's own
// page. What the cell shows is exactly one of two things:
//
//   - From, a Projection role (`card_fields`, 007 §7.6) or a derived role (`created:label`, `date:label`): what the
//     Machine already says about a record's shape, one Field per role;
//   - Field, one Field of the Dataset's Machine by id: a list shows as many columns as the screen needs, and a
//     Machine has one `title` role but a contract list shows its number, its party and its category side by side
//     (mockup K01b; gap U-02). The cell is the Field's value as the record's own page shows it -- a figure grouped, a
//     date as a day, a relation as the related record's label, a status as a badge in the tone its category gives.
type PageColumn struct {
	Header string
	From   string
	Field  string
	Align  string
	Href   string
}

// PageStep is a link's request-relative parameter: Parameter is the one the runtime names (PageParameterWeek) and
// By the whole number of steps from the request's own value, never zero (a link to where you already are is a
// link to nothing).
type PageStep struct {
	Parameter string
	By        int
}

// PageParameterWeek names the query parameter a calendar steps through and a page may test. It repeats
// `expression.WeekParameter`, which `internal/domain` may not import; `TestPageParameterWeekIsTheExpressionOne`
// holds the two equal.
const PageParameterWeek = "week"

// PageEachTags is the one list `each:` can walk: a record's tags, as its Machine's `card_tags:` declares them.
const PageEachTags = "tags"

// PageCondition is a record-scoped test on a Projection role: Role is a `card_fields` role, and exactly one of Is
// and IsNot is set. The operand is a literal value or one of the two sentinels a transition's `becomes:` already
// uses (PageTransitionDone, PageTransitionReopen), resolved from the Machine's `completion:` -- so a page says "not
// done" without retyping the word the Machine already declares (001 #8). No grammar beyond that (007 §9.2): one
// role, one comparison, no `and`/`or`.
type PageCondition struct {
	Role string
	// Parameter is the second kind of condition: a statement about the request instead of a record
	// (`when: {parameter: week, is_not: "0"}`), valid anywhere on a page. Exactly one of Role and Parameter is set.
	// The only parameter a page may test is PageParameterWeek, compared as the runtime reads it -- a whole number,
	// "0" for this week whatever was sent -- so a page says "not this week" without caring how it was spelt.
	Parameter string
	// Viewer is the third kind: a statement about who is looking (`when: {viewer: workspace_role, is: admin}`), valid
	// anywhere on a page. Exactly one of Role, Parameter and Viewer is set. The only thing a page may ask of a viewer is
	// PageViewerWorkspaceRole, and only with Is, never IsNot: a viewer the runtime cannot place (no membership row) holds
	// no role, and `is_not: member` would have shown such a viewer what only an admin should see. `is` fails closed.
	Viewer string
	Is     string
	IsNot  string
}

// PageViewerWorkspaceRole is the one fact about the viewer a page may test: their role in the Workspace, "admin" or
// "member". An Application role is not offered: a Workspace page has no Application to hold one in, and the first
// consumer needs only this.
const PageViewerWorkspaceRole = "workspace_role"

// KnownPageViewerWorkspaceRoles is the closed set of values `is:` may take for PageViewerWorkspaceRole.
var KnownPageViewerWorkspaceRoles = map[string]bool{WorkspaceRoleAdmin: true, WorkspaceRoleMember: true}

// PageCount is a node's plural text over a Relation's child count. Of is a Relation id of the Dataset the
// enclosing Collection is bound to; Other is required and is the text for every n but 1 (zero included, since
// "used on 0 cards" is the English plural and a count that vanishes at zero makes the page's shape depend on
// the data); One is optional and, when absent, Other serves n == 1 too. `{n}` is the one substitution -- a
// number, never an expression (007 §9.2). Two forms are what English and Indonesian need; a language with
// more plural categories is a capability nobody has asked for.
type PageCount struct {
	Of    string
	One   string
	Other string
}

// PageCountToken is the single substitution a PageCount text may carry.
const PageCountToken = "{n}"

// PageCountRole is the key under which a record's child count for one Relation travels in the record map a
// records Binding hands to `ir.Lower`. Like PageRecordRole it is reserved: the colon cannot appear in a
// Projection role, so a count is never mistaken for a `card_fields` role and a role never for a count.
func PageCountRole(relationID string) string {
	return "count:" + relationID
}

// MachineListRoute is the runtime's generic page for a Machine (`GET /machines/{machineID}`), the list of its
// records and where one is created. It belongs to the runtime the way `/home` does, not to an Application, so
// it is assembled in one place and not read from `navigation:`; the page it names still answers for itself
// who may see it, since a link is an address and not a grant.
func MachineListRoute(machineID string) string {
	return "/machines/" + machineID
}

// PageBinding is 007 §11.3's read side in its narrowest honest form: a node takes its content from one
// Dataset's aggregate. No expression, no path, no second Dataset -- §9.2's allowed-context rule, applied by
// not having a grammar to misuse.
//
// Rows is the mode, and there are two. `dimension` expands the bound node into **one node per value of the
// Dataset's Dimension**, each labelled by that value and carrying that group's Measure; it needs `measure`.
// `records` expands a Collection into **one item per record of a `select: records` Dataset**, each item the
// Collection's single template child with its `from:` properties filled from the record's Projection roles;
// it takes no `measure`. A scalar mode (`{dataset, measure}` -> one number) was the first design and was
// dropped on measurement: every aggregate Dataset in the template library declares a Dimension, so a scalar
// Binding had nothing to bind (menata-app-document audits/2026-10-07-rencana-page-yaml..., §9.6).
type PageBinding struct {
	Dataset string
	Measure string
	Rows    string
	// Write is the write-side mode (007 §11.3): `create` binds a `Form` to the Dataset's Machine, so that
	// submitting it creates a record through the generic create route; `update` binds one inside a records
	// template to the item's own record, through the generic patch route. It is a mode of its own and never
	// beside Rows or Measure, which read.
	Write string
	// Group splits a records binding's items into the buckets of one Machine role, each drawn once and in order,
	// the empty ones too (007 §24: "calendar = collection + date dimension", "board = collection + group
	// dimension"). The only value built is PageGroupDate. It is a property of the *binding* because it decides
	// what the Data Plane hands back -- which buckets exist, and which record is in which -- and not how the
	// Collection looks; the page declares what a bucket and a record look like as its two child templates.
	Group string
	// All, with `write: transition`, makes the Button act on every record of Dataset the viewer may move rather than on
	// the record of the Collection it sits in: a "mark all as read". It is the one transition binding that names a
	// Dataset and sits outside a template. Nothing is written except through the same guards as moving each record
	// alone (`edit` Permission, declared edges, write guards, Events); a record the viewer may not move is left as it is.
	All bool
	// Action, with `write: action`, names one of registry.RuntimeActions: what a Form or Button of the runtime's own pages
	// performs. Nothing else about the request is written on the page.
	Action string
	// Fields is never written: it is the Fields a Table's `columns:` name by id (PageColumn.Field), copied here at load
	// so the Data Plane resolves those Fields -- and a relation among them, in one statement by the ids listed -- and
	// no other. A binding with none resolves only the Projection, as before.
	Fields []string
	// CountCut is never written either: a Table that says `complete: true` asks, when its Dataset's bound cuts it, how
	// many records it was cut from. Set at load, so a list that claims nothing pays no count.
	CountCut bool
}

// PageGroupDate buckets a records binding by the Machine's `date` card_fields role, one bucket per day of the
// window the Dataset's own `where:` closes on that Field (a `gte` and a `lte`, which is what a week or a month is).
// The window is *derived* from the Dataset and never written beside it, so the days a page draws and the records
// it selects cannot disagree (001 #8). PageGroupMaxBuckets bounds it: a window of years is a mistake, not a page.
const (
	PageGroupDate       = "date"
	PageGroupMaxBuckets = 62
)

// PageBucketLabelRole is the one `from:` role a bucket's heading template has: the bucket's own name, derived
// ("Mon 05 Oct" for a day). It exists only in the heading template -- a record's template has no such role, so
// reading it there is a lowering error rather than a blank -- and is reserved by its colon, as the other derived
// roles are, so it can never be a Projection role.
const PageBucketLabelRole = "bucket:label"

// The Rows modes. `dimension` is a Metric per Dimension value (or, on a Collection, one item per Dimension value,
// its template reading the value and every Measure through PageRowLabelRole/PageMeasureRole), `records` a
// Collection item per record, and
// `total` one Metric holding a Measure's figure over the whole Dataset -- the mode for a number that has no
// breakdown, whose label the author writes because there is no Dimension value to name it (2026-10-10).
const (
	PageRowsDimension = "dimension"
	PageRowsRecords   = "records"
	PageRowsTotal     = "total"
)

// The write modes. `create` binds a Form to a Dataset's Machine. `update` is valid only inside the item template
// of a records Collection, where the record it changes is the item's own and the Dataset is the Collection's, so
// a binding there names no dataset of its own (a second one would be a join the page has no grammar for).
// `delete` acts on a record in the same place and on the same terms, through a `Button` and not a `Form`: there is
// nothing to ask, only a consequence to confirm. `move` is the same place and terms again, a `Button` that sends the
// record one step up or down in its Machine's order (`direction: up|down`); it needs no confirmation because it is
// undone by the opposite button.
const (
	PageWriteCreate = "create"
	PageWriteUpdate = "update"
	PageWriteDelete = "delete"
	PageWriteMove   = "move"
	// PageWriteTransition is a `Button` that moves the record's status Field (the Machine's `status` Projection role)
	// to the value `becomes:` names, through the same generic patch route as an edit, so the state model's declared
	// edges (`CheckTransitions`) and the `edit` Permission are its guards. Drawn only on a record for which that
	// move is allowed, which is also why a "Reopen" and a "Mark done" Button can sit side by side: one shows.
	PageWriteTransition = "transition"
	// PageWriteAction is a Form or a Button that performs one of the runtime's named operations (registry.RuntimeActions)
	// instead of writing a record of a Machine. Only the runtime's own pages may use it.
	PageWriteAction = "action"
)

// The two words a transition Button's `becomes:` may use instead of a status option, both read from the Machine's own
// `completion:` block so "finished" is declared once there and never retyped in a page: `$done` is the finished
// value, `$reopen` is what finishing's opposite writes (`Machine.ReopenValue`).
const (
	PageTransitionDone   = "$done"
	PageTransitionReopen = "$reopen"
)

// WritableComponents maps each registered Component a Binding may write through to the Write modes it takes, the
// way BindableComponents does for reading. Vocabulary, so it lives here and not in `internal/registry`.
var WritableComponents = map[ComponentType][]string{
	ComponentForm:   {PageWriteCreate, PageWriteUpdate, PageWriteAction},
	ComponentButton: {PageWriteDelete, PageWriteMove, PageWriteTransition, PageWriteAction},
}

// PageRecordRole is the one `from:` role that is not a Projection role: `from: {href: record}` on a
// `static: link` inside a records template takes the record's own generic route
// (`/machines/<Machine>/records/<id>`, which the runtime owns the way it owns `/home`). It is a reserved word
// rather than a `card_fields` role because a role describes what a Machine says about a record, and every
// Machine would otherwise have to declare a "link" to be listed on a page; and it is valid for nothing but a
// link's `href`, so a record's route is never shown as text or handed to another property.
const PageRecordRole = "record"

// PageDateLabelRole and PageDateToneRole are the two `from:` roles that are *derived* from a Machine's `date`
// card_fields role instead of projected from it: the short date a pill shows ("2 Jan"), and the tone it takes
// (neutral while the date is ahead, bad once it has passed, good once the record is complete -- the Machine's own
// `completion:`). A page reads them to draw a `StatusBadge` without any of that rule being retyped in YAML or
// worked out in a renderer (007 §4.4), and the clock is the request's, injected, never read (§4.6). The colon
// keeps them out of every Projection role, as it does for PageCountRole.
//
// A record with no date holds both as the empty string, and a node that reads one of them for such a record is not
// drawn -- a pill without a date is a defect, not a design. That omission belongs to the derived roles
// (PageDerivedRole) and nothing else: any other empty role is drawn as an empty string.
const (
	PageDateLabelRole = "date:label"
	PageDateToneRole  = "date:tone"
)

// PageStatusToneRole is the tone of a record's `status` card_fields role, derived from the Machine's
// `status_categories:` (Machine.StatusTone): `from: {label: status, tone: "status:tone"}` draws the badge every other
// screen draws for that option. Empty when the record has no status, and a node reading it is then not drawn.
const PageStatusToneRole = "status:tone"

// PageFieldRole is the key of one Field's display in a record row, for a Table column that names the Field
// (PageColumn.Field), and PageFieldToneRole the key of its tone when the Field is a status. Reserved by their colon,
// as the other derived roles are, so they can never be a Projection role.
func PageFieldRole(fieldID string) string { return "field:" + fieldID }

// PageFieldToneRole: see PageFieldRole.
func PageFieldToneRole(fieldID string) string { return "tone:" + fieldID }

// PageRowLabelRole and PageMeasureRole are the keys of the row map a Collection bound with `rows: dimension` hands
// to its item template: the Dimension value's display (a status option, or a related record's title), and each of
// the Dataset's Measures for that value, already formatted. Reserved like the count role -- the colon cannot appear
// in a Projection role -- and they are the only keys such a row has: a Dimension row is a group of records, not a
// record, so it has no route, no Projection and nothing a write could act on.
const PageRowLabelRole = "row:label"

// PageMeasureRole is the key of one Measure's figure in a Dimension row (see PageRowLabelRole).
func PageMeasureRole(measureID string) string {
	return "measure:" + measureID
}

// PageGroupLabelRole is the third derived role: the name of the group the record sits in under its Machine's default
// View -- the list a Task is in, for a board grouped by a relation, or the option itself for one grouped by a Field
// of options. Like the date roles it is empty when the record has no group, and a node reading it is then not drawn.
const PageGroupLabelRole = "group:label"

// PageDerivedRole reports whether role is one of the derived roles above, each of which leaves a node undrawn when
// the record has nothing to say for it.
func PageDerivedRole(role string) bool {
	return role == PageDateLabelRole || role == PageDateToneRole || role == PageGroupLabelRole || role == PageCreatedLabelRole || role == PageStatusToneRole
}

// PageCreatedLabelRole is the derived role that says when a record was created ("2 Jan 2006 15:04"), which every record
// has and no Machine declares as a Field. It needs nothing of the Machine, so it is valid on any records Collection or
// Table.
const PageCreatedLabelRole = "created:label"

// BindableComponents maps each registered Component a Binding may supply to the Rows modes it takes. A
// Binding on any other node, or in a mode not listed, is a load error rather than a property the renderer
// ignores.
//
// Vocabulary (a closed map of strings), so it lives here and not in `internal/registry`
// (`conformance.TestDomainHoldsVocabularyAndRegistryHoldsDispatch`). `Metric` takes dimension rows or one
// total: it is the Component whose contract is "a label and a resolved number". `Collection` takes records or
// dimension rows: it is the one with an `item` slot, so a list of already-composed items is exactly what a record
// set is, and what a Dimension's groups are when each needs more than one figure (a bar of open out of all).
var BindableComponents = map[ComponentType][]string{
	ComponentFilterChips:  {PageRowsDimension},
	ComponentFilterSelect: {PageRowsDimension},
	ComponentMetric:       {PageRowsDimension, PageRowsTotal},
	ComponentCollection:   {PageRowsRecords, PageRowsDimension},
	ComponentTable:        {PageRowsRecords},
}

// TransitionAllRoute is the runtime's route for a `write: transition` Binding with `all: true`: the Machine, the Dataset
// whose records it acts on, and the status they move to. The target is already resolved (`$done` is not sent): the route
// checks it is an option of the Machine's status Field and re-selects the Dataset itself, so the page names no record.
func TransitionAllRoute(machineID, datasetID, to string) string {
	return "/machines/" + machineID + "/transition-all?" + url.Values{"dataset": {datasetID}, "to": {to}}.Encode()
}

// PageRecordIDRole is the reserved `from:` key under which a record's own id travels in the map a records Binding hands
// to `ir.Lower`. A page never reads it as text; lowering spends it on the route of an action about that record.
const PageRecordIDRole = "record:id"

// RuntimeActionRoute is the runtime's route for one named operation, with the record it is about when there is one.
func RuntimeActionRoute(actionID, recordID string) string {
	route := "/runtime-actions/" + actionID
	if recordID != "" {
		route += "?" + url.Values{"record": {recordID}}.Encode()
	}
	return route
}
