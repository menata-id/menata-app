package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"menata.app/internal/action"
	"menata.app/internal/behavior"
	"menata.app/internal/composition"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
)

// pageFromQuery reads a 1-indexed ?page= query param, clamped to [1, totalPages], defaulting to
// 1 for anything missing or unparseable.
func pageFromQuery(req *http.Request, totalPages int) int {
	page, err := strconv.Atoi(req.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	if page > totalPages {
		return totalPages
	}
	return page
}

// isDetailContext reports whether an HTMX request targets the record-detail page's own
// container, as opposed to a table row or board card -- the same fragments serve both contexts
// (ROADMAP.md Phase 8).
func isDetailContext(req *http.Request) bool {
	return req.Header.Get("HX-Target") == "record-detail"
}

func toDisplayString(v any) string { return composition.DisplayString(v) }

func recordError(w http.ResponseWriter, err error) {
	if errors.Is(err, data.ErrRecordNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	serverError(w, err)
}

// serverError logs err server-side and returns a generic 500 body -- the caller's error may carry
// internal detail (a DB error, a file path) that has no business reaching an HTTP client.
//
// A cancelled context is logged differently and deliberately: it means the client went away
// mid-request (a browser navigating on from a page still loading), which is not a fault of this
// server and has no recipient left to receive a 500. The log review on 2026-09-22 found these
// filed as `internal error: context canceled` among real faults -- two lines out of six hours,
// so this is about keeping the error channel meaningful rather than about volume. It is the same
// reasoning render's own doc comment already gives for its twin case, applied to the half that
// happens before the response starts.
func serverError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		log.Printf("request abandoned by client: %v", err)
		// Still a 5xx on the wire: nobody is reading it, but a handler must not fall through to
		// writing a success body after its work was cut short.
		http.Error(w, "request cancelled", http.StatusServiceUnavailable)
		return
	}
	log.Printf("internal error: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// render writes c to w, logging any failure. By the time a templ Component starts rendering, the
// response has already started (headers may be sent), so there is no error response left to give
// -- a failure here is almost always the client disconnecting mid-response, worth a log line and
// nothing more.
func render(ctx context.Context, w http.ResponseWriter, c templ.Component) {
	if err := c.Render(ctx, w); err != nil {
		log.Printf("render failed: %v", err)
	}
}

// writeJSON encodes v to w as JSON, logging any write failure -- same reasoning as render: the
// response has already started, so there's nothing left to do but note it happened.
func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write JSON response failed: %v", err)
	}
}

// logActivity appends one mch_activity record (ROADMAP.md Phase 13) -- an ordinary Machine, not
// a new system-data-source concept (007 SS4.1's admission question). Best-effort: a logging
// failure is not allowed to fail the real operation it's describing, only get logged itself.
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
// -- a failure here must never fail the write it's describing.
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

// runCreateEvents is runEvents' own counterpart for the create path: every domain.Event a Machine
// declares OnCreate (behavior.MatchedCreateEvents) fires once, unconditionally, for the record
// just created -- generalizing what used to be a hardcoded per-Machine switch here
// (logRecordCreated, ROADMAP.md Phase 21 round 2 Step I) into the same declarative mechanism
// runEvents already established for field-change Events. renderEventSummary is reused as-is with
// oldValues nil: a creation Event's own summary template only ever uses {field_id} placeholders,
// never {old}/{new}, so nil resolves harmlessly.
func runCreateEvents(ctx context.Context, store *data.Store, mailer mail.Mailer, machine *domain.Machine, record *data.Record, actorID string) {
	for _, e := range behavior.MatchedCreateEvents(machine) {
		switch e.Then.Name {
		case domain.ServiceLogActivity:
			logActivity(ctx, store, machine.ID, record.ID, actorID, renderEventSummary(e, machine, nil, record.Values))
		case domain.ServiceSendNotification:
			sendNotification(ctx, store, mailer, machine, record, *e.Then.Notify, renderEventSummary(e, machine, nil, record.Values))
		}
	}
}

// maxUploadBytes bounds one multipart request body (ROADMAP.md Phase 11) -- generous enough for
// a real PDF or a handful of images, small enough that a malicious upload can't exhaust disk.
const maxUploadBytes = 20 << 20 // 20MB

// eventOldValues fetches a record's pre-write values, but only when machine actually declares an
// Event -- the same fast-path allowsRecordEdit already takes for edit Permission, avoiding the
// extra query for the overwhelming majority of writes that declare none. ok is false only when a
// declared Event exists but the fetch itself failed -- the caller must skip Event dispatch
// entirely then (runEvents), not pass a nil map through: MatchedEvents can't tell "no Events
// declared" apart from "old value unknown," and comparing against a nil map would make almost any
// new value look like a change, firing a bogus Event off a transient read error (code-review
// finding, 2026-09-19) instead of the intended "isn't logged, never a reason to fail the edit."
func eventOldValues(req *http.Request, store *data.Store, machine *domain.Machine, id string) (values map[string]any, ok bool) {
	if len(machine.Events) == 0 {
		return nil, true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		return nil, false
	}
	return existing.Values, true
}

