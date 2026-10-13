package rendering

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"menata.app/internal/domain"
)

// testdata/dtcg holds the DTCG Community Group's own JSON Schemas for 2025.10, downloaded unchanged on 2026-10-13 from
// https://www.designtokens.org/schemas/2025.10/format.json and .../resolver.json (testdata/dtcg/README.md).

func committedAppCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exportTokens(t *testing.T, ws domain.Workspace) DesignTokenFiles {
	t.Helper()
	files, err := ExportDesignTokens(ws, committedAppCSS(t))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// dtcgSchemas compiles the two vendored schemas, offline: the resolver schema refers to the format schema by URL.
func dtcgSchemas(t *testing.T) (format, resolver *jsonschema.Schema) {
	t.Helper()
	c := jsonschema.NewCompiler()
	for url, file := range map[string]string{dtcgFormatSchema: "format.json", dtcgResolverSchema: "resolver.json"} {
		b, err := os.ReadFile(filepath.Join("testdata", "dtcg", file))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.AddResource(url, doc); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	if format, err = c.Compile(dtcgFormatSchema); err != nil {
		t.Fatal(err)
	}
	if resolver, err = c.Compile(dtcgResolverSchema); err != nil {
		t.Fatal(err)
	}
	return format, resolver
}

func validateDTCG(s *jsonschema.Schema, b []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return err
	}
	return s.Validate(inst)
}

// movedThemeWorkspace moves one key of every category the export reads a colour or a length from: a free accent (whose
// hover is a color-mix), a closed ink shade, a free background, a border shade, a tone remap, a radius, a gap, a padding
// and a text size and weight.
func movedThemeWorkspace() domain.Workspace {
	th := domain.DefaultTheme()
	th.Ink[domain.InkAccent] = "#e11d48"
	th.Ink[domain.InkBody] = domain.InkShadeDarkest
	th.Background[domain.BackgroundBase] = "#f1f5f0"
	th.Border[domain.BorderSurface] = domain.BorderDefined
	th.Tone[domain.ToneBad] = domain.PaletteAmber
	th.Radius[domain.RadiusSurface] = domain.RadiusStepFull
	th.Gap[domain.GapDefault] = domain.GapAmountFive
	th.Padding[domain.PaddingPanel] = domain.PaddingAmountFour
	th.Text[domain.RoleBody] = domain.TextMedium
	th.Weight[domain.WeightEmphasis] = domain.WeightStepNormal
	return domain.Workspace{Slug: "moved", Theme: th}
}

// TestDesignTokensValidateAgainstTheDTCGSchemas is rencana 3.4's "Selesai berarti": every file the export writes passes
// the DTCG Community Group's own 2025.10 schema, for the default Theme and for a Workspace that moves every category.
// A deliberately broken copy must fail, so a validator that accepts everything cannot pass this.
func TestDesignTokensValidateAgainstTheDTCGSchemas(t *testing.T) {
	format, resolver := dtcgSchemas(t)
	for _, ws := range []domain.Workspace{{}, movedThemeWorkspace()} {
		files := exportTokens(t, ws)
		for name, b := range map[string][]byte{DesignTokensBaseFile: files.Base, DesignTokensLightFile: files.Light, DesignTokensDarkFile: files.Dark} {
			if err := validateDTCG(format, b); err != nil {
				t.Errorf("%q %s does not validate against the DTCG 2025.10 format schema: %v", ws.Slug, name, err)
			}
		}
		if err := validateDTCG(resolver, files.Resolver); err != nil {
			t.Errorf("%q %s does not validate against the DTCG 2025.10 resolver schema: %v", ws.Slug, DesignTokensResolverFile, err)
		}
	}
	// oklch lightness is 0..1 in DTCG; Tailwind writes a percentage. Writing 54.6 must fail.
	broken := strings.Replace(string(exportTokens(t, domain.Workspace{}).Light), "0.546,", "54.6,", 1)
	if !strings.Contains(broken, "54.6,") {
		t.Fatal("the negative case did not apply -- the accent's lightness is no longer 0.546")
	}
	if validateDTCG(format, []byte(broken)) == nil {
		t.Error("a colour with lightness 54.6 validated -- the schema check accepts what it must refuse")
	}
}

// TestCommittedDesignTokensAreFresh holds design-system/dtcg/default to a fresh export of the default Theme against
// the committed app.css, the way check-generated holds *_templ.go. It is what a mockup reads without running Go.
func TestCommittedDesignTokensAreFresh(t *testing.T) {
	files := exportTokens(t, domain.Workspace{})
	dir := filepath.Join("..", "..", "design-system", "dtcg", "default")
	for name, want := range map[string][]byte{DesignTokensBaseFile: files.Base, DesignTokensLightFile: files.Light,
		DesignTokensDarkFile: files.Dark, DesignTokensResolverFile: files.Resolver} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("design-system/dtcg/default/%s is not what the default Theme exports now -- run `make tokens` and commit it", name)
		}
	}
}

