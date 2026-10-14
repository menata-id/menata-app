package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// designFamilies are the class families a Theme category covers. A token in one of them that **no impl ratchet
// counts** is a design value: the system does not name it, so no reader can stand in for it (rencana 1.3, G1 stage two).
// A token is read with a leading or trailing `!` (important) removed.
//
// Not here, each for its reason:
//   - padding: D4 made padding six roles by job, not a ladder, so a `px-3` is generic spacing rather than an unnamed
//     value;
//   - the corners `radiusToken` names (`rounded`, `-sm` .. `-2xl`, `-full`): TestHandWrittenRadiiOnlyShrink counts
//     every one of them with a reason per file. Other corners (`rounded-none`, `rounded-[10px]`, side corners) are here;
//   - arbitrary sizes (`text-[15px]`): handWrittenOffLadderSizes holds them per file, as the size part of this bucket;
//   - `transparent` and `current`: the absence of a colour, or the parent's, not a value to name.
var designFamilies = func() []struct {
	name string
	re   *regexp.Regexp
} {
	colour := `(?:white|black|` + customThemeColours() + `[a-z]+-\d{2,3}|\[#[0-9a-fA-F]{3,8}\])(?:/(?:\d+|\[[\d.]+\]))?`
	return []struct {
		name string
		re   *regexp.Regexp
	}{
		{"size", regexp.MustCompile(`^text-(?:3xs|2xs|xs|sm|item|base|lg|[2-9]?xl)(?:/[\w.\[\]]+)?$`)},
		{"weight", regexp.MustCompile(`^font-(?:thin|extralight|light|normal|medium|semibold|bold|extrabold|black|\[\d+\])$`)},
		{"ink", regexp.MustCompile(`^text-` + colour + `$`)},
		{"fill", regexp.MustCompile(`^bg-` + colour + `$`)},
		{"edge", regexp.MustCompile(`^(?:border|divide|ring|ring-offset|outline|accent|caret|fill|stroke|decoration|shadow|from|via|to)(?:-[xytrblse])?-` + colour + `$`)},
		{"gap", regexp.MustCompile(`^gap(?:-[xy])?-(?:px|[\d.]+|\[[^\]]+\])$`)},
		{"corner", regexp.MustCompile(`^rounded(?:-(?:[trblse]|tl|tr|bl|br|ss|se|es|ee))?(?:-(?:none|xs|sm|md|lg|[2-4]?xl|full|\[[^\]]+\]))?$`)},
	}
}()

// customThemeColours reads the colours `static/css/input.css` adds in `@theme` (`--color-brand`), as regexp
// alternatives, so `bg-brand` is a colour like any palette step.
func customThemeColours() string {
	b, err := os.ReadFile(filepath.Join(repoRoot(), "static", "css", "input.css"))
	if err != nil {
		panic(err)
	}
	var alt string
	for _, m := range regexp.MustCompile(`(?m)^\s*--color-([a-z][a-z0-9-]*):`).FindAllStringSubmatch(string(b), -1) {
		alt += regexp.QuoteMeta(m[1]) + "|"
	}
	return alt
}

