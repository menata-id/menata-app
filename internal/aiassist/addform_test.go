package aiassist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The 2026-10-25 log (session aise_e9e97472, Dokter Kecil): a person asked for an "add task" button, the model
// answered with a menu entry named "Add Task" that was only another read-only table. A list that carries
// add_label is written with a Popover over a create Form bound to the list's own Machine, so the runtime
// (which already draws one on a hand-written page) gives the person a way to add a record from the list itself.

func TestAddForm_aListAndATableCarryACreateFormTheLoaderAccepts(t *testing.T) {
	manifest := w2Workspace(t)
	app := tableApp()
	app.Navigation[1].List.AddLabel = "Tambah tugas"
	app.Navigation[2].List.AddLabel = "Tugas baru"
	app.Navigation = append(app.Navigation, GeneratedMenuItem{Label: "Daftar", MachineID: "mch_d2_tugas",
		List: &GeneratedList{Scope: ListScopeAll, Limit: 20, AddLabel: "Tambah"}})
	change := GeneratedChange{Kind: KindNewApplication, Application: app}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatalf("Write (it loads what it wrote): %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(manifest), "w2", "applications", "d2.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if n := strings.Count(got, "write: create"); n != 3 {
		t.Errorf("%d create forms written, want 3 (two tables and a plain list):\n%s", n, got)
	}
	for _, label := range []string{"Tambah tugas", "Tugas baru", "Tambah"} {
		if !strings.Contains(got, "label: "+label) {
			t.Errorf("no trigger named %q:\n%s", label, got)
		}
	}
	if strings.Contains(got, "fld_") && strings.Contains(got, "fields:") {
		t.Errorf("the form names Fields; it must ask for the Machine's own controls:\n%s", got)
	}
}

func TestAddForm_aListWithoutAddLabelWritesNoForm(t *testing.T) {
	manifest := w2Workspace(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: tableApp()}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(filepath.Dir(manifest), "w2", "applications", "d2.yaml"))
	if strings.Contains(string(raw), "write: create") {
		t.Errorf("a list that did not ask for a form got one:\n%s", raw)
	}
}

func TestAddForm_eachMisuseIsRefusedForItsOwnReason(t *testing.T) {
	cases := []struct {
		name string
		edit func(app *GeneratedApplication)
		want string
	}{
		{"a label that runs over a line", func(a *GeneratedApplication) { a.Navigation[1].List.AddLabel = "satu\ndua" }, "one line"},
		{"a required group field the form cannot ask for", func(a *GeneratedApplication) {
			a.Navigation[1].List.AddLabel = "Tambah"
			a.Machines[0].Fields = append(a.Machines[0].Fields, GeneratedField{ID: "fld_req", Name: "Req", Type: "group", Required: true})
		}, "cannot ask for"},
		{"a required file field the form cannot ask for", func(a *GeneratedApplication) {
			a.Navigation[1].List.AddLabel = "Tambah"
			a.Machines[0].Fields = append(a.Machines[0].Fields, GeneratedField{ID: "fld_req", Name: "Req", Type: "file", Required: true})
		}, "cannot ask for"},
		{"a label of only spaces", func(a *GeneratedApplication) { a.Navigation[1].List.AddLabel = "   " }, "one line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := tableApp()
			tc.edit(app)
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
