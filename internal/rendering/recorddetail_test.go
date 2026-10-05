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

// The ⋯ menu is on every record's page because Share is a link to the page itself; Move is only in it for a
// record that can be moved, so the menu never offers something the server would refuse.
func TestRecordDetail_ActionsMenuAlwaysSharesAndOnlyOffersMoveWhenItCan(t *testing.T) {
	plain := renderDetail(t, RecordExtras{})
	if !strings.Contains(plain, `aria-label="Card actions"`) || !strings.Contains(plain, "Copy a link to this card") {
		t.Error("a record with nothing to move still has the menu, with Share in it")
	}
	if !strings.Contains(plain, "navigator.clipboard.writeText(location.href)") {
		t.Error("Share must copy the page's own address")
	}
	if strings.Contains(plain, "Move…") {
		t.Error("a record with no board drew Move")
	}
	if !strings.Contains(renderDetail(t, RecordExtras{Move: moveForDetail()}), "Move…") {
		t.Error("a record on a board lost Move")
	}
}

func renderDetailPage(t *testing.T, extras RecordExtras) string {
	t.Helper()
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: resolvedNav([]domain.NavigationItem{
		{ID: "nav_tasks", Label: "Tasks board", Route: "/machines/mch_task"},
	})}, "Test Workspace", false)
	m := &domain.Machine{ID: "mch_task", Name: "Task", Fields: []domain.Field{{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText}}}
	r := &data.Record{ID: "rec_1", Values: map[string]any{"fld_title": "Lock shooting schedule"}}
	var buf bytes.Buffer
	if err := RecordDetailPage(m, r, nil, nil, nil, domain.Actor{ID: "usr_ana"}, nil, "Acme", Viewer{Initials: "AN"}, "", extras, time.Now()).Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// PM02's "Tasks board / Pre-production": the Machine's heading links back to the board View the card is on, and
