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
- **An Application with a role vocabulary is closed to anyone without a role in it** (owner
  decision, 2026-09-21: *someone who is not a member of an application cannot do anything in it --
  not even look*). `web.requireApplicationAccess`, at the one chokepoint every Application route
  already passes through. It is what finally makes a view-only role legible on the Authorization
  Matrix: before it the page had no "view" row at all, so `reviewer` rendered as an empty column,
  which reads as a role that can do nothing rather than one whose whole purpose is looking.
  **Coarser than per-Machine read on purpose** -- this runtime has no `read` Action, so the
  statement is "in or out", and the finer shape stays a deferral row below.
- **The three declared roles now mean three different things** (owner decision, 2026-09-21):
  *approver* decides a step assigned to them and may submit; *submitter* submits and builds the
  approval chain; **reviewer may only look** -- cannot submit, cannot approve, cannot change a
  Document. Until that decision the vocabulary said nothing: approver and reviewer named the
  identical grant everywhere they appeared, so the two words were indistinguishable to anyone
  reading the Members screen, and submitter named no grant at all. Declared on the Machines that
  own the rules (`mch_approval_step`'s three Permissions, `mch_document`'s create/edit/delete,
  and -- on a same-day correction from the owner -- `mch_signature`'s, since a signature has
  exactly one consumer and only an approver reaches it),
  not on the vocabulary, and it closed both of the Authorization Matrix's amber rows on the way.
  **Live effect when it landed**: of nine pending Approval Steps in the dev Workspace, six were
  assigned to people holding no `approver` role and could no longer be decided. **That sentence
  was true for about an hour and is recorded here as past tense on purpose** -- the owner granted
  `approver` to all three members and to the Group the same day, so the number was already wrong
  by the next time it was quoted, and quoting it again as current is exactly the failure this
  table's own preamble warns about. Re-check a live-data claim against the database, not against
  having written it. What is actually left is five pending steps whose assignee is not a Workspace
  member at all -- old test data, two of its three documents deleted 2026-09-21 on owner request.
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

  **Open at the 2026-09-21 session close — three things waiting on the owner, not on engineering.**
  Written here rather than left in a chat log, because a decision that lives only in a reply is
  the failure this whole table exists against, and two of these were already noticed as such.

  1. **Which approval model** (the board 06 row below). Three ways forward are written into that
     row with the trade-off of each; **CAP-V28 is the recommendation**. Nothing else in the
     Document Approval backlog can be sequenced honestly until this is answered, because the stage
     model would rework the wizard, the rollup and boards 07-10 while the other two would not.
  2. **One document's fate.** `Vendor Contract 2026` (`rec_e56a51b513564f98f9da6099`) holds two
     Approval Steps: one still pending and assigned to a user record that is not a Workspace
     member at all, and one **already approved**. It is the last of the orphaned test data; its
     two siblings were deleted on owner request the same day. It was not deleted with them
     precisely because of that approved step -- removing a document to tidy up a dangling
     assignment would destroy a real approval record, and rolling that up would also flip the
     document's own status. Delete the document, or only the pending step?
  3. **Does `approver` vs `submitter` still need narrowing anywhere?** The vocabulary now grants
     three different things, but `submitter` and `approver` are identical on every Document and
     Approval Step *create* rule. That is deliberate (an approver submits their own documents
     too), and it is the kind of sameness that made `approver`/`reviewer` indistinguishable for a
     phase, so it is named here rather than left to be rediscovered.

  **State of the tree at that close**, for whoever picks this up: `menata-app` and
  `menata-app-document` are both pushed and in sync, every test green including the
  Postgres-backed ones, and the dev server running the current build. A parallel session's
  Account-menu work (`1ba1a1a`) and its `ui-sample` chrome redesign landed in the same window; one
  collision is recorded on that commit (it registered a middleware whose function was still
  uncommitted, so it does not build in isolation -- `64662c1` restored it).

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
  | ~~The wizard's approver picker offers every Workspace member~~ — *done 2026-09-21*: `composition.ApproverOptions` narrows the person picker to members holding a role the step Machine's own `decide` Permission requires, which is upstream's CAP-F05 shape ("a query-time filter, no new metadata concept"). Derived, never declared twice: the required roles come from the same Permissions `AllowsAction` enforces, and a member's own come from `data.EffectiveRoles`, so a role held through a Group qualifies exactly as a direct one. A Machine whose `decide` Permission names no role narrows nothing | — | — |
  | ~~Placing signature boxes was gated on "may edit this step"~~ — *done 2026-09-21*: board 09 is `STEP 2 OF 3` of the submit wizard, so the person laying the boxes out is the Document's **submitter**, while `prm_edit_own_step` says only a step's own approver may change it. The wizard redirected submitters to a screen where every marker was static and the write would have been refused. `composition.MayPlaceSignature` resolves the real rule (the Document's submitter, or the step's own approver) for both the marker and the route, and the write moved to its own `PUT .../signature-placement` — which **also ended the carry-forward echo**, since a route writing four named Fields cannot erase a fifth | — | — |
  | Old row, kept for its reasoning: the picker gap | **closed the day it was recorded** | It was written into this table at the moment the reviewer decision created it, and closed in the next change. Left here because the sequence is the point: the deferral row is what made it a scheduled piece of work rather than a bug someone would eventually hit |
  | Only the Document's own submitter may edit or delete it | **no phase yet** — **blocked on data, not on a primitive; the role half landed 2026-09-21** | The *role* half is declared (`prm_edit_document_not_reviewer`/`prm_delete_document_not_reviewer`, `roles: [approver, submitter]`), so a reviewer may not touch a Document at all. What is still open is the owner-scoped half, and a second Permission on the same Action ANDs with the first, so adding it later narrows rather than replaces. `fld_submitted_by` exists as of 2026-09-21 and the wizard stamps it, so `actor_field: fld_submitted_by` on an `edit`/`delete` Permission would express the rule exactly. It is not declared because every Document written before that field is empty, and an empty actor field satisfies **nobody** (`authorization.isActor`: an unassigned record is actionable by no one, rather than by everyone) — declaring it would make every existing Document permanently uneditable and undeletable by every person. Backfilling does not rescue it: checked against the real data, eight of nine existing Documents were created by the shared admin credential's placeholder identity, which is not a real `mch_user` record, so the derived value would match nobody. **The trigger is the existing Documents being gone or re-created, not a second case.** Until then any member may delete anyone's in-review Document (business state is the only guard: not approved, no decided steps) |
  | Only a Document's own submitter may add Approval Steps to it | **no phase yet** — **needs a cross-record Permission; the role half landed 2026-09-21** | `prm_create_step` (`roles: [approver, submitter]`) now keeps a reviewer out, and gates the *generic* create route only — the wizard writes its steps through `internal/data` directly, so a submitter building their own chain is unaffected either way. The rule anyone actually wants still cannot be said: it wants to read a field on the *parent* Document (`fld_submitted_by`) from a Permission declared on `mch_approval_step`. Every Permission arm this runtime has reads the record being acted on, or the actor — none reaches a related record. Same shape as the conditional-required gap above, from the authorization side rather than the validation side. Until then any member may add a step to anyone's Document, naming anyone as its approver (deciding it still requires the role and the assignment) |
  | Machine-level `read` permission, and deny-by-default (upstream CAP-P05 ✅) | **no phase yet** — **the coarse half landed 2026-09-21; this is the fine half, and it has a blast radius worth naming** | Reading is now gated at the **Application** boundary (`requireApplicationAccess`): no role in an Application, no access to any of its screens. What is still missing is reading gated *per Machine and per role* — "a reviewer may see Documents but not Approval Steps" is not sayable, because there is no `read` Action to declare it on. Upstream ships `can_read`/`can_create`/`can_edit` per permission row **plus** the structural half — "a role with no permission row at all on a machine is now denied" — which is the real change and why this is not a small addition here: switching this runtime from Principle #6's *unrestricted-unless-declared* to deny-by-default means **every** Machine must declare permissions first or the app stops working, the same all-or-nothing shape as `KnownFields` in §Planned. It also collides with a rule this repo holds deliberately (metadata describes exceptions, not defaults), so it is a design decision, not just work |
  | **Board 06 presupposes an approval model this app does not have** — the matrix ships and is truthful, but it can never look like the board | **no phase yet** — **and this is a model decision, not a screen one** | Board 06's own rows are `Submit: Draft → In Review`, `Approve — Finance: In Review → Finance OK`, `Approve — Legal: Finance OK → Legal OK`, `Approve — Director: Legal OK → Approved`. That is a **stage model**: the Document itself holds a multi-state status and each stage's transition is gated on a *role*, declared once per Application. This app does the opposite, on purpose: a Document's status is **derived** from its Approval Steps (`evt_step_decision_rollup`, three values), and who approves each step is **per-record data** the submitter picks in the wizard (`fld_assignee`/`fld_approver_group`), not metadata. Under that model there are exactly two role-gated transitions in the whole Application — Approve and Reject on a step — which is what the shipped matrix correctly shows, six `System` rows and two real ones. **Neither model is wrong and this row does not pick one**; what it records is that the screen cannot be made to resemble the board without first changing the approval model, and that no amount of work on the screen is the missing piece. **Three ways forward, written down 2026-09-21 after the owner asked what this row actually means:** *(a) keep the per-record chain* -- flexible, no standard flow, and board 06 never resembles its drawing; *(b) adopt the stage model* -- a fixed per-Application flow gated on roles, matching the board, at the cost of reworking the wizard, the rollup and boards 07-10; *(c) **CAP-V28**, the saved default approval flow per Document Type* -- the chain becomes a declared template without status becoming a stage machine. **(c) is the recommendation**: it delivers what the board is actually reaching for (a flow that is dependable and readable rather than rebuilt per document) without dismantling a model that works. Owner decision, still open. Upstream's own `process:` overlay is the stage model, and its board-06 copy citing `ProcessEdge` and `/{machineID}/process-map` is drawn against it -- the same "mockup draws data the app doesn't have" class as board 03's Procurement/HR cards and board 07's hour-scale SLA. A middle path exists and is already a row below: **CAP-V28**, a saved default approval flow per Document Type, which makes the chain a declared template without making status a stage machine |
  | `draft` as a Document status (board 06: `Submit: Draft → In Review`, `Reopen: Rejected → Draft`) | **no phase yet** — lands with the wizard's third step, below | Two of board 06's six rows need a state `metadata/document.yaml` deliberately removed, with its own note: *"add it back only alongside a real save-as-draft flow, not speculatively."* That flow is the wizard's third step, which has no board anywhere. So this is the same deferral seen from the matrix's end, not a second one — worth listing because from board 06 it reads as a missing transition rather than a missing flow |
  | A transition with more than one `from` (board 06: `Reject: any pending state → Rejected`) | **no phase yet** — **a second real case, not a phase** | `domain.Transition` names exactly one `from`, so "any pending state" is declarable only by enumerating one edge per source state. With `fld_decision`'s single open value that enumeration is one row and costs nothing, which is precisely why the shape should not be generalized yet; it starts costing the moment a status Field has several non-final states, which is the stage model above. Recorded so the board's wording is not read as an unimplemented feature of the matrix |
  | Board 06's columns mix two role namespaces | **answered 2026-09-21: the screen renders Application roles only, and the board's two Workspace columns are dropped** | Its five columns are `ADMIN`, `FINANCE REVIEWER`, `LEGAL REVIEWER`, `DIRECTOR`, `MEMBER`. `ui-sample/member-role-detail.html` settles what those words are, in the owner's own design language: a "Workspace role" section holding exactly `Admin`/`Member` — captioned *"controls workspace administration, independent of application permissions"* — above a separate "Application access" section whose Document Approval vocabulary is Approver/Submitter/Reviewer. So **two** of board 06's five columns are Workspace-level, not one, and the other three appear in no declared vocabulary at all. The shipped screen puts Workspace rules in their own section with no role columns (`06b-authorization-matrix.html`), which is also why `workspace_role:` is a separate key from `roles:`: that same mockup gives HR an *Application* role literally named `Admin`, so one list would make `roles: [admin, approver]` an alternation across two different meanings. `Admin` and `Member` are this runtime's **Workspace** roles (`workspace_members.workspace_role`, what `requireWorkspaceAdmin` reads); the other three would be **Application** roles (`application.roles:`). The shipped screen renders Application roles only, which is the honest choice — a column whose ✓ means a different kind of grant than its neighbour's is worse than an absent column. Whether a Workspace admin should appear as an always-✓ column is an owner call, and it is a real question rather than a rendering detail: admin today bypasses no Permission at all |
  | Process map — the same edges drawn transition-first (`/{machineID}/process-map`, upstream CAP-W05 ✅) | **no phase yet** | Board 06's own copy says the matrix "re-projects `ProcessEdge` (already rendered per-transition on `/{machineID}/process-map`) role-first instead of transition-first" — describing that route as already existing. It does not exist here; Fase 7 built the role-first projection only. Upstream's is *derived*, needing no new metadata concept, and `domain.Machine.Transitions` is now exactly the input it reads, so this is a small screen rather than a capability — it simply has no case asking for it yet |
  | Board m06's mobile layout (role **chips** per transition, not a grid) | **no phase yet** | The desktop grid ports; the mobile board drops the matrix shape entirely and lists `✓ Admin`, `✓ Finance Reviewer` as chips under each transition, which is a different component, not a breakpoint. The shipped screen scrolls the table horizontally instead — usable, not the drawn design. Same class as the bottom-bar row below: a real mobile design exists and is unported |
  | Member full name beside the email (board 04/05) | **Done (2026-09-22)** | Closed by the identity rework, not by the phase it was pinned to. A full name is now identity data (`credentials.full_name`, migration 010) rather than a Field on a per-Workspace `mch_user` record, so `showWorkspaceMembers` resolves it through `data.Store.MemberNames` and the row renders name above email as the board always drew it. `memberInitials` became `personInitials(name, email)`, using the real name when there is one and falling back to the email local part for an invitation nobody has accepted yet — which has no name by design, since only its owner ever states one |
  | ~~Section tab strip (board 07)~~ — *done in 6a, and re-answered 2026-09-21*: 6a built `rendering.inboxTabs` as page chrome — two hardcoded `?tab=` entries plus Groups and Admin borrowed from the Workspace menu — on the reasoning that a second `navigation:` level "never materialized". The owner rejected both halves: Groups/Admin were removed (already reachable from the launcher, so re-listing them was duplication), and the strip is now a **projection of Document Approval's own level-1 `navigation:`**, which is exactly two items — `nav_approval_inbox` and `nav_my_documents` (`/approval-inbox?tab=mine`). So the case for a second level still never materialized, but for the opposite reason than recorded here: a level-1 item's route may carry a query, which is all "My Documents" ever needed. A `tabs:` metadata level was drafted and deleted unbuilt — *"dengan level 1, kebutuhanku sudah cukup"* | — | — |
  | Hour-scale SLA wording (board 07: "SLA breached · 4h"; board 10: "Breached · 4 hours ago") | **no phase yet** | `fld_due_date` is `type: date` — a bare calendar date, no time component — and `domain.KnownFieldTypes` has no datetime type at all. `experience.EvaluateSLA` truncates to day *deliberately*, which is what makes "Due today" mean anything. Adding a Field type to satisfy one label is shape-before-need; the trigger is a second, independent caller that genuinely needs sub-day precision. Noted at the site in `ApprovalInboxPage`'s and `ReviewDocumentPage`'s own doc comments. **Two screens now want it and it is still not built** — that is the row working, not a row going stale: both are the same board's day-vs-hour mismatch, not two independent callers |
  | ~~Review document as its own screen (board 10)~~ — *done in 6b*: `reviewdocument.templ` + `composition.ReviewDocument`, which is what let `detail.templ` out of `projectionRatchet` | — | — |
  | The wizard's third step (boards 08/09 both say "OF 3") | **no phase yet** — and it is a *flow* question, not a screen | No step-3 board exists anywhere in the set, so what it would contain is inference. Behind the label sits the real finding: `submitDocumentWizard` sets `fld_status = in_review` at creation, **before** any signature is placed — so an approver can decide a Document while the submitter is still on the placement screen, and abandoning the wizard leaves a live in-review Document with no placements. A third step would be where sending actually happens, which is also what would earn `draft` back as a `fld_status` option (`metadata/document.yaml` already says: "add it back only alongside a real save-as-draft flow, not speculatively"). The wizard says "Step 1 of 2" meanwhile, because two screens exist |
  | Conditional required (a Field required only when a sibling Field holds a given value) | **no phase yet** | `domain.Constraint` has one shape — block a field transition while a *related Machine* has a matching record — which cannot condition on a sibling Field of the same record. The one real case is live as of 6c-2: `fld_assignee` is required for a User-held Approval Step and meaningless for a Group-held one, so it is declared optional and `internal/web`'s `parseStepInputs` enforces the pairing in Go. Upstream declares the same rule as two conditional Constraints, so the shape is known. Trigger is a second real case, as always |
  | Dashboard (`/dashboard`) still on `pageStyles` | **no phase yet** | **Moved to Project Management on 2026-09-21**, which is the Application whose data it actually composes (Projects, Tasks, Activity); it had been declared under Document Approval only because the two were one Application before Fase 3. That reclassification also drops the "most visible unported screen in Case 3" framing this row used to carry — it is not a Case 3 screen at all. Its board is `12-approval-dashboard-UNUSED.html`, marked TIDAK DIPAKAI by the owner, so there is no mockup to port it against: the blocker is a design decision, not engineering. Either the board is un-retired, or the screen is ported to the Tailwind idiom the other four Case 3 screens now share without one |
  | Document / Approval Step detail pages still on `pageStyles` | **no phase yet** — lands with whatever ports Project Management | They are `detail.templ`, the *generic* record view every Machine shares, so porting it reaches `mch_task`, `mch_project` and the rest. Fase 6b and 6c-3 both deliberately split a Component rather than port it for this reason. The cost is visible today: the Signature positions screen's own back link lands on an old-style page, mid-flow |
  | Board 09's zoom controls, selected-step highlight, and approver department sub-line | **no phase yet** | Zoom and selection are ephemeral view state nothing stores or needs — the app serves one rendered size and the screen works without a selection model. The department ("Finance", "Director") has no Field anywhere: `mch_user` holds none, and 3b's Application role is per-Application, not per-person-per-step. `fld_step_name` is rendered in its place, which is what the step is actually *for* |
  | Uploaded file size (board 10: "6 pages · 2.4 MB") | **no phase yet** | the page count is real (`internal/pdf.PageCount`) and is rendered; **the byte size is stored nowhere** — not on the record, not in `internal/storage`'s key, which embeds only the original filename. It needs a write-path change (capture size at upload) plus a Field to hold it, for one label. The trigger is a second caller that needs file metadata, not this line |
  | Step names with real values (board 10: "Finance Review") | **Fase 6c** | `fld_step_name` is declared as of 6b and `composition.stepLabel` reads it; nothing *writes* it until board 08's wizard collects it. Until then every step is titled with its assignee, which is exactly what the app did before the Field existed — the fallback is the honest state, not a placeholder |
  | New chrome on the Document Approval screens (~~inbox~~, ~~review~~, submit, signature) | **inbox done in 6a, review in 6b**; submit and signature both 6c | Preflight ties chrome to content; their content ports there. Signature moved out of 6b — board 09 is `STEP 2 OF 3` of the submit wizard and shows Group approvers (CAP-F24), so it belongs with board 08, not before it |
  | ~~Approval Role Matrix (board 06)~~ — *done in Fase 7*: `/approval-role-matrix`. The blocker this row named ("neither in 006/007") was answered by step zero rather than by building both from scratch — upstream had already shipped CAP-P01 and a `transitions[]` shape; what this repo's concept docs say about them turned out not to be the binding question | — | — |
  | Declared `requires_role:` on a navigation item | **a second real case, not a phase** — and **Fase 7 came and went without supplying one**, which this row predicted it would ("likeliest around Fase 7") | Re-checked at the Fase 7 close, which is the only reason this line is honest. Fase 7 *did* add a third id to `appShell`'s `hiddenNavIDs` (`nav_role_matrix`, beside Members and Groups) — but all three are gated on the **Workspace** role (admin), which a handler naming an id expresses perfectly well. The trigger this row names is an item gated on an **Application** role, the case a handler *cannot* express, and role-based Permission landing did not create one: no navigation item in the manifest is scoped to an Application role. Three instances of the same shape is, separately, the B1 repetition signal — so the next hardcoded id is worth running through the decomposition criteria even if the trigger below still has not arrived |
  | ~~Mobile bottom bar for an Application's own menu~~ | **done 2026-09-22** (`applicationBottomBar`), extended with a "More" sheet 2026-09-24 | **This row was stale for two further closes after the thing it describes shipped**, which is worse than the wrongness it already records below: a row saying "no phase yet" about built code reads exactly like a real gap, and the 2026-09-23 Flow 2 gap study had to re-derive from the source that the bar existed. Kept, struck through, with its own history: it previously said "nothing in Fase 2–7 displays it", which stopped being true at Fase 3a — Project Management declares no `show_nav`, keeps its menu, and `/my-tasks` renders ten nav links. It then said the blocker was Project Management still being `pageStyles`; those screens ported to `appShell` on 2026-09-22 and the bar came with them. Three wrong states, one cause: nobody re-read the table, which is the one thing this table needs |
  | Saved default approval flow (board 08: "Save this as the default approval flow for **Contract** documents", checked by default) | **no phase yet** | Upstream built it (CAP-V28) as two companion Machines — one template per Document Type, plus its own ordered steps — with a write direction that find-or-creates the template on submit. That is the real blocker: a template entity and a write path, not the screen. **This row exists because the deferral did not.** It has been live since Phase 15, recorded only inside `menata-app-document`'s development history, in no table anyone re-reads at a phase close — and with a reason that was wrong when written ("only one Document Type exists in metadata today"; there were zero) and is wrong now in the other direction (there are three: `Kontrak`, `Tagihan`, `Lain-lain`). Same shape as the bottom-bar row that sat wrong through four closes |
  | One signature box for a Group-held step (boards 08/09) | **no phase yet** — **named, not solved** | Board 09 places exactly one signature box for `Legal Group · 4 members`, and nothing on either board says whose signature image lands in it, or what happens when two of the four act. Upstream has the identical gap on its own compositing capability, recorded there in the same words. Worth holding here rather than discovering it during 6c-2's port |
  | Member search box (board 04) | **no phase yet** — "Planned: search, filtering and pagination" below | search does not exist anywhere in the app |
  | Write-side Binding: a form input's `name=` hardcoded to a Field id (007 §11.3) | **no phase yet** — **and it is the gap a shrinking ratchet hides** | `signatureplacement.templ` carries twelve `name={ action.Field… }`, `documentsubmit.templ` four (this read "six" until it was counted, 2026-09-21), while `machine.templ` already does it generically (`name={ f.ID }` over `m.Fields`). `TestRenderingUsesProjectionNotRawValues` gates *reads*, never writes, so both files could leave `projectionRatchet` with every binding still hand-typed — which is exactly what happened in 6c-2 and 6c-3. Recorded here so the ratchet count is not read as a composability score. Trigger: a third bespoke write screen, or the generic update route stopping its whole-record rewrite (the same route that forced 6c-3's carry-forward list). **Half of that trigger arrived 2026-09-21, and the count did not move at all** -- which is the useful part. Board 09's forms left the generic route for one that writes four named Fields, so the carry-forward echo is gone; the twelve hand-typed bindings are exactly as hand-typed as before. The first guess while writing this row said the count would drop to four, and checking the file said twelve: the echo rendered `name={ f.Name }` from a derived list, never `action.Field*`, so it was never part of this number. The whole-record rewrite was the reason the *echo* existed; it was never the reason the *bindings* are literal, and removing it changes nothing here. The other half of the trigger -- a third bespoke write screen -- is still the one to watch |
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
- **The read diagnostic is measuring the wrong half of the request** (log review 2026-09-22, owner
  request; **all four steps shipped 2026-09-22**, see the close at the end of this entry). Six
  hours of `/var/log/menata-app/app.log` — 1853
  diagnostic lines — say the runtime is healthy on every axis anyone would check first: 7.4M
  resident (peak 9.3M), **798ms of CPU across six hours**, 40 req/min at the busiest minute, and no
  route whose `reads=` grows with the number of records in it. There is no N+1 here and no slow
  query. What the log does have is a **blind spot in the instrument itself**, which is worse than a
  slow page because it is the thing that would have told us about one.

  `queryDiagnostics` is registered at `internal/web/router.go:135` — *after* `requireAuth` (:126),
  `currentWorkspace` (:129) and `requireApplicationAccess` (:134) — and `data.ReadLog.record` is
  called from exactly four Machine-level methods (`internal/data/store.go:101,126,145,182`). No
  identity, Workspace or membership query calls it. So its own doc comment's promise
  (`internal/web/middleware.go:114`, "reports what each request actually read") is not what it
  delivers, and `repeated=0` — printed on 1,700+ of those lines — is reassurance the instrument is
  not entitled to give. This is the same failure mode as the deferral table above, in the read
  path rather than in prose: a number nobody re-checks against what it actually counts.

  | # | Finding | Evidence |
  |---|---|---|
  | 1 | **Up to six queries run before the counter starts.** `CurrentSessionGeneration` (`data/store.go:379`), `ResolveUserWorkspace` (`data/workspace.go:318`), `GetWorkspace` (`web/currentapp.go:33`), and `ActorMembership` (`data/group.go:340`) — which is **three** queries on its own: the group JOIN, `appRolesFor`, and the `workspace_role` row | Read statically from the middleware chain, **not yet measured** — step 1 below exists to replace this row with a number |
  | 2 | **The same identity rows are re-read 2-3× per request, and `repeated=` cannot see it.** On `/approval-inbox`: `GetWorkspace` twice (`web/currentapp.go:33`, then `web/chrome.go:42`), the membership row three times (`ActorMembership`, `resolveChrome`'s `GetMembership` at `chrome.go:58`, `viewerWorkspaceContext`'s at `chrome.go:101`). `composition.Loader` is already this repo's request-scoped memo and covers Machine reads only; the chrome path has none | 387 hits logged at `reads=5 repeated=0`, against ~15 queries read off the call chain |
  | 3 | **`requireApplicationAccess` computes the Actor and throws it away** (`web/currentapp.go:152`), and `currentWorkspace` resolves the Workspace row and keeps only two fields off it (`:33-34`). Both are already on the hot path; both answers are wanted again downstream | The cheapest fix in the list — it removes reads by *keeping* work already done, not by adding a cache |
  | 4 | **The nav badge recomputes the whole Approval Inbox to print one integer.** `showPendingCount` (`web/approval.go:94`) runs the full `composition.ApprovalInbox`; on `/approval-inbox` itself that composition therefore runs **twice per screen**, since the page handler already holds the number (`web/approval.go:52`, `len(inbox.Pending)`) | 463 hits vs 1390 page loads: it fires on a third of all requests. `machine.templ:144` is `hx-trigger="load"` — one shot per page, *not* polling, which is the one thing worth not "fixing" |
  | 5 | **`/decide` is the worst logged ratio: `reads=12 repeated=7`.** Part of that is legitimate — `Loader`'s own doc (`composition/loader.go:27`) forbids one memo spanning a write — but seven repeats implies re-reading *before* the write too. `/documents` (`reads=7 repeated=5`) and `/home` (`mch_document x2`) are the same shape, smaller | Low frequency (POST), so ranked below 1-3 despite the worse ratio |
  | 6 | **Client disconnects are logged as server faults.** `internal error: context canceled` / `get session generation: context canceled` (2026-09-21 03:28) are a browser navigating away, reported through `serverError` as a 500 | Two lines in six hours — trivial volume, but it puts noise in the one channel a real fault would arrive on |
  | 7 | ~~**Three overlapping indexes on `records`**~~ — **this row was wrong, and the way it was wrong is the point.** It was written by reading `CREATE INDEX` lines out of the migrations and named `idx_records_machine_sort` (002) as live; migration 004 line 20 *drops* it, so it does not exist. Checked against `pg_indexes` on 2026-09-22: four indexes, and the redundancy is a different one — `idx_records_workspace_machine` is a strict prefix of `idx_records_workspace_machine_sort`. **The real finding is bloat, not redundancy**: 66 live rows in a 72 kB table carrying **21 MB of indexes**, residue of `make threshold` seeding and deleting hundreds of thousands of rows, which autovacuum marks reusable but never shrinks. **Fixed** — one `REINDEX TABLE CONCURRENTLY records` took it to **64 kB**, and `cleanupBench` now runs it on every harness run. A second wrong claim was corrected on the way: the `Planning Time 0.625ms` vs `Execution Time 0.084ms` ratio this row first blamed on the bloat survived the REINDEX unchanged, so it is a cold-relcache artifact of measuring through `psql`, not a cost the app pays (see the query-layer entry below) | Measured, not read off the DDL — which is what this row failed to do the first time, twice |
  | 8 | **`ListRecordsBy` filters on `data->>$2 = $3` with no supporting index** (`data/store.go:140`) and cannot easily have a plain expression one, since the Field id is a bound parameter. Today it scans a handful of rows behind `idx_records_workspace_machine` and costs nothing | The actual scaling wall, named now so it is not discovered later. **No phase** — a row count that makes it hurt is the trigger, not this entry |

  **Order of work, and why it is this order:** (1) move/extend the instrument until the log tells
  the truth — findings 1 and 2 are read off a call chain, and this repo's own rule is to check a
  live claim against the data rather than against having written it, so every number above stays
  provisional until the diagnostic itself prints it; (2) findings 2 and 3 together, which is one
  change (resolve identity and Workspace once, on ctx, at the chokepoint that already resolves
  both); (3) finding 4; (4) findings 5 and 6. Findings 7 and 8 are recorded, not scheduled.

  **What this is not**: a performance problem. Nothing in the log is slow, and at 40 req/min
  nothing here would be. It is on this list because the runtime's own claim (001 Principle #6 —
  the resolved result of inference must be *exposed through diagnostics*) is currently half-kept,
  and a diagnostic that under-reports is the failure that hides the next one.

  ### Close, 2026-09-22 — and the numbers above were wrong

  **Every count in the table above was read off a call chain, and the first honest measurement
  disagreed with all of them.** The entry estimated ~15 queries for `/approval-inbox`; measured
  through the real router, `/home` alone issued **17**. The reason was a method nobody had
  counted: `data.Store.GetMembership` is **four** queries, not one — the membership row,
  `appRolesFor`, and `GroupsByMember`'s two, the last of which loads *every* Group in the
  Workspace and *every* group membership in order to find one person's. `/home` called it twice,
  so eight of its seventeen queries were one question asked twice. Nothing in the old log could
  have shown this; it printed `reads=3` for the same request.

  That is the entry's own argument landing on the entry: a number that is not measured is not a
  number. Measured results, by `internal/web`'s new `TestAuthenticatedPageQueryCost` /
  `TestNavBadgeQueryCost`, which assert these through the real middleware chain rather than a
  bare handler:

  | Route | Before | After |
  |---|---|---|
  | `/home` | `queries=17 reads=13 repeated=3` | **`queries=12 reads=12 repeated=0`** |
  | `/api/approval-inbox/pending-count` | 4 reads + a possible *write*, within ~10 queries | **`queries=5 reads=5 repeated=0`**, no write |

  What shipped, against the four steps planned:

  1. **The instrument** — `data.QueryTracer` (a `pgx.QueryTracer`, installed on the pool by
     `db.Connect`, wired in `cmd/server/main.go` because `internal/db` may not import
     `internal/data`) counts every statement at the driver, so no Store method can forget to.
     `record()` stays for the *names*, and the gap between the two counts prints as `unnamed=`
     rather than being reconciled away — a statement that did not name itself is exactly what this
     whole entry was about. `queryDiagnostics` moved to the **top** of the authenticated group.
     Two conformance gates hold both halves: `TestQueryDiagnosticsRunsBeforeAuth` and
     `TestPoolInstallsQueryTracer`. **Both were verified to fail** when the wiring is undone, which
     is the only thing that makes them worth having.
  2. **Identity resolved once** — `web.resolveIdentity` puts this request's Workspace row,
     membership, Actor and viewer name on ctx, the shape `data.WithWorkspaceScope` already set the
     precedent for. `ActorMembership` turned out to be redundant on this path entirely:
     `domain.Actor`'s four parts are all derivable from a `Membership` that already carries
     `AppRoles` and `Groups`. **Corrected mid-implementation**: the first version resolved
     everything eagerly and made the badge cost ten queries to print one integer — the exact cost
     `resolveChrome`'s old doc comment warned a middleware would impose. The membership is now
     resolved *lazily and memoized*, the Workspace row eagerly (`currentWorkspace` needs it on
     every request). Every consumer keeps a fallback for a handler mounted without the middleware,
     which is how most of `internal/web`'s own tests run.
  3. **The badge** — `composition.PendingApprovalCount`, two reads and no write, sharing
     `pendingStepsFor` with `buildInbox` so the badge and the list it links to cannot drift apart.
     Worth recording separately: `ApprovalInbox` **writes** (`logSLABreaches`), so the old badge
     was a GET that wrote SLA-breach rows on a third of all requests. Its dedup is read-then-write
     with no unique constraint, so genuinely concurrent composers would double-log; the page/badge
     pair is *not* concurrent (`hx-trigger="load"` fires after the page response completes), and
     the dev database holds **zero** breach rows, so this has never fired here — **unobserved, not
     disproven**.
  4. **The two small ones** — `serverError` now separates a client disconnect
     (`context.Canceled`) from a server fault, so the error channel stays meaningful. And
     `/decide`'s largest repeat was found and removed: it called `store.GetRecord` for the step,
     then `eventOldValues` re-read *the same row* a few lines later — not because of the write
     (which had not happened yet) but because `applyApprovalSignature` mutates `step.Values` in
     place, leaving the handler with no copy of what it started with. `snapshotValues` is what the
     situation actually called for. `eventOldValues` stays right for its other two callers, which
     build new values from a form and genuinely have not read the old ones.

  **`TestHandlersStaySmall` caught the extraction on the way**: inlining that copy pushed
  `decideStep` to 82 lines, and the budget's own advice — move it, don't raise the number — is
  what produced `snapshotValues` as a named helper instead of six lines in a handler.

  **Still open, and now measurable rather than inferred:** `GroupsByMember` reads the whole
  Workspace's groups to answer a question about one member, while `ActorMembership` already has
  the targeted per-user query for exactly that. Two of `/home`'s remaining queries are that shape.
  Not changed here because it alters what `Membership.Groups` costs for every other caller, which
  deserves its own look rather than a ride on a diagnostics change.

  ### Round 2, same day — the gates, and the seven routes they found

  Asked whether this should be gated or left to periodic audit, the answer turned out to be
  neither wholesale: **gate the invariants, not the budgets.** A budget is a threshold someone
  chose, and fifty-four of them would be fifty-four numbers to maintain and an invitation to raise
  whichever fails; `queries == reads` and `repeated == 0` on a read path have one correct value
  that does not move with data volume. Three things landed:

  1. **The log marks its own anomalies** (`web.anomalyPrefix`) — `ANOMALY(unnamed,repeated)`, so
     the next review is a grep rather than a read of 1853 lines. This is the half that covers
     *production traffic on routes no fixture exercises*, which no gate can. `repeated` is flagged
     on GET/HEAD only: `composition.Loader` may not span a write, so flagging a POST would train
     the reader to ignore the marker.
  2. **`TestGetRoutesDoNotWrite`** — a GET may not reach a Store write, call graph walked across
     `internal/web` and `internal/composition`. Verified failing with the exception removed, and
     it traces the whole chain (`showApprovalInbox -> ApprovalInbox -> logSLABreaches ->
     CreateRecord`). One declared exception. Notably `showPendingCount` is **not** among the
     offenders, which is the badge fix above holding.
  3. **`TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed`** — sweeps **22** routes, discovered by
     parsing `router.go` so a new one is covered automatically.

  **And the sweep immediately justified itself.** The two budget tests covered two routes out of
  fifty-four, both of them ones just worked on — coverage that reassures more than it protects.
  Run across every param-free authenticated GET, it found **seven more routes with the same
  disease**, none of it previously visible:

  | Route | |
  |---|---|
  | `/workspace-members` | `unnamed=3, repeated=6` — the worst; per-member membership and group reads |
  | `/workspace-groups` | `repeated=5` |
  | `/authorization-matrix` | `repeated=4` |
  | `/documents/new` | `unnamed=2, repeated=3` — approver picker re-reads groups per row |
  | `/documents/new/approver-row` | `unnamed=2, repeated=1` |
  | `/switch-workspace` | `unnamed=1, repeated=1` |
  | `/create-workspace` | `unnamed=1` |

  These are grandfathered in `getSweepRatchet`, **which may only shrink** — the same posture
  `projectionRatchet` takes, and for the same reason: holding the gate hostage to fixing seven
  routes first is how the check that would prevent the eighth ends up not existing. `resolveIdentity`
  fixed the *request-level* duplication; every `repeated=` above is the *within-handler* kind it
  cannot see — a loop that asks the database about each member separately.

  **One of the original findings turned out to be a labelling artifact**, and it is recorded
  because the correction matters more than the finding: `/home`'s `mch_document x2` (present in
  the very first log read) was `CountRecords` and `ListRecords` sharing one `record()` target.
  They are different statements; the repeat was in the label, not in the request. `CountRecords`
  now records `"<machine> count"`. Whether counting a Machine the page has *already listed* is
  itself waste is a real question this naming now makes askable.
