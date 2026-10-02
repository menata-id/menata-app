package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRoadmapStaysAReleasePlan is two *ratchets* over `ROADMAP.md`'s own shape, on the same
// shrink-only terms every other named population in this package carries: a higher count fails, a
// **lower** count fails too until the entry is updated, and a new section fails.
//
// **Why a gate over a Markdown file.** `menata-app-document`'s README defines the split -- this repo's
// ROADMAP is "short, feature/release-level, forward-looking" and public, while phases, forcing
// conditions, design rationale and verification steps live in that repo's `development-history.md`.
// Using it consistently is what failed. Measured 2026-09-30: `development-history.md` stopped at
// 2026-09-22 with 3,229 lines, while `ROADMAP.md` reached 4,599, of which `## In progress` and
// `## Planned` were ~4,480 lines of history-repo content. Seven days and about twelve commits went into
// the wrong file -- including on the day a staleness header was added to the right one. Prose asking for
// the split existed the whole time; only the file grew.
//
// **The numbers below are the post-migration ones, and that ordering was deliberate.** Freezing first
// would have meant lowering these maps a dozen times in a week while the backlog moved; so 22 entries
// and 3,279 lines went to `development-history.md` first (ROADMAP 4,599 -> 1,377), and the ratchet
// froze on the result. A ratchet is for holding ground already taken -- the same reason
// `TestNoBareMachineIDIdentityChecks` could only be written after all 23 call sites were converted.
//
// **The gate cannot run across repos, and that decided its shape.** `.github/workflows/ci.yml` checks
// out `menata-app` alone, and the companion repo is private with no submodule, so anything reading
// `development-history.md` would `t.Skip` in CI -- guarded by nothing exactly where it matters, the same
// failure mode as the five dokter-kecil routes no test loaded. The cross-repo staleness check is
// therefore a *non-blocking* `.githooks/pre-commit` step that says out loud when it skips, not a test.
//
// **What this gate measures instead: vocabulary and volume, never fit.** It cannot tell whether *one*
// paragraph belongs here or there. A gate claiming to judge that is the ~50%-false-positive shape this
// repo has deleted twice (the static fixture-discovery gate, the naive accessor gate). **So a green run
// means the file stopped growing -- never that the split is right.**

// roadmapDiaryVocabulary is the discriminator, chosen by measurement rather than taste: words that
// belong to a verification log and never to a release plan.
//
// **Dates are deliberately not here.** A release log is entitled to date its releases -- `## Shipped`
// carries eight and every one is correct -- so matching dates would have punished the section that is
// already right.
//
// Audited when written (2026-09-30, before the migration): 78 hits over 76 lines, every one genuine, and
// 18 of those lines survive the migration. The hex arm produced **zero** false positives despite English
// words built from a-f, because the 7-character floor excludes them. If it ever starts matching prose,
// tighten the floor rather than dropping the ratchet.
var roadmapDiaryVocabulary = regexp.MustCompile(`\b[0-9a-f]{7,40}\b|\b[Mm]easured\b|[Mm]utation-prov|\b[Pp]robe[ds]?\b|\bfrozen at\b`)

// roadmapDiaryLines freezes, per `## ` section, how many lines use that vocabulary. It controls the
// **flow**: one new verification paragraph fails.
//
// `Shipped` is frozen at **0**, which is the entry that matters most -- the section that is already
// correct can never take a history paragraph, and 0 is enforceable precisely because the section reached
// it without anyone editing it.
//
// Read the numbers out of this map, not out of any prose describing it.
var roadmapDiaryLines = map[string]int{
	"Shipped":     0,
	"In progress": 14,
	"Planned":     2,
}

