package composition

import (
	"context"
	"fmt"
	"sort"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/behavior"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// The four states a viewer's own Approval Step can be in on the Assigned to me screen -- the
// filter chips and rendering.AssignedRow.DecisionKey both key on these literals, the same
// composition-writes-the-string convention StepApprover.State already uses (rendering switches on
// the value, composition is the only writer of it).
const (
	AssignedWaiting  = "waiting"
	AssignedNotYet   = "not_yet"
	AssignedApproved = "approved"
	AssignedRejected = "rejected"
)

// Assigned is the Assigned to me screen's whole composed content: every Approval Step that has
// ever named this viewer -- directly, or through a Group they belong to -- across every Document,
// whatever that step's own decision is. Counts travel beside Rows for the same reason
// Inbox.OverdueCount does: the filter chips need them before any status filter narrows the list,
// so they are counted once here rather than recounted per chip in the handler.
type Assigned struct {
	Rows          []rendering.AssignedRow
	WaitingCount  int
	NotYetCount   int
	ApprovedCount int
	RejectedCount int
}

// AssignedToMe answers a different question from ApprovalInbox's Pending: not "what can I decide
// right now" but "what has ever asked for my decision, and where does it stand" -- board 07b's own
// subtitle, "Every document that asked for your approval — directly or through a group you belong
// to. Newest request first." A step already decided still belongs here; ApprovalInbox drops it the
// moment it is.
//
// Bespoke Go, not a declared `where:` filter, and deliberately so -- this is not the simple
// field-equality shape ROADMAP.md's own `$current_user` deferral describes ("a Field compared
// against a written value"). Whether a step is this viewer's own is itself a two-armed question (a
// direct fld_assignee match, or fld_approver_group naming a Group they belong to -- CAP-F24, the
// same dynamic gate authorization.AllowsAction already evaluates for decide/edit/delete), and what
// each row displays depends on a second Machine's sequencing state (behavior.CanAct) and a third
// read (the submission activity, for FROM/REQUESTED). That is the same class of derivation
// composition.ApprovalInbox already is, not a filtered list -- so it follows the identical shape:
// an I/O wrapper here, the actual derivation in a pure function (buildAssigned) that a test can
// call without a database.
func AssignedToMe(ctx context.Context, l *Loader, viewerID string, now time.Time, stepMachine *domain.Machine) (Assigned, error) {
	if viewerID == "" {
		return Assigned{}, nil
	}
	steps, err := l.ListRecords(ctx, action.StepMachineID)
	if err != nil {
		return Assigned{}, err
	}
	documents, err := l.ListRecords(ctx, action.DocumentMachineID)
	if err != nil {
		return Assigned{}, err
	}
	activities, err := l.ListRecords(ctx, "mch_activity")
	if err != nil {
		return Assigned{}, err
	}
	names, err := l.PersonNames(ctx)
	if err != nil {
		return Assigned{}, err
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	myGroups, err := l.store.GroupsForMember(ctx, workspaceID, viewerID)
	if err != nil {
		return Assigned{}, err
	}
	myGroupNames := make(map[string]string, len(myGroups))
	for _, g := range myGroups {
		myGroupNames[g.ID] = g.Name
	}
	return buildAssigned(steps, documents, activities, names, myGroupNames, viewerID, now, stepMachine), nil
}

// buildAssigned is AssignedToMe's whole derivation, over records someone else already fetched --
// the same split buildInbox uses, and for the same reason: which of the four states a step is in,
// and which Document it belongs to, is worth testing without a database.
func buildAssigned(steps, documents, activities []*data.Record, names, myGroupNames map[string]string, viewerID string, now time.Time, stepMachine *domain.Machine) Assigned {
	docByID := make(map[string]*data.Record, len(documents))
	for _, d := range documents {
		docByID[d.ID] = d
	}
	stepsByDoc := make(map[string][]*data.Record, len(documents))
	for _, s := range steps {
		docID := DisplayString(s.Values[action.FieldStepDocument])
		stepsByDoc[docID] = append(stepsByDoc[docID], s)
	}
	submissions := submittersFromActivity(activities)

	var seq *domain.Sequencing
	if stepMachine != nil {
		seq = stepMachine.Sequencing
	}

	var out Assigned
	var built []assignedRow
	for _, s := range steps {
		via, mine := stepBelongsTo(s, viewerID, myGroupNames)
		if !mine {
			continue
		}
		docID := DisplayString(s.Values[action.FieldStepDocument])
		doc := docByID[docID]
		if doc == nil {
			continue
		}
		siblings := stepsByDoc[docID]

		sub := submissions[docID]
		from := names[sub.actor]
		if from == "" {
			from = "someone"
		}
		requestedAt := ""
		if !sub.at.IsZero() {
			requestedAt = sub.at.Format("2 Jan 2006")
		}

		key, label := assignedDecision(seq, doc, s, siblings, via)
		switch key {
		case AssignedWaiting:
			out.WaitingCount++
		case AssignedNotYet:
			out.NotYetCount++
		case AssignedApproved:
			out.ApprovedCount++
		case AssignedRejected:
			out.RejectedCount++
		}

		built = append(built, assignedRow{
			at: sub.at,
			row: rendering.AssignedRow{
				Reference:     action.DocumentReference(doc.SortOrder),
				Title:         DisplayString(doc.Values["fld_title"]),
				DocumentType:  DisplayString(doc.Values["fld_document_type"]),
				From:          from,
				RequestedAt:   requestedAt,
				Status:        DisplayString(doc.Values[action.FieldDocumentStatus]),
				DecisionKey:   key,
				DecisionLabel: label,
				// The Review screen, not this Document's generic record page -- the same
				// destination My Documents links since 2026-09-24, and for the same reason
				// (rendering.detailBackLink): composition.ReviewStepForDocument resolves which
				// step it opens on, so a viewer with their own step here lands on exactly that one.
				Href: fmt.Sprintf("/machines/%s/records/%s/review", action.DocumentMachineID, doc.ID),
			},
		})
	}

	sort.SliceStable(built, func(i, j int) bool { return built[i].at.After(built[j].at) })
	out.Rows = make([]rendering.AssignedRow, len(built))
	for i, b := range built {
		out.Rows[i] = b.row
	}
	return out
}

// assignedRow is one row before sorting: the rendered shape plus the raw timestamp it sorts by.
// The timestamp does not travel on rendering.AssignedRow itself -- that type has no reason to
// carry two representations (a time.Time and its own already-formatted RequestedAt string) of the
// same fact, when only this function's own sort needs the former.
type assignedRow struct {
	row rendering.AssignedRow
	at  time.Time
}

// stepBelongsTo reports whether s is viewerID's own step -- directly assigned, or held by a Group
// they belong to (CAP-F24's two-armed shape, the same one authorization.AllowsAction's dynamic
// gate evaluates for decide/edit/delete). via names the Group when it is that arm, "" when it is
// the direct one -- assignedDecision's own "· via {group}" clause reads it.
func stepBelongsTo(s *data.Record, viewerID string, myGroups map[string]string) (via string, mine bool) {
	if DisplayString(s.Values[action.FieldStepApproverType]) == "Group" {
		group := DisplayString(s.Values[action.FieldStepApproverGroup])
		if name, ok := myGroups[group]; ok {
			return name, true
		}
		return "", false
	}
	return "", DisplayString(s.Values[action.FieldStepAssignee]) == viewerID
}

// assignedDecision is the YOUR DECISION column: which of the four states s is in, and the sentence
// naming it. Approved/rejected read the step's own fld_decision directly; a still-pending step
// asks the same question the Inbox's own decision bar asks (behavior.CanAct) to tell "waiting for
// you" from "waiting on someone earlier in the chain".
func assignedDecision(seq *domain.Sequencing, doc, step *data.Record, siblings []*data.Record, via string) (key, label string) {
	switch DisplayString(step.Values[action.FieldStepDecision]) {
	case action.DecisionApproved:
		if at := step.UpdatedAt; !at.IsZero() {
			return AssignedApproved, "You approved " + at.Format("2 Jan 2006")
		}
		return AssignedApproved, "You approved this step"
	case action.DecisionRejected:
		if at := step.UpdatedAt; !at.IsZero() {
			return AssignedRejected, "You rejected " + at.Format("2 Jan 2006")
		}
		return AssignedRejected, "You rejected this step"
	}
	if behavior.CanAct(seq, doc, step, siblings) {
		return AssignedWaiting, "Waiting for your decision"
	}
	seqLabel := DisplayString(step.Values[action.FieldStepSequence])
	label = fmt.Sprintf("Not yet your turn — step %s of %d", seqLabel, len(siblings))
	if via != "" {
		label += " · via " + via
	}
	return AssignedNotYet, label
}
