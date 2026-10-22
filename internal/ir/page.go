package ir

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"menata.app/internal/domain"
)

// Row is one resolved dimension row of a Binding: the Dimension's value and the Measure's number, already
// formatted. The resolver decides ordering and formatting; Lower only places them.
//
// Key is the Dimension's stored value when it is not what the row is called: a reference Field stores an id and
// the row shows the record's title, and a link that carried the title would send a destination something it
// cannot compare against. Empty means the label is the value.
type Row struct {
	Label string
	Key   string
	Value string
	// Measures is every Measure of the Dataset for this Dimension value, formatted, keyed by Measure id. A Metric
	// reads Value (its one bound Measure); a Collection bound with `rows: dimension` hands all of them to its item
	// template under domain.PageMeasureRole.
	Measures map[string]string
	// Numbers is every Measure as a bare number, what a fraction's bar is drawn from.
	Numbers map[string]string
	// Attrs is, for a Dimension over a relation, the related record's own Fields as a reader reads them (a category's
	// type), keyed by Field id.
	Attrs map[string]string
}

func (r Row) linkValue() string {
	if r.Key != "" {
		return r.Key
	}
	return r.Label
}

// RowResolver answers a dimension Binding. It is injected, not imported, for the reason `knownComponents` is:
// this package is a representation (§15), not a data reader, and `conformance` forbids it `internal/data`.
// The loader passes a placeholder resolver to check a tree's *shape* without a database; the handler passes
// one that reads the Dataset.
type RowResolver func(b domain.PageBinding) ([]Row, error)

// RecordResolver answers a records Binding: one map per record, keyed by Projection role (`title`, `status`,
// `date`...) and holding that role's display string, already resolved and formatted. Lower places those
// strings and decides nothing about them; **a role the map lacks is a fault of the declaration** (it asked for
// something the Dataset's Machine does not project), not an empty string.
type RecordResolver func(b domain.PageBinding) (RecordSet, error)

// RecordSet is a records Binding's answer: the records, and whether the Dataset's own `limit:` cut them short.
// Truncated and Limit travel with the records because they are one read's two facts -- asking for the bound
// separately would be a second read, or a second opinion about the first (001 #8).
//
// Edits is the answer to an `update` Form inside the item template, one per record and in the same order: where
// that record is patched, whether this viewer may edit *it* (a Permission may read the record's own Field, so the
// answer differs per record), and the controls starting as its current values. It is empty when the resolver has
// nothing to say, which makes an update Form in the template an error and not a form posting nowhere.
//
// Deletes is the same for a `delete` Button: one per record, in order. Moves is the same for a `move` Button, and
// Transitions for a `transition` Button.
type RecordSet struct {
	Records     []map[string]string
	Edits       []FormSpec
	Deletes     []RecordAction
	Moves       []RecordMove
	Transitions []RecordTransition
	// Tags is each record's tag list for an `each: tags` node, in the same order: per tag, its `title` and
	// `color` roles. Empty when the resolver has none to offer, which makes an `each:` an error and not a chip
	// that is silently never drawn.
	Tags [][]map[string]string
	// Done and Reopen are the Machine's `completion:` values, what the `$done`/`$reopen` of a `when:` mean ("" when
	// it declares none). ShapeOnly marks the load-time placeholder, for which every `when:` holds.
	Done, Reopen string
	ShapeOnly    bool
	Truncated    bool
	Limit        int
	// Total is how many records the Dataset matches without its bound, read only when Truncated: what a cut list says
	// it was cut from. 0 when unknown.
	Total int
	// Buckets and BucketKeys serve a binding with `group:` (domain.PageGroupDate): the buckets in the order they
	// are drawn, empty ones included, and for each record the Key of the bucket it sits in. A record whose key
	// matches no bucket is a defect in the resolver -- the window comes from the Dataset's own `where:` -- and
	// lowering refuses it rather than drawing a page that quietly lacks a task.
	Buckets    []Bucket
	BucketKeys []string
	// Drags is, for a binding with `moves:`, each record's answer in the same order: the route a drop writes and the Field it writes, and whether
	// this viewer may. Courtesy, like every record-level answer here -- the patch route asks again.
	Drags []RecordDrag
	// DropField is the Field a drop onto a bucket writes (the date Field the buckets are days of); "" when the binding declares no `moves:`.
	DropField string
}

// RecordDrag is the answer for a card that can be moved to another bucket: the record's generic patch route, the date Field a drop writes, and
// whether this viewer may write it (the `edit` Permission, a frozen parent, the state model). Not Permitted draws the card still.
type RecordDrag struct {
	Route, Field string
	Permitted    bool
}

// Bucket is one group of a grouped records binding: Key is what records name it by, Label what its heading says.
type Bucket struct {
	Key, Label string
	// Day, Date and Number are the parts of Label ("Mon", "05 Oct", "5") a heading may set apart.
	Day, Date, Number string
	// Outside marks a day added only to make a whole week (domain.PageFillWeeks): it holds nothing and is drawn quiet.
	Outside bool
	// Current marks the bucket that holds the request's own moment (today's day), which lowering names on the
	// section it draws so a renderer can set it apart. The resolver decides it from the request's date; a bucket
	// is never current by what its heading says.
	Current bool
}

// RecordTransition is the answer for a `transition` Button: the record's route, the status Field a transition moves
// (the Machine's `status` Projection role), and which target values are worth offering this viewer for *this*
// record. Allows answers for one target: known=false when it is no option of that Field, and otherwise whether
// moving this record there is a change the state model and the `edit` Permission would let through -- so a target
// equal to the current value is not allowed, and a Button for it is not drawn. It is a function over data the
// resolver already holds (the way a RouteResolver is) so a load-time placeholder can answer for any target.
// Done and Reopen are the Machine's `completion:` read for the two sentinels, "" when it declares none.
// Courtesy, as the others: the patch route asks again.
type RecordTransition struct {
	Route  string
	Field  string
	Done   string
	Reopen string
	// Complete is whether the record stands at the Machine's `completion:` value now: the state a Toggle draws, and so which of
	// Done and Reopen pressing it writes.
	Complete bool
	Allows   func(target string) (allowed, known bool)
	// Edges are the Machine's declared transitions by id, each with whether this viewer may take it from where this
	// record is now: what a Button naming its edge (`binding: {write: transition, transition: trn_x}`) moves, on any
	// status Field of the Machine.
	Edges map[string]EdgeOffer
}

// EdgeOffer is one declared transition as a record offers it: its name, the status Field it moves and to what, and
// whether this viewer may take it from the record's current value.
type EdgeOffer struct {
	Name, Field, To string
	Allowed         bool
	// Blocked is the declared reason a Constraint holds this edge back for this record (domain.PageBlockedRole), "" when
	// nothing does or the Constraint says none.
	Blocked string
	// Inputs are what the edge asks for (domain.Machine.TransitionInputs): a Form that names the edge draws them.
	Inputs []domain.FormInput
}

// RecordAction is the answer to a record-level write that asks nothing -- a delete: where the request goes and
// whether this viewer may make it. Like FormSpec it is plain data, and `Permitted` is courtesy: the route and
// `action.CanDelete` are the guards.
type RecordAction struct {
	Route     string
	Permitted bool
}

// RecordMove is the answer for a `move` Button: the record's route to move, and which of the two directions are
// worth offering this viewer for *this* record. The first record has no "up" and the last no "down" -- a button
// that can only do nothing is not drawn. Like RecordAction it is courtesy; the route is the guard.
type RecordMove struct {
	Route string
	Up    bool
	Down  bool
}

// RecordWrites is what a records Collection's resolver decided about one record's writes, handed down to the
// nodes of that record's item template. A nil field means the resolver said nothing, which makes the matching
// binding an error and not a control wired to nowhere.
type RecordWrites struct {
	Edit   *FormSpec
	Delete *RecordAction
	Move   *RecordMove
	// Transition is the status move this record's resolver offered.
	Transition *RecordTransition
	// Tags is this record's tag list (RecordSet.Tags), and nil when the resolver offered none. InEach marks the
	// writes handed to the copies an `each:` draws, which hold no write of their own: a Button there would act on
	// the parent record under a tag's name, and a second `each:` has nothing to walk.
	Tags   []map[string]string
	InEach bool
	// Done, Reopen and ShapeOnly are the RecordSet's, shared by every record: what a `when:` sentinel means.
	Done, Reopen string
	ShapeOnly    bool
}

