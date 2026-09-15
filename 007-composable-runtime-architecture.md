# 007. Composable Runtime Architecture

> This document defines the architectural direction for evolving Menata Runtime from a
> View-oriented metadata runtime into a **Composable Application Runtime**.
>
> It is an architectural specification, not a rewrite plan. Existing Runtime Model concepts,
> capability registry entries, conformance discipline, Process Overlay compilation, and the
> current server-rendered implementation remain valid. New mechanisms should extend and unify them
> rather than replace them.
>
> Status: Draft v0.4 — adds **Composable Execution Planner** as a first-class execution boundary to
> make composition cost, dependency sharing, batching, and bounded physical work explicit.
> Previously v0.3 — evidence chain and terminology reconciliation (2026-09-11): §14 renamed to
> Static Component Registry with an explicit non-dynamic-dispatch statement; §40 replaced with a
> claim-by-claim citation matrix distinguishing PROVEN (implemented, conformance-cited) from
> PROPOSED (an architectural target, not yet built); a Tier-1-semantics note added below;
> cross-linked to `composable-runtime-blueprint.md`. Previously v0.2 — bounded-component rule,
> composition-cycle rejection, composition cost budget, two admission-gate questions added.
>
> > **What Tier 1 status means for this document.** Tier 1 in this repository means *normative
> > architectural direction and constraint*, not a certification that every mechanism described
> > here is built or proven — §41 (Open Research Questions) and §42 (Recommended Next Studies)
> > already concede that much of this is still a hypothesis. Every substantive claim in §40 below
> > is marked **PROVEN** (implemented and conformance-cited — a `CAP-` row, test ID, or file/line)
> > or **PROPOSED** (an architectural target awaiting the study or case that would validate it).
> > A PROPOSED claim carries no conformance backing and must still pass `capability-lifecycle.md`'
> > §2's A1–A5 admission gate before it becomes a capability — this document does not admit anything
> > by itself.
>
> **Relationship to `composable-runtime-blueprint.md`.** This document (Tier 1) answers *what the
> architecture must be* — the model, contracts, invariants, and boundaries. `composable-runtime-
> blueprint.md` (Tier 3) answers *how the runtime gets there and proves it* — current-state gaps,
> phased sequencing, each phase's own forcing condition, and the benchmark program. Read this
> document for the target shape; read the blueprint for what's actually built today and what has to happen, in what order, before the rest is.

---

# 1. Purpose

Menata Runtime already provides a metadata-driven, declarative runtime in which applications are
realized from Runtime Metadata. The Runtime Model is composable at the artifact level: Machines,
Views, Events, Constraints, Permissions, Pages, Services, APIs, Navigation, and Shared Resources
have distinct responsibilities and stable identities.

The next architectural problem is deeper composition:

```text
Data
  → Query
  → Projection
  → Context
  → Composition
  → Component / Layout / View
  → UI IR
  → Execution Planning
  → Renderer
```

The objective is to make composition a **universal runtime abstraction** across three dimensions:

1. **Data composition** — reusable semantic datasets, queries, projections, relations, measures,
   dimensions, and expressions.
2. **UI composition** — generic layouts, components, views, static content, bindings, slots, and
   nested experiences.
3. **Behavior composition** — reusable events, actions, constraints, permissions, and process
   primitives.

The runtime then compiles these declarations into efficient execution and rendering plans.

The architectural goal is:

> **High composability without sacrificing deterministic execution, security, or performance.**

---

# 2. Scope

This document governs the architecture of:

- Runtime Model evolution related to composition;
- semantic data and query abstractions;
- data projection and View Model concepts;
- generic UI composition;
- component and layout resolution;
- context and binding;
- intermediate representations (IR);
- query and render planning;
- **composable execution planning, cost governance, dependency sharing, batching, and bounded physical work**;
- caching and physical execution choices;
- renderer independence;
- capability-admission rules for new UI/data constructs.

This document does not redefine:

- Business Knowledge;
- the Menata Language as a business-language specification;
- workspace isolation and governance rules;
- existing event/constraint/permission semantics;
- deployment topology;
- the existing capability registry and conformance ratchet.

Those mechanisms remain authoritative where they already define behavior.

---

# 3. Architectural Position

Menata Runtime should evolve from:

```text
Machine
  └── View
        └── Renderer
```

toward:

```text
                         Runtime Metadata
                                │
                                ▼
                           Normalizer
                                │
             ┌──────────────────┼──────────────────┐
             ▼                  ▼                  ▼
         Domain IR           Data IR             UI IR
             │                  │                  │
      Machine / Event      Dataset / Query      Page / Layout
      Constraint           Projection           Component
      Permission           Expression           Binding
             │                  │                  │
             └──────────────────┼──────────────────┘
                                ▼
                        Execution Compiler
                                │
                ┌───────────────┴───────────────┐
                ▼                               ▼
       Composable Execution               Render Plan
            Planner                            │
                │                             ▼
        ┌───────┼────────┐                Renderer
        ▼       ▼        ▼
    shared   batched   bounded
    work      work     work
        │       │        │
        └───────┼────────┘
                ▼
        Physical Execution
                │
                ▼
             Database
```

The key architectural principle is:

> **Metadata expresses semantic intent. Runtime compilation determines physical realization.**

A declarative component must not need to know whether its data is served by a JSONB query,
an expression index, a generated column, a cache, or a materialized representation.

Likewise, a Dataset must not know whether it is rendered as a table, card, chart, API response,
CSV export, or another supported representation.

