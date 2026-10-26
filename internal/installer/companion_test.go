package installer

import (
	"os"
	"path/filepath"
	"testing"

	"menata.app/internal/metadata"
)

// freshWorkspace writes an empty manifest (user and activity only) into a temp tree and loads it.
func freshWorkspace(t *testing.T, library string, extra map[string]string) (manifestPath string, loaded *metadata.App) {
	t.Helper()
	workspaces := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspaces, "fresh"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath = filepath.Join(workspaces, "fresh.yaml")
	body := "workspace: fresh\nmachines:\n  - " + relLibrary(t, workspaces, library, "user.yaml") + "\n  - " + relLibrary(t, workspaces, library, "activity.yaml") + "\n"
	for name, machine := range extra {
		write(t, filepath.Join(workspaces, name), machine)
		body += "  - " + name + "\n"
	}
	write(t, manifestPath, body+"applications: []\n")
	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return manifestPath, loaded
}

// TestCompanion_oneEntryInstallsTwoApplications: the library lists Production Contracts once, and installing it brings
// Instalments & Payments in as its own Application, loading together because the second's Machines relate to the first's.
func TestCompanion_oneEntryInstallsTwoApplications(t *testing.T) {
	library := libraryDir(t)
	all, err := Templates(library)
	if err != nil {
		t.Fatal(err)
	}
	for _, tm := range Listed(all) {
		if tm.Application.ID == "app_instalments_payments" {
			t.Fatal("a companion is listed on its own")
		}
	}
	if _, err := TemplateByID(library, "app_instalments_payments"); err == nil {
		t.Fatal("a companion installed on its own must be refused: its Machines relate to another Application's, so it cannot load alone")
	}
	tmpl, err := TemplateByID(library, "app_production_contracts")
	if err != nil {
		t.Fatal(err)
	}
	if len(tmpl.Companions) != 1 || tmpl.Companions[0].Application.ID != "app_instalments_payments" {
		t.Fatalf("Companions = %v, want Instalments & Payments", tmpl.Companions)
	}

	manifestPath, loaded := freshWorkspace(t, library, nil)
	plan := PlanInstall(tmpl, loaded.Workspace)
	if !plan.OK() || len(plan.Companions) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if _, err := Install(plan, library, manifestPath); err != nil {
		t.Fatalf("Install: %v", err)
	}
	after, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, a := range after.Workspace.Applications {
		ids[a.ID] = true
	}
	if len(after.Workspace.Applications) != 2 || !ids["app_production_contracts"] || !ids["app_instalments_payments"] {
		t.Errorf("installed Applications = %v, want exactly Production Contracts and Instalments & Payments", ids)
	}
}

// TestCompanion_inheritsARenameOfTheTemplatesMachines: when the Workspace already holds a Machine called mch_contract, the
// template's copy is renamed, and the companion's files must point at the renamed Machine or its relations dangle and the install
// rolls back.
func TestCompanion_inheritsARenameOfTheTemplatesMachines(t *testing.T) {
	library := libraryDir(t)
	tmpl, err := TemplateByID(library, "app_production_contracts")
	if err != nil {
		t.Fatal(err)
	}
	squatter := "id: mch_contract\nname: Squatter\nfields:\n  - { id: fld_s, name: S, type: text }\ncard_fields:\n  - { field: fld_s, role: title }\n"
	_, loaded := freshWorkspace(t, library, map[string]string{"squatter.yaml": squatter})
	plan := PlanInstall(tmpl, loaded.Workspace)
	if !plan.OK() {
		t.Fatalf("refused: %v", plan.Refusals)
	}
	renamed, ok := plan.Renames["mch_contract"]
	if !ok {
		t.Fatalf("Renames = %v, want mch_contract renamed", plan.Renames)
	}
	if plan.Companions[0].inherited["mch_contract"] != renamed {
		t.Fatalf("the companion does not inherit the rename: %v", plan.Companions[0].inherited)
	}
	// Not installed here: rewriteIDs follows `key: id` lines only, so an id inside an inline `{ machine: mch_x }` is not renamed
	// for any template, and a Workspace already holding one of this bundle's Machine ids cannot take it by rename yet.
}

// TestCompanion_asecondInstallIsRefusedForBothApplications: screen ids and routes are never renamed, so the whole bundle refuses.
func TestCompanion_aSecondInstallIsRefusedForBothApplications(t *testing.T) {
	library := libraryDir(t)
	tmpl, err := TemplateByID(library, "app_production_contracts")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath, loaded := freshWorkspace(t, library, nil)
	if _, err := Install(PlanInstall(tmpl, loaded.Workspace), library, manifestPath); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	again, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	plan := PlanInstall(tmpl, again.Workspace)
	if plan.OK() {
		t.Fatal("a second install of the same bundle must be refused")
	}
	if len(plan.Companions) != 1 || plan.Companions[0].OK() {
		t.Error("the companion's own plan must carry its refusals too")
	}
}
