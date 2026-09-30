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
	"menata.app/internal/execution"
	"menata.app/internal/mail"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// showApprovalInbox serves Case 3's Approval Inbox (ROADMAP.md Phase 15 Step 1,
// document-approval.html) and, since 2026-09-24, its two sibling tabs (My Documents, Assigned to
// me) that share its route. Composing each tab's own content is composition's job
// (ApprovalInbox, AssignedToMe); what stays here is genuinely about HTTP -- reading ?tab=/
// ?filter=/?status= and reducing the composed list to them.
//
// rendering.TabMine/TabAssigned name the same two ?tab= values nav_my_documents/
// nav_assigned_to_me declare their own routes with -- see that package's own doc comment on why
// the constant lives there and not here. tab=="" (no query at all) is Pending, the route's own
// default and nav_approval_inbox's own declared route.
//
// Each tab composes only its own content, not all three: ApprovalInbox reads five record sets (a
// pure read now -- SLA-breach detection moved off this path entirely onto
// execution.RunScheduledEvents, Flow 2 canvas re-audit 2026-09-27), and AssignedToMe reads a sixth
// (this identity's own Groups) that neither of the other two tabs needs. Composing every tab on
// every request would be the query-budget mistake ROADMAP.md's own performance audit already
// found once on this exact screen (showPendingCount's doc comment tells that story); this avoids
// repeating it on the tab that is new.
func showApprovalInbox(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		machines := machinesFor(ctx)
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		tab := req.URL.Query().Get("tab")
		// q is My Documents' and Assigned to me's own search box (Flow 2 mockup) -- the Pending
		// tab carries none, matching its own board (Inbox.dc.html has no search input at all).
		q := req.URL.Query().Get("q")

		var (
			filters, mineFilters, assignedFilters []rendering.FilterChip
			pending, drafts, mine                 []rendering.PendingApprovalCard
			assignedRows                          []rendering.AssignedRow
			truncated                             rendering.Truncation
			err                                   error
		)
		if tab == rendering.TabAssigned {
			assignedRows, assignedFilters, truncated, err = assignedTabContent(ctx, store, machines, userID, req.URL.Query().Get("status"), q)
		} else {
			pending, drafts, mine, filters, mineFilters, truncated, err = pendingTabContent(ctx, store, machines, userID, req.URL.Query().Get("filter"), req.URL.Query().Get("status"), q)
		}
		if err != nil {
			serverError(w, err)
			return
		}

		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		// Only the switch-workspace href is wanted here now. This used to also take the viewer's
		// Workspace role, to hide the Admin and Groups entries this strip once carried; the strip
		// projects this Application's own declared navigation items and nothing else since
		// 2026-09-21, so there is no Workspace destination on it left to hide.
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.ApprovalInboxPage(
			filters, pending, drafts, mine, mineFilters, assignedRows, assignedFilters, truncated, tab, q,
			chrome.WorkspaceName, chrome.Viewer(), switchHref,
		))
	}
}

// pendingTabContent composes the Pending and My Documents tabs from one call to
// composition.ApprovalInbox, which already returns both (Pending is its own list, My Documents is
// Inbox.Mine) -- the two have shared one composed read since before this tab strip had a third
// tab, and giving My Documents a call of its own would read every record set a second time for a
// screen that already has the answer. filterKey narrows Pending by SLA bucket; mineStatusKey/q
// narrow My Documents by its own status chips (composition.MineFilters) and search box, in that
// order, so a search always narrows within whichever chip is already active. drafts/mine are then
// composition.SplitDrafts' own partition of that narrowed list (Flow 2 gap study Tahap 4,
// 2026-09-25) -- so an active status chip still narrows what lands in either half, e.g. the Draft
// chip active leaves mine (Submitted) empty.
func pendingTabContent(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, userID, filterKey, mineStatusKey, q string) (pending, drafts, mine []rendering.PendingApprovalCard, filters, mineFilters []rendering.FilterChip, truncated rendering.Truncation, err error) {
	inbox, err := composition.ApprovalInbox(ctx, composition.NewLoader(store, machines), userID, time.Now(), machineForStep(ctx), machineForDocument(ctx))
	if err != nil {
		return nil, nil, nil, nil, nil, rendering.Truncation{}, err
	}
	filters = []rendering.FilterChip{
		{Key: "all", Label: "All", Count: len(inbox.Pending), Active: filterKey == "" || filterKey == "all"},
		{Key: "overdue", Label: "Overdue", Count: inbox.OverdueCount, Active: filterKey == composition.BucketOverdue},
		{Key: "today", Label: "Due today", Count: inbox.TodayCount, Active: filterKey == composition.BucketToday},
	}
	pending = inbox.Pending
	if filterKey == composition.BucketOverdue || filterKey == composition.BucketToday {
		pending = nil
		for i, c := range inbox.Pending {
			if inbox.Buckets[i] == filterKey {
				pending = append(pending, c)
			}
		}
	}
	mineFilters = composition.MineFilters(inbox.Mine, mineStatusKey)
	narrowedMine := composition.SearchCards(composition.FilterCardsByStatus(inbox.Mine, mineStatusKey), q)
	drafts, mine = composition.SplitDrafts(narrowedMine)
	// The bound applies to the Document set all three tabs are built from, so it travels with whichever
	// tab ran rather than being a per-tab fact. ?filter=/?status=/?q= narrowing happens *after* it and
	// cannot clear it: a list narrowed from a capped set is still capped.
	return pending, drafts, mine, filters, mineFilters, inbox.Truncated, nil
}

