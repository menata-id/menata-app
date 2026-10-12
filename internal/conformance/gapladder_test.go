package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// handWrittenLadderGaps freezes, per file, the literal `gap-2`/`gap-3`/`gap-4`/`gap-5` left in `internal/rendering`
// (2026-10-12, rencana 0.2a). Those four are the Theme's gap ladder (`GapTight`..`GapLoose`). Unlike `xtight` and
// `region`, which the Design System scoped to one job each, the ladder carries no meaning beyond its size
// (`domain.Gap`), so every authenticated site reads `layoutGap(ctx, domain.Gap*)`.
//
// **137 sites moved, the audit's number exactly.** Two checks were byte-identical:
// - 106 authenticated pages through the router;
// - 17 component renders for the sites the fixture does not reach.
//
// What stays is the pre-auth screens. They render with no Workspace on ctx, so no `theme.gap` reaches them, and the
// field, radius and border ratchets keep their sites as floors for the same reason.
//
// Counted from string literals only (`joinClassLiterals`), unprefixed. `gap-x-*`/`gap-y-*` and `sm:gap-*` are not the
// ladder. `gap-1.5` and `gap-6` have their own ratchets, and `gap-1`/`gap-0.5` wait on an owner decision.
var handWrittenLadderGaps = map[string]struct {
	n   int
	why string
}{
	"authshell.templ":       {2, "pre-auth: no Workspace on ctx, no theme reaches it"},
	"checkyouremail.templ":  {1, "pre-auth: no Workspace on ctx, no theme reaches it"},
	"chooseworkspace.templ": {7, "pre-auth: no Workspace on ctx, no theme reaches it"},
	"login.templ":           {2, "pre-auth: no Workspace on ctx, no theme reaches it"},
}

var ladderGapToken = regexp.MustCompile(`^gap-[2-5]$`)

func TestHandWrittenLadderGapsOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	actual := map[string]int{}
	readers := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := stripTemplComments(string(b))
		if e.Name() == "layout.templ" {
			readers = len(regexp.MustCompile(`return "gap-[2-5]"`).FindAllString(src, -1))
			continue
		}
		for _, m := range classAttr.FindAllStringSubmatch(joinClassLiterals(src), -1) {
			for _, tok := range strings.Fields(m[1]) {
				if ladderGapToken.MatchString(tok) {
					actual[e.Name()]++
				}
			}
		}
	}
	if readers < 4 {
		t.Fatalf("expected layoutGap's four ladder arms in layout.templ, found %d -- the scan is reading the wrong text", readers)
	}
	seen := map[string]bool{}
	for f := range actual {
		seen[f] = true
	}
	for f := range handWrittenLadderGaps {
		seen[f] = true
	}
	var files []string
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		got, want := actual[f], handWrittenLadderGaps[f].n
		switch {
		case got > want:
			t.Errorf("%s: %d literal gap-2..5, frozen at %d -- read layoutGap(ctx, domain.GapTight|GapDefault|GapComfortable|GapLoose), "+
				"so a Workspace's theme.gap moves it; only a pre-auth screen, which no theme reaches, keeps a literal", f, got, want)
		case got < want:
			t.Errorf("%s: %d literal gap-2..5, down from %d -- lower handWrittenLadderGaps to lock it in", f, got, want)
		}
	}
}
