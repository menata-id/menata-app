package web

import (
	"context"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
)

// spyMailer records every recipient it was asked to email, for asserting whether an email was
// sent at all -- sendNotification's own preference gate is otherwise invisible from outside.
type spyMailer struct {
	sent []string
}

func (m *spyMailer) Send(ctx context.Context, to, subject, body string) error {
	m.sent = append(m.sent, to)
	return nil
}

// notificationTestMachine is a minimal stand-in for whichever Machine an Event fires on -- these
// tests exercise sendNotification/rollUpParentStatus directly (same package), not a full HTTP
// request, so they only need the one Field a Notify declaration actually reads.
func notificationTestMachine(id, recipientField string) *domain.Machine {
	return &domain.Machine{
		ID:     id,
		Fields: []domain.Field{{ID: recipientField, Name: "Recipient", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID}},
	}
}

// TestSendNotification_groupHeldStepIsANoop covers CAP-F24's own named gap (approval_step.yaml's
// evt_step_created_notify comment): a Group-held step's fld_assignee is empty, so there is no
// single recipient to notify -- skipped silently, not an error, and no mch_notification row.
func TestSendNotification_groupHeldStepIsANoop(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Notify Group Test", "notify-group-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "unused_notify_group_test@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	machine := notificationTestMachine("mch_step_fixture", "fld_assignee")
	record := &data.Record{ID: "rec_fixture", Values: map[string]any{"fld_assignee": ""}}

	spy := &spyMailer{}
	sendNotification(wsCtx, store, spy, machine, record, domain.Notify{RecipientField: "fld_assignee", PreferenceKey: "assigned"}, "You have a new approval request")

	notifications, err := store.ListRecords(wsCtx, "mch_notification")
	if err != nil {
		t.Fatalf("ListRecords(mch_notification): %v", err)
	}
	if len(notifications) != 0 {
		t.Errorf("sendNotification(no recipient) created %d mch_notification rows, want 0", len(notifications))
	}
	if len(spy.sent) != 0 {
		t.Errorf("sendNotification(no recipient) sent email to %v, want none", spy.sent)
	}
}

