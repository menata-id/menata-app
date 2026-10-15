package aiassist

import (
	"fmt"
	"slices"
	"strings"

	"menata.app/internal/domain"
)

// A menu entry with a list (GeneratedMenuItem.List) is written as three things that must agree: a Dataset on
// the Machine (select: records, a required limit, and for "mine" the viewer_scoped claim with the predicate
// that makes it true), a navigation item whose route is /pages/<its id>, and a `page:` that is one Collection
// over that Dataset. Writing the claim and the predicate together is the point: the model names a person
// Field, and cannot forget the half that scopes (W5-a; the loader refuses a flagged Dataset without it).

// maxListLimit mirrors the cap a hand-written list declares; a larger window is a different capability.
const maxListLimit = 200

// listEmptyWords is what a list shows when it matches nothing. The runtime has no per-language default.
const listEmptyWords = "Nothing to show."

type datasetDoc struct {
	ID           string         `yaml:"id"`
	Select       string         `yaml:"select"`
	ViewerScoped bool           `yaml:"viewer_scoped,omitempty"`
	Where        *wherePredDoc  `yaml:"where,omitempty"`
	Sort         []sortFieldDoc `yaml:"sort,omitempty"`
	Limit        int            `yaml:"limit"`
}

type wherePredDoc struct {
	Field string `yaml:"field"`
	Op    string `yaml:"op"`
	Value string `yaml:"value"`
}

type sortFieldDoc struct {
	Field     string `yaml:"field"`
	Direction string `yaml:"direction"`
}

// pageNodeDoc is the one shape the writer needs of a page node; metadata's own pageNodeDoc has a custom
// decoder and unexported fields, so (like the other docs in writer.go) this is a write-side mirror.
type pageNodeDoc struct {
	Layout    string            `yaml:"layout,omitempty"`
	Static    string            `yaml:"static,omitempty"`
	Component string            `yaml:"component,omitempty"`
	Gap       string            `yaml:"gap,omitempty"`
	Empty     string            `yaml:"empty,omitempty"`
	Tone      string            `yaml:"tone,omitempty"`
	Binding   *pageBindingDoc   `yaml:"binding,omitempty"`
	From      map[string]string `yaml:"from,omitempty"`
	Children  []pageNodeDoc     `yaml:"children,omitempty"`
}

type pageBindingDoc struct {
	Dataset string `yaml:"dataset"`
	Rows    string `yaml:"rows"`
}

// listed is one new menu entry that carries a list, with the ids the writer assigned it.
type listed struct {
	NavID, DatasetID string
	Item             GeneratedMenuItem
}

// assignNavIDs gives every new menu entry its id before any file is written, so the Dataset a list needs
// can go into its Machine's file in the same pass. current holds the ids that already exist (an entry
// naming one keeps it); taken is every navigation id in the Workspace and is added to as ids are assigned.
// bareFirst keeps the id a generated Application has always had for its first entry ("nav_<app>").
func assignNavIDs(appID string, desired []GeneratedMenuItem, current map[string]bool, taken map[string]bool, bareFirst bool) []string {
	appName := strings.TrimPrefix(appID, "app_")
	ids := make([]string, len(desired))
	for i, n := range desired {
		if n.ID != "" && current[n.ID] {
			ids[i] = n.ID
			continue
		}
		base := "nav_" + appName + "_" + strings.TrimPrefix(n.MachineID, "mch_")
		if n.List != nil {
			base += "_" + n.List.Scope
		}
		if i == 0 && bareFirst {
			base = "nav_" + appName
		}
		id := n.ID
		if id == "" || taken[id] {
			id = uniqueNavID(base, taken)
		}
		taken[id] = true
		ids[i] = id
	}
	return ids
}

// listedItems picks out the new entries carrying a list, with their navigation ids and Dataset ids.
func listedItems(desired []GeneratedMenuItem, ids []string, current map[string]bool) []listed {
	var out []listed
	for i, n := range desired {
		if n.List == nil || current[ids[i]] {
			continue
		}
		out = append(out, listed{NavID: ids[i], DatasetID: listDatasetID(ids[i]), Item: n})
	}
	return out
}

// listDatasetID derives a list's Dataset id from its navigation id, which is unique in the Workspace.
func listDatasetID(navID string) string { return "ds_" + strings.TrimPrefix(navID, "nav_") + "_list" }

