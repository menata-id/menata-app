package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/behavior"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
)

// displayString renders a stored field value as text -- a duplicate of composition.DisplayString,
// on purpose: internal/execution cannot import internal/composition (composition builds the
// Experience tree on top of this package's own physical-execution primitives, so the dependency
// only ever runs the other way -- see this package's own boundary rule and composition's "physical
// execution belongs to internal/data and internal/execution"), and threading a formatter through
// every call site here for one function would be a worse trade than the lines below. The same call
// rendering.personInitials already makes about composition.Initials.
func displayString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// logActivity appends one mch_activity record -- a duplicate of internal/web's own logActivity,
// kept in both places rather than shared for the same reason displayString is: internal/web's
// copy is called from many places that have nothing to do with Event dispatch (a document being
// saved as a draft, a step being decided), while this one exists purely to be RunEvents/
// RunCreateEvents/RunScheduledEvents' own ServiceLogActivity arm. Best-effort: a logging failure
// must not fail the real operation it's describing, only get logged itself.
func logActivity(ctx context.Context, store *data.Store, machineID, recordID, actorID, summary string) {
	values := map[string]any{
		"fld_machine_id": machineID,
		"fld_record_id":  recordID,
		"fld_summary":    summary,
	}
	if actorID != "" {
		values["fld_actor"] = actorID
	}
	if _, err := store.CreateRecord(ctx, "mch_activity", values); err != nil {
		log.Printf("failed to log activity (%s %s): %v", machineID, recordID, err)
	}
}

// sendNotification is ServiceSendNotification's own I/O half (Flow 2 gap study Tahap 6): write an
// mch_notification record unconditionally, then email its recipient too if their own preference
// allows it. Best-effort throughout, the same posture logActivity/rollUpParentStatus already take
// -- a failure here must never fail the write (or, for a schedule-triggered call, the tick) it's
// describing.
//
// recipientID comes from notify.RecipientField on the record the Event fired on (domain.Notify's
// own doc comment: no cross-record resolution built yet). Empty is a normal outcome, not an error
// -- a Group-held Approval Step (CAP-F24) has no single person to notify, named and skipped here
// rather than solved.
func sendNotification(ctx context.Context, store *data.Store, mailer mail.Mailer, machine *domain.Machine, record *data.Record, notify domain.Notify, message string) {
	recipientID := fmt.Sprint(record.Values[notify.RecipientField])
	if recipientID == "" || recipientID == "<nil>" {
		return
	}

	values := map[string]any{
		"fld_recipient": recipientID,
		"fld_message":   message,
		"fld_link":      notificationLinkFor(machine.ID, record.ID),
	}
	if _, err := store.CreateRecord(ctx, "mch_notification", values); err != nil {
		log.Printf("failed to create notification for %s: %v", recipientID, err)
	}

	recipient, err := store.GetRecord(ctx, domain.UserMachineID, recipientID)
	if err != nil {
		log.Printf("notify %s: reading recipient %s: %v", notify.PreferenceKey, recipientID, err)
		return
	}
	email := fmt.Sprint(recipient.Values["fld_email"])
	if email == "" || email == "<nil>" {
		return
	}
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		log.Printf("notify %s: reading credential for %s: %v", notify.PreferenceKey, email, err)
		return
	}
	wantsEmail := true
	switch notify.PreferenceKey {
	case "assigned":
		wantsEmail = cred.NotifyAssigned
	case "decided":
		wantsEmail = cred.NotifyDecided
	case "sla_breach":
		wantsEmail = cred.NotifySLABreach
	}
	if !wantsEmail {
		return
	}
	if err := mailer.Send(ctx, email, "Menata App", message); err != nil {
		log.Printf("notify %s: sending email to %s: %v", notify.PreferenceKey, email, err)
	}
}

// notificationLinkFor is a named hardcoding exception (writing-guide.md): a notification's real
// destination is not always the generic record detail page. mch_approval_step's own generic page
// is explicitly "POC scaffolding no real approver should land on" (detail.templ's detailBackLink,
// same reasoning) -- its real screen is /review. Forward pointer: a second notification-emitting
// Machine pair needing a non-generic destination is the trigger to generalize this into a declared
// Field rather than a per-Machine-id branch.
func notificationLinkFor(machineID, recordID string) string {
	if machineID == action.StepMachineID {
		return fmt.Sprintf("/machines/%s/records/%s/review", machineID, recordID)
	}
	return fmt.Sprintf("/machines/%s/records/%s", machineID, recordID)
}

