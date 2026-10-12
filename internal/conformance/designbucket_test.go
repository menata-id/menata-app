package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// designFamilies are the class families a Theme category covers. A token in one of them that is **no counted role's
// default** is a design value: the system does not name it, so no reader can stand in for it. The impl ratchets count
// the role defaults; this bucket counts the rest (rencana 1.3, G1 stage two).
//
// Not here, each for its reason:
//   - padding: D4 made padding six roles by job, not a ladder, so a `px-3` is generic spacing rather than an unnamed
//     value;
//   - whole corners (`rounded`, `rounded-xl`, ...): TestHandWrittenRadiiOnlyShrink counts every corner already, with a
//     reason per file; only side corners (`rounded-t-2xl`) are counted here;
//   - arbitrary sizes (`text-[15px]`): handWrittenOffLadderSizes holds them per file, as the size part of this bucket.
var designFamilies = []struct {
	name string
	re   *regexp.Regexp
}{
	{"size", regexp.MustCompile(`^text-(?:3xs|2xs|xs|sm|item|base|lg|[2-9]?xl)$`)},
	{"weight", regexp.MustCompile(`^font-(?:thin|extralight|light|normal|medium|semibold|bold|extrabold|black)$`)},
	{"ink", regexp.MustCompile(`^text-(?:white|black|[a-z]+-\d{2,3})(?:/\d+)?$`)},
	{"fill", regexp.MustCompile(`^bg-(?:white|black|[a-z]+-\d{2,3})(?:/\d+)?$`)},
	{"edge", regexp.MustCompile(`^(?:border|divide|ring|outline|accent|caret|fill|stroke)(?:-[xytrbl])?-(?:white|black|[a-z]+-\d{2,3})(?:/\d+)?$`)},
	{"gap", regexp.MustCompile(`^gap(?:-[xy])?-(?:px|[\d.]+)$`)},
	{"corner", regexp.MustCompile(`^rounded-(?:[trblse]|tl|tr|bl|br|ss|se|es|ee)(?:-\w+)?$`)},
}

