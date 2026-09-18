package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showApprovalInbox serves Case 3's Approval Inbox (ROADMAP.md Phase 15 Step 1,
// document-approval.html). Composing the inbox is composition.ApprovalInbox's job; what stays
// here is the part that is genuinely about HTTP -- reading the ?filter= tab and reducing the
// composed list to it.
func showApprovalInbox(machines map[string]*domain.Machine, store *data.Store, appName string, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		inbox, err := composition.ApprovalInbox(ctx, composition.NewLoader(store, machines), userID, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		filterKey := req.URL.Query().Get("filter")
		filters := []rendering.SLAFilter{
			{Key: "all", Label: "All", Count: len(inbox.Pending), Active: filterKey == "" || filterKey == "all"},
			{Key: "overdue", Label: "Overdue", Count: inbox.OverdueCount, Active: filterKey == composition.BucketOverdue},
			{Key: "today", Label: "Due today", Count: inbox.TodayCount, Active: filterKey == composition.BucketToday},
		}

		pending := inbox.Pending
		if filterKey == composition.BucketOverdue || filterKey == composition.BucketToday {
			pending = nil
			for i, c := range inbox.Pending {
				if inbox.Buckets[i] == filterKey {
					pending = append(pending, c)
				}
			}
		}

		rendering.ApprovalInboxPage(filters, pending, inbox.Mine, appName).Render(ctx, w)
	}
}

// decideStep is Case 3's core Action (ROADMAP.md Phase 12): Approve or Reject one Approval Step,
// enforcing action.CanDecide's sequencing rule, then recomputing and saving the parent
// Document's own aggregate status. Hardcoded to mch_approval_step/mch_document, matching
// internal/action's own scope -- not a generic action-dispatch route.
func decideStep(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if machine.ID != action.StepMachineID {
			http.Error(w, "this machine has no decide action", http.StatusNotFound)
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		decision := req.FormValue("decision")
		if decision != action.DecisionApproved && decision != action.DecisionRejected {
			http.Error(w, "decision must be approved or rejected", http.StatusUnprocessableEntity)
			return
		}

		ctx := req.Context()
		id := chi.URLParam(req, "id")
		step, err := store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}

		// Authorization before any further work, per 005-runtime-lifecycle.md "Security Ordering"
		// and 007 §20: mch_approval_step declares prm_decide_own_step, so only the step's own
		// fld_assignee gets past here (ROADMAP.md Phase 16).
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		if !authorization.AllowsAction(machine, domain.ActionDecide, step.Values, actor) {
			http.Error(w, "this approval step is assigned to someone else", http.StatusForbidden)
			return
		}

		documentID, _ := step.Values[action.FieldStepDocument].(string)
		document, err := store.GetRecord(ctx, action.DocumentMachineID, documentID)
		if err != nil {
			recordError(w, err)
			return
		}
		siblings, err := store.ListRecordsBy(ctx, machine.ID, action.FieldStepDocument, documentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		mode, _ := document.Values[action.FieldDocumentMode].(string)
		if !action.CanDecide(mode, step, siblings) {
			http.Error(w, "an earlier step has not been decided yet", http.StatusUnprocessableEntity)
			return
		}

		step.Values[action.FieldStepDecision] = decision
		if _, err := store.UpdateRecord(ctx, machine.ID, id, step.Values); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		updatedSiblings, err := store.ListRecordsBy(ctx, machine.ID, action.FieldStepDocument, documentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		document.Values[action.FieldDocumentStatus] = action.DocumentStatus(updatedSiblings)
		if _, err := store.UpdateRecord(ctx, action.DocumentMachineID, documentID, document.Values); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		logActivity(ctx, store, action.DocumentMachineID, documentID, actor, fmt.Sprintf("Step %v %s", toDisplayString(step.Values[action.FieldStepSequence]), decision))

		documentURL := "/machines/" + action.DocumentMachineID + "/records/" + documentID
		if req.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", documentURL)
			return
		}
		http.Redirect(w, req, documentURL, http.StatusSeeOther)
	}
}