---

# 4. Design Principles

## 4.1 Composition over Specialization

A new application requirement should first be attempted as a composition of existing primitives.

A new specialized capability is justified only when composition cannot express the requirement
without introducing semantic ambiguity, unsafe behavior, excessive complexity, or unacceptable
runtime cost.

```text
Existing primitives
      ↓
Can they compose the requirement?
      │
   yes │ no
      │   │
      ▼   ▼
 compose  introduce a generic primitive
             │
             ▼
      specialized capability
      only when semantically unique
```

## 4.2 Semantic over Visual

Metadata should express semantic intent rather than implementation-level styling.

Prefer:

```text
metric
record
collection
section
stack
split
status
person
money
```

over:

```text
<div>
CSS classes
pixel offsets
framework-specific widget names
```

## 4.3 Data over View Coupling

Presentation must consume a semantic data contract rather than directly owning a private query
language whenever the data is reusable.

## 4.4 Projection over Retrieval

Components should request the smallest projection needed for rendering or execution.

The runtime should avoid loading entire records when only a small projection is required.

## 4.5 Runtime-Owned Physical Execution

Metadata defines logical intent. The runtime chooses:

- SQL strategy;
- index usage;
- cache strategy;
- materialization strategy;
- batching;
- parallelism;
- renderer implementation;
- **dependency sharing and physical operation boundaries**.

## 4.6 Determinism

Equivalent metadata must compile to semantically equivalent plans.

Plan construction must not depend on map iteration order, incidental database ordering, renderer
side effects, or non-deterministic capability discovery.

## 4.7 Stable Identity

Datasets, projections, components, layouts, bindings, and other addressable runtime artifacts
must have stable identity where they are persisted or referenced.

## 4.8 Reference over Duplication

Reusable semantic definitions should be referenced by identity instead of copied into multiple
Views or Components.

## 4.9 Security Before Optimization

Permission scoping is part of the logical data plan. It must be applied before aggregation,
projection, caching, and rendering. A cache must never widen the scope of a result.

## 4.10 Server Economy

The composable architecture must remain compatible with Menata's single-binary, modest-server,
server-rendered operating model. Flexibility must not imply an always-on client framework or a
large mandatory dependency graph.

## 4.11 Logical Composition, Physical Economy

Logical composability MUST NOT imply one physical operation per logical node.

The runtime SHOULD minimize physical work by:

- sharing compatible dependencies;
- deduplicating equivalent requests;
- batching compatible operations;
- pushing work down to the database when cheaper and safe;
- bounding concurrent work;
- reusing compiled and immutable execution plans;
- selecting asynchronous execution for work that exceeds interactive budgets.

The desired invariant is:

> **Many logical components, as few physical operations as the semantics permit.**

---

# 5. Composition Model

Composition is defined across a **tree of experience nodes plus a graph of semantic/data
references**. The distinction is intentional:

```text
Experience structure                 Semantic dependency
--------------------                 -------------------
Page                                 Component A ──→ Dataset X
  └── Layout                         Component B ──→ Dataset X
       ├── Component                 Component C ──→ Dataset Y
       └── View                      Dataset Y ──→ Relation Z
```

The UI composition tree determines ownership and rendering order. References create reusable
connections between independently-owned runtime artifacts. Cycles in the experience tree are
invalid; semantic reference cycles are valid only where the referenced capability explicitly
supports them and must not cause unbounded evaluation.

At minimum, the runtime should recognize these composition domains:

```text
Domain
 ├── Machine
 ├── Field
 ├── Event
 ├── Constraint
 └── Permission

Data
 ├── DataSource
 ├── Dataset
 ├── Relation
 ├── Dimension
 ├── Measure
 ├── Projection
 ├── Expression
 ├── Filter
 ├── Sort
 └── Query

Experience
 ├── Page
 ├── Layout
 ├── Section
 ├── Component
 ├── View
 ├── Slot
 ├── Binding
 └── Static Content

Behavior
 ├── Action
 ├── Event
 ├── Trigger
 ├── Constraint
 └── Process
```

These are logical concepts. The concrete serialization and storage representation may evolve.

---

# 6. Domain Plane

The existing Domain Plane remains the source of business capability semantics.

```text
Machine
 ├── Fields
 ├── Events
 ├── Constraints
 ├── Permissions
 └── Views
```

The composable architecture does not replace Machine. Machine remains the primary realization
unit for a business capability.

A Machine is a valid source for Data definitions, but a Data definition may intentionally expose
only a projection of that Machine.

A Machine is therefore not itself a UI data model.

---

# 7. Data Plane

The Data Plane is the architectural layer that decouples data semantics from presentation.

## 7.1 DataSource

A DataSource identifies the logical origin of data.

Initial source kinds may include:

- Machine records;
- another Dataset;
- a declared relation;
- a runtime service source where explicitly supported.

Example:

```yaml
source:
  machine: mch_purchase_request
```

## 7.2 Dataset

A Dataset is a named, reusable semantic data definition.

It describes what data is available without specifying how a particular component renders it.

Example:

```yaml
dataset:
  id: purchase_requests
  source:
    machine: mch_purchase_request

  dimensions:
    - id: status
      field: fld_status

  measures:
    - id: request_count
      aggregate: count

    - id: total_amount
      aggregate: sum
      field: fld_amount
```

A Dataset is the preferred shared data contract for reports, dashboards, tables, cards, charts,
exports, APIs, and other consumers that need the same semantic data.