// roadmapSectionLines freezes each section's size, which is the README's own word -- "short". It
// controls the **volume** the vocabulary ratchet cannot see: before the migration 76 marker lines sat
// inside ~4,480 lines of narrative, so blocking new markers alone would have left the backlog
// unmeasured.
//
// A ratchet rather than a budget on purpose. `TestAuthenticatedPageQueryCost`'s own comment says a
// budget is a threshold that rots; a shrink-only ceiling does not, because it never needs re-picking.
//
// **Frozen exactly, with the cost stated rather than discovered: every addition to ROADMAP.md fails
// until something moves out to `development-history.md`.** That is the strict form, chosen by the owner
// on 2026-09-30 over a ceiling with slack. When it feels obstructive, the answer is to move a paragraph
// to the repo it belongs in -- which is the gate working, not the gate misfiring.
//
// `In progress` at 1,056 is the remaining debt and is expected to fall: it holds the 27 entries the
// migration left because splitting them is a per-paragraph judgement (status belongs here, rationale
// does not) rather than the two mechanical cases the migration covered -- a shipped feature still filed
// under `In progress`/`Planned`, and an entry restating an audit that already exists in `audits/`.
var roadmapSectionLines = map[string]int{
	"Shipped":     130,
	"In progress": 1056,
	"Planned":     150,
}

func TestRoadmapStaysAReleasePlan(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(), "ROADMAP.md"))
	if err != nil {
		t.Fatalf("read ROADMAP.md: %v", err)
	}
	diary, total := roadmapSectionCounts(string(body))
	if len(total) == 0 {
		t.Fatal("ROADMAP.md has no `## ` sections -- this gate would pass by measuring nothing")
	}

	assertRoadmapRatchet(t, "diary-vocabulary lines", diary, roadmapDiaryLines,
		"move the paragraph to menata-app-document/development-history.md and leave a feature-level line plus a pointer here")
	assertRoadmapRatchet(t, "section lines", total, roadmapSectionLines,
		"ROADMAP.md is the release plan (short, feature-level); the narrative belongs in menata-app-document/development-history.md")
}

// assertRoadmapRatchet is the shared shrink-only comparison, and it reports a *drop* as loudly as a
// rise. A drop means the migration this gate exists to encourage actually happened -- so the message
// says congratulations and what to update, rather than reading like a failure the author caused by
// mistake.
func assertRoadmapRatchet(t *testing.T, what string, got, frozen map[string]int, advice string) {
	t.Helper()
	for _, section := range sortedKeys(got) {
		want, declared := frozen[section]
		if !declared {
			t.Errorf("ROADMAP.md has a new section %q (%d %s) that the frozen population does not cover -- add it, at the count it starts with", section, got[section], what)
			continue
		}
		switch {
		case got[section] > want:
			t.Errorf("ROADMAP.md section %q: %d %s, frozen at %d -- %s", section, got[section], what, want, advice)
		case got[section] < want:
			t.Errorf("ROADMAP.md section %q: %d %s, down from %d -- lower the entry in this file to lock the improvement in", section, got[section], what, want)
		}
	}
	for _, section := range sortedKeys(frozen) {
		if _, present := got[section]; !present {
			t.Errorf("the frozen population names section %q, which ROADMAP.md no longer has -- remove the entry", section)
		}
	}
}

// roadmapSectionCounts returns, per `## ` section, how many lines use the diary vocabulary and how many
// lines the section has. A section's own heading counts toward its size, which is why the frozen numbers
// are read off this function rather than off `wc -l`.
func roadmapSectionCounts(body string) (diary, total map[string]int) {
	diary, total = map[string]int{}, map[string]int{}
	section := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		}
		if section == "" {
			continue // the title and any preamble above the first section
		}
		total[section]++
		if roadmapDiaryVocabulary.MatchString(line) {
			diary[section]++
		}
	}
	// A section with no marker line still needs an entry, so the map is keyed the same both ways --
	// otherwise `Shipped: 0` would look like a section the gate forgot rather than one that is clean.
	for section := range total {
		if _, ok := diary[section]; !ok {
			diary[section] = 0
		}
	}
	return diary, total
}
