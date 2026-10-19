package rendering

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestUnmappedLengthTokensMatchTheirLiterals closes the part of GS1 that read "unmapped, with a reason" and compared
// nothing (gap-plan P3e, 2026-10-14). Three of the artifact's lengths have no Theme role because their code is a literal
// in one named function: control-height (`h-9` in buttonClasses and fieldClasses), pad-control-x (`px-4` in
// buttonClasses) and pad-metric (`p-3` in metricTile). A reason is a record; this is the check. If the literal moves, the
// artifact changes, or the function is renamed, it fails -- instead of the two quietly meaning different things.
func TestUnmappedLengthTokensMatchTheirLiterals(t *testing.T) {
	ds := loadDesignSystemTokens(t)
	css := compiledCSSVars(t)
	cases := []struct{ token, file, anchor, class string }{
		{"control-height", "controls.templ", "func buttonClasses(", "h-9"},
		{"control-height", "controls.templ", "func fieldClasses(", "h-9"},
		{"pad-control-x", "controls.templ", "func buttonClasses(", "px-4"},
		{"pad-metric", "machine.templ", "templ metricTile(", "p-3"},
		{"radius-inner", "segmentedcontrol.templ", "templ segmentedTrack(", "rounded"},
	}
	for _, c := range cases {
		if _, ok := dsUnmapped[c.token]; !ok {
			t.Errorf("%s is no longer listed in dsUnmapped -- if a reader answers for it now, retire this case", c.token)
		}
		b, err := os.ReadFile(filepath.Join(c.file))
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		i := strings.Index(src, c.anchor)
		if i < 0 {
			t.Errorf("%s: %q not found -- the literal behind %s moved; update this case", c.file, c.anchor, c.token)
			continue
		}
		body := src[i:]
		if j := strings.Index(body, "\n}"); j >= 0 {
			body = body[:j]
		}
		// Every token of the same utility in the function must be this class: a recipe with three variants of which one
		// drifted would otherwise pass on the two that did not.
		prefix := c.class[:strings.LastIndex(c.class, "-")+1]
		tokenRe := `(?:^|[\s"])(` + regexp.QuoteMeta(prefix) + `[0-9.]+)(?:[\s"]|$)`
		if c.class == "rounded" { // the bare utility has no "-N" to take a prefix from
			tokenRe = `(?:^|[\s"])(rounded(?:-[a-z0-9]+)?)(?:[\s"]|$)` // the whole family: a knob that drifted to rounded-md must fail
		}
		all := regexp.MustCompile(tokenRe).FindAllStringSubmatch(body, -1)
		if len(all) == 0 {
			t.Errorf("%s in %s writes no %s class -- the literal behind %s moved", c.anchor, c.file, prefix+"N", c.token)
			continue
		}
		for _, m := range all {
			if m[1] != c.class {
				t.Errorf("%s in %s writes %s, and the token %s describes %s", c.anchor, c.file, m[1], c.token, c.class)
			}
		}
		want := pxOf(t, ds.length(t, c.token))
		got := 0.0
		if c.class == "rounded" {
			m := regexp.MustCompile(`\.rounded\{border-radius:([0-9.]+rem)\}`).FindStringSubmatch(appCSSText(t))
			if m == nil {
				t.Fatal("app.css has no `.rounded` rule")
			}
			got = remToPx(t, m[1])
		} else {
			got = css.lengthOf(t, c.class)
		}
		if got != want {
			t.Errorf("%s: the artifact says %s = %vpx and %s draws %vpx", c.token, c.token, want, c.class, got)
		}
	}
}

// TestUnmappedShadowTokensMatchTheirCode does the same for the four shadows. The artifact writes `rgba()`, Tailwind
// compiles `#rrggbbaa`, and the two arbitrary shadows are class literals; all are reduced to the same signature (the
// lengths and an rgba colour with the alpha rounded to two places) before they are compared.
func TestUnmappedShadowTokensMatchTheirCode(t *testing.T) {
	ds := loadDesignSystemTokens(t)
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	cssText := string(b)
	compiled := func(class string) string {
		m := regexp.MustCompile(`\.` + regexp.QuoteMeta(class) + `\{[^}]*--tw-shadow:([^;}]*)`).FindStringSubmatch(cssText)
		if m == nil {
			t.Fatalf("app.css has no rule for %s -- the token it backs would draw nothing", class)
		}
		return m[1]
	}
	literal := func(file, prefix string) string {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(regexp.QuoteMeta(prefix) + `\[([^\]]*)\]`).FindStringSubmatch(string(src))
		if m == nil {
			t.Fatalf("%s writes no %s[...] shadow", file, prefix)
		}
		return strings.ReplaceAll(m[1], "_", " ")
	}
	cases := []struct{ token, code string }{
		{"shadow-popover", compiled("shadow-lg")},
		{"shadow-hover", compiled(`hover\:shadow-sm:hover`)},
		{"shadow-card-auth", literal("authshell.templ", "shadow-")},
		{"shadow-sheet", literal("appshell.templ", "shadow-")},
	}
	for _, c := range cases {
		if _, ok := dsUnmapped[c.token]; !ok {
			t.Errorf("%s is no longer listed in dsUnmapped -- if a reader answers for it now, retire this case", c.token)
		}
		want, got := shadowSignature(t, ds.length(t, c.token)), shadowSignature(t, c.code)
		if want != got {
			t.Errorf("%s: the artifact says %s and the code draws %s", c.token, want, got)
		}
	}
}

