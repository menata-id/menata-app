package installer

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// These tests run against the **real template library** (metadata/), not a fixture, for the reason
// this package exists at all: the thing being installed is those files, and a fixture proving a copy
// of a two-field Machine works would say nothing about Document Approval's 266-line one with its
// relations, events, datasets, views and workflow binding.
//
// The target Workspace is a temp tree every time, so nothing here can touch the repo's own
// metadata/workspaces/.

func libraryDir(t *testing.T) string {
	t.Helper()
	// internal/installer -> repo root -> metadata
	dir, err := filepath.Abs(filepath.Join("..", "..", "metadata"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// documentApprovalTemplate is the template every test below installs -- the one the owner actually
// asked for.
func documentApprovalTemplate(t *testing.T) Template {
	t.Helper()
	tmpl, err := TemplateByID(libraryDir(t), "app_document_approval")
	if err != nil {
		t.Fatalf("TemplateByID: %v", err)
	}
	return tmpl
}

// TestTemplates_readsTheRealLibrary is the shape assertion: the library is readable as data, and each
// template resolves every Machine it claims to a real file.
func TestTemplates_readsTheRealLibrary(t *testing.T) {
	all, err := Templates(libraryDir(t))
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("Templates() returned %d templates, want the library's own (Document Approval and Project Management at least)", len(all))
	}

	tmpl := documentApprovalTemplate(t)
	if got := len(tmpl.Machines); got != 5 {
		t.Errorf("Document Approval claims %d machines, want 5", got)
	}
	for _, tm := range tmpl.Machines {
		if tm.File == "" {
			t.Errorf("machine %s resolved to no file", tm.Machine.ID)
		}
		if SharedMachineIDs[tm.Machine.ID] {
			t.Errorf("machine %s is runtime-level and must never be one a template copies", tm.Machine.ID)
		}
	}
	// Derived from its own metadata, not listed anywhere: four of its Events declare
	// send_notification, its Fields relate to mch_user, and it logs activity.
	want := map[string]bool{"mch_activity": true, "mch_notification": true, domain.UserMachineID: true}
	for _, id := range tmpl.RequiresShared {
		if !want[id] {
			t.Errorf("RequiresShared includes %q, which its metadata does not imply", id)
		}
		delete(want, id)
	}
	for id := range want {
		t.Errorf("RequiresShared is missing %q, which Document Approval's own metadata implies", id)
	}
}

// TestPlanInstall_intoAnEmptyWorkspace is the majority case and the property most worth protecting: no
// collision means no rename, which is what makes an installed copy diffable against its template.
func TestPlanInstall_intoAnEmptyWorkspace(t *testing.T) {
	tmpl := documentApprovalTemplate(t)
	ws := domain.Workspace{Slug: "fresh", MachineIDs: []string{domain.UserMachineID, "mch_activity"}}

	p := PlanInstall(tmpl, ws)
	if !p.OK() {
		t.Fatalf("PlanInstall refused an empty Workspace: %v", p.Refusals)
	}
	if len(p.Renames) != 0 {
		t.Errorf("Renames = %v, want none -- nothing collides here, so every copy must be byte-identical", p.Renames)
	}
	if got := p.AddShared; len(got) != 1 || got[0] != "mch_notification" {
		t.Errorf("AddShared = %v, want [mch_notification] -- this Workspace has user and activity but no notification, and the template declares send_notification events", got)
	}
}

// TestPlanInstall_renamesOnlyWhatCollides is the dokter-kecil shape, read from that Workspace's own
// manifest: mch_document is taken by an unrelated generated Application, the other four are free.
func TestPlanInstall_renamesOnlyWhatCollides(t *testing.T) {
	tmpl := documentApprovalTemplate(t)
	ws := domain.Workspace{
		Slug:       "dokter-kecil",
		MachineIDs: []string{domain.UserMachineID, "mch_activity", "mch_document", "mch_document_item"},
		Applications: []domain.Application{
			{ID: "app_document_tracking", Machines: []string{"mch_document"}, AllNavigation: []domain.NavigationItem{
				{ID: "nav_document_tracking", Route: "/machines/mch_document"},
			}},
		},
	}

	p := PlanInstall(tmpl, ws)
	if !p.OK() {
		t.Fatalf("PlanInstall refused: %v", p.Refusals)
	}
	if got, want := p.Renames, map[string]string{"mch_document": "mch_document_approval"}; len(got) != len(want) || got["mch_document"] != want["mch_document"] {
		t.Errorf("Renames = %v, want %v -- only the colliding id, named after the Application installing it", got, want)
	}
	if p.ApplicationID() != "app_document_approval" {
		t.Errorf("ApplicationID() = %q, want the template's own -- it does not collide here", p.ApplicationID())
	}
	if p.RenamedMachineID("mch_approval_step") != "mch_approval_step" {
		t.Error("a Machine that does not collide must keep its id")
	}
}

// A second candidate is taken too, so the rename falls through to a number. Deterministic, and the
// only case where the readable name is unavailable.
func TestPlanInstall_fallsThroughToANumberedSuffix(t *testing.T) {
	tmpl := documentApprovalTemplate(t)
	ws := domain.Workspace{Slug: "busy", MachineIDs: []string{"mch_document", "mch_document_approval"}}

	p := PlanInstall(tmpl, ws)
	if got := p.Renames["mch_document"]; got != "mch_document_2" {
		t.Errorf("Renames[mch_document] = %q, want mch_document_2 -- mch_document_approval is taken as well", got)
	}
}

// The Application's own id is renameable too, and only became so when the approval engine stopped
// matching it (ROADMAP.md Stage A). Before that a second copy under a different name would have
// installed and then never engaged.
func TestPlanInstall_renamesTheApplicationWhenItsIdIsTaken(t *testing.T) {
	tmpl := documentApprovalTemplate(t)
	ws := domain.Workspace{
		Slug:         "twice",
		Applications: []domain.Application{{ID: "app_document_approval"}},
	}

	p := PlanInstall(tmpl, ws)
	if got := p.ApplicationID(); got != "app_document_approval_approval" {
		t.Errorf("ApplicationID() = %q, want a renamed id", got)
	}
}

// The three refusals, each because Go still names the thing. Renaming any of them would recreate the
// coupling the two slices before this one removed -- see PlanInstall's own doc comment.
func TestPlanInstall_refusesWhatItWillNotRename(t *testing.T) {
	tmpl := documentApprovalTemplate(t)

	tests := []struct {
		name string
		ws   domain.Workspace
		want string
	}{
		{
			name: "a navigation route already answering here",
			ws: domain.Workspace{Slug: "x", Applications: []domain.Application{{
				ID: "app_other", AllNavigation: []domain.NavigationItem{{ID: "nav_other", Route: "/approval-inbox"}},
			}}},
			want: `the address "/approval-inbox" is already used`,
		},
		{
			name: "a navigation id already answering here",
			ws: domain.Workspace{Slug: "x", Applications: []domain.Application{{
				ID: "app_other", AllNavigation: []domain.NavigationItem{{ID: "nav_approval_inbox", Route: "/somewhere-else"}},
			}}},
			want: `the screen id "nav_approval_inbox" is already used`,
		},
		{
			name: "a dataset id already declared here",
			ws: domain.Workspace{Slug: "x", Machines: []*domain.Machine{
				{ID: "mch_other", Datasets: []domain.Dataset{{ID: "ds_document_by_status"}}},
			}},
			want: `the saved figure "ds_document_by_status" is already defined`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PlanInstall(tmpl, tt.ws)
			if p.OK() {
				t.Fatalf("PlanInstall accepted it, want a refusal containing %q", tt.want)
			}
			if !strings.Contains(strings.Join(p.Refusals, "\n"), tt.want) {
				t.Errorf("Refusals = %v, want one containing %q", p.Refusals, tt.want)
			}
			// And the refusal is enforced by the write, not only by the screen that shows it.
			if _, err := Install(p, libraryDir(t), filepath.Join(t.TempDir(), "x.yaml")); err == nil {
				t.Error("Install() error = nil, want it to refuse a plan carrying refusals")
			}
		})
	}
}

// TestInstall_intoAWorkspaceWithACollision is the acceptance test: the real Document Approval template
// into a Workspace that already uses mch_document, ending in a manifest that *loads* and an engine that
// engages for the renamed Machine.
func TestInstall_intoAWorkspaceWithACollision(t *testing.T) {
	library := libraryDir(t)
	before := librarySnapshot(t, library)

	workspaces := t.TempDir()
	targetDir := filepath.Join(workspaces, "kecil")
	if err := os.MkdirAll(filepath.Join(targetDir, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A Workspace whose own Machine is already called mch_document -- exactly dokter-kecil's shape,
	// including the two shared references it has and the one (notification) it lacks.
	write(t, filepath.Join(targetDir, "document.yaml"), "id: mch_document\nname: Tracked Document\nfields:\n  - id: fld_title\n    name: Title\n    type: text\n")
	write(t, filepath.Join(targetDir, "applications", "tracking.yaml"), "id: app_document_tracking\nname: Document Tracking\nmachines:\n  - mch_document\nnavigation:\n  - id: nav_document_tracking\n    label: Documents\n    route: /machines/mch_document\n    home_card: true\n")
	manifestPath := filepath.Join(workspaces, "kecil.yaml")
	write(t, manifestPath, "workspace: kecil\nmachines:\n  - "+relLibrary(t, workspaces, library, "user.yaml")+"\n  - "+relLibrary(t, workspaces, library, "activity.yaml")+"\n  - kecil/document.yaml\napplications:\n  - kecil/applications/tracking.yaml\n")

	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("the fixture Workspace must load before anything is installed into it: %v", err)
	}

	plan := PlanInstall(documentApprovalTemplate(t), loaded.Workspace)
	if !plan.OK() {
		t.Fatalf("PlanInstall refused: %v", plan.Refusals)
	}
	appID, err := Install(plan, library, manifestPath)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if appID != "app_document_approval" {
		t.Errorf("Install returned %q, want app_document_approval", appID)
	}

	// The library is untouched. This is the failure that motivated Workspace isolation in the first
	// place, so it is asserted rather than assumed.
	if after := librarySnapshot(t, library); after != before {
		t.Error("the template library changed -- an install may only ever read it")
	}

	after, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("the Workspace no longer loads after the install: %v", err)
	}

	// Both Machines exist, under different ids, and each is the right one.
	byID := map[string]*domain.Machine{}
	for _, m := range after.Workspace.Machines {
		byID[m.ID] = m
	}
	if got := byID["mch_document"]; got == nil || got.Name != "Tracked Document" {
		t.Errorf("the Workspace's own mch_document = %v, want the one it already had, unchanged", got)
	}
	installed := byID["mch_document_approval"]
	if installed == nil {
		t.Fatal("the renamed copy mch_document_approval was not installed")
	}
	if installed.ApplicationID != "app_document_approval" {
		t.Errorf("the renamed copy is claimed by %q, want app_document_approval", installed.ApplicationID)
	}

	// The whole point: the engine engages for the renamed Machine, because the copied Application's
	// workflow: block was rewritten to name it.
	if !action.IsDocument(installed) {
		t.Error("action.IsDocument(mch_document_approval) = false -- the rename must carry the workflow binding with it, or the copy installs and never approves anything")
	}
	if action.IsDocument(byID["mch_document"]) {
		t.Error("the Workspace's own unrelated mch_document must not be the engine's")
	}
	steps := byID["mch_approval_step"]
	if steps == nil || !action.IsStep(steps) {
		t.Error("mch_approval_step did not collide and must have installed unrenamed, still cast as the step")
	}
	// Its relation to the document was rewritten too, or the loader would have refused the manifest.
	if f, ok := steps.FieldByID(action.FieldStepDocument); !ok || f.RelatedMachine != "mch_document_approval" {
		t.Errorf("mch_approval_step.fld_document points at %q, want the renamed copy", f.RelatedMachine)
	}

	// The missing shared reference was added, so its notifications have somewhere to land.
	if _, ok := byID["mch_notification"]; !ok {
		t.Error("mch_notification was not added -- the template declares send_notification events, so installing without it is installing something broken")
	}

	// And the copy carries its own provenance, since its comments still name the template's ids.
	body := read(t, filepath.Join(targetDir, "document_approval.yaml"))
	if !strings.Contains(body, "Installed from metadata/document.yaml") || !strings.Contains(body, "mch_document -> mch_document_approval") {
		t.Error("the installed copy has no provenance header naming the template and the rename")
	}
}

// TestInstall_withNoCollisionCopiesByteForByte is the property that justifies renaming only on
// collision: in the ordinary case a Workspace's copy is diffable against the library it came from.
func TestInstall_withNoCollisionCopiesByteForByte(t *testing.T) {
	library := libraryDir(t)
	workspaces := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspaces, "fresh"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspaces, "fresh.yaml")
	write(t, manifestPath, "workspace: fresh\nmachines:\n  - "+relLibrary(t, workspaces, library, "user.yaml")+"\n  - "+relLibrary(t, workspaces, library, "activity.yaml")+"\napplications: []\n")

	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	plan := PlanInstall(documentApprovalTemplate(t), loaded.Workspace)
	if len(plan.Renames) != 0 {
		t.Fatalf("Renames = %v, want none", plan.Renames)
	}
	if _, err := Install(plan, library, manifestPath); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Identical apart from the provenance header, which is the only thing an uncollided copy adds.
	original := read(t, filepath.Join(library, "document.yaml"))
	copied := read(t, filepath.Join(workspaces, "fresh", "document.yaml"))
	_, body, found := strings.Cut(copied, "internal/installer).\n\n")
	if !found {
		t.Fatalf("the copy has no provenance header:\n%s", firstLines(copied, 4))
	}
	if body != original {
		t.Error("an install with no rename must copy the template byte for byte below its header -- that is what keeps a Workspace's copy diffable against the library")
	}
}

