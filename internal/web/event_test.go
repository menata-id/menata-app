package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// eventTestSetup is the shared fixture every test below needs: a Workspace, a member, and one
// Task record. Unlike approval_test.go's hand-built approvalStepTestMachine, this loads the real
// metadata/task.yaml (record_test.go's own loadRealMachines) rather than a Go literal mirroring
// it -- the point of these tests is proving evt_task_status_changed's behavior actually comes
// from the committed YAML, not from Go code that merely happens to agree with it today.
type eventTestSetup struct {
	store       *data.Store
	cfg         config.Config
	machines    map[string]*domain.Machine
	ctx         context.Context
	workspaceID string
	actor       string
	taskID      string
}

func newEventTestSetup(t *testing.T, testName string) eventTestSetup {
	t.Helper()
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "event-test-secret", SecureCookies: false}
	ctx := context.Background()
	email := testName + "@example.com"

	ws, err := store.CreateWorkspace(ctx, testName, strings.ReplaceAll(testName, "_", "-"))
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	actor, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_name": "Rina Nur", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(actor): %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, actor.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	task, err := store.CreateRecord(wsCtx, "mch_task", map[string]any{
		"fld_title":  "Fix bug",
		"fld_status": "todo",
	})
	if err != nil {
		t.Fatalf("CreateRecord(task): %v", err)
	}

	return eventTestSetup{
		store:       store,
		cfg:         cfg,
		machines:    realMachines(t),
		ctx:         wsCtx,
		workspaceID: ws.ID,
		actor:       actor.ID,
		taskID:      task.ID,
	}
}

