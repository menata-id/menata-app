# 004. Runtime Metadata

> Runtime Metadata is the declarative, executable description from which Menata Runtime realizes an application.
>
> It declares semantic intent. The runtime validates, normalizes, compiles, plans, and executes that intent without generating application source code.

---

# Purpose

Business Knowledge explains what an organization knows and intends.

Runtime Metadata describes how that knowledge is realized as a running application.

The relationship is:

```text
Business Reality
      │
      ▼
Business Knowledge
      │
      ▼
Menata Language / Authoring
      │
      ▼
Runtime Metadata
      │
      ▼
Parse → Validate → Normalize → Compile / Plan
      │
      ▼
Running Application
```

Runtime Metadata is therefore the bridge between organizational meaning and runtime realization.

Business Knowledge remains implementation independent. Runtime Metadata contains realization intent, while the runtime determines the physical execution and rendering strategy.

---

# Runtime Metadata is not Business Knowledge

Business Knowledge should not describe runtime implementation details.

Runtime Metadata should not redefine the meaning of Business Knowledge.

The separation is intentional:

- **Business Knowledge** defines organizational concepts, rules, policies, and intent.
- **Runtime Metadata** declares how those concepts are exposed, composed, queried, acted upon, and presented.
- **Runtime** determines how the declarations are physically realized.

This separation allows the same business knowledge to support different application experiences and allows runtime implementation to evolve without changing the business model.

---

# Metadata Characteristics

Runtime Metadata should be:

- deterministic,
- declarative,
- composable,
- versionable,
- machine-readable,
- implementation independent,
- referenceable by stable identity,
- inspectable after inference and normalization.

Metadata should express **semantic intent**, not framework-specific implementation.

For example, metadata should prefer a semantic `status`, `money`, `person`, `collection`, or `stack` over a renderer-specific widget or HTML structure.

---

# Three Composition Planes

Composable applications are not composed only from Views. Runtime Metadata declares three related planes.

```text
                    Runtime Metadata
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
       Domain            Data          Experience
          │                │                │
   Machine / Field    Dataset / Query   Page / Layout
   Event              Projection        Component
   Constraint         Relation          View
   Permission         Expression        Slot / Binding
```

## Domain Plane

The Domain Plane describes executable business capability:

- Machine,
- Field,
- Event,
- Constraint,
- Permission,
- Action.

These elements describe what the application can do and under which conditions it can do it.

## Data Plane

The Data Plane describes semantic data requirements independently from presentation:

- DataSource,
- Dataset,
- Relation,
- Projection,
- Dimension,
- Measure,
- Expression,
- Filter,
- Sort,
- Query.

A Dataset or Query may be consumed by multiple experiences. A data declaration must not depend on a particular View or renderer.

## Experience Plane

The Experience Plane describes how users encounter application capabilities:

- Page,
- Layout,
- Section,
- Component,
- View,
- Slot,
- Binding,
- Static Content,
- Navigation.

Experience composition is independent from the physical data implementation.

---

# Organizational Scope and Composable Scope

Runtime Metadata has an organizational hierarchy, but that hierarchy is not the same as the execution composition graph.

```text
Workspace
  └── Application
       └── Machine / shared resources
```

Within an application, declarations may compose across machines and shared resources through stable references.

The runtime must distinguish:

1. **ownership** — where metadata belongs;
2. **composition** — what semantic elements are combined;
3. **dependency** — what execution requires;
4. **scope** — what context and security boundary applies.

This distinction is fundamental to composable execution planning.

---

# Workspace

Workspace is the highest organizational boundary.

A workspace owns:

- applications,
- permissions and governance,
- shared resources,
- runtime configuration,
- deployment-related configuration.

Workspace isolation is a runtime invariant. Composition must never allow an implicit cross-workspace data or capability dependency.

---

# Application

Application is an independently realizable business solution within a workspace.

An application may compose capabilities from multiple Machines and shared runtime resources.

Applications share runtime infrastructure but remain logically isolated.

---

# Machine

Machine is the primary runtime realization unit for a business capability and corresponds to an Object in the Menata Language model.

A Machine may carry or reference:

- Fields,
- Events,
- Constraints,
- Permissions,
- Actions,
- Views and experience declarations,
- data definitions or references.

A Machine is not required to be the boundary of every composition. Multiple Machines may contribute to one Dataset, Page, or user experience through explicit relations and permissions.

---

# Data Metadata

## DataSource

Identifies a logical origin of data. A DataSource may represent Machine records, a reusable Dataset, a service result, or another supported runtime source.

## Dataset

A reusable semantic data contract. It describes what data is available and its meaning without deciding how it is rendered.

## Relation

Describes an explicit association between semantic data sources, normally grounded in existing Machine/reference semantics.

## Projection

Defines the semantic shape consumed by an experience or downstream operation. Projection may assign semantic roles such as title, status, person, money, identifier, or timestamp.

