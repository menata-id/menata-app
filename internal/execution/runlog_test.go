package execution

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// What a run is remembered as: failed if anything failed, quiet if it only had no one or nothing to act on, otherwise ran -- the difference between a rule that is silent and one that is broken.
func TestARunIsRememberedAsRanQuietOrFailed(t *testing.T) {
	cases := map[string]struct {
		act     func(ctx context.Context)
		outcome string
		detail  string
	}{
		"it did its work":                       {func(ctx context.Context) { did(ctx, "told 2 people") }, data.EventRanOutcome, "told 2 people"},
		"it had nobody to tell":                 {func(ctx context.Context) { nothingToDo(ctx, "nobody to tell: the field fld_x is empty") }, data.EventQuietOutcome, "nobody to tell: the field fld_x is empty"},
		"it failed":                             {func(ctx context.Context) { fail(ctx, "sending email to a@b: refused") }, data.EventFailedOutcome, "sending email to a@b: refused"},
		"it failed after some work":             {func(ctx context.Context) { did(ctx, "told 1 person"); fail(ctx, "mail: refused") }, data.EventFailedOutcome, "mail: refused"},
		"it did something and had nothing else": {func(ctx context.Context) { did(ctx, "x"); nothingToDo(ctx, "y") }, data.EventRanOutcome, "x"},
		"it did nothing at all":                 {func(ctx context.Context) {}, data.EventRanOutcome, ""},
	}
	for name, c := range cases {
		ctx, rec := withRecorder(context.Background())
		c.act(ctx)
		if outcome, detail := rec.outcome(); outcome != c.outcome || detail != c.detail {
			t.Errorf("%s: %s / %q, want %s / %q", name, outcome, detail, c.outcome, c.detail)
		}
	}
	// Outside a rule the helpers only log: they are the Services' own code paths, run by other callers too.
	fail(context.Background(), "x")
	did(context.Background(), "y")
	nothingToDo(context.Background(), "z")
}

// A rule's runs are remembered, each with its outcome: a rule switched off is not run at all (so remembers nothing), one that finds nobody to tell is quiet and says why, and one
// that tells a person ran and says how many.
func TestRulesRunsAreRememberedAndASwitchedOffRuleDoesNotRun(t *testing.T) {
	pool := testPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Event Runs", "event-runs-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupTest(t, pool, ws.ID, "unused_event_runs@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	person, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Dewi"})
	if err != nil {
		t.Fatal(err)
	}
	machine := notificationTestMachine("mch_runs_fixture", "fld_assignee")
	machine.Events = []domain.Event{
		{ID: "evt_on", OnCreate: true, Then: domain.Service{Name: domain.ServiceSendNotification, Notify: &domain.Notify{RecipientField: "fld_assignee", PreferenceKey: "assigned"}}},
		{ID: "evt_off", OnCreate: true, Disabled: true, Then: domain.Service{Name: domain.ServiceSendNotification, Notify: &domain.Notify{RecipientField: "fld_assignee", PreferenceKey: "assigned"}}},
	}
	svc := Services{Store: store, Mailer: &spyMailer{}}
	nobody := &data.Record{ID: "rec_nobody", Values: map[string]any{"fld_assignee": ""}}
	RunCreateEvents(wsCtx, svc, nil, machine, nobody, "u")
	someone := &data.Record{ID: "rec_someone", Values: map[string]any{"fld_assignee": person.ID}}
	RunCreateEvents(wsCtx, svc, nil, machine, someone, "u")

	runs, err := store.RecentEventRuns(wsCtx, machine.ID, "evt_on", 10)
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs of evt_on: %v %v", runs, err)
	}
	if runs[0].Outcome != data.EventRanOutcome || runs[0].RecordID != "rec_someone" || runs[0].Detail != "told 1 person" {
		t.Errorf("the run that told someone: %+v", runs[0])
	}
	if runs[1].Outcome != data.EventQuietOutcome || runs[1].RecordID != "rec_nobody" || runs[1].Detail == "" {
		t.Errorf("the run that found nobody: %+v", runs[1])
	}
	if off, _ := store.RecentEventRuns(wsCtx, machine.ID, "evt_off", 10); len(off) != 0 {
		t.Errorf("a switched-off rule ran: %+v", off)
	}
	latest, err := store.LatestEventRuns(wsCtx, []string{machine.ID})
	if err != nil || latest[machine.ID+"/evt_on"].RecordID != "rec_someone" || len(latest) != 1 {
		t.Errorf("latest runs: %+v %v", latest, err)
	}
}

