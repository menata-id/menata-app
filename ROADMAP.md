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

### Correction (2026-09-18, owner-prompted): forcing conditions must be constructed, not awaited

The method above silently assumes a system that is *in use*. It is not. menata-app has no users,
one shared credential and one Application -- and nearly every trigger this document defers behind
is production-shaped:

> "the first metadata author who is not the engineer who wrote the inference" · "the first real
> destructive metadata change on data that matters" · "a second Application in the manifest" ·
> "the first Machine whose record count makes a page slow" · "the first real multi-tenant case" ·
> "metadata authored by someone who cannot deploy" · "a metadata author, not an engineer, needs
> to compose a new dashboard-shaped page"

None of those events can occur before deployment. So the method, applied literally, defers them
forever and reports progress while the composable architecture stands still -- which is exactly
what happened: Phase 6 was tested four times, each time concluding "not forced," while the
deferred list only grew. **That is not evidence the architecture is sound. It is evidence that
nothing has been exercised.** In a system nobody uses, the absence of a forcing condition carries
no information at all.

**The correction:** a forcing condition that cannot arise naturally before production must be
*manufactured* during development, while manufacturing it is still cheap -- synthetic record
volume, synthetic metadata breadth, a second Application, a second identity, a deliberately
authored page. Then the architecture is measured against it.

This does **not** relax "never build speculatively." The rule still is: build only what a
measurement forces. What changes is where the measurement comes from -- produced on purpose
instead of waited for. Writing the *test* that reveals whether a mechanism is needed is never
speculative; writing the *mechanism* before that test is. Phase 6's fourth pass is the template:
it measured, found real duplication, and forced exactly one small thing (dedup) while leaving
IR and the dependency graph deferred on the same evidence.

**Rule from here on:** a phase may stay deferred only when its trigger is either (a) constructible
today, and has been constructed and measured negative -- with the numbers recorded, as Phase 6
does -- or (b) genuinely impossible to construct before real use, in which case the entry must say
so explicitly and name what deployment event would produce it. "No forcing case yet" is no longer
an acceptable entry on its own.

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
Case 3 is no longer unstarted (Phases 12-13 built `mch_document`/`mch_approval_step`, the
`/decide` Action, SLA badges, and Activity logging) but its own 4 screens are not all built yet
-- see Phase 15 below for the remainder, following a 2026-09-15 research pass. Phases are
sequenced by architectural forcing condition, per the Method above; which case supplies that
forcing condition next is not predetermined.

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

## Phase 6 -- IR + Dependency Graph + Composable Execution Planner (dedup now forced; IR/DAG still not)

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

**Re-tested 2026-09-18 (fourth time), after Phase 16, as the 2026-09-18 audit instructed. First
non-negative result -- but on the composition axis, not the security one the audit predicted.**

*Method:* a temporary probe on `Store`'s three read entry points (`ListRecords`/`ListRecordsBy`
via `queryRecords`, plus `GetRecord`), real Postgres, session-authenticated `GET`s to all 14
composed and detail routes. The probe was reverted after measuring; raw counts below are what the
running server actually issued, not a static reading of the handlers.

*Security-scoped half: still negative.* Phase 16's Permission gates one write Action
(`/decide`); it narrowed no read path. `/approval-inbox` still issues 4 whole-Machine reads and
filters by identity in Go, exactly as "Concept conformance gaps" describes. The audit's
expectation that Phase 16 would create this forcing condition was wrong: per-Machine visibility
cannot discriminate while one shared credential means every session is the same person. The real
trigger is **per-user login** (still deferred, Phase 7), not Permission.

*Composition half: positive, for the first time.* Three pages issue duplicate whole-Machine
fetches within a single request:

| Route | Queries | Redundant | What repeats |
|---|---|---|---|
| `/machines/mch_task` (board) | 5 | 1 | `mch_list` x2 -- `loadRelationOptions` (`fld_list`) and `loadBoardColumns` (`group_by`) each fetch it |
| `/machines/mch_project/records/{id}` | 6 | 1 | `mch_user` x2 -- the parent's own relation options (`fld_owner`) and the `mch_task` child section's (`fld_assignee`) |
| `/machines/mch_user/records/{id}` | 12 | 3 (25%) | `mch_user` x4 -- one per child section (`mch_task`, `mch_project`, `mch_approval_step`, `mch_activity`) |

Every other route measured clean: `/dashboard` 5, `/approval-inbox` 4, `/sprint` 3, `/my-tasks`
`/activity` `/calendar` `/team-capacity` `/board-settings` 2 each, all with zero duplication --
the "two whole datasets joined in memory" shape the first three tests found, still correct.

*Cause:* `loadRelationOptions` deduplicates *within* one Machine (its own `options` map) but each
call starts a fresh map; `loadChildSections` calls it once per child section, and
`loadBoardColumns` does not consult it at all. Nothing in the request shares a fetch across those
call sites.

*Why this counts, despite small absolute numbers:* the duplication scales with the **number of
Machines that reference the target**, i.e. with metadata size, not record count. Every future
Machine carrying a `person` field adds one more full `mch_user` fetch to `mch_user`'s own detail
page -- 007 §28 invariant #2 (no unbounded fan-out) names exactly this. The first three tests
looked for waste that grows with *data* and correctly found none; this one grows with *schema*.

**Verdict:** deduplication of shared dependencies -- the CEP's own first named job (007 §18,
`internal/planner/doc.go`) -- is forced. The IR and the dependency graph are **not**: per the
Method's "build the minimum," and §4.1's admission question, a request-scoped memo of
`ListRecords` closes all three rows above without any IR or DAG. Build that; the graph earns its
place when dedup alone stops being enough (batching, concurrency bounds, or a case where the
right fetch depends on another fetch's result -- none exists today).

- [x] Request-scoped read memoization: one shared cache per request, keyed by the read's own
      arguments (`composition.Loader`, Phase 18 Step 2). Smallest thing that fixes the measured
      problem; lives with `Store`'s callers, not in `internal/planner`, until a second planner
      concern appears
- [ ] Deferred until a second concern exists: Domain/Data/UI IR (`internal/ir`),
      `internal/composition`'s dependency graph, `internal/planner` proper

**Exit criterion met, 2026-09-18.** Measured with the now-permanent query diagnostic (Phase 18
Step 3), comparing the same binary built with the memo disabled:

| Route | Before | After |
|---|---|---|
| `/machines/mch_task` | 5 reads, 1 repeated | **4 reads, 0 repeated** |
| `/machines/mch_project/records/{id}` | 6 reads, 1 repeated | **5 reads, 0 repeated** |
| `/machines/mch_user/records/{id}` | 12 reads, 3 repeated | **9 reads, 0 repeated** |

The other 11 routes were unchanged, and all 14 routes' rendered HTML is byte-identical between
the two builds.

**Found by that HTML comparison, and fixed:** child sections rendered in a *different order on
every request*. `machineSlice` iterated a Go map, whose iteration order is randomized, so a
record's detail sections shuffled on reload. The defect predated this work (`main.go`'s own
`machineSlice` had it), was invisible to every previous test because none compared rendered
output, and is now sorted and locked by `TestMachineSliceIsStable`. Worth noting as evidence for
the exit criterion's own shape: "identical HTML" earns its place precisely by catching things the
query counts never would.

**Volume threshold, measured 2026-09-18 (`make threshold`, Phase 18 Step 4).** Phase 6 has asked
since 2026-09-15 what record count makes whole-Machine reads stop working, and until now answered
"unknown, no case has hit it." Constructed deliberately rather than waited for:

| Records in one Machine | Whole-Machine read (avg of 3) | |
|---|---|---|
| 100 | 0.7ms | fine |
| 1,000 | 5ms | fine |
| 10,000 | 44.8ms | fine, approaching the budget |
| 50,000 | 217.2ms | **over the 100ms interactive budget** |
| 100,000 | 545ms | over |

