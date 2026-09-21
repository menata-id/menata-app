package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/behavior"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// showApprovalInbox serves Case 3's Approval Inbox (ROADMAP.md Phase 15 Step 1,
// document-approval.html). Composing the inbox is composition.ApprovalInbox's job; what stays
// here is the part that is genuinely about HTTP -- reading the ?filter= tab and reducing the
// composed list to it.
// inboxTabMine is the ?tab= value that selects My Documents.
//
// It is the one thing about that view metadata does not state. nav_my_documents declares *where*
// it is (/approval-inbox?tab=mine, metadata/applications/document-approval.yaml) and the strip
// reads the entry's label and href straight from there -- but what the value *means* ("compose
// every Document this identity submitted, not the steps awaiting it") is a composition decision,
// and no declared filter can express "submitted by me" yet: the submitter is derived from the
// activity log, not stored on the Document. Forward-checkable pointer: 007 §7.7 Filter over a
// Dataset, which is what would let this branch be declared rather than switched on here.
const inboxTabMine = "mine"

func showApprovalInbox(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		inbox, err := composition.ApprovalInbox(ctx, composition.NewLoader(store, machines), userID, time.Now(), machines[action.StepMachineID])
		if err != nil {
			serverError(w, err)
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

		// tab and filter compose rather than replace each other: the SLA chips carry ?filter= and
		// must survive a tab switch, which is why each tab link preserves the other's value
		// (rendering.inboxTabs) instead of being a bare href.
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		// Only the switch-workspace href is wanted here now. This used to also take the viewer's
		// Workspace role, to hide the Admin and Groups entries this strip once carried; the strip
		// projects this Application's own two navigation items and nothing else since 2026-09-21,
		// so there is no Workspace destination on it left to hide.
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		// tab is passed through raw as well as decided on: this handler owns what ?tab=mine *means*
		// (compose Mine rather than Pending), while the strip across the top owns which declared
		// navigation item it marks current, by comparing this value against each item's own declared
		// route. Two readings of one query value, each in the plane that owns that question.
		tab := req.URL.Query().Get("tab")
		render(ctx, w, rendering.ApprovalInboxPage(
			filters, pending, inbox.Mine,
			tab == inboxTabMine, tab, filterKey,
			chrome.WorkspaceName, chrome.Viewer(), switchHref,
		))
	}
}

// showPendingCount serves pageShell's own nav badge (ROADMAP.md Phase 21 round 2, Step J) -- the
// same Pending count showApprovalInbox already computes, reused rather than threading it through
// every one of pageShell's ~20 callers as a new parameter. Renders nothing at all when there's
// nothing pending, so the badge's own :empty CSS rule hides it instead of showing "(0)".
func showPendingCount(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		inbox, err := composition.ApprovalInbox(req.Context(), composition.NewLoader(store, machines), userID, time.Now(), machines[action.StepMachineID])
		if err != nil {
			serverError(w, err)
			return
		}
		if len(inbox.Pending) == 0 {
			return
		}
		_, _ = fmt.Fprintf(w, "%d", len(inbox.Pending))
	}
}

// decideStep is Case 3's core Action (ROADMAP.md Phase 12): Approve or Reject one Approval Step,
// enforcing action.CanDecide's sequencing rule, then dispatching whatever Events the Machine
// declares -- which is how the parent Document's own status now follows its steps
// (mch_approval_step's evt_step_decision_rollup), instead of a hardcoded recompute here.
// Still hardcoded to mch_approval_step/mch_document in every other respect, matching
// internal/action's own scope -- not a generic action-dispatch route.
//
// The order below is the contract, not a convenience: identity is checked before the Document is
// even fetched (005-runtime-lifecycle.md "Security Ordering", 007 §20), and sequencing is checked
// before anything is written.
func decideStep(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if machine.ID != action.StepMachineID {
			http.Error(w, "this machine has no decide action", http.StatusNotFound)
			return
		}
		decision, ok := submittedDecision(w, req)
		if !ok {
			return
		}

		ctx := req.Context()
		id := chi.URLParam(req, "id")
		step, err := store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}

		// mch_approval_step declares prm_decide_own_step, so only the step's own fld_assignee
		// gets past here (ROADMAP.md Phase 16).
		actor := currentActor(req, store, cfg)
		if !authorization.AllowsAction(machine, domain.ActionDecide, step.Values, actor) {
			http.Error(w, "this approval step is assigned to someone else", http.StatusForbidden)
			return
		}

		if !declaredDecision(w, machine, step, decision) {
			return
		}

		documentID, _ := step.Values[action.FieldStepDocument].(string)
		document, ok := decidableDocument(w, ctx, store, machine, step, documentID)
		if !ok {
			return
		}

		if !applyApprovalSignature(w, req, ctx, store, files, actor.ID, step, decision) {
			return
		}

		// Read the stored values before the write, so the declared Events below see a real
		// before/after pair. applyApprovalSignature only mutates step.Values in memory, so
		// nothing has been persisted for this step yet.
		oldValues, oldValuesOK := eventOldValues(req, store, machine, id)

		step.Values[action.FieldStepDecision] = decision
		if _, err := store.UpdateRecord(ctx, machine.ID, id, step.Values); err != nil {
			serverError(w, err)
			return
		}

		if decision == action.DecisionApproved {
			// Owner request, 2026-09-19: every approval, not just the one that completes the
			// whole Document, should land in the PDF immediately -- signDocument recomposites
			// every currently-approved step's stamp plus the growing status banner from
			// scratch each time, so this is safe to call on every approval, not just the last.
			signDocument(ctx, store, files, document, documentID)
		}

		runEvents(ctx, store, machine, step, actor.ID, oldValues, oldValuesOK)

		logActivity(ctx, store, action.DocumentMachineID, documentID, actor.ID,
			fmt.Sprintf("Step %v %s", toDisplayString(step.Values[action.FieldStepSequence]), decision))

		redirectTo(w, req, "/machines/"+action.DocumentMachineID+"/records/"+documentID)
	}
}