### Dataset ownership and scope

Reusable Datasets must have an explicit ownership/scope model. The initial architecture recognizes
three useful scopes:

```text
Workspace Dataset
      ↓
Application Dataset
      ↓
Machine-local / View-local query
```

A reusable Dataset should be promoted to the narrowest scope that satisfies all consumers. A
View-local query does not need a persisted Dataset identity merely because it happens to contain a
query. Conversely, a Dataset that is shared across Views, APIs, reports, or applications should
have stable identity and explicit ownership.

Dataset definitions must not contain renderer-specific properties. Renderer choices belong to the
Experience Plane.

## 7.3 Dimension

A Dimension identifies a grouping, labeling, or categorical axis of a Dataset.

Examples:

- status;
- region;
- customer;
- month;
- category.

Dimensions should retain semantic identity independent of renderer.

## 7.4 Measure

A Measure is a named calculation over the Dataset.

Initial aggregate vocabulary may include:

- count;
- sum;
- avg;
- min;
- max;
- count_distinct.

Derived measures may use expressions such as ratio, difference, or percentage where the expression
engine can prove bounded and deterministic evaluation.

## 7.5 Relation

A Relation describes a reusable association between data sources.

Relations should reuse existing Machine reference semantics rather than inventing a second
relationship identity.

A relation may be compiled to a join, semi-join, lookup, or other physical strategy.

## 7.6 Projection

A Projection describes the exact semantic shape consumed by a renderer or downstream runtime
operation.

Example:

```yaml
projection:
  - id: title
    source: fld_number
    semantic_type: title

  - id: requester
    source: fld_requester
    semantic_type: person

  - id: amount
    source: fld_amount
    semantic_type: money

  - id: status
    source: fld_status
    semantic_type: status
```

Projection is not just a field selection. It establishes a stable semantic contract between Data
execution and presentation.

## 7.7 Filter

Filters select records or aggregate groups according to a bounded expression language.

Existing field/operator filter forms remain valid as syntax sugar and may compile to the common
expression representation.

## 7.8 Sort

Sort describes logical ordering. Physical execution determines whether an index, database sort,
or another strategy is used.

## 7.9 Pagination

Pagination is part of the Data Plane because it determines how much data is retrieved, not how it
is visually presented.

The runtime should preserve the current capability that prevents unbounded list retrieval.

---

# 8. Query Model

The Query Model is the executable logical representation of a data request.

A logical query should be representable as:

```go
DataQuery {
    Source
    Projection
    Filter
    GroupBy
    Measures
    Sort
    Limit
    Offset
    Parameters
}
```

This is an architectural abstraction, not a requirement that the current Go structs use this exact
shape.

## 8.1 Query Composition

A Query may be composed from reusable Dataset definitions plus local predicates, projections,
and parameters.

Example:

```text
Dataset: purchase_requests
        +
Filter: status = Submitted
        +
Projection: number, requester, amount
        +
Sort: amount desc
        ↓
Logical Query
```

## 8.2 Query Normalization

Before execution, the runtime normalizes equivalent forms into a canonical logical representation.

Normalization should:

- resolve references;
- merge compatible predicates;
- remove redundant projections;
- preserve permission predicates;
- validate data types;
- determine required fields;
- canonicalize parameter names;
- produce stable plan identity.

## 8.3 Query Pushdown

The runtime should push computation to the database when it is:

- semantically equivalent;
- supported by the target database;
- cheaper than in-process evaluation;
- safe under the permission model.

Simple filters and aggregates should normally execute in SQL rather than after full retrieval.

## 8.4 Query Cost Awareness

The planner should distinguish at least:

```text
P1 interactive read
P2 interactive write
P3 heavy read
P4 asynchronous
P5 boot/reload
```

The existing NFR performance classes remain the governing budget model.

---

# 9. Expression Layer

Expressions are a shared semantic primitive used by:

- computed values;
- constraints;
- event conditions;
- view filters;
- Dataset filters;
- conditional visibility;
- action values;
- derived measures.

A single bounded expression model should replace proliferation of independent mini-languages.

## 9.1 Expression Safety

Expressions must be:

- deterministic;
- bounded;
- side-effect free;
- incapable of I/O;
- incapable of arbitrary code execution;
- statically validated before execution.

## 9.2 Allowed Context

The initial runtime context may expose only explicitly declared values such as:

```text
record
old
current_user
today
now
parameters
```

Access outside this context must fail closed.

## 9.3 Evaluation Strategy

The same expression may be evaluated:

```text
in process
```

or pushed down:

```text
into SQL
```

when equivalence can be proven.

The expression compiler should therefore expose both a logical AST and an execution capability
assessment.

---

# 10. View Model and Binding

A component should not consume raw records by default.

Instead:

```text
Query
 ↓
Projection
 ↓
View Model
 ↓
Component
```

The View Model is the runtime materialization of the semantic projection plus contextual values
needed by a component.

Example:

```text
RecordSummary
 ├── title
 ├── subtitle
 ├── status
 ├── avatar
 ├── href
 └── actions
```

The component does not need to know whether `status` came from a field, an expression, or a
joined relation.

---

# 11. Context and Scope

Composable experiences require a formal context propagation model.

## 11.1 Context

Context is the set of values available to a node during compilation and rendering.

Initial context domains may include:

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

## 11.2 Scope

Context is resolved through lexical runtime scopes.

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

A child can consume parent scope values only when that binding is explicitly permitted.

## 11.3 Binding

