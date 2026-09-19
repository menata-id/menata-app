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
)

// recordLabel is a Record's display label -- its Machine's first Field's value, the same
// label-field convention used throughout (loadRelationOptions, rendering.recordTitle).
func recordLabel(m *domain.Machine, r *data.Record) string {
	if len(m.Fields) == 0 {
		return r.ID
	}
	return toDisplayString(r.Values[m.Fields[0].ID])
}

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
func serverError(w http.ResponseWriter, err error) {
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

// logRecordCreated logs the same per-Machine "submitted"/"created" Activity event a new record's
// creation gets today, regardless of whether it arrived through the HTML form route or the JSON
// API -- factored out (ROADMAP.md Phase 21 round 2, Step I) so both paths log identically instead
// of the rule living in two places that could drift out of sync.
func logRecordCreated(ctx context.Context, store *data.Store, machine *domain.Machine, record *data.Record, actorID string) {
	switch machine.ID {
	case action.DocumentMachineID:
		logActivity(ctx, store, machine.ID, record.ID, actorID, fmt.Sprintf("%q submitted", toDisplayString(record.Values["fld_title"])))
	case "mch_task", "mch_project":
		logActivity(ctx, store, machine.ID, record.ID, actorID, fmt.Sprintf("%q created", recordLabel(machine, record)))
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

// runEvents performs the I/O half of every domain.Event MatchedEvents returns for this write --
// currently exactly one Service, ServiceLogActivity, with the same best-effort posture
// logActivity already has (a failure is logged, never allowed to fail the write it's describing).
// oldValuesOK is eventOldValues' own second return -- false means its fetch failed, so no Event
// can be evaluated correctly and none should fire.
func runEvents(ctx context.Context, store *data.Store, machine *domain.Machine, record *data.Record, actorID string, oldValues map[string]any, oldValuesOK bool) {
	if !oldValuesOK {
		return
	}
	for _, e := range behavior.MatchedEvents(machine, oldValues, record.Values) {
		if e.Then.Name == domain.ServiceLogActivity {
			logActivity(ctx, store, machine.ID, record.ID, actorID, renderEventSummary(e, machine, oldValues, record.Values))
		}
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
