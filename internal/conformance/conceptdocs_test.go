package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// unportedUpstreamDocs are documents the concept docs cite that live only in `menata-runtime`, with the
// reason each is still cited and what closing it requires.
//
// **Measured 18 citations across 5 documents on 2026-09-30, by grep; this gate counts 7, and the gate is right** -- it counts only backticked citations, where grep also caught prose mentions. Re-measuring a carried number is the rule this repo keeps relearning, and every one was a
// pointer a reader of 001-007 could not follow -- the same class as the 141 dangling roadmap-phase citations fixed the same day, in the documents `CLAUDE.md` calls the normative target.
//
// `capability-lifecycle.md` was ported on 2026-10-02 (D1) and the architecture map the same day (D2); the
// §40 rewrite removed every citation of `capability-registry.md` and `runtime-metadata-schema.md`, and 002's
// see-also list stopped naming `menata-runtime`'s own roadmap. **What is left is two mentions that are
// already retractions**, and the entry exists so the count cannot creep back up.
//
// Shrink-only on the usual terms: a higher count fails, a lower one fails until the entry is updated, and a
// document that stops being cited fails too.
var unportedUpstreamDocs = map[string]struct {
	Citations int
	Reason    string
}{
	"architecture-benchmark.md": {
		Citations: 1,
		Reason: "named once by 002 as **research provenance**, with the sentence saying outright that it " +
			"lives in the closed `menata-runtime` archive and not here. Deliberately not ported: 510 lines " +
			"comparing browser engines, Kubernetes, Terraform, React, EMF and VS Code is the study behind " +
			"the layering, not architecture this runtime carries. The one normative implication -- VS " +
			"Code's small-core lesson -- is stated directly in capability-lifecycle.md §4. Until " +
			"2026-10-02 it was a markdown link that read as though the file were in this repo, which is " +
			"also what exposed this gate's own blind spot: it matched only backticked citations",
	},
	"composable-runtime-blueprint.md": {
		Citations: 2,
		Reason: "both remaining mentions are **retractions** -- 007's header and §40 preamble say outright " +
			"that the document is not in this repository and name the three that fill its role " +
			"(capability-lifecycle.md, capabilities.md, ROADMAP.md). Nothing to port; the entry exists so " +
			"the count cannot creep back up",
	},
}

// sisterRepoDocs are `menata-app-document`'s own files, which 001-007 legitimately cite with that
// repository named in the same sentence. A closed list rather than a prefix match, because "is this
// another repo's path" is not something the filename alone answers -- and the gate found its first one
// (`development-history.md`, cited by 007's own header) on its first run.
var sisterRepoDocs = map[string]bool{
	"development-history.md":     true,
	"case-portfolio.md":          true,
	"writing-guide-reference.md": true,
}

// TestConceptDocsCiteDocumentsThatExist is the document-level twin of TestCitedRoadmapAnchorsResolve:
// 001-007 may not cite a `*.md` this repository does not have, except for the named, counted exceptions
// above.
//
// Why it matters more here than anywhere else: `CLAUDE.md` instructs every session to read 001-007 before
// any architectural decision, and 007 defers to `capability-lifecycle.md` twice for its own admission
// language. A deferral to a file nobody can open is a deferral to nothing -- which is how 007 §40 came to
// carry "CEL expression evaluation -- PROVEN" for a runtime with no CEL, cited to a `CAP-` row in another
// repository.
func TestConceptDocsCiteDocumentsThatExist(t *testing.T) {
	docs, err := filepath.Glob(filepath.Join(repoRoot(), "00*.md"))
	if err != nil || len(docs) == 0 {
		t.Fatalf("found no concept documents (err %v) -- this gate would pass by measuring nothing", err)
	}

	cited := map[string]int{}
	for _, path := range docs {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		// Both citation forms. **Backticks alone were a blind spot**: 002 linked
		// `architecture-benchmark.md` as a markdown link to a file this repo does not have, and the gate
		// could not see it -- found by hand on 2026-10-02, the day the gate was written, which is why the
		// second pattern is here rather than waiting for the next reader to trip over it.
		cites := regexp.MustCompile("`([0-9A-Za-z][0-9A-Za-z._-]*\\.md)`").FindAllStringSubmatch(string(body), -1)
		cites = append(cites, regexp.MustCompile(`\]\(([0-9A-Za-z][0-9A-Za-z._-]*\.md)\)`).FindAllStringSubmatch(string(body), -1)...)
		for _, m := range cites {
			name := m[1]
			if _, err := os.Stat(filepath.Join(repoRoot(), name)); err == nil {
				continue // resolves, nothing to say
			}
			if sisterRepoDocs[name] {
				continue // another repository's file, cited with that repository named -- not this gate's to resolve
			}
			cited[name]++
		}
	}

	for name, n := range cited {
		exc, declared := unportedUpstreamDocs[name]
		if !declared {
			t.Errorf("001-007 cite %q %d time(s) and this repository does not have it -- port it, or record it in unportedUpstreamDocs with the reason and what closing it requires. A normative document deferring to a file nobody can open is deferring to nothing", name, n)
			continue
		}
		switch {
		case n > exc.Citations:
			t.Errorf("citations of the unported %q rose from %d to %d -- this population may only shrink (%s)", name, exc.Citations, n, exc.Reason)
		case n < exc.Citations:
			t.Errorf("citations of the unported %q fell from %d to %d -- lower the entry to lock the improvement in", name, exc.Citations, n)
		}
	}
	for name, exc := range unportedUpstreamDocs {
		if cited[name] == 0 {
			t.Errorf("unportedUpstreamDocs still excuses %q, which 001-007 no longer cite (or which now exists) -- remove the entry (%s)", name, exc.Reason)
		}
	}
	if len(cited) == 0 && len(unportedUpstreamDocs) > 0 {
		t.Fatal("no unresolved document citation found at all -- every entry above is stale, or this gate is looking in the wrong place")
	}
	// Guard against the gate silently measuring nothing: the concept docs must cite *some* resolvable
	// document, or the regexp has stopped matching.
	resolvable := 0
	for _, path := range docs {
		body, _ := os.ReadFile(path)
		if strings.Contains(string(body), "`capability-lifecycle.md`") {
			resolvable++
		}
	}
	if resolvable == 0 {
		t.Fatal("no concept document cites capability-lifecycle.md -- the citation regexp has stopped matching")
	}
}