// TestInstall_rollsBackWhenTheResultWouldNotLoad is the safety net the whole design leans on: a rewrite
// that leaves the Workspace unloadable removes itself, rather than leaving a tree that serves fine
// until the next restart.
//
// Provoked honestly: the target Workspace does not declare mch_user, which Document Approval's person
// Fields require, so the manifest cannot load once the copy is in place. Nothing about the install
// itself is sabotaged -- this is a real, reachable failure.
func TestInstall_rollsBackWhenTheResultWouldNotLoad(t *testing.T) {
	library := libraryDir(t)
	workspaces := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspaces, "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspaces, "broken.yaml")
	manifest := "workspace: broken\nmachines:\n  - " + relLibrary(t, workspaces, library, "activity.yaml") + "\napplications: []\n"
	write(t, manifestPath, manifest)

	tmpl := documentApprovalTemplate(t)
	plan := PlanInstall(tmpl, domain.Workspace{Slug: "broken", MachineIDs: []string{"mch_activity"}})
	// mch_user is missing from this Workspace, so RequiresShared wants to add it -- remove that, to
	// leave the install genuinely unable to load rather than quietly fixing itself.
	plan.AddShared = nil

	if _, err := Install(plan, library, manifestPath); err == nil {
		t.Fatal("Install() error = nil, want a failure: this Workspace cannot satisfy the template's person fields")
	} else if !strings.Contains(err.Error(), "rolled back") {
		t.Errorf("Install() error = %v, want it to say the install was rolled back", err)
	}

	if got := read(t, manifestPath); got != manifest {
		t.Errorf("the manifest was left modified:\n%s\nwant it restored to:\n%s", got, manifest)
	}
	for _, name := range []string{"document.yaml", "approval_step.yaml", "signature.yaml"} {
		if _, err := os.Stat(filepath.Join(workspaces, "broken", name)); !os.IsNotExist(err) {
			t.Errorf("%s survived a rolled-back install", name)
		}
	}
}

