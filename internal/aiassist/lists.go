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
	Dimension    string         `yaml:"dimension,omitempty"`
	Measures     []measureDoc   `yaml:"measures,omitempty"`
	Select       string         `yaml:"select,omitempty"`
	ViewerScoped bool           `yaml:"viewer_scoped,omitempty"`
	Where        *wherePredDoc  `yaml:"where,omitempty"`
	Sort         []sortFieldDoc `yaml:"sort,omitempty"`
	Limit        int            `yaml:"limit,omitempty"`
}

type measureDoc struct {
	ID        string        `yaml:"id"`
	Aggregate string        `yaml:"aggregate"`
	Where     *wherePredDoc `yaml:"where,omitempty"`
}

// The measure and the query parameter a status filter is written with.
const (
	statusMeasureID    = "msr_total"
	statusParameter    = "status"
	statusDetailSuffix = "_in_status"
)

// expandMenu replaces a ByStatus entry with the two entries it is written as: the tiles page and, right
// after it, the page a tile opens. Everything downstream (ids, files, menu nodes) then sees ordinary entries.
func expandMenu(items []GeneratedMenuItem) []GeneratedMenuItem {
	var out []GeneratedMenuItem
	for _, it := range items {
		out = append(out, it)
		if it.ByStatus != nil {
			out = append(out, GeneratedMenuItem{Label: it.ByStatus.DetailLabel, MachineID: it.MachineID, detail: it.ByStatus})
		}
	}
	return out
}

func (n GeneratedMenuItem) hasPage() bool {
	return n.List != nil || n.ByStatus != nil || n.Queue != nil || n.detail != nil
}

type wherePredDoc struct {
	Field string `yaml:"field,omitempty"`
	Op    string `yaml:"op,omitempty"`
	Value string `yaml:"value,omitempty"`
	// Optional makes a comparison against a request parameter apply only when the request carries it, so an
	// unset filter or search box lists everything.
	Optional bool           `yaml:"optional,omitempty"`
	All      []wherePredDoc `yaml:"all,omitempty"`
	Any      []wherePredDoc `yaml:"any,omitempty"`
}

type sortFieldDoc struct {
	Field     string `yaml:"field"`
	Direction string `yaml:"direction"`
}

// pageNodeDoc is the one shape the writer needs of a page node; metadata's own pageNodeDoc has a custom
// decoder and unexported fields, so (like the other docs in writer.go) this is a write-side mirror.
type pageNodeDoc struct {
	Label     string `yaml:"label,omitempty"`
	Heading   string `yaml:"heading,omitempty"`
	Summary   string `yaml:"summary,omitempty"`
	Submit    string `yaml:"submit,omitempty"`
	Text      string `yaml:"text,omitempty"`
	ListOf    string `yaml:"list_of,omitempty"`
	Variant   string `yaml:"variant,omitempty"`
	Becomes   string `yaml:"becomes,omitempty"`
	Align     string `yaml:"align,omitempty"`
	Justify   string `yaml:"justify,omitempty"`
	Divided   bool   `yaml:"divided,omitempty"`
	HideEmpty bool   `yaml:"hide_empty,omitempty"`
	Complete  bool   `yaml:"complete,omitempty"`
	// Columns is a grid's number of columns (an int) or a Table's list of columns ([]tableColumnDoc): the same
	// key means each in the loader, told apart by its shape.
	Columns     any               `yaml:"columns,omitempty"`
	Query       string            `yaml:"query,omitempty"`
	All         string            `yaml:"all,omitempty"`
	Placeholder string            `yaml:"placeholder,omitempty"`
	Framed      bool              `yaml:"framed,omitempty"`
	Mobile      int               `yaml:"mobile,omitempty"`
	To          string            `yaml:"to,omitempty"`
	Param       string            `yaml:"param,omitempty"`
	Title       string            `yaml:"title,omitempty"`
	Trigger     string            `yaml:"trigger,omitempty"`
	Density     string            `yaml:"density,omitempty"`
	Layout      string            `yaml:"layout,omitempty"`
	Static      string            `yaml:"static,omitempty"`
	Component   string            `yaml:"component,omitempty"`
	Gap         string            `yaml:"gap,omitempty"`
	Empty       string            `yaml:"empty,omitempty"`
	Tone        string            `yaml:"tone,omitempty"`
	Binding     *pageBindingDoc   `yaml:"binding,omitempty"`
	From        map[string]string `yaml:"from,omitempty"`
	Children    []pageNodeDoc     `yaml:"children,omitempty"`
}

