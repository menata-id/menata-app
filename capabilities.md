# Capabilities

What this runtime can actually do right now — one reference, kept current as capability lands.
This is a different job from the other planning doc, on purpose:

- **`ROADMAP.md`** — what's shipped, in progress, and planned next, at a feature level.
- **This document** — what exists, right now, in technical detail, regardless of when it landed.

Update this alongside the change that affects it — this file only ever describes the current
state, never a changelog of how it got there.

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
| `group` | A Workspace Group id — reference *sugar* over the platform `workspace_groups` table, the same posture `person` takes over `mch_user`, except that a Group **is not a Machine** and so this carries no `machine:` at all (declaring one is rejected, not ignored) | Built (2026-09-20) | `mch_approval_step.fld_approver_group`, CAP-F24's Group arm. `domain.FieldTypeGroup`, `composition.Loader.GroupOptions`, `rendering.GroupLabel`. `Field.IsReference()` stays **false** for it — pinned by `domain.TestFieldTypeGroup_isNotAReference`, because a true there would make the picker look up a Machine that does not exist and render an empty `<select>` that appears to work |
| `file` | Local-disk upload; value is a storage key, original filename embedded in it; content sniffed and script-capable uploads rejected before `storage.Save` (security audit 2026-09-19, H2) | Built | `mch_document.fld_file`, `mch_task.fld_attachment`; `internal/web/record_test.go`'s `TestHandleFileUploads_rejectsScriptCapableContent`/`TestHandleFileUploads_allowsRealPDF` prove the content check |

