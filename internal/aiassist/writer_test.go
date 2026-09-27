package aiassist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestWrite_extendApplication_appendsOptionPreservingComments(t *testing.T) {
	dir := t.TempDir()
	// document.yaml lives one level up from metadata/workspaces/, exactly like the real repo
	// layout (metadata/workspaces/default.yaml's own "../document.yaml" entries) -- the resolver
	// below returns that same relative path, which writeExtension joins against the workspace
	// manifest's own directory.
	machinePath := filepath.Join(dir, "document.yaml")
	original := `# A hand-written comment that must survive.
id: mch_document
fields:
  - id: fld_document_type
    name: Document Type
    type: status
    options: [Kontrak, Tagihan, Lain-lain]
`
	if err := os.WriteFile(machinePath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	change := GeneratedChange{
		Kind:        KindExtendApplication,
		TargetAppID: "app_document_approval",
		Additions: []MetadataAddition{
			{MachineID: "mch_document", FieldID: "fld_document_type", NewOption: "Nota Dinas"},
		},
	}
	resolver := fixedResolver{"mch_document": "../document.yaml"}
	workspacesDir := filepath.Join(dir, "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspacesDir, "default.yaml")
	if err := os.WriteFile(manifestPath, []byte("workspace: default\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(manifestPath, change, resolver); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	updated, err := os.ReadFile(machinePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.Contains(text, "# A hand-written comment that must survive.") {
		t.Error("surgical edit lost the file's own comment")
	}
	if !strings.Contains(text, "options: [Kontrak, Tagihan, Lain-lain, Nota Dinas]") {
		t.Errorf("option was not appended in place, got:\n%s", text)
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

func TestAppendBlockListItem_missingKey(t *testing.T) {
	if _, err := appendBlockListItem([]byte("workspace: default\n"), "applications:", "x.yaml"); err == nil {
		t.Fatal("appendBlockListItem() = nil error, want one for a missing key")
	}
}

// TestAppendBlockListItem_emptyFlowList covers the real, documented shape of a brand-new
// Workspace's own manifest (CLAUDE.md: "An empty applications: [] is valid and normal") --
// converting it to block style in place rather than refusing it.
func TestAppendBlockListItem_emptyFlowList(t *testing.T) {
	src := "workspace: default\nmachines:\n  - ../user.yaml\napplications: []\n"
	got, err := appendBlockListItem([]byte(src), "applications:", "../applications/leave_requests.yaml")
	if err != nil {
		t.Fatalf("appendBlockListItem() error = %v", err)
	}
	want := "workspace: default\nmachines:\n  - ../user.yaml\napplications:\n  - ../applications/leave_requests.yaml\n"
	if string(got) != want {
		t.Errorf("appendBlockListItem() =\n%s\nwant\n%s", got, want)
	}
}

func TestAppendFlowListItemNearAnchor_missingAnchor(t *testing.T) {
	if _, err := appendFlowListItemNearAnchor([]byte("id: fld_other\noptions: [a, b]\n"), "id: fld_missing", "options:", "c"); err == nil {
		t.Fatal("appendFlowListItemNearAnchor() = nil error, want one for a missing anchor")
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
