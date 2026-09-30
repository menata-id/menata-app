package registry

import (
	"menata.app/internal/domain"
)

// This file is the workflow-engine half of the Service seam's own pattern (§5.2 step 2, 2026-09-30):
// one closed catalogue of engines, each carrying the cast it takes and the derivations each role owes.
//
// **Moved from internal/domain rather than newly written.** It had always been a registry -- a closed map
// whose validator reads it instead of repeating its members in a switch, which is the shape
// TestClosedRegistryMembersAreAcceptedByTheLoader names as the one that needs no drift gate. What it was
// missing is a home: sitting in `domain` made the *declaration* plane own a *dispatch* seam, so
// `internal/action` (which reads Answers to explain an inference) and `internal/metadata` (which validates
// a binding against it) were both reaching into domain for something neither declares.
//
// `domain.WorkflowRole*` and `domain.WorkflowEngineDocumentApproval` deliberately did **not** come along.
// Those are names an Application writes in its own YAML and a Machine is stamped with -- vocabulary, not
// dispatch. Moving them would have made every metadata file's role names belong to the registry, which
// inverts which side declares what.

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
	// Datasets maps a role to the Dataset ids the engine's own composed screens select through, and
	// which the Machine cast in that role must therefore declare. Validated at load
	// (internal/metadata.validateWorkflowDatasets).
	//
	// **This field exists because its absence shipped a 500 to two Workspaces.** Tahap A (2026-09-29)
	// moved the Document-to-Step correlation out of three hand-written index loops and into a declared
	// Relation, `ds_documents_with_steps` -- adding it to the template library and to `default`'s own
	// copy. An install *copies*, so a library change never reaches a Workspace that installed earlier:
	// `hanomerch` and `dokter-kecil` both cast the `step` role, neither declared the Dataset, and
	// `composition.selectRecords` answers a missing Dataset with an error. Every approval screen in
	// both Workspaces returned 500, unconditionally, for a day.
	//
	// Nothing could have caught it. The id is named from Go (which is why `internal/installer` refuses
	// to rename a Dataset id rather than renaming it the way it renames a Machine id), and no
	// declaration said the engine needed it -- so `TestNavigationRoutesAreRegistered` saw a registered
	// handler, the loader saw valid YAML, and the whole suite stayed green. **A capability an engine
	// requires and no metadata declares is reachable only through the failure it causes.**
	//
	// A load error rather than a per-request one, for the same reason a missing Required role is: an
	// engine that cannot select its own records cannot run, and the Workspace should refuse to start
	// rather than serve a screen that throws.
	Datasets map[string][]string
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

// DatasetsFor returns the Dataset ids the Machine cast in role must declare, or nil for a role that
// needs none -- which is most of them.
func (s WorkflowEngineSpec) DatasetsFor(role string) []string { return s.Datasets[role] }

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
// takes -- the same static-seam discipline KnownActions and registry.Services already establish (007 §14),
// and for the same reason: an engine name the runtime cannot realize must fail at load rather than
// leaving a screen that quietly offers nothing.
//
// Every engine here is implemented in Go and stays there (002: physical strategies remain
// runtime-owned) -- what metadata decides is which Machines it runs over.
var KnownWorkflowEngines = map[string]WorkflowEngineSpec{
	domain.WorkflowEngineDocumentApproval: {
		Required: []string{domain.WorkflowRoleDocument, domain.WorkflowRoleStep},
		Optional: []string{domain.WorkflowRoleSignature, domain.WorkflowRoleFlowTemplate, domain.WorkflowRoleFlowTemplateStep},
		// Read off the real Machines rather than composed from this engine's wish list: each entry is
		// a derivation that Machine's role actually resolved in both installed Workspaces, or -- for
		// the optional roles -- the block that role's own accessor reads.
		//
		// `step` owes parent and composite_source as well as its own five, because it is where the
		// relation to the Document and the compositing Event are declared; `document` owes only its
		// status Field, which is why four of its five probes are correctly empty.
		Answers: map[string][]RoleAnswer{
			domain.WorkflowRoleStep: {
				{Derivation: domain.DerivationDecision},
				{Derivation: domain.DerivationOpenValue},
				{Derivation: domain.DerivationOrder},
				{Derivation: domain.DerivationActor},
				{Derivation: domain.DerivationActorType},
				{Derivation: domain.DerivationActorGroup},
				{Derivation: domain.DerivationParent},
				{Derivation: domain.DerivationStatusTargets},
				// Optional: an Action whose whole effect is the status move its own transitions already
				// declare writes no companion Fields, which is a complete Action rather than a broken one.
				// Two of the installed Machines are exactly that.
				{Derivation: domain.DerivationActionWrites, Optional: true},
				// Both features rather than requirements, proven by deleting each from the real
				// approval_step.yaml and watching it still load: an approval Application may capture no
				// signatures, and may composite no PDF.
				{Derivation: domain.DerivationSignaturePlacement, Optional: true},
				{Derivation: domain.DerivationCompositeSource, Optional: true},
			},
			domain.WorkflowRoleDocument: {{Derivation: domain.DerivationDocumentStatus}},
			// Each optional role's own block is optional *within* the role too: a Workspace that casts
			// the role at all is installing the feature, but the cast and the block are two separate
			// declarations and a Machine can be cast before it is finished.
			domain.WorkflowRoleSignature:        {{Derivation: domain.DerivationSignatureStore}},
			domain.WorkflowRoleFlowTemplate:     {{Derivation: domain.DerivationFlowTemplate}},
			domain.WorkflowRoleFlowTemplateStep: {{Derivation: domain.DerivationFlowTemplateStep}},
		},
		// One entry, on the `document` role: the Relation that attaches a Document's own Steps, which
		// all three approval screens select through (composition.buildInbox, PendingApprovalCount,
		// buildAssigned). It sits on `document` because the Dataset is declared by the Machine it
		// selects *from*, and `via` names the reference Field on the step Machine pointing back.
		Datasets: map[string][]string{
			domain.WorkflowRoleDocument: {domain.DatasetDocumentsWithSteps},
		},
	},
}
