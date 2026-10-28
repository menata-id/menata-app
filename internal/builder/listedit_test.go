package builder

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
)

// installedWithList publishes an Application whose menu entry is a table list the assistant writes, with chips and a search box, so a list can be read back
// and changed.
func installedWithList(t *testing.T) (manifest string, navID string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"), []byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest = filepath.Join(dir, "workspaces", "nana.yaml")
	if err := os.WriteFile(manifest, []byte("workspace: nana\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
		ID: appID, Name: "Sampah", Roles: []string{"approver", "submitter"}, PublisherRole: "approver",
		Machines: []aiassist.GeneratedMachine{{ID: "mch_catat", Name: "Catat", Fields: []aiassist.GeneratedField{
			{ID: "fld_judul", Name: "Judul", Type: "text", Required: true},
			{ID: "fld_catatan", Name: "Catatan", Type: "text"},
			{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"draft", "sent"}},
		}, CardFields: []aiassist.GeneratedCardField{{Field: "fld_judul", Role: "title"}, {Field: "fld_status", Role: "status"}},
			Permissions: []aiassist.GeneratedPermission{{ID: "prm_catat_create", Action: "create", Roles: []string{"approver"}}}}},
		Navigation: []aiassist.GeneratedMenuItem{{Label: "Catat", MachineID: "mch_catat", List: &aiassist.GeneratedList{
			Scope: aiassist.ListScopeAll, Limit: 50, Columns: []string{"fld_judul", "fld_status"}, SearchFields: []string{"fld_judul"}, FilterField: "fld_status", AddLabel: "Tambah",
		}}},
	}}
	if _, err := aiassist.Write(manifest, change, nil); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	cur := current(t, manifest)
	return manifest, cur.Navigation[0].ID
}

// A list the assistant wrote reads back as the list it was written from, so it can be edited; a screen that is not such a list is not read as one.
func TestAListReadsBackAsTheListItWasWrittenFrom(t *testing.T) {
	manifest, _ := installedWithList(t)
	got := current(t, manifest).Navigation[0]
	want := &aiassist.GeneratedList{Scope: aiassist.ListScopeAll, SortField: "created_at", Direction: "desc", Limit: 50, Columns: []string{"fld_judul", "fld_status"},
		SearchFields: []string{"fld_judul"}, FilterField: "fld_status", AddLabel: "Tambah"}
	if got.MachineID != "mch_catat" || !reflect.DeepEqual(got.List, want) {
		t.Errorf("read back %+v / %+v, want mch_catat / %+v", got.MachineID, got.List, want)
	}
}

