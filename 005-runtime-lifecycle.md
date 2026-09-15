# 005. Runtime Lifecycle

> Runtime Lifecycle describes how Menata Runtime continuously realizes Business Knowledge into running applications through Runtime Metadata, internal compilation, composable execution planning, and physical realization.

Applications are not generated as source code. They are **runtime-compiled and executed from metadata**.

---

# Purpose

Traditional application development commonly follows a source-code lifecycle:

```text
Source Code
    │
    ▼
Compile
    │
    ▼
Build
    │
    ▼
Deploy
    │
    ▼
Run
```

Menata separates application intent from application implementation:

```text
Business Knowledge
      │
      ▼
Menata Language / Authoring
      │
      ▼
Runtime Metadata
      │
      ▼
Parse / Validate
      │
      ▼
Normalize / Resolve
      │
      ▼
Compile to Domain IR + Data IR + UI IR
      │
      ▼
Dependency Analysis / Planning
      │
      ▼
Physical Execution + Rendering
      │
      ▼
Running Application
```

The runtime may cache normalized metadata and execution plans. These are internal runtime artifacts, not generated application source code.

---

# Lifecycle Overview

The lifecycle is continuous:

```text
Business Reality
      │
      ▼
Business Knowledge
      │
      ▼
Runtime Metadata
      │
      ▼
Validate
      │
      ▼
Normalize / Resolve
      │
      ▼
Compile semantic IRs
      │
      ▼
Build dependency DAG
      │
      ▼
Composable Execution Planning
      │
      ├───────────────┬────────────────┐
      ▼               ▼                ▼
 Data execution   Behavior         Render plan
      │           execution             │
      ▼               │                ▼
 Physical DB /       │             Renderer
 cache / service     │
      └───────────────┴────────────────┘
                      ▼
                User Interaction
                      │
                      ▼
               Business Change
                      │
                      └──────────────► Runtime Metadata Update
```

A metadata change therefore causes the affected semantic model and plans to be revalidated and recompiled. It does not require regenerating an application codebase.

---

# Phase 1 — Business Knowledge

Organizations continuously evolve.

Policies, objects, relationships, permissions, processes, and user needs change.

Business Knowledge remains the source of organizational meaning. It does not need to know which database query, renderer, cache, or component implementation will realize that meaning.

---

# Phase 2 — Runtime Metadata

Business Knowledge is realized through Runtime Metadata.

Runtime Metadata declares:

- Domain capabilities,
- Data requirements,
- Experience composition,
- behavior,
- navigation,
- integrations and runtime configuration.

Metadata is declarative and composable. It expresses intent rather than physical implementation.

---

# Phase 3 — Parse and Validation

Before runtime compilation, metadata is parsed and validated.

Validation includes, as applicable:

- schema/structure validation,
- stable identity validation,
- reference resolution,
- type validation,
- composition validity,
- expression safety,
- permission validity,
- workspace isolation,
- dependency validity,
- compatibility/version constraints.

Invalid metadata must not enter executable planning.

---

# Phase 4 — Normalization and Resolution

The runtime resolves metadata into a canonical semantic representation.

Normalization may:

- resolve references,
- apply safe defaults,
- infer semantic roles,
- expand authoring conveniences,
- canonicalize equivalent declarations,
- resolve component and renderer contracts.

Inference follows:

> **Infer before configure, but make inference inspectable.**

The normalized result must be inspectable enough to explain important runtime decisions.

---

# Phase 5 — Runtime Compilation

Runtime compilation transforms normalized metadata into internal representations:

```text
Normalized Metadata
       │
       ├── Domain IR
       ├── Data IR
       └── UI IR
```

The IRs separate semantic concerns:

- **Domain IR** represents machines, events, actions, constraints, and permissions.
- **Data IR** represents datasets, relations, projections, expressions, filters, dimensions, measures, and logical queries.
- **UI IR** represents pages, layouts, components, slots, bindings, static content, and experience structure.

This compilation is an internal runtime operation. It does not produce application source code.

---

# Phase 6 — Dependency Analysis

Composable experiences create dependencies across semantic elements.

The runtime builds a dependency graph/DAG from those relationships.

For example:

```text
Page
 ├── Customer Collection
 │     └── Customer Dataset
 │           ├── Customer
 │           └── Orders Relation
 │                 └── Order Dataset
 │
 └── Approval Metric
       └── Approval Dataset
```

The dependency graph allows the runtime to identify:

- shared work,
- independent work,
- ordering requirements,
- security scopes,
- cycles,
- execution width,
- opportunities for batching or reuse.

The Experience tree describes composition; the dependency graph describes execution dependencies. They are related but not interchangeable.

---

# Phase 7 — Composable Execution Planning

The Composable Execution Planner (CEP) operates above individual physical query planning.

Its responsibility is to determine how the complete composed experience should be executed efficiently and safely.

CEP may:

- deduplicate shared logical work,
- batch compatible operations,
- bound concurrency,
- order dependent operations,
- identify deferred work,
- reuse compatible results or plans,
- enforce execution-cost budgets.

The canonical boundary is:

```text
Experience / Domain composition
             │
             ▼
       Data IR + DAG
             │
             ▼
           CEP
             │
             ▼
    Logical execution units
             │
             ▼
 Query Planner / Cache / Materialization
             │
             ▼
      Physical operations
```