// putTaskStatus PUTs a status change to s's own Task, through the real updateRecordForm handler.
func putTaskStatus(t *testing.T, s eventTestSetup, newStatus string) *httptest.ResponseRecorder {
	t.Helper()
	form := "fld_title=Fix+bug&fld_status=" + newStatus
	req := httptest.NewRequest(http.MethodPut, "/machines/mch_task/records/"+s.taskID, strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), testWorkspaceFor(s.machines), "Test Workspace"))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, s.actor, 0)})

	r := chi.NewRouter()
	r.Put("/machines/{machineID}/records/{id}", updateRecordForm(s.machines, s.store, nil, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// activitySummariesForTask returns every mch_activity fld_summary logged against s's own Task.
func activitySummariesForTask(t *testing.T, s eventTestSetup) []string {
	t.Helper()
	rows, err := s.store.ListRecordsBy(s.ctx, "mch_activity", "fld_record_id", s.taskID)
	if err != nil {
		t.Fatalf("ListRecordsBy(mch_activity): %v", err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, toDisplayString(r.Values["fld_summary"]))
	}
	return out
}

// TestUpdateRecordForm_taskStatusChangeLogsActivity is the proof of the Event primitive: a
// declared evt_task_status_changed (metadata/task.yaml) reproduces the old hardcoded
// currentTaskStatus/logTaskStatusMove behavior exactly, including its one wording exception.
func TestUpdateRecordForm_taskStatusChangeLogsActivity(t *testing.T) {
	s := newEventTestSetup(t, "event_task_status_moved")

	rec := putTaskStatus(t, s, "in_progress")
	if rec.Code != http.StatusOK {
		t.Fatalf("updateRecordForm(status -> in_progress) status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	got := activitySummariesForTask(t, s)
	want := `"Fix bug" moved from todo to in_progress`
	if len(got) != 1 || got[0] != want {
		t.Errorf("Activity summaries = %v, want exactly [%q]", got, want)
	}
}

// TestUpdateRecordForm_taskStatusChangeToDoneUsesOverride covers Service.SummaryOverride: moving
// to "done" reads "completed", not "moved from X to done" -- the one wording exception the old
// hardcoded logTaskStatusMove had, reproduced here via then.summary_override_when/summary_override.
func TestUpdateRecordForm_taskStatusChangeToDoneUsesOverride(t *testing.T) {
	s := newEventTestSetup(t, "event_task_status_done")

	rec := putTaskStatus(t, s, "done")
	if rec.Code != http.StatusOK {
		t.Fatalf("updateRecordForm(status -> done) status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	got := activitySummariesForTask(t, s)
	want := `"Fix bug" completed`
	if len(got) != 1 || got[0] != want {
		t.Errorf("Activity summaries = %v, want exactly [%q]", got, want)
	}
}

// TestUpdateRecordForm_taskUnchangedStatusLogsNothing: editing a Task without changing fld_status
// must not fire the Event -- MatchedEvents' own "new == old" guard.
func TestUpdateRecordForm_taskUnchangedStatusLogsNothing(t *testing.T) {
	s := newEventTestSetup(t, "event_task_status_unchanged")

	rec := putTaskStatus(t, s, "todo") // same as the record's own starting status
	if rec.Code != http.StatusOK {
		t.Fatalf("updateRecordForm(status unchanged) status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	if got := activitySummariesForTask(t, s); len(got) != 0 {
		t.Errorf("Activity summaries = %v, want none: fld_status did not change", got)
	}
}

// TestCreateRecordForm_taskCreationLogsActivity proves evt_task_created (metadata/task.yaml) --
// the OnCreate shape's own generalization of what used to be internal/web's hardcoded
// logRecordCreated switch -- fires through the real HTML-form create path (createRecordForm).
func TestCreateRecordForm_taskCreationLogsActivity(t *testing.T) {
	s := newEventTestSetup(t, "event_task_created")

	form := "fld_title=New+Task"
	req := httptest.NewRequest(http.MethodPost, "/machines/mch_task/records", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), testWorkspaceFor(s.machines), "Test Workspace"))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, s.actor, 0)})

	r := chi.NewRouter()
	r.Post("/machines/{machineID}/records", createRecordForm(s.machines, s.store, nil, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("createRecordForm status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rows, err := s.store.ListRecordsBy(s.ctx, "mch_task", "fld_title", "New Task")
	if err != nil {
		t.Fatalf("ListRecordsBy(mch_task): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListRecordsBy(mch_task, New Task) = %d rows, want 1", len(rows))
	}
	newTaskID := rows[0].ID

	activities, err := s.store.ListRecordsBy(s.ctx, "mch_activity", "fld_record_id", newTaskID)
	if err != nil {
		t.Fatalf("ListRecordsBy(mch_activity): %v", err)
	}
	want := `"New Task" created`
	if len(activities) != 1 || toDisplayString(activities[0].Values["fld_summary"]) != want {
		t.Errorf("Activity summaries for the new task = %v, want exactly [%q]", activities, want)
	}
}

// TestCreateRecord_api_taskCreationLogsActivity is TestCreateRecordForm_taskCreationLogsActivity's
// own JSON-API-path counterpart -- both createRecordForm and createRecord (api.go) call
// runCreateEvents now, and both changed in the same commit, so both need their own proof.
func TestCreateRecord_api_taskCreationLogsActivity(t *testing.T) {
	s := newEventTestSetup(t, "event_task_created_api")

	body := `{"fld_title":"API Task"}`
	req := httptest.NewRequest(http.MethodPost, "/api/machines/mch_task/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), testWorkspaceFor(s.machines), "Test Workspace"))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, s.actor, 0)})

	r := chi.NewRouter()
	r.Post("/api/machines/{machineID}/records", createRecord(s.machines, s.store, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("createRecord status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	rows, err := s.store.ListRecordsBy(s.ctx, "mch_task", "fld_title", "API Task")
	if err != nil {
		t.Fatalf("ListRecordsBy(mch_task): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListRecordsBy(mch_task, API Task) = %d rows, want 1", len(rows))
	}
	newTaskID := rows[0].ID

	activities, err := s.store.ListRecordsBy(s.ctx, "mch_activity", "fld_record_id", newTaskID)
	if err != nil {
		t.Fatalf("ListRecordsBy(mch_activity): %v", err)
	}
	want := `"API Task" created`
	if len(activities) != 1 || toDisplayString(activities[0].Values["fld_summary"]) != want {
		t.Errorf("Activity summaries for the new task = %v, want exactly [%q]", activities, want)
	}
}

// TestEventOldValues_fetchFailureIsDistinctFromNoEventsDeclared is the regression test for a
// code-review finding (2026-09-19): a GetRecord failure must make runEvents skip dispatch
// entirely, not silently compare against a nil map -- fmt.Sprint on a nil map's missing key
// yields "<nil>", which would look like a change for almost any real new value and fire a bogus
// Event off a transient read error. ok must be false here, not just values == nil, so the caller
// can tell "no Events declared" (also nil, but safe to dispatch -- MatchedEvents finds nothing)
// apart from "the fetch itself failed" (unsafe to dispatch at all).
func TestEventOldValues_fetchFailureIsDistinctFromNoEventsDeclared(t *testing.T) {
	s := newEventTestSetup(t, "event_old_values_fetch_failure")
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	values, ok := eventOldValues(req, s.store, s.machines["mch_task"], "rec_does_not_exist")
	if ok {
		t.Fatalf("eventOldValues(nonexistent record) ok = true, want false: the fetch failed")
	}
	if values != nil {
		t.Errorf("eventOldValues(nonexistent record) values = %v, want nil", values)
	}
}
