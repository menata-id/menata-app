# 006. Runtime Model

> Runtime Model defines the fundamental semantic concepts used by Menata Runtime to realize composable applications.
>
> The model separates organizational ownership from Domain, Data, Experience, and Behavioral composition. It is independent from serialization formats and implementation technologies.

---

# Purpose

The architecture separates three concerns that are often collapsed in conventional application models:

- **Business Knowledge** — organizational meaning and intent;
- **Runtime Metadata** — declarative realization of that meaning;
- **Runtime Model** — the semantic building blocks from which Runtime Metadata is composed.

The runtime then compiles these concepts into internal representations and execution/render plans.

```text
Business Knowledge
      │
      ▼
Runtime Metadata
      │
      ▼
Runtime Model
      │
      ▼
Domain IR + Data IR + UI IR
      │
      ▼
Dependency Graph / Execution Plan
      │
      ▼
Physical Execution + Rendering
```

The Runtime Model is therefore neither a database schema nor a UI widget catalog. It is the semantic contract between metadata and runtime execution.

---

# Design Goals

The Runtime Model should be:

- deterministic,
- composable,
- reusable,
- extensible through bounded seams,
- implementation independent,
- machine friendly,
- security-aware,
- observable and inspectable after normalization.

Every concept should have a clear responsibility.

---

# Organizational Model vs Composition Model

The organizational hierarchy answers **where a capability belongs**:

```text
Workspace
    └── Application
            └── Machine / shared resources
```

The composable model answers **what is being composed and how it is realized**:

```text
                 Application
                      │
       ┌──────────────┼──────────────┐
       ▼              ▼              ▼
    Domain           Data        Experience
       │              │              │
    Machine        Dataset          Page
    Field          Relation         Layout
    Event          Projection       Component
    Action         Expression       View
    Constraint     Query            Slot
    Permission     Dimension        Binding
                   Measure          Static Content
```

These are different structures. Ownership is hierarchical; composition and execution dependencies are graph-shaped.

---

# Domain Model

## Machine

Machine is the primary runtime realization unit for a business capability and corresponds to an Object in the Menata Language model.

A Machine may carry or reference:

- Fields,
- Events,
- Actions,
- Constraints,
- Permissions,
- Views and experience declarations,
- data definitions or references.

A Machine is not the universal composition boundary. Multiple Machines may contribute to a Dataset, Page, or experience through explicit references and relations.

## Field

Field describes a semantic attribute of a Machine/Object, including its identity, type, constraints, and relevant presentation semantics.

A Field may be referenced by Data and Experience declarations without forcing those declarations to become Machine-specific Views.

## Event

Event identifies something that occurs or a trigger that can initiate runtime behavior.

Events may originate from user actions, data changes, schedules, integrations, or other supported runtime sources.

## Constraint

Constraint expresses a declarative condition that must hold for an operation or state transition.

Constraints are evaluated by the runtime and are not arbitrary executable code.

## Permission

Permission expresses authorization requirements for accessing or changing capabilities and data.

Permission scope is part of execution identity and must be established before optimization or sharing of physical work.

## Action

Action describes user or system intent such as Create, Update, Delete, Submit, Approve, Reject, Publish, or Cancel.

An Action may trigger events, enforce constraints and permissions, and invoke runtime services.

---

# Data Model

The Data Model describes semantic data requirements independently of presentation.

## DataSource

DataSource identifies a logical origin of data, such as Machine records, a reusable Dataset, a service result, or another supported runtime source.

## Dataset

Dataset is a reusable semantic data contract. It defines available data and its meaning without prescribing a renderer.

A Dataset may be consumed by multiple Components, Views, Pages, APIs, or other runtime consumers.

## Relation

Relation describes an explicit association between semantic data sources, grounded where possible in existing Machine/reference semantics.

A Relation is reusable and must not create a second, conflicting identity for the same business relationship.

## Projection

Projection defines the semantic shape consumed by a renderer or downstream operation. It may assign semantic roles such as title, identifier, status, person, money, timestamp, dimension, or measure.

## Dimension

Dimension identifies a semantic grouping attribute used by queries or composed experiences, such as status, month, owner, or project.

## Measure

