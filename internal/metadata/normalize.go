package metadata

import "menata.app/internal/domain"

// Normalize applies the inferences a *domain.Machine carries regardless of how it was built.
//
// It exists because 005-runtime-lifecycle.md separates two phases this package had fused. Phase 4,
// Normalization and Resolution, may "resolve references, apply safe defaults, infer semantic roles,
// expand authoring conveniences" -- and binding a `person` Field to mch_user is exactly an authoring
// convenience expanded: 001 Principle #6 ("Infer Before Configure") is why an author never writes
// `machine: mch_user` by hand, and 004 lists `person` as a semantic type in its own right.
//
// That inference lived inside Parse, so it reached YAML and nothing else. The consequence surfaced from
// the far end: `Validate` checks a Permission's actor Field with `IsReference()`, which reads
// RelatedMachine -- so a Machine built in Go with a correct `person` Field failed validation with a
// message complaining about its type. Two test fixtures were in that state, and the fix is not to make
// every hand-built Machine configure what the runtime infers (which would invert 001 #6) but to make
// the inference reachable.
//
// **Idempotent, and deliberately narrow.** It sets only what is unambiguous: a person Field's target.
// Default coercion stays in Parse because it works on the *document's* string form, which a built
// Machine no longer has. When the next inference arrives, this is where it goes -- and 005's own
// requirement applies to it: "the normalized result must be inspectable enough to explain important
// runtime decisions", which is why this mutates named fields rather than hiding behind a rebuild.
func Normalize(m *domain.Machine) *domain.Machine {
	if m == nil {
		return nil
	}
	for i := range m.Fields {
		if m.Fields[i].Type == domain.FieldTypePerson {
			m.Fields[i].RelatedMachine = domain.UserMachineID
		}
	}
	return m
}