- **What a composable runtime actually costs, measured against a conventional app** (owner
  request, 2026-09-22, immediately after the entry above; **study done, one item actionable, the
  rest trigger-gated**). Full method, evidence and repeatable benchmark scripts:
  `menata-app-document`'s `audits/2026-09-22-kinerja-composable-vs-konvensional-kajian.md`.

  The question was whether being metadata-driven makes this app slower and heavier than one that
  hand-writes its models, queries and pages. **Mostly it does not, and the part that does is not
  the part that gets blamed.** Metadata is compiled once at startup (`cmd/server/main.go:41`), the
  31 `.templ` files are compiled to Go, and the process lives in 6.8M resident — so the per-request
  cost of the runtime *being* a runtime is effectively zero. `003-runtime-language.md`'s
  distinction ("compiled to runtime-internal representations, never a flat metadata-to-View
  dispatch") is what buys that, and this measurement is the first evidence the implementation
  actually honours it.

  The one real divergence is the data path, and it has one shape: **load the whole thing, then
  filter in memory.** Measured on 200k synthetic rows against the real `records` shape, and on
  100k records through the real Go decode path:

  | Finding | Evidence | Trigger |
  |---|---|---|
  | **JSONB is not the problem.** A field filter costs 1.66ms on the generic `records` shape with an expression index vs. 0.69ms on a typed column with a btree — 2.4x, both under 2ms. Storage is +42% | Appendix A of the audit, `EXPLAIN (ANALYZE, BUFFERS)` output quoted verbatim | **None — this closes a question rather than opening one.** Replacing JSONB with per-Machine tables would spend the product's own premise to buy 1ms |
  | **The missing index is the problem, and it is finding #8 above with a number on it.** `ListRecordsBy`'s `data->>$2 = $3` is 78ms Seq Scan (199,800 rows discarded to return 200) without an expression index and **1.66ms** with one — 47x, the largest ratio anywhere in the study | Same appendix | A row count that makes it hurt, unchanged from finding #8. What is new: the *fix* now has a named shape — index declared in metadata (007 §21.4), which is the only form that keeps it metadata-first instead of a hand-written migration per Machine |
  | **The dominant cost is decode, not SQL.** `ListRecords` on 100k records: 805ms to decode JSONB into `map[string]any`, **6ms** to then filter it in Go, **+97MB heap per request**. Ten concurrent requests on a Workspace that size is ~1GB on a process that runs at 6.8M today | Appendix B, a standalone Go program | Same as below. Worth stating plainly because it inverts the intuition: composition is the cheapest part of composing, by two orders of magnitude |
  | **Filter/Projection/Aggregate pushdown (007 §21.1-21.3) are generalizations of things already in the tree, not new concepts.** `expression.Comparison` is a closed vocabulary with no parser (so compiling it to SQL is injection-safe *by construction*, 007 §9.1) and `Measure.Where` already uses it — in Go, after every row is loaded. `domain.Dataset{Dimension, Measures}` is already shaped like `GROUP BY dimension, agg(measure)`. `ProjectCardFields`/`card_fields` already knows which fields a screen uses, and uses that knowledge *after* the full JSONB is fetched | `composition/dataset.go:62`, `domain/dataset.go:57`, `internal/planner`+`internal/ir` still `doc.go`-only | **One Machine past ~5,000 rows in one Workspace, or one route whose `queries`/latency tracks record count.** `internal/planner`'s own doc says to build it against real forcing cases; neither has arrived. Pagination (Planned, below) is cheaper than all three if what is wanted is only list screens |
  | **The same shape is already live on the identity path**, not just the record path: `GroupsByMember` reads every Group and every group membership in the Workspace to answer a question about one member — the "still open" note directly above, which this study reclassifies as the same finding rather than a separate one | Two of `/home`'s twelve queries | Already named above; the reclassification is the point, because one fix (push the predicate down) covers both paths |

  **Actionable with no trigger at all, and the only such item: there is no response compression and
  no `Cache-Control` on `/css/*` or `/vendor/*`** — `grep` for gzip/compress/Cache-Control/etag in
  `internal/web` returns nothing. Metadata-driven HTML is unusually repetitive and compresses well,
  so this is the largest perceived-speed win per line of code in the whole study, and the only one
  that touches no architecture. It is an omission, not a design choice.

  **What this is not, again:** a performance problem. 66 records, 40 req/min, `/home` at
  `queries=14 reads=14 repeated=0` against the real installed Workspace, which is exactly
  `maxQueriesPerAuthenticatedPage` — so the budget is tight rather than slack. The next move on
  that path is removing a query, not adding one, and raising the constant to pass is the thing it
  exists to prevent. (This read `queries=12` until the sweep measured against a Workspace with
  Applications installed; the two extra are each Application's own summary-Machine count, which
  the page genuinely needs.)

  **One methodological correction the study made to itself**, recorded because it is the entry
  above's own lesson arriving one level up: two of its Tier-1 proposals (the nav badge recomputing
  the whole inbox; `context.Canceled` logged as a 500) were read off `92e841b` and were **already
  fixed** in an uncommitted working tree that landed as `f1767b0` mid-study. In a checkout several
  sessions share, `git status` is evidence-gathering, not a formality. A third proposal was wrong
  on the merits too: a bare `CountRecords` for the badge would have counted something different
  from what the page shows, which is why `PendingApprovalCount` shares `pendingStepsFor` with
  `buildInbox` instead.
