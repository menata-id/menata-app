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

**Client-side interactivity, whenever a phase touches `internal/rendering`:** HTMX first,
Hyperscript when HTMX's request/response model genuinely isn't enough, vanilla JS only as a
named exception (owner instruction, 2026-09-15). See README.md's "Tech stack" section for the
full statement -- Phase 10's drag-reorder and Phase 14's signature-coordinate editor are the
two places most likely to actually need the Hyperscript/JS tiers, not HTMX alone.

---

## Trial applications (priority target)

Two trial applications are the concrete, owner-prioritized target for this repo (decision,
2026-09-15): **Case 3 (Document Approval)** and **Case 19 (Project Management)**, out of the
larger 21-case portfolio `menata-runtime`'s own discovery phase proved out. See
[`case-portfolio.md`](case-portfolio.md) for all 21 cases and both priority cases' full screen
breakdowns, and [`ui-sample/`](ui-sample/) for their design mockups (copied from that repo,
2026-09-15).

What's already a real slice of one of them: the Task/Project Relation + board Layout (Phases
3-5 below) is genuinely Case 19's Board screen, built because its own forcing conditions
happened to align with that case's shape -- not because a phase was scheduled "for Case 19."
Case 3 is entirely unstarted. Phases are sequenced by architectural forcing condition, per the
Method above; which case supplies that forcing condition next is not predetermined.

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

## Phase 6 -- IR + Dependency Graph + Composable Execution Planner (still not forced)

**Forcing condition:** a single page needs data from more than one Dataset/Query and naive
per-component fetching becomes wasteful or incorrect (e.g. a dashboard combining Task and Project
data). Do not start this phase before that forcing case exists -- 007 §34 explicitly keeps this
PROPOSED, and building it earlier means designing IR/DAG shape from guesses instead of a real
composition to test against.

**Tested for real, 2026-09-15:** built the dashboard this forcing condition names
(`GET /dashboard`, combines Project + Task). Result: **still not forced.** The obvious
implementation fetches each Machine's full record set once (two `store.ListRecords` calls total)
and joins Task -> Project in Go -- that stays O(1) queries regardless of Project count, because
the composition is "two whole datasets joined in memory," not "N per-component queries." There
was no naive-fetch problem to fix. The forcing condition needs a page shape this app doesn't have
yet -- likely N independently-scoped queries (e.g. per-row aggregates, or security-scoped data
that can't just be fetched whole and filtered in Go).

- [ ] Formalize Domain/Data/UI IR (`internal/ir`) -- scoped to what the forcing case above
      actually needs represented, not the full model from 007 §15-16 at once
- [ ] `internal/composition`: derive a real dependency graph from an actual multi-dataset page
- [ ] `internal/planner`: the smallest planner that fixes the specific inefficiency/bug the
      forcing case exposed (dedup, batching, etc. -- whichever one is actually needed first)

**Exit criterion:** the forcing case's specific problem (duplicate queries, or whatever it turns
out to be) is measurably fixed, proven by a before/after comparison, not by the planner's mere
existence.

---

## Phases 7+ -- driven by the trial applications

Phases 1-6 proved the core architecture in the abstract, using Task/Project as convenient toy
Machines. From here, the forcing conditions come directly from **Case 3 (Document Approval)**
and **Case 19 (Project Management)** -- see `case-portfolio.md` for both cases' full screen
breakdowns and `ui-sample/` for their design mockups. Each phase below cites the specific screen
that forces it; the Method above still applies -- build the minimum that screen needs, not the
full generalization a later screen might also want.

---

## Phase 7 -- Real User identity

**Forcing condition:** Case 3's per-step Approver and Case 19's Card Members are currently
impossible to build honestly -- `person`/`fld_assignee` is free-text (`internal/domain`'s
`FieldTypePerson`), so nothing can be assigned to, filtered by "mine," or shown as a real
identity. This is the "second real user" ROADMAP.md's Phase 2 said would force a real model.

- [ ] `mch_user` as an ordinary Machine (name, email) -- reuses existing primitives per 007
      §4.1's admission question, rather than a new system-level concept
