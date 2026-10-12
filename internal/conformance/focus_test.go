package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestKeyboardFocusRuleCoversInteractiveElements holds owner decision DS-D14 (2026-10-12). One base rule in
// static/css/input.css gives every link, button, summary and focusable element the Design System's `focus-ring`
// (blue-300) under :focus-visible, as the mockups do. A per-site class would be the same rule repeated hundreds of
// times. The gate reads the source and the compiled CSS, so a rule that compiled away fails too.
func TestKeyboardFocusRuleCoversInteractiveElements(t *testing.T) {
	root := repoRoot()
	src, err := os.ReadFile(filepath.Join(root, "static", "css", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile(filepath.Join(root, "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	rule := regexp.MustCompile(`:where\((.*?)\):focus-visible\s*\{([^}]*)\}`)
	for name, text := range map[string]string{"input.css": string(src), "app.css": string(css)} {
		m := rule.FindStringSubmatch(text)
		if m == nil {
			t.Errorf("%s: no :where(...):focus-visible rule -- keyboard focus falls back to each browser's own ring (DS-D14)", name)
			continue
		}
		for _, sel := range []string{"a[href]", "button", "summary"} {
			if !strings.Contains(m[1], sel) {
				t.Errorf("%s: the focus rule does not cover %s", name, sel)
			}
		}
		decl := strings.Join(strings.Fields(m[2]), " ")
		if !strings.Contains(decl, "solid var(--color-blue-300)") || !strings.Contains(strings.ReplaceAll(decl, " ", ""), "2px") {
			t.Errorf("%s: the focus rule is %q, want a 2px solid outline in --color-blue-300 (the Design System's focus-ring)", name, decl)
		}
	}
	if !strings.Contains(string(css), "--color-blue-300:") {
		t.Error("app.css does not define --color-blue-300, so the focus rule would resolve to nothing")
	}
}

// TestNoOutlineNoneBesideAFocusRing is a zero-floor gate (2026-10-12). Under Tailwind v4, `outline-none` and
// `outline-hidden` set `--tw-outline-style: none`, and `focus:outline-2` reuses that variable. So a field written
// `outline-none focus:outline-2 ...` never draws its ring. That was every field in the app until DS-D14 measured it.
// A focus ring declares its style with `focus:outline-solid` instead.
var stringSlice = regexp.MustCompile(`\[\]string\{([^}]*)\}`)

func TestNoOutlineNoneBesideAFocusRing(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	scanned := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		src := stripTemplComments(string(b))
		// Three readings, each a set of classes that land on one element:
		//   - a class attribute with its literals joined (a kill and a ring split across two literals);
		//   - every `[]string{...}` with its literals joined (fieldClasses-style recipes built with strings.Join);
		//   - every other string literal (a Go helper's whole class string).
		// The first version read single literals only; the 2026-10-12 review split the kill from the ring and passed.
		var sets []string
		for _, m := range classAttr.FindAllStringSubmatch(joinClassLiterals(src), -1) {
			sets = append(sets, m[1])
		}
		for _, m := range stringSlice.FindAllStringSubmatch(src, -1) {
			var parts []string
			for _, l := range tonePairLiteral.FindAllStringSubmatch(m[1], -1) {
				parts = append(parts, l[1])
			}
			sets = append(sets, strings.Join(parts, " "))
		}
		for _, m := range tonePairLiteral.FindAllStringSubmatch(src, -1) {
			sets = append(sets, m[1])
		}
		for _, set := range sets {
			scanned++
			toks := " " + set + " "
			kills := strings.Contains(toks, " outline-none ") || strings.Contains(toks, " outline-hidden ")
			if kills && !strings.Contains(toks, " focus:outline-solid ") && !strings.Contains(toks, " focus-visible:outline-solid ") {
				// A kill with no ring at all beats the base :focus-visible rule (a utility outranks @layer base), and a kill
				// beside a ring with no style hides that ring under Tailwind v4. Either way keyboard focus is invisible.
				t.Errorf("%s: %q removes the outline with no focus:outline-solid -- keyboard focus would be invisible (Tailwind v4 reuses the killed style, and a utility outranks the base focus rule)", e.Name(), strings.TrimSpace(set))
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no class attribute was scanned -- the gate is reading the wrong text")
	}
}
