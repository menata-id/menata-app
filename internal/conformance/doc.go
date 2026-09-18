// Package conformance holds executable checks for architectural obligations that 001-007 and each
// package's own doc.go state in prose (ROADMAP.md Phase 18).
//
// It contains no runtime code and is imported by nothing. Its only job is to fail `make test`
// when a plane boundary is crossed, so drift is caught the day it is written rather than at the
// next manual audit -- the failure mode that ended menata-runtime's own composable rewrite, whose
// audit found the Behavior plane "was never designed, not merely unbuilt."
package conformance
