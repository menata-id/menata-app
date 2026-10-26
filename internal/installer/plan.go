package installer

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"menata.app/internal/domain"
)

// Plan is what installing one template into one Workspace would do, decided before anything is
// written: which ids have to change, which shared references have to be added, and -- if the answer
// is "this cannot be installed here" -- why.
//
// It exists as a value rather than as a side effect of Install because two callers need the same
// answer: the screen that offers the install has to show what it will do, and the write has to do
// exactly that. A screen computing its own preview would be a second implementation of the rule, free
// to disagree with the one that runs.
type Plan struct {
	Template Template
	// Renames maps a template id -- a Machine's, or the Application's own -- to the id it will take in
	// this Workspace. **Empty is the normal case**, and then every copied file is byte-identical to
	// its template, which is what keeps "install, then diverge" readable as a diff.
	Renames map[string]string
	// AddShared are runtime-level Machines this template needs and the target Workspace does not yet
	// reference (Template.RequiresShared minus what it has). Added as references, never copied.
	AddShared []string
	// Refusals are collisions this package will not resolve. Non-empty means Install refuses, and each
	// entry says which id and why -- see Plan's own "what may be renamed" reasoning below.
	Refusals []string
	// Companions are the plans for the template's companions (Template.Companions), made against this Workspace as it will be
	// once the template itself is in. A companion's refusals are also in Refusals, so one OK() answers for the whole install.
	Companions []Plan
	// inherited are the renames of the template this plan is a companion of: a companion's Machines relate to its template's,
	// so a Machine id renamed there must be renamed in the companion's files too. Never set on the template's own plan.
	inherited map[string]string
}

// fileRenames is every rename a copied file is rewritten with: this plan's own, and for a companion those it inherits.
func (p Plan) fileRenames() map[string]string {
	if len(p.inherited) == 0 {
		return p.Renames
	}
	all := map[string]string{}
	for from, to := range p.inherited {
		all[from] = to
	}
	for from, to := range p.Renames {
		all[from] = to
	}
	return all
}

// OK reports whether this plan can be installed.
func (p Plan) OK() bool { return len(p.Refusals) == 0 }

// ApplicationID is the id the installed Application will carry -- the template's own, or its rename.
func (p Plan) ApplicationID() string {
	if renamed, ok := p.Renames[p.Template.Application.ID]; ok {
		return renamed
	}
	return p.Template.Application.ID
}

// RenamedMachineID is the id machineID will carry in this Workspace.
func (p Plan) RenamedMachineID(machineID string) string {
	if renamed, ok := p.Renames[machineID]; ok {
		return renamed
	}
	return machineID
}

// PlanInstall decides how tmpl would install into ws.
//
// **What may be renamed, and why only that.** Machine ids and an Application's own id, because
// nothing in Go means anything by either one any more: development-history.md's Stage A made the approval engine
// read a declared binding instead of matching `app_document_approval`, and the slice after it made
// every handler, composition and Page resolve a Machine by the role its Application casts it in. A
// renamed copy of Document Approval therefore engages exactly as the original does, which was
// provably false two days earlier.
//
// **What is refused instead of renamed**, because Go still names it:
//
//   - a navigation id -- `routeByID("nav_approval_inbox")` in a .templ, and three conformance gates
//     that hold every Page to reading its own route/label/heading through one;
//   - a navigation route -- domain.Workspace.ApplicationForRoute matches the declared string, so two
//     Applications sharing a route make "which Application is this request in" answer whichever
//     loaded first;
//   - a Dataset id -- internal/composition names a handful of them as Go constants (ds_documents_with_steps
//     and friends), so a renamed Dataset is a screen that silently counts nothing.
//
// Renaming any of those would recreate precisely the coupling those two slices removed, which is why
// this refuses and says so rather than resolving it badly. The route refusal has a second effect worth
// knowing: it makes two approval Applications in one Workspace impossible, and that is the state which
// would otherwise expose the ambiguity domain.MachineInWorkflowRole's own doc comment names (the
// submit wizard's routes belong to no Application, so nothing could say which one they meant).
func PlanInstall(tmpl Template, ws domain.Workspace) Plan {
	p := planOne(tmpl, ws, nil)
	after := ws
	for _, c := range tmpl.Companions {
		after = withInstalled(after, p)
		cp := planOne(c, after, p.Renames)
		for _, r := range cp.Refusals {
			p.Refusals = append(p.Refusals, c.Application.Name+": "+r)
		}
		p.Companions = append(p.Companions, cp)
		p = withShared(p, cp)
	}
	return p
}

// withShared notes in p the shared references cp adds that p does not already add, so a bundle shows its whole change once.
func withShared(p, cp Plan) Plan {
	for _, id := range cp.AddShared {
		if !slices.Contains(p.AddShared, id) {
			p.AddShared = append(p.AddShared, id)
		}
	}
	return p
}

