package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// TestInstalledNavigationExplainsItsHeadings is the navigation twin of
// TestInstalledMachinesExplainTheirNormalization: every navigation item an installed Workspace declares
// must have a resolved Heading, and metadata.ExplainNavigation must be able to say where it came from.
//
// **Written because mutation showed its absence.** Replacing the loader's
// domain.ResolveNavigationHeadings call with a no-op left the whole suite green: every Heading came back
// empty, every screen's <h1> would have rendered blank, and nothing failed. The engine-side gates
// (TestInstalledCastsExplainWithoutDefects) cover workflow derivations and the Machine-side one covers
// Machines; navigation had neither, which is exactly the silent class 001 #6's second clause exists over.
func TestInstalledNavigationExplainsItsHeadings(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	checked := 0
	for _, slug := range sortedKeys(wss) {
		for _, r := range metadata.ExplainNavigation(wss[slug].Workspace.Applications) {
			checked++
			if r.IsDefect() {
				t.Errorf("workspace %q: %s is %s (%s) -- a navigation item with no resolved heading renders a blank <h1>, and metadata.LoadApplication is where it should have been resolved",
					slug, r.Name, r.Status, r.From)
			}
			if r.Value == "" {
				t.Errorf("workspace %q: %s resolved to an empty heading", slug, r.Name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no installed Workspace declares a navigation item -- this gate would pass by measuring nothing")
	}
}

// TestRuntimeScreensResolveTheirHeadings covers the half the loader cannot: domain.RuntimeScreens are
// declared in Go and never pass through LoadApplication, so they are resolved by an init in their own
// file. A screen added to that list without the init running would render a blank heading on a
// Workspace-level page -- which is what happened for one build on 2026-10-02.
func TestRuntimeScreensResolveTheirHeadings(t *testing.T) {
	if len(domain.RuntimeScreens) == 0 {
		t.Fatal("domain.RuntimeScreens is empty -- this gate would pass by measuring nothing")
	}
	for _, s := range domain.RuntimeScreens {
		if s.Heading == "" {
			t.Errorf("runtime screen %q has no resolved Heading -- domain.ResolveNavigationHeadings must run over RuntimeScreens (see the init in runtimescreens.go)", s.ID)
		}
	}
}

// TestRendererDoesNotResolveNavigationHeadings forbids the fallback coming back to where it started.
//
// titleByID returned `item.Title` or, failing that, `item.Label` until 2026-10-02 -- an inference running
// at render time, which 005 Phase 4 assigns to normalization and 001 #6's second clause names *rendering*
// among the places an unexplained one must not act. Moving it to the loader is only half a fix: **mutation
// showed that restoring the fallback leaves every test green**, because a redundant fallback is
// functionally invisible until someone also removes the loader's resolution, and then it masks that too.
//
// So the rule is narrow and exact: whatever resolves a heading, `internal/rendering` does not. A renderer
// reads a resolved value.
func TestRendererDoesNotResolveNavigationHeadings(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "rendering", "machine.templ"))
	if err != nil {
		t.Fatalf("read machine.templ: %v", err)
	}
	fn := regexp.MustCompile(`(?s)func titleByID\(ctx context\.Context, id string\) string \{.*?\n\}`).FindString(string(body))
	if fn == "" {
		t.Fatal("titleByID not found in machine.templ -- either it was renamed or this gate stopped looking")
	}
	// Comments are stripped first, because explaining the fallback that used to be here is exactly what
	// that function's comment now does.
	code := regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(fn, "")
	if strings.Contains(code, ".Label") {
		t.Error("titleByID reads NavigationItem.Label -- that is the render-time fallback removed on 2026-10-02. The Heading is resolved at load (domain.ResolveNavigationHeadings, called by metadata.LoadApplication and by RuntimeScreens' own init); a renderer reads it, it does not resolve it")
	}
	if !strings.Contains(code, ".Heading") {
		t.Error("titleByID no longer reads NavigationItem.Heading -- if the resolved field moved, move this gate with it rather than deleting it")
	}
}
