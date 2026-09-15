// Package registry is the static, compile-time component/field/action/view type dispatch seam
// (007-composable-runtime-architecture.md §14).
//
// It is not a dynamic plugin loader and must not accept a type string that wasn't compiled into
// the binary. Extension means adding Go code and recompiling.
package registry
