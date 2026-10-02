package metadata

import (
	"fmt"

	"menata.app/internal/domain"
)

// ExplainNavigation reports the one Phase-4 inference the Experience Plane carries: a navigation item
// whose heading nobody wrote.
//
// It is separate from Explain because its subject is not a Machine. Explain takes the Workspace's
// Machines and answers for each; navigation lives on domain.Application, so a caller has to hand over
// Applications instead. Two functions rather than one widened signature, because the two populations are
// genuinely different and a combined `Explain(machines, applications)` would make every existing caller
// pass something it does not have.
//
// **Why this exists at all**: until 2026-10-02 the title→label fallback lived inside
// rendering.titleByID, which made it an inference happening at render time, visible to nothing. 001 #6's
// second clause names *rendering* among the things an unexplained inference must not silently affect, and
// 005 Phase 4 requires the normalized result be "inspectable enough to explain important runtime
// decisions". Stamping it at load (LoadApplication) was half the fix; this is the other half, and without
// it the stamping would just move a hidden inference from one plane to another.
//
// A declared title is reported as `not applicable` rather than omitted: the reader's question is "which
// of these headings did an author actually write", and a row that disappears when the answer is "this one"
// cannot answer it.
func ExplainNavigation(applications []domain.Application) []domain.Resolution {
	var out []domain.Resolution
	for _, app := range applications {
		for _, item := range app.AllNavigation {
			r := domain.Resolution{
				Name:   fmt.Sprintf("%s: %s.%s", domain.DerivationNavTitle, app.ID, item.ID),
				Value:  item.Heading,
				From:   "navigation.title",
				Status: domain.StatusNotApplicable,
			}
			switch {
			case item.Heading == "":
				// Unreachable through the loader, which resolves every item -- so reaching it means an
				// Application was built in Go and never loaded. The message says that rather than blaming
				// the metadata, the same posture unboundActorField takes.
				r.Status = domain.StatusUndeclared
				r.From = "nothing -- this Application skipped LoadApplication's own resolution"
			case item.Title == "":
				// The inference, detected exactly: no title: was declared, so the heading is the label.
				// Reading Title rather than comparing Heading to Label is what makes this precise -- a
				// screen whose heading legitimately equals its menu label is *declared*, not inferred.
				r.Status = domain.StatusResolved
				r.From = "navigation.label (no title: declared)"
			}
			out = append(out, r)
		}
	}
	return out
}