Measure identifies a semantic aggregate or quantitative value, such as count, sum, average, amount, or duration.

## Expression

Expression is a bounded deterministic computation. It may derive a value from permitted inputs but cannot perform arbitrary code execution or unrestricted I/O.

## Query

Query is a logical data request assembled from DataSource/Dataset, Relation, Projection, filters, dimensions, measures, expressions, sorting, pagination, and parameters.

Query is logical. SQL shape, indexes, joins, generated columns, JSON operations, caching, materialization, and other physical choices remain runtime responsibilities.

The normalized representation of these concepts is **Data IR**.

---

# Experience Model

The Experience Model describes user-facing composition independently from physical data access.

## Page

Page is an experience root and user interaction surface.

## Layout

Layout is a generic structural composition primitive such as stack, row, columns, grid, split, tabs, panel, or section.

## Component

Component is a reusable semantic presentation primitive with a bounded contract.

A Component may declare:

- inputs,
- semantic data requirements,
- child slots,
- bindings,
- actions/events,
- accessibility semantics,
- renderer implementation.

A Component must not silently perform unrelated data access or bypass authorization.

## View

View is a precomposed or domain-oriented presentation/data contract.

Existing View types remain supported for compatibility and authoring simplicity, but View is **not the universal composition primitive**.

The preferred lowering direction is:

```text
List View
  = Collection + Projection + Table renderer

Card List
  = Collection + Projection + Card renderer

Board
  = Collection + Group Dimension + Board renderer

Dashboard
  = Page + Layout + Metric / Collection components
```

A new ViewType should therefore be introduced only when the requirement has genuinely distinct semantics that cannot be expressed by existing composable primitives.

## Slot

Slot is a named composition point into which compatible child components may be inserted.

## Binding

Binding connects a Component input to a value in an explicit runtime scope.

## Static Content

Static Content represents non-data content that can participate in the same Experience tree as dynamic components.

The normalized representation of these concepts is **UI IR**.

---

# Context and Scope

Composable applications require explicit context propagation.

Typical context domains include:

```text
page
route
parameters
current_user
workspace
record
parent_record
selection
query_result
variables
```

A conceptual scope chain is:

```text
Page Scope
   ↓
Section Scope
   ↓
Collection Scope
   ↓
Record Scope
   ↓
Field Scope
```

Bindings may consume values from an enclosing scope only when explicitly permitted by the component/data contract.

Context is semantic input. It is not an implicit authorization mechanism and must not become an uncontrolled data-access escape hatch.

---

# Behavioral Model

Behavior is composed from Domain primitives rather than requiring one monolithic Workflow artifact.

```text
Event
  ↓
Action
  ↓
Permission / Constraint
  ↓
Service / Data operation
  ↓
State change / Event
```

## Workflow

Workflow describes coordination across Events, Actions, Constraints, Permissions, and Services.

Workflow is a **responsibility**, not a mandatory stored runtime artifact and not a second execution engine.

A higher-level process/workflow declaration may exist as authoring syntax, but it must lower into the same runtime behavioral primitives.

## Service

Service exposes runtime capabilities such as background processing, notification, scheduling, messaging, document generation, email, AI services, integrations, or external communication.

Service implementation belongs to the runtime.

## API

API exposes supported application capabilities externally. Runtime Metadata may declare operations, permissions, serialization, and behavior; implementation remains runtime-owned.

---

# Navigation and Theme

## Navigation

Navigation describes movement between application experiences, including menus, breadcrumbs, tabs, shortcuts, contextual links, and quick actions.

Navigation is part of Experience realization but must remain separate from business execution and physical data semantics.

## Theme

Theme defines visual identity such as typography, spacing, icons, branding, and visual tokens.

Theme must not become a hidden business or data dependency.

---

# Shared Resources

Applications may reuse shared semantic resources, including:

- Datasets,
- Relations,
- Projections,
- Expressions,
- Components,
- Views,
- templates,
- themes,
- services,
- integrations.

Shared resources are referenced by stable identity. Reuse should reduce duplication without weakening ownership or security boundaries.

---

# Composition and Dependency Graphs

Composition is not one tree.

The runtime must distinguish at least:

