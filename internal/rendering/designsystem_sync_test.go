package rendering

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestDesignSystemTokensMatchTheCode is GS1 of menata-app-document's guides/menjaga-design-system.md. It holds the
// artifact and the code to the same values, the kind of drift that guide records three times (L3, L4, L6): an
// artifact built from an old commit, 88 hand-converted copies, and a manual oklch→hex read that named the wrong
// colour.
//
// `design-system/tokens.json` is a copy of the Design System artifact's file. For every token and type style it asks
// the default Theme's own reader for its class, resolves that class to the value Tailwind compiled into the
// committed `static/css/app.css`, and compares the two in the artifact's notation:
//   - colours as `oklch(...)` strings;
//   - lengths in px;
//   - type as size, line height and weight.
//
// **Every token is classified, or the test fails.** It is either checked (in a mapping below), accepted by the
// artifact but not yet in code (`dsAcceptedNotInCode`, which only shrinks), or deliberately unmapped
// (`dsUnmapped`, with the reason no Theme reader answers for it). A token the artifact adds later fails until someone
// decides which. An entry naming a token the artifact no longer has fails too.
//
// **Blind spots, stated:**
//   - Only the default Theme is checked.
//   - An unmapped token is not compared at all; its reason is the only record.
//   - The copy can itself be stale against the live artifact. The README beside it records the version, and R8 says
//     the copy moves in the same commit as the code.
func TestDesignSystemTokensMatchTheCode(t *testing.T) {
	ctx := context.Background()
	ds := loadDesignSystemTokens(t)
	css := compiledCSSVars(t)
	cssDark := compiledCSSVarsDark(t)

	// The token-to-reader list is designTokenClasses, which ExportDesignTokens (rencana 3.4) reads too.
	m := designTokenClasses(ctx)
	colour, length, styles := m.colour, m.length, m.styles

	classified := map[string]string{}
	mark := func(name, how string) {
		if prev, dup := classified[name]; dup {
			t.Errorf("%s is classified twice (%s and %s) -- one place per token", name, prev, how)
		}
		classified[name] = how
	}
	for n := range colour {
		mark(n, "colour")
	}
	for n := range length {
		mark(n, "length")
	}
	for n := range styles {
		mark(n, "type")
	}
	for n := range dsAcceptedNotInCode {
		mark(n, "accepted-not-in-code")
	}
	for n := range dsUnmapped {
		mark(n, "unmapped")
	}

	for _, name := range ds.names() {
		how, ok := classified[name]
		if !ok {
			t.Errorf("the Design System has %q and nothing classifies it -- check it against a reader here, or add it to dsAcceptedNotInCode / dsUnmapped with the reason", name)
			continue
		}
		switch how {
		case "colour":
			want := normalizeColour(ds.colour(t, name))
			got := normalizeColour(css.colourOf(t, colour[name]))
			if want != got {
				t.Errorf("%s: Design System %s, code %s (%s) -- the artifact and the code disagree; decide which is right and move the other (R8)", name, want, got, colour[name])
			}
			// Dark mode (2026-10-13): the artifact's `dark` theme against input.css's dark block. A token with no dark
			// value inherits its light one, as the artifact reads it.
			wantDark := normalizeColour(ds.colourIn(t, name, "dark"))
			gotDark := normalizeColour(cssDark.colourOf(t, colour[name]))
			if wantDark != gotDark {
				t.Errorf("%s (dark): Design System %s, code %s (%s) -- the artifact's dark theme and input.css's dark block disagree (R8)", name, wantDark, gotDark, colour[name])
			}
		case "length":
			want := pxOf(t, ds.length(t, name))
			got := css.lengthOf(t, length[name])
			if want != got {
				t.Errorf("%s: Design System %gpx, code %gpx (%s) -- the artifact and the code disagree; decide which is right and move the other (R8)", name, want, got, length[name])
			}
		case "type":
			st := ds.style(t, name)
			size, lh := css.textOf(t, styles[name].size)
			weight := map[string]float64{"font-normal": 400, "font-medium": 500}[styles[name].weight]
			wantSize := pxOf(t, st.FontSize)
			wantLH := st.lineHeightPx(t, wantSize)
			if size != wantSize || lh != wantLH || weight != st.FontWeight {
				t.Errorf("%s: Design System %gpx/%gpx/%g, code %gpx/%gpx/%g (%s %s) -- the artifact and the code disagree (R8)",
					name, wantSize, wantLH, st.FontWeight, size, lh, weight, styles[name].size, styles[name].weight)
			}
		}
	}
	have := map[string]bool{}
	for _, n := range ds.names() {
		have[n] = true
	}
	for _, n := range sortedStrings(classified) {
		if !have[n] {
			t.Errorf("%s is classified here (%s) but the Design System copy no longer has it -- remove it", n, classified[n])
		}
	}
	for n := range dsAcceptedNotInCode {
		if ds.status(n) != "accepted" {
			t.Errorf("dsAcceptedNotInCode lists %s, which the Design System does not mark accepted", n)
		}
	}
}

