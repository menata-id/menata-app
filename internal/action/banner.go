// Approval-status banner (owner request, 2026-09-19): a growing, one-line-per-step record of
// every approval decided so far, stamped at the top of the Document's own PDF (page 1 only) --
// visible progress on the document itself, not just inside the app. Pure text-building only; the
// actual PDF stamping (fonts, color, layout math) is composite.go's job, matching this package's
// existing split between pure decision logic (decide.go) and pure byte-in/byte-out PDF work
// (composite.go).
package action

import (
	"fmt"
	"strings"
	"time"
)

// ApprovalStatusLine is one already-approved step's own contribution to the banner. Only approved
// steps contribute -- a rejected step halts the approval flow without adding a line here, per the
// owner's own scoping (a rejection is not "approval status" the same way an approval is).
type ApprovalStatusLine struct {
	Sequence     int
	Total        int
	ApproverName string
	ApprovedAt   time.Time
}

// ApprovalStatusBanner renders every approved step decided so far into one growing line, joined
// by " | ". Recomputed from scratch every time signDocument runs (the same posture
// CompositeSignatures already takes for the signature images themselves) so the banner always
// reflects exactly what's recorded in the database, never an accumulated diff. Empty input yields
// an empty string -- CompositeStatusBanner treats that as "nothing to stamp yet".
func ApprovalStatusBanner(lines []ApprovalStatusLine) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = fmt.Sprintf("APPROVED: Step %d/%d - %s - %s", l.Sequence, l.Total, l.ApproverName, l.ApprovedAt.Format("2 Jan 2006"))
	}
	return strings.Join(parts, " | ")
}