- **The query layer itself, and the number the repo already had** (owner request, 2026-09-22,
  closing the day: *"kalau dikaji dari sisi query, apakah ada yang bisa dioptimalkan? apakah ada
  benchmark atau best practice untuk ini?"*). Full study:
  `menata-app-document`'s `audits/2026-09-22-lapisan-query-dan-indeks-kajian.md`.

  **Answer: nothing in the query layer needs optimizing.** No query exceeds 1ms and every plan
  chooses a Seq Scan, which is correct on 66 rows in a four-page table. The standard advice — add
  indexes, tune queries, raise the pool — misses on all three counts here: four indexes exist and
  none is used at this volume, there is no slow query to tune, and on 1 vCPU `pgxpool`'s default
  `MaxConns = 4` sits against one active connection. A GIN index for `data->>$2` would be the
  wrong answer for a different reason: this app pulls every row and filters in Go, so it would
  speed up a filter that never runs.

  **THE BENCHMARK ALREADY EXISTED AND HAD ALREADY ANSWERED.** `make threshold`
  (`internal/composition/threshold_test.go`, Phase 18 Step 4) measures exactly this, and
  `TestVolumeThreshold` closes with the literal instruction *"record it in ROADMAP.md Phase 6"*.
  Nobody did — which is how the performance study directly above came to build two fresh
  benchmarks for a question the repo could already answer. Recording it now, which is the whole of
  what that instruction asked:

  | rows | whole-Machine read | |
  |---|---|---|
  | 100 | 900µs | |
  | 1,000 | 6.2ms | |
  | 10,000 | 42.4ms | |
  | **50,000** | **232.5ms** | **past the 100ms interactive budget** |
  | 100,000 | 980.7ms | |

  10k→50k is 5x the rows for 5.5x the time (linear); **50k→100k is 2x the rows for 4.2x the
  time** — superlinear, and the break is `json.Unmarshal` plus allocation pressure in
  `data.queryRecords`, not the scan. That is Finding D of the study above arriving from a second,
  independent direction. `TestBreadthThreshold` separately shows read count **flat** in schema
  breadth (1 read for 64 referring Machines, `served-from-memo=63`) — `composition.Loader`'s memo,
  still locked.

  **Two triggers, ten times apart, and not in conflict** — worth saying because otherwise they
  read as two answers to one question. **50,000** is where the existing "read the whole Machine"
  pattern leaves the interactive budget. **~5,000** is the study above's trigger for *starting to
  build* pushdown, deliberately more conservative because building it takes time and must not
  begin on the day it is already late.

  **`make threshold` was bloating the database it measured.** It seeds hundreds of thousands of
  rows and deletes them, and autovacuum never shrinks a btree — it only marks pages reusable. The
  state found: 66 live rows, 72 kB table, **21 MB of indexes**, and running the harness once for
  this study is what took it from 9.7 MB to 21 MB. One `REINDEX TABLE CONCURRENTLY records` brought
  it to **64 kB**, and `cleanupBench` now does it on every run, so the harness repairs what
  measuring costs instead of charging it to everything afterwards.

  **A correction this entry owes itself.** The study first attributed a `Planning Time` of 0.6ms
  against an `Execution Time` of 0.08ms — planning seven times dearer than execution — to that
  bloat. Re-measured after the REINDEX, with indexes 336x smaller, **planning did not move**
  (~0.6ms, ~150 planner buffers). The bloat was real and worth fixing; the evidence attached to it
  was not. The 7x ratio is an artifact of measuring through a fresh `psql` backend with a cold
  relcache, and it does not describe the app at all: `pgxpool` holds connections and pgx v5
  defaults to `QueryExecModeCacheStatement`, so planning is amortized per connection, not paid per
  request. **Second time in one day that a claim of this entry's own was read rather than
  measured** — the first being the index that migration 004 deletes.

  `session_generations` also held **369 rows for three credentials**, 367 of them orphans — the
  largest table in the dev database, larger than `records` (66), because `cleanupAuthTest` swept
  five tables and never that one. Now swept, before the records it keys on, since a subject is an
  `mch_user` record id and the subquery finds nothing once the records are gone. The 366 orphans
  already there were deleted on owner approval, keeping the configured admin subject.
- **The ratchet goes seven → three, and 359kB comes off a cold load** (2026-09-22, the last round).
  **The seven routes had three causes between them**, which is why four left the same day rather
  than one at a time:

  | Cause | Reach |
  |---|---|
  | **Ten read methods in `internal/data` carried no `record()` target** (`ListMembers`, `ListMemberships`, `appRolesByMember`, `WorkspaceBySlug`, `GetGroup`, `GroupIDsForMember`, `GroupMemberIDs`, `attachGrants`, `GetPendingInvite`, `ListPendingInvites`) | **every `unnamed=` in the sweep**, now zero |
  | **`requireWorkspaceAdmin` re-read a membership `resolveIdentity` had already resolved** — and `GetMembership` is four queries, not one | all three admin screens at once (`/workspace-members` was `repeated=6`, `/workspace-groups` 5, `/authorization-matrix` 4) |
  | **`GetMembership` answered "which Groups is this one person in" by reading every Group in the Workspace** — while `ActorMembership` already had the targeted query. Now `data.GroupsForMember`, used by both | the "still open" note above, closed; `/home` 14 → 13 queries |

  `/authorization-matrix`, `/create-workspace`, `/workspace-groups` and `/workspace-members` left
  the ratchet. The three that remain are **three different problems, not one shape**, and are
  described individually in `getSweepRatchet`: `/documents/new` and its HTMX fragment reach
  `ListGroups` by two paths (the handler's `ListMembers`, and `composition.Loader`'s one
  un-memoized read); `/switch-workspace` is **the only true N+1 found all day** —
  `loadWorkspaceChoices` calls `GetWorkspace` once per membership, so it is 1+N in how many
  Workspaces the viewer belongs to, and reads 2 in the fixture only because that identity belongs
  to one. Its fix is a join in `ListMemberships`, which deserves its own change and test.

  **`requireWorkspaceAdmin` had no test of its own**, which is uncomfortable for the one gate in
  this repo that is recorded as having *failed open* (2026-09-21 review). It has one now
  (`TestRequireWorkspaceAdmin`, five branches), added because editing an authorization gate for a
  query-count reason is not a reason to verify it by reading. Confirmed it fails when the
  no-membership arm is made to pass.

  **The ratchet reached zero the same day**, and the last three were three more causes rather than
  one: `loadWorkspaceChoices` fetched **one Workspace per membership** — the only true N+1 the whole
  audit found, now a join in `ListMemberships` carrying `WorkspaceName`; `composition.Loader`'s
  `ListGroups` was **the one read of its four without a memo**, so the submit wizard reached the
  Workspace's Groups by two paths (`Loader.Groups`/`Loader.Members` now, over
  `data.ListMembersFrom`/`GroupsByMemberFrom`); and `currentUserEmail` was the last direct
  `store.GetMembership` bypassing the resolved identity — latent rather than live, which is why the
  sweep never saw it.

  **`getSweepRatchet` is empty and stays declared.** An empty ratchet is an ordinary gate: the next
  route that repeats a read or issues an unnamed query fails outright with no list to be added to.

  Two assertions were added that the sweep structurally cannot make. **`repeated=0` does not say
  an N+1 is gone** — a per-row loop repeats nothing when there is one row, which is exactly why
  `/switch-workspace` measured as a mild `repeated=1` for weeks; `TestSwitchWorkspaceCostIsFlatInWorkspaceCount`
  asserts the same request costs the same for an identity in one Workspace and in three. And since
  the Workspace name moved from a per-row fetch onto a join, **nothing had ever asserted that name
  renders at all**, so that is asserted too. Both were confirmed to fail when their fix is undone.

  **Response compression, static assets only** — `hyperscript.min.js` 369→67kB, `htmx.min.js`
  51→16kB, `app.css` 30→6.5kB: **~359kB off a cold load**, the performance study's one Tier-1 item
  needing no trigger. **HTML is deliberately excluded** (owner decision): every page carries the
  CSRF token in its own markup (`hx-headers`, security audit L1), and a secret in a compressed
  response beside attacker-influenceable content is the BREACH precondition. HTML is worth 2.6kB
  per page against that, so the static assets are ~99% of the win without having to answer the
  question. Verified on the live server that HTML returns **no** `Content-Encoding`, rather than
  inferring it from where the middleware sits. `Cache-Control` was deliberately deferred: the
  filenames carry no content hash, so a long max-age would serve stale assets after a deploy.

