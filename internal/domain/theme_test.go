package domain

import "testing"

// TestDefaultThemeResolvesEveryDeclaredTone holds the property `ToneFor`'s fallback depends on: DefaultTheme
// names a known palette for **every** BadgeTone. A tone it omits would fall back to "" and render with no
// colour at all -- a silent cacat, which is the reason the fallback goes to DefaultTheme rather than to a
// zero value in the first place.
func TestDefaultThemeResolvesEveryDeclaredTone(t *testing.T) {
	def := DefaultTheme()
	for tone := range KnownBadgeTones {
		p, ok := def.Tone[tone]
		if !ok {
			t.Errorf("DefaultTheme().Tone has no entry for %q", tone)
			continue
		}
		if !KnownTonePalettes[p] {
			t.Errorf("DefaultTheme().Tone[%q] = %q, not a declared palette", tone, p)
		}
	}
	if len(def.Tone) != len(KnownBadgeTones) {
		t.Errorf("DefaultTheme().Tone holds %d entries and KnownBadgeTones declares %d", len(def.Tone), len(KnownBadgeTones))
	}
}

func TestThemeToneFor(t *testing.T) {
	// The zero Theme is what a pre-auth screen has on ctx: it must draw exactly the default.
	if got := (Theme{}).ToneFor(ToneWarn); got != PaletteAmber {
		t.Errorf("zero Theme: ToneFor(warn) = %q, want %q", got, PaletteAmber)
	}
	// A declared entry wins, and only for the tone it names.
	th := Theme{Tone: map[BadgeTone]TonePalette{ToneWarn: PaletteRed}}
	if got := th.ToneFor(ToneWarn); got != PaletteRed {
		t.Errorf("declared warn=red: got %q", got)
	}
	if got := th.ToneFor(ToneBad); got != PaletteRed {
		t.Errorf("an undeclared tone must inherit its default (bad -> red): got %q", got)
	}
	if got := th.ToneFor(ToneGood); got != PaletteGreen {
		t.Errorf("an undeclared tone must inherit its default (good -> green): got %q", got)
	}
	// An unknown palette never reaches the renderer as "": it falls back, the same as RadiusFor.
	bad := Theme{Tone: map[BadgeTone]TonePalette{ToneWarn: "chartreuse"}}
	if got := bad.ToneFor(ToneWarn); got != PaletteAmber {
		t.Errorf("an unknown palette should fall back to the default: got %q", got)
	}
}
