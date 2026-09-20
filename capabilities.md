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
| `file` | Local-disk upload; value is a storage key, original filename embedded in it; content sniffed and script-capable uploads rejected before `storage.Save` (security audit 2026-09-19, H2) | Built | `mch_document.fld_file`, `mch_task.fld_attachment`; `internal/web/record_test.go`'s `TestHandleFileUploads_rejectsScriptCapableContent`/`TestHandleFileUploads_allowsRealPDF` prove the content check |

Closed set, extended deliberately — `internal/domain.KnownFieldTypes` — not inferred from data
(007 §14's static-registry seam).

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
| Dataset + Dimension + Measure | A named semantic data definition over one Machine's records (007 §7.2): an optional `dimension` to group by (§7.3), and one or more `measures` that count records or sum a number Field (§7.4), each optionally filtered by a `where` predicate | Built (minimal shape: `count`/`sum`) | `mch_task`'s `ds_task_workload` (count + filtered count, grouped by `fld_assignee`) and `mch_user`'s `ds_user_capacity` (a grand-total sum) compose the whole Team Capacity screen. `domain.Dataset`/`Measure`/`KnownAggregates`, `composition.Aggregate` (pure, no database), `internal/metadata.validateDataset`. Generalized from the same "count records, grouped by a field, optionally filtered" loop found hand-written five separate times in `internal/composition/pages.go` — well past B1, per `menata-app-document`'s `audits/2026-09-19-decomposition-maturity-audit.md` §3.2. `where` deliberately reuses the existing `expression.Comparison` a Constraint's `block_if.condition` already declares rather than a second filter syntax (a B3 pass: the existing primitive already fit). Verified live: changing only `msr_active`'s `where.value` in YAML moved Team Capacity's own "Active cards" from 3 to 2 with no code touched. 007 §7.4 names four more aggregates (`avg`/`min`/`max`/`count_distinct`) — not realized, waiting on a real case, the same discipline that kept `KnownActions` at one entry |
| Action | A named business operation beyond a plain field write, with cross-record side effects | Built (one shape, hardcoded) | `POST .../decide` — Approve/Reject, `internal/action`; the closed set is `domain.KnownActions` |
| Permission | Record-scoped authorization on an Action: the acting identity must be the value of a declared `actor_field` | Built (one shape, governs `decide`/`edit`/`delete`) | `mch_approval_step`'s `prm_decide_own_step`/`prm_edit_own_step`/`prm_delete_own_step`, `authorization.AllowsAction` |
| Ordered Lists | A Machine's records used as a board's real, renameable, reorderable columns (replaces a fixed status enum) | Built | `mch_list`, grouping `mch_task`'s board |
| Sort order | Explicit per-Machine ordering (`sort_order` column), assigned at create time | Built | Every Machine — `internal/data.Store.CreateRecord` |
| SLA badge | A `view.sla_field` date Field rendered as OVERDUE / "N day(s) left" instead of a plain date | Built | `mch_document`'s `fld_due_date`, `internal/experience.EvaluateSLA` |
| Activity log | Append-only event record, written as a plain Machine (not a new DataSource kind), on a triggering write | Built | `mch_activity`, `internal/web`'s `logActivity` — Document submission/decision, Task/Project creation, SLA breach (`internal/composition`), and (2026-09-19, no longer hardcoded) Task status moves via the declared `evt_task_status_changed` Event above |
| Field default value | A Field's declared `default:` fills in a value a create leaves empty (absent, nil, or `""`) — create-only, never re-applied on update | Built | `mch_task.fld_status: default: todo`; `domain.Field.Default`, `data.ApplyDefaults`, called from every create path |
| Workspace scoping | `records.workspace_id`, carried on `context.Context` (not a `Store` struct field, which was tried and reverted for leaking data across a per-request scope) and enforced on every read/write; a tampered `workspace_id` is rejected | Built | `data.WithWorkspaceScope`, `migrations/004_workspaces.sql`. Still single-Application per Workspace; Application plurality is not yet built |

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
| `slaBadge` | A `view.sla_field` date as OVERDUE / "Due today" / "N day(s) left" | `machine.templ`; used by `RecordRow` and `RecordDetailView` |
| `summaryCounts` | A row of labelled count tiles | `machine.templ`; Dashboard, Sprint Dashboard, Team Capacity, Approval Inbox |
| `activityFeedList` | `ActivityEntry` rows as one feed shape | `machine.templ`; Dashboard, Activity |
| `sectionHeader` | A composed page's section title + optional "View all →" link | `machine.templ` |
| `recordSummaryCard` / `summaryCardList` | A record as a card face rather than a table row | `machine.templ`; Approval Inbox's My Documents |
| `pendingApprovalCard` | Approval Inbox's own Pending-my-approval grid card (document-approval.html): SLA framing, Mode/approval-ratio badge, submitted-by/date line, per-step progress dots | `approvalinbox.templ`; Approval Inbox's worklist only — page-internal, not yet reused elsewhere |
| `filterChip` | A query-param filter chip with its own count (no JS) | `approvalinbox.templ` |
| `approvalStepper` | A Document's own steps as done / current / waiting | `approvalstepper.templ` |
| `pageShell` | The page frame and the topbar — a projection of `app.yaml`'s own `navigation:` list (2026-09-19), grouped and collapsible, with a live pending-approval-count badge and `aria-current="page"`; see the navigation limit below for what's still out of scope | `machine.templ` |
| `pdfThumbnail` | A Document's own PDF, page 1, as a small linked preview image | `detail.templ`; the Document detail page (reuses the `.../pdf-preview` route, no new route) |
| `detailBackLink` | A record detail page's own "← back" link: `routeByID("nav_approval_inbox")` for a Document/Approval Step (2026-09-19 -- their generic Machine page is POC scaffolding, not a real destination), the generic Machine page for every other Machine | `detail.templ`; `RecordDetailPage` |
| `signatureConfirmation` | Echoes a pending Approval Step's own placement back to its assignee before they decide, or prompts them to place one | `detail.templ`; `decideButtons`, from the step record already fetched for that page -- no extra query |
| `signatureModal` | The canvas `<dialog>` `decideButtons` opens when Approve has no signature on file yet (2026-09-19) -- draw, optionally save, submit as part of the same `hx-post .../decide` | `detail.templ`; `decideButtons`, shown only when `hasSignature` is false |
| `signaturePlacementBlock` | The drag/place/width Component itself (document, steps, relations, page, totalPages -- a bounded contract, 007 §12.3), factored out of `SignaturePlacementPage` so a second Page could compose it too (2026-09-19) -- the composable-runtime kajian's own Fase 0, promoted here now that a second, genuinely different Page needs the identical shape, per this table's own promotion criterion | `signatureplacement.templ`; `SignaturePlacementPage` and `detail.templ`'s `RecordDetailView` (Document's own detail page, gated by `DocumentSignaturePlacement != nil`, fail-open on a PDF read error) |
| `fieldInput` | One Field's own input control, chosen by `domain.FieldType` (text/number/date/status/relation/file/...) | `machine.templ`; `RecordEditRow`, `createFormRow`, and `detail.templ`'s `RecordDetailEdit` |
| `fileLink` | A `file` Field's stored value rendered as a download link | `machine.templ`; `RecordRow`, and `detail.templ`'s `RecordDetailView` |
| `csrfHiddenInput` | The CSRF double-submit hidden `<input>` every state-changing form carries | `machine.templ`; every pre-auth form (`login.templ`, `register.templ`, `forgotpassword.templ`, `resetpassword.templ`, `resendverification.templ`, `chooseworkspace.templ`), `workspacemembers.templ`, and `machine.templ`'s own generic create/edit rows |

