# 003. Runtime Language

> Runtime Language defines how applications are described for Menata Runtime.
>
> Unlike Menata Language, which expresses Business Knowledge, Runtime Language expresses Application Realization.
>
> Runtime Language is designed primarily for deterministic machine interpretation and execution.

---

# Purpose

Business Knowledge explains how organizations work.

Runtime Language explains how applications should behave.

Business Knowledge answers:

> What does the business know?

Runtime Language answers:

> How should the runtime realize that knowledge?

Business Knowledge and Runtime Language serve different purposes.

Neither replaces the other.

---

# Design Philosophy

Runtime Language is designed for machines.

Not for Business Analysts.

Human readability is desirable.

Machine correctness is mandatory.

The language should always prefer:

- determinism,
- consistency,
- explicitness,
- composability,
- predictability.

Runtime Language is not intended to become another programming language.

It is a declarative metadata language.

---

# Relationship with Menata Language

Menata Language and Runtime Language describe different concerns.

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
Runtime Language
        │
        ▼
Runtime Metadata
        │
        ▼
Menata Runtime
        │
        ▼
Applications
```

Menata Language describes business.

Runtime Language describes application realization.

Menata Language itself — its grammar, and how to write Business Knowledge in it — is owned by a separate repository, [`menata-id/menata`](https://github.com/menata-id/menata). This repo (`menata-runtime`) begins at Runtime Language; everything above that line in the diagram belongs to `menata-id/menata`, not here.

---

# Runtime Metadata

Runtime Language is expressed through Runtime Metadata.

Runtime Metadata is the executable description consumed by Menata Runtime.

Applications are realized from Runtime Metadata without generating application source code.

For what Runtime Metadata may describe, its hierarchy, and its lifecycle properties (versioning, stable identity, serialization), see [004-runtime-metadata.md](004-runtime-metadata.md).

---

# Runtime Compilation and Execution

“Interpreted” describes the product-level property that applications remain defined by metadata rather than generated application source code. Internally, the runtime may compile declarative metadata into validated, normalized, and optimized runtime representations.

The canonical realization pipeline is:

```text
Runtime Metadata
      ↓
Parse / Validate
      ↓
Resolve / Normalize
      ↓
Domain + Data + Experience IR
      ↓
Dependency Graph
      ↓
Execution / Render Planning
      ↓
Physical Execution
      ↓
Response / Rendering
```

Compilation here means compilation to runtime-internal representations, not source-code generation.

Physical execution plans are runtime artifacts. They are never Runtime Metadata and must never be required from metadata authors.

The composable target architecture and intermediate representations are specified in [007-composable-runtime-architecture.md](007-composable-runtime-architecture.md). The cross-document contract is maintained in [composable-runtime-architecture-map.md](composable-runtime-architecture-map.md).

---

# Declarative

Runtime Language describes intent.

It does not describe physical execution steps.

For example:

Instead of saying:

> Create HTML.

The language describes:

> Display customer information.

The runtime determines how that information is rendered and which physical operations are appropriate.

---

# Governing Principles

Runtime Language is not a separate principle system. It is governed by the same architectural principles as the rest of Menata Runtime — Machine First, Runtime First, Metadata First, Declarative, Composable, Infer Before Configure, Workspace Isolation, Open Platform, Compatible Authoring, and the others.

See [001-design-principles.md](001-design-principles.md) for the authoritative statement of each. This document does not restate them.

---

# Summary

Menata Language explains Business Knowledge.

Runtime Language explains Application Realization.

Menata Runtime compiles and executes Runtime Language internally without generating application source code.

Applications become living representations of Business Knowledge.

Runtime Language is designed for machines.

Business Knowledge remains designed for humans.
