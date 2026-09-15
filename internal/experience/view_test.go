package experience

import (
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func taskMachine() *domain.Machine {
	return &domain.Machine{
		ID: "mch_task",
		Fields: []domain.Field{
			{ID: "fld_title", Type: domain.FieldTypeText},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "in_progress", "done"}},
		},
		View: domain.View{Layout: domain.LayoutBoard, GroupBy: "fld_status"},
	}
}

func TestGroupRecords_ordersColumnsByFieldOptions(t *testing.T) {
	m := taskMachine()
	records := []*data.Record{
		{ID: "rec_1", Values: map[string]any{"fld_status": "done"}},
		{ID: "rec_2", Values: map[string]any{"fld_status": "todo"}},
		{ID: "rec_3", Values: map[string]any{"fld_status": "todo"}},
	}

	columns := GroupRecords(m, records, nil)

	if len(columns) != 3 {
		t.Fatalf("len(columns) = %d, want 3 (todo, in_progress, done)", len(columns))
	}
	if columns[0].Label != "todo" || len(columns[0].Records) != 2 {
		t.Errorf("columns[0] = %+v, want todo with 2 records", columns[0])
	}
	if columns[1].Label != "in_progress" || len(columns[1].Records) != 0 {
		t.Errorf("columns[1] = %+v, want in_progress with 0 records", columns[1])
	}
	if columns[2].Label != "done" || len(columns[2].Records) != 1 {
		t.Errorf("columns[2] = %+v, want done with 1 record", columns[2])
	}
}

func TestGroupRecords_unknownValueGoesToOther(t *testing.T) {
	m := taskMachine()
	records := []*data.Record{
		{ID: "rec_1", Values: map[string]any{"fld_status": "archived"}},
	}

	columns := GroupRecords(m, records, nil)

	last := columns[len(columns)-1]
	if last.Label != "Other" || len(last.Records) != 1 {
		t.Errorf("last column = %+v, want Other with 1 record", last)
	}
}

func TestGroupRecords_unknownGroupByField(t *testing.T) {
	m := taskMachine()
	m.View.GroupBy = "fld_ghost"

	if columns := GroupRecords(m, nil, nil); columns != nil {
		t.Errorf("GroupRecords() = %+v, want nil for an unknown group-by field", columns)
	}
}

func TestGroupRecords_relationBasedColumns(t *testing.T) {
	m := &domain.Machine{
		ID:   "mch_task",
		View: domain.View{Layout: domain.LayoutBoard, GroupBy: "fld_list"},
	}
	columns := []Column{
		{ID: "rec_list_todo", Label: "To Do"},
		{ID: "rec_list_done", Label: "Done"},
	}
	records := []*data.Record{
		{ID: "rec_1", Values: map[string]any{"fld_list": "rec_list_done"}},
		{ID: "rec_2", Values: map[string]any{"fld_list": "rec_list_todo"}},
	}

	got := GroupRecords(m, records, columns)

	if len(got) != 2 {
		t.Fatalf("len(columns) = %d, want 2 (no GroupBy Field lookup needed for the relation case)", len(got))
	}
	if got[0].Label != "To Do" || len(got[0].Records) != 1 || got[0].Records[0].ID != "rec_2" {
		t.Errorf("columns[0] = %+v, want To Do with rec_2", got[0])
	}
	if got[1].Label != "Done" || len(got[1].Records) != 1 || got[1].Records[0].ID != "rec_1" {
		t.Errorf("columns[1] = %+v, want Done with rec_1", got[1])
	}
}
