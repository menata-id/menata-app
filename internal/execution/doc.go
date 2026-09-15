// Package execution performs physical execution of planned operations against supported
// resources (PostgreSQL, cache, services) (005-runtime-lifecycle.md Phase 8,
// 007-composable-runtime-architecture.md §17, §21-22).
//
// Physical execution plans are runtime-internal artifacts and must never become portable
// Runtime Metadata (§17, §18.13).
package execution