// the second half is the list it is in now -- the same answer the Move panel selects, so the two cannot disagree.
func TestRecordDetailPage_BreadcrumbNamesTheBoardAndTheCurrentList(t *testing.T) {
	html := renderDetailPage(t, RecordExtras{Move: moveForDetail()})
	for _, want := range []string{`aria-label="Breadcrumb"`, `href="/machines/mch_task?view=vw_board"`, ">Tasks board<", `aria-current="page">Doing<`} {
		if !strings.Contains(html, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	if strings.Contains(renderDetailPage(t, RecordExtras{}), `href="/machines/mch_task?view=`) {
		t.Error("a Machine with no board has no list to name, so it keeps its plain back link")
	}
}

func TestRecordDetail_LongTextKeepsItsLineBreaks(t *testing.T) {
	m := &domain.Machine{ID: "mch_task", Name: "Task", Fields: []domain.Field{
		{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
		{ID: "fld_description", Name: "Description", Type: domain.FieldTypeLongText},
	}}
	r := &data.Record{ID: "rec_1", Values: map[string]any{"fld_title": "T", "fld_description": "line one\nline two"}}
	var buf bytes.Buffer
	if err := RecordDetailView(m, r, nil, nil, nil, domain.Actor{ID: "usr_ana"}, nil, RecordExtras{}, time.Now()).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), `whitespace-pre-wrap">line one`+"\n"+`line two</div>`) {
		t.Errorf("a long_text value is not drawn with its newline intact:\n%s", buf.String())
	}
}

func TestFieldInput_LongTextIsATextareaPrefilledWithItsValue(t *testing.T) {
	var buf bytes.Buffer
	f := domain.Field{ID: "fld_description", Type: domain.FieldTypeLongText}
	if err := fieldInput(f, "a\nb", nil, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `<textarea name="fld_description"`) || !strings.Contains(got, ">a\nb</textarea>") {
		t.Errorf("long_text drew %q, want a textarea holding the value", got)
	}
}

func checklistFixture() *Checklist {
	return &Checklist{ParentField: "fld_task", TextField: "fld_text", Done: 1, Items: []ChecklistItem{
		{ID: "a", Text: "One", Complete: CardComplete{Field: "fld_status", Next: "todo", Done: true}},
		{ID: "b", Text: "Two", Complete: CardComplete{Field: "fld_status", Next: "done"}},
	}}
}

func renderChecklist(t *testing.T, canEdit bool) string {
	t.Helper()
	var buf strings.Builder
	m := &domain.Machine{ID: "mch_checklist_item", Name: "Checklist"}
	if err := checklistSection(m, "tsk_1", checklistFixture(), canEdit).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// The section says how far along the list is, writes each circle through the item's own record route, and adds
// the next item through the Machine's create route naming the parent.
func TestChecklistSection_DrawsCountCirclesAndAddForm(t *testing.T) {
	out := renderChecklist(t, true)
	for _, want := range []string{
		"1 of 2",
		`hx-patch="/machines/mch_checklist_item/records/a"`,
		`name="fld_status" value="todo"`, `name="fld_status" value="done"`,
		`hx-post="/machines/mch_checklist_item/records"`,
		`name="fld_task" value="tsk_1"`, `name="fld_text"`, "Add an item",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("checklist is missing %q\n%s", want, out)
		}
	}
}

// Someone who cannot edit the Task sees the list and its count but no control that would be refused.
func TestChecklistSection_HidesWritesFromSomeoneWhoCannotEdit(t *testing.T) {
	out := renderChecklist(t, false)
	if !strings.Contains(out, "One") || !strings.Contains(out, "1 of 2") {
		t.Fatal("a read-only checklist must still show its items and count")
	}
	for _, banned := range []string{"hx-patch", "hx-post", "Add an item"} {
		if strings.Contains(out, banned) {
			t.Errorf("read-only checklist drew %q", banned)
		}
	}
}

func renderAttachments(t *testing.T, canEdit bool) string {
	t.Helper()
	a := &Attachments{ParentField: "fld_task", FileField: "fld_file", Items: []AttachmentItem{
		{ID: "a", Key: "mch_attachment/fld_file/ab__plan.pdf", Name: "plan.pdf", Kind: "PDF", Added: "02 Oct", Size: "2.4 MB"},
		{ID: "b", Key: "mch_attachment/fld_file/cd__gone.png", Name: "gone.png", Kind: "PNG", Added: "01 Oct"},
	}}
	var buf strings.Builder
	m := &domain.Machine{ID: "mch_attachment", Name: "Attachments"}
	if err := attachmentsSection(m, "tsk_1", a, canEdit).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// Each upload is a row (chip, name, "Added ... · size", Download), the size is dropped rather than faked
// when unknown, and the add form is a multipart create naming the parent that submits on selection.
func TestAttachmentsSection_DrawsRowsRemoveAndAddForm(t *testing.T) {
	out := renderAttachments(t, true)
	for _, want := range []string{
		"Attachments", "plan.pdf", ">PDF<", "Added 02 Oct · 2.4 MB", `href="/uploads/mch_attachment/fld_file/ab__plan.pdf"`, "Download",
		"Added 01 Oct<", // no size: no separator either
		`hx-delete="/machines/mch_attachment/records/a"`, `hx-target="closest li"`,
		`hx-post="/machines/mch_attachment/records"`, `hx-encoding="multipart/form-data"`, `hx-trigger="change"`,
		`name="fld_task" value="tsk_1"`, `type="file" name="fld_file"`, "Add an attachment",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("attachments are missing %q\n%s", want, out)
		}
	}
}

// Someone who cannot edit the Task still sees and downloads the files, with nothing that would be refused.
func TestAttachmentsSection_HidesWritesFromSomeoneWhoCannotEdit(t *testing.T) {
	out := renderAttachments(t, false)
	if !strings.Contains(out, "plan.pdf") || !strings.Contains(out, "Download") {
		t.Fatal("a read-only list must still show and link its files")
	}
	for _, banned := range []string{"hx-delete", "hx-post", "Add an attachment", `type="file"`} {
		if strings.Contains(out, banned) {
			t.Errorf("read-only list drew %q", banned)
		}
	}
}

func commentExtras() RecordExtras {
	return RecordExtras{
		ShowActivity: true,
		Comments:     &CommentComposer{MachineID: "mch_comment", ParentField: "fld_task", BodyField: "fld_body"},
		Activity: []ActivityEntry{
			{Summary: "Looks good, shipping it.", Actor: "Ana Putri", When: "2026-10-03 09:20", Comment: true},
			{Summary: `"Lock shooting schedule" moved`, When: "2026-10-03 08:00"},
		},
	}
}

func TestRecordDetail_CommentsFeedDrawsComposerCommentsAndHidableEvents(t *testing.T) {
	html := renderDetail(t, commentExtras())
	for _, want := range []string{
		"Comments and activity", "Write a comment", `hx-post="/machines/mch_comment/records"`,
		`name="fld_task" value="rec_1"`, `name="fld_body"`,
		"Looks good, shipping it.", "Ana Putri", ">AP<", "Show details", "moved",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("detail is missing %q", want)
		}
	}
	if strings.Contains(html, ">History<") {
		t.Error("a record with a comment box still drew the plain History heading")
	}
	if strings.Contains(html, `name="fld_author"`) {
		t.Error("the comment box offered an author input")
	}
	if !strings.Contains(html, "group-has-[:checked]/feed:block") {
		t.Error("recorded events are not behind the Show details toggle")
	}
}