Binding connects a component input to a context or semantic data value.

Examples:

```yaml
binding:
  value: context.record.name
```

or:

```yaml
binding:
  dataset: customer_summary
  value: measure.revenue
```

Bindings must be statically resolvable where possible.

---

# 12. Experience Plane

The Experience Plane expresses how a user experience is assembled.

## 12.1 Page

A Page is an experience root. It should not own business logic.

## 12.2 Layout

Layout is a generic spatial composition primitive.

Initial generic layouts may include:

```text
stack
row
columns
grid
split
tabs
panel
section
```

Layout should accept arbitrary compatible children.

The runtime should not require separate layout concepts such as `DashboardLayout`,
`DetailLayout`, or `FormLayout` when their behavior can be expressed through generic primitives.

## 12.3 Component

A Component is a reusable semantic presentation primitive.

Examples:

```text
RecordSummaryCard
ActivityFeed
Metric
Collection
Record
Field
ActionBar
StatusBadge
Avatar
```

Components should be named for reusable semantic or structural identity rather than one business
case.

### Component boundedness

A Component MUST expose a bounded semantic contract. It MUST NOT become "generic" merely by
accepting arbitrary properties or silently performing arbitrary data access.

A component may declare:

- inputs;
- data requirements;
- child slots;
- bindings;
- actions/events;
- accessibility semantics;
- renderer implementation.

A Component MUST NOT silently acquire additional business data that is not represented by its
contract.

Generic composition is achieved by combining bounded components, not by creating an unbounded
`GenericComponent` escape hatch.

## 12.4 View

View remains a supported abstraction but changes in architectural position.

A View should be understood as a **precomposed or domain-oriented experience/data presentation
contract**, not the only way UI is composed.

Examples:

```text
list
form
detail
calendar
board
report
```

Existing View types remain valid. New capabilities should prefer composition before introducing a
new View type.

**Normative rule:** a View MUST NOT be required as the universal composition primitive. A requirement
that can be expressed using Page, Layout, Component, Binding, Dataset, Projection, and Static Content
should not require a new ViewType solely because the current renderer dispatch is View-oriented.

## 12.5 Slot

A Slot is a named composition point into which compatible child nodes may be inserted.

Slots enable a component to expose controlled extension points without exposing its internal
implementation.

Example:

```text
RecordPage
 ├── header
 ├── summary
 ├── body
 └── actions
```

## 12.6 Static Content

Static content should be a first-class composable node when a page needs to combine explanatory
text or visual material with structured data.

Examples:

- heading;
- paragraph;
- image;
- link;
- callout;
- divider.

Static content does not imply arbitrary HTML or a code hatch.

---

# 13. Component Contract

A component should declare a contract similar to:

```text
Component
 ├── identity
 ├── inputs
 ├── data requirements
 ├── child slots
 ├── actions/events
 ├── accessibility semantics
 └── renderer implementation
```

A component must not silently execute unrelated business queries.

Its data requirements must be resolvable from declared bindings and datasets.

Component contracts should be versionable. Breaking contract changes require explicit compatibility
handling rather than silently changing the meaning of existing metadata.

---

# 14. Static Component Registry (Component Registry Seam)

> **Not a dynamic mechanism.** The Component Registry is a compile-time or statically assembled
> registry seam. It is **not** a client-side interpreter, a dynamic plugin loader, or a runtime
> extension mechanism. `prototype/objectstack/docs/composable-view-proposal-reconciliation.md` §4
> already rejected that shape structurally — a dynamic runtime dispatcher exists to let a client
> receive and safely interpret *arbitrary* metadata at runtime, the exact situation ObjectStack's
> React console is in and this server-rendered runtime is not (`app/ARCHITECTURE.md`'s client-side
> JS policy). §9.1/§10 of the same document reaffirmed that verdict twice more. What follows names
> a *single identifiable seam* for a fixed, closed set of component types — the same shape
> `capability-lifecycle.md` §4 already calls a "compile-time registry seam" for field/action/view
> types — not a second, competing extension mechanism.

A static component registry is the preferred long-term dispatch seam for new generic components,
replacing accumulating business-specific `switch` statements scattered across handlers with one
identifiable resolution point.

Conceptually:

```text
component type (closed, known at compile time)
      ↓
contract / schema
      ↓
validator
      ↓
resolver
      ↓
renderer
```

The first implementation may use a statically compiled registry (a Go map or compiler-checked
switch, resolved and validated at load/compile time); dynamic plugin loading is not a requirement
and is not the target — extension still means adding Go code and recompiling, per
`capability-lifecycle.md` §4, exactly as it already does for field/action/view types today.

The registry is responsible for discovering how a component type is implemented. It must not own
business authorization decisions, and it must not accept a component `type` string that wasn't
compiled into the runtime.

Existing `templ` functions may remain behind registered components during migration.

---

# 15. UI Intermediate Representation

The UI IR is the normalized internal representation of a composed experience.

Conceptually:

```go
UINode {
    Kind
    Identity
    Properties
    Bindings
    Children
    Slots
    Actions
    Conditions
}
```

The actual Go representation may differ.

UI IR represents an **experience tree**. Shared datasets, projections, actions, and other semantic
artifacts remain references and are not recursively copied into every node.

## 15.1 UI Compilation

The UI compilation pipeline should be:

```text
Runtime Metadata
      ↓
Parse
      ↓
Validate
      ↓
Resolve references
      ↓
Normalize
      ↓
Build UI IR
      ↓
Bind context
      ↓
Resolve Data requirements
      ↓
Build Render Plan
      ↓
Renderer
```

