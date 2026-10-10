package composition

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

func TestBuildRecordActivity_KeepsTheDatasetOrderAndNamesTheActor(t *testing.T) {
	at := time.Date(2026, 10, 3, 9, 20, 0, 0, time.UTC)
	events := []*data.Record{
		{Values: map[string]any{"fld_summary": "moved", "fld_actor": "usr_ana"}, CreatedAt: at},
		{Values: map[string]any{"fld_summary": "created"}, CreatedAt: at.Add(-time.Hour)},
	}
	got := buildRecordActivity(events, commentThread{}, map[string]string{"usr_ana": "Ana"})
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

	extras, err := l.RecordExtras(ctx, l.Machine("mch_task"), &data.Record{ID: "rec_a"}, nil)
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
	extras, err := l.RecordExtras(ctx, l.Machine("mch_task"), &data.Record{ID: "rec_a"}, nil)
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

// moveFixture is a Task Machine with a board View grouped by a relation to a List Machine, over a real database.
func moveFixture(t *testing.T) (*Loader, *domain.Machine, context.Context, map[string]*data.Record) {
	t.Helper()
	_, store, ctx := recordExtrasLoader(t, false)
	list := &domain.Machine{ID: "mch_list", Fields: []domain.Field{{ID: "fld_name", Type: domain.FieldTypeText}}}
	task := &domain.Machine{ID: "mch_task",
		Fields: []domain.Field{
			{ID: "fld_title", Type: domain.FieldTypeText},
			{ID: "fld_list", Type: domain.FieldTypeRelation, RelatedMachine: "mch_list"},
		},
		Views: []domain.View{{ID: "vw_table", Type: domain.ViewTable}, {ID: "vw_board", Type: domain.ViewBoard, GroupBy: "fld_list"}},
	}
	made := map[string]*data.Record{}
	create := func(key, machine string, values map[string]any) {
		r, err := store.CreateRecord(ctx, machine, values)
		if err != nil {
			t.Fatalf("create %s: %v", key, err)
		}
		made[key] = r
	}
	create("backlog", "mch_list", map[string]any{"fld_name": "Backlog"})
	create("doing", "mch_list", map[string]any{"fld_name": "Doing"})
	create("a", "mch_task", map[string]any{"fld_title": "A", "fld_list": made["backlog"].ID})
	create("b", "mch_task", map[string]any{"fld_title": "B", "fld_list": made["backlog"].ID})
	create("c", "mch_task", map[string]any{"fld_title": "C", "fld_list": made["backlog"].ID})
	create("d", "mch_task", map[string]any{"fld_title": "D", "fld_list": made["doing"].ID})
	return NewLoader(store, map[string]*domain.Machine{"mch_task": task, "mch_list": list}), task, ctx, made
}

// The detail page's Move panel is the board's own move: the lists a card may go to, the place this card holds
// among the cards of its own list, and the board View the PATCH has to name so the server places it as a drop would.
func TestRecordExtras_MoveNamesTheListsTheCurrentOneAndThePositionInIt(t *testing.T) {
	l, task, ctx, made := moveFixture(t)
	extras, err := l.RecordExtras(ctx, task, made["b"], nil)
	if err != nil {
		t.Fatalf("RecordExtras: %v", err)
	}
	mv := extras.Move
	if mv == nil {
		t.Fatal("a Task on a board drew no Move panel")
	}
	if mv.ViewID != "vw_board" || mv.Field != "fld_list" {
		t.Errorf("move addresses view %q field %q, want vw_board / fld_list", mv.ViewID, mv.Field)
	}
	if mv.Current != made["backlog"].ID || mv.Position != 2 {
		t.Errorf("current %q position %d, want Backlog's id and 2 (B is the second card of its list)", mv.Current, mv.Position)
	}
	var labels []string
	for _, tg := range mv.Targets {
		labels = append(labels, tg.Label)
	}
	if !reflect.DeepEqual(labels, []string{"Backlog", "Doing"}) {
		t.Errorf("targets = %v, want both Lists in board order", labels)
	}
}

// Position is read from the record's own list: one bounded statement, not the whole Machine.
func TestRecordExtras_MoveReadsOnlyTheRecordsOwnList(t *testing.T) {
	l, task, ctx, made := moveFixture(t)
	if _, err := l.RecordExtras(ctx, task, made["d"], nil); err != nil {
		t.Fatalf("RecordExtras: %v", err)
	}
	if _, whole := l.listed["mch_task"]; whole {
		t.Error("the Move panel read every Task to find one card's position")
	}
}

// A Machine with no board View, or a card whose list no longer exists, is offered no move: a panel opening on a
// blank List would say the card is somewhere it is not.
func TestRecordExtras_NoMoveWithoutABoardOrWithoutAKnownList(t *testing.T) {
	l, task, ctx, made := moveFixture(t)
	task.Views = []domain.View{{ID: "vw_table", Type: domain.ViewTable}}
	extras, err := l.RecordExtras(ctx, task, made["a"], nil)
	if err != nil || extras.Move != nil {
		t.Errorf("a Machine with no board drew a Move panel (err %v)", err)
	}

	task.Views = []domain.View{{ID: "vw_board", Type: domain.ViewBoard, GroupBy: "fld_list"}}
	orphan := &data.Record{ID: "rec_orphan", Values: map[string]any{"fld_list": "lst_deleted"}}
	extras, err = l.RecordExtras(ctx, task, orphan, nil)
	if err != nil || extras.Move != nil {
		t.Errorf("a card in a deleted list drew a Move panel (err %v)", err)
	}
}

func checklistMachine() *domain.Machine {
	return &domain.Machine{ID: "mch_checklist_item",
		Fields: []domain.Field{
			{ID: "fld_task", Type: domain.FieldTypeRelation, RelatedMachine: "mch_task"},
			{ID: "fld_text", Type: domain.FieldTypeText},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}, Default: "todo"},
		},
		Completion: &domain.Completion{Field: "fld_status", Done: "done"},
		CardFields: []domain.CardField{{Field: "fld_text", Role: domain.CardFieldRoleTitle}},
	}
}

