package aiassist

import (
	"fmt"
	"sort"
	"strings"
)

// Gap statuses. A gap is open until someone plans it; "built" means the capability exists AND the assistant
// can write it, which is the same sentence registry.Capabilities states with AIWritable.
const (
	GapOpen    = "open"
	GapPlanned = "planned"
	GapBuilt   = "built"
)

// KnownGap is one capability the assistant has recorded as missing, under the key every later record of the
// same need must reuse. The register is the shared vocabulary of the gap log (plan 2026-10-25, A2): the
// prompt shows the model every open key so a need is named once, and the report counts by key.
//
// It holds no refusal logic and no count -- counts are read from the log -- only the name, the ideal
// capability in one sentence, and where the need stands.
type KnownGap struct {
	Key     string // slug; unique
	Class   string // one of GapClasses
	Summary string // the ideal capability, in general terms
	Status  string // GapOpen, GapPlanned or GapBuilt
	// Capability is the registry.Capabilities id of the capability that satisfies it. Required exactly when
	// Status is GapBuilt: a built gap must name a capability the assistant can write.
	Capability string
}

// KnownGaps is seeded from the AI-GAP lines of 2026-10-22 (six rows, three needs). A new need arrives here
// when triage (plan phase D1) names it; the model proposes keys, a developer admits them.
var KnownGaps = []KnownGap{
	{
		Key:     "user-managed-fields-and-statuses",
		Class:   ClassConfiguration,
		Summary: "An application's admin adds, renames, orders and removes the Fields and status options of its Machines from a settings screen, without editing metadata.",
		Status:  GapOpen,
	},
	{
		Key:     "create-from-grouping-sets-the-group",
		Class:   ClassWorkflowLogic,
		Summary: "A record created from inside a group (a board column, a filtered list) is created with the grouping relation already set to that group's record.",
		Status:  GapOpen,
	},
	{
		Key:     "explain-why-an-action-is-unavailable",
		Class:   ClassWorkflowLogic,
		Summary: "When a person cannot do something, the screen says which role or rule blocks them and who can grant it.",
		Status:  GapOpen,
	},
}

// GapSlug turns text into the slug form a capability key takes: lowercase letters and digits joined by single
// hyphens, at most 64 characters. An input with no letter or digit yields "" (the caller falls back).
func GapSlug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = strings.TrimRight(out[:64], "-")
	}
	return out
}

// ValidateKnownGaps reports every way a register is malformed. capability answers, for a registry
// capability id, whether it exists and whether the assistant can write it -- passed in because registry
// imports only domain and this package must not import it.
func ValidateKnownGaps(gaps []KnownGap, capability func(id string) (exists, aiWritable bool)) []string {
	var problems []string
	seen := map[string]bool{}
	for _, g := range gaps {
		at := fmt.Sprintf("gap %q", g.Key)
		if g.Key == "" || GapSlug(g.Key) != g.Key {
			problems = append(problems, at+": key is not a slug")
		}
		if seen[g.Key] {
			problems = append(problems, at+": key appears twice")
		}
		seen[g.Key] = true
		if !validGapClass(g.Class) {
			problems = append(problems, at+": class "+g.Class+" is not one of the closed set")
		}
		if strings.TrimSpace(g.Summary) == "" {
			problems = append(problems, at+": no summary of the ideal capability")
		}
		switch g.Status {
		case GapOpen, GapPlanned:
			if g.Capability != "" {
				problems = append(problems, at+": is "+g.Status+" yet names capability "+g.Capability+"; only a built gap does")
			}
		case GapBuilt:
			exists, writable := capability(g.Capability)
			switch {
			case g.Capability == "":
				problems = append(problems, at+": is built but names no capability")
			case !exists:
				problems = append(problems, at+": names capability "+g.Capability+", which registry.Capabilities does not list")
			case !writable:
				problems = append(problems, at+": is built but "+g.Capability+" is not AIWritable, so the assistant still cannot write it")
			}
		default:
			problems = append(problems, at+": status "+g.Status+" is not open, planned or built")
		}
	}
	return problems
}

// openGapSentence lists, for the prompt, every gap of the register not yet built.
func openGapSentence() string { return openGapsSentence(KnownGaps) }

// openGapsSentence lists every gap in gaps not yet built, so the model reuses a key when the need is the
// same. Sorted by key so the prompt is stable between requests. It takes the slice so a test can give it a
// built gap, which the real register does not hold yet.
func openGapsSentence(gaps []KnownGap) string {
	var open []KnownGap
	for _, g := range gaps {
		if g.Status != GapBuilt {
			open = append(open, g)
		}
	}
	if len(open) == 0 {
		return ""
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Key < open[j].Key })
	var b strings.Builder
	b.WriteString("\n\nNames of capabilities already recorded as missing. This list is only a vocabulary: it exists so that one need " +
		"gets one name. Record a gap ONLY because what this person asked for needs something you cannot write. " +
		"Being on this list is never a reason to record one, and if the request does not need it, do not mention it. " +
		"When it does need one of them, use its key exactly instead of inventing a second name:\n")
	for _, g := range open {
		b.WriteString("- " + g.Key + " (" + g.Class + "): " + g.Summary + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