// conditionHolds answers a `when:` for one record. The role must be one the bound Machine projects (the same
// refusal `from:` makes), a sentinel resolves from the Machine's `completion:`, and a shape-only set (the
// load-time placeholder) answers true for every condition, so a node a real record would hide is still
// validated -- otherwise the placeholder's one record would hide it from the load check for good.
func conditionHolds(c domain.PageCondition, record map[string]string, writes *RecordWrites, path string) (bool, error) {
	if record == nil {
		return false, fmt.Errorf("%s: when: is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageRowsRecords)
	}
	if (c.Is == "") == (c.IsNot == "") {
		return false, fmt.Errorf("%s: when: takes exactly one of is: and is_not:", path)
	}
	have, ok := record[c.Role]
	if !ok {
		return false, fmt.Errorf("%s: when: names the role %q, which the bound Dataset's Machine does not project", path, c.Role)
	}
	operand, negate := c.Is, false
	if c.IsNot != "" {
		operand, negate = c.IsNot, true
	}
	switch operand {
	case domain.PageTransitionDone:
		if writes != nil {
			operand = writes.Done
		}
	case domain.PageTransitionReopen:
		if writes != nil {
			operand = writes.Reopen
		}
	}
	if operand == "" || operand == domain.PageTransitionDone || operand == domain.PageTransitionReopen {
		return false, fmt.Errorf("%s: when: %s needs the Machine to declare a `completion:`, and this one declares none", path, operand)
	}
	if writes != nil && writes.ShapeOnly {
		return true, nil
	}
	// A Field read by id that holds a figure compares as one: "Rp 0" is 0, and `is: "0"` means it.
	if id, byField := domain.PageFieldOfRole(c.Role); byField {
		if n, ok := record[domain.PageRowNumberRole(id)]; ok {
			a, errA := strconv.ParseFloat(n, 64)
			b, errB := strconv.ParseFloat(operand, 64)
			if errA == nil && errB == nil {
				return (a == b) != negate, nil
			}
		}
	}
	return (have == operand) != negate, nil
}

// parameterHolds answers a `when: {parameter: ...}`. It is a statement about the request, so it needs no record and
// is valid anywhere; the only parameter a page may test is the week one, and it is compared as the runtime reads it.
func parameterHolds(c domain.PageCondition, r Resolver, path string) (bool, error) {
	if c.Role != "" {
		return false, fmt.Errorf("%s: when: tests a record's role or a request parameter, not both", path)
	}
	if (c.Is == "") == (c.IsNot == "") {
		return false, fmt.Errorf("%s: when: takes exactly one of is: and is_not:", path)
	}
	if !domain.KnownPageParameters[c.Parameter] {
		return false, fmt.Errorf("%s: when: parameter: %q is not one a page may test (%s, %s, %s)", path, c.Parameter, domain.PageParameterWeek, domain.PageParameterMonth, domain.PageParameterRange)
	}
	operand, negate := c.Is, false
	if c.IsNot != "" {
		operand, negate = c.IsNot, true
	}
	if c.Parameter == domain.PageParameterRange {
		if !domain.KnownPageRanges[operand] {
			return false, fmt.Errorf("%s: when: range is compared with %s or %s, and %q is neither -- it would never (or always) hold", path, domain.PageRangeWeek, domain.PageRangeMonth, operand)
		}
	} else {
		offset, err := strconv.Atoi(operand)
		if err != nil {
			return false, fmt.Errorf("%s: when: %s is compared as a whole number, and %q is not one -- it would never (or always) hold", path, c.Parameter, operand)
		}
		operand = strconv.Itoa(offset)
	}
	if r.Parameter == nil {
		return true, nil
	}
	return (r.Parameter(c.Parameter) == operand) != negate, nil
}

// viewerHolds answers a `when: {viewer: workspace_role, is: admin}`. It is a statement about the request's viewer, so it
// needs no record and is valid anywhere. A node it hides is not lowered at all, so the Dataset it binds is not read for a
// viewer who would not see it. A viewer the resolver cannot place gets "", which equals no declared role: the condition
// fails closed. Only `is:` exists, for that reason.
func viewerHolds(c domain.PageCondition, r Resolver, path string) (bool, error) {
	switch {
	case c.Role != "" || c.Parameter != "":
		return false, fmt.Errorf("%s: when: tests a record's role, a request parameter or the viewer, not more than one", path)
	case c.Viewer != domain.PageViewerWorkspaceRole:
		return false, fmt.Errorf("%s: when: viewer: %q is not a fact a page may test (%s)", path, c.Viewer, domain.PageViewerWorkspaceRole)
	case c.IsNot != "" || c.Is == "":
		return false, fmt.Errorf("%s: when: viewer: takes is: and never is_not: -- a viewer the runtime cannot place holds no role, so a negation would show them what only a role should", path)
	case !domain.KnownPageViewerWorkspaceRoles[c.Is]:
		return false, fmt.Errorf("%s: when: viewer: %s is %q, which is not a Workspace role (admin or member)", path, c.Viewer, c.Is)
	}
	if r.Viewer == nil {
		return true, nil
	}
	return r.Viewer(c.Viewer) == c.Is, nil
}

// lowerWindow is `window:`: a static node whose text is the date window of a Dataset, in words. The text is the
// resolver's, so a page that also wrote one (or took it from a record, or counted into it) would be saying it twice.
func lowerWindow(n domain.PageNode, props map[string]string, r Resolver, path string) (map[string]string, error) {
	if n.Window == "" {
		return props, nil
	}
	if NodeKind(n.Kind) != NodeStatic {
		return nil, fmt.Errorf("%s: window: produces text, so it is valid only on a static node", path)
	}
	if _, written := props["text"]; written || n.Count != nil {
		return nil, fmt.Errorf("%s: text comes from window:, so it may not also be written, taken from a record or counted", path)
	}
	if r.Window == nil {
		return nil, fmt.Errorf("%s: no resolver for windows", path)
	}
	words, err := r.Window(n.Window)
	if err != nil {
		return nil, fmt.Errorf("%s: window: %w", path, err)
	}
	out := make(map[string]string, len(props)+1)
	for k, v := range props {
		out[k] = v
	}
	out["text"] = words
	return out, nil
}

// RouteResolver answers a `to:`: the route and the label of the navigation item named navID, or ok=false when
// the Application declares none. It is injected for the same reason the two above are, and it is a closure
// over *data* (the Application's `navigation:`), so it carries no id of its own.
type RouteResolver func(navID string) (route, label string, ok bool)

// NavigationRoutes is the RouteResolver over a list of navigation items. It is given
// `domain.Application.AllNavigation` -- the list frozen before `hidden_nav_groups` filtering, the same list
// `rendering.routeByID` reads -- so a link to an item in a hidden group still resolves.
func NavigationRoutes(items []domain.NavigationItem) RouteResolver {
	return func(navID string) (string, string, bool) {
		for _, it := range items {
			if it.ID == navID {
				return it.Route, it.Label, true
			}
		}
		return "", "", false
	}
}

// SourceResolver answers a `list_of:`: the id of the Machine the named Dataset reads, or ok=false when this
// Workspace declares no such Dataset. It returns the Machine and not a route, so the route is assembled in one
// place (`domain.MachineListRoute`) however many resolvers there are.
type SourceResolver func(datasetID string) (machineID string, ok bool)

// FormSpec is a write Binding's answer: where the form posts, whether this viewer may create at all, and the
// controls the Machine asks for. It is plain data -- no Machine, no Actor -- so the representation stays free of
// the things that decided it (the resolver applied the viewer's create Permission and the Machine's Fields).
//
// Permitted is the viewer's own answer: a form the viewer may not submit is **not drawn**, the way the generic
// pages hide a "New" button, rather than drawn and refused. The route still refuses on its own, so this is
// courtesy and not enforcement.
type FormSpec struct {
	// Reader is the document reader a Form with `extract:` offers above itself: a small form that sends a document to be
	// read. nil is a host or a viewer with none (and every Form without `extract:`).
	Reader *FormReader
	// Carries is a hidden "name=value" the form sends with itself (the proposal it was filled from).
	Carries string
	// Rows are the proposed child records a document held, each kind with its rows of controls; they ride the same submit.
	Rows []FormRows
	// Document is the words naming the document a proposal was read from ("ctr-c-026.pdf · 1.4 MB"), shown above the form.
	Document string
	// Number is how the Workspace writes a figure, for the total of a proposed rows table.
	Number domain.NumberFormat
	Route  string
	// Method is the verb the route takes (`domain.FormMethodPost`, `domain.FormMethodPatch`); empty means post.
	Method    string
	Permitted bool
	Inputs    []domain.FormInput
}

// FormReader is the reader's small form: where it posts, what its button says and what the file control is called.
type FormReader struct {
	Route, Read, Label string
}

// FormRows is one kind of proposed child record: its heading and each row's controls, already decided and filled.
type FormRows struct {
	Label string
	Rows  [][]domain.FormInput
}

// ActionResolver answers a `write: action` binding for the record it sits beside ("" outside a template): the viewer's
// permission and what a Form for it asks, which for the dynamic inputs depends on that record.
type ActionResolver func(b domain.PageBinding, recordID string) (FormSpec, error)

// FormResolver answers a write Binding (`write: create`).
type FormResolver func(b domain.PageBinding) (FormSpec, error)

// Resolver is what Lower may ask the outside world. Any part may be nil when the tree has no use of it (no
// Binding of that mode, no `to:`); asking for one that is nil is an error rather than a panic.
type Resolver struct {
	Rows    RowResolver
	Records RecordResolver
	Route   RouteResolver
	Source  SourceResolver
	Form    FormResolver
	Total   TotalResolver
	// Number is how the Workspace writes a figure (a Table's total row is written the way its cells are); the zero value is the
	// international format.
	Number domain.NumberFormat
	// Action answers a `write: action` binding: the operation's route, whether this viewer may perform it, and what a
	// Form for it asks. Only the runtime's own pages bind one.
	Action ActionResolver
	// Navigation answers a NavList's `section:`: the label of the list, the route the sidebar leads back to, and the
	// destinations the navigation declares as part of it, in order. PageID is the navigation item the page being lowered
	// belongs to, which is how the row for the screen the viewer is on is marked.
	Navigation NavigationResolver
	PageID     string
	// Query answers a SearchBox: the words the request carried under a query parameter, "" when none. Nil is the load-time
	// placeholder, for which there is no request.
	Query func(name string) string
	// Params answers what a filter keeps: every parameter the request carried, so a chip, a drop-down and a search box
	// change their own parameter and leave the others. Nil is the load-time placeholder, which has no request.
	Params func() map[string]string
	// TransitionAll answers a `write: transition` Button with `all: true`: where the request goes and how many records
	// of the Dataset the viewer could move to the status `becomes` names (the Machine's `completion:` already applied
	// to `$done`/`$reopen`). A count of none is the Button not being drawn.
	TransitionAll TransitionAllResolver
	// Parameter answers a `when: {parameter: ...}`: the value of a request parameter in the form the runtime reads
	// it (the week one is a whole number, "0" for this week). Nil is a shape-only lowering -- the load-time
	// placeholder, for which there is no request -- and every parameter condition then holds, so the nodes a real
	// request would hide are still validated, as ShapeOnly does for a record's.
	Parameter ParameterResolver
	// Viewer answers a `when: {viewer: ...}`: the viewer's value for the named fact (their Workspace role), "" when the
	// runtime cannot place them. Nil in the load-time placeholder, for which there is no viewer -- and every viewer
	// condition then holds, so the nodes a real viewer would not see are still validated.
	Viewer ViewerResolver
	// Step answers a link's `step:`: the value of the parameter `by` steps on from the request's own, "" when that is
	// the value a request without the parameter already means (so a step back to this week links to the plain
	// route, not to `?week=0`).
	Step StepResolver
	// Window answers a `window:`: the words that say a Dataset's date window.
	Window WindowResolver
	// Carry adds to the address of a link that names a navigation item (`to:`) what the request carries to that item: the scope
	// choice of a view group, kept between its screens. Nil is a page of no group; it is given the item's id and the address the
	// link would have, and answers the address to use.
	Carry func(navID, href string) string
	// PDF says whether this host can print a page to PDF. Nil is the load-time placeholder, for which every host can.
	PDF func() bool
}

// ParameterResolver, StepResolver and WindowResolver are the three things a page asks of the request itself rather
// than of the database. They are injected for the reason RouteResolver is: `ir` reads nothing and may not import
// the packages that do.
type ParameterResolver func(name string) string
type ViewerResolver func(name string) string
type StepResolver func(name string, by int) string
type WindowResolver func(datasetID string) (string, error)

// AllTransition is the answer to a bulk transition: the route, and how many records it would move for this viewer.
// Like the other record-level answers it is courtesy; the route selects and judges every record again.
type AllTransition struct {
	Route string
	Count int
}

// NavEntry is one destination of a NavList section as the resolver found it.
type NavEntry struct {
	ID, Group, Label, Description, Href string
	Placeholder, SidebarOnly            bool
}

// NavSection is a section's answer: its own name and hub route, and its entries in the order they are drawn.
type NavSection struct {
	Label, Home string
	Entries     []NavEntry
}

// NavigationResolver is the injected question behind a NavList.
type NavigationResolver func(section string) (NavSection, error)

// TransitionAllResolver is the injected question behind a bulk transition Button.
type TransitionAllResolver func(b domain.PageBinding, becomes string) (AllTransition, error)

// TotalResolver answers a `rows: total` Binding: the Measure's figure over the whole Dataset, already formatted.
type TotalResolver func(b domain.PageBinding) (string, error)

// Lower turns a declared `page:` into UI IR (007 §15.1's "Build UI IR"), expanding every Binding through
// the Resolver and nothing else -- it decides no layout, formats no value, and reads no data itself.
//
// **A bound node is a template for its rows.** `component: Metric` with a `binding:` becomes one Metric per
// row, each `label` the Dimension's value and `value` the Measure's, in the resolver's order. The author
// therefore writes neither `label` nor `value` on it (a hand-typed figure beside a bound one is the "number
// the author typed" this whole key exists to avoid), and the check is here so it cannot be forgotten by a
// caller. Other properties (`hint`, `tone`) are copied to every row.
//
// **A records-bound Collection is a template for its items.** It holds exactly one child, and that child
// (with everything under it) is cloned once per record, each `from:` property filled from the record's
// Projection role. The author writes neither the property nor a second source for it, for the same reason.
// `from:` is meaningful only inside such a template, so one anywhere else is refused here rather than being
// ignored by a renderer that never looks.
//
// Faults that are about the *declaration* (a binding on a node that cannot take one, a missing dataset name,
// a bound node holding the wrong children, a `from:` outside a template) are returned as errors; faults about
// the *tree* (unknown types, cycles, depth) are `Validate`'s, so the five §15.3 rejections stay in one place.
func Lower(root domain.PageNode, r Resolver) (UINode, error) {
	nodes, err := lower(root, r, "page", nil, nil)
	if err != nil {
		return UINode{}, err
	}
	if len(nodes) != 1 || root.Binding != nil {
		return UINode{}, fmt.Errorf("the root of a page must be one unbound node, not a binding that expands to several")
	}
	return nodes[0], nil
}

// lower expands one node. record is non-nil exactly while lowering inside a records template, which is what
// makes `from:` legal and a nested Binding illegal.
func lower(n domain.PageNode, r Resolver, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	if n.HintFigure != nil && (n.Binding == nil || n.Binding.Rows != domain.PageRowsTotal || n.Type != string(domain.ComponentMetric)) {
		return nil, fmt.Errorf("%s: hint_figure: words a hint around a figure of the bound Dataset, so it is valid only on a Metric bound with rows: %s", path, domain.PageRowsTotal)
	}
	if n.When != nil {
		var show bool
		var err error
		if n.When.Viewer != "" {
			show, err = viewerHolds(*n.When, r, path)
		} else if n.When.Parameter != "" {
			show, err = parameterHolds(*n.When, r, path)
		} else {
			show, err = conditionHolds(*n.When, record, writes, path)
		}
		if err != nil || !show {
			return nil, err
		}
	}
	if omittedForWantOfDate(n, record) {
		return nil, nil
	}
	if n.Each != "" {
		return lowerEach(n, r, path, record, writes)
	}
	if n.Step != nil && !(NodeKind(n.Kind) == NodeStatic && n.Type == string(domain.StaticLink) && n.To != "") {
		return nil, fmt.Errorf("%s: step: moves a link's address, so it is valid only on a static: link with to:", path)
	}
	if len(n.Columns) > 0 && !(NodeKind(n.Kind) == NodeComponent && n.Type == string(domain.ComponentTable)) {
		return nil, fmt.Errorf("%s: columns: declares a Table's columns, so it is valid only on a Table", path)
	}
	if NodeKind(n.Kind) == NodeComponent && n.Type == string(domain.ComponentTable) && (n.Binding == nil || n.Binding.Write != "") {
		return nil, fmt.Errorf("%s: a Table lists records or a Dimension's rows, so it is bound with rows: %s or %s", path, domain.PageRowsRecords, domain.PageRowsDimension)
	}
	if n.Window != "" && NodeKind(n.Kind) != NodeStatic {
		return nil, fmt.Errorf("%s: window: produces text, so it is valid only on a static node", path)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteUpdate {
		return lowerUpdateForm(n, path, record, writes)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteTransition && n.Binding.Transition != "" && n.Type == string(domain.ComponentForm) {
		return lowerTransitionForm(n, path, record, writes)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteDelete {
		return lowerDeleteButton(n, path, record, writes)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteMove {
		return lowerMoveButton(n, path, record, writes)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWritePrint {
		return lowerPrintButton(n, path)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWritePDF {
		return lowerPDFButton(n, r, path)
	}
	if NodeKind(n.Kind) == NodeComponent && n.Type == string(domain.ComponentNavList) {
		return lowerNavList(n, r, path)
	}
	if NodeKind(n.Kind) == NodeComponent && n.Type == string(domain.ComponentSearchBox) {
		return lowerSearchBox(n, r, path)
	}
	if NodeKind(n.Kind) == NodeComponent && n.Type == string(domain.ComponentFilterChips) {
		return lowerFilterChips(n, r, path)
	}
	if NodeKind(n.Kind) == NodeComponent && n.Type == string(domain.ComponentFilterSelect) {
		return lowerFilterSelect(n, r, path)
	}
	if NodeKind(n.Kind) == NodeComponent && n.Type == string(domain.ComponentSegmentedControl) {
		return lowerSegmentedControl(n, r, path)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteAction {
		return lowerAction(n, r, path, record)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteTransition && n.Binding.All {
		return lowerTransitionAll(n, r, path, record)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteTransition && NodeKind(n.Kind) == NodeComponent && n.Type == string(domain.ComponentToggle) {
		return lowerToggle(n, path, record, writes)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteTransition {
		return lowerTransitionButton(n, path, record, writes)
	}
	if n.Binding != nil {
		if n.Count != nil {
			return nil, fmt.Errorf("%s: count: counts a Relation's children for one record, so it belongs on a static node inside the item template, not on the bound node", path)
		}
		if record != nil {
			return nil, fmt.Errorf("%s: a binding inside a record template is not supported -- the item is one record, and a second Dataset would be a join the page has no grammar for", path)
		}
		return lowerBound(n, r, path)
	}
	if n.Kind == string(NodeComponent) && n.Type == string(domain.ComponentFormInput) {
		return nil, fmt.Errorf("%s: Input is produced by lowering a bound Form, never written: a control's name= is its Machine's Field id, and a typed one is exactly the retyped identifier a Binding exists to remove (007 §11.3)", path)
	}
	_, fromCurrent := n.From[SectionCurrentProp]
	if _, written := n.Props[SectionCurrentProp]; (written || fromCurrent) && NodeKind(n.Kind) == NodeLayout {
		return nil, fmt.Errorf("%s: %s: is produced by lowering for the day a grouped list is drawing on, and is never written -- a page cannot know which day it is read on", path, SectionCurrentProp)
	}
	if _, written := n.Props[SpanProp]; written {
		return nil, fmt.Errorf("%s: %s: is produced by lowering for a notice about a grouped list's grid, and is never written", path, SpanProp)
	}
	for _, p := range []string{SectionOutsideProp, SectionDropFieldProp, SectionDropValueProp, SectionLabelProp, StackMoveProp} {
		if _, written := n.Props[p]; written && NodeKind(n.Kind) == NodeLayout {
			return nil, fmt.Errorf("%s: %s: is produced by lowering for a grouped list's days and cards, and is never written -- a page cannot know which day is outside its window or what the viewer may move", path, p)
		}
	}
	props := n.Props
	if len(n.From) > 0 {
		if record == nil {
			return nil, fmt.Errorf("%s: from: is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageRowsRecords)
		}
		var err error
		if props, err = fillFrom(n, record, path); err != nil {
			return nil, err
		}
	}
	for _, p := range []string{collectionEmptyProp, collectionHideEmptyProp} {
		if _, written := n.Props[p]; written && n.Kind == string(NodeComponent) && n.Type == string(domain.ComponentCollection) {
			return nil, fmt.Errorf("%s: %s: speaks of what a records binding lists when it lists nothing, so it is valid only on a Collection bound with rows: %s -- a Collection that holds its own children is empty by being written empty", path, p, domain.PageRowsRecords)
		}
	}
	for _, p := range []string{collectionCompleteProp, collectionTruncatedProp} {
		if _, written := n.Props[p]; written && n.Kind == string(NodeComponent) && n.Type == string(domain.ComponentCollection) {
			return nil, fmt.Errorf("%s: %s: speaks of a Dataset's limit, so it is valid only on a Collection bound with rows: %s", path, p, domain.PageRowsRecords)
		}
	}
	props, err := lowerCount(n, props, record, path)
	if err != nil {
		return nil, err
	}
	props, err = lowerWindow(n, props, r, path)
	if err != nil {
		return nil, err
	}
	props, err = lowerLink(n, props, r, path, record)
	if errors.Is(err, errLinkNotDrawn) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tracks, err := rowTracks(n, props, path)
	if err != nil {
		return nil, err
	}
	if tracks != nil {
		props = withoutKey(props, "tracks")
	}
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}
	for i, c := range n.Children {
		kids, err := lower(c, r, fmt.Sprintf("%s.children[%d]", path, i), record, writes)
		if err != nil {
			return nil, err
		}
		if tracks != nil {
			kids = inTrack(kids, tracks[i])
		}
		out.Children = append(out.Children, kids...)
	}
	if err := lowerLayoutEmpty(n, &out, path); err != nil {
		return nil, err
	}
	return []UINode{out}, nil
}

// rowTracks reads a row's `tracks:` -- one word for each child, in order -- and refuses what cannot be drawn: a word outside the closed
// set, a count that is not the children's, or `tracks:` on anything but a row. nil when the node declares none.
func rowTracks(n domain.PageNode, props map[string]string, path string) ([]domain.RowTrack, error) {
	words, declared := props["tracks"]
	if !declared {
		return nil, nil
	}
	if NodeKind(n.Kind) != NodeLayout || n.Type != string(domain.LayoutRow) {
		return nil, fmt.Errorf("%s: tracks: gives the children of a row their columns, so it is valid only on a layout: row", path)
	}
	fields := strings.Fields(words)
	if len(fields) != len(n.Children) {
		return nil, fmt.Errorf("%s: tracks: names %d column(s) and the row holds %d child(ren) -- one word for each child, in order, a child a when: hides still holding its column", path, len(fields), len(n.Children))
	}
	for i, c := range n.Children {
		if c.Each != "" || c.Binding != nil {
			return nil, fmt.Errorf("%s: tracks: a column holds one child, and children[%d] draws once for each of something (each: / binding:), which would put several boxes in one column and shift every row that has a different number of them", path, i)
		}
	}
	out := make([]domain.RowTrack, len(fields))
	for i, w := range fields {
		if !domain.KnownRowTracks[domain.RowTrack(w)] {
			return nil, fmt.Errorf("%s: tracks: %q is not a column (fill, fit, field, badge)", path, w)
		}
		out[i] = domain.RowTrack(w)
	}
	return out, nil
}

// inTrack puts every node a child lowered to in its column. A child that lowered to nothing -- a `when:` that did not hold -- becomes an
// empty caption in that column, so the columns after it keep their place instead of sliding left (an empty `layout` would be refused: a
// Layout exists to hold children).
func inTrack(kids []UINode, track domain.RowTrack) []UINode {
	if len(kids) == 0 {
		kids = []UINode{{Kind: NodeStatic, Type: string(domain.StaticCaption), Props: map[string]string{"text": ""}}}
	}
	out := make([]UINode, len(kids))
	for i, k := range kids {
		k.Track = track
		out[i] = k
	}
	return out
}

func withoutKey(m map[string]string, key string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if k != key {
			out[k] = v
		}
	}
	return out
}

// lowerLayoutEmpty consumes a layout's `empty:` -- the words a page says when everything it holds was hidden by its
// data (every Collection `hide_empty`, every record condition false). When something is drawn the prop is dropped;
// when nothing is, the layout holds one `EmptyState` card with those words (owner decision E-1, 2026-10-14: a layout's
// `empty:` is a whole region of the page and so draws the card, where a Collection's stays plain words), so a quiet day
// is a short page and not a layout with no children, which Validate refuses. It is valid only on a layout that declares children (an
// empty one is a fault whatever it says); on any other kind `empty` means something else or nothing, and is left
// for that kind's own check.
func lowerLayoutEmpty(n domain.PageNode, out *UINode, path string) error {
	words, written := n.Props[collectionEmptyProp]
	if NodeKind(n.Kind) == NodeLayout && !written && len(n.Children) > 0 && allHideEmpty(n.Children) {
		return fmt.Errorf("%s: every child is a %s Collection, so on a day with no records the layout would hold nothing; write %s: <words> on the layout for that day", path, collectionHideEmptyProp, collectionEmptyProp)
	}
	if !written || NodeKind(n.Kind) != NodeLayout {
		return nil
	}
	if len(n.Children) == 0 {
		return fmt.Errorf("%s: %s: is what a layout says when everything it holds is hidden, so it needs children to be hidden", path, collectionEmptyProp)
	}
	if words == "" {
		return fmt.Errorf("%s: %s: is written with no words", path, collectionEmptyProp)
	}
	props := make(map[string]string, len(out.Props))
	for k, v := range out.Props {
		if k != collectionEmptyProp {
			props[k] = v
		}
	}
	out.Props = props
	if len(out.Children) == 0 {
		out.Children = []UINode{{Kind: NodeComponent, Type: string(domain.ComponentEmptyState), Props: map[string]string{"text": words}}}
	}
	return nil
}

// allHideEmpty reports whether every child is a Collection that leaves itself out when it lists nothing -- the one
// shape whose layout can end up with no children whatever the data, and which load cannot see happen, since the
// load-time placeholder record means nothing is ever empty then.
func allHideEmpty(children []domain.PageNode) bool {
	for _, c := range children {
		if NodeKind(c.Kind) != NodeComponent || c.Type != string(domain.ComponentCollection) || c.Props[collectionHideEmptyProp] != "true" {
			return false
		}
	}
	return true
}

// lowerEach draws the node once for each of the current record's tags, or not at all when it has none. The copy is
// lowered with the tag as its record, so `from: {label: title, color: color}` reads the tag and the node needs no
// other grammar; it is lowered with writes of its own that carry only the shared completion words, since a
// write there would be the parent record's under a tag's name.
func lowerEach(n domain.PageNode, r Resolver, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	if n.Each != domain.PageEachTags {
		return nil, fmt.Errorf("%s: each: %q is not a list a record owns (%s)", path, n.Each, domain.PageEachTags)
	}
	if record == nil || writes == nil || writes.InEach {
		return nil, fmt.Errorf("%s: each: %s is valid only directly in the item template of a Collection bound with rows: %s", path, n.Each, domain.PageRowsRecords)
	}
	if len(n.From) == 0 {
		return nil, fmt.Errorf("%s: each: %s draws the node once per tag, so it must take something from the tag with from:", path, n.Each)
	}
	if n.Binding != nil {
		return nil, fmt.Errorf("%s: each: %s and binding: are two ways to repeat a node, and a node has one", path, n.Each)
	}
	each := n
	each.Each = ""
	inner := &RecordWrites{Done: writes.Done, Reopen: writes.Reopen, ShapeOnly: writes.ShapeOnly, InEach: true}
	var out []UINode
	for i, tag := range writes.Tags {
		kids, err := lower(each, r, path, tag, inner)
		if err != nil {
			return nil, fmt.Errorf("tag %d: %w", i, err)
		}
		out = append(out, kids...)
	}
	return out, nil
}

// lowerCount turns `count: {of, one, other}` into the node's `text`: the number of children the record's
// declared Relation found, chosen between the two wordings and substituted into `{n}`. The author writes the
// words and never the number, for the reason a bound Metric's value is never typed; so `text` may not also be
// written or taken `from:`, and `of` must be a Relation the bound Dataset declares -- which is the case
// exactly when the record carries its count (`domain.PageCountRole`), so the check needs no second list.
func lowerCount(n domain.PageNode, props map[string]string, record map[string]string, path string) (map[string]string, error) {
	c := n.Count
	if c == nil {
		return props, nil
	}
	if NodeKind(n.Kind) != NodeStatic {
		return nil, fmt.Errorf("%s: count: produces text, so it is valid only on a static node", path)
	}
	if record == nil {
		return nil, fmt.Errorf("%s: count: is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageRowsRecords)
	}
	if c.Of == "" || c.Other == "" {
		return nil, fmt.Errorf("%s: count: names the Relation it counts (of:) and the words for every number but one (other:)", path)
	}
	if !strings.Contains(c.Other, domain.PageCountToken) {
		return nil, fmt.Errorf("%s: count.other must carry %s, or the number is never shown", path, domain.PageCountToken)
	}
	if _, written := props["text"]; written {
		return nil, fmt.Errorf("%s: text comes from count:, so it may not also be written or taken from a record", path)
	}
	raw, ok := record[domain.PageCountRole(c.Of)]
	if !ok {
		return nil, fmt.Errorf("%s: count: of: names the Relation %q, which the bound Dataset does not declare in relations:", path, c.Of)
	}
	words := c.Other
	if raw == "1" && c.One != "" {
		words = c.One
	}
	out := make(map[string]string, len(props)+1)
	for k, v := range props {
		out[k] = v
	}
	out["text"] = strings.ReplaceAll(words, domain.PageCountToken, raw)
	return out, nil
}

// lowerLink resolves a `static: link`'s destination. A link names its target by navigation item id (`to:`) and
// never by route, so `href` is not a property an author may write: it is the one thing this step produces.
// `text` defaults to the item's own label -- the words the menu already uses for that screen -- and may be
// written (or taken `from:` a record) when the link should say something else. `to:` on any other node is
// refused rather than ignored, for the reason `from:` outside a template is.
// collectionEmptyProp is the one Collection property that is about the *binding's* result and not about the
// list's shape: what to say when the Dataset has no records. It is written by the author (words are content)
// and is meaningful only where a binding can produce zero items.
const collectionEmptyProp = "empty"

// collectionHideEmptyProp is the other thing an author can say about a records Collection that lists nothing: draw
// nothing at all, its heading (a Collection's own `heading`) included, so a section with no rows is not on the page.
// It is consumed by lowering and never reaches the lowered node, and it is exclusive with `empty`, which says the
// opposite -- draw these words in the list's place. A page whose every section is hidden says so with the `empty`
// of the layout that holds them (lowerLayoutEmpty), because a layout with nothing in it is a fault, not a design.
const collectionHideEmptyProp = "hide_empty"

// The two halves of an author-opted truncation notice. `complete` is written by the author: *this list claims to
// show every record its Dataset matches*, which is the one fact nothing else declares -- a `limit:` is a safety
// cap on one Dataset and the definition of "recent" on another, and only the screen knows which (the argument in
// `composition.Selection.Limit`). `truncated` is what lowering produces, the Dataset's bound once it has bitten,
// for the renderer to name; it is never written by an author, since a hand-typed bound would be a notice about
// nothing.
const (
	collectionCompleteProp  = "complete"
	collectionTruncatedProp = "truncated"
)

// lowerComplete consumes `complete` and, when the author claimed completeness and the bound actually bit, puts
// the bound where the renderer reads it. It never copies `complete` onto the node: the lowered tree carries the
// outcome, not the claim, so a list that was not cut short is byte-identical to one that made no claim.
func lowerComplete(n domain.PageNode, set RecordSet, path string) (map[string]string, error) {
	if _, written := n.Props[collectionTruncatedProp]; written {
		return nil, fmt.Errorf("%s: %s: is produced by the runtime when a %s: true list is cut short by its Dataset's limit, and is never written", path, collectionTruncatedProp, collectionCompleteProp)
	}
	v, claimed := n.Props[collectionCompleteProp]
	if !claimed {
		return n.Props, nil
	}
	if v != "true" && v != "false" {
		return nil, fmt.Errorf("%s: %s: %q is not true or false", path, collectionCompleteProp, v)
	}
	props := make(map[string]string, len(n.Props))
	for k, val := range n.Props {
		if k != collectionCompleteProp {
			props[k] = val
		}
	}
	if v == "true" && set.Truncated && set.Limit > 0 {
		props[collectionTruncatedProp] = strconv.Itoa(set.Limit)
	}
	return props, nil
}

// errLinkNotDrawn is lowerLink's answer for a link into another Application that this Workspace does not offer this viewer:
// not an error of the page, a link that is not there.
var errLinkNotDrawn = errors.New("ir: link into an Application not offered here")

func lowerLink(n domain.PageNode, props map[string]string, r Resolver, path string, record map[string]string) (map[string]string, error) {
	isLink := n.Kind == string(NodeStatic) && n.Type == string(domain.StaticLink)
	for prop, role := range n.From {
		if role == domain.PageRecordRole && !(isLink && prop == "href") {
			return nil, fmt.Errorf("%s: from: %s names %q, which is valid only as a link's href", path, prop, role)
		}
	}
	if n.Param != "" { // lowerLink sees only unbound nodes, and a bound Metric is the only node that takes one
		return nil, fmt.Errorf("%s: param: carries a row's value into a destination, so it is valid only beside to: on a Metric bound with rows: %s", path, domain.PageRowsDimension)
	}
	if n.Step != nil && !(isLink && n.To != "") {
		return nil, fmt.Errorf("%s: step: moves the parameter a link sends, so it is valid only on a static: link that names its destination with to:", path)
	}
	if len(n.Set) > 0 && !(isLink && n.To != "") {
		return nil, fmt.Errorf("%s: set: is a parameter a link sends beside its destination, so it is valid only on a static: link that names it with to:", path)
	}
	if !isLink {
		if n.In != "" {
			return nil, fmt.Errorf("%s: in: is valid only on static: link, not %s %q", path, n.Kind, n.Type)
		}
		if n.To != "" {
			return nil, fmt.Errorf("%s: to: is valid only on static: link, not %s %q", path, n.Kind, n.Type)
		}
		if n.ListOf != "" {
			return nil, fmt.Errorf("%s: list_of: is valid only on static: link, not %s %q", path, n.Kind, n.Type)
		}
		return props, nil
	}
	if v, ok := n.Props["variant"]; ok && !domain.KnownLinkVariants[domain.ButtonVariant(v)] {
		return nil, fmt.Errorf("%s: variant: %q -- a link is drawn as a link, as a quiet link, or as a primary or secondary button", path, v)
	}
	if icon, ok := n.Props["icon"]; ok {
		if !domain.KnownLinkIcons[icon] {
			return nil, fmt.Errorf("%s: icon: %q is not a mark a link takes (chevron-left, chevron-right, settings)", path, icon)
		}
		if v := domain.ButtonVariant(n.Props["variant"]); v != "" && v != domain.ButtonPrimary && v != domain.ButtonSecondary {
			return nil, fmt.Errorf("%s: icon: is a mark in place of a button's words (variant: primary or secondary) or before a plain link's (no variant), and variant: %s is neither", path, v)
		}
		if n.Props["text"] == "" && n.From["text"] == "" {
			return nil, fmt.Errorf("%s: icon: replaces the words on the page, not for a screen reader -- the link still writes text:, which is its accessible name", path)
		}
	}
	if _, written := n.Props["href"]; written {
		return nil, fmt.Errorf("%s: a link's destination is to: <navigation item id>, list_of: <dataset id> or, inside a record template, from: {href: %s}; never a typed href -- a route is declared once (001 #3, #8)", path, domain.PageRecordRole)
	}
	if n.ListOf != "" {
		return lowerListOf(n, props, r, path)
	}
	if role, fromHref := n.From["href"]; fromHref {
		if n.To != "" {
			return nil, fmt.Errorf("%s: a link has one destination, and this one names both to: %s and from: href", path, n.To)
		}
		if id, isFile := strings.CutPrefix(role, "filehref:"); role != domain.PageRecordRole && role != string(domain.CardFieldRoleLink) && !(isFile && id != "") {
			return nil, fmt.Errorf("%s: a link's href may be taken from %q (the record's own page), %q (the route the record leads to) or filehref:<file field> (a file it holds) only, not from the role %q", path, domain.PageRecordRole, domain.CardFieldRoleLink, role)
		}
		if _, hasText := props["text"]; !hasText {
			return nil, fmt.Errorf("%s: a link to a record has no menu label to default its text to, so it takes text from a role (from: {text: title})", path)
		}
		return props, nil
	}
	if n.To == "" {
		return nil, fmt.Errorf("%s: a link names its destination with to: <navigation item id>, list_of: <dataset id>, or from: {href: %s} inside a record template", path, domain.PageRecordRole)
	}
	if n.In != "" && (n.ListOf != "" || n.Step != nil) {
		return nil, fmt.Errorf("%s: in: names the Application of the to: this link points at; it does not go with list_of: or step:", path)
	}
	if r.Route == nil {
		return nil, fmt.Errorf("%s: no resolver for routes", path)
	}
	if n.In == "" && strings.Contains(n.To, "/") {
		return nil, fmt.Errorf("%s: to: %s names an item by a slash, which is how another Application's screens are named internally -- link into another Application with in: <application id> beside to: <item id>", path, n.To)
	}
	routeID := n.To
	if n.In != "" {
		routeID = domain.ForeignLinkID(n.In, n.To)
	}
	route, label, ok := r.Route(routeID)
	if !ok {
		if n.In != "" {
			return nil, errLinkNotDrawn // that Application is not installed here, or this viewer may not open it
		}
		return nil, fmt.Errorf("%s: to: %s names no navigation item in this Application", path, n.To)
	}
	out := make(map[string]string, len(props)+2)
	for k, v := range props {
		out[k] = v
	}
	out["href"] = route
	if n.Step != nil {
		step := n.Step
		if !domain.KnownPageStepParameters[step.Parameter] || step.By == 0 {
			return nil, fmt.Errorf("%s: step: moves %s or %s by a whole number other than zero", path, domain.PageParameterWeek, domain.PageParameterMonth)
		}
		value := "1"
		if r.Step != nil {
			value = r.Step(step.Parameter, step.By)
		}
		if value != "" {
			out["href"] = route + "?" + url.QueryEscape(step.Parameter) + "=" + url.QueryEscape(value)
		}
	}
	if len(n.Set) > 0 {
		for name, v := range n.Set {
			if name != domain.PageParameterRange || !domain.KnownPageRanges[v] {
				return nil, fmt.Errorf("%s: set: %s: %s -- a link sets the range of the window and nothing else", path, name, v)
			}
		}
		path0, query, _ := strings.Cut(out["href"], "?")
		q, _ := url.ParseQuery(query)
		for name, v := range n.Set {
			q.Set(name, v)
		}
		out["href"] = path0 + "?" + q.Encode()
	}
	if len(n.Pass) > 0 {
		// pass: carries the record's own values to the destination, so it needs a record and the link's own route.
		if record == nil {
			return nil, fmt.Errorf("%s: pass: carries a record's values to the destination, so it is valid only inside a record template", path)
		}
		if n.Step != nil {
			return nil, fmt.Errorf("%s: a link takes pass: or step:, not both", path)
		}
		q := url.Values{}
		for name, role := range n.Pass {
			if domain.ParamReserved(name) {
				return nil, fmt.Errorf("%s: pass: %s is a parameter the runtime owns", path, name)
			}
			v, ok := record[role]
			if !ok {
				return nil, fmt.Errorf("%s: pass: %s takes the role %q, which the record does not have", path, name, role)
			}
			q.Set(name, v)
		}
		out["href"] = route + "?" + q.Encode()
	}
	if r.Carry != nil {
		out["href"] = r.Carry(routeID, out["href"])
	}
	if _, has := out["text"]; !has && n.From["text"] == "" {
		out["text"] = label
	}
	return out, nil
}

// lowerListOf resolves a link to the generic list page of a Dataset's Machine. It has no menu label to default
// its text to -- the Machine has no navigation item, which is why this form exists -- so the text is written
// (or taken `from:` a role), and one destination only: a link that named `to:` as well would be two.
func lowerListOf(n domain.PageNode, props map[string]string, r Resolver, path string) (map[string]string, error) {
	if n.To != "" {
		return nil, fmt.Errorf("%s: a link has one destination, and this one names both to: %s and list_of: %s", path, n.To, n.ListOf)
	}
	if _, fromHref := n.From["href"]; fromHref {
		return nil, fmt.Errorf("%s: a link has one destination, and this one names both list_of: %s and from: href", path, n.ListOf)
	}
	if _, hasText := props["text"]; !hasText {
		return nil, fmt.Errorf("%s: a link to a Machine's list has no menu label to default its text to, so it writes text:", path)
	}
	if r.Source == nil {
		return nil, fmt.Errorf("%s: no resolver for dataset sources", path)
	}
	machineID, ok := r.Source(n.ListOf)
	if !ok {
		return nil, fmt.Errorf("%s: list_of: %s names no dataset in this Workspace", path, n.ListOf)
	}
	out := make(map[string]string, len(props)+1)
	for k, v := range props {
		out[k] = v
	}
	out["href"] = domain.MachineListRoute(machineID)
	return out, nil
}

// omittedForWantOfDate: a node reading a derived date role (domain.PageDerivedRole) for a record that has no date is
// not drawn. Only those two roles do this, so an empty title or container still draws the empty string it is.
func omittedForWantOfDate(n domain.PageNode, record map[string]string) bool {
	if record == nil {
		return false
	}
	for _, role := range n.From {
		v, present := record[role]
		// A blocked: role is empty or absent when the record has no edge held back, and says nothing then.
		if domain.PageBlockedRole(role) && v == "" {
			return true
		}
		if present && v == "" && domain.PageDerivedRole(role) {
			return true
		}
	}
	return false
}

func fillFrom(n domain.PageNode, record map[string]string, path string) (map[string]string, error) {
	props := make(map[string]string, len(n.Props)+len(n.From))
	for k, v := range n.Props {
		props[k] = v
	}
	for prop, role := range n.From {
		if _, written := n.Props[prop]; written {
			return nil, fmt.Errorf("%s: %q comes from %q, so it may not also be written on the node", path, prop, role)
		}
		v, ok := record[role]
		if !ok && !domain.PageBlockedRole(role) {
			return nil, fmt.Errorf("%s: from: %s names the role %q, which the bound Dataset's Machine does not project", path, prop, role)
		}
		props[prop] = v
	}
	return props, nil
}

func lowerBound(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	b := n.Binding
	if b.Write != "" {
		return lowerForm(n, r, path)
	}
	if NodeKind(n.Kind) != NodeComponent {
		return nil, fmt.Errorf("%s: a binding is only valid on a bindable component, and %s %q is not one", path, n.Kind, n.Type)
	}
	modes, ok := domain.BindableComponents[domain.ComponentType(n.Type)]
	if !ok {
		return nil, fmt.Errorf("%s: a binding is only valid on a bindable component, and %s %q is not one", path, n.Kind, n.Type)
	}
	if !slices.Contains(modes, b.Rows) {
		return nil, fmt.Errorf("%s: component %q takes a binding with rows: %s, not %q", path, n.Type, strings.Join(modes, " or "), b.Rows)
	}
	if b.Dataset == "" {
		return nil, fmt.Errorf("%s: a binding names a dataset", path)
	}
	switch {
	case n.Type == string(domain.ComponentTable):
		return lowerTable(n, r, path)
	case b.Rows == domain.PageRowsRecords:
		return lowerRecords(n, r, path)
	case b.Rows == domain.PageRowsTotal:
		return lowerTotal(n, r, path)
	case n.Type == string(domain.ComponentCollection):
		return lowerDimensionCollection(n, r, path)
	}
	return lowerDimension(n, r, path)
}

// lowerDimensionCollection turns a Collection bound with `rows: dimension` into one item per Dimension value, its
// one child cloned per row with `from:` reading the row: domain.PageRowLabelRole for the value's display and
// domain.PageMeasureRole(<id>) for each Measure. It is the shape for a group that needs more than one figure -- a
// bar of open cards out of all of them -- which a Metric per row cannot say.
//
// A Dimension row is a group of records, not a record, so the template may hold nothing that acts on or speaks of
// one record: no Binding (a write, or a second Dataset), no `count:`, no `each:`, no `when:`, and no
// `from: {href: record}`. Each is refused by name rather than failing later as an unprojected role.
func lowerDimensionCollection(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	b := n.Binding
	if b.Measure != "" {
		return nil, fmt.Errorf("%s: a Collection bound with rows: %s takes no measure: -- its item template reads each Measure with from: %s", path, domain.PageRowsDimension, domain.PageMeasureRole("<id>"))
	}
	if len(n.From) > 0 {
		return nil, fmt.Errorf("%s: from: belongs on the item template, not on the bound Collection", path)
	}
	if n.To != "" || n.Param != "" {
		return nil, fmt.Errorf("%s: to:/param: belong on a Metric bound with rows: %s", path, domain.PageRowsDimension)
	}
	for _, p := range []string{collectionCompleteProp, collectionTruncatedProp, collectionHideEmptyProp} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: speaks of a records binding, and rows: %s lists a Dimension's values", path, p, domain.PageRowsDimension)
		}
	}
	if len(n.Children) != 1 {
		return nil, fmt.Errorf("%s: a dimension-bound collection holds exactly one child, the item template cloned for each row, and has %d", path, len(n.Children))
	}
	if err := recordlessTemplateIssue(n.Children[0], fmt.Sprintf("%s.children[0]", path), "a dimension row"); err != nil {
		return nil, err
	}
	if r.Rows == nil {
		return nil, fmt.Errorf("%s: no resolver for dimension rows", path)
	}
	rows, err := r.Rows(*b)
	if err != nil {
		return nil, err
	}
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: n.Props}
	for i, row := range rows {
		rec := make(map[string]string, len(row.Measures)+1)
		rec[domain.PageRowLabelRole] = row.Label
		for id, v := range row.Measures {
			rec[domain.PageMeasureRole(id)] = v
		}
		items, err := lower(n.Children[0], r, fmt.Sprintf("%s.children[0]", path), rec, &RecordWrites{})
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i, err)
		}
		out.Children = append(out.Children, items...)
	}
	return []UINode{out}, nil
}

// recordlessTemplateIssue refuses, anywhere in a template that stands for a group (a dimension row, a bucket's
// heading) rather than for one record, what only a record could answer. subject names the group in the message.
func recordlessTemplateIssue(n domain.PageNode, path, subject string) error {
	switch {
	case n.Binding != nil:
		return fmt.Errorf("%s: a binding inside %s's template is not supported -- it is a group of records, so there is no record to write and a second Dataset would be a join", path, subject)
	case n.Count != nil:
		return fmt.Errorf("%s: count: counts one record's children, and %s is not a record", path, subject)
	case n.Each != "":
		return fmt.Errorf("%s: each: lists one record's tags, and %s is not a record", path, subject)
	case n.When != nil:
		return fmt.Errorf("%s: when: is not supported inside %s's template, which stands for a group rather than for one record -- a record condition has no record to read, and a request condition is not built here", path, subject)
	}
	for prop, role := range n.From {
		if role == domain.PageRecordRole {
			return fmt.Errorf("%s: from: %s: %s is a record's address, and %s has none", path, prop, role, subject)
		}
	}
	for i, c := range n.Children {
		if err := recordlessTemplateIssue(c, fmt.Sprintf("%s.children[%d]", path, i), subject); err != nil {
			return err
		}
	}
	return nil
}

func lowerDimension(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	b := n.Binding
	if b.Measure == "" {
		return nil, fmt.Errorf("%s: a binding names a dataset and a measure", path)
	}
	if len(n.Children) > 0 {
		return nil, fmt.Errorf("%s: a bound node is a template for its rows and holds no children", path)
	}
	if len(n.From) > 0 {
		return nil, fmt.Errorf("%s: from: is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageRowsRecords)
	}
	for _, supplied := range []string{"label", "value"} {
		if _, ok := n.Props[supplied]; ok {
			return nil, fmt.Errorf("%s: %q comes from the binding, so it may not also be written on the node", path, supplied)
		}
	}
	if r.Rows == nil {
		return nil, fmt.Errorf("%s: no resolver for dimension rows", path)
	}
	route, err := dimensionRoute(n, r, path)
	if err != nil {
		return nil, err
	}
	rows, err := r.Rows(*b)
	if err != nil {
		return nil, err
	}
	out := make([]UINode, 0, len(rows))
	for _, row := range rows {
		props := make(map[string]string, len(n.Props)+3)
		for k, v := range n.Props {
			props[k] = v
		}
		props["label"], props["value"] = row.Label, row.Value
		if route != "" {
			// The row's Dimension value travels as data: url.Values escapes it, and the destination compares it
			// as a bind parameter (007 §9.2), so a value holding `&` or a quote reaches it as itself.
			props["href"] = route + "?" + url.Values{n.Param: {row.linkValue()}}.Encode()
		}
		out = append(out, UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props})
	}
	return out, nil
}

// lowerTotal turns a Metric bound with `rows: total` into one Metric whose `value` is the Measure's figure over
// the whole Dataset. There is no Dimension value to name it, so the author **writes** `label` -- the opposite of
// dimension mode, where writing it is the fault -- and `value` is the one thing they may not write.
//
// `tone:` is a **signal**, not a colour: it is drawn only when the figure is not zero, so "Overdue" is red when
// something is overdue and plain when nothing is. A tone that stayed on a zero would cry wolf on exactly the
// page's good news. (A figure the resolver formats as "0" is the zero; the resolver, not this, owns formatting.)
func lowerTotal(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	b := n.Binding
	if b.Measure == "" {
		return nil, fmt.Errorf("%s: a binding names a dataset and a measure", path)
	}
	if len(n.Children) > 0 {
		return nil, fmt.Errorf("%s: a bound node holds no children", path)
	}
	if len(n.From) > 0 {
		return nil, fmt.Errorf("%s: from: is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageRowsRecords)
	}
	if n.To != "" || n.Param != "" {
		return nil, fmt.Errorf("%s: to:/param: carry a row's value into a destination, and rows: %s has no rows", path, domain.PageRowsTotal)
	}
	if _, ok := n.Props["value"]; ok {
		return nil, fmt.Errorf("%s: \"value\" comes from the binding, so it may not also be written on the node", path)
	}
	if n.Props["label"] == "" {
		return nil, fmt.Errorf("%s: rows: %s has no Dimension value to name the figure, so the node writes its own label", path, domain.PageRowsTotal)
	}
	if r.Total == nil {
		return nil, fmt.Errorf("%s: no resolver for a total", path)
	}
	value, err := r.Total(*b)
	if err != nil {
		return nil, err
	}
	props := make(map[string]string, len(n.Props)+1)
	for k, v := range n.Props {
		props[k] = v
	}
	props["value"] = value
	if figureIs(value, 0) {
		delete(props, "tone")
	}
	if h := n.HintFigure; h != nil {
		hint, err := lowerHintFigure(*h, *b, props, r, path)
		if err != nil {
			return nil, err
		}
		props["hint"] = hint
	}
	return []UINode{{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}}, nil
}

// lowerHintFigure is a total Metric's `hint_figure: {of, one, other}`: the hint worded around another Measure of
// the same Dataset, its figure substituted for `{n}`. The number is the binding's, never typed, so `hint:` may not
// also be written; and the figure is asked of the same resolver as the Metric's own value.
func lowerHintFigure(h domain.PageCount, b domain.PageBinding, props map[string]string, r Resolver, path string) (string, error) {
	if _, written := props["hint"]; written {
		return "", fmt.Errorf("%s: hint comes from hint_figure:, so it may not also be written", path)
	}
	if h.Of == "" {
		return "", fmt.Errorf("%s: hint_figure: of: names the Measure whose figure the hint carries", path)
	}
	if !strings.Contains(h.Other, domain.PageCountToken) {
		return "", fmt.Errorf("%s: hint_figure.other must carry %s, or the number is never shown", path, domain.PageCountToken)
	}
	figure, err := r.Total(domain.PageBinding{Dataset: b.Dataset, Measure: h.Of, Rows: domain.PageRowsTotal})
	if err != nil {
		return "", err
	}
	words := h.Other
	if figureIs(figure, 1) && h.One != "" {
		words = h.One
	}
	return strings.ReplaceAll(words, domain.PageCountToken, figure), nil
}

// lowerHideEmpty consumes `hide_empty` and reports whether the Collection is to be left out altogether: the author
// asked for that and the Dataset listed no records. The prop never reaches the lowered node (the outcome travels, not
// the claim -- lowerComplete's rule), and it cannot sit beside `empty`, which would be two answers to one question.
func lowerHideEmpty(n domain.PageNode, props map[string]string, records int, path string) (bool, map[string]string, error) {
	v, written := n.Props[collectionHideEmptyProp]
	if !written {
		return false, props, nil
	}
	if v != "true" && v != "false" {
		return false, nil, fmt.Errorf("%s: %s: %q is not true or false", path, collectionHideEmptyProp, v)
	}
	if _, both := n.Props[collectionEmptyProp]; both {
		return false, nil, fmt.Errorf("%s: %s: and %s: answer the same question (what a list that shows nothing does) in opposite ways; a Collection has one", path, collectionHideEmptyProp, collectionEmptyProp)
	}
	out := make(map[string]string, len(props))
	for k, val := range props {
		if k != collectionHideEmptyProp {
			out[k] = val
		}
	}
	return v == "true" && records == 0, out, nil
}

func lowerRecords(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	b := n.Binding
	if b.Measure != "" {
		return nil, fmt.Errorf("%s: a records binding takes no measure -- it lists records, it does not aggregate them", path)
	}
	if len(n.From) > 0 {
		return nil, fmt.Errorf("%s: from: is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageRowsRecords)
	}
	if n.To != "" || n.Param != "" {
		return nil, fmt.Errorf("%s: to:/param: belong on a Metric bound with rows: %s; a records-bound Collection links through its item template", path, domain.PageRowsDimension)
	}
	if b.Group != "" {
		return lowerGrouped(n, r, path)
	}
	if len(n.Children) != 1 {
		return nil, fmt.Errorf("%s: a records-bound collection holds exactly one child, the item template cloned for each record, and has %d", path, len(n.Children))
	}
	if r.Records == nil {
		return nil, fmt.Errorf("%s: no resolver for records", path)
	}
	set, err := r.Records(*b)
	if err != nil {
		return nil, err
	}
	props, err := lowerComplete(n, set, path)
	if err != nil {
		return nil, err
	}
	hidden, props, err := lowerHideEmpty(n, props, len(set.Records), path)
	if err != nil || hidden {
		return nil, err
	}
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}
	for i := range set.Records {
		items, err := lowerRecordItem(n.Children[0], r, fmt.Sprintf("%s.children[0]", path), set, i)
		if err != nil {
			return nil, err
		}
		out.Children = append(out.Children, items...)
	}
	return []UINode{out}, nil
}

// lowerRecordItem clones the item template for set.Records[i], with the writes that record is offered.
func lowerRecordItem(tmpl domain.PageNode, r Resolver, itemPath string, set RecordSet, i int) ([]UINode, error) {
	rec := set.Records[i]
	if rec == nil {
		rec = map[string]string{} // lower reads non-nil as "inside a template"
	}
	writes := &RecordWrites{Done: set.Done, Reopen: set.Reopen, ShapeOnly: set.ShapeOnly}
	if i < len(set.Edits) {
		writes.Edit = &set.Edits[i]
	}
	if i < len(set.Deletes) {
		writes.Delete = &set.Deletes[i]
	}
	if i < len(set.Moves) {
		writes.Move = &set.Moves[i]
	}
	if i < len(set.Transitions) {
		writes.Transition = &set.Transitions[i]
	}
	if i < len(set.Tags) {
		writes.Tags = set.Tags[i]
	}
	items, err := lower(tmpl, r, itemPath, rec, writes)
	if err != nil {
		return nil, fmt.Errorf("record %d: %w", i, err)
	}
	return items, nil
}

// bucketGap is the space inside a bucket between its heading and its list.
const bucketGap = string(domain.GapTight)

// SectionCurrentProp, "true" on a bucket's section when the bucket is the one the request is on (today, for a day
// group). Lowering produces it from Bucket.Current; an author never writes it.
const SectionCurrentProp = "current"

// The properties lowering alone produces for a grouped list's draggable cards: a section's `outside` (a day added to make a whole week) and the
// Field and value a drop on it writes, and a stack's `move` (the patch route of the card it holds). A page never writes one -- it cannot know which
// day is outside the window, or what the viewer may move -- and a written one is refused as `current` is.
const (
	SectionOutsideProp   = "outside"
	SectionDropFieldProp = "dropField"
	SectionDropValueProp = "dropValue"
	StackMoveProp        = "move"
	// SpanProp, "all" on a node a grouped list adds after its days: it takes the whole row of the grid around it (a notice about the
	// grid, not a day of it). Produced by lowering and never written.
	SpanProp = "span"
	// SectionLabelProp is a day's whole name ("Sun 18 Oct") on the section a card may be dropped on, for the move script to say where a card
	// is going: the heading is the author's and may show only a part of it (the weekday).
	SectionLabelProp = "dropLabel"
)

// lowerGrouped expands a records Collection whose binding names `group:` (domain.PageGroupDate) into one `section`
// per bucket, **as siblings in the parent** -- the way a bound Metric expands into one Metric per row -- so the
// layout around it (a seven-column grid for a week, a stack for an agenda) is the author's and the primitive
// carries none. Each section holds the bucket's heading, lowered from the Collection's first child with `from:
// bucket:label`, then a Collection of the bucket's records lowered from its second child, which keeps the node's
// own presentation (`empty`, `ordered`, `divided`, `gap`) so a day with nothing says so in the author's words.
//
// Two things a grouped Collection does not take, refused by name and not ignored. `complete:` is the claim that a
// list shows every record its Dataset matches, which a window is by construction -- so the notice is produced
// whenever the bound bit (see below) and an author's claim would be a second statement of it; `hide_empty:` would drop the empty buckets, which are the reason for grouping by a window. Neither is built.
func lowerGrouped(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	b := n.Binding
	if b.Group != domain.PageGroupDate {
		return nil, fmt.Errorf("%s: group: %q is not a dimension a records binding groups by (%s)", path, b.Group, domain.PageGroupDate)
	}
	for _, p := range []string{collectionCompleteProp, collectionHideEmptyProp, collectionTruncatedProp} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: is not built for a grouped Collection -- the notice has no place among the groups, and an empty group is drawn on purpose", path, p)
		}
	}
	if len(n.Children) != 2 {
		return nil, fmt.Errorf("%s: a grouped collection holds exactly two children, the heading template (from: %s) then the item template cloned for each record, and has %d", path, domain.PageBucketLabelRole, len(n.Children))
	}
	if err := recordlessTemplateIssue(n.Children[0], fmt.Sprintf("%s.children[0]", path), "a group heading"); err != nil {
		return nil, err
	}
	if r.Records == nil {
		return nil, fmt.Errorf("%s: no resolver for records", path)
	}
	set, err := r.Records(*b)
	if err != nil {
		return nil, err
	}
	if len(set.BucketKeys) != len(set.Records) {
		return nil, fmt.Errorf("%s: the resolver named %d bucket keys for %d records", path, len(set.BucketKeys), len(set.Records))
	}
	byKey := make(map[string]int, len(set.Buckets))
	for i, bk := range set.Buckets {
		if _, dup := byKey[bk.Key]; dup {
			return nil, fmt.Errorf("%s: bucket %q is named twice", path, bk.Key)
		}
		byKey[bk.Key] = i
	}
	members := make([][]int, len(set.Buckets))
	left, lastLoaded := 0, -1
	for i, key := range set.BucketKeys {
		if key == "" {
			// The resolver could not place it in a day (its date is not one): left out, and counted for the notice below.
			left++
			continue
		}
		at, ok := byKey[key]
		if ok && at > lastLoaded {
			lastLoaded = at
		}
		if !ok {
			return nil, fmt.Errorf("record %d: sits in bucket %q, which the group does not draw -- the window comes from the Dataset's own where:, so a record outside it is a resolver defect", i, key)
		}
		members[at] = append(members[at], i)
	}
	if len(set.Buckets) == 0 {
		// A Dataset that filters on a request parameter the request did not send lists nothing and has no window to
		// draw days from (composition.bindingRecords fails it closed). The list is drawn empty, so the author's
		// `empty:` words are what the viewer sees, as they are for an ungrouped list.
		return []UINode{{Kind: NodeKind(n.Kind), Type: n.Type, Props: n.Props}}, nil
	}
	out := make([]UINode, 0, len(set.Buckets))
	for at, bk := range set.Buckets {
		cell := UINode{Kind: NodeLayout, Type: string(domain.LayoutSection), Props: map[string]string{"gap": bucketGap}}
		if bk.Current {
			cell.Props[SectionCurrentProp] = "true"
		}
		if bk.Outside {
			cell.Props[SectionOutsideProp] = "true"
		} else if set.DropField != "" {
			// A day of the window is a place a card can be dropped: the Field it writes and the value, both for the script to read.
			cell.Props[SectionDropFieldProp], cell.Props[SectionDropValueProp], cell.Props[SectionLabelProp] = set.DropField, bk.Key, bk.Label
		}
		head, err := lower(n.Children[0], r, fmt.Sprintf("%s.children[0]", path), map[string]string{
			domain.PageBucketLabelRole: bk.Label, domain.PageBucketDayRole: bk.Day, domain.PageBucketDateRole: bk.Date, domain.PageBucketNumberRole: bk.Number,
		}, &RecordWrites{})
		if err != nil {
			return nil, fmt.Errorf("bucket %q: %w", bk.Key, err)
		}
		cell.Children = append(cell.Children, head...)
		if bk.Outside {
			// A day added only to make a whole week is outside the Dataset's window, so it was not asked for what is due on it: it
			// shows its date and nothing else, and does not claim (with the author's `empty:` words) that nothing is due. On a phone
			// it is not drawn at all (the renderer), since a column of days that say nothing is the stack's whole length.
			out = append(out, cell)
			continue
		}
		list := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: n.Props}
		for _, i := range members[at] {
			items, err := lowerRecordItem(n.Children[1], r, fmt.Sprintf("%s.children[1]", path), set, i)
			if err != nil {
				return nil, err
			}
			// A card the viewer may move is wrapped in the one node that says so (`move:`, its patch route); the renderer draws the gesture.
			if i < len(set.Drags) && set.Drags[i].Permitted && len(items) > 0 {
				items = []UINode{{Kind: NodeLayout, Type: string(domain.LayoutStack), Props: map[string]string{"gap": bucketGap, StackMoveProp: set.Drags[i].Route}, Children: items}}
			}
			list.Children = append(list.Children, items...)
		}
		cell.Children = append(cell.Children, list)
		out = append(out, cell)
	}
	// A window is complete by construction -- the Dataset selects every record between its two bounds -- so a bound that bit is a
	// week quietly missing tasks, and no author's claim is needed to say so. The notice is one node under the whole grid, not a
	// line inside the first day (which may be a quiet day outside the window and says nothing of which days lost cards): it
	// names the bound and the last day any record was loaded for, from which cards may be missing, since the cut falls on the
	// latest dates.
	var notes []string
	if set.Truncated && set.Limit > 0 {
		from := ""
		if lastLoaded >= 0 {
			from = fmt.Sprintf("; days from %s on may be missing cards", set.Buckets[lastLoaded].Label)
		}
		notes = append(notes, fmt.Sprintf("Showing the first %d only — more match than fit in this window%s.", set.Limit, from))
	}
	if left > 0 {
		who := "records hold a date that is not a day and are"
		if left == 1 {
			who = "record holds a date that is not a day and is"
		}
		notes = append(notes, fmt.Sprintf("%d %s not shown here.", left, who))
	}
	if len(notes) > 0 {
		out = append(out, UINode{Kind: NodeComponent, Type: string(domain.ComponentCallout), Props: map[string]string{"text": strings.Join(notes, " "), "tone": string(domain.ToneWarn), SpanProp: "all"}})
	}
	return out, nil
}

// dimensionRoute resolves a bound Metric's `to:` to the route its rows link to, or "" when the node links
// nowhere. A link carries a value, so `to:` and `param:` are one declaration: either alone is a fault, because a
// destination with no parameter would be a link to the same page for every row, and a parameter with no
// destination would be sent nowhere. A typed `href` is refused as a link's is -- a route is declared once.
func dimensionRoute(n domain.PageNode, r Resolver, path string) (string, error) {
	if _, written := n.Props["href"]; written {
		return "", fmt.Errorf("%s: a Metric's destination is to: <navigation item id> with param:; never a typed href -- a route is declared once (001 #3, #8)", path)
	}
	if n.To == "" && n.Param == "" {
		return "", nil
	}
	if n.To == "" || n.Param == "" {
		return "", fmt.Errorf("%s: a bound Metric that links names both to: <navigation item id> and param: <query parameter>", path)
	}
	if r.Route == nil {
		return "", fmt.Errorf("%s: no resolver for routes", path)
	}
	route, _, ok := r.Route(n.To)
	if !ok {
		return "", fmt.Errorf("%s: to: %s names no navigation item in this Application", path, n.To)
	}
	return route, nil
}

// Form properties lowering produces and an author never writes. The route a form sends to and the verb it uses
// come from the bound Machine, and a typed one would be a route retyped, and would let a page aim a form at
// anything (001 #3, #8).
const (
	formActionProp = "action"
	formMethodProp = "method"
)

// checkFormNode is what any bound Form asks of what its author wrote, whichever mode it is bound in: a
// writable component in the mode it takes, no read-side keys, no children, and no property lowering derives.
func checkFormNode(n domain.PageNode, path string) error {
	b := n.Binding
	modes, ok := domain.WritableComponents[domain.ComponentType(n.Type)]
	if NodeKind(n.Kind) != NodeComponent || !ok {
		return fmt.Errorf("%s: write: is only valid on a writable component, and %s %q is not one", path, n.Kind, n.Type)
	}
	if !slices.Contains(modes, b.Write) {
		return fmt.Errorf("%s: component %q takes a binding with write: %s, not %q", path, n.Type, strings.Join(modes, " or "), b.Write)
	}
	if b.Rows != "" || b.Measure != "" {
		return fmt.Errorf("%s: write: is a mode of its own and takes neither rows: nor measure: -- a write acts on a record, it does not read a list", path)
	}
	if len(n.Children) > 0 {
		return fmt.Errorf("%s: a bound %s holds no children -- a Form's controls come from its Machine's Fields and a Button is a leaf", path, n.Type)
	}
	if len(n.From) > 0 || n.To != "" || n.ListOf != "" || n.Param != "" || n.Count != nil {
		return fmt.Errorf("%s: a bound write takes only its words and its binding; from:, to:, list_of:, param: and count: belong to other nodes", path)
	}
	if _, typed := n.Props["then"]; typed && b.Write != domain.PageWriteCreate {
		return fmt.Errorf("%s: then: opens the record a create Form made, so it belongs on write: %s only", path, domain.PageWriteCreate)
	}
	// multipart and carries are lowered from the Fields a form asks for and the edge it takes; a typed one would plant a
	// field the form never asks (the review of step 2 wrote `carries: fld_term_status=Paid` on a create form).
	for _, derived := range []string{formActionProp, formMethodProp, "multipart", "carries"} {
		if _, typed := n.Props[derived]; typed {
			return fmt.Errorf("%s: %s is derived from the bound Machine, never typed -- a route is declared once (001 #3, #8)", path, derived)
		}
	}
	return nil
}

// lowerForm turns a `Form` bound with `write: create` into `Form{ Field{ Input }... }`, one Field per control the
// Machine asks for. The author writes the submit label and the Dataset; everything a hand-written form would
// type -- the route, each `name=`, each control's kind, its options, its default -- comes from the Machine, which
// is what 007 §11.3 means by a Binding.
//
// A viewer who may not create gets **no node**, and a page whose only child is such a Form is then a layout with
// no children, which `Validate` refuses at request time as it refuses any other empty layout.
func lowerForm(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	b := n.Binding
	if b.Write != domain.PageWriteCreate {
		return nil, fmt.Errorf("%s: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", path, b.Write, domain.PageRowsRecords)
	}
	if b.Dataset == "" {
		return nil, fmt.Errorf("%s: a binding names a dataset", path)
	}
	if r.Form == nil {
		return nil, fmt.Errorf("%s: no resolver for forms", path)
	}
	spec, err := r.Form(*b)
	if err != nil {
		return nil, err
	}
	return buildForm(n, path, spec, domain.PageWriteCreate, b.Dataset)
}

// lowerUpdateForm turns a `Form` bound with `write: update` into the same tree, patching the item's own record.
// It takes no dataset: the record is the Collection's current one and the Machine is the Collection's, so a
// dataset here could only name a different one -- a join -- and is refused. What the record's route is, whether
// this viewer may edit it and what each control starts as were decided by the resolver when it listed the
// records, which is why this reads `edit` and asks nothing.
func lowerUpdateForm(n domain.PageNode, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("%s: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageWriteUpdate, domain.PageRowsRecords)
	}
	if n.Binding.Dataset != "" {
		return nil, fmt.Errorf("%s: write: %s takes no dataset -- it edits the record of the Collection it sits in, and a second dataset would be a join the page has no grammar for", path, domain.PageWriteUpdate)
	}
	if writes == nil || writes.Edit == nil {
		return nil, fmt.Errorf("%s: the Collection's resolver supplied no update form for this record", path)
	}
	// `fields:` narrows the controls to the ones it names, in its order; the record already holds every value it leaves out.
	// (A narrowing that names none of the controls the resolver gave is the load-time placeholder's, which has one stand-in control: the
	// loader has already checked the names against the real Machine, so a real resolver never takes this path.)
	spec := *writes.Edit
	if picked := domain.SelectInputs(spec.Inputs, n.Binding.FormFields); len(picked) > 0 {
		spec.Inputs = picked
	}
	return buildForm(n, path, spec, domain.PageWriteUpdate, "the record's Machine")
}

// Button properties a delete Button's lowering derives. `confirm` is the one thing an author writes beyond the
// label: the sentence a person reads before the record is gone.
const buttonConfirmProp = "confirm"

// lowerDeleteButton turns a `Button` bound with `write: delete` into a Button that sends `DELETE` to the item's
// own record route. Like `update` it takes no dataset and is valid only inside a records template. **`confirm:`
// is required**: a destructive request that names no consequence is the one thing a page must not be able to
// declare, and all three hand-written delete sites it replaces confirm. A viewer the resolver does not permit
// gets no node.
func lowerDeleteButton(n domain.PageNode, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("%s: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageWriteDelete, domain.PageRowsRecords)
	}
	if n.Binding.Dataset != "" {
		return nil, fmt.Errorf("%s: write: %s takes no dataset -- it deletes the record of the Collection it sits in, and a second dataset would be a join the page has no grammar for", path, domain.PageWriteDelete)
	}
	if n.Props[buttonConfirmProp] == "" {
		return nil, fmt.Errorf("%s: a delete Button states what it will do in confirm: -- a destructive request with no consequence named is not declarable", path)
	}
	for _, p := range []string{"name", "value"} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a delete Button sends a request and posts no %s/value pair", path, p)
		}
	}
	if writes == nil || writes.Delete == nil {
		return nil, fmt.Errorf("%s: the Collection's resolver supplied no delete for this record", path)
	}
	if !writes.Delete.Permitted {
		return nil, nil
	}
	props := make(map[string]string, len(n.Props)+2)
	for k, v := range n.Props {
		props[k] = v
	}
	props[formActionProp] = writes.Delete.Route
	props[formMethodProp] = domain.ButtonMethodDelete
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
}

// lowerPrintButton turns a `Button` bound with `write: print` into a Button that opens the browser's print dialog. It
// writes nothing, so it names no Dataset, takes no record and asks nothing; it is valid anywhere on a page.
func lowerPrintButton(n domain.PageNode, path string) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if n.Binding.Dataset != "" {
		return nil, fmt.Errorf("%s: write: %s takes no dataset -- it prints the page", path, domain.PageWritePrint)
	}
	for _, p := range []string{"name", "value", "confirm", "action", "method", buttonDescribesProp} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a print Button sends no request, so it takes no %s", path, p)
		}
	}
	if len(n.From) > 0 {
		return nil, fmt.Errorf("%s: a print Button reads nothing from a record", path)
	}
	props := make(map[string]string, len(n.Props)+1)
	for k, v := range n.Props {
		props[k] = v
	}
	props[formMethodProp] = domain.ButtonMethodPrint
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
}

