package domain

import (
	"math"
	"testing"
)

func TestColourValuesAreExactlyLowercaseHex(t *testing.T) {
	for s, want := range map[string]bool{
		"#e11d48": true, "#000000": true, "#E11D48": false, "#e11d4": false, "e11d48": false, "#e11d488": false,
		"#e11d48;}": false, "red": false, "": false, "#ggg000": false,
	} {
		if IsColourValue(s) != want {
			t.Errorf("IsColourValue(%q) = %v, want %v", s, !want, want)
		}
	}
	if NormalizeColourValue("#E11D48") != "#e11d48" || NormalizeColourValue("darkest") != "darkest" {
		t.Error("NormalizeColourValue must lowercase a #RRGGBB and leave a shade name alone")
	}
}

// The reference pairs are WCAG's own well-known values: black on white is 21:1, and #767676 is the lightest grey that
// passes 4.5:1 on white (4.54:1).
func TestContrastRatioMatchesKnownPairs(t *testing.T) {
	c := func(s string) Colour {
		v, ok := ParseColourValue(s)
		if !ok {
			t.Fatalf("ParseColourValue(%q) refused", s)
		}
		return v
	}
	for _, tc := range []struct {
		a, b string
		want float64
	}{{"#000000", "#ffffff", 21}, {"#767676", "#ffffff", 4.54}, {"#777777", "#ffffff", 4.48}, {"#ffffff", "#ffffff", 1}} {
		if got := ContrastRatio(c(tc.a), c(tc.b)); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("%s on %s = %.3f:1, want %.2f:1", tc.a, tc.b, got, tc.want)
		}
	}
	if got := ContrastRatio(c("#ffffff"), BackgroundShadeColour(BackgroundShadeLightest)); math.Abs(got-1) > 1e-3 {
		t.Errorf("#ffffff against the lightest shade = %.4f:1, want 1", got)
	}
}

// Chromatic references, because every grey has r = g = b and a channel swap passes them all (the 2026-10-12 review
// swapped G and B and the suite stayed green). The OKLCH of the sRGB primaries is Björn Ottosson's published value;
// the ratio pairs are WebAIM's.
func TestParseColourValueOnChromaticReferences(t *testing.T) {
	for _, tc := range []struct {
		hex     string
		l, c, h float64
	}{{"#ff0000", 0.62796, 0.25768, 29.234}, {"#00ff00", 0.86644, 0.29483, 142.495}, {"#0000ff", 0.45201, 0.31321, 264.052}} {
		got, _ := ParseColourValue(tc.hex)
		if math.Abs(got.L-tc.l) > 1e-3 || math.Abs(got.C-tc.c) > 1e-3 || math.Abs(got.H-tc.h) > 0.05 {
			t.Errorf("%s = %+v, want oklch(%.5f %.5f %.3f)", tc.hex, got, tc.l, tc.c, tc.h)
		}
	}
	white, _ := ParseColourValue("#ffffff")
	for hex, want := range map[string]float64{"#2563eb": 5.17, "#16a34a": 3.30, "#dc2626": 4.83, "#1d4ed8": 6.70} {
		c, _ := ParseColourValue(hex)
		if got := ContrastRatio(c, white); math.Abs(got-want) > 0.01 {
			t.Errorf("%s on white = %.3f:1, want %.2f:1", hex, got, want)
		}
	}
}

func TestInkShadeFitsAColourValueOnEveryRoleButFaint(t *testing.T) {
	for role := range KnownInkRoles {
		if got, want := InkShadeFits(role, "#1f2937"), role != InkFaint; got != want {
			t.Errorf("InkShadeFits(%s, #1f2937) = %v, want %v", role, got, want)
		}
	}
	if InkShadeFits(InkBody, "#1F2937") {
		t.Error("an uppercase value must not fit: the loader lowercases it, so one that reaches InkFor unnormalised is a Theme built without the loader")
	}
}

// Over composites in gamma-encoded sRGB, as a browser does for `bg-x/90`: black at 90% over white is #1a1a1a.
func TestOverCompositesInSRGB(t *testing.T) {
	black, _ := ParseColourValue("#000000")
	white, _ := ParseColourValue("#ffffff")
	want, _ := ParseColourValue("#1a1a1a")
	// Within one sRGB code (#191919 or #1a1a1a): white round-trips through OKLCH as 0.99999, and 25.5 rounds down.
	if got := black.Over(white, 0.9); ContrastRatio(got, want) > 1.02 {
		t.Errorf("black at 90%% over white = %+v, want #1a1a1a", got)
	}
	if got := white.Over(white, 0.5); ContrastRatio(got, white) > 1.0001 {
		t.Errorf("white over white must stay white, got %+v", got)
	}
}
