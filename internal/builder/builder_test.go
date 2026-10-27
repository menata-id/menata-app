package builder

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/installer"
	"menata.app/internal/metadata"
	"menata.app/internal/registry"
)

const appID = "app_sampah"

// installed publishes a small Application through the assistant's own writer, so the operations below edit what a
// conversation wrote.
func installed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"),
		[]byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "workspaces", "nana.yaml")
	if err := os.WriteFile(manifest, []byte("workspace: nana\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
		ID: appID, Name: "Sampah", Roles: []string{"approver", "submitter"}, PublisherRole: "approver",
		Machines: []aiassist.GeneratedMachine{{ID: "mch_catat", Name: "Catat", Fields: []aiassist.GeneratedField{
			{ID: "fld_judul", Name: "Judul", Type: "text", Required: true},
			{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"draft", "sent"}},
		}, Permissions: []aiassist.GeneratedPermission{{ID: "prm_catat_create", Action: "create", Roles: []string{"approver"}}}}},
		Navigation: []aiassist.GeneratedMenuItem{{Label: "Catat", MachineID: "mch_catat"}},
	}}
	if _, err := aiassist.Write(manifest, change, nil); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return manifest
}

func current(t *testing.T, manifest string) aiassist.GeneratedApplication {
	t.Helper()
	loaded, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return aiassist.DescribeApplication(appID, aiassist.ExistingStateFrom(loaded.Workspace).Applications[appID])
}

func tree(t *testing.T, manifest string) map[string]string {
	t.Helper()
	out := map[string]string{}
	root := filepath.Dir(manifest)
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			out[rel] = string(b)
		}
		return nil
	})
	return out
}

// Every operation edits a key the catalogue lists, and one the assistant can write: an operation for a key without a
// slot would reach the file by a path the safety policy cannot see.
func TestEveryOperationNamesAWritableCatalogueKey(t *testing.T) {
	byID := map[string]registry.Capability{}
	for _, c := range registry.Capabilities {
		byID[c.ID] = c
	}
	for _, k := range Kinds() {
		c, ok := byID[Capability(k)]
		if !ok {
			t.Errorf("%s names %q, which registry.Capabilities does not list", k, Capability(k))
			continue
		}
		if !c.AIWritable {
			t.Errorf("%s edits %s, which has no assistant slot (%s): give the key its slot first", k, c.ID, c.AIReason)
		}
	}
}

// The same edit made through the assistant's shape and through operations yields the same files.
func TestOperationsProduceTheFilesAConversationEditWould(t *testing.T) {
	manifest := installed(t)
	got, err := Review(manifest, appID, []Op{
		{Kind: OpRenameField, Machine: "mch_catat", Field: "fld_judul", Value: "Judul laporan"},
		{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "done"},
		{Kind: OpSetPermissionRoles, Machine: "mch_catat", Permission: "prm_catat_create", Roles: []string{"approver", "submitter"}},
		{Kind: OpRenameMachine, Machine: "mch_catat", Value: "Pencatatan"},
	})
	if err != nil {
		t.Fatal(err)
	}
	hand := current(t, manifest)
	m := &hand.Machines[0]
	m.Name = "Pencatatan"
	m.Fields[0].Name = "Judul laporan"
	m.Fields[1].Options = append(m.Fields[1].Options, "done")
	m.Permissions[0].Roles = []string{"approver", "submitter"}
	want, err := aiassist.Preview(manifest, aiassist.GeneratedChange{Kind: aiassist.KindUpdateApplication, TargetAppID: appID, Application: &hand})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) == 0 || len(got.Files) != len(want) {
		t.Fatalf("files: operations %d, conversation %d", len(got.Files), len(want))
	}
	for i := range want {
		if got.Files[i] != want[i] {
			t.Errorf("file %s differs between the two paths", want[i].Path)
		}
	}
	if got.Plan.Refusals != nil {
		t.Errorf("refusals: %v", got.Plan.Refusals)
	}
}

// What the policy refuses, the operations refuse with the policy's own words.
func TestOperationsBreakingThePolicyAreRefusedLikeAConversationEdit(t *testing.T) {
	manifest := installed(t)
	_, err := Review(manifest, appID, []Op{{Kind: OpRemoveOption, Machine: "mch_catat", Field: "fld_status", Value: "sent"}})
	if err == nil || !strings.Contains(err.Error(), "cannot be removed: records may hold it") {
		t.Fatalf("removing an option: %v", err)
	}
	var rejected *installer.RejectedError
	if !errors.As(err, &rejected) {
		t.Errorf("a refusal is for the person, not a fault: %T", err)
	}
	hand := current(t, manifest)
	hand.Machines[0].Fields[1].Options = []string{"draft"}
	want := aiassist.PlanUpdate(current(t, manifest), hand).Refusals
	if len(want) == 0 || !strings.Contains(err.Error(), want[0]) {
		t.Errorf("words differ: conversation %v, operations %v", want, err)
	}

	p, _ := Review(manifest, appID, []Op{{Kind: OpRemoveField, Machine: "mch_catat", Field: "fld_judul"}})
	if open := p.Plan.Unconfirmed(nil); len(open) != 1 || open[0] != aiassist.RemovalKey("mch_catat", "fld_judul") {
		t.Errorf("removing a field waits on the owner, got %v", open)
	}
}

// A role the Application does not declare is stopped by Validate, the gate every edit passes, before any file is staged.
func TestAnUndeclaredRoleIsStoppedByValidation(t *testing.T) {
	manifest := installed(t)
	_, err := Review(manifest, appID, []Op{{Kind: OpSetPermissionRoles, Machine: "mch_catat", Permission: "prm_catat_create", Roles: []string{"ghost"}}})
	if err == nil || !strings.Contains(err.Error(), "names role \"ghost\", which the application does not declare") {
		t.Fatalf("got %v", err)
	}
}

func TestAnIdThatNamesNothingIsAnError(t *testing.T) {
	manifest := installed(t)
	for _, op := range []Op{
		{Kind: OpRenameField, Machine: "mch_nope", Field: "fld_judul", Value: "x"},
		{Kind: OpRenameField, Machine: "mch_catat", Field: "fld_nope", Value: "x"},
		{Kind: OpSetPermissionRoles, Machine: "mch_catat", Permission: "prm_nope"},
		{Kind: "teleport", Machine: "mch_catat"},
	} {
		if _, err := Review(manifest, appID, []Op{op}); err == nil {
			t.Errorf("%+v was accepted", op)
		}
	}
}

// Review is a plan; Apply edits a copy.
func TestReviewWritesNothingAndApplyLeavesItsInputAlone(t *testing.T) {
	manifest := installed(t)
	before := tree(t, manifest)
	cur := current(t, manifest)
	// spare capacity, so an append or delete through a shared backing array would show in the input
	cur.Machines[0].Fields[1].Options = append(make([]string, 0, 8), cur.Machines[0].Fields[1].Options...)
	names := slices.Clone(cur.Machines[0].Fields[1].Options)
	if _, err := Review(manifest, appID, []Op{{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "done"}}); err != nil {
		t.Fatal(err)
	}
	for _, op := range []Op{
		{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "done"},
		{Kind: OpRemoveOption, Machine: "mch_catat", Field: "fld_status", Value: "draft"},
	} {
		if _, err := Apply(cur, []Op{op}); err != nil {
			t.Fatal(err)
		}
	}
	if after := tree(t, manifest); !equal(before, after) {
		t.Error("Review changed live files")
	}
	if !slices.Equal(names, cur.Machines[0].Fields[1].Options) {
		t.Error("Apply edited its input")
	}
}

func equal(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
