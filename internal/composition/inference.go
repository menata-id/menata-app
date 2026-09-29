package composition

import (
	"sort"
	"strings"

	"menata.app/internal/action"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// Inference groups every derivation a Workspace's installed engines make, for the diagnostics screen
// (001 Principle #6's second clause; ROADMAP.md's 2026-09-29 kajian, priority 1).
//
// **Pure, and it reads no records at all** -- no ctx, no Loader, no Store. That is worth stating rather
// than leaving as an accident of the implementation: 007 §28's invariants and §33's admission evidence
// both ask what physical work a new capability generates, and the answer here is none. It adds no
// logical dependency, generates no DAG node, and issues no query, so the two GET-sweep invariants
// (`queries == reads`, `repeated == 0`) hold for this route without it having to be careful.
//
// Engines are sorted rather than ranged over as a map: domain.KnownWorkflowEngines is a map, and 007
// §4.6 makes deterministic construction a MUST. action.ExplainCast already holds that property for the
// rows within an engine (TestExplainCast_isDeterministic); this is the same obligation one level up.
func Inference(ws domain.Workspace) rendering.InferenceView {
	engines := make([]string, 0, len(domain.KnownWorkflowEngines))
	for engine := range domain.KnownWorkflowEngines {
		engines = append(engines, engine)
	}
	sort.Strings(engines)

	var view rendering.InferenceView
	for _, engine := range engines {
		spec := domain.KnownWorkflowEngines[engine]
		// An engine no Application here binds has nothing to explain. Skipped rather than rendered
		// empty: a table of "not applicable" for a feature the Workspace never installed is noise of
		// exactly the kind Resolution.Status exists to suppress.
		if len(ws.MachinesInWorkflowRole(engine, domain.WorkflowRoleStep)) == 0 {
			continue
		}

		block := rendering.InferenceEngine{Engine: engine}
		// Index into block.Roles rather than a *rendering.InferenceRole. A pointer taken as
		// &block.Roles[len-1] is invalidated the moment the next append reallocates the backing array,
		// so later rows would be written into the old array and silently vanish -- on a page whose whole
		// job is showing what is missing, which would have been a memorable way to fail.
		at := map[string]int{}
		for _, r := range action.ExplainCast(ws, engine, "") {
			role, derivation := splitResolutionName(r.Name)
			i, seen := at[role]
			if !seen {
				block.Roles = append(block.Roles, rendering.InferenceRole{
					Role:      role,
					MachineID: machineIDInRole(ws, engine, role),
					Required:  spec.IsRequired(role),
				})
				i = len(block.Roles) - 1
				at[role] = i
			}
			block.Roles[i].Rows = append(block.Roles[i].Rows, rendering.InferenceRow{
				Derivation: derivation,
				Value:      r.Value,
				From:       r.From,
				Status:     string(r.Status),
				Tone:       toneFor(r.Status),
				IsDefect:   r.IsDefect(),
			})
			if r.IsDefect() {
				block.Roles[i].Defects++
				block.Defects++
				view.Defects++
			}
		}
		view.Engines = append(view.Engines, block)
	}
	return view
}

// toneFor maps a resolution status to the shared pill's own closed tone set. It lives here, in
// Composition, for the same reason reviewStatusTone lives beside the Review screen: the pill owns the
// shape and each caller owns its vocabulary (007 §12.3's boundedness rule).
//
// `not applicable` is muted deliberately. It is 52 of 65 rows and every one of them is correct, so
// rendering it at the same weight as the two statuses that mean something would bury the signal in its
// own correctness -- which is what made the first two attempts at triaging these numbers useless
// (domain.WorkflowEngineSpec.Answers' own comment).
func toneFor(s domain.ResolutionStatus) rendering.PillTone {
	switch s {
	case domain.StatusResolved:
		return rendering.PillGood
	case domain.StatusUndeclared:
		return rendering.PillBad
	case domain.StatusInputUnavailable:
		return rendering.PillWarn
	case domain.StatusNotApplicable:
		return rendering.PillMuted
	}
	return rendering.PillNeutral
}

// splitResolutionName splits "step.decision" into its role and derivation halves. Resolution.Name is
// built by action.explainOne as role + "." + derivation; splitting on the *first* dot is what keeps a
// derivation containing one (none does today) from silently losing part of its name.
func splitResolutionName(name string) (role, derivation string) {
	if role, derivation, found := strings.Cut(name, "."); found {
		return role, derivation
	}
	return name, ""
}

// machineIDInRole is the Machine this Workspace casts in role, or "" when it casts none -- which the
// screen renders as "uncast" rather than blank, since an optional role nobody fills is a real answer
// (domain.Workspace.MachineInWorkflowRole's own doc comment: "a nil is a real answer").
func machineIDInRole(ws domain.Workspace, engine, role string) string {
	if m := ws.MachineInWorkflowRole(engine, role, ""); m != nil {
		return m.ID
	}
	return ""
}