// TestDesignTokensMatchTheDesignSystemArtifact reads the default export back and compares every token with
// design-system/tokens.json, light and dark. GS1 compares the same readers through its own CSS reading; this one goes
// through the exporter's, so a fault in how the export resolves a value (the dark block, an alias, a unit) fails here
// even though GS1 stays green.
func TestDesignTokensMatchTheDesignSystemArtifact(t *testing.T) {
	ds := loadDesignSystemTokens(t)
	files := exportTokens(t, domain.Workspace{})
	for mode, b := range resolvedModes(t, files) {
		colours := exportedColours(t, b)
		if len(colours) < 50 {
			t.Fatalf("%s export has %d colours -- far fewer than the artifact maps", mode, len(colours))
		}
		for name, got := range colours {
			want := normalizeColour(ds.colourIn(t, name, mode))
			if normalizeColour(got) != want {
				t.Errorf("color.%s (%s): export %s, Design System %s", name, mode, normalizeColour(got), want)
			}
		}
	}
	// Hex fallbacks are converted here, not read from app.css; four are pinned to the hex Tailwind publishes for the
	// step, so a broken OKLab matrix fails beside the snapshot (the 3.4 review's surviving mutant).
	light, dark := exportedHex(t, files.Light), exportedHex(t, files.Dark)
	for name, want := range map[string]string{"accent": "#155dfc", "danger-ink": "#c10007", "ink-body": "#314158"} {
		if light[name] != want {
			t.Errorf("color.%s (light) hex %s, want Tailwind's %s", name, light[name], want)
		}
	}
	if dark["accent"] != "#51a2ff" {
		t.Errorf("color.accent (dark) hex %s, want Tailwind's blue-400 #51a2ff", dark["accent"])
	}
	var base struct {
		Spacing, Radius map[string]json.RawMessage
		Typography      map[string]json.RawMessage
	}
	if err := json.Unmarshal(resolvedBase(t, files), &base); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, group := range []map[string]json.RawMessage{base.Spacing, base.Radius} {
		for name, raw := range group {
			if strings.HasPrefix(name, "$") {
				continue
			}
			var tok struct {
				Value struct{ Value float64 } `json:"$value"`
			}
			if err := json.Unmarshal(raw, &tok); err != nil {
				t.Fatal(err)
			}
			if want := pxOf(t, ds.length(t, name)); tok.Value.Value != want {
				t.Errorf("%s: export %gpx, Design System %gpx", name, tok.Value.Value, want)
			}
			n++
		}
	}
	for name, raw := range base.Typography {
		if strings.HasPrefix(name, "$") {
			continue
		}
		var tok struct {
			Value struct {
				FontSize   struct{ Value float64 }
				FontWeight float64
				LineHeight float64
			} `json:"$value"`
		}
		if err := json.Unmarshal(raw, &tok); err != nil {
			t.Fatal(err)
		}
		st := ds.style(t, name)
		size, weight := tok.Value.FontSize.Value, tok.Value.FontWeight
		lh := round2(tok.Value.LineHeight * size)
		if wantSize := pxOf(t, st.FontSize); size != wantSize || lh != st.lineHeightPx(t, wantSize) || weight != st.FontWeight {
			t.Errorf("typography.%s: export %gpx/%gpx/%g, Design System %gpx/%gpx/%g", name, size, lh, weight, wantSize, st.lineHeightPx(t, wantSize), st.FontWeight)
		}
		n++
	}
	// The artifact's styles carry no letter spacing; the eyebrow's renderer adds `tracking-wide` (0.025em), which at
	// the eyebrow's 10px is 0.25px, and every other style draws none.
	for name, raw := range base.Typography {
		if strings.HasPrefix(name, "$") {
			continue
		}
		var tok struct {
			Value struct{ LetterSpacing struct{ Value float64 } } `json:"$value"`
		}
		if err := json.Unmarshal(raw, &tok); err != nil {
			t.Fatal(err)
		}
		want := map[string]float64{"eyebrow": 0.25}[name]
		if got := tok.Value.LetterSpacing.Value; got != want {
			t.Errorf("typography.%s: letterSpacing %gpx, want %gpx", name, got, want)
		}
	}
	if n < 25 {
		t.Fatalf("compared only %d length and type tokens", n)
	}
}

