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
