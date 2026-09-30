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
// through Write, so the updates below edit exactly what the assistant wrote there.
func installedWasteApp(t *testing.T) string {
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
	manifestPath := filepath.Join(workspacesDir, "nana-2.yaml")
	if err := os.WriteFile(manifestPath, []byte("workspace: nana-2\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change := GeneratedChange{Kind: KindNewApplication, Application: &GeneratedApplication{
		ID: "app_pengelolaan_sampah", Name: "Pengelolaan Sampah",
		Roles: []string{"approver", "submitter"}, PublisherRole: "approver",
		Machines: []GeneratedMachine{
			{ID: "mch_cabang", Name: "Cabang", Fields: []GeneratedField{{ID: "fld_nama_cabang", Name: "Nama Cabang", Type: "text", Required: true}},
				Permissions: []GeneratedPermission{{ID: "prm_cabang_create", Action: "create", Roles: []string{"approver"}}}},
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

func stateOf(t *testing.T, manifestPath string) (ExistingState, *metadata.App) {
	t.Helper()
	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("does not load: %v", err)
	}
	return ExistingStateFrom(loaded.Workspace), loaded
}

// update returns the installed Application as the model is shown it, edited.
func update(t *testing.T, manifestPath string, edit func(a *GeneratedApplication)) GeneratedChange {
	t.Helper()
	state, _ := stateOf(t, manifestPath)
	desired := DescribeApplication("app_pengelolaan_sampah", state.Applications["app_pengelolaan_sampah"])
	edit(&desired)
	return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_pengelolaan_sampah", Application: &desired}
}

func publish(t *testing.T, manifestPath string, change GeneratedChange) *metadata.App {
	t.Helper()
	state, _ := stateOf(t, manifestPath)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifestPath, change, FileMachineResolver{WorkspaceManifestPath: manifestPath}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, loaded := stateOf(t, manifestPath)
	return loaded
}

func renderedMenu(app domain.Application) []string {
	var labels []string
	for _, g := range experience.GroupNavigation(app.Navigation) {
		for _, n := range g.Items {
			labels = append(labels, n.Label)
		}
	}
	return labels
}

// TestUpdate_theWaterUsageRequest is the owner's request, 2026-09-30, returned as the whole desired
// Application: a water-usage report related to the existing branches with a computed total, the
// menu "Data Cabang, Data Sampah, Data Air", and the Application renamed "Data Sustainability".
func TestUpdate_theWaterUsageRequest(t *testing.T) {
	manifestPath := installedWasteApp(t)
	change := update(t, manifestPath, func(a *GeneratedApplication) {
		a.Name = "Data Sustainability"
		a.Machines = append(a.Machines, GeneratedMachine{ID: "mch_pencatatan_air", Name: "Pencatatan Air",
			Fields: []GeneratedField{
				{ID: "fld_cabang", Name: "Cabang", Type: "relation", Required: true, RelatedMachine: "mch_cabang"},
				{ID: "fld_air_pdam", Name: "Air PDAM (m3)", Type: "number"},
				{ID: "fld_air_sumur", Name: "Air Sumur (m3)", Type: "number"},
				{ID: "fld_air_lainnya", Name: "Air Lainnya (m3)", Type: "number"},
				{ID: "fld_total_air", Name: "Total Pemakaian Air (m3)", Type: "number",
					Compute: &GeneratedCompute{Op: "sum", Fields: []string{"fld_air_pdam", "fld_air_sumur", "fld_air_lainnya"}}},
			},
			Permissions: []GeneratedPermission{{ID: "prm_air_create", Action: "create", Roles: []string{"submitter", "approver"}}},
		})
		a.Navigation[0].Label = "Data Cabang"
		a.Navigation = append(a.Navigation,
			GeneratedMenuItem{Label: "Data Sampah", MachineID: "mch_pencatatan_sampah"},
			GeneratedMenuItem{Label: "Data Air", MachineID: "mch_pencatatan_air"})
	})

	state, _ := stateOf(t, manifestPath)
	plan := PlanUpdate(DescribeApplication(change.TargetAppID, state.Applications[change.TargetAppID]), *change.Application)
	if len(plan.Refusals) != 0 || len(plan.Items) != 5 {
		t.Errorf("plan = %+v, want five items (rename, machine, relabel, two menu items) and no refusal", plan)
	}

	loaded := publish(t, manifestPath, change)
	app := loaded.Workspace.Applications[0]
	if app.ID != "app_pengelolaan_sampah" || app.Name != "Data Sustainability" {
		t.Errorf("application = %s %q, want the same id under the new name", app.ID, app.Name)
	}
	if got := strings.Join(renderedMenu(app), ", "); got != "Data Cabang, Data Sampah, Data Air" {
		t.Errorf("menu = %s", got)
	}
	var water *domain.Machine
	for _, m := range loaded.Machines {
		if m.ID == "mch_pencatatan_air" {
			water = m
		}
	}
	if water == nil {
		t.Fatal("the new machine is not installed")
	}
	if total, _ := water.FieldByID("fld_total_air"); total.Compute == nil {
		t.Error("the total is not computed")
	}
}

// TestUpdate_preservesWhatTheModelNeverSaw: the model is shown only what the generated shape
// expresses. A hand-written comment, a key it has no slot for, and a Permission gated on an actor
// Field must all survive an update that never mentions them.
func TestUpdate_preservesWhatTheModelNeverSaw(t *testing.T) {
	manifestPath := installedWasteApp(t)
	machinePath := filepath.Join(filepath.Dir(manifestPath), "nana-2", "pencatatan_sampah.yaml")
	extra := `# A hand-written note that must survive.
id: mch_pencatatan_sampah
name: Pencatatan Sampah
fields:
  - id: fld_status
    name: Status
    type: status
    required: true
    options: [draft, sent]
  - id: fld_pelapor
    name: Pelapor
    type: person
permissions:
  - id: prm_own_edit
    action: edit
    actor_field: fld_pelapor
datasets:
  - id: ds_sampah_count
    select: records
    limit: 100
`
	if err := os.WriteFile(machinePath, []byte(extra), 0o644); err != nil {
		t.Fatal(err)
	}
	state, _ := stateOf(t, manifestPath)
	shown := DescribeApplication("app_pengelolaan_sampah", state.Applications["app_pengelolaan_sampah"])
	for _, m := range shown.Machines {
		if m.ID == "mch_pencatatan_sampah" && len(m.Permissions) != 0 {
			t.Errorf("an actor-gated permission was shown to the model: %+v", m.Permissions)
		}
	}

	publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		for i := range a.Machines {
			if a.Machines[i].ID == "mch_pencatatan_sampah" {
				a.Machines[i].Name = "Data Sampah"
				a.Machines[i].Fields[0].Options = append(a.Machines[i].Fields[0].Options, "approved")
			}
		}
	}))
	after, err := os.ReadFile(machinePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# A hand-written note that must survive.", "actor_field: fld_pelapor", "ds_sampah_count", "name: Data Sampah", "options: [draft, sent, approved]"} {
		if !strings.Contains(string(after), want) {
			t.Errorf("after the update the file lacks %q:\n%s", want, after)
		}
	}
}

