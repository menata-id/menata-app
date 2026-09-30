package installer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"menata.app/internal/metadata"
)

// TestEnsureWorkspaceManifest_writesAnEmptyInstallationThatLoads is the state a Workspace is in the
// moment it is created. Nothing wrote it until 2026-09-30; every manifest had been written by hand.
func TestEnsureWorkspaceManifest_writesAnEmptyInstallationThatLoads(t *testing.T) {
	workspacesDir := filepath.Join(t.TempDir(), "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ws, created, err := EnsureWorkspaceManifest(workspacesDir, libraryDir(t), "nana-workspace")
	if err != nil {
		t.Fatalf("EnsureWorkspaceManifest: %v", err)
	}
	if !created {
		t.Error("created = false for a Workspace that had no manifest")
	}
	if ws.Slug != "nana-workspace" || len(ws.Applications) != 0 {
		t.Errorf("got slug %q with %d applications, want nana-workspace with none", ws.Slug, len(ws.Applications))
	}
	for id := range SharedMachineIDs {
		if !ws.HasMachine(id) {
			t.Errorf("new workspace does not reference %s, which every Workspace needs", id)
		}
	}
	all, err := metadata.LoadWorkspaces(workspacesDir)
	if err != nil {
		t.Fatalf("the directory no longer loads after the new manifest: %v", err)
	}
	if _, ok := all["nana-workspace"]; !ok {
		t.Error("the scanning loader does not see the new Workspace")
	}
}

// TestEnsureWorkspaceManifest_leavesAnExistingManifestAlone: healing must never rewrite a Workspace
// that already has an installation.
func TestEnsureWorkspaceManifest_leavesAnExistingManifestAlone(t *testing.T) {
	workspacesDir := filepath.Join(t.TempDir(), "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureWorkspaceManifest(workspacesDir, libraryDir(t), "existing"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspacesDir, "existing.yaml")
	edited := read(t, path) + "# a person's own edit\n"
	write(t, path, edited)

	_, created, err := EnsureWorkspaceManifest(workspacesDir, libraryDir(t), "existing")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("created = true for a Workspace that already had a manifest")
	}
	if got := read(t, path); got != edited {
		t.Errorf("an existing manifest was rewritten:\n%s", got)
	}
}

// TestEnsureWorkspaceManifest_refusesWhatCannotNameAFile covers the slug that caused the incident
// (empty) and the ones that would escape the directory.
func TestEnsureWorkspaceManifest_refusesWhatCannotNameAFile(t *testing.T) {
	workspacesDir := t.TempDir()
	for _, slug := range []string{"", "../escape", "Upper", "a/b", "-x"} {
		if _, _, err := EnsureWorkspaceManifest(workspacesDir, libraryDir(t), slug); err == nil {
			t.Errorf("slug %q was accepted", slug)
		}
	}
	if _, _, err := EnsureWorkspaceManifest("", libraryDir(t), "ok"); err == nil {
		t.Error("an unconfigured metadata directory was accepted; it would write into the working directory")
	}
	entries, _ := os.ReadDir(workspacesDir)
	if len(entries) != 0 {
		t.Errorf("a refused call wrote %d file(s)", len(entries))
	}
}

// TestInstall_intoAWorkspaceCreatedMomentsAgo is the end-to-end test whose absence let this class
// through: every other install test starts from a hand-written manifest. It plans against the
// Workspace EnsureWorkspaceManifest returns, as the handler does -- planning against the zero
// Workspace on ctx would add shared references the manifest already holds.
func TestInstall_intoAWorkspaceCreatedMomentsAgo(t *testing.T) {
	workspacesDir := filepath.Join(t.TempDir(), "workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ws, _, err := EnsureWorkspaceManifest(workspacesDir, libraryDir(t), "fresh")
	if err != nil {
		t.Fatal(err)
	}

	plan := PlanInstall(documentApprovalTemplate(t), ws)
	if len(plan.AddShared) != 0 {
		t.Errorf("plan adds shared references %v that a new Workspace already holds", plan.AddShared)
	}
	if _, err := Install(plan, libraryDir(t), filepath.Join(workspacesDir, "fresh.yaml")); err != nil {
		t.Fatalf("Install into a freshly created Workspace: %v", err)
	}
	if _, err := metadata.LoadWorkspaces(workspacesDir); err != nil {
		t.Fatalf("does not load after install: %v", err)
	}
}

// TestWorkspaceOwnDir_refusesAnEmptySlug is the near-miss: metadata/workspaces/.yaml made a
// Workspace's own directory the manifest directory itself, and only a rollback kept generated
// Machine files out of the directory the loader scans as manifests.
func TestWorkspaceOwnDir_refusesAnEmptySlug(t *testing.T) {
	if _, err := WorkspaceOwnDir(filepath.Join("metadata", "workspaces", ".yaml")); err == nil {
		t.Error("an empty slug was accepted")
	}
	got, err := WorkspaceOwnDir(filepath.Join("metadata", "workspaces", "hanomerch.yaml"))
	if err != nil || got != filepath.Join("metadata", "workspaces", "hanomerch") {
		t.Errorf("WorkspaceOwnDir = %q, %v", got, err)
	}
}

func TestRejected_isDistinguishable(t *testing.T) {
	var r *RejectedError
	if !errors.As(Rejected(errors.New("x")), &r) {
		t.Error("Rejected does not produce a RejectedError")
	}
	if Rejected(nil) != nil {
		t.Error("Rejected(nil) is not nil")
	}
	if errors.As(errors.New("disk full"), &r) {
		t.Error("a plain error reads as a rejection")
	}
}
