# 002. Runtime Architecture

> This document describes the conceptual architecture of Menata Runtime.
>
> It intentionally avoids implementation details.
>
> The architecture defines responsibilities rather than technologies.

---

# Purpose

Menata Runtime realizes Business Knowledge as living applications.

Business Knowledge itself is not executable.

Runtime Metadata bridges Business Knowledge and executable applications.

The runtime continuously realizes Runtime Metadata into running applications.

Applications are not source-code generated from metadata. They are runtime realizations of metadata through internal compilation, planning, execution, and rendering stages.

---

# High-level Architecture

```text
Business Reality
        │
        ▼
Business Knowledge
        │
        ▼
Menata Language
        │
        ▼
──────────────────────────────
Authoring Layer
──────────────────────────────

Menata Apps Builder
Visual Builder
CLI
Manual Editor
Compatible Tools

        │
        ▼

Runtime Metadata
        │
        ▼
──────────────────────────────
Runtime Realization
──────────────────────────────

Parse / Validate
        ↓
Normalize / Resolve
        ↓
Domain + Data + Experience IR
        ↓
Dependency Graph
        ↓
Execution Planning
        ↓
Physical Execution + Rendering

        │
        ▼

Applications
```

The internal realization pipeline is an implementation concern of the Runtime Layer. Its purpose is to keep metadata declarative while allowing the runtime to optimize physical execution.

---

# Layer Responsibilities

## Business Reality

Business Reality is where organizations operate.

It contains people, activities, assets, policies, workflows, and decisions.

Business Reality continuously changes.

---

## Business Knowledge

Business Knowledge explains Business Reality.

It captures:

- objects,
- rules,
- permissions,
- constraints,
- events,
- relationships,
- views.

Business Knowledge is implementation independent.

---

## Menata Language

Menata Language expresses Business Knowledge.

It is designed primarily for humans.

Its responsibility is to describe business intent.

It does not describe implementation.

---

## Authoring Layer

The Authoring Layer produces Runtime Metadata.

Runtime Metadata may be produced by:

- Menata Apps Builder,
- visual builders,
- command-line tools,
- manual editors,
- compatible third-party tools.

The runtime never depends on how Runtime Metadata is created.

See `benchmarks/018-menata-apps-builder-concept.md` for an early page-concept exploration of what a Menata Apps Builder could contain — exploratory only, no runtime dependency implied.

---

## Runtime Metadata

Runtime Metadata describes how Business Knowledge should be realized.

Runtime Metadata is designed primarily for deterministic machine interpretation.

It contains application realization rather than Business Knowledge.

---

## Menata Runtime

Menata Runtime owns application realization.

Its internal realization may include parsing, validation, reference resolution, normalization, compilation to logical intermediate representations, dependency planning, physical execution, and rendering.

These stages do not create application source code. They create runtime-internal representations and execution plans.

The target composable architecture is defined in `007-composable-runtime-architecture.md`; the cross-document contract is maintained in `composable-runtime-architecture-map.md`.

The runtime owns:

- application lifecycle,
- routing,
- rendering,
- navigation,
- authentication,
- authorization,
- event execution,
- constraint enforcement,
- platform services.

---

## Applications

Applications are runtime realizations.

Applications exist because Runtime Metadata exists.

Applications continuously evolve as Runtime Metadata evolves.

Applications remain isolated from each other through metadata and workspace boundaries.

---

# Separation of Responsibilities

Each layer owns different responsibilities.

| Layer | Responsibility |
|--------|----------------|
| Business Reality | Organizational activities |
| Business Knowledge | Organizational knowledge |
| Menata Language | Business expression |
| Authoring Layer | Runtime Metadata authoring |
| Runtime Metadata | Application realization description |
| Menata Runtime | Application realization |
| Applications | User experience |

Responsibilities should not overlap.

---

# Runtime Boundary

Menata Runtime is responsible only for realizing Runtime Metadata.

The runtime does not own:

- Business Reality,
- Business Knowledge,
- Menata Language,
- metadata authoring.

The runtime realizes Runtime Metadata through internal stages; those stages must not leak physical implementation choices back into metadata.

---

# Composable Runtime Boundary

The composable architecture introduces three logical realization domains:

```text
Domain Plane
    Machine / Field / Event / Constraint / Permission

Data Plane
    DataSource / Dataset / Relation / Projection / Query / Expression

Experience Plane
    Page / Layout / Component / View / Slot / Binding
```

Behavioral constructs connect these domains but do not make presentation elements responsible for business authorization or business execution.

The composition tree belongs to Experience. Data and behavior dependencies form separate semantic graphs. They are combined for planning only after security and reference resolution are known.

`View` remains a supported convenience abstraction and compatibility boundary, not the universal composition primitive.

---

# Metadata Flow

Applications evolve through metadata.

```text
Business changes
        │
        ▼
Business Knowledge changes
        │
        ▼
Menata Language changes
        │
        ▼
Runtime Metadata changes
        │
        ▼
Parse / Validate / Normalize
        │
        ▼
Logical IR + Dependency Graph
        │
        ▼
Execution Plan + Render Plan
        │
        ▼
Applications evolve
```

Business changes should not require rewriting application source code.

---

# Runtime Independence

The runtime should remain independent from:

- programming languages,
- rendering technologies,
- storage technologies,
- infrastructure,
- deployment environments.

Only Runtime Metadata should determine application behavior; physical strategies remain runtime-owned.

---

# Single Runtime

A single runtime may host:

- one application,
- dozens of applications,
- hundreds of applications,
- thousands of applications.

Applications remain independent through Runtime Metadata and workspace isolation.

The runtime remains a single execution environment.

---

# Workspace Boundary

Workspace is the primary organizational boundary.

Each workspace owns:

- applications,
- metadata,
- permissions,
- governance,
- deployment configuration.

Cross-workspace interaction should always be explicit.

Security scope enters the logical data plan before query sharing, caching, batching, or other physical optimization.

---

# Evolution

Business Reality evolves.

Business Knowledge evolves.

Runtime Metadata evolves.

Applications evolve.

The runtime itself also evolves.

Business Knowledge remains the long-term organizational asset.

---

# Related Research

The layered architecture above was informed by studying architecture patterns from other world-class systems — browser engines, Kubernetes, Terraform, React, VS Code, and others. See [architecture-benchmark.md](architecture-benchmark.md) for the full comparison and the design implications drawn from each.

For the composable architecture specifically, see `007-composable-runtime-architecture.md`, `composable-runtime-architecture-map.md`, and `composable-runtime-roadmap.md`.

---

# Summary

Menata Runtime follows a simple architectural philosophy.

Business Knowledge explains organizations.

Runtime Metadata explains application realization.

Menata Runtime compiles, plans, executes, and renders Runtime Metadata without generating application source code.

Applications become living representations of Business Knowledge.
