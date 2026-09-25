package composition

import (
	"reflect"
	"testing"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
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
		"fld_document_type":        "Contract",
		action.FieldDocumentMode:   mode,
		"fld_due_date":             due,
		action.FieldDocumentStatus: "in_review",
	})
}

// stepMachineForTest is mch_approval_step as metadata/approval_step.yaml declares it, reduced to
// what buildInbox reads: the ordering rule. Every test below passes it, because the real inbox
// always has it -- ordering only takes effect when a Document's own mode says sequential, so
// supplying it costs the parallel cases nothing.
func stepMachineForTest() *domain.Machine {
	return &domain.Machine{
		ID: action.StepMachineID,
		// prm_decide_own_step, as metadata/approval_step.yaml really declares it. It was missing
		// here until Fase 6c-1, and nothing noticed: buildReview used to AND an explicit
		// `assignee == viewer` check in front of authorization.AllowsAction, so the screen's gate
		// held even against a Machine that declared no Permission at all. That Go-side check was a
		// duplicate of the declaration -- removing it (a Group-held step has no assignee to match)
		// is what exposed the gap. The fixture now carries the rule the metadata carries, so these
		// tests exercise the real gate rather than a copy of it.
		// ApplicationID and Roles are part of that same faithfulness, added in Fase 7: the role
		// arm resolves against the Application claiming the Machine, so a fixture that stamped
		// neither would exercise the un-roled path forever while the real manifest gates on one.
		ApplicationID: "app_document_approval",
		Permissions: []domain.Permission{{
			ID:         "prm_decide_own_step",
			Action:     domain.ActionDecide,
			Roles:      []string{"approver", "reviewer"},
			ActorField: action.FieldStepAssignee,
			DynamicActor: &domain.DynamicActorGate{
				ActorTypeField:  action.FieldStepApproverType,
				ActorUserField:  action.FieldStepAssignee,
				ActorGroupField: action.FieldStepApproverGroup,
			},
		}},
		// The declared state model, for the same reason (Fase 7). Without it canStillDecide reads
		// "this Machine restricts nothing" and offers the decision bar on an already-decided step
		// -- which is precisely the assertion this fixture's own tests make.
		Transitions: []domain.Transition{
			{ID: "trn_step_approve", Name: "Approve", Field: action.FieldStepDecision, From: action.DecisionPending, To: action.DecisionApproved, Action: domain.ActionDecide},
			{ID: "trn_step_reject", Name: "Reject", Field: action.FieldStepDecision, From: action.DecisionPending, To: action.DecisionRejected, Action: domain.ActionDecide},
		},
		Sequencing: &domain.Sequencing{
			ParentField:     action.FieldStepDocument,
			ModeField:       action.FieldDocumentMode,
			SequentialValue: "sequential",
			OrderField:      action.FieldStepSequence,
			StateField:      action.FieldStepDecision,
			OpenValue:       action.DecisionPending,
		},
	}
}