// RunCreateEvents is RunEvents' own counterpart for the create path: every domain.Event a Machine
// declares OnCreate (behavior.MatchedCreateEvents) fires once, unconditionally, for the record
// just created -- generalizing what used to be a hardcoded per-Machine switch (logRecordCreated,
// ROADMAP.md Phase 21 round 2 Step I) into the same declarative mechanism RunEvents already
// established for field-change Events. renderEventSummary is reused as-is with oldValues nil: a
// creation Event's own summary template only ever uses {field_id} placeholders, never
// {old}/{new}, so nil resolves harmlessly.
func RunCreateEvents(ctx context.Context, store *data.Store, mailer mail.Mailer, machine *domain.Machine, record *data.Record, actorID string) {
	for _, e := range behavior.MatchedCreateEvents(machine) {
		switch e.Then.Name {
		case domain.ServiceLogActivity:
			logActivity(ctx, store, machine.ID, record.ID, actorID, renderEventSummary(e, machine, nil, record.Values))
		case domain.ServiceSendNotification:
			sendNotification(ctx, store, mailer, machine, record, *e.Then.Notify, renderEventSummary(e, machine, nil, record.Values))
		}
	}
}

// RunEvents performs the I/O half of every domain.Event MatchedEvents returns for this write, with
// the same best-effort posture logActivity already has (a failure is logged, never allowed to
// fail the write it's describing). oldValuesOK is the caller's own eventOldValues second return --
// false means its fetch failed, so no Event can be evaluated correctly and none should fire.
//
// machines is threaded through only so rollUpParentStatus can look up the *parent's* own Machine
// (to dispatch its declared Events, Tahap 6) -- every other Service here still only ever touches
// the one machine/record this call already names.
func RunEvents(ctx context.Context, store *data.Store, mailer mail.Mailer, machines map[string]*domain.Machine, machine *domain.Machine, record *data.Record, actorID string, oldValues map[string]any, oldValuesOK bool) {
	if !oldValuesOK {
		return
	}
	for _, e := range behavior.MatchedEvents(machine, oldValues, record.Values) {
		switch e.Then.Name {
		case domain.ServiceLogActivity:
			logActivity(ctx, store, machine.ID, record.ID, actorID, renderEventSummary(e, machine, oldValues, record.Values))
		case domain.ServiceRollupParentStatus:
			rollUpParentStatus(ctx, store, mailer, machines, machine, record, actorID, e.On, *e.Then.Rollup)
		case domain.ServiceSendNotification:
			sendNotification(ctx, store, mailer, machine, record, *e.Then.Notify, renderEventSummary(e, machine, oldValues, record.Values))
		}
	}
}

// RunScheduledEvents is the schedule shape's own dispatcher (domain.Event's third trigger shape),
// called from a ticker (cmd/server/main.go) rather than from a write -- the caller is expected to
// call it once per Workspace, with ctx already carrying that Workspace's scope
// (data.WithWorkspaceScope), since store.ListRecords reads whatever Workspace ctx names.
//
// Idempotency is one check per record, not per declared schedule Event: it looks for an existing
// mch_activity row already carrying this exact rendered summary for (machine, record) -- the same
// marker-prefix convention internal/composition's own (now-retired) logSLABreaches used, and this
// is correct as long as every schedule Event a Machine declares shares one condition, which is
// true of mch_document's two today. A second, differently-conditioned schedule Event on the same
// Machine would need its own marker text to stay distinguishable, not a new mechanism.
func RunScheduledEvents(ctx context.Context, store *data.Store, mailer mail.Mailer, machines []*domain.Machine, now time.Time) error {
	for _, machine := range machines {
		hasSchedule := false
		for _, e := range machine.Events {
			if e.Schedule != nil {
				hasSchedule = true
				break
			}
		}
		if !hasSchedule {
			continue
		}

		records, err := store.ListRecords(ctx, machine.ID)
		if err != nil {
			return fmt.Errorf("scheduled events: listing %s: %w", machine.ID, err)
		}
		activities, err := store.ListRecordsBy(ctx, "mch_activity", "fld_machine_id", machine.ID)
		if err != nil {
			return fmt.Errorf("scheduled events: listing activity for %s: %w", machine.ID, err)
		}

		for _, record := range records {
			matched := behavior.MatchedScheduleEvents(machine, record, now)
			if len(matched) == 0 {
				continue
			}
			marker, hasMarker := scheduleMarker(matched, machine, record)
			if !hasMarker {
				log.Printf("scheduled events: %s %s: no log_activity-shaped schedule event declared to dedupe against, skipping", machine.ID, record.ID)
				continue
			}
			if activityExists(activities, record.ID, marker) {
				continue
			}
			for _, e := range matched {
				switch e.Then.Name {
				case domain.ServiceLogActivity:
					logActivity(ctx, store, machine.ID, record.ID, "", marker)
				case domain.ServiceSendNotification:
					sendNotification(ctx, store, mailer, machine, record, *e.Then.Notify, renderEventSummary(e, machine, nil, record.Values))
				}
			}
		}
	}
	return nil
}

