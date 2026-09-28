package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// Package workflow_binding_test.go holds the property Stage A of ROADMAP.md's "Document Approval:
// closing the last three layers" exists to give: the approval engine engages because an Application
// *declares* that it does, not because anything is named a particular way.
//
// Until 2026-09-28 action.IsDocument/IsStep matched the literals app_document_approval +
// mch_document / mch_approval_step. Everything worked, and none of it could be renamed, varied or
// installed twice -- 001 Principle #2 puts application behavior in the runtime, and an engine that
// only wakes for one set of names is behavior owned by one application (audit Gap C,
// menata-app-document's audits/2026-09-28-kajian-metadata-based-document-approval.md).
//
// Asserted through the real loader over a real manifest, never with a hand-built domain.Machine: the
// whole claim is that *metadata* decides this, so a fixture that stamped the fields itself would
// assert the predicate and skip the declaration, which is the half that was missing.

// TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames deliberately uses names nothing in
// this repo knows -- an Application called app_persetujuan over mch_surat and mch_langkah -- because
// that is the only way to tell a declaration apart from a coincidence. Every id here would have
// failed both predicates the day before.
func TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames(t *testing.T) {
	root := t.TempDir()
	workspaces := filepath.Join(root, "workspaces")
	if err := os.MkdirAll(filepath.Join(workspaces, "kantor"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workspaces, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("kantor/surat.yaml", "id: mch_surat\nname: Surat\nfields:\n  - id: fld_judul\n    name: Judul\n    type: text\n")
	write("kantor/langkah.yaml", "id: mch_langkah\nname: Langkah\nfields:\n  - id: fld_urutan\n    name: Urutan\n    type: number\n")
	// A second Machine inside the *same* Application, holding no role: claiming a Machine is not
	// the same as casting it, and a screen that opts into approval behaviour for everything its
	// Application owns would be the old bug in a new place.
	write("kantor/arsip.yaml", "id: mch_arsip\nname: Arsip\nfields:\n  - id: fld_judul\n    name: Judul\n    type: text\n")
	write("kantor/persetujuan.yaml", `id: app_persetujuan
name: Persetujuan Surat
machines:
  - mch_surat
  - mch_langkah
  - mch_arsip
workflow:
  engine: document_approval
  roles:
    document: mch_surat
    step: mch_langkah
`)
	write("kantor.yaml", "workspace: kantor\nmachines:\n  - kantor/surat.yaml\n  - kantor/langkah.yaml\n  - kantor/arsip.yaml\napplications:\n  - kantor/persetujuan.yaml\n")

	loaded, err := metadata.LoadWorkspaces(workspaces)
	if err != nil {
		t.Fatalf("LoadWorkspaces: %v", err)
	}
	byID := map[string]*domain.Machine{}
	for _, m := range loaded["kantor"].Workspace.Machines {
		byID[m.ID] = m
	}

	if !action.IsDocument(byID["mch_surat"]) {
		t.Error("action.IsDocument(mch_surat) = false -- its Application declares it the document of the document_approval engine, so the engine must engage for it whatever it is called")
	}
	if !action.IsStep(byID["mch_langkah"]) {
		t.Error("action.IsStep(mch_langkah) = false -- its Application declares it the step of the document_approval engine")
	}
	if action.IsDocument(byID["mch_langkah"]) || action.IsStep(byID["mch_surat"]) {
		t.Error("the two roles are interchangeable -- each Machine must answer for the role it was actually given, or every caller that branches on the pair takes whichever branch it tests first")
	}
	if action.IsDocument(byID["mch_arsip"]) || action.IsStep(byID["mch_arsip"]) {
		t.Error("action.IsDocument/IsStep(mch_arsip) = true -- a Machine its Application claims but gives no role is not part of the engine's cast")
	}
}

// TestUnboundMachinesAreNotTheEngines is the other half, over the real installed Workspaces rather
// than a fixture: every Machine belonging to an Application that binds no engine must answer no.
//
// "dokter-kecil" is why this is worth an assertion instead of an argument. It holds its own
// generated mch_document, under the same id Document Approval's template uses, and the day a bare id
// comparison met it two pages panicked. Here it is the control: same id, no binding, correctly not
// the engine's.
func TestUnboundMachinesAreNotTheEngines(t *testing.T) {
	installed, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}

	bound, checked := 0, 0
	for slug, ws := range installed {
		engines := map[string]string{} // machine id -> the engine its Application bound
		for _, app := range ws.Workspace.Applications {
			if app.Workflow == nil {
				continue
			}
			for _, machineID := range app.Workflow.Roles {
				engines[machineID] = app.Workflow.Engine
			}
		}
		for _, m := range ws.Workspace.Machines {
			checked++
			isEngines := action.IsDocument(m) || action.IsStep(m)
			if engines[m.ID] == "" && isEngines {
				t.Errorf("%s: action.IsDocument/IsStep(%s) = true, but no Application in this Workspace binds it to an engine -- the predicates must read the declaration, not a name", slug, m.ID)
			}
			if engines[m.ID] == domain.WorkflowEngineDocumentApproval {
				bound++
				if !isEngines {
					t.Errorf("%s: %s is declared in the document_approval binding, but neither predicate recognises it", slug, m.ID)
				}
			}
		}
	}
	if checked == 0 || bound == 0 {
		t.Fatalf("checked %d Machines, %d of them bound -- with nothing bound this gate passes on an empty tree, and the installed metadata is expected to carry at least one binding", checked, bound)
	}
}
