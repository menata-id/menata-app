package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// submitNavFixture returns a ctx carrying the navigation this page resolves its links through.
// It used to set package state (ConfigureWorkspace) and restore it in a Cleanup; since 2026-09-22
// the installed Workspace is per-request, so a fixture hands back a ctx instead of mutating a
// global -- which also means these tests no longer have to be careful about running in parallel.
func submitNavFixture(t *testing.T) context.Context {
	t.Helper()
	return WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_approval_inbox", Label: "Approval Inbox", Route: "/approval-inbox"},
		{ID: "nav_new_approval", Label: "New Approval", Route: "/documents/new"},
	}}, "Test Workspace", false)
}

// modeField and documentTypeField are the two mch_document Fields the wizard renders options from.
// Both carry values that are NOT the ones the old hardcoded template used, which is the point: if
// the page ever goes back to retyping "Sequential"/"Parallel" the assertions below stop matching.
func modeField() domain.Field {
	return domain.Field{ID: "fld_mode", Name: "Approval mode", Type: domain.FieldTypeStatus,
		Options: []string{"berurutan", "serentak"}}
}

func documentTypeField() domain.Field {
	return domain.Field{ID: "fld_document_type", Name: "Document Type", Type: domain.FieldTypeStatus,
		Options: []string{"Kontrak", "Tagihan"}}
}

func renderSubmitPage(t *testing.T) string {
	t.Helper()
	ctx := submitNavFixture(t)
	approvers := RelationOptions{domain.UserMachineID: {{ID: "usr_rina", Label: "Rina Nur"}}}
	groups := GroupOptions{{ID: "grp_legal", Label: "Legal Group"}}

	var buf bytes.Buffer
	c := DocumentSubmitPage(documentTypeField(), modeField(), approvers, groups, "Dokter Kecil", Viewer{Initials: "AP"}, "", DraftPrefill{})
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// The wizard's option lists must come from the Fields it is handed, not from words typed into the
// template. fld_mode was the last place that was untrue -- two hardcoded radios sitting below a
// select that already read metadata -- so this fixture deliberately declares options no template
// would ever guess.
func TestDocumentSubmitPage_optionsComeFromMetadata(t *testing.T) {
	html := renderSubmitPage(t)
	for _, want := range []string{`value="berurutan"`, `value="serentak"`, `value="Kontrak"`, `value="Tagihan"`} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered wizard is missing %s -- an option list was retyped instead of read from its Field", want)
		}
	}
	for _, unwanted := range []string{`value="sequential"`, `value="parallel"`} {
		if strings.Contains(html, unwanted) {
			t.Errorf("rendered wizard still contains the hardcoded %s", unwanted)
		}
	}
}

// Board 08's User/Group toggle is what makes CAP-F24 reachable by a person. Both pickers are in
// the markup at once and the group one starts hidden; the type select swaps them.
func TestDocumentSubmitPage_offersBothApproverKinds(t *testing.T) {
	html := renderSubmitPage(t)
	for _, want := range []string{
		`name="fld_approver_type"`, `value="User"`, `value="Group"`,
		`name="fld_assignee"`, "Rina Nur",
		`name="fld_approver_group"`, "grp_legal", "Legal Group",
		`name="fld_step_name"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered approver row is missing %q", want)
		}
	}
	if !strings.Contains(html, "approver-group hidden") {
		t.Error("the group picker must start hidden, so a row defaults to the User arm")
	}
}

// The type picker must be a <select>. A radio pair would submit nothing for an untouched row, and
// because row order IS fld_sequence, a missing element shifts every later row's approver onto the
// wrong step. This asserts the shape rather than the styling, because the styling is not what
// makes it correct.
func TestApproverRow_typePickerAlwaysSubmitsAValue(t *testing.T) {
	var buf bytes.Buffer
	if err := ApproverRow(nil, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	if strings.Contains(html, `type="radio" name="fld_approver_type"`) {
		t.Error("fld_approver_type must not be a radio group: an unchecked radio submits nothing, which desynchronizes the parallel row slices")
	}
	if !strings.Contains(html, `<select`) || !strings.Contains(html, `name="fld_approver_type"`) {
		t.Error("fld_approver_type must be a select, so every row submits exactly one value")
	}
	// The step number is a CSS counter, not a Go-passed index: rows are reordered client-side.
	if !strings.Contains(html, "counter-increment:approver-step") {
		t.Error("the row number must be a CSS counter -- a number passed from Go goes stale on the first reorder")
	}
}
