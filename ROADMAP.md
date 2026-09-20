# Menata App Roadmap

A high-level view of what menata-app can do today and what's coming next, kept at the feature
level on purpose. It is not the same document as the root-level concept docs (001-007), which
describe the target architecture. The full day-to-day development history -- design rationale,
forcing conditions, verification steps -- is tracked in a private companion repository,
`menata-app-document`.

## Shipped

- **Metadata-driven CRUD** -- define a data model ("Machine") in YAML and get a working table or
  board view, create/edit/delete, relations (one-to-many and many-to-many), nested child
  collections, and file attachments, with no per-model code required.
- **Multi-user accounts** -- registration (creates a workspace), email verification, login,
  password reset, and session-based authentication.
- **Workspaces & members** -- invite teammates, assign per-application roles, a workspace home
  screen.
- **Document Approval** -- submission wizard, sequential or parallel multi-step approval, SLA
  tracking, an approval inbox and dashboard, an activity feed, drag-to-place PDF signatures, and
  automatic signature compositing onto the approved document.
- **Project Management** (core mechanics shipped, still rounding out) -- task boards with labels
  and ordered lists, my-tasks view, calendar, sprint dashboard, team capacity view, activity feed.
- **Grouped navigation**, declared as metadata rather than hardcoded per page.
- **Continuous architecture and code-quality checks** running in CI on every change.
- **Authentication and file-handling hardening**, based on an internal security review --
  invite-acceptance and session handling, upload validation and access checks, security response
  headers, and CSRF protection.
