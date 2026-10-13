package rendering

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestThemeColourPairsMeetContrast is GS3 of menata-app-document's guides/menjaga-design-system.md. It is the
// gate that would have caught L7 there: five "finished" marks drawn in emerald-500, 2.46:1 on white, where WCAG
// 1.4.11 asks 3:1 of a meaningful graphic. Nothing failed then, because no test ever looked at a colour as a
// colour.
//
// It asks the real readers for their default classes (`toneFillClass`/`toneInkClass`, `inkClass`,
// `backgroundClass`, `markGoodFill`), so a reader changing its answer is measured. It then resolves each class to
// the `oklch` value Tailwind compiled into the committed `static/css/app.css`, so the numbers are the shipped ones
// and not a table retyped here. Thresholds: 4.5:1 for text (WCAG 1.4.3; badge and caption text is 10–14px, never
// "large"), 3:1 for a solid mark (1.4.11).
//
// Pairs that are low on purpose sit in `lowContrastByDesign` with their reason. That map is closed both ways: a
// pair that now passes fails until its entry goes, so an exception cannot outlive the reason it was granted for.
//
// **Blind spots, stated:**
// - Only the default Theme is measured. A Workspace's `theme:` remap is not.
// - Hand-written colours outside the readers are not measured either (the hand-written ratchets count those).
// - Placement is not measured: a pair passing here can still sit on a third colour somewhere.
func TestThemeColourPairsMeetContrast(t *testing.T) {
	ctx := context.Background()
	css := compiledColours(t)
	white := oklchColour{L: 1}

	type pair struct {
		name, fg, bg string
		min          float64
	}
	var pairs []pair
	for tone := range domain.KnownBadgeTones {
		pairs = append(pairs, pair{"tone " + string(tone), toneInkClass(ctx, tone), toneFillClass(ctx, tone), 4.5})
	}
	for ink := range domain.KnownInkRoles {
		for _, bg := range []domain.BackgroundRole{domain.BackgroundRaised, domain.BackgroundBase} {
			pairs = append(pairs, pair{fmt.Sprintf("ink %s on %s", ink, bg), inkClass(ctx, ink), resolveColourRoles(t, ctx, backgroundClass(ctx, bg)), 4.5})
		}
	}
	// Every accent hue a Workspace may pick (DS-D10), not only the default: a hue that fails here would be a
	// declarable `theme.ink.accent` that makes every link unreadable. Measured on the default backgrounds.
	for shade := range domain.KnownInkShades {
		if !domain.IsInkHue(shade) || shade == domain.DefaultTheme().InkFor(domain.InkAccent) {
			continue
		}
		hued := WithCurrentWorkspace(ctx, domain.Workspace{Theme: domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkAccent: shade}}}, "", false)
		// inkClass answers `text-accent` under every theme (DS-D15 stage 2); what the hue draws is the Workspace's
		// override, so the class is resolved under that Workspace first. Without it this loop measured the default
		// blue once per hue and passed.
		accent := resolveColourRoles(t, hued, inkClass(hued, domain.InkAccent))
		if accent == inkClass(ctx, domain.InkAccent) || accent == resolveColourRoles(t, ctx, inkClass(ctx, domain.InkAccent)) {
			t.Fatalf("accent=%s resolves to %q, the default -- the hue never reached the measurement", shade, accent)
		}
		for _, bg := range []domain.BackgroundRole{domain.BackgroundRaised, domain.BackgroundBase} {
			pairs = append(pairs, pair{fmt.Sprintf("ink accent=%s on %s", shade, bg), accent, resolveColourRoles(t, ctx, backgroundClass(ctx, bg)), 4.5})
		}
	}
	pairs = append(pairs,
		pair{"mark-good on raised", markGoodFill(), resolveColourRoles(t, ctx, backgroundClass(ctx, domain.BackgroundRaised)), 3},
		pair{"mark-good on sunken (a progress track)", markGoodFill(), resolveColourRoles(t, ctx, backgroundClass(ctx, domain.BackgroundSunken)), 3},
	)
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })

	seen := map[string]bool{}
	for _, p := range pairs {
		fg, err := css.lookup(p.fg, white)
		if err != nil {
			t.Fatalf("%s: %v", p.name, err)
		}
		bg, err := css.lookup(p.bg, white)
		if err != nil {
			t.Fatalf("%s: %v", p.name, err)
		}
		ratio := contrastRatio(fg, bg)
		reason, excused := lowContrastByDesign[p.name]
		t.Logf("%-40s %-18s on %-14s %5.2f:1 (min %.1f)", p.name, p.fg, p.bg, ratio, p.min)
		switch {
		case ratio < p.min && !excused:
			t.Errorf("%s: %s on %s is %.2f:1, below %.1f:1 -- a colour role that carries meaning must be readable; change the reader's default or, if low contrast is the point, add it to lowContrastByDesign with the reason", p.name, p.fg, p.bg, ratio, p.min)
		case ratio >= p.min && excused:
			t.Errorf("%s is excused in lowContrastByDesign (%q) but now measures %.2f:1, above %.1f:1 -- remove the entry", p.name, reason, ratio, p.min)
		}
		seen[p.name] = true
	}
	for name := range lowContrastByDesign {
		if !seen[name] {
			t.Errorf("lowContrastByDesign names %q, which this gate no longer measures -- remove the entry", name)
		}
	}
}