// lowerPDFButton turns a `Button` bound with `write: pdf` into a Button that downloads the page as a PDF file. Like a
// print Button it names no Dataset, takes no record and asks nothing. A host that cannot print (Resolver.PDF answers
// false) gets no node: a button that cannot work is not offered. Nil Resolver.PDF is the load-time placeholder, which draws
// the button so its shape is still validated.
func lowerPDFButton(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if n.Binding.Dataset != "" {
		return nil, fmt.Errorf("%s: write: %s takes no dataset -- it outputs the page", path, domain.PageWritePDF)
	}
	for _, p := range []string{"name", "value", "confirm", "action", "method", buttonDescribesProp} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a pdf Button asks for the page's own address, so it takes no %s", path, p)
		}
	}
	if len(n.From) > 0 {
		return nil, fmt.Errorf("%s: a pdf Button reads nothing from a record", path)
	}
	if r.PDF != nil && !r.PDF() {
		return nil, nil
	}
	props := make(map[string]string, len(n.Props)+1)
	for k, v := range n.Props {
		props[k] = v
	}
	props[formMethodProp] = domain.ButtonMethodPDF
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
}

// buttonDirectionProp is the one thing an author writes on a move Button beyond its label: which way it sends the
// record. Lowering consumes it into the derived route, so it never reaches the renderer.
const buttonDirectionProp = "direction"

