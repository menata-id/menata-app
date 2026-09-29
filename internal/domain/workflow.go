package domain

// Workflow is how an Application says which runtime workflow engine it runs on, and which of its
// own Machines plays each role in that engine (ROADMAP.md "Document Approval: closing the last
// three layers", Stage A; the audit is menata-app-document's
// audits/2026-09-28-kajian-metadata-based-document-approval.md, Gap C).
//
// It replaces a literal. internal/action's approval engine used to decide "is this Machine mine"
// by matching the Application id `app_document_approval` and the Machine ids `mch_document` /
// `mch_approval_step` -- so the engine woke for exactly one Application name holding exactly two
// Machine names. 001 Principle #2 puts application *behavior* in the runtime, and an engine that
// only recognises one set of names is behavior owned by one application: the same approval
// mechanics could not be renamed, varied, or installed a second time under a different name, even
// though nothing about them is specific to those names.
//
// Declared on the Application rather than on the Machine, because it is a statement about a *set*
// of Machines working together -- which one is the document, which one is the step -- and a Machine
// declaring its own role would leave "whose engine?" unanswered, which is the half that went wrong
// (see Machine.ApplicationID). The loader stamps each named Machine's WorkflowEngine/WorkflowRole
// from here (internal/metadata.stampWorkflowRoles), so the declaration lives in exactly one place
// and the index over it is derived, never retyped (001 Principle #8).
//
// Two Applications in one Workspace may bind the same engine, deliberately: that is the capability
// this exists to give, and each one's own Machines then answer for themselves. Nothing here is
// process-wide -- a Workspace's own copy of an Application declares its own binding, so two
// Workspaces may bind the same engine to Machines named differently.
type Workflow struct {
	// Engine is one of KnownWorkflowEngines.
	Engine string
	// Roles maps each role name the engine requires to the id of the Machine playing it, selected
	// from this Application's own `machines:` list (validated at load, since a role filled by
	// another Application's Machine would hand the engine records it does not own).
	Roles map[string]string
}

// MachineForRole reports the Machine id playing one role of this binding, or "" when this
// Application declares no binding or no such role.
func (w *Workflow) MachineForRole(role string) string {
	if w == nil {
		return ""
	}
	return w.Roles[role]
}

// MachinesInWorkflowRole is every Machine in this Workspace that some Application casts in role of
// engine -- normally one, and the reason this returns a slice rather than a Machine is that
// Workspace isolation made "zero, one, or several" the real range: a Workspace may install two
// approval Applications, or none at all. The Workspace-level chrome that summarises across
// Applications (Workspace Home's inbox card, the navigation's pending badge) has to add them up
// rather than assume the one.
//
// Order follows the Workspace's own Machine order, so a caller that does have to pick reads the
// manifest's order rather than a map's.
func (w Workspace) MachinesInWorkflowRole(engine, role string) []*Machine {
	var out []*Machine
	for _, m := range w.Machines {
		if m.WorkflowEngine == engine && m.WorkflowRole == role {
			out = append(out, m)
		}
	}
	return out
}

// MachineInWorkflowRole is the single Machine *one request* means by (engine, role), and the
// precedence is the whole content of this method:
//
//  1. The Application the request is in, when it binds this engine. This is the answer for every
//     screen inside an Application -- which is most of them, since the /machines/{id}/... surface
//     resolves its Application from the Machine in the path.
//
//  2. Otherwise the Workspace's *sole* Machine in that role. Some real routes belong to no
//     Application: the submit wizard's own /documents/new and POST /documents are named by no
//     navigation item (nav_new_approval was deleted on 2026-09-21, owner instruction -- declaring a
//     submit form as a menu entry was the wrong shape for it), so nothing resolves an Application
//     for them, and a Workspace with one approval Application still has exactly one right answer.
//
//     "Otherwise" includes a request inside an Application that binds *no* engine, which is a
//     fall-through rather than a nil: /dashboard is declared by Project Management and its Document
//     status tiles must still see the approval Application installed beside it. Only an Application
//     that binds this engine scopes the question; one that binds nothing is not a scope at all.
//
//  3. nil when there is none, or when several Applications bind the engine and nothing scopes the
//     question. **That is a real, named limitation, not an oversight**: with two approval
//     Applications installed, /documents/new is genuinely ambiguous, and the honest fix is a route
//     that says which Application it belongs to rather than a guess here. Callers treat nil as
//     "this Workspace has no such screen" -- a 404, or a feature that does not render.
func (w Workspace) MachineInWorkflowRole(engine, role, applicationID string) *Machine {
	if applicationID != "" {
		for _, app := range w.Applications {
			if app.ID != applicationID || app.Workflow == nil || app.Workflow.Engine != engine {
				continue
			}
			id := app.Workflow.MachineForRole(role)
			for _, m := range w.Machines {
				if m.ID == id {
					return m
				}
			}
			return nil
		}
	}
	if candidates := w.MachinesInWorkflowRole(engine, role); len(candidates) == 1 {
		return candidates[0]
	}
	return nil
}

