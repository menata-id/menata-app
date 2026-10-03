// Package ir holds the runtime's intermediate representations (007 §15, §16; 005 Phase 5).
//
// **It holds UI IR as of 2026-10-03** (`ui.go`: `UINode`, and `Validate` implementing 007 §15.3's five
// mandatory rejections). Domain IR and Data IR are still elsewhere or still absent, for the reasons below --
// which are now a *partial* record rather than an explanation of emptiness.
//
// # What it used to claim
//
// Until 2026-09-30 this file said the package "holds the normalized intermediate representations that
// normalized metadata compiles into: Domain IR, Data IR, and UI IR". It holds nothing, and nothing imports
// it. That was a doc asserting a capability the tree does not have -- the same class as 007 §40's
// "CEL expression evaluation -- PROVEN" (this runtime has no CEL) and the AI prompt's "notifications ... no
// such capability exists" (they do). A placeholder is fine; a placeholder that claims to be full is not.
//
// # Where Domain IR actually is
//
// **`internal/domain`.** 005-runtime-lifecycle.md Phase 5 defines Domain IR as representing "machines,
// events, actions, constraints, and permissions", and that is precisely what `domain.Machine` holds --
// Fields, Events, Constraints, Permissions, Transitions -- resolved by `internal/metadata`'s Parse →
// Validate → Normalize → stamp pipeline, which is 001 #17's `Normalize/Resolve → IR/Lowering` stage under
// a different package name.
//
// So a second `ir.Machine` type would be two types for one concept, which 001 #8 (Reference over
// Duplication) rules out at the type level as much as in metadata. **This was investigated as a refactor
// on 2026-09-30 and rejected on the documents' own terms**, not on taste -- see the audit note below.
//
// # Why Data IR and UI IR are not here
//
// Neither is deferred for lack of time; each is blocked by something the concepts state.
//
//   - **Data IR** (007 §16) is consumed by the Query Planner. 007 §34 marks the Composable Execution
//     Planner **PROPOSED** and says outright that "no new capability is implicitly admitted merely by
//     naming this boundary", and §40 lists Data IR itself as PROPOSED (`CR-02`). Building the input to an
//     unadmitted stage is building ahead of the admission discipline §34 requires. Trigger: the planner
//     being admitted, or 005 Phase 11 hot reload needing a plan identity.
//   - **UI IR** (007 §15) was deferred here with the trigger "a second render target -- one target means the
//     templ functions already are the tree". **That deferral expired on its own terms and UI IR is now
//     built**; the retraction is worth keeping rather than deleting, because the shape of the mistake
//     recurs. The reasoning was sound when it was written (2026-09-30) and false three days later: it rested
//     on "the templ functions *are* the tree", which holds only while nothing else could express one. After
//     Stages 1 and 2 there were five Layout primitives with 45 call sites and two registered Components with
//     13 -- the vocabulary was shared across screens, and the only thing still hardcoded was the
//     *composition*. A tree of named values is data.
//     The forcing case was never really a second render target. It is 007 §12.4's normative rule -- a View
//     "MUST NOT be required as the universal composition primitive" -- against 38 screens rendered from 38
//     bespoke Go functions, a breach that was already live. §24 prescribes the remedy as *progressive*
//     lowering, which is why this arrived as one block of §15.1's ten-stage pipeline.
//     **A deferral is a measurement with an expiry date** (CLAUDE.md). This one was cited as a reason to stop
//     before being re-measured; the failure record is `menata-app-document`'s
//     `guides/subordinate-mechanisms-must-not-outrank-the-vision.md`.
//
// `internal/planner`'s own doc.go already states this posture for itself ("Status: this mechanism is
// architecturally PROPOSED (007 §34) ... Build it against real forcing cases, not speculatively"). This
// file now matches its sibling rather than overclaiming beside it.
//
// # The audit note this rejects, and why it is recorded rather than deleted
//
// `menata-app-document`'s `audits/2026-09-30-kajian-pemutusan-dari-menata-runtime.md` §5.1 proposes
// `ir.Machine`, constructible only by `metadata.Compile`, so a test fixture cannot write a
// `domain.Machine{...}` literal and use it as a live Machine. The problem it names is real -- the
// "Normalize first" rule lives as prose because the type system does not enforce it -- but measuring the
// population showed the proposal is the wrong answer to it:
//
//   - the discipline is already gated, in the right order, in five packages'
//     `fixturevalidity_test.go` (`metadata.Validate(metadata.Normalize(m))`, each citing 005);
//   - exactly **one** production call to `metadata.Validate` sits outside `internal/metadata`
//     (`aiassist/validate.go`, validating metadata a model just produced -- legitimately);
//   - the other ~31 references are test fixtures, and about a dozen build a `domain.Machine{}` literal
//     without normalising because they are **narrow unit fixtures that are not standing in for a live
//     Machine**. Forcing those through the loader is the same population, with the same ~50% irrelevance,
//     that got the static fixture-discovery gate deleted on 2026-09-29.
//
// Cost measured: 379 `*domain.Machine` sites across 11 packages, to enforce in the type system a rule
// already held by gates, against violations confined to tests. **If that trade is ever worth making, the
// forcing case will be a second consumer of resolved-vs-declared Machines in production, not fixture
// tidiness** -- and this paragraph is here so the next reader re-measures instead of re-deciding from the
// audit alone.
//
// # If you do build here
//
// IR is a runtime-internal artifact. It must never be required from, or exposed as, Runtime Metadata
// (001 #17, 007 §18.13: "Execution plans are physical runtime artifacts. They MUST NOT become
// user-authored metadata"), and it must not embed HTML, CSS classes, SQL or other physical
// implementation -- which is what this package's boundary rule in internal/conformance enforces.
package ir