// --- helpers -----------------------------------------------------------------------------------

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// relLibrary is the path a temp Workspace's manifest uses to reference a shared library file. The real
// manifests say "../user.yaml"; a temp tree's relationship to the repo's metadata/ is different, so it
// is computed rather than written.
func relLibrary(t *testing.T, workspacesDir, library, name string) string {
	t.Helper()
	rel, err := filepath.Rel(workspacesDir, filepath.Join(library, name))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

// librarySnapshot is every library file's name and bytes, as one comparable string -- the cheapest
// honest way to assert "this directory did not change".
func librarySnapshot(t *testing.T, library string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(library, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		// metadata/workspaces/ lives under metadata/ but is not the library -- it is where installs land.
		if strings.Contains(filepath.ToSlash(path), "/workspaces/") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.WriteString(path)
		b.WriteString("\x00")
		b.Write(body)
		b.WriteString("\x00")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// TestPlanInstall_againstTheRealDokterKecilManifest is the acceptance case asserted against the actual
// Workspace it is about, not a fixture shaped like it -- read-only, so it writes nothing into the repo.
//
// It is the question the owner asked on 2026-09-27 ("aku ingin pakai di workspace lain"), which had no
// answer for two days: dokter-kecil holds mch_document for a generated Document Tracking, so Document
// Approval's own mch_document collided and nothing offered to resolve it. This says what the resolution
// now is, in one read, against the real file.
func TestPlanInstall_againstTheRealDokterKecilManifest(t *testing.T) {
	manifest := filepath.Join(libraryDir(t), "workspaces", "dokter-kecil.yaml")
	if _, err := os.Stat(manifest); os.IsNotExist(err) {
		t.Skip("dokter-kecil is not installed in this checkout")
	}
	loaded, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatalf("load the real dokter-kecil manifest: %v", err)
	}

	p := PlanInstall(documentApprovalTemplate(t), loaded.Workspace)
	if !p.OK() {
		t.Fatalf("Document Approval cannot be installed into dokter-kecil: %v", p.Refusals)
	}
	if got, want := p.Renames["mch_document"], "mch_document_approval"; got != want {
		t.Errorf("Renames[mch_document] = %q, want %q", got, want)
	}
	if len(p.Renames) != 1 {
		t.Errorf("Renames = %v, want only the one collision -- the other four Machine ids are free there", p.Renames)
	}
	if got := p.AddShared; len(got) != 1 || got[0] != "mch_notification" {
		t.Errorf("AddShared = %v, want [mch_notification] -- dokter-kecil has user and activity, and Document Approval sends notifications", got)
	}
}

// TestInstall_intoTheRealDokterKecilWorkspace is the *write* half against metadata that really
// collides. TestPlanInstall_againstTheRealDokterKecilManifest above covers the plan; nothing covered
// the install itself running over a real Workspace's own files.
//
// The distinction matters because a plan is a map of intentions and an install is bytes on disk: the
// rename has to reach every place the copied Machine's id appears, the Workspace's own copies have to
// land under its own directory, the manifest has to gain a line per file, and the result has to load
// through the real loader or roll back. TestInstall_intoAWorkspaceWithACollision asserts all of that
// against a *synthetic* Workspace built to dokter-kecil's shape; this one asserts it against
// dokter-kecil's actual metadata, which is where a real difference would hide.
//
// The repo's own tree is never written to: the manifest and its directory are copied into t.TempDir()
// first, with the two shared-library references rewritten the way relLibrary already does for the
// synthetic fixtures.
func TestInstall_intoTheRealDokterKecilWorkspace(t *testing.T) {
	library := libraryDir(t)
	realManifest := filepath.Join(library, "workspaces", "dokter-kecil.yaml")
	if _, err := os.Stat(realManifest); os.IsNotExist(err) {
		t.Skip("dokter-kecil is not installed in this checkout")
	}
	before := librarySnapshot(t, library)

	workspaces := t.TempDir()
	manifestPath := copyWorkspaceForInstall(t, library, realManifest, workspaces, "dokter-kecil")

	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("the copied dokter-kecil must load before anything is installed into it: %v", err)
	}
	plan := PlanInstall(documentApprovalTemplate(t), loaded.Workspace)
	if !plan.OK() {
		t.Fatalf("PlanInstall refused: %v", plan.Refusals)
	}
	if _, err := Install(plan, library, manifestPath); err != nil {
		t.Fatalf("Install into the real dokter-kecil shape: %v", err)
	}

	// The library is only ever read. This is the failure Workspace isolation exists to prevent -- a
	// generated Application once overwrote metadata/document.yaml -- so it is asserted, not assumed.
	if after := librarySnapshot(t, library); after != before {
		t.Error("the template library changed -- an install may only ever read it")
	}

	after, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("dokter-kecil no longer loads after the install: %v", err)
	}

	// The rename landed in the *bytes*, not just in the plan: the Workspace now holds two Machines that
	// were both called mch_document in their own sources, under two ids.
	ids := map[string]bool{}
	for _, m := range after.Machines {
		ids[m.ID] = true
	}
	for _, want := range []string{"mch_document", "mch_document_approval", "mch_approval_step", "mch_notification"} {
		if !ids[want] {
			t.Errorf("after the install dokter-kecil has no %s (has %v)", want, keysOf(ids))
		}
	}

	// And the installed Application's own copies are the Workspace's, under its own directory -- the
	// half TestInstalledApplicationsAreCopiesNotSharedFiles holds for `default`.
	entries, err := os.ReadDir(filepath.Join(workspaces, "dokter-kecil"))
	if err != nil {
		t.Fatalf("read the Workspace's own directory: %v", err)
	}
	var copied []string
	for _, e := range entries {
		copied = append(copied, e.Name())
	}
	for _, want := range []string{"approval_step.yaml", "document_approval.yaml"} {
		if !slices.Contains(copied, want) {
			t.Errorf("the Workspace's own directory has no %s (has %v) -- an install copies, it does not point at the library", want, copied)
		}
	}
}

// copyWorkspaceForInstall copies one real manifest and its Workspace directory into a temp tree,
// rewriting the `../name.yaml` shared-library references to reach the real library from there. Returns
// the copied manifest's path.
func copyWorkspaceForInstall(t *testing.T, library, manifest, workspaces, slug string) string {
	t.Helper()
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("read %s: %v", manifest, err)
	}
	rewritten := regexp.MustCompile(`\.\./([a-z_]+\.yaml)`).ReplaceAllStringFunc(string(body), func(match string) string {
		return relLibrary(t, workspaces, library, strings.TrimPrefix(match, "../"))
	})
	target := filepath.Join(workspaces, slug+".yaml")
	write(t, target, rewritten)

	source := filepath.Join(filepath.Dir(manifest), slug)
	err = filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(source, path)
		if relErr != nil {
			return relErr
		}
		dest := filepath.Join(workspaces, slug, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		write(t, dest, string(content))
		return nil
	})
	if err != nil {
		t.Fatalf("copy %s: %v", source, err)
	}
	return target
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
