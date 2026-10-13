package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// G2 of the design-system plan (rencana 3.5, 2026-10-13): **a class a screen writes must exist in the stylesheet, and
// the palette is closed.**
//
// Tailwind turns whatever it can parse into CSS and silently drops the rest, so a class it cannot compile does not
// fail the build -- the element simply draws without it. That happened in 3.2d: a rewrite joined two readers without
// a space (`border-ink-strongbg-ink-strong`), a filter chip and two empty states lost their fill, and every test stayed
// green. A Chromium diff found it. Closing the palette (`--color-*: initial` in input.css's `@theme`) makes the same
// failure the normal outcome of writing a colour outside it, so the two halves ship together: without this gate the
// closed palette would turn `text-cyan-600` into an invisible bug instead of a refused one.

// tailwindHues is Tailwind v4's default palette, the names a palette utility or property can carry.
const tailwindHues = `slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose`

// classShape is a Tailwind utility as written: variants, `!`, a negative sign, a root, its dash-separated value
// (words, numbers, an arbitrary `[...]` or `(...)`), and an opacity. A variant may be a word, an arbitrary `[...]`,
// `*`/`**` (children, descendants) or a container query (`@sm`, `@container`). Group 1 is the root, group 2 the value. Anything
// that does not fit -- a time format, a sentence, `%` in a Sprintf -- is not a class and is not checked.
var classShape = regexp.MustCompile(`^((?:(?:[a-z][a-z0-9-]*(?:-\[[^\]\s]+\])?|\[[^\]\s]+\]|\*{1,2}|@[a-z0-9]*)(?:/\w+)?:)*)!?-?([a-z][a-z0-9]*)((?:-[a-z0-9.]+|-\[[^\]\s]+\]|-\([^)\s]+\))*)(?:/(?:\d+|\[[^\]\s]+\]))?!?$`)

// cssClassName is one class in a selector, escapes included.
var cssClassName = regexp.MustCompile(`\.((?:\\.|[A-Za-z0-9_-])+)`)