## 15.2 UI IR Responsibilities

UI IR should encode:

- composition hierarchy;
- semantic component types;
- layout relationships;
- stable identities;
- resolved bindings;
- conditional visibility;
- action references;
- data requirements.

UI IR should not embed HTML, CSS framework classes, SQL, or database-specific implementation.

## 15.3 Composition validity

The compiler MUST reject:

- cyclic experience trees;
- unresolved required child references;
- slot/type mismatches;
- bindings outside the permitted scope;
- recursion that exceeds the configured composition depth limit.

The compiler SHOULD detect duplicate or unnecessary data dependencies before render planning.

---

# 16. Data IR

Data IR is the normalized semantic representation of a data requirement.

Conceptually:

```text
Data IR
 ├── Source
 ├── Projection
 ├── Filter
 ├── Relation
 ├── GroupBy
 ├── Measure
 ├── Sort
 ├── Parameters
 └── Security Scope
```

The Query Planner consumes Data IR and produces a physical execution plan.

Data IR is logical. It MUST NOT expose database-specific storage or index choices to metadata
authors.

---

# 17. Execution Plan

The runtime should distinguish logical intent from physical execution.

```text
Data IR
   ↓
Logical Query Plan
   ↓
Cost / Capability Analysis
   ↓
Physical Query Plan
```

A physical plan may select:

```text
live SQL
indexed SQL
cached result
batched query
materialized dataset
in-process evaluation
```

The selected strategy must remain transparent to metadata authors.

Physical plans are runtime-internal artifacts. They MUST NOT become portable Runtime Metadata.

---

# 18. Composable Execution Planner

The **Composable Execution Planner (CEP)** is a first-class runtime boundary between logical
composition and physical execution. Its purpose is to ensure that increasing application
composability does not cause proportional growth in server-side work.

The planner operates on the combined semantic dependencies of the experience rather than treating
each component as an isolated request.

## 18.1 Canonical pipeline

The normative planning pipeline is:

```text
Composition Tree
      ↓
Dependency DAG
      ↓
Logical Execution Groups
      ↓
Cost / Capability Analysis
      ↓
Costed Execution Plan
      ↓
Shared / Batched / Bounded Physical Operations
      ↓
Execution
      ↓
View Models / Render Inputs
```

The execution planner therefore sits **after semantic composition is known and before physical
work begins**.

## 18.2 Composition Tree → Dependency DAG

The composition tree describes experience structure and ownership. The planner must derive a
separate dependency DAG containing data, expression, relation, security, cache, and render-input
dependencies.

Example:

```text
UI tree                         dependency DAG
-------                         -------------
Dashboard                       KPI Revenue ───────┐
 ├─ KPI Revenue                                  │
 ├─ KPI Orders   ─────────────→ Dataset Sales ───┼→ Query A
 └─ Orders Table                                 │
                                                 └→ Projection P
```

Multiple logical consumers MAY point to the same dependency node. The planner MUST use that fact to
avoid redundant physical work where semantics permit.

## 18.3 Dependency identity and equivalence

The planner SHOULD canonicalize dependencies so that equivalent requests have stable identities.
Equivalence SHOULD account for at least:

- source identity;
- security scope;
- normalized filter;
- normalized parameters;
- projection requirements;
- grouping and measures;
- sort/pagination semantics;
- interpreter/metadata version.

Equivalent or safely coalescible dependencies SHOULD be deduplicated before physical execution.

## 18.4 Shared execution

The planner SHOULD detect shared dependencies such as:

```text
Component A ─┐
Component B ─┼→ same Dataset / compatible Query
Component C ─┘
```

and prefer a shared execution result over one request per component when the resulting physical
shape remains within the projection and cardinality constraints.

## 18.5 Batching

Independent or compatible operations SHOULD be batched when batching reduces database round trips,
network overhead, or serialization work without materially worsening latency for interactive work.

Batching MUST respect security scope, transaction semantics, parameter isolation, and database
planner behavior.

## 18.6 Bounded concurrency

The planner MUST treat parallelism as a budgeted resource. It MUST NOT equate "independent" with
"unlimited goroutines or unlimited database queries".

At minimum, planning SHOULD account for:

```text
request concurrency
workspace concurrency
query concurrency
connection-pool capacity
CPU budget
memory budget
```

A plan MAY serialize, batch, or defer work when the cost model predicts that parallel execution
would exceed those budgets.

## 18.7 Costed execution plan

A Costed Execution Plan should expose runtime-estimated cost dimensions such as:

```text
estimated physical queries
estimated rows read
estimated rows returned
estimated bytes materialized
estimated DB time
estimated CPU work
estimated memory
parallelism width
cache opportunities
```

Exact units are implementation-specific. What matters is that physical execution is selected using
a bounded cost model rather than component count alone.

## 18.8 Interactive budget enforcement

The planner MUST distinguish at least the existing execution classes:

```text
P1 interactive read
P2 interactive write
P3 heavy read
P4 asynchronous
P5 boot/reload
```

When a composed operation exceeds an interactive budget, the planner SHOULD choose an explicit
fallback such as:

```text
shared execution
batching
cache
materialization
partial/lazy loading
asynchronous job
clear rejection with diagnostic
```

It MUST NOT silently execute unbounded work merely because the metadata is structurally valid.

## 18.9 Query reuse versus result reuse

The planner should distinguish:

```text
query-plan reuse
execution reuse
result-cache reuse
```

These are different optimization levels. A query may be planned once but still executed multiple
times; multiple consumers may share one execution; a result may additionally be reused from cache.

The planner SHOULD select the cheapest level that remains semantically correct.

## 18.10 Security ordering

Security filtering is part of the logical DAG and MUST precede optimization that could widen data
visibility.

```text
Component
  ↓
Declared data requirement
  ↓
Permission scope / RLS context
  ↓
Logical DAG
  ↓
Costing + optimization
  ↓
Physical execution
```

No deduplication, batch, cache, or shared execution optimization may merge work across incompatible
security scopes.

## 18.11 Failure isolation

A composed plan SHOULD preserve failure boundaries where practical. A non-critical component that
fails or times out SHOULD not necessarily consume the entire request's remaining execution budget
or invalidate unrelated successful work unless the composition contract requires atomic behavior.

## 18.12 Plan immutability and reuse

Once compiled for a particular metadata/interpreter version and stable security-independent logical
shape, plan fragments SHOULD be immutable and safely reusable across requests. Request-specific
context, parameters, and security scope remain runtime inputs.

## 18.13 Planner output is not metadata

Execution plans are physical runtime artifacts. They MUST NOT become user-authored metadata,
because doing so would couple business declarations to current storage, database, or capacity
characteristics.

---

# 19. Query and UI Plan Relationship

UI composition and query execution are different concerns but should be compiled together enough
to eliminate unnecessary work.

Example:

```text
Page
 ├── Metric revenue
 ├── Orders table
 └── Activity timeline
```

could compile to:

```text
UI IR
 ├── Metric → Dataset A
 ├── Table  → Dataset B
 └── Feed   → Dataset C
```

The **Composable Execution Planner** then derives a dependency DAG from those requirements and may
detect reusable sources, compatible filters, shared projections, or opportunities for batching.
The UI metadata remains unchanged.

---

# 20. Security Integration

Security must enter the logical data plan before physical optimization.

Conceptually:

```text
Component request
      ↓
Dataset
      ↓
Permission scope
      ↓
Logical Data Plan
      ↓
Composable Execution Planner
      ↓
Optimization
      ↓
SQL / Cache / Materialization
```

Never:

```text
query all data
      ↓
render
      ↓
trim unauthorized rows
```

Permissions remain owned by the existing authorization architecture. Data Plane composition must
consume permission decisions; it must not redefine authorization semantics.

Cached results must include security scope in their identity.

Example conceptual cache key:

```text
dataset
+ normalized filter
+ normalized parameters
+ permission scope
+ interpreter/version
```

---

# 21. Performance Architecture

Composable UI must not imply unbounded data retrieval or client-side processing.

## 21.1 Projection Pushdown

Only requested fields should be retrieved.

## 21.2 Filter Pushdown

Filters should execute in PostgreSQL whenever safe and beneficial.

## 21.3 Aggregate Pushdown

Measures should execute in SQL rather than materializing raw records whenever the aggregation is
supported.

## 21.4 Index Awareness

Metadata that declares frequently filtered, sorted, grouped, or joined fields should be available
to index management.

This aligns with the existing metadata-driven index direction.

## 21.5 Cache Awareness

Reusable Dataset results may be cached when:

- the result is expensive;
- freshness requirements permit caching;
- permission scope is part of cache identity;
- invalidation is deterministic enough.

## 21.6 Materialization

Large, expensive, stable semantic datasets may eventually use materialized representations.

Materialization is a runtime optimization, not a new semantic data model.

## 21.7 Batch and Deduplicate

The planner should recognize multiple components that request compatible datasets and avoid
redundant queries.

## 21.8 No Mandatory Browser Data Engine

Server-side data filtering, grouping, and authorization remain the default. Browser virtualization
may be added later for rendering efficiency, but it is not a substitute for server-side query
planning.

## 21.9 Composition Cost Budget

The runtime should measure or bound at least:

- composition depth;
- number of resolved nodes;
- number of distinct Dataset requests;
- number of physical queries;
- estimated render work;
- metadata/IR size;
- **estimated execution DAG width and total physical work**.

A composed experience that exceeds configured budgets should fail clearly or degrade through an
explicit runtime policy. It must not silently produce unbounded work.

---

# 22. Physical Storage Strategy

Logical metadata must remain independent from physical storage.

A logical field may be physically realized as:

```text
JSONB value
expression index
stored column
generated column
materialized aggregate
```

The runtime may evolve physical strategy as workload changes without changing business metadata.

This is the preferred path for keeping a metadata-based logical schema while achieving relational
database performance.

---

# 23. Renderer Architecture

A renderer consumes UI IR and resolved View Models rather than raw Runtime Metadata whenever
possible.

Conceptually:

```text
UI IR + View Model
        ↓
Renderer
```

Possible renderers include:

```text
HTML / Templ
REST / JSON
CSV
XLSX
future client renderer
future mobile renderer
```

The existence of multiple renderers must not duplicate the data semantics.

The current server-rendered HTML/Templ renderer remains the reference implementation.

---

# 24. View Types and the Specialization Rule

Existing View types are not deprecated.

However, new design follows this rule:

> **Do not introduce a new ViewType when the requirement can be represented by existing Data,
> Layout, Component, Binding, and Renderer primitives.**

Examples:

```text
calendar
= collection
+ date dimension
+ calendar renderer
```

```text
board
= collection
+ group dimension
+ board renderer
```

