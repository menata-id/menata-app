package aiassist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/metadata"
)

// installedWasteApp publishes the Nana 2 Workspace's own shape -- a branch list and a waste report --
// through Write, so the extensions below edit exactly what the assistant wrote there.
func installedWasteApp(t *testing.T) (manifestPath string) {
	t.Helper()
	dir := t.TempDir()
	workspacesDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"),
		[]byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath = filepath.Join(workspacesDir, "nana-2.yaml")
	if err := os.WriteFile(manifestPath, []byte("workspace: nana-2\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change := GeneratedChange{Kind: KindNewApplication, Application: &GeneratedApplication{
		ID: "app_pengelolaan_sampah", Name: "Pengelolaan Sampah",
		Roles: []string{"approver", "submitter"}, PublisherRole: "approver",
		Machines: []GeneratedMachine{
			{ID: "mch_cabang", Name: "Cabang", Fields: []GeneratedField{{ID: "fld_nama_cabang", Name: "Nama Cabang", Type: "text", Required: true}}},
			{ID: "mch_pencatatan_sampah", Name: "Pencatatan Sampah", Fields: []GeneratedField{
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"draft", "sent"}},
			}},
		},
		Navigation: []GeneratedMenuItem{{Label: "Pengelolaan Sampah", MachineID: "mch_cabang"}},
	}}
	if _, err := Write(manifestPath, change, nil); err != nil {
		t.Fatalf("installing the fixture application: %v", err)
	}
	return manifestPath
}

func existingFor(t *testing.T, manifestPath string) ExistingState {
	t.Helper()
	app, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	ws := app.Workspace
	state := ExistingState{MachineIDs: map[string]bool{}, ApplicationIDs: map[string]bool{}, Applications: map[string]ExistingApplicationState{}, NavIDs: map[string]bool{}}
	byID := map[string]*domain.Machine{}
	for _, m := range ws.Machines {
		state.MachineIDs[m.ID] = true
		byID[m.ID] = m
	}
	for _, a := range ws.Applications {
		claimed := map[string]*domain.Machine{}
		for _, id := range a.Machines {
			claimed[id] = byID[id]
		}
		var nav []ExistingNavItem
		for _, n := range a.AllNavigation {
			nav = append(nav, ExistingNavItem{ID: n.ID, Label: n.Label})
			state.NavIDs[n.ID] = true
		}
		state.ApplicationIDs[a.ID] = true
		state.Applications[a.ID] = ExistingApplicationState{Name: a.Name, Roles: a.Roles, Machines: claimed, Navigation: nav}
	}
	return state
}

// waterUsageExtension is the owner's own request, 2026-09-30: a water-usage report in m3 beside the
// waste report, keyed to the same branches, with a total, a menu "Data Cabang, Data Sampah, Data
// Air", and the application renamed "Data Sustainability".
func waterUsageExtension() GeneratedChange {
	return GeneratedChange{Kind: KindExtendApplication, TargetAppID: "app_pengelolaan_sampah", Additions: []MetadataAddition{
		{NewMachine: &GeneratedMachine{ID: "mch_pencatatan_air", Name: "Pencatatan Air",
			Fields: []GeneratedField{
				{ID: "fld_cabang", Name: "Cabang", Type: "relation", Required: true, RelatedMachine: "mch_cabang"},
				{ID: "fld_air_pdam", Name: "Air PDAM (m3)", Type: "number"},
				{ID: "fld_air_sumur", Name: "Air Sumur (m3)", Type: "number"},
				{ID: "fld_air_lainnya", Name: "Air Lainnya (m3)", Type: "number"},
				{ID: "fld_total_air", Name: "Total Pemakaian Air (m3)", Type: "number",
					Compute: &GeneratedCompute{Op: "sum", Fields: []string{"fld_air_pdam", "fld_air_sumur", "fld_air_lainnya"}}},
			},
			Permissions: []GeneratedPermission{{ID: "prm_air_create", Action: "create", Roles: []string{"submitter", "approver"}}},
		}},
		{NewNavItem: &GeneratedNavItem{ID: "nav_data_sampah", Label: "Data Sampah", MachineID: "mch_pencatatan_sampah"}},
		{NewNavItem: &GeneratedNavItem{ID: "nav_data_air", Label: "Data Air", MachineID: "mch_pencatatan_air"}},
		{RelabelNavItem: &NavRelabel{NavID: "nav_pengelolaan_sampah", Label: "Data Cabang"}},
		{ReorderNavigation: []string{"nav_pengelolaan_sampah", "nav_data_sampah", "nav_data_air"}},
		{RenameApplication: "Data Sustainability"},
	}}
}

