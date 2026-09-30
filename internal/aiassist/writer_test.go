package aiassist

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/installer"
	"menata.app/internal/metadata"
)

// fixedResolver is a MachineFileResolver test fake -- see writer.go's own doc comment on why the
// interface exists.
type fixedResolver map[string]string

func (r fixedResolver) MachineFile(machineID string) (string, error) {
	path, ok := r[machineID]
	if !ok {
		return "", os.ErrNotExist
	}
	return path, nil
}

func TestWrite_newApplication_writesReloadableFiles(t *testing.T) {
	dir := t.TempDir()
	workspacesDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"),
		[]byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "applications", "document-approval.yaml"),
		[]byte("id: app_document_approval\nname: Document Approval\nmachines:\n  - mch_user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspacesDir, "default.yaml")
	original := "workspace: default\nmachines:\n  - ../user.yaml\napplications:\n  - ../applications/document-approval.yaml\n"
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	change := validLeaveRequestChange()
	appID, err := Write(manifestPath, change, nil)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if appID != "app_leave_requests" {
		t.Errorf("Write() returned app id %q, want app_leave_requests", appID)
	}

	machineBytes, err := os.ReadFile(filepath.Join(workspacesDir, "default", "leave_request.yaml"))
	if err != nil {
		t.Fatalf("machine file was not written: %v", err)
	}
	for _, want := range []string{"id: mch_leave_request", "id: fld_title", "id: fld_status", "id: prm_create", "id: trn_approve", "id: evt_submitted"} {
		if !strings.Contains(string(machineBytes), want) {
			t.Errorf("machine file missing %q, got:\n%s", want, machineBytes)
		}
	}

	appBytes, err := os.ReadFile(filepath.Join(workspacesDir, "default", "applications", "leave_requests.yaml"))
	if err != nil {
		t.Fatalf("application file was not written: %v", err)
	}
	if !strings.Contains(string(appBytes), "id: app_leave_requests") {
		t.Errorf("application file missing its own id, got:\n%s", appBytes)
	}
	// Regression: a generated Application used to declare no navigation at all, so its Workspace
	// Home card had no HomeRoute and linked back to /home -- found 2026-09-27 in the same
	// conversation that also surfaced the missing publisher_role. One item, home_card: true,
	// pointing at the generic page of the Application's own first Machine, is enough.
	for _, want := range []string{"navigation:", "id: nav_leave_requests", "route: /machines/mch_leave_request", "home_card: true"} {
		if !strings.Contains(string(appBytes), want) {
			t.Errorf("application file missing generated navigation %q, got:\n%s", want, appBytes)
		}
	}

	manifestAfter, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(manifestAfter)
	if !strings.Contains(text, "- ../user.yaml") {
		t.Error("manifest lost its original machine entry")
	}
	// The appended paths are the Workspace's own namespace, not the shared template library --
	// that is what makes this an install rather than a reference.
	if !strings.Contains(text, "- default/leave_request.yaml") {
		t.Errorf("manifest was not appended with the new machine, got:\n%s", text)
	}
	if !strings.Contains(text, "- ../applications/document-approval.yaml") {
		t.Error("manifest lost its original application entry")
	}
	if !strings.Contains(text, "- default/applications/leave_requests.yaml") {
		t.Errorf("manifest was not appended with the new application, got:\n%s", text)
	}
}

