package action

import (
	"strings"

	"menata.app/internal/domain"
)

// ExplainCast explains every derivation this engine makes over one Workspace's cast, as
// []domain.Resolution -- the inspection surface 001 #6's second clause requires, 004 §Inference asks
// for by name, and 005 Phase 4 calls "inspectable enough to explain important runtime decisions".
//
// applicationID picks whose binding to read when a Workspace holds more than one, exactly as
// domain.Workspace.MachineInWorkflowRole takes it; "" means "the Workspace's sole binding", which is
// how a diagnostics caller with no Application in hand asks.
//
// **It reports every derivation for every role, including the ones that do not apply.** Omitting
// those would hide the larger half: 38 of 50 derivations on cast Machines are legitimately empty
// because they belong to another role, and silence about them is what made Stage E1's genuinely
// broken ones look ordinary. A reader has to be able to see "this role does not answer that" as a
// stated answer rather than infer it from an absence.
func ExplainCast(ws domain.Workspace, engine, applicationID string) []domain.Resolution {
	spec, known := domain.KnownWorkflowEngines[engine]
	if !known {
		return nil
	}

	roles := spec.Roles()
	machines := make(map[string]*domain.Machine, len(roles))
	for _, role := range roles {
		machines[role] = ws.MachineInWorkflowRole(engine, role, applicationID)
	}
	doc := machines[domain.WorkflowRoleDocument]

	var out []domain.Resolution
	for _, role := range roles {
		m := machines[role]
		for _, d := range allDerivations {
			out = append(out, explainOne(spec, role, d, m, doc))
		}
	}
	return out
}

// allDerivations is the full question set, in reading order rather than declaration order: a screen
// showing these groups the step's own shape first, then the Document, then the optional features.
var allDerivations = []string{
	domain.DerivationDecision, domain.DerivationOpenValue, domain.DerivationOrder,
	domain.DerivationActor, domain.DerivationActorType, domain.DerivationActorGroup,
	domain.DerivationParent, domain.DerivationSignaturePlacement, domain.DerivationCompositeSource,
	domain.DerivationDocumentStatus, domain.DerivationSignatureStore,
	domain.DerivationFlowTemplate, domain.DerivationFlowTemplateStep,
}

// explainOne resolves one question for one role and, crucially, says which of the four answers it is.
//
// The ordering of the three checks is the whole point and is not interchangeable:
//
//  1. Does this role owe the answer at all? If not, NotApplicable -- and nothing is read, because
//     reading would produce an empty string that looks like a defect.
//  2. Was the *input* available? Only Parent needs a second Machine, and that single case is the
//     /review 404: DeclaredFields(stepMachine, nil) resolved Parent to "" and the screen rendered a
//     404 nobody could distinguish from a Document that genuinely has no steps.
//  3. Otherwise read it. Empty now means the Machine declares nothing for something its role owes --
//     Stage E1's shape, and a real defect.
func explainOne(spec domain.WorkflowEngineSpec, role, derivation string, m, doc *domain.Machine) domain.Resolution {
	r := domain.Resolution{Name: role + "." + derivation, From: sourceOf(derivation)}

	owes, optional := spec.Owes(role, derivation)
	if !owes {
		r.Status = domain.StatusNotApplicable
		return r
	}
	if m == nil {
		// An optional role nothing casts. Not a defect: a Workspace that installed no signature store
		// is a smaller, legitimate installation, and every reader of an optional role already has to
		// say what it does without one.
		r.Status = domain.StatusNotApplicable
		r.From = "role uncast in this Workspace"
		return r
	}
	if derivation == domain.DerivationParent && doc == nil {
		r.Status = domain.StatusInputUnavailable
		r.From = "relation to the document Machine -- but no document Machine was supplied"
		return r
	}

	r.Value = valueOf(derivation, m, doc)
	switch {
	case r.Value != "":
		r.Status = domain.StatusResolved
	case optional && blockAbsent(derivation, m):
		// The feature is not installed. Absent is a choice; partial is a bug, and falls through to
		// Undeclared below -- a signature placement with an image Field and no coordinates stamps at
		// (0,0), so reporting it as "not installed" would hide a live defect behind a legitimate state.
		r.Status = domain.StatusNotApplicable
		r.From = "feature not installed in this Workspace (" + r.From + " declared nowhere)"
	default:
		r.Status = domain.StatusUndeclared
	}
	return r
}

// blockAbsent reports whether an optional derivation's whole declaration is missing, as opposed to
// present and incomplete. Only the optional ones need it, which is why it covers exactly those two.
func blockAbsent(derivation string, m *domain.Machine) bool {
	switch derivation {
	case domain.DerivationSignaturePlacement:
		return m.SignaturePlacement == nil
	case domain.DerivationCompositeSource:
		return CompositeFields(m).SourceField == "" && !declaresCompositeEvent(m)
	}
	return false
}