// compiledClasses returns every class name app.css has a selector for, unescaped, and the roots Tailwind's utilities
// use (`text`, `bg`, `gap`, ...) -- the custom classes input.css writes itself are left out of the roots, so an id
// sharing their prefix is not mistaken for a utility.
func compiledClasses(t *testing.T) (emitted map[string]bool, roots map[string]bool, bare map[string]bool) {
	t.Helper()
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(repoRoot(), "static", "css", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	unescape := regexp.MustCompile(`\\(.)`)
	custom := map[string]bool{}
	for _, m := range cssClassName.FindAllStringSubmatch(read("input.css"), -1) {
		custom[m[1]] = true
	}
	emitted, roots, bare = map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, m := range cssClassName.FindAllStringSubmatch(read("app.css"), -1) {
		name := unescape.ReplaceAllString(m[1], "$1")
		emitted[name] = true
		if custom[name] {
			continue
		}
		if s := classShape.FindStringSubmatch(name); s != nil {
			if s[3] != "" {
				roots[s[2]] = true
			} else {
				bare[s[2]] = true
			}
		}
	}
	return emitted, roots, bare
}

// arbitraryProperty is Tailwind's `[property:value]` utility, with or without variants (`before:[content:…]`).
var arbitraryProperty = regexp.MustCompile(`^(?:(?:[a-z][a-z0-9-]*|\[[^\]\s]+\]|\*{1,2}|@[a-z0-9]*):)*\[[a-z-]+:[^\]\s]+\]$`)

// isUtilityShaped reports whether a token is meant as a Tailwind utility: a known root with a value
// (`text-cyan-600`), a variant over a utility some compiled class uses bare (`hover:underline`), or an arbitrary
// property (`[counter-reset:approver-step]`). A bare word (`hidden`, `text`) and a
// root nothing compiles (`menata-leave-guard`, an id) are not claims this gate can judge, unless the token names a
// palette colour.
func isUtilityShaped(tok string, roots, bare map[string]bool) bool {
	if arbitraryProperty.MatchString(tok) {
		return true
	}
	s := classShape.FindStringSubmatch(tok)
	if s == nil {
		return false
	}
	if s[3] != "" {
		// A palette colour counts even in a utility family nothing compiles yet (`fill-`, `stroke-`, `from-`, `caret-`):
		// the 3.5 review wrote five such classes into a .go file, which Tailwind never scans, and they passed.
		return roots[s[2]] || paletteColourToken.MatchString(" "+tok+" ")
	}
	return s[1] != "" && bare[s[2]]
}

// classTokensBySource reads every hand-written source in internal/rendering -- `.templ` files and the plain `.go`
// files beside them, which Tailwind does not scan at all -- and returns each class-context token with the files that
// write it. A class attribute's value is read whole; every other attribute (`id`, `href`, `type`, ...) is removed;
// what remains quoted is a Go or script string, which is where readers, `templ.KV` keys and helpers keep classes.
// designTokenNameFiles are the files whose Go map keys are the Design System artifact's token names (`gap-tight`,
// `bg-raised`, `accent-hover`), which look like utilities and are not classes. Only the keys are skipped; every class
// in such a file is a value, read from a reader or written as a literal, and is still judged. Closed: a file listed
// here that no longer exists fails TestDesignTokenNameFilesExist.
var designTokenNameFiles = map[string]string{
	"designtokens.go": "rendering.designTokenClasses maps each Design System token name to its reader's class (rencana 3.4)",
}

var goMapKey = regexp.MustCompile(`"[^"\n]*"\s*:`)

func TestDesignTokenNameFilesExist(t *testing.T) {
	for f := range designTokenNameFiles {
		if _, err := os.Stat(filepath.Join(repoRoot(), "internal", "rendering", f)); err != nil {
			t.Errorf("designTokenNameFiles names %s, which is gone -- remove the entry", f)
		}
	}
}

func classTokensBySource(t *testing.T) map[string][]string {
	t.Helper()
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	classAttr := regexp.MustCompile(`\bclass="([^"]*)"`)
	otherAttr := regexp.MustCompile(`\b[\w:@.-]+="[^"]*"`)
	// A script attribute adds classes too: Hyperscript's `_="add .x"` and an `onclick="…toggle('x')"`. The 3.5 review
	// changed authshell's toggle('hidden') to a class nothing compiles and every gate stayed green, because the
	// attribute was removed with `id` and `href`.
	scriptAttr := regexp.MustCompile(`\b(?:_|on[a-z]+)="([^"]*)"`)
	hyperscriptClass := regexp.MustCompile(`\b(?:add|remove|toggle)\s+\.([^\s<>/'"]+)`)
	quoted := regexp.MustCompile("\"((?:\\\\.|[^\"\\\\\\n])*)\"|`([^`]*)`|'((?:\\\\.|[^'\\\\\\n])*)'")
	out := map[string][]string{}
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, "_test.go") || strings.HasSuffix(n, "_templ.go") || !(strings.HasSuffix(n, ".templ") || strings.HasSuffix(n, ".go")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		src := stripBlockComments(stripTemplComments(string(b)))
		if _, names := designTokenNameFiles[n]; names {
			src = goMapKey.ReplaceAllString(src, "")
		}
		var toks []string
		for _, m := range scriptAttr.FindAllStringSubmatch(src, -1) {
			for _, h := range hyperscriptClass.FindAllStringSubmatch(m[1], -1) {
				toks = append(toks, h[1])
			}
			for _, q := range quoted.FindAllStringSubmatch(m[1], -1) {
				toks = append(toks, strings.Fields(q[1]+q[2]+q[3])...)
			}
		}
		for _, m := range classAttr.FindAllStringSubmatch(src, -1) {
			toks = append(toks, strings.Fields(m[1])...)
		}
		for _, m := range quoted.FindAllStringSubmatch(otherAttr.ReplaceAllString(src, ""), -1) {
			toks = append(toks, strings.Fields(m[1]+m[2]+m[3])...)
		}
		for _, tok := range toks {
			out[tok] = append(out[tok], n)
		}
	}
	return out
}

// stripBlockComments removes `/* … */` comments (Go and CSS). It runs after stripTemplComments, and an opener must
// follow a space, a line start or punctuation, so `accept="image/*"` does not open one. The 3.5 review found a step
// "named" only by a block comment, which kept a declared, unused step alive.
func stripBlockComments(src string) string {
	return blockComment.ReplaceAllString(src, "$1")
}

var blockComment = regexp.MustCompile(`(?s)(^|[\s;{}(,])/\*.*?\*/`)