// exportedColours reads a colour document back as CSS strings in the artifact's notation: `oklch(L% C H)`, or the hex.
func exportedColours(t *testing.T, b []byte) map[string]string {
	t.Helper()
	var doc struct{ Color map[string]json.RawMessage }
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for name, raw := range doc.Color {
		if strings.HasPrefix(name, "$") {
			continue
		}
		var tok struct {
			Value struct {
				ColorSpace string
				Components []float64
				Hex        string
			} `json:"$value"`
		}
		if err := json.Unmarshal(raw, &tok); err != nil {
			t.Fatal(err)
		}
		v := tok.Value
		switch v.ColorSpace {
		case "srgb":
			// The components are what a DTCG reader takes; the hex is only a fallback. They must say the same colour.
			r, g, b := hexRGB(v.Hex)
			if len(v.Components) != 3 || v.Components[0] != round(r, 4) || v.Components[1] != round(g, 4) || v.Components[2] != round(b, 4) {
				t.Errorf("color.%s: sRGB components %v do not say its hex %s", name, v.Components, v.Hex)
			}
			out[name] = v.Hex
		case "oklch":
			out[name] = fmt.Sprintf("oklch(%s%% %s %s)", strconv.FormatFloat(round(v.Components[0]*100, 4), 'f', -1, 64),
				strconv.FormatFloat(v.Components[1], 'f', -1, 64), strconv.FormatFloat(v.Components[2], 'f', -1, 64))
		default:
			t.Fatalf("color.%s: colour space %q", name, v.ColorSpace)
		}
	}
	return out
}

// TestDesignTokensFollowAMovedTheme is R9 for the export: under a Workspace that moves every category, each moved token
// says the moved value, in the mode the move reaches (DK11: a free value and a shade are light-only, a hue choice
// carries into dark), and nothing else moves. The default Theme cannot show any of it.
func TestDesignTokensFollowAMovedTheme(t *testing.T) {
	def, moved := exportTokens(t, domain.Workspace{}), exportTokens(t, movedThemeWorkspace())
	dl, dd := exportedColours(t, def.Light), exportedColours(t, def.Dark)
	ml, md := exportedColours(t, moved.Light), exportedColours(t, moved.Dark)

	wantLight := map[string]string{
		"accent": "#e11d48", "ink-body": dl["ink-strong"], "bg-base": "#f1f5f0", "hover-wash": "#f1f5f0",
		"border-surface": dl["border-control"], "avatar-fill": dl["border-control"],
		"tone-bad-bg": dl["tone-warn-bg"], "tone-bad-ink": dl["tone-warn-ink"], "danger-ink": dl["tone-warn-ink"],
		"danger-wash": dl["tone-warn-bg"], "danger-border": dl["tone-warn-border"],
	}
	wantDark := map[string]string{ // a tone remap is a hue choice, so it carries into dark; the rest stay light-only
		"tone-bad-bg": dd["tone-warn-bg"], "tone-bad-ink": dd["tone-warn-ink"], "danger-ink": dd["tone-warn-ink"],
		"danger-wash": dd["tone-warn-bg"], "danger-border": dd["tone-warn-border"],
	}
	for _, c := range []struct {
		mode               string
		got, def, expected map[string]string
	}{{"light", ml, dl, wantLight}, {"dark", md, dd, wantDark}} {
		for name, got := range c.got {
			want, moves := c.expected[name]
			if name == "accent-hover" && c.mode == "light" {
				continue // checked below: a color-mix has no literal to expect
			}
			if !moves {
				want = c.def[name]
			}
			if got != want {
				t.Errorf("color.%s (%s) under the moved theme: %s, want %s", name, c.mode, got, want)
			}
		}
	}
	// accent-hover is color-mix(in oklab, #e11d48, black 15%): the accent's hue at 85% of its OKLab lightness and chroma.
	r, g, b := hexRGB("#e11d48")
	l, ch, h := srgbToOKLCH(r, g, b)
	hover := accentHoverOf(t, moved.Light)
	if math.Abs(hover[0]-0.85*l) > 1e-3 || math.Abs(hover[1]-0.85*ch) > 1e-3 || math.Abs(hover[2]-h) > 0.05 {
		t.Errorf("accent-hover under a free accent: oklch %v, want about (%.4f %.4f %.3f)", hover, 0.85*l, 0.85*ch, h)
	}

	var base struct{ Spacing, Radius, Typography map[string]json.RawMessage }
	if err := json.Unmarshal(moved.Base, &base); err != nil {
		t.Fatal(err)
	}
	for group, want := range map[string]string{
		"radius/radius-surface": `"value":9999`, "spacing/gap-default": `"value":20`, "spacing/pad-panel": `"value":16`,
		"typography/body": `"fontSize":{"unit":"px","value":16}`, "typography/heading": `"fontWeight":400`,
	} {
		g, name, _ := strings.Cut(group, "/")
		raw := map[string]map[string]json.RawMessage{"radius": base.Radius, "spacing": base.Spacing, "typography": base.Typography}[g][name]
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(compact.String(), want) {
			t.Errorf("%s under the moved theme: %s, want it to hold %s", group, compact.String(), want)
		}
	}
}

