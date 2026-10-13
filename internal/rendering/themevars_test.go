package rendering

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// Since DS-D15 stage 2 (2026-10-12) a colour role's class is the same under every theme (`text-ink-body`), and what
// it draws is decided by a custom property: input.css's `@theme` default, overridden by the Workspace's themeCSS in
// the page's `<head>`. A byte comparison can no longer show that a site follows the theme, so the tests that used to
// compare classes compare **what a class computes to**: resolveColourRoles rewrites each role class to the palette
// class its property resolves to on a page rendered under ctx. A hand-written literal is left as it is, so it still
// fails a moved-theme assertion, which is R9's point.

var roleClassPattern = regexp.MustCompile(`((?:[a-z-]+:)*)(text|bg|border)-(ink-[a-z]+|accent-hover|accent|bg-(?:raised|base|sunken)|border-(?:surface|control|divider)|hover-wash|tone-[a-z]+-(?:bg|ink|border))\b`)

// themeVarDefaults reads input.css's `@theme` declarations of the role properties (`--color-ink-body:
// var(--color-slate-700);`).
func themeVarDefaults(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(--color-(?:ink-[a-z]+|accent-hover|accent|bg-(?:raised|base|sunken)|border-(?:surface|control|divider)|hover-wash|tone-[a-z]+-(?:bg|ink|border))):\s*([^;]+);`).FindAllStringSubmatch(string(b), -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	if len(out) == 0 {
		t.Fatal("input.css declares no colour-role property -- the pattern no longer matches it")
	}
	return out
}

// themeVarOverrides parses themeCSS(ctx) (`:root{--color-ink-body:var(--color-slate-900);…}`).
func themeVarOverrides(ctx context.Context) map[string]string {
	out := map[string]string{}
	css := strings.TrimSuffix(strings.TrimPrefix(themeCSS(ctx), ":root{"), "}")
	if css == "" {
		return out
	}
	for _, d := range strings.Split(css, ";") {
		k, v, _ := strings.Cut(d, ":")
		out[k] = v
	}
	return out
}

// resolveColourRoles rewrites every role class in s to what it computes to under ctx's Workspace: `text-ink-body`
// becomes `text-slate-700` by default, `text-slate-900` under `ink.body: darkest`, and `text-[#e11d48]` under a colour
// value. A value it cannot name as a class (the free accent's `color-mix` hover) becomes `<prefix>-[<value>]`.
func resolveColourRoles(t *testing.T, ctx context.Context, s string) string {
	t.Helper()
	vars := themeVarDefaults(t)
	for k, v := range themeVarOverrides(ctx) {
		vars[k] = v
	}
	return roleClassPattern.ReplaceAllStringFunc(s, func(m string) string {
		sm := roleClassPattern.FindStringSubmatch(m)
		v, ok := vars["--color-"+sm[3]]
		if !ok {
			t.Fatalf("class %q names --color-%s, which neither input.css nor themeCSS declares", m, sm[3])
		}
		// An alias of another role (`hover-wash` is `bg-base`) resolves through it.
		for i := 0; i < 4; i++ {
			p, isVar := strings.CutPrefix(v, "var(--")
			next, ok := vars["--"+strings.TrimSuffix(p, ")")]
			if !isVar || !ok {
				break
			}
			v = next
		}
		if p, ok := strings.CutPrefix(v, "var(--color-"); ok {
			return sm[1] + sm[2] + "-" + strings.TrimSuffix(p, ")")
		}
		return sm[1] + sm[2] + "-[" + strings.ReplaceAll(v, " ", "_") + "]"
	})
}

// TestThemeCSSIsEmptyForTheDefaultTheme: a Workspace that declares no colour adds no `<style>`, so the default theme
// costs no bytes and the pre-auth screens (which have no Workspace) draw exactly what every default Workspace draws.
func TestThemeCSSIsEmptyForTheDefaultTheme(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: domain.DefaultTheme()}, "", false)
	if css := themeCSS(ctx); css != "" {
		t.Errorf("the default theme must add no override, got %q", css)
	}
	if css := themeCSS(context.Background()); css != "" {
		t.Errorf("no Workspace must add no override, got %q", css)
	}
}

