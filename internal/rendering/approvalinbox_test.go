package rendering

import (
	"bytes"
	"context"
	"fmt"
	"github.com/a-h/templ"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

// pendingCardFixture is the minimal PendingApprovalCard needed to render pendingApprovalCard
// without panicking (parseSLA tolerates an empty/unparseable SLADue, so it's left unset here).
func pendingCardFixture(cardFields []ProjectedField) PendingApprovalCard {
	return PendingApprovalCard{
		Reference:    "DOC-0001",
		Title:        "Contract",
		DocumentType: "Contract",
		Mode:         "sequential",
		Approved:     0,
		TotalSteps:   1,
		Submitter:    "Ana Putri",
		Href:         "/machines/mch_approval_step/records/stp_1",
		CardFields:   cardFields,
	}
}

// TestPendingApprovalCard_rendersCardFields is the Fase 1 pilot's own proof: a card_fields entry
// resolved by internal/composition.ProjectCardFields shows up on the rendered card without this
// file (or pendingApprovalCard/pendingApprovalCardBody) knowing anything about the specific Field
// that produced it -- only its already-resolved Label/Role/Display.
func TestPendingApprovalCard_rendersCardFields(t *testing.T) {
	c := pendingCardFixture([]ProjectedField{
		{Label: "Department", Role: "title", Display: "Legal"},
	})

	var buf bytes.Buffer
	if err := pendingApprovalCard(c).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Department") || !strings.Contains(html, "Legal") {
		t.Errorf("rendered card missing projected field label/value; got:\n%s", html)
	}
}

// TestPendingApprovalCard_noCardFieldsRendersNoMetaRow is the regression guard: every Machine
// today declares no card_fields, so this must render exactly as it did before Fase 1 -- no
// new markup for a card that projects nothing.
func TestPendingApprovalCard_noCardFieldsRendersNoMetaRow(t *testing.T) {
	c := pendingCardFixture(nil)

	var buf bytes.Buffer
	if err := pendingApprovalCard(c).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "pending-card-meta") {
		t.Errorf("card with no CardFields rendered a pending-card-meta block, want none")
	}
}

// assignedNavFixture is ApprovalInboxPage's own nav context for the Assigned to me tab: appShell's
// breadcrumb needs nav_home, the tab strip needs all three of this Application's own items, and
// the heading/subtitle come from whichever one navID names (titleByID/descriptionByID panic on an
// id absent from this list, the same posture routeByID/labelByID already have).
func assignedNavFixture() context.Context {
	return WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: resolvedNav([]domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_approval_inbox", Label: "Inbox", Title: "Pending my approval", Route: "/approval-inbox"},
		{ID: "nav_my_documents", Label: "My Doc", Title: "My documents", Route: "/approval-inbox?tab=mine"},
		{
			ID: "nav_assigned_to_me", Label: "Assign Me", Title: "Assigned to me",
			Description: "Every document that asked for your approval.", Route: "/approval-inbox?tab=assigned",
		},
	})}, "Test Workspace", false)
}

