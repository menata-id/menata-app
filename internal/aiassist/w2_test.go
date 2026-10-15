package aiassist

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// w2Workspace is an empty Workspace (a manifest and the shared user Machine) the W2 fixtures publish into.
func w2Workspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"),
		[]byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "workspaces", "w2.yaml")
	if err := os.WriteFile(manifest, []byte("workspace: w2\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// designRequestApp is the shape the owner wrote by hand for Desain Request, reduced to the five W2 keys:
// a starting status, a stamped requester, "done" over the status, an edit only the requester may make,
// and an approval only the owner of the related room may give.
func designRequestApp() *GeneratedApplication {
	return &GeneratedApplication{
		ID: "app_w2_request", Name: "Permintaan", Roles: []string{"requester", "coordinator"}, PublisherRole: "coordinator",
		Machines: []GeneratedMachine{
			{ID: "mch_w2_room", Name: "Ruangan", Fields: []GeneratedField{
				{ID: "fld_name", Name: "Nama", Type: "text", Required: true},
				{ID: "fld_owner", Name: "Pemilik", Type: "person"},
			}, Permissions: []GeneratedPermission{{ID: "prm_room_create", Action: "create", Roles: []string{"coordinator"}}}},
			{ID: "mch_w2_request", Name: "Permintaan", Fields: []GeneratedField{
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"baru", "dikerjakan", "selesai"}, Default: "baru"},
				{ID: "fld_requester", Name: "Pemohon", Type: "person", Stamp: "current_user"},
				{ID: "fld_room", Name: "Ruangan", Type: "relation", RelatedMachine: "mch_w2_room"},
				{ID: "fld_budget", Name: "Anggaran", Type: "number", Default: "5"},
			},
				Completion: &GeneratedCompletion{Field: "fld_status", Done: "selesai"},
				Permissions: []GeneratedPermission{
					{ID: "prm_req_create", Action: "create", Roles: []string{"requester"}},
					{ID: "prm_req_edit_own", Action: "edit", Roles: []string{}, ActorField: "fld_requester"},
					{ID: "prm_req_edit_owner", Action: "edit", Roles: []string{}, ParentActor: &GeneratedParentActor{Via: "fld_room", Field: "fld_owner"}},
				}},
		},
		Navigation: []GeneratedMenuItem{{Label: "Permintaan", MachineID: "mch_w2_request"}},
	}
}

// TestW2_theFiveKeysAreValidatedWrittenLoadedAndReadBack: roadmap stage 1, W2. A generated Application
// carrying default, stamp, completion, actor_field and parent_actor passes the same validation a
// hand-written file does, is written, loads through the real loader, and is read back by
// DescribeApplication with every one of the five intact.
func TestW2_theFiveKeysAreValidatedWrittenLoadedAndReadBack(t *testing.T) {
	manifest := w2Workspace(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: designRequestApp()}
	state, _ := stateOf(t, manifest)
	if err := Validate(change, state); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	state, loaded := stateOf(t, manifest)

	got := DescribeApplication("app_w2_request", state.Applications["app_w2_request"])
	want := designRequestApp()
	var gotReq, wantReq GeneratedMachine
	for _, m := range got.Machines {
		if m.ID == "mch_w2_request" {
			gotReq = m
		}
	}
	for _, m := range want.Machines {
		if m.ID == "mch_w2_request" {
			wantReq = m
		}
	}
	if !reflect.DeepEqual(gotReq.Completion, wantReq.Completion) {
		t.Errorf("completion read back as %+v, want %+v", gotReq.Completion, wantReq.Completion)
	}
	for i, f := range wantReq.Fields {
		if gotReq.Fields[i].Default != f.Default || gotReq.Fields[i].Stamp != f.Stamp {
			t.Errorf("field %s read back default=%q stamp=%q, want default=%q stamp=%q", f.ID, gotReq.Fields[i].Default, gotReq.Fields[i].Stamp, f.Default, f.Stamp)
		}
	}
	for i, p := range wantReq.Permissions {
		g := gotReq.Permissions[i]
		if g.ActorField != p.ActorField || !reflect.DeepEqual(g.ParentActor, p.ParentActor) {
			t.Errorf("permission %s read back %+v, want %+v", p.ID, g, p)
		}
	}
	_ = loaded
}

