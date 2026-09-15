// Package rendering realizes UI IR plus a resolved View Model into a concrete representation
// (HTML/Templ, JSON, CSV, ...) (007-composable-runtime-architecture.md §23).
//
// The reference renderer is server-rendered HTML via templ + HTMX, matching the single-binary,
// modest-server operating model (007 §4.10) — no mandatory client-side data/reactive framework.
package rendering