func accentHoverOf(t *testing.T, light []byte) []float64 {
	t.Helper()
	var doc struct{ Color map[string]json.RawMessage }
	if err := json.Unmarshal(light, &doc); err != nil {
		t.Fatal(err)
	}
	var tok struct {
		Value struct{ Components []float64 } `json:"$value"`
	}
	if err := json.Unmarshal(doc.Color["accent-hover"], &tok); err != nil {
		t.Fatal(err)
	}
	return tok.Value.Components
}

// resolvedModes follows menata.resolver.json to the colour document each `mode` context names, so a resolver that
// pointed light at the dark file fails here and not only in the snapshot (the 3.4 review).
func resolvedModes(t *testing.T, files DesignTokenFiles) map[string][]byte {
	t.Helper()
	byRef := map[string][]byte{DesignTokensBaseFile: files.Base, DesignTokensLightFile: files.Light, DesignTokensDarkFile: files.Dark}
	var r struct {
		Modifiers map[string]struct {
			Contexts map[string][]struct {
				Ref string `json:"$ref"`
			}
			Default string
		}
		ResolutionOrder []struct {
			Ref string `json:"$ref"`
		}
	}
	if err := json.Unmarshal(files.Resolver, &r); err != nil {
		t.Fatal(err)
	}
	mode, ok := r.Modifiers["mode"]
	if !ok || mode.Default != "light" || len(r.ResolutionOrder) != 2 || r.ResolutionOrder[1].Ref != "#/modifiers/mode" {
		t.Fatalf("the resolver does not apply a `mode` modifier (default light) after the base set: %s", files.Resolver)
	}
	out := map[string][]byte{}
	for ctx, refs := range mode.Contexts {
		if len(refs) != 1 || byRef[refs[0].Ref] == nil {
			t.Fatalf("mode context %q names %v, not one exported file", ctx, refs)
		}
		out[ctx] = byRef[refs[0].Ref]
	}
	if len(out) != 2 || out["light"] == nil || out["dark"] == nil {
		t.Fatalf("mode contexts %v, want light and dark", mode.Contexts)
	}
	return out
}

// resolvedBase is the one document the resolver's `base` set names.
func resolvedBase(t *testing.T, files DesignTokenFiles) []byte {
	t.Helper()
	var r struct {
		Sets map[string]struct {
			Sources []struct {
				Ref string `json:"$ref"`
			}
		}
		ResolutionOrder []struct {
			Ref string `json:"$ref"`
		}
	}
	if err := json.Unmarshal(files.Resolver, &r); err != nil {
		t.Fatal(err)
	}
	src := r.Sets["base"].Sources
	if len(src) != 1 || src[0].Ref != DesignTokensBaseFile || len(r.ResolutionOrder) == 0 || r.ResolutionOrder[0].Ref != "#/sets/base" {
		t.Fatalf("the resolver's first step is not the base set naming %s: %s", DesignTokensBaseFile, files.Resolver)
	}
	return files.Base
}

func exportedHex(t *testing.T, b []byte) map[string]string {
	t.Helper()
	var doc struct{ Color map[string]json.RawMessage }
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for name, raw := range doc.Color {
		var tok struct {
			Value struct{ Hex string } `json:"$value"`
		}
		if json.Unmarshal(raw, &tok) == nil {
			out[name] = tok.Value.Hex
		}
	}
	return out
}
