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
  automatic signature compositing onto the approved document. Two pieces of the flow as originally
  scoped are **not** here: a per-document-type saved default approval flow, and a third wizard step
  where sending actually happens (both in the deferral table below, both "no phase yet"). Read this
  bullet as the feature set that exists, not as the flow being closed.
- **Project Management** (core mechanics shipped, still rounding out) -- task boards with labels
  and ordered lists, my-tasks view, calendar, sprint dashboard, team capacity view, activity feed.
- **Grouped navigation**, declared as metadata rather than hardcoded per page.
- **Continuous architecture and code-quality checks** running in CI on every change.
- **Authentication and file-handling hardening**, based on an internal security review --
  invite-acceptance and session handling, upload validation and access checks, security response
  headers, and CSRF protection.
- **Authorization review, and the four holes it closed** (2026-09-21, prompted by the owner asking
  what a member and an admin can each actually do). The audit's own finding was that the answer was
  almost nowhere in metadata: the manifest declared a role *vocabulary* and exactly one rule using
  it, while nine of ten Machines declared no Permission at all -- which in this runtime means open
  to any authenticated member. Four things were ungoverned and none had a test:
  **record creation**, on every Machine (now `action: create`, whose `actor_field` reads "the
  record must name you" -- the arm that stops a `mch_signature` being written in someone else's
  name, which decided whose signature got composited onto an approved PDF); the **member
  directory** (`mch_user` now `workspace_role: admin`); the **audit trail** (`mch_activity` now
  `append_only: true` -- never changed or removed, by anyone); and `requireWorkspaceAdmin` itself,
  which **failed open** for any identity with no membership row rather than for the one shared
  credential it was written for. `/authorization-matrix` draws all of it, because the review also
  found that an ungoverned action and a fully granted one are indistinguishable in a YAML file.
