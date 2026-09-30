package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/registry"
)

// The three gates in this file exist because of one outage, and they are split the way they are
// because each closes a different half of it.
//
// **What happened (2026-09-29 to 2026-09-30).** Tahap A replaced three hand-written Document-to-Step
// index loops in internal/composition with one declared Relation, `ds_documents_with_steps`, and added
// it to the template library (`metadata/document.yaml`) and to `default`'s own copy. Installing an
// Application *copies* it (CLAUDE.md, 2026-09-27), so a later library change never reaches a Workspace
// that installed earlier: `hanomerch` and `dokter-kecil` both cast the approval engine's roles, neither
// declared the Dataset, and `composition.selectRecords` answers a missing Dataset with an error. Every
// approval screen in both Workspaces returned 500, unconditionally, for a day.
//
// **Why nothing caught it.** The Dataset id is named from Go -- which is exactly why
// `internal/installer` refuses to rename a Dataset id rather than renaming it the way it renames a
// Machine id -- and no declaration said the engine needed it. So `TestNavigationRoutesAreRegistered`
// saw a registered handler, the loader saw valid YAML, `TestInstalledCastsExplainWithoutDefects` saw
// every workflow derivation resolve, and the whole suite stayed green. A requirement that lives only
// inside a Go constant is reachable only through the failure it causes.
//
// The fix was to make the engine **declare** it (`registry.WorkflowEngineSpec.Datasets`) and the loader
// refuse a Workspace that does not provide it (`metadata.validateWorkflowDatasets`). These three gates
// hold the parts a load check structurally cannot.

// TestEveryEngineDatasetIsNamedByComposition is the drift gate, and the direction it checks is the one
// that fails silently.
//
// A Dataset in the registry that no composed screen selects through is a requirement imposed on every
// installation for nothing: a Workspace refuses to load until it declares something the runtime never
// reads. That is worse than the outage it guards, because it breaks a Workspace that was working.
//
// The opposite direction is deliberately *not* checked here -- see
// TestGoNamedDatasetsWithNoEngineRequirement below, which records it as a measured population rather
// than gating it, and says why.
func TestEveryEngineDatasetIsNamedByComposition(t *testing.T) {
	named := datasetIDsNamedIn(t, filepath.Join(repoRoot(), "internal", "composition"))
	if len(named) == 0 {
		t.Fatal("found no ds_ literals in internal/composition -- this gate would pass by measuring nothing")
	}
	for engine, spec := range registry.KnownWorkflowEngines {
		for _, role := range sortedKeys(spec.Datasets) {
			for _, id := range spec.DatasetsFor(role) {
				if !named[id] {
					t.Errorf("engine %q requires dataset %q of its %q role, but no file in internal/composition names it -- every Workspace is forced to declare something nothing selects through. Remove the entry, or name the Dataset where it is selected",
						engine, id, role)
				}
			}
		}
	}
}