// lowerMoveButton turns a `Button` bound with `write: move` into a Button that sends `POST` to the item's own move
// route, one step in `direction:`. It takes no dataset and is valid only inside a records template, like the other
// record writes. It asks nothing (`confirm:` is refused: a move is undone by the opposite button), and the end of
// the list that has nowhere to go gets no node, as does a viewer the resolver does not permit.
func lowerMoveButton(n domain.PageNode, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("%s: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageWriteMove, domain.PageRowsRecords)
	}
	if n.Binding.Dataset != "" {
		return nil, fmt.Errorf("%s: write: %s takes no dataset -- it moves the record of the Collection it sits in, and a second dataset would be a join the page has no grammar for", path, domain.PageWriteMove)
	}
	dir := n.Props[buttonDirectionProp]
	if dir != domain.MoveUp && dir != domain.MoveDown {
		return nil, fmt.Errorf("%s: a move Button names direction: %s or %s, not %q", path, domain.MoveUp, domain.MoveDown, dir)
	}
	for _, p := range []string{"name", "value", "confirm"} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a move Button sends a request and asks nothing, so it takes no %s", path, p)
		}
	}
	if writes == nil || writes.Move == nil {
		return nil, fmt.Errorf("%s: the Collection's resolver supplied no move for this record", path)
	}
	if (dir == domain.MoveUp && !writes.Move.Up) || (dir == domain.MoveDown && !writes.Move.Down) {
		return nil, nil
	}
	props := make(map[string]string, len(n.Props)+1)
	for k, v := range n.Props {
		if k != buttonDirectionProp {
			props[k] = v
		}
	}
	props[formActionProp] = domain.RecordMoveRoute(writes.Move.Route, dir)
	props[formMethodProp] = domain.ButtonMethodPost
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
}

