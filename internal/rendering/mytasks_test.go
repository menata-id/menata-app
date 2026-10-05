package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

func myTaskFixture(id, title string, done bool) TaskRow {
	next := "done"
	if done {
		next = "todo"
	}
	return TaskRow{
		Task:        &data.Record{ID: id, MachineID: "mch_task"},
		Title:       title,
		ProjectName: "Dokter Kecil",
		ListName:    "Shooting",
		Tags:        []CardTag{{Label: "Set", Color: domain.TagEmerald}},
		Complete:    &CardComplete{Field: "fld_status", Next: next, Done: done},
		Date:        experience.CardDate{Label: "6 Oct", Tone: domain.ToneNeutral, Present: true},
	}
}

func renderMyTasks(t *testing.T, overdue, next7, later, done []TaskRow) string {
	t.Helper()
	var buf bytes.Buffer
	if err := taskSection("Overdue", "needs a new date or a tick", overdue).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if err := taskSection("Next 7 days", "", next7).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if err := taskSection("Later", "", later).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if err := taskSection("Completed", "", done).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// A row is the completion circle, the title (linking to the Task's own record) with its board and tags, the
// list it sits in, and its date. A section with no rows draws nothing at all, so no empty heading is left
// on a quiet day.
func TestMyTasks_rowShapeAndEmptySectionsDrawNothing(t *testing.T) {
	got := renderMyTasks(t, nil, []TaskRow{myTaskFixture("rec_1", "Day 1", false)}, nil, []TaskRow{myTaskFixture("rec_2", "Signed", true)})

	for _, want := range []string{
		"Next 7 days", "Completed",
		`hx-patch="/machines/mch_task/records/rec_1"`, `href="/machines/mch_task/records/rec_1"`,
		`name="fld_status" value="done"`, `name="fld_status" value="todo"`,
		`aria-pressed="false"`, `aria-pressed="true"`,
		"Dokter Kecil", "Set", "Shooting", "6 Oct",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("My Tasks is missing %q\n%s", want, got)
		}
	}
	for _, absent := range []string{"Overdue", "needs a new date", "Later"} {
		if strings.Contains(got, absent) {
			t.Errorf("an empty section drew %q", absent)
		}
	}
}

// A Machine that declares no `completion:` gives a row with no circle -- a button that could not say what
// to write would be a control that does nothing.
func TestMyTasks_noCompletionNoCircle(t *testing.T) {
	row := myTaskFixture("rec_1", "Plain", false)
	row.Complete = nil
	if got := renderMyTasks(t, nil, nil, []TaskRow{row}, nil); strings.Contains(got, "<button") || strings.Contains(got, "hx-patch") {
		t.Errorf("a row with no completion drew a control:\n%s", got)
	}
}
