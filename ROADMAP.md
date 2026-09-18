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
   changes that verdict, it answers a different question.
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
- [ ] Step 4: signature-coordinate placement screen (the vanilla-JS exception above)
- [ ] Step 5: `mch_signature` Machine (`fld_owner: person`, `fld_image: file`) -- ordinary
      Machine, no identity-model change
- [ ] Step 6: Document Submit's multi-step wizard flow, flat (non-Group) approver picker

**Deliberately deferred, not part of this phase:** Group-sourced approvers (still no forcing
case beyond this one screen -- ship flat, per `benchmarks/024`'s own "flat first, swap in groups
later" resolution; already named as deferred since Phase 7), "save as default flow per Document
Type" (only one Document Type exists in metadata today), PDF preview zoom/pan (cosmetic).

**Exit criterion:** all 4 Case 3 screens match their own `ui-sample/` mockup's real content,
using only the components in the matrix above -- no metadata-driven composition mechanism, no
Component Registry, built.

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

## Phase 17 -- Case 3 signing: PDF signature compositing Action

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

**Blocking prerequisite: Phase 16 must land first.** `decideStep` today lets any authenticated
user decide any Approval Step, not only its own `fld_assignee` -- otherwise this phase burns a
real approver's signature image onto a PDF for a decision nobody verified that approver actually
made.

- [ ] Add a pure-Go PDF dependency; a `CompositeSignatures` function taking the Document's file +
      each approved step's signature image + coordinates, returning a new file
- [ ] Wire it into the existing `/decide` handler: once `DocumentStatus` reaches `approved`,
      write the composited result to a new `fld_signed_file` Field, mirroring `logActivity`'s own
      "one triggering write, one side effect" shape from Phase 13
- [ ] Smoke-test against a real PDF + real coordinates on a scratch server before touching
      production data, per this repo's own established practice

**Exit criterion:** approving a fully-decided Document produces a real downloadable PDF with
every approver's signature image burned in at its declared position.

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

---

## Concept conformance gaps (001-007 audit)

A different job from the Operational backlog above: those are hygiene items, these are places
where a **stated obligation in 001-007** is not met by the code. Each entry names the clause, what
the code actually does today, and the verdict -- *blocking* (must close before more feature work),
*tracked* (real gap, has a forcing condition, not blocking), or *deferred* (named with the trigger
that would force it, so it stops being an omission).

Nothing here is a 007 PROPOSED mechanism: IR, Data IR, UI IR, Context/Scope/Binding, the
Composable Execution Planner and the Component Registry are correctly deferred (007 §34, §40 mark
them PROPOSED; Phase 6 has tested its own forcing condition three times, all negative) and stay in
"What's deliberately not phased yet" below.

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
  correct to leave alone until Phase 16 makes visibility mean something. **This is also the
  missing half of Phase 6's forcing condition**, which has been looking for "security-scoped data
  that can't just be fetched whole and filtered in Go" -- Phase 16 is what creates it. Re-run
  Phase 6's test after Phase 16 lands, not before.
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
- **Deferred, with trigger: no hot reload.** 005 §Hot Reload and 001 Principle #10 (Live
  Evolution) describe metadata changes taking effect without regenerating the application.
  `metadata.LoadApplication` runs once in `main()`, so a metadata change takes effect on restart.
  Principle #10's actual requirement -- no *source regeneration* -- is met; 005's hot-reload is
  explicitly conditioned on "where the runtime deployment model permits it," and a single-binary
  restart is cheap. Named here so it is a decision rather than an omission. Trigger: metadata
  authored by someone who cannot deploy, or a restart that stops being cheap.

---

## What's deliberately not phased yet

`internal/registry` (static component registry), `internal/execution` as a distinct physical
layer, Events/Services beyond the Action shape Phase 12 builds (006 §Behavioral Model still has
more than that one shape), Group-sourced approvers (named in Phase 7), multi-workspace tenancy
beyond Phase 2's minimal structure, and the full semantic field type vocabulary from 006 -- none
of these have a forcing case yet. Adding any of them before one exists repeats the mistake this
roadmap is written to avoid.

Permission (the fifth Domain Plane primitive, alongside Machine/Field/Event/Constraint) is
deliberately *not* in this list -- it already has a forcing case (Case 3's per-step
`fld_assignee`), which is why the 2026-09-18 audit promoted it into **Phase 16** rather than
leaving it as backlog. Everything else that audit found is in "Concept conformance gaps" above,
each with its own forcing condition or its own explicit trigger -- none of it is silently omitted
any more, which was the actual finding.

Phase 15's own research pass (2026-09-15) re-confirmed two more belong here, explicitly rather
than by omission: a metadata-driven page-composition mechanism / UI IR (007 §15, still PROPOSED)
and a generic Component Registry (007 §14) -- ~15 hand-wired templ page functions today is not
the dispatch-sprawl problem §14 exists to fix, and Phase 6 has already tested negative twice.

`internal/action` is no longer in this list -- Phase 12 names its real forcing case
(Case 3's sequential/parallel approval). Until Phase 12 actually lands, treat this line as the
plan, not the status; check Phase 12's own checkboxes for what's actually built.
