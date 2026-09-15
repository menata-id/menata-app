// Package data implements the Data Plane: DataSource, Dataset, Relation, Dimension, Measure,
// Projection, and Query — semantic data requirements independent of presentation
// (006-runtime-model.md "Data Model", 007-composable-runtime-architecture.md §7-8).
//
// A Query built here is logical only; physical execution belongs to internal/execution.
package data
