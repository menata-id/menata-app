package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

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
	http.Error(w, err.Error(), http.StatusInternalServerError)
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

// maxUploadBytes bounds one multipart request body (ROADMAP.md Phase 11) -- generous enough for
// a real PDF or a handful of images, small enough that a malicious upload can't exhaust disk.
const maxUploadBytes = 20 << 20 // 20MB

// taskMachineID is the one Machine this package still names directly: the record-update route
// logs a Task's status move as a Project Activity event (ROADMAP.md Phase 14), which is a rule
// about that specific Machine and has no home in generic transport code. Case 3's own ids live
// in internal/action for the same reason.
const taskMachineID = "mch_task"

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
