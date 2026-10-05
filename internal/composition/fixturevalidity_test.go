package composition

import (
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// TestMachineFixturesPassProductionValidation runs this package's Machine fixtures through the same
// validator the loader runs, so a fixture cannot describe a Machine the runtime would refuse to load.
//
// **A different question from "does this fixture mirror the real Machine".** That one is about intent --
// a narrow unit fixture legitimately declares fewer blocks than the Machine it borrows an id from -- and
// a static gate for it was built, measured at 40 false findings, and rejected (see
// internal/composition/approval_test.go's own note). This asks whether the fixture is *coherent*:
// metadata.Validate tolerates minimal and rejects incoherent, so there is no judgement in it. A
// Permission gating on a Field the Machine lacks, an actions: block writing one, a signature_placement:
// over absent Fields -- all describe Machines that cannot exist.
//
// Normalize first, then Validate: 005-runtime-lifecycle.md's Phase 4 inferences before Phase 3's checks,
// which is the order metadata.Parse itself ends on. Asking a fixture to hand-write what the runtime
// infers would invert 001 Principle #6.
//
// It found real incoherence in five packages on its first run (2026-09-29). A fixture that is
// *deliberately* invalid belongs excluded by name with its reason, never by loosening this.
func TestMachineFixturesPassProductionValidation(t *testing.T) {
	fixtures := map[string]*domain.Machine{
		"stepMachineForTest":    stepMachineForTest(),
		"docMachineForTest":     docMachineForTest(),
		"taskMachineForTest":    taskMachineForTest(),
		"projectMachineForTest": projectMachineForTest(),
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixtures listed -- this test would pass while checking nothing")
	}
	for name, m := range fixtures {
		if err := metadata.Validate(metadata.Normalize(m)); err != nil {
			t.Errorf("%s would not load:\n%v\n"+
				"  a fixture the real validator rejects is not a smaller Machine, it is an impossible one --\n"+
				"  every test using it is passing against something the runtime would refuse", name, err)
		}
	}
}
