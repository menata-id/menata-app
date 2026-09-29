package metadata

import (
	"fmt"

	"menata.app/internal/domain"
)

// Explain reports what Phase 4 normalization decided for a Workspace's Machines (001 Principle #6's
// second clause, 004 §Inference, 005 Phase 4's own "the normalized result must be inspectable enough
// to explain important runtime decisions").
//
// **It takes the Machine set, not one Machine**, and that is the shape finding rather than a
// convenience: a child collection is a property of a *pair* -- "which Machines point at this one" --
// so the per-subject model action.ExplainCast uses for the engine's roles does not transfer. The
// earlier plan recorded this as a doubt; measuring it settled it.
//
// Three derivations, measured across the three installed Workspaces on 2026-09-29: 21 person
// targets, 31 child collections over 13 Machines (18 have none), and 18 Machines falling back to the
// default table View (13 declare their own). About 70 answers -- more than the engine cast's 65.
//
// **Board columns are not here on purpose**, and their absence is a correction rather than an
// omission: see domain.DerivationChildCollection's neighbours.
func Explain(machines []*domain.Machine) []domain.Resolution {
	var out []domain.Resolution
	for _, m := range machines {
		out = append(out, explainPersonTargets(m)...)
		out = append(out, explainChildCollections(machines, m)...)
		out = append(out, explainDefaultView(m))
	}
	return out
}

// explainPersonTargets covers the one inference that is genuinely an *inference* rather than a
// derivation: nothing in the YAML says a person Field points at mch_user, and 001 #6 is why an author
// never writes it (metadata.Normalize's own comment).
//
// An empty target is StatusUndeclared and a real defect -- it means normalization did not run, which
// is not hypothetical: a Machine built in Go with a correct person Field failed Validate with a
// message complaining about its *type*, and that failure is what produced Normalize in the first
// place. The installed corpus reports 21 resolved and 0 empty.
func explainPersonTargets(m *domain.Machine) []domain.Resolution {
	var out []domain.Resolution
	for _, f := range m.Fields {
		if f.Type != domain.FieldTypePerson {
			continue
		}
		r := domain.Resolution{
			Name:  fmt.Sprintf("%s: %s.%s", domain.DerivationPersonTarget, m.ID, f.ID),
			Value: f.RelatedMachine,
			From:  domain.StepExpandConveniences + " -- a person Field always references " + domain.UserMachineID,
		}
		if r.Value == "" {
			r.Status = domain.StatusUndeclared
			r.From = "normalization did not run for this Machine (metadata.Normalize binds person to " + domain.UserMachineID + ")"
		} else {
			r.Status = domain.StatusResolved
		}
		out = append(out, r)
	}
	return out
}

// explainChildCollections answers "which Machines point at this one", one row per pair.
//
// **Per pair rather than per Machine**, because the per-Machine summary would hide the half that
// matters: *which Field* creates each collection, and whether that Field's own target was inferred.
// All eight of mch_user's child collections in the `default` Workspace come from person Fields --
// measured -- which means they exist only because Normalize ran. If it stops running, mch_user goes
// from eight child collections to none, and nothing else says so. One inference feeding another is
// exactly what an inspection surface is for.
//
// A Machine nothing references gets one NotApplicable row rather than silence: 18 of the 31 installed
// Machines are in that state, and it is a correct, common answer -- omitting them would make the
// absence indistinguishable from a derivation that failed.
func explainChildCollections(machines []*domain.Machine, m *domain.Machine) []domain.Resolution {
	children := domain.FindChildCollections(machines, m.ID)
	if len(children) == 0 {
		return []domain.Resolution{{
			Name:   fmt.Sprintf("%s: %s", domain.DerivationChildCollection, m.ID),
			From:   domain.StepResolveReferences + " -- no Machine declares a reference Field pointing here",
			Status: domain.StatusNotApplicable,
		}}
	}

	out := make([]domain.Resolution, 0, len(children))
	for _, c := range children {
		from := domain.StepResolveReferences + " -- " + c.Machine.ID + "." + c.Field.ID + " references this Machine"
		if c.Field.Type == domain.FieldTypePerson {
			// Naming the chain rather than leaving it to be rediscovered: this row exists because a
			// *different* derivation resolved first.
			from += ", and that Field's own target is itself inferred (" + domain.DerivationPersonTarget + ")"
		}
		out = append(out, domain.Resolution{
			Name:   fmt.Sprintf("%s: %s <- %s.%s", domain.DerivationChildCollection, m.ID, c.Machine.ID, c.Field.ID),
			Value:  c.Machine.ID,
			From:   from,
			Status: domain.StatusResolved,
		})
	}
	return out
}

// explainDefaultView is the safe default (005 Phase 4's own "apply safe defaults"): a Machine that
// declares no `views:` renders as a plain table.
//
// A Machine that declares its own is NotApplicable -- nothing was defaulted, so there is nothing to
// explain, and saying "resolved: table" for a Machine whose first View is a board would be false.
// 13 of 31 installed Machines are in that state.
func explainDefaultView(m *domain.Machine) domain.Resolution {
	r := domain.Resolution{Name: fmt.Sprintf("%s: %s", domain.DerivationDefaultView, m.ID)}
	if len(m.Views) > 0 {
		r.Status = domain.StatusNotApplicable
		r.From = domain.StepApplyDefaults + " -- not applied: this Machine declares its own views:"
		return r
	}
	r.Status = domain.StatusResolved
	r.Value = string(m.DefaultView().EffectiveType())
	r.From = domain.StepApplyDefaults + " -- this Machine declares no views:"
	return r
}