// personNames stands in for data.Store.MemberNames: since 2026-09-22 a person's display name is
// resolved from the identity behind their record rather than read off a Field, so these builders
// take the resolved map instead of the User records they used to scan.
var personNames = map[string]string{
	"usr_ana":  "Ana Putri",
	"usr_budi": "Budi",
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

	got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.Pending) != 0 {
		t.Errorf("step 2 is locked behind step 1, so it must not appear as pending; got %d card(s)", len(got.Pending))
	}

	// Same records, parallel mode: nothing is waiting on anything.
	docs[0].Values[action.FieldDocumentMode] = "parallel"
	got = buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
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

	got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.Pending) != 1 {
		t.Fatalf("want only my own still-pending step, got %d", len(got.Pending))
	}
	if want := "/machines/" + action.StepMachineID + "/records/stp_mine/review"; got.Pending[0].Href != want {
		t.Errorf("Href = %q, want %q", got.Pending[0].Href, want)
	}
	// One of three steps is approved, and the card says so.
	if got.Pending[0].Approved != 1 || got.Pending[0].TotalSteps != 3 {
		t.Errorf("Approved/TotalSteps = %d/%d, want 1/3", got.Pending[0].Approved, got.Pending[0].TotalSteps)
	}
	if got.Pending[0].Submitter != "someone" {
		t.Errorf("Submitter = %q, want %q", got.Pending[0].Submitter, "someone")
	}
	// parallel mode: every pending step is actionable at once, so both undecided steps show
	// "current", not "waiting" -- only sequential mode locks a later step behind an earlier one.
	states := make([]string, 0, len(got.Pending[0].Approvers))
	for _, a := range got.Pending[0].Approvers {
		states = append(states, a.State)
	}
	if want := []string{"done", "current", "current"}; !equalStrings(states, want) {
		t.Errorf("Approvers states = %v, want %v", states, want)
	}
	// Fase 6a carries each approver's name alongside its state, for board 07's approver list.
	// Composed from the names map buildInbox already had, so this asserts the wiring rather than
	// a new lookup: an assignee with no resolvable mch_user record degrades to an empty name.
	if len(got.Pending[0].Approvers) != 3 {
		t.Errorf("Approvers = %d, want one per step", len(got.Pending[0].Approvers))
	}
}

func equalStrings(a, b []string) bool {
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

			got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
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

	got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
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

	got := buildInbox(steps, docs, activities, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.Pending) != 1 {
		t.Fatalf("want one card, got %d", len(got.Pending))
	}
	if got.Pending[0].Submitter != "Budi" {
		t.Errorf("Submitter = %q, want %q", got.Pending[0].Submitter, "Budi")
	}
	if want := "8 Sep 2026"; got.Pending[0].SubmittedAt != want {
		t.Errorf("SubmittedAt = %q, want %q", got.Pending[0].SubmittedAt, want)
	}
}

func TestBuildInbox_UnknownSubmitterFallsBack(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}

	got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if got.Pending[0].Submitter != "someone" {
		t.Errorf("Submitter = %q, want %q", got.Pending[0].Submitter, "someone")
	}
	if got.Pending[0].SubmittedAt != "" {
		t.Errorf("SubmittedAt = %q, want empty -- no activity event was found", got.Pending[0].SubmittedAt)
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

	got := buildInbox(nil, docs, activities, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.Mine) != 1 {
		t.Fatalf("want one submitted Document, got %d", len(got.Mine))
	}
	if got.Mine[0].Title != "Mine" {
		t.Errorf("Title = %q, want %q", got.Mine[0].Title, "Mine")
	}
	if got.Mine[0].DocumentType != "Contract" || got.Mine[0].Mode != "parallel" {
		t.Errorf("DocumentType/Mode = %q/%q, want Contract/parallel", got.Mine[0].DocumentType, got.Mine[0].Mode)
	}
	if got.Mine[0].TotalSteps != 0 || got.Mine[0].Approved != 0 {
		t.Errorf("Approved/TotalSteps = %d/%d, want 0/0", got.Mine[0].Approved, got.Mine[0].TotalSteps)
	}
	// Submitter is deliberately empty on this list: every card here is the viewer's own, so
	// pendingApprovalCard drops the "Submitted by" line rather than printing the viewer's own name
	// back at them (Inbox.Mine's own doc comment).
	if got.Mine[0].Submitter != "" || got.Mine[0].SubmittedAt != "" {
		t.Errorf("Submitter/SubmittedAt = %q/%q, want both empty", got.Mine[0].Submitter, got.Mine[0].SubmittedAt)
	}
}

// A step whose Document was deleted must be dropped, not rendered against a nil Document.
func TestBuildInbox_OrphanStepIsSkipped(t *testing.T) {
	steps := []*data.Record{step("stp_1", "doc_gone", "usr_ana", action.DecisionPending, 1)}

	got := buildInbox(steps, nil, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.Pending) != 0 {
		t.Errorf("a step pointing at a missing Document must be skipped, got %d card(s)", len(got.Pending))
	}
}