// unnamedDesignValues is the design bucket: every value in a Theme family that no role names, with how many times the
// rendering writes it and why it has no role. **Shrink-only per value**, like the other ratchets: a higher count fails,
// a lower one fails until the entry drops, a value not listed fails, and a listed value no longer written fails.
//
// The reasons group into four kinds, which is the reading worth keeping: a token the Design System names and code has
// no role for (`primary`, `scrim`, `accent-ring`, `radius-sheet`; GS1's dsUnmapped); a tone colour at a step the tone
// does not use; a fill or edge that is geometry or state (a track, a drag handle, a knock-out ring); and a half-step or
// two-axis gap. Measured 2026-10-12: 126 tokens, 52 values. Read the numbers out of the map.
//
// Blind spots, as in the impl ratchets: a variant-prefixed token (`hover:`, `sm:`), and a class returned by a Go
// function rather than written in a class attribute (`tagClasses`, `appIconClasses`, a helper's `return`).
var unnamedDesignValues = map[string]struct {
	n   int
	why string
}{
	// ink
	"text-white":       {15, "ink on a filled mark (primary button, step circle, count badge, done check): the Design System has `primary` but no on-fill ink, and a role waits on a fill role beside it (D3 holds background.accent)"},
	"text-slate-300":   {8, "chevrons and a '(planned)' aside: decorative and lighter than `faint`; no ink role sits below faint (rencana 1.2b reviews faint)"},
	"text-slate-600":   {4, "the neutral tone's ink (DS tone-neutral-ink) outside a tone pair; no Ink role is slate-600, and Tone/neutral is exempt from the pair count (themeRolesWithoutImplRatchet)"},
	"text-red-600":     {4, "a lone danger word (pre-auth error, Delete group link, overdue SLA, a 'yes'): DS danger-ink is red-700, and folding 600 into it is a decision (R1)"},
	"text-indigo-600":  {2, "the review tile's indigo: a hue outside the five tones and the Design System's colours"},
	"text-amber-600":   {1, "New Application's capability-gap note: a warn ink one step lighter than tone warn"},
	"text-amber-900":   {1, "Install Application's refusal title: a warn ink one step darker than tone warn"},
	"text-emerald-600": {1, "the signed marker's ink beside markGoodBorder: mark-good is a mark, not an ink"},
	"text-red-400":     {1, "the danger zone's heading: the bad tone has no faint step (open finding f of the Phase 1 review)"},
	// weight
	"font-semibold": {2, "two uppercase labels at 600; the weight ladder is normal and medium only"},
	"font-bold":     {1, "pre-auth brand glyph; no Theme reaches it"},
	// fill
	"bg-blue-600":     {8, "the accent as a fill (current step, numbered approver, signature marker, unread dot): D3 held background.accent"},
	"bg-slate-200":    {6, "DS avatar-fill, and the same grey as a track, a drag handle and a rule: geometry, not a surface"},
	"bg-slate-900":    {4, "DS `primary` (GS1 dsUnmapped): the primary button recipe, the active filter chip, the monogram tile, the pre-auth button"},
	"bg-slate-400":    {3, "a waiting signature marker's dot"},
	"bg-red-100":      {1, "the PDF file-type tile: a file type, not a tone"},
	"bg-red-500":      {1, "the rejected step circle: a filled bad mark, which the bad tone (a -50 wash) does not name"},
	"bg-red-600":      {1, "a notification count's red: a filled mark, not tone bad's wash"},
	"bg-blue-50/40":   {1, "an unread notification's translucent wash: opacity has no role"},
	"bg-blue-50/80":   {1, "the editable signature marker's translucent fill"},
	"bg-slate-50/40":  {1, "Role matrix header band at 40%: opacity has no role (open finding e)"},
	"bg-slate-50/50":  {1, "Role matrix footer band at 50%; see the header band"},
	"bg-slate-50/80":  {1, "the read-only signature marker's translucent fill"},
	"bg-slate-900/40": {1, "DS `scrim` (GS1 dsUnmapped): the mobile sheet's backdrop"},
	"bg-white/70":     {1, "the inbox count chip over a coloured tab"},
	// edge
	"border-indigo-200":  {3, "the review tile's dashed indigo rim; see text-indigo-600"},
	"border-slate-400":   {2, "a waiting signature marker's rim, and a waiting chip's"},
	"border-blue-400":    {1, "your signature box's dashed rim: the accent as an edge"},
	"border-blue-500":    {1, "your signature chip's rim: the accent as an edge"},
	"border-blue-600":    {1, "the editable signature marker's rim: the accent as an edge"},
	"border-emerald-300": {1, "a signed signature box's rim: a good edge one step past tone good's -200"},
	"border-slate-900":   {1, "the active filter chip's rim, with `primary`"},
	"border-white":       {1, "a badge's knock-out ring over a mark"},
	"ring-blue-100":      {2, "DS `accent-ring` (GS1 dsUnmapped): the current step's ring and your signature box"},
	"accent-blue-500":    {5, "a native checkbox's or radio's accent-color; two steps for one job (see accent-blue-600), and folding them is a decision (R1)"},
	"accent-blue-600":    {2, "a native checkbox's accent-color; see accent-blue-500"},
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
	"rounded-r-md":  {1, "pre-auth password reveal button; no Theme reaches it"},
	"rounded-t-2xl": {1, "DS `radius-sheet` (GS1 dsUnmapped): the mobile sheet's top corners, kept literal by owner decision DS-D17 (c)"},
	"rounded-tl-sm": {1, "the signature marker's resize handle"},
}

// countedRoleDefaults is the set of tokens some impl ratchet counts: every token of every Theme role's default class,
// except the roles G1 lists as uncounted. Those stay in the design bucket, since nothing else would see them.
func countedRoleDefaults(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, r := range themeRoleDefaults(t) {
		if _, exempt := themeRolesWithoutImplRatchet[r.category+"/"+r.name]; exempt {
			continue
		}
		for _, tok := range strings.Fields(r.class) {
			out[tok] = true
		}
	}
	return out
}