// buttonBecomesProp is the one thing an author writes on a transition Button beyond its label: the status it moves
// the record to. Lowering consumes it into the derived name/value pair, so it never reaches the renderer.
const buttonBecomesProp = "becomes"

// buttonDescribesProp is the one property a transition Button may take from its record: the record's name, spent on
// the rendered `aria-label` so the Buttons down a list are not all called the same thing.
const buttonDescribesProp = "describes"

// lowerTransitionButton turns a `Button` bound with `write: transition` into a Button that sends `PATCH` of the
// record's status Field to the value `becomes:` names: an option of that Field, or `$done` / `$reopen`, which are
// read from the Machine's `completion:` so a page never retypes which status means finished. It takes no dataset
// and is valid only inside a records template. It asks nothing (`confirm:` is refused -- it is undone by the opposite
// transition), and a record the move is not allowed on, or a viewer the resolver does not permit, gets no node.
// The Field is the Machine's, never typed: `name:` and `value:` are refused as a typed route is.
func lowerTransitionButton(n domain.PageNode, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	from := n.From
	n.From = nil
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	for prop := range from {
		if prop != buttonDescribesProp {
			return nil, fmt.Errorf("%s: a transition Button takes only from: {%s: <role>}, the record's own name for assistive technology; %q is not an input it reads from a record", path, buttonDescribesProp, prop)
		}
	}
	if record == nil {
		return nil, fmt.Errorf("%s: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageWriteTransition, domain.PageRowsRecords)
	}
	if n.Binding.Dataset != "" {
		return nil, fmt.Errorf("%s: write: %s takes no dataset -- it changes the status of the record of the Collection it sits in, and %q would be a join the page has no grammar for", path, domain.PageWriteTransition, n.Binding.Dataset)
	}
	becomes := n.Props[buttonBecomesProp]
	if edge := n.Binding.Transition; edge != "" {
		return lowerEdgeButton(n, path, record, writes, from, edge)
	}
	if becomes == "" {
		return nil, fmt.Errorf("%s: a transition Button names the status it moves to in %s: -- a status option, %s or %s", path, buttonBecomesProp, domain.PageTransitionDone, domain.PageTransitionReopen)
	}
	// `confirm:` is the author's question, for a move that cannot be taken back (a contract cancelled: no edge leads out of it).
	for _, p := range []string{"name", "value"} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a transition Button's Field and value come from the Machine and %s:, so it takes no %s", path, buttonBecomesProp, p)
		}
	}
	if writes == nil || writes.Transition == nil || writes.Transition.Allows == nil {
		return nil, fmt.Errorf("%s: the Collection's resolver supplied no transition for this record", path)
	}
	t := writes.Transition
	target := becomes
	switch becomes {
	case domain.PageTransitionDone:
		target = t.Done
	case domain.PageTransitionReopen:
		target = t.Reopen
	}
	if target == "" {
		return nil, fmt.Errorf("%s: becomes: %s needs the Machine to declare a `completion:`, and this one declares none", path, becomes)
	}
	allowed, known := t.Allows(target)
	if !known {
		return nil, fmt.Errorf("%s: becomes: %q is not an option of the Machine's status Field", path, target)
	}
	if !allowed {
		return nil, nil
	}
	props := make(map[string]string, len(n.Props)+4)
	for k, v := range n.Props {
		if k != buttonBecomesProp {
			props[k] = v
		}
	}
	if len(from) > 0 {
		filled, err := fillFrom(domain.PageNode{Props: n.Props, From: from}, record, path)
		if err != nil {
			return nil, err
		}
		props[buttonDescribesProp] = filled[buttonDescribesProp]
	}
	props[formActionProp] = t.Route
	props[formMethodProp] = domain.ButtonMethodPatch
	props["name"] = t.Field
	props["value"] = target
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
}