// dsAcceptedNotInCode: accepted by the artifact, not yet in code. Shrinks as each lands (rencana 0.7). Empty since
// 2026-10-12 (control-height-compact was the last); an empty map stays declared, like any emptied ratchet.
var dsAcceptedNotInCode = map[string]string{}

// dsUnmapped: tokens no Theme reader answers for, with the reason. Each is a record, not a check.
var dsUnmapped = map[string]string{
	"brand": "--color-brand in input.css; the M mark only, not a Theme role", "brand-icon": "the PWA PNGs' fill, not CSS",
	"accent-ring": "the stepper's current-step ring, literal", "focus-ring": "not a reader: a base :focus-visible rule in input.css (DS-D14) and the field focus utilities, both blue-300 (TestKeyboardFocusRuleCoversInteractiveElements)",
	"primary-hover": "inkFillHoverClass: strong at 90% over raised (CP1), a mix no single value spells; the loader measures it",
	"scrim":         "scrimClass: --color-scrim is a color-mix (slate-900 40%, black 60% in dark, DK10), the artifact writes rgba",
	"pad-metric":    "metric's literal p-3", "pad-control-x": "buttonClasses' literal px-4",
	"control-height": "the literal h-9 in buttonClasses/fieldClasses; no height role", "brand-mark-size": "auth shell literal",
	"radius-brand-mark": "auth shell one-off", "radius-sheet": "the mobile sheet's literal rounded-t-2xl",
	"shadow-card-auth": "shadow is not a Theme category (D0a)", "shadow-hover": "as above", "shadow-popover": "as above",
	"shadow-sheet": "as above", "brand-glyph": "pre-auth, out of Theme's reach",
	"auth-heading": "pre-auth literal text-[21px]", "auth-subtitle": "pre-auth literal text-[15px]",
}

type dsTokens struct {
	Color   struct{ Tokens []dsToken } `json:"color"`
	Spacing struct{ Tokens []dsToken } `json:"spacing"`
	Radius  struct{ Tokens []dsToken } `json:"radius"`
	Shadow  struct{ Tokens []dsToken } `json:"shadow"`
	Type    struct {
		Groups []struct{ Styles []dsStyle } `json:"groups"`
	} `json:"type"`
}

// dsToken's Value is a string, or since dark mode a map of theme id to string ({"light": ..., "dark": ...}).
type dsToken struct {
	Name   string          `json:"name"`
	Value  json.RawMessage `json:"value"`
	Status string          `json:"status"`
}

type dsStyle struct {
	Name       string          `json:"name"`
	FontSize   string          `json:"fontSize"`
	LineHeight json.RawMessage `json:"lineHeight"`
	FontWeight float64         `json:"fontWeight"`
}

func loadDesignSystemTokens(t *testing.T) dsTokens {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "design-system", "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var d dsTokens
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Color.Tokens) == 0 || len(d.Type.Groups) == 0 {
		t.Fatal("design-system/tokens.json parsed to nothing -- the artifact's shape changed")
	}
	return d
}

func (d dsTokens) all() []dsToken {
	var out []dsToken
	out = append(out, d.Color.Tokens...)
	out = append(out, d.Spacing.Tokens...)
	out = append(out, d.Radius.Tokens...)
	return append(out, d.Shadow.Tokens...)
}

func (d dsTokens) names() []string {
	var out []string
	for _, tk := range d.all() {
		out = append(out, tk.Name)
	}
	for _, g := range d.Type.Groups {
		for _, s := range g.Styles {
			out = append(out, s.Name)
		}
	}
	sort.Strings(out)
	return out
}

func (d dsTokens) raw(name string) (string, bool) {
	return d.rawIn(name, "light")
}

// rawIn is a token's value in one theme. A plain string is the first theme's (light) and every other theme inherits
// it; a theme missing from a map inherits light too -- the artifact's own reading (format.md, "a missing one inherits
// the first").
func (d dsTokens) rawIn(name, theme string) (string, bool) {
	for _, tk := range d.all() {
		if tk.Name == name {
			var s string
			if json.Unmarshal(tk.Value, &s) == nil {
				return s, true
			}
			var m map[string]string
			if json.Unmarshal(tk.Value, &m) == nil {
				if v, ok := m[theme]; ok {
					return v, true
				}
				v, ok := m["light"]
				return v, ok
			}
		}
	}
	return "", false
}

func (d dsTokens) status(name string) string {
	for _, tk := range d.all() {
		if tk.Name == name {
			return tk.Status
		}
	}
	return ""
}

// colour resolves a `{alias}` chain the way the artifact does, in the light theme.
func (d dsTokens) colour(t *testing.T, name string) string {
	return d.colourIn(t, name, "light")
}

// colourIn resolves the chain in one theme: an alias is followed in the same theme.
func (d dsTokens) colourIn(t *testing.T, name, theme string) string {
	for i := 0; i < 16; i++ {
		v, ok := d.rawIn(name, theme)
		if !ok {
			t.Fatalf("Design System token %s has no string value", name)
		}
		if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
			name = v[1 : len(v)-1]
			continue
		}
		return v
	}
	t.Fatalf("alias chain from %s is too deep", name)
	return ""
}

