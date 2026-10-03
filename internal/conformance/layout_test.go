package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestLayoutRenderersHaveNoPerScreenBranch is the only real test of whether a layout primitive is a
// primitive.
//
// A generic renderer that has to know *which screen* is calling it is one screen's shape with a new name,
// and that failure is invisible at runtime: every page renders, and the vocabulary is a fiction. 007 §12.2
// names the failure from the other side -- "The runtime should not require separate layout concepts such
// as `DashboardLayout`, `DetailLayout`, or `FormLayout`" -- and building one primitive per screen is the
// most direct route to exactly those.
//
// So `internal/rendering/layout.templ` may not branch on a navigation id, a Machine id, or an Application
// id. It may branch on its own declared vocabulary (`domain.LayoutKind`, `Gap`, `StaticKind`), which is
// what a closed set is for.
func TestLayoutRenderersHaveNoPerScreenBranch(t *testing.T) {
	src := filepath.Join(repoRoot(), "internal", "rendering", "layout.templ")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read layout.templ: %v", err)
	}
	// Comments stripped first: this file's own comments explain the branches it must not have.
	code := regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(string(body), "")

	for pattern, why := range map[string]string{
		`"nav_[a-z_]+"`: "a navigation id -- the primitive would be serving one screen",
		`"mch_[a-z_]+"`: "a Machine id, which is an Application's vocabulary and not a layout's",
		`"app_[a-z_]+"`: "an Application id, which 2026-09-28 established is never an identity to branch on",
	} {
		if m := regexp.MustCompile(pattern).FindString(code); m != "" {
			t.Errorf("internal/rendering/layout.templ contains %s: %s. A layout primitive may branch only on its own declared vocabulary (domain.LayoutKind/Gap/StaticKind)", m, why)
		}
	}

	// pageHeader takes a navigation id as a *parameter* and that is correct -- it composes a screen's own
	// header from declarations. What must not happen is the primitives beneath it knowing which screen.
	if !strings.Contains(code, "templ pageHeader(navID string)") {
		t.Error("pageHeader no longer takes its navigation id as a parameter -- if it resolves one itself, the primitives below it have become screen-aware")
	}
}

