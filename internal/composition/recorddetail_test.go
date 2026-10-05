package composition

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

func TestBuildRecordActivity_KeepsTheDatasetOrderAndNamesTheActor(t *testing.T) {
	at := time.Date(2026, 10, 3, 9, 20, 0, 0, time.UTC)
	events := []*data.Record{
		{Values: map[string]any{"fld_summary": "moved", "fld_actor": "usr_ana"}, CreatedAt: at},
		{Values: map[string]any{"fld_summary": "created"}, CreatedAt: at.Add(-time.Hour)},
	}
	got := buildRecordActivity(events, map[string]string{"usr_ana": "Ana"})
	if len(got) != 2 || got[0].Summary != "moved" || got[1].Summary != "created" {
		t.Fatalf("order not preserved: %+v", got)
	}
	if got[0].Actor != "Ana" || got[1].Actor != "" {
		t.Errorf("actor = %q / %q, want Ana and empty for an event with no actor", got[0].Actor, got[1].Actor)
	}
	if got[0].When != "2026-10-03 09:20" {
		t.Errorf("When = %q, want the event's own time", got[0].When)
	}
}

// recordExtrasLoader is a Loader over the real metadata/activity.yaml (so the test reads the Dataset that
// ships, not a fixture of it) and a real database.
func recordExtrasLoader(t *testing.T, withActivity bool) (*Loader, *data.Store, context.Context) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run the record-detail integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	store := data.NewStore(pool)
	ctx := data.WithWorkspaceScope(context.Background(), "ws_record_extras_test")
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM records WHERE workspace_id = $1`, "ws_record_extras_test"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	task := &domain.Machine{ID: "mch_task", Fields: []domain.Field{{ID: "fld_title", Type: domain.FieldTypeText}}}
	machines := map[string]*domain.Machine{"mch_task": task}
	if withActivity {
		activity, err := metadata.Load(filepath.Join("..", "..", "metadata", "activity.yaml"))
		if err != nil {
			t.Fatalf("load activity.yaml: %v", err)
		}
		machines["mch_activity"] = activity
	}
	return NewLoader(store, machines), store, ctx
}

// A record's history is the events naming *that Machine and that record*. The id collision is the point:
// a second Machine's record sharing an id would be matched by a filter on fld_record_id alone.
func TestRecordExtrasShowsOnlyThisRecordsEventsNewestFirst(t *testing.T) {
	l, store, ctx := recordExtrasLoader(t, true)
	for _, e := range []map[string]any{
		{"fld_machine_id": "mch_task", "fld_record_id": "rec_a", "fld_summary": "first"},
		{"fld_machine_id": "mch_task", "fld_record_id": "rec_b", "fld_summary": "someone else's card"},
		{"fld_machine_id": "mch_other", "fld_record_id": "rec_a", "fld_summary": "another Machine, same id"},
		{"fld_machine_id": "mch_task", "fld_record_id": "rec_a", "fld_summary": "second"},
	} {
		if _, err := store.CreateRecord(ctx, "mch_activity", e); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	extras, err := l.RecordExtras(ctx, l.Machine("mch_task"), &data.Record{ID: "rec_a"})
	if err != nil {
		t.Fatalf("RecordExtras: %v", err)
	}
	if !extras.ShowActivity || len(extras.Activity) != 2 {
		t.Fatalf("activity = %+v, want exactly this record's two events", extras.Activity)
	}
	if extras.Activity[0].Summary != "second" || extras.Activity[1].Summary != "first" {
		t.Errorf("order = %q, %q; want newest first", extras.Activity[0].Summary, extras.Activity[1].Summary)
	}
	if extras.ActivityTruncation.Hit {
		t.Error("two events inside a bound of 50 reported a truncation")
	}
}

// A Workspace that carries no activity Machine has no history to show; the page must still render.
func TestRecordExtras_AWorkspaceWithoutAnActivityLogHasNoSectionAndNoError(t *testing.T) {
	l, _, ctx := recordExtrasLoader(t, false)
	extras, err := l.RecordExtras(ctx, l.Machine("mch_task"), &data.Record{ID: "rec_a"})
	if err != nil {
		t.Fatalf("RecordExtras: %v", err)
	}
	if extras.ShowActivity {
		t.Error("a Workspace with no mch_activity drew an activity section")
	}
}

// The tag join is drawn as chips, so its own rows are not also a table of opaque ids on the same page.
func TestChildSections_OmitsTheMachineCardTagsDraws(t *testing.T) {
	l, _, ctx := recordExtrasLoader(t, false)
	task := l.Machine("mch_task")
	join := &domain.Machine{ID: "mch_card_label", Fields: []domain.Field{
		{ID: "fld_task", Type: domain.FieldTypeRelation, RelatedMachine: "mch_task"},
	}}
	other := &domain.Machine{ID: "mch_subtask", Fields: []domain.Field{
		{ID: "fld_task", Type: domain.FieldTypeRelation, RelatedMachine: "mch_task"},
	}}
	l = NewLoader(l.store, map[string]*domain.Machine{"mch_task": task, "mch_card_label": join, "mch_subtask": other})
	task.CardTags = &domain.CardTags{Machine: "mch_card_label", Via: "fld_task", Tag: "fld_label"}

	sections, err := l.ChildSections(ctx, task, "rec_a")
	if err != nil {
		t.Fatalf("ChildSections: %v", err)
	}
	if len(sections) != 1 || sections[0].Machine.ID != "mch_subtask" {
		var ids []string
		for _, s := range sections {
			ids = append(ids, s.Machine.ID)
		}
		t.Fatalf("sections = %v, want only mch_subtask", ids)
	}
}