// unnamedDesignValues is the design bucket: every value in a Theme family that no role names, with how many times the
// rendering writes it and why it has no role. **Shrink-only per value**, like the other ratchets: a higher count fails,
// a lower one fails until the entry drops, a value not listed fails, and a listed value no longer written fails.
//
// The reasons group into four kinds, which is the reading worth keeping: a token the Design System names and code has
// no role for (`primary`, `scrim`, `accent-ring`, `radius-sheet`; GS1's dsUnmapped); a tone colour at a step the tone
// does not use; a fill or edge that is geometry or state (a track, a drag handle, a knock-out ring); and a half-step or
// two-axis gap. Measured 2026-10-12: 126 tokens, 48 values. Read the numbers out of the map.
//
// Blind spots, as in the impl ratchets:
//   - a variant-prefixed token (`hover:`, `sm:`);
//   - a class in a Go string rather than a class attribute: a helper's `return`, a `strings.Join` recipe, a script
//     string. That is large: 74 family tokens on 2026-10-12, the primary button recipe among them;
//   - a backtick string in a class expression, and a `}` inside one (the shared `classExpression` stops at it).
var unnamedDesignValues = map[string]struct {
	n   int
	why string
}{
	// ink
	"text-white": {2, "the brand glyph on bg-brand, and a signature marker's number on the PDF paper layer (owner decision CP5: paper stays light); every other on-fill ink reads onFillClass since CP1 (D3 holds background.accent)"},

	"text-emerald-600": {1, "the signed marker's ink beside markGoodBorder: mark-good is a mark, not an ink"},
	// weight
	"font-semibold": {2, "two small labels at 600 (an uppercase pill, an inference role heading); the weight ladder is normal and medium only"},
	"font-bold":     {1, "pre-auth brand glyph; no Theme reaches it"},
	// fill
	"bg-brand":       {1, "the pre-auth brand mark's fill, `--color-brand` in input.css (GS1 dsUnmapped `brand`); no Theme reaches it"},
	"bg-blue-600":    {4, "signature markers and their legend on the PDF paper layer (CP5); the current step, numbered approver and unread dot read inkFillClass(InkAccent) since CP2"},
	"bg-slate-400":   {3, "a waiting signature chip's dot, and the read-only marker's"},
	"bg-blue-50/80":  {1, "the editable signature marker's translucent fill"},
	"bg-slate-50/80": {1, "the read-only signature marker's translucent fill"},
	// edge
	"border-slate-400": {2, "a waiting signature marker's rim, and a waiting chip's"},
	"border-blue-400":  {1, "your signature box's dashed rim: the accent as an edge"},
	"border-blue-500":  {1, "your signature chip's rim: the accent as an edge"},
	"border-blue-600":  {1, "the editable signature marker's rim: the accent as an edge"},
	"border-white":     {1, "a badge's knock-out ring over a mark"},
	"ring-blue-100":    {1, "your signature box on the PDF paper layer (CP5); the current step's ring reads accentRingClass since CP3"},
	// gap
	"gap-2.5": {10, "10px, between the ladder's rungs; xtight (6px) is the only half-step token, and folding is a decision (R1)"},
	"gap-3.5": {6, "14px, between the ladder's rungs; see gap-2.5"},
	"gap-4.5": {7, "pre-auth forms' 18px; no Theme reaches them"},
	"gap-x-2": {1, "a two-axis gap: the ladder gives one value for both axes (open finding e)"},
	"gap-x-3": {1, "a two-axis gap; see gap-x-2"},
	"gap-x-4": {2, "a two-axis gap (a definition list's columns); see gap-x-2"},
	"gap-y-1": {2, "a two-axis gap; see gap-x-2"},
	"gap-y-3": {2, "a two-axis gap (a definition list's rows); see gap-x-2"},
	"gap-8":   {1, "Application Settings' 32px tab gutter: above the ladder's top (region, 24px)"},
	// corner
	"rounded-[10px]": {1, "the pre-auth brand mark: DS `radius-brand-mark` (GS1 dsUnmapped); no Theme reaches it"},
	"rounded-r-md":   {1, "pre-auth password reveal button; no Theme reaches it"},
	"rounded-t-2xl":  {1, "DS `radius-sheet` (GS1 dsUnmapped): the mobile sheet's top corners, kept literal by owner decision DS-D17 (c)"},
	"rounded-tl-sm":  {1, "the signature marker's resize handle"},
}

// designRoleSets is what classification needs to know about the Theme's defaults.
type designRoleSets struct {
	// counted holds every token of a role default that an impl ratchet counts on its own: every Theme role except the
	// ones G1 exempts, and except the tones, whose fill and ink are counted only as a pair.
	counted map[string]bool
	// toneHalves holds the tones' default fill and ink tokens (bad, good, info, warn).
	toneHalves map[string]bool
}

func designRoles(t *testing.T) designRoleSets {
	t.Helper()
	sets := designRoleSets{counted: map[string]bool{}, toneHalves: map[string]bool{}}
	for _, r := range themeRoleDefaults(t) {
		if _, exempt := themeRolesWithoutImplRatchet[r.category+"/"+r.name]; exempt {
			continue
		}
		for _, tok := range strings.Fields(r.class) {
			if r.category == "Tone" {
				sets.toneHalves[tok] = true
			} else {
				sets.counted[tok] = true
			}
		}
	}
	return sets
}

// The kinds a token in one class set can be.
const (
	designKindNone   = ""       // in no family this bucket reads
	designKindImpl   = "impl"   // an impl ratchet counts it
	designKindLone   = "lone"   // a tone's default fill or ink written without its partner: a role exists, nothing counts it
	designKindDesign = "design" // in a family, and no role names it
)