// tableColumnDoc is one column of a Table: its header, the Field it shows, and, for the first, the record it links to.
type tableColumnDoc struct {
	Header string `yaml:"header"`
	Field  string `yaml:"field"`
	Href   string `yaml:"href,omitempty"`
}

type pageBindingDoc struct {
	Dataset string `yaml:"dataset,omitempty"`
	Measure string `yaml:"measure,omitempty"`
	Rows    string `yaml:"rows,omitempty"`
	Write   string `yaml:"write,omitempty"`
}

// listed is one new menu entry that carries a list, with the ids the writer assigned it.
type listed struct {
	NavID, DatasetID string
	Item             GeneratedMenuItem
	// DetailNavID is, for a tiles page, the page its tiles open.
	DetailNavID string
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
		switch {
		case n.List != nil:
			base += "_" + n.List.Scope
		case n.ByStatus != nil:
			base += "_by_status"
		case n.Queue != nil:
			base += "_queue"
		case n.detail != nil:
			base += statusDetailSuffix
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
		if !n.hasPage() || current[ids[i]] {
			continue
		}
		l := listed{NavID: ids[i], DatasetID: listDatasetID(ids[i]), Item: n}
		if n.ByStatus != nil && i+1 < len(ids) {
			l.DetailNavID = ids[i+1]
		}
		out = append(out, l)
	}
	return out
}

// listDatasetID derives a list's Dataset id from its navigation id, which is unique in the Workspace.
func listDatasetID(navID string) string { return "ds_" + strings.TrimPrefix(navID, "nav_") + "_list" }

// The limits of a table, and the two request parameters its search box and chips travel in.
const (
	maxTableColumns  = 8
	maxSearchFields  = 4
	tableSearchQuery = "q"
	tableFilterQuery = "filter"
)

// isTable reports whether a list is written as a Table (it names columns) rather than the plain Collection.
func (l *GeneratedList) isTable() bool { return l != nil && len(l.Columns) > 0 }

// tableFilterDatasetID and tableFilterMeasureID name what the chips of a table count by; both follow the
// navigation id, which is unique in the Workspace.
func tableFilterDatasetID(navID string) string {
	return "ds_" + strings.TrimPrefix(navID, "nav_") + "_filter"
}
func tableFilterMeasureID(navID string) string {
	return "msr_" + strings.TrimPrefix(navID, "nav_") + "_filter"
}

// tableWhere is the predicate of a table's Dataset: the viewer's own records for scope "mine", the chip's status
// when one is chosen, and any of the search fields containing the words typed. The last two are optional
// comparisons, so a request that sets neither lists every record the scope allows.
func tableWhere(spec *GeneratedList) *wherePredDoc {
	var all, any []wherePredDoc
	if spec.Scope == ListScopeMine {
		all = append(all, wherePredDoc{Field: spec.PersonField, Op: "equals", Value: "$current_user"})
	}
	if spec.FilterField != "" {
		all = append(all, wherePredDoc{Field: spec.FilterField, Op: "equals", Value: "$parameters." + tableFilterQuery, Optional: true})
	}
	for _, f := range spec.SearchFields {
		any = append(any, wherePredDoc{Field: f, Op: "contains", Value: "$parameters." + tableSearchQuery, Optional: true})
	}
	if len(all) == 0 && len(any) == 0 {
		return nil
	}
	return &wherePredDoc{All: all, Any: any}
}

// datasetFor is the Dataset a list is written as.
func datasetFor(l listed) datasetDoc {
	if st := l.Item.ByStatus; st != nil {
		return datasetDoc{ID: l.DatasetID, Dimension: st.StatusField, Measures: []measureDoc{{ID: statusMeasureID, Aggregate: "count"}}}
	}
	if st := l.Item.detail; st != nil {
		return datasetDoc{ID: l.DatasetID, Select: domain.SelectRecords, Limit: st.Limit,
			Where: &wherePredDoc{Field: st.StatusField, Op: "equals", Value: "$parameters." + statusParameter},
			Sort:  []sortFieldDoc{{Field: "created_at", Direction: "desc"}}}
	}
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
	}
	if spec.isTable() {
		ds.Where = tableWhere(spec)
	} else if spec.Scope == ListScopeMine {
		ds.Where = &wherePredDoc{Field: spec.PersonField, Op: "equals", Value: "$current_user"}
	}
	return ds
}