// declaresCompositeEvent separates "no compositing Event at all" from "an Event whose composite config
// is incomplete" -- the same absent-versus-partial line blockAbsent draws everywhere else.
func declaresCompositeEvent(m *domain.Machine) bool {
	for _, e := range m.Events {
		if e.Then.Name == domain.ServiceCompositeSignedDocument {
			return true
		}
	}
	return false
}

// valueOf reads one derivation off the Machine that owns it. Every branch calls the same accessor
// production calls -- this is an explanation of the real inference, not a second implementation of
// it, which would be the two-lists drift the Derivation* constants exist to avoid.
func valueOf(derivation string, m, doc *domain.Machine) string {
	switch derivation {
	case domain.DerivationDecision:
		return m.ActionField(domain.ActionDecide)
	case domain.DerivationOpenValue:
		return openValueFor(m, domain.ActionDecide)
	case domain.DerivationOrder:
		return m.OrderField()
	case domain.DerivationActor:
		return m.ActorFieldFor(domain.ActionDecide)
	case domain.DerivationActorType:
		if g := m.ActorGateFor(domain.ActionDecide); g != nil {
			return g.ActorTypeField
		}
	case domain.DerivationActorGroup:
		if g := m.ActorGateFor(domain.ActionDecide); g != nil {
			return g.ActorGroupField
		}
	case domain.DerivationParent:
		if f, ok := m.ReferenceFieldTo(doc.ID); ok {
			return f.ID
		}
	case domain.DerivationDocumentStatus:
		return m.StatusField()
	case domain.DerivationSignaturePlacement:
		// The five raw Fields, deliberately *not* SignaturePlacement.Fields(): that method skips the
		// undeclared ones, so a block declaring an image and no coordinates would come back as three
		// happy Field ids. join below has to be able to see the gap to report it.
		p := SignatureFields(m)
		return join([]string{p.ImageField, p.PageField, p.XField, p.YField, p.WidthField})
	case domain.DerivationSignatureStore:
		s := StoreFields(m)
		return join([]string{s.OwnerField, s.ImageField})
	case domain.DerivationCompositeSource:
		return CompositeFields(m).SourceField
	case domain.DerivationFlowTemplate:
		f := FlowTemplateFields(m)
		return join([]string{f.KeyField, f.ModeField})
	case domain.DerivationFlowTemplateStep:
		f := FlowTemplateStepFields(m)
		return join([]string{f.TemplateField, f.OrderField, f.NameField, f.ActorField, f.ActorTypeField, f.ActorGroupField})
	}
	return ""
}

// sourceOf names the declaration each answer is read from. This is the half that makes a resolution
// checkable instead of trusted: a reader who doubts a value can open the named block and see it,
// and a reader looking at an empty one can tell whether the block is missing or merely unread.
func sourceOf(derivation string) string {
	switch derivation {
	case domain.DerivationDecision:
		return "transitions[action=decide].field"
	case domain.DerivationOpenValue:
		return "transitions[action=decide].from"
	case domain.DerivationOrder:
		return "sequencing.order_field"
	case domain.DerivationActor:
		return "permissions[decide].actor_field"
	case domain.DerivationActorType:
		return "permissions[decide] dynamic actor gate .actor_type_field"
	case domain.DerivationActorGroup:
		return "permissions[decide] dynamic actor gate .actor_group_field"
	case domain.DerivationParent:
		return "the relation Field pointing at the document Machine"
	case domain.DerivationDocumentStatus:
		return "transitions[].field -- the Field this Machine's own edges move"
	case domain.DerivationSignaturePlacement:
		return "signature_placement:"
	case domain.DerivationSignatureStore:
		return "signature_store:"
	case domain.DerivationCompositeSource:
		return "events[].then.composite.source_field"
	case domain.DerivationFlowTemplate:
		return "flow_template:"
	case domain.DerivationFlowTemplateStep:
		return "flow_template_step:"
	}
	return ""
}

// join renders a multi-Field block as one value, and returns "" if *any* of its Fields is missing.
//
// All-or-nothing on purpose: a half-declared block is not a smaller feature, it is a broken one --
// a signature placement with an image Field and no coordinates would stamp at (0,0). Reporting it as
// Resolved because one Field answered is the class of half-truth this whole type exists to remove.
func join(fields []string) string {
	for _, f := range fields {
		if f == "" {
			return ""
		}
	}
	return strings.Join(fields, ", ")
}
