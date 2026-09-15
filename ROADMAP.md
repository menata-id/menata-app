# Menata App Roadmap

> Not the same document as root-level concept docs (001-007) -- those describe the target
> architecture. This describes the build order: what gets implemented when, and why.

## Method

Each phase has a **forcing condition**: a real, concrete need that justifies building it now,
not a hypothetical future requirement. This follows 007-composable-runtime-architecture.md §4.1
("Composition over Specialization") and §34, which deliberately marks the Composable Execution
Planner as PROPOSED rather than building it speculatively -- "design now, implement when forced."

It also follows a hard lesson from `menata-runtime`'s own history: an earlier from-scratch
composable rewrite there was abandoned because its own audit found the Behavior plane "was never
designed, not merely unbuilt." Every phase below that touches Behavior/Authorization gets a
design pass before code, not organic accretion.

**Within each phase:** build the minimum that satisfies the forcing condition. Generalize a
primitive only on the second real case that needs it, per 007 §4.1's admission question ("can an
existing primitive express it?") -- never on the first case, and never speculatively for a third
case that doesn't exist yet.

---

## Phase 1 -- Single Machine, full vertical slice (done, 2026-09-15)

**Forcing condition:** prove the metadata -> domain -> data -> render pipeline works end to end
on one real capability before adding any breadth.

- [x] Domain Plane: parse + validate Machine/Field from YAML (`internal/metadata`, `internal/domain`)
- [x] Data Plane: Postgres-backed records, `ValidateRecord` against Field declarations (`internal/data`, `internal/db`)
- [x] Experience Plane: minimal server-rendered page showing schema + records (`internal/rendering`)
- [x] Create records from the browser (an HTML form), not only via `curl`/JSON
- [x] Update and delete a record
- [x] `internal/domain`: add the remaining field types actually needed by a second real field
      (`number`, via `fld_priority`) -- not the full semantic-type list from 006, only what
      Phase 1's own Machine needs

**Exit criterion met (2026-09-15):** a person can open https://menata.app in a browser and
create/edit/delete a Task record via HTMX-driven forms, without touching the API directly.
Verified end-to-end against real Postgres before deploy.

**Phase 1 complete.**

---

## Phase 2 -- Workspace/Application hierarchy + minimal Authorization (done, 2026-09-15)

**Forcing condition:** every write today is anonymous and unscoped -- anyone who can reach the
port can create records. Principle #9 (Workspace Isolation) and §4.9 (Security Before
Optimization) are core, not deferrable, and the longer more surface area gets built on a
zero-auth foundation the more it all has to be retrofitted at once.

**Design pass (resolved before code):**

- Permission scope: per-Machine + per-Action (Create/Update/Delete), not per-Field -- no case has
  forced field-level scoping yet.
- Identity: one shared admin credential (env-configured), no User model -- real multi-user
  identity deferred until a second real user forces it.
- Gating: both reads and writes require login, not just writes -- decided this is a real
  application now, not a public demo.

- [x] Workspace -> Application -> Machine hierarchy in metadata (`internal/domain`, `metadata/app.yaml`,
      `metadata.LoadApplication`) -- one Workspace/Application hardcoded, structure in place for Phase 3
- [x] Session-cookie auth gating every route except `/health` and `/login` (`internal/authorization`)
- [x] `internal/authorization` has its first real code (was a doc.go stub)

**Exit criterion met:** an unauthenticated request is redirected to `/login` (full-page) or gets
`401` (API/HTMX); verified end-to-end (blocked, wrong password, login, authenticated access,
logout) before deploy to https://menata.app.

**Phase 2 complete.**

---

## Phase 3 -- Second Machine (forces generalization) (done, 2026-09-15)

**Forcing condition:** everything built so far has exactly one Machine ("Task") and may be
accidentally hardcoded to it in ways that look generic but aren't. A second, genuinely different
Machine is the real test.

- [x] Second Machine: Project (Name/Status/Owner), with a Relation from Task -> Project
- [x] Generalized every handler to resolve its Machine from the Application manifest instead of
      a single fixed `*domain.Machine`; factored the shared page shell out of per-page duplication
- [x] Dynamic routing: `GET /` lists every Machine, `GET /machines/{id}` replaces the hardcoded
      root page
- [x] First real use of Relation (006 §Data Model) -- shape validation (`internal/metadata`,
      per-Machine and cross-Machine target-exists checks), existence validation
      (`data.ValidateRelations`, store-backed), and a `<select>` input/display label in
      `internal/rendering`

