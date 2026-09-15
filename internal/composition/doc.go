// Package composition builds the Experience tree and derives the semantic dependency graph
// (data, expression, relation, security, render-input dependencies) from it
// (007-composable-runtime-architecture.md §5, §18.2).
//
// The Experience tree and the dependency graph are related but not interchangeable: the tree
// governs ownership and rendering order, the graph governs execution dependencies.
package composition