- [ ] `FieldTypePerson` becomes a Relation to `mch_user` (reusing Phase 3's Relation machinery),
      not a new field type
- [ ] Login identifies a real `mch_user` record, not only the single shared admin credential
      (`internal/authorization` gains its first real multi-identity code)

**Explicitly deferred:** Group-sourced approvers (Case 3 allows User *or* Group) -- no case
needs Group yet since Case 19 has no Group concept at all; named here so it isn't forgotten, not
built speculatively.

**Exit criterion:** a Task/Card can be assigned to a real `mch_user` record and rendered by that
user's actual name, not a free-text string.

---

## Phase 8 -- Record detail page

**Forcing condition:** Case 19's Task Detail (screen 3) and Case 3's Approval Inbox Detail
(screen 3) both need a dedicated per-record view -- editing is inline-only today (table row /
board card), with nowhere to host embedded sections like a checklist or an activity feed.

- [ ] `GET /machines/{machineID}/records/{id}` -- a real detail page, not just the edit-row
      fragment Phase 1 built
- [ ] Closes the same gap already named in "Operational backlog" below

**Exit criterion:** opening a Task or Project record shows a dedicated page, not just an inline
table/board edit state.

---

## Phase 9 -- Child collections (one-to-many detail sections)

**Forcing condition:** Case 19's Checklist (a Card's own child items) and Case 3's Approval
Steps (a Document's own ordered steps) are both "records that belong to this record," shown
embedded on the parent's detail page -- the reverse direction of the Relation Phase 3 already
built (which only renders the child-side label, never the parent-side list).

- [ ] A generic "child collection" section on the Phase 8 detail page: every record of Machine
      X whose Relation field points at this record, rendered inline
- [ ] Ordering within a child collection (`sort_order`), since both Checklist items and Approval
      Steps are meaningfully ordered

**Exit criterion:** a Project's detail page shows its own Tasks as an embedded list -- proving
the mechanism generically -- before Checklist/Approval Steps reuse it.

---

## Phase 10 -- Many-to-many relations + dynamic ordered Lists

**Forcing condition:** two distinct gaps Case 19's Board screen names directly:

1. **Labels and Members are many-valued** (a Card can have several Labels, several Members) --
   Phase 3's Relation is one value per field, structurally unable to express this.
2. **Board columns are real, user-orderable records** ("Lists": To Do / In Progress / Done are
   data a person can rename and reorder via Board Settings), not the fixed `options:` enum
   Phase 5's board Layout currently groups by.

- [ ] A many-to-many Relation shape (a join collection, not a single field value) -- `internal/domain`,
      `internal/data`, and the `<select multiple>`-equivalent in `internal/rendering`
- [ ] `mch_list` (or equivalent): an ordered, renameable Machine that a board's Layout groups by,
      replacing the fixed status enum for Machines that declare it
- [ ] Card `fld_label`/`fld_member` as many-to-many relations, rendered as colored chips /
      avatars matching `project-board.html`'s own markup

**Exit criterion:** a Task/Card can carry several Labels and several Members at once, and the
board's columns can be renamed/reordered without a metadata schema change.

---

## Phase 11 -- File attachments

**Forcing condition:** Case 3 is impossible without a PDF attachment (the whole case is "a
submitted PDF follows an approval flow"); Case 19's Task Detail also lists attachments.

- [ ] `FieldTypeFile`, stored on local disk (matching 007 §4.10's single-binary/modest-server
      constraint -- no object-storage dependency until a real scale case forces one)
- [ ] Upload via the existing create/edit form flow; download/preview on the Phase 8 detail page

**Exit criterion:** a real PDF can be uploaded to a Document record and downloaded back.

---

## Phase 12 -- Multi-step Action workflow

**Forcing condition:** Case 3's actual mechanism -- a document follows a *sequential or
parallel* approval flow across several steps, each with its own assignee and Approve/Reject
decision -- is genuinely new Behavior, beyond the single-Constraint shape Phase 4 built. This is
the real forcing case for `internal/action`, deliberately left unbuilt until now.

**Design pass required before code** (same discipline as Phases 2 and 4): what's the minimal
Action shape -- does "sequential" mean step N+1's Constraint simply checks step N's decision
field, reusing Phase 4's mechanism, or does it need genuinely new Action semantics? Resolve this
by re-reading `document-submit.html`/`document-approval.html` against Phase 9's child-collection
mechanism before writing Go code, not while writing it.

- [ ] `internal/action`: first real code -- Approve/Reject as an Action that writes a decision
      onto one Approval Step (a Phase 9 child collection) and, for sequential mode, unlocks the
      next step
- [ ] Parallel mode: all steps open at once; the Document only advances once every step has a
      decision

**Exit criterion:** a Document with 3 sequential steps only allows step 2's assignee to act
after step 1 is decided; a parallel-mode Document allows any assignee to act in any order.

---

## Phase 13 -- SLA, activity feed, and composed Dashboard

**Forcing condition:** the three remaining pieces of Case 3's own Approval Inbox and Dashboard
screens, plus Case 19's Project Activity -- and a second, more demanding test of Phase 6's
still-unforced Composable Execution Planner (this Dashboard composes three different aggregate
shapes -- Summary/Pending/Activity -- not two whole-machine lists like the current `/dashboard`).

- [ ] SLA badges: compare a due-date Field against `now`, render OVERDUE / "N day(s) left"
      (`document-approval.html`'s own real markup)
- [ ] An append-only activity/event log per record, and a cross-record feed for the Dashboard's
      Recent Activity section
- [ ] Re-test Phase 6's forcing condition against this specific composition before building any
      IR/planner code -- record the result (forced or not) the same honest way Phase 6's first
      test was recorded above

**Exit criterion:** the Approval Dashboard renders real Summary/Pending/Activity sections from
real data; Phase 6's status is re-evaluated with evidence, not assumed either way.

---

## Phase 14 -- Remaining screens (once their own prerequisites above exist)

Grouped because each is small once Phases 7-13 exist, and none has forced anything new in
`internal/` on its own yet:

- [ ] Case 3: signature coordinate placement (`document-signature-placement.html`) -- a new,
      fairly specialized drag-position editor; no other screen needs this interaction pattern
- [ ] Case 19: My Tasks (a query filtered by "assigned to me," needs Phase 7's real User tied to
      the logged-in identity), Timeline/Calendar (date-range and week-grid Layouts, extending
      Phase 5's LayoutKind set), Sprint Dashboard (aggregation, likely reuses Phase 13's
      activity/count mechanisms), Team Capacity (aggregation over Phase 10's Members),
      Workflow Automation (a read-only view over Phase 12's Action metadata), Board Settings
      (CRUD over Phase 10's `mch_list` ordering and label catalog)

**Exit criterion:** each screen above matches its own `ui-sample/` mockup's real content: no new
architectural mechanism required per screen, only composition of what Phases 7-13 already built.

---

## Operational backlog (tracked, not phase-numbered)

Found during a security/hygiene review (2026-09-15) -- real gaps, but not part of the
architectural forcing-condition sequence above, so tracked separately rather than jammed into a
phase they don't belong to:

- [x] `golang.org/x/text` CVE (GO-2026-5970, infinite loop on invalid input, reachable via
      `pgxpool`) -- bumped to v0.39.0, `govulncheck` confirmed clean
- [ ] Rate-limit `POST /login` -- unlimited password guesses are possible today; the
      constant-time comparison in `internal/authorization` stops timing attacks, not repeated
      brute-force attempts
- [ ] CI workflow (`.github/workflows/`) -- build+vet+test on push; none exists yet despite 60+
      tests already in the repo
- [ ] Sort/filter/search on record lists -- `Store.ListRecords` only orders by `created_at desc`
- [ ] JSON API parity -- `/api/machines/{id}/records` only supports create+list; update/delete
      exist only via the browser HTML routes, not the API

---

## What's deliberately not phased yet

`internal/registry` (static component registry), `internal/execution` as a distinct physical
layer, Events/Services beyond the Action shape Phase 12 builds (006 §Behavioral Model still has
more than that one shape), Group-sourced approvers (named in Phase 7), multi-workspace tenancy
beyond Phase 2's minimal structure, and the full semantic field type vocabulary from 006 -- none
of these have a forcing case yet. Adding any of them before one exists repeats the mistake this
roadmap is written to avoid.

`internal/action` is no longer in this list -- Phase 12 names its real forcing case
(Case 3's sequential/parallel approval). Until Phase 12 actually lands, treat this line as the
plan, not the status; check Phase 12's own checkboxes for what's actually built.
