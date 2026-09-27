package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/metadata"
)

// Package workspace_isolation_test.go holds the executable half of the owner's 2026-09-27
// decision that Workspaces are isolated: a *.yaml is a template, installing it into a Workspace
// copies it, and each copy diverges on its own -- "perubahan aplikasi di masing masing workspace
// tidak saling terkait." Only mch_user genuinely crosses Workspaces.
//
// It is written as tests rather than prose because prose is exactly what failed here. Three
// separate comments (metadata/workspaces/default.yaml, domain.Workspace.MachineIDs and
// cmd/server's own loadMetadataState) all correctly described the *old* shared model, and a
// generated Application still overwrote metadata/document.yaml -- the real Document Approval
// Machine installed in another Workspace -- because nothing executable held the boundary.

// sharedMachineIDs are the Machines a Workspace manifest may still reference outside its own
// directory, and the list is deliberately short and closed.
//
// mch_user is the genuine one: a membership row points at one of its records, so a single
// identity holds one per Workspace it belongs to, and the owner named it as the exception --
// "hanya user aja yang bisa lintas workspace, bisa dipakai di seluruh universe". mch_activity and
// mch_notification are runtime chrome rather than any Application's (no Application's machines:
// list claims either), and their records are already Workspace-scoped, so one declaration serves
// every Workspace without letting any Workspace's Application reach into another's.
//
// Anything else appearing here would mean an Application's own Machine had gone back to being
// shared, which is the state this whole change exists to end.
var sharedMachineIDs = map[string]bool{
	"mch_user":         true,
	"mch_activity":     true,
	"mch_notification": true,
}

// TestInstalledApplicationsAreCopiesNotSharedFiles is the isolation boundary itself: every
// Application file a Workspace installs, and every Machine those Applications claim, must live
// inside that Workspace's own metadata/workspaces/<slug>/ directory rather than being referenced
// out of the shared template library.
//
// It reads the manifests as text on purpose. The loaded domain.Workspace says which Machines exist
// but not which file each came from, and "which file" is the entire question here -- two
// Workspaces pointing at one path is precisely the shape being forbidden.
func TestInstalledApplicationsAreCopiesNotSharedFiles(t *testing.T) {
	dir := filepath.Join(repoRoot(), "metadata", "workspaces")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read workspace manifests: %v", err)
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".yaml")
		manifestPath := filepath.Join(dir, e.Name())
		app, err := metadata.LoadApplication(manifestPath)
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		checked++

		// A Machine an Application claims is that Application's own, so its file must be this
		// Workspace's copy. A Machine claimed by nobody is runtime-level and may be shared.
		claimed := map[string]string{} // machine id -> the Application claiming it
		for _, a := range app.Workspace.Applications {
			for _, mID := range a.Machines {
				claimed[mID] = a.ID
			}
		}

		machinePaths, applicationPaths := manifestPaths(t, manifestPath)
		for _, rel := range machinePaths {
			id := machineIDAt(t, filepath.Join(dir, rel))
			own := strings.HasPrefix(filepath.ToSlash(rel), slug+"/")
			switch {
			case own:
				// This Workspace's own copy -- correct however it is claimed.
			case claimed[id] != "":
				t.Errorf("%s: machine %s is claimed by application %s but referenced as %q, outside this Workspace's own directory -- an installed Application's Machines must be its Workspace's own copies, or editing one Workspace's Application changes another's",
					e.Name(), id, claimed[id], rel)
			case !sharedMachineIDs[id]:
				t.Errorf("%s: machine %s is referenced as %q, outside this Workspace's own directory, and is not one of the runtime-level Machines allowed to be shared (%v)",
					e.Name(), id, rel, sortedKeys(sharedMachineIDs))
			}
		}
		for _, rel := range applicationPaths {
			if !strings.HasPrefix(filepath.ToSlash(rel), slug+"/") {
				t.Errorf("%s: application %q is referenced outside this Workspace's own directory -- installing an Application copies it (metadata/applications/ holds the templates it is copied *from*)", e.Name(), rel)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no workspace manifests were checked -- this gate would pass an empty metadata/workspaces/ silently")
	}
}

// TestTwoWorkspacesCanHoldDifferentMachinesUnderOneID is the property the isolation exists to
// give, asserted against the real loader rather than inferred from the file layout: two Workspaces
// may each declare mch_document, meaning genuinely different things, and each gets its own.
//
// Before 2026-09-27 this was impossible twice over -- cmd/server unioned every Workspace's
// Machines into one id-keyed map and deduped by id, so the second Workspace silently got the
// first's Machine; and both would have been written to one file anyway.
func TestTwoWorkspacesCanHoldDifferentMachinesUnderOneID(t *testing.T) {
	root := t.TempDir()
	workspaces := filepath.Join(root, "workspaces")
	for _, slug := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(workspaces, slug), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workspaces, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Same id, deliberately different shapes -- one Workspace's "document" is not the other's.
	write("alpha/document.yaml", "id: mch_document\nname: Contract\nfields:\n  - id: fld_title\n    name: Title\n    type: text\n")
	write("beta/document.yaml", "id: mch_document\nname: Invoice\nfields:\n  - id: fld_amount\n    name: Amount\n    type: money\n")
	write("alpha.yaml", "workspace: alpha\nmachines:\n  - alpha/document.yaml\napplications: []\n")
	write("beta.yaml", "workspace: beta\nmachines:\n  - beta/document.yaml\napplications: []\n")

	loaded, err := metadata.LoadWorkspaces(workspaces)
	if err != nil {
		t.Fatalf("LoadWorkspaces: %v", err)
	}

	got := map[string]string{}
	for slug, ws := range loaded {
		for _, m := range ws.Workspace.Machines {
			if m.ID == "mch_document" {
				got[slug] = m.Name
			}
		}
	}
	if got["alpha"] != "Contract" || got["beta"] != "Invoice" {
		t.Errorf("mch_document resolved to %v, want alpha=Contract beta=Invoice -- each Workspace must get its own Machine, not whichever one loaded first", got)
	}

	// The Workspace, not the process, is what answers "which Machine is this id" -- so the two
	// must also be distinct objects, or editing one Workspace's copy would change the other's.
	if len(loaded) == 2 && loaded["alpha"].Workspace.Machines[0] == loaded["beta"].Workspace.Machines[0] {
		t.Error("both Workspaces share one *domain.Machine value for mch_document -- they must be independent, since either may diverge later")
	}
}

// manifestPaths returns a manifest's own machines:/applications: entries as written, which is what
// this file checks -- the loaded domain.Workspace keeps no record of the file each came from.
func manifestPaths(t *testing.T, manifestPath string) (machines, applications []string) {
	t.Helper()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read %s: %v", manifestPath, err)
	}
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "machines:"):
			section = "machines"
		case strings.HasPrefix(line, "applications:"):
			section = "applications"
		case line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#"):
			section = "" // any other top-level key ends the list
		case strings.HasPrefix(trimmed, "- "):
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if section == "machines" {
				machines = append(machines, item)
			} else if section == "applications" {
				applications = append(applications, item)
			}
		}
	}
	return machines, applications
}

func machineIDAt(t *testing.T, path string) string {
	t.Helper()
	m, err := metadata.Load(path)
	if err != nil {
		t.Fatalf("load machine %s: %v", path, err)
	}
	return m.ID
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