// scheduleMarker is RunScheduledEvents' own dedup key: the rendered summary of whichever matched
// Event uses ServiceLogActivity, since that is the one Activity row the check below can actually
// look for. hasMarker is false when a Machine declares schedule Events with the same condition but
// none of them logs an Activity -- a metadata authoring gap this function refuses to guess past
// (silently notifying on every tick would be worse than not notifying at all).
func scheduleMarker(matched []domain.Event, machine *domain.Machine, record *data.Record) (marker string, hasMarker bool) {
	for _, e := range matched {
		if e.Then.Name == domain.ServiceLogActivity {
			return renderEventSummary(e, machine, nil, record.Values), true
		}
	}
	return "", false
}

// activityExists reports whether an mch_activity row for recordID already carries marker as its
// summary -- generalizing the exact idempotency check internal/composition's own (now-retired)
// logSLABreaches used, so a schedule Event fires at most once per record.
func activityExists(activities []*data.Record, recordID, marker string) bool {
	for _, a := range activities {
		if fmt.Sprint(a.Values["fld_record_id"]) == recordID && displayString(a.Values["fld_summary"]) == marker {
			return true
		}
	}
	return false
}

// rollUpParentStatus is ServiceRollupParentStatus's own I/O half: read every sibling of the record
// just written, let behavior.RollupValue decide from their values, and write the result onto the
// parent. The decision is pure and lives in internal/behavior; only the read and the write are
// here, the same split svc_log_activity already follows. watchField is the Event's own on: --
// the Field whose change triggered this, and whose value every sibling is then read for.
//
// Siblings are re-read rather than derived from the record in hand, so the rollup is always
// computed from what is actually stored -- the same reasoning the hardcoded recomputeDocumentStatus
// this replaced already used.
func rollUpParentStatus(ctx context.Context, store *data.Store, mailer mail.Mailer, machines map[string]*domain.Machine, machine *domain.Machine, record *data.Record, actorID, watchField string, r domain.Rollup) {
	parentID := fmt.Sprint(record.Values[r.ParentField])
	if parentID == "" {
		return
	}
	parentField, ok := machine.FieldByID(r.ParentField)
	if !ok {
		return
	}

	siblings, err := store.ListRecordsBy(ctx, machine.ID, r.ParentField, parentID)
	if err != nil {
		log.Printf("rollup %s: listing children of %s: %v", r.TargetField, parentID, err)
		return
	}
	watched := make([]string, 0, len(siblings))
	for _, s := range siblings {
		watched = append(watched, fmt.Sprint(s.Values[watchField]))
	}

	parent, err := store.GetRecord(ctx, parentField.RelatedMachine, parentID)
	if err != nil {
		log.Printf("rollup %s: reading parent %s: %v", r.TargetField, parentID, err)
		return
	}
	oldParentValues := make(map[string]any, len(parent.Values))
	for k, v := range parent.Values {
		oldParentValues[k] = v
	}
	parent.Values[r.TargetField] = behavior.RollupValue(r, watched)
	if _, err := store.UpdateRecord(ctx, parentField.RelatedMachine, parentID, parent.Values); err != nil {
		log.Printf("rollup %s: writing parent %s: %v", r.TargetField, parentID, err)
		return
	}
	if parentMachine, ok := machines[parentField.RelatedMachine]; ok {
		RunEvents(ctx, store, mailer, machines, parentMachine, parent, actorID, oldParentValues, true)
	}
}

// renderEventSummary fills a Service's own message template -- {old}, {new}, and any Field id in
// braces are its only placeholders, deliberately not a general templating language (the same
// minimalism expression.Comparison already established for Constraint's own condition
// vocabulary). SummaryOverride is used instead of Summary when the Event's own Field just became
// SummaryOverrideWhen.
func renderEventSummary(e domain.Event, m *domain.Machine, oldValues, newValues map[string]any) string {
	tmpl := e.Then.Summary
	if e.Then.SummaryOverrideWhen != "" && fmt.Sprint(newValues[e.On]) == e.Then.SummaryOverrideWhen {
		tmpl = e.Then.SummaryOverride
	}
	tmpl = strings.NewReplacer("{old}", displayString(oldValues[e.On]), "{new}", displayString(newValues[e.On])).Replace(tmpl)
	for _, f := range m.Fields {
		tmpl = strings.ReplaceAll(tmpl, "{"+f.ID+"}", displayString(newValues[f.ID]))
	}
	return tmpl
}