// assignedTabContent composes the Assigned to me tab: composition.AssignedToMe's own reads (a
// sixth record set, this identity's Groups, that neither sibling tab touches), reduced by
// ?status= the same way pendingTabContent reduces Pending by ?filter=, then by ?q= within whichever
// status chip is active.
func assignedTabContent(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, userID, statusKey, q string) (rows []rendering.AssignedRow, filters []rendering.FilterChip, truncated rendering.Truncation, err error) {
	assigned, err := composition.AssignedToMe(ctx, composition.NewLoader(store, machines), userID, time.Now(), machineForStep(ctx), machineForDocument(ctx))
	if err != nil {
		return nil, nil, rendering.Truncation{}, err
	}
	filters = []rendering.FilterChip{
		{Key: "all", Label: "All", Count: len(assigned.Rows), Active: statusKey == "" || statusKey == "all"},
		{Key: composition.AssignedWaiting, Label: "Waiting for me", Count: assigned.WaitingCount, Active: statusKey == composition.AssignedWaiting},
		{Key: composition.AssignedNotYet, Label: "Not yet my turn", Count: assigned.NotYetCount, Active: statusKey == composition.AssignedNotYet},
		{Key: composition.AssignedApproved, Label: "Approved by me", Count: assigned.ApprovedCount, Active: statusKey == composition.AssignedApproved},
		{Key: composition.AssignedRejected, Label: "Rejected by me", Count: assigned.RejectedCount, Active: statusKey == composition.AssignedRejected},
	}
	rows = assigned.Rows
	if statusKey != "" && statusKey != "all" {
		rows = nil
		for _, r := range assigned.Rows {
			if r.DecisionKey == statusKey {
				rows = append(rows, r)
			}
		}
	}
	rows = composition.SearchAssignedRows(rows, q)
	return rows, filters, assigned.Truncated, nil
}

// showPendingCount serves pageShell's own nav badge (ROADMAP.md Phase 21 round 2, Step J) -- the
// same Pending count showApprovalInbox reports, computed from the same predicate rather than
// threaded through every one of pageShell's ~20 callers as a new parameter. Renders nothing at all
// when there's nothing pending, so the badge's own :empty CSS rule hides it instead of showing
// "(0)".
//
// It calls PendingApprovalCount, not ApprovalInbox. Composing the whole inbox for one integer read
// the activity log and every member's name to build cards this endpoint discards, and wrote SLA
// breach rows as a side effect -- on an endpoint that fires on a third of all requests. The two
// still cannot disagree, because both select through composition's own pendingStepsFor.
func showPendingCount(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		pending, err := pendingApprovalTotal(req.Context(), store, userID)
		if err != nil {
			serverError(w, err)
			return
		}
		if pending == 0 {
			return
		}
		_, _ = fmt.Fprintf(w, "%d", pending)
	}
}