// TestEveryCastRoleProvidesItsEngineDatasets is the control over reality: the outage itself, asserted
// against the real manifests through the real loader.
//
// It is not redundant with `validateWorkflowDatasets` being a load error. A load error only fires for a
// manifest something actually loads, and this repo's own history is of Workspaces nothing loaded in a
// test: `TestNavigationRoutesAreRegistered` read `default.yaml` alone until 2026-09-29, which is how
// five of dokter-kecil's declared routes were checked by nothing for days. This sweeps every manifest in
// the directory, the way the loader scans it, and fails if the set is empty.
func TestEveryCastRoleProvidesItsEngineDatasets(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	if len(wss) == 0 {
		t.Fatal("no Workspace manifests found -- this gate would pass by measuring nothing")
	}

	checked := 0
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		byID := map[string]*domain.Machine{}
		for _, m := range ws.Machines {
			byID[m.ID] = m
		}
		for _, app := range ws.Applications {
			if app.Workflow == nil {
				continue
			}
			spec, known := registry.KnownWorkflowEngines[app.Workflow.Engine]
			if !known {
				t.Errorf("workspace %q application %q: engine %q is not in registry.KnownWorkflowEngines", slug, app.ID, app.Workflow.Engine)
				continue
			}
			for _, role := range sortedKeys(spec.Datasets) {
				machineID := app.Workflow.MachineForRole(role)
				if machineID == "" {
					continue // an uncast optional role owes nothing
				}
				m := byID[machineID]
				if m == nil {
					t.Errorf("workspace %q application %q: %q role names machine %q, which the Workspace does not install", slug, app.ID, role, machineID)
					continue
				}
				for _, want := range spec.DatasetsFor(role) {
					checked++
					if !declaresDataset(m, want) {
						t.Errorf("workspace %q: machine %q plays %q's %q role but declares no dataset %q -- every screen selecting through it returns 500",
							slug, machineID, app.Workflow.Engine, role, want)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no installed Workspace casts a role that owes a Dataset -- this gate is measuring nothing, which is how the outage it guards went unseen")
	}
}

// goNamedDatasetsWithNoEngineRequirement is the **measured remainder**, recorded rather than gated --
// and reading it is the point of this file.
//
// Every id here is selected through by name from internal/composition and required by no engine, so the
// class the outage belongs to is closed for the approval engine and *open* for these. A Workspace can
// still install a screen whose Dataset its Machines do not declare and get the same 500.
//
// **It is a list and not a gate because the missing half is a question no scan answers**: which
// Workspaces can reach the screen that selects through this id. `ds_recent_activity` and
// `ds_activity_feed` are safe by accident of scope -- they sit on `mch_activity`, a shared runtime
// reference every Workspace gets the same copy of, so no divergence is possible. The five Case 19 ids
// sit on `mch_task`/`mch_user`/`mch_project`, and whether a Workspace reaches them depends on which
// navigation routes its Applications declare -- a route-to-Dataset mapping that needs the static call
// graph walk `TestGetRoutesDoNotWrite` already performs, pointed at a different question.
//
// Recorded with that cost stated, so "the Dataset class is closed" is not read off a green run. The
// honest scope: the engine half is closed by declaration, the Case 19 half is closed by nothing and
// currently safe only because those Machines are declared once per Workspace by the same template.
// Building the route-to-Dataset walk is the work; the trigger is a Case 19 Machine diverging between
// Workspaces the way the approval Machines already have.
var goNamedDatasetsWithNoEngineRequirement = map[string]string{
	"ds_recent_activity": "on mch_activity, a shared runtime reference -- every Workspace gets the same copy, so it cannot diverge",
	"ds_activity_feed":   "on mch_activity, same shared copy",
	"ds_my_tasks":        "on mch_task (Task Tracker); reachable only from routes that Workspace's Applications declare",
	"ds_task_workload":   "on mch_task; same",
	"ds_task_by_project": "on mch_task; same",
	"ds_task_by_status":  "on mch_task; same",
	"ds_user_capacity":   "on mch_user, a shared runtime reference",
	// Not on this list and not required by the engine either: ds_document_by_status. Both real
	// Workspaces happen to declare it on the Machine their approval Application casts, so adding it to
	// Datasets would be correct -- and it is deliberately left out, because the Dashboard's Document
	// tiles are declared by *Project Management*, not by the approval Application, so "the engine
	// requires it" would be the wrong reason for a true statement. See the file comment: a name is not
	// a reason.
	"ds_document_by_status": "selected by the Dashboard, which Project Management declares -- not the approval engine's own screen, so the engine must not claim it",
}

// TestGoNamedDatasetsWithNoEngineRequirement keeps the list above honest in both directions: an id that
// stops being named in Go, and one that starts being named without landing in the list or in an
// engine's Datasets. A stale entry fails, the same terms every named population in this package carries.
func TestGoNamedDatasetsWithNoEngineRequirement(t *testing.T) {
	named := datasetIDsNamedIn(t, filepath.Join(repoRoot(), "internal", "composition"))

	required := map[string]bool{}
	for _, spec := range registry.KnownWorkflowEngines {
		for _, ids := range spec.Datasets {
			for _, id := range ids {
				required[id] = true
			}
		}
	}

	for id := range named {
		if required[id] {
			continue
		}
		if _, listed := goNamedDatasetsWithNoEngineRequirement[id]; !listed {
			t.Errorf("internal/composition names dataset %q, which no engine requires and goNamedDatasetsWithNoEngineRequirement does not record -- a Workspace whose Machines omit it gets the 500 this file exists for. Either declare it in an engine's Datasets, or add it to the list with the reason it is safe",
				id)
		}
	}
	for id, reason := range goNamedDatasetsWithNoEngineRequirement {
		if !named[id] {
			t.Errorf("goNamedDatasetsWithNoEngineRequirement records %q (%s) but internal/composition no longer names it -- remove the entry", id, reason)
		}
		if required[id] {
			t.Errorf("goNamedDatasetsWithNoEngineRequirement records %q but an engine now requires it -- remove the entry, the declaration covers it", id)
		}
	}
}

func declaresDataset(m *domain.Machine, id string) bool {
	for _, ds := range m.Datasets {
		if ds.ID == id {
			return true
		}
	}
	return false
}

// datasetIDsNamedIn collects every Dataset id a package's non-test Go files name -- **both** as a
// "ds_..." string literal and as a reference to one of internal/domain's own ds_-valued constants.
//
// The second half is not defensive: writing this gate with literals alone made it fail on correct code
// the same minute. Pointing composition's `documentsWithStepsDataset` at `domain.DatasetDocumentsWithSteps`
// is what stops the selector and the engine's requirement being two literals that agree (001 #8) -- and
// it is exactly what deletes the literal this gate was looking for. A scan that only sees literals
// therefore punishes the fix, which is the shape of gate this repo has deleted before.
func datasetIDsNamedIn(t *testing.T, dir string) map[string]bool {
	t.Helper()
	// domain's ds_-valued constants, by name, so a `domain.DatasetX` reference below resolves to the id
	// it actually holds rather than being counted as an opaque identifier.
	byConstName := map[string]string{}
	for name, value := range stringConstantsIn(t, filepath.Join(repoRoot(), "internal", "domain")) {
		if strings.HasPrefix(value, "ds_") {
			byConstName[name] = value
		}
	}

	out := map[string]bool{}
	for _, file := range parsePackage(t, dir) {
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.BasicLit:
				if v.Kind == token.STRING {
					if s := strings.Trim(v.Value, `"`); strings.HasPrefix(s, "ds_") {
						out[s] = true
					}
				}
			case *ast.SelectorExpr:
				if pkg, ok := v.X.(*ast.Ident); ok && pkg.Name == "domain" {
					if id, known := byConstName[v.Sel.Name]; known {
						out[id] = true
					}
				}
			}
			return true
		})
	}
	return out
}

// stringConstantsIn returns a package's top-level `const Name = "value"` pairs.
func stringConstantsIn(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, file := range parsePackage(t, dir) {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if ok && lit.Kind == token.STRING {
						out[name.Name] = strings.Trim(lit.Value, `"`)
					}
				}
			}
		}
	}
	return out
}

func parsePackage(t *testing.T, dir string) []*ast.File {
	t.Helper()
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	var files []*ast.File
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		t.Fatalf("parse %s: no non-test Go files", dir)
	}
	return files
}
