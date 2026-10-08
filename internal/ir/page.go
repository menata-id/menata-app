package ir

import (
	"fmt"

	"menata.app/internal/domain"
)

// Row is one resolved dimension row of a Binding: the Dimension's value and the Measure's number, already
// formatted. The resolver decides ordering and formatting; Lower only places them.
type Row struct {
	Label string
	Value string
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
type RecordResolver func(b domain.PageBinding) ([]map[string]string, error)

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

// Resolver is what Lower may ask the outside world. Any part may be nil when the tree has no use of it (no
// Binding of that mode, no `to:`); asking for one that is nil is an error rather than a panic.
type Resolver struct {
	Rows    RowResolver
	Records RecordResolver
	Route   RouteResolver
}

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
	nodes, err := lower(root, r, "page", nil)
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
func lower(n domain.PageNode, r Resolver, path string, record map[string]string) ([]UINode, error) {
	if n.Binding != nil {
		if record != nil {
			return nil, fmt.Errorf("%s: a binding inside a record template is not supported -- the item is one record, and a second Dataset would be a join the page has no grammar for", path)
		}
		return lowerBound(n, r, path)
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
	props, err := lowerLink(n, props, r, path)
	if err != nil {
		return nil, err
	}
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}
	for i, c := range n.Children {
		kids, err := lower(c, r, fmt.Sprintf("%s.children[%d]", path, i), record)
		if err != nil {
			return nil, err
		}
		out.Children = append(out.Children, kids...)
	}
	return []UINode{out}, nil
}

// lowerLink resolves a `static: link`'s destination. A link names its target by navigation item id (`to:`) and
// never by route, so `href` is not a property an author may write: it is the one thing this step produces.
// `text` defaults to the item's own label -- the words the menu already uses for that screen -- and may be
// written (or taken `from:` a record) when the link should say something else. `to:` on any other node is
// refused rather than ignored, for the reason `from:` outside a template is.
func lowerLink(n domain.PageNode, props map[string]string, r Resolver, path string) (map[string]string, error) {
	isLink := n.Kind == string(NodeStatic) && n.Type == string(domain.StaticLink)
	if !isLink {
		if n.To != "" {
			return nil, fmt.Errorf("%s: to: is valid only on static: link, not %s %q", path, n.Kind, n.Type)
		}
		return props, nil
	}
	if n.To == "" {
		return nil, fmt.Errorf("%s: a link names its destination with to: <navigation item id>", path)
	}
	if _, written := props["href"]; written {
		return nil, fmt.Errorf("%s: a link's destination is to: <navigation item id>, never a typed href -- a route is declared once, in navigation (001 #3, #8)", path)
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
	if NodeKind(n.Kind) != NodeComponent {
		return nil, fmt.Errorf("%s: a binding is only valid on a bindable component, and %s %q is not one", path, n.Kind, n.Type)
	}
	mode, ok := domain.BindableComponents[domain.ComponentType(n.Type)]
	if !ok {
		return nil, fmt.Errorf("%s: a binding is only valid on a bindable component, and %s %q is not one", path, n.Kind, n.Type)
	}
	if b.Rows != mode {
		return nil, fmt.Errorf("%s: component %q takes a binding with rows: %s, not %q", path, n.Type, mode, b.Rows)
	}
	if b.Dataset == "" {
		return nil, fmt.Errorf("%s: a binding names a dataset", path)
	}
	if mode == domain.PageRowsRecords {
		return lowerRecords(n, r, path)
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
	rows, err := r.Rows(*b)
	if err != nil {
		return nil, err
	}
	out := make([]UINode, 0, len(rows))
	for _, row := range rows {
		props := make(map[string]string, len(n.Props)+2)
		for k, v := range n.Props {
			props[k] = v
		}
		props["label"], props["value"] = row.Label, row.Value
		out = append(out, UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props})
	}
	return out, nil
}

func lowerRecords(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	b := n.Binding
	if b.Measure != "" {
		return nil, fmt.Errorf("%s: a records binding takes no measure -- it lists records, it does not aggregate them", path)
	}
	if len(n.From) > 0 {
		return nil, fmt.Errorf("%s: from: is valid only inside the item template of a Collection bound with rows: %s", path, domain.PageRowsRecords)
	}
	if len(n.Children) != 1 {
		return nil, fmt.Errorf("%s: a records-bound collection holds exactly one child, the item template cloned for each record, and has %d", path, len(n.Children))
	}
	if r.Records == nil {
		return nil, fmt.Errorf("%s: no resolver for records", path)
	}
	records, err := r.Records(*b)
	if err != nil {
		return nil, err
	}
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: n.Props}
	for i, rec := range records {
		if rec == nil {
			rec = map[string]string{} // lower reads non-nil as "inside a template"
		}
		items, err := lower(n.Children[0], r, fmt.Sprintf("%s.children[0]", path), rec)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", i, err)
		}
		out.Children = append(out.Children, items...)
	}
	return []UINode{out}, nil
}