// classifyClassSet sorts the tokens of one class set. The order is the gates' own: a token an impl ratchet counts is
// impl, including what the tone-pair, tone-edge and radius ratchets match beyond the defaults (so no literal is told
// both "read the Theme" and "no role exists"); a tone half whose partner is absent is lone; the rest of a family is a
// design value.
func classifyClassSet(set string, roles designRoleSets) map[string][]string {
	toks := strings.Fields(set)
	for i, tok := range toks {
		toks[i] = strings.TrimSuffix(strings.TrimPrefix(tok, "!"), "!")
	}
	tints, inks := map[string]bool{}, map[string]bool{}
	for _, tok := range toks {
		if m := tonePairTint.FindStringSubmatch(tok); m != nil {
			tints[m[1]] = true
		}
		if m := tonePairInk.FindStringSubmatch(tok); m != nil {
			inks[m[1]] = true
		}
	}
	out := map[string][]string{}
	for _, tok := range toks {
		kind := designKindNone
		inFamily := false
		for _, f := range designFamilies {
			inFamily = inFamily || f.re.MatchString(tok)
		}
		tint, ink := tonePairTint.FindStringSubmatch(tok), tonePairInk.FindStringSubmatch(tok)
		switch {
		case roles.counted[tok], radiusToken.MatchString(tok), toneEdgeToken.MatchString(tok):
			kind = designKindImpl
		case tint != nil && inks[tint[1]], ink != nil && tints[ink[1]]:
			kind = designKindImpl
		case roles.toneHalves[tok]:
			kind = designKindLone
		case inFamily:
			kind = designKindDesign
		}
		out[kind] = append(out[kind], tok)
	}
	return out
}

// designFamily names the family a design value belongs to, for the error message.
func designFamily(tok string) string {
	for _, f := range designFamilies {
		if f.re.MatchString(tok) {
			return f.name
		}
	}
	return ""
}

// loneToneHalves freezes, per token, a tone's default fill or ink written without its partner. The tone-pair ratchet
// counts a pair and cannot see a half, and the token is a role's default, so it is not a design value either: before
// 2026-10-12's review of this bucket, nothing counted these. They read toneInkClass/toneFillClass when the meaning is
// the tone's. Measured 2026-10-12; read the numbers out of the map.
var loneToneHalves = map[string]struct {
	n   int
	why string
}{
	"bg-blue-50":       {1, "the paper layer: your signature box on the review page (a mark over a document page, fixed in both modes, DK9)"},
	"bg-emerald-50":    {1, "a signed signature box on the paper layer, beside its -300 rim"},
	"text-blue-700":    {2, "your signature box's labels on the review page (the paper layer; the nav and the inbox tab read the Theme now)"},
	"text-emerald-700": {1, "a signed signature's label"},
}

// TestEveryUnnamedDesignValueHasAReason is rencana 1.3: the `design` bucket of G1. The impl ratchets say "read the
// Theme instead"; that advice is wrong for a value no role names, so this gate says something else: register a role
// through the procedure in menjaga-design-system, or record the value here as a floor with its reason. The two kinds of
// literal therefore fail with two different messages, which is what the plan's "done" asks for. A lone tone half, a
// role default no ratchet sees, is held here too (loneToneHalves), with the impl advice.
func TestEveryUnnamedDesignValueHasAReason(t *testing.T) {
	roles := designRoles(t)
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]int{designKindDesign: {}, designKindLone: {}}
	where := map[string]map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		src := joinClassExpressions(chipNoLine.ReplaceAllString(string(b), ""))
		for _, m := range themeClassValue.FindAllStringSubmatch(src, -1) {
			for kind, toks := range classifyClassSet(m[1], roles) {
				if got[kind] == nil {
					continue
				}
				for _, tok := range toks {
					got[kind][tok]++
					if where[tok] == nil {
						where[tok] = map[string]bool{}
					}
					where[tok][e.Name()] = true
				}
			}
		}
	}
	files := func(tok string) string {
		var fs []string
		for f := range where[tok] {
			fs = append(fs, f)
		}
		sort.Strings(fs)
		return strings.Join(fs, ", ")
	}
	check := func(kind string, frozen map[string]struct {
		n   int
		why string
	}, name, unlisted, raised string) {
		for _, tok := range sortedKeys(got[kind]) {
			entry, listed := frozen[tok]
			switch n := got[kind][tok]; {
			case !listed:
				t.Errorf(unlisted, tok, designFamily(tok), files(tok))
			case n > entry.n:
				t.Errorf(raised, tok, n, files(tok), entry.n, entry.why)
			case n < entry.n:
				t.Errorf("`%s`: %d written, down from %d -- lower %s to lock it in", tok, n, entry.n, name)
			}
		}
		for _, tok := range sortedKeys(frozen) {
			if got[kind][tok] == 0 {
				t.Errorf("%s lists `%s`, which no class attribute writes any more -- drop the stale entry", name, tok)
			}
			if strings.TrimSpace(frozen[tok].why) == "" {
				t.Errorf("%s[%q] has no reason", name, tok)
			}
		}
	}
	check(designKindDesign, unnamedDesignValues, "unnamedDesignValues",
		"`%s` (%s, in %s) has no Theme role, so no reader can stand in for it. Register a role "+
			"(menjaga-design-system, the registration procedure: Design System artifact first, then code), or add it to "+
			"unnamedDesignValues as a floor with the reason it has none. A value the system does not name is a design "+
			"decision, not a literal (R1)",
		"`%s`: %d written (in %s), frozen at %d. It has no Theme role (%s); use a value the system names, or "+
			"raise the floor only with a reason the entry does not already give")
	check(designKindLone, loneToneHalves, "loneToneHalves",
		"`%s` (%s, in %s) is one half of a tone pair, written without its partner, so the tone-pair ratchet cannot "+
			"see it. If the meaning is the tone's, read toneInkClass/toneFillClass(ctx, domain.Tone*); a Workspace's "+
			"`theme:` moves the reader and not the literal (006). Otherwise add it to loneToneHalves with the reason",
		"`%s`: %d written alone (in %s), frozen at %d (%s) -- read toneInkClass/toneFillClass(ctx, domain.Tone*) instead")
	if len(got[designKindDesign]) == 0 {
		t.Fatal("no design value found anywhere -- either every value has a role (keep the gate, empty) or the scan stopped reading class attributes")
	}
}

