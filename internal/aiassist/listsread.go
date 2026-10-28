package aiassist

import (
	"slices"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// listFromExisting reads an installed menu entry back as the list the assistant would have written for it, so a list can be edited in place (its columns,
// its order, who it shows, its search and chips, its add button) instead of only being made. It answers only for an entry written in the shape
// pageFor/tablePage produce -- the route `/pages/<id>`, a Dataset named `ds_<id>_list` that selects records in one order, and a page made only of the
// parts a generated list is made of -- and says nothing (nil) for anything else, a hand-written screen included, which stays carried through unchanged.
// It returns the Machine the list is over.
// ListOf is listFromExisting for a caller outside the package (the builder's screen): the Machine an installed entry lists and the list, nil when the entry is not one
// the assistant wrote.
func ListOf(navID string, page *domain.PageNode, machines map[string]*domain.Machine) (string, *GeneratedList) {
	return listFromExisting(navID, page, machines)
}

func listFromExisting(navID string, page *domain.PageNode, machines map[string]*domain.Machine) (string, *GeneratedList) {
	if page == nil {
		return "", nil
	}
	dsID := listDatasetID(navID)
	var machine *domain.Machine
	var ds *domain.Dataset
	for _, m := range machines {
		for i := range m.Datasets {
			if m.Datasets[i].ID == dsID {
				machine, ds = m, &m.Datasets[i]
			}
		}
	}
	if ds == nil || ds.Select != domain.SelectRecords || len(ds.Sort) != 1 || ds.Sort[0].By != "" || len(ds.Relations) != 0 {
		return "", nil
	}
	spec := &GeneratedList{Scope: ListScopeAll, Limit: ds.Limit, SortField: ds.Sort[0].Field, Direction: ds.Sort[0].Direction}
	var table, collection *domain.PageNode
	if !generatedPageParts(page, dsID, &table, &collection) || (table == nil) == (collection == nil) {
		return "", nil
	}
	if table != nil {
		names := map[string]string{}
		for _, f := range machine.Fields {
			names[f.ID] = f.Name
		}
		for i, c := range table.Columns {
			// The generator heads a column with its Field's name and links the first one to the record; a header or a link of a person's own is theirs to keep.
			if c.Field == "" || c.Header != names[c.Field] || (i == 0) != (c.Href == domain.PageRecordRole) {
				return "", nil
			}
			spec.Columns = append(spec.Columns, c.Field)
		}
	}
	if ds.ViewerScoped {
		spec.Scope = ListScopeMine
	}
	if ds.Where != nil {
		for _, c := range ds.Where.All {
			switch {
			case c.IfParam != "":
				return "", nil
			case c.Value == expression.SentinelCurrentUser && c.Op == expression.OpEquals && !c.Optional:
				spec.PersonField = c.Field
			case c.Value == expression.SentinelParameterPrefix+tableFilterQuery && c.Optional && c.Op == expression.OpEquals:
				spec.FilterField = c.Field
			default:
				return "", nil
			}
		}
		for _, c := range ds.Where.Any {
			if c.Value != expression.SentinelParameterPrefix+tableSearchQuery || !c.Optional || c.Op != expression.OpContains || c.IfParam != "" {
				return "", nil
			}
			spec.SearchFields = append(spec.SearchFields, c.Field)
		}
	}
	if (spec.Scope == ListScopeMine) != (spec.PersonField != "") {
		return "", nil
	}
	if bar := findPageNode(page, func(n domain.PageNode) bool {
		return n.Type == string(domain.ComponentForm) && n.Binding != nil && n.Binding.Write == "create"
	}); bar != nil {
		spec.AddLabel = bar.Props["submit"]
	}
	return machine.ID, spec
}

// generatedPageParts walks a page and says whether it is made **exactly** of the parts a generated list is made of, setting table or collection to the one that
// lists the Dataset. "Exactly" is the point: the list is written again from what is read back, so anything the reader does not model -- a property on a node, a column's
// total or header, a form's `fields:`, a row template of a person's own -- would be lost, and a page that carries one is not read as a list at all.
func generatedPageParts(n *domain.PageNode, dsID string, table, collection **domain.PageNode) bool {
	if !onlyModelled(*n) {
		return false
	}
	switch {
	case n.Kind == "layout" && n.Type == "stack":
		if !propsWithin(n.Props, "gap") {
			return false
		}
	case n.Kind == "layout" && n.Type == "row":
		if !propsWithin(n.Props, "gap", "align") {
			return false
		}
	case n.Kind == "component" && n.Type == string(domain.ComponentActionBar):
		if !propsWithin(n.Props, "label") || n.Binding != nil {
			return false
		}
	case n.Kind == "component" && n.Type == string(domain.ComponentPopover):
		if !propsWithin(n.Props, "label", "title", "trigger", "align", "density") || n.Binding != nil {
			return false
		}
	case n.Kind == "component" && n.Type == string(domain.ComponentFilterChips):
		if !propsWithin(n.Props, "label", "query", "all") || n.Binding == nil || n.Binding.Dataset != tableFilterDatasetID(strings.TrimSuffix(strings.TrimPrefix(dsID, "ds_"), "_list")) {
			return false
		}
	case n.Kind == "component" && n.Type == string(domain.ComponentSearchBox):
		if !propsWithin(n.Props, "label", "query", "placeholder") || n.Binding != nil {
			return false
		}
	case n.Kind == "component" && n.Type == string(domain.ComponentForm):
		if !propsWithin(n.Props, "submit") || n.Binding == nil || n.Binding.Write != "create" || n.Binding.Dataset != dsID || n.Binding.Rows != "" {
			return false
		}
	case n.Kind == "component" && n.Type == string(domain.ComponentTable):
		if !propsWithin(n.Props, "label", "empty", "framed") || n.Binding == nil || n.Binding.Dataset != dsID || n.Binding.Rows != "records" || *table != nil || *collection != nil {
			return false
		}
		for _, c := range n.Columns {
			if c != (domain.PageColumn{Header: c.Header, Field: c.Field, Href: c.Href}) || c.Field == "" {
				return false
			}
		}
		*table = n
	case n.Kind == "component" && n.Type == string(domain.ComponentCollection):
		if !propsWithin(n.Props, "gap", "empty") || n.Binding == nil || n.Binding.Dataset != dsID || n.Binding.Rows != "records" || *table != nil || *collection != nil || !generatedRow(n.Children) {
			return false
		}
		*collection = n
		return true
	default:
		return false
	}
	for i := range n.Children {
		if !generatedPageParts(&n.Children[i], dsID, table, collection) {
			return false
		}
	}
	return true
}

// generatedRow says whether a plain list's row template is the generator's: one row holding the title as a link to the record, then optionally the status badge and the
// person caption.
func generatedRow(children []domain.PageNode) bool {
	if len(children) != 1 || children[0].Kind != "layout" || children[0].Type != "row" || !onlyModelled(children[0]) || !propsWithin(children[0].Props) {
		return false
	}
	parts := children[0].Children
	if len(parts) == 0 || len(parts) > 3 {
		return false
	}
	want := []struct{ kind, typ string }{{"static", "link"}, {"component", string(domain.ComponentStatusBadge)}, {"static", "caption"}}
	for i, p := range parts {
		w := want[i]
		if p.Kind != w.kind || p.Type != w.typ || !onlyModelled(p) || p.Binding != nil || len(p.Children) != 0 {
			return false
		}
		switch {
		case i == 0 && !(len(p.From) == 2 && p.From["text"] == "title" && p.From["href"] == domain.PageRecordRole && len(p.Props) == 0):
			return false
		case p.Type == string(domain.ComponentStatusBadge) && !(len(p.From) == 1 && p.From["label"] == "status" && len(p.Props) == 1 && p.Props["tone"] == "info"):
			return false
		case p.Type == "caption" && !(len(p.From) == 1 && p.From["text"] == "person" && len(p.Props) == 0):
			return false
		}
	}
	return true
}

// onlyModelled says a node carries nothing but what the list reader models: its kind and type, scalar properties (judged by propsWithin), a binding, columns (judged
// per column), `from:` (judged by the caller where it matters) and children.
func onlyModelled(n domain.PageNode) bool {
	// (A Table binding carries the Fields its columns read; they are derived, so they are not judged.) Empty maps and lists are how a parsed node says "nothing declared": only a value in one of the keys the reader does not model disqualifies the node.
	if n.To != "" || n.In != "" || n.ListOf != "" || n.Count != nil || n.HintFigure != nil || n.Param != "" || n.When != nil || n.Each != "" || n.Step != nil ||
		len(n.Set) != 0 || n.Window != "" || len(n.Pass) != 0 || len(n.Counts) != 0 || len(n.Values) != 0 || len(n.Starts) != 0 || len(n.FormFields) != 0 || n.Extract != nil || n.Cell != 0 {
		return false
	}
	if b := n.Binding; b != nil {
		if b.Group != "" || b.Fill != "" || b.Moves || b.All || b.Action != "" || len(b.RelatedFields) != 0 || b.Transition != "" ||
			len(b.Starts) != 0 || len(b.FormFields) != 0 || b.Extract != nil || len(b.Values) != 0 || b.CountCut {
			return false
		}
	}
	return true
}

// propsWithin says every scalar property of a node is one of the allowed names.
func propsWithin(props map[string]string, allowed ...string) bool {
	for k := range props {
		if !slices.Contains(allowed, k) {
			return false
		}
	}
	return true
}

func findPageNode(n *domain.PageNode, match func(domain.PageNode) bool) *domain.PageNode {
	if match(*n) {
		return n
	}
	for i := range n.Children {
		if found := findPageNode(&n.Children[i], match); found != nil {
			return found
		}
	}
	return nil
}
