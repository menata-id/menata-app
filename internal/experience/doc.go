// Package experience implements the Experience Plane: Page, Layout, Section, Component, View,
// Slot, Binding, and Static Content (006-runtime-model.md "Experience Model",
// 007-composable-runtime-architecture.md §12).
//
// View is a supported convenience abstraction, not the universal composition primitive — new
// requirements should compose from Layout/Component/Binding before introducing a new View type.
package experience