func TestRecordDetail_CommentsFeedOffersNoToggleWhenThereAreNoEvents(t *testing.T) {
	e := commentExtras()
	e.Activity = e.Activity[:1]
	if html := renderDetail(t, e); strings.Contains(html, "Show details") {
		t.Error("a feed of only comments offered a toggle with nothing behind it")
	}
}

func TestRecordDetail_CommentsFeedWithoutActivityLogStillDrawsTheBox(t *testing.T) {
	e := commentExtras()
	e.ShowActivity = false
	if html := renderDetail(t, e); !strings.Contains(html, "Write a comment") {
		t.Error("a Workspace with no activity log lost its comment box")
	}
}

func TestRecordDetail_CopyPanelPostsToTheCopyRouteAndNamesTheTitleField(t *testing.T) {
	html := renderDetail(t, RecordExtras{Copy: &RecordCopy{TitleField: "fld_title", Title: "Lock shooting schedule (copy)"}})
	for _, want := range []string{"Copy…", `hx-post="/machines/mch_task/records/rec_1/copy"`, `name="fld_title"`, `value="Lock shooting schedule (copy)"`} {
		if !strings.Contains(html, want) {
			t.Errorf("the Copy panel is missing %q", want)
		}
	}
	if strings.Contains(renderDetail(t, RecordExtras{}), "Copy…") {
		t.Error("a record whose Machine cannot be copied drew a Copy panel")
	}
}

// Copying is a create, so the panel follows the create Permission the way Move follows Edit.
func TestRecordDetail_CopyPanelIsHiddenFromAnActorWhoMayNotCreate(t *testing.T) {
	m := &domain.Machine{ID: "mch_task", Name: "Task",
		Fields:      []domain.Field{{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText}, {ID: "fld_owner", Name: "Owner", Type: domain.FieldTypePerson}},
		Permissions: []domain.Permission{{Action: domain.ActionCreate, ActorField: "fld_owner"}},
	}
	r := &data.Record{ID: "rec_1", Values: map[string]any{"fld_title": "T", "fld_owner": "usr_other"}}
	var buf bytes.Buffer
	if err := RecordDetailView(m, r, nil, nil, nil, domain.Actor{ID: "usr_ana"}, nil, RecordExtras{Copy: &RecordCopy{TitleField: "fld_title", Title: "T (copy)"}}, time.Now()).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "Copy…") {
		t.Error("an actor the create permission refuses was offered a copy")
	}
}