// pageFor is the page a list is written as: one Collection over the Dataset, one row per record. The row
// shows what the Machine's card_fields project: always the title as a link to the record, and the status
// and person when the Machine projects them. It names no Field.
func pageFor(l listed, m GeneratedMachine) pageNodeDoc {
	if l.Item.ByStatus != nil {
		return pageNodeDoc{Layout: "stack", Gap: "default", Children: []pageNodeDoc{{
			Layout: "grid", Gap: "default", Mobile: 2, Columns: 4,
			Children: []pageNodeDoc{{
				Component: "Metric", To: l.DetailNavID, Param: statusParameter,
				Binding: &pageBindingDoc{Dataset: l.DatasetID, Measure: statusMeasureID, Rows: "dimension"},
			}},
		}}}
	}
	if l.Item.List.isTable() {
		return tablePage(l, m)
	}
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
	return pageNodeDoc{Layout: "stack", Gap: "default", Children: withAddForm(l, []pageNodeDoc{{
		Component: "Collection", Gap: "tight", Empty: listEmptyWords,
		Binding:  &pageBindingDoc{Dataset: l.DatasetID, Rows: "records"},
		Children: []pageNodeDoc{row},
	}})}
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
		issues = append(issues, validateByStatus(item, machines)...)
		if item.Queue != nil {
			if m, ok := machines[item.MachineID]; ok {
				issues = append(issues, validateQueue(item, m)...)
			}
			continue
		}
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
		if l.AddLabel != "" && !oneLine(l.AddLabel) {
			issues = append(issues, fmt.Sprintf("%s: add_label must be words on one line, or left out", where))
		}
		if l.AddLabel != "" {
			// The loader refuses these at publish, after the person approved the review (writeBindingIssues); a file Field is skipped by
			// the create form, so a required one could never be submitted either.
			for _, f := range m.Fields {
				if f.Required && (domain.FieldType(f.Type) == domain.FieldTypeGroup || domain.FieldType(f.Type) == domain.FieldTypeFile) {
					issues = append(issues, fmt.Sprintf("%s: add_label puts a create form on the list, and the form cannot ask for %s (a required %s field) -- leave add_label out, or make that field optional", where, f.ID, f.Type))
				}
			}
		}
		if l.isTable() {
			issues = append(issues, validateTable(where, l, fields)...)
		} else {
			if len(l.SearchFields) > 0 || l.FilterField != "" {
				issues = append(issues, where+": search_fields and filter_field belong to a table; give columns, or leave them out")
			}
			if !slices.ContainsFunc(m.CardFields, func(cf GeneratedCardField) bool { return cf.Role == "title" }) {
				issues = append(issues, fmt.Sprintf("%s: %s needs a card_fields entry with role \"title\" -- a list draws each record by its title", where, m.ID))
			}
		}
	}
	return issues
}

// oneLine reports whether a Field's name can be drawn as a column header, a chips label or a search placeholder: the
// loader refuses a blank one or one that runs over a line, and only at publish, after the person approved the review.
func oneLine(name string) bool {
	return strings.TrimSpace(name) != "" && !strings.ContainsAny(name, "\r\n")
}

