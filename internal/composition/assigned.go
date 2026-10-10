package composition

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/behavior"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/expression"
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
	Rows []rendering.AssignedRow
	// Same bound, same reason as Inbox.Truncated: board 07b says "every document that asked for your
	// approval", and a capped list saying that is wrong rather than merely incomplete.
	Truncated     rendering.Truncation
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
func AssignedToMe(ctx context.Context, l *Loader, viewerID string, viewerGroups []data.Group, now time.Time, stepMachine, docMachine *domain.Machine) (Assigned, error) {
	if viewerID == "" {
		return Assigned{}, nil
	}
	if stepMachine == nil || docMachine == nil {
		return Assigned{}, nil // no approval Application here -- see ApprovalInbox's own note
	}
	// Declared correlation (ds_documents_with_steps, 007 §7.5), same as the Inbox's.
	sel, err := l.SelectRelated(ctx, documentsWithStepsDataset, expression.Context{})
	if err != nil {
		return Assigned{}, err
	}
	names, err := l.PersonNames(ctx)
	if err != nil {
		return Assigned{}, err
	}
	// viewerGroups comes from the caller's already-resolved membership: reading them here again
	// was the repeated `groups for member` on 176 of 857 /approval-inbox visits (log review
	// 2026-10-05), because identity resolution had already read them once per request.
	myGroupNames := make(map[string]string, len(viewerGroups))
	for _, g := range viewerGroups {
		myGroupNames[g.ID] = g.Name
	}
	return buildAssigned(sel, names, myGroupNames, viewerID, now, stepMachine, docMachine), nil
}

// SearchAssignedRows is composition.SearchCards' own counterpart for this screen's row shape
// (Flow 2 mockup: "Search title or DOC number…", the same placeholder My Documents' box carries).
// An empty q returns rows unchanged.
func SearchAssignedRows(rows []rendering.AssignedRow, q string) []rendering.AssignedRow {
	if q == "" {
		return rows
	}
	needle := strings.ToLower(q)
	out := make([]rendering.AssignedRow, 0, len(rows))
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Title), needle) || strings.Contains(strings.ToLower(r.Reference), needle) {
			out = append(out, r)
		}
	}
	return out
}

// buildAssigned is AssignedToMe's whole derivation, over records someone else already fetched --
// the same split buildInbox uses, and for the same reason: which of the four states a step is in,
// and which Document it belongs to, is worth testing without a database.
func buildAssigned(sel Selection, names, myGroupNames map[string]string, viewerID string, now time.Time, stepMachine, docMachine *domain.Machine) Assigned {
	documents := sel.Records
	// See buildInbox: one derivation for every Field id this screen reads.
	f := action.DeclaredFields(stepMachine, docMachine)
	// See buildInbox: docByID is a lookup over one set, stepsByDoc is the correlation and is declared.
	docByID := make(map[string]*data.Record, len(documents))
	stepsByDoc := make(map[string][]*data.Record, len(documents))
	var steps []*data.Record
	for _, d := range documents {
		docByID[d.ID] = d
		children := sel.Related(documentStepsRelation, d.ID)
		stepsByDoc[d.ID] = children
		steps = append(steps, children...)
	}
	submissions := submissionsOf(documents, f.Submitter)

	var seq *domain.Sequencing
	if stepMachine != nil {
		seq = stepMachine.Sequencing
	}

	var out Assigned
	// See Inbox.Truncated: the Data Plane's answer, carried, not recomputed.
	out.Truncated = rendering.Truncation{Limit: sel.Limit, Hit: sel.Truncated}
	var built []assignedRow
	for _, s := range steps {
		via, mine := stepBelongsTo(s, viewerID, myGroupNames, f)
		if !mine {
			continue
		}
		docID := DisplayString(s.Values[f.Parent])
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

		key, label := assignedDecision(seq, doc, s, siblings, f)
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

		// The time left is only worth a line while the decision is still the viewer's to make.
		var sla experience.SLABadge
		link := ""
		if key == AssignedWaiting {
			sla = experience.ResolveSLABadge(doc.Values["fld_due_date"], now)
			link = "Review →"
		}

		built = append(built, assignedRow{
			at: sub.at,
			row: rendering.AssignedRow{
				Reference:     action.DocumentReference(doc.SortOrder),
				Title:         DisplayString(doc.Values["fld_title"]),
				DocumentType:  DisplayString(doc.Values["fld_document_type"]),
				From:          from,
				RequestedAt:   requestedAt,
				Status:        DisplayString(doc.Values[f.DocumentStatus]),
				DecisionKey:   key,
				DecisionLabel: label,
				Via:           via,
				SLA:           sla,
				Action:        link,
				// The Review screen, not this Document's generic record page -- the same
				// destination My Documents links since 2026-09-24, and for the same reason
				// (rendering.detailBackLink): composition.ReviewStepForDocument resolves which
				// step it opens on, so a viewer with their own step here lands on exactly that one.
				Href: fmt.Sprintf("/machines/%s/records/%s/review", docMachine.ID, doc.ID),
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
// the direct one -- AssignedRow.Via carries it.
func stepBelongsTo(s *data.Record, viewerID string, myGroups map[string]string, f action.EngineFields) (via string, mine bool) {
	if DisplayString(s.Values[f.ActorType]) == "Group" {
		group := DisplayString(s.Values[f.ActorGroup])
		if name, ok := myGroups[group]; ok {
			return name, true
		}
		return "", false
	}
	return "", DisplayString(s.Values[f.Actor]) == viewerID
}

// assignedDecision is the YOUR DECISION column: which of the four states s is in, and the sentence
// naming it. The Group a step came through is not part of the sentence: it is AssignedRow.Via, drawn
// on every Group row whatever its state (board 07b). Approved/rejected read the step's own fld_decision directly; a still-pending step
// asks the same question the Inbox's own decision bar asks (behavior.CanAct) to tell "waiting for
// you" from "waiting on someone earlier in the chain".
func assignedDecision(seq *domain.Sequencing, doc, step *data.Record, siblings []*data.Record, f action.EngineFields) (key, label string) {
	switch DisplayString(step.Values[f.Decision]) {
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
	seqLabel := DisplayString(step.Values[f.Order])
	return AssignedNotYet, fmt.Sprintf("Not yet your turn — step %s of %d", seqLabel, len(siblings))
}
