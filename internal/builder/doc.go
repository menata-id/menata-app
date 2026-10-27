// Package builder is the Authoring Layer's edit path (001 #16, 002): small, named operations on an installed
// Application -- rename a Field, make it required, add an option, change who may do an action -- that the Apps
// Builder's inspector sends instead of a conversation.
//
// It adds no write path. An operation list is applied to the Application as DescribeApplication shows it, which
// yields the whole desired state, and that goes through the one route every metadata edit takes:
// aiassist.Validate -> PlanUpdate -> Preview -> Write. So the safety policy (add anything; never remove or retype
// what holds records; removals wait for the owner) is written once, in plan.go, and an operation that breaks it is
// refused with the same words as the same edit made through the assistant. The pairing test holds that.
//
// Each operation names the catalogue entry (registry.Capabilities) of the key it edits, and a test holds every
// operation to a key the catalogue lists as writable -- the first step of generating operations from the
// catalogue instead of keeping a list by hand (menata-app-document guides/membangun-menuju-mockup.md, P3).
// Operations for keys the generated shape cannot yet carry arrive with their slot (audits/2026-10-27-kajian-apps-builder.md,
// Fase 1 and the editor phases that own each key).
package builder
