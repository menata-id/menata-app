# Capabilities

What this runtime can actually do right now — one reference, kept current as each `ROADMAP.md`
phase lands. This is a different job from the other two planning docs, on purpose:

- **`ROADMAP.md`** — why and when each phase was built (the forcing-condition history).
- **`case-portfolio.md`** — what the two priority trial applications (Case 3, Case 19) still need.
- **This document** — what exists, right now, regardless of which phase or case built it.

Update this alongside the phase that changes it; don't let it drift into a second history the
way `ROADMAP.md` already is one — this file only ever describes the current state, never a
changelog of how it got there.

---

## Field Types

| Type | Meaning | Status | Proven by |
|---|---|---|---|
| `text` | Free-text string | Built | `mch_task.fld_title`, most Machines |
| `number` | Numeric value | Built | `mch_task.fld_priority`, `fld_sequence` |
| `boolean` | True/false | Declared, not yet exercised | `internal/rendering` renders it (checkbox); no current Machine declares one |
| `date` | Calendar date | Built | `mch_task.fld_due_date` |
| `status` | Enum, closed `options:` list | Built | `mch_task.fld_status`, `mch_document.fld_mode` |
| `person` | Implicit relation to `mch_user` — `machine:` is never written by hand, `internal/metadata.Parse` sets it automatically | Built | `mch_task.fld_assignee`, `mch_project.fld_owner` |
| `money` | Currency value | Declared, not yet exercised | No current Machine declares one; no forcing case yet |
| `relation` | Single-value reference to another Machine (`machine:` required) | Built | `mch_task.fld_project`, `fld_list` |
| `file` | Local-disk upload; value is a storage key, original filename embedded in it | Built | `mch_document.fld_file`, `mch_task.fld_attachment` |

