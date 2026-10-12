package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestNoTwoKVKeysFromOneThemeReader is a zero-floor gate (2026-10-12): in one class expression, a `templ.KV` key
// may not be able to equal another class part under **any** Theme.
//
// **Why.** templ keys its class map on the whole string, and a later `false` disables an earlier `true`. A Theme can
// map two roles to one step (`good` and `info` to one palette, `strong` and `body` to one shade), so two parts that
// differ under the default theme become one string, and the true state loses its class. A byte-diff under the default
// theme cannot see this. Two shipped sites did it, and both now choose the class in Go:
// - `approvalinbox`: a done chip drew with no colour;
// - `appsettings`: a current settings link drew with no ink.
//
// **How, precisely.** Each part's possible outputs are computed from the readers' own literal `return`s (every
// arm, not only the default), concatenated through `+`. A pair is a finding when at least one is a KV and their sets
// intersect. Two plain parts are not a finding, since both are always on. This replaced a first version (same reader
// name twice) that the 2026-10-12 independent review broke four ways:
//   - a plain part beside a KV from the same reader;
//   - two readers that share an output (`inkClass(InkFaint)` and `toneInkClass(ToneMuted)` were both `text-slate-400`
//     under the default theme, until DS-D15 stage 2 gave each ink role its own class);
//   - a literal key beside a reader key;
//   - a false finding on `inkClass(...)+" font-medium"`.
//
// Blind spot: a part built by a Go helper whose body is not all literal returns (e.g. `settingsNavLinkInk`). It is
// skipped, not guessed.
func TestNoTwoKVKeysFromOneThemeReader(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	var sources []string
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		files[e.Name()] = string(b)
		sources = append(sources, string(b))
	}
	readers := themeReaderOutputs(strings.Join(sources, "\n"))
	for _, want := range []string{"inkClass", "toneInkClass", "toneFillClass", "radiusClass", "layoutGap", "backgroundClass"} {
		if len(readers[want]) == 0 {
			t.Fatalf("no outputs parsed for %s -- the reader scan is reading the wrong text", want)
		}
	}
	readers["toneClass"] = joinSets(readers["toneFillClass"], " ", readers["toneInkClass"])

	comment := regexp.MustCompile(`(?m)^[ \t]*//[^\n]*`)
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	expressions := 0
	for _, name := range names {
		for _, m := range classExpression.FindAllStringSubmatch(comment.ReplaceAllString(files[name], ""), -1) {
			expressions++
			type part struct {
				text string
				kv   bool
				outs map[string]bool
			}
			var parts []part
			for _, p := range splitTopLevel(m[1], ',') {
				p = strings.TrimSpace(p)
				kv := false
				if strings.HasPrefix(p, "templ.KV(") && strings.HasSuffix(p, ")") {
					args := splitTopLevel(p[len("templ.KV("):len(p)-1], ',')
					p, kv = strings.TrimSpace(args[0]), true
				}
				if outs := possibleOutputs(p, readers); outs != nil {
					parts = append(parts, part{p, kv, outs})
				}
			}
			for i := range parts {
				for j := i + 1; j < len(parts); j++ {
					if !parts[i].kv && !parts[j].kv {
						continue
					}
					for o := range parts[i].outs {
						if parts[j].outs[o] {
							t.Errorf("%s: %q and %q can both be %q under some Theme -- templ keys classes on the whole string, so the "+
								"later value wins and the true state can lose its class; choose the class in Go", name, parts[i].text, parts[j].text, o)
							break
						}
					}
				}
			}
		}
	}
	if expressions == 0 {
		t.Fatal("no class expression was scanned -- the gate is reading the wrong text")
	}
}

// themeReaderOutputs maps each `func name(ctx context.Context, x T) string` whose every return is a string literal to
// the set of those literals.
func themeReaderOutputs(src string) map[string]map[string]bool {
	fn := regexp.MustCompile(`(?s)\nfunc (\w+)\(ctx context\.Context(?:, \w+ [\w.]+)?\) string \{\n(.*?)\n\}`)
	ret := regexp.MustCompile(`(?m)^\s*return (.*)$`)
	lit := regexp.MustCompile(`^"([^"]*)"$`)
	out := map[string]map[string]bool{}
	for _, m := range fn.FindAllStringSubmatch(src, -1) {
		set, ok := map[string]bool{}, true
		for _, r := range ret.FindAllStringSubmatch(m[2], -1) {
			l := lit.FindStringSubmatch(strings.TrimSpace(r[1]))
			if l == nil {
				ok = false
				break
			}
			set[l[1]] = true
		}
		if ok && len(set) > 0 {
			out[m[1]] = set
		}
	}
	return out
}

// possibleOutputs is every string a class part can be: literals and known readers joined by `+`. Nil means unknown.
func possibleOutputs(expr string, readers map[string]map[string]bool) map[string]bool {
	call := regexp.MustCompile(`^(\w+)\(ctx(?:,[^()]*)?\)$`)
	lit := regexp.MustCompile(`^"([^"]*)"$`)
	acc := map[string]bool{"": true}
	for _, piece := range splitTopLevel(expr, '+') {
		piece = strings.TrimSpace(piece)
		var set map[string]bool
		if l := lit.FindStringSubmatch(piece); l != nil {
			set = map[string]bool{l[1]: true}
		} else if c := call.FindStringSubmatch(piece); c != nil && readers[c[1]] != nil {
			set = readers[c[1]]
		} else {
			return nil
		}
		acc = joinSets(acc, "", set)
	}
	return acc
}

func joinSets(a map[string]bool, sep string, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for x := range a {
		for y := range b {
			out[x+sep+y] = true
		}
	}
	return out
}

// splitTopLevel splits s on sep outside parentheses and string literals.
func splitTopLevel(s string, sep rune) []string {
	var out []string
	depth, inStr, start := 0, false, 0
	for i, r := range s {
		switch {
		case r == '"' && (i == 0 || s[i-1] != '\\'):
			inStr = !inStr
		case inStr:
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == sep && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
