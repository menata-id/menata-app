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
- [ ] Case 19: Timeline (date-range bars grouped by workstream) and Sprint Dashboard (aggregation,
      likely reuses Phase 13's activity/count mechanisms) -- Timeline specifically needs a Task
      start-date Field that doesn't exist yet, plus proportional-width bar rendering; the two
      screens in this phase that plausibly still need a real new rendering shape, not just
      composition of what already exists
- [ ] Case 3: signature coordinate placement (`document-signature-placement.html`) -- a new,
      fairly specialized drag-position editor; no other screen needs this interaction pattern

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