So the threshold sits between **10,000 and 50,000 records in a single Machine**. Below it, this
app's "fetch the whole Machine and reduce it in Go" shape is correct and the planner is genuinely
unnecessary. Above it, projection/filter pushdown and pagination are forced (007 §21.1, §28
invariants #2 and #4) -- and that, not more composed pages, is what finally gives the dependency
graph something to order, since only then must a fetch's arguments come from another fetch's
result. **Pagination lands before the planner, and neither lands before 10k rows.** "Not forced"
is now a number rather than an assumption.

**Breadth claim: confirmed, then closed.** Phase 6's fourth test predicted the duplication grows
with the number of Machines referencing a target rather than with record count -- inferred from
9 Machines. `TestBreadthThreshold` constructs 1, 4, 16 and 64 referring Machines: without the memo
that is one read per referrer, with it exactly one read at every breadth. The prediction was
right, and the growth is now closed and locked by a test rather than argued about.

---

## Phases 7+ -- driven by the trial applications

Phases 1-6 proved the core architecture in the abstract, using Task/Project as convenient toy
Machines. From here, the forcing conditions come directly from **Case 3 (Document Approval)**
and **Case 19 (Project Management)** -- see `case-portfolio.md` for both cases' full screen
breakdowns and `ui-sample/` for their design mockups. Each phase below cites the specific screen
that forces it; the Method above still applies -- build the minimum that screen needs, not the
full generalization a later screen might also want.

---

## Phase 7 -- Real User identity (done, 2026-09-15)

**Forcing condition:** Case 3's per-step Approver and Case 19's Card Members are currently
impossible to build honestly -- `person`/`fld_assignee` is free-text (`internal/domain`'s
`FieldTypePerson`), so nothing can be assigned to, filtered by "mine," or shown as a real
identity. This is the "second real user" ROADMAP.md's Phase 2 said would force a real model.

- [x] `mch_user` as an ordinary Machine (name, email) -- reuses existing primitives per 007
      §4.1's admission question, rather than a new system-level concept
- [x] `FieldTypePerson` becomes a Relation to `mch_user` (reusing Phase 3's Relation machinery,
      via the new `Field.IsReference()` -- true for Relation and Person alike), not a new field type
- [x] Login identifies a real `mch_user` record, not only the single shared admin credential --
      session cookies now sign the actual subject (`config.AdminUserID`) instead of a hardcoded
      "admin" literal (`internal/authorization` gains its first real multi-identity code)

**Explicitly deferred:** Group-sourced approvers (Case 3 allows User *or* Group) -- no case
needs Group yet since Case 19 has no Group concept at all; named here so it isn't forgotten, not
built speculatively. Per-user passwords are also still deferred -- the shared admin credential
now *resolves to* a real `mch_user` record (bootstrap: log in, create the record, set
`ADMIN_USER_ID`), but logging in is not yet per-user.

**Exit criterion met:** a Task can be assigned to a real `mch_user` record via a `<select>`
populated with actual users, rendered by that user's name, not a free-text string; assigning a
nonexistent user id is rejected (`422`). Verified end-to-end against real Postgres before deploy.

**Phase 7 complete.**

---

## Phase 8 -- Record detail page (done, 2026-09-15)

**Forcing condition:** Case 19's Task Detail (screen 3) and Case 3's Approval Inbox Detail
(screen 3) both need a dedicated per-record view -- editing is inline-only today (table row /
board card), with nowhere to host embedded sections like a checklist or an activity feed.

- [x] `GET /machines/{machineID}/records/{id}` -- a real detail page (`rendering.RecordDetailPage`),
      not just the edit-row fragment Phase 1 built. One route serves three renderings depending
      on the request (direct navigation vs. HTMX targeting the detail container vs. HTMX
      targeting a table/board row) -- `internal/rendering/detail.templ`, `cmd/server/main.go`'s
      `isDetailContext`
- [x] Closes the same gap already named in "Operational backlog" below

**Exit criterion met:** opening a Task or Project record shows a dedicated page (`dl.detail-
fields` label/value layout, Edit/Delete actions), not just an inline table/board edit state.
Every Layout's `RecordRow` now links its first field to this page for free. Verified end-to-end:
direct navigation, edit/update/delete from the detail context (delete redirects back to the
Machine's list/board via `HX-Redirect`), and the pre-existing table/board inline-edit flows
confirmed unchanged.

**Phase 8 complete.**

---

## Phase 9 -- Child collections (one-to-many detail sections) (done, 2026-09-15)

**Forcing condition:** Case 19's Checklist (a Card's own child items) and Case 3's Approval
Steps (a Document's own ordered steps) are both "records that belong to this record," shown
embedded on the parent's detail page -- the reverse direction of the Relation Phase 3 already
built (which only renders the child-side label, never the parent-side list).

- [x] A generic "child collection" section on the Phase 8 detail page: every record of Machine X
      whose reference field points at this record, rendered inline (`domain.FindChildCollections`,
      reused verbatim for Relation and Person -- a User's detail page gets "Tasks assigned to me"
      for free, with zero extra code)
- [x] Ordering within a child collection (`sort_order`, `migrations/002_sort_order.sql`) --
      `Store.CreateRecord` assigns the next value per Machine; `ListRecords`/`ListRecordsBy` both
      order by it, replacing the old newest-first default everywhere, not only in child collections

**Exit criterion met:** a Project's detail page shows its own Tasks as an embedded list, reusing
`RecordRow` per child row with no per-Machine rendering code. Verified end-to-end against real
Postgres, including the Person-based bonus case (User → assigned Tasks).

**Phase 9 complete.**

---

## Phase 10 -- Many-to-many relations + dynamic ordered Lists (done, 2026-09-15)

**Forcing condition:** two distinct gaps Case 19's Board screen names directly:

1. **Labels and Members are many-valued** (a Card can have several Labels, several Members) --
   Phase 3's Relation is one value per field, structurally unable to express this.
2. **Board columns are real, user-orderable records** ("Lists": To Do / In Progress / Done are
   data a person can rename and reorder via Board Settings), not the fixed `options:` enum
   Phase 5's board Layout currently groups by.

- [x] A many-to-many Relation shape (a join collection, not a single field value) --
      `metadata/card_label.yaml` (`mch_card_label`: `fld_task` + `fld_label`, both ordinary
      Relation fields). Composed entirely from Phase 3's Relation and Phase 9's Child Collection;
      zero new engine code was needed -- it shows up as a real child collection on both `mch_task`'s
      and `mch_label`'s own detail pages, in both directions
- [x] `mch_list`: an ordered, renameable Machine that Task's board Layout now groups by
      (`fld_list`), replacing the fixed status enum. `experience.GroupRecords` gained a `columns`
      parameter (nil keeps Phase 5's original Options-based behavior; the caller's pre-fetched
      relation-target records drive the new case) -- `cmd/server`'s `loadBoardColumns` resolves them
- [ ] Card `fld_label`/`fld_member` rendered as colored chips/avatars matching `project-board.html`'s
      own markup -- **deliberately deferred to Phase 14**, this phase proves the mechanism (a
      Task really can carry several Labels) and the dynamic Lists work, not the final visual
      polish of showing them inline on the card face

**Exit criterion met:** a Task carries several Labels via the join Machine (proven both
directions); the board's columns are renamed (`To Do` -> `Backlog`) with the change appearing
immediately, zero metadata/code deploy. Verified end-to-end against real Postgres.

`domain.Machine.FieldByID` added, replacing two identical private duplicates in
`internal/experience` and `internal/metadata` (Reference over Duplication).

**Phase 10 complete.**

---

## Phase 11 -- File attachments (done, 2026-09-15)

**Forcing condition:** Case 3 is impossible without a PDF attachment (the whole case is "a
submitted PDF follows an approval flow"); Case 19's Task Detail also lists attachments.

- [x] `FieldTypeFile`, stored on local disk (`internal/storage`, matching 007 §4.10's
      single-binary/modest-server constraint -- no object-storage dependency until a real scale
      case forces one). Original filename survives inside the storage key itself
      (`<random>__<filename>`), no second field needed to carry it
- [x] Upload via the existing create/edit form flow (`hx-encoding="multipart/form-data"` on every
      form now, `ParseMultipartForm` universally); download on the Phase 8 detail page and every
      table/board row (`GET /uploads/*`, `Content-Disposition` names the original filename)

A real gap found and fixed during this phase, not merely anticipated: a browser can't pre-fill
`<input type="file">`, so an edit that didn't touch the file input was silently dropping the
attachment (`Store.UpdateRecord` replaces the whole JSONB value) -- `updateRecordForm` now
explicitly carries the existing file value forward when a fresh upload isn't present.

**Exit criterion met:** a real PDF was uploaded to a Task record and downloaded back
byte-identical, with the correct `Content-Type` and original filename; an edit that didn't
re-upload correctly preserved the existing attachment. Verified against real Postgres and local
disk before deploy.

---

## Phase 12 -- Multi-step Action workflow (done, 2026-09-15)

**Forcing condition:** Case 3's actual mechanism -- a document follows a *sequential or
parallel* approval flow across several steps, each with its own assignee and Approve/Reject
decision -- is genuinely new Behavior, beyond the single-Constraint shape Phase 4 built. This is
the real forcing case for `internal/action`, deliberately left unbuilt until now.

**Design pass (resolved before code):** does "sequential" reduce to Phase 4's single-hop
Constraint, or does it need genuinely new Action semantics? **Resolves to the latter.**
Constraint can only check another Machine's records pointing at this one; sequencing needs to
compare ordinal position among sibling records of the *same* Machine (other Approval Steps of
the same Document) -- a shape Constraint cannot express. `internal/action` is hardcoded to
`mch_document`/`mch_approval_step`'s own field ids, not a generic metadata-driven engine, per
this roadmap's own Method -- generalize on a second real case that needs something similar,
never the first.

- [x] `internal/action`: first real code -- `CanDecide`/`DocumentStatus`, pure functions over
      already-fetched records (same posture as `behavior.CheckConstraints`). `POST
      /machines/{id}/records/{id}/decide` is the one real Action endpoint: enforces `CanDecide`,
      writes the decision onto one Approval Step (a Phase 9 child collection), recomputes and
      saves the Document's aggregate status
- [x] Parallel mode: `CanDecide` never locks a step when `fld_mode != "sequential"` -- any
      assignee can act in any order; the Document still only reaches `approved` once every step
      is `approved` (`rejected` wins immediately if any step is)
- [x] The generic edit route was a live bypass of the sequencing rule until this phase closed
      it: `updateRecordForm` now rejects any attempt to change `fld_decision` outside `/decide`

**Real regression found and fixed the same day:** Phase 11's `ParseMultipartForm` switch
rejected plain url-encoded bodies outright (`http.ErrNotMultipart`), even though
`ParseMultipartForm` already runs `ParseForm` first regardless -- a new shared `parseRecordForm`
helper fixes this for both `createRecordForm` and `updateRecordForm`.

**Exit criterion met:** verified end-to-end against real Postgres -- a 3-step sequential Document
blocks step 2 until step 1 is decided (`422`), correctly unlocks after, and reaches `approved`
once all three are; a direct generic-edit bypass attempt on a pending step is blocked (`422`); a
parallel-mode Document allows step 2 to be decided before step 1 (`303` success). A real Case 3
demo (one Document, a real PDF, two Approval Steps, one already approved) is seeded live.

---

## Phase 13 -- SLA, activity feed, and composed Dashboard (done, 2026-09-15)

**Forcing condition:** the three remaining pieces of Case 3's own Approval Inbox and Dashboard
screens, plus Case 19's Project Activity -- and a second, more demanding test of Phase 6's
still-unforced Composable Execution Planner (this Dashboard composes three different aggregate
shapes -- Summary/Pending/Activity -- not two whole-machine lists like the current `/dashboard`).

- [x] SLA badges: `internal/experience.EvaluateSLA` compares a `view.sla_field` date Field
      against `now` (day-truncated), rendered as `slaBadge` in both `RecordRow` and
      `RecordDetailView` -- OVERDUE / "Due today" / "N day(s) left". Metadata-declared
      (`document.yaml`'s `view.sla_field: fld_due_date`), not hardcoded to one Machine, since it's
      a genuinely reusable concept unlike Phase 12's Action
- [x] `mch_activity`: an ordinary Machine (`metadata/activity.yaml`), not a new "system data
      source" concept -- 007 §4.1's admission question says an existing primitive already
      expresses this. `logActivity` (`main.go`) appends one record on Document submission
      (`createRecordForm`) and on each step decision (`decideStep`), resolving the acting user via
      `authorization.CurrentUserID`
- [x] `/dashboard` composes four sections now: Projects (Phase 6's original case), Documents
      Summary (status counts), Pending Approval (in_review Documents with SLA badges), and Recent
      Activity (newest 10 events, actor names resolved)

**Phase 6 re-tested, 2026-09-15 (second time): still not forced.** Four sections, five
`store.ListRecords` whole-machine calls (projects, tasks, documents, activity, users), joined/
filtered/sorted in Go (`sort.Slice` on `CreatedAt` for the feed, a `switch` over status for the
summary). Still O(1) queries regardless of record counts -- no per-row or per-section query loop
appeared even with twice as many composed sections as the first test. The forcing condition still
needs a page shape this app doesn't have: N independently-scoped queries, not N aggregate views
over already-whole-fetched data.

**Exit criterion met:** verified against real Postgres on a scratch server -- a Document with a
past due date renders `OVERDUE`, a future due date renders "N days left"; submitting a Document
and deciding an Approval Step each produce a real Activity record, newest-first on the Dashboard;
all four Dashboard sections render real data. Test records and one real decision made during
verification were reverted afterward so the live "Vendor Contract 2026" demo data is unchanged.
One known gap carried over from Phase 8, not new here: the shared admin credential resolves to
`config.AdminUserID` ("admin"), not a real `mch_user` record id, so Activity entries logged in via
the admin login show no actor name -- real per-user login is still a later, separate phase.

---

## Phase 14 -- Remaining screens (once their own prerequisites above exist)

Grouped because each is small once Phases 7-13 exist, and none has forced anything new in
`internal/` on its own yet. Landing incrementally, one screen per commit, rather than as one
single Phase 14 change -- each screen's own composition is independently verifiable.

- [x] Case 19: **My Tasks** (`project-my-tasks.html`, done 2026-09-15) -- `GET /my-tasks`, every
      `mch_task` filtered to `authorization.CurrentUserID` ("assigned to me" -- Phase 8's identity
      resolution, not new per-user login), bucketed Today/Upcoming/Completed by reusing Phase 13's
      `experience.EvaluateSLA` rather than re-deriving day-truncation. Verified live: real
      assignee filtering and OVERDUE/"Due today" bucketing confirmed with a scratch
      `ADMIN_USER_ID` override against real Task data, then fully reverted
- [x] Case 19: **Board Settings** (`project-settings.html`, done 2026-09-15) -- `GET
      /board-settings` previews `mch_list`/`mch_label` (already ordinary Machines) and links to
      their own already-built generic Machine pages for the actual CRUD; no new settings
      mechanism, pure composition
- [x] Case 19: **Project Activity** (`project-activity.html`, done 2026-09-15) -- `GET /activity`
      groups Phase 13's `mch_activity` data by day (Today/Yesterday/Earlier) instead of the
      Dashboard's flat top-10; `logActivity` coverage widened from Document/Approval-Step-only to
      `mch_task`/`mch_project` creation and Task status moves ("completed" when the new status is
      `done`). Verified live: created/moved/completed events all logged and grouped correctly,
      test records fully reverted afterward
- [x] Case 19: **Team Capacity** (`project-team.html`, done 2026-09-15) -- `GET /team-capacity`,
      every `mch_user`'s declared weekly capacity (`fld_weekly_capacity`, a new Number field on an
      existing Machine -- using an already-declared Field Type on a new field is not a new
      mechanism) alongside real active/total `mch_task` counts. Deliberately has no per-task hour
      estimates: the mockup's "Assigned hours" column would need data this app doesn't have yet
      and no case has forced adding it
- [x] Case 19: **Workflow Automation** (`project-automation.html`, done 2026-09-15) -- `GET
      /automation` describes this Application's real Constraint metadata and Action behavior as
      Trigger/Condition/Action, not a generic automation engine (no case has forced one) and not
      fictional example workflows like the mockup's own placeholder content
- [x] Case 19: **Calendar** (`project-calendar.html`, done 2026-09-15) -- `GET /calendar`, the
      current Monday-Sunday week as one column per day, populated by `mch_task.fld_due_date` and
      reusing the existing board-column CSS shape. No new Field or Layout mechanism needed after
      all -- `fld_due_date` already existed, this is a different grouping/rendering shape over it,
      same posture as My Tasks' bucketing
- [x] Case 19: **Sprint Dashboard** (`project-dashboard.html`, done 2026-09-15) -- `GET /sprint`
      composes a real Task-status summary (open/in-progress/done/completion %), a workload
      preview reusing Team Capacity's `MemberCapacity`, and an Attention Needed list reusing My
      Tasks' SLA bucketing. The mockup's own points/burndown/blocked content is deliberately not
      reproduced: no story-points Field, no historical snapshot data a real burndown needs, and no
      "blocked" status option exist -- fabricating any of them would misrepresent real data, which
      this app avoids everywhere else too
- [ ] Case 19: Timeline (date-range bars grouped by workstream) -- needs a Task start-date Field
      that doesn't exist yet, plus proportional-width bar rendering; the one screen in this phase
      that still plausibly needs a real new rendering shape, not just composition

**Exit criterion:** each screen above matches its own `ui-sample/` mockup's real content: no new
architectural mechanism required per screen, only composition of what Phases 7-13 already built.

Case 3's own remaining screens (previously a single bullet here) are broken out into Phase 15
below, following a dedicated research pass -- they turned out to need more than one line.

---

## Phase 15 -- Case 3 completion: composable UI component research pass + PoC

**Forcing condition:** Case 3 is a priority trial application (see "Trial applications" above)
with 4 real screens in `ui-sample/`; only its backend half (Document/Approval Step Machines, the
`/decide` Action, SLA, Activity) exists so far. Before writing more UI code for it, the owner
asked for a research pass: re-study 001-007's UI/Experience-plane guidance, benchmark against
`menata-runtime`'s own prior composable-UI-component research, gap-check against applicable
world-class practice (filtered for this stack -- server-rendered HTML + HTMX + Hyperscript, not
React/SPA), and propose architecture changes if warranted, before building the PoC.

**Research verdict (2026-09-15):** the gap is discipline, not a missing mechanism. 007
(`007-composable-runtime-architecture.md` §1, §40) marks nearly everything on the UI/Experience
side (Dataset/Projection §7, Context/Scope/Binding §11, UI IR §15, the Composable Execution
Planner §18) as PROPOSED, not built -- the concept docs give component vocabulary (Page/Layout/
Component/Slot, §12-13) and admission principles (§4.1 Composition over Specialization, §24's
Specialization Rule), not a component catalog. `menata-runtime/benchmarks/029` (component
inventory method), `030` (promotion criteria P1-P5, esp. P3 domain-specific-data-stays-
product-specific and P4 metadata-invocation-reachability), `032` (Case 19's own page/component
matrix, the format this phase's own matrix below follows), and `024` (Case 3's own prior
capability study -- CAP-F22 PDF compositing, CAP-V20 decision stepper, CAP-V21 coordinate editor,
percentage-based coordinates needing no new Field type) are the load-bearing prior art. templ
already has the composition mechanism this needs (`templ.Component` params, `{ children... }`
slots, already used once in `pageShell`) -- it's just under-applied: `<ul class="activity-feed">`
is hand-rendered independently in `dashboard.templ`, `activity.templ`, `sprintdashboard.templ`,
`mytasks.templ` (4 copies), and `<ul class="summary-counts">` in 4 more places, both past the
P1 cross-domain-recurrence bar and not yet consolidated -- `slaBadge` (Phase 13) is the proof
this codebase's own composition mechanism already works, just not generalized to these two.

**Adopted -- Step 0 (do first, unblocks the rest):** promote `summaryCounts`/`activityFeedList`/
`sectionHeader` to real shared `templ` components (in `machine.templ`, alongside `slaBadge`);
migrate the ~8 existing hand-rendered call sites to call them; verify identical rendered HTML
before/after. Pure refactor, zero new capability, small and self-contained.

**Correction (2026-09-15, same day, owner-prompted):** the first version of this section read as
"not needed yet, don't even plan it," which overstates what PROPOSED means. 007's own header says
a PROPOSED claim is "an architectural target awaiting the study or case that would validate it,"
not an undesigned one -- and `menata-runtime/benchmarks/030-ui-subcomponent-decomposition-
criteria.md` (Study 40) shows the target schema for several of these mechanisms is not merely
architecturally proposed, it is **already PROVEN, working, conformance-cited code in `prototype/
go`**: `type: dashboard` + `sections` (CAP-V10), `display: cards` (CAP-V02), `type:
decision_stepper` (CAP-V20), `type: coord_placement` (CAP-V21) all shipped there with real schema
keys before that codebase was frozen. menata-app deliberately did not port that code (owner
decision, start `app/` from scratch) -- but not porting the code doesn't mean discarding the
knowledge of what shape already worked. The corrected posture below documents that target shape
now, informed by real proven prior art and this app's own known case portfolio (not guessed), and
separates *documenting the target* from *building the generalized mechanism*, which still waits
for its own real second occurrence -- exactly Study 40's own P4/P5 distinction, not a blanket
"not yet" applied to everything alike.

**Two different questions, previously conflated:**
1. **Composable Execution Planner / IR / dependency graph (007 §15-18)** -- a *query-efficiency*
   question: does hand-composing a page in Go produce a naive-fetch problem? Phase 6 has tested
   this twice, both negative. **Still correctly not forced** -- nothing in this research pass
   changes that verdict, it answers a different question. (Superseded 2026-09-18 by Phase 6's
   fourth test, which found real duplicate fetches on three routes: the *dedup* part of the CEP
   is now forced, the IR/DAG part still is not. See Phase 6 for the measurements.)
2. **Metadata-driven page/View composition (`type: dashboard`'s own `children`/`sections`
   mechanism, CAP-V10)** -- an *authoring* question: can someone declare "compose these Views
   into a page" from YAML, without writing Go? menata-app has never actually tested this axis --
   every composed page here (`/dashboard`, `/sprint`, `/activity`...) is hand-written Go+templ,
   same as Phase 6 found cheap and correct, but for a *different* reason (no metadata author has
   needed to compose one without touching code). `prototype/go` already answered this axis real,
   with a real capability id and Tier 2 conformance backing -- this is closer to forced than
   axis 1, just not by a menata-app-native case yet.

**Target schema (documented now, converge when a real second occurrence appears -- not built
speculatively for Case 3 alone):**

| Target mechanism | Proven prior art | menata-app's Case 3 instance (build now, as one component) | Converges when |
|---|---|---|---|
| `display: cards` alternate collection rendering | CAP-V02, `runtime-metadata-schema.md`'s `vw_list_cards` | `recordSummaryCard` (Phase 15 Step 1) -- Case 3's Approval Worklist is the *first* menata-app occurrence, P1 not yet cleared | A second Machine (Case 19's own Board card face is the likely next one) needs the identical shape -- reuse this schema key, don't invent a second one |
| `type: dashboard` + `sections`/`children` page composition | CAP-V10, `internal/ui/dashboard.templ` | Hand-written `showDashboard`/`showSprintDashboard` etc. (Phase 13-14), same posture Phase 6 already found cheap | A metadata author, not an engineer, needs to compose a new dashboard-shaped page without a Go change -- the real P4 test, not "a third page exists" |
| `type: decision_stepper` | CAP-V20 | `approvalStepper` (Phase 15 Step 2), one instance | A second sequential/staged-decision Machine appears (none in Case 19 today) |
| `type: coord_placement` | CAP-V21 | Signature placement (Phase 15 Step 4), one instance, vanilla-JS exception | A second coordinate-on-an-image interaction appears (none named in any of the 21 cases yet) |
| Activity Feed sourced from `record_events` | **Genuinely unresolved even in `prototype/go`** -- Study 40's own Cluster 6 is marked ❌ "no real metadata mechanism, faked with a static placeholder" | `mch_activity` as an ordinary Machine (Phase 13) -- **menata-app's own design already differs from, and arguably improves on, prototype/go's unresolved approach**, by treating activity as ordinary data instead of inventing a bespoke ViewType | Already effectively converged -- no further action, worth noting as a real improvement, not a gap |

**Still rejected, with reasoning (this part of the original verdict stands):**
- A generic Component Registry (007 §14) -- ~15 hand-wired templ page functions today is not the
  dispatch-sprawl problem §14 exists to fix; no case names a need for *dynamic* View-type
  selection at runtime, only for more View types to exist, which is a metadata/schema question
  (the table above), not a registry question.
- Generic React-style component-library patterns (hooks, client state) -- inapplicable to a
  server-rendered, minimize-JS stack; explicitly rejected rather than silently ignored.
- A generic "Choice Card" primitive -- single-occurrence evidence (Study 38's own Cluster 10
  verdict); build the one real instance Case 3 needs, not a primitive.

**Case 3 UI page/component matrix** (format follows `benchmarks/032`; "Reuse" names the actual
existing menata-app symbol):

| Screen | New component needed | Reuse already in menata-app |
|---|---|---|
| `document-submit.html` | Multi-step HTMX form flow (new, small); per-step approver picker (flat, not Group-sourced -- see below) | `fieldInput` file upload, `fld_mode` status field, Phase 9 child collections, `sort_order` |
| `document-signature-placement.html` | PDF page-as-image render; draggable coordinate marker (**the named vanilla-JS exception** -- `pointerdown/move/up`, `(clientX-rect.left)/rect.width`, ~30 lines, posts to the existing generic `PUT` route) | `ChildSectionView` (extend with 3 numeric columns) |
| `document-approval.html` | Record Summary Card (worklist row, a real new presentation shape -- Study 38 Cluster 2); filter chips; approval progress stepper (visual only) | `slaBadge`, `decideButtons` (sticky bar is CSS-only), My Tasks' SLA-bucketing logic for the chips |
| `approval-dashboard.html` | Step 0's `sectionHeader` | **already built** -- Phase 13's `/dashboard` already composes DocumentSummary + Pending Approval + Recent Activity, the same Summary/Pending/Activity shape this screen names; no new route needed |

- [x] Phase 6 re-test (third time, via Phase 13's existing `/dashboard`, same Document-focused
      composition `approval-dashboard.html` names): **still not forced** -- same verdict as the
      first two tests, same reasoning (whole-machine queries joined in Go stay cheap regardless
      of section count)
- [x] Step 0 (done, 2026-09-15): shared `summaryCounts`/`activityFeedList`/`sectionHeader`
      components (`machine.templ`, alongside `slaBadge`), migrated into Dashboard/My Tasks/Sprint
      Dashboard/Team Capacity/Activity. Verified identical rendered content on every page before
      committing, plus one small real enhancement (Dashboard's sections now link "View all →" to
      their own already-built pages via `sectionHeader`)
- [x] Step 1 (done, 2026-09-15): `GET /approval-inbox` -- Record Summary Card (`recordSummaryCard`/
      `summaryCardList`, `machine.templ`) + SLA filter chips (plain query-param links, no JS) for
      the Approval Worklist, plus the "My Documents" section. "Submitted by" derived from the
      existing `mch_activity` log, not a new Field. Verified live: real assignee filtering,
      SLA bucket counts/filtering, and submitter resolution all confirmed with a scratch
      `ADMIN_USER_ID` override, then fully reverted
- [x] Step 2 (done, 2026-09-16): Approval progress stepper (`approvalStepper`, `machine.templ`) --
      replaces the generic child-collection table on a Document's own detail page, done/current/
      waiting states via `action.CanDecide`, no metadata change. Verified live against the real
      Vendor Contract 2026 document
- [x] Step 3 (done, 2026-09-18): `fld_signature_page`/`fld_signature_x`/`fld_signature_y` (plain
      `number` Fields, percentage-based, on `approval_step.yaml`) + a PDF-page-to-image preview
      step (`internal/pdf`, `PageCount`/`RenderPagePNG`, a thin wrapper over
      `github.com/richardwilkes/pdfview`). Library choice required a real supply-chain check, not
      a blind `go get`: of the "pure-Go PDF" candidates surfaced by search, one
      (`aspose-pdf-foss`) was near-certainly a fake trading on Aspose's name (4 stars/0 forks,
      955 commits, an org with no other footprint), another (`go-pdfkit`) had no independent
      corroboration anywhere despite elaborate self-claims. `richardwilkes/pdfview` was chosen on
      author reputation (a long-standing, widely-starred Go author -- `unison`, `gcs`) despite
      being 10 days old and pre-1.0 (v0.8.1, 0 importers at the time) -- the owner's own call,
      accepting that immaturity risk over pdftoppm/poppler-utils (mature, but breaks 007 §4.10's
      single-binary/no-native-deps posture) or cgo MuPDF (same posture problem). Pulling it in
      bumped `go.mod`'s toolchain requirement 1.25.14 -> 1.27.0 (the dependency's own floor), a
      real side effect worth knowing about if `go build` ever fails elsewhere in this repo for a
      toolchain reason. Verified against a hand-built minimal single-page PDF fixture
      (`internal/pdf/testdata/blank.pdf`): correct page count, in-bounds rendered dimensions,
      and a real error on an out-of-range page.
- [x] Step 4 (done, 2026-09-18): signature-coordinate placement screen (the vanilla-JS exception
      above) -- `GET .../signature-placement` renders one page of the Document's own PDF (Step
      3's `internal/pdf`, served by a new `.../pdf-preview` route) with one draggable marker per
      Approval Step (`rendering.SignaturePlacementPage`). Only the drag itself is vanilla JS
      (~25 lines: `pointerdown`/`pointermove`/`pointerup`, `(clientX-rect.left)/rect.width`) --
      placing a marker on the current page, and saving a drag's final position, are both ordinary
      HTMX submits to the existing generic PUT route (`hx-swap="none"`, reload on success), no new
      write path. Both new routes and the screen itself are hardcoded to `mch_document`, same
      posture as `internal/action` -- Case 3's own screen, not a generic per-Machine mechanism.
      Verified end-to-end on a temporary second server instance against the real Postgres database
      (not the live process on :4000, left untouched and never restarted): a hand-built 2-page PDF
      fixture, two scratch Approval Steps, page navigation (Prev/Next appear correctly at the
      first/last page), placing a marker, dragging it to a new position (persisted and
      re-rendered at the exact coordinates), and per-page marker filtering (each step's marker
      shows only on its own `fld_signature_page`) all confirmed; the wrong-Machine 404 guard on
      both new routes confirmed too. All scratch records (Document, 2 Approval Steps, the
      Activity entry the submission logged) and the scratch upload deleted afterward -- record
      counts per Machine verified identical before and after
- [x] Step 5 (done, 2026-09-18): `mch_signature` Machine (`fld_owner: person`, `fld_image: file`,
      `metadata/signature.yaml`) -- ordinary Machine, no identity-model change, no new Field type,
      zero new engine code (same "composed entirely from existing primitives" posture as Phase
      10's many-to-many join Machine). Verified end-to-end on a temporary second server instance
      against the real Postgres database (the live process on :4000 left untouched): the Machine
      appears on the landing page and gets its own generic CRUD page for free; a real PNG uploaded
      via the create form, downloaded back byte-identical; the detail page resolves `fld_owner` to
      a real user's name; that user's own detail page shows the signature back as a Phase 9 child
      collection ("Signature (1)") with zero extra code, the same free reverse-relation Phase 9
      already proved for Tasks. Scratch record and upload deleted afterward, record counts
      verified unchanged
- [x] Step 6 (done, 2026-09-18): Document Submit's multi-step wizard flow (`GET /documents/new`,
      `rendering.DocumentSubmitPage`), flat (non-Group) approver picker. One POST
      (`/documents`, `submitDocumentWizard`) creates the Document and its own Approval Steps
      together -- the submitted `<select>`s' own form-submission order becomes each step's
      `fld_sequence`, so no hidden sequence input is needed; reordering (▲/▼) and removal (✕) are
      pure client-side DOM operations (Hyperscript's `put ... before/after the previous/next
      <.approver-row/>` and `remove closest <.approver-row/>`) since nothing server-meaningful
      exists to persist until the whole form submits -- README.md's own "local UI toggles" case
      for Hyperscript, verified grammatically correct against hyperscript.org's own command docs
      (no live browser available in this environment to exercise the click handlers themselves,
      so that residual risk is real, not zero). Step numbering on the approver list is a pure CSS
      counter (`counter-increment`/`counter(...)`), staying correct through add/remove/reorder
      with no JS of its own. "New Approval" linked from the Approval Inbox page. "Document Type"
      selector and "save as default flow" are correctly omitted, matching the deferrals below --
      no `fld_document_type` exists in metadata to back either one, and inventing one wasn't
      forced by this step
- [x] Phase 15 exit criterion re-checked: all 4 Case 3 screens now match their own `ui-sample/`
      mockup's real content (Approval Inbox/document-approval.html -- Step 1; the Document detail
      page's stepper -- Step 2; signature-placement -- Step 4; this wizard -- Step 6;
      approval-dashboard.html was already covered by the pre-existing `/dashboard`)

**Deliberately deferred, not part of this phase:** Group-sourced approvers (still no forcing
case beyond this one screen -- ship flat, per `benchmarks/024`'s own "flat first, swap in groups
later" resolution; already named as deferred since Phase 7), "save as default flow per Document
Type" (only one Document Type exists in metadata today), PDF preview zoom/pan (cosmetic).

**Exit criterion met (2026-09-18):** all 4 Case 3 screens match their own `ui-sample/` mockup's
real content, using only the components in the matrix above -- no metadata-driven composition
mechanism, no Component Registry, built. Verified end-to-end on a temporary second server instance
against the real Postgres database (the live process on :4000 left untouched): the wizard's own
create -> two Approval Steps in submitted order -> redirect to signature-placement -> Document
detail page's stepper showing the correct current/waiting steps, all confirmed with a real PDF and
two real users. Scratch records (Document, 2 Approval Steps, the Activity entry) and the scratch
upload deleted afterward; record counts verified unchanged.

**Phase 15 complete.**

---

## Phase 16 -- Permission: the Domain Plane's fifth primitive (done, 2026-09-18)

**Forcing condition:** Case 3 already assigns each Approval Step to a real person
(`mch_approval_step.fld_assignee`, Phase 7) and Phase 15 Step 1's Approval Inbox already *shows*
each identity only its own steps -- but `POST .../decide` enforces none of it, so any
authenticated user can approve or reject any step. Permission is one of the five Domain Plane
primitives 004 §Domain Plane and 006 §Domain Model name alongside Machine/Field/Event/Constraint,
and it is the only one with a real forcing case that is not built. The 2026-09-18 audit found it
had fallen through this roadmap's own tracking discipline entirely: neither built, nor named as
deliberately deferred.

Sequenced here, before Phase 17, because Phase 17 burns an approver's signature image onto a real
PDF -- doing that for a decision nobody verified that approver actually made is a correctness
defect, not merely an access-control one.

**Design pass (resolve before code, same discipline as Phases 4 and 12):**

- *Does it reduce to an existing primitive?* No. Constraint (Phase 4) compares a transition
  against related records of *another* Machine; `internal/expression` has no notion of the acting
  identity at all -- `current_user` is not in its vocabulary, and 006 §Context is explicit that
  context "is not an implicit authorization mechanism." Authorization is its own concern.
- *Declared or hardcoded?* Declared. Hardcoding an actor check in `decideStep` the way Phase 12
  hardcoded `CanDecide` would close the `/decide` hole while leaving the audit's actual finding --
  that the Permission primitive does not exist -- untouched. 007 §27 says a capability should be
  admitted at the lowest useful abstraction level; here that is one small `permissions:` block on
  the Machine that owns the Action, validated like every other declaration.
- *Granularity:* record-scoped on one Action (`actor_field` names a `person` Field the acting
  identity must match). Deliberately **not** built: roles, groups, field-level scoping, a
  per-Machine CRUD permission matrix, or any expression language in the permission itself. None
  is forced while there is exactly one shared login credential -- per-Machine CRUD permission has
  nothing to discriminate on until per-user login exists, which is still deferred (Phase 7).
- *Where it lives:* `internal/authorization`, as a pure function over an already-fetched record,
  the same posture as `behavior.CheckConstraints` and `action.CanDecide` -- 007 §20 is explicit
  that "Permissions remain owned by the existing authorization architecture," and that the Data
  Plane must consume permission decisions rather than redefine them.

- [x] `permissions:` on a Machine (`id: prm_*`, `action:`, `actor_field:`), parsed and validated
      like every other declaration (`domain.Permission`, `metadata.validatePermission`) --
      `actor_field` must be a reference Field on the same Machine, and `action` must be in
      `domain.KnownActions`, the same closed static seam `KnownFieldTypes` already is (007 §14):
      a Permission naming an Action the runtime does not realize would silently protect nothing
- [x] `internal/authorization.AllowsAction`: the pure evaluator, unit-tested (actor matches /
      is someone else / is unidentified / record unassigned / no Permission declared / a
      different Action). An unassigned record is actionable by no one rather than by everyone
- [x] `decideStep` enforces it immediately after loading the step and before anything else is
      fetched, returning `403` -- authorization is established before any work, per 005
      §Security Ordering and 007 §20
- [x] `metadata/approval_step.yaml` declares the one real instance
      (`prm_decide_own_step`: `action: decide`, `actor_field: fld_assignee`)
- [x] `decideButtons` asks `AllowsAction` the same question, so a step assigned to someone else
      shows "Awaiting its own assignee's decision" instead of buttons that would 403. Presentation
      only -- the server denies the POST either way

**Known consequence, accepted:** the single shared admin credential resolves to one `mch_user`
record (`ADMIN_USER_ID`), so after this lands that identity can only decide steps assigned to
*it*. That is the correct behavior and it already matches what Phase 15 Step 1's Approval Inbox
shows; it does mean the live demo's seeded steps must be assigned to the admin identity to remain
decidable through the UI.

**Exit criterion met (2026-09-18):** verified end-to-end on a scratch database and scratch server,
with two real `mch_user` records (Rina, Maya) and the session identity switched between them --
deciding a step assigned to the other person returns `403` and leaves both the step's own
`fld_decision` and the parent Document's aggregate status untouched; deciding one's own step
returns `303` and drives the Document to `approved` exactly as before; a sequential Document's
step 2, assigned to the acting identity but still locked behind an undecided step 1, returns
Phase 12's own `422` rather than `403` -- the two rules are independent and both still fire. The
Approval Inbox's own listing was confirmed to agree with the new enforcement. Scratch database
dropped afterward; no production data touched.

---

## Phase 17 -- Case 3 signing: PDF signature compositing Action (done, 2026-09-19)

**Forcing condition:** Case 3's Approve step should produce a real signed PDF (CAP-F22 in
`menata-runtime/benchmarks/024`) -- burning each approver's signature image onto the document at
its declared coordinate (Phase 15's `fld_signature_x`/`y`) when the Document reaches `approved`.
This is genuinely new *capability* (menata-app has no PDF-manipulation code at all today,
`FieldTypeFile` is opaque blob storage), not new presentation -- Phase 15 deliberately excludes
it for that reason.

**Design pass (resolve before code, same discipline as Phase 12):** does compositing reduce to
an existing primitive? No -- `internal/action` (Phase 12's `CanDecide`/`DocumentStatus`) only
ever reads/writes Field values; burning an image onto a PDF byte stream is a real new operation
with a real new dependency (a pure-Go PDF library, e.g. `pdfcpu`). It belongs in `internal/action`
as a second hardcoded function alongside `CanDecide` -- same posture (fires on one triggering
write, hardcoded to `mch_document`/`mch_approval_step`'s own field IDs, not a generic engine) --
not a generic "PDF processing" service, since no second case needs one yet.

**Design clarification (2026-09-19, owner-prompted): signature source is irrelevant to this
Action.** A person may reach `mch_signature.fld_image` by uploading an existing image file, or by
signing manually (e.g. drawing on a canvas, saved as an image) -- both converge to the exact same
representation before `CompositeSignatures` ever sees them: a plain image file. No new Field type,
no metadata distinguishing "uploaded" from "drawn," and `CompositeSignatures` needs no branch for
how the image was produced -- it places whichever image is stored, the same way regardless of
source. If a manual-signing capture screen is ever built, it is purely a Phase 15-style Experience
concern (an input method for `fld_image`), not a change to this phase's own compositing logic.

**Design clarification, same date: placement needs a size, not only a position.**
`approval_step.yaml`'s `fld_signature_page/x/y` (Phase 15) name where the stamp's anchor point
goes, not how big it is -- there is no size Field today. Add `fld_signature_width` (a fourth
percentage-based `number` Field, sized relative to the page's own width, same convention as x/y);
height is derived from the image's own aspect ratio at composite time (`image.DecodeConfig`, no
full decode needed) rather than a second `fld_signature_height` Field, per this roadmap's own
admission question -- a redundant height Field isn't forced when the image's own dimensions
already answer it. Phase 15 Step 4's placement screen shipped with a position-only marker, no
resize handle; giving it one is this phase's own small follow-up, not a reason to reopen Phase 15.

**Blocking prerequisite, now satisfied: Phase 16 landed 2026-09-18.** Before Phase 16,
`decideStep` let any authenticated user decide any Approval Step, not only its own `fld_assignee`
-- this phase would otherwise have burned a real approver's signature image onto a PDF for a
decision nobody verified that approver actually made. `prm_decide_own_step` closes that; this
phase is no longer blocked.

- [x] `fld_signature_width` (percentage, `number` Field) added to `approval_step.yaml`; Step 4's
      placement screen (`widthControl`, `internal/rendering/signatureplacement.templ`) gains a
      number-input resize control, defaulting to 20% for a never-placed step, submitted on
      `change` to the same generic PUT route
- [x] `github.com/pdfcpu/pdfcpu` added; `action.CompositeSignatures` (`internal/action/composite.go`)
      takes the Document's PDF bytes + a `[]Stamp` (page/x/y/width/image) and returns the
      composited PDF. Pure over bytes, no I/O -- same posture as `CanDecide`/`DocumentStatus`.
      `action.StampFor` builds one approved step's `Stamp` from its own already-fetched Values, or
      `ok=false` if the step has no placement or the person has no signature on file -- both
      silently skippable, not an error, since this composites whatever is ready rather than
      blocking a real Approve on a placement someone forgot to make
- [x] The exact `pdfcpu` watermark parameters (`position:bl`, `offset:<pt> <pt>`,
      `scalefactor:<f> abs`, `rotation:0`) were **verified empirically, not from documentation
      alone** -- pdfcpu's own docs don't spell out the image-pixel-to-point convention its
      absolute scale mode uses. Method: composite a known image at a known percentage onto a real
      PDF, render the result back with Step 3's `internal/pdf`, and measure the rendered stamp's
      actual bounding box. First attempt rendered at the correct position and size but on a
      diagonal (`rotation:0` isn't the implicit default); the corrected formula measured within
      0.1 percentage points of every target across width, height, and both center coordinates.
      That exact check is now `TestCompositeSignatures_placesStampAtExpectedPositionAndSize`
      (`internal/action/composite_test.go`) -- a real regression test, not a one-off
- [x] Wired into `internal/web/approval.go`'s `decideStep`, via a new `signDocument`
      (`internal/web/signing.go`): once `recomputeDocumentStatus` drives the Document to
      `DocumentStatusApproved`, it gathers every approved step's `Stamp` (resolving each step's
      own signature by `fld_assignee` against Phase 15 Step 5's `mch_signature.fld_owner`),
      composites, and saves the result to `document.yaml`'s new `fld_signed_file` Field --
      mirroring `logActivity`'s own "one triggering write, one side effect" shape from Phase 13,
      and best-effort for the same reason: a compositing failure must not undo a decision that
      already succeeded, only get logged. `decideStep` itself gained one `if` and one call,
      staying at 53 of Phase 19's 70-line handler budget
- [x] Verified end-to-end on a temporary second server instance against the real Postgres
      database (the live process on :4000 left untouched, `ADMIN_USER_ID` overridden to a real
      `mch_user` so the scratch session's identity could actually pass Phase 16's own assignee
      check): a real signature-scribble PNG uploaded as a Signature, placed at 25%/80%/35% width
      on a real PDF, approved via `/decide` -- `fld_status` became `approved` and
      `fld_signed_file` was populated with a real composited PDF, downloaded and rendered back to
      confirm the signature actually appears bottom-left, correctly sized, transparency intact.
      Scratch records (Signature, Document, Approval Step, 2 Activity entries) and every scratch
      upload deleted afterward; record counts verified unchanged

**Exit criterion met (2026-09-19):** approving a fully-decided Document produces a real
downloadable PDF with every approver's signature image -- whether originally uploaded or manually
signed -- burned in at its declared position and size.

---

## Phase 18 -- Drift detection: make conformance automatic, not audited (done, 2026-09-18)

**Forcing condition:** this repo's protection against architectural drift is currently a manual,
sporadic audit. The 2026-09-18 pass proved both that it works (it caught Permission) and that it
is insufficient -- Permission "had fallen through this repo's own tracking discipline entirely"
and was found only because someone went looking. That is the same failure mode that killed
`menata-runtime`'s own composable rewrite, whose audit found the Behavior plane "was never
designed, not merely unbuilt": the problem there was not a missing mechanism, it was that nobody
learned they had drifted until the drift was too large to correct.

**Measured state, 2026-09-18 (the evidence this phase answers):**

*Clean, verified by the actual import graph, not by intent:* `internal/domain` imports no pgx, no
`net/http`, no templ; `internal/rendering` imports neither `internal/db` nor pgx, so no SQL
reaches the Experience Plane; `internal/db` imports pgx alone, holding its own doc.go claim of
having "no knowledge of Runtime Metadata semantics." No plane boundary is violated today. This is
a materially different state from the prior repo -- the contracts exist (001-007 plus each
package's `doc.go`), so a deviation can at least be named.

*Drifting, and the real finding:* **`cmd/server/main.go` has become the de facto composition
layer** -- 1591 lines, holding `loadChildSections`, `loadRelationOptions` and `loadBoardColumns`
(the exact three functions Phase 6's fourth test found duplicating fetches) while
`internal/composition/` contains only its `doc.go`. Nobody bypassed a built mechanism; the
mechanism's work simply accumulated somewhere unnamed, which is how the prior repo's drift would
have started too. Concrete consequence: `showApprovalInbox` is 143 lines of join/derive/bucket
logic and `go test` reports `cmd/server [no test files]` -- that logic is untested and not
testable where it currently sits.

*Also noted, smaller:* `internal/rendering/detail.templ` hardcodes `action.StepMachineID` and
`action.DocumentMachineID` -- Case 3-specific Machine identity inside a generic plane. Phase 12
deliberately accepted that hardcoded posture, but accepted it in `internal/action`, not in
`rendering`. Tracked here, not fixed by this phase; no second case makes it wrong yet.

**Why this phase is not "build IR/CEP early":** the prior repo did not fail for lack of
mechanism. Its composable rewrite was abandoned for building composition machinery against
*guessed* shapes. An IR written now, with no real composition to test it against, would be both
wrong and load-bearing -- worse than absent, because everything built afterwards inherits the
wrong shape and the correction cost is exactly the "already too big" outcome this phase exists to
prevent. The remedy is to make drift cheap to detect, not to build the deferred mechanisms early.
Nothing in this phase builds a 007 PROPOSED mechanism.

- [x] **Step 1: import-boundary test.** Encode each package's `doc.go` contract as an executable
      check that runs in `make test` -- the Domain Plane imports nothing physical, the Experience
      Plane reaches no database, `internal/db` carries no metadata semantics. Catches a violation
      the day it is written instead of at the next audit
- [x] **Step 2: move the composition functions into `internal/composition`, then fix Phase 6's
      measured duplication there.** Relocation first (`loadChildSections`/`loadRelationOptions`/
      `loadBoardColumns`, pure move, identical behavior), then the request-scoped read memo Phase
      6 already forced. Puts the code where its contract already is, makes it unit-testable, and
      shrinks `main.go` -- the seam starts doing its job at minimum size, with no IR and no
      dependency graph
- [x] **Step 3: permanent query diagnostic.** Make Phase 6's throwaway probe a real diagnostic
      surface: which Machines a request read, how many times each. Detects the next forcing
      condition automatically instead of by guess, and closes half of the "inference is not
      inspectable" gap below (001 Principle #6 requires inference results be exposable through
      diagnostics)
- [x] **Step 4: threshold harness -- construct the forcing conditions that cannot arrive on their
      own.** Per the Method's 2026-09-18 correction: this app has no users, so waiting for
      production-shaped triggers defers them forever. Build synthetic conditions instead and
      measure the architecture against them, recording the numbers whichever way they come out:
    - *Record volume* -- seed `mch_user`/`mch_task` at increasing scale and find where
      whole-Machine reads stop being viable. This is the named trigger for projection/filter
      pushdown and pagination (007 §21.1, §28 invariants #2 and #4), and the prerequisite for the
      dependency graph: once a referenced Machine is too large to fetch whole, a fetch's arguments
      start coming from another fetch's result, which is the first real dependency edge
    - *Metadata breadth* -- generate Machines that reference `mch_user` and confirm (or refute)
      Phase 6's claim that the duplication grows with schema size rather than record count
    - Record the measured threshold in Phase 6, so "not forced" is always backed by a number

**Exit criterion met, 2026-09-18** -- all four parts verified, not asserted:

- A boundary violation fails the suite: adding `net/http` to `internal/domain` produced
  `internal/domain must not import "net/http"` with the 004 clause it breaks; a file with a broken
  package clause fails too, so an unparseable file cannot hide its imports. `TestEveryPackageHasARule`
  additionally fails for any new package under `internal/` that has not declared what it may not
  depend on -- an undeclared boundary is how drift starts
- Phase 6's counts dropped to 4/5/9 with zero repeats, and all 14 routes' HTML is byte-identical
  to the memo-disabled build (table in Phase 6). `cmd/server/main.go` fell from 1591 to ~1380
  lines, and the moved logic now has unit tests where it previously had none
- Every request logs `reads=N repeated=M <path> [per-target breakdown]`, from
  `internal/data`'s own read paths, so it covers handlers that never touch the Loader
- The volume threshold is measured and recorded in Phase 6: whole-Machine reads pass 100ms
  between 10,000 and 50,000 records

**What this phase did not fix, deliberately:** `internal/rendering` still hardcodes
`action.StepMachineID`/`DocumentMachineID` (noted above). Left alone because no second case makes
it wrong yet, and because moving the Experience-tree types (`ChildSection`, `RelationOptions`) out
of `rendering` *is* the UI IR question -- designing their home now would be exactly the
speculative build this phase argues against.

---

## Phase 19 -- The transport seam: give the handlers a home, and a gate (done, 2026-09-18)

Phase 18 moved the composition functions out of `cmd/server/main.go` and said plainly what it was
leaving behind: the handlers themselves, in a file that "will grow again". It did. Phase 15 Step 6
added 110 lines and three handlers, taking `main.go` to 1621.

**Measured state, 2026-09-18 (why this is a forcing case and not a complaint about length):**

- **`cmd/server` had no boundary rule.** `internal/conformance` scanned `internal/` only
  (`internalDir()`), so the largest file in the repo was the one file free to import whatever it
  liked -- `database/sql` included -- and nothing would have failed. The least-supervised package
  was the one holding the most code.
- **146 lines of untested, untestable derivation.** `showApprovalInbox` resolved submitters from
  the activity log, counted approved siblings and bucketed by SLA. `go test` reported
  `cmd/server [no test files]`, and a `package main` binary has nowhere to put a test file.
- **Four doc comments had drifted off their functions.** `showTeamCapacity`, `showAutomation`,
  `showCalendar` and `showSprintDashboard` had stacked up above `showApprovalInbox`, so Go read
  them as one comment belonging to a fifth function. Four functions looked undocumented and one
  looked documented five times. Nobody had noticed, which is the point.

Neither of the first two is "the file is long". Length was the symptom the 2026-09-18 audit was
told to check against a number; what it actually found was a package with no declared boundary and
business logic with no reachable test.

- [x] **Step 1: cover `cmd/` with boundary rules.** Generalize `rule` to packages outside
      `internal/`, give `cmd/server` a rule, and extend `TestEveryPackageHasARule` to scan `cmd/`
      so a new binary must declare its boundary like any other package
- [x] **Step 2: `internal/web`.** Move the handlers, middleware and helpers out of `main.go`
      behind `web.Routes(web.Deps{...})`, leaving the composition root as a composition root.
      Scripted rather than retyped, so no line could be silently reworded
- [x] **Step 3: derivation down a plane.** Per-screen composition into `internal/composition`,
      each split into a fetch half taking the `Loader` and a pure build half taking records --
      the split is what makes the derivation testable without a database. Mutation handlers
      broken into named guards, so the order they run in is legible
- [x] **Step 4: handler size budget.** A measured per-handler limit in `internal/conformance`,
      which is the number ROADMAP.md's own deferral asked for and never recorded

**Exit criterion met, 2026-09-18** -- verified, not asserted:

- `cmd/server/main.go` is **71 lines**, from 1621. The largest handler in `internal/web` is
  `decideStep` at 55, from `updateRecordForm`'s 106
- All **55 captured GET responses** are byte-identical before and after, and per-route read
  diagnostics are unchanged at `repeated=0`. Two captures of unchanged code differ in nothing, so
  that comparison has a measured noise floor of zero
- The GET capture never calls a write path, so the three refactored mutation handlers were
  verified separately: **14 checks** over the running app covering the status-move event, a
  missing required field, the wizard's two steps, decide's sequencing in both orders, the
  recomputed Document status, and the edit route's refusal to move a decision
- **25 unit tests** where this logic had none. All four gates verified by violating them:
  `database/sql` in `cmd/server`, `internal/db` in `internal/web`, a ruleless package under
  `cmd/`, and a handler inflated past the budget

**What this phase did not fix, deliberately:** the Experience-tree types (`ChildSection`,
`RelationOptions`) still live in `internal/rendering`, and `internal/composition` still imports
them. Moving them *is* the UI IR question Phase 18 deferred, and Phase 19 has no better claim on
answering it. `internal/rendering`'s hardcoded `action.StepMachineID`/`DocumentMachineID` is also
untouched, for the same reason as before: no second case makes it wrong yet.

**Found while doing this, not fixed here:** a form body that is not `multipart/form-data` fails
with 400 on any Machine that declares a file field, because `handleFileUploads` treats
`FormFile`'s "request Content-Type isn't multipart/form-data" as a real error rather than as "no
file was submitted". Every form in the app sets `hx-encoding`, so the UI never hits it -- but
`parseRecordForm`'s own comment promises a plain url-encoded body works "for any other client,
e.g. a direct API caller", and for `mch_task` and `mch_document` it does not. Pre-existing
(confirmed against `543bcda`), and a behaviour change rather than a move, so it is filed in the
Operational backlog instead of being smuggled into a refactor.

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
- [ ] Sort/filter/search on record lists -- `Store.ListRecords` only orders by `sort_order`
      (Phase 9 replaced the original `created_at desc`; this line said otherwise until the
      2026-09-18 audit). See "Concept conformance gaps" below for the architectural half of this:
      there is no projection, filter pushdown, or pagination either
- [ ] JSON API parity -- `/api/machines/{id}/records` only supports create+list; update/delete
      exist only via the browser HTML routes, not the API
- [ ] Non-multipart form bodies fail on any Machine with a file field -- `handleFileUploads`
      (`internal/web/record.go`) reads `req.FormFile`'s "request Content-Type isn't
      multipart/form-data" as a failure instead of as "no file submitted", so a url-encoded
      `POST`/`PUT` to `mch_task` or `mch_document` returns 400. Every form in the app sets
      `hx-encoding`, so only a non-browser client hits it -- but `parseRecordForm`'s own comment
      promises exactly that client works. Found during Phase 19, pre-existing (reproduced against
      `543bcda`), left alone there because fixing it changes behaviour

---

## Concept conformance gaps (001-007 audit)

A different job from the Operational backlog above: those are hygiene items, these are places
where a **stated obligation in 001-007** is not met by the code. Each entry names the clause, what
the code actually does today, and the verdict -- *blocking* (must close before more feature work),
*tracked* (real gap, has a forcing condition, not blocking), or *deferred* (named with the trigger
that would force it, so it stops being an omission).

Nothing here is a 007 PROPOSED mechanism: IR, Data IR, UI IR, Context/Scope/Binding and the
Component Registry are correctly deferred (007 §34, §40 mark them PROPOSED) and stay in "What's
deliberately not phased yet" below. The Composable Execution Planner is the one exception as of
Phase 6's fourth test (2026-09-18): its *dedup* job is now forced by measured duplicate fetches
on three routes, and has its own checkbox in Phase 6. Its IR and dependency-graph halves remain
deferred with the rest.

### First pass (2026-09-18) -- Permission

- **Blocking -> closed by Phase 16 (2026-09-18).** The Domain Plane's **Permission** primitive
  (004 §Domain Plane, 006 §Domain Model, alongside Machine/Field/Event/Constraint) was not built,
  and until this audit was not named as deferred anywhere either -- it had fallen through this
  repo's own tracking discipline. `internal/authorization` only answered "is this session valid"
  (`IsAuthenticated`/`CurrentUserID`); there was no per-Machine, per-Action, or per-record check
  anywhere in `cmd/server/main.go`. Concretely, `POST .../decide` never checked that the acting
  user is the Approval Step's own `fld_assignee` (`action.CanDecide` enforces sequencing, not
  identity), so any authenticated user could approve or reject any step -- contradicting Phase
  12's own stated design. Promoted out of this list into **Phase 16**, since it has a real forcing
  case and therefore deserves a phase, not a backlog line. `capabilities.md`'s Authorization table
  previously overstated this as "Current granularity is Machine+Action only" -- corrected
  2026-09-18.

  **Still open after Phase 16, deliberately:** Permission now exists as a real declared primitive,
  but with exactly one shape (record-scoped, on one Action) and one instance. Ordinary record
  create/update/delete on every Machine remains ungoverned -- any authenticated identity can still
  edit or delete any record through the generic routes. That is not an oversight carried forward
  silently: it is unforced while one shared credential means every session is the same person, and
  the mechanism to close it now exists rather than having to be invented. Per-user login is its
  forcing condition.

### Second pass (2026-09-18, deeper) -- the rest of 001-007

- **Tracked: inference is not inspectable.** 001 Principle #6 does not merely prefer inference, it
  requires that "when inference materially affects data access, composition, authorization,
  rendering, or execution planning, the runtime should be able to expose the resolved result
  through diagnostics or equivalent tooling"; 004 §Inference and 005 Phase 4 repeat it ("infer
  before configure, **but make inference inspectable**"). menata-app infers materially in at least
  four places -- `person` silently becomes a Relation to `mch_user` (`metadata.Parse` writes
  `machine:` itself), `domain.FindChildCollections` derives every detail-page child section from
  reverse references, `loadBoardColumns` resolves board columns from a relation target, and the
  default `table` Layout applies to any Machine with no `view:` block -- and **none of it can be
  inspected**: there is no diagnostics route, no `--explain` mode, no dump of the normalized
  Application. Nothing in the repo tracked this until now. Forcing condition for closing it: the
  first time an inferred result is wrong or surprising and cannot be explained without reading Go
  source -- likely the first metadata author who is not the engineer who wrote the inference.
- **Tracked: no metadata versioning or change classification.** 004 §Metadata Versioning and
  Evolution and 005 §Safe Evolution / §Versioning and Plan Identity both require that metadata
  changes be classified by impact and that "potentially destructive changes should always require
  explicit migration decisions" (001 Principle #11, Data Preservation). No metadata file carries a
  version key, and nothing classifies a change: deleting a Field from a `*.yaml` today silently
  orphans that field's data inside every record's JSONB -- no warning, no migration decision, no
  rollback path. Business data survives by accident (JSONB keeps what it was given), not by
  design. Forcing condition: the first real destructive metadata change on data that matters.
- **Tracked: Navigation is code, not metadata.** 004 §Navigation Metadata and 006 §Navigation both
  name Navigation as an Experience-plane metadata concept. `rendering.pageShell` hardcodes a flat
  ten-link topbar, so every new page needs a `machine.templ` edit -- the one place where adding an
  application surface still requires a code change, against 001 Principle #3 (Metadata First).
  Related and also untracked until now: `ui-sample/README.md`'s own "Menu Navigasi (ini belum
  dibuat)" names a two-level navigation this app has never had -- a cross-application launcher and
  a per-application menu (desktop top bar / mobile bottom bar). Forcing condition: a second
  Application in the manifest, or the mobile bottom-bar layout Case 3's own mockups
  (`document-approval.html`) already assume.
- **Tracked: security scope is applied after retrieval, not inside the plan.** 005 §Security
  Ordering and 007 §20 are explicit that the runtime must establish workspace, user, record scope
  and action authorization *before* retrieval and optimization, and 007 §20 names the exact
  anti-pattern: "query all data -> render -> trim unauthorized rows." That is literally today's
  shape -- `showApprovalInbox` and `showMyTasks` both `ListRecords` a whole Machine and filter by
  the current identity in Go. Harmless while one shared admin identity sees everything, and
  correct to leave alone until visibility means something. **This is also the missing half of
  Phase 6's forcing condition**, which has been looking for "security-scoped data that can't just
  be fetched whole and filtered in Go."

  *Updated 2026-09-18, after re-running Phase 6's test as this entry instructed:* the prediction
  that Phase 16 would create that forcing condition was **wrong**. Permission gates one write
  Action and narrowed no read path -- `/approval-inbox` still measures 4 whole-Machine reads
  filtered in Go. Per-Machine visibility has nothing to discriminate on while one shared
  credential makes every session the same person, so the real trigger is **per-user login**
  (Phase 7's own deferred item), not Permission. Re-run Phase 6's security half after that lands,
  not before.
- **Tracked: Workspace never enters the data path.** 001 Principle #9 calls Workspace "the primary
  execution boundary" and 004 calls its isolation "a runtime invariant." It is metadata-only here:
  `domain.Workspace` is parsed from `app.yaml`, and then the `records` table has no workspace or
  application column at all (`migrations/001_records.sql`) and `Store` keys everything on
  `machine_id` alone. "Multi-workspace tenancy" is already listed as deliberately not phased --
  what was *not* named is the consequence: because the boundary is absent from storage rather than
  merely unused, adding a second Workspace later is a data migration on live records, not a
  metadata change. Worth pricing in before the first real multi-tenant case, per Principle #11.
- **Tracked: every read is a whole-Machine read.** 007 §21.1 (Projection Pushdown), §28 invariant
  #2 (no unbounded fan-out) and #4 (no full-record-by-default) are all unmet: `Store` offers
  `ListRecords`/`ListRecordsBy`/`GetRecord` only -- no projection, no filter pushdown, no
  pagination, no limit. Every page fetches each Machine's entire record set and reduces it in Go.
  This is a deliberate and so far correct trade (it is exactly why Phase 6 keeps testing negative),
  but it is bounded by record count, and nothing in the repo said so. Forcing condition: the first
  Machine whose record count makes a page slow -- at which point pagination lands before, not
  after, the planner.

  *Quantified 2026-09-18 (`make threshold`, Phase 18 Step 4).* That bound is no longer unknown: a
  whole-Machine read costs 0.7ms at 100 records, 5ms at 1,000, 44.8ms at 10,000, and **217ms at
  50,000** -- past the 100ms interactive budget somewhere between 10k and 50k rows in one Machine.
  The trade stays correct below that and fails above it. This entry previously waited for a real
  Machine to get slow, which is a production event this app cannot have; constructing it instead
  cost one test run, per the Method's 2026-09-18 correction.
- **Deferred, with trigger: no hot reload.** 005 §Hot Reload and 001 Principle #10 (Live
  Evolution) describe metadata changes taking effect without regenerating the application.
  `metadata.LoadApplication` runs once in `main()`, so a metadata change takes effect on restart.
  Principle #10's actual requirement -- no *source regeneration* -- is met; 005's hot-reload is
  explicitly conditioned on "where the runtime deployment model permits it," and a single-binary
  restart is cheap. Named here so it is a decision rather than an omission. Trigger: metadata
  authored by someone who cannot deploy, or a restart that stops being cheap.

---

## UI mockup conformance gaps (`ui-sample/` audit, 2026-09-19)

A third audit job, distinct from the two lists above. Those compare the code against 001-007
(stated concept obligations) and against hygiene. This one compares the code *and this roadmap*
against the 23 design mockups in `ui-sample/` -- the artifact both priority cases' own screen
lists in `case-portfolio.md` point at, and the only written description of what these two
applications are supposed to look like when finished.

Scope: all 23 files, read as text (every heading, label, column, badge and caption), against
`internal/web.Routes`, `internal/rendering`, `metadata/*.yaml`, `capabilities.md` and this
document. Same verdict vocabulary as the audit above, and the same rule from the Method's
2026-09-18 correction: a deferral must say whether its trigger is *constructible today* or
genuinely needs a deployment event. **This section records findings only** -- nothing here is
scheduled, and no code was changed to produce it. Each entry still needs its own phase decision.

### A. Built, but recorded nowhere

Found by reading `internal/web/router.go` and `internal/rendering/*.templ` against
`capabilities.md`. Each of these is real, working, verified code whose only description is a
checkbox in a completed phase -- which is exactly the tracking failure Phase 18 exists to prevent,
arriving from the opposite direction (capability drift, not architecture drift).

| Built capability | Where it lives | What the docs said |
|---|---|---|
| **Approval Inbox** -- Case 3's worklist screen: SLA filter chips, pending cards, "My Documents" | `GET /approval-inbox`, `rendering.ApprovalInboxPage`, `composition/approval.go` (Phase 15 Step 1) | Absent from `capabilities.md`'s Experience *and* Routes tables. Its one mention is in the *limits* table, as an example of the 007 §20 anti-pattern -- the doc names the screen only to say what is wrong with it |
| **Record Summary Card** (`recordSummaryCard`/`summaryCardList`) and **filter chips** (`filterChip`) | `internal/rendering/machine.templ`, `approvalinbox.templ` | Not recorded. Phase 15's matrix called the card "a real new presentation shape"; nothing lists it as existing |
| **Approval progress stepper** (`approvalStepper`, done/current/waiting from `action.CanDecide`) | `internal/rendering/approvalstepper.templ` (Phase 15 Step 2) | Not recorded anywhere outside Phase 15's own checkbox |
| **The shared component set** -- `slaBadge`, `summaryCounts`, `activityFeedList`, `sectionHeader` | `internal/rendering/machine.templ` (Phase 13, Phase 15 Step 0) | No inventory of this repo's own components exists. The mockups ship one (`case-19-component-breakdown.html`); menata-app has no equivalent list, so "does a component for this already exist?" is answered by grep |
| **Per-request read diagnostics** (`reads=N repeated=M` per route, per-target breakdown) | `internal/data.ReadLog`, `web.queryDiagnostics` (Phase 18 Step 3) | No row in `capabilities.md`; its "inference is not inspectable" limit says "no diagnostics route", which reads as though nothing is observable. Both statements are true and they are about different things -- what a page *read* is observable, what the runtime *inferred* is not |
| **Conformance gates** -- import boundaries per package, "every package declares a rule", 70-line handler budget | `internal/conformance` (Phase 18 Step 1, Phase 19 Step 4) | Mentioned once in passing under the Routes table; not listed as a capability, though it is one of the few mechanisms here that actively prevents regression |
| `GET /documents/new/approver-row` (the wizard's HTMX row fragment) | `web.newApproverRow` (Phase 15 Step 6) | Missing from the Routes table |
| `GET /ui-sample/*` (the app serves the mockups) | `router.go` | Missing from the Routes table, and `ui-sample/README.md` still claimed "none of this HTML is executed or served by `menata-app`" -- true when written, false since the route landed |

**Two inaccurate claims found in the same pass** (both corrected in place, 2026-09-19):

1. `capabilities.md`'s "Not yet built" line attributed colored label chips and board drag-and-drop
   to "their own forcing condition in Phase 14". Only Timeline is in Phase 14. Chips/avatars are
   Phase 10's own last bullet, "deliberately deferred to Phase 14" -- a deferral Phase 14 never
   received. Drag-and-drop appears nowhere in any phase; its only mention in this document is the
   Method's HTMX/Hyperscript note. See B2.
2. Phase 15's deferral reads "only one Document Type exists in metadata today". No
   `fld_document_type` exists at all -- zero, not one. The conclusion (don't build the saved
   default flow) stands; the reason given for it was wrong.

### B. In the mockups, absent from this roadmap

#### B1. Case 3 -- Document Approval

| Mockup shows | Today | Verdict |
|---|---|---|
| A human-readable document reference (`DOC-0091`), on every worklist card, dashboard row and detail header | Records carry a UUID and nothing else; no Machine declares a reference/sequence Field | **Untracked.** Not a screen concern -- a per-Machine display identity (Field- or Store-level). Trigger is constructible today: two people discussing one document without a way to name it |
| Document Type (Contract / SOP / Policy / Report / Other), shown on cards and driving the saved default flow | No `fld_document_type` | **Half-tracked** -- named only inside Phase 15 Step 6's deferral (with the wrong reason, see A). Nothing lists it as a missing Field |
| Mode + progress on the worklist card: `ALL · 2/3`, `ANY · 1/2` | `fld_mode` exists and is set by the wizard; the card shows neither mode nor decided/total | **Untracked**, and small -- `composition/approval.go` already counts approved siblings for the stepper |
| Document viewing from the inbox/detail: "View PDF · Download · 6 pages · 2.4 MB" | `internal/pdf.PageCount`/`RenderPagePNG` and `.../pdf-preview` exist, but are reachable only from the signature-placement screen; the detail page offers a raw download | **Untracked.** The capability is built and the screen that needs it doesn't use it |
| One screen: worklist on the left, selected document's detail + sticky Approve/Reject bar on the right | Worklist (`/approval-inbox`) and detail (`/machines/mch_document/records/{id}`) are two pages | **Untracked deviation.** Possibly the right call for this stack; it has simply never been written down as a decision |
| "Draft · not yet submitted" documents in My Documents | `fld_status` declares `draft`, but `submitDocumentWizard` hardcodes `in_review` and no other path writes a Document status | **Untracked.** A declared status option no flow can reach -- either a missing save-as-draft flow or a metadata option to remove |
| SLA breach as a real event: "Legal Review SLA breached ... escalated to Manager" in the activity feed | `experience.EvaluateSLA` is display-only, computed per render; nothing writes a breach event, notifies anyone, or escalates | **Untracked.** Trigger is constructible today -- the demo data already contains a past-due Document |
| On the approval detail: "Your saved signature image will be stamped at this position automatically when you approve", with the step's own page/position echoed back | Phase 17 composites at decide time; the approver is shown no confirmation of where their signature will land before they approve | **Untracked**, and adjacent to Phase 17 rather than inside it |

#### B2. Case 19 -- Project Management

| Mockup shows | Today | Verdict |
|---|---|---|
| **Project Workspace** (`project-workspace.html`, screen 1 of 11): project cards with % complete, card/member counts, on-track/at-risk/planning state, cross-project stat tiles (open tasks, due this week, blockers), workspace activity | No route. Phase 14 covers 8 of the 11 screens; Board is Phase 5 and Task Detail is Phase 8's generic page -- **screen 1 is in no phase at all** | **Untracked.** The largest single omission this audit found: a priority case's own entry screen, missing from the phase that exists to enumerate that case's screens |
| Every Case 19 screen scoped to one project -- breadcrumb "Projects / Website Redesign", tabs Board / Timeline / Calendar / Dashboard within it | Every built screen is application-global: one board, one calendar, one sprint dashboard, all Machine-wide | **Untracked**, and the most architectural of these findings: it is 007 §11's Context/Scope question (PROPOSED) arriving as a concrete UI requirement rather than a theory |
| Task Detail body: description block, checklist with progress, comment thread, several members, several attachments, Move/Copy/Archive | Generic detail page with `dl.detail-fields` + child collections. No multi-line text Field type exists (`KnownFieldTypes` has `text` only); no checklist, comment, or membership Machine; `fld_attachment` is one file | **Untracked as roadmap entries.** `case-portfolio.md` calls them "still unbuilt here -- real future scope, not yet attempted", which records the fact but assigns it nowhere. Note Phase 9's own forcing condition was *Case 19's Checklist*; the mechanism landed, the Machine that motivated it never did |
| Card face: colored label chips, member avatars, checklist progress `☑ 3/6`, due-date badge, blocked flag | Board cards are generic `RecordRow`s | **Tracking chain broken** -- Phase 10 deferred chips/avatars "to Phase 14"; Phase 14 has no such item. Checklist progress and the blocked flag depend on data that doesn't exist |
| Drag a card between lists; reorder lists manually | `sort_order` and ordered `mch_list` exist (Phase 9/10), with no interaction to drive them | **Untracked**, despite being Case 19's own one-line definition in `case-portfolio.md` ("Users reorder Lists and Cards freely and move Cards between Lists"). Only mention anywhere: the Method's HTMX note |
| Board affordances: inline "+ Add a card" per column, "+ Add list", per-list ••• menu, per-column counts, Filter | Creation is the Machine page's own shared form; no per-column count, no menu, no filter | **Untracked** (the per-column count and inline add are small; the filter overlaps the backlog item below) |
| Per-screen filters: My Tasks "All projects", Activity "Filter projects" | Neither screen filters | **Half-tracked** -- the Operational backlog's "Sort/filter/search on record lists" is about `Store.ListRecords` on Machine pages, not these composed screens |
| Timeline: workstream rows, milestone markers, Month/Quarter zoom | Not built | **Half-tracked.** Phase 14's Timeline bullet names date-range bars and the missing start-date Field, but not milestones, and does not say what a "workstream" would be in metadata (a Label? a Project? a new Machine?) |
| Calendar: "+ Add event", entries that are not Tasks (e.g. "Team sync · 15:00") | Week grid populated purely from `mch_task.fld_due_date` | **Untracked**, small -- either a Machine of its own or an explicit "calendar shows Tasks only" note |

#### B3. Platform-level -- six mockups, no phase owns any of them

`login.html` is the only one of the six with a built counterpart. The rest describe the
multi-tenant, multi-application, role-based shell every one of the 21 cases would sit inside, and
this roadmap names none of them as screens.

- **Group, Role, Membership and Invitation as domain concepts.** `workspace-members.html`,
  `member-role-detail.html` and `approval-role-matrix.html` all assume a Group primitive, a
  per-Application Role, an effective-access rule (direct assignment ∪ Group-derived, with
  Group-derived access read-only on the member screen), and an invite flow. This repo names
  exactly one narrow consumer of one of them -- "Group-sourced approvers", deferred since Phase 7
  -- and Phase 16 explicitly built Permission *without* roles or groups. **Untracked as a whole.**
  Trigger is partly constructible (a second identity + a second Application), partly not (real
  multi-person administration).
- **Workspace Home** (`workspace-home.html`): the application launcher -- each Application the
  signed-in identity can reach, with its own pending count and the identity's role in it -- plus
  an "Your access" summary. **Untracked.** Related to the tracked "Navigation is code" gap, whose
  trigger (a second Application in the manifest) is constructible today, but that entry names the
  *menu*, not this screen.
- **Choose Workspace** (`choose-workspace.html`): workspace picker with per-workspace role.
  **Half-tracked** -- multi-workspace tenancy is in "not phased yet" and its storage consequence
  is in the conformance gaps, but the screen is named nowhere.
- **Approval Role Matrix** (`approval-role-matrix.html`): role × transition grid, explicitly
  derived from Machines' own Permissions rather than a second permission model. It also
  references a per-Machine **process map** (`/{machineID}/process-map`) as something that already
  exists in `menata-runtime` and renders the same ProcessEdge data transition-first. Neither the
  matrix nor the process map appears in any list here. **Untracked.** Worth noting the matrix is
  a *projection of already-declared metadata*, which makes it the cheapest of these and a real
  test of whether `domain.Permission` carries enough to project.
- **Login extras**: forgot-password, "keep me signed in", self-service "create a workspace".
  Per-user login is tracked (Phase 7's deferral); these three are not, and the third is a
  provisioning flow rather than an auth detail.

#### B4. Cross-cutting

- **Responsive / mobile posture.** Every mockup is responsive; `pageShell` ships one hand-written
  desktop stylesheet. Only the mobile bottom bar is named (in the Navigation conformance gap);
  whether screens are expected to work on a phone at all is unstated. **Untracked.**
- **Visual language.** The mockups are a coherent Tailwind design system (spacing, type scale,
  status colors, card shapes). Every built screen approximates it ad hoc in inline CSS, and no
  decision is recorded on whether that system is adopted, approximated deliberately, or ignored.
  **Untracked** -- and cheap to decide once, expensive to decide per screen, which is what is
  happening now.
- **Empty, loading and error states, and write feedback.** The mockups show populated states only;
  the app has a few hand-written "Nothing pending your approval" lines and no convention.
  **Untracked**, minor, but it is a component-level concern the shared component set should own.
- **Accessibility semantics.** The component-contract mockup lists accessibility among a
  Component's declared inputs (`accessibility: { role: article }`). No component here declares
  any, and no conformance rule checks. **Untracked.**
- **Component contract and versioning** (007 §13) -- bounded inputs, declared child slots,
  actions/events, renderer, versioned contracts. The deferral list names UI IR and the Component
  Registry; the *contract* idea, which is what would discipline the shared components that already
  exist, is named nowhere. **Untracked, and the nearest of the three to being useful today.**

### C. Verified present or correctly tracked -- no action

Recorded so the audit is complete rather than only a complaint list: the mobile bottom bar and
cross-application launcher (Navigation conformance gap, with trigger); sequential/parallel mode
(Phase 12); Group-sourced approvers and "save as default flow" (Phase 15 deferrals); burndown,
story points and the blocked status (Phase 14's Sprint Dashboard entry, with the reason);
per-task assigned hours (Phase 14's Team Capacity entry); a generic automation engine (Phase 14's
Automation entry); Board Settings' tab structure (Phase 14, "no new settings mechanism");
UI IR / Component Registry / Dataset-Projection-ViewModel binding, which is what
`case-19-component-breakdown.html` documents end to end (deferred, 007 §34/§40); multi-workspace
storage consequences (conformance gaps); sort/filter/search on Machine lists (Operational
backlog); Timeline (Phase 14, unchecked, with its missing Field named).

---

## What's deliberately not phased yet

`internal/registry` (static component registry), `internal/execution` as a distinct physical
layer, Events/Services beyond the Action shape Phase 12 builds (006 §Behavioral Model still has
more than that one shape), Group-sourced approvers (named in Phase 7), multi-workspace tenancy
beyond Phase 2's minimal structure, and the full semantic field type vocabulary from 006 -- none
of these have a forcing case yet. Adding any of them before one exists repeats the mistake this
roadmap is written to avoid.

`internal/execution` specifically is not the same gap Phase 18 closed. Phase 18 moved the
*composition/data-fetch* functions (`loadChildSections`/`loadRelationOptions`/`loadBoardColumns`)
into `internal/composition`, where their own `doc.go` contract already said they belonged -- it
did not move the HTTP handlers themselves. `cmd/server/main.go` was 1512 lines as of Phase 18's own
commit, all of it route registration and ~30 handler functions (`showDashboard`,
`showApprovalInbox`, `decideStep`, `showSignaturePlacement`, ...), and every future phase that adds
a screen or Action adds another handler there -- the file will grow again, just without Phase 18's
specific duplicate-fetch failure mode repeating.

*Closed by Phase 19, 2026-09-18.* This paragraph used to end "No forcing case for splitting the
handlers out exists yet", and asked that the next audit check the file's size against a number.
Two things arrived before that audit did. First, the prediction came true faster than expected:
Phase 15 Step 6 added 110 lines and three handlers, taking the file to 1621. Second, and the
actual forcing case, the audit found two measurements rather than an aesthetic complaint:

- **`cmd/server` had no boundary rule at all.** `internal/conformance` scanned `internal/` only,
  so the largest file in the repo was the one file free to import anything, `database/sql`
  included. The package with the least supervision was the one with the most code in it.
- **146 lines of derivation with no test, and nowhere to put one.** `showApprovalInbox` resolved
  submitters from the activity log, counted approved siblings and bucketed by SLA, all inside a
  package that by definition holds no test files -- `go test` reported `cmd/server [no test
  files]` for the whole thing.

Neither is "the file is long", which would not have been a forcing case. The handlers now live in
`internal/web` and the derivation in `internal/composition`, both under boundary rules, with a
measured 70-line-per-handler budget in `internal/conformance` so the number this paragraph asked
for now exists and is enforced rather than remembered. `main.go` is 71 lines.

Permission (the fifth Domain Plane primitive, alongside Machine/Field/Event/Constraint) is
deliberately *not* in this list -- it already has a forcing case (Case 3's per-step
`fld_assignee`), which is why the 2026-09-18 audit promoted it into **Phase 16** rather than
leaving it as backlog. Everything else that audit found is in "Concept conformance gaps" above,
each with its own forcing condition or its own explicit trigger -- none of it is silently omitted
any more, which was the actual finding.

Phase 15's own research pass (2026-09-15) re-confirmed two more belong here, explicitly rather
than by omission: a metadata-driven page-composition mechanism / UI IR (007 §15, still PROPOSED)
and a generic Component Registry (007 §14) -- ~15 hand-wired templ page functions today is not
the dispatch-sprawl problem §14 exists to fix. Both still stand after Phase 6's fourth test
(2026-09-18): what that test forced is read deduplication, which is neither a UI IR nor a
registry concern.

`internal/action` is no longer in this list -- Phase 12 names its real forcing case
(Case 3's sequential/parallel approval). Until Phase 12 actually lands, treat this line as the
plan, not the status; check Phase 12's own checkboxes for what's actually built.
