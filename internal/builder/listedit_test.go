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
