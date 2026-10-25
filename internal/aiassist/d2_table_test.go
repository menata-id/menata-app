package aiassist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// tableApp: one Machine with a title, a status, a person, a text note and a date, and a menu with the plain entry
// plus an "all" table (search and chips) and a "mine" table, the shapes D2's first family writes.
func tableApp() *GeneratedApplication {
	app := boardApp()
	app.ID = "app_d2"
	app.Machines[0].ID = "mch_d2_tugas"
	app.Machines[0].Views = nil
	app.Machines[0].Fields = append(app.Machines[0].Fields, GeneratedField{ID: "fld_note", Name: "Catatan", Type: "long_text"})
	app.Navigation = []GeneratedMenuItem{
		{Label: "Tugas", MachineID: "mch_d2_tugas"},
		{Label: "Semua tugas", MachineID: "mch_d2_tugas", List: &GeneratedList{
			Scope: ListScopeAll, SortField: "fld_due", Direction: "asc", Limit: 100,
			Columns: []string{"fld_title", "fld_status", "fld_pic", "fld_due"}, SearchFields: []string{"fld_title", "fld_note"}, FilterField: "fld_status"}},
		{Label: "Tugas saya", MachineID: "mch_d2_tugas", List: &GeneratedList{
			Scope: ListScopeMine, PersonField: "fld_pic", Limit: 50, Columns: []string{"fld_title", "fld_due"}}},
	}
	return app
}

// TestD2_aTableIsWrittenAsDatasetsAndATablePageThatTheLoaderAccepts: plan 2026-10-25, D2 family 1. What is read
// back is what a person would meet: the Dataset's predicates, the chips' own Dataset, and the page's controls.
func TestD2_aTableIsWrittenAsDatasetsAndATablePageThatTheLoaderAccepts(t *testing.T) {
	manifest := w2Workspace(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: tableApp()}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, loaded := stateOf(t, manifest)
	var all, mine, chips bool
	for _, m := range loaded.Machines {
		if m.ID != "mch_d2_tugas" {
			continue
		}
		for _, ds := range m.Datasets {
			switch {
			case ds.Dimension == "fld_status" && len(ds.Measures) == 1 && ds.Measures[0].Aggregate == "count":
				chips = true
			case ds.ViewerScoped && ds.Where != nil && len(ds.Where.All) == 1 && ds.Where.All[0].Field == "fld_pic" && ds.Where.All[0].Value == "$current_user" && len(ds.Where.Any) == 0:
				mine = ds.Limit == 50
			case !ds.ViewerScoped && ds.Where != nil && ds.Select == "records":
				optional := true
				for _, c := range append(append([]expression.Comparison{}, ds.Where.All...), ds.Where.Any...) {
					optional = optional && c.Optional
				}
				all = optional && len(ds.Where.All) == 1 && ds.Where.All[0].Field == "fld_status" && ds.Where.All[0].Value == "$parameters.filter" &&
					len(ds.Where.Any) == 2 && ds.Where.Any[0].Op == "contains" && ds.Where.Any[0].Value == "$parameters.q" && ds.Limit == 100
			}
		}
	}
	if !all || !mine || !chips {
		t.Errorf("datasets read back: all=%v mine=%v chips=%v", all, mine, chips)
	}

	// The first column of each table links to the record; no other column does.
	written, err := os.ReadFile(filepath.Join(filepath.Dir(manifest), "w2", "applications", "d2.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(written), "href: record"); got != 2 {
		t.Errorf("%d columns link to the record, want 2 (the first column of each table):\n%s", got, written)
	}
}

func TestD2_eachTableMisuseIsRefusedForItsOwnReason(t *testing.T) {
	cases := []struct {
		name string
		edit func(l *GeneratedList)
		want string
	}{
		{"no column left", func(l *GeneratedList) { l.Columns = []string{}; l.SearchFields = []string{"fld_title"} }, "belong to a table"},
		{"too many columns", func(l *GeneratedList) {
			l.Columns = []string{"fld_title", "fld_status", "fld_pic", "fld_due", "fld_cost", "fld_note", "fld_title2", "fld_a", "fld_b"}
		}, "1 to 8 columns"},
		{"a column listed twice", func(l *GeneratedList) { l.Columns = []string{"fld_title", "fld_title"} }, "listed twice"},
		{"a column that is not a field", func(l *GeneratedList) { l.Columns = []string{"fld_nope"} }, "not a field of this machine"},
		{"a search over a date", func(l *GeneratedList) { l.SearchFields = []string{"fld_due"} }, "looks in text or long_text"},
		{"a search over a missing field", func(l *GeneratedList) { l.SearchFields = []string{"fld_nope"} }, "not a field of this machine"},
		{"five search fields", func(l *GeneratedList) {
			l.SearchFields = []string{"fld_title", "fld_note", "fld_a", "fld_b", "fld_c"}
		}, "at most 4"},
		{"chips over a text field", func(l *GeneratedList) { l.FilterField = "fld_title" }, "must be a status field"},
		{"a column with a blank name", func(l *GeneratedList) { l.Columns = []string{"fld_blank"} }, "no one-line name to be its header"},
		{"a column whose name runs over a line", func(l *GeneratedList) { l.Columns = []string{"fld_multi"} }, "no one-line name to be its header"},
		{"a search field with a blank name", func(l *GeneratedList) { l.SearchFields = []string{"fld_blank"} }, "no one-line name for the search box"},
		{"chips over a status field with a blank name", func(l *GeneratedList) { l.FilterField = "fld_blank_status" }, "no one-line name to label its chips"},
		{"chips on a viewer's own list", func(l *GeneratedList) { l.Scope = ListScopeMine; l.PersonField = "fld_pic" }, "only on a table with scope \"all\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := tableApp()
			app.Machines[0].Fields = append(app.Machines[0].Fields,
				GeneratedField{ID: "fld_blank", Name: " ", Type: "text"},
				GeneratedField{ID: "fld_multi", Name: "dua\nbaris", Type: "text"},
				GeneratedField{ID: "fld_blank_status", Name: "", Type: "status", Options: []string{"a"}})
			tc.edit(app.Navigation[1].List)
			state, _ := stateOf(t, w2Workspace(t))
			err := Validate(GeneratedChange{Kind: KindNewApplication, Application: app}, state)
			if err == nil {
				t.Fatalf("Validate accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal for %s does not say %q: %v", tc.name, tc.want, err)
			}
		})
	}
}

