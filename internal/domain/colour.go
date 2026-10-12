package domain

import (
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

// BackgroundShadeColour is the colour each closed background shade renders, so the loader can measure a free ink
// value against the Workspace's own surfaces. These are colour values, not Tailwind names (§15.2);
// rendering.TestBackgroundShadeColoursMatchTheCompiledCSS holds them to what `app.css` really draws.
func BackgroundShadeColour(s BackgroundShade) Colour {
	switch s {
	case BackgroundShadeLighter:
		return Colour{L: 0.984, C: 0.003, H: 247.858}
	case BackgroundShadeLight:
		return Colour{L: 0.968, C: 0.007, H: 247.896}
	default:
		return Colour{L: 1}
	}
}
