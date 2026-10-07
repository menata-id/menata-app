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

// RowResolver answers a Binding. It is injected, not imported, for the reason `knownComponents` is: this
// package is a representation (§15), not a data reader, and `conformance` forbids it `internal/data`. The
// loader passes a placeholder resolver to check a tree's *shape* without a database; the handler passes one
// that reads the Dataset.
type RowResolver func(b domain.PageBinding) ([]Row, error)

// Lower turns a declared `page:` into UI IR (007 §15.1's "Build UI IR"), expanding every Binding through
// resolve and nothing else -- it decides no layout, formats no value, and reads no data itself.
//
// **A bound node is a template for its rows.** `component: Metric` with a `binding:` becomes one Metric per
// row, each `label` the Dimension's value and `value` the Measure's, in the resolver's order. The author
// therefore writes neither `label` nor `value` on it (a hand-typed figure beside a bound one is the "number
// the author typed" this whole key exists to avoid), and the check is here so it cannot be forgotten by a
// caller. Other properties (`hint`, `tone`) are copied to every row.
//
// Faults that are about the *declaration* (a binding on a node that cannot take one, a missing dataset name,
// a bound node holding children) are returned as errors; faults about the *tree* (unknown types, cycles,
// depth) are `Validate`'s, so the five §15.3 rejections stay in one place.
func Lower(root domain.PageNode, resolve RowResolver) (UINode, error) {
	nodes, err := lower(root, resolve, "page")
	if err != nil {
		return UINode{}, err
	}
	if len(nodes) != 1 || root.Binding != nil {
		return UINode{}, fmt.Errorf("the root of a page must be one unbound node, not a binding that expands to several")
	}
	return nodes[0], nil
}

func lower(n domain.PageNode, resolve RowResolver, path string) ([]UINode, error) {
	if n.Binding != nil {
		return lowerBound(n, resolve, path)
	}
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: n.Props}
	for i, c := range n.Children {
		kids, err := lower(c, resolve, fmt.Sprintf("%s.children[%d]", path, i))
		if err != nil {
			return nil, err
		}
		out.Children = append(out.Children, kids...)
	}
	return []UINode{out}, nil
}

func lowerBound(n domain.PageNode, resolve RowResolver, path string) ([]UINode, error) {
	b := n.Binding
	if NodeKind(n.Kind) != NodeComponent || !domain.BindableComponents[domain.ComponentType(n.Type)] {
		return nil, fmt.Errorf("%s: a binding is only valid on a bindable component, and %s %q is not one", path, n.Kind, n.Type)
	}
	if b.Dataset == "" || b.Measure == "" {
		return nil, fmt.Errorf("%s: a binding names a dataset and a measure", path)
	}
	if b.Rows != domain.PageRowsDimension {
		return nil, fmt.Errorf("%s: binding rows %q is not %q, the only mode there is", path, b.Rows, domain.PageRowsDimension)
	}
	if len(n.Children) > 0 {
		return nil, fmt.Errorf("%s: a bound node is a template for its rows and holds no children", path)
	}
	for _, supplied := range []string{"label", "value"} {
		if _, ok := n.Props[supplied]; ok {
			return nil, fmt.Errorf("%s: %q comes from the binding, so it may not also be written on the node", path, supplied)
		}
	}
	rows, err := resolve(*b)
	if err != nil {
		return nil, err
	}
	out := make([]UINode, 0, len(rows))
	for _, r := range rows {
		props := make(map[string]string, len(n.Props)+2)
		for k, v := range n.Props {
			props[k] = v
		}
		props["label"], props["value"] = r.Label, r.Value
		out = append(out, UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props})
	}
	return out, nil
}