// WorkflowEngineDocumentApproval is the sequential/parallel multi-step approval engine
// internal/action implements: a document whose approval steps are decided in order or all at once
// (006-runtime-model.md names Approve and Reject as Actions; ROADMAP.md Phase 12).
const WorkflowEngineDocumentApproval = "document_approval"

const (
	// WorkflowRoleDocument is the Machine whose records are the thing being approved.
	WorkflowRoleDocument = "document"
	// WorkflowRoleStep is the Machine whose records are the individual approval steps, one per
	// approver, ordered when the document asks for sequential mode.
	WorkflowRoleStep = "step"
	// WorkflowRoleSignature is the Machine holding a person's own *reusable* signature image, so an
	// approver who saved one does not redraw it on every decision. Optional: an approval Application
	// that casts none still works, capturing a one-time image onto the step itself instead.
	WorkflowRoleSignature = "signature"
	// WorkflowRoleFlowTemplate and WorkflowRoleFlowTemplateStep hold a saved default approval flow
	// per document type (CAP-V28), so a submitter is offered the steps their last submission of that
	// type used. Optional, and optional *together* -- a template with no steps saves nothing, so
	// casting one without the other is refused at load.
	WorkflowRoleFlowTemplate     = "flow_template"
	WorkflowRoleFlowTemplateStep = "flow_template_step"
)

// WorkflowEngineSpec is one engine's cast: the roles a binding must fill, and the ones it may.
//
// The split is the difference between "this engine cannot run" and "this engine runs without that
// feature". A missing Required role would fail at the first request rather than at load, so it is a
// load error; a missing Optional one is a legitimate, smaller installation, and every code path that
// reads an optional role has to say what it does without it (internal/web's own early returns).
type WorkflowEngineSpec struct {
	Required []string
	Optional []string
	// Answers maps each role to the derivations that role is responsible for answering (the
	// Derivation* constants in resolution.go), each marked required or feature-dependent. It is what
	// makes an empty derivation triageable, and it was added because measurement showed nothing else
	// could be.
	//
	// **Measured 2026-09-29, and it refuted two framings before this one.** Probing every derivation
	// over every Machine in both installed Workspaces gave 173 empties out of 308 -- useless, since
	// mch_user legitimately declares no `decide` transition. Narrowing to Machines *cast in a role*
	// gave 38 of 50 still empty -- also useless, and for a reason worth keeping: a Document is not
	// decided (its steps are), a signature store has no state model, and nothing decides a flow
	// template. Every one of those 38 is a derivation belonging to a *different* role. Meanwhile the
	// Machine cast as `step` resolved all five of its own, in both Workspaces.
	//
	// So the discriminator is neither "is it empty" nor "is the Machine cast" but "does this role owe
	// this answer" -- and no declaration stated that. This field is that statement. Without it, the
	// 38 correct empties and Stage E1's genuinely-broken ones are the same observation, which is
	// precisely why no gate over these numbers could be written before.
	Answers map[string][]RoleAnswer
}

// RoleAnswer is one derivation a role is responsible for, and whether the whole feature it belongs to
// is optional.
//
// **The distinction was found by mutation, not designed in.** The first version of Answers listed
// nine flat derivations for the `step` role, read off what the real Machine resolved -- which
// conflated "declares it" with "owes it". Deleting `signature_placement:` from the real
// approval_step.yaml showed the file still *loads*: a step Machine with no signature block is an
// approval Application that captures no signatures, a legitimate smaller installation exactly like an
// uncast optional role. Reporting it as a defect would be the same over-reporting StatusNotApplicable
// exists to prevent, one level up.
//
// So Optional does not mean "may be half-declared". A feature entirely absent is NotApplicable; a
// feature *partly* declared is still Undeclared, because a signature placement with an image Field
// and no coordinates would stamp at (0,0). Absent is a choice, partial is a bug.
//
// Everything not marked Optional is already enforced at load -- removing `order_field` from the real
// file is refused by validateSequencing rather than reaching this surface -- so the required entries
// here are a second line, not the only one.
type RoleAnswer struct {
	Derivation string
	Optional   bool
}