func (d dsTokens) length(t *testing.T, name string) string {
	v, ok := d.raw(name)
	if !ok {
		t.Fatalf("Design System token %s has no string value", name)
	}
	return v
}

func (d dsTokens) style(t *testing.T, name string) dsStyle {
	for _, g := range d.Type.Groups {
		for _, s := range g.Styles {
			if s.Name == name {
				return s
			}
		}
	}
	t.Fatalf("no type style %s", name)
	return dsStyle{}
}

func (s dsStyle) lineHeightPx(t *testing.T, size float64) float64 {
	var str string
	if json.Unmarshal(s.LineHeight, &str) == nil {
		return pxOf(t, str)
	}
	var n float64
	if err := json.Unmarshal(s.LineHeight, &n); err != nil {
		t.Fatalf("%s: lineHeight %s", s.Name, s.LineHeight)
	}
	return round2(n * size)
}

type cssVars map[string]string

func compiledCSSVars(t *testing.T) cssVars {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	out := cssVars{}
	for _, m := range regexp.MustCompile(`--([a-z0-9-]+):([^;}]+)`).FindAllStringSubmatch(string(b), -1) {
		if _, seen := out[m[1]]; !seen {
			out[m[1]] = strings.TrimSpace(m[2])
		}
	}
	return out
}

// compiledCSSVarsDark is compiledCSSVars as the browser resolves it under `prefers-color-scheme: dark`: the dark block's
// declarations over the light ones.
func compiledCSSVarsDark(t *testing.T) cssVars {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	out := compiledCSSVars(t)
	_, dark := compiledCSSModes(t, string(b))
	for _, m := range regexp.MustCompile(`--([a-z0-9-]+):([^;}]+)`).FindAllStringSubmatch(dark, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

func (c cssVars) colourOf(t *testing.T, class string) string {
	name := class[strings.Index(class, "-")+1:]
	if name == "white" {
		return "#fff"
	}
	v, ok := c["color-"+name]
	if !ok {
		t.Fatalf("class %s: no --color-%s in app.css", class, name)
	}
	// A colour role is a property aliasing a palette step (DS-D15 stage 2: `--color-ink-body:var(--color-slate-700)`),
	// so follow the alias to the value it draws by default.
	for i := 0; strings.HasPrefix(v, "var(--color-"); i++ {
		next := strings.TrimSuffix(strings.TrimPrefix(v, "var(--"), ")")
		if v, ok = c[next]; !ok || i > 4 {
			t.Fatalf("class %s: --%s does not resolve in app.css", class, next)
		}
	}
	return v
}

// lengthOf turns a spacing or radius utility into px: gap-1.5, px-2.5, p-5 (Tailwind's 4px step), rounded-md.
func (c cssVars) lengthOf(t *testing.T, class string) float64 {
	if strings.HasPrefix(class, "rounded-") {
		step := strings.TrimPrefix(class, "rounded-")
		if step == "full" {
			return 9999
		}
		return remToPx(t, c["radius-"+step])
	}
	n, err := strconv.ParseFloat(class[strings.LastIndex(class, "-")+1:], 64)
	if err != nil {
		t.Fatalf("class %s: not a spacing step", class)
	}
	return n * 4
}

// textOf returns a text utility's size and line height in px.
func (c cssVars) textOf(t *testing.T, class string) (float64, float64) {
	key := class
	size := remToPx(t, c[key])
	lh := c[key+"--line-height"]
	if lh == "" {
		return size, 0
	}
	if m := regexp.MustCompile(`^calc\(([0-9.]+) / ([0-9.]+)\)$`).FindStringSubmatch(lh); m != nil {
		a, _ := strconv.ParseFloat(m[1], 64)
		b, _ := strconv.ParseFloat(m[2], 64)
		return size, round2(a / b * size)
	}
	r, err := strconv.ParseFloat(lh, 64)
	if err != nil {
		t.Fatalf("%s: line height %q", class, lh)
	}
	return size, round2(r * size)
}

func remToPx(t *testing.T, v string) float64 {
	n, err := strconv.ParseFloat(strings.TrimSuffix(v, "rem"), 64)
	if err != nil || !strings.HasSuffix(v, "rem") {
		t.Fatalf("expected a rem value, got %q", v)
	}
	return round2(n * 16)
}

func pxOf(t *testing.T, v string) float64 {
	n, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64)
	if err != nil {
		t.Fatalf("expected a px value, got %q", v)
	}
	return n
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

// normalizeColour makes the artifact's and Tailwind's spellings of one colour comparable without converting it.
func normalizeColour(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	s = regexp.MustCompile(`([( ,/])0\.`).ReplaceAllString(s, "$1.")
	switch s {
	case "#ffffff":
		return "#fff"
	case "#000000":
		return "#000"
	}
	return s
}

func sortedStrings(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