// validateTable holds a table to what it can show: 1..8 distinct columns that are Fields of the Machine and of a
// type a column can show, search over text Fields only, and chips over a status Field only with scope "all"
// (the chips count every record, which for the viewer's own list would count what the table does not show).
func validateTable(where string, l *GeneratedList, fields map[string]GeneratedField) []string {
	var issues []string
	if n := len(l.Columns); n < 1 || n > maxTableColumns {
		issues = append(issues, fmt.Sprintf("%s: a table has 1 to %d columns, not %d", where, maxTableColumns, n))
	}
	seen := map[string]bool{}
	for _, id := range l.Columns {
		f, ok := fields[id]
		switch {
		case seen[id]:
			issues = append(issues, fmt.Sprintf("%s: column %q is listed twice", where, id))
		case !ok:
			issues = append(issues, fmt.Sprintf("%s: column %q is not a field of this machine", where, id))
		case !tableColumnTypes[domain.FieldType(f.Type)]:
			issues = append(issues, fmt.Sprintf("%s: column %q is a %s field, which a table column cannot show", where, id, f.Type))

		case !oneLine(f.Name):
			issues = append(issues, fmt.Sprintf("%s: column %q has no one-line name to be its header -- name the field first", where, id))
		}
		seen[id] = true
	}
	if len(l.SearchFields) > maxSearchFields {
		issues = append(issues, fmt.Sprintf("%s: a search box looks in at most %d fields", where, maxSearchFields))
	}
	seenSearch := map[string]bool{}
	for _, id := range l.SearchFields {
		f, ok := fields[id]
		switch {
		case seenSearch[id]:
			issues = append(issues, fmt.Sprintf("%s: search field %q is listed twice", where, id))
		case !ok:
			issues = append(issues, fmt.Sprintf("%s: search field %q is not a field of this machine", where, id))
		case domain.FieldType(f.Type) != domain.FieldTypeText && domain.FieldType(f.Type) != domain.FieldTypeLongText:
			issues = append(issues, fmt.Sprintf("%s: search field %q is a %s field; a search box looks in text or long_text fields", where, id, f.Type))

		case !oneLine(f.Name):
			issues = append(issues, fmt.Sprintf("%s: search field %q has no one-line name for the search box to name it by -- name the field first", where, id))
		}
		seenSearch[id] = true
	}
	if l.FilterField != "" {
		if f, ok := fields[l.FilterField]; !ok || domain.FieldType(f.Type) != domain.FieldTypeStatus {
			issues = append(issues, fmt.Sprintf("%s: filter_field %q must be a status field of this machine", where, l.FilterField))
		} else if !oneLine(f.Name) {
			issues = append(issues, fmt.Sprintf("%s: filter_field %q has no one-line name to label its chips -- name the field first", where, l.FilterField))
		}
		if l.Scope != ListScopeAll {
			issues = append(issues, where+": filter chips count every record, so they go only on a table with scope \"all\"; for \"mine\" drop filter_field")
		}
	}
	return issues
}

// tableColumnTypes is the field types a table column shows. It is measured, not assumed: see
// TestTable_everyFieldTypeAsAColumnIsKnown.
var tableColumnTypes = map[domain.FieldType]bool{
	domain.FieldTypeText: true, domain.FieldTypeLongText: true, domain.FieldTypeNumber: true, domain.FieldTypeBoolean: true,
	domain.FieldTypeDate: true, domain.FieldTypeStatus: true, domain.FieldTypePerson: true, domain.FieldTypeMoney: true,
	domain.FieldTypeRelation: true,
	// file and group are refused by the loader as columns: a file's value is a storage key, a group is read by no page binding.
}

// validateByStatus holds a status filter to what it needs: a status field to count by, a label for the page
// a tile opens, a bounded limit, a title to draw each record by, and no list on the same entry.
func validateByStatus(item GeneratedMenuItem, machines map[string]GeneratedMachine) []string {
	st := item.ByStatus
	if st == nil {
		return nil
	}
	where := fmt.Sprintf("by_status on menu entry %q", item.Label)
	if item.List != nil {
		return []string{where + ": an entry is a list or a status filter, not both"}
	}
	m, ok := machines[item.MachineID]
	if !ok {
		return nil // validateMenu already says so
	}
	var issues []string
	isStatus := false
	for _, f := range m.Fields {
		if f.ID == st.StatusField && domain.FieldType(f.Type) == domain.FieldTypeStatus {
			isStatus = true
		}
	}
	if !isStatus {
		issues = append(issues, fmt.Sprintf("%s: status_field must be a status field of %s", where, m.ID))
	}
	if strings.TrimSpace(st.DetailLabel) == "" || strings.ContainsAny(st.DetailLabel, "\r\n") {
		issues = append(issues, where+": detail_label is the one-line menu label of the page a tile opens")
	}
	if st.Limit < 1 || st.Limit > maxListLimit {
		issues = append(issues, fmt.Sprintf("%s: limit must be between 1 and %d", where, maxListLimit))
	}
	if !slices.ContainsFunc(m.CardFields, func(cf GeneratedCardField) bool { return cf.Role == "title" }) {
		issues = append(issues, fmt.Sprintf("%s: %s needs a card_fields entry with role \"title\" -- the page a tile opens draws each record by its title", where, m.ID))
	}
	return issues
}