// TestUpdate_refusesWhatHoldsRecordsOrGrantsAccess: leaving something out of the desired state reads
// as removing it, and removing what holds records or grants access is refused.
func TestUpdate_refusesWhatHoldsRecordsOrGrantsAccess(t *testing.T) {
	manifestPath := installedWasteApp(t)
	state, _ := stateOf(t, manifestPath)
	cases := map[string]struct {
		edit func(a *GeneratedApplication)
		want string
	}{
		"machine":    {func(a *GeneratedApplication) { a.Machines = a.Machines[:1] }, "machine mch_pencatatan_sampah"},
		"field":      {func(a *GeneratedApplication) { a.Machines[0].Fields = nil }, "field fld_nama_cabang"},
		"role":       {func(a *GeneratedApplication) { a.Roles = []string{"approver"} }, `role "submitter"`},
		"permission": {func(a *GeneratedApplication) { a.Machines[0].Permissions = nil }, "open to everyone"},
		"type":       {func(a *GeneratedApplication) { a.Machines[0].Fields[0].Type = "number" }, "cannot change type"},
		"retarget":   {func(a *GeneratedApplication) { a.Navigation[0].MachineID = "mch_pencatatan_sampah" }, "pointed somewhere else"},
		"id":         {func(a *GeneratedApplication) { a.ID = "app_other" }, "must stay"},
		"stolen id": {func(a *GeneratedApplication) {
			a.Machines = append(a.Machines, GeneratedMachine{ID: "mch_user", Name: "U", Fields: []GeneratedField{{ID: "fld_a", Name: "A", Type: "text"}}})
		}, "already exists"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			desired := DescribeApplication("app_pengelolaan_sampah", state.Applications["app_pengelolaan_sampah"])
			tc.edit(&desired)
			err := Validate(GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_pengelolaan_sampah", Application: &desired}, state)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %v, want an issue containing %q", err, tc.want)
			}
		})
	}
}

// TestUpdate_menuRemovalKeepsTheRecordsAndTheHomeCard: dropping a Machine's menu item is allowed (the
// Machine stays), and when the dropped item was the Home card's, the first remaining item takes it.
func TestUpdate_menuRemovalKeepsTheRecordsAndTheHomeCard(t *testing.T) {
	manifestPath := installedWasteApp(t)
	publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		a.Navigation = append(a.Navigation, GeneratedMenuItem{Label: "Data Sampah", MachineID: "mch_pencatatan_sampah"})
	}))
	loaded := publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		a.Navigation = a.Navigation[1:]
	}))
	app := loaded.Workspace.Applications[0]
	if got := renderedMenu(app); len(got) != 1 || got[0] != "Data Sampah" {
		t.Errorf("menu = %v", got)
	}
	if app.HomeRoute != "/machines/mch_pencatatan_sampah" {
		t.Errorf("HomeRoute = %q, want the remaining item's route", app.HomeRoute)
	}
	if len(app.Machines) != 2 {
		t.Errorf("machines = %v, want both kept", app.Machines)
	}
}

// TestUpdate_prioritizedMenu: priority orders ahead of position, so in a menu that uses it an update
// renumbers every item, or the order the person asked for would not show.
func TestUpdate_prioritizedMenu(t *testing.T) {
	manifestPath := installedWasteApp(t)
	appPath := filepath.Join(filepath.Dir(manifestPath), "nana-2", "applications", "pengelolaan_sampah.yaml")
	src, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appPath, []byte(strings.Replace(string(src), "    home_card: true", "    home_card: true\n    priority: 1\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		a.Navigation = append(a.Navigation, GeneratedMenuItem{Label: "Data Sampah", MachineID: "mch_pencatatan_sampah"})
	}))
	if got := strings.Join(renderedMenu(loaded.Workspace.Applications[0]), ","); got != "Pengelolaan Sampah,Data Sampah" {
		t.Errorf("after adding, menu = %s, want the new item last", got)
	}
	loaded = publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		a.Navigation[0], a.Navigation[1] = a.Navigation[1], a.Navigation[0]
	}))
	if got := strings.Join(renderedMenu(loaded.Workspace.Applications[0]), ","); got != "Data Sampah,Pengelolaan Sampah" {
		t.Errorf("after reordering, menu = %s", got)
	}
}
