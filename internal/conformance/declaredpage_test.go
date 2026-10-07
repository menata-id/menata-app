package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
	"menata.app/internal/metadata"
	"menata.app/internal/registry"
)

// The two gates here are the `page:` block's post-primitive gates (007 §12.4, §15.3), installed after the
// primitive and its first consumers exist -- build, migrate, then gate -- so each tests a failure class that
// can now happen.
//
// **Probe, not prediction (2026-10-07).** With `validatePages` disabled at its one call in
// `metadata.LoadApplication`, the suites for `metadata`, `conformance`, `composition` and `ir` were run: exactly
// one test failed, `metadata.TestPage_faultsAreRefusedAtLoad`, a unit test over a fixture. No gate over the
// *installed* manifests noticed, so a Workspace could have shipped a `page:` the loader no longer checked and
// the suite would have stayed green until a visitor got a 500. The first gate closes that.

// TestEveryInstalledPageLowersAndValidates sweeps every installed Workspace, the way the loader scans the
// directory, and checks each declared `page:` against `ir.Validate` and the Component validators itself.
//
// It is not redundant with `metadata.validatePages` being a load error, for the reason
// `TestEveryCastRoleProvidesItsEngineDatasets` states: a load error fires only if the loader still makes the
// call. This holds the *outcome* against the real manifests -- and fails if no Workspace declares a page, so
// it cannot pass by measuring nothing.
func TestEveryInstalledPageLowersAndValidates(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	ir.RegisterComponentTypes(registry.ComponentTypeNames())
	ir.RegisterSlottedComponentTypes(registry.SlottedComponentTypeNames())

	checked := 0
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		datasets := map[string]domain.Dataset{}
		for _, m := range ws.Machines {
			for _, ds := range m.Datasets {
				datasets[ds.ID] = ds
			}
		}
		for _, app := range ws.Applications {
			for _, item := range app.AllNavigation {
				if item.Page == nil {
					continue
				}
				checked++
				where := slug + "/" + app.ID + "/" + item.ID
				if want := "/pages/" + item.ID; item.Route != want {
					t.Errorf("%s: route is %q but a page is rendered at %q", where, item.Route, want)
				}
				tree, err := ir.Lower(*item.Page, func(b domain.PageBinding) ([]ir.Row, error) {
					if _, ok := datasets[b.Dataset]; !ok {
						t.Errorf("%s: binding names dataset %q, which this Workspace does not declare -- the page would answer 500 on every visit", where, b.Dataset)
					}
					return []ir.Row{{Label: "label", Value: "0"}}, nil
				})
				if err != nil {
					t.Errorf("%s: does not lower: %v", where, err)
					continue
				}
				for _, msg := range ir.Validate(tree) {
					t.Errorf("%s: %s", where, msg)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no installed Workspace declares a `page:` -- this gate is measuring nothing, which is how a loader that stopped validating them would go unseen")
	}
}

// TestDeclaredPagePipelineHasNoPerScreenBranch is the `page:` twin of `TestLayoutRenderersHaveNoPerScreenBranch`,
// and the only real test of whether the declared-page path is a primitive: a generic handler, lowering or
// renderer that must know *which screen* it serves is one screen's shape with a new name, which is 007 §12.4's
// MUST NOT from the other side.
//
// It covers every file that touches a declared page between the YAML and the pixels, comments stripped first
// (their own explanations name the ids they must not contain). A Dataset or Field id is held out as well as a
// navigation, Machine and Application id: the Binding is the one place a page names data, and it names it in
// YAML, so a `ds_` or `fld_` literal in this pipeline means a screen's data got hardcoded beside its
// declaration.
func TestDeclaredPagePipelineHasNoPerScreenBranch(t *testing.T) {
	files := []string{
		"internal/domain/page.go",
		"internal/ir/page.go",
		"internal/metadata/page.go",
		"internal/composition/declaredpage.go",
		"internal/web/declaredpage.go",
		"internal/rendering/declaredscreen.templ",
	}
	comment := regexp.MustCompile(`(?m)//.*$`)
	for pattern, why := range map[string]string{
		`"nav_[a-z0-9_]+"`: "a navigation id -- the pipeline would be serving one screen",
		`"mch_[a-z0-9_]+"`: "a Machine id, which is an Application's vocabulary",
		`"app_[a-z0-9_]+"`: "an Application id, never an identity to branch on (2026-09-28)",
		`"ds_[a-z0-9_]+"`:  "a Dataset id -- a Binding names its Dataset in YAML, so this is a screen's data hardcoded beside its declaration",
		`"fld_[a-z0-9_]+"`: "a Field id, which composition resolves through declarations",
	} {
		re := regexp.MustCompile(pattern)
		for _, f := range files {
			body, err := os.ReadFile(filepath.Join(repoRoot(), f))
			if err != nil {
				t.Fatalf("read %s: %v -- the declared-page pipeline moved; update this list rather than letting it measure nothing", f, err)
			}
			if m := re.FindString(comment.ReplaceAllString(string(body), "")); m != "" {
				t.Errorf("%s contains %s: %s", f, m, why)
			}
		}
	}
}