// pendingApprovalTotal is the badge's and Workspace Home's own number: how many decisions this
// identity owes, across **every** approval Application installed in this Workspace.
//
// Both callers sit outside any Application -- the badge renders on a third of all requests, on
// whatever page, and Workspace Home belongs to the Workspace -- so neither can resolve "the" step
// Machine the way an Application's own screen does. Until 2026-09-28 both simply read
// machines[action.StepMachineID], which assumed exactly one approval Application and one name for
// it; Workspace isolation made "zero, one, or several" the real range, and a Workspace with two
// would have counted only whichever one happened to own that id.
//
// One Loader across the loop on purpose: two Applications sharing a Machine is impossible, so there
// is nothing to dedupe between iterations, but the Loader also memoizes mch_activity and the member
// names both compositions want.
func pendingApprovalTotal(ctx context.Context, store *data.Store, userID string) (int, error) {
	ws := rendering.CurrentWorkspace(ctx)
	l := composition.NewLoader(store, machinesFor(ctx))
	total := 0
	for _, steps := range ws.MachinesInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleStep) {
		documents := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, steps.ApplicationID)
		n, err := composition.PendingApprovalCount(ctx, l, userID, steps, documents)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
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
func decideStep(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if !action.IsStep(machine) {
			http.Error(w, "this machine has no decide action", http.StatusNotFound)
			return
		}
		decision, ok := submittedDecision(w, req, machine)
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

		f := action.DeclaredFields(machine, machineForDocument(ctx))
		documentID, _ := step.Values[f.Parent].(string)
		if _, ok := decidableDocument(w, ctx, store, machine, step, documentID); !ok {
			return
		}

		// Captured before anything mutates step.Values, so the declared Events below see a real
		// before/after pair -- see snapshotValues for why this is a copy rather than a re-read.
		oldValues := snapshotValues(step.Values)

		if !applyApprovalSignature(w, req, ctx, store, files, machine, actor.ID, step, decision) {
			return
		}

		// What this Action writes is declared, not typed here (mch_approval_step's own actions: block,
		// Stage B): the status move comes from the Transition that names `decide`, and the companion
		// write -- the decider's name, snapshotted -- comes from the effect. Until 2026-09-28 these were
		// two lines naming fld_decision and fld_decided_by_name, which is why the approval engine could
		// only ever write a step shaped exactly like Document Approval's own.
		if _, err := action.ApplyStatusMove(machine, domain.ActionDecide, step.Values, decision); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if _, err := action.ApplyEffect(machine, domain.ActionDecide, step.Values, action.EffectInput{
			Submitted: decision, ActorID: actor.ID, ActorName: deciderName(ctx, store, actor.ID),
		}); err != nil {
			serverError(w, err)
			return
		}
		if _, err := store.UpdateRecord(ctx, machine.ID, id, step.Values); err != nil {
			serverError(w, err)
			return
		}

		afterDecision(ctx, store, files, mailer, machine, step, documentID, decision, actor.ID, oldValues)
		redirectTo(w, req, "/machines/"+approvalMachineID(ctx, domain.WorkflowRoleDocument)+"/records/"+documentID)
	}
}

// afterDecision is everything a recorded decision sets in motion, extracted from decideStep so the
// handler stays inside its own budget (internal/conformance.TestHandlersStaySmall) -- and because
// none of it can fail the request: the decision is already saved, so each of the three is
// best-effort in its own way.
func afterDecision(ctx context.Context, store *data.Store, files *storage.Store, mailer mail.Mailer,
	machine *domain.Machine, step *data.Record, documentID, decision, actorID string, oldValues map[string]any,
) {
	// Every declared Event this decision fires, including the one that produces the signed PDF: that
	// was a direct signDocument call here until Stage C (2026-09-28), which made the single most
	// visible outcome of an approval reachable only from Go. mch_approval_step declares it now
	// (evt_step_signed_document), and it still recomposites from the original on every approval rather
	// than only the last -- owner request, 2026-09-19 -- because that is what the Service does.
	//
	// oldValues came from a read decideStep already made and checked, so there is no second fetch
	// left to fail.
	execution.RunEvents(ctx, execution.Services{Store: store, Mailer: mailer, Files: files}, machinesFor(ctx), machine, step, actorID, oldValues, true)

	logActivity(ctx, store, approvalMachineID(ctx, domain.WorkflowRoleDocument), documentID, actorID,
		fmt.Sprintf("Step %v %s", toDisplayString(step.Values[machine.OrderField()]), decision))
}

