package aiassist

import (
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
	if ds == nil || ds.Select != domain.SelectRecords || len(ds.Sort) != 1 {
		return "", nil
	}
	spec := &GeneratedList{Scope: ListScopeAll, Limit: ds.Limit, SortField: ds.Sort[0].Field, Direction: ds.Sort[0].Direction}
	var table, collection *domain.PageNode
	if !generatedPageParts(page, dsID, &table, &collection) || (table == nil) == (collection == nil) {
		return "", nil
	}
	if table != nil {
		for _, c := range table.Columns {
			if c.Field == "" {
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
			case c.Value == expression.SentinelCurrentUser && c.Op == expression.OpEquals:
				spec.PersonField = c.Field
			case c.Value == expression.SentinelParameterPrefix+tableFilterQuery && c.Optional:
				spec.FilterField = c.Field
			default:
				return "", nil
			}
		}
		for _, c := range ds.Where.Any {
			if c.Value != expression.SentinelParameterPrefix+tableSearchQuery || !c.Optional || c.Op != expression.OpContains {
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

// generatedPageParts walks a page and says whether it is made only of the parts a generated list is made of, setting table or collection to the one that
// lists the Dataset. Any other part -- a Metric, a heading a person added, a second Table -- means the page is not the generator's and is left alone.
func generatedPageParts(n *domain.PageNode, dsID string, table, collection **domain.PageNode) bool {
	switch {
	case n.Kind == "layout" && (n.Type == "stack" || n.Type == "row"):
	case n.Kind == "component" && (n.Type == string(domain.ComponentActionBar) || n.Type == string(domain.ComponentPopover) || n.Type == string(domain.ComponentFilterChips) || n.Type == string(domain.ComponentSearchBox)):
	case n.Kind == "component" && n.Type == string(domain.ComponentForm):
		if n.Binding == nil || n.Binding.Write != "create" || n.Binding.Dataset != dsID {
			return false
		}
	case n.Kind == "component" && n.Type == string(domain.ComponentTable):
		if n.Binding == nil || n.Binding.Dataset != dsID || n.Binding.Rows != "records" || *table != nil || *collection != nil {
			return false
		}
		*table = n
	case n.Kind == "component" && n.Type == string(domain.ComponentCollection):
		if n.Binding == nil || n.Binding.Dataset != dsID || n.Binding.Rows != "records" || *table != nil || *collection != nil {
			return false
		}
		*collection = n
		return true // the row it draws is the Machine's own card, not read back
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