// A plain list keeps its own rule: search and chips belong to a table, and the Machine still needs its title.
func TestD2_aPlainListRefusesWhatBelongsToATable(t *testing.T) {
	app := tableApp()
	l := app.Navigation[2].List
	l.Columns = nil
	l.FilterField = "fld_status"
	state, _ := stateOf(t, w2Workspace(t))
	err := Validate(GeneratedChange{Kind: KindNewApplication, Application: app}, state)
	if err == nil || !strings.Contains(err.Error(), "belong to a table") {
		t.Errorf("a plain list with filter_field: want a refusal naming the table, got %v", err)
	}
}

// tableColumnTypes is measured, not assumed: for every field type a table is written with that field as a column,
// and the loader behind Write either accepts it or does not. The map must say exactly what the loader said, so a
// type is never offered a column the loader will refuse at publish, nor withheld one it would show.
func TestTable_everyFieldTypeAsAColumnIsKnown(t *testing.T) {
	for _, typ := range domain.FieldTypes() {
		t.Run(string(typ), func(t *testing.T) {
			f := GeneratedField{ID: "fld_x", Name: "X", Type: string(typ)}
			switch typ {
			case domain.FieldTypeStatus:
				f.Options = []string{"a", "b"}
			case domain.FieldTypeRelation:
				f.RelatedMachine = "mch_user"
			}
			app := tableApp()
			app.Machines[0].Fields = append(app.Machines[0].Fields, f)
			app.Navigation = app.Navigation[:2]
			app.Navigation[1].List = &GeneratedList{Scope: ListScopeAll, Limit: 10, Columns: []string{"fld_title", "fld_x"}}
			manifest := w2Workspace(t)
			_, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil)
			t.Logf("%s as a column: write error = %v", typ, err)
			if accepted := err == nil; accepted != tableColumnTypes[typ] {
				t.Errorf("the loader accepts %s as a column = %v, but tableColumnTypes says %v (%v)", typ, accepted, tableColumnTypes[typ], err)
			}
		})
	}
}