// toggleStateProps are the properties a Toggle's lowering derives from the Machine and the record; an author writes none.
var toggleStateProps = []string{"checked", formActionProp, "name", "value", buttonBecomesProp}

// lowerToggle turns a `Toggle` bound with `write: transition` into the completion circle of one record: drawn finished when the
// record stands at the Machine's `completion:` value, and pressing it sends `PATCH` of the status Field to the *other* side --
// the Reopen value for a finished record, the Done value for an open one -- so the one declaration serves both directions
// and a page never names a status, a Field or a route. Like the transition Button it asks nothing (it is undone by
// pressing it again), takes the record's name only for assistive technology (`from: {describes: title}`), and is not drawn
// for a record or a viewer the move is not allowed on.
func lowerToggle(n domain.PageNode, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	from := n.From
	n.From = nil
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	for prop := range from {
		if prop != buttonDescribesProp {
			return nil, fmt.Errorf("%s: a Toggle takes only from: {%s: <role>}, the record's own name for assistive technology; %q is not an input it reads from a record", path, buttonDescribesProp, prop)
		}
	}
	switch {
	case record == nil:
		return nil, fmt.Errorf("%s: a Toggle acts on a record, so it is valid only inside a record template (a Table's cell:, or the item of a Collection bound with rows: %s)", path, domain.PageRowsRecords)
	case n.Binding.Dataset != "" || n.Binding.Transition != "" || n.Binding.All:
		return nil, fmt.Errorf("%s: a Toggle writes the Machine's completion on the record it sits in, so its binding is write: %s alone", path, domain.PageWriteTransition)
	case len(from) == 0:
		return nil, fmt.Errorf("%s: a Toggle is named for the record it completes, so it takes from: {%s: <role>} (an icon-only control with no name is a circle a screen reader cannot find)", path, buttonDescribesProp)
	}
	for _, p := range toggleStateProps {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: %s is derived from the Machine's `completion:` and the record, never typed", path, p)
		}
	}
	if writes == nil || writes.Transition == nil || writes.Transition.Allows == nil {
		return nil, fmt.Errorf("%s: the Collection's resolver supplied no transition for this record", path)
	}
	t := writes.Transition
	if t.Done == "" || t.Reopen == "" {
		return nil, fmt.Errorf("%s: a Toggle needs the Machine to declare a `completion:`, and this one declares none", path)
	}
	target := t.Done
	if t.Complete {
		target = t.Reopen
	}
	allowed, known := t.Allows(target)
	if !known {
		return nil, fmt.Errorf("%s: the Machine's status Field has no option %q to write", path, target)
	}
	if !allowed {
		return nil, nil
	}
	filled, err := fillFrom(domain.PageNode{Props: n.Props, From: from}, record, path)
	if err != nil {
		return nil, err
	}
	props := make(map[string]string, len(n.Props)+5)
	for k, v := range n.Props {
		props[k] = v
	}
	props[buttonDescribesProp] = filled[buttonDescribesProp]
	props["checked"] = strconv.FormatBool(t.Complete)
	props[formActionProp] = t.Route
	props["name"] = t.Field
	props["value"] = target
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentToggle), Props: props}}, nil
}