```text
card list
= collection
+ card renderer
+ projection
```

```text
dashboard
= page
+ generic layout
+ metric/chart/collection components
```

The implementation may continue exposing convenience View types for compatibility or authoring
simplicity. The runtime should progressively lower them to generic primitives where practical.

---

# 25. Convenience Abstractions

Menata should support **semantic presets**.

A preset is a higher-level declaration that compiles into generic primitives.

Examples:

```text
List View
Detail View
Dashboard View
Approval Page
```

A preset is successful when the runtime can lower it into a common substrate.

This is the same architectural pattern already proven by Process Overlay, where a higher-level
process declaration compiles into Fields, Events, Permissions, Constraints, and Actions.

---

# 26. Compilation and Lowering

Composable Runtime should use a consistent lowering model:

```text
High-level declarative metadata
           ↓
      normalize / validate
           ↓
      generic intermediate model
           ↓
         lower
           ↓
 existing runtime primitives
```

This permits convenience syntax without creating independent execution engines for every feature.

Examples:

```text
Process
  ↓
Events + Permissions + Constraints
```

```text
Dashboard
  ↓
Page + Grid + Components + Datasets
```

```text
Card List
  ↓
Collection + Card Renderer + Projection
```

The execution-specific lowering continues through the Composable Execution Planner before physical
operations are selected.

---

# 27. Capability Admission Gate

Before adding a new capability, the architecture review must ask:

1. Can an existing primitive express it?
2. Can a combination of existing primitives express it?
3. Would one new generic primitive unlock this and other cases?
4. Is the requirement semantically unique enough to justify a new capability?
5. What is the performance cost of its generic form?
6. What conformance proof will demonstrate it?
7. What existing abstraction should the new capability lower into?
8. Will the capability introduce a new business-specific UI primitive or query mini-language?
9. **How does the capability lower into the Composable Execution Planner, and what physical work can it generate?**

A capability should normally be admitted at the lowest useful abstraction level.

---

# 28. Performance Invariants

The following invariants are architectural constraints for the composable runtime:

1. **No logical-node proportionality requirement.** Adding a component MUST NOT imply a one-to-one increase in physical queries or other physical operations when dependencies are compatible and shareable.
2. **No unbounded fan-out.** A composition MUST have bounded execution width and physical work under configured budgets.
3. **No security-unsafe coalescing.** Optimization MUST NOT merge requests across incompatible security scopes.
4. **No full-record-by-default rule.** Data retrieval SHOULD be projection-driven.
5. **No planner bypass for composable data.** Composed data requirements MUST pass through the execution planning boundary before physical execution.
6. **No arbitrary client-side escape.** Server-side authorization, filtering, grouping, and cost controls remain authoritative.
7. **Deterministic planning.** Equivalent logical inputs MUST produce equivalent plan semantics, subject only to explicitly versioned physical strategy changes.
8. **Bounded interactive latency.** Plans classified as interactive MUST remain inside their configured budgets or use an explicit fallback policy.

These invariants are intended to preserve server performance as composition capability grows.

---

# 29. Compatibility and Migration

The introduction of the Composable Execution Planner does not require the immediate replacement of
existing handlers.

A migration path may be:

```text
existing View
   ↓
lower to implicit logical data requirements
   ↓
Composable Execution Planner
   ↓
existing SQL / repository functions
```

This permits performance planning to become a shared runtime concern before all metadata surfaces
become fully composable.

The planner may initially support a narrow closed set of operations and expand only when real cases
and benchmark evidence justify it.

---

# 30. Reference to the Scale Study

The planner operationalizes performance concerns already identified in
`benchmarks/004-scale-architecture-study.md`, including:

- lazy per-workspace metadata loading and cache;
- singleflight cold-load protection;
- batch loading rather than nested metadata queries;
- metadata-driven index awareness;
- per-workspace concurrency limits;
- separate analytics/report execution capacity;
- pagination and bounded result retrieval.

The Composable Execution Planner does not replace those mechanisms. It provides the common planning
boundary through which composable UI/data requirements can consume them without creating a second
performance architecture.

---

# 31. Open Research Questions

The following remain implementation/research questions rather than settled mechanisms:

- how much query coalescing is beneficial before query complexity hurts the database optimizer;
- how to estimate cost accurately for JSONB/expression-index workloads;
- when to batch versus parallelize;
- when to share a broad projection versus execute narrower projections independently;
- how to expose execution diagnostics without coupling metadata to physical plans;
- whether a persistent plan cache is beneficial beyond immutable in-process plans;
- what workload-dependent thresholds should govern interactive-to-async promotion.

These questions should be answered with representative case benchmarks rather than assumptions.

---

# 32. Recommended Next Studies

The next evidence-gathering work should include a **Composable Execution Planner benchmark** with at
least:

1. one component → one dataset;
2. ten components → ten independent datasets;
3. ten components → three shared datasets;
4. one dataset → multiple compatible projections;
5. mixed OLTP + analytics composition;
6. 100 concurrent workspaces with cold and warm metadata caches;
7. security scopes that prevent otherwise-similar requests from being coalesced.

Measure at least:

```text
p50 / p95 / p99 latency
queries / request
physical operations / request
rows scanned / request
rows returned / request
CPU / request
memory / request
DB pool utilization
cache hit ratio
planner/compile time
execution DAG width
```

A benchmark should compare a **naive component-per-query baseline** against planner-enabled
shared/batched/bounded execution. A planner optimization is only accepted when it improves or
preserves the relevant SLOs without changing semantics or security.