// A child Machine that says what finished means and which Field is an item's text is a checklist: the count
// is the finished ones, each circle writes the opposite of what its item holds, and the add form is told which
// Field names the parent.
func TestChecklistFor_CountsFinishedItemsAndResolvesEachCircle(t *testing.T) {
	m := checklistMachine()
	recs := []*data.Record{
		{ID: "a", Values: map[string]any{"fld_text": "One", "fld_status": "done"}},
		{ID: "b", Values: map[string]any{"fld_text": "Two", "fld_status": "todo"}},
	}
	c := checklistFor(m, "fld_task", recs)
	if c == nil {
		t.Fatal("a Machine with completion and a title role was not drawn as a checklist")
	}
	if c.Done != 1 || len(c.Items) != 2 || c.ParentField != "fld_task" || c.TextField != "fld_text" {
		t.Fatalf("checklist = %+v", c)
	}
	if got := c.Items[0].Complete; !got.Done || got.Next != "todo" || got.Field != "fld_status" {
		t.Errorf("a finished item's circle = %+v, want to reopen it to todo", got)
	}
	if got := c.Items[1].Complete; got.Done || got.Next != "done" {
		t.Errorf("an open item's circle = %+v, want to finish it", got)
	}
}

// Either declaration missing keeps the table: a Machine with no completion has no finished items, and one with
// no title role has no text to draw.
func TestChecklistFor_NeedsBothDeclarations(t *testing.T) {
	noCompletion := checklistMachine()
	noCompletion.Completion = nil
	noTitle := checklistMachine()
	noTitle.CardFields = nil
	for name, m := range map[string]*domain.Machine{"no completion": noCompletion, "no title role": noTitle} {
		if checklistFor(m, "fld_task", nil) != nil {
			t.Errorf("%s: drawn as a checklist", name)
		}
	}
}

func attachmentMachine() *domain.Machine {
	return &domain.Machine{ID: "mch_attachment",
		Fields: []domain.Field{
			{ID: "fld_task", Type: domain.FieldTypeRelation, RelatedMachine: "mch_task"},
			{ID: "fld_file", Type: domain.FieldTypeFile},
		},
		CardFields: []domain.CardField{{Field: "fld_file", Role: domain.CardFieldRoleFile}},
	}
}