// lowContrastByDesign is the closed list of measured pairs that are below their threshold on purpose.
var lowContrastByDesign = map[string]string{
	"ink faint on raised": "faint is for hints, placeholders and decoration -- text that may be missed (domain.InkFaint's own comment); ~2.6:1",
	"ink faint on base":   "the same role on the page canvas",
	"tone muted":          "muted means not applicable, deliberately de-emphasised and distinct from neutral; never for content that must be read",
}

type oklchColour struct{ L, C, H float64 }

type compiledCSS map[string]oklchColour

// compiledColours reads every `--color-*: oklch(...)` variable Tailwind compiled into the committed app.css.
func compiledColours(t *testing.T) compiledCSS {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	light, _ := compiledCSSModes(t, string(b))
	return parseCompiledColours(t, light)
}

// compiledDarkColours is compiledColours as the browser resolves it under `prefers-color-scheme: dark`: the light
// values with app.css's dark block applied over them.
func compiledDarkColours(t *testing.T) compiledCSS {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	light, dark := compiledCSSModes(t, string(b))
	return parseCompiledColours(t, light+dark)
}

// appCSSDarkBlock opens the compiled dark-mode block (input.css's, minified).
const appCSSDarkBlock = "@media (prefers-color-scheme:dark){:root{"

// compiledCSSModes splits app.css into everything but the dark block, and the dark block's declarations.
func compiledCSSModes(t *testing.T, css string) (light, dark string) {
	t.Helper()
	before, rest, ok := strings.Cut(css, appCSSDarkBlock)
	if !ok {
		t.Fatalf("app.css has no %q -- the dark-mode block moved or was not compiled", appCSSDarkBlock)
	}
	dark, after, ok := strings.Cut(rest, "}}")
	if !ok {
		t.Fatal("app.css's dark block does not close")
	}
	return before + after, ";" + dark + ";"
}

func parseCompiledColours(t *testing.T, css string) compiledCSS {
	t.Helper()
	out := compiledCSS{}
	re := regexp.MustCompile(`--color-([a-z]+-[0-9]+):oklch\(([0-9.]+)% ([0-9.]+) ([0-9.]+)\)`)
	for _, m := range re.FindAllStringSubmatch(css, -1) {
		l, _ := strconv.ParseFloat(m[2], 64)
		c, _ := strconv.ParseFloat(m[3], 64)
		h, _ := strconv.ParseFloat(m[4], 64)
		out[m[1]] = oklchColour{L: l / 100, C: c, H: h}
	}
	// Colour roles alias a palette step (DS-D15 stage 2: `--color-ink-body:var(--color-slate-700)`); each resolves
	// to the step's value, as the browser resolves it on a page with no Workspace override.
	out["white"] = oklchColour{L: 1}
	out["black"] = oklchColour{}
	aliases := regexp.MustCompile(`--color-([a-z]+(?:-[a-z]+)*):var\(--color-([a-z]+(?:-[a-z0-9]+)*)\)`).FindAllStringSubmatch(css, -1)
	for range 3 { // an alias of an alias (`hover-wash` is `bg-base`, which is `slate-50`)
		for _, m := range aliases {
			if col, ok := out[m[2]]; ok {
				out[m[1]] = col
			}
		}
	}
	if len(out) < 20 {
		t.Fatalf("found only %d oklch colour variables in app.css -- the pattern no longer matches what Tailwind emits", len(out))
	}
	return out
}