// TestSendNotification_respectsEmailPreference is the preference gate's own proof: the in-app row
// is written unconditionally either way, and only the email is gated by the recipient's own
// notify_* column.
func TestSendNotification_respectsEmailPreference(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "notify_pref_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Notify Pref Test", "notify-pref-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	recipient, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Notify Pref", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(recipient): %v", err)
	}
	if err := store.CreateCredential(ctx, email, "Notify Pref", "hashed-value", true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	if err := store.UpdateNotificationPreferences(ctx, email, true, false); err != nil {
		t.Fatalf("UpdateNotificationPreferences: %v", err)
	}

	machine := notificationTestMachine("mch_document_fixture", "fld_submitted_by")
	notify := domain.Notify{RecipientField: "fld_submitted_by", PreferenceKey: "decided"}

	// notify_decided = false: the in-app row still appears, no email goes out.
	record := &data.Record{ID: "rec_doc_1", Values: map[string]any{"fld_submitted_by": recipient.ID}}
	spy := &spyMailer{}
	sendNotification(wsCtx, store, spy, machine, record, notify, "Your document was rejected")

	notifications, err := store.ListRecordsBy(wsCtx, "mch_notification", "fld_recipient", recipient.ID)
	if err != nil {
		t.Fatalf("ListRecordsBy(mch_notification): %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("sendNotification(notify_decided=false) created %d mch_notification rows, want 1 (the in-app row is unconditional)", len(notifications))
	}
	if len(spy.sent) != 0 {
		t.Errorf("sendNotification(notify_decided=false) sent email to %v, want none", spy.sent)
	}

	// Flip the preference on: a second event, on a second record, now also sends the email.
	if err := store.UpdateNotificationPreferences(ctx, email, true, true); err != nil {
		t.Fatalf("UpdateNotificationPreferences: %v", err)
	}
	record2 := &data.Record{ID: "rec_doc_2", Values: map[string]any{"fld_submitted_by": recipient.ID}}
	sendNotification(wsCtx, store, spy, machine, record2, notify, "Your document was approved")
	if len(spy.sent) != 1 || spy.sent[0] != email {
		t.Errorf("sendNotification(notify_decided=true) sent to %v, want exactly [%q]", spy.sent, email)
	}
}

// TestRollUpParentStatus_dispatchesParentEvents is the real behavioral fix this Tahap adds:
// rollUpParentStatus's own write to the parent (mch_document's fld_status) now also dispatches the
// parent Machine's own declared Events -- closing the gap rollUpParentStatus's doc comment used to
// name ("no Machine declares an Event that would need that here today"). Without this,
// evt_document_approved_notify/_rejected_notify would silently never fire.
func TestRollUpParentStatus_dispatchesParentEvents(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "notify_rollup_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Notify Rollup Test", "notify-rollup-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	submitter, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Notify Rollup", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(submitter): %v", err)
	}
	if err := store.CreateCredential(ctx, email, "Notify Rollup", "hashed-value", true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	document, err := store.CreateRecord(wsCtx, "mch_document_fixture", map[string]any{
		"fld_submitted_by": submitter.ID,
		"fld_status":       "in_review",
	})
	if err != nil {
		t.Fatalf("CreateRecord(document): %v", err)
	}
	step, err := store.CreateRecord(wsCtx, "mch_step_fixture", map[string]any{
		"fld_document": document.ID,
		"fld_decision": "pending",
	})
	if err != nil {
		t.Fatalf("CreateRecord(step): %v", err)
	}

	documentMachine := &domain.Machine{
		ID: "mch_document_fixture",
		Fields: []domain.Field{
			{ID: "fld_submitted_by", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"in_review", "approved", "rejected"}},
		},
		Events: []domain.Event{{
			ID: "evt_document_approved_notify_fixture",
			On: "fld_status", WhenEquals: "approved",
			Then: domain.Service{Name: domain.ServiceSendNotification, Summary: "Your document was approved",
				Notify: &domain.Notify{RecipientField: "fld_submitted_by", PreferenceKey: "decided"}},
		}},
	}
	stepMachine := &domain.Machine{
		ID: "mch_step_fixture",
		Fields: []domain.Field{
			{ID: "fld_document", Type: domain.FieldTypeRelation, RelatedMachine: "mch_document_fixture"},
			{ID: "fld_decision", Type: domain.FieldTypeStatus, Options: []string{"pending", "approved", "rejected"}},
		},
	}
	machines := map[string]*domain.Machine{
		documentMachine.ID: documentMachine,
		stepMachine.ID:     stepMachine,
	}

	// rollUpParentStatus re-reads every sibling's *stored* value (store.ListRecordsBy), it does not
	// take the new decision from the record handed to it -- so, matching decideStep's own order of
	// operations, the step's decision is written to the DB first, then rollUpParentStatus is asked
	// to recompute from what is now actually stored.
	step, err = store.UpdateRecord(wsCtx, "mch_step_fixture", step.ID, map[string]any{
		"fld_document": document.ID,
		"fld_decision": "approved",
	})
	if err != nil {
		t.Fatalf("UpdateRecord(step, approved): %v", err)
	}

	rollup := domain.Rollup{
		ParentField: "fld_document", TargetField: "fld_status",
		AllValue: "approved", AllSet: "approved",
		AnyValue: "rejected", AnySet: "rejected",
		Default: "in_review",
	}
	spy := &spyMailer{}
	rollUpParentStatus(wsCtx, store, spy, machines, stepMachine, step, submitter.ID, "fld_decision", rollup)

	updatedDocument, err := store.GetRecord(wsCtx, "mch_document_fixture", document.ID)
	if err != nil {
		t.Fatalf("GetRecord(document): %v", err)
	}
	if updatedDocument.Values["fld_status"] != "approved" {
		t.Fatalf("document fld_status = %v, want %q (the rollup itself must still work)", updatedDocument.Values["fld_status"], "approved")
	}

	notifications, err := store.ListRecordsBy(wsCtx, "mch_notification", "fld_recipient", submitter.ID)
	if err != nil {
		t.Fatalf("ListRecordsBy(mch_notification): %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("rollUpParentStatus dispatched %d notifications for the submitter, want 1 -- the parent's own approved-notify Event never fired", len(notifications))
	}
	if notifications[0].Values["fld_message"] != "Your document was approved" {
		t.Errorf("notification message = %q, want %q", notifications[0].Values["fld_message"], "Your document was approved")
	}
	if len(spy.sent) != 1 || spy.sent[0] != email {
		t.Errorf("rollUpParentStatus's dispatched notification sent email to %v, want exactly [%q] (credential defaults notify_decided=true)", spy.sent, email)
	}
}

var _ mail.Mailer = (*spyMailer)(nil)