// A newly-overdue Document not yet logged must show up in NewBreaches exactly once, and never
// again once an Activity entry for it already exists -- the idempotency check buildInbox relies on
// instead of a Document Field (see slaBreachMarker's own comment for why).
func TestBuildInbox_NewBreaches_detectsOverdueUnloggedDocument(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "2026-09-01")} // due well before "now"
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}

	got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.NewBreaches) != 1 || got.NewBreaches[0].DocumentID != "doc_1" {
		t.Fatalf("NewBreaches = %+v, want one breach for doc_1", got.NewBreaches)
	}
}

func TestBuildInbox_NewBreaches_skipsAlreadyLogged(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "2026-09-01")}
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}
	activities := []*data.Record{
		{ID: "act_1", Values: map[string]any{"fld_record_id": "doc_1", "fld_summary": slaBreachMarker + `"Contract" (due 1 Sep 2026)`}},
	}

	got := buildInbox(steps, docs, activities, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.NewBreaches) != 0 {
		t.Errorf("NewBreaches = %+v, want none -- this Document's breach was already logged", got.NewBreaches)
	}
}

func TestBuildInbox_NewBreaches_skipsNotOverdue(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "2026-09-20")} // due well after "now"
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}

	got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.NewBreaches) != 0 {
		t.Errorf("NewBreaches = %+v, want none -- not overdue yet", got.NewBreaches)
	}
}

func TestBuildInbox_NewBreaches_skipsDecidedDocuments(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "2026-09-01")}
	docs[0].Values[action.FieldDocumentStatus] = action.DocumentStatusApproved

	got := buildInbox(nil, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.NewBreaches) != 0 {
		t.Errorf("NewBreaches = %+v, want none -- an already-decided Document has no active SLA to breach", got.NewBreaches)
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

// TestBuildInbox_ProjectsCardFields is the Fase 1 pilot's end-to-end proof: a stepMachine
// declaring card_fields (the same shape metadata/approval_step.yaml would carry, had it
// opted in) flows all the way through buildInbox into PendingApprovalCard.CardFields, resolved
// via composition.ProjectCardFields -- no metadata/approval_step.yaml change, no .templ change.
func TestBuildInbox_ProjectsCardFields(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "sequential", "")}
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}
	stepMachine := &domain.Machine{
		ID: action.StepMachineID,
		Fields: []domain.Field{
			{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
		},
		CardFields: []domain.CardField{
			{Field: "fld_assignee", Role: domain.CardFieldRolePerson},
		},
	}
	relations := rendering.RelationOptions{
		"mch_user": {{ID: "usr_ana", Label: "Ana Putri"}, {ID: "usr_budi", Label: "Budi"}},
	}

	got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachine, relations)
	if len(got.Pending) != 1 {
		t.Fatalf("len(Pending) = %d, want 1", len(got.Pending))
	}
	want := []rendering.ProjectedField{{Label: "Assignee", Role: "person", Display: "Ana Putri"}}
	if !reflect.DeepEqual(got.Pending[0].CardFields, want) {
		t.Errorf("Pending[0].CardFields = %+v, want %+v", got.Pending[0].CardFields, want)
	}
}

// TestBuildInbox_NilStepMachineProjectsNothing is the regression guard: every Machine today
// (nil stepMachine, the caller's own zero value when card_fields isn't declared) must render
// exactly as before Fase 1 -- an empty CardFields, not a nil-pointer panic.
func TestBuildInbox_NilStepMachineProjectsNothing(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "sequential", "")}
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}

	got := buildInbox(steps, docs, nil, personNames, "usr_ana", at(10), stepMachineForTest(), nil)
	if len(got.Pending) != 1 {
		t.Fatalf("len(Pending) = %d, want 1", len(got.Pending))
	}
	if len(got.Pending[0].CardFields) != 0 {
		t.Errorf("Pending[0].CardFields = %+v, want empty", got.Pending[0].CardFields)
	}
}

