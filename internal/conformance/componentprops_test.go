package conformance

import (
	"slices"
	"testing"

	"menata.app/internal/ir"
	"menata.app/internal/registry"
)

// TestPermittedPropsAreTheContractsInputs ties UI IR's per-Component key list to the registry contract (independent
// review of SegmentedControl, 2026-10-14): `allowedProps` widened to accept `class` passed every test, because the
// registry validator refused it first. Two lists that decide one thing drift unless something holds them together.
func TestPermittedPropsAreTheContractsInputs(t *testing.T) {
	for typ, c := range registry.Components {
		inputs := map[string]bool{}
		for _, in := range c.Contract.Inputs {
			inputs[in.Name] = true
		}
		permitted := ir.PermittedProps(ir.NodeComponent, string(typ))
		if permitted == nil {
			t.Errorf("%s is registered and ir.allowedProps has no entry for it -- a page cannot declare it", typ)
		}
		for _, k := range permitted {
			if inputs[k] {
				continue
			}
			t.Errorf("ir.allowedProps lets a %s node carry %q, which its contract does not declare -- add it to the contract or remove it", typ, k)
		}
		for in := range inputs {
			if !slices.Contains(permitted, in) {
				t.Errorf("%s's contract declares input %q and ir.allowedProps does not permit it -- a declared page cannot pass it", typ, in)
			}
		}
	}
}
