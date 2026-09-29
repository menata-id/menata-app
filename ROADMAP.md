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
  `workflow-behavior-decomposition-criteria.md`. The Document-approval decide cascade remains
  hardcoded, on purpose. SLA-breach detection, which used to fire on a page read for lack of a
  scheduler, is the Event primitive's own third shape now (`schedule:`, 2026-09-27) -- see the
  "SLA-breach reminder via a scheduler primitive" entry below.
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

  **Composability is layered here, and measuring it as a yes/no gets the answer wrong** (owner,
  2026-09-28, correcting exactly that mistake made in this session: *"bukannya ini juga sudah
  composable juga? hanya level composable nya memang bisa beberapa lapis bukan?"*). Asked whether
  the real Document Approval could be reused in another Workspace, the first answer given was
  "no, it is not metadata-based" -- demonstrated by renaming the Application id and watching the
  engine stop engaging. That demonstration proved something real but much narrower than claimed,
  and the owner's framing is the accurate one. Measured layer by layer against the actual files:

  | Layer | Status | Where |
  |---|---|---|
  | Data shape -- Machines, Fields, relations | **metadata** | `fields:` across `document`/`approval_step`/`signature.yaml` |
  | Access -- who may do what | **metadata** | `permissions:` + `actor_field:`, an Application's own `roles:` |
  | State model -- which moves exist, and their gating | **metadata** | `transitions:`, `sequencing:` |
  | Reactions -- rollup, notification, activity log | **metadata** | `events:` → `service:`, from `domain.KnownServices` |
  | Aggregation and list presentation | **metadata** | `datasets:`, `views:`, `card_fields:`, `sla_field:` |
  | Bespoke screens | Go | `approvalinbox`/`documentsubmit`/`reviewdocument`/`signatureplacement.templ` |
  | Action *effect* -- which Fields `decide` writes | Go | `internal/web.decideStep` |
  | Domain service -- signature capture, PDF compositing | Go | `internal/web.signDocument`, absent from `KnownServices` |
  | Binding -- how the Go engine finds "its" Machines | Go | `action.IsDocument`/`IsStep`, matching a literal Application id |

  Five layers are fully declarative and four are not, so the honest statement is **"composable,
  and the remaining work is deepening and widening the metadata"** -- not "not metadata-based".
  The app genuinely installs from its metadata and runs: a copy into a fresh Workspace loads and
  the engine engages (verified against the real loader, 2026-09-28).

  **What that narrower demonstration did prove** is worth keeping, correctly sized: the *binding*
  layer is itself hardcoded. The engine finds its Machines by matching the literal
  `app_document_approval`, so the app can be installed and run as-is but cannot be renamed, varied,
  or used as the engine behind a second, differently-named approval Application. That is one of
  the four remaining layers, not evidence against the other five.

  **Each remaining layer already has the mechanism waiting for it**, which is why this is
  deepening rather than a rewrite: `KnownServices` is a closed registry with three members and
  needs a fourth for signatures/PDF; `KnownActions` already carries `decide` as a *name* that
  Permissions gate and Transitions reference, and what is missing is a way to declare an Action's
  *effect* (`decideStep` writes exactly two Fields -- `fld_decision` from the submitted value,
  `fld_decided_by_name` from the actor -- then dispatches Events that are already declarative);
  007 §12 already defines Page/Layout/Component/Slot for the screens; and the binding could be
  declared by the Application rather than matched by literal. Measured against 001 #2 ("Application
  behavior belongs to the runtime") and 006's own Action definition (which names Approve and Reject
  explicitly), those four are the distance left to close.
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
  | **Board 06 presupposes an approval model this app does not have** — the matrix ships and is truthful, but it can never look like the board | **no phase yet** — **and this is a model decision, not a screen one** | Board 06's own rows are `Submit: Draft → In Review`, `Approve — Finance: In Review → Finance OK`, `Approve — Legal: Finance OK → Legal OK`, `Approve — Director: Legal OK → Approved`. That is a **stage model**: the Document itself holds a multi-state status and each stage's transition is gated on a *role*, declared once per Application. This app does the opposite, on purpose: a Document's status is **derived** from its Approval Steps (`evt_step_decision_rollup`, three values), and who approves each step is **per-record data** the submitter picks in the wizard (`fld_assignee`/`fld_approver_group`), not metadata. Under that model there are exactly two role-gated transitions in the whole Application — Approve and Reject on a step — which is what the shipped matrix correctly shows, six `System` rows and two real ones. **Neither model is wrong and this row does not pick one**; what it records is that the screen cannot be made to resemble the board without first changing the approval model, and that no amount of work on the screen is the missing piece. **Three ways forward, written down 2026-09-21 after the owner asked what this row actually means:** *(a) keep the per-record chain* -- flexible, no standard flow, and board 06 never resembles its drawing; *(b) adopt the stage model* -- a fixed per-Application flow gated on roles, matching the board, at the cost of reworking the wizard, the rollup and boards 07-10; *(c) **CAP-V28**, the saved default approval flow per Document Type* -- the chain becomes a declared template without status becoming a stage machine. **(c) is the recommendation**: it delivers what the board is actually reaching for (a flow that is dependable and readable rather than rebuilt per document) without dismantling a model that works. **Owner confirmed (c), 2026-09-27 -- shipped the same day** (below). Upstream's own `process:` overlay is the stage model, and its board-06 copy citing `ProcessEdge` and `/{machineID}/process-map` is drawn against it -- the same "mockup draws data the app doesn't have" class as board 03's Procurement/HR cards and board 07's hour-scale SLA. A middle path exists and is already a row below: **CAP-V28**, a saved default approval flow per Document Type, which makes the chain a declared template without making status a stage machine |
  | `draft` as a Document status (board 06: `Submit: Draft → In Review`, `Reopen: Rejected → Draft`) | **no phase yet** — lands with the wizard's third step, below | Two of board 06's six rows need a state `metadata/document.yaml` deliberately removed, with its own note: *"add it back only alongside a real save-as-draft flow, not speculatively."* That flow is the wizard's third step, which has no board anywhere. So this is the same deferral seen from the matrix's end, not a second one — worth listing because from board 06 it reads as a missing transition rather than a missing flow |
  | A transition with more than one `from` (board 06: `Reject: any pending state → Rejected`) | **no phase yet** — **a second real case, not a phase** | `domain.Transition` names exactly one `from`, so "any pending state" is declarable only by enumerating one edge per source state. With `fld_decision`'s single open value that enumeration is one row and costs nothing, which is precisely why the shape should not be generalized yet; it starts costing the moment a status Field has several non-final states, which is the stage model above. Recorded so the board's wording is not read as an unimplemented feature of the matrix |
  | Board 06's columns mix two role namespaces | **closed 2026-09-25, by the Flow 2 port rather than by answering the question** | Its five columns are `ADMIN`, `FINANCE REVIEWER`, `LEGAL REVIEWER`, `DIRECTOR`, `MEMBER`. `ui-sample/member-role-detail.html` settles what those words are, in the owner's own design language: a "Workspace role" section holding exactly `Admin`/`Member` — captioned *"controls workspace administration, independent of application permissions"* — above a separate "Application access" section whose Document Approval vocabulary is Approver/Submitter/Reviewer. So **two** of board 06's five columns are Workspace-level, not one, and the other three appear in no declared vocabulary at all. The 2026-09-21 answer put Workspace rules in their own section with no role columns (`06b-authorization-matrix.html`, this screen's shipped shape at the time); the remaining open half -- "whether a Workspace admin should appear as an always-✓ column" -- is moot now rather than merely unanswered, because this file's own "Application Settings hub" entry (Phase 2) rewrote the Permissions content into the Flow 2 mockup's actual shape, which has **no role-per-column grid at all** any more, on either screen: `Who can do it` is a plain phrase per row, not a tick per role. There is no column left for an admin, or anyone, to always occupy |
  | Process map — the same edges drawn transition-first (`/{machineID}/process-map`, upstream CAP-W05 ✅) | **no phase yet** | Board 06's own copy says the matrix "re-projects `ProcessEdge` (already rendered per-transition on `/{machineID}/process-map`) role-first instead of transition-first" — describing that route as already existing. It does not exist here; Fase 7 built the role-first projection only. Upstream's is *derived*, needing no new metadata concept, and `domain.Machine.Transitions` is now exactly the input it reads, so this is a small screen rather than a capability — it simply has no case asking for it yet |
  | Board m06's mobile layout (role **chips** per transition, not a grid) | **closed 2026-09-25 -- superseded, not fixed** | This row described Flow 1's `m06-approval-role-matrix.html`, a role×transition matrix whose mobile board dropped the grid for chips under each transition. That whole screen concept is gone: Flow 2's own `M06-RoleMatrix.dc.html` is not a matrix at all, on either breakpoint -- a plain two-column list, no grid and no chips -- and that is what got ported (`ROADMAP.md`'s "Application Settings hub" entry, Phase 2). Asking "did the chips ship" no longer has an answer, because nothing in the current design draws a matrix for chips to be a mobile variant of |
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
  | Saved default approval flow (board 08: "Save this as the default approval flow for **Contract** documents", checked by default) | **shipped 2026-09-27** | See below -- CAP-V28, two companion Machines (one template per Document Type, plus its own ordered steps) with a find-or-create write path, exactly as this row's own prior text described the blocker rather than a fix |
  | One signature box for a Group-held step (boards 08/09) | **no phase yet** — **named, not solved** | Board 09 places exactly one signature box for `Legal Group · 4 members`, and nothing on either board says whose signature image lands in it, or what happens when two of the four act. Upstream has the identical gap on its own compositing capability, recorded there in the same words. Worth holding here rather than discovering it during 6c-2's port |
  | Member search box (board 04) | **closed 2026-09-25** | See this file's "In progress" section, "Search on Workspace Members, My Documents and Assigned to me" |
  | Write-side Binding: a form input's `name=` hardcoded to a Field id (007 §11.3) | **no phase yet** — **and it is the gap a shrinking ratchet hides** | `signatureplacement.templ` carries twelve `name={ action.Field… }`, `documentsubmit.templ` four (this read "six" until it was counted, 2026-09-21), while `machine.templ` already does it generically (`name={ f.ID }` over `m.Fields`). `TestRenderingUsesProjectionNotRawValues` gates *reads*, never writes, so both files could leave `projectionRatchet` with every binding still hand-typed — which is exactly what happened in 6c-2 and 6c-3. Recorded here so the ratchet count is not read as a composability score. Trigger: a third bespoke write screen, or the generic update route stopping its whole-record rewrite (the same route that forced 6c-3's carry-forward list). **Half of that trigger arrived 2026-09-21, and the count did not move at all** -- which is the useful part. Board 09's forms left the generic route for one that writes four named Fields, so the carry-forward echo is gone; the twelve hand-typed bindings are exactly as hand-typed as before. The first guess while writing this row said the count would drop to four, and checking the file said twelve: the echo rendered `name={ f.Name }` from a derived list, never `action.Field*`, so it was never part of this number. The whole-record rewrite was the reason the *echo* existed; it was never the reason the *bindings* are literal, and removing it changes nothing here. The other half of the trigger -- a third bespoke write screen -- is still the one to watch |
  | `NavBadgeApprovalInboxPending` still resolved in the Domain Plane | **no phase yet** | `domain.NavigationItem.Badge` names one live count the runtime special-cases end to end (`domain/navigation.go`, `web/workspacehome.go`, `appshell.templ`, `machine.templ`). It cannot become an ordinary Dataset: the count filters on assignee **and** on `behavior.CanAct`, which reads a *sibling* record, while a Measure's `where:` is evaluated per record against its own values. So the blocker is record selection (007 §8), not the identity sentinel below — a point the companion repo's Stage 0 first got wrong in the other direction |
  | PDF signature compositing is hand-written Go (upstream CAP-F22) | **no phase yet** | The third of the three Case 3 behaviours named above; the other two (`activate_next`, `aggregate_status`) landed 2026-09-20 and this one did not, so it should stop riding on their line. `internal/action/composite.go`/`banner.go` manipulate a binary PDF rather than express a rule, which is the lowest generalization value of the three — but "lowest value" is a ranking, not a deferral reason, and it had neither a phase nor a row until now |
  | ~~The approval stepper is five constants, not a declared View~~ (upstream CAP-V20 ✅) | **done 2026-09-26**: `domain.ViewStepper` — "a View composing other Views" (this file's own Planned entry) implemented as the narrow real case it always was | `mch_approval_step` declares `vw_step_progress` (`type: stepper`), validated to require `sequencing:` (`internal/metadata.validateView`, mirroring the existing `cards`/`card_fields` requirement). `internal/rendering/detail.templ` composes it via `domain.Machine.StepperView()`, replacing the `m.ID == action.DocumentMachineID && cc.Machine.ID == action.StepMachineID` check with a declared-View lookup any future Sequencing-declaring Machine pair gets for free. `approvalstepper.templ` reads `Sequencing.OrderField`/`StateField` (dynamic field names) and the Machine's first `person` Field generically instead of `action.Field*` constants — closing `approvalstepper.templ`, the **last Case 3 entry**, out of `internal/conformance`'s `projectionRatchet` (five Case 19 screens remain). Matches upstream's own admitted CAP-V20 shape exactly (done/current/pending, three states) rather than keeping the hardcoded version's extra red/green approved-vs-rejected split — the outcome still reads from the decided step's own text label. Deliberately did **not** reuse `card_fields`/Projection for the assignee: `mch_approval_step`'s own dormant `card_fields` wiring already serves the Approval Inbox's pending-card list, where every value would be constant ("pending", "me") — declaring `card_fields` to feed the stepper would have silently activated that unrelated screen's own chip rendering too, checked directly before ruling it out |
  | "Keep me signed in" (board 01) | **no phase yet** | a session-lifetime change; `internal/authorization` has no remember-me concept |
- Rounding out Project Management: a project-level workspace overview, richer task detail
  (checklist, comments, attachments), and scoping views to one project at a time. **Its standing
  relative to the Document Approval scope decision is unsettled, and that matters more than it
  looks:** the 2026-09-20 decision narrowed the proof-of-concept to Document Approval, while this
  line still reads as active work — and the files left in `projectionRatchet` were, until
  2026-09-28, all Project Management screens whose stated exit was "migrates with
  Dataset/Projection". So the ratchet's own path to empty ran through work whose scope is
  undecided. **That tension resolved itself the cheap way**: the migration turned out to need no
  Project Management *feature* work at all — four `card_fields:` declarations and a derivation —
  so the ratchet drained to one entry without the scope question being answered. The scope
  question itself is still open for the rest of the line (project overview, task detail, per-
  project view scoping); what is no longer true is that the ratchet is waiting on it.
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
- **Fixtures run through the production validator, and the inference that was locked inside Parse**
  (2026-09-29). Re-reading 001-007 on the owner's instruction is what turned this from a test chore into
  a design fix, so the order matters.

  **The design changed because 005 says it should.** `005-runtime-lifecycle.md` separates Phase 3 (Parse
  and Validation) from Phase 4 (Normalization and Resolution, which may "expand authoring conveniences"),
  and binding a `person` Field to `mch_user` is exactly such a convenience -- 001 Principle #6 ("Infer
  Before Configure") is *why* an author never writes `machine: mch_user`. That inference lived inside
  `Parse`, so it reached YAML and nothing else. A Machine built in Go with a perfectly correct `person`
  Field therefore failed validation, with a message blaming its type.
  My first fix was to make the fixtures write `RelatedMachine` by hand. **That was backwards** -- it asks
  every test to configure what the runtime infers, inverting 001 #6. `metadata.Normalize` is the fix:
  the inference, reachable by anything holding a `*domain.Machine`, called by `Parse` as its last step.

  **The validator's message was wrong in two of its three cases.** It read `must reference an identity (a
  person or relation field), got %q` and answered `got "person"` for a person field -- contradicting
  itself on one line. What is missing differs by type, so it is three branches now: a `relation` field is
  missing a `machine:` **target**; a `text` field can never have one and naming the type is the whole
  answer (the case the old wording got right); and a `person` field here means the Machine skipped
  normalisation, which is unreachable through the loader and now says so. Each has its own test.

  **Then the gate.** `TestMachineFixturesPassProductionValidation`, one per package, running every fixture
  through `Normalize` + `Validate`. **It is a sweep where the mirror gate is a named list**, and the
  difference is judgement: mirroring asks whether a fixture declares what the real Machine declares, which
  a narrow unit fixture legitimately does not -- a static gate for *that* measured 40 false findings and
  was rejected two days ago. Validity asks whether the fixture is coherent, and `Validate` tolerates
  minimal while rejecting impossible.

  **Measured, and the measurement changed the scope mid-slice.** The plan called the yield beyond
  `internal/web` unmeasured; probing found **five packages, ~25 findings**, split into ~11 missing display
  names and ~14 genuine incoherences: `signature_placement:` over Fields the Machine did not declare,
  `actions:` writing an absent Field, Events and Constraints gating on absent Fields, a Dataset summing
  one, `actor_type_field` typed `text` where the real Machine declares `status`. Owner chose to fix all
  five rather than land `internal/web` alone.

  **Where it stops, said so it is not read as closing the class.** A fixture that is merely *looser* than
  the real Machine is valid: `documentTestMachine` with no Transitions passes this gate, and that is the
  one that cost a real miss (`StatusField()` returning `""`). Mirroring is still the named list's job.

- **The three AI-session routes, and the cross-Workspace write one of them allowed** (2026-09-29).
  `POST /new-application/message`, `GET /{session}/review` and `POST /{session}/discard` were the last
  routes with no behaviour coverage. Covering them found a real defect, which is the reason the slice
  was worth more than its coverage.

  **`discardNewApplication` could discard another Workspace's draft Application.** Four of the five
  AI-session handlers resolve the session through `GetAISession(ctx, workspaceID, id)` before touching
  it; this one passed the URL parameter straight to `UpdateAISessionStatus`, whose statement was
  `WHERE id = $1` with **no Workspace predicate**. A Workspace admin naming another Workspace's session
  id discarded it. The ids are random, which is not scoping -- and Workspace scoping is the single
  invariant the whole data layer is built on.

  **Fixed in both places on purpose.** The handler now resolves first, like its four siblings, so it
  reads the way the next one will be written; and the store carries the predicate, so a future caller
  cannot reintroduce it -- the posture `installer.RefuseIfExists` states for itself ("a guard that holds
  even when a future caller builds ExistingState wrong"). Relying on every caller to scope is a rule
  that holds until one does not, and one did not.

  **The test was written before the fix and watched failing** -- it reported the victim session as
  `discarded` against today's code. A test written after a fix proves only that the fix is present.

  **And the shared fixture's proposal had never been valid.** `createGeneratedSession` built a change
  with no Machines, which `aiassist.Validate` rejects. Every existing test using it still passed,
  because they read the session's *status* -- which the fixture sets directly -- and none of them
  validated. So the review screen's own re-validation, the thing its doc comment says it exists for,
  had been exercised by nothing. The first test to call it found that. Third fixture-faithfulness
  finding in three days, all the same shape: a fixture that declares less than the real thing does not
  fail, it passes against something looser.

  **Live, and honestly partial.** The scoping is confirmed on the running service: another Workspace's
  session is 404 from `default`, and an `open` session is 422 rather than a blank screen. **The review
  screen itself could not be checked live** -- no `generated` session exists in any Workspace (all are
  `open` or `published`), and one cannot be created without a real model call. The two Postgres-backed
  tests are the whole of that coverage.

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
  inferring it from where the middleware sits. ~~`Cache-Control` was deliberately deferred: the
  filenames carry no content hash, so a long max-age would serve stale assets after a deploy.~~
  **Done 2026-09-24** — the hash arrived, so the header could too. See the asset-fingerprint entry
  below; the deferral was right about the dependency and the dependency is what got built.

- **The Flow 2 mockup — gap recorded 2026-09-23, nothing scheduled yet.** A second owner canvas
  ("Menata Runtime — Case 03 Flow", 39 artboards: 19 desktop at 1280px, 20 mobile at 390px)
  revises the Flow 1 set this section's seven phases ported, rather than replacing it. The live
  canvas itself (an appifact Design canvas, not a static export) is
  `https://claude.ai/artifact/Wzkc6reCNHBJvtq2DJHsU6` — read its `project/canvas.json` for the
  board index, one `project/<Name>.dc.html` per artboard (desktop unprefixed, mobile `M##-`
  prefixed; `NewApp`/`NewAppReview` are boards 15/16, the generated-Application flow named below;
  `WorkspaceArchived`/`M02b-`, `M03b-WorkspaceMenu`, `M03c-NewApp`, `M04a-Settings`,
  `M06a-AppSettings`, `M07a-MyDocuments`, `M07b-InboxMore`, `M07c-AssignedToMe` are boards this file
  had previously only seen described secondhand, via the audit below). Full study, board-by-board,
  with what each gap would cost in *primitives* rather than in screens: `menata-app-document`'s
  `audits/2026-09-23-kajian-gap-mockup-flow2.md`. Feature level, and only what a reader of this
  file needs:

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

  **Status as of 2026-09-27, read back against this list rather than left to age**: draft →
  published shipped as Tahap 8 (AI Metadata Assistant); Workspace archive/restore shipped as
  Tahap 7; notifications shipped as Tahap 6 (plus the SLA-breach reminder, 2026-09-27); deactivating
  a member shipped 2026-09-27 (Flow 2 canvas re-audit gap #2). Only the workspace **audit log**
  remains genuinely unbuilt of the five.

  **And one that is not a phase but a product:** boards 15/16 generate a whole Application from a
  natural-language description, review it, then publish it. It lands on the three deferrals this
  file already carries as "not urgent while the manifest ships with the binary" — runtime-writable
  metadata, hot reload with change classification, and rejecting unknown keys at load — because
  it *is* the end state those rows name: metadata edited by someone who cannot restart the
  process. Its danger is the ordinary one stated inside out: a generator that emits Go, or emits
  YAML that then needs per-Application code to work, would break this runtime's premise while
  looking like it succeeded.

  **Kajian requested by the owner, 2026-09-25**: `menata-app-document`'s
  `audits/2026-09-25-kajian-new-application-ai.md` -- reads boards 15/16/M03c directly, finds
  generating a *new* Application is actually the additive-only, lowest-risk case
  `metadata-hot-reload-safety.md`'s own §7.2 table already names (no prior Application to compare
  against, no stored data to violate), and formalizes four best practices the owner asked for:
  the generator's own conversation grounded in `capabilities.md`'s declared vocabulary (so it can
  never propose what the runtime can't build), confirming via `internal/metadata.Validate` failures
  rather than a memorized question list, scope-narrowing questions driven by structural ambiguity,
  and -- the one with no precedent anywhere yet -- an append-only record of every "New application"
  conversation, with the AI's own "the runtime can't say this yet" moments recorded as structured
  rows, generalizing this repo's own "second real case" discipline from code that had to be
  hand-written twice to conversations that ask for the same missing capability more than once.
  Four things still wait on the owner before any of it is built (the kajian's own §4): build order
  (additive-only first, vs. the full hot-reload gate), where conversation history is stored and who
  may read it, the generator's own model/prompt work (out of this kajian's scope), and confirming
  `requireWorkspaceAdmin` is the right gate (the mockup's own "Only workspace admins can add
  applications" suggests it already is).

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

  - **My Documents was sending submitters to the page this repo had already ruled out**
    (2026-09-24, owner: *"card nya masih mengarahkan ke halaman yang salah"*). Its cards linked
    `/machines/mch_document/records/{id}` -- the *generic* record page, a field-by-field CRUD form
    with Edit and Delete buttons. A submitter asking "where has my document got to" got that.

    **The comment at the call site explained the bug and did not see it.** It read: "the href goes
    to the Document, not to a step's review screen, because on this list the viewer is the
    submitter and has no step to decide." The premise is right; the conclusion does not follow.
    Having no step to decide is a reason not to show a *decision bar* -- which the Review screen
    already handles, gating it on `authorization.AllowsAction` -- not a reason to send someone to
    the generic page that `rendering.detailBackLink`'s own comment had called "POC scaffolding no
    real approver should land on" since 2026-09-19, on the owner's own instruction. Two parts of
    the codebase held the rule and the exception to it, three days apart, and neither knew.

    The Review screen was always the right destination -- it draws the document, its PDF, every
    step's state and the SLA -- so what was missing was a way to reach it holding a *Document*.
    `/machines/{machineID}/records/{id}/review` accepts a Document id now;
    `composition.ReviewStepForDocument` decides which step it opens on, as a rule rather than a
    guess: the viewer's own actionable step, else the step the Document is waiting on, else the
    first undecided one, else the last by sequence. A Document with no steps at all 404s there and
    the card falls back to the generic page, which is the one case that screen cannot draw.

    **Opening an approver's screen as a non-approver exposed a second defect, in words rather than
    logic.** The panel headed "Your Signature Position" told the submitter that "your saved
    signature image is stamped here automatically when you approve" -- about somebody else's
    signature, on a document they cannot approve. Every clause was wrong. It carries whose
    signature it is now and switches to the third person: *"Isnawan's signature is stamped here
    when they approve."* Verified in a browser, along with the decision bar correctly not
    rendering at all.

  - **Why the app felt slow to open, answered from the logs** (2026-09-24, owner: *"kenapa baru
    saja aku akses, tidak langsung membuka ya? apakah terlihat di log?"*). It was visible, but not
    in this app's log -- which records query counts and no status, duration or request headers at
    all. The edge log had it.

    **The access itself was fine**: 38 requests, every one 200, 1-18ms server time bar two honest
    outliers (177ms for the first request after three hours idle, 425ms for a 129kB PDF render).
    The `303 -> /login` sitting at the tail of the log, which looks exactly like "it would not
    open", was a crawler from another address, not the owner.

    **What was real was in a header.** `/home` and `/approval-inbox` arrived with
    `Sec-Fetch-Dest: empty` 410 times against `document` 11. A navigation is always `document`;
    `empty` is a `fetch()` -- the service worker re-issuing every request. It did two wasteful
    things: proxied every subresource through `event.respondWith(fetch(event.request))`, which is
    what the browser does unaided minus a trip through the worker, and made each navigation wait
    for the worker to *boot* before its own request started. That wait happens in the browser
    before anything reaches the server, which is precisely why no amount of reading this app's own
    log would have found it.

    Fixed with navigation preload plus returning without `respondWith` for anything that is not a
    navigation (a registered fetch handler is all installability requires). Verified at the edge:
    navigations now arrive as `document` with `Service-Worker-Navigation-Preload: true`, one
    request per navigation instead of three.

    **And the edge was overriding the asset fix from the entry above.** `caddy` matched `*.css
    *.js` by path and stamped `immutable` on all of them whatever this app sent -- the actual
    mechanism behind the stale stylesheet, and it also pinned `/sw.js`, the worker in front of
    every navigation (bounded at 24h by the browser, not permanent). Both patterns are out of that
    site's block now; images and fonts keep the long cache because their URLs never move. `/sw.js`
    additionally sends `no-cache` from the app, which is where that decision belongs.

  - **Static assets carry a content hash** (item 3). A page names `/css/app.<8 hex>.css` and the
    two vendored scripts likewise; the hash is computed once at startup off the files the same
    router serves, so the URL a page emits and the file it resolves to cannot disagree. A
    fingerprinted request is `immutable` for a year -- safe precisely because its URL moves the
    moment the bytes do -- and a bare one is `no-cache`.

    **This closes a deferral that named its own precondition and was right about it.** The
    `Cache-Control` row above read: "the filenames carry no content hash, so a long max-age would
    serve stale assets after a deploy." True, and the conclusion drawn from it -- defer the header
    -- left the *other* half unhandled: with no hash and no header, browsers fall back to
    heuristic freshness, which is how a stylesheet from before a deploy got served twice in one
    day and was reported as a UI that "still looks the old way". A cache problem wearing a
    rendering problem's clothes, and it cost a round of looking in the wrong place.

    The hash goes in the filename rather than a `?v=` query: both move the URL, but a query is the
    weaker form, since proxies and CDNs may drop it from the cache key or refuse to cache at all,
    and this app is already behind one.

    Gated by `TestFingerprintedAssetCachePolicy` and `TestAssetURLChangesWithContent`, which
    assert the two halves as one mechanism -- a fingerprint with no long max-age buys nothing, and
    a long max-age with no fingerprint *is* the bug. The second test began as one that skipped
    when `static/css/app.css` did not resolve from the package directory, which is a test passing
    for the wrong reason; the hashing was split into `fingerprintOf` so it could run against a
    file the test writes.

  - **Workspace Home is board 03 now** (item 2). The three-card grid plus a separate "Your
    access" grid at the bottom became one row list: icon tile, name, description, the viewer's
    role in that Application, its pending badge, a chevron.

    **The "Your access" section did not shrink, it dissolved.** It answered "which role do I hold
    where" in its own grid at the foot of the page; the row answers it where the question is
    actually being asked, next to the Application it is about. `accessTile`'s own doc comment had
    predicted half of this -- that Fase 3 would turn two tiles into one per Application, which it
    did -- and then the rows absorbed them.

    `nav_home` gained a `title:` and `description:` in `domain.RuntimeScreens`, which is the
    label/title split from two commits earlier applied to a *runtime* screen: "Home" is what a
    menu and a breadcrumb call this page, "Applications" is what the page calls itself (board 03).
    The board's own subtitle names the Workspace ("Apps you can open in Dokter Kecil"); a static
    string cannot, and the Workspace's name is one row up in the chrome anyway, so it does not.

    Two things the board asks for and this does not render, both for reasons already recorded: the
    **"Add an application" panel** (§5.1 of the gap study -- there is no such capability) and the
    **member count** in the closing row (not a field on `domain.Workspace`). The closing row's
    role pill also renders only when there is a role to name: `displayRole` prints an em dash for
    an empty one, which reads fine in a table cell and reads broken inside a pill -- and empty is
    exactly the shared admin credential's case, which still sees the row because it can open the
    destination.

    Caught on a phone, not in review: reserving a fixed-width column for the declared summary
    count wrapped every Application's name onto two lines at 390px. The count gives way below
    `sm` -- board M03 shows the pending badge and nothing else.

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
  - **Assigned to me** (board 07b, owner: *"ok untuk assign to me. ubah label jadi assign me di
    teks bawah ikonnya"*) — the Approval Inbox's third `?tab=` destination: every Approval Step
    that has ever named this viewer, directly or through a Group they belong to, whatever its own
    decision is. `nav_assigned_to_me` declares `label: Assign Me` (the tab strip and the mobile
    bottom bar), `title: Assigned to me` (the page's own `<h1>`), `route:
    /approval-inbox?tab=assigned`, `icon: user-check` — moved out of the icon set's chrome-only
    group into the declarable one, its first real user.

    **This is the third real case for a filter that reads the viewing identity**, past this
    repo's own two-case trigger for the `$current_user` sentinel `datasets:`' `where:` still
    cannot express — named explicitly rather than left implicit, because the shape genuinely does
    not fit that sentinel (the Planned section's own entry on this, below, says why: a two-armed
    membership test plus a second Machine's sequencing state, not a Field compared against a
    written value) and a reader should not have to rediscover that. `composition.AssignedToMe`
    follows `ApprovalInbox`'s own shape instead — an I/O wrapper plus a pure `buildAssigned` a
    test can call with no database, exactly the split `buildInbox` already uses, with nine tests
    of its own (sequential locking, Group membership both ways, the submission-activity source for
    FROM/REQUESTED, newest-first ordering, orphan and other-people's-step skipping).

    **Reused `reviewStatusPill` rather than inventing a second status pill** — board 10's own
    Document-status colouring, now promoted out of `capabilities.md`'s page-internal row on its
    first real second caller. And found, while wiring the handler, that the original design
    (building each tab's filter-chip hrefs in `internal/web`) would have hardcoded a declared
    route literal there — `TestHandlersHaveNoHardcodedApplicationRoute`'s own territory — so
    `FilterChip` stayed plain data and `filterChip` resolves its own href from `ctx` via
    `routeByID`, same as everywhere else in this package; the Inbox's own SLA chips moved onto the
    same generalized component rather than keeping a separate one now that a second real caller
    existed for it too.

    Each tab composes only its own content — `showApprovalInbox` calls either
    `composition.ApprovalInbox` or `composition.AssignedToMe`, never both — so the new tab costs
    the other two nothing, and its own `GroupsForMember` read is the one query specific to it.
    Verified live: `/approval-inbox?tab=assigned` measures `queries=9 reads=9 repeated=0`, `queries
    == reads` and no repeats, same invariant the empty `getSweepRatchet` holds every other
    authenticated GET route to (this route is not swept by that test itself, since the sweep
    parses `router.go`'s literal path and never appends a query, but the property holds anyway).

  - **Application Settings hub (Fase 8) — *shipped 2026-09-25*, all 5 phases.** Asked
    for a mobile "chip layout" fix on the Role Matrix (this file's own deferral table, "Board m06's
    mobile layout"), reading that row's premise against the actual Flow 2 canvas (owner-supplied
    link, `ui-sample/README.md`'s "The Flow 2 canvas — its live link") found it stale twice over:
    board 06 is not a grid at all any more on *either* breakpoint, desktop or mobile — a
    plain-language two-column "What you can do / Who can do it" list — and that list lives inside
    a per-Application **Settings hub** (`M06a-AppSettings.dc.html`: Access → Members & roles /
    Groups / Permissions; Communication → Notifications; Configuration → Document types / Approval
    flow) that `appshell.templ`'s own doc comments already named and deliberately deferred ("no
    per-Application settings hub exists yet ... pointing it at `/authorization-matrix` ... would
    be a worse lie than the gap").

    Answers the fourth of this same bullet's own "four things wait on the owner" list, above
    ("whether Groups/Permissions really move into the Application"): **Permissions** becomes
    genuinely per-Application (a new page, scoped to one Application's own block); **Members/Groups
    administration stays exactly where it is** (`/workspace-members`/`/workspace-groups`), since a
    Group's and a Member's roles already span multiple Applications by design
    (`data.Group.Grants map[string]string`; `roleApplications`/`submittedAppRoles`,
    `internal/web/workspacemembers.go`) — moving either route's ownership into one Application
    would misrepresent that. The hub's own Members/Groups rows just door into those existing
    screens. The mockup's *Workspace*-level half (`M04a-Settings.dc.html`: General/Applications/
    Invitations/Authentication/Audit log/Danger zone) is explicitly **not** part of this — separate,
    tied to Workspace concepts (archive/restore a Workspace, an audit log) with no phase of their
    own anywhere in this file yet.

    Full design: `/root/.claude/plans/robust-squishing-pelican.md` (local to the machine that wrote
    it, not checked in — summarized here so a session without that file can still resume from this
    entry alone). Five phases, each its own commit and a safe place to stop between sessions:

    1. **Domain + metadata plumbing — *shipped 2026-09-25, corrected same day*.**
       `domain.NavigationItem` gained `SettingsHub bool` / `SettingsHubMember bool` — a new pair,
       not a reuse of the existing `Group` field, which `writing-guide.md` §12.1 already documents
       for a different purpose (the main nav's own dropdown-vs-inline submenu) and which no
       manifest populates today. `internal/metadata`'s `navItemDoc`/`toNavigationItems`/
       `validateNavigation` carry and validate both (an at-most-one `settings_hub` check mirroring
       `home_card`'s, plus `settings_hub`/`settings_hub_member` declared mutually exclusive on one
       item). Two new nav items (`nav_app_settings`, `nav_app_settings_permissions`) in
       `metadata/applications/document-approval.yaml`.

       Two things done in this phase that its own plan had marked optional, both to keep
       `go test ./...` green rather than expecting a red window until Phase 3: `appshell.templ`'s
       `defaultAppMenu` now filters out any item carrying `SettingsHub`/`SettingsHubMember`
       *before* sorting, so the two new items don't silently show up as a fourth/fifth tab in
       Document Approval's own strip and bottom bar the moment they were declared — the plan's
       design section had named this filter as part of the field's own meaning, but the phase
       breakdown had left it ambiguous which phase actually wires it, and leaving it for Phase 3/4
       would have been a real, if brief, visual regression. And `internal/web/appsettings.go` now
       exists with two handlers that answer `501 Not Implemented` (named, not a bare panic or 404),
       registered in `router.go` purely so `TestNavigationRoutesAreRegistered` has something to
       find — the plan's own preferred alternative over letting that gate go red until Phase 3.
       Neither route is reachable from the UI yet (no "Settings" link exists before Phase 4), so
       this is inert outside someone typing the URL by hand. `go test ./...` and
       `go test -race ./...` both green, with `DATABASE_URL` set (Postgres-backed suite included).

       **Per-Application by declaration, not a default every Application gets** — asked and worth
       recording rather than assumed: `SettingsHub`/`SettingsHubMember` are schema capability, not
       behavior. Only `metadata/applications/document-approval.yaml` declares the two new items;
       `project-management.yaml` is untouched and therefore still has no Settings hub or "Settings"
       link at all (Phase 4's own design checks whether the *current* Application declares a
       `SettingsHub` item before rendering the link, precisely so an Application with none is
       silently unaffected rather than shown a broken one). If Project Management ever needs one,
       its own file declares its own items — Document Approval's Permissions row makes no sense
       for an Application declaring no `roles:` at all, the same conditionality `roles:` itself
       already has, which is exactly the correction below turns into an enforced rule rather than a
       reader's inference.

       **Corrected the same day, before anything downstream depended on it: `SettingsSection
       string` (a per-item section *label*, e.g. `"Access"`) was itself a duplication this file's
       own conventions already warn against.** Asked directly ("bukannya kalau ini tinggal kalau
       tidak ada, tidak perlu muncul... bisa diotomatiskan?"), and the honest answer is that it
       should have been from the start: `internal/composition/rolematrix.go`'s `applicationBlock`
       already derives its own "Access" row from `len(app.Roles) > 0` rather than from a second
       declared fact ("It is derived rather than typed... so this row exists exactly when that
       list is non-empty"), and `SettingsSection` asked a metadata author to restate that
       distinction by hand, one string per item, for a grouping concept this hub has exactly one
       real value of. Two things survive the correction and one does not:
       - **Still declared, because routing genuinely needs it**: both `nav_app_settings` and
         `nav_app_settings_permissions` stay real nav items — this runtime resolves "which
         Application" a request belongs to by matching a *declared* route
         (`domain.Workspace.ApplicationForRoute`), the same reason `nav_my_documents`/
         `nav_assigned_to_me` are each declared despite sharing one physical handler with
         `nav_approval_inbox`. That is addressability, not classification, and dropping it would
         make `/document-approval/settings/permissions` unresolvable to any Application at all.
       - **Renamed, not removed**: `SettingsHubMember bool` replaces `SettingsSection string` on
         `nav_app_settings_permissions` — still needed to keep it out of the tab strip/bottom bar
         (`SettingsHub` alone only ever marks the *one* hub root), but a plain bool rather than an
         open string, since there is no second section value to distinguish yet. The section's own
         label ("Access") becomes a plain string in Phase 3's `appsettings.templ`, not a metadata
         field, until a real second section is a proven need.
       - **Actually removed, and this is the part worth naming**: whether the Access section's rows
         (Members & roles, Groups, Permissions) *render* is now Phase 3's job, reading
         `domain.Application.Roles` directly in `internal/composition/appsettings.go` — never "does
         a `SettingsHubMember` item exist". A `nav_app_settings_permissions` item that outlived its
         Application's own `roles:` being emptied would previously have kept showing a dead-looking
         row forever; deriving the row's *content* from `Roles`, while the nav item only ever
         decided its *addressability*, is what a second real case (a future Application's own hub)
         would otherwise have had to teach this file the hard way.

       Confirmed with the same `SettingsHub`-marked owner-decision question asked of Notifications/
       Document types/Approval flow, which this correction does **not** extend to: those three have
       no existing declared fact to derive from at all (no `document_types:`, no notification
       config anywhere in this schema), so there is nothing to automate yet — they stay static
       "Not built yet" rows in Phase 3, each its own future capability with its own metadata shape
       when it lands, not a placeholder mechanism built ahead of that need.

       **Not yet committed** — `git status` on this tree shows the changes above still uncommitted
       (`internal/domain/navigation.go`, `internal/metadata/application.go`, `internal/metadata/
       validate.go`, `internal/rendering/appshell.templ` (+ generated `appshell_templ.go`),
       `internal/web/router.go`, `metadata/applications/document-approval.yaml`, this file, plus
       the new `internal/web/appsettings.go`). A session resuming this work should diff or commit
       them before starting Phase 2, not assume a clean tree.
    2. **Permissions content reshape — *shipped 2026-09-25*.** `roleMatrixApp`'s role-per-column
       `<table>` is gone; every Application block now renders the same responsive two-column list
       (`grid ... sm:grid-cols-[320px_minmax(0,1fr)]`, header row hidden below `sm:`) the Workspace
       section of this same file already used for `RoleMatrixNote` — one pattern, not two, and the
       same reuse Flow 2's own `RoleMatrix.dc.html`/`M06-RoleMatrix.dc.html` draw (no grid, no
       chips, at either breakpoint). `RoleMatrixRow` gained `Who string` in place of
       `Granted []bool`, derived in `internal/composition/rolematrix.go`'s new `whoText` from the
       same `requiredRoles` intersection `actionRow` already computed -- "All roles" when
       unrestricted, the matching role names (declared order) when restricted, and "No one" for the
       disjoint-permissions state `requiredRoles`'s own doc comment names as real but previously had
       no rendering at all. `restrictionText` (renamed from `qualifier`) dropped the role-clause
       Who now carries, substituted a Machine's own name for the generic "record" in the ungoverned
       amber sentence, and returns an empty string (no second line) when Who already says
       everything a row needs to. `foldIdentical`/`sameGrant` compare on `Who`+`Qualifier`+`Open`
       now, and `TestWhoText`/the existing `rolematrix_test.go` suite were rewritten off `Granted`
       onto `Who` string assertions, plus one new edge-case test for the disjoint-permissions "No
       one" state that had no test before this pass either.

       The Application card's own header gained the mockup's framing: a `routeByID(ctx,
       "nav_workspace_members")` link (rendered as the real declared label, "Workspace Members →",
       not the mockup's own "Members & roles" copy -- CLAUDE.md's rule against a second literal for
       an already-declared label applies to a link's own text, not just its href) and a
       `RolesSummary` line ("Roles in this application: Approver, Submitter, Reviewer.") --
       capitalized and joined once in composition (`rolesSummary`/`capitalizeRole`), never in the
       `.templ`, the same "resolved view model, not a raw value worked out on the page" rule this
       file's own top doc comment already states. Replaces the bare role-name pill row that was
       there before.

       Verified live at `/authorization-matrix` (unchanged route and scope -- still every
       Application plus the Workspace notes on one page) against a logged-in admin session at both
       1280px and 390px: every row's Who matches what `document.yaml`/`approval_step.yaml`/
       `signature.yaml` actually declare (e.g. "Edit or delete a document" reads "Approver,
       Submitter" with **no** second line, correctly, since that Permission is role-restricted with
       no record-scoped arm yet -- the owner-scoped half ROADMAP.md's deferral table already names
       as blocked on data, not on this screen). `go test ./...` and `go test -race ./...` both
       green with `DATABASE_URL` set.
    3. **The hub page — *shipped 2026-09-25*.** Filled in the real bodies of the two Phase 1
       placeholder handlers (`internal/web/appsettings.go`, now one `showApplicationSettings`
       factory instead of two near-duplicates), plus new `internal/composition/appsettings.go`
       (`SettingsHubView`/`ApplicationSettingsHub` — two booleans, deliberately not a generic
       metadata-iterated row list: only one real Application has ever needed this hub, and a
       generic shape ahead of a second real case is exactly what the B1-B5 decomposition criteria
       warn against) and new `internal/rendering/appsettings.templ`. The Access section's rows
       (Members & roles, Groups, Permissions) are hardcoded structure gated on `hub.ShowAccess`
       (`len(app.Roles) > 0`) / `hub.ShowMembersAndGroups` (additionally the viewer's own
       Workspace role), read directly off `domain.Application` and the request's own identity —
       never derived from iterating `SettingsHubMember`-flagged nav items, honoring Phase 1's own
       correction rather than leaving it written down and unenforced. Notifications/Document
       types/Approval flow render as honest non-links ("Not built yet" tag, no `<a href>`), each
       with a comment naming the missing capability.

       One page, one route pair, reusing rather than duplicating: `roleMatrixApp` (Phase 2's own
       component) renders the content pane/detail unchanged, and a new
       `composition.RoleMatrixForApplication` (a five-line exported wrapper around the existing
       unexported `applicationBlock`, `internal/composition/rolematrix.go`) is the only new
       row-building code — Phase 2's own "one content pipeline, two pages" note finally cashed in.
       `/document-approval/settings` and `/document-approval/settings/permissions` render
       identically on desktop (sidebar + content always together, matching the Flow 2 mockup's own
       `RoleMatrix.dc.html`, which has no bare-list state at all) and differ only on a phone, where
       the root shows the row list (`M06a-AppSettings.dc.html`) and `/permissions` shows a "←
       back" link plus the detail alone (`M06-RoleMatrix.dc.html`) — the whole reason two routes
       exist. The current sub-page's own row is highlighted (`aria-current="page"`, a filled
       background) in the desktop sidebar, matching the mockup's own current-item styling.

       **One real bug caught before it shipped, not after:** the page's own `activeSection ==
       "permissions"` local variable was first named `onPermissions`, which itself contains the
       substring "Permissions" — `TestRenderingHasNoHardcodedPageHeading` correctly failed on it,
       since that gate matches a declared title/description as a plain substring after stripping
       comments, with no notion of word boundaries or identifier context. Renamed to
       `showingDetail`. Worth recording because it is exactly the class of false positive a gate
       with no comment-vs-identifier distinction will keep producing — not a defect in the gate,
       which did its job, but a naming trap worth remembering the next time a local variable's name
       happens to echo a declared string.

       **A pre-existing quirk this phase surfaced rather than caused, left unfixed on purpose:**
       the mobile bottom bar and desktop tab strip both default to marking their *first* item
       (Inbox) active when the current path matches none of their own declared items
       (`appshell.templ`'s `defaultAppMenu`, `matched == -1` fallback) — visiting either Settings
       route today shows "Inbox" highlighted, which is wrong. This is not new to this phase and
       Phase 4 does not fix it either: Settings reaches the tab strip via a *separate*,
       right-aligned link (§6 below), never one of `defaultAppMenu`'s own items, so the fallback's
       "nothing matched, default to first" logic has nothing to compare Settings against either
       way. Recorded here since this is the first screen to make the fallback visibly wrong rather
       than merely imprecise; fixing it is a `defaultAppMenu` change, out of scope for this phase.

       Verified live (browser, admin session, `.env` credentials) at 1280px and 390px against both
       routes: sidebar/list shows all three Access rows for an admin, content pane matches Phase
       2's own `/authorization-matrix` output exactly, mobile root shows the list, mobile
       `/permissions` shows back+detail only, and the Permissions row highlights correctly as
       current on desktop. `go build`, `go test ./...` (with and without `DATABASE_URL`) and
       `go test -race ./...` all green;
       `TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed` swept both new routes cleanly (`queries ==
       reads`, `repeated == 0`) with no ratchet entry needed.
    4. **Wire the entry point — *shipped 2026-09-25*.** New `settingsHubItem(app) (domain.
       NavigationItem, bool)` (`appshell.templ`) searches `app.AllNavigation` for the
       `SettingsHub`-marked item rather than looking one up by a fixed id -- the same reason
       `defaultAppMenu` had to become a filter rather than an unconditional call: a hardcoded
       `routeByID(ctx, "nav_app_settings")` would panic the moment `applicationMenuRow`/`moreSheet`
       render for an Application declaring none (Project Management, today, and any future one
       before it earns a hub of its own). `applicationMenuRow` gains a right-aligned link
       (`ml-auto`, icon + label, matching board 07's own placement) when found, nothing when not;
       `moreSheet` gains a row in the same position the mockup's own third row occupied, right
       after Workspace Home and before the per-Application switcher list. Both doc comments, which
       had explicitly deferred this ("no per-Application settings hub exists yet ... would be a
       worse lie than the gap"), are updated to say it now exists rather than left stale.

       Verified live at 1280px and 390px: Document Approval's desktop row shows "⚙ Settings" after
       Assign Me; its More sheet shows "Document Approval settings" after Home; Project
       Management's own desktop row and More sheet show neither, confirmed in the same browser
       session. `go build`, `go test ./...` (with and without `DATABASE_URL`) and
       `go test -race ./...` all green -- no conformance gate needed touching, since every string
       rendered is a field read off the already-resolved `domain.NavigationItem` `settingsHubItem`
       returns, never a retyped literal.
    5. **Docs — *shipped 2026-09-25*.** `capabilities.md` gained two rows: `SettingsHub`/
       `SettingsHubMember` in "Composition primitives", and `roleMatrixApp` in "Shared rendering
       components" -- the latter for its own real reason, not just to have an entry: `applicationBlock`'s
       row-building code now has two genuinely different callers (`RoleMatrixPage`'s multi-Application
       overview and `ApplicationSettingsPage`'s single-Application detail), which is exactly the
       "second real caller" this table's own promotion criterion asks for, not a row added out of
       obligation. `appsettings.templ`'s own page-internal fragments (`settingsRow`,
       `settingsAccessRows`, the static-section rows) stay undocumented here on purpose: each has
       exactly one caller (`ApplicationSettingsPage`'s two render modes are one Page, not two), and
       this table's own stated default is page-internal until a second real Page needs the identical
       shape -- adding them now would be the premature promotion the criterion exists to prevent.
       `ui-sample/README.md` gained a dated note tying the shipped screens back to the exact
       `RoleMatrix.dc.html`/`M06-RoleMatrix.dc.html`/`M06a-AppSettings.dc.html` boards they port.
       Two deferral rows in this file's own table closed as **superseded**, not fixed: "Board 06's
       columns mix two role namespaces" (the remaining "should admin get an always-✓ column" half is
       moot -- the shipped page has no columns left at all) and "Board m06's mobile layout (role
       chips per transition, not a grid)" (that whole matrix concept is gone; Flow 2's actual design
       was ported instead). `go test ./...`/`-race` both green; `TestCapabilitiesComponentsTableMatchesTempl`/
       `...MachinesTableMatchesMetadata`/`...CitesPromotionGuide` all pass against the new rows.

       **All five phases of this entry are now shipped.** The Workspace-level half of the Flow 2
       mockup (`M04a-Settings.dc.html`: General/Applications/Invitations/Authentication/Audit
       log/Danger zone) remains explicitly out of scope, as stated when this entry was opened --
       tied to Workspace concepts that had no phase of their own anywhere in this file yet at the
       time: archive/restore a Workspace (since shipped, Tahap 7) and deactivating a member (since
       shipped, Flow 2 canvas re-audit gap #2, 2026-09-27) both have one now; only the workspace
       audit log remains unbuilt.

- **Search on Workspace Members, My Documents and Assigned to me — *shipped 2026-09-25*.**
  "Search, filtering and pagination on record lists" (below, "Planned") was checked against the
  Flow 2 mockup canvas rather than assumed: search is what three boards concretely ask for
  (`Members.dc.html`, `MyDocuments.dc.html`, `AssignedToMe.dc.html`, each with an
  `<input type="search">`); **none of the 39 boards show pagination controls**, and every list
  shown is small (≤24 rows). `internal/data.Store` also has zero `LIMIT`/`OFFSET` capability today
  (every statement in the package is a fixed-literal `WHERE`, confirmed by reading all of it), so
  building pagination would mean writing genuinely new SQL against a `data jsonb` column with no
  forcing case yet -- the same "never build ahead of a real need" discipline already governing
  `internal/expression`'s missing `lt`/`gt` and 007 §8's Query Model. **This shipped search only;
  pagination stays "Planned" until a list actually reaches the ~200-row trigger
  `capabilities.md`'s own volume-threshold harness measures.**

  Every new filter follows the one pattern this app already had three working copies of
  (`pendingTabContent`'s Overdue/Due-today chips, `assignedTabContent`'s four decision chips): read
  the query param, reduce the **already-fetched** full list in Go, build any chip's count from the
  *unfiltered* set first. Zero new SQL and zero new queries, which is what makes it trivially
  compatible with `internal/web/querycount_test.go`'s sweep and the two numeric budgets --
  `TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed`/`TestAuthenticatedPageQueryCost`/
  `TestNavBadgeQueryCost` all measured unchanged after this change (`/home` still 13 queries, the
  nav badge still 5-8, the sweep's 24 routes still `queries == reads`, `repeated == 0`).

  New shared `searchBox` (`controls.templ`) -- a plain `<form method="get">` that submits `q` on
  Enter natively, no script, matching every existing filter chip's own no-JS posture. Used
  identically on all three screens from the start (the "second and third real case at once" this
  repo's promotion criterion asks for). One real HTML-forms subtlety it exists to hide: a GET
  form's own query string always **replaces** whatever query its `action` URL already carried
  (`?tab=mine` does not survive submission by itself), so `tab`/`status` travel as hidden inputs
  instead, and searching never resets whichever filter chip was already active
  (`activeFilterKey`, `approvalinbox.templ`).

  **My Documents gained a status filter row it never had** (`composition.MineFilters`,
  `internal/web`'s `pendingTabContent` widened rather than a new function, since it already returns
  `Mine` from the one `composition.ApprovalInbox` call): `All / In review / Approved / Rejected` --
  no "Draft" chip, since `metadata/document.yaml`'s own comment says that option was deliberately
  removed ("Add it back only alongside a real save-as-draft flow, not speculatively") and
  `fld_status` declares exactly the three real values. `PendingApprovalCard` gained a `Status`
  field (the Document's own `fld_status`, the same convention `AssignedRow.Status` already
  documented) to make this possible. **Assigned to me already had its own four-state filter row**
  (`assignedTabContent`, shipped with the screen itself) -- confirmed reading the real code before
  assuming a gap that wasn't there; it only needed the search box added on top.

  Verified live (browser, admin session): `/workspace-members` narrows by name or email while its
  total-count pill stays fixed (the mockup's own "24 members" does not change as you type);
  `/approval-inbox?tab=mine` shows real chip counts, clicking one narrows the grid, and searching
  narrows further within the active chip (confirmed by URL: `?tab=mine&status=in_review&q=...` --
  the chip survives the search); `/approval-inbox?tab=assigned` behaves identically. `go test ./...`
  (with and without `DATABASE_URL`) and `go test -race ./...` all green.

- **Flow 2's Tahap 3 is closed** (`menata-app-document`'s gap study §8: "Tiga worklist +
  pencarian + chip"), recorded here because it never was anywhere -- the two entries above shipped
  its whole scope (`composition.AssignedToMe` 2026-09-24, search+chips 2026-09-25) with no line
  tying them to the gap study's own numbering, the exact "the work happened, the plan doesn't say
  so" gap this file's own preamble warns about (see the bottom-bar row's history, above). Checked
  line by line against the study's §4.2/§4.3 tables rather than assumed:
  - Assigned to me: screen (`nav_assigned_to_me`, a tab by deliberate design --
    `document-approval.yaml`'s own comment on why a query is one destination, not a second nav
    level), all four filter chips (`assignedTabContent`), the two-armed "mine" test
    (`stepBelongsTo` -- direct assignee or a Group the viewer belongs to), the "via {group}"
    provenance note, and the decision date ("You approved 13 Sep") are all shipped.
  - My Documents: status chips (`MineFilters`), search, and the PROGRESS figure
    (`PendingApprovalCard.Approved`/`TotalSteps`) are all shipped.
  - **Not in scope here, and correctly still absent**: the **Drafts** section and the "Revise"
    action. The gap study puts both in Tahap 4 on purpose (`draft` status + `Rejected → Draft`
    don't exist yet) -- their absence is not a Tahap 3 gap.

- **Flow 2's Tahap 4 is shipped** (`menata-app-document`'s gap study §8: "`draft` + 'Revise'"),
  2026-09-25 -- `draft` as a real `fld_status` option, "Save as draft" in the submit wizard,
  "Continue" reopening it on an existing Draft, and "Revise" on a Rejected row in My Documents. The
  wizard's step count and reorder buttons, also named under this Tahap in the gap study, were
  already shipped in 6c-2/6c-3 (`documentsubmit.templ` already said "Step 1 of 2" and already had
  working ▲/▼/✕) -- confirmed by reading the file before assuming a gap, not carried forward
  unreviewed.

  **A real architectural conflict was found and resolved during this work, not assumed away.** The
  first design declared two new `fld_status` transitions (`draft → in_review`, `rejected → draft`)
  with an `action:`, mirroring how `mch_approval_step`'s own `fld_decision` edges carry `decide`.
  `internal/conformance.TestDocumentStatusIsDerivedNotSettable` -- a pre-existing gate this session
  had read but not connected to this design until the test failed -- holds a stricter, deliberate
  invariant than that: **no `mch_document` transition on `fld_status` may declare an Action at
  all**, because a Document's status is derived from its Approval Steps, never person-set, "not even
  an admin" (this file's own Flow 2 gap-study copy from board 06 new). Both new moves are real
  person-triggered status changes, so declaring them as Transitions was the wrong mechanism outright,
  not a test to work around. **Resolution:** neither move is a declared Transition. Each is gated
  the same way `action.CanDeleteDocument` already gates `delete` -- a plain business-state check in
  Go (`action.CanContinueDraft`/`CanReviseDocument`) -- plus a Permission for *who*
  (`prm_revise_document`, a new `revise` Action; Continue reuses the existing
  `prm_edit_document_not_reviewer`). The generic edit route still cannot move either value (an
  undeclared transition refuses every Action, `edit` included), so the hole this test exists to
  prevent stays closed; only the two new dedicated, Permission-gated routes below can.

  - **`draft`** added to `fld_status.options` (`metadata/document.yaml`), closing the condition its
    own header comment named for leaving it out ("add it back only alongside a real save-as-draft
    flow") -- that comment is now deleted rather than left stale.
  - **Save as draft** -- `submitDocumentWizard` (`internal/web/document.go`) branches on the
    wizard's own `intent` form field (`"draft"` vs `"submit"`, a second button,
    `documentsubmit.templ`): a draft skips `hasApprover` entirely and creates no Approval Steps,
    which is what makes a Draft the *only* status guaranteed to have zero steps by construction --
    the property `composition.SplitDrafts`/`reviewHref` both key on to tell it apart from the
    pre-existing "zero-step Document the generic form can make" edge case.
  - **Continue** -- `GET`/`POST .../records/{id}/continue-submit` (`showDocumentContinue`/
    `continueDocumentWizard`) reopen the same wizard prefilled from the Draft's own stored values
    and finalize it into `in_review` with fresh Approval Steps, as an `UpdateRecord` rather than a
    `CreateRecord`. **A second whole-record-rewrite trap was found and closed before it could ship**:
    the wizard's own bespoke form carries only four of `mch_document`'s eight Fields
    (title/type/file/mode), so a naive `UpdateRecord` with just the submitted values would have
    silently erased `fld_due_date`, `fld_submitted_by` and `fld_signed_file` on every Continue --
    the exact class of bug `signatureplacement`'s own carry-forward history already found once. Fixed
    by carrying forward every Field the form doesn't mention from one fetch, Machine-agnostic like
    `carryForwardExistingFiles` already is, rather than naming the three fields by hand. Scope,
    stated once rather than left to be discovered: Continue only ever finalizes a Draft into review
    in this pass -- there is no "save this edit, stay draft" loop on the form.
  - **Revise** -- `POST .../records/{id}/revise` (`reviseDocument`) moves a Rejected Document back
    to Draft and deletes its own Approval Steps. **A second deliberate exception, found and
    documented rather than papered over**: `action.CanDeleteApprovalStep` explicitly blocks deleting
    a *decided* step ("the audit trail... blocked outright"), and Revise needs exactly that --
    mixing old decided steps into `evt_step_decision_rollup`'s pool alongside a fresh submission
    would immediately roll the new cycle back to rejected before anyone decides anything. Reached
    through `store.DeleteRecord` directly (never the generic delete route, so that guard never
    runs), on the reasoning that Revise is not "delete a decided step in place" but "start a
    Document's approval history over" -- a different question that function's own rule was never
    written to answer. The human-readable fact ("Step N rejected") survives regardless, in
    `mch_activity`, which is append-only; only the structured step row itself is traded away, and
    only for a step whose Document is about to be resubmitted from scratch.
  - **My Documents' Drafts section** -- `composition.SplitDrafts` partitions the already-filtered
    `mine` list into Drafts/Submitted (`internal/web/approval.go`'s `pendingTabContent`); each
    renders only when non-empty. `MineFilters` gained the Draft chip its own doc comment used to
    explain the absence of. A Draft's card shows "Not submitted -- Last edited {date}" (its own
    creation activity, the same `submissions` map the Pending branch already reads -- resolved all
    along, just never rendered until now) instead of a progress bar (`TotalSteps > 0` now guards
    `pendingApprovalCard`'s approvers list and progress bar, so a zero-step Draft shows neither
    rather than a misleading "0 of 0 approved").
  - `pendingApprovalCard` changed from one whole-card `<a>` to a `<div>` wrapping an inner `<a>`,
    because Revise's own `<form>`/`<button>` cannot legally nest inside an `<a>` -- invalid HTML
    that browsers handle inconsistently on click. Every other status renders identically; only a
    Rejected card gained the sibling form.
  - `capabilities.md`: the Action row (second real case, `revise`), the Role-based Action permission
    row (`prm_revise_document`), and three new routes documented. `domain.KnownActions` grew to five
    (`ActionRevise`), gated by `TestCapabilitiesDocumentsKnownActionsAndServices`.

  **What's still open, unchanged**: the Workspace-level Settings hub (Tahap 5's Workspace half),
  notifications (Tahap 6), Workspace lifecycle -- archive/restore (Tahap 7), and generated
  Applications (Tahap 8) -- all still waiting on the four owner decisions this section's own §7
  names, none of them engineering.

- **Flow 2's Tahap 5 is shipped, with a scope the gap study did not anticipate.** The gap study
  (`menata-app-document`'s §8, "Dua hub Settings") asked to move Members/Groups/Permissions into a
  new IA and named that as depending on **Q4** (whether Groups & Permissions really move into the
  Application, changing routes and the authorization gate). Checking the actual code before
  building anything found that Q4 had already been answered, differently, by the prior "Application
  Settings hub" entry above: Permissions genuinely moved to `/document-approval/settings/
  permissions`, but Members/Groups deliberately did **not** -- kept Workspace-level
  (`requireWorkspaceAdmin`), reasoned there because a Group's/Member's roles already span multiple
  Applications by design, and because an Application-level admin role (which any real move would
  need) does not exist anywhere in `domain`. The owner confirmed that answer stands.

  That leaves exactly one real gap in Tahap 5: the **Workspace-level** Settings hub
  (`M04a-Settings.dc.html`) did not exist in any form. Read directly from the mockup canvas rather
  than assumed, row by row, against what the app can actually do today:

  - **Real, linked**: Applications (`nav_home`'s own Title/Description already say exactly what the
    mockup's row asks for -- reused, not retyped) and Members/Invitations, both resolving to
    `nav_workspace_members` -- the same "two mockup rows, one physical destination" shape
    `nav_my_documents`/`nav_assigned_to_me` already established, since `showWorkspaceMembers`
    already renders both lists on one page.
  - **Honest placeholders, "Not built yet"**: General (no logo/time zone/language field exists on
    `domain.Workspace`, and nothing renames one), Authentication (no 2FA/domain-allowlist concept
    anywhere), Audit log (a genuinely new concept -- `/activity` is a different thing, a running
    feed for any member owned by Project Management's own navigation, not an admin-only permanent
    record; reusing it would overstate what it does). Danger zone was in this list too, until
    Tahap 7 (below) built half of it -- Archive is now real, Transfer ownership is the placeholder
    that remains.

  `GET /workspace-settings` (`showWorkspaceSettings`, `internal/web/workspacesettings.go`), gated by
  the same `requireWorkspaceAdmin` group as `/workspace-members`/`/workspace-groups`/
  `/authorization-matrix` -- simpler than the Application hub's own handler in the one way that
  matters: no per-viewer admin check of its own (the middleware already refused everyone else), and
  no composition function (every row is either a real RuntimeScreen lookup or a static placeholder,
  nothing derived per request). `nav_workspace_settings` joins `nav_home`/`nav_workspace_members`/
  `nav_workspace_groups`/`nav_role_matrix` as a fifth `domain.RuntimeScreen` -- a Workspace-level
  concept, not an Application one, for the identical reason the other four already are.

  **Entry point, read from `M03b-WorkspaceMenu.dc.html` rather than guessed**: the Workspace
  dropdown menu's own single settings row, which used to link straight to Workspace Members, now
  reads "Workspace settings" and opens the hub instead -- matching the mockup's own consolidated IA
  (one settings entry, not a shortcut to one of the hub's own rows). Nothing is lost: Members stays
  fully reachable, one hop further through the hub's own People section.

  **This entry originally also claimed Workspace Home's own "Manage members" link was
  untouched.** That was never actually checked against `M03-WorkspaceHome.dc.html` -- whose own
  closing row already read "Settings →" pointing at this hub, not at Members directly -- and a
  screenshot caught the live page still linking straight to `/workspace-members` within the hour.
  Fixed the same day: that row also opens the hub now. Left here, not quietly corrected in place,
  because the sequence is the point this file's own preamble keeps making: an unverified "X is
  unchanged" is a claim like any other, and this one only got checked because someone using the
  actual page noticed it was wrong.

  `settingsRow`/`settingsPlaceholderRow` (written for the Application hub) were reused verbatim
  rather than rebuilt -- capabilities.md's own promotion criterion's second real caller, moved into
  the "Shared rendering components" table on arrival rather than staying page-internal a second
  time.

  **What's still open, unchanged**: notifications (Tahap 6 -- shipped below), Workspace lifecycle
  (Tahap 7 -- shipped below), and generated Applications (Tahap 8 -- shipped below too, by the time
  all four of these entries are read together). Nothing from Flow 2's own gap study remains open at
  this point except reminders/SLA-breach notifications and per-Application notification config,
  both named explicitly in the Tahap 6 entry as needing a primitive (a scheduler) this runtime has
  never built.

- **Flow 2's Tahap 8 is shipped -- the owner answered Q2 (2026-09-26): build the AI Metadata
  Assistant in full, live, no restart required.** `menata-app-document`'s own
  `audits/2026-09-25-kajian-new-application-ai.md` is the design this implements; read that first
  for the reasoning, this entry only records what actually shipped and where it diverged.

  **Two boundaries confirmed with the owner before writing any code, both load-bearing:**
  1. The assistant only ever proposes what is *already fully composable* today -- Machines,
     Fields, role-based Permissions, status Transitions moved through the generic edit form,
     on-create Events. It can never propose a real Approve/Reject-with-signatures workflow:
     `internal/action`'s own doc comment says that engine is hardcoded to
     `mch_document`/`mch_approval_step`, not generic, and a generated Application promising one
     would be exactly the "generator that emits YAML that then needs per-Application code to
     work" this file has warned against since Tahap 8 was first named. "Approval" in a generated
     Application means a role-gated status field, stated as such in the conversation, not
     approximated into something that looks like Document Approval and isn't.
  2. Two modes, unified in one conversation rather than built as two features, per the owner's own
     request: **new_application** (a whole new Application) and **extend_application** (a purely
     additive change to one already installed -- a new status option, a new role). Both produce
     the identical shape of artifact -- a validated `aiassist.GeneratedChange` -- and go through
     the same review → publish → reload pipeline.

  **The hot-reload question resolved smaller than the kajian's own §4 open question implied.**
  Generating a *new* Application is the one case `metadata-hot-reload-safety.md`'s own §3.3/§7.2
  classification table already calls the safest kind of change there is -- purely additive, no
  prior Application to compare against, no stored data to violate. That meant this shipped without
  building that design's own data-compatibility gate at all, not because it was skipped but
  because nothing this feature ever writes can produce the class of change that gate exists to
  catch (a removal, a rewrite, a retargeted relation) -- `internal/aiassist`'s writer is
  structurally incapable of emitting one. What *did* need building, and didn't exist anywhere in
  this codebase before today: an actual live-reload mechanism. `cmd/server`'s own
  `dynamicHandler` holds the whole route table behind an `atomic.Pointer[http.Handler]`; publishing
  rebuilds it from `metadata.LoadWorkspaces` and swaps it in only on success, leaving whatever was
  live untouched on any failure. Deliberately *not* the `AppState`-inside-`Deps` shape
  `metadata-hot-reload-safety.md` §3.4 sketches -- every handler in `internal/web` closes over
  `Deps`'s plain fields once, and there was no forcing case to change that everywhere; treating the
  *whole route table* as the swappable unit gets the identical atomicity property by reusing
  `web.Routes`/`web.Deps` completely unchanged, at the cost of resetting a couple of in-memory-only
  rate-limiter counters on the rare, deliberate admin action that triggers a reload -- named here as
  an accepted trade-off, not discovered later as a surprise.

  **`internal/aiassist` is a new package**, and its own boundary rule
  (`internal/conformance.boundary_test.go`) is worth naming since it is unlike every sibling: it may
  import `net/http` (calling the Gemini API is a plain HTTPS/JSON request, stdlib only -- `go.mod`
  gained no new dependency) *and* `internal/metadata` (reusing the real, exported `Validate`
  one-way -- the same gate every hand-written `*.yaml` file already passes), while still being
  forbidden from touching the database or the renderer directly. `internal/web` is still forbidden
  from importing `internal/metadata` itself (005 Phase 3-4's "the composition root builds both
  once"), so the reload trigger reaches it as a plain `func() error` on `Deps` --
  `Deps.ReloadMetadata` -- built and injected by `cmd/server`, the identical injected-capability
  shape `Mailer` already uses.

  **The one genuinely new capability with no precedent anywhere in this codebase**: every
  conversation is recorded (`migrations/012_ai_sessions.sql`), and every moment the assistant says
  "the runtime can't say this yet" is written as its own structured row
  (`ai_capability_gaps` -- requested capability, a one-sentence note, which session), separate from
  the free-text turn so a pattern across many conversations is a `GROUP BY` away rather than a
  prose-parsing exercise. This is the kajian's own §3.4 ask, generalizing the "second real case"
  discipline this whole file already runs on (`card_fields`, `Event.OnCreate`, both generalized
  only after a real second case, never predicted) from evidence that used to only come from code
  someone had already hand-written twice, to evidence that can now come from what people actually
  ask the assistant for. No dedicated review screen for it yet -- queryable directly, on purpose
  (the owner's own words: "untuk akses baca tanpa ui"), the same "don't build a screen before
  there's real content" discipline already applied to the Application Settings hub's static rows.

  **Scoped down from the mockup in one place, named rather than silently carried forward**: the
  conversation is a plain form POST + redirect per message (`POST /new-application/message`
  redirecting back to the same `GET`), not an htmx fragment swap or the mockup's own live-typing
  feel -- there is no chat-log precedent anywhere else in this app to extend, and a full round trip
  per message is judged an honest simplification for a screen used rarely and deliberately, not one
  worth optimizing perceived latency on in this pass. The entry point
  (`M03b-WorkspaceMenu.dc.html`'s own "Add an application" panel, a whole describe-it box inline in
  the menu) is similarly scoped to a plain link into the real conversation screen rather than
  replicating that box in the menu itself.

  **Not built in this pass, named rather than assumed**: navigation-item generation for an
  extended Application (`GeneratedNavItem` exists in the schema; `writeExtension` refuses it with a
  clear error rather than guessing a route); a review screen for the capability-gap log; and, per
  the owner's own explicit answer, the general hot-reload compatibility gate for editing an
  Application that already has real data behind it -- still exactly the scope
  `metadata-hot-reload-safety.md` describes, still unbuilt, and now with one real, narrower
  precedent next to it instead of zero.

  **Three real bugs found and fixed 2026-09-27**, chasing one live conversation that tried to
  build "Document Tracking": publishing it 403'd its own creator immediately on redirect, and the
  assistant's own attempt to repair that via `extend_application` could never succeed either.
  (1) `existingStateFor` declared its `Applications` map but never populated it, since this
  package's first commit -- `extend_application` validation looks it up and had therefore rejected
  every extension ever attempted, "not installed" even when it plainly was. (2) Publishing granted
  nobody a role in a brand-new Application, including its own publisher; the schema holds one
  direct role per person per Application (no room for "grant all of them" without adding a Group,
  which this first increment does not do), so the conversation itself now asks which one role the
  person will hold (`GeneratedApplication.PublisherRole`, required whenever roles are declared) and
  `publishNewApplication` grants exactly that. (3) A generated Application declared no navigation
  at all, so its Workspace Home card had no `HomeRoute` and linked back to `/home` -- closes the
  "not built in this pass" line above for `new_application`: `writeNewApplication` now generates
  one `home_card: true` item at the first Machine's generic page, itself. Regression tests for (1)
  and (2) were each verified to fail against the pre-fix code. The one already-published casualty
  ("Document Tracking" in "dokter-kecil") was repaired by hand to match.

  **A fourth, found by the owner asking the right question of the third.** The `icon:` field's own
  schema described the set as closed ("one of the known icon names named in the system prompt")
  while the prompt never named them, so a real conversation spent three straight turns inventing
  `Palette`, `Sparkles`, `FileText`, each rejected by `domain.KnownIcons`, none of them real. It
  is an `Enum` now, built from a new `domain.DeclarableIcons` (`KnownIcons` minus the chrome-only
  names no manifest may declare) -- the Gemini API cannot return a value outside one, which is how
  `color:` had been right all along. Verified against the live API with the exact prompt that
  produced the loop: `"icon":"file-text"`.

  **And a fifth, which is really the same lesson one level up: validate by loading, not by
  copying the rules.** `aiassist.Validate` is a hand-maintained subset of the load-time
  validators, so it drifts by construction -- a Permission naming a role its own Application does
  not declare passed it, because the real check (`metadata.validatePermissionRoles`) is
  cross-Machine, unexported, and runs only at load. Combined with `publishNewApplication` writing
  files *before* reloading, with no rollback, any such miss left metadata on disk that the running
  app refused to reload and the next restart would have refused to start from. That is precisely
  what happened at 15:45 and had to be cleaned up by hand. `aiassist.Write` now runs the real
  `metadata.LoadApplication` over the manifest once every file is in place, and `writeSet` undoes
  every write if it does not load -- created files removed, edited files restored byte for byte.
  Loading is not a copy of the rules, it *is* them. Writing that test found a second hole
  immediately, which is the argument in miniature: `Validate` checks no navigation at all, while
  the loader requires nav ids unique Workspace-wide, and since (3) above a generated Application
  brings one derived from its own id. It also revealed three existing fixtures describing
  Workspaces that had never actually loaded.

  **Failed publishes go back to the conversation** (owner's own framing: *"bukannya harusnya
  kembali ke layar chat dengan info kesalahan tersebut, dan metadata diperbaiki, sesuai dengan
  capability"*). A proposal that stops validating, or metadata that will not load, is a *fixable*
  problem -- and the only participant who can fix it is the assistant, since the person who
  clicked Publish never wrote the metadata. `returnToConversation` records what went wrong as a
  turn the assistant reads, runs its next turn so a corrected proposal is already waiting, and
  redirects back to the chat; the session drops to `open` so the broken proposal stops being
  offered as a draft. Safe only because `Write` rolls back first -- nothing is half-applied by the
  time it runs. The two exits that stay hard errors are the ones no conversation can fix: no
  reload hook configured, and a reload that failed after a write that succeeded.

  **Verified end to end by the owner's own collision test** (2026-09-27, committed as
  `metadata/workspaces/dokter-kecil/applications/document_review.yaml`): a second,
  deliberately Document-Approval-like Application generated into a Workspace that already had one
  built around `mch_document`. It named its own Machine `mch_document_item` rather than colliding
  -- and had it not, `Validate` would have refused it, since a Workspace's own ids stay reserved
  against itself, the half isolation never relaxed. One conversation exercised every fix above at
  once: generated navigation, an enum icon, a publisher role that opens instead of 403ing, a write
  into the Workspace's own namespace, and Machines told apart by Application. Zero errors in the
  log across the whole flow.

- **Flow 2's Tahap 7 is shipped (2026-09-26): Workspace lifecycle, archive/restore only.**
  `menata-app-document`'s own `audits/2026-09-23-kajian-gap-mockup-flow2.md` §5.2 and the mockup
  canvas's `Workspace`/`WorkspaceArchived`/`M02b-WorkspaceArchived` boards (read directly) are the
  design this implements.

  **Scope decision, confirmed with the owner before writing any code**: the Danger Zone mockup also
  names "Transfer ownership", but no artboard exists for it anywhere in the canvas, and this
  runtime has no singular Workspace Owner concept today -- `workspace_role` is admin/member, and
  several members can hold admin at once. Answered: ship Archive/Restore only; Transfer ownership
  stays a `settingsPlaceholderRow`, the same treatment every other not-yet-built row already gets.
  No `workspaces.owner_id`, no new Owner concept invented to have something to transfer.

  **The gap study's own warning turned out to be the real work**: *"Read-only global adalah yang
  paling perlu dipikirkan: ia bukan Permission per-Machine, melainkan satu gerbang di atas semua
  aksi tulis"* -- one gate above every write action, not a per-Machine Permission.
  `web.blockWritesToArchivedWorkspace` is that gate: method-based (any non-GET/HEAD request against
  an archived Workspace's own ctx scope is refused, `archivedWriteAllowlist` names the four writes
  that must still work regardless -- sign out, switch to a different Workspace, create a new one,
  restore this one), costing no extra query since `Archived`/`ArchivedAt` (`migrations/
  013_workspace_archive.sql`) ride on the same `workspaces` row `resolveIdentity` already fetches
  once per request. `appShell`'s own read-only banner (`rendering.CurrentWorkspaceArchived`) is the
  "keep the UI honest" half at banner granularity, not a pass over every individual form/button in
  the app -- the gate above enforces it regardless of what the UI shows.

  **Archive and Restore act from opposite sides of the same read-only gate, following the mockup's
  own words exactly**: `WorkspaceArchived.dc.html`'s copy is *"Archived workspaces are read-only and
  hidden from members. Archive from Workspace settings"* -- Archive is an ordinary
  `requireWorkspaceAdmin` action taken from *inside* the still-live Workspace (`POST
  /workspace-settings/archive`, its Danger Zone). The archived row on Choose Workspace has no "open"
  link, only Restore, so there is no "act from inside a read-only Workspace to leave read-only"
  problem to solve -- Restore is reached from *outside* it instead (`restoreWorkspaceIfAdmin`, a
  per-target-workspace admin check via `store.ListMemberships`, not the ambient ctx-scoped
  Workspace), the same shape `resolveWorkspaceMembership` already used for switching into one.

  **Edge case the mockup doesn't cover, found rather than silently mishandled**: `completeLogin`'s
  single-membership fast path (`internal/web/auth.go`) used to skip Choose Workspace entirely for
  an identity with exactly one membership. If that one Workspace were archived, the fast path would
  strand its admin on `/home` with no visible way to reach Restore. Fixed: the fast path also
  requires that one membership not be archived, otherwise falling through to Choose Workspace as if
  there were several -- zero live rows, plus (for an admin) the archived section with Restore, or
  (for anyone else) an honest empty list.

  Verified against the real dev database (`internal/data/workspace_test.go`,
  `internal/web/archivedworkspace_test.go`, `internal/web/auth_test.go`'s own new case,
  `internal/rendering/chooseworkspace_test.go`): archive/restore round-trips and refuses a
  double-archive/double-restore; the gate refuses a write and allows a GET plus every allow-listed
  path against a real archived Workspace; Restore refuses a plain member and succeeds for an admin;
  the sole-archived-membership login case falls through to Choose Workspace; an archived, admin-held
  membership renders under "Archived workspaces (1)" while the identical row held as a plain member
  renders nothing at all. A full manual click-through was not run against the shared dev
  database's real default Workspace -- archiving it would have made it read-only and hidden for
  whoever else is using this checkout, which the shared-working-directory convention this file
  already documents rules out; the bootstrap admin credential also cannot create a throwaway one to
  test against instead (`/create-workspace` needs a real per-email membership `currentUserEmail`
  can resolve, which the shared admin credential's placeholder subject does not have). The
  automated coverage above exercises the same handlers against the same real Postgres a manual
  session would have, which is why this is recorded as shipped rather than as verified-pending-manual-check.

- **Flow 2's Tahap 6 is shipped (2026-09-26): Notifications, in-app + email, two triggers.** The
  gap study's own framing was "a whole new concept in full (in-app + email, per-user and
  per-Application preferences). No foundation today beyond `internal/mail`" -- confirmed: that
  package's only three call sites before this were invite/verify-email/password-reset. The Flow 2
  mockup canvas itself draws no content for this screen, only entry points (Account menu →
  Notifications, Document Approval settings → COMMUNICATION → Notifications); the only real design
  reference is the older `ui-sample/account-notifications.html`.

  **Scope, confirmed with the owner before writing any code**: build the two triggers that are
  actually buildable today without a new primitive -- *"a document is assigned to me for
  approval"* and *"my submitted document is approved or rejected"*. The mockup's other two
  (reminders, SLA-breach) need a time/schedule trigger this runtime has never built (this file's
  own Planned entry, "Extending the Event primitive... a schedule/time trigger... no scheduler");
  its own "step overdue" toggle is disabled for the identical reason. No per-Application admin
  configuration screen either -- the mockup itself never draws one.

  **Every piece reuses an existing primitive rather than adding a new mechanism.** A third
  `Service`, `send_notification` (`domain.Notify`, alongside `Rollup` on `domain.Service`), is the
  same "declared field-change/on-create Event → closed, runtime-owned Service" shape
  `rollup_parent_status`/`log_activity` already are. "Assigned to me" is
  `mch_approval_step.evt_step_created_notify` (`on_create: true`, `recipient_field: fld_assignee`)
  -- exactly `evt_task_created`/`evt_document_submitted`'s own on-create shape, reading a Field on
  the record the Event fired on, no cross-record resolution built (a case needing one is the
  trigger to add `Rollup`'s own `ParentField` indirection to `Notify` too, not before). A
  Group-held step (`fld_assignee` empty, CAP-F24) has no single recipient and is silently skipped
  -- named, not solved, the same posture the "One signature box for a Group-held step" deferral
  row already takes for the identical gap. "My document was decided" is
  `mch_document.evt_document_approved_notify`/`_rejected_notify` (`on: fld_status, when_equals:
  ...`, `recipient_field: fld_submitted_by`) -- declared on the *aggregate* status, not a step's
  own decision, since a per-step Event would tell the submitter "approved" after the first of
  several sequential steps.

  In-app storage is a plain declared Machine, `mch_notification` -- the same admission
  `mch_activity` already won, not a bespoke table. "Mark as read" was originally planned as the
  generic `PUT /machines/mch_notification/records/{id}` route with an ordinary `edit` Permission
  (`actor_field: fld_recipient`); building it surfaced a real hazard instead, corrected before
  shipping rather than after: the generic route replaces a record's *whole* `data` column from
  whatever the form submits (`data.ValuesFromForm`), so a minimal "just `fld_read`" form would have
  silently wiped `fld_recipient`/`fld_message`/`fld_link` -- the exact carry-forward hazard
  `signatureplacement.templ`'s own composed placement view was built to avoid. `POST
  /notifications/mark-all-read` (`submitMarkAllNotificationsRead`) reads and rewrites each record's
  full `Values` directly in Go instead, which has no such hazard; the `edit`/`delete` Permissions
  stay declared for a future per-row action, unused by this pass. The unread badge and the
  notification list (`GET /notifications`, `GET /api/notifications/unread-count`) are bespoke
  composed screens, the same "filter by identity in Go" shape Approval Inbox/My Tasks already are.

  **The one real gap this surfaced, found rather than assumed away**: `mch_document.fld_status` is
  written by `rollUpParentStatus`, never by a handler that itself calls `runEvents` --
  that function's own doc comment already named this precisely ("this write does not itself run
  Events on the parent... no Machine declares an Event that would need that here today"). The
  approved/rejected notification is exactly such an Event, so it would have silently never fired.
  Closed as part of this change: `rollUpParentStatus` now snapshots the parent's own old values
  before its write and calls `runEvents` on it after a successful one, one level deep (no Machine
  has a second rollup level to recurse into today; not guarded against, since nothing forces the
  case) -- `mailer`/`machines` threaded through `runEvents`/`runCreateEvents`/`rollUpParentStatus`
  from their five existing call sites to make this possible.

  Email preferences (`notify_assigned`/`notify_decided`, `migrations/
  014_notification_preferences.sql`, default true) live on `credentials`, identity-level like the
  rest of it (`/account-notifications`, ports `ui-sample/account-notifications.html`'s two real
  rows) -- the in-app record is written unconditionally either way, only the email is gated.
  `notificationLinkFor` is a named hardcoding exception (`writing-guide.md`): `mch_approval_step`'s
  real destination is its own `/review` route, not the generic detail page.

  **A second query-cost regression found and fixed the same session, by the same ratchet that
  caught the Choose Workspace one in the Tahap 7 entry above**: `/account-notifications`'s first
  version called `store.GetCredential` a second time for the notify columns, on top of the one
  `resolveChrome`'s own identity resolution already makes for the display name --
  `TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed` caught it immediately (`repeated=1`). Fixed by
  caching the whole credential on `requestIdentity` (`Credential(ctx)`, alongside the existing
  `Membership`/`Actor`/`ViewerName`) instead of just the one field `viewerNameFor` used to fetch for
  itself -- `viewerNameFor` is now pure, taking an already-resolved credential rather than fetching
  one.

  **A stale placeholder found and closed the same day, by the owner asking where notification
  settings actually belong**: `appsettings.templ`'s own Application Settings hub (`Application
  Settings hub` entry, above) already carried a "Notifications" row under Communication --
  `settingsPlaceholderRow`, "Not built yet" -- written in Phase 3 of that hub for the reason its own
  comment gave: *"no existing declared fact to derive from at all... nothing to automate yet."* That
  reason predates this entry and stopped being fully true the moment `/account-notifications`
  existed. The two are genuinely different scopes, and re-checking the *original* gap study line
  this row was always answering ("notifications (in-app and email, per user **and per
  Application**)") is what settled it rather than guessing: Tahap 6 shipped the per-*identity* half
  (an email preference on `credentials`, not a per-Application admin policy table), so an admin
  still cannot configure which alerts an Application sends by default -- that half of the original
  ask stays unbuilt, same as Document types/Approval flow beside it. But the identity-level page
  already groups its content by Application ("Document Approval" as of Tahap 6), which is a real,
  honest destination this row can point at today. Changed from `settingsPlaceholderRow` to a real
  `settingsRow` linking to `/account-notifications`; Document types/Approval flow stay placeholders,
  since neither has any shipped concept behind it at all. Verified live at
  `/document-approval/settings`, both mobile and desktop sections.

- **The Flow 2 canvas re-audited end-to-end, board by artifact board, not from `ui-sample` --
  2026-09-26, the same day Tahap 8 shipped.** Owner request: re-check all 39 boards on the live
  canvas (`https://claude.ai/artifact/Wzkc6reCNHBJvtq2DJHsU6`) against the running app, since the
  `ui-sample/` mirror is a static, possibly-stale export and every Tahap above was ported against
  the canvas directly. All 19 distinct screens (mobile+desktop pair each) plus the two special
  boards were walked; six "shipped" claims already in this file (`appshell.templ`'s bars/sheets,
  `approvalinbox.templ`'s three worklists, `appsettings.templ`/`rolematrix.templ`'s Permissions
  page, `workspacesettings.templ`'s hub, and the Tahap 6/7/8 routes in `router.go`) were re-read
  against current code rather than trusted from prose, per the "verify before claiming untouched"
  lesson -- none were stale.

  **Result: the canvas is essentially fully ported.** Every screen from 01 through 16 (minus board
  12, the Dashboard, which is titled "TIDAK DIPAKAI" on the board itself -- parked by the owner,
  not a gap) resolves to a live route or in-page component that structurally matches its board.
  What remains open is a short, already-named list, not a new backlog:

  1. ~~**The submit wizard has no step 3.**~~ **Not a gap -- corrected 2026-09-27.** This line
     was wrong the moment it was written: it is a stale echo of the *old* `ui-sample` mockup's "OF
     3" eyebrow, already checked and closed against this same canvas on 2026-09-23 (this file's own
     "Flow 2 mockup" entry, above: "the wizard likewise drops to `Step 1 of 2` / `Step 2 of 2`,
     closing 'the wizard's third step' by deletion"). Reading `SubmitDetails.dc.html`/
     `SignaturePositions.dc.html` directly confirms it: both boards say "Step 1/2 **of 2**", the
     first's own "Continue →" goes straight to the second, and the second's own "Save positions →"
     is the wizard's real, final action -- no third board exists anywhere in the 39. The shipped
     code (`documentsubmit.templ`/`signatureplacement.templ`) already matches this exactly, down to
     a doc comment recording the same correction independently. What the earlier deferral row (this
     file, "The wizard's third step (boards 08/09 both say 'OF 3')") was actually naming is a real,
     separate, and still-true observation -- `submitDocumentWizard` sets `fld_status = in_review`
     at creation, before any signature is placed -- but that is a fact about *when signing happens*
     (per-step, later, via Review/decide), not a missing wizard screen.
  2. ~~**"Save as default flow" (board 08) is unbuilt"**~~ **Shipped 2026-09-27** -- see below.
  3. **SLA is drawn in hours** ("breached 4h ago," boards 07/10) but `fld_due_date` is date-only --
     no datetime Field type exists yet (007, Field Types table in `capabilities.md`).
  4. **Uploaded file byte size** (board 10: "2.4 MB") is never captured, in storage or on the
     record.
  5. **One signature box for a Group-held step** (boards 08/09) -- whose signature renders is
     undefined once a step's approver is a Group rather than a person.
  6. **Notification triggers beyond the two Tahap 6 shipped (approval-needed, decision-made):
     SLA-breach reminders and a per-Application admin notification policy.** This is the one item
     the Tahap 6 entry above already flagged as still open, and this audit found nothing else at
     that scale -- it is the largest remaining item in the whole study. Both halves need a
     scheduler primitive this runtime has never built (something that fires on the *passage of
     time*, not on a record write), which is why it has stayed open through six shipped Tahaps
     rather than being a simple wiring gap like the others above.
  7. ~~**Orphan route, found rather than assumed away**: `/authorization-matrix`~~ **Deleted
     2026-09-27** -- see the dedicated entry below rather than repeated here.
  8. **Desktop: the topbar's own content width doesn't match the page content's width** (found
     2026-09-27, live on `/document-approval/settings` -- a wide sidebar+card layout makes it
     obvious, but every page has the same mismatch). `appShell`'s `<header>`
     (`internal/rendering/appshell.templ`, the `<div class="flex h-15 ...">` row carrying the
     launcher/breadcrumb/application menu) has no `max-w-` at all -- just `px-4 sm:pr-7 sm:pl-9`
     padding, so it runs edge to edge. `<main>` (same file, a few lines below) is capped at
     `max-w-[1180px]` and centered (`mx-auto`). On any viewport wider than ~1180px plus padding,
     the header's own content (the Application menu row, the account menu on the right) extends
     further right than the page content ever does, which is what the screenshot on this finding
     shows: the Settings hub's card stops well short of where "Inbox / My Doc / Assign Me /
     Settings" reaches above it. Not fixed here -- noted per owner instruction, to be picked up
     later.

  **Two owner decisions this audit found still unresolved in this file's own text**, both cheap to
  close (no engineering, just a confirmation) and both already asked once, above (the study's
  §7 "four things wait on the owner" list): **Q1**, whether board-06-new (plain-language
  Permissions) supersedes board-06-old (the role×stage matrix) -- the shipped screen already
  renders the new shape, but this file never recorded that as a decision rather than an
  implementation detail; confirming it closes six old deferral rows the table above still lists as
  open. **Q3**, whether pending invitations belong merged into the Members table or stay a
  separate section resolving to the same route (today's shape, and consistent with the identity
  model's "an invitation is not a membership") -- still open as a display question, not answered by
  anything shipped since. (The other two original questions are answered by what already shipped:
  generated-Application timing by Tahap 8, Groups/Permissions moving into the Application by
  Tahap 5.)

- **Q1 and Q3 answered by the owner, 2026-09-27; Q3's answer shipped the same day.**
  **Q1: board-06-new supersedes board-06-old.** The plain-language Permissions page
  (`appsettings.templ` / `rolematrix.templ`, live at `/document-approval/settings/permissions`) is
  now this file's recorded final shape; the six deferral rows above that were waiting on this
  question (the "Board 06 presupposes an approval model this app does not have" row and its
  siblings) are closed as "owner confirmed 2026-09-27, no further work" rather than left open.

  **Q3: follow the artifact canvas, not the separate-section shape this file had defended as the
  model.** Re-reading `Members.dc.html`/`M04-Members.dc.html` directly (not from memory) showed
  invited people rendered as ordinary rows inside the same Members list -- an amber "Invited"
  status dot in place of the green "Active" one, same columns, no second section -- with a
  separate, undrawn "Invitations" nav destination beside it in the board's own sidebar. The
  previous entry's defense of a separate "Waiting to accept" block as "the model rather than a
  presentation choice" was itself the thing this audit corrected: the data model distinction
  (`data.PendingInvite` holds no membership or role, per the identity model, and still cannot be
  picked as an approver) does not require a second visual section to stay true.

  **Shipped**: `WorkspaceMembersPage` (`internal/rendering/workspacemembers.templ`) now appends
  `pending` invite rows to the same list container as `members`, immediately after the member rows,
  instead of a separate bordered block with its own "Waiting to accept" header. The amber "Invited"
  badge, dashed avatar (no identity exists yet to initial), and "Revoke" action are unchanged --
  matching the board's status-dot distinction without adopting its literal "Edit →" label on an
  invite row, since an unaccepted invitation has no membership to edit, only to revoke; named here
  rather than silently deviating, per this file's own citation discipline. Regenerated
  (`make generate`), built, and verified against the full suite: `internal/conformance` (routes/
  label/ratchet gates unaffected), the Member/Invite tests in `internal/web`
  (`TestSubmitInviteMember_*`, `TestSubmitAcceptInvite_*`), and `go test -race ./...` end to end,
  all green.

- **SLA-breach reminder via a scheduler primitive -- shipped 2026-09-27.** The one item the
  Flow-2-canvas re-audit found still open across the entire gap study: `domain.Event`'s own doc
  comment had named schedule/time-based triggers "the one deferred shape, waiting on their own
  second real case" since Fase 6, and `internal/composition`'s `logSLABreaches` stood in for it --
  a GET-triggered write, the only entry `internal/conformance`'s `readPathWriters` allowlist has
  ever carried, its own comment saying outright "this app has no scheduler to do it any other way
  yet."

  **Owner decisions, confirmed before writing any code (2026-09-27)**: the reminder's recipient is
  the Document's own submitter (`fld_submitted_by`), reusing `send_notification` exactly as the
  Tahap 6 triggers already do rather than adding cross-record recipient resolution to reach a
  pending step's assignee; the per-Application admin notification-policy screen stays out of this
  pass, the same scope line Tahap 6 already drew; the scheduler is a `time.Ticker` goroutine inside
  the existing `menata-app.service` process, not a second systemd unit.

  **`domain.Event` gained its third, mutually exclusive trigger shape**: `Schedule` (`DateField`,
  `When` -- only `"overdue"` today -- and an optional `GuardField`/`GuardEquals`, generalizing
  `On`/`WhenEquals`'s own field-equality shape rather than inventing a second one).
  `internal/metadata`'s `validateEvent`/new `validateSchedule` enforce exactly one of
  `on`/`on_create`/`schedule`. `internal/behavior.MatchedScheduleEvents` is the pure matcher, same
  shape as `MatchedEvents`/`MatchedCreateEvents`, reusing `experience.EvaluateSLA` for day-grain
  truncation rather than reimplementing it.

  **`internal/execution` -- empty since its own `doc.go` was written, reserved by `composition`'s
  own boundary comment ("physical execution belongs to internal/data and internal/execution") --
  is real code now.** The write-triggered dispatch logic (`sendNotification`, `notificationLinkFor`,
  `rollUpParentStatus`, `renderEventSummary`, and the `RunEvents`/`RunCreateEvents` entry points)
  moved out of `internal/web/helpers.go` into `internal/execution/events.go`, exported, so a
  schedule-triggered caller could reuse it instead of duplicating it -- the second real caller
  CLAUDE.md's own decomposition rule is written for. `internal/web`'s five call sites
  (`record.go`, `api.go`, `approval.go`) now call `execution.RunEvents`/`RunCreateEvents`. New:
  `execution.RunScheduledEvents(ctx, store, mailer, machines, now)`, called once per Workspace on
  every tick. Its one dedup check per record -- an existing `mch_activity` row already carrying the
  matched `log_activity` Event's own rendered summary -- generalizes the exact idempotency check
  `logSLABreaches` used, rather than inventing a second one; it refuses to dispatch (and logs a
  warning) if a Machine declares a `send_notification` schedule Event with no paired
  `log_activity` one to dedupe against, rather than silently re-notifying on every tick.

  **`metadata/document.yaml` declares two schedule Events sharing one condition** (`fld_due_date`
  overdue AND `fld_status == in_review`) -- `evt_document_overdue_log` (`log_activity`, the dedup
  marker) and `evt_document_overdue_notify` (`send_notification`, `recipient_field:
  fld_submitted_by`, a new `sla_breach` preference key). `domain.KnownNotificationPreferenceKeys`
  gained the third key; `migrations/015_notify_sla_breach_preference.sql` adds `credentials.
  notify_sla_breach` (default true, same shape as migration 014's two); `data.Credential`/
  `GetCredential`/`UpdateNotificationPreferences` extend to it; `AccountNotificationsPage`
  (`internal/rendering/account.templ`) gains a third toggle, "My submitted document is overdue for
  approval" -- the mockup's own "step overdue" row, unbuilt since Tahap 6 for exactly this missing
  primitive.

  **`cmd/server/main.go`**: `dynamicHandler` gained a second atomic snapshot
  (`schedulerState` -- `machineList`/`workspaces`), swapped in the same `Reload()` call as the
  route table, so a metadata hot-reload changes what the scheduler evaluates without a restart.
  `runScheduler` ticks every `cfg.ScheduleIntervalMinutes` (env `SCHEDULE_INTERVAL_MINUTES`,
  default 15), resolving each installed Workspace's real id via `store.WorkspaceBySlug` (the
  `workspaces` map is keyed by slug -- `domain.Workspace` carries no id at all, by design) and
  scoping a ctx per Workspace with `data.WithWorkspaceScope`, since every `data.Store` call needs
  one and there is no HTTP request here to have set it.

  **The `readPathWriters` allowlist is empty again**, closing the exception it was created to
  hold: `logSLABreaches`/`SLABreach`/`slaBreachMarker`/`Inbox.NewBreaches` and their four dedicated
  `buildInbox` tests are deleted from `internal/composition`, replaced by
  `internal/execution`'s own DB-backed tests
  (`TestRunScheduledEvents_detectsAndDedupesOverdueDocument`,
  `TestRunScheduledEvents_skipsNotOverdueAndDecided`) plus `behavior`'s pure matcher tests
  (`TestMatchedScheduleEvents_*`) and `metadata`'s parse/validate tests
  (`TestParse_eventSchedule`, `TestValidate_schedule*`). Verified end to end against the real dev
  database (not just fixtures): `make migrate-up`, `go test -race ./...` (all green), then
  `systemctl restart menata-app` with the new binary -- process stayed healthy, no panic, `/login`
  serving 200 immediately after.

  **Scoped down from the mockup in one place, named rather than silently carried forward**: the
  reminder notifies the submitter, not the currently-pending approver -- the mockup's own framing
  ("breached 4h ago" on the Approval Inbox) reads as being for the approver, but resolving "the
  step currently actionable on this Document" from a schedule Event declared on `mch_document`
  would need the cross-record resolution `domain.Notify`'s own doc comment says no real case has
  forced yet. A second schedule-shaped Event needing that reach is the trigger to add it, mirroring
  `Rollup`'s own `ParentField` indirection, not before.

- **The Flow 2 canvas re-audit above was re-run by hand, 2026-09-27, after it produced one false
  claim.** The 2026-09-26/27 re-audit entry (above) said the submit wizard "has no step 3" as if
  that were an open gap. It is not, and re-reading `SubmitDetails.dc.html`/
  `SignaturePositions.dc.html` directly confirms it: both boards read "Step 1/2 **of 2**", the
  first's "Continue →" goes straight to the second, the second's "Save positions →" is the
  wizard's real final action, and no third board exists anywhere in the 39 -- this was already
  checked and closed once before, on 2026-09-23 (this file's own "Flow 2 mockup" entry: "the
  wizard likewise drops to `Step 1 of 2` / `Step 2 of 2`, closing 'the wizard's third step' by
  deletion"). The shipped code (`documentsubmit.templ`/`signatureplacement.templ`) already matches
  the canvas exactly, down to a doc comment recording the same correction independently.

  **Root cause, worth naming so it does not repeat**: the original re-audit was produced by a
  forked subagent instructed to read every board directly, but its report on this one point
  quietly substituted `ROADMAP.md`'s own historical deferral-table row (which itself cites the
  *old* `ui-sample` mockup's "STEP 1 OF 3" wording as the context for a finding already closed
  elsewhere in this same file) for the canvas board it was supposed to be reading. The coordinating
  session relayed that line without re-opening the board itself to check it -- the identical
  "verify against the source before claiming it" lapse the Workspace Members invite-merge entry
  above was already caught making once this week, repeated instead of learned from.

  **Redone by reading every remaining board's raw HTML directly** (`Main`, `Workspace`,
  `WorkspaceArchived`, `WorkspaceHome`, `WorkspaceMenu`, `MemberEdit`, `AccountMenu`,
  `AppLauncher`, `Inbox`, `RoleMatrix`, `Review`, `MyDocuments`) rather than a subagent's synthesis
  of them, this pass found four real gaps the original audit missed entirely -- none a product
  decision, all a straight port left unfinished:

  1. ~~**The App Launcher (9-dot) has no "ADMINISTRATION" section.**~~ **Shipped 2026-09-27.**
     `AppLauncher.dc.html` drew one, two rows -- "Workspace settings" ("Members, applications,
     security") and "New application" ("Describe it, Menata builds a draft") -- that `appLauncher`
     (`internal/rendering/appshell.templ`) did not render at all, traced to a 2026-09-21 decision
     ("the launcher offers the Workspace's administration screens to nobody now," this file's own
     history) made three days *before* this canvas's own link reached any session working on this
     repo (2026-09-24) -- not a deliberate divergence, the canvas simply did not exist yet as a
     checkable source. `appLauncher` gained a fourth parameter, `viewer Viewer` (its one call site
     is inside `appShell`, where `viewer` was already in scope), and the identical Administration
     block `workspaceMenu` already carries, gated the same `viewer.WorkspaceRole != member` way --
     additive, not a reversal of the 2026-09-21 fix. No new query, no new route. Verified live: both
     rows render inside the launcher panel on `/home`, gated correctly, via a real authenticated
     `curl` request against the running service (not just `go build`).
  2. ~~**Edit Member has no "Remove from workspace" section at all.**~~ **Shipped 2026-09-27.**
     `MemberEdit.dc.html` drew a red-bordered section with a "Deactivate member" button ("A
     deactivated member can no longer open Dokter Kecil. Documents they submitted and their
     approval history stay."), and no such capability existed anywhere in this codebase.

     **Three decisions locked in with the owner before writing code**, all reshaping the build from
     a straight UI port into a real capability: (1) reversible -- a Reactivate action exists too,
     mirroring `migrations/013`'s Workspace archive/restore shape exactly (`migrations/
     016_member_deactivation.sql`: `workspace_members.deactivated_at`), even though the board draws
     only the one direction; (2) still visible in the Members list with a "Deactivated" badge
     beside the role badge, not a separate section or a hidden row; (3) **the guard against
     deactivating someone with open work assigned to them had to be a general, metadata-declarable
     mechanism, not a hardcoded Approval-Step check** -- the owner's own instruction, reasoning that
     other capabilities may later want to gate the same action.

     That third decision is what makes this a real new primitive, not a port: `domain.Event`'s
     `Schedule` shape (shipped earlier this same day) got a sibling, `MemberRemovalBlock`
     (`blocks_member_removal:` in YAML) -- a declared actor Field plus an `expression.Comparison`
     condition, reusing the identical condition primitive `Constraint.BlockIf`/`Schedule`'s own
     `GuardField` already share rather than a fourth copy. Introduced on its *first* real case
     (`metadata/approval_step.yaml`'s own `blk_step_pending`, guarding a pending step's assignee)
     rather than its second -- a deliberate, named exception to this repo's usual discipline,
     because the owner judged the shape general on inspection. `internal/behavior.
     MatchedMemberRemovalBlocks` (pure) and `internal/composition.BlockingReasonsForMemberRemoval`
     (the read, sweeping every currently-loaded Machine that declares one) are the two new pieces;
     a second Machine declaring its own block needs no Go code to be honored.

     The live-session half turned out to matter as much as the data model: the mockup's own
     guarantee ("can no longer open") is present tense, so `requireActiveMembership`
     (`internal/web/middleware.go`) evicts a deactivated member on their *very next request*, not
     merely their next login -- `resolveIdentity` already re-fetches membership fresh every
     request, so this reads what is already there at zero extra query cost on every route that
     already needed membership for something else. The one real cost this added was on the routes
     that previously did *not* need membership at all: `TestNavBadgeQueryCost` caught
     `/api/approval-inbox/pending-count` going from 8 queries to 9 immediately, which is exactly
     what that test exists for. Fixed with a named allowlist (`activeMembershipReadAllowlist`, the
     same discipline `archivedWriteAllowlist` already established) rather than a blanket `/api/`
     exemption -- the JSON API's own mutating routes live under `/api/` too, and a blanket
     exemption would have let a deactivated member keep writing through it.

     (The mockup's own per-Application access as an on/off toggle + "Manage in app →" link, versus
     the shipped role-dropdown shape, is a second, smaller, still-open divergence on the same
     screen -- not addressed in this pass.)
  3. ~~**The Applications list is missing the "draft/unpublished generated Application" row and
     the three suggestion chips."**~~ **Shipped 2026-09-27, and mis-scoped in the original
     finding, corrected before writing code.** The original wording named four boards
     (`WorkspaceHome.dc.html`, `WorkspaceMenu.dc.html`, `AccountMenu.dc.html`,
     `AppLauncher.dc.html`) as all drawing this row. Re-reading them closely at planning time found
     that only `WorkspaceHome.dc.html` actually does -- the other three boards are screenshots of
     *Home with one dropdown open on top of it*, and those dropdowns' own content (shipped as gap
     #1, above) has no draft-row or chips inside it. This cut the real work from "four call sites"
     to one.

     **Two owner decisions, both reshaping the build**: (1) the suggestion chips are
     metadata-driven, not literal strings -- `domain.Workspace.SuggestedApplications`
     (`suggested_applications:` in a Workspace manifest, `internal/metadata/application.go`), so a
     Workspace whose business the mockup's three examples don't fit declares its own;
     `metadata/workspaces/default.yaml` declares the mockup's own three as a plausible default, not
     a hardcoded global list. (2) the draft-Application row loads lazily
     (`hx-get="/api/home/draft-applications" hx-trigger="load"`, the same convention
     `notificationBell`/the nav-pending-badge already use for an unconditional once-per-render
     fetch) rather than through `/home`'s own eager render, which was already at this repo's hard
     13-query ceiling (`maxQueriesPerAuthenticatedPage`) with no room to spare.

     `showHomeDraftApplications` (`internal/web/newapplication.go`) lists `ai_sessions` rows with
     `status = 'generated'` (`Store.ListAISessionsByStatus`) and reuses `latestChange` -- the same
     pure decode of already-stored turns `showNewApplicationReview` already uses, no Gemini call --
     to name each draft and link to its own review screen. The whole "Add an application" section
     (description, an `idea` GET-navigation input, the chip row) is gated identically to
     `workspaceMenu`/`appLauncher`'s own Administration block (`viewer.WorkspaceRole != member`):
     hidden for a plain member rather than a form that only ever 403s. A chip's `Prompt` reaches
     the conversation as a plain `?idea=` query param, pre-filling `NewApplicationPage`'s own
     message input (`ConversationView.PrefillIdea`) -- the user still reviews and sends it
     themselves; nothing is created by the GET. Verified against the running service (curl through
     a real admin session): the section, all three chips (correctly URL-encoded), and the lazy
     placeholder all render on `/home`, and `/api/home/draft-applications` returns 200. `/home`'s
     own query cost confirmed unchanged at 13 (`TestAuthenticatedPageQueryCost`).

     **Reversed the same day.** The paragraph above ("the user still reviews and sends it
     themselves") was the first cut; the owner found it redundant in practice -- typing the idea on
     Workspace Home and then having to retype-by-clicking-Send on the very next screen is two steps
     for one action, and "Start →" already reads as the start. `rendering.NewApplicationPage`'s
     conversation form now carries a small hyperscript `on load` handler that calls
     `requestSubmit()` on itself when its own `#msg` input already holds a value -- true only for a
     PrefillIdea'd first message, since a redirect back into an existing session always renders
     `#msg` empty (`ConversationView.PrefillIdea` is never set once a session exists). `GET
     /new-application` still performs no write of its own -- "nothing is created by the GET" is
     still accurate -- the Gemini call happens from the same `postNewApplicationMessage` POST it
     always did, just fired by the browser instead of a click.
  4. ~~**The account (avatar) menu has no "WORKSPACES" section.**~~ **Shipped 2026-09-27.**
     `AccountMenu.dc.html` drew a list of every Workspace the identity belongs to, its role in
     each, and "Switch workspace →," inside the avatar dropdown itself; `accountMenu` was
     Profile/Security/Sign out only. **Owner decision, confirmed before writing code**: `appShell`
     renders on almost every authenticated page, and the list needs a real query
     (`store.ListMemberships`, already `loadWorkspaceChoices`' own query for the Choose/Switch
     Workspace pages) -- paying that eagerly on every page load for a menu most requests never open
     would be exactly the query-budget cost `TestAuthenticatedPageQueryCost`/
     `TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed` exist to catch, so it loads lazily instead: a new
     `GET /api/account-menu/workspaces` (`internal/web/accountmenu.go`, reusing
     `currentUserEmail`/`loadWorkspaceChoices` rather than a third copy of either query), fetched by
     a new `#account-menu-workspaces` placeholder with `hx-trigger="toggle once
     from:#menata-account-menu"` -- `toggle` is the native Popover-API event `sheetPanel`'s own
     popover div already dispatches on open/close (every menu in this chrome has been the Popover
     API since 2026-09-24), `once` meaning the fetch happens at most once per page view, only if the
     menu is actually opened. Archived Workspaces are dropped before rendering, matching the
     mockup's own account menu (unlike the full Choose Workspace page's expandable Archived
     section); the current Workspace gets a checkmark rather than a role pill, the other(s) get
     their role via the already-shared `displayRole`. Verified live against the running service: the
     new route returns 200 with the expected fragment (a real authenticated request against the
     shared admin credential's placeholder identity, which has no membership row, correctly returns
     an empty list plus "Switch workspace →" rather than erroring) and the placeholder's own
     `hx-get`/`hx-trigger` attributes render correctly on `/home`.

  All four are shipped as of 2026-09-27. Named here anyway, and left in place rather than deleted,
  because the process this section records -- rediscovering the wizard-step-3 non-gap, and #3's
  own "four call sites" mis-scoping caught before writing code -- is worth a future re-audit
  reading, not just the outcome.

- **Orphan route `/authorization-matrix` -- deleted, not just delinked, 2026-09-27.** Item 7 of
  this study's own remaining-gaps list (above): the route was still registered in `router.go` but
  linked from no `navigation:` entry in any Workspace's metadata, unreachable except by typing the
  URL directly, ever since Permissions moved to `/document-approval/settings/permissions` (Tahap
  5). CLAUDE.md's own metadata-hardcoding convention offers keeping an orphan with a named,
  forward-pointered exception as one option -- not taken here, because Q1 of this same re-audit
  (above, "Q1 and Q3 answered by the owner") had already answered the forward-pointer's question:
  board-06-new (this Permissions page) supersedes board-06-old (the deleted screen). A superseded
  screen kept alive as an unreachable second implementation of the same fact is dead weight with a
  name, not a real exception.

  Deleted: `internal/web/rolematrix.go` (`showRoleMatrix`, the route's only handler) and its
  `router.go` registration; `nav_role_matrix` from `domain.RuntimeScreens` (routeByID/labelByID no
  longer resolve it, correctly -- nothing should); `composition.RoleMatrix`/`workspaceSection`/
  `workspaceRoleActions` and `rendering.RoleMatrixPage`/`RoleMatrixView`/`RoleMatrixNote` -- the
  Workspace-wide-overview half of the old screen. **Not deleted**, because it is still live
  infrastructure for `/document-approval/settings/permissions`: `composition.
  RoleMatrixForApplication`/`applicationBlock` and `rendering.RoleMatrixApp`/`RoleMatrixGroup`/
  `RoleMatrixRow`/`roleMatrixApp` -- one Application's own card was always a real second caller
  (`appsettings.templ`'s desktop content pane and mobile detail view), not a page-internal shape,
  so deleting the page it was extracted alongside would have deleted the Permissions page's own
  content too. `internal/composition/rolematrix_test.go`'s nine tests were rewritten to call
  `RoleMatrixForApplication` directly rather than through the deleted `RoleMatrix`; one test
  (`workspaceSection`) was deleted outright since nothing calls that function any more.

  Verified: `make generate && go build ./...` clean; `go test -race ./...` and, against the real
  dev database, `TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed`/`TestAuthenticatedPageQueryCost`
  both still green (29 GET routes swept, one fewer than before, `/home` unchanged at 13) --
  confirming the deletion touched no other route's query shape. `capabilities.md`'s route table,
  `requireWorkspaceAdmin` route list, `roleMatrixApp` row and "Workspace's menu is derived" section
  updated to match.

- **CAP-V28: a saved default approval flow per Document Type -- shipped 2026-09-27.** Item 1 of
  the Flow 2 canvas re-audit's remaining-gap list (above): board 08's own "Save this as the
  default approval flow for **Contract** documents" checkbox, unbuilt since Fase 6c-2 for the
  reason `documentsubmit.templ`'s own top comment named outright -- "it needs a per-Document-Type
  template entity and a write direction, not a checkbox." Owner confirmed building CAP-V28 (the
  board-06 table's own recommendation since 2026-09-21) rather than either of the other two ways
  forward that table named.

  **Two follow-up decisions, both reshaping the build, confirmed before writing code:** (1)
  picking a Document Type in the wizard auto-loads that type's saved flow into the approver rows,
  if one exists -- without it, saving has no visible payoff; (2) the two new Machines are declared,
  ordinary, generically browsable/editable Machines (Metadata First), not bespoke backing data with
  no UI of their own.

  **Two new Machines** (`metadata/approval_flow_template.yaml`, `..._step.yaml`), claimed by
  `app_document_approval` but given no `navigation:` entry of their own -- the owner's own
  2026-09-21 instruction on this Application's menu ("dengan level 1, kebutuhanku sudah cukup")
  already closed it at exactly two items, so both stay reachable generically from All Machines
  instead, the same as any claimed Machine with no menu entry. `mch_approval_flow_template_step`
  is deliberately the *subset* of `mch_approval_step`'s own fields that describes a step's shape
  rather than one Document's live decision on it -- no decision, signature, or decided-by fields,
  since a template step is never decided, only a pattern to copy from. Neither Machine needed a
  migration: `records` (`migrations/001_records.sql`) is generic JSONB storage keyed by
  `machine_id`, and `/machines/{machineID}`/`.../records/{id}` are already parameterized by URL, so
  both got a real generic list/detail/edit UI for free the moment they were declared.

  **`internal/web/document.go`**: `createApprovalSteps`'s per-row value-building was factored into
  `stepRowValues(parentField, parentID string, i int, row stepInput)`, shared with the new
  `saveApprovalFlowTemplate` -- the second real caller CLAUDE.md's own decomposition rule is
  written for, not a speculative split. `findApprovalFlowTemplate` is the "find" half
  (`ListRecordsBy` on `fld_document_type`, then that template's own steps sorted by
  `fld_sequence` -- `ListRecordsBy` gives no ordering guarantee). `saveApprovalFlowTemplate` is
  find-or-create: reuse an existing template (update its mode, delete every existing step) or
  create one, then write fresh steps from the just-submitted rows -- **never append**, so
  resubmitting the same Document Type with a different chain replaces the old default rather than
  accumulating two. Called from both `submitDocumentWizard` and `continueDocumentWizard` right
  after their real Approval Steps are committed, gated on the checkbox and never on a Draft (a
  Draft creates no Approval Steps at all, so there is no completed chain yet to save); its own
  error is logged, never surfaced, since the real Document submission has already succeeded by
  that point. New `showApprovalFlowTemplateRows` serves the "load" half as an htmx fragment,
  `GET /documents/new/approval-flow-template` -- a Document Type with no saved template renders
  exactly a fresh wizard's own starting state (one blank row, the Field's first mode), so switching
  *away* from a type with a saved flow resets rather than leaves stale rows.
  `continueDocumentWizard`'s own carry-forward loop was factored into `carryForwardMissingFields`
  along the way, to stay under `TestHandlersStaySmall`'s budget after gaining the same save call
  `submitDocumentWizard` did.

  **`internal/rendering/documentsubmit.templ`**: `ApproverRow` gained a `StepPrefill` parameter
  (zero value = today's blank row, unchanged for its two existing callers) so the same component
  renders a saved template's own steps, not just an empty one -- the approver-type visibility
  (`.approver-user`/`.approver-group`) is decided server-side from the prefill now, since the
  hyperscript toggle that used to hardcode "start on User" only reacts to a future `change` event.
  The Mode `<fieldset>` was extracted into `modeFieldset`/`modeFieldsetOOB` (identical markup, the
  second carrying `hx-swap-oob="true"`) so the new fragment can update it independently of
  `#approver-rows`, the only target its own `hx-target` can point at. New checkbox, checked by
  default (board 08's own state): "Save this as the default approval flow for this document type."

  **Verified**: `internal/metadata` (both new Machine files load and validate, `TestAppManifestLoads`);
  seven new DB-backed `internal/web` tests (`internal/web/approvalflowtemplate_test.go`) covering
  save-when-checked, no-save-when-unchecked, replace-not-append on a second save, no-save-on-draft,
  and the load fragment's both cases (saved template found / none); `TestCapabilitiesMachinesTableMatchesMetadata`
  (both new Machines documented); `TestHandlersStaySmall`/`TestGetRoutesDoNotWrite`/`TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed`
  all green (30 GET routes swept, one more than before); `TestAuthenticatedPageQueryCost` confirms
  `/home` unchanged at 13 -- the new fragment is a separate lazy `hx-get`, never part of any page's
  own eager render. `go test -race ./...` end to end, all green. `make build`, `systemctl restart
  menata-app`, then verified live against the running service with a real authenticated session:
  `/documents/new` renders the checkbox and the `doc_type` `<select>`'s htmx wiring;
  `/machines/mch_approval_flow_template`/`..._step` both render through the generic Machine UI with
  no bespoke code; `GET /documents/new/approval-flow-template?fld_document_type=Kontrak` against
  the real dev database (no template saved for it there) returns one blank row and the default
  mode, exactly the fragment's own documented empty case.

- **Not fixed, only noted 2026-09-27 per owner instruction: Choose Workspace
  (`ChooseWorkspacePage`, `internal/rendering/chooseworkspace.templ`) still doesn't match its own
  artifact board** (`02-ChooseWorkspace.dc.html`), compared live via screenshot rather than
  assumed. Three concrete gaps, not one:
  1. **No member count or slug line under a workspace's name** ("dokter-kecil · 24 members") --
     already named in this file's own top comment (lines 31-36) as blocked on `domain.Workspace`
     having no slug/count field yet; still true, not a new finding.
  2. **The role badge renders the raw stored value, lowercase** ("admin"/"member") **where the
     board shows it capitalized** ("Admin"/"Member"). Checked against the rest of the codebase
     rather than assumed fixable by reuse: `displayRole` (`workspacehome.templ`, this screen's own
     candidate) does not capitalize either -- it only swaps an empty role for "—" -- and no
     capitalize-a-role-for-display helper is applied anywhere this broadly (`rolematrix.go`'s
     `capitalizeRole` is scoped to that one page). This page would need one, not reuse one.
  3. **The footer is bare "← Sign out"** (`authFooterNote`, just `backLabel`) **where the board
     reads "Signed in as `silvia@menata.id` · Sign out"** -- naming which identity is signed in
     before offering to sign out of it. Nothing in `ChooseWorkspacePage`'s own props carries an
     email today (`backHref`/`backLabel` only), so this needs a new parameter threaded from
     `showChooseWorkspace`/`showSwitchWorkspace` (`internal/web/auth.go`), not a template-only fix.
  4. **"Archived workspaces (N)" has no disclosure indicator at all** (found 2026-09-27, second
     screenshot pair) -- the board shows a `>` chevron marking it as expandable; the shipped
     `<summary>` (`chooseworkspace.templ:96`) strips both the native `<details>` marker and the
     generic list one (`list-none [&::-webkit-details-marker]:hidden`) and adds no replacement.
     Not a bug relative to *this app's own* convention -- the same line's comment cites
     `groups.templ`'s identical "+ New group" disclosure (`groups.templ:34`) as precedent, and that
     one also hides the marker, relying on its own "+" prefix as the affordance instead of a
     chevron. This row just has no such stand-in text, so it reads as a plain, non-interactive
     label rather than something to click.

  **All four shipped 2026-09-27.** (1) `data.Membership` gained `WorkspaceSlug`/`MemberCount`
  (`internal/data/workspace.go`), `ListMemberships`' query gained `w.slug` and a correlated
  `count(*) ... WHERE deactivated_at IS NULL` subquery -- still one `s.pool.Query` call, so
  `TestSwitchWorkspaceCostIsFlatInWorkspaceCount` stays flat (confirmed live: 8 queries
  regardless of workspace count). `rendering.WorkspaceChoice` carries both through to a live
  row's new second line ("dokter-kecil · 24 members" shape). (2) `chooseworkspace.templ` gained
  its own page-local `capitalizeRole`, deliberately not shared with `composition.capitalizeRole`
  (plane boundary, `internal/rendering` cannot import `internal/composition`) or `displayRole`
  (`workspacehome.templ`, which doesn't capitalize) -- exactly the "this page would need one, not
  reuse one" call already made above. (3) `ChooseWorkspacePage` gained a `signedInEmail`
  parameter; both callers already had the email in scope (`showChooseWorkspace` from
  `PendingEmail`, `showSwitchWorkspace` from `currentUserEmail`), so this is plumbing, not a new
  read. (4) The archived section's `<summary>` gained the same shared `chevron-right`
  `@icon(...)` already used for this exact affordance elsewhere (`appsettings.templ`,
  `appshell.templ`, `workspacehome.templ`), static rather than rotate-on-open -- this codebase
  has no `group-open:` Tailwind usage anywhere yet, so adding the first one wasn't warranted for
  a static disclosure marker. Verified: new/updated tests in `internal/rendering/
  chooseworkspace_test.go`, `go test -race ./...` end to end, and live against the running
  service with a minted session (a real Workspace, a real admin membership, no fixtures) --
  `/switch-workspace`'s rendered HTML shows "live-verify-co · 1 member", the "Admin" badge, "Signed
  in as ... · Back to Menata", and the chevron's own path data inside the archived section.

- **Bug, found and diagnosed 2026-09-27, fixed the same day: archiving
  the Workspace you're currently in signs you out, and there is no legitimate reason for it --
  it is a wrong redirect target, not intended behaviour.** `submitArchiveWorkspace`
  (`internal/web/workspacesettings.go:19-29`) redirects the admin who just archived to
  `/choose-workspace`. That route is registered *outside* the `requireAuth` group entirely
  (`router.go:168`), and its handler, `showChooseWorkspace` (`internal/web/auth.go:168-181`),
  resolves who is asking purely from `authorization.PendingEmail` -- a cookie that exists only
  during the pre-session login flow, before a real session is ever issued. An admin who was
  already signed in when they archived has a valid `menata_session` cookie but no
  `pending_email` one, so `PendingEmail` fails and the handler bounces them straight to
  `/login` (`auth.go:171-174`) -- indistinguishable, from the admin's side, from being signed
  out.

  **Fixed 2026-09-27: a one-line redirect change, not new plumbing.** This app already had the
  mid-session equivalent of this exact screen, `/switch-workspace`
  (`showSwitchWorkspace`, `internal/web/auth.go:229-254`), which resolves identity through
  `currentUserEmail` -- the real authenticated session -- and renders the identical
  `ChooseWorkspacePage`. `submitArchiveWorkspace` now redirects there instead of
  `/choose-workspace` (`/home` was the other option named here; `/switch-workspace` was picked
  since the admin's current Workspace is now read-only and they most likely want to pick another
  one, not sit on its now-archived Home).

  **Regression test**: `TestSubmitArchiveWorkspace_authenticatedAdminStaysSignedIn`
  (`internal/web/archivedworkspace_test.go`) reproduces the exact failure shape -- a real session
  cookie, no `pending_email` cookie anywhere -- archives, follows the `/switch-workspace`
  redirect, and asserts 200 (not a bounce to `/login`). Verified live too: a minted admin session
  against a real Workspace, `POST /workspace-settings/archive` returned `Location:
  /switch-workspace`, and following it with the same session cookie (still no `pending_email`
  cookie) returned 200 with the just-archived Workspace now listed under "Archived workspaces" --
  the admin stayed signed in throughout, never touching `/login`.

- **Question asked and answered 2026-09-27, not a gap: why does "Didn't get your verification
  email? Resend it" sit on `/login` rather than, say, `/reset-password`?** A real, functional
  reason, not arbitrary placement -- `LoginPage`'s own doc comment (`internal/rendering/
  login.templ:22-24`) already names *that* it's kept (the board omits it, but dropping an
  existing account-recovery path would strand someone whose verification email never arrived),
  but not *why here specifically*. The concrete answer is `submitLogin`
  (`internal/web/auth.go:74-78`): an unverified account's login attempt fails *on this page*,
  with the error "Please verify your email before signing in -- check your inbox, or resend the
  link below" -- a message that names "the link below" because it is the same page's own
  "Resend it" link. The failure and its recovery are the same screen on purpose: `/reset-password`
  solves a different problem entirely (a real, verified account whose password is forgotten),
  which is not the mental bucket someone who never got a verification email is in -- they know
  their password, they just never received the link. No change needed; recorded so the reasoning
  doesn't have to be rediscovered.

## Planned

- **Kajian: what is left, and in what order (owner request, 2026-09-29: *"masih ada pekerjaan apa aja
  saat ini? ... pelajari kembali untuk konsep 001 - 007, dan lakukan kajian terhadap pekerjaan yang
  belum, untuk prioritaskan"*). Read this entry before the ones below it — it is what orders them.**
  Full version, with the per-finding evidence, is `menata-app-document`'s
  `audits/2026-09-29-kajian-pekerjaan-tersisa-dan-prioritas.md`.

  **Why a fresh kajian rather than reading further down this file.** The five gaps the 2026-09-28
  Document Approval audit named (A–E) are all closed, and the ratchets say so: `documentApprovalCoupling`
  **empty**, `documentApprovalFieldCoupling` **11 across 7 files** (from 129), `projectionRatchet` **1**,
  `getSweepRatchet` **empty** — read those out of the maps, not out of this line. That matters for
  planning rather than as a milestone: the method those stages used, *remove a literal by finding the
  declaration that already answers its question*, is **exhausted**. None of the 11 survivors is a
  derivation away (4 are a form's `name=`, 007 §11.3 Binding with no primitive; the rest name a step's
  label, which nothing declares at all). So the next priority could not come from continuing that
  program, and had to be measured against 001–007 directly.

  **Finding 1 — 001 #6 is built on one of its two clauses, and the missing one is its safety clause.
  Ranked first.** Principle #6 authorises inference (*"Inference is preferred over explicit
  configuration"*) and then constrains it: *"**Inference must be inspectable.** … Hidden inference that
  cannot be explained is not an acceptable substitute for explicit configuration."* 004 §Inference and
  005 Phase 4 state the same obligation in their own words — three documents, one of them a `must`. This
  runtime has **no** mechanism for it: no diagnostics route, no `--explain`, no normalized-Application
  dump. The one diagnostic that exists counts queries, not inferences.

  The first clause is the entire method of Stages B through E2, so the surface needing explanation
  roughly tripled in two days: 9 derivation methods on `internal/domain/actioneffect.go`, 7 accessors in
  `internal/action/fields.go` (`DeclaredFields` deriving 8 ids by itself), `metadata.Normalize` — none of
  which existed before Stage B — on top of the four older inferences `capabilities.md` has listed as
  uninspectable for weeks. **The cost is demonstrated twice this week, which is why this outranks every
  other unbuilt capability**: `/review` 404'd on every Document for a day because a derivation returned
  `""` and the screen rendered a legitimate-looking empty result (nothing logged, nothing panicked, no
  test failed); and Stage E1 was one probe away from writing four values under an empty key and saving
  an approval flow with no approvers. **Both were caught by throwaway hand-written probes** — an
  inspection surface is that probe made permanent. The smallest honest shape: for one Machine, show each
  derivation's resolved result *and which declaration it was read from*, and show an empty result **as**
  an empty result, since that is precisely what went unnoticed.

  It is also the one item whose seam landed on its own: `metadata.Normalize` (`cd340fd`) made 005's
  Phase 4 a named callable function, and the 16 derivation accessors are already concentrated in two
  files.

  **Shipped 2026-09-29, Go level: `domain.Resolution` + `action.ExplainCast`.** For each role in an
  engine's cast, every derivation now reports its resolved value, **the declaration it was read from**,
  and a **status** — `resolved`, `undeclared`, `not applicable`, `input unavailable`. The status is the
  substance: the two defects above both resolved to `""`, and nothing could tell them apart.
  `input unavailable` is the `/review` 404 (the Field *is* declared; the caller had no document
  Machine), `undeclared` is Stage E1 (the role is cast and declares nothing it owes). Gated by
  `internal/conformance.TestInstalledCastsExplainWithoutDefects` over every installed Workspace plus
  `TestEveryDerivationIsOwedBySomeRole`; five unit cases in `internal/action`, each mutation-proved by
  deleting the branch and watching the intended message appear.

  **Step 1 was "measure the population first", and it overturned this entry's own framing twice —
  recorded because the plan promised not to predict.** A throwaway probe over both installed
  Workspaces gave **173 empty of 308**, which is useless: `mch_user` declares no `decide` transition
  and never should. Narrowing to Machines *cast in a role* still gave **38 of 50 empty** — also
  useless, and for the reason that became the design: a Document is not decided (its steps are), a
  signature store has no state model, nothing decides a flow template. Every one of those 38 is a
  derivation belonging to a *different role*, while the Machine cast as `step` resolved all nine of its
  own in both Workspaces. So the discriminator is neither "is it empty" nor "is the Machine cast" but
  **"does this role owe this answer"** — and no declaration said so. `domain.WorkflowEngineSpec.Answers`
  is that declaration, and it is why a gate over these numbers is possible now and was not before.

  **A third correction came from mutation, not design.** The first `Answers` listed nine flat
  derivations for `step`, read off what the real Machine resolved — which conflated *declares it* with
  *owes it*. Deleting `signature_placement:` from the real `approval_step.yaml` showed the file still
  **loads**: a step with no signature block is an approval Application that captures no signatures, a
  legitimate smaller installation. Marking it a defect would repeat, one level up, exactly the
  over-reporting `not applicable` exists to prevent. So `RoleAnswer.Optional` distinguishes a feature
  *absent* (a choice) from *partly declared* (a bug — a placement with an image Field and no
  coordinates stamps at (0,0)). Both halves re-verified against the real metadata: removing the whole
  block passes and moves the count 13→12 resolved, removing three of its five Fields fails.

  **The screen shipped the same day: `/inference`**, `nav_inference` in `domain.RuntimeScreens`,
  **workspace-admin only** (owner decision). Grouped by role, four statuses toned so the 52 correct
  empties stay quiet, empty values rendered as a visible dash rather than a blank cell — because a
  blank cell reads as "no problem here", which is how the `/review` 404 stayed invisible. Tests:
  two at the route (200 with real resolved values *and* their sources; a demoted member is refused),
  three at the page, five in Composition.

  **A re-read of 001–007 before building it changed three things, and it is worth recording which**,
  because two of them were corrections to this entry's own plan rather than additions:

  1. **The bespoke Go screen is required, not tolerated — the plan had it right for the wrong reason.**
     It was chosen as the pragmatic path without checking whether it was the correct one. 007 §18.13
     ("Execution plans are physical runtime artifacts. They MUST NOT become user-authored metadata"),
     001 #17 and 002's Runtime Boundary ("those stages must not leak physical implementation choices
     back into metadata") make declaring this as an Application's View over a Dataset a *prohibition*:
     a `domain.Resolution` describes the output of Phase 4 normalisation. 007 §7.1 confirms it from the
     other side — its DataSource kinds are Machine records, another Dataset, a declared relation, or "a
     runtime service source where explicitly supported", and a resolution is none of the first three.
     And 007 §31 lists *"how to expose execution diagnostics without coupling metadata to physical
     plans"* as an **open research question**, so this is unsettled ground with a named constraint
     rather than doctrine that was ignored. Those two citations are the forward-checkable pointers
     CLAUDE.md step 2b requires, and they are in the `nav_inference` comment.
  2. **007 §27's Capability Admission Gate had been skipped entirely**, and §33's performance
     extension with it. Answered: no existing primitive can express it (§7.1); one new generic
     primitive *would* unlock it and other cases (a "runtime service source" DataSource kind) but
     building that for one case is what B5 refuses, so it is admitted "at the lowest useful abstraction
     level" as §27's own closing line prescribes; it introduces no business-specific UI primitive and
     no query mini-language; and its fan-out is **zero** — `composition.Inference` is pure, issues no
     query, adds no logical dependency and generates no DAG node, which also satisfies §28 invariant 4
     and is why the GET sweep's two invariants hold for the route without it having to be careful.
  3. **007 §4.6 states deterministic construction as a MUST, and nothing held it.** `ExplainCast`'s
     output order comes from two slices, so it was correct by construction — but
     `KnownWorkflowEngines` and `Answers` are both maps, and one refactor ranging over either would
     have passed every test while making a diagnostics page reorder itself between refreshes. On a
     surface whose whole job is being compared against itself, that is not cosmetic.
     `TestExplainCast_isDeterministic` now holds it, mutation-proved.

  **Mutation testing found two assertions that did not exist**, and the honest note is that the first
  round of tests *passed* both mutations: colouring `not applicable` with the loudest tone (it is 52 of
  65 rows, all correct) and rendering an empty value as a blank cell. The rendering tests injected
  `Tone` directly and so never exercised `toneFor`, and Composition had no test at all. Both closed;
  `internal/composition/inference_test.go` says in its own header why it exists. A third mutation —
  restoring the `&block.Roles[len-1]` pointer-into-a-slice that the first draft was written with and
  that the next `append` invalidates — now fails three tests; on a page whose only job is showing what
  is missing, rows silently going missing would have been a memorable way to fail.

  **`statusPill` was extracted rather than duplicated** (007 §4.1, §12.3's `StatusBadge`): the
  component owns the pill, each caller owns its vocabulary, so `reviewStatusPill` keeps mapping
  `fld_status` and this page maps resolution statuses without either knowing the other's words.
  Second case, not first — CLAUDE.md's decision path, step 3.

  **Still unexplained by anything, and re-reading 001 #6 raised it rather than confirming it as a
  footnote**: the four inferences *outside* the workflow engine. #6's own scope is "when inference
  materially affects data access, **composition**, **authorization**, **rendering**, or execution
  planning" — child collections affect composition, board columns and the default table Layout affect
  rendering, and `person`→`mch_user` affects authorization, being the inference that makes
  `actor_field` resolve to an identity at all (the validation failure that produced
  `metadata.Normalize`). So they sit in the same clause as what just shipped, not a lesser one, and the
  plan's original ordering — which put them last with a shrug about likely small yield — was wrong
  about their standing. Measure the population first, as the engine half did; the model may not
  transfer, since a child collection is a property of a *pair* of Machines rather than of one, and
  "which role owes this answer" may simply not be the right question for it. That is a finding to
  reach honestly, not a shape to assume.

  **Planned 2026-09-29 after a 007 re-check, which corrected the population and the shape.**

  **Three, not four.** Board columns are not an inference: `View.GroupBy` is *declared* (`group_by:`)
  and the columns are that Field's own declared `options:`. Two declarations read together is a
  derivation, and a per-View one on the rendering side rather than a Phase 4 normalization — 007 §7.3
  Dimension is its home when the Data Plane grows one. `capabilities.md` corrected.

  **No conflict with 007's own layering, checked rather than assumed.** §15.1's pipeline is
  `Parse → Validate → Resolve references → Normalize → Build UI IR`, and all three remaining
  derivations sit left of "Build UI IR". §15.2 does list "layout relationships" as a UI IR
  responsibility, and UI IR is PROPOSED — but `DefaultView()` is a Machine-level *safe default*
  (005 Phase 4's own words), not a layout relationship in §15's sense. Nothing blocks.

  **Measured before planning**: 21 `person`→`mch_user` inferences, 31 child collections, 18 Machines
  falling back to the default table View (13 declare their own), across the three installed
  Workspaces. About 70 derivations — more than the engine cast's 65, so this is not a token slice.

  **The shape that does not transfer, now concrete.** A child collection is a property of a *pair* of
  Machines, so `metadata.Explain` takes the Machine **set** rather than one Machine, and
  `Resolution.Name` identifies the pair (`children: mch_task.fld_project`). That was the doubt this
  entry recorded above; it turned out justified.

  **And the four statuses do apply, which the measurement settled rather than symmetry**: a Machine
  that declares its own `views:` is `NotApplicable` for the default-Layout derivation (13 of 31), and
  a Machine nothing references is `NotApplicable` for child collections. A person Field whose
  `RelatedMachine` is empty after normalization is a **defect** — the exact failure that produced
  `metadata.Normalize`.

  **One dependency worth surfacing on the page itself**: `FindChildCollections` filters on
  `Field.IsReference()`, which reads `RelatedMachine`, which `Normalize` fills in for person Fields.
  If normalization does not run, `mch_user` loses every child collection — silently. One inference
  feeding another is exactly what an inspection surface should make visible.

  Out of scope: board columns (corrected in documentation, not explained here), UI IR (§15, PROPOSED),
  and §7.5 Relation.

  **§7.6 Projection is deliberately *not* next, reversing a recommendation made an hour earlier in the
  same session.** The argument for it was that it targets the measured cost centre — the index audit
  established that `json.Unmarshal`, not the scan, is the expensive half. That is true about *what* the
  cost centre is and false about whether there is any cost to target: the largest Machine averages
  **297 bytes** of JSONB over 13 rows, so projecting two of eight Fields saves about 6 KB in total.

  Against that, Projection introduces a **new silent-failure class**. `Record.Values` is a
  `map[string]any`; a projected record makes an unfetched Field read as `nil`, indistinguishable from a
  Field that is genuinely empty. That is the same shape as the `/review` 404, in a session that has
  spent days removing exactly it. 007 §27's question 5 asks what a capability's generic form costs, and
  here the answer is a silent failure mode for no measured gain.

  **Triggered deferral, so it can be re-checked rather than forgotten**: the 50,000-row threshold
  `make threshold` already measures, or the first Machine with a large Field (an attachment, a long
  text) that some screen does not need. Either makes §21.1's "only requested fields should be
  retrieved" worth its hazard; neither exists today.


  **§7.5 Relation — Step 0, measured 2026-09-29, and it cut "ten sites" to three.**

  The ten was a count of *reads*, not of *correlations*, and nobody had checked the difference.
  Measured by finding every place two record sets are indexed against each other:

  | Function | Correlation |
  |---|---|
  | `approval.go buildInbox` | `docByID` + `stepsByDoc` |
  | `approval.go PendingApprovalCount` | `docByID` + `stepsByDoc` |
  | `assigned.go buildAssigned` | `docByID` + `stepsByDoc` |

  **One shape, three times** — index Documents by id, group Steps by the derived parent Field. Well
  past the second-case trigger, and far narrower than a general join model.

  **Three things were miscounted as joins**, each a different shape. `buildSprint` reads two Machines
  but correlates neither: `ds_task_workload`'s `dimension: fld_assignee` already does that work
  declaratively, and reading two Machines is not the same as joining them. `buildCalendarWeek` groups
  by a *Field value*, which is §7.3 Dimension. And `projectNames` / `savedSignatureImages` /
  `PersonNames` are id→scalar label lookups, a third and simpler shape. **Fifth time in this session a
  carried number failed on measurement**, and the first time it made the work smaller.

  **007 §7.5 settles the design rather than leaving it to taste**, which is why the re-read came before
  planning. "Relations should reuse existing Machine reference semantics rather than inventing a second
  relationship identity" — the declared relation Field is the grounding, and
  `domain.FindChildCollections` already answers which Machines point where. And "a relation may be
  compiled to a join, semi-join, **lookup**, or other physical strategy" **explicitly sanctions two
  queries plus correlation**, so choosing it is a compilation decision rather than a shortcut. At 23
  Steps and 13 Documents it is almost certainly the faster one.

  **So the criterion is not "fewer queries" but "less Go".** A different test from Stage 1's, and the
  honest one here: what a Relation removes is three hand-written correlations, which is also what holds
  `documentApprovalFieldCoupling` at 11.

  **Not closed by it, stated up front**: `buildInbox` also reads `mch_activity` for submitters (the
  lookup shape, not a join), and the four "legitimately reads everything" sites are untouched.


  **A side-finding from the same re-read, recorded because nobody planned it.** 007 §16 lists Data
  IR's components as `Source, Projection, Filter, Relation, GroupBy, Measure, Sort, Parameters,
  Security Scope`. `domain.Dataset` now satisfies **seven of the nine** — everything but Projection
  and Relation — having arrived there one forcing case at a time rather than by anyone building
  toward §16. The claim matrix still marks Data IR **PROPOSED**, and that is still the right label:
  nothing *lowers* into an IR, the Dataset is consumed directly. But the vocabulary converged on its
  own, which is worth knowing before someone proposes building Data IR from scratch — and it makes
  Projection (§7.6) and Relation (§7.5) the two named gaps rather than an open-ended list.

  **Shipped 2026-09-29: the drift gate over the derivation accessors themselves.**
  `TestEveryDerivationIsOwedBySomeRole` guards the `Derivation*` constants against `Answers`. What
  nothing guards is the other direction — **a new accessor added to `internal/domain/actioneffect.go`
  or `internal/action/fields.go` that never reaches `Explain` becomes an inference the runtime makes
  and nothing can explain, silently.** That is the two-lists-that-drift shape this repo has already
  paid for three times (the `checkDocs` mirrors, the closed-registry members, `Answers` itself).

  **The population was measured before designing the gate, and the measurement says a naive gate must
  not be built.** Of 16 exported accessors across those two files, 12 are reached by `explain.go` and
  4 are not — but the four are four *different* things, so "every accessor must appear in `Explain`"
  would be right about one and a half of them:

  | Accessor | Real callers | Verdict |
  |---|---|---|
  | `ActionTargets` | 2 | **A real gap.** Its own doc comment says "the check is a derivation rather than a list" — the derived status move, read from `transitions[action].to`, explained by nothing |
  | `EffectFor` | 2 | **A judgement call**, flagged rather than decided here. It reads `actions:` verbatim, so it is arguably a read and not an inference — but `TestMachineFixturesPassProductionValidation` found *five* fixtures declaring `actions:` over Fields their Machine did not have, so "which Fields does `decide` write" is demonstrably a question that goes wrong quietly |
  | `FlowTemplateRowFields` | 1 | **Legitimately absent.** A remapping of `flow_template_step:`, which is already explained; explaining it again would be 001 #8 |
  | `OpenValue` | **0** | **Dead, and a trap** — see below |

  A ~50% false-or-arguable rate is precisely the shape of the static fixture-discovery gate that was
  built, measured at 40 false findings across 10 packages, and deleted. So the gate cannot ask "is this
  called by `explain.go`". It has to ask "is there a derivation the runtime trusts that no `Resolution`
  reports", and that needs judgement — which this repo already has a pattern for: a **closed map with a
  stated reason per entry, failing on a stale one**, exactly as `readPathWriters` and
  `applicationSubScreens` do. The judgement is recorded once and reviewed, never embedded in the sweep.

  **One finding is worth fixing whether or not the gate gets built.** `domain.Machine.OpenValue()` has
  **zero callers, tests included**. It is not merely dead: it reads `sequencing.open_value`, which
  `action.EngineFields.Open`'s own doc comment names as *the wrong source* for this question ("it
  belongs to ordering, and a Machine that orders nothing still has open records"). `action.openValueFor`
  reads `transitions[action].from` instead, deliberately. So what is sitting there is an unused accessor
  that would hand its next caller the source the engine explicitly rejected — a trap rather than
  clutter, and the argument is for deleting it rather than documenting it.

  Order held: the real findings were fixed first, *then* the gate — the repo's standing rule, and here
  it also kept the allowlist from being born pre-loaded with known debt.

  **What shipped.** `Machine.OpenValue()` deleted (the full suite stayed green without it, which is the
  proof it was dead). `DerivationStatusTargets` and `DerivationActionWrites` added, owed by the `step`
  role — the second `Optional`, since an Action whose whole effect is the status move its own
  transitions declare writes no companion Fields and two installed Machines are exactly that. The
  installed corpus went `resolved=13` → `resolved=15` in all three Workspaces.
  `TestEveryDerivationAccessorIsExplainedOrExcused` scans both files by AST and requires every exported
  accessor taking or receiving a Machine to be reached by `explain.go` or carry a reason in
  `unexplainedDerivationAccessors`.

  **`EffectFor` was the one judgement call this entry left open, and it is decided: explained.** The
  argument for leaving it out — it reads `actions:` verbatim rather than inferring — is real, but every
  *other* verbatim block on this surface is explained (`signature_placement`, `signature_store`, both
  flow-template blocks), so excluding this one would have been the inconsistency rather than the
  principle.

  **Three things this build got wrong and the gates caught**, worth recording because two were caught by
  the gates being built:

  - `TestEveryDerivationIsOwedBySomeRole` failed the moment the two constants were added — complaining
    that `status_targets` and `action_writes` were "not a `domain.Derivation*` constant", about two
    constants that are exactly that. Its list was hand-maintained: **a gate against drift keeping its
    own third copy of the list.** Now read out of `resolution.go` by AST.
  - The accessor gate rejected its own first allowlist on its first run: `ReferenceFieldTo` was excused
    as "a generic lookup", which it is — but `explain.go` calls it to resolve `step.parent`, so the
    excuse was false about the only thing the gate asks. The stale-entry arm earned its keep before the
    gate was committed.
  - Mutation-proved three ways after that: a new unexplained accessor, an entry naming a function that
    does not exist, and removing a legitimate entry. All three fail with the intended message.

  **Finding 2 — the identity-filter trigger was met six cases ago, and two comments in one file said
  so and denied it simultaneously.** Seven exported `internal/composition` functions take a viewing
  identity and filter records in Go after reading them all: `ApprovalInbox`, `PendingApprovalCount`,
  `AssignedToMe`, `MyNotifications`, `UnreadNotificationCount`, `PersonalTasks`,
  `ReviewStepForDocument`. `PersonalTasks`' doc comment read *"neither has a second case yet"* while
  `MyNotifications`', forty lines above it, read *"the same 'filter by identity in Go' shape
  PersonalTasks/PendingApprovalCount already are"*. CLAUDE.md's step 3 fires on the **second** case;
  this is the seventh. Corrected in place (`internal/composition/pages.go`), where the reading also
  found the comment **detached** — it sat above `MyNotifications`' comment with no blank line, so godoc
  fused the two and `PersonalTasks` had no documentation at all, having its own refutation rendered as
  someone else's.

  Ranked **third**, after Finding 1 rather than before it, and the reason is not scheduling: a
  per-request filter is another inference that will need explaining, so building it first adds to
  exactly the debt Finding 1 is about. **Scope discipline**: what is earned is a `where:` that can name
  the viewing identity — *not* 007 §8's Query Model, which the decomposition audit's §7 explicitly
  refuses ("Jangan bangun Query Model penuh (007 §8) sekarang"), and which measured data volume does not
  pressure either (largest Machine: 28 records). `case-03`'s Stage 0 deferral stays **correct on its own
  terms** — the badge really cannot be a Dataset, because it filters on `behavior.CanAct`, which reads a
  *sibling* record. What changed is that six consumers appeared that need no sibling. The date half
  (`lt`/`gt` in `internal/expression`, 2 ops today) has about two cases and stays parked; B5 still
  refuses it.

  **Superseded 2026-09-29 by measurement, and the correction is about arithmetic rather than filters.**
  The paragraph above is kept as written because the reasoning it records is sound and the conclusion it
  draws is not. "Seven cases" is right about the **coupling shape** — seven exported functions do filter
  by identity in Go — and wrong as a **forcing case**, because the number was never checked against the
  primitive it was being used to justify. Checked site by site: **none of the seven is closed by an
  identity-aware `where:` alone.** Four (`ApprovalInbox`, `AssignedToMe`, `MyNotifications`,
  `ReviewStepForDocument`) need record *selection*, and a Dataset produces numbers.
  `PendingApprovalCount` reads a *sibling* through `behavior.CanAct` and can never be a Dataset at all —
  which the paragraph above already says, without noticing it removes a case from its own count.
  `UnreadNotificationCount` and `PersonalTasks` need a second predicate, and `Measure.Where` is a single
  `expression.Comparison` with no conjunction.

  So the sentinel on its own would have shipped with **zero consumers** — the premature declaration B5
  refuses, and the rule this very entry invokes. **A count of sites sharing a shape is not a count of
  cases a primitive would close**, and this file conflated the two across three separate answers before
  anyone checked. `internal/composition/pages.go`'s own comment carried the same error and is corrected
  in place.

  What the measurement replaced it with is the "(c)" entry below.

  - **Declared record selection — `select: records` (007 §7.7 Filter, §7.8 Sort, §7.9 Pagination).**
    Chosen 2026-09-29 over two narrower options after comparing what each makes writable in YAML.
    **(a)** an identity sentinel alone adds one token, closes nothing, and would be rejected by this
    repo's own activation gate (`TestClosedRegistryMembersAreActivatedByMetadata`: "a capability no
    manifest can start is one only Go can reach, which is 001 #3 inverted"). **(b)** sentinel plus
    conjunction closes one case, the notification badge, and still returns only numbers — `views:`
    changes by not one line, because a number cannot be rendered as a list. **(c)** adds
    `select: records`, `sort:` and `limit:`, and is the only one of the three where a View gains a
    `dataset:` line — the Experience Plane consuming the Data Plane, which is what 007 §4.3 asks for.

    **And it is the only one that changes what is actually read.** Under (a) and (b) the runtime still
    loads the whole Machine and discards in Go, so §28 invariant 4, §4.4 and §20's "never: query all
    data → render → trim" all stay violated while the metadata merely gets more expressive.

    **Per-site classification, which is what the plan is scoped to** (24 real read sites in
    `internal/composition`, excluding the Loader's own 4 plumbing calls):

    | | Sites | Needs |
    |---|---|---|
    | Single-Machine `select: records` | **8** | `where`/`sort`/`limit`, `$current_user`, `$parameters.x` |
    | Join / sibling correlation | **10** | 007 §7.5 Relation — a separate primitive, **not** in this plan |
    | Legitimately reads everything | 4 | nothing (`projectNames`, `TeamCapacity`) |
    | Date comparison | 2 | `lt`/`gt`, still ~2 cases, still parked |

    The ten join sites are the approval screens — the ones carrying the most Go. **`select: records`
    does not close them**, and saying so here is the point: the earlier framing of "33 read sites" as
    the prize was a third unchecked number.

    **Design inputs 007 hands over rather than leaving to taste**: §9.2 names the expression context
    (`record`, `old`, `current_user`, `today`, `now`, `parameters`) and requires that access outside it
    **fail closed** — so an unknown `$sentinel` is a load error, not a literal. §9 requires one shared
    expression model across constraints, event conditions, view filters, Dataset filters and action
    values, so conjunction extends `expression.Comparison` rather than forking it. §7.7 keeps the
    existing `field`/`op`/`value` form valid as syntax sugar, so nothing already declared changes.
    §9.3 sanctions in-process evaluation; §8.3's preference for SQL is noted and not followed, because
    the largest Machine holds 28 records.

    **What stays refused**: §8.2 normalization, §8.3 general pushdown, §8.4 cost classes. The
    decomposition audit's §7 refused those for want of a forcing condition and that is still true.

    **Step 0 — baseline, measured 2026-09-29 before any code. It corrected the case for (c) twice,
    and the corrections matter more than the numbers.**

    *Volume* (`make threshold`, which seeds its own rows since this app has no users):

    | records | whole-Machine read | |
    |---|---|---|
    | 100 | 900µs | within budget |
    | 1,000 | 4.2ms | within budget |
    | 10,000 | 38ms | within budget |
    | 50,000 | **222ms** | over the 100ms interactive budget |
    | 100,000 | 667ms | over |

    The largest real Machine holds **30 records**. So the volume forcing condition is roughly
    **1,700× away**, and **(c) must not be argued for on performance grounds.** Its case is §20,
    §4.4 and §28 as *architectural* invariants and the metadata-basis 001 #3 asks for — not speed.
    Saying otherwise would be a claim this baseline refutes.

    *Per-route reads* (the existing diagnostic, real admin session, `repeated=0` everywhere):
    `/home` 8, `/dashboard` 9, `/approval-inbox` 8, `/my-tasks` 6, `/notifications` 5,
    `/api/notifications/unread-count` 4, `/api/approval-inbox/pending-count` 5.

    **Correction 1 — five of the eight "expressible" sites are already filtered in SQL.**
    `MyNotifications`, `UnreadNotificationCount`, `ReviewStepForDocument`, `ReviewDocument`'s
    siblings and `BlockingReasonsForMemberRemoval` all go through `ListRecordsBy`, which the
    diagnostic confirms (`mch_notification by fld_recipient`). Migrating them to `select: records`
    moves the *declaration* from Go into YAML — which is the point of (c) — but changes **nothing**
    about what is read. Only about three whole-Machine sites would actually see a read shrink.

    **Correction 2 — `/dashboard` would get *worse* before it gets better.** Its single
    `mch_document` read serves two consumers through the Loader memo: `buildDashboard` filters it to
    `in_review` for the Pending list, and `AggregateDataset(ds_document_by_status)` needs all of it.
    Declaring the Pending list as a filtered Dataset produces **two** reads where there is now one,
    unless the aggregate pushes its filter down as well. So a migration that looks like an
    improvement per-site can raise a page's read count, and the GET sweep would not catch it — the
    sweep holds `queries == reads` and `repeated == 0`, and two legitimately different reads violate
    neither.

    **What Step 0 therefore changes in the plan**: the first migration target is **not**
    `MyNotifications` (already filtered, nothing to prove) but a whole-Machine site whose read
    actually drops; `/dashboard` is explicitly *not* first, because it needs the aggregate story
    settled in the same slice; and the per-route numbers above are the before-figures every later
    claim is checked against.

    **Stage 1, broken into five committable slices.** Ordering follows from Step 0 rather than from
    convenience: the first target must be a whole-Machine site whose retrieval actually narrows, and it
    must not drag the expression work in with it.

    - **Slice A — `select: records` + `sort:` + `limit:`, no expression work at all.** Target:
      `recentEvents`, chosen because it has **no filter whatsoever** — it reads every `mch_activity`
      row, sorts by `CreatedAt` in Go and truncates. So it proves the retrieval-bounding primitive in
      isolation; if the shape is wrong, it is wrong in one place with nothing else mixed in. Its
      checkable outcome is a literal leaving Go: `dashboardActivityLimit = 10` and the `activityLimit`
      parameter both disappear.

      Three design decisions the target forced, and one left open:

      1. **`sort:` must accept record-level columns** (`created_at`, `updated_at`, `sort_order`), not
         only Field ids — `mch_activity` declares no timestamp Field, so without this its Dataset
         cannot be written at all. Closed set, validated.
      2. **`limit:` required for `select: records`**, grounded in §7.9 ("preserve the current
         capability that prevents unbounded list retrieval"). *Open to reversal*: it means a site
         wanting "all of mine" must name an arbitrary number. A safe default is easier to loosen later
         than to impose afterwards.
      3. **The Loader memo must key on the Dataset id, not the Machine id.** A correctness trap rather
         than a style point: a `select: records` result is a *different set for the same Machine*, so
         memoising it under `machineID` would poison the whole-Machine memo and hand `/dashboard` ten
         activity rows where it needs all of them. Its mutation test is the most important one in the
         slice, because that bug is silent and breaks a different screen than the one being changed.
      4. The Store signature stays minimal (`ListRecordsSelect(ctx, machineID, sort, limit)`) and Slice
         C widens it. Stated as a choice rather than left implicit: adding a predicate parameter now
         would be a parameter with no caller.

    - **Slice B — the expression context, fail-closed.** `$current_user`, `$parameters.<name>`, a
      closed `KnownContextValues`. The content is the *refusal*: an unknown `$sentinel` is a load
      error, never a literal (§9.2). `today`/`now` are named by §9.2 and deliberately not built — two
      cases, B5.
    - **Slice C — conjunction.** `expression.Predicate{All []Comparison}` wraps the existing leaf, so
      the four current users of `Comparison` change by not one line and a single comparison still
      parses as it does today (§7.7 keeps the old form valid as syntax sugar).
    - **Slice D — second migration plus the ratchet.** `PersonalTasks` (`fld_assignee ==
      $current_user` **and** `fld_status != done`), the first site needing B and C together. The
      whole-Machine-read ratchet lands *here*, not after Slice A: a ratchet locks in a pattern, and one
      migration is not yet a pattern.
    - **Slice E — `/inference` and the documentation.** A `where:` resolving `$current_user` is a
      runtime decision affecting **data access**, the first category 001 #6 names, so it belongs on the
      diagnostics surface — or this work adds back the debt the last three slices just paid off.

    **Shipped 2026-09-29, in three commits.** Slice A (`select: records` + `sort:` + `limit:`,
    `recentEvents`), conjunction on its own, then the merged B/C/D (expression context, `where:`,
    `PersonalTasks`, two gates). Ratchet 19 → 17; `dashboardActivityLimit`, `activityFeedLimit` and
    the Go-side assignee filter all left Go.

    **B, C and D merged, and the reason is the rule this entry used against option (a).** Landing the
    expression context alone would have added a registry member no manifest names — exactly what
    `TestClosedRegistryMembersAreActivatedByMetadata` exists to reject. Conjunction went separately
    *because* it adds no member, which is the same test applied in the other direction.

    **Four claims of mine failed on contact, and the fourth changed the slice.** This entry predicted
    `PersonalTasks` would be the first consumer of conjunction, needing `assignee == me AND status !=
    done`. The code says otherwise: `done` is a **bucket**, not a filter — completed Tasks still
    render in their own section, so pushing that predicate down would empty it. Only the identity
    predicate moved, and **conjunction ships with no consumer**. Recorded rather than papered over by
    inventing one; the next candidate is the aggregate side (`Measure.Where` gaining the same
    Predicate and sentinel resolution), which `UnreadNotificationCount` would use.

    **Two measured details worth keeping.** `op: not_equals` compiles to `IS DISTINCT FROM`, not
    `!=`: a missing JSONB path is NULL, `NULL != 'done'` drops the row, and `expression.Comparison`
    in Go keeps it — different queries, differing only on records that never declared the Field. And
    `EXPLAIN` shows the scan still touching every row: a `LIMIT` without a matching index saves the
    **decode**, not the scan, which sharpens Slice A's own commit message. The index audit measured
    that decode is the expensive half anyway, and its refusal of new indexes was explicitly
    conditional on the read-all-then-filter pattern this work removes — so 007 §21.4 becomes live at
    the 50,000-row threshold, not now.

    **The existing read diagnostic cannot see this improvement, and that must be stated rather than
    discovered.** It counts *statements*, not rows: `mch_activity` with `LIMIT 10` is still one
    statement, so `queries`/`reads` will not move. Reading that as "no improvement" would be wrong, and
    it is the same shape as the N+1 blind spot already recorded against that sweep. Proof is structural
    and behavioural instead — a Store test against the real database asserting 10 rows come back from
    30, and a mutation removing `limit:` from the YAML to confirm the declaration is what bounded it.

    **Gates to build with it**, planned rather than discovered later:
    1. `KnownContextValues` joins `TestClosedRegistryMembersAreAcceptedByTheLoader` /
       `...AreActivatedByMetadata`. This is the gate that would have rejected option (a), and it is
       the reason to write it *with* the primitive rather than after.
    2. **A whole-Machine-read ratchet**, shrink-only, in the shape `documentApprovalCoupling` and
       `projectionRatchet` already use. Baseline measured 2026-09-29: **19** `Loader.ListRecords`
       calls across 4 files (`approval.go` 5, `assigned.go` 3, `pages.go` 9, `review.go` 2), beside 5
       already-filtered `ListRecordsBy` calls. It measures the thing (c) exists to fix, a lower count
       fails as well as a higher one, and — like every ratchet here — it can only be written *after*
       the primitive exists, never before.
    3. The migrated routes stay under the existing GET sweep's two invariants (`queries == reads`,
       `repeated == 0`), which needs no new gate, only the discipline of not adding a ratchet entry.
    4. `internal/installer`'s `FullMachineCheckDoc` mirror — not a new gate, but
       `TestCheckDocsMirrorMetadatasOwnKeys` **will** fire when `select:`/`where:`/`sort:`/`limit:`
       reach `machineDoc`, and it has caught this exact class twice before.

  **Finding 3 — a lesson recorded on 2026-09-20 was re-failed on 2026-09-29. Ranked second, because it
  is an hour's work and one of the two is a debt owed.** `development-history.md:3038` already wrote,
  nine days earlier, that verifying a deploy means matching `bin/server` against `/proc/$PID/exe` **with
  no `(deleted)` marker**, citing `audits/2026-09-20-written-claim-vs-actual-state.md`. On 2026-09-29
  `cd340fd`'s own commit message claimed *"The service was restarted to confirm the binary still boots"*
  — **which was untrue when written**: only `systemctl is-active` had been run, and the owner's question
  ("sudah rebuild restart?") revealed the running process was on a **deleted, stale binary**, meaning
  production had been serving pre-`Parse`/`Validate`-change code. It was rebuilt, restarted and verified
  by `cmp bin/server /proc/$PID/exe` plus a five-route smoke test the same day; `cd340fd`'s message is
  pushed and stays wrong, and this paragraph is the correction rather than a rewritten history. The
  general form is this repo's own rule (CLAUDE.md: *"If you find yourself re-explaining the same
  architectural rule … twice, consider whether it should be … a conformance test instead"*): the deploy
  check is prose, and prose got skimmed. It should be a `make` target that builds, restarts, `cmp`s and
  smoke-tests, so it cannot be claimed without being run.

  **Finding 4 — `development-history.md` states it is the authoritative record and stopped 2026-09-22.**
  Seven days and roughly twelve commits missing — Stages D, E1, E2, Workspace isolation, the
  dokter-kecil install. Exactly the class `audits/2026-09-20-written-claim-vs-actual-state.md` exists to
  catch, in the file that carries Finding 3's own lesson. The open question (asked, unanswered) is
  whether it is still the living log or has been superseded by this file plus `audits/*`; **a staleness
  header now says which parts are current rather than leaving the claim standing**, so the file no
  longer overstates itself whichever way that is answered.

  **Finding 5 — still no holistic fitness function**, and it waits on Finding 1 structurally. Every gate
  in `internal/conformance` is atomic (one dimension); the product thesis — *"a new Machine added to a
  manifest renders on every generic screen with no Go change"* — is proven by none of them
  (`audits/2026-09-19-decomposition-maturity-audit.md` §5). Part of it landed 2026-09-28
  (`TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames`). The full version has to assert on
  *resolved* results, which is the surface Finding 1 builds, so it is fourth by dependency and not by
  preference.

  **What this kajian deliberately does not propose**, listed so it cannot be read as "generalize
  everything" — each parked with a written reason that still holds: the full Query Model, Data IR, UI IR
  and the CEP (007 §34's own instruction, *"Build it against real forcing cases, not speculatively"*;
  `internal/ir`, `internal/planner` and `internal/registry` are deliberately `doc.go` boundaries and
  nothing else); metadata versioning and change classification (no case); hot reload (a deliberate
  triggered deferral); `requires_role:` on a navigation item and a `color` card_fields role — the last
  `projectionRatchet` entry — (both waiting on a second case); the write-side Binding primitive and a
  step's label (trigger is a third bespoke write screen, and a second case, respectively); signature
  placement as a View type (`Q3` justifies it staying page-internal); and generalizing `decide.go`
  (already assessed as failing B4, in writing).

  **One thing in this ordering is judgement, not measurement, and is flagged as such**: Finding 1 changes
  nothing a user sees, while Finding 2 does. It is still ranked first because it is the safety clause on
  the mechanism this runtime now leans on hardest — but that is a call, and the measurements above are
  what to re-argue it against.

- **Installing a template into a Workspace that already uses its ids -- ~~planned~~ shipped 2026-09-28**
  (owner, 2026-09-28: *"bukannya harusnya bisa antisipasi jika tabrakan nama aplikasi bukan? ...
  komponen composable harusnya siap untuk ini"*). Both halves of that were fair, and they needed
  different answers.

  **What shipped.** `internal/installer` -- `Templates` (the library as data), `PlanInstall` (every
  rename, addition and refusal, decided before anything is written) and `Install` (copy, rewrite,
  append to the manifest, load-verify, roll back). `GET/POST /install-application` behind the same
  workspace-admin gate as `/new-application`, a `domain.RuntimeScreens` entry so the page reads its own
  route and heading by id, `rendering.InstallApplicationPage`, one link in Workspace Home's existing
  "Add an application" section, and `cfg.TemplatePath` (`TEMPLATE_PATH`, default `metadata`).

  The write primitives moved out of `internal/aiassist` rather than being copied: `WriteSet`,
  `WriteFileStrict`, `AppendBlockListItem`, `RefuseIfExists` and the strict-decode targets are now
  `internal/installer`'s, and `aiassist` imports them. That package's charter is "proposes Runtime
  Metadata and calls an external AI API"; a hand-written template install is neither, and forking the
  one piece of code whose whole job is not corrupting metadata on disk would have been the worse of the
  two options. `internal/installer` has its own `TestEveryPackageHasARule` entry.

  **What it renames, and what it refuses.** Machine ids and an Application's own id are renamed on
  collision -- `mch_document` becomes `mch_document_approval`, the Application's own last name segment,
  falling through to `_2`/`_3` -- and that is safe only because Stage A and the role-resolution slice
  had already removed every Go-side meaning of both. A colliding **navigation id**, **navigation route**
  or **Dataset id** is refused with the id named, because `routeByID("nav_...")`,
  `Workspace.ApplicationForRoute` and `internal/composition`'s five `ds_*` constants still name those
  from Go; renaming one would recreate exactly the coupling the two slices before this deleted. The
  route refusal earns its keep twice: it makes two copies of one Application in a single Workspace
  impossible, which is the state that would otherwise expose the submit wizard's own ambiguity
  (`/documents/new` belongs to no Application -- `domain.MachineInWorkflowRole`).

  **Byte-identical when nothing collides**, asserted by comparing bytes, because that is what keeps
  "install then diverge" readable as a diff; a copy that did rename carries a provenance header naming
  its template, the date and every rename, since the prose below it still names the original ids. The
  library is only ever read -- also asserted by comparing bytes, since writing into it is the incident
  that motivated Workspace isolation. A runtime-level Machine the template implies and the Workspace
  lacks is added as a *reference* (`mch_notification` when the template declares `send_notification`),
  derived from the template's own metadata rather than listed per template.

  **Two live bugs fell out of it**, both of the "two lists that drift" shape and both now gated by
  `TestCheckDocsMirrorMetadatasOwnKeys`: `installer`'s `FullMachineCheckDoc` was missing
  `blocks_member_removal` (added to the real loader 2026-09-27), so copying `mch_approval_step` failed
  its own strict re-parse -- a file the loader is perfectly happy with. Writing the gate then found
  `WorkspaceManifestCheckDoc` missing `suggested_applications`, which `metadata/workspaces/default.yaml`
  declares: **every AI publish into that Workspace would have failed and rolled back**, with an error
  about a key nothing was wrong with. Neither was reachable before something tried to write those files.

  **Verification.** `internal/web.TestInstallApplication_endToEndOverACollision` drives session, admin
  gate, CSRF, POST, install, reload hook and redirect against the real library.
  `installer.TestPlanInstall_againstTheRealDokterKecilManifest` asserted the plan against the actual
  Workspace the owner's question was about (one rename, four clean, `mch_notification` added) -- until
  the install actually happened on 2026-09-29 and that premise stopped being true; it is
  `TestPlanInstall_refusesASecondInstallOfTheSameApplication` now, see that entry.
  `TestInstall_rollsBackWhenTheResultWouldNotLoad` provokes a real unloadable result and confirms the
  tree is left as found. Live: the screen renders in `default`, where both templates are already
  installed, and correctly refuses both by naming every colliding id.

  **Not done live in `dokter-kecil`** at the time -- installing there needs a session in that
  Workspace, and the shared admin credential holds no membership in it. **Done 2026-09-29** on the
  owner's authorisation, through `PlanInstall`/`Install` directly rather than a session, which is where
  the one real gap in that route turned up: `Install` writes metadata, and the *role* it takes to use
  the Application is membership data the handler grants separately (`grantInstallerRole`). Skipping that
  step left the Application installed and answering "you have no role" to its only member. See the
  Stage E2 entry's own account.

  **The anticipation half is done** (same day): the assistant's system prompt now carries every
  Machine id already taken in the target Workspace, not only the ones an Application claims. It
  had been told about Application-claimed Machines all along -- which is why the owner's own
  collision test produced `mch_document_item` rather than a rejection -- but `mch_user`,
  `mch_activity` and `mch_notification` are claimed by none, so validation knew more than the
  assistant was told. Closing that asymmetry makes avoiding a collision by design rather than by
  luck.

  **The capability half is not, and there is no mechanism for it at all.** Installing a template
  is copying it (Workspace isolation, 2026-09-27), and the only automated installer is the
  assistant's own publish path. So a Workspace that already uses one of a template's ids simply
  cannot take it: "dokter-kecil" holds `mch_document` for its generated Document Tracking, and
  Document Approval's own `mch_document` collides -- within one Workspace, two Machines cannot
  share an id, and nothing offers to resolve it.

  **Resolution means renaming on install, and that became safe on 2026-09-27 -- reversing an
  argument made here the day before.** Prefixing ids was rejected then, and *as the default it
  stays rejected*: a renamed copy is no longer diffable against its template, which is what makes
  "install then diverge" legible, and ids leak into `records.machine_id`. What was also argued --
  that a rename risks a subtly broken copy, since a missed cross-reference would go unnoticed --
  no longer holds. Every place a Machine id is referenced is load-validated: an Application's own
  `machines:` (`validateApplicationClaims`), `summary_machine:`, relation targets
  (`validateRelationTargets`), and nav ids (`validateNavigationIDsAreUnique`). A half-completed
  rename **cannot load**, and since `aiassist.Write` load-verifies and rolls back, what cannot
  load cannot be published. The machine now catches exactly the class the argument was about.

  So: renaming as the *default* remains wrong; renaming as *collision resolution*, only for ids
  actually taken in the target, is right and is now provably safe.

  **The plan as written before it was built**, kept because its reasoning is this entry's substance --
  step 3's "let the loader prove it" is the whole reason attempting a rewrite was reasonable, and step
  2's byte-identical-when-clean is the property the shipped version asserts by comparing bytes. Step 2's
  suffix landed as the Application's own last name segment (`mch_document_approval`) rather than the
  Workspace's, and step 3 gained one site it did not anticipate: `workflow:`'s own `roles:`, which only
  came to exist that morning.

  1. **An install operation that takes a rename map.** Today `aiassist.Write` only creates from a
     `GeneratedChange`. What is missing is "copy this template into this Workspace, renaming these
     ids" -- the same temp-file/strict-parse/rollback discipline, applied to files read from
     `metadata/` rather than built from a proposal.
  2. **Compute the map, do not ask for it.** The collision set is known before any write:
     `domain.Workspace.MachineIDs` against the template's declared ids. A taken id gets a suffix
     derived from the target Workspace or the Application, and everything else is copied verbatim
     -- so a Workspace with no collision gets a byte-identical copy and stays diffable, which is
     the property the default-rename would have destroyed for everyone.
  3. **Rewrite only the references the loader validates**, and let the loader prove it: `machines:`,
     `summary_machine:`, relation `machine:` targets, nav ids, and the Application id itself if it
     too is taken. Then load-verify. A missed reference fails the load and rolls the whole install
     back, which is why this is worth attempting at all.
  4. **Surface it.** There is no UI for installing a template even without collisions; the
     assistant is the only installer, and it is forbidden from proposing Document Approval. A
     plain "install this template here" path is the smaller half of this work but the reason any
     of it is visible to a person.

  **What the two blocking entries below contributed**, recorded because this entry was capped by both
  for two days: Stage A made a renamed *Application* engage the approval engine at all, and the
  role-resolution slice made a renamed *Machine* reachable by every handler, composition and Page. With
  both, step 3 is only about rewriting references inside the copied metadata -- which is what it says,
  and was not true when it was written.

- **Read what is already declared -- shipped 2026-09-28**, the first slice of the audit's read side and
  the one that needed no new primitive at all.

  **Why it was available.** After Stage B the remaining coupling was 126 references where Go names one
  of Document Approval's Field ids. Reading them showed most were not a missing capability: they name a
  Field metadata already declares somewhere else. `internal/composition/review.go`'s `canStillDecide`
  was the shape in miniature -- it already read `TransitionsFrom` and still passed
  `action.FieldStepDecision` as the Field to read them on.

  **What shipped.** Six accessors on `domain.Machine` (`ActorFieldFor`, `ActorGateFor`, `StatusField`,
  `ReferenceFieldTo`, `OrderField`, `OpenValue`) and `action.DeclaredFields`, which derives the whole
  set once per composition and is then read like any other value. Migrated across
  `internal/composition` (the four composed screens), `internal/execution` (the compositing) and the
  `internal/web` handlers. **No new metadata key, no new registry member** -- criterion B3, the existing
  primitive already fits.

  **Each id takes the declaration that answers its own question**, which is the discipline that makes
  this correct rather than merely convenient: the decision Field from the `transitions:` edges naming
  `decide`, "still open" from those edges' own `from:`, the parent from the relation, the order from
  `sequencing:`, the actor from the Permission, a Document's status Field from its own edges.
  `sequencing.state_field` names the same Field as the first of those and is **deliberately refused**:
  Sequencing declares how records are ordered and locked, so a Machine that orders nothing would have no
  decision Field for no reason. Mutation-proven by pointing it there and watching the inbox go empty.

  **126 -> 78 references across 11 files.** Proven by
  `composition.TestBuildInbox_overAMachineThatNamesItsFieldsDifferently`: the Approval Inbox composing a
  Machine whose Fields are `fld_putusan`/`fld_urutan`/`fld_petugas`/`fld_surat`, with none of Document
  Approval's own ids anywhere.

  **One prediction this entry got wrong, recorded rather than quietly fixed:** the plan said every
  existing test would pass unchanged. Three needed editing, and not because behaviour moved -- the
  composition fixtures declared *no Fields at all*, relying on the constants the code used to name. That
  is a fixture looser than the Machine that runs, the same class this repo already names elsewhere, and
  the fix was to declare what the manifest declares.

  **What is left is not a derivation away**, which is why this is a first slice rather than the whole
  read side: `internal/rendering` (22) and `internal/web/document.go` (34) are Binding (007 §11.3, no
  primitive at all) and Projection (§7.6, its own ratchet), and the signature store's own shape -- the
  four placement Fields, `fld_owner`/`fld_image` -- is declared nowhere, so removing it is a capability
  question rather than a reading one. That is the next slice's subject.

- **Resolve a Machine by the role its Application casts it in, not by its id -- shipped 2026-09-28**,
  the slice immediately after Stage A and the one that made the binding load-bearing rather than
  decorative.

  **Why it was needed.** Stage A stopped the engine *identifying* its Machines by name; it did not stop
  67 references across 18 files from *naming* them -- `machines[action.StepMachineID]`,
  `store.ListRecordsBy(ctx, action.StepMachineID, ...)`,
  `fmt.Sprintf("/machines/%s/...", action.DocumentMachineID)`. All legal under
  `TestNoBareMachineIDIdentityChecks`, since each names a Machine rather than claiming an identity --
  and all wrong the moment an install renames a colliding Machine, which is the entry above this one.
  A renamed copy would have loaded, bound, engaged, and then read nothing.

  **What shipped.** `domain.Workspace.MachineInWorkflowRole(engine, role, applicationID)` with an
  explicit precedence -- the request's Application when it binds the engine, else the Workspace's sole
  binding, else nil -- and `MachinesInWorkflowRole` for every binding. Reached through
  `internal/web.machineForStep`/`machineForDocument`/`approvalMachine(ctx, role)`,
  `internal/composition`'s own parameters (that plane takes what it needs rather than reading ctx, and
  still does), and `rendering.approvalMachineID(ctx, role)` for a `.templ`. The engine is named once
  per package rather than 67 times in total.

  The engine's cast grew to carry it: `mch_signature`, `mch_approval_flow_template` and
  `..._step` are now **optional** roles (`domain.WorkflowEngineSpec` splits required from optional), so
  an approval Application may cast neither reusable signatures nor saved flows and still run. The
  flow-template pair is optional *together* -- half a saved flow stores nothing, so a one-sided cast
  fails at load.

  **Three real defects fell out of the conversion, none of them the point of it:**

  1. `action.CanDelete` switched on the bare Machine id, and all three callers passed `machine.ID`. So
     *any* Workspace's own Machine named `mch_approval_step` had this engine's decided-step delete rule
     applied to its records -- and since that rule *refuses*, the leak blocked real deletions rather
     than allowing them. It survived 2026-09-27's conversion because `internal/action` is precisely the
     package `TestNoBareMachineIDIdentityChecks` excludes, so nothing was watching the one file where
     the shape still lived. It now takes the `*domain.Machine`, and
     `TestActionDoesNotSwitchOnItsOwnMachineIDs` closes the hole.
  2. `saveDefaultApprovalFlow` dereferenced `machines[action.TemplateMachineID].ID`, so a Workspace
     that installed Document Approval without the two flow-template Machines panicked on document
     submit. Optional roles make that state expressible, and the guard explicit.
  3. Workspace Home and the navigation badge -- both outside any Application -- assumed exactly one
     approval Application and one name for it. They now sum across every binding
     (`pendingApprovalTotal`). Workspace Home got cheaper as a side effect: it was composing the whole
     Inbox to take its length, and /home fell from 13 reads to 11.

  **Verification.** `internal/web.TestDecideStep_worksOverRenamedMachines` drives the real decide
  handler over `mch_surat`/`mch_langkah`/`mch_ttd` -- redirect, decision write, decided-by-name
  snapshot, the declared rollup on the parent, and the composited signed PDF -- names this repo has
  never used, and an assertion no test could make before. Mutation-proven: putting one lookup back to
  `action.DocumentMachineID` fails that test with "record not found" while every template-id test still
  passes, which is exactly the discrimination the slice exists for.
  `domain.TestMachineInWorkflowRole_precedence` covers all three branches plus the fall-through
  `/dashboard` depends on (an Application that binds *no* engine is not a scope, so it falls through
  rather than answering nil). Query budgets unmoved or better; the ratchet is empty.

  **The one named limitation.** Two approval Applications in one Workspace leave `/documents/new` and
  `POST /documents` ambiguous: those routes are named by no navigation item (`nav_new_approval` was
  deleted 2026-09-21 on owner instruction), so nothing resolves an Application for them, and they
  answer 404 rather than guessing. The honest fix is a route that says which Application it belongs to
  -- not a guess in the resolver -- and it is out of scope until a Workspace actually installs two.

- **Document Approval: closing the last three layers** (owner request, 2026-09-28: audit how
  metadata-based the real Document Approval is against 001-007, then plan it). The audit is
  `menata-app-document`'s `audits/2026-09-28-kajian-metadata-based-document-approval.md`; read it
  first, since it carries the per-layer evidence and the normative citations this entry only
  summarizes.

  **Where it actually stands.** Nine layers, five already declarative -- data shape, access, state
  model with its sequencing, reactions through the Service registry, aggregation and list
  presentation. The measured ratio is 411 lines of YAML against 3,366 lines of Application-specific
  Go and templ, but that ratio is **not** the target: 002 ("physical strategies remain
  runtime-owned") keeps Service implementations in Go, and 007 §12 governs how screens are
  *composed*, not what draws them. The three gaps below are what 001-007 actually asks for and this
  runtime does not yet have. Screens stay with `case-03-composability-checklist.md` Stage 3/4 and
  are deliberately not duplicated here.

  Both new gaps (A and C) sit in the **Domain Plane**, so all three stages below can run without
  touching the Experience-architecture decisions that make the screen work expensive -- the same
  property that let Stage 0/1 go first in that checklist.

  **Stage A -- declare the binding, stop matching a literal -- ~~planned~~ shipped 2026-09-28.**
  Smallest, and the only one that was *wrong* rather than merely absent. `action.IsDocument`/`IsStep`
  matched the literal `app_document_approval`, so the engine ran for exactly one Application name:
  the app installed and ran as-is, but could not be renamed, varied, or reused behind a second,
  differently-named approval Application. 001 #2 says application behavior belongs to the runtime,
  and an engine that only wakes for one name is behavior owned by one application. The two
  predicates were already the single seam every call site goes through (23 of them were converted to
  it on 2026-09-27), so this changed one function, not the call sites.

  **What shipped.** An Application declares the binding on itself:

  ```yaml
  workflow:
    engine: document_approval
    roles:
      document: mch_document
      step: mch_approval_step
  ```

  `domain.Workflow` + `KnownWorkflowEngines` (a closed registry mapping each engine to the roles it
  requires -- the same static-seam discipline `KnownActions`/`KnownServices` already follow),
  `metadata.validateWorkflowBinding` (unknown engine, missing role, misspelled role, a role naming a
  Machine this Application does not claim, one Machine in two roles -- all load-time errors, since
  every one of them is silent at runtime), `metadata.stampWorkflowRoles` (the per-Machine index over
  that one declaration, exactly as `stampApplicationIDs` is over `machines:`), and
  `domain.Machine.WorkflowEngine`/`WorkflowRole`, which is what the two predicates now read.
  `action.ApplicationID` is deleted -- nothing needs it, which is the proof the literal is gone.

  **Verified, in the order that matters.** The property first, through the real loader rather than a
  hand-stamped fixture: an Application called `app_persetujuan` over `mch_surat`/`mch_langkah`
  engages both predicates, a third Machine it claims but casts in no role engages neither
  (`TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames`). Every name in that test would have
  failed the day before. Then the control over the real installed Workspaces
  (`TestUnboundMachinesAreNotTheEngines`), where "dokter-kecil"'s own unbound `mch_document` -- the
  Machine whose arrival panicked two pages on 2026-09-27 -- is correctly nothing to the engine. Then
  three mutations, each watched to fail: reverting the predicates to matching literals (the property
  test *and* the new `TestNoApplicationIDIdentityChecks`), deleting the `workflow:` block from the
  installed Application (the activation gate, plus two live web tests), and removing the stamping
  call (five tests across three packages). Then live, against `localhost:4000` with a real admin
  session: a Document's detail page still reads `mch_approval_step by fld_document` and still renders
  its signature-placement block, which is `IsDocument` engaging through the declaration.

  **Two gates landed with it**, in the order this repo requires (build, migrate, *then* gate):
  `TestNoApplicationIDIdentityChecks`, which forbids comparing an id against an `"app_..."` literal
  and deliberately covers `internal/action` too, since that is the one place the shape ever actually
  appeared; and `domain.KnownWorkflowEngines` joining `TestClosedRegistryMembersAreActivatedByMetadata`,
  so an engine no manifest binds fails the build. That second one is the clearest case yet of what
  that gate is for: this engine ran for weeks with *no* metadata seam at all, selecting its own
  Machines by matching literals, so there was nothing a manifest could have named.

  **The one prediction this entry got wrong, recorded rather than quietly dropped:** the note below
  says a stage that works will make `documentApprovalCoupling` drop, and "if the number does not
  move, the stage did not." It did not move -- 67 across 18 files, unchanged -- and the stage was
  still correct. The ratchet counts references to the Machine-id *constants*, and every one of those
  66 is a lookup, a record query or a URL built from an id; the identity coupling Stage A removed was
  already funnelled through `IsDocument`/`IsStep`, which the ratchet cannot see by design (it is
  `TestNoBareMachineIDIdentityChecks` that holds that shape). So the ratchet measures *naming*
  coupling, not identity coupling, and Stage A was never going to move it. What will: resolving those
  lookups **by role** rather than by id -- `machines[action.StepMachineID]` becoming "this Workspace's
  Machine playing the step role", which the binding now makes possible for the first time. That is
  also the missing piece for the install-collision entry above: renaming a colliding `mch_document`
  on install produces a copy that loads, binds and engages, while all 66 of those lookups still ask
  for the old id. Two open questions came with it, neither mechanical: whether the engine's declared
  cast extends to `mch_signature`/`mch_approval_flow_template`/`..._step` (8 of the 67 name those, and
  they held no role), and where the resolver lives for `internal/composition` and `.templ` callers that
  hold a Loader or a ctx rather than a Machine map.

  **Both are now answered, and the slice shipped the same day -- see "Resolve a Machine by the role
  its Application casts it in" below.** The ratchet is empty.

  **Stage B -- an Action may declare what it writes -- ~~planned~~ shipped 2026-09-28.** 006 names Approve and Reject explicitly as
  Actions, and `decide` already exists here as a *name*: `domain.KnownActions` carries it,
  Permissions gate it (`prm_decide_own_step`), Transitions reference it (`action: decide`). What is
  missing is its *effect*. `decideStep` writes exactly two Fields -- `fld_decision` from the
  submitted value, `fld_decided_by_name` from the actor -- and then dispatches Events that are
  already declarative. So the primitive needed is narrow and nameable: **which Fields an Action
  writes, and from which of three sources** (submitted value, acting identity, now). Not an
  expression language. `revise` and the wizard's `continue-submit` have the identical shape and
  close with it rather than needing their own pass. This is what makes `decide` available to any
  Machine, which 007 §4.1 (Composition over Specialization) is the argument for.

  **What shipped.** `domain.ActionEffect`/`FieldWrite`/`KnownWriteSources`,
  `metadata.validateActionEffects`, `action.ApplyEffect`/`ApplyStatusMove`, and an `actions:` block on
  `metadata/approval_step.yaml` and `metadata/document.yaml` (both copies). Three sources, each with two
  real cases read off the call sites rather than from this entry's own sketch: the submitted value, the
  acting identity's id (`fld_submitted_by`) and its display **name** (`fld_decided_by_name`, a snapshot);
  plus a declared literal, for a Field whose state model deliberately declares no person-performed edge.

  **`now` was in the sketch above and is not in the primitive.** Nothing in this repo writes a Field
  from the clock -- there is no `fld_decided_at` -- so shipping it would have been generalizing on zero
  cases. The day a Machine declares such a Field is the day it earns a line in `KnownWriteSources`.

  **The status move is derived, not declared twice**, which the sketch missed and the files make obvious:
  `trn_step_approve`/`trn_step_reject` already say field `fld_decision`, to `approved`/`rejected`, action
  `decide`. So `Machine.ActionField`/`ActionTargets` read it from there, and `writes:` carries only the
  companions. That also removed `submittedDecision`'s hardcoded approved/rejected pair -- those two
  strings were a copy of the declaration, and the reason any other Machine bound to this engine had to
  use the same two words.

  **Proven by `internal/web.TestDecideStep_writesTheFieldsTheMachineDeclares`**: the real decide route
  writing `fld_putusan` and `fld_diputus_oleh` on a Machine that declares them, plus an assertion that
  nothing wrote the template's own Field ids. Mutation-proven (put the companion write back as a literal
  and it fails). The existing decide/revise/wizard tests pass unchanged; the only test edit was the
  fixture gaining the `actions:` block the real manifest declares, the same way it already carried the
  rollup Event and the `roles:` arm.

  **What Stage B deliberately did not close, stated because the boundary is real:** the *read* side. The
  signed-PDF compositing picks approved steps by `fld_decision` (`action.decisionOf`, `StampFor`),
  `CanDeleteApprovalStep` reads it too, and `internal/composition` projects named Fields onto cards --
  129 references across 13 files. And one operation stays a literal in place: the wizard's "submit this
  draft" writes `in_review` through the `edit` Action, which every ordinary field change also uses, so
  declaring it as edit's effect would set `in_review` on every edit. It wants an Action of its own, and
  inventing one on a single case is what this repo's Method forbids.

  **The gate came after the migration, and corrected a claim this entry made.** The note below said
  Stages B and C would make `documentApprovalCoupling` drop; they would not have -- that map counts
  `action.*MachineID`, and Stage B removes *Field*-id coupling, which was uncounted. So a second frozen
  population now exists, `documentApprovalFieldCoupling` (129 references, 13 files),
  `TestDocumentApprovalFieldCouplingOnlyShrinks`, on the same shrink-only terms. Extending the regex is
  what makes the claim checkable rather than asserted.

  **Stage C -- the signature/PDF work becomes a named Service -- ~~planned~~ shipped 2026-09-28.** `KnownServices` is a closed
  registry with three members; PDF compositing is not one of them, so `signDocument` is called from
  flow code rather than named from `events:`. **Half this gap is not real and should not be
  worked**: the compositing itself (`action/composite.go`, `banner.go` -- 178 lines of binary PDF
  manipulation) is a physical strategy and stays Go, exactly as 002 intends. What lands is the
  *invocation*: a fourth member of a registry that already exists, so Stage B's Action can trigger
  it without knowing anything about PDFs. The existing checklist rates CAP-F22 the lowest-value of
  its three behaviours to generalize, and that stays true of the implementation -- this entry only
  separates naming it from rewriting it.

  **What shipped.** `domain.Composite` + `ServiceCompositeSignedDocument` in `KnownServices`,
  `metadata.validateComposite` (same-file half) and `validateCompositeTargets` (the cross-Machine half,
  beside `validateRollupTargets`, which is the identical shape), and `mch_approval_step`'s own
  `evt_step_signed_document`. The operation moved from `internal/web/signing.go` into
  `internal/execution`, which is the I/O dispatcher by charter, and still calls `internal/action`'s
  compositing primitives unchanged -- 002 is the reason that half was never the work.

  `execution.Services{Store, Mailer, Files}` replaced the positional parameter list on
  `RunEvents`/`RunCreateEvents`: compositing needs a file store, and those have seven call sites plus
  one internal recursion, so every future Service needing something new would otherwise be a
  seven-file change to add an argument nothing else reads. The JSON API's own create/update twins gained
  the file store in the same pass rather than being left with a Service they silently could not run.

  Three Field ids stopped being hardcoded on the way: which relation reaches the parent, which Field
  holds the PDF, which Field receives the result. The ones that remain inside -- a step's decision,
  sequence, signature image and placement -- are the read side Stage B left, and are what
  `documentApprovalFieldCoupling` measures.

  **Verified, and one gate amended.** `TestDecideStep_signsDocumentAfterEveryApproval` and
  `TestDecideStep_worksOverRenamedMachines` pass with no change to their assertions -- the trigger moved,
  the behaviour did not -- and mutation-proving them (delete the Event from the fixture that mirrors the
  manifest) makes both fail on the empty signed file, which is the assertion that was impossible while
  the call was hardcoded. `TestApprovalStepDeclaresItsSignedDocumentEvent` keeps the real manifest honest
  in the "absent, not malformed" family: without the Event, approvals stop producing a signed PDF while
  every screen keeps rendering.

  The amended gate is `documentApprovalFieldCoupling`, and the amendment is a message rather than a
  loosening. A *relocation* adds a file to the population and subtracts from another, and the gate's "a
  new file is not the way to pass" advice is exactly wrong for that -- so it now carries a total, and
  when the total falls it says "this looks like a relocation: add this entry and lower the one it moved
  out of". Every change to the population still fails until the map is updated. 129 across 13 files
  became 126 across 14.

  **Stage D -- the signature shape becomes a declaration -- shipped 2026-09-28.** This one was not in the
  original A/B/C plan; the audit named it as what those three would leave behind, in words worth
  quoting because they turned out to be the whole brief: *"the signature store's own shape (the four
  placement Fields, `fld_owner`/`fld_image` on `mch_signature`), which nothing declares, so removing
  those is a **capability question** rather than a reading one."*

  **That distinction is the point.** Stages A-C removed literals by *deriving* them from declarations
  that already existed -- the `transitions:` edges, the relation, `sequencing:`, the Permission, the
  Service's own trigger config. Nothing in metadata said which Field holds a signature image or which
  four numbers place a box on a page, so there was nothing to derive from: an Application could cast any
  Machine in the `step` role under any name (Stage A) and the approval would still fail to composite
  unless that Machine's Fields happened to be called `fld_signature_x`. Stage A's own promise was
  therefore only half-true until this landed.

  **What shipped.** Two Machine-level blocks beside `fields:`, following `sequencing:`'s precedent --
  `signature_placement:` (image, page, x, y, width) on the `step` Machine, `signature_store:` (owner,
  image) on the optional `signature` one. `domain.SignaturePlacement`/`SignatureStore`,
  `metadata.validateSignaturePlacement`/`validateSignatureStore` (each named Field must be the
  declaring Machine's own and hold the right type), `action.SignatureFields`/`StoreFields`, and
  `installer.FullMachineCheckDoc` extended in the same change -- `TestCheckDocsMirrorMetadatasOwnKeys`
  failed the moment it was not, which is the third time that gate has caught this exact class.

  **Two blocks, not one**, because they answer different questions for different Machines: where a box
  sits on *this* step, versus where a person's reusable image is kept. And *not* an extension of the
  `composite:` Event config, although that already carries `parent_field`/`source_field`/`target_field`
  -- the placement screen and the generic detail page read these Fields with no Event involved at all,
  and a screen reaching into a Service's trigger config to know how to draw a box is the wrong
  direction.

  **The write side came too, by owner decision.** `signatureplacement.templ`'s twelve
  `name={ action.Field... }` attributes now render ids Composition resolved (`rendering.PlacementFields`),
  so board 09 submits under whatever its Machine calls them. **What that does and does not settle** is
  stated in the deferral table's own Binding row rather than left implied: those particular names are
  declared; the *general* primitive -- metadata declaring a form's Fields, 007 §11.3 -- is still unbuilt,
  and its trigger is still a third bespoke write screen. A screen can still leave the ratchet with every
  binding hand-typed, and `documentsubmit.templ`'s four still are.

  **`StampFor` took the Machine too**, and that one is worth naming because no gate could see it:
  `documentApprovalFieldCoupling` excludes `internal/action` as the owner of those constants, so the
  function that actually reads the four coordinates named them itself, invisibly, right through Stages
  A-C. Only reading the code found it. `TestStampFor_overAMachineThatNamesItsPlacementDifferently` and
  `TestStampFor_undeclaredPlacementStampsNothing` are the cover, the second proving there is no silent
  fallback: a Machine declaring nothing stamps nothing even when the record does hold the template
  library's own ids.

  **Verified.** `documentApprovalFieldCoupling` went **78 across 11 files to 43 across 7**, with five
  files leaving entirely (`execution/composite.go`, `web/signing.go`, `web/approval.go`,
  `rendering/signatureplacement.templ`, and `review.go`/`detail.templ`/`placement.go` down to one each) --
  exactly the prediction the plan made, re-measured rather than asserted. Mutation-proved on four
  separate guards: putting one constant back into `review.go` fails the ratchet; naming a Field that
  does not exist, or one of the wrong type, fails `TestAppManifestLoads` with the message an author can
  act on; an unknown key *inside* the new block is refused by `decodeStrict`; and reverting
  `applyApprovalSignature` to the constant fails the new end-to-end assertion in
  `TestDecideStep_writesTheFieldsTheMachineDeclares`, which approves over `fld_gambar_ttd` and checks
  that nothing was written under `fld_signature_image`.

  **Four test fixtures were declaring less than the Machines they mirror**, and every one of them was
  found by this change rather than by a gate -- `approvalStepTestMachine`, `signatureTestMachine`,
  `stepMachineForTest` and the placement tests. That is the same finding as the derivation slice two
  changes earlier, and it has now happened twice: a fixture that omits a declaration does not fail, it
  passes against a Machine looser than the one that runs.

  **That gate now exists** (`TestFixturesMirrorTheRealMachines`, one copy in `internal/web` and one in
  `internal/composition`, over `domain.Machine.DeclaredRuleBlocks`). It compares **presence, not
  equality**, because a fixture is deliberately smaller and deliberately renamed in the tests that
  prove this runtime does not depend on the template library's own names -- what must not differ is
  which *rule* blocks exist. Structure and presentation (fields, views, card_fields, datasets) are
  excluded on purpose: forcing a reduction to copy the manifest would bury what its test is about.

  **It found four more drifts on its first run**, which is the argument for it:
  `approvalStepTestMachine` had no `sequencing:` -- so every `decideStep` test in `internal/web` was
  running with ordering switched off, and a step locked behind an earlier one would have decided
  cleanly; `signatureTestMachine` had no Permissions at all; `docMachineForTest` had no Events,
  Permissions or Action effects; and `stepMachineForTest`'s own doc comment **claimed** a rollup Event
  "is here" while the fixture declared none. A comment asserting a block that is not there is exactly
  the failure this gate is for.

  **Its limit, stated rather than discovered later**: the population is a named list per package, not
  a sweep, so a new fixture is uncovered until someone adds it -- the same posture `readPathWriters`
  and the ratchets take.

  **What is left, and what it is waiting for.** 34 of the remaining 43 are `internal/web/document.go`:
  the submit wizard reading its own form by Field id. That is Binding on the read side, and it is
  additionally blocked on `continue-submit` getting an Action of its own -- it shares the generic `edit`
  Action with every ordinary edit today, which is exactly why Stage B could not reach it (audit §6).
  The other nine are a step's label falling back to `fld_step_name`, a Document's own file and status
  Fields in two composed screens, and `documentsubmit.templ`'s four bindings.

  **That paragraph was half wrong, and Stage E1 below is the correction.** "Blocked on
  `continue-submit` getting an Action" is true of the wizard's *write effects* and was wrong as a
  reason to defer the file: counting it found **18 of the 34 derivable that day**, from declarations
  that already existed. The lesson is the one this file keeps relearning -- a deferral reason that
  covers *part* of a population reads as if it covers all of it, and only counting tells you which.

  **Document Approval is installed in `dokter-kecil`** (2026-09-29, owner-authorised). The collision
  resolved the way the plan test had predicted for two days: `mch_document` -> `mch_document_approval`,
  the other four Machine ids free, `mch_notification` added as a shared reference. Six files under
  `metadata/workspaces/dokter-kecil/`, the manifest six lines longer, the template library untouched.
  **That Workspace now holds two Machines that were both `mch_document` in their own sources**, under
  two ids, serving two Applications -- the isolation thesis on real data rather than in a fixture.
  `default` verified unaffected: its own `mch_document` records still resolve and its Approval Inbox
  still renders.

  **Two things went wrong on the way, and both are worth more than the install.**

  **1. The install left the Application unusable, and I reported it as complete.** I ran it through a
  throwaway command calling `TemplateByID` -> `PlanInstall` -> `Install`, and described that as "the
  same sequence /install-application runs". It is not: that handler then calls **`grantInstallerRole`**,
  because an Application *role* is membership data (`workspace_member_app_roles`), not metadata, and
  without one `requireApplicationAccess` answers 403 on the very first redirect. The owner hit exactly
  that -- "you have no role at document approval" -- which is the trap `grantInstallerRole`'s own doc
  comment says the AI publish path fell into on 2026-09-27. Fixed by granting the Workspace admin
  `approver`, the same default that handler uses (the Application's first declared role). **The lesson
  is about the claim, not the grant**: "runs the same sequence" was checkable and I did not check it.

  **2. Two installer tests had to change, and the property that replaced them is better.** Both asserted
  Document Approval *can* be installed into dokter-kecil; it now is, so a second install is refused --
  on five routes, five screen ids and one Dataset id, the three things the installer will not rename
  because Go still names them. `TestPlanInstall_refusesASecondInstallOfTheSameApplication` asserts that,
  **checking the reasons rather than a bare `!OK()`** (mutation-proved: making the route check permissive
  fails it). `TestInstall_intoTheRealDokterKecilWorkspace`, added hours earlier, was deleted rather than
  rebased onto a reconstructed pre-install state -- a fixture describing a world that no longer exists is
  the failure this repo keeps catching in stale prose, and `TestInstall_intoAWorkspaceWithACollision`
  already covers install-with-rename against a Workspace built to dokter-kecil's exact shape.

  **And the install exposed a gate that only ever watched one Workspace.**
  `TestNavigationRoutesAreRegistered` read `default.yaml` alone, so dokter-kecil's five new routes were
  checked by nothing. Widened to scan `metadata/workspaces/*.yaml`, the way the loader itself scans --
  and it immediately failed on `/machines/mch_document_item`, a per-Machine screen whose handler is
  registered under the `{machineID}` *pattern* rather than as a literal. That is precisely the decision
  the gate's own comment said it wanted someone to look at instead of papering over, and the answer is a
  **stronger** check: for a `/machines/<id>` route the id must be a Machine *that Workspace installs*,
  because a nav item pointing at an absent Machine is the real failure and a literal-handler check could
  never see it.

  **Three follow-ups, 2026-09-29, and the first one was thrown away on purpose.**

  **Discovering the fixture-gate population was built, measured and rejected.** The plan was a static
  gate parsing every `&domain.Machine{...}` in the repo's test files (68 of them), resolving the id
  where it could, and requiring any literal naming a real Machine to declare that Machine's rule
  blocks. Predicted: quiet except on real mirrors. Measured: **40 findings across 10 packages, and not
  one was a real mirror** -- every fixture the named list already covers passed it. All forty were
  narrow unit fixtures *borrowing* a real id: `internal/authorization`'s `stepMachine` calls itself
  `mch_approval_step` while taking its Permissions as a parameter; `internal/behavior`'s
  `projectMachine` calls itself `mch_project` to exercise one Constraint. Going green meant 40 sites of
  irrelevant declarations -- a gate that gets deleted rather than obeyed. **The distinction it needed is
  intent, which is exactly what the named list encodes.** Recorded in
  `composition.TestFixturesMirrorTheRealMachines`' own comment so the next person does not rebuild it,
  along with the one honest alternative: give unit fixtures ids no Workspace installs, a 40-site rename
  and a judgement call rather than an oversight. Owner chose to record rather than rename.

  **Nine of the eleven uncovered POST routes got behaviour coverage** (`writeroutes2_test.go`), each
  asserting the *stored* consequence rather than the response, because a rendered screen is what a
  silent no-op also produces. Six store writes mutation-proved in one batch; the two most interesting
  assertions are the ones about scope rather than success: `mark-all-read` must leave *another* person's
  notifications unread, and `switch-workspace/restore` must refuse a Workspace you do not administer --
  the route takes the Workspace id straight from the form, so that guard is its whole security.
  **Two remained uncovered**: `/new-application/message` and `/new-application/{session}/discard`,
  recorded here as needing "a stored assistant conversation, a fixture of a different kind". **That was
  overstated, and the correction is the start of the next slice** -- `createGeneratedSession` in
  `newapplication_test.go` already built exactly that fixture and already drove `publishNewApplication`
  twice. It had simply never been pointed at these routes. Covered 2026-09-29, below.

  **And the real dokter-kecil install now has its write half** (`TestInstall_intoTheRealDokterKecil-
  Workspace`). The plan half was already covered against the real manifest; nothing ran `Install` over a
  real Workspace's own colliding files. A plan is a map of intentions, an install is bytes: the rename
  has to reach every place the copied id appears, the copies have to land under the Workspace's own
  directory, and the result has to load or roll back. Asserted against dokter-kecil's actual metadata,
  copied into `t.TempDir()`, with the library snapshot unchanged and `git status` clean afterwards.
  **Actually installing Document Approval there is still the owner's call** -- it changes what that
  Workspace renders for its one member -- so it is not done.

  **The write side, measured for the first time** (2026-09-29). 14 of 29 POST routes were named by no
  test, and the one that mattered was `revise`: a declared-Action write path edited by `14c9223` -- the
  same commit that broke /review, by the same kind of edit (a literal Field id replaced with
  `machine.StatusField()`). The change was correct; nothing proved it, which is exactly where /review
  stood the day before.

  `TestPostRoutesRefuseUnauthenticatedAndUnCSRFed` sweeps all 37 POSTs, and **its first design was
  wrong in a way mutation-testing caught immediately**: discovering only `pr`/`ar` routes meant moving
  a POST to the *public* router did not fail it -- the route left the population instead. So the
  receiver is asserted rather than filtered on, and a public POST must be in one of two closed sets.
  The split between them is itself a finding: `/choose-workspace` is not a pre-session flow, it runs on
  login's half-identity and verifies it itself, so its allowlist entry carries a *checked* claim (it
  must redirect to /login without that cookie) rather than an excuse.

  **Live, and the limit is the same one the wizard had.** `revise` cannot be exercised through the
  running service from this session: `POST .../revise` returns 403 because `prm_revise_document`
  requires an approver or submitter role and the admin pseudo-identity holds none -- checked, not
  assumed. Production also holds no rejected Document to revise. So the live check confirms the route
  is wired and refusing correctly, and the behaviour is covered by the Postgres-backed tests through
  the real handler, mutation-proved.

  **What it deliberately does not do**: refusing correctly is not behaviour coverage. Eleven routes
  still have none. Behaviour tests were written for the three where a silent no-op is an authorization,
  credential or workflow failure -- `revise`, group role grants, the password change -- all three
  mutation-proved against a handler that renders and writes nothing.

  **And the fixture gate's stated limit bit immediately, which is the useful part.** Its population is
  a named list, and `documentTestMachine` was not on it -- so that fixture declared no Transitions, no
  Permissions and no Action effects. Consequence, found by the first test of `revise` rather than by a
  gate: with no Transitions the Machine has no `StatusField()`, so the derivation `reviseDocument` and
  `continueDocumentWizard` read their status Field through returned `""`, while the decide tests kept
  passing because the rollup Event's own config names `fld_status` literally. Two more stale claims
  fell out of fixing it: the fixture created its Document with **no status at all** (production's
  `create` effect always sets one), and `TestUpdateRecordForm_unrestrictedMachineAllowsAnyAuthenticated-
  User`'s own name and comment described mch_document as "declaring no edit Permission", which stopped
  being true on 2026-09-21 -- the fixture was what still said so. Renamed and corrected.

  **The blind spot that let it ship is now closed** (2026-09-28, `TestPerRecordGetRoutes`). The 404
  lived in a gap a gate already declared: `TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed` parses
  router.go precisely so a new route is covered without anyone remembering -- and then dropped every
  `{...}` path, saying why in place ("would need a seeded record this fixture does not create").
  **33 routes swept, 12 exercised by nothing at all**, including the one that broke.

  The new sweep seeds one record per installed Machine, built from each Machine's own declared Fields
  in dependency order rather than from a hand-written table, and requests all twelve -- **60 route
  instances across 12 patterns**. It asserts *both* directions, which is the part that matters: a
  route valid for this Machine renders, and a Machine cast in no workflow role still 404s. Only the
  first half would have been satisfied by "make /review always return 200"; the second is the branch
  the real bug hid behind. Role-gated routes ask `MachineInWorkflowRole` rather than naming
  `mch_document`. Mutation-proved three ways: restoring the original `nil` fails it, making /review
  always succeed fails it, and dropping a route from the case list fails it.

  **What the query invariants found, and what fixing them cost.** Owner chose to fix rather than
  ratchet, and all five were real:

  | route | before | cause |
  |---|---|---|
  | `/machines/{m}/records/{id}` | `repeated=5`, 19 queries | the detail page, its signature-placement embed and the PDF page count each built their *own* `composition.Loader`, so there was nothing to memoize against |
  | `/review` (Document) | `repeated=3` | same, plus resolving which Step to open on listed the steps `ReviewDocument` then listed again |
  | `/review` (Step) | `repeated=1` | one Document read twice |
  | `/workspace-members/{id}/edit` | `repeated=3` | **not a bug** -- see below |
  | `/workspace-groups/{id}` | `repeated=1` | `ListMembers` reaches `ListGroups` itself, so a screen that also lists Groups read them, and their grants, twice |

  Fixes: `composition.Loader` gained `Record` (a single-record memo -- it had `ListRecords` and
  `ListRecordsBy` but no per-id one, which is why three reads of one Document were invisible to it);
  one Loader per request on the detail and review routes instead of two or three; and the Group detail
  screen now reads through `Loader.Members`/`Groups`, which already feed `ListMembersFrom` a memoized
  Group list. All five routes are at `repeated=0`, and the detail page dropped from 19 queries to 15.

  **One of the five was the metric being wrong, and proving that mattered more than fixing it.**
  `/workspace-members/{id}/edit` reported `membership x2` -- but an admin editing another member
  legitimately reads two memberships, theirs and the viewer's, and `ReadLog.record` counts by target
  *name* with no subject. Rather than assume either way, the fixture was changed to edit a *different*
  member: the repeat persisted, which proved the metric under-specified rather than the screen wrong.
  `recordFor(target, subject)` now names the subject for the five per-subject reads, so two reads
  about two people stop reading as a repeat. A diagnostic that cries wolf is worse than one that says
  less.

  **A live 404 shipped in the middle of this sequence, and how it hid is the part worth keeping.**
  `14c9223` (the derivation slice) replaced `action.FieldStepDocument` with
  `action.DeclaredFields(stepMachine, nil).Parent` inside `composition.ReviewStepForDocument`. That
  Field is derived by *comparing* the step Machine's relations against the document Machine's id, so
  nil meant no id to match, an empty Field name, a `ListRecordsBy` filtering on `""`, and zero rows --
  every Document looked like a Document with no steps. **Which is a legitimate state with a legitimate
  answer: 404.** So the screen failed in a shape it is supposed to have. Nothing logged an error,
  nothing panicked, no test failed, and the whole signal was three 404s in an access log the owner
  happened to hit.
  Two things let it through, both fixable and both fixed: `ReviewStepForDocument` had **no test at
  all** (it needs a Loader, so it never got a unit test and nobody wrote the integration one), and the
  derivation was called with nil for a Machine the caller had in hand. `TestShowReviewDocument_*`
  covers both arms of the route now -- by Document id and by Step id -- plus the genuine no-steps 404
  beside them, so "make /review always return 200" cannot pass. Mutation-proved by restoring the nil.
  **The general lesson for the derivation pattern**: `action.DeclaredFields(x, nil)` is a silent
  half-answer, not a compile error. Where a caller has the second Machine, pass it; the one remaining
  nil call site (`composition.buildPlacement`) reads no relation-derived Field and is now the only one.

  **Stage E1 -- the wizard's derivable half -- shipped 2026-09-28.** The Document's status Field from
  its own state model (`StatusField`), the ordering mode from the step Machine's `sequencing.mode_field`
  (it lives on the *parent* by design, which is why the child is where the pair is stated), the order,
  actor and actor gate from the Permission and `sequencing:`, the relation from `DeclaredFields`, and
  the PDF from the compositing Event's already-declared `source_field` (`action.CompositeFields`).

  **The prediction was 16 remaining and the measurement is 20**, and the four are the finding rather
  than a rounding error. `stepRowValues` is shared between a real Approval Step and a *saved flow
  template* step, and `mch_approval_flow_template_step` declares no `decide` Permission, no actor gate
  and no `sequencing:` -- nothing decides a template -- so `DeclaredFields` returns every id empty for
  it. Passing that through would have written four values under the empty key and produced a saved flow
  with no approvers at all. **Checked with a probe rather than reasoned about**, which is the only
  reason it was caught before the commit. So the template path now names its own Fields explicitly
  (`templateStepFields`) where it previously borrowed the real step's constants and was correct only
  because the two Machines happen to use identical strings -- four references that did not exist
  before, and a file that no longer relies on that coincidence.

  That is **Stage E2**, unstarted: the roles are cast (`flow_template`/`flow_template_step`, Stage A)
  but the Fields they hold are declared nowhere -- Stage D's gap exactly, one layer out.

  `continueDocumentWizard` crossed the 70-line budget on the way and was fixed the way that gate asks
  for, by extracting `approverRows` (a pair both wizard POSTs perform identically), never by raising
  `maxHandlerLines`.

  **The live check falsified its own plan, and that is worth recording.** The plan said the wizard
  "is reachable by the admin session (unlike My Tasks), so this one can genuinely be exercised end to
  end". Rendering is: `/documents/new` returns 200 with `fld_mode` resolved through the new derivation,
  which is itself the proof that `modeFieldFor` reads `sequencing:`. **Writing is not** -- `POST
  /documents` returns 403, because `prm_create_own_document` requires an Application role and the admin
  pseudo-identity holds none. The write path is covered instead by six Postgres-backed wizard tests
  through the real handler, mutation-proved: breaking one derived id fails four of them.

  **Stage E2 -- the saved approval flow's shape becomes a declaration -- shipped 2026-09-29.** The gap
  Stage E1 named and left with a forward pointer, now closed: `flow_template:` (key + mode) and
  `flow_template_step:` (template link, order, label, actor, actor kind, Group), two Machine-level
  blocks on the pair an Application casts in the `flow_template` roles. `domain.FlowTemplate`/
  `FlowTemplateStep`, validated against the declaring Machine's own Fields, mirrored into
  `installer.FullMachineCheckDoc` in the same change -- the **fourth** time
  `TestCheckDocsMirrorMetadatasOwnKeys` has asked for that, which is the argument for one commit rather
  than two.

  **The six row keys are not a second source of truth, and this is the stage's real content.** They ask
  what `sequencing:` and the `decide` Permission answer for the *live* step Machine. But a template is
  never decided, so it declares neither -- and Stage E1 established by probe, not by argument, that
  deriving from them returns **every id empty**, which would write four values under the empty key and
  save a flow with no approvers. Two Machines answering for themselves is not duplication. The
  alternative -- declaring `sequencing:` on a template so one derivation serves both -- would assert
  locking behaviour a template does not have, and was rejected for that reason rather than on taste.

  `documentApprovalFieldCoupling`: **29 across 7 files -> 11**, `internal/web/document.go` **20 -> 2**,
  exactly the prediction, re-measured. One design correction on the way: a first pass moved the row
  *label* with a `relabelTemplateRow` helper, which added three references of its own -- passing the
  label Field to `stepRowValues` as a parameter removed the helper and hit the predicted 2. The label is
  the one part still a constant for a live step, because nothing declares a step's label.

  **Verified, including live.** The eight existing flow-template tests pass **unchanged** over a changed
  mechanism, the same proof Stage C used. Two new ones:
  `TestApprovalFlowTemplate_overMachinesThatNameTheirFieldsDifferently` saves *and* reloads a flow over
  `mch_pola_persetujuan`/`mch_baris_pola` and asserts nothing was written under the template library's
  own ids, and `..._undeclaredShapeIsRefusedNotGuessed` holds the no-fallback half -- the assertion that
  would have caught Stage E1's near-miss before the probe did. Mutation-proved: putting one constant
  back fails the first; swapping the declaration for `DeclaredFields` fails the suite loudly.

  **Live, and one of the two halves genuinely worked.** Renaming `fld_sequence` -> `fld_urutan` in the
  `default` Workspace's own copy *without* updating the block made the running service refuse to load,
  naming the dangling entry -- so the declaration really is read at load in production, not just in
  tests. Renaming both loaded clean and served the wizard and its template fragment at 200. **What could
  not be checked live**: saving a flow, because `POST /documents` returns 403 for the admin
  pseudo-identity (`prm_create_own_document` needs an Application role it does not hold), and production
  holds no saved template to reload. That half is the Postgres-backed tests', and the renamed-Machine
  test is stronger than anything this session's identity can reach.

  **Order is load-bearing.** A before B because a declared binding is what lets a generalized
  `decide` know which Application it is acting for; B before C because the Service needs an Action
  to be triggered by; D after all three because it is the only one that had to *add* a declaration
  rather than read one, and adding it earlier would have been shape-before-need. Each stage is independently shippable and independently verifiable against a
  real Workspace, the same way A07/A08 were verified by mutating YAML rather than by test alone.

  **What is already gated, and what each stage will have to do about it** (added 2026-09-28, after
  the gates below it landed -- read this before starting a stage, since two of them will fail on
  correct work unless you also do the bookkeeping):

  - `internal/conformance.TestDocumentApprovalCouplingOnlyShrinks` holds
    `documentApprovalCoupling`, a per-file count of references to Document Approval's own
    Machine-id constants, frozen at 66 across 18 files. **A stage that works will make counts
    drop, and a count that drops *fails* until its entry is lowered** -- that is deliberate, so an
    improvement is locked in rather than left as headroom. Lowering an entry (or deleting it when a
    file reaches zero) is part of finishing the stage, not a workaround.

    It was also claimed here to be "the honest measure of whether a stage did what it claimed: if
    the number does not move, the stage did not." **Two stages have now shown that too strong, in two
    different ways, and the corrections are worth more than the slogan.** Stage A removed *identity*
    coupling, already funnelled into `IsDocument`/`IsStep` and held by
    `TestNoBareMachineIDIdentityChecks`, so there was never a reference here for it to remove. Stage B
    removed *Field*-id coupling, which this map does not count at all -- it matches
    `action.*MachineID` only. So the answer was to extend the gate rather than restate the claim:
    `documentApprovalFieldCoupling` (`TestDocumentApprovalFieldCouplingOnlyShrinks`, frozen at 129
    references across 13 files) is the Field-side population, added the day Stage B made a declarative
    alternative exist. Before starting a stage, ask which coupling it addresses and which population
    counts it -- and if neither does, extend one rather than claim the number will move.
  - `TestClosedRegistryMembersAreActivatedByMetadata` will fail Stage C the moment a
    `composite_signature_pdf` (or whatever it is called) is added to `domain.KnownServices` without
    an `events:` block naming it. That is the gate doing its job: the Service must arrive *with*
    the declaration that activates it, never as a registry line plus another direct call from flow
    code.
  - `TestClosedRegistryMembersAreAcceptedByTheLoader` will fail the same addition if its validation
    case is missing -- the registry and `internal/metadata`'s own switch are two lists that drift.
  - `TestNoBareMachineIDIdentityChecks` already forbids the shape Stage A exists to remove, so
    Stage A cannot regress into it while being written.

  **Gap A is deliberately ungated, and must stay that way until Stage B lands.** A gate locks in
  progress; it does not create it. Forbidding an Action's effect from being written in Go before
  an Action can declare its effect would forbid the only way there is. Build the primitive,
  migrate `decide`/`revise`/`continue-submit` onto it, *then* add the gate -- the same order
  `TestNoBareMachineIDIdentityChecks` followed, which could only be written after all 23 call
  sites were converted.

- **Workspace isolation: a Workspace's metadata is its own copy, not a shared file -- ~~planned~~ shipped 2026-09-27** (owner
  decision, 2026-09-27, prompted by the data-loss incident below). **Written up here as a plan
  first, then built the same day; what actually shipped is recorded at the end of this entry.**

  **What the owner asked for, in their own framing**: a `*.yaml` is a **template**. Sharing it
  between Workspaces means sharing its *contents* -- installing **copies** it into the target
  Workspace, under names that make the copies distinct Applications. The reason is divergence:
  a Workspace may install the same template another one already uses and then want to change its
  own copy, and *"perubahan aplikasi di masing masing workspace tidak saling terkait."* Only
  `mch_user` genuinely crosses Workspaces -- *"bisa dipakai di seluruh universe."*

  **What the code does today is the opposite, and says so out loud.** Three places state the
  shared model as a deliberate design, so this is a reversal to make explicitly, not a bug to
  quietly patch:

  - `metadata/workspaces/default.yaml`: *"An Application file itself names no Workspace -- it is a
    reusable declaration, and several Workspaces may install the same one."* Manifests reference
    Machine files by relative path (`../user.yaml`), so installing is *pointing at*, not copying.
  - `domain.Workspace.MachineIDs`' own doc comment: *"The Machines themselves are loaded
    process-wide and shared (one file, one object, however many Workspaces install it); this is
    the per-Workspace membership list."*
  - `cmd/server/main.go`'s `loadMetadataState` flattens every Workspace's Machines into one
    id-keyed map and **dedupes by id, first loaded wins** (`if _, seen := machines[m.ID]; seen {
    continue }`). Two Workspaces cannot hold different definitions under one id today: the second
    silently gets the first one's Machine.

  **What is already isolated** is records -- the `records` table keys on `machine_id` *and*
  `workspace_id`, with indexes on the pair. It is the schema/definition layer that is shared.

  **What surfaced it**: publishing an AI-generated Application into the empty "Dokter Kecil"
  Workspace overwrote `metadata/document.yaml`, the real Document Approval Machine installed in
  "default" -- 266 lines of Permissions, Transitions, Events, datasets and views replaced by a
  generated 69-line one. A generated Machine's filename comes from its id alone into one flat
  directory, so "different Workspace" and "different Application" were not different files. Two
  guards shipped the same day (`aiassist.refuseIfExists`, and widening
  `internal/web.existingStateFor` to treat machine ids as the global namespace they currently
  are) -- but both are containment for the shared model, not this.

  **Two candidate designs.** The second is the recommendation.

  - **(A) Namespace the ids.** Copy the template, rewriting every id with a Workspace-derived
    prefix (`mch_dokter_kecil_document`). The flat directory and the global map survive untouched,
    so it is much the cheaper change. Its cost is that uniqueness lives in a string convention --
    which is exactly what failed here, since nothing can enforce a convention -- and every
    cross-reference inside a copied file (relation targets, `summary_machine`, an Application's own
    `machines:`, a Permission's `actor_field`, navigation) must be rewritten consistently or the
    copy is subtly broken.
  - **(B) Namespace the storage, keep ids readable.** Copies live under the Workspace's own
    directory and the runtime keys Machines by *(Workspace, id)* rather than id. This finishes a
    migration this repo already started: CLAUDE.md's own rule is that **which Workspace a request
    is in is a per-request fact** travelling on ctx (`rendering.WithCurrentWorkspace`), and
    `web.Deps.Machines` -- captured once at router-build time and closed over by 41 call sites in
    `router.go` -- is the last package-level contradiction of it. `metadata.LoadWorkspaces`
    already returns per-Workspace `App{Workspace, Machines}` values; only `cmd/server`'s
    flattening collapses them, so the loader needs no reshaping at all.

  **Plan, in order:**

  1. **Owner decides the copy's shape** -- (A) or (B) above, and for (B) where a Workspace's own
     files live (`metadata/workspaces/<slug>/`, most likely). Everything below assumes (B); under
     (A), steps 2 and 5 mostly fall away and step 3 grows an id-rewriting pass.
  2. **Make Machines a per-request fact.** Remove the global flatten in `loadMetadataState`;
     resolve a request's Machines from the Workspace already on ctx, the way `routeByID`/
     `labelByID` already take `ctx`. This is the load-bearing step and the whole of the risk:
     `Deps.Machines`/`Deps.MachineList` and their 41 `router.go` call sites are the blast radius.
     Worth doing on its own, before any copying exists, so it can be reviewed as a pure refactor.
  3. **Build "install" as a real operation.** Copying a template into a Workspace -- used by the
     AI assistant's own publish path and by whatever UI installs a template later -- replacing
     today's "append a relative path to the manifest". `aiassist.Write`'s new-Application path is
     the closest existing shape to extend.
  4. **Migrate the metadata already on disk.** "default" gets its own copies of the twelve Machine
     files and two Application files it currently points at; "dokter-kecil" keeps `user`/`activity`.
     Machine **ids must not change for an already-installed Workspace** -- `records.machine_id`
     holds them and there is real data in "default" -- which is another point for (B), where ids
     stay stable by construction.
  5. **Lock it with conformance tests**, since prose is what got contradicted here: no
     process-wide Machine map exists; two Workspaces can hold different definitions under one id;
     editing one Workspace's copy provably does not change another's.
  6. **Retire the statements this reverses.** `default.yaml`'s "several Workspaces may install the
     same one", `domain.Workspace.MachineIDs`' "one file, one object", and CLAUDE.md's own
     Machine-sharing wording all describe the old model. Also revisit
     `internal/web.existingStateFor`'s global collision check: it is correct for today's flat
     shared-file model and becomes **wrong** under isolation, where a new Workspace naming its own
     `mch_document` is entirely legitimate. `aiassist.refuseIfExists` stays correct under both.

  **The payoff worth naming**: once installing is copying, "build me a document approval app"
  stops being a thing the AI assistant must refuse (`internal/aiassist/prompt.go`'s
  `composableSurface`) and becomes *install the Document Approval template into this Workspace,
  then diverge* -- which is the composable-runtime premise working as advertised rather than a
  capability gap to apologize for.

  **Shipped 2026-09-27, in the three stages planned, each green on its own before the next.**

  **Stage 1 -- Machines became a property of the Workspace.** `domain.Workspace` now carries its
  own loaded `Machines`, populated by `metadata.LoadApplication`, which had produced them per
  manifest all along; only `cmd/server` flattened them away. `Deps.Machines`/`MachineList` are
  gone, and handlers resolve theirs from the Workspace on ctx (`machinesFor`/`installedMachines`),
  across 46 call sites. The seam really was already there: `installedMachines` and `resolveMachine`
  each paired the union with a `ws.HasMachine` gate, and that filtering step is exactly what
  disappeared. `web.Deps` keeps one Machine, `UserMachine` -- the three handlers needing `mch_user`
  (`/register`, `/accept-invite`, `/create-workspace`) run before a ctx Workspace exists or are
  creating the Workspace in question. **One real bug fixed in passing**, as predicted:
  `runScheduler` scoped a ctx per Workspace but passed the whole cross-Workspace list to
  `RunScheduledEvents`. `decideStep` did tip over `TestHandlersStaySmall` at 71 lines, and was
  resolved the way that test's own message prescribes -- the single `machines` use was inlined,
  not the budget raised.

  **Stage 2 -- copies on disk.** `metadata/*.yaml` + `metadata/applications/*.yaml` stayed put as
  the template library (which keeps `TestCapabilitiesMachinesTableMatchesMetadata` both passing and
  *meaningful* -- that table documents the library). "default" was migrated: ten Application-owned
  Machines and both Application files became its own copies under `metadata/workspaces/default/`,
  byte-identical to their templates, so divergence from here is a plain diff. **No data migration
  at all** -- ids unchanged, `records.machine_id` untouched, which is the concrete payoff of
  choosing scoping over id-prefixing. `mch_user`/`mch_activity`/`mch_notification` stay shared
  references. `aiassist.Write` now derives the target Workspace's own directory from its manifest
  path, so a generated Application can no longer land in the shared library at all.

  **Stage 3 -- locked, and the old statements retired.** Two new gates in
  `internal/conformance/workspace_isolation_test.go`: every Application an installed Workspace
  claims (and every Machine those Applications claim) must be that Workspace's own copy, with a
  short closed allowlist for the three runtime-level Machines; and two Workspaces must be able to
  hold *different* Machines under one id, asserted against the real loader. Both were verified to
  fail when the property is broken, not merely to pass. `internal/web.existingStateFor`'s global
  collision check was **reverted** as the plan warned it would need to be -- correct that morning,
  wrong by that evening, since a new Workspace naming its own `mch_document` is now exactly right;
  its test was rewritten to assert the opposite of what it had asserted hours earlier.
  `aiassist.refuseIfExists` stays, being the one guard correct under either model. CLAUDE.md,
  `capabilities.md`, `domain.Workspace` and `metadata/workspaces/default.yaml` all had their
  shared-model wording replaced.

  **Verified live** with minted sessions against the running service: "default" still lists both
  Applications and renders 14 `mch_document` records, `/approval-inbox`, `/dashboard` and
  `/activity` all 200; the empty "Dokter Kecil" Workspace still shows "Nothing installed here yet".
  The four query-cost tests are **unmoved**, which is the assertion that matters most here: it
  proves Machines resolve in memory off the ctx Workspace and never from the database.

  **A real cost of this design, found and paid the same night.** The Document Approval engine
  identified its own Machines by a bare `machine.ID == action.DocumentMachineID` comparison in 23
  places across `internal/web` and `internal/rendering` -- a sound proxy for "this is *the*
  Document Approval Document" while ids were unique across the process, and false the moment
  isolation let two Workspaces each hold their own Machine under one id. A generated "Document
  Tracking" Application did exactly that within hours, and its own detail page panicked twice,
  reaching for an `mch_approval_step` and a `nav_approval_inbox` its Workspace does not have.

  **The first fix was the wrong one and is recorded here because the reasoning matters**: a
  reserved-id list in `aiassist.Validate`, forbidding seven names outright so the engine's
  assumption could stay unexamined. The owner rejected it on exactly the right grounds -- under
  this model another Workspace naming its own `mch_document` *is* supposed to be fine, so
  forbidding the name makes the model give way to the code. It was reverted the same night.

  **The real fix**: `domain.Machine.ApplicationID`, stamped at load from the claiming
  Application's own `machines:` list (`internal/metadata`'s claim-then-stamp pass) and
  per-Workspace since Stage 1 moved Machines onto the Workspace. `action.IsDocument`/`IsStep` ask
  both halves -- which Application claims this Machine, and which Machine within it -- and
  replaced all 23 comparisons. Confirmed against the real metadata: "default" stamps its
  `mch_document` `app_document_approval`, "dokter-kecil" stamps its own `app_document_tracking`,
  same id, different Applications, and the engine now tells them apart. A Workspace that merely
  shares the id no longer enters Document Approval's branches at all, rather than entering them
  and failing gracefully -- two fewer queries on that page, as a side effect of no longer asking a
  question that never applied. The two defensive patches written before the real fix
  (`loadSignaturePlacementData`'s nil guard, `detailBackLink`'s `hasNavItem` check) stay as
  belt-and-braces; neither is reached any more.

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
- **Extending the Event primitive to a schedule/time trigger -- ~~planned~~ shipped 2026-09-27**
  (the SLA-breach reminder, "SLA-breach reminder via a scheduler primitive" entry below). Its own
  second-real-case generalization, not assumed ahead of a concrete need, the same discipline that
  governed building the primitive itself (and its `on_create` shape).
- **A View composing other Views -- ~~planned~~ one real case shipped 2026-09-26** (`domain.ViewStepper`,
  the deferral table's "The approval stepper is five constants, not a declared View" row, above).
  The declared-View gate itself shipped 2026-09-20 (`views:` with ids, names and types; `?view=`
  selects one; `table`/`board`/`cards`), deliberately without composition: a View that arranges
  other Views rather than a Machine's own records, which is what the approval stepper needed to
  stop being Go. `stepper` is that fourth type -- upstream's own CAP-V20 (the sequential
  done/current/waiting stepper over a parent's children), needing no new View config at all since
  this runtime's own `sequencing:` (`domain.Sequencing`) already carries the field bindings
  upstream had to add a `DecisionStepperConfig` for. Composed only from a child collection's own
  renderer (`internal/rendering/detail.templ`'s `domain.Machine.StepperView`), never offered by
  `?view=`'s switcher (`internal/rendering/machine.templ`'s `selectableViews` excludes it) -- a
  parent's children make no sense as an alternate arrangement of the declaring Machine's own flat
  list. The three types that shipped 2026-09-20 were the three that already existed in code;
  upstream's registry carries ten, and each further one still arrives when a screen earns it on its
  own evidence rather than by import. Splitting the old anonymous `view:` block was part of the
  same decision: `sla_field` and `card_fields` describe how a Machine's records look *wherever*
  they appear (the detail page selects no View at all), so they became Machine-level and a View now
  carries only the arrangement.
- **A filter that reads the viewing identity, not a literal** — `datasets:`' `where:` compares a
  Field against a written value, so any count scoped to "mine" stays Go. Three screens do it now:
  `composition.PersonalTasks` (assignee == the viewer), `composition.ApprovalInbox` (the inbox and
  the nav badge), and — since 2026-09-24 — `composition.AssignedToMe` (Assigned to me, below).
  Three real cases is past this repo's own two-case trigger, and it is worth being explicit about
  why the third did not generalize the primitive rather than leaving that looking like an oversight:
  `AssignedToMe`'s own "mine" test is not `where:`'s simple shape at all. It is two-armed (a direct
  `fld_assignee` match, or `fld_approver_group` naming a Group the viewer belongs to — CAP-F24, the
  same dynamic gate `authorization.AllowsAction` already evaluates) and its per-row output depends
  on a second Machine's sequencing state and a third read (the submission activity) — the same
  class of derivation `ApprovalInbox` already is, not a filtered list. `PersonalTasks` and the
  Inbox's own "mine" test *are* that simple shape and remain the real candidates whenever this is
  picked up; `PersonalTasks`' doc comment still says "neither has a second case yet" — true when
  written, and now stale against its own sibling, worth fixing in the same change that finally
  builds the sentinel. Upstream settled the shape rather than leaving it open: a `$current_user`
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
- **Reject unknown metadata keys at load -- ~~planned~~ shipped 2026-09-28.** `internal/metadata`'s
  three `yaml.Unmarshal` calls decoded without `KnownFields`, so a key the parser had no home for was
  dropped in silence and the Machine loaded clean. 005 Phase 3 says invalid metadata must not reach
  compilation; this was the one validation hole that made *documentation* drift dangerous rather than
  merely untidy — a guide that still showed a retired key produced no error anywhere, which is how the
  pre-2026-09-20 singular `view:` block stayed in `writing-guide.md` §6/§12.7 unnoticed (verified by
  loading a copy of `metadata/` that used it: no error, zero Views).

  **The prerequisite this entry named — "every existing manifest must be clean before it can be turned
  on" — was checked before any change, and was already satisfied**: 33 files, zero unknown top-level
  keys; 94 distinct keys used across `metadata/` against 92 the loader knows, and the five that looked
  unknown (`document`, `step`, `signature`, `flow_template`, `flow_template_step`) are `workflow.roles`
  **map keys**, which `KnownFields` does not constrain. The inline YAML in tests was clean too. So this
  was a three-line change plus the care around it, not a migration.

  **What shipped.** `internal/metadata.decodeStrict`, used by all three sites, and an error rewritten
  for the person who wrote the file rather than the person who wrote the struct: yaml's own
  `field foo not found in type metadata.machineDoc` becomes `"foo" is not a key a machine file
  declares`, keeping the line number, which is the useful half. That wording matters beyond taste --
  on the assistant's path the message is handed straight back into the conversation as the thing to
  correct (`returnToConversation`), and a Go type name there is worse than noise.
  `TestMetadataDecodesStrictly` gates it: no bare `yaml.Unmarshal` in that package, because the lenient
  way is the one the yaml package makes easiest.

  **Why now, and one claim corrected while doing it.** The plan argued this mattered most for the AI
  assistant -- that a key the model invented was dropped without a word. **That turned out to be wrong,
  and checking beat assuming:** `aiassist` marshals from typed Go structs and appends list items to
  existing files, so it has never been able to emit a key at all, let alone an unknown one. What this
  protects is hand-written metadata, the template library, and any future writer. The error wording
  still earns its place on the assistant's path, because `aiassist.Write` load-verifies and hands any
  failure back to the conversation, where a Go type name is worse than noise. The real trigger was the
  same failure shape showing up twice that day one level up -- `installer`'s strict-decode mirrors
  drifting from the loader's own docs, which `TestCheckDocsMirrorMetadatasOwnKeys` closed. This is the
  read side of that rule.

  **The trade, named rather than discovered later:** a manifest written for a newer runtime now fails
  hard on an older binary instead of loading with that key ignored. That is 005 Phase 3's own posture,
  and it is free only while metadata and binary ship together (`capabilities.md`'s "metadata loads
  once, at startup").
- **Metadata hot reload and change classification** — tracked in `capabilities.md`'s limits as two
  separate deliberate deferrals (a `*.yaml` edit needs a restart; deleting a Field silently orphans
  its data in every record's JSONB) and in the companion repo's own
  `guides/metadata-hot-reload-safety.md`, but named in no plan at all until now. Neither is
  urgent while the manifest ships with the binary; both stop being optional the moment metadata is
  edited by someone who cannot restart the process, which is the actual end state this runtime is
  for.
- **Pagination on record lists** — search itself shipped 2026-09-25 ("In progress", above), checked
  against the Flow 2 mockup rather than assumed: no board shows pagination controls, so it stays
  here. **Pagination is also the cheapest answer to the data-path cost measured in the study
  above** (007 §7.9): it breaks the "a page costs what the Machine holds" relationship outright,
  without needing Filter/Projection/Aggregate pushdown first. Its own trigger arrives sooner than
  theirs — one list screen past ~200 rows, `capabilities.md`'s own volume-threshold harness
  (`make threshold`) is what would confirm it's been reached.
- **Background/scheduled jobs -- ~~planned~~ the first real case shipped 2026-09-27** (SLA-breach
  notifications that don't depend on someone opening the page -- "SLA-breach reminder via a
  scheduler primitive" entry below). A `time.Ticker` goroutine in `cmd/server/main.go` is the
  mechanism; a second, unrelated background job is the trigger to reconsider whether that stays
  sufficient (e.g. a dedicated worker process), not before.
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
  **(3) is now done for the Case 19 group (2026-09-28).** `mch_task`, `mch_project` and `mch_list`
  declared `card_fields:` for the first time (and `mch_document` gained the `title` role it was
  missing), so My Tasks, the Calendar week, the Sprint dashboard's Attention list, the Dashboard's
  Project summaries and pending-Document rows, and Team Capacity all render a shape Composition
  resolved from a declaration. `projectionRatchet` went five entries -> one.
  **The trap this avoided is worth more than the count**: the easy migration is to move each
  `Values["fld_title"]` out of the `.templ` and into `internal/composition`, which passes the gate
  (it only scans `.templ` files) while relocating the coupling -- gaming the gate rather than
  satisfying it. The rule held to instead: a value may move into Composition only if Composition
  *derives* it from a declaration. Title/status/date come from `card_fields` via
  `composition.ProjectedByRole`; Team Capacity's weekly number comes from `ds_user_capacity`'s own
  `field:` (`datasetMeasureField`), which already declared it.
  **`boardsettings.templ` stays, and its reason changed rather than being repeated**: it renders a
  List's `fld_color`, and the closed role set (title/person/money/status/date) has no role that
  describes a colour token. Inventing one for a single case is what CLAUDE.md's step 3 forbids --
  the trigger is a second case.
  **Verified by before/after HTML, because no rendering test exists for any of these screens.**
  The first capture proved almost nothing: the database holds zero `mch_task` and zero
  `mch_project` records, so four of the five screens were comparing empty states. Seeded tasks, a
  Project and two weekly capacities, built HEAD in a throwaway `git worktree` on port 4101, and
  diffed the six pages (adding `/sprint`) against the post-change binary on 4000: **byte-identical
  once the per-request CSRF token is normalised**, with real rows rendering -- task titles and
  statuses in the Calendar, the Attention list's overdue/due-today rows, `40h/week` on Team
  Capacity, the Dashboard's pending Documents. Seed rows deleted afterwards and the clean-state
  pages re-diffed against the original baseline. One thing the live check could *not* cover: the
  admin session's viewer id is the literal `admin`, which is not an `mch_user` record, so My Tasks
  is empty for it by construction and the generic create route refuses `fld_assignee: admin`. Its
  rows are covered by `composition`'s own unit tests and by the Calendar/Sprint, which share
  `taskRowList`'s markup and the same `composition.taskRow` builder.
- Re-evaluating `internal/composition/pages.go`'s Case 19 Machine-id/status-option constants
  (`taskMachineID`, the `todo`/`in_progress`/`done` switch) against the B1-B5 decomposition
  criteria now that `card_fields` (Projection) has shipped -- flagged, not decided, in
  `menata-app-document`'s `audits/2026-09-19-metadata-hardcoding-gate-mapping.md`; these predate
  the "exception needs a forward-checkable pointer" convention (`CLAUDE.md`), so this is also the
  first case of applying that convention retroactively. Owner decision, not assumed here.

---

Full build history, architecture decisions, and audit records live in the private
`menata-app-document` repository.
