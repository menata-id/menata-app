package aiassist

import (
	"fmt"
	"slices"
	"strings"

	"menata.app/internal/domain"
)

// A queue (GeneratedMenuItem.Queue) is the page a work-queue Application needs: figures on top, then what is
// open, in sections, each row carrying the buttons that move it along. It is a closed recipe, not free-form
// page YAML: the model picks it and (optionally) a date Field and some words, and everything else -- the
// Datasets, the count measures, the Metrics, the Collections, the transition Buttons -- is derived here from the
// Machine's own status_categories, completion, transitions and card_fields. That is the point: the model cannot
// forget the half that makes the page work (a limit, a `$done` guard, a button per declared edge), and the
// Maintenance, Lending and Helpdesk pages in the default Workspace are this shape written by hand.

// Default words of a queue. The runtime has no per-language default, so the person may give their own
// (GeneratedQueueWords); an empty one reads as this.
var queueDefaultWords = GeneratedQueueWords{
	Overdue: "Overdue", DueToday: "Due today", NextWeek: "Next 7 days", Later: "Later",
	RecentlyDone: "Recently completed", Empty: "Nothing is open.", All: "All records", Add: "Add",
}

const queueRecentLimit = 5

type queueSection struct {
	Key, Heading, Hint string
	Where              *wherePredDoc
	Sort, Dir          string
	Tone               string
	Buttons            []queueButton
	Window             bool
}

type queueButton struct{ Label, Becomes, Variant string }

func queueWords(q *GeneratedQueue) GeneratedQueueWords {
	w, d := q.Words, queueDefaultWords
	pick := func(a, b string) string {
		if strings.TrimSpace(a) != "" {
			return a
		}
		return b
	}
	return GeneratedQueueWords{
		Overdue: pick(w.Overdue, d.Overdue), DueToday: pick(w.DueToday, d.DueToday), NextWeek: pick(w.NextWeek, d.NextWeek),
		Later: pick(w.Later, d.Later), RecentlyDone: pick(w.RecentlyDone, d.RecentlyDone), Empty: pick(w.Empty, d.Empty),
		All: pick(w.All, d.All), Add: pick(w.Add, d.Add),
	}
}

func statusFieldOf(m GeneratedMachine) string {
	if m.StatusCategories != nil {
		return m.StatusCategories.Field
	}
	return ""
}

func categoryOf(m GeneratedMachine, option string) string {
	if m.StatusCategories == nil {
		return ""
	}
	for _, v := range m.StatusCategories.Values {
		if v.Option == option {
			return v.Category
		}
	}
	return ""
}

// doneValue is how the Machine's own words name "finished" in a predicate: `$done` when a completion is
// declared (the runtime resolves it), otherwise the option categorised done.
func doneValue(m GeneratedMachine) string {
	if m.Completion != nil {
		return domain.PageTransitionDone
	}
	if m.StatusCategories != nil {
		for _, v := range m.StatusCategories.Values {
			if v.Category == "done" {
				return v.Option
			}
		}
	}
	return ""
}

func toneOfCategory(c string) string {
	switch c {
	case "attention":
		return "warn"
	case "done":
		return "good"
	case "stopped":
		return "bad"
	case "start", "active":
		return "info"
	}
	return "neutral"
}

func queueSections(q *GeneratedQueue, m GeneratedMachine) []queueSection {
	w := queueWords(q)
	status := statusFieldOf(m)
	if q.DateField != "" {
		open := wherePredDoc{Field: status, Op: "not_equals", Value: domain.PageTransitionDone}
		date := func(op, v string) wherePredDoc { return wherePredDoc{Field: q.DateField, Op: op, Value: v} }
		all := func(ps ...wherePredDoc) *wherePredDoc { return &wherePredDoc{All: append([]wherePredDoc{open}, ps...)} }
		var complete []queueButton
		for _, t := range m.Transitions {
			if t.Field == status && t.To == m.Completion.Done {
				complete = []queueButton{{Label: t.Name, Becomes: domain.PageTransitionDone, Variant: "secondary"}}
				break
			}
		}
		return []queueSection{
			{Key: "overdue", Heading: w.Overdue, Where: all(date("lt", "$today")), Sort: q.DateField, Dir: "asc", Tone: "bad", Buttons: complete},
			{Key: "today", Heading: w.DueToday, Where: all(date("equals", "$today")), Sort: q.DateField, Dir: "asc", Tone: "warn", Buttons: complete},
			{Key: "week", Heading: w.NextWeek, Where: all(date("gt", "$today"), date("lte", "$today+7")), Sort: q.DateField, Dir: "asc", Buttons: complete},
			{Key: "later", Heading: w.Later, Where: all(date("gt", "$today+7")), Sort: q.DateField, Dir: "asc"},
		}
	}
	var out []queueSection
	for _, v := range m.StatusCategories.Values {
		if v.Category != "start" && v.Category != "active" && v.Category != "attention" {
			continue
		}
		s := queueSection{Key: sectionKey(v.Option), Heading: v.Option, Sort: "created_at", Dir: "asc",
			Where: &wherePredDoc{Field: status, Op: "equals", Value: v.Option}}
		if v.Category == "attention" {
			s.Tone = "warn"
		}
		for _, t := range m.Transitions {
			if t.Field == status && t.From == v.Option {
				variant := "secondary"
				if len(s.Buttons) == 0 {
					variant = "primary"
				}
				s.Buttons = append(s.Buttons, queueButton{Label: t.Name, Becomes: t.To, Variant: variant})
			}
		}
		out = append(out, s)
	}
	return out
}