// TestWrite_newApplication_isLoadableByRealMetadataLoader is the one test that matters most for
// this package's own central promise: the bytes it writes are not merely well-formed YAML, they
// are a file the *real*, unmodified internal/metadata.LoadWorkspaces accepts and turns into a
// working domain.Application -- proving round-trip fidelity against the actual loader rather than
// against this package's own mirrored understanding of its shape.
func TestWrite_newApplication_isLoadableByRealMetadataLoader(t *testing.T) {
	dir := t.TempDir()
	workspacesDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspacesDir, "default.yaml")
	if err := os.WriteFile(manifestPath, []byte("workspace: default\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// mch_user is the one Machine every real manifest assumes exists (Person fields resolve to
	// it) -- write a minimal stand-in so LoadWorkspaces has something real to load.
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"), []byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(manifestPath, validLeaveRequestChange(), nil); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	loaded, err := metadata.LoadWorkspaces(workspacesDir)
	if err != nil {
		t.Fatalf("the real loader rejected what this package wrote: %v", err)
	}
	app, ok := loaded["default"].Workspace.ApplicationByID("app_leave_requests")
	if !ok {
		t.Fatal("the generated application is not installed after loading")
	}
	if app.Name != "Leave Requests" {
		t.Errorf("app.Name = %q, want Leave Requests", app.Name)
	}
	if len(app.Roles) != 3 {
		t.Errorf("app.Roles = %v, want 3 roles", app.Roles)
	}
	var found bool
	for _, m := range loaded["default"].Machines {
		if m.ID == "mch_leave_request" {
			found = true
			if len(m.Fields) != 2 {
				t.Errorf("mch_leave_request has %d fields, want 2", len(m.Fields))
			}
		}
	}
	if !found {
		t.Error("mch_leave_request was not loaded as one of the workspace's machines")
	}
}

// TestWrite_newApplication_refusesToOverwriteAnExistingMachineFile reproduces the 2026-09-27
// incident exactly: a generated Machine whose id maps onto a file that already exists. The
// generated filename comes from the id alone into one flat directory, so an id another Workspace's
// Application already uses is the same path -- "different Workspace" and "different Application"
// are not different files. Before refuseIfExists, this silently replaced metadata/document.yaml,
// the real Document Approval Machine, with a generated 69-line one.
func TestWrite_newApplication_refusesToOverwriteAnExistingMachineFile(t *testing.T) {
	dir := t.TempDir()
	workspacesDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspacesDir, "dokter-kecil.yaml")
	if err := os.WriteFile(manifestPath, []byte("workspace: dokter-kecil\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	existing := "id: mch_leave_request\nname: Something Real Someone Else Installed\n"
	if err := os.MkdirAll(filepath.Join(workspacesDir, "dokter-kecil"), 0o755); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(workspacesDir, "dokter-kecil", "leave_request.yaml")
	if err := os.WriteFile(existingPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Write(manifestPath, validLeaveRequestChange(), nil)
	if err == nil {
		t.Fatal("Write() = nil error, want a refusal to overwrite an existing machine file")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Write() error = %v, want it to name the collision", err)
	}
	var rejected *installer.RejectedError
	if !errors.As(err, &rejected) {
		t.Errorf("a collision is not marked as a rejection, so the publish handler would not hand it back to the assistant: %v", err)
	}

	after, readErr := os.ReadFile(existingPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != existing {
		t.Errorf("the pre-existing machine file was modified:\ngot:\n%s\nwant:\n%s", after, existing)
	}

	manifestAfter, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(manifestAfter), "leave_request") {
		t.Errorf("the workspace manifest was modified by a refused write:\n%s", manifestAfter)
	}
}

// TestWrite_newApplication_refusesToOverwriteAnExistingApplicationFile is the same guard on the
// other file this path writes.
func TestWrite_newApplication_refusesToOverwriteAnExistingApplicationFile(t *testing.T) {
	dir := t.TempDir()
	workspacesDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(filepath.Join(dir, "workspaces", "dokter-kecil", "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspacesDir, "dokter-kecil.yaml")
	if err := os.WriteFile(manifestPath, []byte("workspace: dokter-kecil\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(workspacesDir, "dokter-kecil", "applications", "leave_requests.yaml")
	if err := os.WriteFile(existingPath, []byte("id: app_leave_requests\nname: Already Installed\nmachines: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Write(manifestPath, validLeaveRequestChange(), nil)
	if err == nil {
		t.Fatal("Write() = nil error, want a refusal to overwrite an existing application file")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Write() error = %v, want it to name the collision", err)
	}
}

// TestWrite_rollsBackWhenGeneratedMetadataDoesNotLoad is the guarantee Write's own doc comment
// makes, exercised through a hole Validate genuinely still has: navigation ids. Validate checks no
// navigation at all, while the loader requires them unique across the whole Workspace
// (metadata.validateNavigationIDsAreUnique -- a duplicate would make routeByID resolve to
// whichever loaded first). Since 2026-09-27 a generated Application brings one nav item of its
// own, derived from its id, so a Workspace already using that id is a real collision nobody
// checks before the write.
//
// That is the point of load-verifying rather than adding check number twelve: this hole was found
// by writing the test, not before it, and the next one will be too. The assertion is about the
// tree, not the error -- nothing created, nothing edited, byte for byte.
func TestWrite_rollsBackWhenGeneratedMetadataDoesNotLoad(t *testing.T) {
	dir := t.TempDir()
	workspacesDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(filepath.Join(dir, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"),
		[]byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An Application already installed here, holding the very navigation id the generated one will
	// derive for itself ("app_leave_requests" -> "nav_leave_requests").
	if err := os.WriteFile(filepath.Join(dir, "applications", "existing.yaml"),
		[]byte("id: app_existing\nname: Existing\nmachines:\n  - mch_user\nnavigation:\n  - id: nav_leave_requests\n    label: Already Taken\n    route: /machines/mch_user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspacesDir, "default.yaml")
	originalManifest := "workspace: default\nmachines:\n  - ../user.yaml\napplications:\n  - ../applications/existing.yaml\n"
	if err := os.WriteFile(manifestPath, []byte(originalManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	change := validLeaveRequestChange()
	if err := Validate(change, ExistingState{MachineIDs: map[string]bool{}, ApplicationIDs: map[string]bool{}}); err != nil {
		t.Fatalf("precondition failed: Validate() = %v, want nil so this test exercises the load check", err)
	}

	if _, err := Write(manifestPath, change, nil); err == nil {
		t.Fatal("Write() = nil error, want a refusal -- this metadata cannot load")
	}

	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != originalManifest {
		t.Errorf("the workspace manifest was left modified after a failed write:\ngot:\n%s\nwant:\n%s", after, originalManifest)
	}
	for _, leftover := range []string{
		filepath.Join(workspacesDir, "default", "leave_request.yaml"),
		filepath.Join(workspacesDir, "default", "applications", "leave_requests.yaml"),
	} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s survived a failed write -- the tree must be left exactly as it was found", leftover)
		}
	}

	// And the whole thing still loads, which is the practical consequence: the next process
	// restart reads a Workspace that works.
	if _, err := metadata.LoadApplication(manifestPath); err != nil {
		t.Errorf("the workspace no longer loads after a rolled-back write: %v", err)
	}
}

// TestWrite_refusesAManifestNamingNoWorkspace is the 2026-09-30 incident: a Workspace with no manifest
// resolved to an empty slug, and publishing aimed at metadata/workspaces/.yaml. That made the
// Workspace's own directory the manifest directory itself, so the generated files were written into
// the directory the loader scans as manifests before the missing manifest was noticed. It must refuse
// before writing anything, and the refusal is the environment's, not the proposal's.
func TestWrite_refusesAManifestNamingNoWorkspace(t *testing.T) {
	workspacesDir := t.TempDir()
	_, err := Write(filepath.Join(workspacesDir, ".yaml"), validLeaveRequestChange(), nil)
	if err == nil {
		t.Fatal("Write() into metadata/workspaces/.yaml succeeded")
	}
	var rejected *installer.RejectedError
	if errors.As(err, &rejected) {
		t.Errorf("an unusable manifest path is marked as a rejection, so the assistant would be asked to fix it: %v", err)
	}
	entries, _ := os.ReadDir(workspacesDir)
	if len(entries) != 0 {
		t.Errorf("Write() left %d entries in the manifest directory", len(entries))
	}
}

// TestWrite_newApplication_writesTheMenuAsGiven: the menu entries, their order and labels are the
// person's answer, written through unchanged; only the first opens from Workspace Home.
func TestWrite_newApplication_writesTheMenuAsGiven(t *testing.T) {
	dir := t.TempDir()
	workspacesDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"),
		[]byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspacesDir, "cabang.yaml")
	if err := os.WriteFile(manifestPath, []byte("workspace: cabang\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change := validLeaveRequestChange()
	change.Application.Machines = append(change.Application.Machines, GeneratedMachine{
		ID: "mch_leave_balance", Name: "Leave Balance",
		Fields: []GeneratedField{{ID: "fld_days", Name: "Days", Type: "number", Required: true}},
	})
	change.Application.Navigation = []GeneratedMenuItem{
		{Label: "Sisa Cuti", MachineID: "mch_leave_balance"},
		{Label: "Pengajuan", MachineID: "mch_leave_request"},
	}
	if _, err := Write(manifestPath, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	app, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	nav := app.Workspace.Applications[0].AllNavigation
	if len(nav) != 2 || nav[0].Label != "Sisa Cuti" || nav[1].Label != "Pengajuan" {
		t.Fatalf("navigation = %+v, want the two entries in the order given", nav)
	}
	if nav[0].Route != "/machines/mch_leave_balance" || !nav[0].HomeCard || nav[1].HomeCard {
		t.Errorf("navigation = %+v, want the first entry to open from Home and only the first", nav)
	}
}
