package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// declaredPlaceholders are packages that exist, carry a boundary rule, and contain nothing but their own
// doc.go -- deliberately, each because the concepts block building it rather than because nobody got to it.
//
// A placeholder is a legitimate thing: it reserves the seam's name and its import boundary before anything
// fills it, which is how `internal/registry`'s rule was already in place when Services moved in. What is
// not legitimate is a placeholder whose doc **claims to hold something**. `internal/ir/doc.go` said it held
// "Domain IR, Data IR, and UI IR" while holding nothing and being imported by nothing, which is the same
// class as 007 §40's "CEL expression evaluation -- PROVEN" in a runtime with no CEL.
//
// So the entry is the *reason it is empty*, and the gate below makes an empty package a recorded decision
// instead of an accident.
var declaredPlaceholders = map[string]string{
	"ir": "Domain IR is realised as internal/domain (005 Phase 5's definition is domain.Machine's own " +
		"content); Data IR waits on the planner, which 007 §34 marks PROPOSED and unadmitted; UI IR waits " +
		"on a second render target. See the package's own doc.go, which also records why the ir.Machine " +
		"refactor proposed in menata-app-document audits/2026-09-30 was measured and rejected",
	"planner": "the Composable Execution Planner is PROPOSED (007 §34), which says outright that naming " +
		"the boundary admits no capability; its own doc.go says to build it against real forcing cases",
}

// TestDeclaredPlaceholdersStayDeclared holds both directions, because each failure means something
// different and both are easy to leave behind.
//
// A package that becomes empty without an entry is one somebody gutted. An entry for a package that now has
// code is a stale excuse -- and worse, it is an excuse that would keep reading as a deferral after the
// deferral ended, which is the failure this repo spent 2026-09-30 retracting in four places.
func TestDeclaredPlaceholdersStayDeclared(t *testing.T) {
	checked := 0
	for _, parent := range []string{"internal", "cmd"} {
		entries, err := os.ReadDir(filepath.Join(repoRoot(), parent))
		if err != nil {
			t.Fatalf("read %s/: %v", parent, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(repoRoot(), parent, e.Name()))
			if err != nil {
				t.Fatalf("read %s/%s: %v", parent, e.Name(), err)
			}
			var goFiles []string
			for _, f := range files {
				if strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), ".templ") {
					goFiles = append(goFiles, f.Name())
				}
			}
			onlyDoc := len(goFiles) == 1 && goFiles[0] == "doc.go"
			_, declared := declaredPlaceholders[e.Name()]
			switch {
			case onlyDoc && !declared:
				t.Errorf("%s/%s contains only doc.go and is not in declaredPlaceholders -- an empty package should be a recorded decision with the reason it is empty, not something a reader has to guess about", parent, e.Name())
			case !onlyDoc && declared:
				t.Errorf("%s/%s has code now but declaredPlaceholders still excuses it as empty -- remove the entry, and check its doc.go still describes what the package does rather than why it does not exist yet", parent, e.Name())
			case onlyDoc && declared:
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no declared placeholder was found -- either both filled up (remove their entries) or this gate is looking in the wrong place")
	}
}

// TestPlaceholderDocsClaimNoContent is the half that caught the real defect: a placeholder may describe what
// *will* live there, but must not assert in the present tense that it already does.
//
// **It reads only the package summary -- the first paragraph, up to the first blank comment line -- and that
// scope is load-bearing rather than tidy.** Checking the whole body failed on `internal/ir/doc.go`'s own
// *retraction*, which quotes the sentence it is retracting. That is the fourth gate today whose text sat
// inside the data it read (the others: the pointer gate's example citations, the prompt gate's vacuous
// FieldTypeLabels arm, and this). A summary is also the right scope on its own terms: an overclaim lives in
// "Package X does Y", while the honest account of what blocks the package belongs in the body and must stay
// free to quote what was wrong before.
//
// Matched on phrasing rather than on a general rule, because "does this sentence overclaim" is a judgement
// no scan makes -- and a looser pattern would flag exactly the forward-looking prose these files should
// carry.
func TestPlaceholderDocsClaimNoContent(t *testing.T) {
	for pkg := range declaredPlaceholders {
		body, err := os.ReadFile(filepath.Join(repoRoot(), "internal", pkg, "doc.go"))
		if err != nil {
			t.Fatalf("read internal/%s/doc.go: %v", pkg, err)
		}
		summary := string(body)
		if i := strings.Index(summary, "\n//\n"); i >= 0 {
			summary = summary[:i]
		}
		for _, claim := range []string{"holds the normalized", "implements the"} {
			if strings.Contains(summary, claim) {
				t.Errorf("internal/%s/doc.go's summary asserts %q in the present tense while the package is empty -- say what blocks it and what would trigger it, the way internal/planner's own doc.go does", pkg, claim)
			}
		}
	}
}