// TestThemeCSSDefaultsMatchTheDefaultTheme is condition 1 of DS-D15 stage 2 held both ways: every colour role's
// property has a default in input.css, and that default is the shade domain.DefaultTheme() gives the role. A page with
// no `<style>` (pre-auth) therefore draws the default Theme, and a change to one side without the other fails.
func TestThemeCSSDefaultsMatchTheDefaultTheme(t *testing.T) {
	defaults := themeVarDefaults(t)
	def := domain.DefaultTheme()
	want := map[string]string{}
	for role := range domain.KnownInkRoles {
		want[inkRoleVar(role)] = "var(" + inkShadeVar(def.InkFor(role)) + ")"
	}
	want["--color-accent-hover"] = "var(" + accentHoverShadeVar(def.InkFor(domain.InkAccent)) + ")"
	for role := range domain.KnownBorderRoles {
		want["--color-border-"+string(role)] = "var(" + borderShadeVar(def.BorderFor(role)) + ")"
	}
	for role := range domain.KnownBackgroundRoles {
		want["--color-bg-"+string(role)] = "var(" + backgroundShadeVar(def.BackgroundFor(role)) + ")"
	}
	want["--color-hover-wash"] = "var(--color-bg-base)"
	for tone := range domain.KnownBadgeTones {
		p := def.ToneFor(tone)
		n := "--color-tone-" + string(tone)
		want[n+"-bg"], want[n+"-ink"], want[n+"-border"] = "var("+toneFillVar(p)+")", "var("+toneInkVar(p)+")", "var("+toneEdgeVar(p)+")"
	}
	for k, v := range want {
		if defaults[k] != v {
			t.Errorf("input.css declares %s: %q, the default Theme says %q -- move one so they agree", k, defaults[k], v)
		}
	}
	for k := range defaults {
		if _, ok := want[k]; !ok {
			t.Errorf("input.css declares %s, which no Theme role draws through -- remove it or give it a role", k)
		}
	}
}

// TestThemeCSSWritesEveryMovedRoleAndOnlyThose moves every ink role at once, to a closed shade and then to a colour
// value, and checks the override names each role once with the value the Theme gave it.
func TestThemeCSSWritesEveryMovedRoleAndOnlyThose(t *testing.T) {
	moved := domain.Theme{Ink: map[domain.InkRole]domain.InkShade{
		domain.InkStrong: domain.InkShadeDark, domain.InkBody: domain.InkShadeDarkest, domain.InkSecondary: domain.InkShadeDark,
		domain.InkFaint: domain.InkShadeMedium, domain.InkAccent: domain.InkHueViolet,
	}}
	got := themeVarOverrides(WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: moved}, "", false))
	want := map[string]string{
		"--color-ink-strong": "var(--color-slate-700)", "--color-ink-body": "var(--color-slate-900)",
		"--color-ink-secondary": "var(--color-slate-700)", "--color-ink-faint": "var(--color-slate-500)",
		"--color-accent": "var(--color-violet-600)", "--color-accent-hover": "var(--color-violet-700)",
	}
	if len(got) != len(want) {
		t.Errorf("override has %d declarations, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}

	others := domain.Theme{
		Border:     map[domain.BorderRole]domain.BorderShade{domain.BorderDivider: domain.BorderDefined},
		Background: map[domain.BackgroundRole]domain.BackgroundShade{domain.BackgroundSunken: domain.BackgroundShadeLighter, domain.BackgroundBase: domain.BackgroundShadeLight},
		Tone:       map[domain.BadgeTone]domain.TonePalette{domain.ToneWarn: domain.PaletteRed},
	}
	got = themeVarOverrides(WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: others}, "", false))
	want = map[string]string{
		"--color-border-divider": "var(--color-slate-300)",
		"--color-bg-sunken":      "var(--color-slate-50)", "--color-bg-base": "var(--color-slate-100)",
		"--color-tone-warn-bg": "var(--color-red-50)", "--color-tone-warn-ink": "var(--color-red-700)", "--color-tone-warn-border": "var(--color-red-200)",
	}
	if len(got) != len(want) {
		t.Errorf("override has %d declarations, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}

	brand := domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkAccent: "#be123c", domain.InkBody: "#1f2937"}}
	got = themeVarOverrides(WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: brand}, "", false))
	if got["--color-accent"] != "#be123c" || got["--color-ink-body"] != "#1f2937" {
		t.Errorf("a colour value must be written as itself: %v", got)
	}
	if got["--color-accent-hover"] != "color-mix(in oklab,#be123c,"+accentHoverMix+")" {
		t.Errorf("a free accent's hover must darken the same value: %q", got["--color-accent-hover"])
	}
}

