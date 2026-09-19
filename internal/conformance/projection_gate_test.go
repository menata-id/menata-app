package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Package projection_gate_test.go holds the adoption ratchet for 007 §4.4 (Projection over
// Retrieval) / §7.6 (Projection): a Page renders a shape the Composition layer already resolved,
// it does not reach into a record and pick fields out by name itself.
//
// This is a different kind of gate from the rest of internal/conformance, and deliberately so.
// The others state an invariant that already holds everywhere. This one states an invariant that
// does *not* hold yet -- ten .templ files violate it today -- so instead of failing the build it
// freezes the violation set: the listed files are grandfathered, and the list may only shrink.
// A new file reaching into raw values fails; a listed file that has been migrated and no longer
// needs its entry also fails, so the list can't silently keep claiming debt that's already paid.
//
// Why a ratchet at all: the Projection primitive (composition.ProjectCardFields, view.card_fields)
// shipped as a pilot on exactly one screen (Approval Inbox) and then stopped there for weeks with
// every test green, because nothing anywhere measured *adoption*. Adoption was the one dimension
// the conformance suite didn't cover -- see menata-app-document's
// audits/2026-09-19-decomposition-maturity-audit.md §5.

// rawFieldRead matches a Page picking one named field off a record: `Values["fld_title"]`, or the
// same thing laundered through a Machine-specific Go constant, `Values[action.FieldStepDecision]`.
//
// Both forms are the same variation point, so both are gated. Checking only the literal would
// leave the exact bypass internal/web's own route gate already exists to prevent (its doc comment:
// a violation must not be able to "move down one level" into a clean-looking variable) -- and that
// bypass isn't hypothetical here, `action.Field*` constants are already in use.
//
// Generic access is the target pattern and must not match: `Values[f.ID]` (from ranging over
// m.Fields), `Values[fieldID]`, `Values[m.Fields[0].ID]`. The `Field[A-Z]` boundary is what
// separates a Machine-specific constant (`action.FieldStepDecision`) from the generic field list
// (`m.Fields`) -- singular-plus-capital versus plural.
var rawFieldRead = regexp.MustCompile(`Values\[(?:"fld_|[a-z][A-Za-z]*\.Field[A-Z])`)

// projectionRatchet is the frozen violation set: every internal/rendering/*.templ file that reads
// raw field values as of the day this gate was added, and why each is still allowed to.
//
// Every entry is already in writing-guide.md's "What comes free vs. what's hardcoded today" table,
// so none of them is new or undocumented drift -- but the two groups differ in how soon they can
// leave, which is why they're commented separately rather than pooled into one anonymous list.
var projectionRatchet = map[string]string{
	// Case 19's composed Project Management screens. These are ROADMAP.md's own named next step:
	// Dataset + Dimension + Measure (007 §7.2-§7.4) resolves their data, Projection resolves their
	// field shape, and each file leaves this list as it's migrated.
	"boardsettings.templ":   "Case 19 composed screen, migrates with Dataset/Projection (ROADMAP Planned)",
	"calendar.templ":        "Case 19 composed screen, migrates with Dataset/Projection (ROADMAP Planned)",
	"dashboard.templ":       "Case 19 composed screen, migrates with Dataset/Projection (ROADMAP Planned)",
	"mytasks.templ":         "Case 19 composed screen, migrates with Dataset/Projection (ROADMAP Planned)",
	"sprintdashboard.templ": "Case 19 composed screen, migrates with Dataset/Projection (ROADMAP Planned)",
	"teamcapacity.templ":    "Case 19 composed screen, migrates with Dataset/Projection (ROADMAP Planned)",

	// Case 3's bespoke Document Approval flow. These have a stronger claim to stay than the group
	// above: the stepper and the signature canvas render an orchestration whose own logic is
	// already assessed as failing B4 (internal/action/decide.go's doc comment), not a generic
	// record shape. Listed here so the ratchet covers them rather than leaving a silent hole --
	// not as a promise that they migrate on the same schedule.
	"approvalstepper.templ":    "Case 3 bespoke approval UI (writing-guide.md honest map)",
	"detail.templ":             "Case 3 special-casing inside the otherwise-generic detail renderer",
	"documentsubmit.templ":     "Case 3 submission wizard (writing-guide.md honest map)",
	"signatureplacement.templ": "Case 3 signature-coordinate canvas (writing-guide.md honest map)",
}

func TestRenderingUsesProjectionNotRawValues(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	violating := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if rawFieldRead.Match(src) {
			violating[e.Name()] = true
		}
	}

	for name := range violating {
		if _, allowed := projectionRatchet[name]; !allowed {
			t.Errorf("internal/rendering/%s reads raw record field values (Values[\"fld_...\"] or Values[<pkg>.Field...]) -- a Page renders a shape Composition already resolved (composition.ProjectCardFields / view.card_fields, 007 §4.4, §7.6), it does not pick fields off a record itself. This list is a ratchet and may only shrink: migrate the page rather than adding an entry", name)
		}
	}

	var stale []string
	for name := range projectionRatchet {
		if !violating[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("projectionRatchet still lists internal/rendering/%s, but that file no longer reads raw field values -- delete its entry, so the list keeps measuring real remaining debt instead of freezing a number that's already been paid down", name)
	}
}
