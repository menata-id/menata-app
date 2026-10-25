package conformance

import (
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/registry"
)

// The gap register (aiassist.KnownGaps, plan 2026-10-25 A2) is the vocabulary the assistant names a missing
// capability by, and a status the log's readers trust: "built" has to mean the capability exists AND the
// assistant can write it -- the same sentence registry.Capabilities states with AIWritable. So a gap cannot be
// declared built while its slot is missing, and an open or planned gap cannot name a capability as if it were
// satisfied. aiassist.ValidateKnownGaps holds the rules; this test hands it the real catalogue, which that
// package may not import. Its mutations are in the plan's slice A record: set a seeded gap built with no
// capability, with a capability that is not AIWritable, and with an unknown id.
func TestGapRegisterAgreesWithTheCapabilityCatalogue(t *testing.T) {
	byID := map[string]registry.Capability{}
	for _, c := range registry.Capabilities {
		byID[c.ID] = c
	}
	problems := aiassist.ValidateKnownGaps(aiassist.KnownGaps, func(id string) (bool, bool) {
		c, ok := byID[id]
		return ok, ok && c.AIWritable
	})
	for _, p := range problems {
		t.Error(p)
	}
}