Closed set, extended deliberately — `internal/domain.KnownFieldTypes` — not inferred from data
(007 §14's static-registry seam). `group` is the first type added since the original nine, and it
was added because a capability needed it rather than to round the set out: `capabilities.md` itself
used to record "whether a Group also needs a thin `mch_group` for identity … CAP-F24's
`approver_group` is what will force that question." It did, and the answer is this row — no
`mch_group`, a Field type over the platform table.

`text` is single-line: there is no multi-line, long-text or rich-text type yet — the Task Detail
screen's description block (`ui-sample/project-card.html`) is the first real thing that would
need one; not built.

---

## Composition primitives

| Primitive | What it does | Status | Proven by |
|---|---|---|---|
| Machine | The primary realization unit; a business capability | Built | 9 Machines currently declared (below) |
| Relation | Single-value reference field, validated for shape (`internal/data.ValidateRecord`) and existence (`internal/data.ValidateRelations`) | Built | `mch_task.fld_project` |
| Many-to-many | A join Machine with two Relation fields — not a new field type or storage shape | Built | `mch_card_label` (`fld_task` + `fld_label`) |
| Child Collection | Reverse-relation display: every record of another Machine whose field points at this one, shown on the detail page | Built | `internal/domain.FindChildCollections`; a Project's detail page shows its own Tasks |
| Constraint | Blocks a field transition while a related Machine has a matching record | Built (one shape) | `mch_project`'s `cst_project_done_no_open_tasks` |
| Event | Declarative post-write trigger, two mutually exclusive shapes: a Field's value changing (optionally to one target value), or a record being created (`on_create: true`) — runs a closed, runtime-owned Service either way | Built (two shapes) | `mch_task`'s `evt_task_status_changed` (field-change), `mch_document`'s `evt_document_submitted`/`mch_task`'s `evt_task_created`/`mch_project`'s `evt_project_created` (`on_create`); `domain.Event`/`domain.Service`, `behavior.MatchedEvents`/`MatchedCreateEvents` (pure decision), `internal/web`'s `runEvents`/`runCreateEvents` (I/O dispatch). Field-change replaced hardcoded Task-only Go (`currentTaskStatus`/`logTaskStatusMove`) as the first proof of 006's Behavioral Model chain; `on_create` (2026-09-19) replaced `logRecordCreated`'s own hardcoded `switch machine.ID` — the `menata-app-document` `workflow-behavior-decomposition-criteria.md` guide's own worked B1-B5 example, a real third case (not just second) before it was built. The one Service realized today is `svc_log_activity` (`domain.KnownServices`) |
| Dataset + Dimension + Measure | A named semantic data definition over one Machine's records (007 §7.2): an optional `dimension` to group by (§7.3), and one or more `measures` that count records or sum a number Field (§7.4), each optionally filtered by a `where` predicate | Built (minimal shape: `count`/`sum`) | Five declarations compose three screens: `mch_task`'s `ds_task_workload` (Team Capacity, and Sprint Dashboard's workload column — one declaration, two screens, which is what makes it *reusable* rather than per-screen config), `ds_task_by_project` (Dashboard's Project rollup), `ds_task_by_status` (Sprint's headline counts), `mch_user`'s `ds_user_capacity` (a grand-total sum), and `mch_document`'s `ds_document_by_status` (Dashboard's status tiles). `domain.Dataset`/`Measure`/`KnownAggregates`, `composition.Aggregate` (pure, no database), `internal/metadata.validateDataset`. A Dataset carries its own `Source` (007 §7.2's `source.machine`), derived from the Machine file that declares it rather than written twice, and its id is unique across the Application (`validateDatasetIDsAreUnique`) — so `Loader.AggregateDataset(ctx, "ds_...")` resolves source, reads and aggregates in one call, and a screen composing numbers names no Machine at all. Machine ids survive in `internal/composition` only where a screen needs *records* (rows to list), not counts. Generalized from the same "count records, grouped by a field, optionally filtered" loop found hand-written five separate times in `internal/composition/pages.go` — well past B1, per `menata-app-document`'s `audits/2026-09-19-decomposition-maturity-audit.md` §3.2. `where` deliberately reuses the existing `expression.Comparison` a Constraint's `block_if.condition` already declares rather than a second filter syntax (a B3 pass: the existing primitive already fit). Verified live: changing only `msr_active`'s `where.value` in YAML moved Team Capacity's own "Active cards" from 3 to 2 with no code touched. **What a Dataset deliberately cannot do:** select records (Dashboard's Pending list, Sprint's Attention list stay Go — that is 007 §8's Query Model, unbuilt), filter by a per-request value, or compare dates — which is exactly why My Tasks is the one counted screen not migrated (`PersonalTasks`' own doc comment explains). 007 §7.4 names four more aggregates (`avg`/`min`/`max`/`count_distinct`) — not realized, waiting on a real case, the same discipline that kept `KnownActions` at one entry |
| Parent status rollup | A child Machine declares how its parent's own status follows its siblings: one value held by *any* child decides the parent immediately, another held by *every* child decides it otherwise, with a declared default (including "no children yet") | Built (2026-09-20) | `mch_approval_step`'s `evt_step_decision_rollup` — a Document's status follows its Approval Steps by declaration, replacing `internal/web`'s own `recomputeDocumentStatus` and `internal/action.DocumentStatus`, which computed exactly this by hand for one Machine pair. `domain.Rollup`, the `rollup_parent_status` Service (`domain.KnownServices`), `behavior.RollupValue` (pure decision), `internal/web`'s `rollUpParentStatus` (the read and write). Declared on the *child*, where the relation to the parent already lives, so a single Machine file plus the Machine it points at can check every field it names — the cross-Machine half in `metadata.validateRollupTargets`. This is `menata-runtime`'s `capability-registry.md` **CAP-A08 `aggregate_status`** implemented rather than re-decided: admitted there on Case 3 + WCP-3/9/19/20 (van der Aalst's workflow patterns), conformance T24/T26. Its "cancel cascade" third arm is deliberately not built here either, for the same reason — no case declares one. **Found and closed alongside it:** the decide route never dispatched Events at all, so any Event declared on `mch_approval_step` was dead from the one route that decides a step |
| Ordered activation | A Machine declares that its records are acted on in order: one is locked while a sibling earlier in the order is still open, and only when the parent record's own mode says ordering applies | Built (2026-09-20) | `mch_approval_step`'s `sequencing:` block — an Approval Step is locked behind an earlier undecided one exactly when its Document is sequential, and never in parallel mode. `domain.Sequencing`, `behavior.CanAct` (pure), `metadata.validateSequencing` + `validateSequencingModes` (the `mode_field` half lives on the parent, so it is checked once every Machine is loaded). Replaces `internal/action`'s own `CanDecide`, whose *rule* was already generic — "a sibling earlier in the order is still open" — but whose field bindings were hardcoded to one Machine pair; the four call sites that asked it (the decide guard, the inbox's actionable filter, and the stepper's state on two screens) now all read one declaration. This is `menata-runtime`'s `capability-registry.md` **CAP-A07 `activate_next`** implemented rather than re-decided: admitted there on Case 3 + WCP-1 Sequence, conformance T22–23/T25. Ordering is opt-in — a Machine declaring no `sequencing:` never locks anything, which is why `internal/conformance.TestApprovalStepDeclaresSequencing` exists: dropping the block would unlock every step at once with no error anywhere |
| Action | A named business operation beyond a plain field write, with cross-record side effects | Built (one shape, hardcoded) | `POST .../decide` — Approve/Reject, `internal/action`; the closed set is `domain.KnownActions` |
| Permission | Record-scoped authorization on an Action: the acting identity must be the value of a declared `actor_field` | Built (governs `decide`/`edit`/`delete`) | `mch_approval_step`'s `prm_decide_own_step`/`prm_edit_own_step`/`prm_delete_own_step`, `authorization.AllowsAction` |
| Dynamic actor gate | A **record** chooses, at write time, which kind of actor gates it: one named person or a whole Workspace Group's membership. Three keys on a Permission (`actor_type_field`/`actor_user_field`/`actor_group_field`) beside the existing `actor_field` | Built (2026-09-20, CAP-F24) | `prm_decide_own_step`'s own gate; `domain.DynamicActorGate`, `authorization.allowsOne`, `domain.Actor`. The per-record part is what is new — a person gate and group membership (`data.EffectiveRoles`, Fase 4) both already existed separately; nothing could express a step deciding for *itself* which applies. **`actor_field` stays as the fallback**, taken whenever a record's own `actor_type_field` is empty or unrecognized, which is what let an existing Permission adopt this with no migration, no backfill and no rewritten test. Membership is resolved late — at the moment someone tries to act — so changing a Group's members changes who may approve, without touching a single Document |
| Transition | One declared edge of a Machine's own state model: a named move of a status Field from one declared option to another, and which Action performs it (`""` for one the runtime performs itself) | Built (2026-09-21) | `mch_approval_step`'s `trn_step_approve`/`trn_step_reject` (`action: decide`) and `mch_document`'s six `fld_status` edges (no action — that status is derived by `evt_step_decision_rollup`). `domain.Transition`, `behavior.CheckTransitions`, `metadata.validateTransition`. Three consumers read the one declaration: the write guard (`internal/web`'s `allowsTransition`/`declaredDecision`), the Review screen's Approve/Reject bar (`composition.canStillDecide`, which replaced a comparison against the literal `pending`), and the Approval Role Matrix's own rows. See the Authorization table for what it replaced and what it closed |
| Ordered Lists | A Machine's records used as a board's real, renameable, reorderable columns (replaces a fixed status enum) | Built | `mch_list`, grouping `mch_task`'s board |
| Sort order | Explicit per-Machine ordering (`sort_order` column), assigned at create time | Built | Every Machine — `internal/data.Store.CreateRecord` |
| SLA badge | A machine-level `sla_field` date Field rendered as OVERDUE / "N day(s) left" instead of a plain date | Built | `mch_document`'s `fld_due_date`, `internal/experience.EvaluateSLA` |
| Activity log | Append-only event record, written as a plain Machine (not a new DataSource kind), on a triggering write | Built | `mch_activity`, `internal/web`'s `logActivity` — Document submission/decision, Task/Project creation, SLA breach (`internal/composition`), and (2026-09-19, no longer hardcoded) Task status moves via the declared `evt_task_status_changed` Event above |
| Field default value | A Field's declared `default:` fills in a value a create leaves empty (absent, nil, or `""`) — create-only, never re-applied on update | Built | `mch_task.fld_status: default: todo`; `domain.Field.Default`, `data.ApplyDefaults`, called from every create path |
| Application plurality | A Workspace declares several Applications, each in its own file, selecting Machines by id from the Workspace's own set and carrying its own navigation | Built (2026-09-20) | `metadata/workspaces/*.yaml` + `metadata/applications/*.yaml` — Task Tracker split into `app_document_approval` and `app_project_management`, which is what the navigation `group:` labels already said and what `ui-sample/nav-metadata.js` calls `case3`/`case19`. `domain.Workspace.Applications`, `metadata.loadApplicationFile`. **Machines are workspace-level**: loaded once, unique by id (`validateMachineIDsAreUnique`), because `mch_user` and `mch_activity` are genuinely shared — per-Application ownership would load one file twice and make dataset-id uniqueness incoherent. An Application's `machines:` is a *selection*, and it is presentational and derivational only, never an access gate: Permission stays the single authority (006 §Behavioral Model). Each Application also declares its own card face — `description`, `icon`, `color` (a closed set, `domain.KnownApplicationColors`) and `summary_machine`, the Machine whose record count its card reports. The count is declared rather than summed because summing an Application's Machines counts configuration as work: Project Management's total 13, of which 4 are tasks and the rest lists, labels and join rows. `summary_machine` is validated to be a Machine that Application itself claims, so a card can never report another's number |
| Current-Application resolution | Which Application a request is in, resolved per request from the route | Built (2026-09-20) | `web.currentApplication` middleware → `rendering.CurrentApplication(ctx)`. **Machine-first, navigation as fallback**: the whole `/machines/{machineID}/...` surface (record detail, `/decide`, `/edit`, `/pdf-preview`, `/signature-placement`) plus `/documents` and the member-edit routes are named by *no* navigation item, and those are most of Document Approval's real screens — navigation-only derivation would return nothing for exactly them. `domain.Workspace.ApplicationForMachine` / `ForRoute`; `validateApplicationClaims` keeps the Machine answer unambiguous by refusing a Machine claimed by two Applications. Resolving to *no* Application is normal (Workspace-level screens, shared Machines); `CurrentApplicationName` degrades to the Workspace's name |
| `show_nav` | An Application declares whether it renders persistent menu chrome | Built (2026-09-20) | `metadata/applications/document-approval.yaml`. Replaces `hidden_nav_groups`, which named a group label *inside* one Application's navigation; this says it on the Application, exactly as `nav-metadata.js`'s own `showNav: false` does. Absent means **true** (`applicationDoc.ShowNav` is a `*bool` — a plain bool would default to false and silently mute every Application). It suppresses the Application's own menu only: every route stays a valid destination, reachable from contextual in-page links and from the launcher. The launcher itself now reads this field too (2026-09-20), but only to decide *how* to present the Application (a compact card when true, since the Application keeps its own menu chrome elsewhere; the pre-redesign flat item list when false, since the launcher is then the only place those screens stay reachable) — never *whether* it appears. **Capability note:** hiding one group *within* an Application is no longer expressible. Nothing used it — the only hidden group became an Application — but it is a real narrowing, not an oversight |
| `title:` / `description:` on a navigation item | A screen's own `<h1>` and the sentence under it, declared beside the menu `label:` that names it | Built (2026-09-24) | `metadata/applications/document-approval.yaml` — the Approval Inbox is `label: Inbox` in a tab strip and a bottom bar, `title: Pending my approval` on its own page (Flow 2 mockup, board 07). `title` falls back to `label`, so a screen whose heading really is its menu label says that by staying silent. Before this a Page had only `labelByID`, so every heading in the app was whatever string also had to fit in a four-column bottom bar, and every subtitle was a literal in its `.templ`. Gated by `TestRenderingHasNoHardcodedPageHeading` |
| `icon:` | An Application or a NavigationItem names an icon from a closed set (`domain.KnownIcons`), drawn by the runtime as inline stroke SVG | Built (2026-09-24) | `metadata/applications/*.yaml` — every Application and every navigation item declares one. A *name*, never a path or a glyph: metadata says which icon, the renderer (`rendering.icon`) says how it is drawn, so restyling the whole set is one file. Closed for the same reason `color:` is — an unrecognized name renders nothing, and the failure is invisible in a browser, so it fails at load instead. Superseded the single-character convention (`icon: "▣"`) whose own declaration comment had always flagged it as a placeholder |
| Application roles | An Application declares its own role vocabulary (`roles:`), and a member holds one role per Application | Built (2026-09-20) | `metadata/applications/document-approval.yaml` declares approver/submitter/reviewer -- and since 2026-09-21 each grants something different (approver decides; submitter submits; **reviewer may only look**, an owner decision, not an inference); `project-management.yaml` declares none and therefore offers no role at all, rather than inventing a vocabulary or borrowing another's. `domain.Application.Roles`, `migrations/008_workspace_member_app_roles.sql`, `data.Membership.AppRoles` (keyed by Application id — "no role" is the *absence* of a row, never a stored `""`). Replaced a hardcoded `appRoleOptions` list whose own comment already conceded it was single-Application. **Now an authorization input** (Fase 7, 2026-09-21): `authorization.AllowsAction` reads it through a Permission's own `roles:` arm — see "Role-based Action permission" in the Authorization table. This row read "not an authorization input yet … roles become grantable in Fase 7" until that landed. The write paths now reject a role the target Application does not declare (422), closing a gap that predated this: `workspace_role` was always checked, the Application role never was, because there was no vocabulary to check against |
| Groups | A named set of Workspace members that holds Application roles of its own — assign once, then manage membership | Built (2026-09-20) | `migrations/009_workspace_groups.sql`, `data.Group`, `internal/web/groups.go`, screens ported from `menata-runtime`'s own `groups.html`/`group-detail.html` (copied into `ui-sample/`). Mirrors upstream's shipped CAP-O07 schema: **one role per Group per Application**, and one per member per Application (008) — a person holding several roles in one Application, via two Groups or direct-plus-Group, is produced by **merging at read time** (`data.EffectiveRoles`), never stored. That is upstream's own rule: "holding a role through either path grants it identically … see the session-resolution merge, not this table". `EffectiveRoles` is pure and unit-tested against the cases CAP-O07 and board 05 name, including direct and group granting *different* roles — the set holds both, because neither source states a precedence and inventing one would decide semantics nobody has. Group grants validate through the same `submittedAppRoles` the member path uses, so an undeclared role is rejected 422 on both. **What this does not decide:** whether a Group also needs a thin `mch_group` for identity, the way `mch_user` is a Machine while `workspace_members` is platform — Fase 6's `approver_group` (CAP-F24) is what will force that question. The narrow claim here is that a Group's *role grants* are platform data, like every other membership fact |
| Workspace scoping | `records.workspace_id`, carried on `context.Context` (not a `Store` struct field, which was tried and reverted for leaking data across a per-request scope) and enforced on every read/write; a tampered `workspace_id` is rejected | Built | `data.WithWorkspaceScope`, `migrations/004_workspaces.sql` |

---

## Experience / Rendering

| Capability | Status | Proven by |
|---|---|---|
| Declared Views: a Machine names several arrangements of its own records, each addressable by `?view=vw_...` | Built (2026-09-20) | `mch_document` declares `vw_document_table` + `vw_document_cards`; an undeclared id 404s (`internal/web.resolveView`) rather than silently rendering another arrangement. A query parameter, deliberately not a route: a View decides what renders inside the page, never which shell |
| View type `table` (flat list + inline create/edit/delete) | Built, default | Every Machine that declares no `views:` at all |
| View type `board`, grouped by a status field's Options | Built | (superseded on Task by the relation-based case below, but still the default board behavior for any View that groups by a status field) |
| View type `board`, grouped by a relation field (ordered Lists) | Built | `mch_task`'s `vw_task_board` groups by `fld_list` |
| View type `cards` — every record rendered through its Machine's own `card_fields` | Built (2026-09-20) | `mch_document`'s `vw_document_cards`. Projection's first consumer whose output varies per record; changing a `card_fields` row changes what the cards show with no Go touched |
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
| Approval Inbox (one-screen worklist, SLA filter chips, plus "My Documents") | Built, hardcoded to one Machine pair | `GET /approval-inbox` — `rendering.ApprovalInboxPage`, `composition/approval.go`; submitter resolved from `mch_activity`, not a Field; every card carries a `DOC-0091`-style `Reference` (derived from `sort_order`) and `mode · N/M approved`; a worklist/My Documents card is a plain navigation to the record's own detail page (2026-09-19: dropped the inline hx-get/hx-target swap this used to layer on top -- a click opens its own page, not a pane below the list) |
| Approve/Reject action bar | Built, hardcoded to one Machine | `mch_approval_step`'s own detail page only; Approve is gated on a signature being on file (below) |
| Signature capture on Approve (draw on a canvas the first time, optionally remember it) | Built, hardcoded to one Machine pair | Owner request, 2026-09-19: `decideButtons` (`detail.templ`) opens a canvas `<dialog>` nested in its own form when the assignee has no signature on file yet -- the second named vanilla-JS exception alongside signature-placement's own drag math (freehand drawing has no non-JS equivalent). `internal/web/approval.go`'s `applyApprovalSignature` is the write-time half of the gate (422 if approving with neither a saved signature nor a submitted one); `hasSavedSignature`/`hasSignatureForGate` (`internal/web/signing.go`) is the render-time half. Checking "save my signature" persists a reusable `mch_signature`; leaving it unchecked writes only the step's own one-time `fld_signature_image`, so the next document asks again |
| Approval progress stepper (done / current / waiting, replacing the generic child-collection table on a Document's detail page) | Built, hardcoded to one Machine pair | `approvalStepper` (`internal/rendering/approvalstepper.templ`), states from `action.CanDecide`; no metadata change |
| PDF page-to-image rendering | Built | `internal/pdf.PageCount`/`RenderPagePNG`, pure-Go (`richardwilkes/pdfview`); served by `GET .../pdf-preview` |
| Signature-coordinate placement (drag a marker over a rendered PDF page) | Built, hardcoded to one Machine | `GET .../signature-placement`, `rendering.SignaturePlacementPage` — one named vanilla-JS exception (drag math only; placing/saving a position is ordinary HTMX to the generic PUT route). The same drag/place/width UI is now also composable, not just reachable by this route: `signaturePlacementBlock` (below) additionally renders inline on the Document's own detail page — a code-only Page/Component split (007 §12.1 vs §12.3), no metadata involved |
| Document submission wizard (Document + its own Approval Steps created together, dynamic flat approver picker) | Built, hardcoded to one Machine pair | `GET /documents/new`, `POST /documents`; add/reorder/remove approver rows are Hyperscript (no server-meaningful state until the whole form submits) |
| PDF signature compositing (burn each approved step's signature image onto the Document at its declared page/x/y/width) | Built, hardcoded to one Machine triple | `action.CompositeSignatures`/`StampFor`, pure-Go (`pdfcpu`); triggered by `decideStep` on *every* approved decision (owner request, 2026-09-19 — previously only once the whole Document reached `approved`), writes `fld_signed_file`. Recomposited from scratch each time from the Document's own original `fld_file`, never from a previous `fld_signed_file`, so calling this more often is safe and makes each new approval visible immediately, not just the final one. Best-effort -- a compositing failure is logged, never undoes the decision. `internal/web/signing.go`'s `signatureImageForStep` resolves the image per step: its own one-time `fld_signature_image` first, the assignee's reusable `mch_signature` otherwise |
| Approval-status banner (full-width, top of page 1, black Courier text on a translucent light-purple band, growing one line per approval) | Built, hardcoded to one Machine pair | Owner request, 2026-09-19: `action.ApprovalStatusBanner` (pure text, `internal/action/banner.go`) lists every currently-approved step by name and date, joined `" | "`; `action.CompositeStatusBanner` (`composite.go`) stamps it via two separate pdfcpu watermark passes — a background-color image band sized 1:1 in points-as-pixels (no independent alpha in pdfcpu's own single-pass text+backgroundcolor form, which is why this isn't one pass), then opaque black `Courier` text laid on top, wrapped via the exported `model.WordWrap` at the same width used to size the band. `signDocument` calls it every time it runs, page 1 only, a no-op when nothing is approved yet |

Every screen above composes in `internal/composition` and renders from `internal/web`: a handler
resolves what the request carries, asks composition for the page's content, and renders it. The
joins, rollups and SLA bucketing behind these pages are ordinary unit-tested functions, not
handler bodies.

### Shared rendering components

The reusable pieces those screens are assembled from, all in `internal/rendering`. Listed because
"does a component for this already exist?" was previously answerable only by grep — and because
`ui-sample/case-19-component-breakdown.html` ships an inventory of the components the mockups
assume, which this table is the real counterpart to.

**When a row belongs here (and when it doesn't):** the `menata-app-document` companion repo's
`guides/ui-composition-decomposition-criteria.md` (grounded in `references/ui-composition-
decomposition-frameworks.md`'s survey of Atomic Design, Shopify Polaris, GitHub Primer, Atlassian
Design System and Material Design 3) is the checklist. Short version: a shape is written inline in
its one Page first; it earns a row here only once a second, genuinely different Page needs the
identical shape and a markup-diff proves nothing is lost in generalizing it — premature promotion
has its own cost, so staying page-internal is the correct default, not a gap, until that's proven.
`pendingApprovalCard` below is a deliberate single-Page counter-example: real, cataloged (so
"already exists?" stays answerable by this table), but honestly marked not-yet-reused.

| Component | What it renders | Where |
|---|---|---|
| `summaryCountTiles` / `activityFeedListRow` / `sectionHeaderRow` / `slaBadgePill` | A composed page's count strip, event feed, section title and SLA badge. They were the **Tailwind twins** of four `pageStyles` renderers drawing the identical things in the other stylesheet — a deliberate, documented cost of the two-system transition. `summaryCounts`/`activityFeedList` went with their last callers on 2026-09-22; `sectionHeader` and `slaBadge` followed on 2026-09-24 when the last `pageStyles` screen ported, because once one stylesheet was left the pair was just the same component written twice. Same data shapes (`SummaryItem`, `ActivityEntry`, plain title/link args, a Field value) -- same data shapes (`SummaryItem`, `ActivityEntry`, plain title/link args, a Field value), different markup, because Preflight ties chrome to content (below) so a `pageStyles` class name renders unstyled under `appShell`'s `app.css`. `summaryCounts`/`activityFeedList` themselves were deleted 2026-09-22 with their last callers, all four ported the same day -- see "Styling: two systems" below | `appshell.templ`; Dashboard, My Tasks, Sprint Dashboard, Team Capacity, Activity |
| `taskRowList` | My Tasks' own Today/Upcoming/Attention row shape: title, status pill, project name, optional SLA badge | `mytasks.templ`; My Tasks and Sprint Dashboard's own Attention Needed section |
| `pendingApprovalCard` | Approval Inbox's own Pending-my-approval grid card (`case-03-flow1/07-approval-inbox.html`, Fase 6a): SLA framing, Mode/approval-ratio badge, submitted-by/date line, the approver list and a labelled `role="progressbar"`. Its two sub-shapes (`approverChip`, `approvalProgress`) are deliberately *not* rows of their own: board 10 will plausibly want both in Fase 6b, but a predicted second caller is not a second caller, and this table's default is page-internal until one exists | `approvalinbox.templ`; Approval Inbox's worklist only — page-internal, not yet reused elsewhere |
| `filterChip` | A query-param filter chip with its own count (no JS) | `approvalinbox.templ` |
| `approvalStepper` | A Document's own steps as done / current / waiting | `approvalstepper.templ` |
| `pdfThumbnail` | A Document's own PDF, page 1, as a small linked preview image | `detail.templ`; the Document detail page (reuses the `.../pdf-preview` route, no new route) |
| `detailBackLink` | A record detail page's own "← back" link: `routeByID("nav_approval_inbox")` for a Document/Approval Step (2026-09-19 -- their generic Machine page is POC scaffolding, not a real destination), the generic Machine page for every other Machine | `detail.templ`; `RecordDetailPage` |
| `reviewSignaturePanel` | Where the reviewer's own signature will land, drawn over the real page image (the existing `pdf-preview` route) rather than described in a sentence | `reviewdocument.templ`; board 10's right column. Replaced `signatureConfirmation`, which stated the coordinates in prose because the Document detail page's inline placement block shows *every* step's marker and is pinned to page 1 -- neither of which answers "where does mine go" |
| `signatureModal` | The canvas `<dialog>` the decision bar opens when Approve has no signature on file yet (2026-09-19); its styling became real Tailwind utilities in 6c-3 — under `appShell` the `pageStyles` `.sig-modal*` rules never loaded, so it had been rendering unsized and, behaviourally, without `touch-action: none` on its canvas, which broke signing on a phone -- draw, optionally save, submit as part of the same `hx-post .../decide` | `reviewdocument.templ` (moved from `detail.templ` in Fase 6b with its markup unchanged); `reviewDecisionBar`, shown only when `HasSignature` is false |
| `reviewDecisionBar` / `reviewProgress` / `reviewDocumentCard` / `reviewStatusPill` / `reviewStepMarker` | Board 10's four panels: SLA + Approve/Reject, the Approval Progress list, the file card with View PDF / Download, and the status pill. Page-internal and marked so on purpose (Q1: one caller) -- `reviewProgress` is deliberately *not* `approvalStepper`, which still serves the Document detail page in `pageStyles`; they share the composed `[]StepApprover` and not the markup, because Preflight ties chrome to content | `reviewdocument.templ`; `ReviewDocumentPage` |
| `signaturePlacementBlock` | The drag/place/width Component itself (document, steps, relations, page, totalPages -- a bounded contract, 007 §12.3), factored out of `SignaturePlacementPage` so a second Page could compose it too (2026-09-19) -- the composable-runtime kajian's own Fase 0, promoted here now that a second, genuinely different Page needs the identical shape, per this table's own promotion criterion | `signatureplacement.templ`; `SignaturePlacementPage` and `detail.templ`'s `RecordDetailView` (Document's own detail page, gated by `DocumentSignaturePlacement != nil`, fail-open on a PDF read error) |
| `fieldInput` | One Field's own input control, chosen by `domain.FieldType` (text/number/date/status/relation/file/...) | `controls.templ` (moved there 2026-09-24, beside the vocabulary it renders with); `RecordEditRow`, `createFormRow`, and `detail.templ`'s `RecordDetailEdit` |
| `fileLink` | A `file` Field's stored value rendered as a download link | `controls.templ`; `RecordRow`, and `detail.templ`'s `RecordDetailView` |
| `csrfHiddenInput` | The CSRF double-submit hidden `<input>` every state-changing form carries | `machine.templ`; every pre-auth form (`login.templ`, `register.templ`, `forgotpassword.templ`, `resetpassword.templ`, `resendverification.templ`, `chooseworkspace.templ`), `workspacemembers.templ`, and `machine.templ`'s own generic create/edit rows |
| `appShell` / `appLauncher` / `launcherAppCard` / `headMeta` | The Workspace-level page frame (`ui-sample/case-03-flow1` boards 03–05): a 9-dot launcher, a `Workspace / Page` breadcrumb and the viewer's avatar. Redesigned 2026-09-20 (owner request) from a flat list of every declared item into `ui-sample/nav-chrome.js`'s own card shape: the panel opens on a "WORKSPACE" header row linking to Workspace Home, then one `launcherAppCard` per `Workspace.Applications` (icon, name, description, linking to its `HomeRoute`) -- uniformly, `show_nav` no longer branches its shape (2026-09-21 removed the flat-list alternative with its last caller) -- and closes on an "All Workspaces →" link to `/switch-workspace`, shown for any real identity (`internal/web.viewerWorkspaceContext`'s `switchHref`) unconditionally rather than only when there is a second Workspace to switch to, since that screen is where an "add workspace" entry point is meant to land next (owner request, 2026-09-20); omitted only for the shared admin credential's placeholder identity, which has no membership row to key a Workspace list on. Both the launcher and `appShell`'s own menu below read `Application.AllNavigation` — the list *before* any filtering — for the same reason: a suppressed menu never suppresses the launcher entry or destination that reaches it. `headMeta` is the `<head>` shared with `authShell` | `appshell.templ`; every `appShell`-based screen |
| `applicationMenuRow` / `applicationBottomBar` / `moreSheet` | The current Application's own menu, generalized 2026-09-22 from Approval Inbox's one-off strip (`inboxTabs`, deleted with this change) to every `appShell` screen -- owner spec, `ui-sample/README.md`'s "Menu Navigasi": level 2 is a second row inside the header on desktop (`applicationMenuRow`, `hidden sm:flex`, opening on the Application's own icon tile and name — with the Workspace one row up and the page `<h1>` below, this row is the only place that says *which Application you are in* — then a rule, then a link per declared item) and a fixed bottom bar on mobile (`applicationBottomBar`, `sm:hidden`, `NavigationItem.Icon` over the label -- the Flow 2 mockup's M07-Inbox board is the visual reference). Both are driven by `defaultAppMenu`, which reads `CurrentApplication(ctx).AllNavigation` and marks the entry matching `CurrentPath(ctx)` active (path plus `?tab=`, generalizing what `routeTabParam` did for Approval Inbox alone) -- no client-side script, matching `appLauncher`/`accountMenu`'s existing no-JS posture. Renders only when the request is inside an Application that declares at least one navigation item; a Workspace-level screen (Home, Members) gets neither row. Ignores `show_nav` deliberately, the same as `inboxTabs` always did for Document Approval (`show_nav: false` there predates this component and only ever meant "no `pageShell` topbar"). **Three declared items plus "More" since 2026-09-24** (Flow 2 mockup, M07/M07b): `moreSheet` is the fourth column, a `<details>` bottom sheet holding Workspace Home, one row per Application in the Workspace (the current one marked `aria-current`), and "All Workspaces" -- a phone's only way *out* of an Application, since the 9-dot launcher is a pointer-sized top-bar dropdown. The live badge (`NavigationItem.Badge`) is drawn on both rows by the shared `navBadge`, whose `empty:hidden` is load-bearing: the endpoint renders *nothing* when there is nothing pending, so without it an empty inbox shows a bare coloured dot. Each badge is its own `hx-get` to `/api/approval-inbox/pending-count`, so a page pays for the count twice — named rather than hidden, and one request again once that count no longer has to fetch itself. **All three menus are the native Popover API since 2026-09-24**, not `<details>`: light dismiss (a click outside closes) and Escape come from the browser with no script, and the top layer puts them above every z-index. `<details>` gave none of that — its panels closed only by re-clicking their own control, which the owner met as a bottom sheet that would not close. The More sheet's shade is a full-width `<button popovertargetaction="hide">` rather than a styled `::backdrop`, because a real backdrop covers the whole viewport including the bottom bar that M07b deliberately leaves undimmed, and because light dismiss does not fire for clicks *inside* a popover | `appshell.templ`; every `appShell`-based screen with a current Application |
| `workspaceMenu` | The Workspace's own name made into a menu (mockup 03b / M03b): a tile with its initials, the viewer's Workspace role, then Workspace settings and Switch workspace. Renders at Workspace level **only**, never inside an Application — the mockup's own split, not a simplification: board 07's in-Application breadcrumb keeps the Workspace name a one-tap link home, and the launcher and More sheet already offer these destinations there. Two rows, not the mockup's three: this app has no invite route of its own (the form is a `<details>` on the Members screen), and a second row pointing where the row above it already goes is worse than one honest row. The "· 24 members" count is dropped for the recorded reason — a member count is not a field on `domain.Workspace`. Its settings row tests `!= member`, not `== admin`: the shared admin credential holds no membership row so its role reads `""` while still being able to open the destination, which is `workspacehome.templ`'s own deliberate three-way split | `appshell.templ`; every Workspace-level screen |
| `sheetPanel` | The one shape all three of `appShell`'s menus share: a `popover` that is a bottom sheet on a phone — a dark shade over everything *except* the bottom bar, a rounded panel pinned to the bottom, a drag handle — and, for the two in the header, an anchored dropdown at `sm` and up. `aboveBar` is `hasAppMenu`: a Workspace-level screen has no bar, so its shade covers the whole viewport (mockup M11/M12), while inside an Application it stops at the bar's top edge (M07b) and leaves the bar lit. The shade is a `<button popovertargetaction="hide">`, not a styled `::backdrop` — a real backdrop covers the bar too, light dismiss does not fire for clicks *inside* a popover so the shade must dismiss the sheet itself, and a button is keyboard-reachable where a div with a handler is not. **Extracted at the third use, and the bug that forced it is the argument for the rule**: the More sheet was built with a shade and positioned above the bar while the account menu kept its old bare panel, so for a day that panel covered the bottom bar with no shade and the only thing that would dismiss it was the avatar that opened it — tapping the bar to escape was a tap *inside* the popover | `appshell.templ`; `moreSheet`, `accountMenu`, `appLauncher` |
| `icon` | One of `domain.KnownIcons` as an inline stroke SVG -- 24x24, `stroke="currentColor"` at 1.8, round caps and joins, one `<svg>` with a `switch` inside it so those shared attributes are written once and cannot drift between icons. Inline rather than a sprite or `<img>` because every icon here takes its neighbouring text's colour (an active tab is blue, an inactive one slate, an Application tile follows its declared `color:`), which `currentColor` gives free and a file per colour would not. **Replaced the single-character glyph convention on 2026-09-24** (Flow 2 mockup; `icon: "▣"` became `icon: inbox`) -- `ui-sample/nav-metadata.js`'s own comment had always called those glyphs placeholders for "a real SVG icon set ... not-yet-scoped". Metadata names *which* icon, never how it is drawn. Unknown names draw an empty box rather than panicking, because an icon is decoration beside a label that still reads -- the real gate is at load (`internal/metadata` validates against `KnownIcons`) plus `internal/conformance.TestKnownIconsAreAllDrawn`, which asserts the switch covers every declared name | `icons.templ`; the bottom bar, `moreSheet`, `launcherAppCard` and Workspace Home's Application tiles |
| `authPasswordToggle` | The eye button inside a password box that reveals what was typed (2026-09-20): flips its input's own `type` between `password` and `text` via `aria-controls`, so the field keeps its name, minlength and autocomplete. The *toggle* is what's shared, not a whole labelled control -- the two password inputs differ (new-password carries minlength/required; sign-in's label row carries "Forgot password?"), the same split that already separates `authPasswordField` from `authField`. Inline `onclick`, not hyperscript: `authShell` loads no JavaScript at all, and nothing in CSS can change an input's type | `authshell.templ`; both password inputs -- `authPasswordField` (registration, set-password/accept-invite) and `login.templ`'s own sign-in field |
| `authShell` / `authCard` / `authError` / `authFooterNote` / `authField` / `authPasswordField` / `authSubmit` | The pre-auth page kit: `pageShell`'s counterpart for screens with no Workspace or Application chosen yet, so nothing to project a navigation from — a centered brand lockup over one white card, plus the labelled control, primary button and footer-note shapes those forms repeat | `authshell.templ`; all seven pre-auth screens (`login`, `register`, `chooseworkspace`, `forgotpassword`, `resetpassword`, `resendverification`, `checkyouremail`). Promoted on arrival rather than after a second caller: those seven each carried their own near-identical `<style>` block before this, all repeating one `body` rule verbatim, so the duplication this table exists to prevent was already seven deep. The first components styled with Tailwind (`static/css/app.css`) rather than `pageStyles` — see the layout note below |

### Styling: one system, since 2026-09-24

Every page renders from `static/css/app.css`, built from `static/css/input.css` by `make css` and
committed so `go build ./cmd/server` needs neither Node nor the Tailwind binary.

This section used to be called "two systems, on purpose" and described a planned transition: a
hand-written 122-line `<style>` block (`pageStyles`) served the generic per-Machine list and
detail screens through `pageShell`, while everything ported to a `ui-sample` board rendered from
Tailwind. The boundary was per *screen* rather than per layer for a real reason, worth keeping
written down because it governs any future migration of this shape: Tailwind's Preflight resets
heading sizes, list markers and button defaults that a hand-written sheet leaves to the browser,
so a page linking `app.css` must have its **content** ported in the same change or it visibly
breaks. That is what stopped the chrome being migrated on its own.

The transition finished when the last three screens moved (`MachineList`, `MachinePage`,
`RecordDetailPage`). What it cost while it lasted was not visible as a missing feature: those
screens loaded no `app.css` at all, so on a phone they had no sticky header, no bottom bar and no
launcher -- opening one record dropped the viewer out of the chrome entirely, with the browser's
back button as the only way back -- and they rendered in the platform's default font rather than
Ubuntu, which is why they read as a different application rather than a plainer page.

`pageShell`, `pageHead`, `pageStyles`, `navLink`, `navSections` and `CurrentApplicationName` were
deleted with them.

**Static assets carry a content hash, since 2026-09-24.** A page names
`/css/app.<8 hex>.css` and the two vendored scripts the same way; the hash is computed once at
startup (`internal/web.installAssetFingerprints`) off the files the same router serves, so the URL
a page emits and the file it resolves to cannot disagree. A request carrying a fingerprint is
`public, max-age=31536000, immutable` — safe precisely because its URL moves the moment the bytes
do — and one without is `no-cache`, still cached but always revalidated.

`ROADMAP.md` had deferred `Cache-Control` naming this exact dependency ("the filenames carry no
content hash, so a long max-age would serve stale assets after a deploy"). The cost of leaving it
was not theoretical: with no hash *and* no header, browsers fall back to heuristic freshness, and
a stylesheet from before a deploy was served twice in one day — reported as a UI that "still looks
the old way", which is a cache problem wearing a rendering problem's clothes. Gated by
`TestFingerprintedAssetCachePolicy` / `TestAssetURLChangesWithContent`, because nothing else in the
app can see it fail.

**One control vocabulary, since 2026-09-24.** `controls.templ` holds one literal class string per
control kind -- `controlPrimary`, `controlSecondary`, `controlDanger`, `controlField`, plus
`tableCell`/`tableHeadCell` -- and every `appShell` screen reads them instead of hand-writing its
own. They are constants rather than components, so they are not in the table above: that table is
gated on each entry being a real `templ` function, and a class string is not one.

It was added against a measurement, not a preference. While two stylesheets existed, a control
written twice was written in two different systems and the duplication was invisible. With one
stylesheet left it became countable, and the count was bad: **four different primary buttons**
(`h-9 px-4`, `h-8 px-3`, `h-11 sm:h-9`, plus two differing only by a leading `flex items-center`)
and **six different text inputs**, some with a focus ring and some without. Two of those variants
were introduced by the very change that made them countable, which is the argument for fixing it
before the next screen adds a seventh.

`controlField` deliberately carries no width: every caller has an opinion (`w-full` in a table
cell, `min-w-44 grow` in the wizard's approver row, `w-36` for a filter), and baking one in would
make each of them fight it with a second width utility whose winner depends on the order Tailwind
happens to emit them in.

**The pre-auth screens are deliberately excluded.** `authShell`'s own kit renders at
`h-10.5 sm:h-9.5` / `h-11 sm:h-9` with a 15px label -- a bigger touch target for a screen someone
reaches on a phone before they have an account. That is a decision, already captured in named
components (`authField`, `authPasswordField`, `authSubmit`); unifying the two sizes would undo it
rather than remove a duplicate.

### The Workspace's menu is derived, not declared (2026-09-21)

A Workspace manifest has no `navigation:` block any more, by owner instruction. A
Workspace's menu is now **the Applications it contains**, plus the link out to All Workspaces —
computed, not authored, by `appLauncher`'s panel (`pageShell`'s topbar and its own
`applicationNavEntry` were deleted 2026-09-24). Hand-listing Home / All Machines / Workspace Members / Groups /
Authorization Matrix made five *runtime* screens look like application metadata, which they are
not: they exist identically in a Workspace with ten Applications or none.

Those screens did not become literals when their declarations went away. Their route and label
live in `domain.RuntimeScreens`, which `rendering.declaredNavigation` appends, so every existing
`routeByID`/`labelByID` call site kept working unchanged and both hardcoding gates still cover
them. `metadata.validateNavigationIDsAreUnique` seeds itself with those ids, so an Application
cannot redeclare one. What *is* no longer offered anywhere in a menu is Groups and the
Authorization Matrix — both stay reachable (Workspace Members links Groups; both stay
`requireWorkspaceAdmin`-gated), and `TestWorkspaceHomePage_membersLinkVisibility` was narrowed to
the half that can still leak rather than left asserting a path that no longer exists.

`membersHiddenFor` and `appShell`'s own `hiddenNavIDs` parameter -- the viewer-level filter this
section used to describe -- were both deleted on 2026-09-21, with the launcher's last caller of
either (appshell.templ's own doc comment records why: once the launcher stopped listing individual
navigation items and started listing Applications, there was no item row left for a viewer-level
filter to hide). `ROADMAP.md`'s "Per-user/role navigation filtering" therefore goes back to having
no real case built at all -- see the "Known architectural limits" table below, not a stand-in.

### Navigation: one filter now, not two

This section used to describe a second, viewer-level filter (`hiddenNavIDs`) alongside the one
below; that filter is gone (previous paragraph), so there is one left.

`show_nav` (an Application's own file, e.g. `metadata/applications/document-approval.yaml`)
is **metadata-level**: identical for every viewer, it replaced the older `hidden_nav_groups:`
Workspace-level key (both now historical, kept only as forward-pointers in comments). **Today it suppresses nothing at all.** Its one reader was `pageShell`'s topbar, and `pageShell`
was deleted on 2026-09-24 when the last screens using it moved to `appShell` -- so a field two
Applications declare now changes no pixel anywhere. That is a real question for the owner, not a
tidy-up: either `show_nav` should mean something to `applicationMenuRow`/`applicationBottomBar`,
or it should go. It does not
reach the launcher (`appLauncher` reads `AllNavigation`, uniformly, for every Application) and it
does not gate `applicationMenuRow`/`applicationBottomBar` either (that row's own doc comment above
says so): both read `AllNavigation` the same way `inboxTabs` always did for Document Approval, so
an Application declaring `show_nav: false` still gets appShell's own menu chrome. Whether `show_nav`
should mean anything now that every screen renders through `appShell` is the open question above.

Tailwind is the **standalone CLI binary**, pinned and checksum-verified in the `Makefile` the same
way CI pins `goose` — it bundles its own runtime, so this stays a Node-free build. `app.css` is
generated *and committed*, exactly like the `*_templ.go` files, so `go build ./cmd/server` needs
neither Node nor the binary; `make check-generated` fails if either artifact is stale. The Ubuntu
faces `app.css` names are self-hosted under `static/vendor/fonts/ubuntu` (six files: 400/500/700 ×
latin and latin-ext), because `secureheaders.go`'s CSP is `default-src 'self'` and `font-src`
falls back to it.

Two scanner rules this depends on, both of which fail *silently* — as missing styling in a
browser, never as a build error:

- **Class names must be literal, and must live in a `.templ` file.** Tailwind finds them by
  reading text, so a name assembled at runtime (`"text-" + size`) never reaches `app.css` — and
  because the `@source` glob is `internal/rendering/*.templ`, neither does a class string in a
  `.go` file, **including a `.go` file inside `internal/rendering`**. That is why
  `appIconClasses` (`workspacehome.templ`) is a `switch` returning whole literal strings and lives
  in a `.templ` rather than beside the other Go helpers, and why `domain.KnownApplicationColors`
  is a closed set that has to be extended in step with it.
- **`input.css` imports Tailwind with `source(none)`**, disabling automatic content detection, so
  the only inputs are its own explicit `@source` lines. Without it Tailwind walks the whole repo
  and also reads every `ui-sample/*.html` mockup — design references that are explicitly "never
  current-code intent". That inflated `app.css` roughly fivefold (1029 rules to 206; 12.4KB to
  3.8KB gzipped once fixed) and, worse, made the build **non-deterministic in a shared checkout**:
  this pipeline's first CI run failed because the local build had scanned another session's
  uncommitted edits to four of those mockups and produced a different stylesheet than CI built
  from the committed ones.

**Not yet built:** Timeline Layout; colored label chips and member avatars on a card face;
drag-and-drop reordering of board columns and cards. The full list of what the `ui-sample/`
mockups show and this app does not build yet is tracked internally.

---

## Authorization

| Capability | Status | Notes |
|---|---|---|
| Session-cookie auth, gating every route except `/health`, `/login`, `/register`, `/verify-email`, `/resend-verification`, `/forgot-password`, `/reset-password`, `/choose-workspace` | Built | `internal/authorization`, HMAC-signed cookie; `requireAuth` group in `internal/web.Routes` |
| Identity owns name, email and credential | Built (2026-09-22) | `credentials.full_name` (migration 010). A person's full name is theirs: stated once when their account is created (registration, or accepting an invitation), changed only by them (`/account-profile` → `data.Store.SetFullName`), and read by every Workspace through `MemberNames` rather than copied into each one's `mch_user` record. This is what makes `/account-profile` honest about its own scope — it sits beside Security in the Account menu, and until the name moved it looked identity-level while silently editing one Workspace's record. **The one deliberate copy** is `mch_approval_step.fld_decided_by_name`: a signature snapshots the signer's name at the moment of the decision, because a live lookup would rewrite the name printed on an already-signed PDF whenever that person later changed theirs |
| Real identity resolution (session names an actual `mch_user` record) | Built | Per-user — no longer `config.AdminUserID`; `requireAuth`'s Workspace fallback for a non-user subject is `DefaultWorkspaceID` |
| Per-user login (distinct bcrypt-hashed passwords per person) | Built | `internal/auth`; registration (`POST /register`), blocking email verification, real per-user credential storage |
| Self-service password reset | Built | `GET`/`POST /forgot-password`, `GET`/`POST /reset-password` — `internal/web/passwordreset.go` |
| Workspace membership + invite, gated to Workspace admins | Built | `GET /workspace-members`, `POST /workspace-members/invite`, `GET`/`POST /workspace-members/{id}/edit` — `requireWorkspaceAdmin` |
| Application-level access (a role is required to enter at all) | Built (2026-09-21) | `web.requireApplicationAccess`, registered immediately after `currentApplication` and reading what it just resolved — so **every** route inside an Application is covered, the generic `/machines/...` surface and the bespoke screens alike. Owner decision: *someone who is not a member of an application cannot do anything in it, not even look.* Derived, not declared separately: an Application that declares `roles:` is gated on holding one of them; one that declares none (Project Management) gates on nothing, since requiring a role nobody can hold would deny everyone. A request in no Application (the Workspace screens) is untouched, and the shared admin credential is exempt by id, the same narrow carve-out `requireWorkspaceAdmin` takes. **Deliberately coarser than a per-Machine `read` Permission**: this runtime has no `read` Action (upstream's CAP-P05 `can_read` is the finer shape, unbuilt), so what is sayable is "in or out of this Application", never "may see Documents but not Approval Steps". Chosen at the middleware rather than per handler because the alternative left `/dashboard` and `/approval-inbox` — the two screens that read across everyone's documents — depending on nobody forgetting them |
| Workspace-role Action permission (declared) | Built (2026-09-21) | `workspace_role: admin` on a `permissions:` entry — the actor must hold that *Workspace* role. A separate key from `roles:`, not a reserved word inside it, because the two namespaces genuinely overlap: an Application may declare `admin` in its own vocabulary (`ui-sample/member-role-detail.html` shows exactly that for HR), so one list would make `roles: [admin, approver]` an alternation across two different meanings. `admin` is the only value — requiring `member` would be a rule every member passes and every admin fails. Declared on `mch_user` (`prm_edit_user_is_admin`/`prm_delete_user_is_admin`): the User Machine is the member directory, so changing a row in it is administering the Workspace, and until this it was editable and deletable by any authenticated member through the generic CRUD screens. **Holding admin is not a bypass**: it satisfies a Permission that asks for it and changes nothing about one that does not — a decision rather than an omission, held by `internal/conformance.TestWorkspaceAdminIsNotAPermissionBypass` because there is no bypass branch in the code whose absence anything would otherwise mark as deliberate. The owner's own `member-role-detail.html` states the same rule in its caption ("controls workspace administration, independent of application permissions"), while board 06 draws the opposite as an always-✓ ADMIN column |
| `create` as a governed Action | Built (2026-09-21) | `action: create` on a `permissions:` entry, enforced by `web.allowsRecordCreate` on the form and JSON create routes. Creation was the **one write path in this runtime with no authorization check of any kind, on every Machine** — found by an authorization review, not by a case. The reason it had none was real and had simply stopped being the whole story: a record-scoped rule needs a record, which creation has none of, but a role- or workspace-role-bearing rule reads no record at all. `actor_field` keeps a meaning here and only the one it can have — at creation the values checked are the ones being *submitted*, so it reads "the record must name you", which is what closes the signature hole below with no new key |
| Append-only Machine | Built (2026-09-21) | `append_only: true` on a Machine — every update and delete refuses, for everyone, including a Workspace admin, before any actor is consulted (`web.refusesAppendOnlyWrite`, `domain.Machine.AppendOnly`). A Machine property rather than a Permission because a Permission answers "which actor may" and here there is no such actor; saying it as one would mean naming a role nobody can hold, a rule that reads as a grant and denies everyone. Declaring it alongside an `edit`/`delete` Permission is a load-time error, since one of the two could never fire. `mch_activity` is the whole of it: the audit trail was editable and deletable by any member until this. `menata-runtime`'s CAP-R07 narrowed to the one case that exists here — upstream's state-triggered form ("frozen once posted") needs a case this repo does not have |
| Role-based Action permission (declared) | Built (2026-09-21, CAP-P01) | `roles:` on a `permissions:` entry — the actor must hold at least one of those Application roles for the Permission to pass. `domain.Permission.Roles`, `domain.Actor.Roles`/`HasRole`, `authorization.holdsOneOf`. The roles are read from the Application that claims the Permission's Machine (`domain.Machine.ApplicationID`, stamped at load from that Application's own `machines:` list), because a role word only means something inside one declared vocabulary — `approver` in two Applications is two different grants, and a Workspace-wide namespace would silently merge them. **Several roles on one Permission are alternatives; several Permissions on one Action stay requirements** (AllowsAction's existing contract, unchanged), so `roles: [approver, reviewer]` beside an `actor_field` reads "either role, AND the person this record names". The actor's side is their *effective* roles — `data.EffectiveRoles`, direct ∪ every role their Groups hold there — so a role granted through a Group gates identically to a direct one (CAP-O07's rule), resolved once per request by `web.currentActor`. Declared today on all three of `mch_approval_step`'s Permissions (`roles: [approver]`) and on `mch_document`'s create/edit/delete (`roles: [approver, submitter]`) -- the owner's decision of 2026-09-21 that *a reviewer may only look*, which is what finally made the three declared roles mean three different things: before it, approver and reviewer granted the same thing everywhere they appeared and submitter granted nothing at all; `internal/metadata.validatePermissionRoles` refuses a role the claiming Application does not declare, and a role-bearing Permission on a Machine no Application claims, because both deny everyone forever while reading as a grant. `internal/conformance.TestApprovalStepPermissionsCarryRoles` is what stops the arm being dropped from the manifest silently |
| Declared transition model | Built (2026-09-21) | `transitions:` on a Machine — which moves of a status Field exist, and which Action performs each. `domain.Transition`, `behavior.CheckTransitions` (pure), enforced by `internal/web`'s `allowsTransition` (generic update route + JSON twin) and `declaredDecision` (`/decide`). Opt-in per Field: a status Field no Transition mentions moves freely (Principle #6). It replaced `allowsDecisionChange`, a hand-written Go rule naming one Machine and one Field, and closed two things that rule never covered — an already-approved step could be re-decided on a *parallel* Document (sequencing only ever locked a step behind an *earlier* one), and a Document's own `fld_status` could be written straight to `approved` through the generic route with no step decided, skipping the whole approval flow, the sequencing rule and the PDF compositing. Verified live against the dev database: that PUT is now `422 fld_status moves from "in_review" to "approved" by itself -- it is not set directly`, record unchanged. This is `menata-runtime`'s Process Overlay `transitions[]` implemented rather than re-decided; it deliberately does *not* compile into an Event the way upstream's does, because a `domain.Event` here is a post-write notification rather than a triggerable operation — see `domain.Transition`'s own doc comment for that divergence and the one about `actor:` |
| Record-scoped Action permission (declared) | Built (one shape, three Actions) | `permissions:` on a Machine — `action:` + `actor_field:`, the acting identity must be that Field's value on the record being acted upon. `domain.Permission`, `authorization.AllowsAction`, `domain.KnownActions` = `decide`/`edit`/`delete`. `decide` is enforced by `decideStep` (`403`); `edit`/`delete` (2026-09-19) generalize the same shape to the generic update/delete routes — `allowsRecordEdit`/`deleteAllowed` (`internal/web/record.go`), also enforced in `internal/web/api.go`'s JSON twins, and hidden client-side (Edit/Delete/Save buttons, signature-marker drag controls) wherever `AllowsAction` refuses |
| `POST .../decide` assignee check | Built | `mch_approval_step`'s `prm_decide_own_step`; independent of, and evaluated before, the sequencing rule (`CanDecide`, `422`) |
| Machine-level / CRUD permission | Built (record-scoped, declared per Machine) | The generic create/update/delete routes ANDed business-state guards (`action.CanDeleteDocument`/`CanDeleteApprovalStep`) with zero identity check until 2026-09-19; now `allowsRecordEdit`/`deleteAllowed` also enforce any declared `edit`/`delete` Permission, same shape as `decide`. Declared today only on `mch_approval_step` (`prm_edit_own_step`/`prm_delete_own_step`, its own assignee) — every other Machine still has no declared Permission and stays fully unrestricted for any authenticated Workspace member, per Principle #6 (metadata describes exceptions, not defaults), not because CRUD permission itself is unbuilt. Record *creation* has no existing record to match an `actor_field` against, so it has no Action of its own yet |
| Record-scoped *visibility* applied in the data plan | Not built | `/approval-inbox` and `/my-tasks` fetch a whole Machine and filter by identity in Go — the shape 007 §20 names as the anti-pattern |
| Login rate-limiting | Built | `internal/web/ratelimit.go`, in-memory sliding window (10 attempts / 5 min), keyed by client address + attempted email; enforced on `POST /login`. Same shape also guards `POST /register` and `POST /forgot-password` (address-keyed) |

---

## Outbound email

| Capability | Status | Proven by |
|---|---|---|
| Pluggable Mailer (SMTP, with implicit-TLS/port-465 and STARTTLS support) | Built | `internal/mail.Mailer`; falls back to logging the message instead of sending when SMTP isn't configured, so registration/reset flows still work in dev without a real mail server |
| Email verification on registration | Built, blocking | `POST /register` sends a verification email; the account cannot sign in until `GET /verify-email` is completed |
| Self-service password reset email | Built | `POST /forgot-password` sends a reset link, consumed by `GET`/`POST /reset-password` |

---

## Machines currently defined

| Machine | Purpose | Fields | Constraints / Actions |
|---|---|---|---|
| `mch_user` | A person's participation in *this* Workspace | weekly capacity | `prm_edit_user_is_admin`/`prm_delete_user_is_admin` — changing the member directory is Workspace administration (`workspace_role: admin`). **Name, email and credential are all on the identity**, not here (migration 010, 2026-09-22): a full name belongs to whoever owns the email, so it is stated once by them and read by every Workspace they join. That makes these two rules narrower than they read, deliberately — an admin can change what someone declared *here* and can remove them from the Workspace, but cannot rename them or take over their login, because there is nothing here to edit rather than because of a carve-out. Display names resolve through `data.Store.MemberNames` (the membership row joins record to identity), which is also why this Machine's Field list no longer starts with a name and `Loader.RelationOptions` labels `person` options through that resolver instead of "the target's first Field" |
| `mch_project` | Project tracking | name, status, owner | `cst_project_done_no_open_tasks` |
| `mch_list` | Board columns | name | — |
| `mch_label` | Label catalog | name, color | — |
| `mch_task` | Task tracking, board layout | title, status, assignee, due date, priority, project, list, attachment | Board layout |
| `mch_card_label` | Task↔Label join | task, label | — |
| `mch_document` | Document approval workflow | title, document type, file, mode, status, due date, signed file | Aggregate status driven by its steps; `sla_field`; `fld_signed_file` written by the signature-compositing Action; `fld_document_type` |
| `mch_approval_step` | An approval step in a document workflow | document, sequence, step name, assignee, approver type, approver group, decision, signature page/x/y/width, signature image | `/decide` Action, sequencing enforced, `prm_decide_own_step`; `fld_signature_image` is a one-time signature captured at Approve time when its assignee chose not to save it as their own reusable `mch_signature` (2026-09-19); `fld_step_name` is what a step is *for* ("Finance Review") independent of who holds it — declared in Fase 6b for board 10's step titles, written by nothing until board 08's wizard in 6c, with `composition.stepLabel` falling back to the assignee's name meanwhile; `fld_approver_type`/`fld_approver_group` are CAP-F24's own pair (Fase 6c-1) — with `fld_assignee` serving as the User half, so there is no second person picker |
| `mch_activity` | Cross-machine event log | machine id, record id, summary, actor | Written by `logActivity`, never by a user form; **`append_only: true`** since 2026-09-21 — never changed or removed once written, by anyone |
| `mch_signature` | Approver's own reusable signature image | owner, image | `prm_create_own_signature`/`prm_edit_own_signature`/`prm_delete_own_signature`, all `roles: [approver]` + `actor_field: fld_owner` — your own signature, and only if you are someone who signs. A signature has exactly one consumer (`signatureImageFor`, at the moment its holder approves a step) and since 2026-09-21 only an approver decides, so a submitter's or reviewer's signature record would be an image nothing can ever use. This row read "deliberately no role" for a few hours; the owner overruled it, and the correction is the useful part — that argument was about signatures in general, the rule is about *this* Application. Input to the PDF-compositing Action; created by `decideButtons`' own canvas modal when its "save my signature" checkbox is checked (2026-09-19) -- previously only reachable through the generic Machine CRUD form |

---

## Metadata validation

| Rule | Scope | Enforced by |
|---|---|---|
| Stable-identity ID patterns (`mch_*`, `fld_*`, `cst_*`, `evt_*`, `prm_*`, `ws_*`, `app_*`) | Every declaration | `internal/metadata.Validate`/`validateConstraint`/`validateEvent`/`validatePermission` |
| Permission's `action` is one the runtime realizes, `actor_field` is a reference Field on the same Machine, no duplicate permission IDs | Per-Machine | `internal/metadata.validatePermission` |
| Known field type, no duplicate field IDs | Per-Machine | `internal/metadata.Validate` |
| `status` field requires at least one option | Per-Machine | `internal/metadata.Validate` |
| `default:` coerces to the Field's storage type at load time (bad coercion fails startup); a `status` Field's default must be one of its own options | Per-Machine | `internal/metadata.Parse`/`Validate` |
| Relation/Person target Machine must exist in the Application | Cross-Machine, once all loaded | `validateRelationTargets` |
| Constraint's related Machine/field must exist, and the related field must actually be a relation pointing back | Cross-Machine | `validateConstraintTargets` |
| Each declared View: id matches `^vw_[a-z][a-z0-9_]*$` and is unique within the Machine, `name` is non-empty (a viewer reads it when choosing), `type` is a known kind | Per-Machine | `internal/metadata.validateView` |
| A `board` View's `group_by` names a real field; a non-board View declaring `group_by` is rejected rather than ignored | Per-Machine | `internal/metadata.validateView` |
| A `cards` View's Machine must declare `card_fields` — a cards view over no projection would render empty cards forever | Per-Machine | `internal/metadata.validateView` |
| `sla_field` names a real field on the same Machine, and that field is a `date` | Per-Machine | `internal/metadata.Validate` |
| `card_fields` entries name real fields on the same Machine and carry a known role | Per-Machine | `internal/metadata.Validate` |
| Event declares exactly one of `on`/`on_create`; `when_equals` rejected when `on_create` is set (no prior value to compare against) | Per-Machine | `internal/metadata.validateEvent` |
| Record values: unknown field rejected, required field enforced, type-shape checked (status option, number, reference id, file key) | Every write | `internal/data.ValidateRecord` |
| Reference existence (does the referenced record actually exist) | Every write with a Relation/Person field | `internal/data.ValidateRelations` |

---

### Applications are installed per Workspace (2026-09-22)

`metadata/workspaces/<slug>.yaml` is one Workspace's installation manifest: the Workspace it is for
(named by **slug**, because the `ws_...` id is generated when a Workspace is created through the UI
and is not something a person can write down), the Machines that exist there, and the Applications
installed. The directory is scanned rather than indexed, so **installing is dropping a file in**
and uninstalling is deleting it. `applications: []` is valid — and is exactly what a Workspace
looks like the moment it is created.

It replaced a single process-wide `metadata/app.yaml`, loaded once at startup and handed to every
request. That arrangement meant a `workspaces` row scoped *records and membership* but not the
Applications, so every Workspace rendered the same ones. The owner found it the honest way: created
"Dokter Kecil", opened its Home, and saw two Applications nobody had installed in it.

Which Workspace a request is in is therefore now a per-request fact, resolved by
`internal/web.currentWorkspace` (session → `workspaces` row → slug → manifest) and carried on ctx
(`rendering.WithCurrentWorkspace`). A Workspace with no manifest resolves to the zero Workspace,
which is a real answer rather than an error. **The package-level `workspace` variable is gone**,
along with `ConfigureWorkspace` and its own admission that it was "not safe to call concurrently
with a request in flight"; `routeByID`/`labelByID` take `ctx` for the same reason.

**What this does not yet do:** a Workspace's *Machine set* is still process-wide, unioned across
every manifest, so All Machines lists every Machine in every Workspace. Their records are
workspace-scoped so the pages are empty, but the list itself is not per-Workspace yet.

---

## Navigation / Routes

| Route | Purpose |
|---|---|
| `GET /health` | Liveness check, no auth |
| `GET /login`, `POST /login` | Sign in (rate-limited, 10/5min, by address+email) |
| `GET /register`, `POST /register` | Self-service registration (rate-limited, 10/5min, by address) |
| `GET /verify-email` | Consume an email-verification link, blocking until confirmed |
| `GET /resend-verification`, `POST /resend-verification` | Re-send the verification email |
| `GET /forgot-password`, `POST /forgot-password` | Request a password-reset email (rate-limited, 10/5min, by address) |
| `GET /reset-password`, `POST /reset-password` | Consume a password-reset link and set a new password |
| `GET /choose-workspace`, `POST /choose-workspace` | Pick which Workspace to enter, for an identity with more than one |
| `GET /create-workspace`, `POST /create-workspace` | /switch-workspace's own "Add workspace" affordance (ui-sample/choose-workspace.html): create a new Workspace and become its Admin, without a new credential. Gated to an identity that is already Admin of at least one Workspace |
| `POST /logout` | Sign out |
| `GET /` | Machine list (landing page) |
| `GET /home` | Workspace Home landing page |
| `GET /dashboard` | Composed dashboard: Project+Task, Document status summary, Pending Approval, Recent Activity |
| `GET /my-tasks` | Personal work queue: Tasks assigned to the current identity, Today/Upcoming/Completed |
| `GET /activity` | Cross-Machine event feed, grouped by day |
| `GET /board-settings` | Lists/Labels catalog hub, linking to their own Machine pages |
| `GET /team-capacity` | Members' weekly capacity + real active/total Task counts |
| `GET /automation` | Read-only Trigger/Condition/Action view over real Constraint + Action |
| `GET /calendar` | Week-grid Layout, Tasks grouped by due date |
| `GET /sprint` | Sprint Dashboard: status summary, workload preview, attention-needed |
| `GET /approval-inbox` | Approval Inbox: one-screen worklist + detail, steps awaiting the current identity, SLA filter chips (`?filter=`), My Documents |
| `GET /api/approval-inbox/pending-count` | JSON count for the nav's pending-count badge |
| `GET /machines/{id}` | A Machine's own page (table or board) |
| `POST /machines/{id}/records` | Create a record |
| `GET /machines/{id}/records/{id}` | Record detail page (or a fragment, for HTMX) |
| `GET .../edit`, `PUT .../{id}`, `DELETE .../{id}` | Edit / update / delete a record |
| `POST .../{id}/decide` | Approve/Reject an Approval Step |
| `GET /documents/new`, `POST /documents` | Document submission wizard: create a Document and its own Approval Steps together |
| `GET /documents/new/approver-row` | The wizard's "+ Add approver" HTMX fragment — one more approver row, populated with real `mch_user` options |
| `GET`/`PUT /machines/mch_document/records/{id}/signature-placement` | Signature-coordinate placement screen, and its own write route (2026-09-21). The three forms on board 09 used to PUT to the *generic* record route, which rewrites a whole record from whatever is submitted — so every other Field had to be echoed back as hidden inputs or be erased, a list that forgot four Fields once and destroyed a one-time signature on the next drag. This route writes four named Fields and touches nothing else, so the echo is gone rather than merely correct. It also asks a different authorization question (`composition.MayPlaceSignature`): **the Document's own submitter, or the step's own approver** — board 09 is `STEP 2 OF 3` of the submit wizard, so the person laying the boxes out is the one submitting, while `mch_approval_step`'s edit Permission says only a step's own approver may change it. Gating the screen on the latter sent every submitter to a page where nothing moved |
| `GET /machines/mch_document/records/{id}/pdf-preview` | One page of the Document's PDF, rasterized to PNG |
| `GET /workspace-members`, `POST /workspace-members/invite`, `POST /workspace-members/revoke-invite`, `GET`/`POST /workspace-members/{id}/edit` | Workspace membership management, gated behind `requireWorkspaceAdmin`. The screen shows two lists: members, and **invitations not yet accepted** — separate because an invitation is not a membership (migration 011, 2026-09-22). Inviting takes an email and roles only, writes a `pending_invites` row, and creates no membership, no `mch_user` record and no role; all three come into existence when the invitee accepts (`submitAcceptInvite`), which is also where a brand-new person states their own full name. Revoking is a single delete, and the emailed token stops working immediately because acceptance re-checks the invitation still exists |
| `GET /workspace-groups`, `POST /workspace-groups`, `GET /workspace-groups/{id}`, `POST /workspace-groups/{id}/members`, `POST /workspace-groups/{id}/roles`, `POST /workspace-groups/{id}/delete` | Groups administration (Fase 4), same `requireWorkspaceAdmin` gate. **Absent from this table until Fase 7** — Fase 4 added the routes and not the row, which is the drift `TestNavigationRoutesAreRegistered` cannot see (it checks declared nav routes have handlers, never that a handler is written down here) |
| `GET /authorization-matrix` | The Authorization Matrix (`ui-sample/case-03-flow1/06b-authorization-matrix.html`): who can do what here, in two sections by *scope* — the Workspace, then one block per Application, every Application on the page at once (no selector). Restructured 2026-09-21 on owner request from a split by mechanism ("Transitions" / "Record actions"), which asked the reader to know what a transition is before they could find out what they may do and buried the two rows that matter under six nobody can ever be granted; those six are now one sentence per Machine. Rows are phrased in the words the screens use, each carrying its record-scoped arm underneath, and an action no rule governs is drawn granted-to-everyone **in amber** rather than hidden — in metadata "ungoverned" and "granted to all" are indistinguishable, which is how three of them stayed ungoverned. Read-only, metadata-only — no store call at all — and behind `requireWorkspaceAdmin` like the two rows above. Shipped 2026-09-21 as `/approval-role-matrix` (transitions only) and widened the same day, because a matrix of transitions alone is nearly empty under this app's approval model (ROADMAP.md's deferral table) while the actions it left out were the ones actually ungoverned |
| `GET /uploads/*` | Download an uploaded file |
| `GET /api/machines`, `GET /api/machines/{id}/records`, `POST /api/machines/{id}/records`, `PUT /api/machines/{id}/records/{id}`, `DELETE /api/machines/{id}/records/{id}` | JSON API — full CRUD parity with the HTML routes, including update and delete |
| `GET /ui-sample/*` | The static design mockups, served as-is from `ui-sample/` so a built page can be compared against the design it was built toward. Reference material, never current-code intent |

All routes except `/health` through `/choose-workspace` above sit inside the `requireAuth` group.
`/workspace-members*`, `/workspace-groups*` and `/authorization-matrix` additionally require
`requireWorkspaceAdmin`. **That middleware used to fail open** — an absent membership row was let
through, so the gate admitted whatever its own lookup could not identify. Narrowed 2026-09-21 to
the one configured identity it was written for (`config.AdminUserID`) and refusing everyone else.

Every route above is registered in `internal/web.Routes` and served by a handler in that package;
`cmd/server` builds the dependencies and mounts it. A per-handler size budget in
`internal/conformance` keeps a handler from quietly becoming a screen's worth of logic again.

Navigation itself **is** metadata, as of 2026-09-19: each installed Application's own `navigation:` list drives
`appShell`'s own Application menu row and mobile bottom bar (`domain.NavigationItem`,
`rendering.defaultAppMenu`) — adding or moving a link is a metadata edit, not a code edit. This
extends to a page's own in-content links to its sibling screens, not just the topbar: any Machine-
independent screen (Approval Inbox's "+ New Approval", the Document wizard's "← Approval Inbox",
Sprint Dashboard's "Full team capacity →") looks its target up by navigation item id via
`rendering.routeByID`, never a literal route string — `internal/conformance`'s
`TestRenderingHasNoHardcodedApplicationRoute`/`TestHandlersHaveNoHardcodedApplicationRoute` hold
every `.templ` and every `internal/web` handler (router.go's own registrations excepted) to that.
`routeByID` reads `domain.Application.AllNavigation` (the full declared list, before
`hidden_nav_groups` filtering) rather than the topbar's own filtered `Navigation`, so a link stays
resolvable even for an item whose group is currently hidden from the topbar. Its label-side
counterpart, `rendering.labelByID`, holds the same discipline for a page's own title/`<h1>`/card
text (`TestRenderingHasNoHardcodedApplicationLabel`/`TestHandlersHaveNoHardcodedApplicationLabel`)
— the label gate alone found violations across nine `.templ` files the first time it ran (three
more had already been found and fixed by hand while designing it), almost all a Page's own
page title retyping its nav item's `label:` right next to an already-correct
`routeByID` href.

---

## Diagnostics and self-enforcement

Mechanisms that observe or constrain the runtime rather than serve a screen. They are capabilities
like any other — and the two that prevent regression are the ones most easily forgotten.

| Capability | Status | Proven by |
|---|---|---|
| Per-request read diagnostics | Built | Every request logs `queries=N reads=M repeated=R <path>` plus a per-target breakdown — so a page's cost is a number, and the next duplicate-fetch forcing condition announces itself. `queries` is `data.QueryTracer`'s, counted at the pgx driver, so no Store method can forget to increment it; `reads` is the named half the breakdown lists, recorded by the Store methods themselves. A gap between them prints as `unnamed=`, because it means a statement was issued by something that did not name itself. A line whose own counts say something is wrong with it is prefixed `ANOMALY(...)` so a periodic review is a grep rather than a read — `unnamed` always, `repeated` on GET/HEAD only, since `composition.Loader` may not span a write and a POST re-reading after its own mutation is correct. **This row overstated itself until 2026-09-22** — it claimed "every request", while `web.queryDiagnostics` was registered below four middlewares issuing six queries between them and counted none of them; the driver-level count and `TestQueryDiagnosticsRunsBeforeAuth`/`TestPoolInstallsQueryTracer` are what make the claim true rather than merely written. This is the *observable-reads* half of 001 §6; the *inference* half is still missing (see the limits below) |
| Response compression (static assets only) | Built | `middleware.Compress` on the `/css/*`, `/vendor/*` and `/icons/*` routes alone (`internal/web/router.go`) — `hyperscript.min.js` 369→67kB, `htmx.min.js` 51→16kB, `app.css` 30→6.5kB, about **359kB off a cold load**. **HTML is deliberately not compressed** (owner decision, 2026-09-22): every page carries the CSRF token in its own markup (`hx-headers`, security audit L1), and a secret in a compressed response beside content an attacker can influence is the BREACH precondition — HTML is worth 2.6kB/page against that, so the static assets are ~99% of the win without having to answer it. Revisit alongside per-response token masking. No `Cache-Control` yet: the filenames carry no content hash, so a long max-age would serve stale assets after a deploy |
| Import-boundary conformance | Built | `internal/conformance` encodes each package's `doc.go` contract as a test: the Domain Plane imports nothing physical, the Experience Plane reaches no database, `internal/db` carries no metadata semantics. `TestEveryPackageHasARule` fails for any new package under `internal/` or `cmd/` that declares no boundary |
| Handler size budget | Built | A measured per-handler line limit in `internal/conformance`, so a handler cannot quietly become a screen's worth of logic again |
| Volume threshold harness | Built | `make threshold` (`internal/composition/threshold_test.go`) measures whole-Machine read cost at increasing record counts — the measured trigger for pagination/projection work |
| Navigation route existence, cross-checked | Built (2026-09-19) | `internal/conformance.TestAppManifestLoads`/`TestNavigationRoutesAreRegistered`: every Workspace manifest loads and validates (no database needed), and every declared `navigation:` route has a matching `internal/web/router.go` `GET` handler — a typo now fails `go test`/pre-commit, not silently at click time |
| Every page's links stay metadata-driven, not hand-typed | Built (2026-09-19) | `internal/conformance.TestRenderingHasNoHardcodedApplicationRoute` / `TestHandlersHaveNoHardcodedApplicationRoute`: no `internal/rendering/*.templ` file and no `internal/web/*.go` handler (router.go's own registrations excepted) may hardcode a literal equal to a declared `navigation:` route, unless it's one of the small set of runtime-level routes (`/home`, `/login`, ...) that exist regardless of which Application is configured. An Application's own route must come from `rendering.routeByID(id)` (reads `domain.Application.AllNavigation`) or an equivalent `domain.Application` field (`HomeRoute`, `PrimaryNavGroup`) threaded through `web.Deps`, checked at both the rendering and handler layer so the obligation can't just move down one level. CLAUDE.md's "Where a metadata-derived value belongs" is the convention this enforces |
| Every page's own title/text stays metadata-driven, not hand-typed | Built (2026-09-19) | `internal/conformance.TestRenderingHasNoHardcodedApplicationLabel` / `TestHandlersHaveNoHardcodedApplicationLabel`: the label-side counterpart of the route gate above — no `.templ` file or handler may hardcode a literal equal to a declared `navigation:` label. A page's own title/`<h1>`/card text must come from `rendering.labelByID(id)` instead. Found across nine `.templ` files the day it was added (writing-guide.md §2, capabilities.md's "Navigation / Routes" section above) |
| Projection adoption, ratcheted | Built (2026-09-19) | `internal/conformance.TestRenderingUsesProjectionNotRawValues`: no `.templ` may read a named field off a record (`Values["fld_..."]`, or via a Machine-specific `action.Field*` constant — both forms gated, so the violation can't move down one level into a clean-looking variable). Unlike every other gate here it states an invariant that does *not* hold yet: **seven** files are grandfathered in `projectionRatchet` and the list may only shrink, failing both on a new violator and on a stale entry left behind after a migration — it started at ten, and `detail.templ` (Fase 6b), `documentsubmit.templ` (6c-2) and `signatureplacement.templ` (6c-3) have left it; the six Case 19 composed screens and `approvalstepper.templ` remain. **What the ratchet does not measure**, and what a shrinking count must not be read as: it gates *reads* (`Values["fld_…"]`), never *writes* — `signatureplacement.templ` left the list still carrying twelve hardcoded `name={ action.Field… }` form bindings, which are 007 §11.3 Binding, a different variation point with no primitive here yet. It exists because `card_fields` shipped its mechanism and stalled with every test green — *adoption* was the one dimension nothing measured. **How complete that stall was is worth keeping on the record:** until 2026-09-20 no Machine declared `card_fields` at all, so `composition.ProjectCardFields` resolved an empty list for every record and `approvalinbox.templ`'s own projected branch had never executed — Projection was wired end to end and had run zero times. Declaring it on `mch_approval_step` would not have fixed that honestly either: its only projectable Fields under the five known roles are `fld_assignee` and `fld_decision`, both constant across a list that is by definition *this viewer's still-pending steps*. **Projection first ran on 2026-09-20**, when `type: cards` gave it a consumer whose output varies per record (`mch_document`'s `vw_document_cards`) — which is also why `internal/metadata.validateView` refuses a cards View on a Machine declaring no `card_fields`, so the stall cannot recur one View at a time. The ratchet stays: one screen projecting is adoption starting, not adoption done (`menata-app-document`'s `audits/2026-09-19-decomposition-maturity-audit.md` §5) |
| `capabilities.md`'s own inventory tables stay honest | Built (2026-09-19) | `internal/conformance.TestCapabilitiesMachinesTableMatchesMetadata` / `...ComponentsTableMatchesTempl`: the "Machines currently defined" table is cross-checked against `metadata/*.yaml` (both directions — undocumented and stale entries both fail), and every "Shared rendering components" row names a real `templ` function in `internal/rendering/*.templ` |

---

## Known architectural limits

Current-state facts, the same job as the rest of this file — not a plan. Each one is a place where
a stated obligation in 001-007 is not met today.

| Limit | What that means concretely |
|---|---|
| Inference is not inspectable (001 §6) | `person`→`mch_user`, child collections, board columns and the default table Layout are all inferred, and nothing can show the resolved result — no diagnostics route, no `--explain`, no normalized-Application dump |
| No metadata versioning or change classification (004, 005) | No version key anywhere; deleting a Field from a `*.yaml` silently orphans that field's data inside every record's JSONB, with no migration decision |
| Every read is a whole-Machine read (007 §21.1, §28) | `ListRecords`/`ListRecordsBy`/`GetRecord` only — no projection, filter pushdown, pagination or limit; pages reduce whole record sets in Go |
| Metadata loads once, at startup (005 §Hot Reload) | A metadata change takes effect on restart. Principle #10 (no source regeneration) is met; hot reload is a deliberate, triggered deferral |
| Navigation is not per-user/role-filtered (006 §Navigation, 007 §20) | Every Application's and the Workspace's own navigation is resolved once at process startup into package-level state (`internal/rendering.ConfigureWorkspace`), identical for every request/viewer; `domain.NavigationItem` has no role/permission field. **This row used to describe a viewer-level stand-in (`appShell`'s `hiddenNavIDs`, naming `nav_workspace_members` for a plain member) -- that parameter was deleted 2026-09-21 with the launcher's last caller of it (see "Navigation: one filter now, not two" above), so there is no stand-in left, declared or otherwise.** `nav_workspace_members` is a declared item pointing at a `requireWorkspaceAdmin`-gated route; the launcher no longer lists individual destinations at all (it lists Applications), so there is nothing there left to hide. Workspace Home's own "Manage members" link still keeps its own inline `workspaceRole != "member"` check (`workspacehome.templ`) -- one page-local condition, not the declared form. `requires_role:` on a navigation item stays unbuilt, waiting on a real case (ROADMAP.md, Planned) |
| Unknown metadata keys are ignored, not rejected (005 Phase 3, "invalid metadata must not reach compilation") | The three `yaml.Unmarshal` call sites in `internal/metadata` decode without `KnownFields`, so a key the parser has no home for is dropped in silence. Everything the runtime *knows* is validated strictly (see "Metadata validation" above), which is what makes the hole easy to miss: a retired or misspelled key produces a Machine that loads cleanly and simply lacks the capability. Verified 2026-09-20 by loading a copy of `metadata/` whose `mch_document` used the pre-2026-09-20 singular `view:` block — `LoadApplication` returned no error and the Machine had zero Views |
| Write-side Binding has no primitive (007 §11.3) | A form input's `name=` is a metadata id chosen by the page: `machine.templ` does it generically (`name={ f.ID }` over `m.Fields`), but the two bespoke Case 3 screens hardcode it — `signatureplacement.templ` twelve times, `documentsubmit.templ` six. `TestRenderingUsesProjectionNotRawValues` does not see this: it gates reads, not writes, so a file can leave the ratchet with every binding still hand-typed |
| Design mockups ahead of the build | The `ui-sample/` mockups describe screens, data, and a platform shell this app does not fully have yet — a project workspace overview and per-project scoping, a richer task detail body (description/checklist/comments), board drag-and-drop, and the rest of the Workspace/Group/Role platform shell (Group-derived roles, per-Application role rows). Registration/login/Workspace, Application plurality and the cross-app launcher, the document-type field, and the one-screen worklist+detail layout for approvals are already built |
| Security page has no session list or 2FA (Account menu port, 2026-09-21) | `/account-security` ships change-password and "sign out of other devices" only. `internal/authorization`'s session model is one signed cookie plus a generation counter per `mch_user` record — no per-device/browser/location tracking exists to back a real session list, and no 2FA primitive exists either; both stay marked "(planned)" rather than fabricated |
