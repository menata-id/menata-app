package rendering

import (
	"context"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// Dark mode (rencana 3.3, owner decisions DK1–DK11, 2026-10-13) is one block in input.css that redefines every colour
// property under `prefers-color-scheme: dark`. These tests hold that block: every property has a dark value
// (TestEveryColourPropertyHasADarkValue), every pair the light gates measure is measured again in the dark
// (TestDarkColourPairsMeetContrast), and a Workspace's hue choices reach the dark block while its shade and value
// choices stay light (TestThemeCSSCarriesOnlyHueChoicesIntoDark).

// darkFollowsByAlias are light properties that need no dark value of their own, with the reason.
var darkFollowsByAlias = map[string]string{
	"--color-hover-wash": "an alias of --color-bg-base in @theme (the Design System's hover-wash), so it follows base into dark",
}

// TestEveryColourPropertyHasADarkValue: a colour property declared for light and not for dark would keep its light
// value on a dark page -- a white card, a pale badge -- and nothing else would fail. Closed both ways: a dark value
// for a property light does not declare fails too, and so does a stale alias entry.
func TestEveryColourPropertyHasADarkValue(t *testing.T) {
	// Every `--color-*` input.css declares, not only the names the readers draw through: the 3.3 review added a
	// `--color-paper` read from a Go string and this gate, scoped to colourPropertyNames, stayed green.
	lightCSS, darkCSS := inputCSSModes(t)
	light, dark := allColourDecls(lightCSS), allColourDecls(darkCSS)
	if len(light) < 40 {
		t.Fatalf("found only %d colour properties on input.css's light side -- the scan stopped matching", len(light))
	}
	compiled := compiledCSSVars(t)
	for _, k := range slices.Sorted(maps.Keys(light)) {
		if _, same := darkSameInBothModes[k]; same {
			if _, ok := dark[k]; ok {
				t.Errorf("%s is in darkSameInBothModes and also has a dark value -- drop one", k)
			}
			continue
		}
		why, alias := darkFollowsByAlias[k]
		v, ok := dark[k]
		switch {
		case !ok && !alias:
			t.Errorf("%s has a light value and no dark one -- add it to input.css's dark block (DK1–DK8), or to darkFollowsByAlias with the reason", k)
		case ok && alias:
			t.Errorf("%s is in darkFollowsByAlias (%q) and also has a dark value -- drop one", k, why)
		case ok:
			// Every palette step the dark block names must be compiled, or the property resolves to nothing.
			for _, ref := range strings.Split(v, "var(--")[1:] {
				name, _, _ := strings.Cut(ref, ")")
				if _, ok := compiled[name]; !ok {
					t.Errorf("%s's dark value %q names --%s, which app.css does not define", k, v, name)
				}
			}
		}
	}
	for k := range dark {
		if _, ok := light[k]; !ok {
			t.Errorf("input.css's dark block declares %s, which has no light value", k)
		}
	}
	for _, m := range []map[string]string{darkFollowsByAlias, darkSameInBothModes} {
		for k := range m {
			if _, ok := light[k]; !ok {
				t.Errorf("%s is excused from a dark value but input.css no longer declares it -- remove the entry", k)
			}
		}
	}
}

// darkSameInBothModes are colour properties deliberately one value in both modes, with the reason.
var darkSameInBothModes = map[string]string{
	"--color-brand": "the brand mark's own blue (the M lockup); a mark, not a surface or an ink",
}

// allColourDecls is every `--color-<name>: <value>;` in css, comments removed.
func allColourDecls(css string) map[string]string {
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(--color-[a-z0-9-]+):\s*([^;]+);`).FindAllStringSubmatch(css, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

// TestPaperLayerReadsNoThemeRole holds DK9 where the 3.3 review broke it: what is drawn over a PDF page (a signature
// marker, its label, its number) sits on a page that is white in both modes, so it may not read a Theme colour role,
// which turns light in dark mode. It reads paperInkClass and literal marker colours instead.
func TestPaperLayerReadsNoThemeRole(t *testing.T) {
	for file, fns := range map[string][]string{
		"reviewdocument.templ":     {"reviewSignatureBox"},
		"signatureplacement.templ": {"placementBoxLabel"},
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, fn := range fns {
			src := string(b)
			i := strings.Index(src, "templ "+fn+"(")
			if i < 0 {
				t.Fatalf("%s: no templ %s -- update this gate", file, fn)
			}
			body := src[i:]
			body = body[:strings.Index(body, "\n}\n")]
			for _, reader := range []string{"inkClass(", "backgroundClass(", "borderClass(", "toneInkClass(", "toneFillClass(", "onFillClass("} {
				if strings.Contains(body, reader) {
					t.Errorf("%s: %s calls %s -- a mark on the PDF page sits on white in both modes; use paperInkClass or a literal paper colour (DK9)", file, fn, reader)
				}
			}
		}
	}
}

// TestDarkColourPairsMeetContrast is GS3 for the dark block: the same thresholds (4.5:1 text, 3:1 a mark), resolved
// from app.css as the browser resolves it under `prefers-color-scheme: dark`. It covers what the light gates cover --
// every tone pair, every ink role, every accent hue, the inverse fills (the primary button, a count badge, an avatar),
// mark-good -- plus every tone palette a Workspace may remap to, since the dark block is reached by those too, and the
// categorical colours (DK8). The three surfaces must stay at least SurfaceSeparationMin apart (DK1).
//
// **Blind spots, stated:** placement (a pair passing here can sit on a third colour), hand-written literals (counted
// by TestHandWrittenColoursHaveAReason), and the PDF paper layer, which is light in both modes on purpose (DK9).
func TestDarkColourPairsMeetContrast(t *testing.T) {
	ctx := context.Background()
	css := compiledDarkColours(t)
	col := func(name string) domain.Colour {
		t.Helper()
		c, ok := css[strings.TrimPrefix(name, "--color-")]
		if !ok {
			t.Fatalf("%s does not resolve in app.css's dark mode", name)
		}
		return domain.Colour(c)
	}
	// class strips a utility to its property name: `text-ink-body` -> `ink-body`, `bg-bg-raised` -> `bg-raised`.
	class := func(c string) domain.Colour {
		t.Helper()
		resolved := c[strings.Index(c, "-")+1:]
		return col(resolved)
	}
	raised, base, sunken := class(backgroundClass(ctx, domain.BackgroundRaised)), class(backgroundClass(ctx, domain.BackgroundBase)), class(backgroundClass(ctx, domain.BackgroundSunken))
	surfaces := map[string]domain.Colour{"raised": raised, "base": base, "sunken": sunken}

	type pair struct {
		name   string
		fg, bg domain.Colour
		min    float64
	}
	var pairs []pair
	for _, ink := range slices.Sorted(maps.Keys(domain.KnownInkRoles)) {
		for _, s := range slices.Sorted(maps.Keys(surfaces)) {
			pairs = append(pairs, pair{fmt.Sprintf("ink %s on %s", ink, s), class(inkClass(ctx, ink)), surfaces[s], 4.5})
		}
	}
	hover := class(strings.TrimPrefix(accentHoverClass(ctx), "hover:"))
	for _, s := range slices.Sorted(maps.Keys(surfaces)) {
		pairs = append(pairs, pair{"accent hover on " + s, hover, surfaces[s], 4.5})
	}
	for shade := range domain.KnownInkShades {
		if !domain.IsInkHue(shade) {
			continue
		}
		for _, s := range slices.Sorted(maps.Keys(surfaces)) {
			pairs = append(pairs,
				pair{fmt.Sprintf("accent=%s on %s", shade, s), col(accentDarkVar(shade)), surfaces[s], 4.5},
				pair{fmt.Sprintf("accent=%s hover on %s", shade, s), col(accentHoverDarkVar(shade)), surfaces[s], 4.5})
		}
	}
	for tone := range domain.KnownBadgeTones {
		ink, fill := class(toneInkClass(ctx, tone)), class(toneFillClass(ctx, tone))
		pairs = append(pairs, pair{"tone " + string(tone), ink, fill, 4.5})
		if tone != domain.ToneMuted {
			// A tone's ink also stands alone (a danger link, a metric value), as fillPairIssues measures in light.
			pairs = append(pairs, pair{"tone " + string(tone) + " ink on raised", ink, raised, 4.5}, pair{"tone " + string(tone) + " ink on base", ink, base, 4.5})
		}
	}
	for p := range domain.KnownTonePalettes {
		pairs = append(pairs, pair{"palette " + string(p), col(toneInkDarkVar(p)), col(toneFillDarkVar(p)), 4.5})
	}
	strong := class(inkFillClass(ctx, domain.InkStrong))
	onFill := class(onFillClass(ctx))
	pairs = append(pairs,
		pair{"primary button (raised on strong)", onFill, strong, 4.5},
		pair{"primary hover (raised on strong/90)", onFill, strong.Over(raised, domain.PrimaryHoverAlpha), 4.5},
		pair{"count badge (raised on bad ink)", onFill, class(toneInkFillClass(ctx, domain.ToneBad)), 4.5},
		pair{"done mark (raised on good ink)", onFill, class(toneInkFillClass(ctx, domain.ToneGood)), 4.5},
		pair{"avatar (body on surface edge)", class(inkClass(ctx, domain.InkBody)), class(borderFillClass(ctx, domain.BorderSurface)), 4.5},
		pair{"mark-good on raised", class(markGoodFill()), raised, 3},
		pair{"mark-good on sunken", class(markGoodFill()), sunken, 3},
		pair{"focus ring on raised", col("blue-300"), raised, 3},
		pair{"focus ring on base", col("blue-300"), base, 3},
	)
	for _, tc := range []domain.TagColor{domain.TagBlue, domain.TagPurple, domain.TagAmber, domain.TagEmerald, domain.TagCyan, domain.TagRose, domain.TagSlate} {
		chip, _ := tagClasses(tc)
		hue := class(strings.Fields(chip)[2]) // `text-tag-<hue>`
		pairs = append(pairs, pair{"tag " + string(tc), hue, hue.Over(raised, 0.1), 4.5})
	}
	for _, c := range []string{"blue", "emerald", "amber", "slate"} {
		f := strings.Fields(appIconClasses(c))
		pairs = append(pairs, pair{"app icon " + c, class(f[1]), class(f[0]), 4.5})
	}
	f := strings.Fields(fileTypeTileClasses())
	pairs = append(pairs,
		pair{"file-type tile", class(f[1]), class(f[0]), 4.5},
		pair{"draft tile", class(draftTileInkClass()), raised, 4.5})
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })

	seen := map[string]bool{}
	for _, p := range pairs {
		r := domain.ContrastRatio(p.fg, p.bg)
		reason, excused := darkLowContrastByDesign[p.name]
		t.Logf("%-42s %5.2f:1 (min %.1f)", p.name, r, p.min)
		switch {
		case r < p.min && !excused:
			t.Errorf("dark mode: %s is %.2f:1, below %.1f:1 -- change its dark value in input.css (or the dark reader), or add it to darkLowContrastByDesign with the reason", p.name, r, p.min)
		case r >= p.min && excused:
			t.Errorf("darkLowContrastByDesign excuses %q (%q) but it measures %.2f:1 -- remove the entry", p.name, reason, r)
		}
		seen[p.name] = true
	}
	for name := range darkLowContrastByDesign {
		if !seen[name] {
			t.Errorf("darkLowContrastByDesign names %q, which this gate no longer measures -- remove it", name)
		}
	}
	for _, s := range [][2]string{{"raised", "base"}, {"base", "sunken"}, {"raised", "sunken"}} {
		if r := domain.ContrastRatio(surfaces[s[0]], surfaces[s[1]]); r < domain.SurfaceSeparationMin {
			t.Errorf("dark mode: %s and %s are %.3f:1 apart, below %.2f -- the hover wash or a selected row would not show (DK1)", s[0], s[1], r, domain.SurfaceSeparationMin)
		}
	}
}

// darkLowContrastByDesign is the dark twin of lowContrastByDesign: faint and muted are low on purpose in both modes.
var darkLowContrastByDesign = map[string]string{
	"ink faint on raised": "faint is for hints, placeholders and decoration (domain.InkFaint); 3.74:1 in dark, higher than light's 2.63",
	"ink faint on base":   "the same role on the page canvas",
	"ink faint on sunken": "the same role in a lane",
	"tone muted":          "muted means not applicable, deliberately de-emphasised",
	"palette grey-faint":  "the palette muted draws, for the same reason",
}

// TestThemeCSSCarriesOnlyHueChoicesIntoDark holds DK11. A Workspace's accent hue and tone remaps reach the dark block
// at their dark steps; its ink shades, a free accent value, background and border choices stay in the light block, so
// in dark mode the page keeps input.css's dark defaults for them.
func TestThemeCSSCarriesOnlyHueChoicesIntoDark(t *testing.T) {
	th := domain.Theme{
		Ink:        map[domain.InkRole]domain.InkShade{domain.InkAccent: domain.InkHueViolet, domain.InkBody: domain.InkShadeDarkest},
		Border:     map[domain.BorderRole]domain.BorderShade{domain.BorderSurface: domain.BorderDefined},
		Background: map[domain.BackgroundRole]domain.BackgroundShade{domain.BackgroundBase: domain.BackgroundShadeLight},
		Tone:       map[domain.BadgeTone]domain.TonePalette{domain.ToneWarn: domain.PaletteRed},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "", false)
	light, dark := themeVarOverrides(ctx), themeVarDarkOverrides(ctx)
	for k, want := range map[string]string{
		"--color-accent": "var(--color-violet-400)", "--color-accent-hover": "var(--color-violet-300)",
		"--color-tone-warn-bg": "var(--color-red-950)", "--color-tone-warn-ink": "var(--color-red-300)", "--color-tone-warn-border": "var(--color-red-800)",
	} {
		if dark[k] != want {
			t.Errorf("dark block: %s = %q, want %q", k, dark[k], want)
		}
	}
	if len(dark) != 5 {
		t.Errorf("the dark block must carry the hue choices only, got %v", dark)
	}
	for _, k := range []string{"--color-accent", "--color-ink-body", "--color-border-surface", "--color-bg-base", "--color-tone-warn-ink"} {
		if light[k] == "" {
			t.Errorf("the light block lost %s: %v", k, light)
		}
	}
	free := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkAccent: "#be123c"}}}, "", false)
	if css := themeCSS(free); strings.Contains(css, themeCSSDark) || !strings.HasPrefix(css, themeCSSLight) {
		t.Errorf("a free accent value has no dark step and must stay light only: %q", css)
	}
}

// TestDarkDefaultsMatchTheDarkReaders: input.css's dark tone and accent values are what the dark readers answer for
// the default Theme, so a Workspace that remaps a tone and then back draws the same dark badge as one that never did.
func TestDarkDefaultsMatchTheDarkReaders(t *testing.T) {
	dark, def := themeVarDarkDefaults(t), domain.DefaultTheme()
	want := map[string]string{
		"--color-accent":       "var(" + accentDarkVar(def.InkFor(domain.InkAccent)) + ")",
		"--color-accent-hover": "var(" + accentHoverDarkVar(def.InkFor(domain.InkAccent)) + ")",
	}
	for tone := range domain.KnownBadgeTones {
		p, n := def.ToneFor(tone), "--color-tone-"+string(tone)
		want[n+"-bg"], want[n+"-ink"], want[n+"-border"] = "var("+toneFillDarkVar(p)+")", "var("+toneInkDarkVar(p)+")", "var("+toneEdgeDarkVar(p)+")"
	}
	for k, v := range want {
		if dark[k] != v {
			t.Errorf("input.css's dark block says %s: %q, the dark readers say %q -- move one so they agree", k, dark[k], v)
		}
	}
}
