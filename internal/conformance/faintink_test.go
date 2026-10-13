package conformance

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// faintInkSites is every place `internal/rendering` draws text in the `faint` ink role, per file, with the reason the
// text may be missed without loss. The Design System's own definition of `ink-faint` is "Hints, placeholders,
// decoration only" at about 2.6:1 on white, which "FAILS WCAG AA for real content" (rencana 1.2b, 2026-10-13).
//
// Before this list, 98 sites drew faint text and most were content: emails, roles, SLA lines, document references,
// timestamps, member counts, empty-state messages, field labels, section labels, and nine controls (icon buttons and
// a text "Discard" button, which need 3:1 and 4.5:1). Those moved to `secondary`. What is left is below.
//
// Whether a piece of text may be missed is a judgement no pattern can make, so the gate does not try: it holds the
// count per file to this map, both ways, and a new faint site fails until its reason is written here.
var faintInkSites = map[string]struct {
	n      int
	reason string
}{
	"account.templ":          {1, "the disabled Enable button: a disabled control is exempt from WCAG 1.4.3"},
	"approvalstepper.templ":  {1, "the number of a step not reached yet, beside its readable name"},
	"appsettings.templ":      {4, "a setting that is not built yet (its label, description, the Not built yet tag, the Notifications row): a disabled row"},
	"chooseworkspace.templ":  {1, "the chevron on a Workspace row: decoration"},
	"detail.templ":           {2, "a record's technical id, shown for support; the document reference people cite reads secondary"},
	"inference.templ":        {4, "the inference inspector's not-applicable rows and the em dash for no value: de-emphasised on purpose (muted's own reason)"},
	"layout.templ":           {2, "a Collection's ordinal, aria-hidden because the <ol> announces position"},
	"machine.templ":          {3, "a Metric's optional hint, and a Machine's technical id"},
	"workspacemembers.templ": {2, "the em dash for no value"},
}

// faintInkReaderFuncs are the functions in layout.templ whose bodies name the role as the reader, not as a site.
var faintInkReaderFuncs = []string{"inkClass", "inkShadeVar"}

// TestFaintInkSitesHaveAReason holds every `domain.InkFaint` read in internal/rendering to faintInkSites.
// Blind spot: a literal faint colour (`text-slate-400`) is not this gate's; TestHandWrittenThemeTokensOnlyShrink
// counts those, and its two remaining floors are Go helpers with no ctx (`avatarClasses`, `diffLineClass`).
func TestFaintInkSitesHaveAReason(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
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
			for _, fn := range faintInkReaderFuncs {
				var cut bool
				if src, cut = withoutFuncBody(src, fn); !cut {
					t.Fatalf("reader %s not found in layout.templ -- update faintInkReaderFuncs", fn)
				}
			}
		}
		if c := strings.Count(src, "domain.InkFaint"); c > 0 {
			got[n] = c
		}
	}
	files := map[string]bool{}
	for f := range got {
		files[f] = true
	}
	for f := range faintInkSites {
		files[f] = true
	}
	names := make([]string, 0, len(files))
	for f := range files {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		want := faintInkSites[f].n
		switch {
		case got[f] > want:
			t.Errorf("%s draws %d faint text site(s), %d with a reason -- faint is for hints, placeholders and decoration (about 2.6:1, below WCAG AA). Read the text: if it must be read, use domain.InkSecondary; if it may be missed, say why in faintInkSites", f, got[f], want)
		case got[f] < want:
			t.Errorf("%s draws %d faint text site(s), down from %d -- lower (or remove) its entry in faintInkSites", f, got[f], want)
		}
	}
}