**Not yet built:** Timeline Layout; colored label chips and member avatars on a card face;
drag-and-drop reordering of board columns and cards. The full list of what the `ui-sample/`
mockups show and this app does not build yet is tracked internally.

---

## Authorization

| Capability | Status | Notes |
|---|---|---|
| Session-cookie auth, gating every route except `/health`, `/login`, `/register`, `/verify-email`, `/resend-verification`, `/forgot-password`, `/reset-password`, `/choose-workspace` | Built | `internal/authorization`, HMAC-signed cookie; `requireAuth` group in `internal/web.Routes` |
| Real identity resolution (session names an actual `mch_user` record) | Built | Per-user — no longer `config.AdminUserID`; `requireAuth`'s Workspace fallback for a non-user subject is `DefaultWorkspaceID` |
| Per-user login (distinct bcrypt-hashed passwords per person) | Built | `internal/auth`; registration (`POST /register`), blocking email verification, real per-user credential storage |
| Self-service password reset | Built | `GET`/`POST /forgot-password`, `GET`/`POST /reset-password` — `internal/web/passwordreset.go` |
| Workspace membership + invite, gated to Workspace admins | Built | `GET /workspace-members`, `POST /workspace-members/invite`, `GET`/`POST /workspace-members/{id}/edit` — `requireWorkspaceAdmin` |
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
| `mch_user` | Real identity | name, email, weekly capacity | — |
| `mch_project` | Project tracking | name, status, owner | `cst_project_done_no_open_tasks` |
| `mch_list` | Board columns | name | — |
| `mch_label` | Label catalog | name, color | — |
| `mch_task` | Task tracking, board layout | title, status, assignee, due date, priority, project, list, attachment | Board layout |
| `mch_card_label` | Task↔Label join | task, label | — |
| `mch_document` | Document approval workflow | title, document type, file, mode, status, due date, signed file | Aggregate status driven by its steps; `view.sla_field`; `fld_signed_file` written by the signature-compositing Action; `fld_document_type` |
| `mch_approval_step` | An approval step in a document workflow | document, sequence, assignee, decision, signature page/x/y/width, signature image | `/decide` Action, sequencing enforced, `prm_decide_own_step`; `fld_signature_image` is a one-time signature captured at Approve time when its assignee chose not to save it as their own reusable `mch_signature` (2026-09-19) |
| `mch_activity` | Cross-machine event log | machine id, record id, summary, actor | Written by `logActivity`, never by a user form |
| `mch_signature` | Approver's own reusable signature image | owner, image | Input to the PDF-compositing Action; created by `decideButtons`' own canvas modal when its "save my signature" checkbox is checked (2026-09-19) -- previously only reachable through the generic Machine CRUD form |

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
| `view.layout` known, `view.group_by` names a real field | Per-Machine | `internal/metadata.Validate` |
| `view.sla_field` names a real field on the same Machine, and that field is a `date` | Per-Machine | `internal/metadata.Validate` |
| Event declares exactly one of `on`/`on_create`; `when_equals` rejected when `on_create` is set (no prior value to compare against) | Per-Machine | `internal/metadata.validateEvent` |
| Record values: unknown field rejected, required field enforced, type-shape checked (status option, number, reference id, file key) | Every write | `internal/data.ValidateRecord` |
| Reference existence (does the referenced record actually exist) | Every write with a Relation/Person field | `internal/data.ValidateRelations` |

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
| `GET /machines/mch_document/records/{id}/signature-placement` | Signature-coordinate placement screen |
| `GET /machines/mch_document/records/{id}/pdf-preview` | One page of the Document's PDF, rasterized to PNG |
| `GET /workspace-members`, `POST /workspace-members/invite`, `GET`/`POST /workspace-members/{id}/edit` | Workspace membership management, gated behind `requireWorkspaceAdmin` |
| `GET /uploads/*` | Download an uploaded file |
| `GET /api/machines`, `GET /api/machines/{id}/records`, `POST /api/machines/{id}/records`, `PUT /api/machines/{id}/records/{id}`, `DELETE /api/machines/{id}/records/{id}` | JSON API — full CRUD parity with the HTML routes, including update and delete |
| `GET /ui-sample/*` | The static design mockups, served as-is from `ui-sample/` so a built page can be compared against the design it was built toward. Reference material, never current-code intent |

