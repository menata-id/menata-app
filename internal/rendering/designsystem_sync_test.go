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

	"menata.app/internal/domain"
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

	colour := map[string]string{
		"ink-strong": inkClass(ctx, domain.InkStrong), "ink-body": inkClass(ctx, domain.InkBody),
		"ink-secondary": inkClass(ctx, domain.InkSecondary), "ink-faint": inkClass(ctx, domain.InkFaint),
		"bg-raised": backgroundClass(ctx, domain.BackgroundRaised), "bg-base": backgroundClass(ctx, domain.BackgroundBase),
		"bg-sunken":      backgroundClass(ctx, domain.BackgroundSunken),
		"border-divider": borderClass(ctx, domain.BorderDivider), "border-surface": borderClass(ctx, domain.BorderSurface),
		"border-control": borderClass(ctx, domain.BorderControl),
		"mark-good":      markGoodFill(),
	}
	for tone := range domain.KnownBadgeTones {
		colour["tone-"+string(tone)+"-bg"] = toneFillClass(ctx, tone)
		colour["tone-"+string(tone)+"-ink"] = toneInkClass(ctx, tone)
	}
	length := map[string]string{
		"gap-xtight": layoutGap(ctx, domain.GapXtight), "gap-tight": layoutGap(ctx, domain.GapTight),
		"gap-default": layoutGap(ctx, domain.GapDefault), "gap-comfortable": layoutGap(ctx, domain.GapComfortable),
		"gap-loose":  layoutGap(ctx, domain.GapLoose),
		"gap-region": layoutGap(ctx, domain.GapRegion),
		"pad-panel":  paddingClass(ctx, domain.PaddingPanel), "pad-section": paddingClass(ctx, domain.PaddingSection),
		"pad-badge-x": paddingClass(ctx, domain.PaddingBadgeX), "pad-badge-y": paddingClass(ctx, domain.PaddingBadgeY),
		"pad-badge-compact-x": paddingClass(ctx, domain.PaddingBadgeCompactX),
		"pad-badge-compact-y": paddingClass(ctx, domain.PaddingBadgeCompactY),
		"radius-control":      radiusClass(ctx, domain.RadiusControl), "radius-surface": radiusClass(ctx, domain.RadiusSurface),
		"radius-pill": radiusClass(ctx, domain.RadiusPill),
	}
	type style struct{ size, weight string }
	styles := map[string]style{
		"display":       {textClass(ctx, domain.RoleDisplay), weightClass(ctx, domain.WeightEmphasis)},
		"heading":       {textClass(ctx, domain.RoleHeading), weightClass(ctx, domain.WeightEmphasis)},
		"subheading":    {textClass(ctx, domain.RoleSubheading), weightClass(ctx, domain.WeightEmphasis)},
		"panel-heading": {textClass(ctx, domain.RoleBody), weightClass(ctx, domain.WeightEmphasis)},
		"body":          {textClass(ctx, domain.RoleBody), weightClass(ctx, domain.WeightBody)},
		"meta":          {textClass(ctx, domain.RoleMeta), weightClass(ctx, domain.WeightBody)},
		"label":         {textClass(ctx, domain.RoleLabel), weightClass(ctx, domain.WeightBody)},
		"caption":       {textClass(ctx, domain.RoleCaption), weightClass(ctx, domain.WeightBody)},
		"eyebrow":       {textClass(ctx, domain.RoleEyebrow), weightClass(ctx, domain.WeightBody)},
	}

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

// dsAcceptedNotInCode: accepted by the artifact, not yet a Theme role. Shrinks as each lands (rencana 0.7).
var dsAcceptedNotInCode = map[string]string{
	"control-height-compact": "32px; hand-written h-8 until a slice adds it",
}

// dsUnmapped: tokens no Theme reader answers for, with the reason. Each is a record, not a check.
var dsUnmapped = map[string]string{
	"brand": "--color-brand in input.css; the M mark only, not a Theme role", "brand-icon": "the PWA PNGs' fill, not CSS",
	"accent": "links are literal text-blue-600 by owner decision (DH 10-11); no ink role", "accent-hover": "as accent",
	"accent-ring": "the stepper's current-step ring, literal", "focus-ring": "fieldClasses' literal outline-blue-300",
	"primary": "buttonClasses' literal bg-slate-900", "primary-hover": "buttonClasses' literal hover",
	"hover-wash": "hover is state, not a role (theme.go)", "avatar-fill": "avatarClasses' literal",
	"scrim": "the dialog backdrop, literal", "danger-ink": "danger Button recipe literal; tone-bad-ink aliases it and is checked",
	"danger-border": "danger Button recipe literal; no border role (D9)", "danger-wash": "tone-bad-bg aliases it and is checked",
	"tone-warn-border": "the truncation notice's literal border; no border role for tones (D9)",
	"pad-metric":       "metric's literal p-3", "pad-control-x": "buttonClasses' literal px-4",
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
	for _, tk := range d.all() {
		if tk.Name == name {
			var s string
			if json.Unmarshal(tk.Value, &s) == nil {
				return s, true
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

// colour resolves a `{alias}` chain the way the artifact does.
func (d dsTokens) colour(t *testing.T, name string) string {
	for i := 0; i < 16; i++ {
		v, ok := d.raw(name)
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

func (c cssVars) colourOf(t *testing.T, class string) string {
	name := class[strings.Index(class, "-")+1:]
	if name == "white" {
		return "#fff"
	}
	v, ok := c["color-"+name]
	if !ok {
		t.Fatalf("class %s: no --color-%s in app.css", class, name)
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
	if s == "#ffffff" {
		return "#fff"
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