// lowerEdgeButton is a transition Button that names its declared edge (`transition: trn_contract_activate`): it moves the
// edge's own status Field to the edge's `to`, so a record page offers Activate and Close on its lifecycle Field however
// its `status` role is cast. Drawn only where the record stands at the edge's `from` and this viewer may take it; its
// label defaults to the edge's name.
func lowerEdgeButton(n domain.PageNode, path string, record map[string]string, writes *RecordWrites, from map[string]string, edge string) ([]UINode, error) {
	if _, written := n.Props[buttonBecomesProp]; written {
		return nil, fmt.Errorf("%s: a Button that names its transition takes no %s: -- the edge says where it goes", path, buttonBecomesProp)
	}
	for _, p := range []string{"name", "value"} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a transition Button's Field and value are its edge's, so it takes no %s", path, p)
		}
	}
	if writes == nil || writes.Transition == nil {
		return nil, fmt.Errorf("%s: the Collection's resolver supplied no transition for this record", path)
	}
	e, ok := writes.Transition.Edges[edge]
	if !ok {
		if writes.ShapeOnly {
			e = EdgeOffer{Name: "x", Field: "x", To: "x", Allowed: true}
		} else {
			return nil, fmt.Errorf("%s: transition: %s is not a declared edge of the record's Machine", path, edge)
		}
	}
	if !e.Allowed {
		return nil, nil
	}
	props := make(map[string]string, len(n.Props)+4)
	for k, v := range n.Props {
		props[k] = v
	}
	if props["label"] == "" {
		props["label"] = e.Name
	}
	if len(from) > 0 {
		filled, err := fillFrom(domain.PageNode{Props: n.Props, From: from}, record, path)
		if err != nil {
			return nil, err
		}
		props[buttonDescribesProp] = filled[buttonDescribesProp]
	}
	props[formActionProp] = writes.Transition.Route
	props[formMethodProp] = domain.ButtonMethodPatch
	props["name"] = e.Field
	props["value"] = e.To
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
}

// lowerNavList turns `component: NavList` with a `section:` and a `variant:` into the NavList whose rows are the
// destinations the navigation declares for that section. The page writes neither the rows nor the label nor the way
// back: they are the navigation's, so a destination is declared once and the sidebar and the hub cannot disagree. The
// row for the page's own navigation item is marked current; a row that is not built carries no href. The page-wide
// variant leaves out the rows that exist only for the sidebar.
func lowerNavList(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	if len(n.Children) > 0 || n.Binding != nil || len(n.From) > 0 {
		return nil, fmt.Errorf("%s: a NavList takes a section and a variant and nothing else: its rows are the navigation's", path)
	}
	for p := range n.Props {
		if p != "section" && p != "variant" {
			return nil, fmt.Errorf("%s: NavList takes only section: and variant:, and %q is derived from the section", path, p)
		}
	}
	section, variant := n.Props["section"], n.Props["variant"]
	if !domain.KnownNavSections[section] {
		return nil, fmt.Errorf("%s: NavList section %q is not one the navigation declares", path, section)
	}
	if !domain.KnownNavListVariants[variant] {
		return nil, fmt.Errorf("%s: NavList variant %q is not sidebar or page", path, variant)
	}
	if r.Navigation == nil {
		return nil, fmt.Errorf("%s: no resolver for a navigation list", path)
	}
	s, err := r.Navigation(section)
	if err != nil {
		return nil, err
	}
	var rows []string
	for _, e := range s.Entries {
		if e.SidebarOnly && variant != "sidebar" {
			continue
		}
		state := ""
		switch {
		case e.Placeholder:
			state = "placeholder"
		case e.ID != "" && e.ID == r.PageID:
			state = "current"
		}
		rows = append(rows, strings.Join([]string{e.Group, e.Label, e.Description, e.Href, state}, "\t"))
	}
	props := map[string]string{"label": s.Label, "variant": variant, "rows": strings.Join(rows, "\n")}
	if variant == "sidebar" {
		props["home"] = s.Home
	}
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentNavList), Props: props}}, nil
}

// lowerSegmentedControl fills a SegmentedControl's `selected` with the choice the request carried under its `query` (the first option when it
// carried none, or one that is no option) and `keep` with the request's other parameters, so choosing an option changes only its own parameter and a
// calendar's Week | Month switch leaves the project and the window where they were. The page writes the label, the parameter and the options; the
// state is the request's and a page that writes one is refused.
func lowerSegmentedControl(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	if len(n.Children) > 0 || n.Binding != nil || len(n.From) > 0 {
		return nil, fmt.Errorf("%s: a SegmentedControl takes a label, a query and its options and nothing else", path)
	}
	for _, p := range []string{"selected", "keep"} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: is produced by lowering from the request, and is never written", path, p)
		}
	}
	props := make(map[string]string, len(n.Props)+2)
	for k, v := range n.Props {
		props[k] = v
	}
	options := domain.ParseSegmentOptions(props["options"])
	if len(options) < 2 {
		return nil, fmt.Errorf("%s: a SegmentedControl offers fewer than two options (a value=Label per line), and has %d", path, len(options))
	}
	props["selected"] = options[0].Value
	if r.Query != nil {
		for _, o := range options {
			if o.Value == r.Query(props["query"]) {
				props["selected"] = o.Value
			}
		}
	}
	if keep := keepQuery(r, props["query"]); keep != "" {
		props["keep"] = keep
	}
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentSegmentedControl), Props: props}}, nil
}

// lowerSearchBox fills a SearchBox's `value` with the words the request already carried under its `param`, so the page
// never shows an empty box over a narrowed list. The page writes the label, the parameter and the hint; the value is the
// request's and a page that writes one is refused. A parameter the runtime owns is refused here as in the contract.
func lowerSearchBox(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	if len(n.Children) > 0 || n.Binding != nil || len(n.From) > 0 {
		return nil, fmt.Errorf("%s: a SearchBox takes a label, a query and a placeholder and nothing else", path)
	}
	for _, p := range []string{"value", "keep"} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: is produced by lowering from the request, and is never written", path, p)
		}
	}
	if _, written := n.Props["action"]; written {
		return nil, fmt.Errorf("%s: a SearchBox goes to a destination with to: <navigation item id>, never a typed action -- a route is declared once (001 #3, #8)", path)
	}
	param := n.Props["query"]
	if domain.ParamReserved(param) {
		return nil, fmt.Errorf("%s: SearchBox query %q is one the runtime owns", path, param)
	}
	props := make(map[string]string, len(n.Props)+1)
	for k, v := range n.Props {
		props[k] = v
	}
	if n.To != "" {
		if r.Route == nil {
			return nil, fmt.Errorf("%s: no resolver for routes", path)
		}
		route, _, ok := r.Route(n.To)
		if !ok {
			return nil, fmt.Errorf("%s: to: %s names no navigation item in this Application", path, n.To)
		}
		props["action"] = route
	} else if r.Query != nil && param != "" {
		// A box that sends its words elsewhere does not carry the ones this request had: they were for this page.
		props["value"] = r.Query(param)
		// Nor the page's filters; on this page, a search keeps the chip and the drop-down it was typed under.
		if keep := keepQuery(r, param); keep != "" {
			props["keep"] = keep
		}
	}
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentSearchBox), Props: props}}, nil
}

