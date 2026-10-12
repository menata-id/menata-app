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
		// Class attributes (literals joined) and every other string literal: fieldClasses builds the field recipe in a
		// Go function, which a class-attribute scan alone cannot see (mutation-proved).
		var sets []string
		for _, m := range classAttr.FindAllStringSubmatch(joinClassLiterals(src), -1) {
			sets = append(sets, m[1])
		}
		for _, m := range tonePairLiteral.FindAllStringSubmatch(src, -1) {
			sets = append(sets, m[1])
		}
		for _, set := range sets {
			scanned++
			toks := " " + set + " "
			kills := strings.Contains(toks, " outline-none ") || strings.Contains(toks, " outline-hidden ")
			ring := strings.Contains(toks, " focus:outline-") || strings.Contains(toks, " focus-visible:outline-")
			if kills && ring && !strings.Contains(toks, " focus:outline-solid ") && !strings.Contains(toks, " focus-visible:outline-solid ") {
				t.Errorf("%s: %q removes the outline style and then sets only its width and colour on focus -- under Tailwind v4 the ring never shows; use focus:outline-solid", e.Name(), strings.TrimSpace(set))
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no class attribute was scanned -- the gate is reading the wrong text")
	}
}