**Exit criterion met:** adding a third Machine now requires only a new `metadata/*.yaml` file plus
one line in `app.yaml`'s `machines:` list -- no code changes. Verified end-to-end (list, create
Project, create Task with valid/invalid relation, label display) against real Postgres before
deploy.

**Phase 3 complete.**

---

## Phase 4 -- Behavior: Constraints (done, 2026-09-15)

**Forcing condition:** two Machines with a Relation creates real behavioral questions (e.g., can
a Project be archived while it has open Tasks?) that Domain/Data/Experience alone can't express.

**Design pass (resolved before code):** the rule shipped is "a Project's status cannot become
`done` while a related Task still references it with a non-`done` status"
(`metadata/project.yaml`'s `cst_project_done_no_open_tasks`). Built exactly the shape that one
rule needs -- a single Comparison operator set (`equals`/`not_equals`) and one Constraint pattern
(block a transition while a related record matches a condition) -- not the full 006 Behavioral
Model (`internal/action`, Events, Services remain unbuilt; no forcing case for them yet).

- [x] `internal/expression`: `Comparison` -- the entire expression vocabulary this rule needs,
      deterministic, no I/O, no parser (007 §9.1)
- [x] `internal/behavior`: `CheckConstraints`, a pure evaluator (the caller fetches related
      records via `internal/data`; behavior performs no I/O itself, fully unit-testable)
- [x] `internal/metadata` validates constraint shape per-Machine and, once the Application is
      loaded, that the related Machine/field actually exist and the related field is genuinely a
      relation pointing back (otherwise it can't identify which records belong to the transition)

**Exit criterion met:** verified end-to-end against real Postgres -- marking a Project `done`
while an open Task references it returns `422` with a clear message; marking it `done` once that
Task is also `done` succeeds (`200`).

**Phase 4 complete.**

---

## Phase 5 -- Experience generalization: table and board Layout (done, 2026-09-15)

**Forcing condition:** Phase 3's second Machine likely needs a different presentation than a flat
table (e.g. a board grouped by status) -- the single hardcoded `MachinePage` template won't
express that without duplicating structure. Confirmed real: Task naturally wants a status board,
Project stays a flat list.

- [x] Two generic Layouts -- table (default) and board -- declared per-Machine via
      `view: {layout, group_by}` metadata, not per-Machine renderer code
- [x] `internal/experience` gets its first real code (`GroupRecords`, pure/unit-tested)
- [x] `internal/composition` deliberately left untouched -- no multi-dataset/dependency-sharing
      case exists yet to force the dependency-graph machinery it's meant for (007 §34's own
      "design now, implement when forced" applied here too, not only to the CEP)

**Exit criterion met:** Task renders as a 3-column board (todo/in_progress/done), Project renders
flat -- both from the same `tableLayout`/`RecordRow` primitives, board columns are literally that
same table reused per group (006 §View: "Board = Collection + Group Dimension + Board renderer").
Create/edit/delete verified working identically on both layouts.

---

## Phase 6 -- IR + Dependency Graph + Composable Execution Planner (only when forced)

**Forcing condition:** a single page needs data from more than one Dataset/Query and naive
per-component fetching becomes wasteful or incorrect (e.g. a dashboard combining Task and Project
data). Do not start this phase before that forcing case exists -- 007 §34 explicitly keeps this
PROPOSED, and building it earlier means designing IR/DAG shape from guesses instead of a real
composition to test against.

- [ ] Formalize Domain/Data/UI IR (`internal/ir`) -- scoped to what the forcing case above
      actually needs represented, not the full model from 007 §15-16 at once
- [ ] `internal/composition`: derive a real dependency graph from an actual multi-dataset page
- [ ] `internal/planner`: the smallest planner that fixes the specific inefficiency/bug the
      forcing case exposed (dedup, batching, etc. -- whichever one is actually needed first)

**Exit criterion:** the forcing case's specific problem (duplicate queries, or whatever it turns
out to be) is measurably fixed, proven by a before/after comparison, not by the planner's mere
existence.

---

## What's deliberately not phased yet

`internal/registry` (static component registry), `internal/execution` as a distinct physical
layer, `internal/action` and Events/Services (006 §Behavioral Model beyond the one Constraint
shape Phase 4 built), multi-workspace tenancy beyond Phase 2's minimal structure, and the full
semantic field type vocabulary from 006 -- none of these have a forcing case yet. Adding any of
them before one exists repeats the mistake this roadmap is written to avoid.
