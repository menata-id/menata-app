package domain

// Resolution is one inference explained: what was asked, what it resolved to, which declaration
// that came from, and -- the part the two incidents below turn on -- *why* it is empty when it is.
//
// 001 Principle #6 has two clauses. The first ("Inference is preferred over explicit configuration")
// is the method every stage from Stage B to Stage E2 used. The second is its safety clause, and it
// is a `must`:
//
//	**Inference must be inspectable.** When inference materially affects data access, composition,
//	authorization, rendering, or execution planning, the runtime should be able to expose the
//	resolved result through diagnostics or equivalent tooling. Hidden inference that cannot be
//	explained is not an acceptable substitute for explicit configuration.
//
// 004 §Inference and 005 Phase 4 state the same obligation in their own words. Until this type there
// was no mechanism for any of them: the one diagnostic this runtime had (data.ReadLog) counts
// queries, not inferences.
//
// **Status exists because a bare value cannot tell two different defects apart, and both happened.**
// A derivation that resolves to "" means one of four things, and they are not interchangeable:
//
//   - StatusResolved -- it answered.
//   - StatusUndeclared -- the Machine declares nothing for a question its role *is* supposed to
//     answer. This is Stage E1: every id came back empty for a flow-template step, and the wizard
//     was one throwaway probe away from writing four values under the empty key and saving an
//     approval flow with no approvers.
//   - StatusNotApplicable -- the question does not belong to this role at all. Measured 2026-09-29:
//     38 of 50 derivations on Machines cast in a role are empty, and every one of them is this --
//     a Document is not decided (its steps are), a signature store has no state model, nothing
//     decides a template. Without this status those 38 are indistinguishable from the one above,
//     which is exactly why no gate could be written over them before.
//   - StatusInputUnavailable -- the declaration exists but the *caller* supplied no input to read
//     it from. This is the /review 404: ReviewStepForDocument called DeclaredFields(stepMachine,
//     nil), so Parent resolved to "", so the query matched no rows, so the screen rendered a 404
//     that looked entirely legitimate -- because "a Document with no steps" is a real state.
//     Nothing logged, nothing panicked, no test failed, for a day, in production.
//
// Both incidents were found by hand-written throwaway probes. This type is that probe made
// permanent, which is the whole argument for building it: the work is not a new capability, it is
// stopping the discard of one.
type Resolution struct {
	// Name is what was asked, stable across Machines so two Workspaces' answers line up:
	// "step.decision", "document.document_status". Never a Field id -- that is Value.
	Name string
	// Value is the resolved answer, and is deliberately left empty rather than defaulted. An
	// invented "usual name" here would reproduce the silent-mismatch class this whole type exists
	// to expose (see action.EngineFields' own "empty means undeclared, never assume" contract).
	Value string
	// From names the declaration the answer was read from -- "transitions[action=decide].field",
	// "sequencing.order_field". It is the half that makes an answer checkable rather than trusted,
	// and the half that distinguishes InputUnavailable from Undeclared: a reader can see that the
	// declaration named here exists while the value does not.
	From   string
	Status ResolutionStatus
}

// ResolutionStatus is why Resolution.Value is what it is. Closed set, and every member is a
// distinction one of the four cases in Resolution's own comment turns on.
type ResolutionStatus string

const (
	StatusResolved         ResolutionStatus = "resolved"
	StatusUndeclared       ResolutionStatus = "undeclared"
	StatusNotApplicable    ResolutionStatus = "not applicable"
	StatusInputUnavailable ResolutionStatus = "input unavailable"
)

// IsDefect reports whether this resolution is one a reader should act on.
//
// Resolved and NotApplicable are both correct outcomes -- the second is the larger population by
// far, and reporting it as a problem is what made the first attempt at triaging these numbers
// useless. Undeclared and InputUnavailable are the two that produced real, shipped defects.
func (r Resolution) IsDefect() bool {
	return r.Status == StatusUndeclared || r.Status == StatusInputUnavailable
}

// Derivations are the questions a workflow engine asks of the Machines it is cast over. They are
// named here, in the Domain Plane, rather than in internal/action, for one reason: both the engine's
// own explainer *and* WorkflowEngineSpec.Answers below have to name the same set, and two lists
// naming the same things is the drift this repo has already paid for three times (checkDocs mirrors,
// closed-registry members).
const (
	DerivationDecision           = "decision"
	DerivationOpenValue          = "open_value"
	DerivationOrder              = "order"
	DerivationActor              = "actor"
	DerivationActorType          = "actor_type"
	DerivationActorGroup         = "actor_group"
	DerivationParent             = "parent"
	DerivationDocumentStatus     = "document_status"
	DerivationSignaturePlacement = "signature_placement"
	DerivationSignatureStore     = "signature_store"
	DerivationCompositeSource    = "composite_source"
	DerivationFlowTemplate       = "flow_template"
	DerivationFlowTemplateStep   = "flow_template_step"
)