// A child Machine giving a file Field the `file` role is a list of uploads: each row carries the name the
// storage key embeds, the extension as its chip, when the record was made and how big the file on disk is.
func TestAttachmentsFor_NamesEachUploadAndReadsItsSize(t *testing.T) {
	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, err := files.Save("mch_attachment", "fld_file", "shooting-schedule.pdf", strings.NewReader(strings.Repeat("x", 2_400_000)))
	if err != nil {
		t.Fatal(err)
	}
	recs := []*data.Record{
		{ID: "a", CreatedAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC), Values: map[string]any{"fld_file": key}},
		{ID: "b", Values: map[string]any{}}, // no upload: not a row
	}
	a := attachmentsFor(attachmentMachine(), "fld_task", recs, files)
	if a == nil || len(a.Items) != 1 || a.ParentField != "fld_task" || a.FileField != "fld_file" {
		t.Fatalf("attachments = %+v", a)
	}
	got := a.Items[0]
	if got.ID != "a" || got.Name != "shooting-schedule.pdf" || got.Kind != "PDF" || got.Added != "02 Oct" || got.Size != "2.4 MB" || got.Key != key {
		t.Errorf("item = %+v", got)
	}
	if blank := attachmentsFor(attachmentMachine(), "fld_task", recs, nil); blank.Items[0].Size != "" {
		t.Errorf("a Loader with no store should leave the size blank, got %q", blank.Items[0].Size)
	}
}

// Without the role the Machine keeps its table.
func TestAttachmentsFor_NeedsTheFileRole(t *testing.T) {
	m := attachmentMachine()
	m.CardFields = nil
	if attachmentsFor(m, "fld_task", nil, nil) != nil {
		t.Error("drawn as attachments with no file role")
	}
}

