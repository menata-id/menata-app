# Menata App — agent instructions

## What this project is

This is **not an ordinary application** — it's a **composable runtime** (README.md: "The runtime
application that turns Menata Runtime Metadata into a running application"). The product is the
runtime itself; any one app it renders (Task Tracker, Document Approval, whatever's in
`metadata/workspaces/*.yaml` today) is just one metadata description it happens to be interpreting right
now. `007-composable-runtime-architecture.md` is the normative target for this.

That distinction changes what "just add a button/field/route" means here. In an ordinary app,
hand-writing a button is the whole job. In this repo, hand-writing something that metadata could
express instead is a regression against the actual product — it's exactly the kind of change that
works today and quietly breaks the runtime's own premise (`metadata/workspaces/*.yaml`, no per-model code)
the next time someone tries to evolve the app by editing YAML alone. Default to asking "can
metadata express this?" before "let me just write the code" — not the other way around.

Read `001-design-principles.md` through `007-composable-runtime-architecture.md` before making
any architectural decision in this repo — adding a route, a field, a UI element, or anything that
isn't a pure bugfix. They are short, numbered, and load fast; do this even if the task looks like
"just" a UI or handler change. `writing-guide.md` and `capabilities.md` are the practical/current-
state companions once you've read the principles.

**Read them as a destination first, and only then as a constraint list.** That ordering is not a
style preference — reversing it produced three phases in a row whose outcome was "don't" or "less", and
the owner had to correct the frame twice before the pattern was visible (2026-09-30; the full record is
`menata-app-document`'s `audits/2026-09-30-kajian-arah-pengembangan-dan-protokol-kolaborasi-agen.md`).

The distinction that went wrong, and it is worth carrying exactly:

| question | the tool for it | where it comes from |
|---|---|---|
| "Should a **new** capability be added?" | the admission test (A1–A5), the second-case rule, the ratchets | `capability-lifecycle.md` §2; 007 §27, whose own words are *"Before adding a **new capability**"* |
| "How do we execute architecture that is **already agreed**?" | sequencing and design; gates **lock** each step afterwards | 001–007 as the normative target |

Most of what is unbuilt in this runtime is the second row, not the first. UI IR, Data IR, the planner,
the Experience Plane's primitives — those are 005 Phase 5 and 007 §12/§15/§16, already decided. Asking
them to pass an admission test is asking agreed architecture to re-prove itself as a proposal.

And **007 §40's `PROPOSED` is not a veto.** Its own sentence is *"A PROPOSED row is not admitted by
appearing here"* — an anti-overclaim device, stopping you from treating the document's mention as proof
the thing exists. It does not withhold permission to build. Read as a veto, it stalls everything §40
lists.

Two corollaries the same day produced:

- **007 §27 read as a design checklist argues *for* building.** Its Q3 — *"Would one new generic
  primitive unlock this and other cases?"* — is the question a genuine substrate passes most strongly.
  A review that never asks Q3 is using the gate list to decline rather than to design.
- **The second-case rule governs *semantics*, not *structure*.** It guards against inventing meaning
  nobody needs (a colour role for one Machine). It was never meant to guard against adopting a
  structural vocabulary that 007 already closes, that Bootstrap/Tailwind/CSS Grid settled long ago, and
  that this tree already uses — `flex flex-col gap-N` (007 §12.2's `stack`) appears **61 times** across
  38 screens. Proposing to "create a second case" for that was a miscount, and §12.2 forbids the
  incremental version of it outright (*"should not require separate layout concepts such as
  `DashboardLayout`…"*).

**Gates preserve what has been achieved; the vision directs the next step.** Both are needed and
neither may take the other's job. Keep the execution order the repo already uses — build the primitive,
migrate the uses, *then* gate — because a gate installed first tests failure classes that do not exist
yet, which 007 §15.3 demonstrates about itself.

**Knowing that paragraph is not enough — one agent broke it four times in one session after writing it
down.** The failure record, with all four instances and the four-step check that prevents them, is
`menata-app-document`'s `guides/subordinate-mechanisms-must-not-outrank-the-vision.md`. Read it before
concluding that something 001–007 directs should not be built yet. The short form:

1. **Check the mechanism's scope against its own sentence.** §27 reads *"Before adding a **new
   capability**"*; §40 says a `PROPOSED` row *"is not admitted by appearing here"*. A mechanism applied
   outside its scope is not refusing — it is being misused.
2. **Look for the imperative word in the normative text.** `MUST`, `MUST NOT`, *progressively*, *should
   not require*. A claim that contradicts the text is the claim that is wrong: "Stage 3 cannot be split
   honestly" died to one word in §24 (*"progressively* lower them to generic primitives").
3. **Ask whether a MUST is in breach right now.** §12.4 says a View **MUST NOT** be the universal
   composition primitive, and this runtime renders 38 screens from 38 bespoke Go functions. That breach
   is live, and "no second case yet" is not an answer to a MUST already failing.
4. **If the reason to stop is yours — change the reason, not the work.** Remaining context, slice size
   and verification standard are all under your control. Split the pipeline, ship one fully verified
   stage, and name which stage. **Never dress an operating limit as an architectural finding**: that is
   the same overclaim class as `ir/doc.go` once claiming to hold three IRs while empty.
   Self-recognisable warning sign: if your summary contains *"better not to start than to start half"*,
   you are in this failure. The right answer is a smaller whole slice, not zero slices.

The two that most often get violated by an unreviewed edit:

- **001 Principle #3, Metadata First** — "Application evolution should primarily occur by
  changing Runtime Metadata rather than application source code." If a route, label, or
  navigation entry already exists in a Workspace's manifest, a page must reference it, not retype it
  as a second literal.
- **001 Principle #8, Reference over Duplication** — "Duplicated metadata should be avoided
  whenever possible." Before hardcoding a string that looks like it describes application
  behavior (a route, a label, a count), grep `metadata/*.yaml` for it first.

## One manifest per Workspace (2026-09-22)

An Application is **installed into** a Workspace, not owned by the process. `metadata/workspaces/
<slug>.yaml` is one Workspace's installation: which Workspace it is for (by **slug** — the `ws_...`
id is generated when a Workspace is created through the UI, so nobody can write it into a file),
which Machines exist there, and which Applications are installed. The directory is *scanned*, so
dropping a file in installs; deleting it uninstalls. An empty `applications: []` is valid and
normal — it is what a Workspace looks like the moment it is created.

**That sentence was only true on paper until 2026-09-30.** Creating a Workspace wrote a `workspaces`
row and no file; every manifest was hand-written. Reading tolerated it (no manifest resolves to the
zero Workspace), but publish and install built their path from that zero Workspace's empty slug, so
the first write into any UI-created Workspace aimed at `metadata/workspaces/.yaml`. Creation now
writes the empty manifest (`installer.EnsureWorkspaceManifest`, deliberately without a reload, since
reloading resets `/register`'s rate limiter), and a handler writing into a Workspace goes through
`internal/web.workspaceInstallation`: slug from the row, installation from disk, missing manifest
healed. **Never build a manifest path from `rendering.CurrentWorkspace(ctx).Slug`.** A failed write
that no proposal can fix is not handed to the AI assistant either; only an `installer.RejectedError`
is. Audit: `menata-app-document`'s `audits/2026-09-30-kajian-workspace-baru-tanpa-manifest.md`.

This replaced a single process-wide `metadata/app.yaml` loaded once at startup, which made every
Workspace render the same Applications no matter which one you were in: a `workspaces` row scoped
records and membership, but not what the Workspace *was*. The owner found it by creating a
Workspace and seeing two Applications in it that nobody had installed.

So **which Workspace a request is in is a per-request fact**, like which Application already was.
It travels on ctx (`rendering.WithCurrentWorkspace`, set by `internal/web.currentWorkspace`), and
`routeByID`/`labelByID` take `ctx` for that reason. There is no package-level `workspace` any more;
do not reintroduce one.

**And so is which Machine an id means (2026-09-27).** `domain.Workspace` carries its own loaded
`Machines`; a handler resolves the request's set from the Workspace on ctx (`machinesFor`/
`installedMachines` in `internal/web/machine.go`), never from a process-wide map. `web.Deps` holds
exactly one Machine, `UserMachine` (`mch_user`), because that one genuinely crosses Workspaces and
its three callers — `/register`, `/accept-invite`, `/create-workspace` — run before a ctx Workspace
exists or are creating the Workspace in question. Do not add a second.

**Installing an Application copies it; it does not point at a shared file.** `metadata/*.yaml` and
`metadata/applications/*.yaml` are the **template library**. A Workspace's own copies live in
`metadata/workspaces/<slug>/`, which is where anything generated for it must be written
(`aiassist.Write` derives that directory from the manifest path). Two Workspaces may therefore each
hold an `mch_document` meaning different things, and either may edit its own copy without touching
the other's — which is the point (owner, 2026-09-27: "perubahan aplikasi di masing masing workspace
tidak saling terkait"). Only `mch_user`, `mch_activity` and `mch_notification` stay shared
references, claimed by no Application. `internal/conformance`'s
`TestInstalledApplicationsAreCopiesNotSharedFiles` and
`TestTwoWorkspacesCanHoldDifferentMachinesUnderOneID` hold both halves; this replaced a shared-file
model whose own three comments described it accurately right up until a generated Application
overwrote `metadata/document.yaml` and destroyed it.

**So a Machine id no longer identifies a Machine — and neither does an Application's name (2026-09-28).**
Use `action.IsDocument`/`action.IsStep`, never `m.ID == action.DocumentMachineID`. Twenty-three call
sites did the latter, correctly, while ids were unique across the process; the first Workspace to
legitimately name its own `mch_document` made every one of them wrong, and two of them panicked
before anyone noticed. `TestNoBareMachineIDIdentityChecks` holds this now.

The fix that day put `m.ApplicationID == "app_document_approval"` inside those predicates, which was
right about the question and wrong about the answer: it identified the Application *by its name*, so
the approval engine woke for exactly one Application name — renaming Document Approval, or installing
a second approval Application beside it, silently disengaged every approval mechanic while the
screens kept rendering. What the predicates read now is what the Application **declares about
itself**: `workflow: {engine: document_approval, roles: {document: ..., step: ...}}`, resolved at load
onto `domain.Machine.WorkflowEngine`/`WorkflowRole` (`domain.Workflow`, `metadata.stampWorkflowRoles`).
`TestNoApplicationIDIdentityChecks` forbids the literal coming back, and
`TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames` proves an `app_persetujuan` over
`mch_surat`/`mch_langkah` engages identically.

The general rule, and it applies to any future hardcoded Case 19-style behavior: **a name is never an
identity the runtime may branch on.** The Machine id says *which Machine within an Application*, never
which Application; the Application id says *which installation*, never which behavior. If code needs
to know "is this one of mine", the Application has to declare it, and `workflow:` is the worked
example of how — a closed engine registry (`registry.KnownWorkflowEngines`) whose roles are validated at
load against the Application's own `machines:`.

**And what an Action writes is declared too (2026-09-28).** A Machine's `actions:` block says which
Fields a named Action sets and where each value comes from (`submitted`, `actor`, `actor_name`, or a
declared literal) — `domain.ActionEffect`, applied by `action.ApplyEffect`. Do not write
`record.Values[action.FieldX] = ...` in a handler for an Action's own effect; declare it. Two things it
deliberately does *not* cover, both named in place rather than left to be rediscovered: the status move
itself (derived from the `transitions:` edge that names the Action — declaring it twice would be 001 #8),
and the *read* side (the signed-PDF compositing still filters steps by `fld_decision`), which is what
`TestDocumentApprovalFieldCouplingOnlyShrinks` now measures.

**And where a signature lives is declared, not named (2026-09-28, Stage D).** Two Machine-level
blocks beside `fields:` — `signature_placement:` (image/page/x/y/width) on whichever Machine an
Application casts in the `step` role, `signature_store:` (owner/image) on the optional `signature`
one. Read them through `action.SignatureFields`/`StoreFields`, never `action.FieldStepSignatureX`.

This one was **added rather than derived**, and that is the distinction to carry forward: every
earlier stage removed a literal by finding a declaration that already answered its question. Nothing
answered "which Field holds a signature", so Stage A's promise — cast any Machine in the `step` role,
under any name — was only half-true until the capability existed. When a gate cannot shrink because
no declaration exists, building the declaration *is* the work; do not derive from the nearest-looking
block instead. Note the gate's own blind spot this exposed: `internal/action` is excluded from
`documentApprovalFieldCoupling` as the owner of those constants, so `StampFor` named all four
coordinates itself, invisibly, through three stages. Only reading it found that.

**And so does a saved approval flow (2026-09-29, Stage E2).** `flow_template:` (which Field a saved
flow is keyed by, which holds its mode) and `flow_template_step:` (the template link, order, label,
actor, actor kind, Group), on the Machines an Application casts in the `flow_template` roles. Read them
through `action.FlowTemplateFields`/`FlowTemplateStepFields`/`FlowTemplateRowFields`.

**This pair is where the "derive or declare" rule gets its sharpest edge, so read it before adding
either kind.** A *live* Approval Step's shape is derivable — `sequencing:` gives the order, the `decide`
Permission gives the actor and its gate — so deriving is right and restating would be 001 #8. A
*template* row's is not, because nothing decides a template and so it declares neither block. Stage E1
tried the derivation anyway and a **probe** showed every id coming back empty: the wizard would have
written four values under the empty key and saved a flow with no approvers. So six keys that look like a
duplicate of the live step's are two Machines each answering for itself. The tempting "fix" —
declaring `sequencing:` on the template so one derivation serves both — would assert locking behaviour
a template does not have, and is wrong for that reason rather than on taste. **When a derivation returns
empty, check whether the Machine can legitimately answer before making it.**

**And a Workspace's own ids are still reserved against itself.** Isolation relaxed exactly one
thing — the same name in a *different* Workspace. Two Machines under one id inside one Workspace
stays a load-time error (`metadata.validateMachineIDsAreUnique`), and `aiassist.Validate` refuses
it before a generated Application is ever written.

**Installing a template resolves an id collision by renaming it (2026-09-28).** `internal/installer`
copies a library template into a Workspace's own directory, renaming any **Machine id** or
**Application id** that Workspace already uses — `mch_document` becomes `mch_document_approval` — and a
Workspace with no collision gets a **byte-identical copy**, which is what keeps "install then diverge"
readable as a diff. Reached from `/install-application` (workspace-admin only), or from
`installer.PlanInstall`/`Install` directly. Renaming is safe only because the two changes above removed
every Go-side meaning of both kinds of id; **a collision on a navigation id, a navigation route or a
Dataset id is refused with a reason instead**, because `routeByID("nav_...")`,
`Workspace.ApplicationForRoute` and `internal/composition`'s `ds_*` constants still name those from Go.
Do not "fix" one of those refusals by renaming — that reintroduces exactly the coupling the two slices
above deleted.

This paragraph used to say a Workspace already using one of a template's ids "cannot take that template
at all", and that was true for two days. What is still true: the library (`metadata/*.yaml`,
`metadata/applications/*.yaml`) is **only ever read**, an install is load-verified through the real
loader and rolled back if the result does not load, and the assistant's own publish path anticipates
collisions when generating (its prompt carries every Machine id already taken in the target Workspace,
not only the Application-claimed ones).

**A capability the runtime needs from metadata must be declared, or the copy model silently diverges
(2026-09-30).** Installing copies, so **a later change to `metadata/*.yaml` never reaches a Workspace
that installed earlier** — that is the isolation model working, and it is also how a Go-side requirement
becomes a per-Workspace outage.

Tahap A moved the Document↔Step correlation into a declared Relation, `ds_documents_with_steps`, adding
it to the library template and to `default`'s own copy. `hanomerch` and `dokter-kecil` both cast the
approval engine's roles, neither had the Dataset, and `composition.selectRecords` answers a missing
Dataset with an error — so every approval screen in both returned **500, unconditionally, for a day**.
Nothing was red: the handler was registered, the YAML was valid, every workflow derivation resolved, and
the suite was green. **A requirement that lives only inside a Go constant is reachable only through the
failure it causes.**

So an engine now *declares* what it selects through: `domain.WorkflowEngineSpec.Datasets` maps a role to
the Dataset ids the Machine cast in that role must provide, checked at load by
`metadata.validateWorkflowDatasets` — by **role**, never by file or id, which matters because
dokter-kecil's `document` role is `mch_document_approval` while its own unrelated `mch_document` is the
unbound Machine that has panicked two pages. A load error, for the same reason a missing Required role
is: an engine that cannot select its own records cannot run, and refusing to start beats serving a
screen that throws. Note what that costs and accept it deliberately — one Workspace's missing Dataset now
stops the **whole process**, which is how every other invalid manifest already behaves (005 Phase 3) and
why `internal/installer` load-verifies and rolls back.

**The general rule: when you add a Dataset id (or any metadata key) that Go names, ask which Workspaces
already installed the Machine that must declare it.** The library is a template, not a live reference.
Adding the id to `metadata/*.yaml` is half the change; the other half is every
`metadata/workspaces/<slug>/` copy, and the declaration that makes the loader demand it instead of
trusting you to remember.

**A store method that takes an id takes the Workspace too (2026-09-29).** Workspace scoping is the one
invariant this whole data layer rests on, and it is enforced *in the statement* --
`records.workspace_id` on every read and write, `data.WorkspaceScope` on ctx. `Store.UpdateAISessionStatus`
was the exception: `UPDATE ai_sessions ... WHERE id = $1`, relying on its callers to have resolved a
scoped session first. Four of five did. The fifth (`discardNewApplication`) passed the raw URL parameter,
so a Workspace admin could discard **another Workspace's** draft Application by id — random ids are not
scoping, and this repo has rejected "hard to guess" as a guard before.

So: **put the Workspace in the `WHERE`, not only in the caller.** Relying on every caller to scope is a
rule that holds until one does not. The handler was made consistent with its siblings *as well*, because
a handler that reads like its four neighbours is how the next one gets written correctly — but the store
predicate is the half that survives a future caller being built wrong, which is the same reasoning
`installer.RefuseIfExists` states for itself.

Concretely, before adding any hardcoded `href`, label, or button to a page under
`internal/rendering/`: check the installed Application's own `navigation:` list first. If the
same route/label is already declared there, that's a signal the value belongs in metadata (or
should be read from the `Navigation` the handler already has, e.g. `internal/web/router.go`'s
`d.Navigation`), not typed again in the `.templ`. If you add it anyway because metadata can't
express it yet, say so in a comment — don't leave it silently duplicated.

## Where a metadata-derived value belongs

When something a handler or a page needs should come from metadata rather than be a literal, it
has exactly one home: a named, doc-commented field on `domain.Application` (or `domain.Workspace`)
-- not a local variable computed ad hoc in a handler, not a string re-typed in a `.templ`. The
full chain, worked example `HomeRoute`:

1. An Application's own file declares it (`home_card: true` on a navigation item).
2. `internal/metadata.LoadApplication` resolves it once, at load time, into a field on
   `domain.Application` (`HomeRoute`) -- doc-commented with where its value comes from and any
   ordering subtlety (here: resolved *before* `hidden_nav_groups` filtering, the same reason
   `PrimaryNavGroup` already is -- deriving it later from the filtered `Navigation` slice would
   silently go blank whenever that item's own group is hidden).
3. `cmd/server/main.go` copies it onto `web.Deps` (`Deps.HomeRoute`) -- Deps is the composition
   root's own registry of what a handler may depend on, so a value skipping this step can't reach
   a handler through anything but a literal.
4. The handler (`internal/web/workspacehome.go`) takes it as an explicit parameter and passes it
   straight through to the rendering function.
5. The `.templ` (`internal/rendering/workspacehome.templ`) takes it as an explicit parameter and
   renders it -- never `href="/approval-inbox"`.

`domain.Application`'s own fields are therefore the inventory: before hardcoding something that
looks like application behavior, check whether it's already a field there. If it should be
metadata-driven but isn't yet, add the field there first (with a doc comment explaining where it
comes from), wire it through the four steps above, and only then reference it downstream.

**A page linking to one of its own sibling screens is the common case, and has a shortcut**: call
`rendering.routeByID("nav_xxx")` (`internal/rendering/machine.templ`) instead of retyping the
route, and `rendering.labelByID("nav_xxx")` instead of retyping its `label:` text (a page's own
title/`<h1>`/card text is exactly as much metadata as its link target -- see the next section).
Both look the id up in `domain.Application.AllNavigation` -- the full declared navigation list,
frozen before `hidden_nav_groups` filtering runs, the same "before filtering" reasoning
`HomeRoute`/`PrimaryNavGroup` already follow, so a link to a hidden group's own item still
resolves. This isn't limited to Workspace-level chrome: `approvalinbox.templ`'s "+ New Approval",
`documentsubmit.templ`'s "← Approval Inbox" and `sprintdashboard.templ`'s "Full team capacity →"
all use it -- an *Application's own* page linking to its *own* sibling screen still means the
route/label comes from metadata, not from assuming "same Application, so hardcoding is fine."
Checking the actual data proved that assumption wrong
(`internal/conformance.TestRenderingHasNoHardcodedApplicationRoute` /
`...ApplicationLabel` cover every `.templ` file for exactly this reason, not just Workspace-level
ones -- the label gate alone found nine live violations the day it was added: every Page's own
`pageShell(...)` title was retyping its nav item's `label:` on a line right next to an already-
correct `routeByID` href).

## Deciding whether a literal is a metadata-hardcoding violation

Use this decision path any time you're about to write a string/number literal that looks like it
describes application behavior (a route, a label, a Machine/Field id, an option value, a badge
name) -- not just for routes and labels, which are the only two `internal/conformance` currently
gates by name.

1. **Is it declared in `metadata/*.yaml`?** If yes, it must be referenced through an id-keyed
   lookup -- `rendering.routeByID`/`labelByID`, or a doc-commented `domain.Application`/
   `domain.Workspace` field threaded through `web.Deps` (see "Where a metadata-derived value
   belongs" above) -- never retyped as a second literal. This is true even when it's *your own*
   Application's own screen doing the retyping; "I already know the value" is exactly the
   assumption that produced the nine label violations found above.
2. **If metadata genuinely can't express it yet**, hardcoding is allowed, but the site must:
   (a) say so in a comment naming the missing capability (already required, above);
   (b) cite a forward-checkable pointer -- a `007-composable-runtime-architecture.md` section
   number or a `ROADMAP.md` phase -- so a later reader can check whether that capability has since
   landed, rather than trusting the comment forever; and
   (c) be listed in `writing-guide.md`'s "What comes free vs. what's hardcoded today" table.
   `internal/composition`'s Case 19 Machine-id constants predate this convention -- treat any
   *new* exception you add as needing all three, and flag an old one you touch that's missing
   them.
3. **If the same hardcoded shape is now needed a second time** (a second Machine, a second case),
   that repetition is the trigger to run it through the `menata-app-document` companion repo's
   decomposition criteria *before* adding a third: `workflow-behavior-decomposition-criteria.md`
   (B1-B5) for business logic in `internal/action`/handlers, `ui-composition-decomposition-
   criteria.md` (Q1-Q5) for rendering fragments in `internal/rendering`. Both were written for
   exactly this repo and already have real worked examples (`ActionDecide` generalizing into
   `ActionEdit`/`ActionDelete`; `logRecordCreated` generalizing into `Event.OnCreate`).
4. **Capability keeps growing, so a standing exception isn't permanent.** At each `ROADMAP.md`
   phase close, re-check exceptions already in the codebase (`internal/composition`'s Case 19
   constants, `internal/conformance`'s `runtimeLevelRoutes`/label allowlist if one exists) against
   *current* metadata capability, not the capability that existed when the exception was written --
   `card_fields` (Projection) and `Event.OnCreate` both landed by generalizing something that
   was excused as "metadata can't do this yet" not long before. An exception with no forward
   pointer (step 2b) can't be checked this way, which is exactly why that pointer is mandatory
   going forward.

## Design reference vs. current code

`ui-sample/*.html` is the design target (`internal/web/router.go`'s own comment: "a design
reference, never current-code intent"). When a page under `internal/rendering/` is meant to match
one of these mockups, match it structurally — don't add elements the mockup doesn't have (e.g. an
extra CTA button on a card) just because older code already had one. If porting a mockup surfaces
a pre-existing hardcoded value that should have come from metadata, fix it as part of the port
rather than carrying it forward unreviewed.

## The component inventory: `capabilities.md`

`capabilities.md` is where every composable piece is already catalogued — README.md's own
description: "what the runtime can actually do right now." Before adding a Machine concept, a
Field type, a route, or a reusable rendering piece, check the matching table there first
("Composition primitives", "Shared rendering components", "Machines currently defined", "Field
Types") — "does something like this already exist?" is answerable by reading a table, not by
grepping the whole tree. When you add something that belongs in one of these tables, add the row
in the same change, not as a follow-up — `internal/conformance` (below) fails the build if the
Machines and Shared rendering components tables drift from the code, so an unrecorded addition is
a matter of when, not if, it gets caught.

## Enforcement, not just prose

`internal/conformance` holds executable tests for architectural obligations 001-007 and
`capabilities.md` state in prose (plane boundaries, handler size, metadata/code/doc alignment).
Prose gets skimmed; a failing `go test` doesn't. Currently gated, by name (`go test
./internal/conformance/... -run <name>` to run just one):

- `TestPlaneBoundaries` / `TestEveryPackageHasARule` — import-boundary obligations per package.
- `TestHandlersStaySmall` — the `internal/web` handler-size budget.
- `TestAppManifestLoads` — `metadata/workspaces/default.yaml` parses and validates.
- `TestNavigationRoutesAreRegistered` — every declared `navigation:` route has a real
  `internal/web/router.go` handler, across **every** installed Workspace (`metadata/workspaces/*.yaml`
  is scanned, the way the loader scans it). It read `default.yaml` alone until 2026-09-29, so the five
  routes the dokter-kecil install declared were checked by nothing. A `/machines/<id>` route is the one
  shape judged differently and more strictly: its handler is registered under the `{machineID}`
  *pattern*, so what is checked is that the id names a Machine **that Workspace installs** — a nav item
  pointing at an absent Machine is the real failure, and a literal-handler check cannot see it.
- `TestRenderingHasNoHardcodedApplicationRoute` / `TestHandlersHaveNoHardcodedApplicationRoute` —
  no `internal/rendering/*.templ` file and no `internal/web` handler (router.go excepted) may
  hardcode a route metadata already declares, except the small set of runtime-level routes that
  exist regardless of which Application is configured (`/home`, `/login`, ...). Use
  `rendering.routeByID("nav_xxx")` to link to a sibling screen instead.
- `TestRenderingHasNoHardcodedApplicationLabel` / `TestHandlersHaveNoHardcodedApplicationLabel` —
  the label-side counterpart of the pair above: no `.templ`/handler may hardcode a `label:`
  metadata already declares (a page's own title, `<h1>`, or card text). Use
  `rendering.labelByID("nav_xxx")` instead.
- `TestRenderingHasNoHardcodedPageHeading` — the third of the family, and the one that exists
  because the label gate structurally could not see its class. A navigation item declares three
  strings, not one: `label:` (what a menu says), `title:` (what the screen says about itself) and
  `description:` (the line under it). The label gate matches Title-Case phrases, which every
  `label:` is and no heading or sentence ever is, so subtitles sat as raw text in a `.templ` the
  gate was passing. This one matches the declared string verbatim, with `//` comments stripped
  first. Use `rendering.titleByID`/`descriptionByID`.
- `TestRenderingUsesProjectionNotRawValues` — the one *ratchet* in this suite: a `.templ` file may
  not read a named field off a record (`Values["fld_..."]`, or the same laundered through a
  `action.Field*` constant) — Composition resolves the shape, a Page renders it (007 §4.4, §7.6).
  `projectionRatchet` grandfathered ten files when it was written, seven for most of its life, one from
  2026-09-28, and **none since 2026-10-05** (Board Settings was the last; an empty ratchet is an ordinary
  gate, so the map stays declared). **The list may only shrink**: adding an entry is not the way to
  pass, and an entry left behind after a file is migrated fails too. Read the count out of the test,
  not out of this line. Use
  `composition.ProjectCardFields`/`card_fields`; generic access (`Values[f.ID]` from ranging
  over `m.Fields`, as `machine.templ`/`detail.templ` do) is the target pattern, not a violation.
  It gates *reads only* — an input's `name=` is 007 §11.3 Binding, ungated, so leaving this list
  is not the same as the screen being composable.
- `TestQueryDiagnosticsRunsBeforeAuth` / `TestPoolInstallsQueryTracer` — the read diagnostic's own
  wiring. It must be the *first* middleware in the authenticated group (it installs the ReadLog on
  ctx, so anything registered above it is counted by nothing), and `internal/db` must install the
  `pgx.QueryTracer` it is handed. Both failures are invisible at runtime: the app works, the log
  just quietly reports a smaller number. That is what happened between Phase 18 Step 3 and
  2026-09-22.
- `TestDiagnosticLineSurvivesAPanicAndMarksIt` / `TestDiagnosticLineCarriesMethodStatusAndRoutePattern` /
  `TestAnomalyPrefixMarksFailedRequests` / `TestSlowStatementIsLoggedByTextAndNeverByArguments` -- the
  diagnostic's own contract (2026-10-05), beside the wiring pair above. The line is written from a `defer`
  because a panic unwinds past a log written after `next.ServeHTTP`: the most broken request was the one
  line missing, and a page that failed early looked *cheaper* than one that worked. `route=` is chi's
  pattern, not the path. `SLOW-QUERY` logs SQL text and never arguments. `db.ReportPool`'s `POOL` line
  marks only `canceled` as an anomaly -- `empty` acquires are normal on a cold pool, and a marker that
  fires on healthy traffic is the `unnamed` mistake again. A log-review script must pick fields **by
  name** (`menata-app-document/guides/log-review.md` appendix B), since three line formats coexist in any
  window that crosses a deploy.
- `TestGetRoutesDoNotWrite` — no handler registered for GET may reach `CreateRecord`/
  `UpdateRecord`/`DeleteRecord`, walking the call graph across `internal/web` and
  `internal/composition` (the one that prompted this was four calls deep). Exactly one exception
  is declared, `readPathWriters`, and it carries its reason plus a forward pointer; a stale entry
  fails too.
- `TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed` (`internal/web`, needs `DATABASE_URL`) — the
  second *ratchet*. It sweeps every authenticated GET route taking no path parameter (discovered
  by parsing `router.go`, so a new route is covered without anyone remembering) and holds each to
  two invariants: `queries == reads` (every statement named itself) and `repeated == 0`.
  `getSweepRatchet` started at seven routes and **is now empty** — read that out of the map, not
  out of this line. **The list may only shrink**: adding an entry is not the way to pass, and an
  entry left behind after a route is fixed fails too. An empty ratchet is an ordinary gate, not a
  retired one, so the map stays declared. Writes are deliberately out of scope:
  `composition.Loader` may not span a mutation, so a POST re-reading after its write is correct.
  **Neither invariant can see an N+1**, which is the sweep's own blind spot: a per-row loop repeats
  nothing when there is one row. `TestSwitchWorkspaceCostIsFlatInWorkspaceCount` is the shape of
  assertion that catches those — same request, more rows, same cost. `TestAuthenticatedPageQueryCost`/`TestNavBadgeQueryCost` sit beside
  it with per-route *budgets* — kept to two on purpose, since a budget is a threshold that rots
  while the two invariants above do not.
- `TestCheckDocsMirrorMetadatasOwnKeys` — `internal/installer`'s strict-decode targets
  (`FullMachineCheckDoc` and friends) are hand-mirrors of `internal/metadata`'s own unexported
  `machineDoc`/`applicationDoc`/`workspaceDoc`, and every metadata write goes through one. A key added
  to the real doc and not the mirror makes the write reject a file the *loader* accepts. Not
  hypothetical twice over: `blocks_member_removal` (2026-09-27) broke the first template install, and
  writing this gate immediately found `suggested_applications` missing too — which would have failed
  every AI publish into `default`.
- `TestCapabilitiesMachinesTableMatchesMetadata` / `...ComponentsTableMatchesTempl` —
  `capabilities.md`'s own Machines and Shared rendering components tables match the real
  `metadata/*.yaml` and `internal/rendering/*.templ`.
- `TestInstalledApplicationsAreCopiesNotSharedFiles` / `TestTwoWorkspacesCanHoldDifferentMachinesUnderOneID`
  — Workspace isolation, both halves. Every Application a Workspace installs, and every Machine
  those Applications claim, must be that Workspace's own copy under `metadata/workspaces/<slug>/`,
  with a short closed allowlist for the three runtime-level Machines that stay shared
  (`mch_user`, `mch_activity`, `mch_notification`); and two Workspaces must be able to hold
  *different* Machines under one id, asserted against the real loader.
- `TestNoBareMachineIDIdentityChecks` / `TestNoApplicationIDIdentityChecks` — the two halves of "a
  name is not an identity". No code outside `internal/action` may decide "is this Document
  Approval's Machine" by comparing against `action.DocumentMachineID` and friends; and no code —
  `internal/action` **included**, since that is where it lived — may compare an id against an
  `"app_..."` literal. Use `action.IsDocument`/`IsStep`, which read the Application's own declared
  `workflow:` binding. Only comparisons are gated: building a URL from those constants, looking a
  Machine up by id, querying records by one, or using an `app_...` string as a map key or column
  value all name something rather than claim an identity (`internal/web.legacyAppRoleApplicationID`
  is the documented case of the last one).
- `TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames` / `TestUnboundMachinesAreNotTheEngines`
  — the property those two gates exist to protect, asserted through the real loader: an Application
  called `app_persetujuan` over `mch_surat`/`mch_langkah` engages the approval engine because it
  *declares* the binding, and every Machine whose Application declares none — including
  "dokter-kecil"'s own unbound `mch_document`, the one that panicked two pages — engages nothing.
- `TestMachineFixturesPassProductionValidation` (`internal/web`, `composition`, `action`, `behavior`,
  `authorization`) — every Machine fixture goes through `metadata.Normalize` + `Validate`, the loader's
  own pipeline, so a fixture cannot describe a Machine the runtime would refuse to load. **Normalize
  first**: the `person` → `mch_user` binding is an inference (001 #6, 005 Phase 4), so asking a fixture
  to hand-write `RelatedMachine` inverts the principle — call `metadata.Normalize`. This is a *sweep*
  where the mirror gate is a named list, because validity needs no judgement and mirroring does; it does
  **not** catch a fixture that is merely looser than the real Machine.
- `TestPostRoutesRefuseUnauthenticatedAndUnCSRFed` (`internal/web`, needs `DATABASE_URL`) — the write
  side's structural sweep, over **every** POST including the public ones. An authenticated POST must
  refuse a request with no session and 403 one with no CSRF token; a POST on the public router must be
  in `preIdentityPostRoutes` or `pendingIdentityPostRoutes`, both closed, both failing on a stale
  entry. The receiver is *asserted*, not filtered on — a version that discovered only `pr`/`ar` routes
  could not see a write moved to the public router, because the route just left the population. It is
  a middleware-wiring guarantee, **not** behaviour coverage; say so rather than reading a green run as
  "the write side is tested".
- `TestPerRecordGetRoutes` (`internal/web`, needs `DATABASE_URL`) — the *other half* of the route
  table. The sweep above covers GET routes with no path parameter; this one seeds a record per
  installed Machine and covers the 12 that take one, both directions: a route valid for this Machine
  must render, and a Machine in no workflow role must still 404. A per-record route with no case
  fails, so a new one lands in one sweep or the other rather than in neither. It exists because
  `/review` 404'd on every Document for a day in production while failing in a shape it is supposed
  to have — nothing logged, nothing panicked, no test failed. Resolve a role-gated route's Machine
  with `MachineInWorkflowRole`, never by naming `mch_document`.
- `TestFixturesMirrorTheRealMachines` (`internal/web` and `internal/composition`) — a test fixture
  standing in for a real Machine may not declare fewer *rule* blocks than it
  (`domain.Machine.DeclaredRuleBlocks`: permissions, transitions, actions, events, sequencing,
  constraints, the two signature blocks, `blocks_member_removal`, `append_only`). A fixture missing one
  does not fail — it passes against a Machine looser than the one that runs. Presence, not equality:
  fixtures are deliberately smaller and deliberately renamed, and structure/presentation blocks are
  excluded so a reduction stays a reduction. When you add a block to a Machine, add it to the mirror in
  the same change. The population is a named list per package, so a *new* fixture is uncovered until
  someone adds it there.
- `TestClosedRegistryMembersAreAcceptedByTheLoader` / `...AreActivatedByMetadata` — a new
  capability must arrive with its metadata seam, not just its Go. Every `domain.KnownActions`/
  `registry.Services` member must be (a) accepted by `internal/metadata`'s own validation, since the
  registry and that validation are two lists that drift, and (b) actually named by an installed
  Workspace's metadata — a capability no manifest can start is one only Go can reach, which is
  001 #3 inverted. `registry.KnownWorkflowEngines` is held to (b) as well, and deliberately not to
  (a): its validator reads the registry itself rather than repeating it in a switch, so it has no
  second list to drift from.
- `TestDocumentApprovalFieldCouplingOnlyShrinks` — the Field-side twin of the ratchet below, frozen at
  129 references across 13 files the day Stage B (2026-09-28) gave an Action a way to declare what it
  writes, and 126 across 14 by that evening when Stage C moved the compositing operation out of
  `internal/web`. Both ratchets carry a **total** as well as per-file numbers, for one reason: a
  relocation adds a file and subtracts from another, and "a new file is not the way to pass" is exactly
  the wrong advice for that. When the total falls the message says so and says what to do; the map is
  still authoritative, so every change to the population fails until it is updated (`actions:`, `domain.ActionEffect`). Same shrink-only terms. It exists because the Machine-id
  ratchet could not see what Stage B removed, which made this repo's own prediction ("a stage that works
  makes the number drop") false as the gate stood — so the gate was extended rather than the claim
  restated. **Read the current numbers out of the map, not out of this line** — the trajectory since is
  129 → 126 → 78 (Stage D's derivations) → 43 (the signature shape) → 29 (Stage E1's wizard
  derivations) → 11 (Stage E2's flow-template shape) → 9 → 4 → **1**.

  This line used to end "and none of it is a derivation away: a step's *label*, which nothing declares
  at all, and a form input's `name=`, which is 007 §11.3 Binding with no primitive yet." **Both halves
  were wrong, in the two different ways worth knowing about.** The label was not a missing declaration
  but a Field nobody filled — `fld_step_name` was empty in 0 of 23 records, collected by no mockup, a
  free-text stand-in for a job title, so it was deleted rather than declared. The `name=` attributes
  were not waiting on a primitive at all: the *read* side of that same form had derived those ids from
  `action.DeclaredFields` since it was written, and Stage D had already shipped the exact shape next
  door (`rendering.PlacementFields`), so the fix was a struct and a parameter. What is left is
  `rendering/detail.templ` (1), a props decision on an already-ten-parameter signature.

  **The rule to carry, since this line asserted the opposite twice:** a deferral is a measurement with
  an expiry date. "No primitive yet" is grep-checkable in a minute; "nothing declares it" is answerable
  by querying the Field. Neither survives being written down and trusted — and a *frozen* count is what
  makes stale prose beside it read as settled. When a stage ships a primitive, re-measure every deferral
  phrased that way, not only the ones the stage names.
- `TestDocumentApprovalCouplingOnlyShrinks` — the third *ratchet*. `documentApprovalCoupling`
  freezes how many times each file outside `internal/action` names Document Approval's own
  Machine-id constants — frozen at 67 across 18 files on 2026-09-28 and **emptied the same day**, by
  the slice that made a Machine resolvable by the role its Application casts it in. Read the numbers
  out of the map, not out of this line. **It may only shrink**: a higher count fails, a new file
  fails, and a *lower* count fails too, so an improvement is locked in rather than left as room to
  regress. Comment lines are skipped, so explaining the shape a file used to have does not count
  against it. An empty map is an ordinary gate rather than a retired one — the next file to name one
  of these constants fails, which is the whole point of leaving it declared.

  Ask a Machine for its **role**, not its id: `internal/web.machineForStep`/`machineForDocument`
  (and `approvalMachine(ctx, role)` for the rest), `composition`'s own parameters, and
  `rendering.approvalMachineID(ctx, role)` in a `.templ`. All three read
  `domain.Workspace.MachineInWorkflowRole`, whose doc comment carries the precedence — the request's
  Application first, then the Workspace's sole binding, then nil for "this Workspace has no such
  screen". A nil is a real answer: an optional role (a signature store, a saved approval flow) is
  uncast whenever that feature was simply not installed.

- `TestInstalledCastsExplainWithoutDefects` / `TestEveryDerivationIsOwedBySomeRole` /
  `TestEveryDerivationAccessorIsExplainedOrExcused` — the inference-inspection family (001 #6's second
  clause: *"**Inference must be inspectable.** … Hidden inference that cannot be explained is not an
  acceptable substitute for explicit configuration"*). The first runs `action.ExplainCast` over every
  installed Workspace and fails on a **defect**, which is exactly two of the four statuses: `undeclared`
  (a Machine declares nothing for a question its role owes) and `input unavailable` (the declaration
  exists, the caller supplied nothing to resolve it against — the `/review` 404's own shape).
  `not applicable` is **not** a defect and must never be treated as one: it is the majority, and the
  reason the other two are visible at all.
  The second holds `domain.Derivation*` against `WorkflowEngineSpec.Answers`, reading the constants out
  of the source rather than retyping them. The third holds the *accessors*: a new exported function in
  `internal/domain/actioneffect.go` or `internal/action/fields.go` that `explain.go` never reaches
  becomes an inference nothing can explain, silently. It is a closed map with a reason per entry
  (`unexplainedDerivationAccessors`, `readPathWriters`' shape) rather than a sweep, and the map was
  chosen by **measurement**: a naive "every accessor must be explained" gate produced 4 findings of
  which ~1.5 were real, the same ~50% false rate that got the static fixture-discovery gate deleted. A
  stale entry fails, and so does one naming a function that *is* now explained. **When adding a
  derivation, add its `Derivation*` constant, the role that owes it in `Answers`, and its branch in
  `explain.go`'s `valueOf`/`sourceOf` — all three, or one of these three gates fails.**
- `TestWholeMachineReadsOnlyShrink` / `TestPerViewerDatasetsScopeByIdentity` — the record-selection
  pair (2026-09-29). The first is a **ratchet** over `internal/composition`: how many times each file
  reads an entire Machine, shrink-only on the usual terms (a higher count fails, a *lower* one fails
  until the entry is lowered, a new file fails). It measures what 007 objects to in three places —
  §28 invariant 4, §4.4, and §20, which names the shape with the word *never*. **A falling number is
  not a composable Data Plane**: ten of the remaining reads correlate two Machines and wait on 007
  §7.5 Relation, four legitimately read everything.
  The second guards a class that became *silent* when it became declarative: a Dataset whose whole
  purpose is showing one identity its own records must declare a `$current_user` predicate. That
  scoping used to be a line in Go where deleting it failed a unit test; deleting it from metadata
  leaves the Dataset loading, the screen rendering, and every viewer seeing everyone's records —
  measured, the whole suite stayed green. A named list (`perViewerDatasets`), because "is this
  per-viewer" is a judgement no scan can make.
  **When declaring `select: records`**: `limit:` is required (007 §7.9), `where:` values may name only
  `$current_user` or `$parameters.<name>` (§9.2 — anything else is a load error, on purpose), and
  `sort:` may name a Field or a record column (`created_at`, `updated_at`, `sort_order`).
- `TestServiceRegistryAndExecutorsAgree` / `TestRegistryDependsOnDomainOnly` /
  `TestDomainHoldsVocabularyAndRegistryHoldsDispatch` — the dispatch-seam family (2026-09-30,
  `internal/registry`). **The line it draws is mechanical, which is why it is gateable:** a closed set
  whose value is `bool` is *vocabulary* ("is this string legal") and belongs in `internal/domain`; one
  whose value carries **structure** — a validator func, a cast, a contract — is a *dispatch seam*
  (007 §14) and belongs in `internal/registry`. Two have moved so far: `Services` (was
  `domain.KnownServices`, names only, plus a validating `switch` in `internal/metadata` and **three
  more** in `internal/execution`) and `KnownWorkflowEngines` with its `Answers`.
  **`internal/registry` may import `internal/domain` and nothing else**, and that is a constraint, not a
  preference: `internal/metadata` must import it to validate and is forbidden from `internal/data`;
  `internal/execution` must import it to dispatch and is forbidden from `internal/metadata`. So a
  `Service` carries its **validator** and never its executor — the executor stays in `internal/execution`
  and `TestServiceRegistryAndExecutorsAgree` is the only thing binding the two. **That is a weaker claim
  than "one registration point" and it is the true one**; read `registry.Service`'s own comment before
  trying to move `Execute` in.
  `domain.WorkflowRole*` and `domain.WorkflowEngineDocumentApproval` deliberately stayed in `domain`:
  those are names an Application writes in its own YAML, vocabulary rather than dispatch.
- `TestLayoutRenderersHaveNoPerScreenBranch` / `TestLayoutVocabularyIsRenderedAndUsed` /
  `TestLayoutClassesStayInRendering` — the Experience Plane's first primitives (007 §12.2, §12.6), built
  2026-10-02. The first is **the only real test of whether a layout primitive is a primitive**: a generic
  renderer that must know *which screen* calls it is one screen's shape with a new name, and §12.2 names
  that failure from the other side ("should not require separate layout concepts such as
  `DashboardLayout`…"). The third holds §15.2 — `internal/domain` carries the vocabulary, the Tailwind
  mapping lives only in `internal/rendering`.
  **The second gate was too weak on its first run and that is the lesson here.** It checked only that
  `pageHeader` had a caller, and passed while `rowLayout`, `gridLayout` and `panelLayout` sat with **zero**
  — the unused grid renderer having already put a three-column desktop class into the shipped CSS bundle, a
  class no screen uses. All three were removed, and the gate now requires every declared kind, every Gap
  step **and every `GridCols` member** to be both drawn by a renderer and named by a caller. **A primitive arrives with the uses it replaces**, or it is a name with nothing
  behind it; the measured counts for the unbuilt kinds are in `domain.LayoutKind`'s comment so the next
  slice migrates rather than re-declares.
  **And a warning that cost two wrong conclusions in two days**: when deciding whether something is a
  primitive, do not count class strings. `row` and `grid` were written off as "not primitives in this
  corpus" because 21 and 6 sites spanned many class strings — but class-string variety is *configuration*
  variety, which is exactly what a parameterised primitive absorbs. §12.3 forbids a component accepting
  **arbitrary** properties, not enumerated ones. Re-measuring by *meaning* found `split` (which §12.2
  lists and the rejection said had zero uses) at **5 sites**, and only 2 genuine one-offs. The first
  measurement counted frequency and not uniformity; the second counted uniformity and not
  parameterisability. Neither was caught by a gate — both were caught by someone asking whether the
  conclusion was plausible.
- `TestHandWrittenLayoutSitesOnlyShrink` — **the one gate here that directs work rather than only
  preventing regression**, and the odd one out for that reason. It freezes, per file, how many layout sites
  are still hand-written: measured at **45 across 20 files** on 2026-10-03 (row 33, grid 5, split 7) and
  **40 across 18** the same day once `grid` landed. A *new*
  hand-written site fails and the message names the primitive it should have used; a *lower* count fails
  until the entry drops, so a migration is locked in. The next session finds this work from a failing test
  rather than from remembering to read a plan.
  Counted **syntactically** on purpose — "is this really a row" is a judgement, and a gate making it would
  be the ~50%-false-finding shape deleted twice here. What syntax cannot separate is noted per entry:
  `split`'s floor is **2, not 0**, because `automation.templ`'s `[6rem_1fr]` definition list and
  `rolematrix.templ`'s bordered grid are genuine one-offs, and a one-off is what a primitive is not for.
  **All three reached their floor on 2026-10-03** — `grid`'s key was *removed* (an entry naming a kind with no
  sites fails too), `split` dropped to its two one-offs, `row` to eleven. 45 sites in 20 files -> **13 in
  10**. Read the current numbers out of the map, and **read a floor as a finding rather than as debt**: a site
  stays hand-written when absorbing it would make the primitive accept a CSS class string (§12.3's "arbitrary
  properties"), a second gap ladder for two sites, or a choice of HTML element the vocabulary has no word for.
  Only the last of `row`'s three kinds is a missing capability; the other two are the boundary working.
  **`split` is also the slice to read before migrating `row`, because it is the one that changed what
  renders.** Its five asides were 316/320/320/340/360px: four widths for one idea, so two named steps widened
  two screens (4px and 20px). More importantly three sites gave the main column `minmax(0,1fr)` and two gave
  it `1fr`, which differ in **behaviour** — `1fr` cannot shrink below its content, so the two screens that
  render a `<pre>` of generated YAML carried a latent overflow. A primitive has to pick one answer, and
  picking it is what surfaced the bug. When a migration forces a choice between inconsistent literals, the
  choice is a finding; state which screens move and by how much in the commit, and put the pre-migration
  literal beside the intended one in the test (`rendering.TestSplitLayout_*`) so it stays legible without git.
  One site keeps its htmx wiring on a wrapper **outside** the primitive, costing one DOM node: a Layout
  primitive accepting arbitrary HTML attributes is exactly what §12.3 forbids by name, and a screen's
  self-refresh is behaviour, not layout. `row` did the same for three `role="group"`/`aria-label` filter
  groups — accessibility semantics are not layout, and separating them is how a declared Page would express
  the pair anyway.
  **Two things `row` taught that are not about layout at all.** First, **measure what templ emits before
  writing call sites**: `class={ a, b, c }` does not drop an empty value, it emits a *double space*, and 18 of
  22 sites pass at least one default — the naive form would have left eighteen screens differing from their
  baseline by a space nobody would look for. `rowClasses` joins in Go, and
  `rendering.TestRowLayout_*` asserts no double space. Second, a render-diff over a baseline **finds defects
  that have nothing to do with the change**: this one surfaced `searchBox` ranging over a
  `map[string]string`, so its hidden inputs rendered in a random order between requests — a straight breach of
  007 §4.6 Determinism (a MUST) that broke nothing visible and would have made every future byte comparison of
  that page unreliable noise. It took six fetches of the *baseline* to see it flip once. Fixed with sorted
  keys, held by `TestSearchBox_hiddenInputsRenderInAStableOrder`, which renders 50 times because one render of
  a two-key map passes about half the time.
- `TestHandWrittenChipsOnlyShrink` -- the second directive ratchet, beside `TestHandWrittenLayoutSitesOnlyShrink`, built **after** 25 chips moved to `statusBadge` (2026-10-05; build, migrate, then gate). It counts a `<span>`/`<code>`/`<li>` that is `rounded` with a `-50`/`-100` tint, `px-` and a small text size, including `templ.KV` conditional classes. Four floors remain with a reason each (uppercase typography, the selected tab, a `<code>`). **The first pattern allowed any element and matched ~13 files** -- alert paragraphs, buttons, wells -- so it was narrowed before anything was frozen; what it cannot see (`div`/`a` chips, `pl-`/`pr-` pills) is stated in the test. Mutation-proved for both the plain and `templ.KV` forms.
- `TestHandWrittenMutedParagraphsOnlyShrink` -- the third directive ratchet, built after eleven sites moved to `staticText(StaticMessage|StaticNote)` (2026-10-06). It counts a `<p>` whose class *set* holds `m-0`, `text-sm|xs` and `text-slate-500`; **15 remain across 9 files, every one carrying something the Static kinds must not accept** (inline markup, `leading-6`, per-site spacing or width, an escaped quote), each with its reason in the map. **The census that chose the two kinds matched an exact class literal and found 11 sites; the gate's class-set pattern found 15 more** -- the shape of a pattern decides what it counts, which is the same lesson as `row`/`grid`, from the opposite side.
- `TestEveryThemeRoleHasAReaderOrAReason` -- the Theme-side counterpart of "a primitive arrives with its callers" (2026-10-06). Every `Role` constant in `domain/theme.go` (read by AST) must be named as `domain.<Role>` in a hand-written `.templ`/`.go` under `internal/rendering`, comments stripped, or sit in `unreadThemeRoles` with a reason; **two do now** (`WeightBody`, `BorderDivider`; `RadiusControl` and `InkBody` left 2026-10-06 when `Button` became their first reader, `BorderControl` the same day when `fieldClasses` did), each needing a divider or a table primitive (007 §12.3). It exists because seven roles were declarable, documented and validated while nothing read them -- a YAML that loaded and changed no pixel. Shrink-only both ways (a role gaining a reader fails until its entry goes). Mutation-proved four ways, including that a comment naming a role does not count. **It checks that a reader exists, not that it is right.** `writing-guide.md` marks the two keys `[declared, nothing reads it yet]`.
- `TestHandWrittenCaptionsOnlyShrink` -- the fourth directive ratchet, built after eight `text-2xs` help lines moved to `staticText(StaticCaption)` (2026-10-06). It counts a `<p>` whose class *set* holds `m-0`, `text-2xs` and `text-slate-500`; **11 remain across 6 files, each with its reason in the map** (an apostrophe `{ text }` would escape, a conditional or multi-line run, an inline `<span>`, `mb-2`/`leading-5`, a bordered well). `caption` is the eighth text role and the first one added *because* a ratchet census named it: the plan's 16 exact-literal sites were a count of class strings, and the class-set pattern is what found the 11 floors. Mutation-proved both ways (a new site fails; a migrated one fails until the entry drops).
- `TestHandWrittenButtonsOnlyShrink` -- the fifth directive ratchet, built after 17 `<button>` sites moved to `@button(label, variant, name, value)` (2026-10-06; build, migrate, then gate), and `Button` is the **sixth registered Component** (`registry.validateButton`: `name`/`value` together or not at all, a closed variant set, no `hx-*`/`class`). It freezes two populations: **18** `buttonClasses(` calls outside `controls.templ` (floors, each with a reason -- `hx-*`, `<a>`/`<summary>`/`<label>`, a script hook id, `sig-*`, `data-modal`, per-site spacing; things a Component must not accept, §12.3) and **4** literal `h-9` buttons (`px-3` and a slate-300 outline, not Button's shape). Mutation-proved both ways, and a `//` comment naming `buttonClasses(` is not counted. **Owner decision D8 is open**: a secondary button's outline is slate-200 in the mockups (5:1) and `border.control` is slate-300, so reading the role would shift 14 buttons. Button uses the literal until the owner chooses; the 4 literal buttons are what that choice unlocks. (`BorderControl` has a reader regardless, since 2026-10-06: `fieldClasses`.)
- `TestHandWrittenFieldsOnlyShrink` -- the sixth directive ratchet, built after `controlField` (one literal, 24 sites) became `fieldClasses(ctx)` (2026-10-06; build, migrate, then gate). It counts an `<input>`/`<select>`/`<textarea>` whose own `class="..."` holds a literal `border-slate-300`: **7 remain across 5 files**, each a floor with its reason (the 15px / `h-10.5` mobile touch targets in `account`, `authshell`, `login` and `workspacemembers`, and a `w-16` number box in `signatureplacement`). **`fieldClasses` is a class reader and deliberately not a registered Component**: an input's attributes are 007 §11.3 Binding, which has no primitive, so a Component for it would accept arbitrary properties (§12.3). It gave `BorderControl` its first reader without deciding D8 for buttons. Mutation-proved both ways. Blind spots stated: a class arriving from a Go function (those call `fieldClasses`), and account's read-only email box (slate-200 on slate-50, a disabled look).
- `TestNoHandWrittenSubheading` -- a zero-floor gate (2026-10-06): no `<h1>`..`<h6>` in `internal/rendering` may carry `text-base`; a section's own title goes through `@staticText(domain.StaticSubheading, ...)`, so `theme.text.subheading` moves all of them. **It reopened a verdict this file's own Theme text claimed: `text-base` "has no consistent role" was true of the 11 sites *as counted* and false once each was read** -- seven are one job, two (a menu label, a monogram) are not headings. The 23 body-sized `<h2>` panel titles are a different job and stay hand-written.
- `TestNoHandWrittenHeadingOrEyebrow` -- the same zero-floor shape (2026-10-06) for the two kinds whose *roles* were the finding: `theme.text.heading` and `theme.text.eyebrow` were declarable, validated, documented in `writing-guide.md`, and **read by no renderer** -- seven declared Theme roles still have no reader (`RadiusControl`, `WeightBody`, `BorderControl`, `BorderDivider`, `InkBody`, plus these two until this slice). Measured by grepping `domain.<Const>` in non-test Go/templ with comments stripped. The gate flags a `<h1>`..`<h6>` carrying `text-xl` and a blue `text-3xs` uppercase span without `font-medium`, and (from the same day) the faint `text-slate-400` form of that span, which is `@staticText(domain.StaticOverline, ...)` -- `eyebrow` role, `faint` ink -- and an `<h2 m-0 text-sm font-medium>` panel title, which is `@staticText(domain.StaticPanelHeading, ...)` (body role, emphasis weight; separate from `subheading` pending owner decision D7). The vocabulary gate derives a kind's Go name by title-casing each hyphen-separated word, because it used to capitalise only the first letter and a hyphenated kind could never match. Mutation-proved on one site of each. **The test to ask of any new Theme role: who *reads* it, not who may write it** -- the `ToneWarn` lesson from the other direction.
- `TestComponentRegistryAndRenderersAgree` / `TestRegisteredComponentsStayBounded` /
  `TestRenderingDoesNotReadTheClock` — the **Component contract** family (007 §12.3, §13, §14), Stage 2 of
  the Experience Plane, 2026-10-03. `registry.Components` is the catalogue: identity, the §13 contract, and a
  validator. The renderer stays in `internal/rendering` for `registry.Service`'s own plane reason, and the
  first gate binds the two across that boundary — a contract naming a renderer that does not exist is a
  Component nothing can draw, and one with no caller is the shape-before-need that put three zero-caller
  layout primitives in this tree a day earlier.
  **The second gate is the one that does work, and it reads a signature rather than a call site.** §12.3's
  normative sentence is *"MUST NOT silently acquire additional business data that is not represented by its
  contract"*, and boundedness is a property of the renderer's parameters: a Component declaring no
  `DataRequirements` may not take `any`, `*data.Record`, `*domain.Machine`, `context.Context` or `time.Time`.
  That is unenforceable against an ordinary templ function — every signature is legal — which is the honest
  answer to "what does a registry buy that a function does not".
  **What it caught, and why it is worth reading before registering a second Component**: `slaBadgePill(v any)`
  drew a status badge while parsing `"2006-01-02"` out of its argument and calling
  `experience.EvaluateSLA(due, time.Now())`. Three defects in one signature — an untyped input, a business
  rule evaluated in the Page (§4.4), and the clock (§4.6 Determinism, a **MUST**: the same record rendered
  "1 day left" and later "OVERDUE" with no input having changed) — and `internal/composition` was already
  evaluating that rule correctly in four places with an injected `now`, so it was 001 #8 as well.
  `experience.ResolveSLABadge` does it where `now` is an argument. The third gate forbids the clock in
  `internal/rendering` outright; `time.Time` as a **parameter** is the fix, not the violation.
  **Unifying two markups for one semantic state changed what four screens draw** — overdue was red text with
  no pill and is now a red pill, which `ToneBad` already meant on the eight sites using the shared shape.
  Same class as `split`'s four aside widths: when a contract forces a choice between inconsistent literals,
  the choice is a finding, so state it.
  **The second member answered the plan's own question, and the answer was "no".** `Avatar` arrived with four
  measured sites and two closed parameter sets, and the contract did **not** have to change structurally:
  `Inputs` is a slice, both validators share one derived helper (extracted on the second case, not the first).
  What it *did* expose is `Accessibility`. For `StatusBadge` that field describes an absence — the text is the
  name. An avatar's content is initials, "SI" is not a name, and **none of the four sites carried an accessible
  name at all**, so the contract declares `label` *required* and the renderer spends it on `aria-label` plus
  `title`. A field that only ever described absences would have been decoration; this made it a requirement,
  and fixed four screens nobody had reviewed for it. **When registering a Component, ask what its contract
  forces that a reviewer would not have asked for.**
  It looked like it also gave a **second case** for "a Component cannot choose its element" — `appshell`'s
  account avatar is a `<button>`, beside a `row` site that is a `<form>` and one that is a `<span>`. **Measured
  the next step, that trigger dissolves, and the claim above it was wrong when first written here.** Those are
  not three cases of one capability; they are one case each of three:
  a bare element swap (`reviewdocument`'s `<span>`, which a closed element enum would fix); an element plus
  form binding (`installapplication`'s `method`/`action`, which is 007 §11.3); and an element plus control
  behaviour (the `<button>`'s `popovertarget`, which is §12.3's `actions`). An element enum would fix **one of
  three** and be shape-before-need for the other two.
  **Counting what is present may be syntactic. Concluding something is absent may not.** That is the rule
  the whole class reduces to, and it was derived after three failures in one session
  (`menata-app-document`'s `audits/2026-10-03-audit-klaim-nol-case-di-dokumentasi-dan-gerbang.md`). A
  ratchet counting 129 Field references by regex is sound -- it measures something that is there. A
  conclusion that "this primitive has zero uses" by regex is **not**, because the absence of a *class
  string* is not the absence of a *meaning*. The two look like the same measurement and have opposite
  reliability.
  Worked examples of it being wrong: `row`/`grid` rejected by counting class strings; `columns` recorded
  twice as "zero measured uses" and measured at **9**, three of them inside Document Approval; `section`
  dismissed as overlapping `panel` and measured at **34**, the largest remaining population in the corpus;
  `tabs` recorded as zero, then **wrongly recorded as running in production** — the Approval Inbox's "three tabs" are declared navigation items, not a tab layout, and no `.templ` draws a tab bar. That correction is the fifth of this class and the first in the *opposite* direction, which sharpens the rule: **assert neither presence nor absence without measuring.** Over-claiming a case is the same failure as under-claiming one, and it is more dangerous, because it gets built. Each time the
  owner disproved it by *reading a screen*, which is what a class-string grep structurally cannot do.
  **And the directive gate itself carried the flaw**: `handWrittenLayoutSites` had patterns only for `row`,
  `grid` and `split`, so it reported 13 sites of Experience Plane debt when the real population was **51**.
  A gate whose job is to point the next session at the work, under-reporting it four times over, is worse
  than no gate -- it supplies a number that looks measured. Fixed 2026-10-03.
  Before writing "no case yet": open `case-portfolio.md` (21 written use cases, 105 lines); decompose one
  running application and read its screens; check whether established framework vocabulary has a name for
  it. And if you still conclude zero, **state your measurement method beside the claim**, so the next
  reader can judge the method instead of trusting the number.

  **The lesson this repo keeps relearning: "two cases" is a claim about sameness, and sameness is
  what the measurement decides.** Counting sites that share a *symptom* is how `row` and `grid` were wrongly
  rejected (class strings), and it is how this was wrongly accepted. Look at what each site would actually
  need.
  **And the verification could not be live.** Thirteen screens diffed against a worktree baseline came back
  token-identical because **no record in the dev database holds a non-empty `fld_due_date`** — the badge has
  never rendered in this environment. Thirteen identical screens would otherwise read as proof of something
  they cannot prove; the real proof is `experience.TestResolveSLABadge*` and
  `rendering.TestStatusBadge_rendersEachDeclaredTone`. **Check that a diff can reach the code before
  reporting it as verification** — this is the third slice in a row where it could not, after
  `pendingApprovalCardGrid` and New Application's review pane — and a fourth the next day, `Avatar`'s
  `pending` variant, which needs an outstanding invitation the dev database does not have. Three of its four
  sites *were* reachable, which is the useful shape: diff what you can, pin what you cannot, and say which is
  which.
- **Theme (006) is declarable for all nine token categories, and the bookkeeping rule its first one broke is worth more
  than the feature.** `domain.Theme` maps `RadiusRole` to `RadiusStep`; a `theme:` block in a Workspace
  manifest is validated by `metadata.resolveTheme` and read by `rendering.radiusClass`. **Verified live, not
  asserted**: a declared `surface: full` changed `sectionLayout`'s radius with no recompile, which is the only
  thing distinguishing a Theme from `Gap`/`BadgeTone` — closed vocabularies only Go can set.
  **Adding a metadata key means four documents, and I shipped the code without them.** The plan for that slice
  stated the rule — *update `capabilities.md` and 007 §40 in the same change* — and the next commit violated
  it: `capabilities.md`, §40, `CLAUDE.md` and `ROADMAP.md` all had zero mentions of Theme after it shipped,
  found only because the owner asked where it was documented. **A new metadata key is not done until
  `writing-guide.md` shows an author how to write it** — that file is the only one a metadata author reads,
  and it is the easiest to forget because no gate reads it.
  Also: the `theme:` key broke `installer.WorkspaceManifestCheckDoc` within minutes — the **third** instance,
  and the first its own plan had predicted by name. Check that mirror before the gate does.
  **All nine are declarable** (radius, weight, text size, gap, border colour, semantic tone, text colour, background colour -- the last took four roles, not the decided three, because reading all 80 `slate-600`/`slate-700` sites showed `700` has its own job -- and padding, 2026-10-06); shadow is deliberately not a tenth. The order they arrived in was measured, not chosen:
  `menata-app-document/audits/2026-10-04-inventaris-token-design-system.md` carries all 104 values with
  per-value counts and a verdict each. Two findings there change the plan — `domain.Gap` is **missing its two
  smallest steps** (`gap-1` 49 uses, `gap-0.5` 24, both below `tight`), and padding is **56** values, not 20.
  **Padding (D4–D6) is declared as six roles, not 56 values, and three things changed on measurement.** `px` and `py` came out as the *same* eight-amount set over what a primitive can reach (the two ladders differ in each role's default, not in membership); the all-sides ladder is the smaller one and stops at `five`, because a base `p-6` had no site and the first build emitted exactly that one new class (`app.css` is byte-identical to HEAD now). `panel` (four) and `section` (five) **stay different by default** -- D6 is an owner decision, and a Workspace can declare them equal to see it -- and measuring found `panelLayout` has **one** live caller, so the debt is one site wide, not twelve. The one-offs (`pt-18`, `pb-24`, `pr-11`) are excluded in `domain.PaddingRole`'s comment because each is sized by another element.
- `TestNoClassLivesOnlyInAComment` — **Tailwind scans the `.templ` files as text, so a comment naming a
  utility class emits that class.** Nothing in `make css` knows what a Go comment is. Three leaks were found
  this way, all self-inflicted and all invisible: the comment recording that the first Experience Plane pass
  had leaked a three-column class kept that class alive for a day; `activity.templ` described a feed as "a
  flat top-10 list" and shipped `.top-10`; two `appshell.templ` comments cited `top-17` where the code uses
  only the `sm:` variant. Intent is invisible to the scanner — describe a class in prose, or cite one the
  code beside it actually uses.
  **Two things worth carrying forward.** First, this gate shipped with `strings.Contains` for its live-check,
  which made `grid-cols-4` look live inside `sm:grid-cols-4`; the leak it missed was caught by the
  worktree render-diff instead, which is the honest order of events. A containment check is not a token
  check. Second, it reads the *committed* `app.css`, so `make check-generated` is the half that catches a
  leak committed without regenerating — neither alone is sufficient.
- `TestClaimMatrixCitesRealArtifacts` / `TestConceptDocsCiteDocumentsThatExist` — the normative documents
  must cite artifacts a reader can open. **007 §40 described a different repository until 2026-10-02**: its
  evidence column cited `CAP-` rows, `CR-` gap numbers and `internal/metadata/compile.go`, none of which
  exist here, and it was wrong in both directions — "CEL expression evaluation — PROVEN" for a runtime with
  no CEL, and Dataset/Relation/Projection listed PROPOSED after all three shipped. Rewritten so every
  status cites a file, a package-qualified symbol, or a test **in this tree**; the gate checks each of
  those resolves (a renamed symbol fails too) and reads **only the table rows**, because the preamble quotes
  the citations the rewrite removed — the fifth gate here whose own text would otherwise sit in the data it
  reads.
  The second gate is the document-level twin of the pointer gate: 001–007 may not cite a `*.md` this repo
  lacks, except through `unportedUpstreamDocs`, a shrink-only named list with the reason and what closing
  it needs. **The population went 18 → 3 on 2026-10-02**, by porting `capability-lifecycle.md` (D1) and
  `composable-runtime-architecture-map.md`'s §2/§12/§13 (D2), and by the §40 rewrite dropping the rest; the
  three that remain are mentions that *are themselves retractions*, saying the document is not here. It
  matches **both** citation forms — backticks and markdown links — because backticks alone were a blind
  spot found by hand the day it was written (002 linked `architecture-benchmark.md` as if it were local).
  **Neither gate can check whether a status is true** — PROVEN versus PARTIAL is a judgement; a green run
  means the citations resolve.
- `TestNoRouteBreaksInAWorkspaceThatDidNotInstallItsApplication` / `TestApplicationOwnedRoutesComeFromNavigation` /
  `TestRequireInstalledApplication` -- the multi-Workspace half of the route sweeps (2026-10-05). Every other
  sweep builds one Workspace (`default`, which installs everything), so a route that breaks only where an
  Application is *absent* was outside their population: eight routes in each of four Workspaces panicked in
  `routeByID` or answered 500, found by a throwaway probe. `requireInstalledApplication` answers 404 for a
  route an Application owns in a Workspace that did not install it; the owned set is **derived** from
  `navigation:` (installed Applications plus the template library), never listed. `routeByID` stays
  fail-loud on purpose. `installedRoutesRatchet` starts **empty** -- read it out of the map. A fixture
  detail worth knowing: `Config.TemplatePath` must be set or `/install-application` 500s for a reason that
  is the fixture's, not the route's.
- `TestInstalledNavigationExplainsItsHeadings` / `TestRuntimeScreensResolveTheirHeadings` /
  `TestRendererDoesNotResolveNavigationHeadings` — the navigation half of 001 #6's second clause, and the
  family that exists because **mutation showed the first fix was only half a fix**. A navigation item's
  heading is its `title:` when declared and its `label:` otherwise; that inference lived as a fallback
  inside `rendering.titleByID` until 2026-10-02, which made it happen at render time where nothing could
  explain it — and 001 #6 names *rendering* explicitly while 005 Phase 4 owns "expand authoring
  conveniences". It is resolved at load now (`domain.ResolveNavigationHeadings`, called by
  `metadata.LoadApplication` and by `RuntimeScreens`' own init) and reported by
  `metadata.ExplainNavigation`.
  **Two things the first attempt got wrong, both worth carrying.** It stamped `Title = Label`, which
  destroyed the one fact a reader needs — whether an author wrote the heading — so `ExplainNavigation` had
  to guess from `Title == Label`, wrong for a screen whose heading legitimately equals its label; and it
  doubled `TestRenderingHasNoHardcodedPageHeading`'s population, because that gate compares against
  *declared* strings. `Title` stays as declared; `NavigationItem.Heading` carries the resolved answer.
  And the resolver lives in `internal/domain`, not beside the loader, because **`internal/rendering` may
  not import `internal/metadata`** — a rendering test building a nav fixture by hand has to reach the same
  resolution, and asking a fixture to hand-write an inference is the inversion
  `TestMachineFixturesPassProductionValidation` already rejects for `RelatedMachine`.
- `ir.TestValidateRejectsAllFiveMandatoryFaults` / `TestValidateAcceptsTheHeaderTree` /
  `TestValidateRefusesUnknownTypes` — **UI IR exists** (2026-10-03, 007 §15), with §15.3's five mandatory
  rejections: cyclic tree, unresolved required child, slot/type mismatch, binding outside the permitted
  scope, depth beyond `ir.MaxDepth`. The cycle check is an **identity repeating on its own ancestor path**,
  because a Go value tree cannot contain itself and that is the reachable form of the fault. The positive
  case matters as much: a validator that rejects everything would pass all five.
  **Its first consumer is `rendering.headerTree`** — the eyebrow/heading/subtitle block eight screens share,
  now built as data and walked by `uiNode` instead of being templ structure. Byte-diffed over all eight:
  −2 bytes each, every one inter-tag whitespace between block elements in a flex-col container, nothing
  else. **A primitive arrives with its consumer**, and UI IR was no exception.
  `internal/ir` left `declaredPlaceholders` the same day. Its entry said UI IR "waits on a second render
  target" — measured 2026-09-30, before the Experience Plane's primitives, and **expired on its own terms**
  once five Layout primitives had 45 call sites and the only hardcoded thing left was the *composition*. The
  real forcing case was never a second render target: it is §12.4's normative rule, with 38 screens rendered
  from 38 bespoke Go functions, a breach that was already live.
- `TestDeclaredPlaceholdersStayDeclared` / `TestPlaceholderDocsClaimNoContent` — `internal/ir` and
  `internal/planner` exist, carry boundary rules, and contain **only `doc.go`**, deliberately. A
  placeholder is legitimate: it reserves the seam's name and its import boundary before anything fills it,
  which is why `internal/registry`'s rule was already in place when Services moved in. What is not
  legitimate is a placeholder whose *summary* claims to hold something — `ir/doc.go` said it held "Domain
  IR, Data IR, and UI IR" while holding nothing and being imported by nothing, and `planner/doc.go` said
  it "implements" the CEP. Both fixed 2026-09-30. `declaredPlaceholders` carries the reason each is empty
  and fails in **both** directions, so an entry cannot outlive the emptiness it excuses.
  **Do not propose `ir.Machine` again without re-measuring**: 005 Phase 5 defines Domain IR as exactly
  `domain.Machine`'s own content, so a second type would be 001 #8 at the type level; Data IR's consumer
  is the planner, which 007 §34 marks PROPOSED and says admits nothing by being named; UI IR waits on a
  second render target. `ir/doc.go` records the full measurement — 379 sites across 11 packages to enforce
  in the type system a rule five packages' `fixturevalidity_test.go` already gate, against violations
  confined to tests.
- `TestPromptNamesEveryRegisteredCapability` (`internal/conformance`) +
  `aiassist.TestComposableSurfaceRendersEveryFieldType` — the AI assistant's grounding prompt against the
  registries it describes. `composableSurface`'s own comment used to claim that keeping it in sync was
  "a review discipline ... not a new kind of drift risk"; measured 2026-09-30 it had drifted three ways,
  and the consequence was not untidy prose — the assistant **declined work and gave false reasons**. The
  field-type sentence is generated from `domain.KnownFieldTypes` now, so that part cannot drift; the rest
  is gated on *naming* every `registry.Services` and `registry.KnownWorkflowEngines` member, which found
  two more unmentioned Services on its first run.
  **What it cannot check is whether what the prompt says is true** — "you may generate X" versus "X exists
  but you cannot emit it" is a claim about `aiassist.GeneratedEvent`'s own shape, and only reading that
  shape settles it. Doing so is what stopped this change telling the model it could emit notifications:
  the capability shipped, but `GeneratedEvent` carries no service field and the writer sets
  `log_activity` unconditionally. **Right conclusion, wrong reason, is the failure mode prose hides best.**
- `TestEveryCastRoleProvidesItsEngineDatasets` / `TestEveryEngineDatasetIsNamedByComposition` /
  `TestGoNamedDatasetsWithNoEngineRequirement` — the Dataset-provision family (2026-09-30), written after
  a missing `ds_documents_with_steps` 500'd every approval screen in two Workspaces for a day. The first
  sweeps every installed Workspace and asserts each cast role provides its engine's Datasets; it is not
  redundant with the load error, because a load error only fires for a manifest something loads and this
  repo has shipped Workspaces no test loaded. The second checks the *other* direction, which fails
  silently: a registry entry no composed screen selects through forces every Workspace to declare
  something nothing reads — worse than the outage, since it breaks a Workspace that worked. The third
  records the **remainder** (`goNamedDatasetsWithNoEngineRequirement`, seven ids) rather than gating it,
  because "which Workspaces can reach the screen that selects through this id" needs a route-to-Dataset
  call-graph walk nobody has built. **So the class is closed for the engine's Datasets and open for Case
  19's** — say that, rather than reading a green run as the whole class.
  Writing this also found `conformance.sortedKeys` never sorted, despite the name.
- `TestActionDoesNotSwitchOnItsOwnMachineIDs` — the fourth of the identity family, and the one that
  exists because the other three could not see the place it mattered. `internal/action` is excluded
  from the Machine-id gate as the owner of those constants, and `action.CanDelete` used that
  exclusion to `switch machineID` for a whole day: all three callers passed `machine.ID`, so any
  Workspace's own Machine named `mch_approval_step` had this engine's delete rule applied to it.
  Take the `*domain.Machine` and ask `IsStep`/`IsDocument`.

  **A gate locks in progress; it does not create it.** None of these forbids a shape whose
  declarative alternative does not exist yet — `TestNoBareMachineIDIdentityChecks` could only be
  written after all 23 call sites were converted, and the audit's Gap A (an Action cannot declare
  which Fields it writes) is deliberately ungated until that primitive exists. Build the primitive,
  migrate the uses, *then* gate. The reasoning and the plan are in `menata-app-document`'s
  `audits/2026-09-28-kajian-metadata-based-document-approval.md` and this repo's `ROADMAP.md`.

If you find yourself re-explaining the same architectural rule in a PR/commit twice, or adding a
row to a `capabilities.md` table by hand, consider whether it should be (or already is) a
conformance test instead.

## Where a write-up goes: this repo vs. the companion doc repo

The split is already defined, in `menata-app-document`'s own README, and **using it consistently is
the part that has failed** — measured 2026-09-30: `development-history.md` stopped at 2026-09-22 with
3,229 lines while this repo's `ROADMAP.md` grew to 4,599, of which `## In progress` + `## Planned` are
~4,480 lines of exactly the content the README assigns to the companion repo. Seven days and about
twelve commits of design rationale went into the wrong file, by me, including on the day I put a
staleness header on the right one.

| file | holds | shape |
|---|---|---|
| `menata-app/ROADMAP.md` | what is shipped / in progress / planned | **short, feature-level, forward-looking, public** — the `## Shipped` bullets are the model |
| `menata-app/capabilities.md` | what the runtime can do right now | current-state tables |
| `menata-app-document/development-history.md` | phases, forcing conditions, design rationale, verification steps | append-only detailed history, private |
| `menata-app-document/audits/` | one-off dated findings citing commits/lines | review records |
| `menata-app-document/guides/` | methodology proven reused ≥2× on genuinely different cases | standing practice |

**The operational rule, before you write:** a finding's narrative — what forced it, what you measured,
what the mutation proved, what could not be checked live — goes to `development-history.md`.
`ROADMAP.md` gets a feature-level line plus a pointer to it. A paragraph containing a commit hash,
"measured", "mutation-proved" or "probe" is a history entry; if you are about to put one in
`ROADMAP.md`, that is the signal it belongs in the other repo.

`TestRoadmapStaysAReleasePlan` holds the flow shrink-only (below). It cannot judge whether *one*
paragraph belongs here or there — it counts vocabulary and volume, not fit — so a green run means the
file stopped growing, never that the split is right.

## Working with other agent sessions

Other sessions use this same checkout, and the handoff mechanism that works is already proven: a dated
file in `menata-app-document/audits/` written **as a work list for another session** — each item naming
*where* (a path), *why* (a document citation), *what done means* (testable), and *what blocks it*. One
session wrote `audits/2026-09-30-kajian-pemutusan-dari-menata-runtime.md` that way; another picked it
up the same night, did part of it, and found two of its numbers wrong by measuring.

**That last part is the protocol, not a complaint: re-measure every number you inherit.** Six carried
figures failed on measurement in one session — "ten join sites" (three), "8 dangling citations" (18),
"68 field-type sites" (zero that were a seam), `stack` "needs a second case" (61 uses), "notifications
are generatable" (no — the generated shape has no service slot), "correlations hold the ratchet at 11"
(Relation shipped and the number moved zero). Re-measuring is part of the work, not distrust of the
author.

Mechanics, each learned the hard way in one session:

- **Never `git add -A`** — it swallowed 7 files belonging to another session, and the push failed on
  their uncommitted generated output.
- **Never `git checkout` to restore during mutation testing** — it discarded in-progress edits three
  times. Copy to the scratchpad instead.
- An **untracked file that is not yours**: do not commit it, ask. The session that wrote one committed
  it itself an hour later.
- Verify a mutation actually applied (`grep -c`) **before** trusting a green run. A no-op mutation
  reads exactly like a gate that does not bite.
- Watch for a gate whose own text sits inside the data it reads — **four** were found in one session,
  each passing when it should have failed.

Two more mechanics, both proven on 2026-10-02 and neither obvious until it bites:

- **`pre-push` runs the whole tree's suite** (`make test`), so you cannot push while *another session* has
  a failing test — even when your change is entirely separate. The answer is to wait, not `--no-verify`:
  that flag would bypass a gate telling the literal truth, which is that the tree is not green. A
  concurrent-run artifact is also possible, since both sessions share the dev database — a Postgres test
  failing on a duplicate key is worth re-running alone before believing it.
- **`git commit -- <paths>`** is the right form here. The index may already hold another session's staged
  work (a staged deletion, in the case that taught this), and a plain `git commit` would carry it. Naming
  your paths commits only those and leaves their staging untouched.

The gates are what make this safe: they are the shared contract that lets one session change code
another wrote. Breaking someone else's gate is a conversation, not a wall — lower the number if it is
an improvement, or fix the code.

## Commands

- `make generate` — regenerate `*_templ.go` from `*.templ` (needed after any `.templ` edit).
- `make build` — generate + `go build ./cmd/server`.
- `make test` — `go test -race ./...` (the pre-push hook runs this; don't skip it locally either).
- `go test ./... ` with `DATABASE_URL` set runs the Postgres-backed integration tests
  (`internal/web`'s `*_test.go` `t.Skip` without it); they clean up their own rows via
  `t.Cleanup`, so it's safe to run against the shared local dev database.

## Working directory is shared

Other Claude Code sessions may use this same checkout concurrently (see `ROADMAP.md`/git log for
what's in flight). Check `git status`/`git log` before a long refactor, and be careful restarting
the local dev server (`bin/server` on port 4000) — another session may be using it.

The dev server is managed by systemd, not a bare process: `menata-app.service`
(`/etc/systemd/system/menata-app.service`, `ExecStart=/root/projects/menata-app/bin/server`,
`Restart=on-failure`). After `make build` changes `bin/server`, restart it with `systemctl restart
menata-app` — never `kill`/`pkill` the PID and relaunch it by hand (e.g. `nohup ./bin/server &`):
that orphans an untracked process holding port 4000 while systemd reports the unit as inactive, and
the manual process loses `Restart=on-failure` and the unit's log redirection
(`/var/log/menata-app/app.log`). Check state first with `systemctl status menata-app`. This is a
host-wide convention, not specific to this repo — every app on this box (this checkout included)
runs under its own systemd unit (e.g. `portal-ga3-dev.service`, `amaliah-web.service`), each with a
`*-crash-alert` companion unit, fronted by `caddy.service`.