// designFamily names the family a token belongs to when it is a design value, and "" otherwise.
func designFamily(tok string, counted map[string]bool) string {
	if counted[tok] {
		return ""
	}
	for _, f := range designFamilies {
		if f.re.MatchString(tok) {
			return f.name
		}
	}
	return ""
}

// TestEveryUnnamedDesignValueHasAReason is rencana 1.3: the `design` bucket of G1. The impl ratchets say "read the
// Theme instead"; that advice is wrong for a value no role names, so this gate says something else: register a role
// through the procedure in menjaga-design-system, or record the value here as a floor with its reason. The two kinds of
// literal therefore fail with two different messages, which is what the plan's "done" asks for.
func TestEveryUnnamedDesignValueHasAReason(t *testing.T) {
	counted := countedRoleDefaults(t)
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
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
			for _, tok := range strings.Fields(m[1]) {
				if designFamily(tok, counted) == "" {
					continue
				}
				got[tok]++
				if where[tok] == nil {
					where[tok] = map[string]bool{}
				}
				where[tok][e.Name()] = true
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
	for _, tok := range sortedKeys(got) {
		entry, listed := unnamedDesignValues[tok]
		switch {
		case !listed:
			t.Errorf("`%s` (%s, in %s) has no Theme role, so no reader can stand in for it. Register a role "+
				"(menjaga-design-system, the registration procedure: Design System artifact first, then code), or add it to "+
				"unnamedDesignValues as a floor with the reason it has none. A value the system does not name is a design "+
				"decision, not a literal (R1)", tok, designFamily(tok, counted), files(tok))
		case got[tok] > entry.n:
			t.Errorf("`%s`: %d written (in %s), frozen at %d. It has no Theme role (%s); use a value the system names, or "+
				"raise the floor only with a reason the entry does not already give", tok, got[tok], files(tok), entry.n, entry.why)
		case got[tok] < entry.n:
			t.Errorf("`%s`: %d written, down from %d -- lower unnamedDesignValues to lock it in", tok, got[tok], entry.n)
		}
	}
	for _, tok := range sortedKeys(unnamedDesignValues) {
		if got[tok] == 0 {
			t.Errorf("unnamedDesignValues lists `%s`, which no class attribute writes any more -- drop the stale entry", tok)
		}
		if strings.TrimSpace(unnamedDesignValues[tok].why) == "" {
			t.Errorf("unnamedDesignValues[%q] has no reason", tok)
		}
	}
	if len(got) == 0 {
		t.Fatal("no design value found anywhere -- either every value has a role (keep the gate, empty) or the scan stopped reading class attributes")
	}
}

// TestDesignBucketAndImplRatchetsSplitTheTokens holds the line between the two buckets: no counted role default is a
// design value, and every role default that an impl ratchet does **not** count is one. Without the second half, a
// role G1 exempts (tone neutral's slate-600) would be counted by neither gate.
func TestDesignBucketAndImplRatchetsSplitTheTokens(t *testing.T) {
	counted := countedRoleDefaults(t)
	for tok := range counted {
		if f := designFamily(tok, counted); f != "" {
			t.Errorf("`%s` is a counted role default and also a design value (%s)", tok, f)
		}
	}
	for _, r := range themeRoleDefaults(t) {
		if _, exempt := themeRolesWithoutImplRatchet[r.category+"/"+r.name]; !exempt {
			continue
		}
		for _, tok := range strings.Fields(r.class) {
			if counted[tok] {
				continue // another, counted role has the same default
			}
			inFamily := false
			for _, f := range designFamilies {
				inFamily = inFamily || f.re.MatchString(tok)
			}
			if inFamily && designFamily(tok, counted) == "" {
				t.Errorf("%s/%s's `%s` is counted by neither the impl ratchets nor the design bucket", r.category, r.name, tok)
			}
		}
	}
}
