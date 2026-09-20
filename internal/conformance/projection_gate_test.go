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
// Why a ratchet at all: the Projection primitive (composition.ProjectCardFields, card_fields)
// shipped its mechanism and then stopped, with every test green, because nothing anywhere measured
// *adoption* -- the one dimension the conformance suite didn't cover (menata-app-document's
// audits/2026-09-19-decomposition-maturity-audit.md §5).
//
// How complete that stall was is worse than this comment first claimed, and the correction is the
// point. It said Projection was "a pilot on exactly one screen (Approval Inbox)". It is on zero:
// `grep -rn card_fields metadata/` returns nothing, so ProjectCardFields resolves an empty list
// for every record and approvalinbox.templ's own `if len(c.CardFields) > 0` branch has never
// executed. The mechanism is wired end to end and has never once run.
//
// The mistake that produced the wrong number is the same one that produced the nav gate's empty
// list an hour later: reading the *consuming code*, seeing it handle card_fields, and inferring a
// declaration must exist -- a private view of the shape standing in for the source of truth. The
// metadata is the source of truth for what is declared, and it was never read.
//
// Nor is the fix simply to declare some: Projection's only wired consumer is the pending-approval
// card, fed from mch_approval_step, whose only projectable Fields under the five known roles are
// fld_assignee (person) and fld_decision (status) -- both constant across that list, since it is
// by definition the steps assigned to the viewer and still pending. fld_sequence would say
// something real and has no role to carry it. Declaring card_fields there would render two
// identical values on every card: metadata written to make a claim true rather than to serve a
// screen. Projection gets a consumer that can say something when a Machine's records render as
// cards generically (CAP-V02 Tier 2 upstream, admitted on Case 3's own inbox), not before.

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
	//
	// "detail.templ" left this list in Fase 6b, the first entry ever to do so. It was here for two
	// reads -- fld_decision in decideButtons and fld_document in signatureConfirmation -- both of
	// which existed only because an Approval Step had no screen of its own and the generic detail
	// page was standing in for one. Board 10 is that screen now (reviewdocument.templ), its values
	// are resolved by composition.ReviewDocument, and the new file contains no raw read at all,
	// because a new file may not be added here. What is *not* claimed: detail.templ still carries
	// five `m.ID == action.DocumentMachineID` branches. This gate measures raw field reads, not
	// Machine-id branches, and saying otherwise would be the same written-claim-for-the-thing
	// failure this package exists to catch.
	//
	// "documentsubmit.templ" left in Fase 6c-2, the second entry to go. It was here for a single
	// line -- an <option> list built by hand from mch_user records, reading fld_name itself -- and
	// that line was a copy of something the Composition layer already did: Loader.RelationOptions
	// resolves a reference Field's targets to {ID, Label} using the target Machine's own first
	// Field, which for mch_user *is* fld_name. So this entry was never really debt about
	// Projection's shape; it was one screen not asking for what it already had. Worth
	// distinguishing from detail.templ's exit in 6b, which needed a whole new screen first.
	//
	// "signatureplacement.templ" left in Fase 6c-3, the third. Its exit is worth reading next to
	// the other two, because a third of its raw reads were not a projection at all: seven of them
	// echoed a record back verbatim as hidden inputs, forced by the generic update route rewriting
	// a whole record from whatever the form submits. Giving those a "shape" would have claimed a
	// meaning they do not have. Composition derives them from the Machine's own declared Fields
	// instead (composition.carryForward), which is generic access -- explicitly fine here -- and
	// which turned a hand-maintained list that had already forgotten four Fields into one that
	// cannot forget. The rest composed normally.
	"approvalstepper.templ": "Case 3 bespoke approval UI (writing-guide.md honest map)",
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
			t.Errorf("internal/rendering/%s reads raw record field values (Values[\"fld_...\"] or Values[<pkg>.Field...]) -- a Page renders a shape Composition already resolved (composition.ProjectCardFields / card_fields, 007 §4.4, §7.6), it does not pick fields off a record itself. This list is a ratchet and may only shrink: migrate the page rather than adding an entry", name)
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
