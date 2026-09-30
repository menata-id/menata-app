package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCitedRoadmapAnchorsResolve holds the mechanism the whole hardcoding-exception convention rests on.
//
// `CLAUDE.md`'s decision path makes a forward-checkable pointer **mandatory** for every literal metadata
// cannot express yet -- step 2(b): "cite a forward-checkable pointer -- a
// `007-composable-runtime-architecture.md` section number or a roadmap phase -- so a later reader can
// check whether that capability has since landed, rather than trusting the comment forever" -- and step 4
// makes re-checking those pointers part of closing a phase. A pointer that does not resolve turns that
// convention into decoration.
//
// **Measured 2026-09-30: 148 code comments cited a roadmap phase, and 141 named a phase the roadmap does
// not contain.** Every one resolved in `menata-app-document`'s `development-history.md` instead -- nothing
// was lost, the pointers simply named the wrong file. Phases moved there on 2026-09-19 when the repo
// split, which stranded about 83 of them that day; the roadmap migration of 2026-09-30 stranded the rest.
// Both times the pointers kept compiling, so nothing said so.
//
// The gate is cheap because the failure is cheap: an anchor is a string, and either the roadmap contains
// it or the comment should name the file that does.
//
// **This comment deliberately spells no citation the way the regex below matches**, and that is not
// fussiness: the first version quoted two real examples, which put this file into its own population.
// One mutation then failed on the gate's own prose, and a *second* mutation -- removing the last real
// citation in the tree -- could not reach the "measuring nothing" guard at all, because these examples
// were still counted. A gate whose own documentation is data it reads has a blind spot exactly where it
// explains itself.
//
// **What it cannot do, and four sites proved it the day it was written.** Resolution is *textual*, not
// semantic. A citation naming phase two resolved -- the roadmap does contain that string -- inside a
// board-06 deferral row with nothing to do with the authentication phase its three citing comments meant.
// A citation naming phase three resolved against a sentence about **005-runtime-lifecycle.md's** Phase 3
// (Parse), not a roadmap phase at all. Reading found those; this gate would have passed them forever. So
// a green run means every cited anchor exists, never that it means what the comment intends -- the same
// limit TestRoadmapStaysAReleasePlan states about vocabulary versus fit.
func TestCitedRoadmapAnchorsResolve(t *testing.T) {
	road, err := os.ReadFile(filepath.Join(repoRoot(), "ROADMAP.md"))
	if err != nil {
		t.Fatalf("read ROADMAP.md: %v", err)
	}

	cites := 0
	for _, path := range citableSourceFiles(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		rel, _ := filepath.Rel(repoRoot(), path)
		for _, m := range citedRoadmapAnchor.FindAllStringSubmatch(string(body), -1) {
			cites++
			anchor := m[2]
			// Word-boundary, because an early phase number is a prefix of a later one ("2" of "21") and a
			// substring match would resolve a citation against a phase months of work away from the one
			// it means. Mutation-proved by stripping the real occurrences and leaving the longer ones.
			if !regexp.MustCompile(regexp.QuoteMeta(anchor) + `\b`).Match(road) {
				t.Errorf("%s cites %q, which ROADMAP.md does not contain -- phases live in menata-app-document's development-history.md since the 2026-09-19 repo split, so write `development-history.md %s` instead. A pointer that does not resolve makes CLAUDE.md step 2(b) decoration",
					rel, m[0], anchor)
			}
		}
	}
	if cites == 0 {
		t.Fatal("found no ROADMAP.md phase citations at all -- either the convention was abandoned or this gate stopped looking; both are worth a failure")
	}
}

// citedRoadmapAnchor matches a citation that names a *specific* phase. A bare `ROADMAP.md` is not a
// finding: the file exists, and 112 comments legitimately point at it as a whole.
var citedRoadmapAnchor = regexp.MustCompile(`ROADMAP\.md('s)? ((?:Phase|Fase|Stage) [0-9A-Za-z]+)`)

// citableSourceFiles walks the hand-written Go and templ sources. Generated `*_templ.go` files are
// skipped because they carry copies of their `.templ`'s comments -- counting both would report every
// rendering finding twice and, worse, let a fix look incomplete until `make generate` ran.
func citableSourceFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(repoRoot(), dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			name := d.Name()
			if strings.HasSuffix(name, "_templ.go") {
				return nil
			}
			if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".templ") {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(out) == 0 {
		t.Fatal("walked no source files -- this gate would pass by measuring nothing")
	}
	return out
}
