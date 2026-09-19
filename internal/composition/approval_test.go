package composition

import (
	"testing"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/data"
)

// These are the first tests this logic has ever had. It lived in cmd/server/main.go until Phase
// 19, in a package that cannot hold a test file at all -- 146 lines deciding who sees what in an
// approval queue, verified only by looking at the page.

func rec(id string, values map[string]any) *data.Record {
	return &data.Record{ID: id, Values: values}
}

func at(day int) time.Time { return time.Date(2026, 9, day, 12, 0, 0, 0, time.UTC) }

func event(docID, actor string, when time.Time) *data.Record {
	return &data.Record{
		ID:        "act_" + docID + actor,
		Values:    map[string]any{"fld_record_id": docID, "fld_actor": actor},
		CreatedAt: when,
	}
}

func step(id, docID, assignee, decision string, seq float64) *data.Record {
	return rec(id, map[string]any{
		action.FieldStepDocument: docID,
		action.FieldStepDecision: decision,
		action.FieldStepSequence: seq,
		"fld_assignee":           assignee,
	})
}

func doc(id, title, mode, due string) *data.Record {
	return rec(id, map[string]any{
		"fld_title":                title,
		action.FieldDocumentMode:   mode,
		"fld_due_date":             due,
		action.FieldDocumentStatus: "in_review",
	})
}

var users = []*data.Record{
	rec("usr_ana", map[string]any{"fld_name": "Ana Putri"}),
	rec("usr_budi", map[string]any{"fld_name": "Budi"}),
}

// A sequential Document only makes its earliest undecided step actionable; a parallel one makes
// every pending step actionable at once. The inbox must not show a step that decideStep would
// then refuse, which is the whole reason it consults action.CanDecide rather than fld_decision.
func TestBuildInbox_SequentialLocksLaterSteps(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "sequential", "")}
	steps := []*data.Record{
		step("stp_1", "doc_1", "usr_budi", action.DecisionPending, 1),
		step("stp_2", "doc_1", "usr_ana", action.DecisionPending, 2),
	}

	got := buildInbox(steps, docs, nil, users, "usr_ana", at(10))
	if len(got.Pending) != 0 {
		t.Errorf("step 2 is locked behind step 1, so it must not appear as pending; got %d card(s)", len(got.Pending))
	}

	// Same records, parallel mode: nothing is waiting on anything.
	docs[0].Values[action.FieldDocumentMode] = "parallel"
	got = buildInbox(steps, docs, nil, users, "usr_ana", at(10))
	if len(got.Pending) != 1 {
		t.Fatalf("parallel mode makes every pending step actionable; got %d card(s)", len(got.Pending))
	}
}

func TestBuildInbox_SkipsOtherPeopleAndDecidedSteps(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{
		step("stp_mine_done", "doc_1", "usr_ana", action.DecisionApproved, 1),
		step("stp_theirs", "doc_1", "usr_budi", action.DecisionPending, 2),
		step("stp_mine", "doc_1", "usr_ana", action.DecisionPending, 3),
	}

	got := buildInbox(steps, docs, nil, users, "usr_ana", at(10))
	if len(got.Pending) != 1 {
		t.Fatalf("want only my own still-pending step, got %d", len(got.Pending))
	}
	if want := "/machines/" + action.StepMachineID + "/records/stp_mine"; got.Pending[0].Href != want {
		t.Errorf("Href = %q, want %q", got.Pending[0].Href, want)
	}
	// One of three steps is approved, and the card says so.
	if want := "parallel · 1/3 approved · Submitted by someone"; got.Pending[0].Subtitle != want {
		t.Errorf("Subtitle = %q, want %q", got.Pending[0].Subtitle, want)
	}
}

// The SLA bucket is computed per day, not per instant: a Document due today is "today" even when
// its due date is hours in the past, and only the day after does it become overdue.
func TestBuildInbox_BucketsByDay(t *testing.T) {
	// now is 2026-09-10 noon throughout.
	for _, tc := range []struct {
		name string
		due  string
		want string
	}{
		{"due earlier today", "2026-09-10", BucketToday},
		{"due tomorrow", "2026-09-11", BucketUpcoming},
		{"due yesterday", "2026-09-09", BucketOverdue},
		{"no due date", "", BucketUpcoming},
		{"unparseable due date", "not-a-date", BucketUpcoming},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docs := []*data.Record{doc("doc_1", "Contract", "parallel", tc.due)}
			steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}

			got := buildInbox(steps, docs, nil, users, "usr_ana", at(10))
			if len(got.Buckets) != 1 {
				t.Fatalf("want one card, got %d", len(got.Buckets))
			}
			if got.Buckets[0] != tc.want {
				t.Errorf("bucket = %q, want %q", got.Buckets[0], tc.want)
			}
		})
	}
}