// datasetFor is the Dataset a list is written as.
func datasetFor(l listed) datasetDoc {
	spec := l.Item.List
	field, dir := spec.SortField, spec.Direction
	if field == "" {
		field = "created_at"
	}
	if dir == "" {
		dir = "desc"
	}
	ds := datasetDoc{ID: l.DatasetID, Select: domain.SelectRecords, Sort: []sortFieldDoc{{Field: field, Direction: dir}}, Limit: spec.Limit}
	if spec.Scope == ListScopeMine {
		ds.ViewerScoped = true
		ds.Where = &wherePredDoc{Field: spec.PersonField, Op: "equals", Value: "$current_user"}
	}
	return ds
}

// pageFor is the page a list is written as: one Collection over the Dataset, one row per record. The row
// shows what the Machine's card_fields project: always the title as a link to the record, and the status
// and person when the Machine projects them. It names no Field.
func pageFor(l listed, m GeneratedMachine) pageNodeDoc {
	roles := map[string]bool{}
	for _, cf := range m.CardFields {
		roles[cf.Role] = true
	}
	row := pageNodeDoc{Layout: "row", Children: []pageNodeDoc{
		{Static: "link", From: map[string]string{"text": "title", "href": domain.PageRecordRole}},
	}}
	if roles["status"] {
		row.Children = append(row.Children, pageNodeDoc{Component: "StatusBadge", Tone: "info", From: map[string]string{"label": "status"}})
	}
	if roles["person"] {
		row.Children = append(row.Children, pageNodeDoc{Static: "caption", From: map[string]string{"text": "person"}})
	}
	return pageNodeDoc{Layout: "stack", Gap: "default", Children: []pageNodeDoc{{
		Component: "Collection", Gap: "tight", Empty: listEmptyWords,
		Binding:  &pageBindingDoc{Dataset: l.DatasetID, Rows: "records"},
		Children: []pageNodeDoc{row},
	}}}
}

// validateLists holds what a list needs from the Machine it lists. The loader would refuse most of this
// too, but at publish, after a review the person already approved; refusing here lets the model correct it
// in the same conversation.
func validateLists(app *GeneratedApplication) []string {
	machines := map[string]GeneratedMachine{}
	for _, m := range app.Machines {
		machines[m.ID] = m
	}
	var issues []string
	for _, item := range app.Navigation {
		l := item.List
		if l == nil {
			continue
		}
		m, ok := machines[item.MachineID]
		if !ok {
			continue // validateMenu already says so
		}
		where := fmt.Sprintf("list on menu entry %q", item.Label)
		fields := map[string]GeneratedField{}
		for _, f := range m.Fields {
			fields[f.ID] = f
		}
		switch l.Scope {
		case ListScopeMine:
			f, ok := fields[l.PersonField]
			if !ok || domain.FieldType(f.Type) != domain.FieldTypePerson {
				issues = append(issues, fmt.Sprintf("%s: scope \"mine\" needs person_field to be a person field of %s -- ask which person field says whose record it is", where, m.ID))
			}
		case ListScopeAll:
			if l.PersonField != "" {
				issues = append(issues, fmt.Sprintf("%s: scope \"all\" takes no person_field; to show only someone's records the scope is \"mine\"", where))
			}
		default:
			issues = append(issues, fmt.Sprintf("%s: scope must be \"mine\" or \"all\", not %q -- showing a list per role is not something the runtime can do yet, so record it as a runtime gap instead", where, l.Scope))
		}
		if l.Limit < 1 || l.Limit > maxListLimit {
			issues = append(issues, fmt.Sprintf("%s: limit must be between 1 and %d", where, maxListLimit))
		}
		if l.SortField != "" && l.SortField != "created_at" && l.SortField != "updated_at" {
			if _, ok := fields[l.SortField]; !ok {
				issues = append(issues, fmt.Sprintf("%s: sort_field %q is neither a field of %s nor created_at/updated_at", where, l.SortField, m.ID))
			}
		}
		if l.Direction != "" && l.Direction != "asc" && l.Direction != "desc" {
			issues = append(issues, fmt.Sprintf("%s: direction must be \"asc\" or \"desc\"", where))
		}
		if !slices.ContainsFunc(m.CardFields, func(cf GeneratedCardField) bool { return cf.Role == "title" }) {
			issues = append(issues, fmt.Sprintf("%s: %s needs a card_fields entry with role \"title\" -- a list draws each record by its title", where, m.ID))
		}
	}
	return issues
}
