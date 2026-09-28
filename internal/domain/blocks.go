package domain

// DeclaredBlocks names every capability block this Machine declares -- "what can this Machine
// actually do", answered from the Machine itself rather than by reading its YAML back.
//
// It exists for a failure mode that has now been found by hand twice, and never by a gate: a **test
// fixture that declares less than the Machine it mirrors**. Such a fixture does not fail. It passes,
// against a Machine looser than the one that runs, and the test goes on reading as though it covered
// the rule. `internal/composition`'s stepMachineForTest carries the first occurrence in its own
// comment -- its `decide` Permission was missing for a whole phase and "nothing noticed", because the
// screen under test ANDed an equivalent check in Go. Stage D (2026-09-28) found four more in one
// afternoon, all of them mirrors that had simply not been updated when a new block landed.
//
// The comparison a gate can make with this is **presence, not equality**: a fixture is deliberately
// smaller, and deliberately renamed (mch_surat, fld_putusan, fld_gambar_ttd) in the tests that prove
// this runtime does not depend on the template library's own names. What must not differ is which
// blocks exist at all.
//
// Names are the YAML keys rather than Go field names, so a failure message says what an author would
// go and look for.
func (m *Machine) DeclaredBlocks() []string {
	if m == nil {
		return nil
	}
	var out []string
	add := func(declared bool, name string) {
		if declared {
			out = append(out, name)
		}
	}
	add(len(m.Fields) > 0, "fields")
	add(len(m.Constraints) > 0, "constraints")
	add(len(m.Events) > 0, "events")
	add(len(m.Permissions) > 0, "permissions")
	add(len(m.Transitions) > 0, "transitions")
	add(len(m.ActionEffects) > 0, "actions")
	add(len(m.Datasets) > 0, "datasets")
	add(len(m.MemberRemovalBlocks) > 0, "blocks_member_removal")
	add(m.Sequencing != nil, "sequencing")
	add(m.SignaturePlacement != nil, "signature_placement")
	add(m.SignatureStore != nil, "signature_store")
	add(m.SLAField != "", "sla_field")
	add(len(m.CardFields) > 0, "card_fields")
	add(len(m.Views) > 0, "views")
	add(m.AppendOnly, "append_only")
	return out
}

// ruleBlocks are the blocks that make a Machine *behave* -- the ones a Permission check, a transition
// walk, an Action effect, an Event dispatch or a sequencing lock actually reads at run time.
//
// The split exists because a fixture-drift gate over *every* block would be wrong in a way that makes
// tests worse, not better. A reduction like composition's docMachineForTest exists to carry one
// Machine's status model into a view-model builder; forcing it to also declare mch_document's Views,
// Datasets, card_fields and full Field list would bury what the test is about under a copy of the
// manifest. Structure and presentation are where a reduction is legitimate; rules are where it is a
// silent false pass.
var ruleBlocks = map[string]bool{
	"constraints": true, "events": true, "permissions": true, "transitions": true,
	"actions": true, "sequencing": true, "signature_placement": true, "signature_store": true,
	"blocks_member_removal": true, "append_only": true,
}

// DeclaredRuleBlocks is DeclaredBlocks narrowed to those -- what a fixture mirroring this Machine
// must not be missing.
func (m *Machine) DeclaredRuleBlocks() []string {
	var out []string
	for _, b := range m.DeclaredBlocks() {
		if ruleBlocks[b] {
			out = append(out, b)
		}
	}
	return out
}

// MissingBlocksFrom returns the rule blocks `real` declares and m does not -- the whole of what a
// fixture-drift gate asserts, so the two packages holding mirror fixtures share one definition
// instead of writing the comparison twice.
//
// The direction is deliberate and only checked one way: a fixture declaring *more* than the real
// Machine is a different question (usually a test exploring a shape the manifest has not adopted),
// and failing it would discourage exactly the kind of test that proves a capability works before it
// is declared anywhere.
func (m *Machine) MissingBlocksFrom(real *Machine) []string {
	have := map[string]bool{}
	for _, b := range m.DeclaredRuleBlocks() {
		have[b] = true
	}
	var missing []string
	for _, b := range real.DeclaredRuleBlocks() {
		if !have[b] {
			missing = append(missing, b)
		}
	}
	return missing
}
