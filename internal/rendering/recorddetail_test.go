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