- **The Flow 2 mockup — gap recorded 2026-09-23, nothing scheduled yet.** A second owner canvas
  ("Menata Runtime — Case 03 Flow", 39 artboards: 19 desktop at 1280px, 20 mobile at 390px)
  revises the Flow 1 set this section's seven phases ported, rather than replacing it. Full study,
  board-by-board, with what each gap would cost in *primitives* rather than in screens:
  `menata-app-document`'s `audits/2026-09-23-kajian-gap-mockup-flow2.md`. Feature level, and only
  what a reader of this file needs:

  **It subtracts before it adds, and the subtraction is the largest item.** Board 06 is no longer
  a role × *stage* matrix; it is a plain-language **Permissions** page whose every statement is a
  projection of what this runtime already declares (`permissions:`, `roles:`, `transitions:`,
  `aggregate_status`), including an honest "No restriction set yet" for the two rules nobody has
  declared. Its own **"HAPPENS ON ITS OWN"** section states the model this app actually runs:
  a Document's status follows its Steps, and *"Nobody sets it by hand — not even an admin."*
  If the owner confirms that board 06 new supersedes board 06 old, **open question 1 above is
  answered without building the stage model**, and six deferral rows close as *not wanted* rather
  than *not done*. The one piece of CAP-V28 the new set still asks for is the saved default
  approval flow per Document Type, which stands on its own without stages. The wizard likewise
  drops to `Step 1 of 2` / `Step 2 of 2`, closing "the wizard's third step" by deletion.

  **Already built, and worth saying so:** Ubuntu + the slate/blue palette, the 9-dot card
  launcher, breadcrumb, account menu, and the mobile bottom bar. Chrome is four small items from
  done (a badge on the bottom bar's Inbox tab, its "More" sheet, the workspace-name dropdown, a
  launcher entry) — this is finishing, not porting. The **bottom-bar deferral row above is stale**
  and says so at the next phase close: it was built in Fase 2, while the row still reads
  "no phase yet".

  **New screens:** Assigned to me (a third worklist beside Inbox and My Documents — and the
  *third* real case for a filter that reads the viewing identity, after My Tasks and the Inbox);
  My Documents promoted to a full screen with a **Drafts** section and a "Revise" action, both of
  which need the `draft` status and the `Rejected → Draft` edge two deferral rows already carry;
  two **Settings hubs** with sidebars of their own, one per Workspace and one per Application —
  which moves Groups and Permissions *into* the Application and leaves Workspace settings with an
  access toggle and a "Manage in app →" link; and search boxes on three screens at once.

  **New concepts, none of which exist in any form today:** an Application with a **draft →
  published** lifecycle and a publish-time access scope; **archiving and restoring a Workspace**
  (a read-only gate above every write, which is not a per-Machine Permission); **deactivating a
  member**; a workspace **audit log**; and **notifications** (in-app and email, per user and per
  Application) — `internal/mail` serves invitations and verification only.

  **And one that is not a phase but a product:** boards 15/16 generate a whole Application from a
  natural-language description, review it, then publish it. It lands on the three deferrals this
  file already carries as "not urgent while the manifest ships with the binary" — runtime-writable
  metadata, hot reload with change classification, and rejecting unknown keys at load — because
  it *is* the end state those rows name: metadata edited by someone who cannot restart the
  process. Its danger is the ordinary one stated inside out: a generator that emits Go, or emits
  YAML that then needs per-Application code to work, would break this runtime's premise while
  looking like it succeeded.

  **Four things wait on the owner, not on engineering** (the study's §7): whether board 06 new
  supersedes board 06 old; how soon the generated-Application work is real, since it reorders
  everything after it; whether merging pending invitations into the Members table is a display
  choice or a model change (the identity model says an invitation is not a membership); and
  whether Groups/Permissions really move into the Application, which changes routes, gates, and
  who may open them.

  **Flow 2 step 1 — the bars and the icon set — *shipped 2026-09-24*** (owner request: chrome
  first, "top bar dan bottom bar dibuat selalu terlihat di screen", plus "pilihkan juga icon yang
  style nya cocok dan bisa dipakai"). The gap study's §8 Tahap 1, less the workspace-name dropdown
  and the launcher's "New application" entry, which belong to features that do not exist yet.

  - **Both bars stay on screen.** The bottom bar was already `fixed`; the top bar was not, and on
    the mockup's own tallest mobile boards that was not cosmetic — M06 is 1500px and M08 1560px,
    so scrolling either took the launcher, the breadcrumb and the account menu away together and
    left a long form with no way out but the browser's back button. `appShell`'s header is
    `sticky top-0 z-40` now, above the bar's `z-30` so `accountMenu`'s mobile sheet still paints
    over it.
  - **The bottom bar's fourth column is "More"**, a `<details>` bottom sheet (`moreSheet`) holding
    Workspace Home, one row per Application in the Workspace with the current one marked, and
    "All Workspaces". This is the phone's only way *out* of an Application: the 9-dot launcher is
    a pointer-sized dropdown, and until the top bar became sticky it was not even reliably on
    screen. `<details>` and no JavaScript, the third component to make that same call after
    `appLauncher` and `accountMenu` — consistency rather than a new pattern, and the same accepted
    cost (a tap outside does not dismiss it). **The mockup's third sheet row, "<Application>
    settings", is deliberately absent**: no per-Application settings hub exists yet, and a row
    that links nowhere — or points at a Workspace-level admin screen while calling it the
    Application's — would be worse than its absence.
  - **A real trade, named rather than buried:** the bar now shows three declared items instead of
    four. The right way round — items past the third stay reachable from the desktop row and the
    launcher, while leaving the Application had no mobile affordance at all.
  - **The nav badge reaches the bottom bar** (`NavigationItem.Badge`, already declared), and
    deliberately **not** the desktop row: each badge is its own `hx-get` to
    `/api/approval-inbox/pending-count`, so drawing both would double that endpoint's per-page
    cost while only one row is ever visible. `/approval-inbox` itself still measures
    `queries=11 reads=11 repeated=0` — the bars add no server-side read. The asymmetry dissolves
    when that count stops needing its own round trip (the `NavBadgeApprovalInboxPending` deferral
    row below).
  - **Same-day correction, from the owner testing it** (2026-09-24). Three things the first cut
    got wrong, and one it never had:

    **The menus could not be closed.** `<details>` panels close only by re-clicking their own
    summary; the shipped comment called that an "accepted cost" of having no JavaScript, and the
    owner's report shows what the cost actually was — *"behaviour bottom sheet juga masih belum
    bisa tertutup, kecuali klik di avatar pojok kanan atas"*: with no light dismiss anywhere, the
    avatar had become the way to dismiss a sheet it does not own. All three menus (launcher,
    account, More) are the native **Popover API** now — `popover` + `popovertarget`, which gives
    click-outside and Escape from the browser and paints in the top layer, **still with no
    script**. The no-JS posture was never the problem; reaching for `<details>` to keep it was.

    **The sheet had no shade.** M07b dims everything above the bar and leaves the bar itself
    legible. A real `::backdrop` cannot do that — it covers the whole viewport — so the shade is a
    full-width `<button popovertargetaction="hide">` inside the popover, which also solves the
    second half: light dismiss deliberately does not fire for clicks *inside* a popover, so the
    shade has to dismiss the sheet itself. Being a button, it is keyboard-reachable rather than a
    dead div.

    **An empty badge rendered as a bare red dot.** `showPendingCount` writes nothing when nothing
    is pending, so the bubble needs `empty:hidden` — the Tailwind half of the `.nav-badge:empty`
    rule `pageStyles` has carried since Phase 21. It was missing for the whole first day, and it
    is exactly the class of defect a screenshot catches and a test does not.

    **The top bar was never ported, only kept.** Board 07's second row opens on the Application's
    own icon and name; ours opened straight into links. With the Workspace name one row up and the
    page's `<h1>` below, that row was the only place that could say *which Application you are in*,
    and it said it nowhere. On a phone, where there is no second row, the Application's name
    becomes the breadcrumb's second line instead (M07's stacked pair). The badge reaches the
    desktop row with it, at the cost of a second `hx-get` per page — named in `capabilities.md`
    rather than hidden, since only one row is ever visible. The mockup's right-hand **Settings**
    link stays absent for the same reason `moreSheet`'s third row does: no per-Application
    settings hub exists, and pointing it at a Workspace-level admin screen would be a worse lie
    than the gap.

    **Tailwind ships no components** — it is a utility framework with no JavaScript and no widget
    layer, so there is no stock bottom sheet, bottom bar or top bar to adopt. The component
    libraries built on it (daisyUI, Flowbite, Preline) all want npm and their own class
    vocabulary, which this repo's Node-free standalone-CLI build deliberately does not have. The
    platform had the missing piece instead, and it is better than any of them for this: `popover`
    is one attribute, needs no bundle, and cannot go stale.

  - **One control vocabulary** (2026-09-24, item 1 of the post-port cleanup). `controls.templ`
    holds one literal class string per control kind -- `controlPrimary`, `controlSecondary`,
    `controlDanger`, `controlField`, `tableCell`, `tableHeadCell` -- and every `appShell` screen
    reads them.

    **Added against a measurement, not a preference, and the measurement indicted the change that
    produced it.** While two stylesheets existed a control written twice was written in two
    different systems, so the duplication was invisible. With one left it became countable: four
    different primary buttons (`h-9 px-4`, `h-8 px-3`, `h-11 sm:h-9`, plus two differing only by a
    leading `flex items-center`) and six different text inputs, some with a focus ring and some
    without. **Two of those variants were added by the port that made them countable** -- the
    generic record screens got their own local `btnPrimary`/`inputBase` a day earlier -- which is
    why this is item 1 rather than a later tidy-up: the next screen would have added a seventh.

    `controlField` carries no width on purpose. Every caller has one (`w-full` in a table cell,
    `min-w-44 grow` in the wizard's approver row, `w-36` for a filter), and baking one in would
    make each fight it with a second width utility whose winner depends on the order Tailwind
    emits them in.

    **The pre-auth screens are deliberately excluded**: `authShell`'s kit renders larger
    (`h-10.5 sm:h-9.5`, 15px labels) for a screen reached on a phone before an account exists.
    That is a decision already captured in named components, and unifying the sizes would undo it.
    Verified in a browser: every form control on the ported screens now measures 36px, and the
    only other heights left belong to chrome (the 28px launcher, the 32px avatar).

    One defect fixed on the way, introduced by the Workspace menu two commits earlier: the
    breadcrumb separator rendered as `/Title` with no space, because both parts are flex items and
    whitespace at a flex item's own edge collapses. It is a `gap` now, not a padded string.

  - **The last two screens left the other stylesheet** (2026-09-24) — `/`, `/machines/{id}` and
    the generic record detail page moved from `pageShell` to `appShell`, and `pageStyles` was
    deleted with them. This closes two deferral rows below ("Dashboard (`/dashboard`) still on
    `pageStyles`" was closed on 2026-09-22; "Document / Approval Step detail pages still on
    `pageStyles`" is closed here) and retires `capabilities.md`'s whole "Styling: two systems, on
    purpose" section, which had described the split as a planned transition since Fase 1.

    **What it was costing did not read as a missing feature, which is why it survived four
    phases.** Those screens linked no `app.css` at all: no sticky header, no bottom bar, no
    launcher, and the platform's default font instead of Ubuntu. On a phone, opening one record
    dropped the viewer out of the chrome entirely, with the browser's back button as the only way
    back — measured, not guessed: the page's `link[rel=stylesheet]` list was empty. It read as a
    different application rather than a plainer page.

    The reason the boundary was per *screen* rather than per layer still holds and is worth
    keeping written down for the next migration of this shape: Preflight resets heading sizes,
    list markers and button defaults that a hand-written sheet leaves to the browser, so a page
    linking `app.css` must have its content ported in the same change or it visibly breaks.

    Deleted with the shell, each with its last caller: `pageShell`, `pageHead`, `pageStyles`,
    `navLink`, `navSections` (and its three tests), `CurrentApplicationName`,
    `applicationNavEntry` — the grouped, collapsible topbar has no counterpart in `appShell`,
    whose launcher lists Applications rather than every declared item.

    **Two components stopped being twins.** `slaBadge`/`slaBadgePill` and
    `sectionHeader`/`sectionHeaderRow` each drew the identical thing in the two stylesheets — a
    deliberate, documented cost of the transition. With one stylesheet left they were simply the
    same component written twice, so the `pageStyles` halves went. A test went with them in
    spirit: `TestMachineBody_cardsViewRendersProjectedFields` asserted on the class name
    `summary-card-list`, which made it a test of styling rather than of rendering; it asserts on
    the link every card carries now.

    **Two things this surfaced rather than caused.** `show_nav` now suppresses **nothing at all**
    — its only reader was `pageShell`'s topbar, so a field two Applications declare changes no
    pixel anywhere; either it should mean something to `applicationMenuRow`/`applicationBottomBar`
    or it should go, and that is the owner's call (`capabilities.md` states it plainly rather than
    leaving the field looking live). And the Document detail page's PDF thumbnail returns 422 for
    the one old test Document — same route, same `<img>`, unchanged by this port; it is the
    orphaned `Vendor Contract 2026` record the open-questions list above still names.

  - **The Workspace menu** (boards 03b / M03b, owner: *"workspace dulu"*) — the last item of the
    Flow 2 gap study's Tahap 1. The Workspace name gains a chevron and opens `workspaceMenu`:
    its initials tile, the viewer's role, Workspace settings, Switch workspace. Straight onto
    `sheetPanel`, so it was a sheet on mobile and a dropdown on desktop with no new mechanism.

    It renders at Workspace level only, never inside an Application. That is the mockup's own
    split rather than a shortcut: board 07's in-Application breadcrumb keeps the Workspace name a
    one-tap link home, and inside an Application the launcher and the More sheet already offer
    these same destinations. Two rows, not three — **"Invite members" is not rendered**, because
    this app has no invite route of its own (the form is a `<details>` inside the Members screen)
    and a second row pointing where the row above it already goes is worse than one honest row.
    The mockup's "Admin · 24 members" loses its count for the reason already recorded twice: a
    member count is not a field on `domain.Workspace`.

    **What it actually cost was a duplication it exposed, not the menu.** The row is admin-gated,
    so the viewer's Workspace role had to reach `appShell` — and `WorkspaceHomePage` was already
    taking that same role as a parameter of its own beside `Viewer`. Two sources for one fact, and
    they disagreed on the first build: `TestWorkspaceHomePage_membersLinkVisibility` failed
    because the page hid its Members link from a member while the new menu, reading the other
    source, still offered it. The role lives on `Viewer` now — "who is looking" is what that type
    already is — and the page's parameter is gone. The test caught it because it asserts on the
    whole rendered page rather than on one element, which is the property its own comment claims
    and this is the second time that has paid.

    The gate is `!= member`, not `== admin`, matching `workspacehome.templ`'s deliberate three-way
    split: the shared admin credential holds no membership row, so its role reads `""` while it
    *can* still open the destination — hiding the row from it would hide a link that works.
    `domain.WorkspaceRoleMember` exists now so that split is a named constant in both readers
    rather than a bare literal, which is how the second one nearly got `== admin` instead.

    Workspace Home's own breadcrumb also stops reading "Workspace / Home": the page names itself
    in its `<h1>`, and board 03's breadcrumb is the Workspace name alone. Every other
    Workspace-level screen still says which one it is.

  - **The same fix, missed on two of the three menus** (owner, 2026-09-24, testing the build):
    *"jika klik avatar bulat di pojok kanan atas, perilaku bottom sheet masih menimpa bottom bar
    dan tidak ada transparan gelap"*. The More sheet got a shade and was positioned above the bar;
    the account menu kept the bare panel it had always had. Three symptoms, one cause — with no
    shade there is no visible "outside" to tap, and the panel sat *over* the bottom bar, so
    tapping the bar to escape was a tap **inside** the popover, where light dismiss deliberately
    does not fire. The avatar was the only control left.

    `sheetPanel` now holds that shape once, for all three menus, and the launcher became a real
    bottom sheet on mobile with it (mockup M12, which it never matched). Extracted at the *third*
    use rather than the second, which is what `ui-composition-decomposition-criteria.md` asks for
    — and the day between the second use and the third is exactly the bug above. Verified in a
    browser at 390px: the popover spans 0–768 inside an Application (the bar's top edge) and
    0–844 on a Workspace screen with no bar, and closes on an outside tap, a tap on the bar, and
    Escape.

    One thing it made visible rather than caused: the shared admin credential resolves to an
    identity with no name and no email, so the sheet's identity block rendered as an empty strip
    above a divider. Invisible as a small dropdown, obvious as a full-width sheet.

  - **A navigation item declares three strings now, not one** (owner, 2026-09-24, on being asked
    whether to rename the Inbox tab: *"pengaturan ini seharusnya ada di metadata bukan?"* — and it
    was the right answer to a question that should not have been asked). `label:` is what a menu
    says, `title:` what the screen says about itself, `description:` the line under it. Board 07
    needs all three and they are all different: the tab reads **Inbox**, the page is headed
    **Pending my approval**.

    What was actually wrong was bigger than the rename. Every `<h1>` in the app came from
    `labelByID`, so **every heading was constrained by how it had to read in a four-column bottom
    bar** — and every subtitle was a literal in its own `.templ` (`approvalinbox.templ` carried
    one per tab). `title:` falls back to `label:`, so a screen whose heading really is its menu
    label still says that by staying silent, and nothing else in the app had to change.

    **`TestRenderingHasNoHardcodedPageHeading` is the third gate in the family**, and it exists
    because the label gate could not structurally see this class: `templTagText` matches a run of
    Title-Case words, which every declared `label:` is and no heading or sentence ever is. So the
    subtitles sat in plain text inside a file that gate was passing, while its own doc comment
    claimed to cover "a card's plain-text body". The new one matches the declared string verbatim
    with comments stripped, and was confirmed to fail on a reintroduced violation rather than
    assumed to.

    The "APPROVAL" eyebrow above that heading is gone with it: board 07 has none, and it was a
    hardcoded word naming the Application on a screen whose chrome now names it two rows up.

  - **`icon:` is a name from a closed set now, not a literal glyph** (`domain.KnownIcons`,
    `internal/rendering/icons.templ`, validated at load like `color:`; `icon: "▣"` became
    `icon: inbox`). This was already owed: `ui-sample/nav-metadata.js`'s own comment had called
    those glyphs placeholders for "a real SVG icon set ... not-yet-scoped" since before Fase 3c,
    and every declaration carrying one repeated the caveat. Seventeen icons, one drawing style —
    24x24, `stroke="currentColor"` at 1.8, round caps — written once as shared attributes on a
    single `<svg>` so they cannot drift apart, which is the only thing making seventeen separate
    drawings read as one set. A glyph could never have that: its weight belongs to whichever font
    happened to carry the codepoint. Metadata names *which* icon and never how it is drawn, so
    restyling the set is one file. `TestKnownIconsAreAllDrawn` closes the seam a name-list cannot
    close by itself — a declared name nothing draws would ship as an empty box with no error
    anywhere, the same invisible failure a colour token with no `appIconClasses` branch has.

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
- Search, filtering and pagination on record lists. **Pagination is also the cheapest answer to the
  data-path cost measured in the study above** (007 §7.9): it breaks the "a page costs what the
  Machine holds" relationship outright, without needing Filter/Projection/Aggregate pushdown first.
  Its own trigger arrives sooner than theirs — one list screen past ~200 rows.
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