// reviseDocument is Rejected -> Draft (Flow 2 gap study Tahap 4, 2026-09-25): "Revise" on a
// Rejected row in My Documents. Mirrors decideStep's own ordering contract (identity checked
// before anything is fetched twice, business-state checked before anything is written).
//
// Step 5 below is a deliberate, documented exception to action.CanDeleteApprovalStep's own rule
// ("a decided step... is the audit trail Phase 16/17 exist to protect... blocked outright") --
// reached through store.DeleteRecord directly rather than the generic delete route, so that
// function's own guard never runs. It is not a bypass of that rule so much as a different question:
// CanDeleteApprovalStep protects a decided step from being deleted *in place*, on a Document that
// otherwise keeps its own outcome. Revise is not that -- it is the one place in this runtime that
// deliberately starts a Document's approval history over, and doing that with the old, decided
// steps of a *finished* rejected cycle still attached would corrupt the new one: evt_step_decision_
// rollup recomputes a Document's status from *every* step naming it, so a fresh pending step sitting
// beside an old rejected one would immediately roll the Document back to rejected before anyone
// decides anything. The human-readable fact ("Step N rejected") survives regardless, in
// mch_activity, which is append-only -- only the structured step row itself (assignee, signature
// image, decided-by-name) is traded away, and only for a step whose Document is about to be
// resubmitted from scratch.
func reviseDocument(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if !action.IsDocument(machine) {
			http.Error(w, "this machine has no revise action", http.StatusNotFound)
			return
		}

		ctx := req.Context()
		id := chi.URLParam(req, "id")
		document, err := store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}

		actor := currentActor(req, store, cfg)
		if !authorization.AllowsAction(machine, domain.ActionRevise, document.Values, actor) {
			http.Error(w, "not allowed to revise this document", http.StatusForbidden)
			return
		}

		// Not behavior.CheckTransitions: internal/conformance.TestDocumentStatusIsDerivedNotSettable
		// holds that no mch_document transition on fld_status may declare an Action, so this move is
		// gated by a plain business-state check instead (action.CanReviseDocument), the same posture
		// action.CanDeleteDocument already takes for delete.
		if ok, reason := action.CanReviseDocument(toDisplayString(document.Values[machine.StatusField()])); !ok {
			http.Error(w, reason, http.StatusUnprocessableEntity)
			return
		}

		// Declared, not typed here (mch_document's own actions: block, Stage B): `revise` writes this
		// Machine's status Field back to the value the declaration names. The Field's state model
		// deliberately declares no person-performed edge (see that block's own note), so this is a
		// literal write rather than a Transition -- reachable only from this route and this Permission,
		// never from the generic update route, which is what that invariant protects.
		if _, err := action.ApplyEffect(machine, domain.ActionRevise, document.Values, action.EffectInput{
			ActorID: actor.ID,
		}); err != nil {
			serverError(w, err)
			return
		}
		if _, err := store.UpdateRecord(ctx, machine.ID, id, document.Values); err != nil {
			serverError(w, err)
			return
		}

		stepMachineID := approvalMachineID(ctx, domain.WorkflowRoleStep)
		steps, err := store.ListRecordsBy(ctx, stepMachineID, action.DeclaredFields(machineForStep(ctx), machine).Parent, id)
		if err != nil {
			serverError(w, err)
			return
		}
		for _, s := range steps {
			if err := store.DeleteRecord(ctx, stepMachineID, s.ID); err != nil {
				serverError(w, err)
				return
			}
		}

		logActivity(ctx, store, machine.ID, id, actor.ID,
			fmt.Sprintf("%q moved back to draft for revision", toDisplayString(document.Values["fld_title"])))

		redirectTo(w, req, navRouteByID(ctx, "nav_my_documents"))
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
		map[string]any{machine.ActionField(domain.ActionDecide): decision}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// submittedDecision reads the decision the form carried, accepting only a value this Machine's own