// TestLayoutVocabularyIsRenderedAndUsed holds both ends of the closed set: a kind the renderer cannot draw
// is a declaration that silently does nothing, and a kind nothing calls is shape-before-need.
//
// The second direction is the one 007 §34 warns about and the one `capability-lifecycle.md` §4 records
// upstream failing: a seam named and not built is worse than a switch, because the name suggests
// otherwise. Four of §12.2's eight primitives were built because they were *measured in use*
// (`stack` 61 times); the other four are deliberately absent, and this gate is what stops them being
// added speculatively.
func TestLayoutVocabularyIsRenderedAndUsed(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "rendering", "layout.templ"))
	if err != nil {
		t.Fatalf("read layout.templ: %v", err)
	}
	layout := string(body)

	for kind := range domain.KnownLayoutKinds {
		if !strings.Contains(layout, string(kind)+"Layout") {
			t.Errorf("domain.KnownLayoutKinds declares %q and internal/rendering has no %sLayout to draw it -- a declarable kind nothing can render is a declaration that silently does nothing", kind, kind)
		}
	}
	for kind := range domain.KnownStaticKinds {
		if !strings.Contains(layout, "domain.Static"+strings.ToUpper(string(kind)[:1])+string(kind)[1:]) {
			t.Errorf("domain.KnownStaticKinds declares %q and staticText has no case for it", kind)
		}
	}

	// And the other direction, over the whole rendering package: a primitive with no caller at all.
	used, err := os.ReadDir(filepath.Join(repoRoot(), "internal", "rendering"))
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	var all strings.Builder
	for _, e := range used {
		if strings.HasSuffix(e.Name(), ".templ") && e.Name() != "layout.templ" {
			b, _ := os.ReadFile(filepath.Join(repoRoot(), "internal", "rendering", e.Name()))
			all.Write(b)
		}
	}
	callers := all.String() + layout
	if !strings.Contains(callers, "@pageHeader(") {
		t.Error("no screen calls pageHeader -- the layout primitives have no consumer, which is the shape-before-need 007 §34 forbids")
	}

	// **Every declared kind, not just the entry point.** This check was `@pageHeader` alone in the first
	// pass, and it passed while `rowLayout`, `gridLayout` and `panelLayout` sat with zero callers -- and
	// the unused `gridLayout` had put `sm:grid-cols-3` into the shipped Tailwind bundle, a class no screen
	// uses. The gate's own stated purpose was "a kind nothing calls is shape-before-need" and it was not
	// testing that. All three were removed; this is what stops them, or any successor, coming back
	// unconsumed.
	for kind := range domain.KnownLayoutKinds {
		if !strings.Contains(callers, "@"+string(kind)+"Layout(") {
			t.Errorf("domain.KnownLayoutKinds declares %q and nothing calls @%sLayout -- a primitive arrives with the uses it replaces, or it is a name with nothing behind it. Measured counts for the unbuilt kinds are in domain.LayoutKind's comment; migrate rather than re-declare", kind, kind)
		}
	}
	for g := range domain.KnownGaps {
		if !strings.Contains(callers, "domain.Gap"+strings.ToUpper(string(g)[:1])+string(g)[1:]) {
			t.Errorf("domain.KnownGaps declares %q and nothing names it -- an unused spacing step is one more class in the shipped bundle for nothing", g)
		}
	}

	// And the same both-ends rule for a primitive's own closed parameter set. GridCols is where the
	// vocabulary could grow most quietly: adding a member costs one line in domain and emits a utility class
	// the moment a renderer draws it, whether or not a screen ever asks for that count.
	//
	// Drawn and called are checked separately on purpose. A member no renderer draws is a declaration that
	// silently renders the default -- the footgun domain.GridCols names in its own comment. A member no
	// screen calls is shape-before-need, and `sm:grid-cols-3` is this repository's own worked example of the
	// cost: a column count nothing used, shipped in the bundle, kept alive for a day by the comment
	// recording that it should not be there (see TestNoClassLivesOnlyInAComment).
	for c := range domain.KnownGridCols {
		name := fmt.Sprintf("domain.GridCols%d", c)
		if !strings.Contains(layout, name) {
			t.Errorf("domain.KnownGridCols declares %d and no switch in layout.templ names %s -- a column count no renderer draws silently renders the default instead", c, name)
		}
		if !strings.Contains(callers, name) {
			t.Errorf("domain.KnownGridCols declares %d and no screen passes %s -- a column count arrives with the site that needs it, or it is a utility class in the bundle for nothing", c, name)
		}
	}
}

// TestLayoutClassesStayInRendering holds §15.2's own rule: an intermediate representation "should not embed
// HTML, CSS framework classes, SQL, or database-specific implementation".
//
// internal/domain carries the vocabulary; the Tailwind mapping belongs to internal/rendering, the only
// plane entitled to know Tailwind exists. The failure this prevents is subtle rather than loud: a `gap-3`
// in domain still renders correctly, and the plane boundary is gone with nothing to show it.
func TestLayoutClassesStayInRendering(t *testing.T) {
	for _, pkg := range []string{"domain", "metadata"} {
		dir := filepath.Join(repoRoot(), "internal", pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", pkg, err)
		}
		checked := 0
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("read %s/%s: %v", pkg, e.Name(), err)
			}
			checked++
			code := regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(string(b), "")
			// Unambiguously-Tailwind tokens only. A bare `"grid"` or `"flex"` is also a legitimate
			// *value* -- domain.LayoutGrid's own string is "grid", and icon.go has an icon named it --
			// and a gate that flagged those would be reporting the vocabulary it exists to protect.
			// Found immediately: the first version failed on both.
			if m := regexp.MustCompile(`"[^"]*\b(?:flex-col|grid-cols-\d|gap-\d|gap-x-\d|rounded-(?:lg|md|full)|items-center|justify-between|text-\w+-\d{3})\b[^"]*"`).FindString(code); m != "" {
				t.Errorf("internal/%s/%s contains the CSS class literal %s -- 007 §15.2 forbids a logical representation embedding framework classes; the mapping belongs in internal/rendering", pkg, e.Name(), m)
			}
		}
		if checked == 0 {
			t.Fatalf("internal/%s has no non-test Go files -- this gate would pass by measuring nothing", pkg)
		}
	}
}