// A trial of a rule on a real record says what the rule would do and does none of it: no mail is sent, no notification or activity entry is written, no Field changes.
func TestDryRunSaysWhatARuleWouldDoAndWritesNothing(t *testing.T) {
	pool := testPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Dry Run", "dry-run-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupTest(t, pool, ws.ID, "unused_dry_run@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	person, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Dewi"})
	if err != nil {
		t.Fatal(err)
	}
	machine := notificationTestMachine("mch_dry_fixture", "fld_assignee")
	machine.Fields = append(machine.Fields, domain.Field{ID: "fld_stage", Name: "Stage", Type: domain.FieldTypeStatus, Options: []string{"Open", "Done"}})
	tell := domain.Event{ID: "evt_tell", On: "fld_stage", WhenEquals: "Done", Then: domain.Service{Name: domain.ServiceSendNotification, Summary: "Done: {fld_stage}", Notify: &domain.Notify{RecipientField: "fld_assignee", PreferenceKey: "assigned"}}}
	off := domain.Event{ID: "evt_off", OnCreate: true, Disabled: true, Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "x"}}
	log := domain.Event{ID: "evt_log", OnCreate: true, Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "Created {fld_stage}"}}
	machine.Events = []domain.Event{tell, off, log}
	record := &data.Record{ID: "rec_dry", Values: map[string]any{"fld_assignee": person.ID, "fld_stage": "Open"}}

	before, _ := store.ListRecords(wsCtx, "mch_notification")
	beforeActivity, _ := store.ListRecords(wsCtx, "mch_activity")
	said := strings.Join(DryRun(wsCtx, store, nil, machine, record, tell, time.Now()), "\n")
	for _, want := range []string{"becomes Done", "Open", "It would tell 1: Dewi", "Nothing was sent", "Done: Open"} {
		if !strings.Contains(said, want) {
			t.Errorf("the trial of the notifying rule lacks %q:\n%s", want, said)
		}
	}
	if said := strings.Join(DryRun(wsCtx, store, nil, machine, &data.Record{ID: "r2", Values: map[string]any{}}, tell, time.Now()), "\n"); !strings.Contains(said, "tell no one") || !strings.Contains(said, "fld_assignee") {
		t.Errorf("a record with nobody to tell: %s", said)
	}
	if said := strings.Join(DryRun(wsCtx, store, nil, machine, record, off, time.Now()), "\n"); !strings.Contains(said, "switched off") {
		t.Errorf("a switched-off rule: %s", said)
	}
	if said := strings.Join(DryRun(wsCtx, store, nil, machine, record, log, time.Now()), "\n"); !strings.Contains(said, "write an activity entry: Created Open") {
		t.Errorf("a logging rule: %s", said)
	}
	after, _ := store.ListRecords(wsCtx, "mch_notification")
	afterActivity, _ := store.ListRecords(wsCtx, "mch_activity")
	if len(after) != len(before) || len(afterActivity) != len(beforeActivity) {
		t.Errorf("a trial wrote records: notifications %d -> %d, activity %d -> %d", len(before), len(after), len(beforeActivity), len(afterActivity))
	}
	if runs, _ := store.RecentEventRuns(wsCtx, machine.ID, "evt_tell", 5); len(runs) != 0 {
		t.Errorf("a trial was remembered as a run: %+v", runs)
	}
}

// The detail of a run is cut on a character boundary and carries no NUL, so a name in any script never makes the row that remembers the run fail to be written; and the history is
// trimmed per rule, so a busy rule does not push a rare one's last run out.
func TestRunHistoryIsWrittenForAnyScriptAndTrimmedPerRule(t *testing.T) {
	pool := testPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Runs Trim", "runs-trim-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupTest(t, pool, ws.ID, "unused_runs_trim@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	if err := store.RecordEventRun(wsCtx, "mch_x", "evt_rare", "rec_0", data.EventRanOutcome, strings.Repeat("é", 400)+"\x00end"); err != nil {
		t.Fatalf("a long line in a script of two-byte characters, with a NUL: %v", err)
	}
	rare, err := store.RecentEventRuns(wsCtx, "mch_x", "evt_rare", 5)
	if err != nil || len(rare) != 1 || !utf8.ValidString(rare[0].Detail) || strings.Contains(rare[0].Detail, "\x00") || len(rare[0].Detail) > 500 {
		t.Fatalf("stored detail: %+v %v", rare, err)
	}
	for i := 0; i < 230; i++ {
		if err := store.RecordEventRun(wsCtx, "mch_x", "evt_busy", "rec", data.EventRanOutcome, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if rare, _ := store.RecentEventRuns(wsCtx, "mch_x", "evt_rare", 5); len(rare) != 1 {
		t.Errorf("a busy rule pushed a rare rule's last run out: %d left", len(rare))
	}
	if busy, _ := store.RecentEventRuns(wsCtx, "mch_x", "evt_busy", 1000); len(busy) != 200 {
		t.Errorf("a busy rule keeps %d runs, want 200", len(busy))
	}
}

// A set_field rule that finds the Field already holding the value did nothing, and the history says so instead of "ran".
func TestASetFieldThatFindsTheValueAlreadyThereIsQuiet(t *testing.T) {
	ctx, rec := withRecorder(context.Background())
	m := &domain.Machine{ID: "mch_x"}
	setField(ctx, Services{}, nil, m, &data.Record{ID: "r", Values: map[string]any{"fld_s": "Closed"}}, "u", domain.SetField{TargetField: "fld_s", Value: "Closed"})
	if outcome, detail := rec.outcome(); outcome != data.EventQuietOutcome || !strings.Contains(detail, "already holds Closed") {
		t.Errorf("outcome = %s / %q", outcome, detail)
	}
}