// TestEveryClassTokenIsCompiled is G2's first half: every utility-shaped token in a class attribute or a string in
// internal/rendering must have a selector in the committed app.css. A miss names the token and the files writing it.
//
// What it cannot see, stated rather than implied: a class assembled at runtime (`"text-" + size` leaves only `text-`,
// which is skipped) and a bare word nothing compiles. The rendered half, `web.TestEveryRenderedClassIsCompiled`, sees
// what a page actually draws, including classes assembled in Go. It reads the *committed* app.css, so a stale bundle is
// `make check-generated`'s to catch, as for TestNoClassLivesOnlyInAComment.
func TestEveryClassTokenIsCompiled(t *testing.T) {
	emitted, roots, bare := compiledClasses(t)
	for _, want := range []string{"text", "bg", "border", "gap", "rounded"} {
		if !roots[want] {
			t.Fatalf("app.css yields no %q utility root -- the selector parse has stopped matching, so this gate checks nothing", want)
		}
	}
	tokens := classTokensBySource(t)
	checked := 0
	var missing []string
	for tok, files := range tokens {
		if !isUtilityShaped(tok, roots, bare) {
			continue
		}
		checked++
		if emitted[tok] {
			continue
		}
		missing = append(missing, tok+" in "+strings.Join(uniqueSorted(files), ", "))
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("%s -- app.css has no such class, so the element draws without it. A colour outside the closed palette "+
			"(input.css's @theme), a typo, or two classes run together; if it is meant, run `make css` and commit app.css", m)
	}
	if checked < 400 {
		t.Fatalf("only %d utility-shaped tokens were checked (about 540 distinct ones on 2026-10-13) -- the scan is measuring far less than it appears to", checked)
	}
	t.Logf("checked %d utility-shaped tokens against %d compiled classes", checked, len(emitted))
}

func uniqueSorted(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// TestClosedPaletteDeclaresExactlyWhatIsNamed is G2's second half. input.css's `@theme` must reset the palette
// (`--color-*: initial`) and declare exactly the palette steps something names:
//
//   - a declared step nothing names is a colour the product cannot draw but a class could reach -- remove it;
//   - a step named but not declared is a colour outside the palette: a utility for it compiles to nothing (the gate
//     above), and a `var(--color-…)` a reader writes into a Workspace's override resolves to nothing at all, which no
//     class check can see.
//
// "Named" means: a `--color-<hue>-<step>` or palette utility in a hand-written internal/rendering source (comments
// stripped), or a `var(--color-…)` in input.css outside the palette block. Which of those names *should* exist is held
// elsewhere -- the role readers by GS1/GS3 and the dark-mode gates, the literals by TestHandWrittenColoursHaveAReason.
func TestClosedPaletteDeclaresExactlyWhatIsNamed(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(), "static", "css", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	input := stripBlockComments(string(b))
	if !regexp.MustCompile(`(?m)^\s*--color-\*:\s*initial;`).MatchString(input) {
		t.Fatal("input.css's @theme does not reset the palette with `--color-*: initial;` -- Tailwind's full palette is open again, so a colour outside the system compiles")
	}
	decl := regexp.MustCompile(`(?m)^\s*--color-((?:` + tailwindHues + `)-\d+|white|black):\s*[^v][^;]*;`)
	declared := map[string]bool{}
	for _, m := range decl.FindAllStringSubmatch(input, -1) {
		declared[m[1]] = true
	}
	if len(declared) < 20 {
		t.Fatalf("only %d palette steps declared in input.css -- the declaration pattern has stopped matching", len(declared))
	}

	named := map[string][]string{}
	property := regexp.MustCompile(`--color-((?:` + tailwindHues + `)-\d+|white|black)\b`)
	utility := regexp.MustCompile(`-((?:` + tailwindHues + `)-\d+|white|black)(?:/\d+)?$`)
	add := func(step, where string) { named[step] = append(named[step], where) }
	for _, m := range property.FindAllStringSubmatch(decl.ReplaceAllString(input, ""), -1) {
		add(m[1], "input.css")
	}
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, "_test.go") || strings.HasSuffix(n, "_templ.go") || !(strings.HasSuffix(n, ".templ") || strings.HasSuffix(n, ".go")) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		text := stripBlockComments(stripTemplComments(string(src)))
		for _, m := range property.FindAllStringSubmatch(text, -1) {
			add(m[1], n)
		}
		for _, tok := range regexp.MustCompile("[\\s\"'`{}(),]+").Split(text, -1) {
			if !paletteColourToken.MatchString(" " + tok + " ") {
				continue
			}
			if m := utility.FindStringSubmatch(tok); m != nil {
				add(m[1], n)
			}
		}
	}

	for step := range declared {
		if len(named[step]) == 0 {
			t.Errorf("input.css declares --color-%s, which nothing names -- remove it, so no class can reach a colour the system does not use", step)
		}
	}
	for step, where := range named {
		if !declared[step] {
			t.Errorf("--color-%s is named in %s but is not in the closed palette -- draw it through a role, or declare the step in input.css's @theme with the reader or reason that needs it",
				step, strings.Join(uniqueSorted(where), ", "))
		}
	}
	t.Logf("closed palette: %d steps declared, %d named", len(declared), len(named))
}