Closed set, extended deliberately — `internal/domain.KnownFieldTypes` — not inferred from data
(007 §14's static-registry seam).

`text` is single-line: there is no multi-line, long-text or rich-text type. Case 19's Task Detail
description block (`ui-sample/project-card.html`) is the first real thing that would need one —
recorded in `ROADMAP.md`'s "UI mockup conformance gaps", not built.

---

## Composition primitives

| Primitive | What it does | Status | Proven by |
|---|---|---|---|
| Machine | The primary realization unit; a business capability | Built | 9 Machines currently declared (below) |
| Relation | Single-value reference field, validated for shape (`internal/data.ValidateRecord`) and existence (`internal/data.ValidateRelations`) | Built | `mch_task.fld_project` |
| Many-to-many | A join Machine with two Relation fields — not a new field type or storage shape | Built | `mch_card_label` (`fld_task` + `fld_label`) |
| Child Collection | Reverse-relation display: every record of another Machine whose field points at this one, shown on the detail page | Built | `internal/domain.FindChildCollections`; a Project's detail page shows its own Tasks |
| Constraint | Blocks a field transition while a related Machine has a matching record | Built (one shape) | `mch_project`'s `cst_project_done_no_open_tasks` |
| Action | A named business operation beyond a plain field write, with cross-record side effects | Built (one shape, hardcoded) | `POST .../decide` — Approve/Reject, `internal/action`; the closed set is `domain.KnownActions` |
| Permission | Record-scoped authorization on an Action: the acting identity must be the value of a declared `actor_field` | Built (one shape) | `mch_approval_step`'s `prm_decide_own_step`, `authorization.AllowsAction` |
| Ordered Lists | A Machine's records used as a board's real, renameable, reorderable columns (replaces a fixed status enum) | Built | `mch_list`, grouping `mch_task`'s board |
| Sort order | Explicit per-Machine ordering (`sort_order` column), assigned at create time | Built | Every Machine — `internal/data.Store.CreateRecord` |
| SLA badge | A `view.sla_field` date Field rendered as OVERDUE / "N day(s) left" instead of a plain date | Built | `mch_document`'s `fld_due_date`, `internal/experience.EvaluateSLA` |
| Activity log | Append-only event record, written as a plain Machine (not a new DataSource kind), on a triggering write | Built | `mch_activity`, `internal/web`'s `logActivity` — Document submission/decision, Task/Project creation, Task status moves |

---

## Experience / Rendering

| Capability | Status | Proven by |
|---|---|---|
| Table Layout (flat list + inline create/edit/delete) | Built, default | Every Machine without a `view:` block |
| Board Layout, grouped by a status field's Options | Built | (superseded on Task by the relation-based case below, but still the default board behavior for any Machine that groups by a status field) |
| Board Layout, grouped by a relation field (ordered Lists) | Built | `mch_task` groups by `fld_list` |
| Record detail page (`GET /machines/{id}/records/{id}`) | Built | One route, three renderings depending on requester — direct nav / HTMX-detail-context / HTMX-row-context |
| File download with original filename (`GET /uploads/*`) | Built | `Content-Disposition` names the real filename, not the storage key |
| Composed Dashboard (hand-assembled, not a generic mechanism) | Built | `GET /dashboard` — Projects+Tasks, Documents status summary, Pending Approval (with SLA badges), Recent Activity feed |
| My Tasks (personal work queue, "assigned to me" filter + SLA bucketing) | Built | `GET /my-tasks` |
| Project Activity (day-grouped cross-Machine event feed) | Built | `GET /activity` |
| Board Settings (Lists/Labels catalog hub, links to their own Machine pages) | Built | `GET /board-settings` |
| Team Capacity (weekly capacity + real active/total Task counts per Member) | Built | `GET /team-capacity` |
| Workflow Automation (read-only Trigger/Condition/Action view over real Constraint + Action) | Built | `GET /automation` |
| Calendar (week-grid Layout, Tasks grouped by due date) | Built | `GET /calendar` |
| Sprint Dashboard (real status summary + workload + attention-needed, no fabricated points/burndown) | Built | `GET /sprint` |
| Approval Inbox (worklist of the steps awaiting the current identity, SLA filter chips, plus "My Documents") | Built, hardcoded to one Machine pair | `GET /approval-inbox` — `rendering.ApprovalInboxPage`, `composition/approval.go`; submitter resolved from `mch_activity`, not a Field; every card carries a `DOC-0091`-style `Reference` (Phase 9's `sort_order`) and `mode · N/M approved` |
| Approve/Reject action bar | Built, hardcoded to one Machine | `mch_approval_step`'s own detail page only |
| Approval progress stepper (done / current / waiting, replacing the generic child-collection table on a Document's detail page) | Built, hardcoded to one Machine pair | `approvalStepper` (`internal/rendering/approvalstepper.templ`), states from `action.CanDecide`; no metadata change |
| PDF page-to-image rendering | Built | `internal/pdf.PageCount`/`RenderPagePNG`, pure-Go (`richardwilkes/pdfview`); served by `GET .../pdf-preview` |
| Signature-coordinate placement (drag a marker over a rendered PDF page) | Built, hardcoded to one Machine | `GET .../signature-placement`, `rendering.SignaturePlacementPage` — the one named vanilla-JS exception (drag math only; placing/saving a position is ordinary HTMX to the generic PUT route) |
| Document submission wizard (Document + its own Approval Steps created together, dynamic flat approver picker) | Built, hardcoded to one Machine pair | `GET /documents/new`, `POST /documents`; add/reorder/remove approver rows are Hyperscript (no server-meaningful state until the whole form submits) |
| PDF signature compositing (burn each approved step's signature image onto the Document at its declared page/x/y/width) | Built, hardcoded to one Machine triple | `action.CompositeSignatures`/`StampFor`, pure-Go (`pdfcpu`); triggered by `decideStep` once a Document reaches `approved`, writes `fld_signed_file`. Best-effort -- a compositing failure is logged, never undoes the decision |

Every screen above composes in `internal/composition` and renders from `internal/web`: a handler
resolves what the request carries, asks composition for the page's content, and renders it. The
joins, rollups and SLA bucketing behind these pages are ordinary unit-tested functions, not
handler bodies (`ROADMAP.md` Phase 19).

### Shared rendering components

The reusable pieces those screens are assembled from, all in `internal/rendering`. Listed because
"does a component for this already exist?" was previously answerable only by grep — and because
`ui-sample/case-19-component-breakdown.html` ships an inventory of the components the mockups
assume, which this table is the real counterpart to.

| Component | What it renders | Where |
|---|---|---|
| `slaBadge` | A `view.sla_field` date as OVERDUE / "Due today" / "N day(s) left" | `machine.templ`; used by `RecordRow` and `RecordDetailView` |
| `summaryCounts` | A row of labelled count tiles | `machine.templ`; Dashboard, Sprint Dashboard, Team Capacity, Approval Inbox |
| `activityFeedList` | `ActivityEntry` rows as one feed shape | `machine.templ`; Dashboard, Activity, Sprint Dashboard, My Tasks |
| `sectionHeader` | A composed page's section title + optional "View all →" link | `machine.templ` |
| `recordSummaryCard` / `summaryCardList` | A record as a card face rather than a table row | `machine.templ`; Approval Inbox's worklist and My Documents |
| `filterChip` | A query-param filter chip with its own count (no JS) | `approvalinbox.templ` |
| `approvalStepper` | A Document's own steps as done / current / waiting | `approvalstepper.templ` |
| `pageShell` | The page frame and the hardcoded topbar (see the navigation limit below) | `machine.templ` |
| `pdfThumbnail` | A Document's own PDF, page 1, as a small linked preview image | `detail.templ`; the Document detail page (reuses Phase 15 Step 3's `.../pdf-preview` route, no new route) |
| `signatureConfirmation` | Echoes a pending Approval Step's own placement back to its assignee before they decide, or prompts them to place one | `detail.templ`; `decideButtons`, from the step record already fetched for that page -- no extra query |

**Not yet built:** Timeline Layout; colored label chips and member avatars on a card face
(`ROADMAP.md` Phase 10's own last bullet, deferred there "to Phase 14" — a deferral Phase 14 never
actually received); drag-and-drop reordering of board columns and cards (named in no phase at all,
despite being Case 19's own defining interaction). Only Timeline carries a real Phase 14 entry
with its own missing Field named. Corrected 2026-09-19 — this line previously attributed all three
to Phase 14. The full list of what the `ui-sample/` mockups show and this app does not is
`ROADMAP.md`'s "UI mockup conformance gaps" section.

---

## Authorization

| Capability | Status | Notes |
|---|---|---|
| Session-cookie auth, gating every route except `/health` and `/login` | Built | `internal/authorization`, HMAC-signed cookie |
| Real identity resolution (session names an actual `mch_user` record) | Built | `config.AdminUserID` |
| Per-user login (distinct passwords per person) | Not built | Still one shared admin credential; no second real user has forced this yet |
| Record-scoped Action permission (declared) | Built (one shape) | `permissions:` on a Machine — `action:` + `actor_field:`, the acting identity must be that Field's value on the record being acted upon. `domain.Permission`, `authorization.AllowsAction`, enforced by `decideStep` (`403`) before any other work |
| `POST .../decide` assignee check | Built | `mch_approval_step`'s `prm_decide_own_step`; independent of, and evaluated before, Phase 12's sequencing rule (`CanDecide`, `422`) |
| Machine-level / CRUD permission | Not built | The generic create/update/delete routes are still ungoverned — any authenticated identity can edit or delete any record. Unforced while one shared credential means every session is the same person; per-user login is its forcing condition |
| Record-scoped *visibility* applied in the data plan | Not built | `/approval-inbox` and `/my-tasks` fetch a whole Machine and filter by identity in Go — the shape 007 §20 names as the anti-pattern. Tracked in `ROADMAP.md`'s "Concept conformance gaps"; also the missing half of Phase 6's own forcing condition |
| Login rate-limiting | Not built | Tracked in `ROADMAP.md`'s Operational backlog |

---

## Machines currently defined

| Machine | Purpose | Fields | Constraints / Actions |
|---|---|---|---|
| `mch_user` | Real identity | name, email, weekly capacity | — |
| `mch_project` | Case 19 groundwork | name, status, owner | `cst_project_done_no_open_tasks` |
| `mch_list` | Board columns | name | — |
| `mch_label` | Label catalog | name, color | — |
| `mch_task` | Case 19 groundwork | title, status, assignee, due date, priority, project, list, attachment | Board layout |
| `mch_card_label` | Task↔Label join | task, label | — |
| `mch_document` | Case 3 core | title, file, mode, status, due date, signed file | Aggregate status driven by its steps; `view.sla_field`; `fld_signed_file` written by Phase 17's compositing Action |
| `mch_approval_step` | Case 3 core | document, sequence, assignee, decision, signature page/x/y/width | `/decide` Action, sequencing enforced, `prm_decide_own_step` |
| `mch_activity` | Cross-case event log | machine id, record id, summary, actor | Written by `logActivity`, never by a user form |
| `mch_signature` | Case 3 core | owner, image | Input to Phase 17's PDF-compositing Action |

---

## Metadata validation

| Rule | Scope | Enforced by |
|---|---|---|
| Stable-identity ID patterns (`mch_*`, `fld_*`, `cst_*`, `prm_*`, `ws_*`, `app_*`) | Every declaration | `internal/metadata.Validate`/`validateConstraint`/`validatePermission` |
| Permission's `action` is one the runtime realizes, `actor_field` is a reference Field on the same Machine, no duplicate permission IDs | Per-Machine | `internal/metadata.validatePermission` |
| Known field type, no duplicate field IDs | Per-Machine | `internal/metadata.Validate` |
| `status` field requires at least one option | Per-Machine | `internal/metadata.Validate` |
| Relation/Person target Machine must exist in the Application | Cross-Machine, once all loaded | `validateRelationTargets` |
| Constraint's related Machine/field must exist, and the related field must actually be a relation pointing back | Cross-Machine | `validateConstraintTargets` |
| `view.layout` known, `view.group_by` names a real field | Per-Machine | `internal/metadata.Validate` |
| `view.sla_field` names a real field on the same Machine, and that field is a `date` | Per-Machine | `internal/metadata.Validate` |
| Record values: unknown field rejected, required field enforced, type-shape checked (status option, number, reference id, file key) | Every write | `internal/data.ValidateRecord` |
| Reference existence (does the referenced record actually exist) | Every write with a Relation/Person field | `internal/data.ValidateRelations` |

---

## Navigation / Routes

| Route | Purpose |
|---|---|
| `GET /health` | Liveness check, no auth |
| `GET /login`, `POST /login` | Sign in |
| `POST /logout` | Sign out |
| `GET /` | Machine list (landing page) |
| `GET /dashboard` | Composed dashboard: Project+Task, Document status summary, Pending Approval, Recent Activity |
| `GET /my-tasks` | Personal work queue: Tasks assigned to the current identity, Today/Upcoming/Completed |
| `GET /activity` | Cross-Machine event feed, grouped by day |
| `GET /board-settings` | Lists/Labels catalog hub, linking to their own Machine pages |
| `GET /team-capacity` | Members' weekly capacity + real active/total Task counts |
| `GET /automation` | Read-only Trigger/Condition/Action view over real Constraint + Action |
| `GET /calendar` | Week-grid Layout, Tasks grouped by due date |
| `GET /sprint` | Sprint Dashboard: status summary, workload preview, attention-needed |
| `GET /approval-inbox` | Approval Inbox: steps awaiting the current identity, SLA filter chips (`?filter=`), My Documents |
| `GET /machines/{id}` | A Machine's own page (table or board) |
| `POST /machines/{id}/records` | Create a record |
| `GET /machines/{id}/records/{id}` | Record detail page (or a fragment, for HTMX) |
| `GET .../edit`, `PUT .../{id}`, `DELETE .../{id}` | Edit / update / delete a record |
| `POST .../{id}/decide` | Approve/Reject an Approval Step |
| `GET /documents/new`, `POST /documents` | Document submission wizard: create a Document and its own Approval Steps together |
| `GET /documents/new/approver-row` | The wizard's "+ Add approver" HTMX fragment — one more approver row, populated with real `mch_user` options |
| `GET /machines/mch_document/records/{id}/signature-placement` | Signature-coordinate placement screen |
| `GET /machines/mch_document/records/{id}/pdf-preview` | One page of the Document's PDF, rasterized to PNG |
| `GET /uploads/*` | Download an uploaded file |
| `GET /api/machines`, `GET /api/machines/{id}/records`, `POST /api/machines/{id}/records` | JSON API — create+list only, no update/delete yet (tracked in Operational backlog) |
| `GET /ui-sample/*` | The static design mockups, served as-is from `ui-sample/` so a built page can be compared against the design it was built toward. Reference material, never current-code intent |

Every route above is registered in `internal/web.Routes` and served by a handler in that package;
`cmd/server` builds the dependencies and mounts it. A per-handler size budget in
`internal/conformance` keeps a handler from quietly becoming a screen's worth of logic again
(`ROADMAP.md` Phase 19).

Navigation itself is **not** metadata: `rendering.pageShell` hardcodes the topbar link list, so a
new page needs a code edit. See the architectural limits below.

---

## Diagnostics and self-enforcement

Mechanisms that observe or constrain the runtime rather than serve a screen. They are capabilities
like any other — and the two that prevent regression are the ones most easily forgotten.

| Capability | Status | Proven by |
|---|---|---|
| Per-request read diagnostics | Built | Every request logs `reads=N repeated=M <path>` plus a per-target breakdown, from `internal/data`'s own read paths (`data.ReadLog`, `web.queryDiagnostics`) — so a page's cost is a number, and the next duplicate-fetch forcing condition announces itself. This is the *observable-reads* half of 001 §6; the *inference* half is still missing (see the limits below) |
| Import-boundary conformance | Built | `internal/conformance` encodes each package's `doc.go` contract as a test: the Domain Plane imports nothing physical, the Experience Plane reaches no database, `internal/db` carries no metadata semantics. `TestEveryPackageHasARule` fails for any new package under `internal/` or `cmd/` that declares no boundary |
| Handler size budget | Built | A measured per-handler line limit in `internal/conformance`, so a handler cannot quietly become a screen's worth of logic again |
| Volume threshold harness | Built | `make threshold` (`internal/composition/threshold_test.go`) measures whole-Machine read cost at increasing record counts — the constructed forcing condition for pagination/projection, recorded in `ROADMAP.md` Phase 6 |

---

## Known architectural limits

Current-state facts, the same job as the rest of this file — not a plan. Each one is a place where
a stated obligation in 001-007 is not met today; `ROADMAP.md`'s "Concept conformance gaps" carries
the clause citation, the verdict, and the forcing condition for closing it (001-007 audit,
2026-09-18).

| Limit | What that means concretely |
|---|---|
| Inference is not inspectable (001 §6) | `person`→`mch_user`, child collections, board columns and the default table Layout are all inferred, and nothing can show the resolved result — no diagnostics route, no `--explain`, no normalized-Application dump |
| No metadata versioning or change classification (004, 005) | No version key anywhere; deleting a Field from a `*.yaml` silently orphans that field's data inside every record's JSONB, with no migration decision |
| Navigation is code, not metadata (004, 006) | `pageShell`'s topbar is a hardcoded link list; `ui-sample/README.md`'s two-level nav (cross-application launcher + mobile bottom bar) has never existed |
| Workspace never reaches storage (001 §9, 004) | `records` has no workspace/application column; `Store` keys on `machine_id` alone. A second Workspace later is a data migration, not a metadata change |
| Every read is a whole-Machine read (007 §21.1, §28) | `ListRecords`/`ListRecordsBy`/`GetRecord` only — no projection, filter pushdown, pagination or limit; pages reduce whole record sets in Go |
| Metadata loads once, at startup (005 §Hot Reload) | A metadata change takes effect on restart. Principle #10 (no source regeneration) is met; hot reload is a deliberate, triggered deferral |
| Both priority cases are partly designed and not fully built | The `ui-sample/` mockups describe screens, data and a platform shell this app does not have — Case 19's Project Workspace screen and per-project scoping, the Task Detail body (description/checklist/comments), board drag-and-drop, Case 3's Document Type and one-screen worklist+detail layout, and the entire Workspace/Group/Role platform shell. Document reference (`DOC-0091`) closed by `ROADMAP.md` Phase 20. Enumerated with verdicts in `ROADMAP.md`'s "UI mockup conformance gaps" (audit, 2026-09-19) |