// withInstalled is ws as it will be once p is installed -- the Machines and Application p brings, under the ids it gives them --
// so the next plan of a bundle sees them taken. Only what PlanInstall reads is carried over.
func withInstalled(ws domain.Workspace, p Plan) domain.Workspace {
	out := ws
	out.MachineIDs = slices.Clone(ws.MachineIDs)
	out.Machines = slices.Clone(ws.Machines)
	for _, tm := range p.Template.Machines {
		m := *tm.Machine
		m.ID = p.RenamedMachineID(m.ID)
		out.MachineIDs = append(out.MachineIDs, m.ID)
		out.Machines = append(out.Machines, &m)
	}
	app := *p.Template.Application
	app.ID = p.ApplicationID()
	out.Applications = append(slices.Clone(ws.Applications), app)
	return out
}

func planOne(tmpl Template, ws domain.Workspace, inherited map[string]string) Plan {
	p := Plan{Template: tmpl, Renames: map[string]string{}, inherited: inherited}

	taken := map[string]bool{}
	for _, id := range ws.MachineIDs {
		taken[id] = true
	}
	// The whole set, including Machines no Application claims. The assistant's own prompt had this
	// asymmetry until 2026-09-28 -- it was told the claimed ids only -- and validation knew more than
	// it did. Repeating the mistake here would produce a rename onto mch_user.
	for _, m := range ws.Machines {
		taken[m.ID] = true
	}

	suffix := lastSegment(tmpl.Application.ID)
	for _, tm := range tmpl.Machines {
		if !taken[tm.Machine.ID] {
			continue
		}
		renamed := freeID(tm.Machine.ID, suffix, taken)
		p.Renames[tm.Machine.ID] = renamed
		taken[renamed] = true
	}

	takenApps := map[string]bool{}
	for _, app := range ws.Applications {
		takenApps[app.ID] = true
	}
	if takenApps[tmpl.Application.ID] {
		renamed := freeID(tmpl.Application.ID, suffix, takenApps)
		p.Renames[tmpl.Application.ID] = renamed
	}

	for _, id := range tmpl.RequiresShared {
		if !taken[id] {
			p.AddShared = append(p.AddShared, id)
		}
	}

	p.Refusals = refusals(tmpl, ws)
	return p
}

// refusals collects every collision this package declines to resolve, all of them, rather than
// stopping at the first: an admin deciding whether a template fits their Workspace deserves the whole
// list in one read.
//
// The wording is for that admin, not for a developer: it says which id is taken and that this kind of id
// cannot be changed on the way in. *Why* it cannot is a Go fact (see PlanInstall's own doc comment
// above), and putting package names on a screen would be telling the wrong person the wrong half.
func refusals(tmpl Template, ws domain.Workspace) []string {
	navIDs, routes := declaredNavigation(ws)
	datasets := declaredDatasets(ws)

	var out []string
	for _, item := range tmpl.Application.AllNavigation {
		if navIDs[item.ID] {
			out = append(out, fmt.Sprintf("the screen id %q is already used in this workspace, and screen ids cannot be changed on install", item.ID))
		}
		if routes[item.Route] {
			out = append(out, fmt.Sprintf("the address %q is already used by another application here, and two applications cannot share one", item.Route))
		}
	}
	for _, tm := range tmpl.Machines {
		for _, ds := range tm.Machine.Datasets {
			if datasets[ds.ID] {
				out = append(out, fmt.Sprintf("the saved figure %q is already defined in this workspace, and its id cannot be changed on install", ds.ID))
			}
		}
	}
	sort.Strings(out)
	return out
}

// declaredNavigation is every navigation id and route already answering in this Workspace: each
// Application's own unfiltered list plus the runtime's own screens, which an Application may not
// redeclare either (metadata.validateNavigationIDsAreUnique holds the id half at load).
func declaredNavigation(ws domain.Workspace) (ids, routes map[string]bool) {
	ids, routes = map[string]bool{}, map[string]bool{}
	note := func(items []domain.NavigationItem) {
		for _, item := range items {
			ids[item.ID] = true
			routes[item.Route] = true
		}
	}
	note(ws.Navigation)
	note(domain.RuntimeScreens)
	for _, app := range ws.Applications {
		note(app.AllNavigation)
	}
	return ids, routes
}

func declaredDatasets(ws domain.Workspace) map[string]bool {
	out := map[string]bool{}
	for _, m := range ws.Machines {
		for _, ds := range m.Datasets {
			out[ds.ID] = true
		}
	}
	return out
}

// freeID is the rename itself: try the template's own id with the Application's last name segment
// appended (mch_document + app_document_approval -> mch_document_approval, which is what a person
// would have called it), then numeric suffixes.
//
// Deterministic on purpose -- the same template into the same Workspace always produces the same
// names, so a plan shown on a screen is the plan that runs, and a test can assert the result rather
// than a shape.
func freeID(id, suffix string, taken map[string]bool) string {
	if candidate := id + "_" + suffix; suffix != "" && !taken[candidate] {
		return candidate
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s_%d", id, n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// lastSegment is app_document_approval -> "approval": the word that distinguishes this Application
// from the plain noun its Machine is named after, which is why it reads as a name rather than as a
// counter.
func lastSegment(applicationID string) string {
	parts := strings.Split(strings.TrimPrefix(applicationID, "app_"), "_")
	return parts[len(parts)-1]
}