// AnswersFor returns the derivations role owes an answer for, or nil for a role this engine does not
// know. A nil result and an empty one are the same to callers on purpose: a role that owes nothing
// and a role that does not exist both mean "expect no derivations here".
func (s WorkflowEngineSpec) AnswersFor(role string) []RoleAnswer { return s.Answers[role] }

// Owes reports whether role is responsible for derivation, and whether that responsibility is
// feature-dependent. Explain asks it to choose between StatusUndeclared (owed and missing) and
// StatusNotApplicable (never owed, or an optional feature not installed).
func (s WorkflowEngineSpec) Owes(role, derivation string) (owes, optional bool) {
	for _, a := range s.Answers[role] {
		if a.Derivation == derivation {
			return true, a.Optional
		}
	}
	return false, false
}

// Roles is every role this engine knows, required first -- for the "declares no role %q" message,
// which is otherwise the one validation failure that leaves an author guessing.
func (s WorkflowEngineSpec) Roles() []string {
	return append(append([]string{}, s.Required...), s.Optional...)
}

// IsRequired reports whether role is one this engine cannot run without.
func (s WorkflowEngineSpec) IsRequired(role string) bool {
	for _, r := range s.Required {
		if r == role {
			return true
		}
	}
	return false
}

// KnownWorkflowEngines is the closed set of engines a Workflow may name, mapped to the cast each one
// takes -- the same static-seam discipline KnownActions/KnownServices already establish (007 §14),
// and for the same reason: an engine name the runtime cannot realize must fail at load rather than
// leaving a screen that quietly offers nothing.
//
// Every engine here is implemented in Go and stays there (002: physical strategies remain
// runtime-owned) -- what metadata decides is which Machines it runs over.
var KnownWorkflowEngines = map[string]WorkflowEngineSpec{
	WorkflowEngineDocumentApproval: {
		Required: []string{WorkflowRoleDocument, WorkflowRoleStep},
		Optional: []string{WorkflowRoleSignature, WorkflowRoleFlowTemplate, WorkflowRoleFlowTemplateStep},
		// Read off the real Machines rather than composed from this engine's wish list: each entry is
		// a derivation that Machine's role actually resolved in both installed Workspaces, or -- for
		// the optional roles -- the block that role's own accessor reads.
		//
		// `step` owes parent and composite_source as well as its own five, because it is where the
		// relation to the Document and the compositing Event are declared; `document` owes only its
		// status Field, which is why four of its five probes are correctly empty.
		Answers: map[string][]RoleAnswer{
			WorkflowRoleStep: {
				{Derivation: DerivationDecision},
				{Derivation: DerivationOpenValue},
				{Derivation: DerivationOrder},
				{Derivation: DerivationActor},
				{Derivation: DerivationActorType},
				{Derivation: DerivationActorGroup},
				{Derivation: DerivationParent},
				{Derivation: DerivationStatusTargets},
				// Optional: an Action whose whole effect is the status move its own transitions already
				// declare writes no companion Fields, which is a complete Action rather than a broken one.
				// Two of the installed Machines are exactly that.
				{Derivation: DerivationActionWrites, Optional: true},
				// Both features rather than requirements, proven by deleting each from the real
				// approval_step.yaml and watching it still load: an approval Application may capture no
				// signatures, and may composite no PDF.
				{Derivation: DerivationSignaturePlacement, Optional: true},
				{Derivation: DerivationCompositeSource, Optional: true},
			},
			WorkflowRoleDocument: {{Derivation: DerivationDocumentStatus}},
			// Each optional role's own block is optional *within* the role too: a Workspace that casts
			// the role at all is installing the feature, but the cast and the block are two separate
			// declarations and a Machine can be cast before it is finished.
			WorkflowRoleSignature:        {{Derivation: DerivationSignatureStore}},
			WorkflowRoleFlowTemplate:     {{Derivation: DerivationFlowTemplate}},
			WorkflowRoleFlowTemplateStep: {{Derivation: DerivationFlowTemplateStep}},
		},
	},
}