1. **Experience tree** — ownership and rendering order;
2. **Data dependency graph** — Dataset, Query, Relation, Projection, and expression dependencies;
3. **Behavior dependency graph** — Events, Actions, Constraints, Permissions, and Services.

These graphs may be combined for planning, but they must not be collapsed into a single View abstraction.

```text
Experience Tree
      │
      ├──────────────┐
      ▼              ▼
   Data DAG       Behavior DAG
      │              │
      └──────┬───────┘
             ▼
     Runtime Planning
```

Cycles that cannot be semantically resolved must be rejected before execution.

---

# Runtime Compilation Contract

The Runtime Model is lowered through the following conceptual stages:

```text
Runtime Metadata
      │
      ▼
Parse / Validate
      │
      ▼
Normalize / Resolve
      │
      ├──────────────┬──────────────┐
      ▼              ▼              ▼
  Domain IR        Data IR         UI IR
      │              │              │
      └──────────────┼──────────────┘
                     ▼
              Dependency Graph
                     ▼
        Composable Execution Planner
                     ▼
          Physical Execution / Render Plan
```

The Runtime Model therefore defines the semantic vocabulary; it does not dictate the internal class hierarchy, database schema, or renderer implementation.

---

# Extensibility

New capabilities should use existing primitives whenever possible.

A static/compile-time registry may dispatch a closed set of known component, field, action, view, or renderer implementations. This registry is an extension seam, not a dynamic plugin loader and does not permit arbitrary runtime code execution.

A specialized runtime capability is justified when composition cannot satisfy the requirement without semantic ambiguity, unsafe behavior, unacceptable complexity, or unacceptable cost.

---

# Identity and References

Every persisted or addressable runtime element has a stable identity.

Identity should survive:

- renaming,
- presentation changes,
- implementation changes,
- compatible schema evolution.

References preserve target identity without redefining ownership.

Stable identity enables:

- dependency sharing,
- versioning,
- caching,
- migration,
- compatibility,
- incremental recompilation.

---

# Runtime Ownership

| Concept | Responsibility |
|---|---|
| Workspace | Organizational boundary and isolation |
| Application | Cohesive business solution |
| Machine | Business capability realization |
| Field | Semantic attribute |
| Event | Trigger/occurrence |
| Action | User/system intent |
| Constraint | Declarative condition |
| Permission | Authorization requirement |
| DataSource / Dataset | Semantic data source/contract |
| Relation | Semantic association |
| Projection | Semantic data shape |
| Dimension | Grouping semantic |
| Measure | Quantitative/aggregate semantic |
| Expression | Bounded computation |
| Query | Logical data request |
| Page | Experience root |
| Layout | Structural composition |
| Component | Reusable presentation primitive |
| View | Precomposed/domain-oriented presentation contract |
| Slot | Composition point |
| Binding | Context/data connection |
| Static Content | Non-data experience content |
| Service | Runtime capability |
| API | External application access |
| Navigation | User movement |
| Theme | Visual identity |
| Workflow | Behavioral coordination responsibility |

Responsibilities should remain distinct even when existing implementation structures temporarily combine them.

---

# Evolution

The Runtime Model is expected to evolve incrementally.

Evolution should:

- preserve stable semantic identities,
- prefer composition over specialization,
- preserve compatibility where practical,
- avoid duplicating concepts,
- keep physical implementation replaceable,
- make inferred/normalized decisions inspectable,
- maintain deterministic and secure realization.

Existing View-oriented metadata may coexist with the composable model during migration. The compatibility layer should lower legacy Views into the generic composition model rather than allowing two unrelated execution architectures to persist indefinitely.

---

# Summary

The Runtime Model defines the semantic building blocks of Menata Runtime.

Applications are composed across Domain, Data, Experience, and Behavioral concerns.

Ownership is hierarchical, while composition and execution dependencies are graph-shaped.

The runtime validates and normalizes Runtime Metadata, compiles it into Domain/Data/UI IR, builds dependency graphs, plans bounded physical work, executes behavior and data operations, and renders supported experiences.

> **The Runtime Model defines semantic intent; composition defines relationships; planning defines execution; physical technologies remain runtime implementation details.**