// lookup resolves one utility class (`text-slate-500`, `bg-emerald-50`, `bg-white`) to its compiled colour.
func (c compiledCSS) lookup(class string, white oklchColour) (oklchColour, error) {
	name := class[strings.Index(class, "-")+1:]
	if name == "white" {
		return white, nil
	}
	col, ok := c[name]
	if !ok {
		return oklchColour{}, fmt.Errorf("class %q has no --color-%s in app.css", class, name)
	}
	return col, nil
}

// contrastRatio is WCAG 2.x's (L1+0.05)/(L2+0.05) over relative luminance, from oklch through linear sRGB.
func contrastRatio(a, b oklchColour) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c oklchColour) float64 {
	a := c.C * math.Cos(c.H*math.Pi/180)
	bb := c.C * math.Sin(c.H*math.Pi/180)
	l := math.Pow(c.L+0.3963377774*a+0.2158037573*bb, 3)
	m := math.Pow(c.L-0.1055613458*a-0.0638541728*bb, 3)
	s := math.Pow(c.L-0.0894841775*a-1.2914855480*bb, 3)
	clamp := func(v float64) float64 { return math.Min(1, math.Max(0, v)) }
	r := clamp(4.0767416621*l - 3.3077115913*m + 0.2309699292*s)
	g := clamp(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s)
	bl := clamp(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s)
	return 0.2126*r + 0.7152*g + 0.0722*bl
}

// TestInkShadeColoursMatchTheCompiledCSS holds domain.InkShadeColour -- what the loader measures a closed ink shade
// with -- to the step inkShadeVar names, as compiled into app.css.
func TestInkShadeColoursMatchTheCompiledCSS(t *testing.T) {
	css := compiledColours(t)
	for shade := range domain.KnownInkShades {
		name := strings.TrimPrefix(inkShadeVar(shade), "--color-")
		got, ok := css[name]
		if !ok {
			t.Fatalf("shade %q: no --color-%s in app.css", shade, name)
		}
		want, ok := domain.InkShadeColour(shade)
		if !ok || math.Abs(got.L-want.L) > 1e-9 || math.Abs(got.C-want.C) > 1e-9 || math.Abs(got.H-want.H) > 1e-9 {
			t.Errorf("shade %q: domain says %+v, %s compiles to %+v -- move domain.InkShadeColour", shade, want, name, got)
		}
	}
}

// TestEveryAccentHueDrawsItsOwnHue: each hue's arm names that hue, and its hover is the same hue one step darker. A
// swapped arm (emerald drawing cyan) passes every contrast check, because both hues clear 4.5:1 (the 2026-10-12
// review's surviving mutation).
func TestEveryAccentHueDrawsItsOwnHue(t *testing.T) {
	step := regexp.MustCompile(`^--color-([a-z]+)-([0-9]+)$`)
	for shade := range domain.KnownInkShades {
		if !domain.IsInkHue(shade) {
			continue
		}
		a, h := step.FindStringSubmatch(inkShadeVar(shade)), step.FindStringSubmatch(accentHoverShadeVar(shade))
		if a == nil || h == nil || a[1] != string(shade) || h[1] != string(shade) {
			t.Errorf("hue %q draws %s and hovers %s -- both must be the %s palette", shade, inkShadeVar(shade), accentHoverShadeVar(shade), shade)
			continue
		}
		as, _ := strconv.Atoi(a[2])
		hs, _ := strconv.Atoi(h[2])
		if hs != as+100 {
			t.Errorf("hue %q hovers at step %d, want one step darker than %d", shade, hs, as)
		}
	}
}