- **Role-based Permission and a declared transition model** (Case 03 Fase 7, 2026-09-21) -- a
  Permission may require an Application role (`roles:`, upstream's CAP-P01), and a Machine may
  declare which moves of a status Field exist and which Action performs each (`transitions:`).
  Together they replaced one hand-written Go rule and closed two real holes it never covered: an
  already-approved step could be decided again on a parallel Document, and a Document's status
  could be written straight to `approved` through the generic edit route with no step decided.
  The **Approval Role Matrix** (`/approval-role-matrix`) draws both declarations as one table.
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
  2b. **The launcher becomes cards** — *shipped 2026-09-20* (`e973630`), an owner-requested
     redesign authored in a parallel session. The panel was a flat list of every declared
     navigation item; it becomes `ui-sample/nav-chrome.js`'s card shape — a WORKSPACE header row,
     the Workspace's own remaining destinations, one entry per Application, closing on "All
     Workspaces". `show_nav` gained a second reader, and only to decide **how** an Application
     appears, never whether it does. Full description in `capabilities.md`'s `appShell` row.

     **This entry was missing entirely until the Fase 7 close**, which is the same failure this
     section's own table warns about one paragraph below: the work was recorded in
     `capabilities.md` and in its commit message, and in no plan anyone re-reads. Numbered 2b
     rather than appended to Fase 2 because it post-dates Fase 3a (it reads
     `Workspace.Applications`), and renumbering 3-7 would break every reference to them.
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
  6. **The approval-flow screens (boards 07-10)** — next, split three ways because it is larger
     than Fase 3 was. These port as **bespoke**: Fase 5's increment carries three view types
     (`table`/`board`/`cards`), and these screens compose *across* Machines with layouts no type
     covers, so what improves is the projection ratchet rather than the screens becoming
     declarative.
     - **6a Approval Inbox (board 07)** — *shipped 2026-09-20*. `nav_workspace_groups` is
       declared, which emptied the undeclared-route gate's allowlist rather than adding to it; the
       four-tab strip is page chrome, not a second `navigation:` level (two tabs are `?tab=` views
       of one route, so the case Fase 2 predicted would force one never materialized); and the
       card face gained an approver list and `role="progressbar"`. The constraint held:
       `approvalinbox.templ` stayed out of `projectionRatchet`, so the per-approver state the
       board draws is composed in `internal/composition.stepStates` (`[]rendering.StepApprover`,
       replacing a `[]string` that carried states with nothing to attach them to) rather than read
       off records in the `.templ`. **Hour-scale SLA wording is the one board element not
       rendered** — `fld_due_date` is `type: date` and no datetime Field type exists; see the
       deferral table.
     - **6b Review (board 10)** — *shipped 2026-09-20*, and **board 09 moved to 6c** rather than
       shipping here as this line first paired it. Three pieces of evidence, all from the mockup
       and the companion repo rather than from convenience: board 09's own eyebrow reads `STEP 2
       OF 3`, so it is a step of the submit wizard 6c owns, not a standalone screen; its approver
       list shows `Legal Group · Group · 4 members` under the explicit note *"Approvers are
       supplied by Groups"*, which is CAP-F24, also 6c; and it replaces per-interaction saves with
       a batch `Save positions →`, a rework of the interaction model rather than a port. The
       companion repo's own staged plan already placed it last (`case-03-composability-
       checklist.md` Stage 4, *"boleh tidak sampai"*), because Q3 names signature coordinates as
       the canonical shape that stays page-internal.

       What board 10 bought: **`detail.templ` left `projectionRatchet` — the first entry ever to
       do so.** The Approve/Reject bar, the signature canvas and the placement confirmation lived
       on the *generic* record-detail page only because an Approval Step had no screen of its own,
       and the bar read `fld_decision` straight off the record. They now live on
       `reviewdocument.templ`, whose values `composition.ReviewDocument` resolves, and which
       contains no raw read at all — a new file may not be added to that list. A second thing fell
       out: `hasSignatureForGate` was being run on *every* Machine's detail page, `mch_task`
       included; only the review screen asks it now. **What is not claimed:** `detail.templ` still
       carries five `m.ID ==` branches. The ratchet measures raw field reads, not Machine-id
       branches, and saying otherwise would be this repo's own recorded failure shape.
     - **6c-1 CAP-F24, the actor gate** — *shipped 2026-09-20*. Split out from the boards because
       it lands in the Domain Plane and can be proven through the existing `/decide` gate with no
       new screen at all. **This line used to describe CAP-F24 as "a Field pair … each an ordinary
       relation targeting one `machine:`", and that was wrong.** Upstream's *shipped* artifacts
       say otherwise, and the shipped artifact is what runs: `approver_group` is a new Field
       **type** with no target machine, and the capability also adds three columns to the
       *permissions* table — it is a Permission-gate feature, not only a Field feature. The
       registry row never said so. A relation was impossible here regardless: this repo has no
       `mch_group`, and `capabilities.md` had already named "whether a Group needs a thin
       `mch_group`" as the question `approver_group` would force. **Answered: no.** A `group` Field
       type over the platform `workspace_groups` table, the same posture `person` takes over
       `mch_user`.

       One deliberate divergence from upstream, and it removes a bug they recorded: `fld_assignee`
       *is* the User half (`actor_user_field` points at it), so there is no second person picker —
       upstream added one beside its legacy Field and found "two independent Approver pickers on
       one screen". Two new Fields, not three. `actor_field` stays as the fallback, so every
       Approval Step already in the database kept working with no migration and no rewritten test;
       the existing permission tests needed only a type wrapper, which is the evidence rather than
       the claim.
     - **6c-2 Submit wizard (board 08)** — *shipped 2026-09-20*. The chrome port, the User/Group
       toggle per approver row, and **the first writer of `fld_step_name` and the CAP-F24 pair** —
       until this, the gate 6c-1 built was reachable only by hand-editing a record through the
       generic form. `documentsubmit.templ` left `projectionRatchet` (eight entries now): it sat
       there for one hand-built `<option>` list reading `fld_name`, which
       `composition.Loader.RelationOptions` already resolves — so unlike `detail.templ`'s exit in
       6b, which needed a whole new screen, this entry was one screen not asking for what it
       already had. `fld_mode` now reads its options from metadata too; it was the last hardcoded
       option list in the wizard, sitting two sections below a select that already did it right.

       **A silent data-loss trap was found while planning this and fixed before it could fire.**
       `signatureplacement.templ`'s hidden-field block carried four of a step's Fields through its
       generic PUT, and the generic route rewrites a record from what it is given — so the moment
       the wizard started writing the three new Fields, the next marker drag would have erased
       them with no error at all, turning a Group-held step back into an ungated one. It was
       invisible for two phases because the Fields were always empty. Board 09's own port is
       6c-3; this fix could not wait for it.
     - **6c-3 Signature positions (board 09)** — *shipped 2026-09-20*. Three things landed, and the port
       was the smallest of them.

       **This entry originally claimed "Case 3 is fully ported". That was false, and the owner
       caught it the same day** -- the fourth instance of this repo's own recorded failure shape,
       written an hour after a note about that shape went into the companion repo. Three Case 3
       screens are still `pageStyles`: `/dashboard`, which is a *declared navigation item* at
       priority 1, and the Document and Approval Step detail pages. See the deferral rows below.

       **A live bug 6c-2 created, closed.** `prm_edit_own_step`/`prm_delete_own_step` gated only on
       `actor_field`, which a Group-held step leaves empty — so the signature marker on the one
       screen built for dragging it was draggable by *nobody*, and the step deletable by nobody.
       Both Permissions now carry the same dynamic arm `decide` got in 6c-1; the fix is three lines
       of metadata each, and the test that proves it runs against the **real manifest**, because
       the failure was a missing declaration and a hand-built fixture would simply have declared it.

       **`signatureplacement.templ` left `projectionRatchet` — seven entries left — and its exit is
       the most interesting of the three.** A third of its raw reads were not a projection at all:
       seven echoed the record back verbatim as hidden inputs, forced by the generic update route
       rewriting a whole record from whatever the form submits. Composition now derives that list
       from the Machine's own declared Fields, which turned a hand-maintained list **that had
       already forgotten four Fields** into one that cannot forget. 6c-2's fix added three names
       and missed `fld_signature_image`, so a one-time signature was still being erased by the next
       drag after the "fix" shipped.

       **Three defects in shipped code, found while planning and fixed here.** A doc comment named
       a test that did not exist (`TestSignaturePlacementPut_preservesApproverFields`) — written
       inside the 6c-2 change whose own argument is that claims must match artifacts; that test now
       exists. Board 10's signature dialog had been rendering with its `pageStyles` rules unloaded
       since 6b, including `touch-action: none`, which is behaviour: signing on a phone did not
       work. And the drag script threw a TypeError on any marker the viewer could not edit.
  7. **Role-based Permission + a declared transition model, then board 06** — *shipped
     2026-09-21*. Three things landed, and step zero is what decided the shape of all three.

     **Step zero, run this time, and it overturned this entry's own premise.** This line used to
     read "both primitives are still absent from 006 and 007, which is why this stays last" —
     true about *this repo's* concept docs and irrelevant, because the companion rule
     (`case-03-composability-checklist.md` §5) binds on upstream's shipped artifacts as well.
     Upstream carries role-based event permission (**CAP-P01 ✅**, conformance T11/T12) and a
     whole `process:` overlay whose `transitions[]` compile to guarded Events. So neither
     primitive needed deciding; both needed implementing. Two divergences were then forced by the
     *shipped* shapes differing (the rule's own "if two kinds of prior art disagree, the shipped
     schema wins"), and both are recorded at the declaration rather than here:
     a Transition does **not** compile into an Event, because a `domain.Event` in this runtime is
     a post-write notification rather than a triggerable operation — compiling an edge into one
     would produce a declaration that notifies and never guards; and a Transition carries no
     `actor:`, because Permission here already says *which records* an actor may move, more
     richly than upstream can (CAP-F24's dynamic gate), and restating a role on the edge would be
     two sources for one answer.

     - **Role-based Permission (CAP-P01).** `roles:` on any `permissions:` entry; the actor must
       hold one of them in the Application that claims the Machine (`domain.Machine.
       ApplicationID`, stamped at load from that Application's own `machines:` list — an index
       over a claim already declared once, never a second declaration). Roles within one
       Permission are alternatives, Permissions on one Action stay requirements, which is
       `AllowsAction`'s existing contract unchanged — so the arm was added to the *one* function
       both the button and the POST already call, rather than beside it. The actor's half is
       their **effective** roles (`data.EffectiveRoles`, direct ∪ group-granted), so CAP-O07's
       union reaches a Permission gate without a second merge existing anywhere.
     - **A declared transition model.** `transitions:` on a Machine — which moves of a status
       Field exist and which Action performs each, `behavior.CheckTransitions` deciding, the
       generic update route, its JSON twin and `/decide` enforcing. It **replaced**
       `allowsDecisionChange`, a hand-written Go rule naming one Machine and one Field, and
       closed two holes that rule never covered. Both were live and neither had a test:
       an already-approved step could be decided again on a *parallel* Document (sequencing only
       ever locked a step behind an *earlier* one, so nothing anywhere said a decision was
       final); and a Document's own `fld_status` could be written straight to `approved` through
       the generic route with no step decided at all — skipping the approval flow, the sequencing
       rule and the PDF compositing together. Verified live against the dev database: that PUT is
       now `422 fld_status moves from "in_review" to "approved" by itself`, record unchanged.
     - **Board 06.** `/approval-role-matrix`, Workspace-level and `requireWorkspaceAdmin`-gated
       beside Members and Groups — which the board itself dictated rather than this list: it
       carries an Application selector and an appShell breadcrumb, not an Application topbar.
       `composition.RoleMatrix` is a pure projection with no store call at all, which is the
       honest shape of a screen every fact of which is declared. Its one subtlety is that a cell
       is the **intersection** of the role sets across an Action's Permissions, not the union —
       a union would tick a role the server refuses, which is the exact button-says-yes /
       server-says-no disagreement this screen exists to make visible.

     **A third consumer moved onto the declaration rather than a constant:** the Review screen's
     Approve/Reject bar asked `decision == "pending"`; it now asks whether any declared edge
     leaves the current value through `decide`. Same answer today, one declaration instead of
     two places that could drift.

     **What this narrows, and it is visible in the live data.** `mch_approval_step`'s three
     Permissions now declare `roles: [approver, reviewer]`, so being the person a step names is
     no longer sufficient. Of the dev Workspace's three members, two hold a qualifying role
     (one directly, one through the Groups grant — both paths reach the gate, which is the
     property the placement integration test now pins); the third holds only `submitter` and has
     one pending step assigned, which they can no longer decide until a role is granted on
     Workspace Members or through a Group. That is the capability working rather than a
     regression, but it is a real access change on real data and is called out rather than left
     to be discovered.

     **Board 06's Application selector renders only when more than one Application declares
     roles, so today it does not render at all** — Project Management declares none. The mockup's
     own three options (Document Approval, Procurement, HR) assume Applications this Workspace
     has no Machines, routes or roles for, the same board-03 situation the deferral table already
     records.

     **The screen is shipped and truthful; it is not board 06, and it cannot be made into board
     06 by working on the screen.** That board presupposes a *stage* model — the Document itself
     holding `Draft → In Review → Finance OK → Legal OK → Approved`, each stage gated on a role —
     while this app derives a Document's status from its Approval Steps and treats *who approves*
     as per-record data the submitter picks. Under the model that actually runs there are exactly
     two role-gated transitions in the whole Application, which is what the matrix shows. Six
     deferral rows below record that and the five smaller things board 06 asks for that this
     runtime cannot yet say (`draft`, a multi-`from` transition, the mixed Workspace/Application
     role columns, the transition-first process map, and m06's chip layout). Closing the first of
     them is an approval-model decision for the owner, not engineering — it is deliberately not
     taken here.

  **Penyempurnaan — what each shipped phase left undone, and when it lands.** Kept as one table
  on purpose: a comment at a call site answers *why* something is missing, this answers *when* it
  stops being. The "no phase yet" rows are the point of having it — an unscheduled gap that looks
  scheduled is worse than one that admits it, and those are the rows to re-check at each phase
  close (the same discipline `CLAUDE.md` asks for standing metadata exceptions).

  **Re-reading this table is part of closing a phase, not a tidy-up.** That is written here
  because it was not done: the bottom-bar row below sat wrong through the 3a, 3b, 3c and 4 closes,
  still naming a blocker that had dissolved at 3a. A row whose blocker quietly stops being true
  reads exactly like a row that is still blocked — which is the same failure shape as a
  conformance gate that passes while checking nothing, and it happened twice in one day. Check
  each row against the code, not against memory of having written it.

  | Left undone | Finished in | Blocked on |
  |---|---|---|
  | ~~Workspace Home's Application cards~~ — *done in 3a/3c*: one card per declared Application, with its own icon, description and declared count. Board 03's three cards assume Procurement and HR, which this app has no Machines, routes or roles for | — | — |
  | ~~Per-Application role columns on Members~~ — *done in 3b*: one line per Application that declares a `roles:` vocabulary, captioned with its name | — | — |
  | ~~Source / "Group: Reviewers" column~~ — *done in Fase 4* | — | — |
  | ~~Effective-access panel, direct ∪ group-inherited~~ — *done in Fase 4*: `data.EffectiveRoles`, computed not stored | — | — |
  | Only the Document's own submitter may edit or delete it | **no phase yet** — **blocked on data, not on a primitive** | `fld_submitted_by` exists as of 2026-09-21 and the wizard stamps it, so `actor_field: fld_submitted_by` on an `edit`/`delete` Permission would express the rule exactly. It is not declared because every Document written before that field is empty, and an empty actor field satisfies **nobody** (`authorization.isActor`: an unassigned record is actionable by no one, rather than by everyone) — declaring it would make every existing Document permanently uneditable and undeletable by every person. Backfilling does not rescue it: checked against the real data, eight of nine existing Documents were created by the shared admin credential's placeholder identity, which is not a real `mch_user` record, so the derived value would match nobody. **The trigger is the existing Documents being gone or re-created, not a second case.** Until then any member may delete anyone's in-review Document (business state is the only guard: not approved, no decided steps) |
  | Only a Document's own submitter may add Approval Steps to it | **no phase yet** — **needs a cross-record Permission** | The rule wants to read a field on the *parent* Document (`fld_submitted_by`) from a Permission declared on `mch_approval_step`. Every Permission arm this runtime has reads the record being acted on, or the actor — none reaches a related record. Same shape as the conditional-required gap above, from the authorization side rather than the validation side. Until then any member may add a step to anyone's Document, naming anyone as its approver (deciding it still requires the role and the assignment) |
  | Machine-level `read` permission, and deny-by-default (upstream CAP-P05 ✅) | **no phase yet** — **and this is the one with a blast radius worth naming first** | Everything declared so far governs *writes*. Reads are ungoverned: any member can open any Machine's records, including another Application's. Upstream ships `can_read`/`can_create`/`can_edit` per permission row **plus** the structural half — "a role with no permission row at all on a machine is now denied" — which is the real change and why this is not a small addition here: switching this runtime from Principle #6's *unrestricted-unless-declared* to deny-by-default means **every** Machine must declare permissions first or the app stops working, the same all-or-nothing shape as `KnownFields` in §Planned. It also collides with a rule this repo holds deliberately (metadata describes exceptions, not defaults), so it is a design decision, not just work |
  | **Board 06 presupposes an approval model this app does not have** — the matrix ships and is truthful, but it can never look like the board | **no phase yet** — **and this is a model decision, not a screen one** | Board 06's own rows are `Submit: Draft → In Review`, `Approve — Finance: In Review → Finance OK`, `Approve — Legal: Finance OK → Legal OK`, `Approve — Director: Legal OK → Approved`. That is a **stage model**: the Document itself holds a multi-state status and each stage's transition is gated on a *role*, declared once per Application. This app does the opposite, on purpose: a Document's status is **derived** from its Approval Steps (`evt_step_decision_rollup`, three values), and who approves each step is **per-record data** the submitter picks in the wizard (`fld_assignee`/`fld_approver_group`), not metadata. Under that model there are exactly two role-gated transitions in the whole Application — Approve and Reject on a step — which is what the shipped matrix correctly shows, six `System` rows and two real ones. **Neither model is wrong and this row does not pick one**; what it records is that the screen cannot be made to resemble the board without first changing the approval model, and that no amount of work on the screen is the missing piece. Upstream's own `process:` overlay is the stage model, and its board-06 copy citing `ProcessEdge` and `/{machineID}/process-map` is drawn against it -- the same "mockup draws data the app doesn't have" class as board 03's Procurement/HR cards and board 07's hour-scale SLA. A middle path exists and is already a row below: **CAP-V28**, a saved default approval flow per Document Type, which makes the chain a declared template without making status a stage machine |
  | `draft` as a Document status (board 06: `Submit: Draft → In Review`, `Reopen: Rejected → Draft`) | **no phase yet** — lands with the wizard's third step, below | Two of board 06's six rows need a state `metadata/document.yaml` deliberately removed, with its own note: *"add it back only alongside a real save-as-draft flow, not speculatively."* That flow is the wizard's third step, which has no board anywhere. So this is the same deferral seen from the matrix's end, not a second one — worth listing because from board 06 it reads as a missing transition rather than a missing flow |
  | A transition with more than one `from` (board 06: `Reject: any pending state → Rejected`) | **no phase yet** — **a second real case, not a phase** | `domain.Transition` names exactly one `from`, so "any pending state" is declarable only by enumerating one edge per source state. With `fld_decision`'s single open value that enumeration is one row and costs nothing, which is precisely why the shape should not be generalized yet; it starts costing the moment a status Field has several non-final states, which is the stage model above. Recorded so the board's wording is not read as an unimplemented feature of the matrix |
  | Board 06's columns mix two role namespaces | **answered 2026-09-21: the screen renders Application roles only, and the board's two Workspace columns are dropped** | Its five columns are `ADMIN`, `FINANCE REVIEWER`, `LEGAL REVIEWER`, `DIRECTOR`, `MEMBER`. `ui-sample/member-role-detail.html` settles what those words are, in the owner's own design language: a "Workspace role" section holding exactly `Admin`/`Member` — captioned *"controls workspace administration, independent of application permissions"* — above a separate "Application access" section whose Document Approval vocabulary is Approver/Submitter/Reviewer. So **two** of board 06's five columns are Workspace-level, not one, and the other three appear in no declared vocabulary at all. The shipped screen puts Workspace rules in their own section with no role columns (`06b-authorization-matrix.html`), which is also why `workspace_role:` is a separate key from `roles:`: that same mockup gives HR an *Application* role literally named `Admin`, so one list would make `roles: [admin, approver]` an alternation across two different meanings. `Admin` and `Member` are this runtime's **Workspace** roles (`workspace_members.workspace_role`, what `requireWorkspaceAdmin` reads); the other three would be **Application** roles (`application.roles:`). The shipped screen renders Application roles only, which is the honest choice — a column whose ✓ means a different kind of grant than its neighbour's is worse than an absent column. Whether a Workspace admin should appear as an always-✓ column is an owner call, and it is a real question rather than a rendering detail: admin today bypasses no Permission at all |
  | Process map — the same edges drawn transition-first (`/{machineID}/process-map`, upstream CAP-W05 ✅) | **no phase yet** | Board 06's own copy says the matrix "re-projects `ProcessEdge` (already rendered per-transition on `/{machineID}/process-map`) role-first instead of transition-first" — describing that route as already existing. It does not exist here; Fase 7 built the role-first projection only. Upstream's is *derived*, needing no new metadata concept, and `domain.Machine.Transitions` is now exactly the input it reads, so this is a small screen rather than a capability — it simply has no case asking for it yet |
  | Board m06's mobile layout (role **chips** per transition, not a grid) | **no phase yet** | The desktop grid ports; the mobile board drops the matrix shape entirely and lists `✓ Admin`, `✓ Finance Reviewer` as chips under each transition, which is a different component, not a breakpoint. The shipped screen scrolls the table horizontally instead — usable, not the drawn design. Same class as the bottom-bar row below: a real mobile design exists and is unported |
  | Member full name beside the email (board 04/05) | **no phase yet** | `data.Membership` carries only `Email`; `memberInitials` derives from the local part meanwhile. 3b moved roles but not identity, so this outlived the phase it was pinned to |
  | ~~Section tab strip (board 07)~~ — *done in 6a*: `rendering.inboxTabs`, page chrome rather than a second `navigation:` level. Fase 2 had recorded this as "the only part needing" one; with two tabs being `?tab=` views of a single route and the other two being real declared items, the case never materialized | — | — |
  | Hour-scale SLA wording (board 07: "SLA breached · 4h"; board 10: "Breached · 4 hours ago") | **no phase yet** | `fld_due_date` is `type: date` — a bare calendar date, no time component — and `domain.KnownFieldTypes` has no datetime type at all. `experience.EvaluateSLA` truncates to day *deliberately*, which is what makes "Due today" mean anything. Adding a Field type to satisfy one label is shape-before-need; the trigger is a second, independent caller that genuinely needs sub-day precision. Noted at the site in `ApprovalInboxPage`'s and `ReviewDocumentPage`'s own doc comments. **Two screens now want it and it is still not built** — that is the row working, not a row going stale: both are the same board's day-vs-hour mismatch, not two independent callers |
  | ~~Review document as its own screen (board 10)~~ — *done in 6b*: `reviewdocument.templ` + `composition.ReviewDocument`, which is what let `detail.templ` out of `projectionRatchet` | — | — |
  | The wizard's third step (boards 08/09 both say "OF 3") | **no phase yet** — and it is a *flow* question, not a screen | No step-3 board exists anywhere in the set, so what it would contain is inference. Behind the label sits the real finding: `submitDocumentWizard` sets `fld_status = in_review` at creation, **before** any signature is placed — so an approver can decide a Document while the submitter is still on the placement screen, and abandoning the wizard leaves a live in-review Document with no placements. A third step would be where sending actually happens, which is also what would earn `draft` back as a `fld_status` option (`metadata/document.yaml` already says: "add it back only alongside a real save-as-draft flow, not speculatively"). The wizard says "Step 1 of 2" meanwhile, because two screens exist |
  | Conditional required (a Field required only when a sibling Field holds a given value) | **no phase yet** | `domain.Constraint` has one shape — block a field transition while a *related Machine* has a matching record — which cannot condition on a sibling Field of the same record. The one real case is live as of 6c-2: `fld_assignee` is required for a User-held Approval Step and meaningless for a Group-held one, so it is declared optional and `internal/web`'s `parseStepInputs` enforces the pairing in Go. Upstream declares the same rule as two conditional Constraints, so the shape is known. Trigger is a second real case, as always |
  | Approval Dashboard (`/dashboard`) still on `pageStyles` | **no phase yet** | It is a **declared navigation item at priority 1** — the first entry in Document Approval's own menu — so this is the most visible unported screen in Case 3, not an edge. Its board is `12-approval-dashboard-UNUSED.html`, marked TIDAK DIPAKAI by the owner, so there is no mockup to port it against: the blocker is a design decision, not engineering. Either the board is un-retired, or the screen is ported to the Tailwind idiom the other four Case 3 screens now share without one |
  | Document / Approval Step detail pages still on `pageStyles` | **no phase yet** — lands with whatever ports Project Management | They are `detail.templ`, the *generic* record view every Machine shares, so porting it reaches `mch_task`, `mch_project` and the rest. Fase 6b and 6c-3 both deliberately split a Component rather than port it for this reason. The cost is visible today: the Signature positions screen's own back link lands on an old-style page, mid-flow |
  | Board 09's zoom controls, selected-step highlight, and approver department sub-line | **no phase yet** | Zoom and selection are ephemeral view state nothing stores or needs — the app serves one rendered size and the screen works without a selection model. The department ("Finance", "Director") has no Field anywhere: `mch_user` holds none, and 3b's Application role is per-Application, not per-person-per-step. `fld_step_name` is rendered in its place, which is what the step is actually *for* |
  | Uploaded file size (board 10: "6 pages · 2.4 MB") | **no phase yet** | the page count is real (`internal/pdf.PageCount`) and is rendered; **the byte size is stored nowhere** — not on the record, not in `internal/storage`'s key, which embeds only the original filename. It needs a write-path change (capture size at upload) plus a Field to hold it, for one label. The trigger is a second caller that needs file metadata, not this line |
  | Step names with real values (board 10: "Finance Review") | **Fase 6c** | `fld_step_name` is declared as of 6b and `composition.stepLabel` reads it; nothing *writes* it until board 08's wizard collects it. Until then every step is titled with its assignee, which is exactly what the app did before the Field existed — the fallback is the honest state, not a placeholder |
  | New chrome on the Document Approval screens (~~inbox~~, ~~review~~, submit, signature) | **inbox done in 6a, review in 6b**; submit and signature both 6c | Preflight ties chrome to content; their content ports there. Signature moved out of 6b — board 09 is `STEP 2 OF 3` of the submit wizard and shows Group approvers (CAP-F24), so it belongs with board 08, not before it |
  | ~~Approval Role Matrix (board 06)~~ — *done in Fase 7*: `/approval-role-matrix`. The blocker this row named ("neither in 006/007") was answered by step zero rather than by building both from scratch — upstream had already shipped CAP-P01 and a `transitions[]` shape; what this repo's concept docs say about them turned out not to be the binding question | — | — |
  | Declared `requires_role:` on a navigation item | **a second real case, not a phase** — and **Fase 7 came and went without supplying one**, which this row predicted it would ("likeliest around Fase 7") | Re-checked at the Fase 7 close, which is the only reason this line is honest. Fase 7 *did* add a third id to `appShell`'s `hiddenNavIDs` (`nav_role_matrix`, beside Members and Groups) — but all three are gated on the **Workspace** role (admin), which a handler naming an id expresses perfectly well. The trigger this row names is an item gated on an **Application** role, the case a handler *cannot* express, and role-based Permission landing did not create one: no navigation item in the manifest is scoped to an Application role. Three instances of the same shape is, separately, the B1 repetition signal — so the next hardcoded id is worth running through the decomposition criteria even if the trigger below still has not arrived |
  | Mobile bottom bar for an Application's own menu | **no phase yet** — lands when the Project Management screens get boards | Those screens are still `pageStyles`, so a bottom bar there would be hand-written CSS thrown away when they port — that is the real blocker. **This row previously said "nothing in Fase 2–7 displays it", which stopped being true at Fase 3a**: Project Management declares no `show_nav`, keeps its menu, and `/my-tasks` renders ten nav links today. It stayed wrong through four phase closes because nobody re-read the table, which is the one thing this table needs. The owner's decision (`navigation.html`, 3–4 icons, top-4 by priority) was never the missing part |
  | Saved default approval flow (board 08: "Save this as the default approval flow for **Contract** documents", checked by default) | **no phase yet** | Upstream built it (CAP-V28) as two companion Machines — one template per Document Type, plus its own ordered steps — with a write direction that find-or-creates the template on submit. That is the real blocker: a template entity and a write path, not the screen. **This row exists because the deferral did not.** It has been live since Phase 15, recorded only inside `menata-app-document`'s development history, in no table anyone re-reads at a phase close — and with a reason that was wrong when written ("only one Document Type exists in metadata today"; there were zero) and is wrong now in the other direction (there are three: `Kontrak`, `Tagihan`, `Lain-lain`). Same shape as the bottom-bar row that sat wrong through four closes |
  | One signature box for a Group-held step (boards 08/09) | **no phase yet** — **named, not solved** | Board 09 places exactly one signature box for `Legal Group · 4 members`, and nothing on either board says whose signature image lands in it, or what happens when two of the four act. Upstream has the identical gap on its own compositing capability, recorded there in the same words. Worth holding here rather than discovering it during 6c-2's port |
  | Member search box (board 04) | **no phase yet** — "Planned: search, filtering and pagination" below | search does not exist anywhere in the app |
  | Write-side Binding: a form input's `name=` hardcoded to a Field id (007 §11.3) | **no phase yet** — **and it is the gap a shrinking ratchet hides** | `signatureplacement.templ` carries twelve `name={ action.Field… }`, `documentsubmit.templ` six, while `machine.templ` already does it generically (`name={ f.ID }` over `m.Fields`). `TestRenderingUsesProjectionNotRawValues` gates *reads*, never writes, so both files could leave `projectionRatchet` with every binding still hand-typed — which is exactly what happened in 6c-2 and 6c-3. Recorded here so the ratchet count is not read as a composability score. Trigger: a third bespoke write screen, or the generic update route stopping its whole-record rewrite (the same route that forced 6c-3's carry-forward list) |
  | `NavBadgeApprovalInboxPending` still resolved in the Domain Plane | **no phase yet** | `domain.NavigationItem.Badge` names one live count the runtime special-cases end to end (`domain/navigation.go`, `web/workspacehome.go`, `appshell.templ`, `machine.templ`). It cannot become an ordinary Dataset: the count filters on assignee **and** on `behavior.CanAct`, which reads a *sibling* record, while a Measure's `where:` is evaluated per record against its own values. So the blocker is record selection (007 §8), not the identity sentinel below — a point the companion repo's Stage 0 first got wrong in the other direction |
  | PDF signature compositing is hand-written Go (upstream CAP-F22) | **no phase yet** | The third of the three Case 3 behaviours named above; the other two (`activate_next`, `aggregate_status`) landed 2026-09-20 and this one did not, so it should stop riding on their line. `internal/action/composite.go`/`banner.go` manipulate a binary PDF rather than express a rule, which is the lowest generalization value of the three — but "lowest value" is a ranking, not a deferral reason, and it had neither a phase nor a row until now |
  | The approval stepper is five constants, not a declared View (upstream CAP-V20 ✅) | **no phase yet** — re-checked at the Fase 7 close and still blocked, but on *less* than before: the stepper's own done/current/pending vocabulary is now half-expressible, since `transitions:` says which states a step can still leave. What it still needs is a View that composes other Views | `approvalstepper.templ` is the **last Case 3 entry left in `projectionRatchet`**, and nothing anywhere states how it leaves. Upstream carries the sequential decision stepper as a shipped View *type* (ordered done/current/pending over a parent's child steps), so this is an implement-a-known-capability question rather than a design one — the same posture `activate_next` was in before 6c. Blocked in practice on a View that composes other Views (Planned, below), since the stepper renders a parent record's children, not its own Machine's records |
  | "Keep me signed in" (board 01) | **no phase yet** | a session-lifetime change; `internal/authorization` has no remember-me concept |
- Rounding out Project Management: a project-level workspace overview, richer task detail
  (checklist, comments, attachments), and scoping views to one project at a time. **Its standing
  relative to the Document Approval scope decision is unsettled, and that matters more than it
  looks:** the 2026-09-20 decision narrowed the proof-of-concept to Document Approval, while this
  line still reads as active work — and six of the seven files left in `projectionRatchet` are
  Project Management screens whose stated exit is "migrates with Dataset/Projection". So the
  ratchet's own path to empty runs through work whose scope is undecided. Either these screens are
  in scope and the ratchet can drain, or they are parked and the ratchet's remaining entries are
  parked with them; both are fine, being unsaid is not.
- ~~Two-level navigation for switching between applications inside a workspace~~ — *shipped*: the
  9-dot launcher (Fase 2) over real Applications (Fase 3a).
- Accessibility and mobile/responsive polish across existing screens. The Case 03 port carries its
  own responsive rules per component as each screen lands, rather than deferring them to a later
  sweep. The `navigation.html` / M01-M10 disagreement over a mobile bottom bar is **resolved and
  not a conflict**: M01-M10 are all Document Approval or Workspace screens, which have no
  Application menu to put in a bottom bar, so neither source is wrong. The bottom bar belongs to
  the Project Management screens, and waits on their own boards — see the deferral table.
- "Keep me signed in" on the sign-in board — deliberately *not* rendered during phase 1, because it
  is a session-lifetime change rather than styling and `internal/authorization` has no remember-me
  concept; a checkbox that does nothing would be worse than none.

## Planned

- Installable as a PWA (Progressive Web App) -- add to home screen on a phone and open it like a
  native app, no app-store install required.
- **Role-based Permission -- ~~planned~~ shipped 2026-09-21** (Case 03 Fase 7, above). This
  section listed "Group-based roles and a visual approval role matrix" as one Planned line; both
  halves are now built (Groups in Fase 4, the matrix and the role gate in Fase 7). What stays
  unbuilt, and is a different thing, is **scope depth** on a role -- "their own records", "their
  unit and below" -- which upstream carries as a proposed, unbuilt row (CAP-P08) depending on an
  organizational-unit tree this repo does not have either. Narrowing *which* records a role
  reaches is still `actor_field`'s job alone.
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
- **A filter that reads the viewing identity, not a literal** — `datasets:`' `where:` compares a
  Field against a written value, so any count scoped to "mine" stays Go. Two screens do it today:
  `composition.PersonalTasks` (assignee == the viewer) and `composition.ApprovalInbox` (the inbox
  and the nav badge). That is the second real case this repo's own rule waits for, and
  `PersonalTasks`' doc comment still says "neither has a second case yet" — true when written,
  worth re-reading now. Upstream settled the shape rather than leaving it open: a `$current_user`
  sentinel resolved at evaluation time, not a parameter threaded through metadata. **What it does
  not unblock:** the Approval Inbox itself, which needs record *selection* and a sibling read as
  well (see the deferral table).
- **Ordered comparison operators in `internal/expression`** (`lt`/`lte`/`gt`/`gte`, type-aware
  rather than the current `fmt.Sprint` string compare). Named here because two unrelated gaps are
  waiting on the same three lines: My Tasks' Overdue/Due-today counts compare a date against now,
  and the hour-scale SLA wording in the deferral table needs the same ordering once a datetime
  Field type exists. `expression.Comparison`'s own doc comment asks for a second differently-shaped
  Constraint before growing the vocabulary; these two are that evidence arriving from the filter
  side instead.
- **Reject unknown metadata keys at load** — `internal/metadata`'s three `yaml.Unmarshal` calls
  decode without `KnownFields`, so a key the parser has no home for is dropped in silence and the
  Machine loads clean. 005 Phase 3 says invalid metadata must not reach compilation; this is the
  one validation hole that makes *documentation* drift dangerous rather than merely untidy — a
  guide that still shows a retired key produces no error anywhere, which is how the pre-2026-09-20
  singular `view:` block stayed in `writing-guide.md` §6/§12.7 unnoticed (verified by loading a
  copy of `metadata/` that used it: no error, zero Views). Small change, real blast radius: every
  existing manifest must be clean before it can be turned on.
- **Metadata hot reload and change classification** — tracked in `capabilities.md`'s limits as two
  separate deliberate deferrals (a `*.yaml` edit needs a restart; deleting a Field silently orphans
  its data in every record's JSONB) and in the companion repo's own
  `guides/metadata-hot-reload-safety.md`, but named in no plan at all until now. Neither is
  urgent while the manifest ships with the binary; both stop being optional the moment metadata is
  edited by someone who cannot restart the process, which is the actual end state this runtime is
  for.
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