func TestExtend_theWaterUsageRequestValidatesAndLoads(t *testing.T) {
	manifestPath := installedWasteApp(t)
	change := waterUsageExtension()
	if err := Validate(change, existingFor(t, manifestPath)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifestPath, change, FileMachineResolver{WorkspaceManifestPath: manifestPath}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("does not load: %v", err)
	}
	app := loaded.Workspace.Applications[0]
	if app.ID != "app_pengelolaan_sampah" || app.Name != "Data Sustainability" {
		t.Errorf("application = %s %q, want the same id under the new name", app.ID, app.Name)
	}
	var labels []string
	for _, n := range app.AllNavigation {
		labels = append(labels, n.Label)
	}
	if got := strings.Join(labels, ", "); got != "Data Cabang, Data Sampah, Data Air" {
		t.Errorf("menu = %s", got)
	}
	var water *domain.Machine
	for _, m := range loaded.Machines {
		if m.ID == "mch_pencatatan_air" {
			water = m
		}
	}
	if water == nil || !contains(app.Machines, "mch_pencatatan_air") {
		t.Fatal("the new machine is not installed and claimed by the application")
	}
	total, _ := water.FieldByID("fld_total_air")
	if total.Compute == nil || len(total.Compute.Fields) != 3 {
		t.Errorf("the total is not computed: %+v", total)
	}
	cabang, _ := water.FieldByID("fld_cabang")
	if cabang.RelatedMachine != "mch_cabang" {
		t.Errorf("the branch relation points at %q", cabang.RelatedMachine)
	}
}

func TestExtend_refusesWhatCannotBeApplied(t *testing.T) {
	manifestPath := installedWasteApp(t)
	existing := existingFor(t, manifestPath)
	cases := map[string]struct {
		additions []MetadataAddition
		want      string
	}{
		"nothing":            {nil, "carries no additions"},
		"unknown role":       {[]MetadataAddition{{NewMachine: &GeneratedMachine{ID: "mch_x", Name: "X", Fields: []GeneratedField{{ID: "fld_a", Name: "A", Type: "text"}}, Permissions: []GeneratedPermission{{ID: "prm_x", Action: "create", Roles: []string{"auditor"}}}}}}, "does not declare"},
		"relation nowhere":   {[]MetadataAddition{{NewMachine: &GeneratedMachine{ID: "mch_x", Name: "X", Fields: []GeneratedField{{ID: "fld_a", Name: "A", Type: "relation", RelatedMachine: "mch_nope"}}}}}, "neither a machine in this change"},
		"taken machine id":   {[]MetadataAddition{{NewMachine: &GeneratedMachine{ID: "mch_cabang", Name: "X", Fields: []GeneratedField{{ID: "fld_a", Name: "A", Type: "text"}}}}}, "already exists"},
		"menu to foreign":    {[]MetadataAddition{{NewNavItem: &GeneratedNavItem{ID: "nav_users", Label: "Users", MachineID: "mch_user"}}}, "not part of application"},
		"menu id taken":      {[]MetadataAddition{{NewNavItem: &GeneratedNavItem{ID: "nav_pengelolaan_sampah", Label: "Again", MachineID: "mch_cabang"}}}, "already used"},
		"relabel unknown":    {[]MetadataAddition{{RelabelNavItem: &NavRelabel{NavID: "nav_nope", Label: "X"}}}, "is not in application"},
		"reorder incomplete": {[]MetadataAddition{{NewNavItem: &GeneratedNavItem{ID: "nav_data_sampah", Label: "Data Sampah", MachineID: "mch_pencatatan_sampah"}}, {ReorderNavigation: []string{"nav_data_sampah"}}}, "every menu item exactly once"},
		"same name":          {[]MetadataAddition{{RenameApplication: "Pengelolaan Sampah"}}, "already called"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(GeneratedChange{Kind: KindExtendApplication, TargetAppID: "app_pengelolaan_sampah", Additions: tc.additions}, existing)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %v, want an issue containing %q", err, tc.want)
			}
		})
	}
}