- **Permission-aware action visibility** -- Edit/Delete/Save buttons, and an Approval Step's own
  signature-marker drag controls, follow the current user's authorization the same way
  Approve/Reject already did: hidden (not just disabled) when the backend's own
  `authorization.AllowsAction` would refuse the action, declared per Machine
  (`mch_approval_step`'s `prm_edit_own_step`/`prm_delete_own_step`, its own assignee). A Machine
  declaring no such Permission stays open to any authenticated Workspace member, unchanged. The
  Workspace Home "Members" link is likewise hidden from non-admins now, matching the
  `requireWorkspaceAdmin` gate the destination already enforced. Not yet covered: `application.navigation`
  role-filtering (below) and declaring this Permission on any Machine besides Approval Step, which
  is a business-rule decision, not assumed here.
- **Event primitive** -- the first real declaration of 006-runtime-model.md's Behavioral Model
  (`Event → Action → Permission/Constraint → Service/Data operation → State change`): a Field
  changing (optionally to one target value), or a record being created, runs a closed,
  runtime-owned Service purely from metadata (`domain.Event`/`domain.Service`,
  `behavior.MatchedEvents`/`MatchedCreateEvents`, `internal/web`'s `runEvents`/`runCreateEvents`;
  `capabilities.md`, `writing-guide.md` §10). Proven by replacing what used to be hardcoded Go
  (Task status move logging) with a declared `mch_task.evt_task_status_changed`, and verified
  live: editing only the YAML wording changes the Activity feed's own text, no code touched.
  `/automation` now lists Events alongside Constraints. Extended 2026-09-19 to a second shape,
  `on_create: true`, generalizing what was `internal/web`'s own hardcoded `logRecordCreated`
  switch (`mch_document`/`mch_task`/`mch_project` record-creation logging) -- a real third case
  proven before the shape was built, per `menata-app-document`'s
  `workflow-behavior-decomposition-criteria.md`. The Document-approval decide cascade and
  SLA-breach detection (which still fires on a page read, since there's no scheduler) remain
  hardcoded, on purpose -- named next candidates once a real second need reaches them, not
  converted speculatively.
- **Metadata-hardcoding gate extended to `label:`, not just `route:`** -- a Page's own title
  (`pageShell(...)`, `<h1>`, card/back-link text) must now come from `rendering.labelByID(id)`
  (`internal/rendering/machine.templ`), the label-side counterpart of the existing `routeByID`,
  gated by `internal/conformance.TestRenderingHasNoHardcodedApplicationLabel`/
  `TestHandlersHaveNoHardcodedApplicationLabel`. Found real drift across twelve `.templ` files
  before it existed -- nine caught by the test itself the first time it ran, three more (including
  `documentsubmit.templ`, `workspacehome.templ`) found and fixed by hand while designing it -- every
  case the same shape: a page's title retyped on the line right next to an already-correct
  `routeByID` href. `CLAUDE.md`'s new "Deciding whether a literal is a metadata-hardcoding
  violation" section generalizes the underlying question beyond routes/labels; full kajian in
  `menata-app-document`'s `audits/2026-09-19-metadata-hardcoding-gate-mapping.md`.

## In progress

- **Document Approval is the current proof-of-concept scope** (owner decision, 2026-09-20), with
  an explicit constraint attached: narrowing the scope must not become a licence to hardcode for
  it. Its work is broken into four screens plus three behaviours, each tracked on two independent
  axes -- does the feature exist at all, and is it composable -- so a screen can never be reported
  finished on the strength of the first axis alone. Three of those behaviours (approval-step
  sequencing, document status rollup, PDF signature compositing) are hand-written Go here while
  the upstream capability registry already carries them as admitted, built, conformance-tested
  rows, so the work is to implement those capabilities rather than to re-decide them. Breakdown
  and running order: `menata-app-document`'s `case-03-composability-checklist.md`.
- **Porting the Case 03 Flow 1 design** (owner mockup, 2026-09-20, unpacked into
  `ui-sample/case-03-flow1/` — twelve desktop boards at 1280px, ten mobile at 390px, two of them
  marked TIDAK DIPAKAI). The gap study behind the sequence below found three things worth
  recording, because each one contradicts an assumption that looked safe: the app was **not**
  actually using Tailwind (`static/vendor/tailwind.play.js` was loaded only by the `ui-sample`
  mockups); the boards' palette **is** Tailwind's own default slate/blue scale, so the color
  tokens came free; and board 06's explanatory copy cites a `ProcessEdge` primitive and a
  `/{machineID}/process-map` route that **exist nowhere in this repo**, while the word "role"
  appears nowhere in `006`/`007` either — the Permission shape that does exist is record-scoped
  (`actor_field`), which cannot express "Finance Reviewer may approve the Finance transition".
  Owner decisions, 2026-09-20: Tailwind enters via a **CLI build step** (not the dev-only Play
  build), the declared-View gate below is **decided before** the composed screens are ported, and
  board 06 is built **in full**, role-based Permission and transition model included. Seven
  phases, in dependency order:
  1. **Design system + Tailwind pipeline** — *shipped 2026-09-20*. See `capabilities.md`'s
     "Styling: two systems, on purpose". Boards 01 and 02 are ported, and the `authShell` kit
     replaced the seven near-identical `<style>` blocks the pre-auth screens each carried.
  2. **Chrome: launcher + breadcrumb** — *shipped 2026-09-20*. `appShell` (9-dot launcher,
     `Workspace / Page` breadcrumb, avatar) plus boards 03/04/05, replacing `workspaceHomeShell`.
     Scoped to those three screens rather than all fourteen because Preflight ties chrome to
     content (above). The section tab strip was the only piece needing `navigation:` to grow a
     second level, and it is deferred — so this phase changed **no metadata** and left
     `domain.NavigationItem` untouched. Two things it did surface: putting navigation in the
     launcher created the first real case for **per-viewer navigation filtering** (see
     `capabilities.md`, "Navigation: two filters"), and it confirmed the launcher must read
     `AllNavigation`, since a hidden group's routes stay reachable there.
  3. **Multi-application** — 3a *shipped 2026-09-20*: `applications:` is a list of per-Application
     files, Machines are workspace-level (loaded once, unique by id), and Task Tracker is now
     `app_document_approval` + `app_project_management`. Which Application a request is in is
     resolved Machine-first with navigation as fallback, because most of Document Approval's
     routes are named by no navigation item. `hidden_nav_groups` became per-Application
     `show_nav`. **3b** *shipped 2026-09-20*: an Application declares its own `roles:` vocabulary
     and a member holds one role per Application (`workspace_member_app_roles`, migration 008,
     which keeps the old `app_role` column so a rollback loses nothing). It also closed a
     pre-existing gap — the write paths accepted any `app_role` string, since until roles were
     declared there was nothing to validate against. **3c** *shipped 2026-09-20*: the card face —
     `description`, `icon`, `color` and `summary_machine`, the Machine whose count a card reports.
     Declared rather than summed, because summing an Application's Machines counts its
     configuration as work (Project Management totals 13, of which 4 are tasks). **Fase 3 is
     complete.**
  4. **Groups** — *shipped 2026-09-20*. Three platform tables mirroring `menata-runtime`'s own
     shipped CAP-O07 schema, plus a Groups admin screen ported from its `groups.html` /
     `group-detail.html` (copied into `ui-sample/`). Effective access is the **union** of direct
     and group-granted roles, computed at read time by the pure `data.EffectiveRoles` — CAP-O07's
     rule, not an invented one, and deliberately without a precedence rule where the two differ.
     Board 04's Source column and board 05's Effective-access panel are closed.
     **Group-as-Approval-Step-assignee moved to Fase 6**, with the screens that show it: upstream's
     **CAP-F24** (✅ admitted, not yet built) solves it *without* a polymorphic relation — a Field
     pair, `approver_type: value_list [User, Group]` plus `approver_user`/`approver_group`, each an
     ordinary relation targeting one `machine:`. Read that row before designing Fase 6.
  5. ~~**A View as a declared object**~~ — *shipped 2026-09-20*, ahead of the screens that would
     otherwise have hardcoded around it, exactly as this ordering intended. `mch_document` declares
     two arrangements; `type: cards` gave Projection the first consumer whose output varies per
     record, so a primitive that had run zero times since it shipped now actually runs.
  6. The approval-flow screens (boards 07-10).
  7. Role-based Permission + a declared transition model, then board 06 itself.

  **Penyempurnaan — what each shipped phase left undone, and when it lands.** Kept as one table
  on purpose: a comment at a call site answers *why* something is missing, this answers *when* it
  stops being. The three "no phase yet" rows are the point of having it — an unscheduled gap that
  looks scheduled is worse than one that admits it, and those are the rows to re-check at each
  phase close (the same discipline `CLAUDE.md` asks for standing metadata exceptions).

  | Left undone | Finished in | Blocked on |
  |---|---|---|
  | ~~Workspace Home's Application cards~~ — *done in 3a/3c*: one card per declared Application, with its own icon, description and declared count. Board 03's three cards assume Procurement and HR, which this app has no Machines, routes or roles for | — | — |
  | ~~Per-Application role columns on Members~~ — *done in 3b*: one line per Application that declares a `roles:` vocabulary, captioned with its name | — | — |
  | ~~Source / "Group: Reviewers" column~~ — *done in Fase 4* | — | — |
  | ~~Effective-access panel, direct ∪ group-inherited~~ — *done in Fase 4*: `data.EffectiveRoles`, computed not stored | — | — |
  | Member full name beside the email (board 04/05) | **no phase yet** | `data.Membership` carries only `Email`; `memberInitials` derives from the local part meanwhile. 3b moved roles but not identity, so this outlived the phase it was pinned to |
  | Section tab strip (board 07) | Fase 4 (Groups tab) + Fase 6 (My Documents tab) | 2 of its 4 destinations do not exist; the only part needing a `navigation:` second level |
  | Review document as its own screen (board 10) | Fase 6 | today it rides the generic record-detail page shared by every Machine |
  | New chrome on the Document Approval screens (inbox, submit, signature) | Fase 6 | Preflight ties chrome to content; their content ports there |
  | Approval Role Matrix (board 06) | Fase 7 | role-based Permission + a declared transition model, neither in 006/007 |
  | Declared `requires_role:` on a navigation item | **a second real case, not a phase** — likeliest around Fase 7, when an Application role could gate an item (a handler naming an id cannot express that) | one case exists today and has code: `appShell`'s `hiddenNavIDs`. See "Per-user/role navigation filtering" below for why the second case, not the calendar, is the trigger |
  | Application menu row + mobile bottom bar | **no phase yet** — lands with the Project Management screens' own boards | nothing in Fase 2–7 displays it: Workspace-level screens have no Application open, Document Approval declares `hidden_nav_groups`. The owner's bottom-bar decision (`navigation.html`, 3–4 icons, top-4 by priority) is already made; it is the *screens* that are missing, not the decision |
  | Member search box (board 04) | **no phase yet** — "Planned: search, filtering and pagination" below | search does not exist anywhere in the app |
  | "Keep me signed in" (board 01) | **no phase yet** | a session-lifetime change; `internal/authorization` has no remember-me concept |
- Rounding out Project Management: a project-level workspace overview, richer task detail
  (checklist, comments, attachments), and scoping views to one project at a time.
- Two-level navigation for switching between applications inside a workspace (phase 2-3 above).
- Accessibility and mobile/responsive polish across existing screens. The Case 03 port carries its
  own responsive rules per component as each screen lands, rather than deferring them to a later
  sweep — but one thing needs reconciling first: `ui-sample/navigation.html` specifies a mobile
  bottom bar of 3-4 icons, while the new boards' own mobile artboards (M01-M10) have no bottom bar
  at all and reuse the desktop header.
- "Keep me signed in" on the sign-in board — deliberately *not* rendered during phase 1, because it
  is a session-lifetime change rather than styling and `internal/authorization` has no remember-me
  concept; a checkbox that does nothing would be worse than none.

## Planned

- Installable as a PWA (Progressive Web App) -- add to home screen on a phone and open it like a
  native app, no app-store install required.
- Group-based roles and a visual approval role matrix.
- **Per-user/role navigation filtering -- the first real case has now arrived** (Fase 2 of the
  Case 03 port, 2026-09-20). This entry used to read "no declared nav item needs role-gating yet",
  and noted that the one concrete case found -- Workspace Home's "Members" link -- was
  workspace-level chrome rather than a metadata nav item. `appShell`'s launcher changed that: it
  renders `application.navigation` itself, so `nav_workspace_members` *is* now a declared nav item
  pointing at a `requireWorkspaceAdmin`-gated route, and every plain member was being offered a
  link that only ever 403s.

  What shipped is a deliberate one-case stand-in, not the declared form: `appShell` takes a
  `hiddenNavIDs` list and the handler names the item it already gates (`membersHiddenFor`,
  `workspacehome.templ`), feeding both the launcher and the page's own link from one test.
  `application.navigation` is still resolved once at process startup, identical for every viewer.

  **The trigger for building the declared form (`requires_role:` on a navigation item) is a
  *second* real case** -- the obvious candidate being an item gated on an Application role rather
  than on workspace admin, since that one cannot be expressed by a handler naming an id. That is
  the same way `Event` and `Permission` were both grown here: one real case earns code, the second
  earns the declaration. Not before.
- Extending the Event primitive (shipped above) to its one remaining real candidate case: a
  schedule/time trigger (for SLA-breach detection, currently read-triggered because there's no
  scheduler) -- its own second-real-case generalization, not assumed ahead of a concrete need,
  the same discipline that governed building the primitive itself (and its `on_create` shape).
- **A View composing other Views** -- the declared-View gate itself shipped 2026-09-20 (`views:`
  with ids, names and types; `?view=` selects one; `table`/`board`/`cards`). What it deliberately
  did *not* build is composition: a View that arranges other Views rather than a Machine's own
  records, which is what the remaining Document Approval screens would need to stop being Go. The
  three types that shipped are the three that already existed in code; upstream's registry carries
  ten, and a fourth arrives when a screen earns it on its own evidence rather than by import.
  Splitting the old anonymous `view:` block was part of the same decision: `sla_field` and
  `card_fields` describe how a Machine's records look *wherever* they appear (the detail page
  selects no View at all), so they became Machine-level and a View now carries only the
  arrangement.
- Search, filtering and pagination on record lists.
- Background/scheduled jobs (e.g. SLA-breach notifications that don't depend on someone opening
  the page).
- Expanding beyond the first two applications into the wider portfolio of business cases this
  runtime is designed to support (HR, inventory, point of sale, e-commerce, helpdesk, and more).
- **Closing the composition-layer decomposition gap**, in this order (full audit, with the
  binding-time leveling and the evidence count behind each step, in `menata-app-document`'s
  `audits/2026-09-19-decomposition-maturity-audit.md`): (1) **done** -- a ratchet conformance gate
  (`TestRenderingUsesProjectionNotRawValues`) stops any *new* `internal/rendering/*.templ` from
  reading named fields off a record instead of using Projection; ten files are grandfathered and
  the list may only shrink. It gates the Machine-specific-constant form (`Values[action.Field*]`)
  as well as the literal one, since that bypass was already in use; (2) Dataset +
  Dimension + Measure (007 §7.2-§7.4), minimal shape only (`source.machine`, `dimensions[].field`,
  `measures[].aggregate` limited to `count`/`sum`), whose "count grouped by a dimension, with a
  filter" pattern is already hand-written five separate times in `internal/composition/pages.go`
  -- past B1, not speculative; (2b) reusing `internal/expression`'s existing, tested
  `equals`/`not_equals` as that Dataset's filter predicate rather than a new expression language;
  (3) migrating the nine grandfathered screens onto Projection one at a time. Explicitly *not*
  included: generalizing `decide.go` (documented B4 failure) or building 007 §8's full Query Model
  (no forcing condition yet). **(2) and (2b) are now done for the first screen:** `datasets:` is a
  real metadata block (`domain.Dataset`, `composition.Aggregate`, `count`/`sum` only), Team
  Capacity composes from `mch_task`'s `ds_task_workload` and `mch_user`'s `ds_user_capacity`, and
  its filter is an ordinary `expression.Comparison` rather than a new syntax. Verified live:
  changing only `where.value` in YAML moved that page's own "Active cards" from 3 to 2, no code
  touched. **Dashboard and Sprint Dashboard followed**, so four of the five hand-written
  aggregations are now declared across five Datasets -- and Sprint reuses Team Capacity's own
  `ds_task_workload` unchanged, which is the first time two screens agree on what a number means
  by reading one declaration instead of two matching Go loops. My Tasks is deliberately *not*
  migrated: its counts filter on the viewing identity (a per-request value, not a literal) and on
  a due date against now, neither of which `where:` can express -- both real gaps, neither with a
  second case yet (`PersonalTasks`' own doc comment states this).
- Re-evaluating `internal/composition/pages.go`'s Case 19 Machine-id/status-option constants
  (`taskMachineID`, the `todo`/`in_progress`/`done` switch) against the B1-B5 decomposition
  criteria now that `card_fields` (Projection) has shipped -- flagged, not decided, in
  `menata-app-document`'s `audits/2026-09-19-metadata-hardcoding-gate-mapping.md`; these predate
  the "exception needs a forward-checkable pointer" convention (`CLAUDE.md`), so this is also the
  first case of applying that convention retroactively. Owner decision, not assumed here.

---

Full build history, architecture decisions, and audit records live in the private
`menata-app-document` repository.
