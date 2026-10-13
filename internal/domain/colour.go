package domain

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// A **colour value** is what a Theme may give a colour role instead of choosing one of its closed shades (owner
// decision DS-D15 stage 2, 2026-10-12): a Workspace's brand colour. It is a value, not a Tailwind class, so 007
// §15.2 still holds: the logical plane carries `#e11d48`, and only `internal/rendering` knows how a role reaches CSS
// (a custom property, `--color-ink-body`, overridden in the page's `<head>`).
//
// Exactly `#rrggbb`, lowercase after load. Nothing looser: the value is written into a `<style>` block, so the
// pattern is also what keeps a manifest from writing CSS.
var colourValuePattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// IsColourValue reports whether s is a colour value (`#rrggbb`, lowercase) rather than a named shade.
func IsColourValue(s string) bool { return colourValuePattern.MatchString(s) }

// NormalizeColourValue lowercases a `#RRGGBB` the loader accepts; any other string is returned unchanged.
func NormalizeColourValue(s string) string {
	if len(s) == 7 && s[0] == '#' {
		return strings.ToLower(s)
	}
	return s
}

// Colour is a colour in OKLCH, the space Tailwind v4 and the Design System artifact both write their palette in.
type Colour struct{ L, C, H float64 }

// ParseColourValue converts a colour value to OKLCH. ok is false for anything IsColourValue refuses.
func ParseColourValue(s string) (Colour, bool) {
	if !IsColourValue(s) {
		return Colour{}, false
	}
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(s[i:i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r, g, b := ch(1), ch(3), ch(5)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sx := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*l + 0.7936177850*m - 0.0040720468*sx
	A := 1.9779984951*l - 2.4285922050*m + 0.4505937099*sx
	B := 0.0259040371*l + 0.7827717662*m - 0.8086757660*sx
	h := math.Atan2(B, A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return Colour{L: L, C: math.Hypot(A, B), H: h}, true
}

// srgb is the colour in gamma-encoded sRGB, each channel clamped to 0..1.
func (c Colour) srgb() (float64, float64, float64) {
	a := c.C * math.Cos(c.H*math.Pi/180)
	b := c.C * math.Sin(c.H*math.Pi/180)
	l := math.Pow(c.L+0.3963377774*a+0.2158037573*b, 3)
	m := math.Pow(c.L-0.1055613458*a-0.0638541728*b, 3)
	s := math.Pow(c.L-0.0894841775*a-1.2914855480*b, 3)
	enc := func(v float64) float64 {
		v = math.Min(1, math.Max(0, v))
		if v <= 0.0031308 {
			return 12.92 * v
		}
		return 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return enc(4.0767416621*l - 3.3077115913*m + 0.2309699292*s), enc(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s),
		enc(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s)
}

// Over is c drawn at opacity alpha over the colour beneath, composited the way a browser composites `bg-x/90`: in
// gamma-encoded sRGB.
func (c Colour) Over(beneath Colour, alpha float64) Colour {
	r1, g1, b1 := c.srgb()
	r2, g2, b2 := beneath.srgb()
	mix := func(x, y float64) uint64 { return uint64(math.Round((alpha*x + (1-alpha)*y) * 255)) }
	out, _ := ParseColourValue(fmt.Sprintf("#%02x%02x%02x", mix(r1, r2), mix(g1, g2), mix(b1, b2)))
	return out
}

// Luminance is WCAG 2.x relative luminance, from OKLCH through linear sRGB (clamped to the sRGB gamut).
func (c Colour) Luminance() float64 {
	a := c.C * math.Cos(c.H*math.Pi/180)
	b := c.C * math.Sin(c.H*math.Pi/180)
	l := math.Pow(c.L+0.3963377774*a+0.2158037573*b, 3)
	m := math.Pow(c.L-0.1055613458*a-0.0638541728*b, 3)
	s := math.Pow(c.L-0.0894841775*a-1.2914855480*b, 3)
	clamp := func(v float64) float64 { return math.Min(1, math.Max(0, v)) }
	r := clamp(4.0767416621*l - 3.3077115913*m + 0.2309699292*s)
	g := clamp(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s)
	bl := clamp(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s)
	return 0.2126*r + 0.7152*g + 0.0722*bl
}

// ContrastRatio is WCAG 2.x's (L1+0.05)/(L2+0.05).
func ContrastRatio(a, b Colour) float64 {
	la, lb := a.Luminance(), b.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TextContrastMin is WCAG 1.4.3's floor for text that is not large; the runtime's text is 10–16px, never large.
const TextContrastMin = 4.5

// InkShadeColour is the colour each closed ink shade renders (the default step of each grey and hue), so the loader
// can measure a closed shade on a moved background as well as a colour value. Held to `app.css` by
// rendering.TestInkShadeColoursMatchTheCompiledCSS. ok is false for a colour value or an unknown shade.
func InkShadeColour(s InkShade) (Colour, bool) {
	c, ok := map[InkShade]Colour{
		InkShadeDarkest: {0.208, 0.042, 265.755}, InkShadeDark: {0.372, 0.044, 257.287},
		InkShadeMedium: {0.554, 0.046, 257.417}, InkShadeLight: {0.704, 0.04, 256.788},
		InkHueBlue: {0.546, 0.245, 262.881}, InkHueIndigo: {0.511, 0.262, 276.966}, InkHueViolet: {0.541, 0.281, 293.009},
		InkHueEmerald: {0.508, 0.118, 165.612}, InkHueRose: {0.514, 0.222, 16.935}, InkHueCyan: {0.52, 0.105, 223.128},
	}[s]
	return c, ok
}

// InkColour is what an ink shade draws: a colour value parsed, or a closed shade's colour.
func InkColour(s InkShade) (Colour, bool) {
	if c, ok := ParseColourValue(string(s)); ok {
		return c, true
	}
	return InkShadeColour(s)
}

// InkSurfaces is where an ink role is held to TextContrastMin, at load, for every Theme that moves ink or background
// (DS-D15 stage 2, condition 2). `strong`, `body` and `accent` are drawn on all three surfaces (the Application
// Settings current link is strong ink on `sunken`, a board lane is sunken). `secondary` is held on raised and base
// only: its default (slate-500) is 4.35:1 on the default `sunken`, so requiring it there would refuse the default
// Theme -- a measured gap, recorded rather than hidden. `faint` is low contrast by design and held nowhere.
func InkSurfaces(role InkRole) []BackgroundRole {
	switch role {
	case InkFaint:
		return nil
	case InkSecondary:
		return []BackgroundRole{BackgroundRaised, BackgroundBase}
	default:
		return []BackgroundRole{BackgroundRaised, BackgroundBase, BackgroundSunken}
	}
}

// SurfaceLuminanceFloor is how dark a surface colour value may be: no darker than the darkest closed shade a surface
// role may already take (`light`, slate-100). WCAG contrast depends on luminance alone, so every fixed colour drawn on
// a surface -- a hand-written `text-slate-900`, a tone's ink alone, a field's `bg-white` box, mark-good -- reads at
// least as well on any accepted value as it does on that shade, which the gates measure. A value may tint a surface; it
// may not darken one. Dark surfaces are dark mode's job (rencana 3.3), with their own inks. The 2026-10-13 review of
// 3.2c loaded a navy theme with white inks, and every field's typed text became white on white.
func SurfaceLuminanceFloor() float64 { return BackgroundShadeColour(BackgroundShadeLight).Luminance() }

// SurfaceSeparationMin is how far apart `base` must stay from `raised` and `sunken` (as a contrast ratio): base is the
// page behind every surface and the hover wash (DS-D11), so too close and a hover draws nothing or looks selected. The
// closest default pair, slate-50 against slate-100, is 1.04:1; 1.02 refuses a value that is the same colour in all
// but name.
const SurfaceSeparationMin = 1.02

// TonePaletteInkColour is the colour of each tone palette's ink, the half that is drawn alone (danger text) and as a
// solid fill under raised text (a count badge, CP3). Held to app.css by rendering.TestToneInkColoursMatchTheCompiledCSS.
func TonePaletteInkColour(p TonePalette) Colour {
	switch p {
	case PaletteGreyFaint:
		return Colour{L: 0.704, C: 0.04, H: 256.788}
	case PaletteBlue:
		return Colour{L: 0.488, C: 0.243, H: 264.376}
	case PaletteGreen:
		return Colour{L: 0.508, C: 0.118, H: 165.612}
	case PaletteRed:
		return Colour{L: 0.505, C: 0.213, H: 27.518}
	case PaletteAmber:
		return Colour{L: 0.473, C: 0.137, H: 46.201}
	default:
		return Colour{L: 0.446, C: 0.043, H: 257.281}
	}
}

// BorderShadeColour is the colour of each closed border shade, for text drawn on an edge role used as a fill (an
// avatar's initials on `border.surface`, CP2). Held to app.css by rendering.TestBorderShadeColoursMatchTheCompiledCSS.
func BorderShadeColour(s BorderShade) Colour {
	switch s {
	case BorderFaint:
		return Colour{L: 0.968, C: 0.007, H: 247.896}
	case BorderDefined:
		return Colour{L: 0.869, C: 0.022, H: 252.894}
	default:
		return Colour{L: 0.929, C: 0.013, H: 255.508}
	}
}

// PrimaryHoverAlpha is the primary fill's hover opacity (`hover:bg-ink-strong/90`, CP1).
const PrimaryHoverAlpha = 0.9

// BackgroundShadeColour is the colour each closed background shade renders, or a colour value parsed, so the loader can
// measure every ink against the Workspace's own surfaces. These are colour values, not Tailwind names (§15.2);
// rendering.TestBackgroundShadeColoursMatchTheCompiledCSS holds them to what `app.css` really draws.
func BackgroundShadeColour(s BackgroundShade) Colour {
	if c, ok := ParseColourValue(string(s)); ok {
		return c
	}
	switch s {
	case BackgroundShadeLighter:
		return Colour{L: 0.984, C: 0.003, H: 247.858}
	case BackgroundShadeLight:
		return Colour{L: 0.968, C: 0.007, H: 247.896}
	default:
		return Colour{L: 1}
	}
}
