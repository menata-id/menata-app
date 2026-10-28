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
const uncoveredFloor = 10

func TestUncoveredWritableKeysOnlyShrink(t *testing.T) {
	if got := len(Uncovered()); got != uncoveredFloor {
		t.Errorf("%d writable keys have no operation (floor %d): %v", got, uncoveredFloor, Uncovered())
	}
}

// A field added from an operation reaches the file with its type, options and requiredness; a default and a new permission
// likewise; a field id already taken is an error and not an overwrite.
func TestAddFieldSetDefaultAndAddPermission(t *testing.T) {
	manifest := installed(t)
	plan, err := Review(manifest, appID, []Op{
		{Kind: OpAddField, Machine: "mch_catat", Field: "fld_prioritas", Value: "Prioritas", Type: "status", Options: []string{"rendah", "tinggi"}},
		{Kind: OpSetDefault, Machine: "mch_catat", Field: "fld_status", Value: "draft"},
		{Kind: OpAddPermission, Machine: "mch_catat", Permission: "prm_catat_edit", Action: "edit", Roles: []string{"approver"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var diff string
	for _, f := range plan.Files {
		diff += f.Diff
	}
	for _, want := range []string{"fld_prioritas", "Prioritas", "rendah", "tinggi", "prm_catat_edit", "default: draft"} {
		if !strings.Contains(diff, want) {
			t.Errorf("the diff lacks %q:\n%s", want, diff)
		}
	}
	if _, err := Review(manifest, appID, []Op{{Kind: OpAddField, Machine: "mch_catat", Field: "fld_judul", Value: "Lain", Type: "text"}}); err == nil {
		t.Error("a field id already taken was accepted")
	}
	if _, err := Review(manifest, appID, []Op{{Kind: OpAddPermission, Machine: "mch_catat", Permission: "prm_catat_create", Action: "create", Roles: []string{"approver"}}}); err == nil {
		t.Error("a permission id already taken was accepted")
	}
	if _, err := Review(manifest, appID, []Op{{Kind: OpAddField, Machine: "mch_catat", Field: "fld_x", Value: "X", Type: "no-such-type"}}); err == nil {
		t.Error("an unknown field type was accepted")
	}
}

// Unchanged is a claim about the update policy, so it is probed against PlanUpdate itself: one key of each shape is changed
// in the whole desired state, and the plan must refuse it.
func TestUnchangedKeysAreRefusedByThePolicy(t *testing.T) {
	manifest := installed(t)
	base := current(t, manifest)
	// give the Machine one of each thing so there is something to change
	base.Machines[0].Transitions = []aiassist.GeneratedTransition{{ID: "trn_a", Name: "Kirim", Field: "fld_status", From: "draft", To: "sent", Roles: []string{"approver"}}}
	base.Machines[0].Events = []aiassist.GeneratedEvent{{ID: "evt_a", On: "create", Summary: "dibuat"}}
	probes := map[string]func(m *aiassist.GeneratedMachine){
		"fieldDoc.type":          func(m *aiassist.GeneratedMachine) { m.Fields[0].Type = "number" },
		"fieldDoc.stamp":         func(m *aiassist.GeneratedMachine) { m.Fields[0].Stamp = "current_user" },
		"permissionDoc.action":   func(m *aiassist.GeneratedMachine) { m.Permissions[0].Action = "delete" },
		"transitionDoc.roles":    func(m *aiassist.GeneratedMachine) { m.Transitions[0].Roles = []string{"submitter"} },
		"eventDoc.on":            func(m *aiassist.GeneratedMachine) { m.Events[0].On = "update" },
		"machineDoc.transitions": func(m *aiassist.GeneratedMachine) { m.Transitions = nil },
	}
	for key, change := range probes {
		if Unchanged[key] == "" {
			t.Errorf("%s is probed but not in Unchanged", key)
		}
		desired := clone(base)
		desired.Machines[0].Transitions = slices.Clone(base.Machines[0].Transitions)
		desired.Machines[0].Events = slices.Clone(base.Machines[0].Events)
		change(&desired.Machines[0])
		if plan := aiassist.PlanUpdate(base, desired); len(plan.Refusals) == 0 {
			t.Errorf("%s: PlanUpdate accepted a change that Unchanged says is refused: %v", key, plan.Items)
		}
	}
}

// An option's colour is the category its Machine's status_categories gives it. A new option can be given one, a Field added with options can colour them,
// and the policy that a status someone has learned to read never changes colour still holds: only options that were not there can gain an entry.
func TestANewOptionCanBeColouredAndAnOldOneCannotChange(t *testing.T) {
	manifest := installed(t)
	diffOf := func(ops []Op) string {
		t.Helper()
		plan, err := Review(manifest, appID, ops)
		if err != nil {
			t.Fatal(err)
		}
		var d string
		for _, f := range plan.Files {
			d += f.Diff
		}
		return d
	}
	if d := diffOf([]Op{{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "done", Category: "done"}}); !strings.Contains(d, "status_categories") || !strings.Contains(d, "done: done") {
		t.Errorf("a new coloured option does not reach status_categories:\n%s", d)
	}
	if d := diffOf([]Op{{Kind: OpAddField, Machine: "mch_catat", Field: "fld_stage", Value: "Stage", Type: "status", Options: []string{"open", "closed"}, Categories: map[string]string{"closed": "done"}}}); !strings.Contains(d, "closed: done") {
		t.Errorf("a status added with a colour does not carry it:\n%s", d)
	}
	if _, err := Review(manifest, appID, []Op{{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_judul", Value: "x", Category: "done"}}); err == nil {
		t.Error("a colour was given to an option of a Field that is not a status")
	}
	if _, err := Review(manifest, appID, []Op{{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "late", Category: "purple"}}); err == nil {
		t.Error("a category outside the closed five was accepted")
	}

	// Once the Machine colours its status: a second new option may join, and nothing already there may change or go.
	if _, err := Review(manifest, appID, []Op{{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "done", Category: "done"}}); err != nil {
		t.Fatal(err)
	}
	if err := Commit(manifest, appID, []Op{{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "done", Category: "done"}}, mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	if d := diffOf([]Op{{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "late", Category: "attention"}}); !strings.Contains(d, "late: attention") {
		t.Errorf("a second coloured option is not accepted:\n%s", d)
	}
	old := current(t, manifest)
	for name, edit := range map[string]func(m *aiassist.GeneratedMachine){
		"an existing option's colour changed": func(m *aiassist.GeneratedMachine) { m.StatusCategories.Values[0].Category = "stopped" },
		"an option that was already there coloured": func(m *aiassist.GeneratedMachine) {
			m.StatusCategories.Values = append(m.StatusCategories.Values, aiassist.GeneratedStatusCategory{Option: "draft", Category: "start"})
		},
	} {
		next := current(t, manifest)
		edit(&next.Machines[0])
		if r := aiassist.PlanUpdate(old, next).Refusals; len(r) == 0 {
			t.Errorf("%s was not refused", name)
		}
	}
}

func mustVersion(t *testing.T, manifest string) string {
	t.Helper()
	v, err := Version(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// Who may change a Field is one operation: it writes `editable_by`, a role the Application does not declare is stopped, and clearing it again is refused
// (it would open to everyone what was held back), exactly as a conversation's update is.
func TestSetEditableByWritesRolesAndRefusesClearing(t *testing.T) {
	manifest := installed(t)
	set := func(roles ...string) []Op {
		return []Op{{Kind: OpSetEditableBy, Machine: "mch_catat", Field: "fld_judul", Roles: roles}}
	}
	plan, err := Review(manifest, appID, set("approver"))
	if err != nil {
		t.Fatal(err)
	}
	var diff string
	for _, f := range plan.Files {
		diff += f.Diff
	}
	if !strings.Contains(diff, "editable_by") || !strings.Contains(diff, "approver") {
		t.Errorf("the diff does not carry the roles:\n%s", diff)
	}
	if _, err := Review(manifest, appID, set("janitor")); err == nil {
		t.Error("a role the Application does not declare was accepted")
	}
	if err := Commit(manifest, appID, set("approver"), mustVersion(t, manifest), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Review(manifest, appID, set("approver", "submitter")); err != nil {
		t.Errorf("widening to another role was refused: %v", err)
	}
	if _, err := Review(manifest, appID, set()); err == nil || !strings.Contains(err.Error(), "opened to everyone") {
		t.Errorf("clearing who may change a Field was not refused: %v", err)
	}
}

// A coloured choice added and removed again in one draft leaves nothing behind; an installed coloured choice is still refused, once, for the real reason.
func TestRemovingAChoiceTakesItsColourWithIt(t *testing.T) {
	manifest := installed(t)
	if _, err := Review(manifest, appID, []Op{
		{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "late", Category: "attention"},
		{Kind: OpAddOption, Machine: "mch_catat", Field: "fld_status", Value: "done", Category: "done"},
		{Kind: OpRemoveOption, Machine: "mch_catat", Field: "fld_status", Value: "late"},
	}); err != nil {
		t.Errorf("a choice added then removed in one draft: %v", err)
	}
}

// The menu operations edit the Application's navigation in order: rename, move to a place among the items, take out, add one that opens a Machine; an item
// that is not there, a place outside the list and a Machine the Application lacks are errors, and removing an item that opens no Machine is the plan's refusal.
func TestMenuOperationsEditTheNavigationInOrder(t *testing.T) {
	manifest := installed(t)
	cur := current(t, manifest)
	first := cur.Navigation[0].ID
	out, err := Apply(cur, []Op{
		{Kind: OpAddMenuItem, Machine: "mch_catat", Value: "Second"},
		{Kind: OpAddMenuItem, Machine: "mch_catat", Value: "Third"},
		{Kind: OpMoveMenuItem, Item: first, Position: 3},
		{Kind: OpRenameMenuItem, Item: first, Value: "Last"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, n := range out.Navigation {
		labels = append(labels, n.Label)
	}
	if strings.Join(labels, ",") != "Second,Third,Last" {
		t.Errorf("menu order = %v, want Second,Third,Last", labels)
	}
	if len(cur.Navigation) != 1 {
		t.Error("Apply edited its input")
	}
	for name, ops := range map[string][]Op{
		"an item that is not there": {{Kind: OpRenameMenuItem, Item: "nav_nowhere", Value: "x"}},
		"a place outside the list":  {{Kind: OpMoveMenuItem, Item: first, Position: 2}},
		"place zero":                {{Kind: OpMoveMenuItem, Item: first, Position: 0}},
		"a Machine the app lacks":   {{Kind: OpAddMenuItem, Machine: "mch_nowhere", Value: "x"}},
	} {
		if _, err := Apply(cur, ops); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	plan, err := Review(manifest, appID, []Op{{Kind: OpAddMenuItem, Machine: "mch_catat", Value: "Second"}, {Kind: OpRemoveMenuItem, Item: first}})
	if err != nil {
		t.Fatalf("taking out the only item that opens a Machine while adding another: %v", err)
	}
	if len(plan.Plan.Items) == 0 {
		t.Error("the plan says nothing of the menu change")
	}
}