func TestBuildInbox_CountsMatchBuckets(t *testing.T) {
	docs := []*data.Record{
		doc("doc_over", "Late", "parallel", "2026-09-09"),
		doc("doc_today", "Now", "parallel", "2026-09-10"),
		doc("doc_later", "Soon", "parallel", "2026-09-20"),
	}
	steps := []*data.Record{
		step("stp_1", "doc_over", "usr_ana", action.DecisionPending, 1),
		step("stp_2", "doc_today", "usr_ana", action.DecisionPending, 1),
		step("stp_3", "doc_later", "usr_ana", action.DecisionPending, 1),
	}

	got := buildInbox(steps, docs, nil, users, "usr_ana", at(10))
	if got.OverdueCount != 1 || got.TodayCount != 1 {
		t.Errorf("OverdueCount/TodayCount = %d/%d, want 1/1", got.OverdueCount, got.TodayCount)
	}
	if len(got.Pending) != 3 {
		t.Errorf("all three stay in Pending; the filter is applied by the caller, got %d", len(got.Pending))
	}
}

// "Submitted by" comes from the activity log's earliest event per Document. A later event must
// not overwrite it, and a Document with no logged event at all falls back rather than rendering
// an empty name.
func TestBuildInbox_SubmitterFromEarliestEvent(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}
	activities := []*data.Record{
		event("doc_1", "usr_ana", at(9)),  // a later decision by someone else
		event("doc_1", "usr_budi", at(8)), // the actual submission, logged first
	}

	got := buildInbox(steps, docs, activities, users, "usr_ana", at(10))
	if len(got.Pending) != 1 {
		t.Fatalf("want one card, got %d", len(got.Pending))
	}
	if want := "parallel · 0/1 approved · Submitted by Budi"; got.Pending[0].Subtitle != want {
		t.Errorf("Subtitle = %q, want %q", got.Pending[0].Subtitle, want)
	}
	if got.Pending[0].AvatarInitials != "B" {
		t.Errorf("AvatarInitials = %q, want %q", got.Pending[0].AvatarInitials, "B")
	}
}

func TestBuildInbox_UnknownSubmitterFallsBack(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}

	got := buildInbox(steps, docs, nil, users, "usr_ana", at(10))
	if want := "parallel · 0/1 approved · Submitted by someone"; got.Pending[0].Subtitle != want {
		t.Errorf("Subtitle = %q, want %q", got.Pending[0].Subtitle, want)
	}
	if got.Pending[0].AvatarInitials != "S" {
		t.Errorf("AvatarInitials = %q, want %q", got.Pending[0].AvatarInitials, "S")
	}
}

// Mine answers a different question from Pending -- "what did I submit", not "what waits on me"
// -- so it is keyed off the activity log and ignores assignment entirely.
func TestBuildInbox_MineIsWhatISubmitted(t *testing.T) {
	docs := []*data.Record{
		doc("doc_mine", "Mine", "parallel", ""),
		doc("doc_theirs", "Theirs", "parallel", ""),
	}
	activities := []*data.Record{
		event("doc_mine", "usr_ana", at(8)),
		event("doc_theirs", "usr_budi", at(8)),
	}

	got := buildInbox(nil, docs, activities, users, "usr_ana", at(10))
	if len(got.Mine) != 1 {
		t.Fatalf("want one submitted Document, got %d", len(got.Mine))
	}
	if got.Mine[0].Title != "Mine" {
		t.Errorf("Title = %q, want %q", got.Mine[0].Title, "Mine")
	}
	if want := "parallel · 0/0 approved"; got.Mine[0].Subtitle != want {
		t.Errorf("Subtitle = %q, want %q", got.Mine[0].Subtitle, want)
	}
}

// A step whose Document was deleted must be dropped, not rendered against a nil Document.
func TestBuildInbox_OrphanStepIsSkipped(t *testing.T) {
	steps := []*data.Record{step("stp_1", "doc_gone", "usr_ana", action.DecisionPending, 1)}

	got := buildInbox(steps, nil, nil, users, "usr_ana", at(10))
	if len(got.Pending) != 0 {
		t.Errorf("a step pointing at a missing Document must be skipped, got %d card(s)", len(got.Pending))
	}
}

// submittersFromActivity must not reorder its input: with a request-scoped Loader the slice it
// receives is the cache's own, and sorting it in place would leave every later reader of
// mch_activity in this request with silently reordered records.
func TestSubmittersFromActivity_DoesNotReorderCallersSlice(t *testing.T) {
	activities := []*data.Record{
		event("doc_1", "usr_ana", at(9)),
		event("doc_1", "usr_budi", at(8)),
	}
	first := activities[0]

	submittersFromActivity(activities)

	if activities[0] != first {
		t.Error("input slice was reordered; the Loader's cached records must be left alone")
	}
}

func TestInitials(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"Ana Putri", "AP"},
		{"Budi", "B"},
		{"ana putri santoso", "AP"},
		{"", "?"},
		{"   ", "?"},
	} {
		if got := Initials(tc.name); got != tc.want {
			t.Errorf("Initials(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