// snapshotValues copies a record's values so a caller that is about to mutate them in place still
// holds what they were.
//
// It is the alternative to eventOldValues for a handler that has *already* read the record.
// decideStep used to call both: one store.GetRecord to fetch the step, then a second of the very
// same row a few lines later, purely because applyApprovalSignature mutates step.Values in place
// and left the handler with no copy of the original. That second read was the largest single
// entry in /decide's `repeated=7` and had nothing to do with the write, which had not happened
// yet. eventOldValues remains right for its other two callers (record.go, api.go), which build
// new values from a form and genuinely have not read the old ones.
//
// Shallow on purpose: Events compare Field values, which are scalars, and an in-place mutation
// replaces map entries rather than reaching inside one.
func snapshotValues(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}

// runEvents performs the I/O half of every domain.Event MatchedEvents returns for this write,
// with the same best-effort posture logActivity already has (a failure is logged, never allowed
// to fail the write it's describing). oldValuesOK is eventOldValues' own second return -- false
// means its fetch failed, so no Event can be evaluated correctly and none should fire.
//
// machines is threaded through only so rollUpParentStatus can look up the *parent's* own Machine
// (to dispatch its declared Events, Tahap 6) -- every other Service here still only ever touches
// the one machine/record this call already names.
func runEvents(ctx context.Context, store *data.Store, mailer mail.Mailer, machines map[string]*domain.Machine, machine *domain.Machine, record *data.Record, actorID string, oldValues map[string]any, oldValuesOK bool) {
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

// rollUpParentStatus is ServiceRollupParentStatus's own I/O half: read every sibling of the record
// just written, let behavior.RollupValue decide from their values, and write the result onto the
// parent. The decision is pure and lives in internal/behavior; only the read and the write are
// here, the same split svc_log_activity already follows. watchField is the Event's own on: --
// the Field whose change triggered this, and whose value every sibling is then read for.
//
// Siblings are re-read rather than derived from the record in hand, so the rollup is always
// computed from what is actually stored -- the same reasoning the hardcoded recomputeDocumentStatus
// this replaces already used.
//
// Closes a gap this function's own comment used to name: until 2026-09-26 this write never ran
// Events on the *parent*, because this runtime dispatches Events from handlers rather than from
// data.Store.UpdateRecord, and no Machine declared an Event that would have needed it. Tahap 6's
// "my document was decided" notification is exactly such an Event
// (mch_document.evt_document_approved_notify/_rejected_notify, on fld_status), so this now
// snapshots the parent's own old values before the write and calls runEvents on it after a
// successful one -- the same dispatch any handler would perform, one level deep (no Machine here
// has a second rollup level to recurse into; not guarded against, because nothing forces the case).
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
	oldParentValues := snapshotValues(parent.Values)
	parent.Values[r.TargetField] = behavior.RollupValue(r, watched)
	if _, err := store.UpdateRecord(ctx, parentField.RelatedMachine, parentID, parent.Values); err != nil {
		log.Printf("rollup %s: writing parent %s: %v", r.TargetField, parentID, err)
		return
	}
	if parentMachine, ok := machines[parentField.RelatedMachine]; ok {
		runEvents(ctx, store, mailer, machines, parentMachine, parent, actorID, oldParentValues, true)
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
	tmpl = strings.NewReplacer("{old}", toDisplayString(oldValues[e.On]), "{new}", toDisplayString(newValues[e.On])).Replace(tmpl)
	for _, f := range m.Fields {
		tmpl = strings.ReplaceAll(tmpl, "{"+f.ID+"}", toDisplayString(newValues[f.ID]))
	}
	return tmpl
}

// redirectTo sends the visitor to url, the way the caller asked to be sent. HTMX swaps a fragment
// into the current page and would otherwise follow a 303 and swap a whole document into it, so it
// gets an HX-Redirect header instead; anything else gets an ordinary redirect.
func redirectTo(w http.ResponseWriter, req *http.Request, url string) {
	if req.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		return
	}
	http.Redirect(w, req, url, http.StatusSeeOther)
}

// validRecord runs the two checks every create has to clear: the record's own shape, and the
// existence of whatever it points at.
func validRecord(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, values map[string]any) bool {
	if err := data.ValidateRecord(machine, values); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	if err := data.ValidateRelations(req.Context(), store, machine, values); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	return true
}