All routes except `/health` through `/choose-workspace` above sit inside the `requireAuth` group.
`/workspace-members*` additionally requires `requireWorkspaceAdmin`.

Every route above is registered in `internal/web.Routes` and served by a handler in that package;
`cmd/server` builds the dependencies and mounts it. A per-handler size budget in
`internal/conformance` keeps a handler from quietly becoming a screen's worth of logic again.

Navigation itself **is** metadata, as of 2026-09-19: `app.yaml`'s own `navigation:` list drives
`rendering.pageShell`'s topbar (`domain.NavigationItem`, `experience.GroupNavigation`,
`rendering.navSections`) — adding or moving a link is a metadata edit, not a code edit. This
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
`pageShell(...)` title retyping its nav item's `label:` right next to an already-correct
`routeByID` href. See the architectural limits below for what this doesn't cover (the
cross-Application launcher).

---

## Diagnostics and self-enforcement

Mechanisms that observe or constrain the runtime rather than serve a screen. They are capabilities
like any other — and the two that prevent regression are the ones most easily forgotten.

| Capability | Status | Proven by |
|---|---|---|
| Per-request read diagnostics | Built | Every request logs `reads=N repeated=M <path>` plus a per-target breakdown, from `internal/data`'s own read paths (`data.ReadLog`, `web.queryDiagnostics`) — so a page's cost is a number, and the next duplicate-fetch forcing condition announces itself. This is the *observable-reads* half of 001 §6; the *inference* half is still missing (see the limits below) |
| Import-boundary conformance | Built | `internal/conformance` encodes each package's `doc.go` contract as a test: the Domain Plane imports nothing physical, the Experience Plane reaches no database, `internal/db` carries no metadata semantics. `TestEveryPackageHasARule` fails for any new package under `internal/` or `cmd/` that declares no boundary |
| Handler size budget | Built | A measured per-handler line limit in `internal/conformance`, so a handler cannot quietly become a screen's worth of logic again |
| Volume threshold harness | Built | `make threshold` (`internal/composition/threshold_test.go`) measures whole-Machine read cost at increasing record counts — the measured trigger for pagination/projection work |
| Navigation route existence, cross-checked | Built (2026-09-19) | `internal/conformance.TestAppManifestLoads`/`TestNavigationRoutesAreRegistered`: `metadata/app.yaml` loads and validates (no database needed), and every declared `navigation:` route has a matching `internal/web/router.go` `GET` handler — a typo now fails `go test`/pre-commit, not silently at click time |
| Every page's links stay metadata-driven, not hand-typed | Built (2026-09-19) | `internal/conformance.TestRenderingHasNoHardcodedApplicationRoute` / `TestHandlersHaveNoHardcodedApplicationRoute`: no `internal/rendering/*.templ` file and no `internal/web/*.go` handler (router.go's own registrations excepted) may hardcode a literal equal to a declared `navigation:` route, unless it's one of the small set of runtime-level routes (`/home`, `/login`, ...) that exist regardless of which Application is configured. An Application's own route must come from `rendering.routeByID(id)` (reads `domain.Application.AllNavigation`) or an equivalent `domain.Application` field (`HomeRoute`, `PrimaryNavGroup`) threaded through `web.Deps`, checked at both the rendering and handler layer so the obligation can't just move down one level. CLAUDE.md's "Where a metadata-derived value belongs" is the convention this enforces |
| Every page's own title/text stays metadata-driven, not hand-typed | Built (2026-09-19) | `internal/conformance.TestRenderingHasNoHardcodedApplicationLabel` / `TestHandlersHaveNoHardcodedApplicationLabel`: the label-side counterpart of the route gate above — no `.templ` file or handler may hardcode a literal equal to a declared `navigation:` label. A page's own title/`<h1>`/card text must come from `rendering.labelByID(id)` instead. Found across nine `.templ` files the day it was added (writing-guide.md §2, capabilities.md's "Navigation / Routes" section above) |
| Projection adoption, ratcheted | Built (2026-09-19) | `internal/conformance.TestRenderingUsesProjectionNotRawValues`: no `.templ` may read a named field off a record (`Values["fld_..."]`, or via a Machine-specific `action.Field*` constant — both forms gated, so the violation can't move down one level into a clean-looking variable). Unlike every other gate here it states an invariant that does *not* hold yet: ten files are grandfathered in `projectionRatchet` and the list may only shrink, failing both on a new violator and on a stale entry left behind after a migration. It exists because `view.card_fields` shipped as a one-screen pilot and stalled there with every test green — *adoption* was the one dimension nothing measured (`menata-app-document`'s `audits/2026-09-19-decomposition-maturity-audit.md` §5) |
| `capabilities.md`'s own inventory tables stay honest | Built (2026-09-19) | `internal/conformance.TestCapabilitiesMachinesTableMatchesMetadata` / `...ComponentsTableMatchesTempl`: the "Machines currently defined" table is cross-checked against `metadata/*.yaml` (both directions — undocumented and stale entries both fail), and every "Shared rendering components" row names a real `templ` function in `internal/rendering/*.templ` |

---

## Known architectural limits

Current-state facts, the same job as the rest of this file — not a plan. Each one is a place where
a stated obligation in 001-007 is not met today.

| Limit | What that means concretely |
|---|---|
| Inference is not inspectable (001 §6) | `person`→`mch_user`, child collections, board columns and the default table Layout are all inferred, and nothing can show the resolved result — no diagnostics route, no `--explain`, no normalized-Application dump |
| No metadata versioning or change classification (004, 005) | No version key anywhere; deleting a Field from a `*.yaml` silently orphans that field's data inside every record's JSONB, with no migration decision |
| Cross-Application launcher not built (004, 006) | The per-Application menu half of Navigation is now a declared metadata schema (`app.yaml`'s `navigation:`, closed 2026-09-19), but the cross-Application launcher and mobile bottom bar from `ui-sample/README.md`'s two-level nav are not — there is nothing to launch *between* with only one real Application yet. (Route existence in `navigation:` *is* now cross-checked, closed 2026-09-19 — see Diagnostics and self-enforcement above; this limit is about the launcher UI, not route validation.) |
| Every read is a whole-Machine read (007 §21.1, §28) | `ListRecords`/`ListRecordsBy`/`GetRecord` only — no projection, filter pushdown, pagination or limit; pages reduce whole record sets in Go |
| Metadata loads once, at startup (005 §Hot Reload) | A metadata change takes effect on restart. Principle #10 (no source regeneration) is met; hot reload is a deliberate, triggered deferral |
| Navigation is not per-user/role-filtered (006 §Navigation, 007 §20) | `application.navigation` (`app.yaml`) is resolved once at process startup into package-level state (`internal/rendering.ConfigureNavigation`), identical for every request/viewer; `domain.NavigationItem` has no role/permission field. No declared nav item needs this yet — the one concrete link-level gap found (Workspace Home's "Members" link, shown to every viewer though `/workspace-members*` is already admin-only server-side) lives in workspace-level chrome, not `application.navigation`, and was fixed directly (`workspaceHomeShell`'s `isAdmin` parameter) without this rework. Building per-request nav filtering for zero current call sites would be speculative; tracked here for when a real one exists |
| Design mockups ahead of the build | The `ui-sample/` mockups describe screens, data, and a platform shell this app does not fully have yet — a project workspace overview and per-project scoping, a richer task detail body (description/checklist/comments), board drag-and-drop, and most of the Workspace/Group/Role platform shell (Application plurality, the cross-app launcher, Group-derived roles). Registration/login/Workspace (single-Application), the document-type field, and the one-screen worklist+detail layout for approvals are already built |
