// Package authorization evaluates workspace boundary, current-user permissions, record/data
// scope, and constraint conditions. Security is established before optimization, batching,
// caching, or reuse of physical work — never after (005-runtime-lifecycle.md "Security
// Ordering", 007-composable-runtime-architecture.md §18.10, §20).
//
// Two logically identical requests are not shareable if their authorization scopes differ.
//
// Phase 2 (ROADMAP.md) starts minimal: one shared admin credential and a signed session cookie,
// gating every Action at the Machine level. Real multi-user identity and per-Field permission
// are deferred until a real case forces them.
package authorization
