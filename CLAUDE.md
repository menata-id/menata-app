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
  One file is grandfathered in `projectionRatchet` (ten when it was written, seven for most of its
  life, one since 2026-09-28) and **the list may only shrink**: adding an entry is not the way to
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
