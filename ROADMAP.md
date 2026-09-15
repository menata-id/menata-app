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

## Phase 3 -- Second Machine (forces generalization)

**Forcing condition:** everything built so far has exactly one Machine ("Task") and may be
accidentally hardcoded to it in ways that look generic but aren't. A second, genuinely different
Machine is the real test.

- [ ] Pick a second Machine that differs meaningfully (different field types, and a Relation to
      Task -- e.g. "Project" that Tasks belong to)
- [ ] Generalize whatever Phase 1-2 code turns out to assume "Task" specifically
- [ ] Dynamic routing (`/machines/{id}`) instead of one hardcoded root page
- [ ] First real use of Relation (006 §Data Model) -- only introduced now because a real
      relationship exists to model, not speculatively

**Exit criterion:** adding a third Machine later requires no code changes, only metadata.

---

## Phase 4 -- Behavior: Events, Actions, Constraints

**Forcing condition:** two Machines with a Relation creates real behavioral questions (e.g., can
a Project be archived while it has open Tasks?) that Domain/Data/Experience alone can't express.

Design pass before code, same reasoning as Phase 2:

- What's the minimal Event/Action/Constraint shape that expresses the *first* real behavioral
  rule this app needs -- not the full behavioral model from 006 §Behavioral Model at once?

Then:

- [ ] `internal/expression`: the bounded expression language, scoped to what Constraints actually
      need (007 §9) -- not a general-purpose scripting language
- [ ] `internal/behavior`, `internal/action`: first real Constraint and Action, tied to the rule
      identified above

**Exit criterion:** at least one real business rule is enforced by the runtime, not by convention.

---

## Phase 5 -- Experience generalization: Layout/Component/View

**Forcing condition:** Phase 3's second Machine likely needs a different presentation than a flat
table (e.g. a board grouped by status) -- the single hardcoded `MachinePage` template won't
express that without duplicating structure.

- [ ] Replace the single template with composable primitives: Page, Layout, Component
      (007 §12) -- only the specific ones needed by the two real presentations that exist by now
- [ ] `internal/experience`, `internal/composition` get their first real code

**Exit criterion:** two Machines render with genuinely different layouts from the same generic
primitives, no per-Machine renderer code.

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
layer, multi-workspace tenancy beyond Phase 2's minimal structure, and the full semantic field
type vocabulary from 006 -- none of these have a forcing case yet. Adding any of them before one
exists repeats the mistake this roadmap is written to avoid.