// sectionKey turns a status option into a Dataset-id-safe word.
func sectionKey(option string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(option) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func queueBase(l listed) string { return strings.TrimSuffix(l.DatasetID, "_list") }

// queueDatasets is every Dataset a queue page names: one records Dataset per section, the recently-done window,
// and the figures Dataset holding a count per section.
func queueDatasets(l listed, m GeneratedMachine) []datasetDoc {
	q, base := l.Item.Queue, queueBase(l)
	secs := queueSections(q, m)
	var out []datasetDoc
	figures := datasetDoc{ID: base + "_figures"}
	for _, s := range secs {
		out = append(out, datasetDoc{ID: base + "_" + s.Key, Select: domain.SelectRecords, Where: s.Where,
			Sort: []sortFieldDoc{{Field: s.Sort, Direction: s.Dir}}, Limit: q.Limit})
		figures.Measures = append(figures.Measures, measureDoc{ID: "msr_" + s.Key, Aggregate: "count", Where: s.Where})
	}
	if done := doneValue(m); done != "" {
		out = append(out, datasetDoc{ID: base + "_recent", Select: domain.SelectRecords,
			Where: &wherePredDoc{Field: statusFieldOf(m), Op: "equals", Value: done},
			Sort:  []sortFieldDoc{{Field: "updated_at", Direction: "desc"}}, Limit: queueRecentLimit})
	}
	return append([]datasetDoc{figures}, out...)
}

func queueRow(q *GeneratedQueue, m GeneratedMachine, s queueSection, roles map[string]bool) pageNodeDoc {
	badge := pageNodeDoc{Component: "StatusBadge", Tone: toneOfCategory(categoryOf(m, s.Heading)), From: map[string]string{"label": "status"}}
	if q.DateField != "" {
		badge = pageNodeDoc{Component: "StatusBadge", From: map[string]string{"label": "date:label", "tone": "date:tone"}}
	}
	left := []pageNodeDoc{{Static: "link", From: map[string]string{"text": "title", "href": domain.PageRecordRole}}, badge}
	if roles["person"] {
		left = append(left, pageNodeDoc{Static: "caption", From: map[string]string{"text": "person"}})
	}
	var right []pageNodeDoc
	if roles["money"] {
		right = append(right, pageNodeDoc{Static: "paragraph", From: map[string]string{"text": "money"}})
	}
	for _, b := range s.Buttons {
		right = append(right, pageNodeDoc{Component: "Button", Label: b.Label, Variant: b.Variant, Becomes: b.Becomes,
			From: map[string]string{"describes": "title"}, Binding: &pageBindingDoc{Write: "transition"}})
	}
	row := pageNodeDoc{Layout: "row", Gap: "default", Align: "center", Justify: "spread",
		Children: []pageNodeDoc{{Layout: "stack", Gap: "tight", Children: left}}}
	if len(right) > 0 {
		row.Children = append(row.Children, pageNodeDoc{Layout: "row", Gap: "tight", Align: "center", Children: right})
	}
	return row
}

func queuePage(l listed, m GeneratedMachine) pageNodeDoc {
	q, base := l.Item.Queue, queueBase(l)
	w := queueWords(q)
	secs := queueSections(q, m)
	roles := map[string]bool{}
	for _, cf := range m.CardFields {
		roles[cf.Role] = true
	}
	var metrics, sections []pageNodeDoc
	for _, s := range secs {
		metrics = append(metrics, pageNodeDoc{Component: "Metric", Label: s.Heading, Tone: s.Tone,
			Binding: &pageBindingDoc{Dataset: base + "_figures", Measure: "msr_" + s.Key, Rows: "total"}})
		sections = append(sections, pageNodeDoc{Component: "Collection", Heading: s.Heading, Gap: "tight", Divided: true, HideEmpty: true, Complete: true,
			Binding: &pageBindingDoc{Dataset: base + "_" + s.Key, Rows: "records"}, Children: []pageNodeDoc{queueRow(q, m, s, roles)}})
	}
	children := []pageNodeDoc{
		{Layout: "grid", Gap: "default", Columns: 4, Mobile: 2, Children: metrics},
		{Layout: "stack", Gap: "default", Empty: w.Empty, Children: sections},
	}
	if done := doneValue(m); done != "" {
		row := pageNodeDoc{Layout: "row", Gap: "default", Align: "center", Justify: "spread", Children: []pageNodeDoc{
			{Static: "link", From: map[string]string{"text": "title", "href": domain.PageRecordRole}},
			{Component: "StatusBadge", Tone: "good", From: map[string]string{"label": "status"}},
		}}
		children = append(children, pageNodeDoc{Component: "Collection", Heading: w.RecentlyDone, Gap: "tight", Divided: true, HideEmpty: true,
			Binding: &pageBindingDoc{Dataset: base + "_recent", Rows: "records"}, Children: []pageNodeDoc{row}})
	}
	first := base + "_" + secs[0].Key
	children = append(children,
		pageNodeDoc{Component: "Disclosure", Summary: "+ " + m.Name, Children: []pageNodeDoc{
			{Component: "Form", Submit: w.Add, Binding: &pageBindingDoc{Dataset: first, Write: "create"}}}},
		pageNodeDoc{Static: "link", ListOf: first, Text: w.All})
	return pageNodeDoc{Layout: "stack", Gap: "comfortable", Children: children}
}

// validateQueue holds a queue to what its recipe derives from: a status that says what is open, a title and a
// status to draw rows by, and for a dated queue a date Field the card projects and a completion to close against.
func validateQueue(item GeneratedMenuItem, m GeneratedMachine) []string {
	q := item.Queue
	where := fmt.Sprintf("queue on menu entry %q", item.Label)
	var issues []string
	if item.List != nil || item.ByStatus != nil {
		issues = append(issues, where+": an entry is a list, a status filter or a queue, not more than one")
	}
	if q.Limit < 1 || q.Limit > maxListLimit {
		issues = append(issues, fmt.Sprintf("%s: limit must be between 1 and %d", where, maxListLimit))
	}
	for _, s := range []string{q.Words.Overdue, q.Words.DueToday, q.Words.NextWeek, q.Words.Later, q.Words.RecentlyDone, q.Words.Empty, q.Words.All, q.Words.Add} {
		if strings.ContainsAny(s, "\r\n") {
			issues = append(issues, where+": every word is one line")
			break
		}
	}
	hasRole := func(role, field string) bool {
		return slices.ContainsFunc(m.CardFields, func(cf GeneratedCardField) bool { return cf.Role == role && (field == "" || cf.Field == field) })
	}
	if !hasRole("title", "") {
		issues = append(issues, fmt.Sprintf("%s: %s needs a card_fields entry with role \"title\"", where, m.ID))
	}
	if m.StatusCategories == nil {
		return append(issues, fmt.Sprintf("%s: %s needs status_categories -- a queue is the records that are not finished, and the categories say which those are", where, m.ID))
	}
	if q.DateField != "" {
		isDate := slices.ContainsFunc(m.Fields, func(f GeneratedField) bool {
			return f.ID == q.DateField && domain.FieldType(f.Type) == domain.FieldTypeDate
		})
		if !isDate {
			issues = append(issues, fmt.Sprintf("%s: date_field must be a date field of %s", where, m.ID))
		} else if !hasRole("date", q.DateField) {
			issues = append(issues, fmt.Sprintf("%s: %s needs a card_fields entry with role \"date\" on %s -- the rows show how late each is", where, m.ID, q.DateField))
		}
		if m.Completion == nil {
			issues = append(issues, fmt.Sprintf("%s: a dated queue needs %s to declare completion, so \"not finished\" has one meaning", where, m.ID))
		} else if m.Completion.Field != m.StatusCategories.Field {
			issues = append(issues, fmt.Sprintf("%s: completion and status_categories must be on the same status field", where))
		}
		return issues
	}
	if !hasRole("status", "") {
		issues = append(issues, fmt.Sprintf("%s: %s needs a card_fields entry with role \"status\"", where, m.ID))
	}
	if len(queueSections(q, m)) == 0 {
		issues = append(issues, fmt.Sprintf("%s: no status of %s is categorised start, active or attention, so there is nothing to queue", where, m.ID))
	}
	return issues
}