// TestExtend_addsAnOptionToABlockStyleList: generated Machines are written with block-style lists,
// and the text edit this replaced could only append to a flow-style one -- so adding an option to
// anything the assistant had generated failed.
func TestExtend_addsAnOptionToABlockStyleList(t *testing.T) {
	manifestPath := installedWasteApp(t)
	change := GeneratedChange{Kind: KindExtendApplication, TargetAppID: "app_pengelolaan_sampah", Additions: []MetadataAddition{
		{MachineID: "mch_pencatatan_sampah", FieldID: "fld_status", NewOption: "approved"},
	}}
	if err := Validate(change, existingFor(t, manifestPath)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifestPath, change, FileMachineResolver{WorkspaceManifestPath: manifestPath}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range loaded.Machines {
		if f, ok := m.FieldByID("fld_status"); ok && !contains(f.Options, "approved") {
			t.Errorf("options = %v", f.Options)
		}
	}
}

// TestExtend_prioritizedMenu: priority wins over declared order, so in a menu that uses it a new item
// must take the next number (or it lands first) and a reorder must renumber (or nothing moves).
func TestExtend_prioritizedMenu(t *testing.T) {
	manifestPath := installedWasteApp(t)
	appPath := filepath.Join(filepath.Dir(manifestPath), "nana-2", "applications", "pengelolaan_sampah.yaml")
	src, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	withPriority := strings.Replace(string(src), "    home_card: true", "    home_card: true\n    priority: 1\n", 1)
	if err := os.WriteFile(appPath, []byte(withPriority), 0o644); err != nil {
		t.Fatal(err)
	}
	menuAfter := func(additions ...MetadataAddition) []string {
		t.Helper()
		change := GeneratedChange{Kind: KindExtendApplication, TargetAppID: "app_pengelolaan_sampah", Additions: additions}
		if _, err := Write(manifestPath, change, FileMachineResolver{WorkspaceManifestPath: manifestPath}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		loaded, err := metadata.LoadApplication(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		// The order a person sees: experience.GroupNavigation is what the menu renders through.
		var ids []string
		for _, g := range experience.GroupNavigation(loaded.Workspace.Applications[0].Navigation) {
			for _, n := range g.Items {
				ids = append(ids, n.ID)
			}
		}
		return ids
	}
	got := menuAfter(MetadataAddition{NewNavItem: &GeneratedNavItem{ID: "nav_data_sampah", Label: "Data Sampah", MachineID: "mch_pencatatan_sampah"}})
	if strings.Join(got, ",") != "nav_pengelolaan_sampah,nav_data_sampah" {
		t.Errorf("after adding, menu = %v, want the new item last", got)
	}
	got = menuAfter(MetadataAddition{ReorderNavigation: []string{"nav_data_sampah", "nav_pengelolaan_sampah"}})
	if strings.Join(got, ",") != "nav_data_sampah,nav_pengelolaan_sampah" {
		t.Errorf("after reordering, menu = %v", got)
	}
}
