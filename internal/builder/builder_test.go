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

func fieldNames(t *testing.T, manifest string) []string {
	t.Helper()
	var out []string
	for _, f := range current(t, manifest).Machines[0].Fields {
		out = append(out, f.Name)
	}
	return out
}

// Commit writes what Review showed, through the assistant's writer, and the installation's version moves.
func TestCommitWritesThePlanAndMovesTheVersion(t *testing.T) {
	manifest := installed(t)
	ops := []Op{{Kind: OpRenameField, Machine: "mch_catat", Field: "fld_judul", Value: "Judul laporan"}}
	plan, err := Review(manifest, appID, ops)
	if err != nil {
		t.Fatal(err)
	}
	if err := Commit(manifest, appID, ops, plan.Version, nil); err != nil {
		t.Fatal(err)
	}
	if got := fieldNames(t, manifest); got[0] != "Judul laporan" {
		t.Errorf("fields after commit: %v", got)
	}
	if v, _ := Version(manifest); v == plan.Version {
		t.Error("the version did not move after a write")
	}
}

// A plan made before someone else wrote is not applied.
func TestCommitRefusesAStalePlan(t *testing.T) {
	manifest := installed(t)
	plan, _ := Review(manifest, appID, []Op{{Kind: OpRenameField, Machine: "mch_catat", Field: "fld_judul", Value: "A"}})
	if err := Commit(manifest, appID, []Op{{Kind: OpRenameMachine, Machine: "mch_catat", Value: "Lain"}}, plan.Version, nil); err != nil {
		t.Fatal(err)
	}
	before := tree(t, manifest)
	err := Commit(manifest, appID, []Op{{Kind: OpRenameField, Machine: "mch_catat", Field: "fld_judul", Value: "A"}}, plan.Version, nil)
	if !errors.Is(err, ErrStale) {
		t.Fatalf("got %v, want ErrStale", err)
	}
	if !equal(before, tree(t, manifest)) {
		t.Error("a stale commit wrote")
	}
}

// A removal needs the owner's tick, by the key the plan names; a policy refusal writes nothing.
func TestCommitHoldsRemovalsAndRefusalsBack(t *testing.T) {
	manifest := installed(t)
	remove := []Op{{Kind: OpRemoveField, Machine: "mch_catat", Field: "fld_judul"}}
	plan, _ := Review(manifest, appID, remove)
	before := tree(t, manifest)
	if err := Commit(manifest, appID, remove, plan.Version, nil); err == nil || !strings.Contains(err.Error(), "the owner has not confirmed") { // Commit's own check, made before anything is staged; the writer's wording differs
		t.Fatalf("an unticked removal: %v", err)
	}
	refused := []Op{{Kind: OpRemoveOption, Machine: "mch_catat", Field: "fld_status", Value: "sent"}}
	if err := Commit(manifest, appID, refused, plan.Version, nil); err == nil {
		t.Fatal("a refused edit was committed")
	}
	if !equal(before, tree(t, manifest)) {
		t.Fatal("a held-back commit wrote")
	}
	if err := Commit(manifest, appID, remove, plan.Version, []string{aiassist.RemovalKey("mch_catat", "fld_judul")}); err != nil {
		t.Fatal(err)
	}
	if got := fieldNames(t, manifest); len(got) != 1 || got[0] != "Status" {
		t.Errorf("fields after a confirmed removal: %v", got)
	}
}

// Two admins applying against the same version: one wins, the other is stale -- not both written.
func TestCommitAdmitsOneOfTwoConcurrentApplies(t *testing.T) {
	manifest := installed(t)
	plan, _ := Review(manifest, appID, []Op{{Kind: OpRenameMachine, Machine: "mch_catat", Value: "X"}})
	results := make(chan error, 2)
	for _, name := range []string{"Satu", "Dua"} {
		go func() {
			results <- Commit(manifest, appID, []Op{{Kind: OpRenameMachine, Machine: "mch_catat", Value: name}}, plan.Version, nil)
		}()
	}
	var ok, stale int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			ok++
		case errors.Is(err, ErrStale):
			stale++
		default:
			t.Error(err)
		}
	}
	if ok != 1 || stale != 1 {
		t.Errorf("applied %d, stale %d; want 1 and 1", ok, stale)
	}
}

// Every supported key the assistant has no slot for belongs to an editor, or is named as one the builder never edits: a
// key added to the catalogue without that decision fails here, and so does an entry left behind once its key gains a slot.
func TestEveryKeyWithoutASlotHasAnEditor(t *testing.T) {
	editors := map[string]bool{EditorField: true, EditorMenu: true, EditorView: true, EditorWorkflow: true, EditorAutomation: true, EditorNone: true}
	owed := map[string]bool{}
	for _, c := range registry.Capabilities {
		if c.Stage != registry.StageSupported || c.AIWritable {
			continue
		}
		owed[c.ID] = true
		o, ok := Ownership[c.ID]
		switch {
		case !ok:
			t.Errorf("%s has no assistant slot (%s) and no editor in builder.Ownership", c.ID, c.AIReason)
		case !editors[o.Editor] || strings.TrimSpace(o.Why) == "":
			t.Errorf("%s: editor %q, why %q -- name one of the editors and say why", c.ID, o.Editor, o.Why)
		}
	}
	for id := range Ownership {
		if !owed[id] {
			t.Errorf("builder.Ownership lists %s, which is not a supported key without a slot -- remove the entry", id)
		}
	}
}

// uncoveredFloor is how many writable keys no operation names yet. It only goes down: an operation for a key lowers it
// (and the number here), a new writable key raises it and fails until someone decides.
const uncoveredFloor = 43

func TestUncoveredWritableKeysOnlyShrink(t *testing.T) {
	if got := len(Uncovered()); got != uncoveredFloor {
		t.Errorf("%d writable keys have no operation (floor %d): %v", got, uncoveredFloor, Uncovered())
	}
}
