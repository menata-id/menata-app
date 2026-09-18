package web

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/rendering"
)

// showApprovalInbox is Case 3's Approval Inbox (ROADMAP.md Phase 15 Step 1,
// document-approval.html): every Approval Step assigned to the current identity, still pending,
// and actually actionable right now (action.CanDecide -- a locked sequential step doesn't belong
// in "pending my approval" even though its own fld_decision is "pending"), filtered by an SLA
// bucket; plus every Document the current identity has submitted. Both rendered as
// rendering.SummaryCard (Phase 15 Step 1's new shared component). "Submitted by" is derived from
// the existing mch_activity log (Phase 13's own "submitted" event) rather than a new Document
// Field -- Document already has no user-editable slot for this, and the data already exists.
func showApprovalInbox(store *data.Store, appName string, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		steps, err := store.ListRecords(ctx, action.StepMachineID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		documents, err := store.ListRecords(ctx, action.DocumentMachineID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		activities, err := store.ListRecords(ctx, "mch_activity")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users, err := store.ListRecords(ctx, "mch_user")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		docByID := make(map[string]*data.Record, len(documents))
		for _, d := range documents {
			docByID[d.ID] = d
		}
		stepsByDoc := make(map[string][]*data.Record, len(documents))
		for _, s := range steps {
			docID := toDisplayString(s.Values[action.FieldStepDocument])
			stepsByDoc[docID] = append(stepsByDoc[docID], s)
		}
		names := make(map[string]string, len(users))
		for _, u := range users {
			names[u.ID] = toDisplayString(u.Values["fld_name"])
		}

		sort.Slice(activities, func(i, j int) bool {
			return activities[i].CreatedAt.Before(activities[j].CreatedAt)
		})
		submitterByDoc := make(map[string]string, len(documents))
		for _, a := range activities {
			docID := toDisplayString(a.Values["fld_record_id"])
			if _, ok := submitterByDoc[docID]; ok {
				continue
			}
			if actor := toDisplayString(a.Values["fld_actor"]); actor != "" {
				submitterByDoc[docID] = actor
			}
		}

		now := time.Now()
		var allPending []rendering.SummaryCard
		var allBuckets []string
		var overdueCount, todayCount int
		for _, s := range steps {
			if toDisplayString(s.Values["fld_assignee"]) != userID {
				continue
			}
			if toDisplayString(s.Values[action.FieldStepDecision]) != action.DecisionPending {
				continue
			}
			docID := toDisplayString(s.Values[action.FieldStepDocument])
			doc := docByID[docID]
			if doc == nil {
				continue
			}
			mode := toDisplayString(doc.Values[action.FieldDocumentMode])
			if !action.CanDecide(mode, s, stepsByDoc[docID]) {
				continue
			}

			approved := 0
			for _, sib := range stepsByDoc[docID] {
				if toDisplayString(sib.Values[action.FieldStepDecision]) == action.DecisionApproved {
					approved++
				}
			}
			title := toDisplayString(doc.Values["fld_title"])
			submitter := names[submitterByDoc[docID]]
			if submitter == "" {
				submitter = "someone"
			}

			bucket := "upcoming"
			if due, err := time.Parse("2006-01-02", toDisplayString(doc.Values["fld_due_date"])); err == nil {
				if status, label := experience.EvaluateSLA(due, now); status == experience.SLAOverdue {
					bucket = "overdue"
					overdueCount++
				} else if label == "Due today" {
					bucket = "today"
					todayCount++
				}
			}
			allPending = append(allPending, rendering.SummaryCard{
				AvatarInitials: initials(submitter),
				Title:          title,
				Subtitle:       fmt.Sprintf("%s · %d/%d approved · Submitted by %s", mode, approved, len(stepsByDoc[docID]), submitter),
				StatusLabel:    toDisplayString(doc.Values["fld_status"]),
				SLADue:         doc.Values["fld_due_date"],
				Href:           fmt.Sprintf("/machines/%s/records/%s", action.StepMachineID, s.ID),
			})
			allBuckets = append(allBuckets, bucket)
		}

		filterKey := req.URL.Query().Get("filter")
		filters := []rendering.SLAFilter{
			{Key: "all", Label: "All", Count: len(allPending), Active: filterKey == "" || filterKey == "all"},
			{Key: "overdue", Label: "Overdue", Count: overdueCount, Active: filterKey == "overdue"},
			{Key: "today", Label: "Due today", Count: todayCount, Active: filterKey == "today"},
		}

		pending := allPending
		if filterKey == "overdue" || filterKey == "today" {
			pending = nil
			for i, c := range allPending {
				if allBuckets[i] == filterKey {
					pending = append(pending, c)
				}
			}
		}

		var mine []rendering.SummaryCard
		for _, d := range documents {
			if submitterByDoc[d.ID] != userID {
				continue
			}
			title := toDisplayString(d.Values["fld_title"])
			mine = append(mine, rendering.SummaryCard{
				AvatarInitials: initials(names[userID]),
				Title:          title,
				Subtitle:       "Submitted by you",
				StatusLabel:    toDisplayString(d.Values["fld_status"]),
				Href:           fmt.Sprintf("/machines/%s/records/%s", action.DocumentMachineID, d.ID),
			})
		}

		rendering.ApprovalInboxPage(filters, pending, mine, appName).Render(ctx, w)
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
