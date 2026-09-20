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
  7. Role-based Permission + a declared transition model, then board 06 itself. Fase 4 supplied
     roles, so the foundation is closer than when this list was written — but both primitives are
     still absent from 006 and 007, which is why this stays last.

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
  | Approval Role Matrix (board 06) | Fase 7 | role-based Permission + a declared transition model, neither in 006/007 |
  | Declared `requires_role:` on a navigation item | **a second real case, not a phase** — likeliest around Fase 7, when an Application role could gate an item (a handler naming an id cannot express that) | one case exists today and has code: `appShell`'s `hiddenNavIDs`. See "Per-user/role navigation filtering" below for why the second case, not the calendar, is the trigger |
  | Mobile bottom bar for an Application's own menu | **no phase yet** — lands when the Project Management screens get boards | Those screens are still `pageStyles`, so a bottom bar there would be hand-written CSS thrown away when they port — that is the real blocker. **This row previously said "nothing in Fase 2–7 displays it", which stopped being true at Fase 3a**: Project Management declares no `show_nav`, keeps its menu, and `/my-tasks` renders ten nav links today. It stayed wrong through four phase closes because nobody re-read the table, which is the one thing this table needs. The owner's decision (`navigation.html`, 3–4 icons, top-4 by priority) was never the missing part |
  | Saved default approval flow (board 08: "Save this as the default approval flow for **Contract** documents", checked by default) | **no phase yet** | Upstream built it (CAP-V28) as two companion Machines — one template per Document Type, plus its own ordered steps — with a write direction that find-or-creates the template on submit. That is the real blocker: a template entity and a write path, not the screen. **This row exists because the deferral did not.** It has been live since Phase 15, recorded only inside `menata-app-document`'s development history, in no table anyone re-reads at a phase close — and with a reason that was wrong when written ("only one Document Type exists in metadata today"; there were zero) and is wrong now in the other direction (there are three: `Kontrak`, `Tagihan`, `Lain-lain`). Same shape as the bottom-bar row that sat wrong through four closes |
  | One signature box for a Group-held step (boards 08/09) | **no phase yet** — **named, not solved** | Board 09 places exactly one signature box for `Legal Group · 4 members`, and nothing on either board says whose signature image lands in it, or what happens when two of the four act. Upstream has the identical gap on its own compositing capability, recorded there in the same words. Worth holding here rather than discovering it during 6c-2's port |
  | Member search box (board 04) | **no phase yet** — "Planned: search, filtering and pagination" below | search does not exist anywhere in the app |
  | "Keep me signed in" (board 01) | **no phase yet** | a session-lifetime change; `internal/authorization` has no remember-me concept |
- Rounding out Project Management: a project-level workspace overview, richer task detail
  (checklist, comments, attachments), and scoping views to one project at a time.
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