// Columns, order, the filter chips and the search of a list are changed by operations: the page and the Datasets are written again, the old Datasets do not stay
// beside the new, and the Application still loads.
func TestListOperationsRewriteThePageAndItsDatasets(t *testing.T) {
	manifest, nav := installedWithList(t)
	ops := []Op{
		{Kind: OpSetListColumns, Item: nav, Fields: []string{"fld_judul", "fld_catatan"}},
		{Kind: OpSetListSort, Item: nav, Field: "fld_judul", Value: "asc"},
		{Kind: OpSetListFilter, Item: nav, Field: ""},
		{Kind: OpSetListSearch, Item: nav, Fields: []string{"fld_judul", "fld_catatan"}},
	}
	plan, err := Review(manifest, appID, ops)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.Items) == 0 || !strings.Contains(plan.Plan.Items[0].What, "columns fld_judul, fld_catatan") {
		t.Errorf("the plan does not say what changed: %+v", plan.Plan.Items)
	}
	if err := Commit(manifest, appID, ops, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	got := current(t, manifest).Navigation[0].List
	if got == nil || !reflect.DeepEqual(got.Columns, []string{"fld_judul", "fld_catatan"}) || got.SortField != "fld_judul" || got.Direction != "asc" || got.FilterField != "" || !reflect.DeepEqual(got.SearchFields, []string{"fld_judul", "fld_catatan"}) {
		t.Errorf("the list after the operations: %+v", got)
	}
	files := tree(t, manifest)
	machineFile := ""
	for path, text := range files {
		if strings.HasSuffix(path, "catat.yaml") {
			machineFile = text
		}
	}
	if strings.Count(machineFile, "_list") != strings.Count(machineFile, "id: ds_") || strings.Contains(machineFile, "_filter") {
		t.Errorf("the Machine's Datasets after the rewrite are wrong (an old one stayed, or the chips' Dataset did):\n%s", machineFile)
	}
	if strings.Count(machineFile, "id: ds_") != 1 {
		t.Errorf("want exactly one Dataset (the list's) after dropping the chips:\n%s", machineFile)
	}
	// What is not a list this way is refused, not rewritten: an unknown column, more than eight, an unknown Field.
	for name, bad := range map[string][]Op{
		"a column that is not a Field": {{Kind: OpSetListColumns, Item: nav, Fields: []string{"fld_nowhere"}}},
		"no column":                    {{Kind: OpSetListColumns, Item: nav, Fields: nil}},
		"an order by a missing Field":  {{Kind: OpSetListSort, Item: nav, Field: "fld_nowhere", Value: "asc"}},
		"a chip over a text Field":     {{Kind: OpSetListFilter, Item: nav, Field: "fld_judul"}},
	} {
		if _, err := Review(manifest, appID, bad); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A page is read as a list only if it is exactly what the generator writes: a person's own addition anywhere in it -- a column's total or header or `under:`, `complete:` on the Table,
// a `fields:` on the add form, a row template of their own -- means the page is not a list this editor rewrites, so a rewrite can never lose it.
func TestAListWithAPersonsOwnAdditionsIsNotReadAsOne(t *testing.T) {
	for name, edit := range map[string]func(string) string{
		"a column total": func(y string) string {
			return strings.Replace(y, "              field: fld_judul\n", "              field: fld_judul\n              total: count\n", 1)
		},
		"a column header": func(y string) string { return strings.Replace(y, "header: Judul", "header: Situasi", 1) },
		"complete on the Table": func(y string) string {
			return strings.Replace(y, "          framed: true\n", "          framed: true\n          complete: true\n", 1)
		},
		"fields on the form": func(y string) string {
			return strings.Replace(y, "                - submit: Tambah\n", "                - submit: Tambah\n                  fields: [fld_judul, fld_status]\n", 1)
		},
		"an extra prop on the row": func(y string) string {
			return strings.Replace(y, "          layout: row\n          gap: tight\n", "          layout: row\n          gap: tight\n          wrap: nowrap\n", 1)
		},
	} {
		manifest, nav := installedWithList(t)
		_ = nav
		changed := false
		for path, text := range tree(t, manifest) {
			if strings.HasSuffix(path, "applications/sampah.yaml") {
				if after := edit(text); after != text {
					full := filepath.Join(filepath.Dir(manifest), path)
					if err := os.WriteFile(full, []byte(after), 0o644); err != nil {
						t.Fatal(err)
					}
					changed = true
				}
			}
		}
		if !changed {
			t.Errorf("%s: the fixture has no such text to edit", name)
			continue
		}
		if got := current(t, manifest).Navigation[0]; got.List != nil {
			t.Errorf("%s: the page was read as a list and would be rewritten", name)
		}
	}
}

// A list echoed back with empty slices where it was read with nothing is the same list, not a change (and not a rewrite that drops a hand-edited row).
func TestAnEmptyListOfColumnsEchoedBackIsNoChange(t *testing.T) {
	manifest, _ := installedWithList(t)
	cur := current(t, manifest)
	desired := current(t, manifest)
	l := *desired.Navigation[0].List
	l.SearchFields = append([]string{}, l.SearchFields...)
	desired.Navigation[0].List = &l
	desired.Navigation[0].Label = "Renamed"
	plan := aiassist.PlanUpdate(cur, desired)
	for _, it := range plan.Items {
		if strings.Contains(it.What, "list of menu item") {
			t.Errorf("an unchanged list is reported as changed: %q", it.What)
		}
	}
}

// Adding an entry that opens a Machine another entry restricts is refused: it would open to everyone what was held back; and a status gets its first move only with the plan saying every other change is now refused.
func TestMenuAndWorkflowPlansSayWhatTheyCost(t *testing.T) {
	manifest := installed(t)
	if err := Commit(manifest, appID, []Op{{Kind: OpSetMenuVisibleTo, Item: current(t, manifest).Navigation[0].ID, Roles: []string{"approver"}}}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Review(manifest, appID, []Op{{Kind: OpAddMenuItem, Machine: "mch_catat", Value: "Everyone's"}}); err == nil || !strings.Contains(err.Error(), "open the screens") {
		t.Errorf("an entry opening a restricted Machine to everyone: %v", err)
	}
	plan, err := Review(installedFresh(t), appID, []Op{{Kind: OpAddTransition, Machine: "mch_catat", Field: "fld_status", Value: "Send", From: "draft", To: "sent"}})
	if err != nil {
		t.Fatal(err)
	}
	said := false
	for _, it := range plan.Plan.Items {
		said = said || strings.Contains(it.What, "governed from now on")
	}
	if !said {
		t.Errorf("the first move of a status does not say it governs the status: %+v", plan.Plan.Items)
	}
	if _, err := Apply(current(t, installedFresh(t)), []Op{{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "DRAFT"}}); err == nil {
		t.Error("an option equal to another but for its case was accepted")
	}
	if _, err := Apply(current(t, installedFresh(t)), []Op{{Kind: OpSetListColumns, Item: "x", Fields: nil}}); err == nil {
		t.Error("a list with no column was accepted")
	}
}

func installedFresh(t *testing.T) string {
	t.Helper()
	return installed(t)
}

// A Dataset of a person's own that sits under the id the chips' Dataset would have is not the generator's, and a rewrite of the list leaves it alone.
func TestARewriteLeavesAForeignDatasetUnderTheChipsId(t *testing.T) {
	manifest, nav := installedWithList(t)
	if err := Commit(manifest, appID, []Op{{Kind: OpSetListFilter, Item: nav, Field: ""}}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	var machineFile string
	for path := range tree(t, manifest) {
		if strings.HasSuffix(path, "catat.yaml") {
			machineFile = filepath.Join(filepath.Dir(manifest), path)
		}
	}
	text, err := os.ReadFile(machineFile)
	if err != nil {
		t.Fatal(err)
	}
	foreign := strings.Replace(string(text), "datasets:\n", "datasets:\n  - id: ds_sampah_filter\n    dimension: fld_status\n    measures:\n      - { id: msr_mine, aggregate: count }\n", 1)
	if err := os.WriteFile(machineFile, []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Commit(manifest, appID, []Op{{Kind: OpSetListSort, Item: nav, Field: "fld_judul", Value: "asc"}}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(machineFile)
	if !strings.Contains(string(after), "msr_mine") {
		t.Errorf("a Dataset of the person's own was dropped by a rewrite of the list:\n%s", after)
	}
}

// A table's layout -- how close its rows sit, the Field that folds them into groups, the role whose tone each row takes -- is read back and changed by one operation; a tone the Machine
// has no role for is refused by the loader at review, with its reason.
func TestListLayoutOperationFoldsAndTonesATable(t *testing.T) {
	manifest, nav := installedWithList(t)
	ops := []Op{{Kind: OpSetListLayout, Item: nav, Value: "compact", Field: "fld_status", Watch: "status:tone"}}
	if err := Commit(manifest, appID, ops, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	got := current(t, manifest).Navigation[0].List
	if got == nil || got.Density != "compact" || got.GroupBy != "fld_status" || got.RowTone != "status:tone" {
		t.Errorf("the layout after the operation: %+v", got)
	}
	text := ""
	for path, body := range tree(t, manifest) {
		if strings.HasSuffix(path, "applications/sampah.yaml") {
			text = body
		}
	}
	for _, want := range []string{"density: compact", "group_by: fld_status", "row_tone: status:tone"} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q:\n%s", want, text)
		}
	}
	for name, bad := range map[string][]Op{
		"a role the Machine has no card for": {{Kind: OpSetListLayout, Item: nav, Watch: "date:tone"}},
		"a density that is not one":          {{Kind: OpSetListLayout, Item: nav, Value: "tight"}},
		"a Field that is not there":          {{Kind: OpSetListLayout, Item: nav, Field: "fld_nowhere"}},
		"a role that is not one":             {{Kind: OpSetListLayout, Item: nav, Watch: "money:tone"}},
	} {
		if _, err := Review(manifest, appID, bad); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// Taking the layout off again is a change too, and it reads back as none.
	if err := Commit(manifest, appID, []Op{{Kind: OpSetListLayout, Item: nav}}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	if after := current(t, manifest).Navigation[0].List; after.Density != "" || after.GroupBy != "" || after.RowTone != "" {
		t.Errorf("the layout stayed after it was taken off: %+v", after)
	}
}
