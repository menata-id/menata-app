// Package metadata parses, validates, and normalizes Runtime Metadata — the front door of the
// realization pipeline (003-runtime-language.md, 004-runtime-metadata.md). It resolves
// references and applies safe inference before handing normalized declarations to internal/ir.
//
// Invalid metadata must not reach compilation (005-runtime-lifecycle.md, Phase 3-4).
package metadata
