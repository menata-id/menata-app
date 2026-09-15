# 001. Design Principles

> Design Principles define the architectural philosophy of Menata Runtime.
>
> They guide every architectural decision while remaining independent from implementation technologies.
>
> As technologies evolve, these principles should remain stable.

---

# Core Principles

## 1. Machine First

Menata Runtime is designed primarily for deterministic machine interpretation and execution.

Human readability is important.

Machine correctness is mandatory.

Runtime Metadata should always prioritize consistency, determinism, and correctness over human convenience.

---

## 2. Runtime First

The runtime owns application realization.

Metadata describes application intent.

The runtime determines how that intent is realized.

Application behavior belongs to the runtime.

---

## 3. Metadata First

Applications are defined by Runtime Metadata.

Application source code implements the runtime.

Runtime Metadata defines applications.

Application evolution should primarily occur by changing Runtime Metadata rather than application source code.

---

## 4. Declarative

Runtime Metadata describes **what** applications should become.

The runtime determines **how** applications are realized.

Implementation details belong to the runtime.

Runtime Metadata must not become an imperative programming language or a physical execution plan.

---

# Architecture Principles

## 5. Convention over Configuration

The runtime should provide intelligent defaults.

Configuration should only exist where application intent cannot be inferred safely.

Simple applications should require minimal configuration.

---

## 6. Infer Before Configure

Inference is preferred over explicit configuration.

Whenever application behavior can be inferred safely, explicit metadata should not be required.

Configuration should primarily describe exceptions.

**Inference must be inspectable.** When inference materially affects data access, composition, authorization, rendering, or execution planning, the runtime should be able to expose the resolved result through diagnostics or equivalent tooling. Hidden inference that cannot be explained is not an acceptable substitute for explicit configuration.

---

## 7. Composable

Applications should be assembled from reusable runtime elements across three complementary composition domains:

- **Domain** — Machine, Field, Event, Constraint, Permission;
- **Data** — DataSource, Dataset, Relation, Dimension, Measure, Projection, Expression, Filter, Sort, Query;
- **Experience** — Page, Layout, Section, Component, View, Slot, Binding, Static Content.

Behavioral elements such as Action, Trigger, Event, Constraint, and Process connect these domains without making UI elements responsible for business logic.

Each reusable element should have a bounded semantic contract and stable identity where it is persisted or referenced.

A View remains a supported convenience abstraction, but it is not the universal composition primitive. New capabilities should prefer composition from existing generic primitives before introducing another specialized View type.

The canonical composition model and lowering rules are defined by `007-composable-runtime-architecture.md` and operationalized by `composable-runtime-architecture-map.md`.

---

## 8. Reference over Duplication

Relationships should be expressed through references.

Business Knowledge should have a single source of truth.

Duplicated metadata should be avoided whenever possible.

Logical composition should reference reusable semantic artifacts rather than recursively copying their definitions.

---

## 9. Workspace Isolation

Workspace is the primary execution boundary.

Applications belong to workspaces.

Isolation applies to:

- ownership,
- visibility,
- governance,
- security,
- deployment.

Cross-workspace interaction should always be explicit.

---

# Evolution Principles

## 10. Live Evolution

Applications should evolve continuously.

Changing Runtime Metadata changes application behavior.

Application evolution should not require application source regeneration.

---

## 11. Data Preservation

Business data is more valuable than Runtime Metadata.

Runtime Metadata may evolve.

Applications may evolve.

The runtime may evolve.

Business data should remain preserved.

Potentially destructive changes should always require explicit migration decisions.

---

## 12. Long-term Compatibility

Business Knowledge should survive multiple runtime generations.

Runtime evolution should preserve compatibility whenever reasonably possible.

Organizations should not lose Business Knowledge because runtime implementation evolves.

---

## 13. Technology Adaptable

Implementation technologies will evolve.

Programming languages may change.

Rendering engines may change.

Databases may change.

Infrastructure may change.

Business Knowledge should remain stable across technological evolution.

---

# Platform Principles

## 14. Single Runtime

A single runtime should be capable of realizing one application or thousands of independent applications.

Applications are isolated by Runtime Metadata.

Not by runtime instances.

---

## 15. Open Platform

Menata Runtime should remain extensible.

Additional capabilities should be introduced through extension rather than modification of unrelated runtime layers.

The core runtime should remain stable.

A compile-time/static registry seam is an implementation mechanism for closed, known capability types; it does not imply dynamic plugin loading or arbitrary runtime code execution.

---

## 16. Compatible Authoring

Runtime Metadata should be implementation independent.

Menata Apps Builder is the reference authoring tool.

However, Runtime Metadata may be produced by:

- Menata Apps Builder,
- visual builders,
- command-line tools,
- manual editors,
- compatible third-party builders,
- any implementation that follows the Runtime Language specification.

The runtime does not depend on how Runtime Metadata was created.

---

# Runtime Realization Principle

## 17. Compile, Plan, Execute — Without Code Generation

Menata Runtime should be described as a **runtime-compiled and executed metadata system** rather than a pure interpreter or a source-code generator.

The runtime may internally perform:

```text
Runtime Metadata
      ↓
Parse / Validate
      ↓
Normalize / Resolve
      ↓
IR / Lowering
      ↓
Execution Planning
      ↓
Physical Execution
      ↓
Render / Respond
```

These internal stages do not generate application source code. They compile declarative metadata into runtime representations and execution plans that remain implementation artifacts.

The distinction matters:

- **No application source generation** is required.
- **Runtime compilation** is allowed and expected.
- **Physical execution plans** are never user-authored Runtime Metadata.

See `007-composable-runtime-architecture.md` for the normative target and `composable-runtime-architecture-map.md` for the cross-document contract.

---

# Performance and Safety Principle

## 18. Logical Composition, Physical Economy

Logical composability must not imply one physical operation per logical node.

The runtime should share compatible dependencies, deduplicate equivalent work, batch compatible operations, push safe computation to the database, and bound concurrency.

Security scope must be applied before optimization that could widen visibility.

The desired invariant is:

> **Many logical components, as few physical operations as the semantics permit.**

---

# Vision

Applications should evolve at the pace of Business Knowledge.

Business Knowledge evolves.

Runtime Metadata evolves.

The runtime evolves.

Applications continuously evolve.

Business Knowledge remains the long-term organizational asset.

---

# Summary

Menata Runtime is built upon four fundamental beliefs.

- Machine First
- Runtime First
- Metadata First
- Declarative

Everything else supports these four principles.