---

# 33. Capability Admission Gate (Performance Extension)

For any new composable capability, its admission evidence should include an answer to:

```text
What logical dependency does it add?
        ↓
What DAG nodes does it generate?
        ↓
Can those nodes be shared or batched?
        ↓
What is the worst-case physical fan-out?
        ↓
What budget controls prevent unbounded work?
        ↓
What benchmark proves the chosen strategy is acceptable?
```

A capability that is semantically valid but causes uncontrolled physical fan-out is not complete as
an architectural capability until its execution behavior is bounded.

---

# 34. Status of the Planner

The **Composable Execution Planner is PROPOSED architectural infrastructure**, not a claim that the
full planner already exists in the current implementation.

The current runtime already contains pieces of the required foundation: compiled metadata,
execution classes, query normalization concepts, pagination, cache/index direction, and scale
controls. The planner is the architectural boundary that unifies those mechanisms for composable
execution.

Its implementation must follow the repository's existing evidence and admission discipline. No new
capability is implicitly admitted merely by naming this boundary.

---

# 40. Claim Citation Matrix (PROVEN / PROPOSED)

This section is the mechanism named by this document's own changelog (v0.3): every substantive
architectural claim made above, marked **PROVEN** (implemented, cited to a `CAP-` row, conformance
test, or file/line) or **PROPOSED** (an architectural target awaiting the study/case that would
validate it, per `capability-lifecycle.md` §2's A1–A5 admission gate). A PROPOSED row is not
admitted by appearing here. This matrix does not re-derive evidence — it cites
`composable-runtime-blueprint.md` §3 (current-state inventory) and `composable-runtime-roadmap.md`
(the `CR-01`–`CR-28` gap register), which already carry the underlying evidence trail.

| Claim / mechanism | Section | Status | Evidence |
|---|---|---|---|
| Domain model (Machine, Field, Event, Constraint, Permission) | §6 | **PROVEN** | `runtime-metadata-schema.md`; stable core, unchanged by this document |
| View / page composition | §10, §12.4 | **PROVEN** | `CAP-V10` Tier 2, registry + conformance |
| CEL expression evaluation | §9 | **PROVEN** | `CAP-C13` |
| Static Component Registry as non-dynamic-dispatch seam | §14 | **PROVEN** (as a pattern) / **PROPOSED** (as a generic Component contract) | The seam pattern itself is proven by existing closed View-type dispatch (`runtime-metadata-schema.md` `### View Types`); a generic, registry-driven Component contract per §13 is not yet built |
| Metadata compiles to lower-level runtime representations at load time | §17, §26 | **PROVEN** (as a general mechanism) | `internal/metadata/compile.go`, `CAP-W01` — proves the *pattern* of compiling declarative metadata, not the specific Domain/Data/Experience IR this document targets |
| DataSource, Dataset, Dimension, Measure | §7.1–§7.4 | **PROPOSED** | `CAP-V22`/`CAP-V23` (reclassified to new Grammar area `D` — `CR-27`, resolved 2026-09-11; IDs retained unchanged for stability, full rows stay under `capability-registry.md`'s `## Views`, pointer rows added under `## Data`); `CR-03`/`CR-04` |
| Relation, Projection (independent of View) | §7.5–§7.6 | **PROPOSED** | No `CAP-` row exists yet; `capability-registry.md`'s own note calls this an "unadmitted framing"; `CR-04`, `CR-05` |
| Query Model (§8), bare Query/Projection reuse | §8 | **PROPOSED** | No `CAP-` row; `CR-05` |
| Context, Scope, Binding (parent→child propagation) | §11 | **PROPOSED** | No shipped case requires it yet (`approval-dashboard.html` shipped without it); `CR-06` |
| Layout vocabulary beyond `main`/`aside` | §12.2 | 🟡 **PARTIAL** | `CAP-V10` Tier 2 ships the one pairing; stack/grid/columns extensions are `CR-` tracked design triggers, not yet built |
| UI Intermediate Representation | §15 | **PROPOSED** | No forcing case (one server-rendered target only, per `app/ARCHITECTURE.md`); `CR-07` |
| Data IR | §16 | **PROPOSED** | `CR-02` |
| Execution Plan as a runtime-internal artifact | §17 | **PROPOSED** | `CR-11`, `CR-17` |
| Composable Execution Planner (dependency DAG, coalescing, batching, bounded concurrency, cost-based strategy selection) | §18 | **PROPOSED** | `composable-runtime-blueprint.md` §5 Phase 6/7 — "design now, implement when forced"; `CR-11`–`CR-15` |
| CEP → Query Planner boundary | §19 | **PROPOSED** (boundary is documented, not yet enforced by code) | `CR-13` |
| Security-aware dependency identity / coalescing ordering | §18.10, §20 | **PROPOSED** (RLS/permission foundations are `PROVEN`, the planner-level dependency-identity mechanism is not) | `CR-16` |
| Renderer-neutral View Model / Render Input | §23 | **PROPOSED** | Current runtime has exactly one server-rendered target; `CR-25` |
| Composability benchmark harness | §8 (Benchmark program, blueprint) | **PROPOSED** | Benchmark design exists (blueprint §8, this doc §32); no executable planner benchmark yet; `CR-19` |
| Grammar-area classification for Data-plane primitives | — | **DECIDED, not yet implemented** | `CR-27` resolved 2026-09-11 (new `D` area); registry/lifecycle edits pending |
