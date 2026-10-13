package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// colourRoleClass is a class drawn through a colour role's custom property (DS-D15 stage 2, 2026-10-12):
// `text-ink-body`, `text-accent`, `hover:text-accent-hover`, and the same names on a fill or an edge. Bounded only
// by "not part of a longer name", so any variant prefix (`group-hover/card:`, `[&>a]:`, `aria-[current=page]:`), an
// opacity (`/80`) and an important mark (`!`, either end) are all still the class -- the first version stopped at
// whitespace and quotes, and the 2026-10-12 review passed all five of those shapes through it.
var colourRoleClass = regexp.MustCompile(`(?:^|[^a-z0-9-])((?:[^\s"'{}` + "`" + `]*:)?!?(?:text|bg|border|ring|outline|fill|stroke|decoration|divide)-(?:ink-[a-z]+|accent-hover|accent|bg-(?:raised|base|sunken)|border-(?:surface|control|divider)|hover-wash|tone-[a-z]+-(?:bg|ink|border)))(?:[^a-z0-9-]|$)`)

// colourRoleReaders are the only functions that may write a colour role's class. Each is in layout.templ.
var colourRoleReaders = []string{"inkClass", "accentHoverClass", "borderClass", "backgroundClass", "hoverWashClass",
	"toneFillClass", "toneInkClass", "toneEdgeClass", "toneHoverFillClass"}

// TestColourRoleClassesComeOnlyFromTheirReaders is a zero-floor gate beside DS-D15 stage 2. A colour role is a
// custom property now, so `text-ink-body` written by hand would still follow the Workspace's theme -- the danger is
// not a stuck colour but a site that no longer goes through inkClass, which is where the next change to a role (a
// dark mode, a second property) will be made. The rule input.css states ("read through rendering.inkClass, never
// written by hand") is held here: a role class may appear only inside its reader's body.
//
// It reads every hand-written `.templ` and `.go` file under internal/rendering, tests and generated files excluded,
// comments stripped, so a class in a Go string is seen as well as one in a class attribute. Blind spot: a class
// assembled at runtime ("text-ink-" + role), which Tailwind would not emit either.
func TestColourRoleClassesComeOnlyFromTheirReaders(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string][]string{}
	readersSeen := 0
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, "_test.go") || strings.HasSuffix(n, "_templ.go") || !(strings.HasSuffix(n, ".templ") || strings.HasSuffix(n, ".go")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		src := stripTemplComments(string(b))
		if n == "layout.templ" {
			for _, fn := range colourRoleReaders {
				var cut bool
				src, cut = withoutFuncBody(src, fn)
				if cut {
					readersSeen++
				}
			}
		}
		for _, m := range colourRoleClass.FindAllStringSubmatch(src, -1) {
			found[n] = append(found[n], m[1])
		}
	}
	if readersSeen != len(colourRoleReaders) {
		t.Fatalf("found %d of the %d colour-role readers in layout.templ -- a reader was renamed or moved; update colourRoleReaders", readersSeen, len(colourRoleReaders))
	}
	files := make([]string, 0, len(found))
	for f := range found {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		t.Errorf("%s writes the colour-role class(es) %v by hand -- read the role through inkClass/accentHoverClass, so a change to how a role reaches CSS is made in one place", f, found[f])
	}
}

// withoutFuncBody removes `func name(...) ... {` through its closing `}` at column 0, reporting whether it found it.
func withoutFuncBody(src, name string) (string, bool) {
	start := strings.Index(src, "func "+name+"(")
	if start < 0 {
		return src, false
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		return src, false
	}
	return src[:start] + src[start+end+3:], true
}
