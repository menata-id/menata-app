package web

import (
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// TestMachineFixturesPassProductionValidation runs this package's Machine fixtures through the same
// validator the loader runs, so a fixture cannot describe a Machine the runtime would refuse to load.
//
// **This is a different question from TestFixturesMirrorTheRealMachines**, and the difference is why it
// can be a sweep where that one is a named list. That test asks whether a fixture declares the blocks
// the real Machine declares -- a question about *intent*, since a narrow unit fixture legitimately
// declares fewer. This asks whether the fixture is **coherent at all**: metadata.Validate tolerates
// minimal (one Field, no Permissions, fine) and rejects incoherent (a Permission gating on a Field that
// names no identity, an actions: block writing a Field the Machine does not have). There is no judgement
// in it: if production would reject the data, the fixture describes something impossible.
//
// It found two on its first run, which is why it exists. approvalStepTestMachine declared an `actions:`
// effect writing fld_decided_by_name while its Fields list did not contain it, and both it and
// signatureTestMachine had person Fields with no RelatedMachine -- a state only a hand-built Machine can
// reach, since Parse fills that in for YAML.
//
// A fixture that is *deliberately* invalid (a test feeding Validate a bad Machine on purpose) belongs
// excluded by name with its reason, never by loosening this.
func TestMachineFixturesPassProductionValidation(t *testing.T) {
	ids := templateLibraryIDs()
	fixtures := map[string]*domain.Machine{
		"approvalStepTestMachine": approvalStepTestMachine(ids),
		"documentTestMachine":     documentTestMachine(ids),
		"signatureTestMachine":    signatureTestMachine(ids),
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixtures listed -- this test would pass while checking nothing")
	}
	for name, m := range fixtures {
		// Normalize first, then Validate -- 005's Phase 4 then Phase 3's own checks, which is the order
		// the loader itself runs (metadata.Parse ends by normalising). Validating un-normalised metadata
		// is what made a correct `person` Field look like a type error, and asking every fixture to
		// hand-write what the runtime infers would invert 001 Principle #6.
		if err := metadata.Validate(metadata.Normalize(m)); err != nil {
			t.Errorf("%s would not load:\n%v\n"+
				"  a fixture the real validator rejects is not a smaller Machine, it is an impossible one --\n"+
				"  every test using it is passing against something the runtime would refuse", name, err)
		}
	}
}