// TestApprovalInboxPage_AssignedTabRendersRows is the third tab's own proof: given a real
// AssignedRow, the page renders its heading, the row's own fields, and no "+ New document" button
// -- the one piece of chrome this tab deliberately drops (ApprovalInboxPage's own doc comment: a
// worklist over other people's documents is not a place to start one).
func TestApprovalInboxPage_AssignedTabRendersRows(t *testing.T) {
	rows := []AssignedRow{{
		Reference:     "DOC-0103",
		Title:         "Lead Actor Contract Amendment",
		DocumentType:  "Contract",
		From:          "Rina Nur",
		RequestedAt:   "17 Sep 2026",
		Status:        "in_review",
		DecisionKey:   "not_yet",
		DecisionLabel: "Not yet your turn — step 2 of 3",
		Via:           "Legal Group",
		Href:          "/machines/mch_document/records/doc_1/review",
	}}
	filters := []FilterChip{{Key: "all", Label: "All", Count: 1, Active: true}}

	var buf bytes.Buffer
	page := ApprovalInboxPage(nil, nil, nil, nil, nil, rows, filters, Truncation{}, TabAssigned, "", "Acme", Viewer{Initials: "AN"}, "")
	if err := page.Render(assignedNavFixture(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		"Assigned to me", "Lead Actor Contract Amendment", "Rina Nur", "17 Sep 2026",
		"Not yet your turn", "via Legal Group", "Waiting items also appear in your Inbox", "/machines/mch_document/records/doc_1/review",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	if strings.Contains(html, "+ New document") {
		t.Error("Assigned to me rendered the \"+ New document\" button, want none")
	}
}

// TestApprovalInboxPage_AssignedTabEmptyState is the other end: no rows at all still renders the
// filter chips and an honest empty message rather than an empty table.
func TestApprovalInboxPage_AssignedTabEmptyState(t *testing.T) {
	filters := []FilterChip{{Key: "all", Label: "All", Count: 0, Active: true}}

	var buf bytes.Buffer
	page := ApprovalInboxPage(nil, nil, nil, nil, nil, nil, filters, Truncation{}, TabAssigned, "", "Acme", Viewer{Initials: "AN"}, "")
	if err := page.Render(assignedNavFixture(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "No document has asked for your approval yet.") {
		t.Error("empty Assigned to me did not render its own empty-state message")
	}
}

// TestApprovalInboxPage_EveryEmptyCaseDrawsTheEmptyCard covers the five places the Inbox says "nothing here" -- two tabs,
// each with and without a search, and the default tab. The 2.3c review found that passing `@emptyState("")` at the
// default tab's call survived the whole suite: only one of the five had a test, and it asserted the text but not the card.
func TestApprovalInboxPage_EveryEmptyCaseDrawsTheEmptyCard(t *testing.T) {
	filters := []FilterChip{{Key: "all", Label: "All", Count: 0, Active: true}}
	const hostile = `a"b<c`
	for _, tc := range []struct{ name, tab, q, message string }{
		{"default tab", "", "", "Nothing pending your approval."},
		{"My Documents", TabMine, "", "You haven't submitted any documents yet."},
		{"My Documents, searched", TabMine, hostile, fmt.Sprintf("No document matches %q.", hostile)},
		{"Assigned to me", TabAssigned, "", "No document has asked for your approval yet."},
		{"Assigned to me, searched", TabAssigned, hostile, fmt.Sprintf("No document matches %q.", hostile)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var page, card bytes.Buffer
			if err := ApprovalInboxPage(filters, nil, nil, nil, filters, nil, filters, Truncation{}, tc.tab, tc.q, "Acme", Viewer{Initials: "AN"}, "").Render(assignedNavFixture(), &page); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if err := emptyState(tc.message).Render(assignedNavFixture(), &card); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if !strings.Contains(page.String(), card.String()) {
				t.Errorf("the page does not draw the EmptyState card for %q:\nwant to contain %s", tc.message, card.String())
			}
			if strings.Count(page.String(), "border-dashed") != 1 {
				t.Errorf("want exactly one dashed card on the page, got %d", strings.Count(page.String(), "border-dashed"))
			}
		})
	}
}

// TestPendingApprovalCard_SLALabelOnlyWhenUrgent holds board 07's two places for time: the pill above the
// title is for a card that needs attention now, and the footer carries the detail either way.
func TestPendingApprovalCard_SLALabelOnlyWhenUrgent(t *testing.T) {
	render := func(c PendingApprovalCard) string {
		var buf bytes.Buffer
		if err := pendingApprovalCard(c).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	base := PendingApprovalCard{Title: "Contract", Status: "in_review", Approved: 1, TotalSteps: 3, Href: "/r", Action: "Review →"}

	calm := base
	calm.SLA = experience.SLABadge{Label: "6 days left", Tone: domain.ToneNeutral, Detail: "6 days remaining", Present: true}
	html := render(calm)
	if strings.Contains(html, "6 days left") {
		t.Error("a card with days to spare drew its pill above the title")
	}
	for _, want := range []string{"6 days remaining", "Review →", "Ongoing · 1 of 3 approved"} {
		if !strings.Contains(html, want) {
			t.Errorf("calm card missing %q", want)
		}
	}

	late := base
	late.SLA = experience.SLABadge{Label: "OVERDUE", Tone: domain.ToneBad, Detail: "SLA breached · 3 days", Urgent: true, Present: true}
	html = render(late)
	for _, want := range []string{"OVERDUE", "SLA breached · 3 days"} {
		if !strings.Contains(html, want) {
			t.Errorf("overdue card missing %q", want)
		}
	}
}

// Rencana 0.2c: the approver chip's settled states and the archived banner read the Theme's tone pairs, so a
// Workspace that remaps a tone moves them. Byte identity under the default theme is what the migration diff proved;
// this is the other half, that the reader is really in the path.
func TestTonePairs_followTheThemeAtMigratedSites(t *testing.T) {
	th := domain.Theme{Tone: map[domain.BadgeTone]domain.TonePalette{
		domain.ToneGood: domain.PaletteBlue,
		domain.ToneWarn: domain.PaletteRed,
	}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", true)
	render := func(c templ.Component) string {
		var buf strings.Builder
		if err := c.Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return resolveColourRoles(t, ctx, buf.String())
	}
	if got := render(approverChip(StepApprover{Name: "Ana", State: "done"})); !strings.Contains(got, "bg-blue-50 text-blue-700") || strings.Contains(got, "emerald") {
		t.Errorf("a remapped good tone must move the done chip: %s", got)
	}
	if got := render(appShell("T", "WS", Viewer{}, "/s")); !strings.Contains(got, `border-b border-red-200 bg-red-50 px-4 py-2 text-center text-xs font-medium text-red-700">This workspace is archived`) {
		t.Error("a remapped warn tone must move the archived banner's edge, fill and ink (the edge since DS-D16)")
	}
}

// S1 (owner decision, 2026-10-12): the urgent SLA pill reads the tones. Due today is now warn's ink (amber-800, was
// a literal amber-700: the one visible change), overdue stays red, and both follow theme.tone.
func TestSLAPill_readsTheBadAndWarnTones(t *testing.T) {
	if got := resolveColourRoles(t, context.Background(), slaPillTone(context.Background(), false)); got != "bg-amber-50 text-amber-800" {
		t.Errorf("due today must be warn's pair, got %q", got)
	}
	if got := resolveColourRoles(t, context.Background(), slaPillTone(context.Background(), true)); got != "bg-red-50 text-red-700" {
		t.Errorf("overdue must be bad's pair, got %q", got)
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: domain.Theme{Tone: map[domain.BadgeTone]domain.TonePalette{domain.ToneWarn: domain.PaletteBlue}}}, "WS", false)
	if got := resolveColourRoles(t, ctx, slaPillTone(ctx, false)); got != "bg-blue-50 text-blue-700" {
		t.Errorf("a remapped warn tone must move the due-today pill, got %q", got)
	}
}

// The active filter chip is told to assistive technology, not by colour alone (gap-plan P3a, 2026-10-14); the others are not.
func TestFilterChip_marksOnlyTheActiveOne(t *testing.T) {
	render := func(f FilterChip) string {
		var buf bytes.Buffer
		if err := filterChip(f, "?filter=").Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	if got := render(FilterChip{Key: "all", Label: "All", Count: 3, Active: true}); !strings.Contains(got, `aria-current="true"`) {
		t.Errorf("the active chip carries no aria-current: %s", got)
	}
	if got := render(FilterChip{Key: "late", Label: "Late", Count: 1}); strings.Contains(got, "aria-current") {
		t.Errorf("an inactive chip is marked current: %s", got)
	}
}