// declared Transitions say `decide` may move its status Field to.
//
// It compared against the literals action.DecisionApproved/DecisionRejected until 2026-09-28 (Stage B).
// Those two strings are exactly the `to:` values of the two edges declaring `action: decide`, so the
// check was a copy of the declaration rather than a rule of its own -- and it meant any other Machine
// bound to this engine had to use those same two words. The answer is now read from the Machine, which
// is also what makes "a decision is never sent back to pending" a fact of the metadata rather than of
// this function.
func submittedDecision(w http.ResponseWriter, req *http.Request, machine *domain.Machine) (string, bool) {
	if err := req.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return "", false
	}
	decision := req.FormValue("decision")
	field := machine.ActionField(domain.ActionDecide)
	targets := machine.ActionTargets(domain.ActionDecide, field)
	for _, target := range targets {
		if decision == target {
			return decision, true
		}
	}
	http.Error(w, fmt.Sprintf("decision must be one of %v", targets), http.StatusUnprocessableEntity)
	return "", false
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
func applyApprovalSignature(w http.ResponseWriter, req *http.Request, ctx context.Context, store *data.Store, files *storage.Store, stepMachine *domain.Machine, actor string, step *data.Record, decision string) bool {
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

	// Which Field holds the one-time image is this Machine's own declaration (Stage D). A step
	// Machine declaring none has nowhere to put a captured signature, so the capture is refused
	// rather than written to an empty key -- the explicit handling "empty means undeclared" asks for.
	imageField := action.SignatureFields(stepMachine).ImageField
	if imageField == "" {
		http.Error(w, "this machine declares no signature field to capture into", http.StatusUnprocessableEntity)
		return false
	}
	image, err := decodeSignatureDataURL(req.PostFormValue("signature_image"))
	if err != nil {
		http.Error(w, "a signature is required to approve: "+err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	key, err := files.Save(stepMachine.ID, imageField, "signature.png", bytes.NewReader(image))
	if err != nil {
		serverError(w, err)
		return false
	}
	step.Values[imageField] = key

	// "Save this signature for next time" is only offerable where the Application casts a Machine
	// in the signature role -- an optional one (domain.WorkflowEngineSpec). Where it casts none, the
	// one-time image written onto the step above is the whole feature, and there is nowhere to keep a
	// reusable copy; the checkbox is not rendered either (reviewdocument.templ's own gate). A store
	// declaring no signature_store: block is the same case for the same reason.
	if req.PostFormValue("save_signature") != "" {
		if err := saveReusableSignature(ctx, store, actor, key); err != nil {
			serverError(w, err)
			return false
		}
	}
	return true
}

// saveReusableSignature keeps this actor's captured image as their own reusable signature, where the
// Application casts a Machine in the optional signature role and that Machine says which Fields hold
// an owner and an image (signature_store:, Stage D).
//
// Doing nothing is the correct outcome for either absence, and both are ordinary rather than
// exceptional: casting no store means the feature was not installed, and declaring no block means the
// Machine cast is not shaped like a signature store. Writing under an empty Field id instead would
// produce a record nothing can ever find by owner.
func saveReusableSignature(ctx context.Context, store *data.Store, actor, key string) error {
	signatures := approvalMachine(ctx, domain.WorkflowRoleSignature)
	if signatures == nil {
		return nil
	}
	declared := action.StoreFields(signatures)
	if declared.OwnerField == "" || declared.ImageField == "" {
		return nil
	}
	_, err := store.CreateRecord(ctx, signatures.ID, map[string]any{
		declared.OwnerField: actor,
		declared.ImageField: key,
	})
	return err
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
//
// Its record is no longer used for anything else: the signed PDF is produced by a declared Event that
// reads its own parent (Stage C, 2026-09-28), so the sequencing check is all this fetch is for now.
func decidableDocument(w http.ResponseWriter, ctx context.Context, store *data.Store, machine *domain.Machine, step *data.Record, documentID string) (*data.Record, bool) {
	document, err := store.GetRecord(ctx, approvalMachineID(ctx, domain.WorkflowRoleDocument), documentID)
	if err != nil {
		recordError(w, err)
		return nil, false
	}
	siblings, err := store.ListRecordsBy(ctx, machine.ID, action.DeclaredFields(machine, approvalMachine(ctx, domain.WorkflowRoleDocument)).Parent, documentID)
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

// deciderName resolves the deciding actor's name once, at the moment of the decision, so
// fld_decided_by_name can hold what the signature actually said (metadata/approval_step.yaml).
// Empty when the actor has no membership in this Workspace -- the shared admin credential's
// placeholder identity -- which signing.go then falls back on as it always did.
func deciderName(ctx context.Context, store *data.Store, actorID string) string {
	workspaceID, _ := data.WorkspaceScope(ctx)
	names, err := store.MemberNames(ctx, workspaceID)
	if err != nil {
		return ""
	}
	return names[actorID]
}