var shadowColour = regexp.MustCompile(`#[0-9a-fA-F]{8}\b|#[0-9a-fA-F]{6}\b|rgba?\([^)]*\)`)

// shadowSignature reduces a box-shadow to its layers' numbers, the colour as r,g,b,a with a rounded to two places.
func shadowSignature(t *testing.T, s string) string {
	t.Helper()
	s = regexp.MustCompile(`var\(--tw-shadow-color,\s*([^)]*)\)`).ReplaceAllString(s, "$1")
	s = shadowColour.ReplaceAllStringFunc(s, func(c string) string {
		var r, g, bl, a float64 = 0, 0, 0, 1
		switch {
		case strings.HasPrefix(c, "#"):
			h := strings.TrimPrefix(c, "#")
			v := func(i int) float64 { n, _ := strconv.ParseUint(h[i:i+2], 16, 8); return float64(n) }
			r, g, bl = v(0), v(2), v(4)
			if len(h) == 8 {
				a = v(6) / 255
			}
		default:
			nums := regexp.MustCompile(`[0-9.]+`).FindAllString(c, -1)
			f := func(i int) float64 {
				if i >= len(nums) {
					return 1
				}
				n, _ := strconv.ParseFloat(nums[i], 64)
				return n
			}
			r, g, bl = f(0), f(1), f(2)
			if len(nums) > 3 {
				a = f(3)
			}
		}
		return "C(" + strconv.FormatFloat(r, 'f', 0, 64) + "," + strconv.FormatFloat(g, 'f', 0, 64) + "," + strconv.FormatFloat(bl, 'f', 0, 64) + "," + strconv.FormatFloat(round2(a), 'f', 2, 64) + ")"
	})
	s = strings.ReplaceAll(s, "px", "")
	return strings.Join(strings.Fields(s), " ")
}

// TestUnmappedAlphaTokensMatchTheirReaders gives the two tokens whose code is a role at an opacity a comparison
// (gap plan K-08, owner decision 2026-10-14: the artifact follows the code). `accent-ring` is the accent at 20% (the halo
// around the current step, `accentRingClass`) and `primary-hover` is the strong ink at 90% (`inkFillHoverClass`). Each is
// the reader's own opacity applied to the role's colour in each mode, written the way the artifact writes it.
func TestUnmappedAlphaTokensMatchTheirReaders(t *testing.T) {
	ds := loadDesignSystemTokens(t)
	light, dark := compiledCSSVars(t), compiledCSSVarsDark(t)
	cases := []struct{ token, class string }{
		{"accent-ring", accentRingClass(context.Background())},
		{"primary-hover", strings.TrimPrefix(inkFillHoverClass(context.Background()), "hover:")},
	}
	for _, c := range cases {
		if _, ok := dsUnmapped[c.token]; !ok {
			t.Errorf("%s is no longer listed in dsUnmapped -- if a reader answers for it now, retire this case", c.token)
		}
		base, pct, ok := strings.Cut(c.class[strings.Index(c.class, "-")+1:], "/")
		if !ok {
			t.Fatalf("%s: %s has no opacity", c.token, c.class)
		}
		for mode, css := range map[string]cssVars{"light": light, "dark": dark} {
			rgb := css.colourOf(t, "x-"+base)
			alpha := strings.TrimPrefix(strconv.FormatFloat(mustFloat(t, pct)/100, 'f', -1, 64), "0")
			want := normalizeColour(strings.TrimSuffix(rgb, ")") + " / " + alpha + ")")
			if got := normalizeColour(ds.colourIn(t, c.token, mode)); got != want {
				t.Errorf("%s (%s): the artifact says %s, the code draws %s at %s%% = %s", c.token, mode, got, base, pct, want)
			}
		}
	}
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%q is not a number", s)
	}
	return f
}

func appCSSText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