// TestAccentHoverIsAtLeastAsReadableAsTheAccent measures the hover the readers really draw: for every closed hue the
// compiled step, and for a free value the colour-mix themeCSS writes, computed here as the same interpolation in
// OKLab. Hover darkens, so on a light surface it must never read worse than the accent itself.
func TestAccentHoverIsAtLeastAsReadableAsTheAccent(t *testing.T) {
	css := compiledColours(t)
	raised := domain.BackgroundShadeColour(domain.DefaultTheme().BackgroundFor(domain.BackgroundRaised))
	for shade := range domain.KnownInkShades {
		if !domain.IsInkHue(shade) {
			continue
		}
		a := css[strings.TrimPrefix(inkShadeVar(shade), "--color-")]
		h := css[strings.TrimPrefix(accentHoverShadeVar(shade), "--color-")]
		if ra, rh := domain.ContrastRatio(domain.Colour(a), raised), domain.ContrastRatio(domain.Colour(h), raised); rh < ra {
			t.Errorf("hue %q: hover %.2f:1 reads worse than the accent %.2f:1", shade, rh, ra)
		}
	}
	m := regexp.MustCompile(`^black ([0-9]+)%$`).FindStringSubmatch(accentHoverMix)
	if m == nil {
		t.Fatalf("accentHoverMix %q is not a mix with black -- a free accent's hover could read worse than the accent", accentHoverMix)
	}
	p, _ := strconv.Atoi(m[1])
	if p <= 0 || p > 50 {
		t.Fatalf("accentHoverMix mixes %d%% black; want a visible darkening that keeps the hue (1-50)", p)
	}
	for _, v := range []string{"#be123c", "#2563eb", "#0f766e"} {
		c, _ := domain.ParseColourValue(v)
		// color-mix(in oklab, c, black p%): L, a and b scale by (1-p); chroma scales with them and hue is kept.
		k := 1 - float64(p)/100
		hover := domain.Colour{L: c.L * k, C: c.C * k, H: c.H}
		if domain.ContrastRatio(hover, raised) < domain.ContrastRatio(c, raised) {
			t.Errorf("%s: the mixed hover reads worse than the accent", v)
		}
	}
}

// TestBackgroundShadeColoursMatchTheCompiledCSS holds domain.BackgroundShadeColour -- what the loader measures a
// Workspace's free ink value against (DS-D15 stage 2) -- to the colour each shade's class really compiles to. The
// table lives in domain because the loader may not read app.css; this is what keeps the two from drifting.
func TestBackgroundShadeColoursMatchTheCompiledCSS(t *testing.T) {
	css := compiledColours(t)
	white := oklchColour{L: 1}
	for shade := range domain.KnownBackgroundShades {
		ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: domain.Theme{Background: map[domain.BackgroundRole]domain.BackgroundShade{domain.BackgroundRaised: shade}}}, "", false)
		class := resolveColourRoles(t, ctx, backgroundClass(ctx, domain.BackgroundRaised))
		got, err := css.lookup(class, white)
		if err != nil {
			t.Fatal(err)
		}
		want := domain.BackgroundShadeColour(shade)
		if math.Abs(got.L-want.L) > 1e-6 || math.Abs(got.C-want.C) > 1e-6 || (got.C > 0 && math.Abs(got.H-want.H) > 1e-3) {
			t.Errorf("shade %q: domain says %+v, %s compiles to %+v -- move domain.BackgroundShadeColour", shade, want, class, got)
		}
		if r := domain.ContrastRatio(domain.Colour(got), want); r > 1.0001 {
			t.Errorf("shade %q: the two colours differ by %.4f:1", shade, r)
		}
	}
}

// TestDomainContrastAgreesWithThisGate: the loader's contrast (domain.ContrastRatio) and this gate's (contrastRatio)
// are the same formula written twice, because this file predates the loader needing one. They must agree on every
// pair the gate measures.
func TestDomainContrastAgreesWithThisGate(t *testing.T) {
	css := compiledColours(t)
	for a, ca := range css {
		for _, b := range []string{"slate-50", "slate-100", "slate-900"} {
			cb := css[b]
			if d := math.Abs(contrastRatio(ca, cb) - domain.ContrastRatio(domain.Colour(ca), domain.Colour(cb))); d > 1e-9 {
				t.Errorf("%s on %s: the gate and domain differ by %g", a, b, d)
			}
		}
	}
}