// tablePage is the page a table is written as: the filter chips and the search box in one row when it has either,
// then the Table over the list's Dataset. Headers are the Fields' own names; the first column links to the record.
// It names no route and no colour.
func tablePage(l listed, m GeneratedMachine) pageNodeDoc {
	spec := l.Item.List
	names := map[string]string{}
	for _, f := range m.Fields {
		names[f.ID] = f.Name
	}
	cols := make([]tableColumnDoc, len(spec.Columns))
	for i, id := range spec.Columns {
		cols[i] = tableColumnDoc{Header: names[id], Field: id}
	}
	cols[0].Href = domain.PageRecordRole
	var controls []pageNodeDoc
	if spec.FilterField != "" {
		controls = append(controls, pageNodeDoc{Component: "FilterChips", Label: names[spec.FilterField], Query: tableFilterQuery, All: "All",
			Binding: &pageBindingDoc{Dataset: tableFilterDatasetID(l.NavID), Measure: tableFilterMeasureID(l.NavID), Rows: "dimension"}})
	}
	if len(spec.SearchFields) > 0 {
		var words []string
		for _, f := range spec.SearchFields {
			words = append(words, strings.ToLower(names[f]))
		}
		controls = append(controls, pageNodeDoc{Component: "SearchBox", Label: "Search", Query: tableSearchQuery, Placeholder: "Search " + strings.Join(words, ", ")})
	}
	children := []pageNodeDoc{}
	if len(controls) > 0 {
		children = append(children, pageNodeDoc{Layout: "row", Gap: "tight", Align: "center", Children: controls})
	}
	children = append(children, pageNodeDoc{Component: "Table", Label: l.Item.Label, Empty: listEmptyWords, Framed: true,
		Columns: cols, Binding: &pageBindingDoc{Dataset: l.DatasetID, Rows: "records"}})
	return pageNodeDoc{Layout: "stack", Gap: "default", Children: withAddForm(l, children)}
}

// withAddForm puts the "add" button of a list (ActionBar > Popover > create Form, the shape a hand-written list
// page declares) before its other children. The Form names no Field: it asks for the Machine's own controls, and
// the Dataset only names the Machine it writes to.
func withAddForm(l listed, children []pageNodeDoc) []pageNodeDoc {
	if l.Item.List == nil || l.Item.List.AddLabel == "" {
		return children
	}
	label := l.Item.List.AddLabel
	bar := pageNodeDoc{Component: "ActionBar", Label: label, Children: []pageNodeDoc{{
		Component: "Popover", Label: label, Title: label, Trigger: "filled", Align: "end", Density: "regular",
		Children: []pageNodeDoc{{Component: "Form", Submit: label, Binding: &pageBindingDoc{Dataset: l.DatasetID, Write: "create"}}}}}}
	return append([]pageNodeDoc{bar}, children...)
}

// datasetsFor is every Dataset an entry's page needs: one for a list or a tile page, two for a table with chips,
// several for a queue.
func datasetsFor(l listed, m GeneratedMachine) []datasetDoc {
	if l.Item.Queue != nil {
		return queueDatasets(l, m)
	}
	out := []datasetDoc{datasetFor(l)}
	if spec := l.Item.List; spec.isTable() && spec.FilterField != "" {
		out = append(out, datasetDoc{ID: tableFilterDatasetID(l.NavID), Dimension: spec.FilterField,
			Measures: []measureDoc{{ID: tableFilterMeasureID(l.NavID), Aggregate: "count"}}})
	}
	return out
}

// pageForEntry is the page an entry is written as, whichever kind it is.
func pageForEntry(l listed, m GeneratedMachine) pageNodeDoc {
	if l.Item.Queue != nil {
		return queuePage(l, m)
	}
	return pageFor(l, m)
}
