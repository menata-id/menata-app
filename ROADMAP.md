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
  2. Chrome: breadcrumb + app launcher + section tabs, replacing the projected topbar. Needs
     `navigation:` to grow a second level (today's `group:` renders a dropdown, not a tab strip).
  3. Multi-application — `app.yaml` declares one `application:`, and `workspace_members` has one
     `app_role` column; `capabilities.md` already states Application plurality is not built.
  4. Groups — absent entirely, and present in five of the ten boards. A Group and its membership
     can be ordinary Machines (many-to-many is already proven by `mch_card_label`); the parts that
     cannot be, and need a decision: effective access as the union of direct and group-inherited
     roles, and an Approval Step assignable to a User *or* a Group (`relation` targets exactly one
     `machine:`).
  5. **A View as a declared object** — the gate already named below, taken before the screens that
     would otherwise hardcode around it.
  6. The approval-flow screens (boards 07-10).
  7. Role-based Permission + a declared transition model, then board 06 itself.
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
- Per-user/role navigation filtering -- `application.navigation` (`app.yaml`) is resolved once at
  process startup, identical for every viewer; no declared nav item needs role-gating yet, so this
  isn't built speculatively (the one concrete case found, Workspace Home's "Members" link, was
  workspace-level chrome, not a metadata nav item, and is already fixed). Build this once a real
  `application.navigation` item needs to be hidden from some viewers, not before.
- Extending the Event primitive (shipped above) to its one remaining real candidate case: a
  schedule/time trigger (for SLA-breach detection, currently read-triggered because there's no
  scheduler) -- its own second-real-case generalization, not assumed ahead of a concrete need,
  the same discipline that governed building the primitive itself (and its `on_create` shape).
- **A View as a declared object** -- today a Machine carries exactly one anonymous view
  configuration (layout, grouping, SLA field, card fields). Every composed screen that isn't a
  plain table or board therefore has to be written in Go, because there is no way to declare a
  second view of the same Machine, give it a type, or have one view compose others. This is the
  single gate standing between the Document Approval screens and being metadata-driven, and it is
  a real architecture decision rather than something to slip into one screen's work -- named here
  so it gets decided deliberately. The two steps that need no such decision (a current-user filter
  sentinel, and moving approval sequencing and status rollup out of Go) are sequenced ahead of it.
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
  criteria now that `view.card_fields` (Projection) has shipped -- flagged, not decided, in
  `menata-app-document`'s `audits/2026-09-19-metadata-hardcoding-gate-mapping.md`; these predate
  the "exception needs a forward-checkable pointer" convention (`CLAUDE.md`), so this is also the
  first case of applying that convention retroactively. Owner decision, not assumed here.

---

Full build history, architecture decisions, and audit records live in the private
`menata-app-document` repository.