// TestTextOnSunkenMeetsContrast holds owner decision 2026-10-13: no `secondary` text on `sunken`. The default
// secondary is 4.35:1 on the default sunken, so text drawn on a sunken surface by a shared component reads a darker
// role. The loader cannot require secondary to pass on sunken without refusing the default Theme, which is why the
// rule is held here, at the sites, and why domain.InkSurfaces leaves sunken out for secondary.
// Blind spot: only the components named below; a hand-written sunken surface with secondary text is not seen.
func TestTextOnSunkenMeetsContrast(t *testing.T) {
	ctx := context.Background()
	css := compiledColours(t)
	sunken, err := css.lookup(resolveColourRoles(t, ctx, backgroundClass(ctx, domain.BackgroundSunken)), oklchColour{L: 1})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := notBuiltYetTag().Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := resolveColourRoles(t, ctx, buf.String())
	if !strings.Contains(html, resolveColourRoles(t, ctx, backgroundClass(ctx, domain.BackgroundSunken))) {
		t.Fatalf("the Not built yet tag no longer sits on sunken -- move this check with it: %s", html)
	}
	ink := regexp.MustCompile(`\btext-(slate|blue|red|emerald|amber)-[0-9]+\b`).FindString(html)
	fg, err := css.lookup(ink, oklchColour{L: 1})
	if err != nil {
		t.Fatalf("%s: %v", html, err)
	}
	if r := contrastRatio(fg, sunken); r < 4.5 {
		t.Errorf("the Not built yet tag draws %s on sunken at %.2f:1, below 4.5:1", ink, r)
	}
}

// TestSurfaceFloorKeepsFixedColoursReadable holds the reason the loader may stop measuring what it does not know
// about: a surface value may be no darker than the `light` shade (domain.SurfaceLuminanceFloor), and contrast is
// luminance alone, so a fixed colour that passes on `light` passes on every accepted value. This checks the ones that
// matter on the floor shade itself: mark-good (3:1), and the fixed inks drawn on surfaces without a role -- the page's
// own text, a field's text, and each tone's ink alone (4.5:1). A change that darkens `light`, or a fixed colour that
// fails on it, fails here rather than in a Workspace.
func TestSurfaceFloorKeepsFixedColoursReadable(t *testing.T) {
	css := compiledColours(t)
	white := oklchColour{L: 1}
	floor, ok := css[strings.TrimPrefix(backgroundShadeVar(domain.BackgroundShadeLight), "--color-")]
	if !ok {
		t.Fatal("the light shade has no compiled colour")
	}
	if d := math.Abs(luminance(floor) - domain.SurfaceLuminanceFloor()); d > 1e-6 {
		t.Errorf("domain.SurfaceLuminanceFloor differs from the compiled light shade by %g", d)
	}
	check := func(name, class string, min float64) {
		c, err := css.lookup(class, white)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if r := contrastRatio(c, floor); r < min {
			t.Errorf("%s (%s) is %.2f:1 on the light shade, below %.1f:1 -- a surface value at the floor would make it unreadable", name, class, r, min)
		}
	}
	check("mark-good", markGoodFill(), 3)
	check("the page's own text", "text-slate-900", 4.5)
	for tone := range domain.KnownBadgeTones {
		if tone == domain.ToneMuted || tone == domain.ToneNeutral {
			continue // drawn on their own fill (badges), not alone; muted is low by design
		}
		check("tone "+string(tone)+" ink alone", "text-"+strings.TrimPrefix(toneInkVar(domain.DefaultTheme().ToneFor(tone)), "--color-"), 4.5)
	}
}

// TestToneInkAndBorderColoursMatchTheCompiledCSS holds domain.TonePaletteInkColour and domain.BorderShadeColour --
// what the loader measures 3.2d's fill pairs with -- to the steps toneInkVar and borderShadeVar name in app.css.
func TestToneInkAndBorderColoursMatchTheCompiledCSS(t *testing.T) {
	css := compiledColours(t)
	near := func(a oklchColour, b domain.Colour) bool {
		return math.Abs(a.L-b.L) < 1e-9 && math.Abs(a.C-b.C) < 1e-9 && math.Abs(a.H-b.H) < 1e-9
	}
	for p := range domain.KnownTonePalettes {
		name := strings.TrimPrefix(toneInkVar(p), "--color-")
		if !near(css[name], domain.TonePaletteInkColour(p)) {
			t.Errorf("palette %q: domain %+v, %s compiles to %+v", p, domain.TonePaletteInkColour(p), name, css[name])
		}
	}
	for b := range domain.KnownBorderShades {
		name := strings.TrimPrefix(borderShadeVar(b), "--color-")
		if !near(css[name], domain.BorderShadeColour(b)) {
			t.Errorf("border %q: domain %+v, %s compiles to %+v", b, domain.BorderShadeColour(b), name, css[name])
		}
	}
}