// Each key is judged by the loader's own rules, and a refusal names the cause.
func TestW2_eachKeyIsRefusedForTheReasonTheLoaderGives(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *GeneratedApplication)
		want string
	}{
		{"a default outside the status options", func(a *GeneratedApplication) { a.Machines[1].Fields[0].Default = "tidak_ada" }, "default"},
		{"a default that is not a number", func(a *GeneratedApplication) { a.Machines[1].Fields[3].Default = "banyak" }, "default"},
		{"a stamp on a text field", func(a *GeneratedApplication) { a.Machines[0].Fields[0].Stamp = "current_user" }, "stamp"},
		{"a stamp and a default together", func(a *GeneratedApplication) { a.Machines[1].Fields[1].Default = "x" }, "stamp"},
		{"completion on a field that is not a status", func(a *GeneratedApplication) { a.Machines[1].Completion.Field = "fld_budget" }, "completion"},
		{"completion done outside the options", func(a *GeneratedApplication) { a.Machines[1].Completion.Done = "lunas" }, "completion"},
		{"an actor_field that is not a person field", func(a *GeneratedApplication) { a.Machines[1].Permissions[1].ActorField = "fld_budget" }, "actor_field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := w2Workspace(t)
			app := designRequestApp()
			tc.edit(app)
			state, _ := stateOf(t, manifest)
			err := Validate(GeneratedChange{Kind: KindNewApplication, Application: app}, state)
			if err == nil {
				t.Fatalf("Validate accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal for %s does not name %q: %v", tc.name, tc.want, err)
			}
		})
	}
}

// An update keeps what the model never touched (all five keys, hand-set), may change a default and add a
// completion, and is refused when it re-stamps a field or re-targets an existing permission's gate.
func TestW2_updatesKeepChangeAndRefuseTheRightThings(t *testing.T) {
	manifest := w2Workspace(t)
	app := designRequestApp()
	app.Machines[1].Completion = nil // added later, by an update
	if _, err := Write(manifest, GeneratedChange{Kind: KindNewApplication, Application: app}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	desire := func(edit func(a *GeneratedApplication)) GeneratedChange {
		state, _ := stateOf(t, manifest)
		d := DescribeApplication("app_w2_request", state.Applications["app_w2_request"])
		edit(&d)
		return GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_w2_request", Application: &d}
	}
	req := func(a *GeneratedApplication) *GeneratedMachine {
		for i := range a.Machines {
			if a.Machines[i].ID == "mch_w2_request" {
				return &a.Machines[i]
			}
		}
		t.Fatal("no request machine")
		return nil
	}

	// refused
	for name, edit := range map[string]func(a *GeneratedApplication){
		"re-stamping a field": func(a *GeneratedApplication) { req(a).Fields[1].Stamp = "" },
		"re-targeting a gate": func(a *GeneratedApplication) {
			req(a).Permissions[1].ActorField = "fld_requester"
			req(a).Permissions[1].ParentActor = &GeneratedParentActor{Via: "fld_room", Field: "fld_owner"}
		},
		"removing a completion": func(a *GeneratedApplication) {
			req(a).Completion = &GeneratedCompletion{Field: "fld_status", Done: "dikerjakan"}
		},
	} {
		change := desire(edit)
		if name == "removing a completion" {
			// first give it one, then try to change it
			add := desire(func(a *GeneratedApplication) {
				req(a).Completion = &GeneratedCompletion{Field: "fld_status", Done: "selesai"}
			})
			state, _ := stateOf(t, manifest)
			if err := Validate(add, state); err != nil {
				t.Fatalf("adding a completion by update was refused: %v", err)
			}
			if _, err := Write(manifest, add, FileMachineResolver{WorkspaceManifestPath: manifest}); err != nil {
				t.Fatalf("Write(add completion): %v", err)
			}
			change = desire(edit)
		}
		state, _ := stateOf(t, manifest)
		err := Validate(change, state)
		if err == nil {
			t.Errorf("%s was accepted", name)
			continue
		}
		t.Logf("%s refused: %v", name, err)
		cause := map[string]string{"re-stamping a field": "stamp", "re-targeting a gate": "limited to", "removing a completion": "completion"}[name]
		if !strings.Contains(err.Error(), cause) {
			t.Errorf("%s was refused, but not for the plan's reason (%q): %v", name, cause, err)
		}
	}

	// allowed: change a default; everything hand-set survives
	change := desire(func(a *GeneratedApplication) { req(a).Fields[3].Default = "10" })
	publish(t, manifest, change)
	state, _ := stateOf(t, manifest)
	after := DescribeApplication("app_w2_request", state.Applications["app_w2_request"])
	m := req(&after)
	if m.Fields[3].Default != "10" || m.Fields[0].Default != "baru" || m.Fields[1].Stamp != "current_user" ||
		m.Completion == nil || m.Permissions[1].ActorField != "fld_requester" || m.Permissions[2].ParentActor == nil {
		t.Errorf("after an update changing one default, the rest did not survive: %+v", m)
	}
}