// TestThemeCSSWritesEachRoleAndToneWhenMovedAlone moves every border role, background role and tone on its own, to a
// value other than its default, and requires an override naming it. The 2026-10-13 review dropped `neutral` and
// `muted` from themeCSS's tone list and every other test stayed green, because none moved those two.
func TestThemeCSSWritesEachRoleAndToneWhenMovedAlone(t *testing.T) {
	def := domain.DefaultTheme()
	at := func(th domain.Theme) map[string]string {
		return themeVarOverrides(WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "", false))
	}
	for role := range domain.KnownBorderRoles {
		for shade := range domain.KnownBorderShades {
			if shade == def.BorderFor(role) {
				continue
			}
			got := at(domain.Theme{Border: map[domain.BorderRole]domain.BorderShade{role: shade}})
			if want := "var(" + borderShadeVar(shade) + ")"; got["--color-border-"+string(role)] != want || len(got) != 1 {
				t.Errorf("border.%s: %s: got %v", role, shade, got)
			}
		}
	}
	for role := range domain.KnownBackgroundRoles {
		for shade := range domain.KnownBackgroundShades {
			if shade == def.BackgroundFor(role) {
				continue
			}
			got := at(domain.Theme{Background: map[domain.BackgroundRole]domain.BackgroundShade{role: shade}})
			if want := "var(" + backgroundShadeVar(shade) + ")"; got["--color-bg-"+string(role)] != want || len(got) != 1 {
				t.Errorf("background.%s: %s: got %v", role, shade, got)
			}
		}
	}
	for tone := range domain.KnownBadgeTones {
		for p := range domain.KnownTonePalettes {
			if p == def.ToneFor(tone) {
				continue
			}
			got := at(domain.Theme{Tone: map[domain.BadgeTone]domain.TonePalette{tone: p}})
			n := "--color-tone-" + string(tone)
			if got[n+"-bg"] != "var("+toneFillVar(p)+")" || got[n+"-ink"] != "var("+toneInkVar(p)+")" || got[n+"-border"] != "var("+toneEdgeVar(p)+")" || len(got) != 3 {
				t.Errorf("tone.%s: %s: got %v", tone, p, got)
			}
		}
	}
}

// TestThemeCSSNeverWritesAnUnvalidatedValue: the override goes into a `<style>`, so a value the loader would refuse
// must not reach it even when a Theme is built in Go without the loader (a fixture, a future caller). InkFor already
// falls back for a value that does not fit; this pins that the fallback is what themeCSS sees.
func TestThemeCSSNeverWritesAnUnvalidatedValue(t *testing.T) {
	for _, bad := range []domain.InkShade{"red", "#e11d48;}body{display:none", "#E11D48", "#e11d4", "</style><script>"} {
		th := domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkAccent: bad, domain.InkBody: bad}}
		css := themeCSS(WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "", false))
		if css != "" {
			t.Errorf("%q reached the override: %q", bad, css)
		}
	}
	th := domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkFaint: "#111111"}}
	if css := themeCSS(WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "", false)); css != "" {
		t.Errorf("faint takes no colour value, but the override wrote %q", css)
	}
}

// TestEveryPaletteVariableTheOverrideCanNameIsCompiled: Tailwind v4 emits a palette property only when something
// names it, so an override naming `--color-cyan-800` on a page whose CSS lacks it would draw no colour at all and
// nothing would fail. Every shade a Workspace may choose is checked against the committed app.css.
//
// Measured while mutation-testing it: Tailwind emits any `--color-*` that a scanned file **mentions**, so inkShadeVar's
// own string literals are what keep these in the bundle (an arm changed to `--color-cyan-950` simply emitted that).
// What this catches is a name the scanner cannot read -- one assembled at runtime, or a reader moved out of the
// `@source` glob (mutation-proved with `"--color-" + "lime-950"`).
func TestEveryPaletteVariableTheOverrideCanNameIsCompiled(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for shade := range domain.KnownInkShades {
		vars := []string{inkShadeVar(shade)}
		if domain.IsInkHue(shade) {
			vars = append(vars, accentHoverShadeVar(shade))
		}
		for _, v := range vars {
			if !strings.Contains(css, v+":") {
				t.Errorf("shade %q draws through %s, which app.css does not define -- a Workspace choosing it would draw no colour", shade, v)
			}
		}
	}
	for role := range domain.KnownInkRoles {
		if !strings.Contains(css, inkRoleVar(role)+":") {
			t.Errorf("role %q draws through %s, which app.css does not define", role, inkRoleVar(role))
		}
	}
	var steps []string
	for s := range domain.KnownBorderShades {
		steps = append(steps, borderShadeVar(s))
	}
	for s := range domain.KnownBackgroundShades {
		steps = append(steps, backgroundShadeVar(s))
	}
	for p := range domain.KnownTonePalettes {
		steps = append(steps, toneFillVar(p), toneInkVar(p), toneEdgeVar(p))
	}
	for _, v := range steps {
		if !strings.Contains(css, v+":") {
			t.Errorf("a closed shade draws through %s, which app.css does not define", v)
		}
	}
}

// TestAppShellWritesTheThemeOverrideInItsHead renders the shell under a brand accent and checks the `<style>` sits in
// the `<head>`, before any content, and that the default theme writes none.
func TestAppShellWritesTheThemeOverrideInItsHead(t *testing.T) {
	render := func(th domain.Theme) string {
		ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "t", Theme: th}, "", false)
		var b strings.Builder
		if err := appShell("T", "W", Viewer{Name: "A", Initials: "A"}, "").Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	html := render(domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkAccent: "#be123c"}})
	head, _, _ := strings.Cut(html, "</head>")
	if !strings.Contains(head, "<style>:root{--color-accent:#be123c;") {
		t.Errorf("the brand accent's override is not in the head: %s", head)
	}
	if strings.Contains(render(domain.DefaultTheme()), "<style>") {
		t.Error("the default theme must write no <style>")
	}
}
