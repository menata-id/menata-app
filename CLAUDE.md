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
example of how — a closed engine registry (`domain.KnownWorkflowEngines`) whose roles are validated at
load against the Application's own `machines:`.

**And a Workspace's own ids are still reserved against itself.** Isolation relaxed exactly one
thing — the same name in a *different* Workspace. Two Machines under one id inside one Workspace
stays a load-time error (`metadata.validateMachineIDsAreUnique`), and `aiassist.Validate` refuses
it before a generated Application is ever written.

That has a consequence worth knowing before you promise anyone an install: **a Workspace that
already uses one of a template's ids cannot take that template at all**, and nothing resolves it —
there is no install path except the assistant's own publish, and no rename-on-collision anywhere.
"dokter-kecil" holds `mch_document` for its generated Document Tracking, so Document Approval
cannot go in beside it today. The assistant *anticipates* collisions when generating (its prompt
carries every taken Machine id in the target Workspace, not only the Application-claimed ones),
but anticipation is not resolution. Planned: `ROADMAP.md`, "Installing a template into a Workspace
that already uses its ids".

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
  `internal/web/router.go` handler.
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
  Seven files are grandfathered in `projectionRatchet` (ten when it was written) and **the list
  may only shrink**: adding an entry is not the way to pass, and an entry left behind after a file
  is migrated fails too. Read the count out of the test, not out of this line. Use
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
- `TestClosedRegistryMembersAreAcceptedByTheLoader` / `...AreActivatedByMetadata` — a new
  capability must arrive with its metadata seam, not just its Go. Every `domain.KnownActions`/
  `KnownServices` member must be (a) accepted by `internal/metadata`'s own validation, since the
  registry and that validation are two lists that drift, and (b) actually named by an installed
  Workspace's metadata — a capability no manifest can start is one only Go can reach, which is
  001 #3 inverted. `domain.KnownWorkflowEngines` is held to (b) as well, and deliberately not to
  (a): its validator reads the registry itself rather than repeating it in a switch, so it has no
  second list to drift from.
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