// A table written by one conversation survives the next: an update that changes something else describes the
// Application, finds the table's menu entries by id, and leaves their pages and Datasets as they were -- no
// second chips Dataset, no table turned back into a list.
func TestD2_aTableSurvivesAnUpdateThatChangesSomethingElse(t *testing.T) {
	manifest := w2Workspace(t)
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: tableApp()}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	state, _ := stateOf(t, manifest)
	d := DescribeApplication("app_d2", state.Applications["app_d2"])
	d.Machines[0].Fields = append(d.Machines[0].Fields, GeneratedField{ID: "fld_extra", Name: "Tambahan", Type: "text"})
	loaded := publish(t, manifest, GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_d2", Application: &d})

	var chips, lists int
	for _, m := range loaded.Machines {
		for _, ds := range m.Datasets {
			if ds.Dimension == "fld_status" {
				chips++
			}
			if ds.Select == "records" {
				lists++
			}
		}
	}
	if chips != 1 || lists != 2 {
		t.Errorf("after the update: %d chips Datasets and %d record Datasets, want 1 and 2", chips, lists)
	}
	var tables int
	for _, n := range loaded.Workspace.Applications[0].AllNavigation {
		if n.Page == nil {
			continue
		}
		for _, c := range n.Page.Children {
			if c.Type == "Table" {
				tables++
			}
		}
	}
	if tables != 2 {
		t.Errorf("after the update %d menu entries still draw a Table, want 2", tables)
	}
}

// An update may add a table as a new menu entry over an existing Machine, with its chips Dataset and all; it may
// not turn an existing entry into one (the same rule a plain list has).
func TestD2_anUpdateAddsATableButNeverTurnsAnExistingEntryIntoOne(t *testing.T) {
	manifest := w2Workspace(t)
	app := tableApp()
	app.Navigation = app.Navigation[:1]
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	desire := func(edit func(a *GeneratedApplication)) GeneratedChange {
		state, _ := stateOf(t, manifest)
		d := DescribeApplication("app_d2", state.Applications["app_d2"])
		edit(&d)
		return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_d2", Application: &d}
	}
	state, _ := stateOf(t, manifest)
	err := Validate(desire(func(a *GeneratedApplication) {
		a.Navigation[0].List = &GeneratedList{Scope: ListScopeAll, Limit: 10, Columns: []string{"fld_title"}}
	}), state)
	if err == nil || !strings.Contains(err.Error(), "cannot be given a list") {
		t.Errorf("turning an existing entry into a table: want a refusal saying so, got %v", err)
	}
	loaded := publish(t, manifest, desire(func(a *GeneratedApplication) {
		a.Navigation = append(a.Navigation, GeneratedMenuItem{Label: "Semua tugas", MachineID: "mch_d2_tugas", List: &GeneratedList{
			Scope: ListScopeAll, Limit: 100, Columns: []string{"fld_title", "fld_status"}, SearchFields: []string{"fld_title"}, FilterField: "fld_status"}})
	}))
	var chips, lists int
	for _, m := range loaded.Machines {
		for _, ds := range m.Datasets {
			if ds.Dimension == "fld_status" {
				chips++
			}
			if ds.Select == "records" {
				lists++
			}
		}
	}
	if chips != 1 || lists != 1 {
		t.Errorf("the update wrote %d chips Datasets and %d record Datasets, want 1 and 1", chips, lists)
	}
}
