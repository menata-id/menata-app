package composition

import (
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func task(id, project, assignee, status, due string) *data.Record {
	return rec(id, map[string]any{
		"fld_project":  project,
		"fld_assignee": assignee,
		"fld_status":   status,
		"fld_due_date": due,
	})
}

var projectLabels = map[string]string{"prj_1": "Apollo", "prj_2": "Borneo"}

// A week's range says the year and month once when both ends share them and carries each end's own when they do
// not -- the words `window:` writes over a Dataset's two bounds.
func TestCalendarRange(t *testing.T) {
	day := func(v string) time.Time {
		d, err := time.Parse("2006-01-02", v)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	for _, tc := range []struct{ name, first, last, want string }{
		{"one month", "2026-09-14", "2026-09-20", "14 \u2013 20 Sep 2026"},
		{"across a month", "2026-09-28", "2026-10-04", "28 Sep \u2013 04 Oct 2026"},
		{"across a year", "2026-12-28", "2027-01-03", "28 Dec 2026 \u2013 03 Jan 2027"},
		// A window that is exactly one whole month is said as the month, in full -- what a month grid's heading reads.
		{"a whole month", "2026-10-01", "2026-10-31", "October 2026"},
		{"a whole leap February", "2028-02-01", "2028-02-29", "February 2028"},
		{"not quite a month", "2026-10-02", "2026-10-31", "02 \u2013 31 Oct 2026"},
	} {
		if got := calendarRange(day(tc.first), day(tc.last)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// taskMachineForTest is mch_task reduced to the declaration My Tasks reads: its own card_fields, which
// is where the title, the status and the due Field come from since 2026-09-28. A fixture without them
// would exercise a Machine that declares less than the one that runs, and every row would come out
// blank -- which is exactly what the screens did before the declaration existed.
func taskMachineForTest() *domain.Machine {
	return &domain.Machine{
		ID: "mch_task", Name: "Task",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"todo", "in_progress", "done"}},
			{ID: "fld_due_date", Name: "Due", Type: domain.FieldTypeDate},
			{ID: "fld_project", Name: "Project", Type: domain.FieldTypeRelation, RelatedMachine: "mch_project"},
			{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: "mch_user"},
		},
		CardFields: []domain.CardField{
			{Field: "fld_title", Role: domain.CardFieldRoleTitle},
			{Field: "fld_status", Role: domain.CardFieldRoleStatus},
			{Field: "fld_due_date", Role: domain.CardFieldRoleDate},
			{Field: "fld_assignee", Role: domain.CardFieldRolePerson},
		},
		Completion: &domain.Completion{Field: "fld_status", Done: "done"},
	}
}

// projectMachineForTest is the same reduction as taskMachineForTest, for the Machine the dashboard
// composes its Project summaries from: it reads a declared display shape, not a Field id.
func projectMachineForTest() *domain.Machine {
	return &domain.Machine{
		ID: "mch_project", Name: "Project",
		Fields: []domain.Field{
			{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"active", "archived"}},
		},
		CardFields: []domain.CardField{
			{Field: "fld_name", Role: domain.CardFieldRoleTitle},
			{Field: "fld_status", Role: domain.CardFieldRoleStatus},
		},
	}
}
