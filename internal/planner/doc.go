// Package planner implements the Composable Execution Planner (CEP): the boundary between
// logical composition and physical execution (007-composable-runtime-architecture.md §18).
//
// The CEP deduplicates shared dependencies, batches compatible operations, bounds concurrency,
// and enforces interactive execution budgets. Security scope must be established before any
// optimization that could widen visibility (§18.10, §20) — this ordering is not optional.
//
// Status: this mechanism is architecturally PROPOSED (007 §34), not proven by prior
// implementation. Build it against real forcing cases, not speculatively.
package planner
