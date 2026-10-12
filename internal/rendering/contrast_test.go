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
			pairs = append(pairs, pair{fmt.Sprintf("ink %s on %s", ink, bg), inkClass(ctx, ink), backgroundClass(ctx, bg), 4.5})
		}
	}
	// Every accent hue a Workspace may pick (DS-D10), not only the default: a hue that fails here would be a
	// declarable `theme.ink.accent` that makes every link unreadable. Measured on the default backgrounds.
	for shade := range domain.KnownInkShades {
		if !domain.IsInkHue(shade) || shade == domain.DefaultTheme().InkFor(domain.InkAccent) {
			continue
		}
		hued := WithCurrentWorkspace(ctx, domain.Workspace{Theme: domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkAccent: shade}}}, "", false)
		for _, bg := range []domain.BackgroundRole{domain.BackgroundRaised, domain.BackgroundBase} {
			pairs = append(pairs, pair{fmt.Sprintf("ink accent=%s on %s", shade, bg), inkClass(hued, domain.InkAccent), backgroundClass(ctx, bg), 4.5})
		}
	}
	pairs = append(pairs,
		pair{"mark-good on raised", markGoodFill(), backgroundClass(ctx, domain.BackgroundRaised), 3},
		pair{"mark-good on sunken (a progress track)", markGoodFill(), backgroundClass(ctx, domain.BackgroundSunken), 3},
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
	out := compiledCSS{}
	re := regexp.MustCompile(`--color-([a-z]+-[0-9]+):oklch\(([0-9.]+)% ([0-9.]+) ([0-9.]+)\)`)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		l, _ := strconv.ParseFloat(m[2], 64)
		c, _ := strconv.ParseFloat(m[3], 64)
		h, _ := strconv.ParseFloat(m[4], 64)
		out[m[1]] = oklchColour{L: l / 100, C: c, H: h}
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