func TestMineFilters(t *testing.T) {
	mine := []rendering.PendingApprovalCard{
		{Status: action.DocumentStatusInReview},
		{Status: action.DocumentStatusInReview},
		{Status: action.DocumentStatusApproved},
		{Status: action.DocumentStatusRejected},
	}
	got := MineFilters(mine, action.DocumentStatusInReview)
	want := map[string]struct {
		count  int
		active bool
	}{
		"all":                         {4, false},
		action.DocumentStatusInReview: {2, true},
		action.DocumentStatusApproved: {1, false},
		action.DocumentStatusRejected: {1, false},
	}
	if len(got) != 4 {
		t.Fatalf("MineFilters returned %d chips, want 4 (no Draft -- fld_status declares none)", len(got))
	}
	for _, chip := range got {
		w, ok := want[chip.Key]
		if !ok {
			t.Errorf("unexpected chip key %q", chip.Key)
			continue
		}
		if chip.Count != w.count {
			t.Errorf("chip %q count = %d, want %d", chip.Key, chip.Count, w.count)
		}
		if chip.Active != w.active {
			t.Errorf("chip %q active = %v, want %v (statusKey was %q)", chip.Key, chip.Active, w.active, action.DocumentStatusInReview)
		}
	}
	// Counts must not depend on which chip is active -- built from the unfiltered set, matching
	// pendingTabContent's own Overdue/Due-today chips and assignedTabContent's four decision chips.
	gotAll := MineFilters(mine, "")
	for i, chip := range gotAll {
		if chip.Count != got[i].Count {
			t.Errorf("chip %q count changed with statusKey (%d vs %d) -- counts must come from the unfiltered list", chip.Key, chip.Count, got[i].Count)
		}
	}
}

func TestFilterCardsByStatus(t *testing.T) {
	cards := []rendering.PendingApprovalCard{
		{Title: "A", Status: action.DocumentStatusInReview},
		{Title: "B", Status: action.DocumentStatusApproved},
	}
	if got := FilterCardsByStatus(cards, ""); len(got) != 2 {
		t.Errorf("empty statusKey = %v, want both cards unchanged", got)
	}
	if got := FilterCardsByStatus(cards, "all"); len(got) != 2 {
		t.Errorf("\"all\" statusKey = %v, want both cards unchanged", got)
	}
	got := FilterCardsByStatus(cards, action.DocumentStatusApproved)
	if len(got) != 1 || got[0].Title != "B" {
		t.Errorf("FilterCardsByStatus(..., approved) = %+v, want just card B", got)
	}
}

func TestSearchCards(t *testing.T) {
	cards := []rendering.PendingApprovalCard{
		{Title: "Catering Contract", Reference: "DOC-0106"},
		{Title: "Lighting Rental", Reference: "DOC-0104"},
	}
	if got := SearchCards(cards, ""); len(got) != 2 {
		t.Errorf("empty query = %v, want both cards unchanged", got)
	}
	if got := SearchCards(cards, "catering"); len(got) != 1 || got[0].Reference != "DOC-0106" {
		t.Errorf("case-insensitive title match = %+v, want just DOC-0106", got)
	}
	if got := SearchCards(cards, "doc-0104"); len(got) != 1 || got[0].Title != "Lighting Rental" {
		t.Errorf("case-insensitive reference match = %+v, want just Lighting Rental", got)
	}
	if got := SearchCards(cards, "nothing matches this"); len(got) != 0 {
		t.Errorf("no-match query = %v, want an empty slice", got)
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

// approverActor is a viewer holding `approver` in Document Approval -- the shape
// internal/web.currentActor builds from data.EffectiveRoles.
//
// Every composition test that had written domain.Actor{ID: ...} now goes through here, because
// mch_approval_step's Permissions carry a roles: arm as of Fase 7 and the fixture above mirrors
// it. Keeping the id-only literal would have made each of those tests pass or fail for a reason
// it was not written to ask about.
func approverActor(id string) domain.Actor {
	return domain.Actor{ID: id, Roles: map[string][]string{"app_document_approval": {"approver"}}}
}
