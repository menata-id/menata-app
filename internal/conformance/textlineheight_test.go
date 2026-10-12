package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestCustomTextSizesDeclareTheirLineHeight holds the one thing a custom `--text-*` size in `static/css/input.css`
// cannot get from Tailwind by itself: a line height. Every built-in size ships a paired `--text-*--line-height`, so
// `.text-sm` sets both; a size added in `@theme` without one compiles to a `font-size`-only rule, and the element
// inherits whatever line height its parent happens to have.
//
// That is not hypothetical. `text-2xs` (11px) and `text-3xs` (10px) shipped that way, and the Design System
// artifact's `label`/`caption`/`eyebrow` styles say 1.5. Measured 2026-10-11 with Playwright over the 16
// authenticated pages the GET sweep fixture reaches, at 1280 and 390px: 416 of 738 such elements rendered at the
// parent's ratio (an 11px label at 15.71px instead of 16.5px). Nothing failed and nothing looked broken, which
// is why it lasted. Record: `menata-app-document`'s `audits/2026-10-11-kajian-sinkron-design-system-kode-css.md`, C2.
//
// It reads `--text-<name>:` declarations, skipping the `--text-<name>--<modifier>` companions themselves, and
// requires `--text-<name>--line-height` beside each. It does not check the value. 1.5 is the Design System's
// number, and choosing it is a design decision, not something a test can derive.
func TestCustomTextSizesDeclareTheirLineHeight(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(), "static", "css", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	src := stripCSSComments(string(b))

	sizes := regexp.MustCompile(`--text-([a-z0-9]+):`).FindAllStringSubmatch(src, -1)
	if len(sizes) == 0 {
		t.Fatal("no custom --text-* size found in input.css's @theme; the gate is reading the wrong file or the pattern broke")
	}
	for _, m := range sizes {
		name := m[1]
		if !regexp.MustCompile(`--text-` + name + `--line-height:`).MatchString(src) {
			t.Errorf("input.css declares --text-%s but no --text-%s--line-height\n"+
				"  a custom size without one compiles to a font-size-only utility, so the element inherits its parent's\n"+
				"  line height. Declare it beside the size (the Design System artifact's type styles give the value)",
				name, name)
		}
	}
}

// stripCSSComments removes /* ... */ so a comment naming a declaration cannot satisfy the gate, which is the
// failure TestNoClassLivesOnlyInAComment exists for, from the other side.
func stripCSSComments(s string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(s, "")
}
