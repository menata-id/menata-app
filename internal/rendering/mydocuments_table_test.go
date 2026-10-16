package rendering

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func myDocumentsFixture() (drafts, mine []PendingApprovalCard) {
	drafts = []PendingApprovalCard{{ID: "rec_d1", Reference: "DOC-0001", Title: "Draft <b>plan</b>", DocumentType: "Memo", Status: "draft", SubmittedAt: "3 Oct 2026", Href: "/documents/rec_d1", Action: "Continue →"}}
	mine = []PendingApprovalCard{
		{ID: "rec_m1", Reference: "DOC-0002", Title: "Budget 2027", DocumentType: "Finance", Status: "in_review", SubmittedAt: "12 Sep 2026", Approved: 1, TotalSteps: 2, Href: "/review/rec_m1", Action: "View →",
			Approvers: []StepApprover{{Name: "Sari", Initials: "SA", State: "done"}, {Name: "Dedi", Initials: "DE", State: "current"}}},
		{ID: "rec_m2", Reference: "DOC-0003", Title: "Vendor contract", DocumentType: "Legal", Status: "approved", SubmittedAt: "1 Sep 2026", Approved: 2, TotalSteps: 2, Href: "/review/rec_m2", Action: "View →"},
		{ID: "rec_m3", Reference: "DOC-0004", Title: "Travel request", DocumentType: "HR", Status: "rejected", SubmittedAt: "30 Aug 2026", Approved: 1, TotalSteps: 2, Href: "/review/rec_m3", Action: "View →",
			Approvers: []StepApprover{{Name: "Sari", Initials: "SA", State: "done"}, {Name: "Dedi", Initials: "DE", State: "rejected"}}},
	}
	return drafts, mine
}

func renderMyDocuments(t *testing.T) string {
	t.Helper()
	drafts, mine := myDocumentsFixture()
	var page bytes.Buffer
	if err := ApprovalInboxPage(nil, nil, drafts, mine, nil, nil, nil, Truncation{}, TabMine, "", "Acme", Viewer{Initials: "AN"}, "").Render(assignedNavFixture(), &page); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if path := os.Getenv("MYDOCS_DUMP"); path != "" {
		_ = os.WriteFile(path, page.Bytes(), 0o644)
	}
	return page.String()
}

// My Documents is a table on a desktop and cards on a phone (owner decision M-D7). Both are in the page and CSS picks one,
// so each is held complete on its own.
func TestMyDocuments_isATableFromTheSmallBreakpointAndCardsBelowIt(t *testing.T) {
	html := renderMyDocuments(t)
	for _, want := range []string{
		`aria-label="Submitted documents"`, `aria-label="Draft documents"`,
		`<th scope="col"`, ">Document</th>", ">Submitted</th>", ">Status</th>", ">Progress</th>", ">Action</th>", ">Last edited</th>",
		`class="hidden sm:block"`, `class="sm:hidden"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	// Every document is in both shapes, and a title is escaped in both.
	for _, title := range []string{"Budget 2027", "Vendor contract", "Travel request"} {
		if n := strings.Count(html, ">"+title+"<"); n != 2 {
			t.Errorf("%q appears %d times, want once in the table and once in a card", title, n)
		}
	}
	if strings.Contains(html, "<b>plan</b>") || strings.Count(html, "Draft &lt;b&gt;plan&lt;/b&gt;") != 2 {
		t.Errorf("a title was not escaped in both shapes")
	}
}

func TestMyDocuments_theTableSaysWhereEachDocumentStands(t *testing.T) {
	html := renderMyDocuments(t)
	table := func(label string) string {
		i := strings.Index(html, `aria-label="`+label+`"`)
		if i < 0 {
			t.Fatalf("no table %q", label)
		}
		rest := html[i:]
		return rest[:strings.Index(rest, "</table>")]
	}
	sub, dr := table("Submitted documents"), table("Draft documents")
	for _, want := range []string{">In review<", ">Approved<", ">Rejected<", "Ongoing · 1 of 2 approved", "Approved · 2 of 2 approved", "Rejected by Dedi · 1 of 2 approved", "12 Sep 2026"} {
		if !strings.Contains(sub, want) {
			t.Errorf("the submitted table lacks %q", want)
		}
	}
	if !strings.Contains(dr, ">Draft<") || strings.Contains(dr, ">Progress</th>") {
		t.Errorf("the draft table should say Draft and have no Progress column: %s", dr)
	}
	if n := strings.Count(sub, "/revise"); n != 1 {
		t.Errorf("the submitted table has %d Revise forms, want one (the rejected row)", n)
	}
	if !strings.Contains(sub, "/records/rec_m3/revise") {
		t.Errorf("the Revise form of the rejected row does not post to its own document")
	}
	if n := strings.Count(html, "/revise"); n != 2 {
		t.Errorf("the page has %d Revise forms, want the card's and the table row's", n)
	}
	// A status the table does not know is shown as it is, never guessed at.
	var b bytes.Buffer
	if err := myDocumentStatusBadge("on_hold").Render(assignedNavFixture(), &b); err != nil || !strings.Contains(b.String(), "on_hold") {
		t.Errorf("an unknown status badge = %q, %v", b.String(), err)
	}
	if got := b.String(); strings.Contains(got, string(domain.ToneMuted)) {
		t.Errorf("an unknown status drew the muted tone, which is faint ink: %s", got)
	}
}
