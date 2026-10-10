package ir

import (
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
	// Done and Reopen are the Machine's `completion:` values, what the `$done`/`$reopen` of a `when:` mean ("" when
	// it declares none). ShapeOnly marks the load-time placeholder, for which every `when:` holds.
	Done, Reopen string
	ShapeOnly    bool
	Truncated    bool
	Limit        int
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
	Allows func(target string) (allowed, known bool)
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
	return (have == operand) != negate, nil
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
	Route string
	// Method is the verb the route takes (`domain.FormMethodPost`, `domain.FormMethodPatch`); empty means post.
	Method    string
	Permitted bool
	Inputs    []domain.FormInput
}

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
}

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
	if n.When != nil {
		show, err := conditionHolds(*n.When, record, writes, path)
		if err != nil || !show {
			return nil, err
		}
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteUpdate {
		return lowerUpdateForm(n, path, record, writes)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteDelete {
		return lowerDeleteButton(n, path, record, writes)
	}
	if n.Binding != nil && n.Binding.Write == domain.PageWriteMove {
		return lowerMoveButton(n, path, record, writes)
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
	if _, written := n.Props[collectionEmptyProp]; written && n.Kind == string(NodeComponent) && n.Type == string(domain.ComponentCollection) {
		return nil, fmt.Errorf("%s: %s: is the words shown when a records binding lists nothing, so it is valid only on a Collection bound with rows: %s -- a Collection that holds its own children is empty by being written empty", path, collectionEmptyProp, domain.PageRowsRecords)
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
	props, err = lowerLink(n, props, r, path)
	if err != nil {
		return nil, err
	}
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}
	for i, c := range n.Children {
		kids, err := lower(c, r, fmt.Sprintf("%s.children[%d]", path, i), record, writes)
		if err != nil {
			return nil, err
		}
		out.Children = append(out.Children, kids...)
	}
	return []UINode{out}, nil
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

func lowerLink(n domain.PageNode, props map[string]string, r Resolver, path string) (map[string]string, error) {
	isLink := n.Kind == string(NodeStatic) && n.Type == string(domain.StaticLink)
	for prop, role := range n.From {
		if role == domain.PageRecordRole && !(isLink && prop == "href") {
			return nil, fmt.Errorf("%s: from: %s names %q, which is valid only as a link's href", path, prop, role)
		}
	}
	if n.Param != "" { // lowerLink sees only unbound nodes, and a bound Metric is the only node that takes one
		return nil, fmt.Errorf("%s: param: carries a row's value into a destination, so it is valid only beside to: on a Metric bound with rows: %s", path, domain.PageRowsDimension)
	}
	if !isLink {
		if n.To != "" {
			return nil, fmt.Errorf("%s: to: is valid only on static: link, not %s %q", path, n.Kind, n.Type)
		}
		if n.ListOf != "" {
			return nil, fmt.Errorf("%s: list_of: is valid only on static: link, not %s %q", path, n.Kind, n.Type)
		}
		return props, nil
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
		if role != domain.PageRecordRole {
			return nil, fmt.Errorf("%s: a link's href may be taken from %q only, not from a Projection role %q", path, domain.PageRecordRole, role)
		}
		if _, hasText := props["text"]; !hasText {
			return nil, fmt.Errorf("%s: a link to a record has no menu label to default its text to, so it takes text from a role (from: {text: title})", path)
		}
		return props, nil
	}
	if n.To == "" {
		return nil, fmt.Errorf("%s: a link names its destination with to: <navigation item id>, list_of: <dataset id>, or from: {href: %s} inside a record template", path, domain.PageRecordRole)
	}
	if r.Route == nil {
		return nil, fmt.Errorf("%s: no resolver for routes", path)
	}
	route, label, ok := r.Route(n.To)
	if !ok {
		return nil, fmt.Errorf("%s: to: %s names no navigation item in this Application", path, n.To)
	}
	out := make(map[string]string, len(props)+2)
	for k, v := range props {
		out[k] = v
	}
	out["href"] = route
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
		if !ok {
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
	switch b.Rows {
	case domain.PageRowsRecords:
		return lowerRecords(n, r, path)
	case domain.PageRowsTotal:
		return lowerTotal(n, r, path)
	}
	return lowerDimension(n, r, path)
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
	if value == "0" {
		delete(props, "tone")
	}
	return []UINode{{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}}, nil
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
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}
	for i, rec := range set.Records {
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
		items, err := lower(n.Children[0], r, fmt.Sprintf("%s.children[0]", path), rec, writes)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", i, err)
		}
		out.Children = append(out.Children, items...)
	}
	return []UINode{out}, nil
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
	for _, derived := range []string{formActionProp, formMethodProp} {
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
	return buildForm(n, path, *writes.Edit, domain.PageWriteUpdate, "the record's Machine")
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

// lowerTransitionButton turns a `Button` bound with `write: transition` into a Button that sends `PATCH` of the
// record's status Field to the value `becomes:` names: an option of that Field, or `$done` / `$reopen`, which are
// read from the Machine's `completion:` so a page never retypes which status means finished. It takes no dataset
// and is valid only inside a records template. It asks nothing (`confirm:` is refused -- it is undone by the opposite
// transition), and a record the move is not allowed on, or a viewer the resolver does not permit, gets no node.
// The Field is the Machine's, never typed: `name:` and `value:` are refused as a typed route is.
func lowerTransitionButton(n domain.PageNode, path string, record map[string]string, writes *RecordWrites) ([]UINode, error) {
	if err := checkFormNode(n, path); err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("%s: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageWriteTransition, domain.PageRowsRecords)
	}
	if n.Binding.Dataset != "" {
		return nil, fmt.Errorf("%s: write: %s takes no dataset -- it changes the status of the record of the Collection it sits in, and %q would be a join the page has no grammar for", path, domain.PageWriteTransition, n.Binding.Dataset)
	}
	becomes := n.Props[buttonBecomesProp]
	if becomes == "" {
		return nil, fmt.Errorf("%s: a transition Button names the status it moves to in %s: -- a status option, %s or %s", path, buttonBecomesProp, domain.PageTransitionDone, domain.PageTransitionReopen)
	}
	for _, p := range []string{"name", "value", "confirm"} {
		if _, typed := n.Props[p]; typed {
			return nil, fmt.Errorf("%s: a transition Button sends a request and asks nothing, and its Field and value come from the Machine and %s:, so it takes no %s", path, buttonBecomesProp, p)
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
	props := make(map[string]string, len(n.Props)+3)
	for k, v := range n.Props {
		if k != buttonBecomesProp {
			props[k] = v
		}
	}
	props[formActionProp] = t.Route
	props[formMethodProp] = domain.ButtonMethodPatch
	props["name"] = t.Field
	props["value"] = target
	return []UINode{{Kind: NodeComponent, Type: string(domain.ComponentButton), Props: props}}, nil
}

// buildForm is the tree both modes lower to. A viewer the spec does not permit gets no node.
func buildForm(n domain.PageNode, path string, spec FormSpec, mode, what string) ([]UINode, error) {
	if !spec.Permitted {
		return nil, nil
	}
	if len(spec.Inputs) == 0 {
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
	form := UINode{Kind: NodeComponent, Type: string(domain.ComponentForm), Props: props}
	for i, in := range spec.Inputs {
		id := formControlID(path, i)
		ctl := map[string]string{"id": id, "name": in.FieldID, "kind": string(in.Kind)}
		if in.Required {
			ctl["required"] = "true"
		}
		if in.Default != "" {
			ctl["value"] = in.Default
		}
		if in.Kind == domain.InputSelect {
			ctl["options"] = strings.Join(in.Options, domain.InputOptionsSep)
		}
		form.Children = append(form.Children, UINode{
			Kind: NodeComponent, Type: string(domain.ComponentField),
			Props:    map[string]string{"label": in.Label, "for": id},
			Children: []UINode{{Kind: NodeComponent, Type: string(domain.ComponentFormInput), Props: ctl}},
		})
	}
	return []UINode{form}, nil
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