// declaredDecision holds this Machine's own declared state model against the decision being made
// (Case 03 Fase 7): the move has to be an edge mch_approval_step actually declares, and one
// reserved for the decide Action.
//
// It runs before anything is written and before the signature is even captured, alongside the
// other two pre-write guards, per decideStep's own ordering contract.
//
// It closes a real hole neither of those guards covered. Sequencing only locks a step behind an
// *earlier* one, and only when its Document is sequential -- so on a parallel Document an
// already-approved step could be decided again, flipping approved -> rejected and re-running the
// rollup onto the Document. Nothing anywhere said a decision was final; mch_approval_step's
// transitions do, by declaring no edge that leaves `approved` or `rejected`.
func declaredDecision(w http.ResponseWriter, machine *domain.Machine, step *data.Record, decision string) bool {
	if err := behavior.CheckTransitions(machine, domain.ActionDecide, step.Values,
		map[string]any{action.FieldStepDecision: decision}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// submittedDecision reads the decision field, accepting only the two values an Approval Step can
// actually move to -- "pending" is where it starts, never somewhere it is sent.
func submittedDecision(w http.ResponseWriter, req *http.Request) (string, bool) {
	if err := req.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return "", false
	}
	decision := req.FormValue("decision")
	if decision != action.DecisionApproved && decision != action.DecisionRejected {
		http.Error(w, "decision must be approved or rejected", http.StatusUnprocessableEntity)
		return "", false
	}
	return decision, true
}

// applyApprovalSignature is Approve's own signature-capture gate (owner request, 2026-09-19): an
// approved decision needs a signature image on file for this actor -- either their own reusable
// mch_signature, or a fresh one captured by decideButtons's canvas modal (rendering/detail.templ)
// and carried in this same POST. Reject never reaches here -- no stamp is ever composited for a
// rejection.
//
// This is the write-time half of the gate; hasSavedSignature threaded into decideButtons is the
// render-time half deciding whether the modal shows up in the first place. Both check the same
// thing so a client that bypasses the modal (or has JS disabled) still can't approve without one.
func applyApprovalSignature(w http.ResponseWriter, req *http.Request, ctx context.Context, store *data.Store, files *storage.Store, actor string, step *data.Record, decision string) bool {
	if decision != action.DecisionApproved {
		return true
	}
	saved, err := hasSavedSignature(ctx, store, actor)
	if err != nil {
		serverError(w, err)
		return false
	}
	if saved {
		return true
	}

	image, err := decodeSignatureDataURL(req.PostFormValue("signature_image"))
	if err != nil {
		http.Error(w, "a signature is required to approve: "+err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	key, err := files.Save(action.StepMachineID, action.FieldStepSignatureImage, "signature.png", bytes.NewReader(image))
	if err != nil {
		serverError(w, err)
		return false
	}
	step.Values[action.FieldStepSignatureImage] = key

	if req.PostFormValue("save_signature") != "" {
		values := map[string]any{
			action.FieldSignatureOwner: actor,
			action.FieldSignatureImage: key,
		}
		if _, err := store.CreateRecord(ctx, action.SignatureMachineID, values); err != nil {
			serverError(w, err)
			return false
		}
	}
	return true
}

// decodeSignatureDataURL decodes the canvas's own `data:image/png;base64,...` payload and
// verifies (http.DetectContentType, the same mechanism rejectDangerousUpload uses for generic
// uploads) that the decoded bytes are really a PNG -- a strict allowlist rather than
// rejectDangerousUpload's denylist, appropriate here because a canvas always emits real PNG
// bytes; anything else means the client didn't actually send what the modal produces.
func decodeSignatureDataURL(dataURL string) ([]byte, error) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, fmt.Errorf("no signature image was submitted")
	}
	image, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, prefix))
	if err != nil {
		return nil, fmt.Errorf("signature image was not valid base64")
	}
	if len(image) == 0 || http.DetectContentType(image) != "image/png" {
		return nil, fmt.Errorf("signature image was not a valid PNG")
	}
	return image, nil
}

// decidableDocument fetches the step's parent Document and refuses the decision if the Document's
// mode says an earlier step has not been decided yet.
func decidableDocument(w http.ResponseWriter, ctx context.Context, store *data.Store, machine *domain.Machine, step *data.Record, documentID string) (*data.Record, bool) {
	document, err := store.GetRecord(ctx, action.DocumentMachineID, documentID)
	if err != nil {
		recordError(w, err)
		return nil, false
	}
	siblings, err := store.ListRecordsBy(ctx, machine.ID, action.FieldStepDocument, documentID)
	if err != nil {
		serverError(w, err)
		return nil, false
	}
	if !behavior.CanAct(machine.Sequencing, document, step, siblings) {
		http.Error(w, "an earlier step has not been decided yet", http.StatusUnprocessableEntity)
		return nil, false
	}
	return document, true
}
