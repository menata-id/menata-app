package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// faintInkSites is every place `internal/rendering` draws text in a faint colour, keyed by `file:function`, with the
// reason the text may be missed without loss. Faint is `inkClass(ctx, domain.InkFaint)` and the `muted` tone, which
// draws the same slate-400 (domain.DefaultTheme). The Design System's own definition of `ink-faint` is "Hints,
// placeholders, decoration only" at about 2.6:1 on white, which "FAILS WCAG AA for real content" (rencana 1.2b,
// 2026-10-13).
//
// Before this list, 98 sites drew faint text and most were content: emails, roles, SLA lines, document references,
// timestamps, member counts, empty-state messages, field labels, section labels, a Metric's hint (which a page may
// bind to a computed figure), a not-built setting's explanation, and nine controls (icon buttons and a text "Discard"
// button, which need 3:1 and 4.5:1). Those read `secondary` now. What is left is below.
//
// Whether a piece of text may be missed is a judgement no pattern can make, so the gate holds counts to this map, both
// ways. It is keyed by function, not file, so moving faint ink from one site to another in the same file fails too
// (the 2026-10-13 review swapped a document reference and an id in one file, and a per-file count passed).
var faintInkSites = map[string]struct {
	n      int
	reason string
}{
	"account.templ:SecurityPage":                   {1, "the disabled Enable button: a disabled control is exempt from WCAG 1.4.3"},
	"appsettings.templ:settingsListRow":            {1, "a row's chevron: decoration (it was slate-300, below the ladder; folded to faint by owner decision CP6)"},
	"appsettings.templ:settingsRow":                {1, "a row's chevron: decoration"},
	"appshell.templ:moreSheet":                     {3, "the More sheet rows' chevrons: decoration"},
	"workspacehome.templ:WorkspaceHomePage":        {1, "an Application card's chevron: decoration"},
	"workspacehome.templ:HomeDraftApplicationRows": {1, "a draft review row's chevron: decoration"},
	"chooseworkspace.templ:ChooseWorkspacePage":    {1, "the chevron on a Workspace row: decoration"},
	"detail.templ:RecordDetailView":                {1, "a record's technical id, shown for support; the document reference people cite reads secondary"},
	"detail.templ:RecordDetailEdit":                {1, "the same technical id in the aside"},
	"inference.templ:inferenceRoleBlock":           {3, "the developer inspector's not-applicable rows, its em dash for no value, and the `uncast` badge in the muted tone: de-emphasised on purpose (muted's own reason in TestThemeColourPairsMeetContrast)"},
	"inference.templ:inferenceStepBlock":           {2, "the same inspector's not-applicable rows and em dash, for a step"},
	"layout.templ:collectionBody":                  {2, "a Collection's ordinal, aria-hidden because the <ol> announces position"},
	"machine.templ:avatarClasses":                  {1, "a pending member's initials in a dashed ring; the avatar's name is its aria-label"},
	"machine.templ:MachineList":                    {1, "a Machine's technical id"},
	"machine.templ:machineScreen":                  {1, "a Machine's technical id"},
	"workspacefiles.templ:diffLineClass":           {1, "a diff's hunk header (`@@ -1 +1 @@`): position metadata for the reader of a diff"},
	"workspacemembers.templ:WorkspaceMembersPage":  {2, "the em dash for no value"},
}

// faintInkReaderFuncs are the functions in layout.templ whose bodies name a faint role as a reader, not as a site.
var faintInkReaderFuncs = []string{"inkClass", "toneFillClass", "toneInkClass", "toneEdgeClass", "toneHoverFillClass"}

var (
	faintSite      = regexp.MustCompile(`inkClass\(ctx, domain\.InkFaint\)|domain\.ToneMuted`)
	inkClassCall   = regexp.MustCompile(`inkClass\(([^,()]+), ([^()]*?)\)`)
	inkConstant    = regexp.MustCompile(`^domain\.Ink[A-Z][a-z]+$`)
	enclosingDecl  = regexp.MustCompile(`(?m)^(?:templ|func)(?: \([^)]*\))? (\w+)\(`)
	inkConversions = regexp.MustCompile(`domain\.InkRole\(`)
)

// TestFaintInkSitesHaveAReason holds every faint text read in internal/rendering to faintInkSites, and closes the roads
// around it the 2026-10-13 review drove through a token count: an `inkClass` role that is not a `domain.Ink…` constant
// (a variable, an alias in another file, `domain.InkRole("faint")`), and `domain.InkFaint` anywhere but as that
// argument. Blind spots: a tone that arrives as data (composition's `toneFor`, or a `page:` prop that layout.templ
// converts with `domain.BadgeTone(...)`) may be `muted`; that is a producer, held by TestEveryBadgeToneHasAProducer, not
// a site. And `toneInkClass(ctx, t)` with a variable `t` is not counted.
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
		for _, m := range inkClassCall.FindAllStringSubmatch(src, -1) {
			if !inkConstant.MatchString(strings.TrimSpace(m[2])) {
				t.Errorf("%s: %s -- an ink role must be a domain.Ink… constant at the call, so a reader of this file can see which role it draws", n, m[0])
			}
		}
		if inkConversions.MatchString(src) {
			t.Errorf("%s converts a string to an ink role -- name the constant, so a faint read cannot hide behind a value", n)
		}
		if extra := strings.Count(src, "domain.InkFaint") - strings.Count(src, "inkClass(ctx, domain.InkFaint)"); extra > 0 {
			t.Errorf("%s names domain.InkFaint %d time(s) outside inkClass(ctx, domain.InkFaint) -- an alias moves faint ink out of this gate's sight", n, extra)
		}
		decls := enclosingDecl.FindAllStringSubmatchIndex(src, -1)
		for _, loc := range faintSite.FindAllStringIndex(src, -1) {
			fn := "(file scope)"
			for _, d := range decls {
				if d[0] < loc[0] {
					fn = src[d[2]:d[3]]
				}
			}
			got[n+":"+fn]++
		}
	}
	keys := map[string]bool{}
	for k := range got {
		keys[k] = true
	}
	for k := range faintInkSites {
		keys[k] = true
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		want := faintInkSites[k].n
		switch {
		case got[k] > want:
			t.Errorf("%s draws %d faint text site(s), %d with a reason -- faint is for hints, placeholders and decoration (about 2.6:1, below WCAG AA). Read the text: if it must be read, use domain.InkSecondary; if it may be missed, say why in faintInkSites", k, got[k], want)
		case got[k] < want:
			t.Errorf("%s draws %d faint text site(s), down from %d -- lower (or remove) its entry in faintInkSites", k, got[k], want)
		}
	}
}
