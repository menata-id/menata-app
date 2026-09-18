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
| Activity log | Append-only event record, written as a plain Machine (not a new DataSource kind), on a triggering write | Built | `mch_activity`, `main.go`'s `logActivity` — Document submission/decision, Task/Project creation, Task status moves |

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
| Approve/Reject action bar | Built, hardcoded to one Machine | `mch_approval_step`'s own detail page only |
| PDF page-to-image rendering | Built | `internal/pdf.PageCount`/`RenderPagePNG`, pure-Go (`richardwilkes/pdfview`); served by `GET .../pdf-preview` |
| Signature-coordinate placement (drag a marker over a rendered PDF page) | Built, hardcoded to one Machine | `GET .../signature-placement`, `rendering.SignaturePlacementPage` — the one named vanilla-JS exception (drag math only; placing/saving a position is ordinary HTMX to the generic PUT route) |

**Not yet built:** Timeline Layout, colored label chips on a card face, drag-and-drop reordering
of board columns/list items — all named with their own forcing condition in `ROADMAP.md` Phase 14.

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
| `mch_document` | Case 3 core | title, file, mode, status, due date | Aggregate status driven by its steps; `view.sla_field` |
| `mch_approval_step` | Case 3 core | document, sequence, assignee, decision, signature page/x/y | `/decide` Action, sequencing enforced, `prm_decide_own_step` |
| `mch_activity` | Cross-case event log | machine id, record id, summary, actor | Written by `logActivity`, never by a user form |
| `mch_signature` | Case 3 core | owner, image | Input to Phase 17's PDF-compositing Action, not yet built |

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
| `GET /machines/{id}` | A Machine's own page (table or board) |
| `POST /machines/{id}/records` | Create a record |
| `GET /machines/{id}/records/{id}` | Record detail page (or a fragment, for HTMX) |
| `GET .../edit`, `PUT .../{id}`, `DELETE .../{id}` | Edit / update / delete a record |
| `POST .../{id}/decide` | Approve/Reject an Approval Step |
| `GET /machines/mch_document/records/{id}/signature-placement` | Signature-coordinate placement screen |
| `GET /machines/mch_document/records/{id}/pdf-preview` | One page of the Document's PDF, rasterized to PNG |
| `GET /uploads/*` | Download an uploaded file |
| `GET /api/machines`, `GET /api/machines/{id}/records`, `POST /api/machines/{id}/records` | JSON API — create+list only, no update/delete yet (tracked in Operational backlog) |

Navigation itself is **not** metadata: `rendering.pageShell` hardcodes the topbar link list, so a
new page needs a code edit. See the architectural limits below.

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