CEP does not replace the database query planner. The database planner remains responsible for physical SQL execution choices within its scope.

---

# Phase 8 — Physical Execution

The runtime executes the planned operations against supported physical resources:

- PostgreSQL/database operations,
- caches,
- services,
- external integrations,
- background execution where applicable.

Physical execution is an implementation concern.

A Dataset should not need to know whether its data is obtained through a join, generated column, JSON operation, cache, materialized representation, or another optimized mechanism.

---

# Phase 9 — Rendering

The render plan realizes the Experience IR through a supported renderer.

```text
UI IR
  │
  ▼
Render Plan
  │
  ▼
Renderer
  │
  ▼
HTML / API / Other supported representation
```

The same semantic Dataset may be consumed by multiple experiences or output forms.

Components must consume declared semantic inputs and data contracts rather than owning hidden queries unrelated to their contract.

---

# Phase 10 — Running Application

A running application is the live realization of the current Runtime Metadata and its compiled plans.

Users interact with pages, components, views, actions, and navigation.

Business data changes through authorized operations.

Runtime execution remains governed by Domain, Data, Experience, permission, and constraint semantics.

---

# Phase 11 — Continuous Evolution

When Runtime Metadata changes:

```text
Metadata Change
      │
      ▼
Validate
      │
      ▼
Invalidate affected normalized model / plans
      │
      ▼
Re-normalize / recompile
      │
      ▼
Re-plan affected dependencies
      │
      ▼
New runtime realization
```

Unchanged plans and compiled representations may be retained when their dependency identity and compatibility remain valid.

This enables incremental evolution rather than treating every metadata change as a complete application rebuild.

---

# Safe Evolution

Not every metadata change has the same impact.

Changes should be classified, for example, as:

- presentation-only,
- additive semantic changes,
- data-shape changes,
- behavioral changes,
- permission/security changes,
- destructive schema/data changes.

The runtime should determine which compiled representations and execution plans are affected.

Potentially destructive changes require explicit migration or compatibility decisions. Business data must be preserved whenever reasonably possible.

---

# Versioning and Plan Identity

Runtime Metadata versions enable:

- compatibility,
- rollback,
- auditing,
- migration,
- dependency analysis.

Internal compiled artifacts and plans should have identities derived from the semantic metadata and relevant execution/security context.

A presentation-only change should not unnecessarily invalidate an unrelated data plan. A permission change must not reuse a plan whose security scope is no longer valid.

---

# Hot Reload and Incremental Realization

Where the runtime deployment model permits it:

1. metadata changes are detected;
2. changes are validated;
3. affected semantic artifacts are normalized and compiled;
4. affected dependency plans are rebuilt;
5. the new realization becomes active atomically or through a safe transition.

Existing valid applications should remain stable while invalid changes are rejected.

---

# Failure Handling

Failure must be isolated by lifecycle stage.

```text
Parse failure       → reject metadata
Validation failure  → reject activation
Normalization error  → reject affected realization
Planning failure     → reject affected execution
Physical failure     → fail affected operation/request
Render failure       → fail affected representation
```

An invalid metadata update must not corrupt business data or silently replace a valid running realization.

Where composition contains independent branches, the runtime may isolate branch failure when the experience contract explicitly permits partial realization. Otherwise, safety and deterministic behavior take precedence.

---

# Security Ordering

Security is evaluated before execution optimization.

The runtime must establish the applicable:

- workspace boundary,
- current-user permissions,
- record/data scope,
- action authorization,
- constraint conditions

before deduplicating, coalescing, batching, caching, or reusing work.

Two logically identical queries are not necessarily shareable if their authorization or security scopes differ.

---

# Runtime Responsibilities

Throughout the lifecycle, the runtime owns:

- metadata loading,
- validation,
- normalization,
- inference,
- reference resolution,
- semantic compilation,
- dependency analysis,
- composable execution planning,
- physical query/execution integration,
- rendering,
- event/action execution,
- constraint enforcement,
- permission enforcement,
- caching and invalidation,
- platform services.

The runtime does not own Business Knowledge as an organizational source of truth.

---

# Lifecycle Principles

The Runtime Lifecycle follows these principles:

- Business Knowledge defines meaning.
- Runtime Metadata declares realization intent.
- Metadata is validated before execution.
- Runtime compilation produces internal IR, not application source code.
- Domain, Data, and Experience remain separate semantic concerns.
- Composition creates explicit dependency graphs.
- CEP optimizes the composed workload; physical planners optimize individual operations.
- Security scope precedes optimization and reuse.
- Physical execution is bounded and observable.
- Metadata changes invalidate only affected runtime artifacts where possible.
- Invalid changes do not replace valid running realizations.
- Business data is preserved.

---

# Summary

Menata Runtime is continuously driven by Runtime Metadata.

The lifecycle is not merely:

```text
Metadata → Interpret → Application
```

It is:

```text
Metadata
  → Validate
  → Normalize / Resolve
  → Domain/Data/Experience IR
  → Dependency DAG
  → Composable Execution Planning
  → Physical Execution + Render Plan
  → Running Application
```

This lifecycle makes composability a runtime concern rather than merely a metadata-authoring feature.

> **Applications are runtime-compiled and executed from metadata; they are not generated as source code.**
