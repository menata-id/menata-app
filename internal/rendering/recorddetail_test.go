package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func renderDetail(t *testing.T, extras RecordExtras) string {
	t.Helper()
	m := &domain.Machine{ID: "mch_task", Name: "Task", Fields: []domain.Field{{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText}}}
	r := &data.Record{ID: "rec_1", Values: map[string]any{"fld_title": "Lock shooting schedule"}}
	var buf bytes.Buffer
	if err := RecordDetailView(m, r, nil, nil, nil, domain.Actor{ID: "usr_ana"}, nil, extras, time.Now()).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

func TestRecordDetail_DrawsTagsAndHistoryFromWhatCompositionResolved(t *testing.T) {
	html := renderDetail(t, RecordExtras{
		Tags:         []CardTag{{Label: "Schedule", Color: domain.TagBlue}},
		ShowActivity: true,
		Activity: []ActivityEntry{
			{Summary: `"Lock shooting schedule" moved from todo to in_progress`, Actor: "Ana", When: "2026-10-03 09:20"},
		},
	})
	for _, want := range []string{"Schedule", "History", "moved from todo to in_progress", "Ana", "2026-10-03 09:20"} {
		if !strings.Contains(html, want) {
			t.Errorf("detail is missing %q", want)
		}
	}
	if strings.Contains(html, "Showing the first") {
		t.Error("a history inside its bound drew a truncation notice")
	}
}

func TestRecordDetail_NoActivitySectionWhenTheWorkspaceKeepsNoLog(t *testing.T) {
	html := renderDetail(t, RecordExtras{})
	if strings.Contains(html, ">History<") || strings.Contains(html, "Nothing has happened") {
		t.Errorf("a Workspace with no activity log drew a section: %s", html)
	}
}

func TestRecordDetail_AnEmptyHistoryIsSaidAndATruncatedOneIsFlagged(t *testing.T) {
	empty := renderDetail(t, RecordExtras{ShowActivity: true})
	if !strings.Contains(empty, "Nothing has happened to this record yet.") {
		t.Error("a record with no events did not say so")
	}
	cut := renderDetail(t, RecordExtras{ShowActivity: true, Activity: []ActivityEntry{{Summary: "x"}}, ActivityTruncation: Truncation{Limit: 50, Hit: true}})
	if !strings.Contains(cut, "Showing the first") || !strings.Contains(cut, "50") {
		t.Error("a truncated history did not say so, so it would read as the whole story")
	}
}

func moveForDetail() *RecordMove {
	return &RecordMove{
		ViewID: "vw_board",
		CardMove: CardMove{Field: "fld_list", Current: "lst_b", Position: 3, Targets: []MoveTarget{
			{Value: "lst_a", Label: "Backlog"}, {Value: "lst_b", Label: "Doing"},
		}},
	}
}

// The panel is the board card's move: the group Field and `position`, patched to the record naming the board View.
func TestRecordDetail_MovePanelPatchesTheGroupFieldAndPositionThroughTheBoardView(t *testing.T) {
	html := renderDetail(t, RecordExtras{Move: moveForDetail()})
	for _, want := range []string{
		"Move…", `hx-patch="/machines/mch_task/records/rec_1?view=vw_board"`,
		`name="fld_list"`, `<option value="lst_b" selected>Doing</option>`, `<option value="lst_a">Backlog</option>`,
		`name="position"`, `value="3"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("detail is missing %q", want)
		}
	}
	if strings.Contains(renderDetail(t, RecordExtras{}), "Move…") {
		t.Error("a Machine with nothing to move within drew a Move panel")
	}
}

// Offering a move the server will refuse would only lie, so the panel follows the Edit permission.
func TestRecordDetail_MovePanelIsHiddenFromAnActorWhoMayNotEdit(t *testing.T) {
	m := &domain.Machine{ID: "mch_task", Name: "Task",
		Fields:      []domain.Field{{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText}, {ID: "fld_owner", Name: "Owner", Type: domain.FieldTypePerson}},
		Permissions: []domain.Permission{{Action: domain.ActionEdit, ActorField: "fld_owner"}},
	}
	r := &data.Record{ID: "rec_1", Values: map[string]any{"fld_title": "T", "fld_owner": "usr_other"}}
	var buf bytes.Buffer
	if err := RecordDetailView(m, r, nil, nil, nil, domain.Actor{ID: "usr_ana"}, nil, RecordExtras{Move: moveForDetail()}, time.Now()).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "Move…") {
		t.Error("an actor the Edit permission refuses was offered a move")
	}
}
