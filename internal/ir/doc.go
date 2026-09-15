// Package ir holds the normalized intermediate representations that normalized metadata compiles
// into: Domain IR, Data IR, and UI IR (005-runtime-lifecycle.md Phase 5,
// 007-composable-runtime-architecture.md §15-16).
//
// IR is a runtime-internal artifact. It must never be required from or exposed as Runtime
// Metadata, and it must not embed HTML, CSS classes, SQL, or other physical implementation.
package ir
