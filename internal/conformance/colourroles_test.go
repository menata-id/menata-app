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
// whitespace and quotes, and the 2026-10-12 review passed all five of those shapes through it. The utility family is
// any word, not a list: Tailwind v4 makes a colour utility of every family from a `--color-*` property
// (`border-t-`, `caret-`, `shadow-`, `from-`, `ring-offset-`), and the 2026-10-13 review passed seven of those through a
// listed set.
var colourRoleClass = regexp.MustCompile(`(?:^|[^a-z0-9-])((?:[^\s"'{}` + "`" + `]*:)?!?[a-z]+(?:-[a-z]+)*?-(?:ink-[a-z]+|accent-hover|accent|bg-(?:raised|base|sunken)|border-(?:surface|control|divider)|hover-wash|tone-[a-z]+-(?:bg|ink|border)))(?:[^a-z0-9-]|$)`)

// colourRoleReaders are the only functions that may write a colour role's class. Each is in layout.templ.
var colourRoleReaders = []string{"inkClass", "accentHoverClass", "borderClass", "backgroundClass", "hoverWashClass",
	"toneFillClass", "toneInkClass", "toneEdgeClass", "toneHoverFillClass",
	// 3.2d's reader forms: an existing role in another utility.
	"inkFillClass", "inkEdgeClass", "checkedFillClass", "onFillClass", "borderFillClass", "inkHoverClass", "sunkenHoverClass",
	"controlAccentClass", "accentActiveClass", "baseTintClass",
	// 3.2d slice B's forms.
	"toneInkHoverClass", "toneInkFillClass", "inkFillHoverClass", "accentEdgeClass", "accentEdgeHoverClass", "accentRingClass", "inkClassFromSm",
	// 3.2d review's forms.
	"surfaceFromSm", "openSurfaceClasses", "raisedTintClass", "unreadTintClass"}

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
		t.Errorf("%s writes the colour-role class(es) %v by hand -- read the role through its reader (%v), so a change to how a role reaches CSS is made in one place", f, found[f], colourRoleReaders)
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

// paletteColourToken is a Tailwind palette colour utility (any family, any variant prefix, an opacity, `!`):
// `bg-red-600`, `hover:border-blue-300`, `focus:outline-blue-300`, `bg-white/70`.
var paletteColourToken = regexp.MustCompile(`(?:^|[^\w-])((?:[\w\[\]&=/.-]+:)*!?(?:text|bg|border|divide|ring|outline|accent|fill|stroke|from|to|via|shadow|decoration|caret|placeholder)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|white|black)(?:-\d+)?(?:/\d+)?)(?:[^\w-]|$)`)

// handWrittenColours is every palette colour still written in `internal/rendering`, per file, with the reason it is
// not a role (rencana 3.2d, closed 2026-10-13). Everything else draws through a colour role, so a later dark mode moves
// it. Owner decisions CP4 (focus ring and scrim are right in both modes) and CP5 (the PDF paper layer stays light; the
// categorical palettes wait for dark mode) are the reasons below.
var handWrittenColours = map[string]struct {
	n   int
	why string
}{
	"account.templ":            {2, "the focus ring (CP4): a field's and the notification toggle's"},
	"attachments.templ":        {1, "the focus ring (CP4) of the attach button"},
	"authshell.templ":          {4, "the focus ring (CP4) of three pre-auth controls, and the brand glyph's white on bg-brand"},
	"controls.templ":           {1, "the focus ring (CP4) in fieldClasses"},
	"login.templ":              {2, "the focus ring (CP4) of the two sign-in fields"},
	"leaveguard.templ":         {1, "the dialog scrim (CP4)"},
	"appshell.templ":           {4, "the htmx error toast, a class string a script writes (3), and the More sheet's scrim (CP4)"},
	"layout.templ":             {3, "markGoodFill/Border/HoverBorder: the Design System's mark-good, a plain reader"},
	"machine.templ":            {31, "the Tag palette (CP5: an author's colour, 7 hues x 4) and the board's drag-and-drop marks in its script (markGoodScriptUses)"},
	"reviewdocument.templ":     {25, "the PDF paper layer (CP5): signature markers, their legend and the signature canvas drawn over a white page image; and the PDF file-type tile"},
	"signatureplacement.templ": {8, "the PDF paper layer (CP5): placement markers drawn over the page image"},
	"workspacehome.templ":      {15, "the Application icon palette (CP5) and the draft-review tile's indigo"},
}

// TestHandWrittenColoursHaveAReason is the closing gate of rencana 3.2d: every palette colour left in
// `internal/rendering` -- in a class attribute, a templ expression, a Go string or a script string -- is counted per
// file and must be in handWrittenColours with its reason. A new one fails ("draw it through a role"), and so does a
// lower count until the entry drops. It exists because the review found slice B's folds could revert inside a Go
// string argument (the nav count badge) with the design bucket, which reads class attributes, blind to it.
//
// It does not check that a role is the *right* one, and comments are stripped (`//` lines only).
func TestHandWrittenColoursHaveAReason(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	comment := regexp.MustCompile(`(?m)^\s*//.*$`)
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
		if c := countPaletteColours(comment.ReplaceAllString(string(b), "")); c > 0 {
			got[n] = c
		}
	}
	files := map[string]bool{}
	for f := range got {
		files[f] = true
	}
	for f := range handWrittenColours {
		files[f] = true
	}
	names := make([]string, 0, len(files))
	for f := range files {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		want := handWrittenColours[f].n
		switch {
		case got[f] > want:
			t.Errorf("%s writes %d palette colour(s), %d with a reason -- draw it through a colour role (inkClass, backgroundClass, borderClass, the tone readers or a reader form in layout.templ), so a theme and a dark mode move it", f, got[f], want)
		case got[f] < want:
			t.Errorf("%s writes %d palette colour(s), down from %d -- lower (or remove) its entry in handWrittenColours", f, got[f], want)
		}
	}
}

// countPaletteColours counts paletteColourToken matches, letting two tokens share the delimiter between them: RE2 has
// no lookahead, so FindAll consumed the space after one token and missed an adjacent one (`bg-red-50 text-red-700`
// counted as one).
func countPaletteColours(src string) int {
	n := 0
	for i := 0; i < len(src); {
		loc := paletteColourToken.FindStringSubmatchIndex(src[i:])
		if loc == nil {
			break
		}
		n++
		i += loc[3] // resume at the end of the token itself, before its trailing delimiter
	}
	return n
}