// TestDesignBucketAndImplRatchetsSplitTheTokens holds the line between the buckets. Every token of every role's
// default (padding aside, see designFamilies) must land somewhere when written alone: counted by an impl ratchet, held
// as a lone tone half, or a design value. The first version compared the bucket's regexes with themselves and could not
// fail; narrowing a family so that tone neutral's `text-slate-600` fell out of both gates passed the whole suite. The
// cases below pin the classification itself.
func TestDesignBucketAndImplRatchetsSplitTheTokens(t *testing.T) {
	roles := designRoles(t)
	for _, r := range themeRoleDefaults(t) {
		if r.category == "Padding" {
			continue
		}
		for _, tok := range strings.Fields(r.class) {
			if kinds := classifyClassSet(tok, roles); len(kinds[designKindNone]) > 0 {
				t.Errorf("%s/%s's default `%s`, written alone, is counted by no impl ratchet and is no design value -- "+
					"nothing would see a hand-written copy", r.category, r.name, tok)
			}
		}
	}
	cases := []struct{ set, tok, want string }{
		{"bg-red-50 text-red-700", "bg-red-50", designKindImpl},   // a pair: the tone-pair ratchet
		{"bg-red-100 text-red-700", "bg-red-100", designKindImpl}, // a pair beyond the defaults, still that ratchet's
		{"text-red-700", "text-red-700", designKindLone},
		{"bg-amber-50 p-3", "bg-amber-50", designKindLone},
		{"border-emerald-300", "border-emerald-300", designKindImpl}, // the tone-edge ratchet matches -300
		{"rounded-xl", "rounded-xl", designKindImpl},                 // the radius ratchet
		{"rounded-[10px]", "rounded-[10px]", designKindDesign},
		{"rounded-none", "rounded-none", designKindDesign},
		{"text-red-600", "text-red-600", designKindDesign}, // a tone hue at a step no role uses
		{"!text-pink-600", "text-pink-600", designKindDesign},
		{"bg-[#fff]", "bg-[#fff]", designKindDesign},
		{"bg-brand", "bg-brand", designKindDesign},
		{"text-slate-600", "text-slate-600", designKindDesign}, // tone neutral, exempt from G1
		{"text-slate-500", "text-slate-500", designKindImpl},
		{"bg-transparent", "bg-transparent", designKindNone},
		{"px-3", "px-3", designKindNone},
	}
	for _, c := range cases {
		got := classifyClassSet(c.set, roles)
		found := false
		for _, tok := range got[c.want] {
			found = found || tok == c.tok
		}
		if !found {
			t.Errorf("in %q, `%s` should be %q; classified as %v", c.set, c.tok, c.want, got)
		}
	}
}