func TestHumanSizeAndKind(t *testing.T) {
	for n, want := range map[int64]string{0: "", -3: "", 512: "512 B", 36_000: "36 KB", 840_400: "840 KB", 2_400_000: "2.4 MB"} {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
	for key, want := range map[string]string{"m/f/ab__plan.pdf": "PDF", "m/f/ab__book.xlsx": "XLSX", "m/f/ab__notes": "FILE", "m/f/ab__a.markdown": "MARK"} {
		if got := attachmentKind(key); got != want {
			t.Errorf("attachmentKind(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestBuildRecordActivity_MergesCommentsWithEventsNewestFirst(t *testing.T) {
	at := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	events := []*data.Record{
		{Values: map[string]any{"fld_summary": "moved"}, CreatedAt: at.Add(2 * time.Hour)},
		{Values: map[string]any{"fld_summary": "created"}, CreatedAt: at},
	}
	thread := commentThread{body: "fld_body", author: "fld_author", records: []*data.Record{
		{Values: map[string]any{"fld_body": "looks good", "fld_author": "usr_ana"}, CreatedAt: at.Add(time.Hour)},
	}}
	got := buildRecordActivity(events, thread, map[string]string{"usr_ana": "Ana"})
	if len(got) != 3 || got[0].Summary != "moved" || got[1].Summary != "looks good" || got[2].Summary != "created" {
		t.Fatalf("merged order = %+v, want moved, looks good, created", got)
	}
	if !got[1].Comment || got[1].Actor != "Ana" || got[0].Comment || got[2].Comment {
		t.Errorf("Comment/Actor flags wrong: %+v", got)
	}
}

func commentsFixture(task *domain.Machine) *domain.Machine {
	return &domain.Machine{ID: "mch_comment", Fields: []domain.Field{
		{ID: "fld_task", Type: domain.FieldTypeRelation, RelatedMachine: task.ID},
		{ID: "fld_body", Type: domain.FieldTypeLongText},
		{ID: "fld_author", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID, Stamp: domain.FieldStampCurrentUser},
	}, CardFields: []domain.CardField{{Field: "fld_body", Role: domain.CardFieldRoleComment}}}
}

// A Machine with a comment child collection gets the composer and its comments in the feed, with or without
// an activity log, and the comments are not also a child table.
func TestRecordExtras_CommentsJoinTheFeedAndAreNotAChildTable(t *testing.T) {
	base, store, ctx := recordExtrasLoader(t, false)
	task := base.Machine("mch_task")
	l := NewLoader(store, map[string]*domain.Machine{"mch_task": task, "mch_comment": commentsFixture(task)})
	if _, err := store.CreateRecord(ctx, "mch_comment", map[string]any{"fld_task": "rec_a", "fld_body": "ship it", "fld_author": "usr_ana"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRecord(ctx, "mch_comment", map[string]any{"fld_task": "rec_b", "fld_body": "someone else's", "fld_author": "usr_ana"}); err != nil {
		t.Fatal(err)
	}

	extras, err := l.RecordExtras(ctx, task, &data.Record{ID: "rec_a"}, nil)
	if err != nil {
		t.Fatalf("RecordExtras: %v", err)
	}
	if extras.Comments == nil || extras.Comments.MachineID != "mch_comment" || extras.Comments.ParentField != "fld_task" || extras.Comments.BodyField != "fld_body" {
		t.Fatalf("composer = %+v", extras.Comments)
	}
	if len(extras.Activity) != 1 || extras.Activity[0].Summary != "ship it" || !extras.Activity[0].Comment {
		t.Errorf("feed = %+v, want exactly this record's one comment", extras.Activity)
	}
	sections, err := l.ChildSections(ctx, task, "rec_a")
	if err != nil {
		t.Fatalf("ChildSections: %v", err)
	}
	if len(sections) != 0 {
		t.Errorf("comments were also drawn as a child section: %+v", sections)
	}
}

// The Copy panel starts from the record's own name, marked as a copy, and is offered only where the Machine can
// be copied at all.
func TestRecordCopy_NamesTheCopyAfterTheRecordAndIsAbsentWhereACopyIsNotAllowed(t *testing.T) {
	m := &domain.Machine{ID: "mch_task", CardFields: []domain.CardField{{Field: "fld_title", Role: domain.CardFieldRoleTitle}}}
	r := &data.Record{ID: "rec_1", Values: map[string]any{"fld_title": "Call sheet"}}
	cp := recordCopy(m, r)
	if cp == nil || cp.TitleField != "fld_title" || cp.Title != "Call sheet (copy)" {
		t.Fatalf("recordCopy = %+v, want fld_title / %q", cp, "Call sheet (copy)")
	}
	m.AppendOnly = true
	if recordCopy(m, r) != nil {
		t.Error("an append-only Machine was offered a copy")
	}
}

// The Move panel offers every further relation Field of the Machine beside the List (a Task's Project), from the
// options the page already loaded, and a person Field is not one; Position offers the list's cards plus one.
func TestRecordExtras_MoveOffersOtherRelationsAndOneSlotPastTheList(t *testing.T) {
	l, task, ctx, made := moveFixture(t)
	task.Fields = append(task.Fields,
		domain.Field{ID: "fld_project", Name: "Project", Type: domain.FieldTypeRelation, RelatedMachine: "mch_project"},
		domain.Field{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
	)
	relations := rendering.RelationOptions{
		"mch_project":        {{ID: "prj_1", Label: "Launch"}, {ID: "prj_2", Label: "Retro"}},
		domain.UserMachineID: {{ID: "usr_1", Label: "Ana"}},
	}
	extras, err := l.RecordExtras(ctx, task, made["b"], relations)
	if err != nil {
		t.Fatalf("RecordExtras: %v", err)
	}
	mv := extras.Move
	if mv == nil || len(mv.Scopes) != 1 {
		t.Fatalf("scopes = %+v, want exactly the Project (the List is the group Field and the assignee is a person)", mv)
	}
	if sc := mv.Scopes[0]; sc.Field != "fld_project" || sc.Label != "Project" || len(sc.Targets) != 2 {
		t.Errorf("scope = %+v, want Project with its two records", sc)
	}
	if mv.Positions != 4 {
		t.Errorf("positions = %d, want 4 (three cards in Backlog, plus one to put it last)", mv.Positions)
	}
}
