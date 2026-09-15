// Package expression implements the bounded, deterministic expression language shared by
// constraints, event conditions, filters, derived measures, and conditional visibility
// (007-composable-runtime-architecture.md §9).
//
// Expressions must be side-effect free, incapable of I/O or arbitrary code execution, and
// statically validated before execution (§9.1).
package expression