// lowerAction turns a Form or Button bound with `write: action` into the Form or Button that performs one of the
// runtime's named operations. The route is the runtime's (`domain.RuntimeActionRoute`), with the record the action is
// about when the node sits in a records template; the Form's controls are what the action asks, and a Button asks
// nothing and may carry the `confirm` sentence an author writes. A viewer the resolver does not permit gets no node.
func lowerAction(n domain.PageNode, r Resolver, path string, record map[string]string) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if r.Action == nil {
		return nil, fmt.Errorf("%s: no resolver for runtime actions", path)
	}
	for _, p := range []string{"name", "value", "action", "method"} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a runtime action's route comes from its name, so the node takes no %s", path, p)
		}
	}
	recordID := ""
	if record != nil {
		recordID = record[domain.PageRecordIDRole]
	}
	spec, err := r.Action(*n.Binding, recordID)
	if err != nil {
		return nil, err
	}
	if !spec.Permitted {
		return nil, nil
	}
	route := domain.RuntimeActionRoute(n.Binding.Action, recordID)
	if n.Type == string(domain.ComponentButton) {
		props := make(map[string]string, len(n.Props)+2)
		for k, v := range n.Props {
			props[k] = v
		}
		props[formActionProp] = route
		props[formMethodProp] = domain.ButtonMethodPost
		return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
	}
	if n.Type != string(domain.ComponentForm) {
		return nil, fmt.Errorf("%s: write: %s is for a Form or a Button, not %s", path, domain.PageWriteAction, n.Type)
	}
	spec.Route = route
	return buildForm(n, path, spec, domain.PageWriteAction, "the action")
}

// lowerTransitionAll turns a `Button` bound with `write: transition, all: true` into a Button that posts to the bulk
// route, which moves every record of the Dataset the viewer may move to the status `becomes:` names. It sits outside a
// template (inside one a transition already means that record), names the Dataset, asks nothing, and is not drawn when
// there is nothing for this viewer to move.
func lowerTransitionAll(n domain.PageNode, r Resolver, path string, record map[string]string) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if record != nil {
		return nil, fmt.Errorf("%s: write: %s with all: true acts on a Dataset, so it sits outside a Collection's item template", path, domain.PageWriteTransition)
	}
	if n.Binding.Dataset == "" {
		return nil, fmt.Errorf("%s: write: %s with all: true names the dataset whose records it moves", path, domain.PageWriteTransition)
	}
	becomes := n.Props[buttonBecomesProp]
	if becomes == "" {
		return nil, fmt.Errorf("%s: a transition Button names the status it moves to in %s: -- a status option, %s or %s", path, buttonBecomesProp, domain.PageTransitionDone, domain.PageTransitionReopen)
	}
	for _, p := range []string{"name", "value", "confirm", "action", "method"} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a bulk transition Button sends a request and asks nothing, and its route comes from the Dataset and %s:, so it takes no %s", path, buttonBecomesProp, p)
		}
	}
	if len(n.From) > 0 {
		return nil, fmt.Errorf("%s: from: is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageRowsRecords)
	}
	if r.TransitionAll == nil {
		return nil, fmt.Errorf("%s: no resolver for a bulk transition", path)
	}
	t, err := r.TransitionAll(*n.Binding, becomes)
	if err != nil {
		return nil, err
	}
	if t.Count == 0 {
		return nil, nil
	}
	props := make(map[string]string, len(n.Props)+2)
	for k, v := range n.Props {
		if k != buttonBecomesProp {
			props[k] = v
		}
	}
	props[formActionProp] = t.Route
	props[formMethodProp] = domain.ButtonMethodPost
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
}

// figureIs says whether a written figure is n: "0", "Rp 0" and "0.00" are all zero. A money total is written with its
// currency (the review of step 4: "Rp 0" kept a Metric's alarm tone), so the figure is read past its symbol and grouping.
func figureIs(written string, n float64) bool {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return -1
	}, written)
	v, err := strconv.ParseFloat(digits, 64)
	return err == nil && v == n
}

// lowerTransitionForm is a Form that takes a declared edge with what the edge asks (`binding: {write: transition,
// transition: trn_plan_settle}`): its inputs are the Fields the move makes required, the move itself travels as the
// edge's Field and `to`, and the form is drawn only where this viewer may take the edge from where the record is.
func lowerTransitionForm(n domain.PageNode, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("%s: a transition Form acts on a record, so it is valid only inside a record template", path)
	}
	if writes == nil || writes.Transition == nil {
		return nil, fmt.Errorf("%s: the Collection's resolver supplied no transition for this record", path)
	}
	e, ok := writes.Transition.Edges[n.Binding.Transition]
	if !ok {
		if !writes.ShapeOnly {
			return nil, fmt.Errorf("%s: transition: %s is not a declared edge of the record's Machine", path, n.Binding.Transition)
		}
		e = EdgeOffer{Field: "x", To: "x", Allowed: true, Inputs: []domain.FormInput{{FieldID: "x", Label: "x", Kind: domain.InputFile, Required: true}}}
	}
	if !e.Allowed {
		return nil, nil
	}
	spec := FormSpec{Route: writes.Transition.Route, Method: domain.FormMethodPatch, Permitted: true, Inputs: e.Inputs}
	nodes, err := buildForm(n, path, spec, domain.PageWriteTransition, "the edge")
	if err != nil || len(nodes) == 0 {
		return nodes, err
	}
	nodes[0].Props[formMethodProp] = domain.FormMethodPatch
	nodes[0].Props["carries"] = e.Field + "=" + e.To
	return nodes, nil
}

// buildForm is the tree both modes lower to. A viewer the spec does not permit gets no node.
func buildForm(n domain.PageNode, path string, spec FormSpec, mode, what string) ([]UINode, error) {
	if !spec.Permitted {
		return nil, nil
	}
	// A form for a runtime action may ask nothing (a template that declares no role): it is its submit and the route.
	if len(spec.Inputs) == 0 && mode != domain.PageWriteAction && mode != domain.PageWriteTransition {
		return nil, fmt.Errorf("%s: %s reads a Machine with no Field a form can ask for", path, what)
	}
	props := make(map[string]string, len(n.Props)+2)
	for k, v := range n.Props {
		props[k] = v
	}
	props[formActionProp] = spec.Route
	if mode == domain.PageWriteUpdate {
		props[formMethodProp] = domain.FormMethodPatch
	}
	if spec.Carries != "" {
		props["carries"] = spec.Carries
	}
	form := UINode{Kind: NodeComponent, Type: string(domain.ComponentForm), Props: props}
	for _, in := range spec.Inputs {
		if in.Kind == domain.InputFile {
			form.Props["multipart"] = "true"
		}
	}
	if spec.Document != "" {
		form.Children = append(form.Children, UINode{Kind: NodeStatic, Type: string(domain.StaticCaption), Props: map[string]string{"text": spec.Document}})
	}
	for i, in := range spec.Inputs {
		form.Children = append(form.Children, formField(formControlID(path, i), in.FieldID, in))
	}
	// Proposed child records ride the same submit: each kind is one Table -- one header row, a row per proposed record, a total
	// -- whose cells are the row's controls, named rows.<kind>.<n>.<field>, and a last tick that keeps or leaves out the row.
	for k, kind := range spec.Rows {
		if len(kind.Rows) == 0 {
			continue
		}
		form.Children = append(form.Children,
			UINode{Kind: NodeStatic, Type: string(domain.StaticSubheading), Props: map[string]string{"text": kind.Label}},
			rowsTable(path, len(spec.Inputs)+k*1000, k, kind, spec.Number))
	}
	out := []UINode{}
	if spec.Reader != nil {
		out = append(out, readerForm(path, *spec.Reader))
	}
	return append(out, form), nil
}

// rowsTable is one kind of proposed child record as a Table of controls: the first column numbers the rows, then one column per
// Field, then the keep tick. A column whose Field is a number is added up in the total row from the values proposed (a
// total of what is proposed, not of what a person then types). Each control carries its column and row as a hidden label,
// since the header is the only visible one; a proposed value's source and flag sit under its control.
func rowsTable(path string, base, k int, kind FormRows, nf domain.NumberFormat) UINode {
	first := kind.Rows[0]
	headers := []string{"#"}
	aligns := []string{string(domain.TableAlignStart)}
	for _, in := range first {
		headers = append(headers, in.Label)
		if in.Kind == domain.InputNumber {
			aligns = append(aligns, string(domain.TableAlignEnd))
		} else {
			aligns = append(aligns, string(domain.TableAlignStart))
		}
	}
	headers, aligns = append(headers, "Keep"), append(aligns, string(domain.TableAlignStart))
	sums := make([]float64, len(first))
	table := UINode{Kind: NodeComponent, Type: string(domain.ComponentTable), Props: map[string]string{
		"label": kind.Label, "framed": "true",
		tableHeadersProp: strings.Join(headers, "\n"), tableAlignsProp: strings.Join(aligns, ","),
	}}
	for r, row := range kind.Rows {
		prefix := fmt.Sprintf("%s%d.%d.", domain.ExtractRowPrefix, k, r)
		table.Children = append(table.Children, UINode{Kind: NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": fmt.Sprint(r + 1)}})
		for i, in := range row {
			if n, err := strconv.ParseFloat(in.Default, 64); err == nil && in.Kind == domain.InputNumber && i < len(sums) {
				sums[i] += n
			}
			table.Children = append(table.Children, rowCell(formControlID(path, base+r*50+i), prefix+in.FieldID, in, fmt.Sprintf("%s %d · %s", kind.Label, r+1, in.Label)))
		}
		tick := domain.FormInput{Kind: domain.InputBoolean, Default: "true"}
		table.Children = append(table.Children, controlNode(formControlID(path, base+r*50+49), prefix+domain.ExtractRowInclude, tick, fmt.Sprintf("Keep %s %d", strings.ToLower(kind.Label), r+1)))
	}
	// The count goes in the first column that is not a figure (a narrow "#" column would wrap it), else in the first.
	footer := make([]string, 1+len(first)+1)
	count, at := fmt.Sprintf("%d %s", len(kind.Rows), strings.ToLower(kind.Label)), 0
	for i, in := range first {
		if in.Kind != domain.InputNumber {
			at = 1 + i
			break
		}
	}
	footer[at] = count
	for i, in := range first {
		switch {
		case in.Money:
			footer[1+i] = domain.FormatMoney(sums[i], nf)
		case in.Kind == domain.InputNumber:
			footer[1+i] = strconv.FormatFloat(sums[i], 'f', -1, 64)
		}
	}
	table.Props[tableFooterProp] = strings.Join(footer, "\t")
	return table
}

// formField is one labelled control of a form: the Field that names it and the Input that is it, with the note and the
// flag a proposed value carries.
func formField(id, name string, in domain.FormInput) UINode {
	label := in.Label
	if in.Required {
		label += " *"
	}
	field := map[string]string{"label": label, "for": id}
	if in.Note != "" {
		field["note"] = in.Note
	}
	if in.Flag != "" {
		field["flag"] = in.Flag
	}
	return UINode{
		Kind: NodeComponent, Type: string(domain.ComponentField), Props: field,
		Children: []UINode{controlNode(id, name, in, "")},
	}
}

// controlNode is the Input a Form's control lowers to; label, when given, is its hidden name (a control under a column header).
func controlNode(id, name string, in domain.FormInput, label string) UINode {
	ctl := map[string]string{"id": id, "name": name, "kind": string(in.Kind)}
	if label != "" {
		ctl["label"] = label
	}
	if in.Required {
		ctl["required"] = "true"
	}
	if in.Default != "" {
		ctl["value"] = in.Default
	}
	if in.Fixed {
		ctl["fixed"] = "true"
	}
	if in.Money {
		ctl["money"] = "true"
	}
	if in.Kind == domain.InputSelect || in.Kind == domain.InputMultiSelect {
		ctl["options"] = strings.Join(in.Options, domain.InputOptionsSep)
	}
	if in.Reference != "" {
		lines := make([]string, 0, len(in.Choices))
		for _, c := range in.Choices {
			lines = append(lines, oneField(c.Value)+domain.InputChoiceSep+oneField(c.Label))
		}
		ctl["options"] = strings.Join(lines, domain.InputOptionsSep)
	}
	return UINode{Kind: NodeComponent, Type: string(domain.ComponentFormInput), Props: ctl}
}

// rowCell is one cell of a proposed row: its control, and under it where the value was read and the words that say a person
// should look twice.
func rowCell(id, name string, in domain.FormInput, label string) UINode {
	ctl := controlNode(id, name, in, label)
	if in.Note == "" && in.Flag == "" {
		return ctl
	}
	cell := UINode{Kind: NodeLayout, Type: string(domain.LayoutStack), Props: map[string]string{"gap": string(domain.GapTight)}, Children: []UINode{ctl}}
	if in.Flag != "" {
		cell.Children = append(cell.Children, UINode{Kind: NodeComponent, Type: string(domain.ComponentTag), Props: map[string]string{"label": in.Flag, "color": string(domain.TagAmber)}})
	}
	if in.Note != "" {
		cell.Children = append(cell.Children, UINode{Kind: NodeStatic, Type: string(domain.StaticCaption), Props: map[string]string{"text": in.Note}})
	}
	return cell
}

// readerForm is the small form that sends a document to be read: one file control, named as the runtime's reader expects,
// and the author's words on its button.
func readerForm(path string, r FormReader) UINode {
	id := formControlID(path, 9999)
	return UINode{
		Kind: NodeComponent, Type: string(domain.ComponentForm),
		Props: map[string]string{"submit": r.Read, formActionProp: r.Route, "multipart": "true"},
		Children: []UINode{{
			Kind: NodeComponent, Type: string(domain.ComponentField), Props: map[string]string{"label": r.Label, "for": id},
			Children: []UINode{{Kind: NodeComponent, Type: string(domain.ComponentFormInput), Props: map[string]string{
				"id": id, "name": domain.ExtractFileParam, "kind": string(domain.InputFile), "required": "true"}}},
		}},
	}
}

// formControlID is the DOM id a Field's label names, derived from the Form's place in the tree and the control's
// position -- deterministic (007 §4.6), unique per page, and never typed by an author.
func formControlID(path string, i int) string {
	var b strings.Builder
	b.WriteString("ctl-")
	for _, r := range path {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String() + "-" + strconv.Itoa(i)
}