## Query

Represents a logical data request. It may contain source, projection, relation, filters, dimensions, measures, expressions, sorting, pagination, and parameters.

A Query is logical; SQL or another physical operation is a runtime concern.

## Expression

A bounded deterministic computation used by metadata. Expressions cannot perform arbitrary code execution or unrestricted I/O.

These concepts are normalized into **Data IR** before physical execution.

---

# Experience Metadata

## Page

A Page is an experience root and user interaction surface.

## Layout

Layout composes child experiences spatially or structurally, such as stack, row, grid, split, tabs, panel, or section.

## Component

Component is a reusable semantic presentation primitive with a bounded contract. It may declare inputs, data requirements, child slots, bindings, actions/events, and accessibility semantics.

A Component must not silently introduce unrelated data access or bypass authorization.

## View

View is a precomposed or domain-oriented presentation/data contract retained for compatibility and authoring convenience.

View is **not the universal composition primitive**. Requirements expressible using generic Page, Layout, Component, Dataset, Projection, Binding, and Static Content should not require a new ViewType merely because the current implementation is View-oriented.

## Slot

A named composition point into which compatible child components may be inserted.

## Binding

Connects a component input to an explicit context or semantic data value.

## Static Content

Represents non-data content that can participate in the same Experience tree as dynamic components.

Experience declarations are normalized into **UI IR**.

---

# Context, Scope, and Binding

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

A Binding resolves a value against an explicit scope. Child components may consume parent context only where the contract permits it.

Context is semantic input; it is not an implicit authorization or data-access mechanism.

---

# Behavior Metadata

Behavior is composed from:

- Event,
- Action,
- Constraint,
- Permission,
- Service,
- API.

A higher-level workflow or process description may exist as an authoring convenience, but it must lower into the same behavioral primitives rather than creating an unrelated execution model.

The runtime is responsible for enforcing authorization and constraints before performing physical work.

---

# Navigation Metadata

Navigation describes movement between experiences:

- menus,
- breadcrumbs,
- tabs,
- shortcuts,
- contextual links,
- quick actions.

Navigation is part of Experience composition but must remain independent from business execution and physical data access.

---

# Metadata Compilation Boundary

Runtime Metadata is not interpreted as an unstructured collection of configuration at every request.

The canonical realization path is:

```text
Runtime Metadata
      │
      ▼
Parse / Validate
      │
      ▼
Normalize / Resolve
      │
      ├───────────────┬───────────────┐
      ▼               ▼               ▼
   Domain IR        Data IR          UI IR
      │               │               │
      └───────────────┼───────────────┘
                      ▼
              Dependency Graph
                      │
                      ▼
         Composable Execution Planner
                      │
             ┌────────┴────────┐
             ▼                 ▼
       Data execution       Render plan
             │                 │
             ▼                 ▼
       physical runtime     renderer
```

This is **runtime compilation**, not application source-code generation. Internal IRs, dependency graphs, execution plans, caches, and compiled metadata representations are valid and expected implementation mechanisms.

---

# Stable Identity

Every persisted or addressable metadata element should have a stable identity.

Identity must survive:

- label changes,
- presentation changes,
- layout changes,
- implementation changes.

Stable identity enables dependency sharing, versioning, migration, caching, compatibility, and safe evolution.

---

# References and Reuse

References connect existing semantic elements without duplicating their identity.

Composition should prefer references over copied definitions.

A reference must preserve:

- target identity,
- ownership boundary,
- authorization boundary,
- version/compatibility semantics.

The runtime may resolve references during normalization and planning.

---

# Metadata Versioning and Evolution

Runtime Metadata evolves as applications evolve.

Versioning should support:

- compatibility,
- rollback,
- migration,
- auditing,
- dependency analysis.

Changes should be classified by impact. Additive changes may be realized directly; behavioral or destructive changes may require explicit migration or compatibility decisions.

Application source-code regeneration is not required.

---

# Inference

The runtime may infer safe defaults from metadata, conventions, or established relationships.

The governing rule is:

> **Infer before configure, but make inference inspectable.**

Whenever inference materially changes the normalized model or execution plan, the runtime should be able to expose the resulting decision for diagnostics, testing, and debugging.

Inference must not silently weaken security, ownership, or deterministic semantics.

---

# Serialization Independence

Runtime Metadata is conceptual and does not require one serialization format.

Possible representations include YAML, JSON, TOML, XML, database-backed metadata, or future formats.

Serialization is an authoring/storage concern. The normalized Runtime Model and IRs remain the execution contract.

---

# Summary

Business Knowledge defines organizational meaning.

Runtime Metadata declares application realization.

Domain, Data, and Experience metadata provide composable semantic primitives.

The runtime validates and normalizes those declarations into internal representations and dependency graphs, plans bounded physical work, executes behavior and data operations, and renders experiences.

> **Runtime Metadata expresses intent; the runtime determines physical realization.**
