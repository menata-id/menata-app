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
is: an engine that cannot select its own records cannot run, and refusing to serve that Workspace beats serving a
screen that throws. Since 2026-10-10 (K09) that refusal is **per Workspace**: `metadata.LoadWorkspacesTolerant`
marks the manifest unavailable (`web.Deps.UnavailableWorkspaces`, a 503 for requests in it, the error logged as
`WORKSPACE UNAVAILABLE` with its file) and the process serves the rest. It used to stop the whole process. Only
"no Workspace loaded at all" is still fatal, and a *reload* that breaks a previously healthy Workspace is still
rejected whole — which is why `internal/installer` still load-verifies and rolls back.

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

### The design system has three homes: know which one answers your question (2026-10-11)

| Question | Where the answer lives | Link / path |
|---|---|---|
| What does the runtime render today, and what can a Workspace override? | **Code**: `internal/domain/theme.go` (roles → steps), readers in `internal/rendering/layout.templ`/`controls.templ`, `registry.Components`, `static/css/input.css` | this repo |
| What tokens, type scale, and components do the **mockups** use, and which proposals were accepted or rejected? | **Design System artifact "Menata App"**: `project/README.md` (decisions with reasons) and `project/tokens.json` (64 tokens, oklch values, `status: accepted` on the ones not yet in code) | `https://claude.ai/artifact/Xi4TYQPxsMQUmL3chScmaS` |
| What should a screen look like? | **Mockup canvases**. Both have the Design System installed (`project/ds/menata-app/tokens.json` plus a `designSystems` record). On the UI mockups canvas, each of the 88 artboards' `:root` blocks is *generated* from it (version `1791884364-e589`, canvas v161; the canvases take the light theme's values). The Builder canvas's copy is at the same version (v32) | UI mockups `https://claude.ai/artifact/Wzkc6reCNHBJvtq2DJHsU6`; Application Builder (process-map components: ProcessNode, ProcessStep, ProcessExit, ActorChip, IssuePin, ProcessMap; Halaman 4) `https://claude.ai/artifact/4tdbksPLQdwogQ19a9rPbq` |
| Why was a Theme role decided the way it was? | `menata-app-document`'s `guides/design-system-decisions.md` (D1–D9, and its "where everything lives" header) | companion repo |

**Governance, lessons and the gate design live in `menata-app-document`'s `guides/menjaga-design-system.md`.** It
records fourteen measured inconsistencies between code, artifact and canvas (2026-10-03 to 10-12), and none of them
turned a build red. It also holds rules R1–R9 and cross-source gates GS1–GS7. R9 says a migration to a Theme reader
is also tested under a moved theme, because the default theme cannot show a collision. The rule that would have
prevented four of the first eleven: **folding, normalising or replacing a value is a decision, not a tidy-up.** Test it on the
densest surface, measure, record it with its reversal condition, and leave it as a reasoned floor until then. The
2026-10-03 `row` migration folded 6px into 8px "as an accident"; four days later the artifact showed 8px breaks
lane titles, and `GapXtight` reversed it.

**Read the Design System artifact's README before proposing a token, a type step, or a mockup component.**
An agent that skipped it on 2026-10-11 invented 18 canvas tokens for the process map while the README had
already decided *"Tidak ada token baru"* for exactly those components. It also proposed a `-500` indicator
the README had rejected for contrast (2.1:1, below WCAG 1.4.11's 3:1). It also read the README's canonical
`oklch` values in the Builder artboards as a "second dialect". All three were reverted the same day. The
record is in `menata-app-document`'s `audits/2026-10-11-kajian-mockup-vs-design-system.md`.

Rules that follow from it:
- **Change a token in the Design System artifact, then regenerate the canvas `:root` blocks from its
  `tokens.json`.** Never edit one artboard's block by hand: the 88 copies drift.
- **An artboard should reference tokens (`var(--accent)`), not copy their values.** Since 2026-10-12 the
  five Builder artboards (A01, U01–U04) do, through `design-system/canvas_vars.py`. It is a pure substitution,
  oklch literals only, verified by resolving every `var()` back. The rest of the canvas is Tailwind **v3 hex**, a
  slightly different palette from the v4 tokens, so converting it is owner decision K1, not a substitution.
- **Artifact and code are held in sync by GS1** (`TestDesignSystemTokensMatchTheCode` over
  `design-system/tokens.json`). As of 2026-10-12, all four tokens the artifact accepted are in code (`mark-good`,
  `gap-xtight`, `gap-region`, `control-height-compact`), so `dsAcceptedNotInCode` is empty. Still open:
  - `gap-1`/`gap-0.5`, which neither side owns;
  - green-700 on Tag `emerald`;
  - the 15px question;
  - the pre-auth `auth-*` sizes (unmapped).
  **The one status list for all design-system work** is `menata-app-document`'s
  `audits/2026-10-11-rencana-design-system.md`, section "STATUS MENYELURUH". Update it at the end of every slice, and
  do not keep a second list here. Treat a difference as a sync item to record, not as permission to pick one side
  silently.

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

Since 2026-10-12 the Shared rendering components table has **Kind** and **YAML** columns. Kind is one of: registered
Component, layout primitive or static node, class reader, shell, shared fragment, page-internal. YAML says whether a
`page:` can name it. Fill both when you add a row. The gate reads only the first column, so the two columns are kept
by review, not by a test.

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
  derivations) → 11 (Stage E2's flow-template shape) → 9 → 4 → 1 → **0** (2026-10-07; the map is empty, and stays declared).

  This line used to end "and none of it is a derivation away: a step's *label*, which nothing declares
  at all, and a form input's `name=`, which is 007 §11.3 Binding with no primitive yet." **Both halves
  were wrong, in the two different ways worth knowing about.** The label was not a missing declaration
  but a Field nobody filled — `fld_step_name` was empty in 0 of 23 records, collected by no mockup, a
  free-text stand-in for a job title, so it was deleted rather than declared. The `name=` attributes
  were not waiting on a primitive at all: the *read* side of that same form had derived those ids from
  `action.DeclaredFields` since it was written, and Stage D had already shipped the exact shape next
  door (`rendering.PlacementFields`), so the fix was a struct and a parameter. The last one,
  `rendering/detail.templ`'s page thumbnail, was recorded as "a props decision on a ten-parameter signature" and as
  "presentation, not the compositing question"; **both were wrong, again.** ctx already carries the Workspace, so no
  parameter was needed, and the thumbnail is an `<img>` of `/pdf-preview`, whose handler reads
  `action.CompositeFields(step).SourceField` -- the same question, not a nearby one. `rendering.documentPreviewField`
  reads it (2026-10-07). The gate counts `action.Field*` constants only: literal id strings in
  `internal/web/document.go` are outside it.

  **The rule to carry, since this line asserted the opposite twice:** a deferral is a measurement with
  an expiry date. "No primitive yet" is grep-checkable in a minute; "nothing declares it" is answerable
  by querying the Field. Neither survives being written down and trusted — and a *frozen* count is what
  makes stale prose beside it read as settled. When a stage ships a primitive, re-measure every deferral
  phrased that way, not only the ones the stage names.
- `TestBespokeScreensOnlyShrink` / `TestPageBlockDeferralStaysTrueAndLandsDocumented` -- the two steering
  gates of the `page:` block (2026-10-07; plan `menata-app-document`'s
  `audits/2026-10-07-rencana-page-yaml-dan-gate-yang-akan-terkena.md`), installed **before** the primitive and
  testing nothing that does not exist yet. The first is a directive ratchet over the `templ …Page(` functions
  in `internal/rendering`: a closed map, a category and a reason per entry, **35 across five categories when installed (34 on 2026-10-12; read the map), 2 of
  them `candidate`** (read the numbers out of the map); a new function fails and says to ask whether `page:` can
  declare it, and an entry whose function is gone fails until deleted. Its categories are a *reading*, not a
  measurement, and `candidate` was a hypothesis until Tahap 0.2 read all six (2026-10-07): none is Jalur A, and `InferencePage` moved to `workspace` because the runtime, not an Application, owns it. The second is a declared deferral in
  `declaredPlaceholders`' shape that fails both ways: it must exist while `navItemDoc` has no `page` key, and the day
  the key lands it must go **and** `writing-guide.md`, `capabilities.md` and 007 must mention `` `page:` `` -- the
  four-document rule `theme:` broke. **The probe is the part to carry:** the plan predicted 22 gates would bite and
  a scratchpad probe found two (the loader's strict decode, and `TestNavigationRoutesAreRegistered` once the route
  is `/pages/…`); `installer`'s mirror carries navigation as `[]any`, so a nav key cannot break it. Predicting which
  gates bite from reading is wrong often enough here that the probe comes first. Seven mutations, all biting.

  **The deferral is cleared (2026-10-07) and `page:` exists** (007 §12.4/§15, `writing-guide.md` §12.1a): a navigation
  item's `page:` is validated at load (`metadata.validatePages`: route `/pages/<id>`, root a layout, Binding against the
  Workspace's own Datasets, `ir.Lower` + `ir.Validate`, each Component's registered validator), lowered per request by
  `composition.DeclaredPage`, and drawn by one handler (`web.showDeclaredPage`) through `rendering.DeclaredScreen` --
  named so `TestBespokeScreensOnlyShrink`'s `…Page(` pattern does not count it. `pageBlockDeferral` is nil and the gate
  that held it now only checks the four documents. **Say plainly what it is not:** no existing screen was migrated, the
  two consumers (`nav_documents_by_status` in nana-workspace and nana-2-workspace) are new, and Binding is the **read
  side only** (`{dataset, measure, rows: dimension}` on a `Metric`, and from 2026-10-08 `{dataset, rows: records}` on a
  `Collection` whose one template child takes `from: {property: role}` out of the Machine's `card_fields`). That Collection takes `empty: <words>` (2026-10-08, slice 3a): drawn in place of the list when the Dataset lists nothing, a load error on any other node. **No truncation notice was built, deliberately** -- `ds_recent_documents`' `limit: 5` is a window, so "showing the first 5" would be false there, and nothing declares which meaning a `limit:` has; it waits for a consumer whose limit is a safety cap (3c). **Static text on a page is nine roles, not three** (2026-10-08; the tenth, `link`, took `to:` the same day -- see below): the renderer drew all of them, and `ir.allowedProps` -- a second list beside `domain.KnownStaticKinds` -- was missing seven, which `ir.TestValidateAcceptsEveryStaticKindWithItsText` now holds (mutation-proved on `note` and `eyebrow`). **`static: link` takes `to: <navigation item id>` and never a route** (Tahap 2a, 2026-10-08): `ir.Lower` resolves the href and the default text through an injected `ir.RouteResolver` built from `domain.Application.AllNavigation` (the pre-`hidden_nav_groups` list `routeByID` reads), a typed `href:` and an unknown id are load errors, and `TestEveryInstalledLinkNamesItsDestinationOnce` holds the installed manifests (mutation-proved with a typed `href` spliced in). `heldOutStaticKinds` is empty and stays declared. The first gate to bite was `TestEveryAcceptedNodeTypeHasAWalkerArm`, which reads `case domain.StaticLink:` as text -- the walker had an `if`, so the arm was added rather than the gate narrowed. **A link to a record is built (Tahap 2b, 2026-10-08)**: inside a records template, `from: {text: title, href: record}` -- `record` is a reserved word (`domain.PageRecordRole`, filled by `composition.RecordRoute` with the runtime's generic `/machines/<m>/records/<id>`), valid only as a link's `href`, never a `card_fields` role and never an address taken from a Projection role (`ir.TestLower_refusesARecordLinkThatCouldLeakOrFabricateAnAddress`, `web.TestDeclaredPageRecordLinkOpensTheRecordsOwnFullPage`; four mutations bit). The recent-documents titles in both Nana Workspaces use it; it reaches the detail page only, not `/review` or `/edit`. **A link to a Machine's list page is built (Tahap 3c-1, 2026-10-08)**: `static: link` with `list_of: <dataset id>` and written `text:` -- `domain.PageNode.ListOf`, resolved by `ir.Resolver.Source` into `domain.MachineListRoute`, the Dataset checked at load against the Workspace's own (`metadata.bindingIssues`) and again, independently, by `TestEveryInstalledPageLowersAndValidates`; `TestEveryInstalledLinkNamesItsDestinationOnce` now holds three destinations. It exists because Board Settings' two "Manage →" links point at Machines no `navigation:` names; its consumer is an "All documents" link on the recent-documents page in both Nana Workspaces. Mutation-proved twice (the loader check off beside a manifest naming a missing Dataset; the composition resolver removed). Two installed-manifest gates bit on the first run and were updated rather than narrowed: `TestEveryInstalledPageLowersAndValidates` read `from: record` as a Projection role, and the link gate demanded `to:` (now `TestEveryInstalledLinkNamesItsDestinationOnce`: one of `to:` / `from: {href: record}`, never both). **A record's label and colour bind to `Tag` (Tahap 3c-2, 2026-10-08)**: `from: {label: title, color: color}` inside a records template; the consumer is `nav_board_settings` (every Label as a Tag) in `default`'s Project Management copy **and** the library template, because `ds_board_labels` is in both. `metadata.PlaceholderResolver` now gives the `color` role a palette entry instead of "x", since a Tag's registered validator would otherwise refuse every load-time placeholder; the same change makes `from: {color: title}` a load error, which is the check that a colour comes from something that is one (`web.TestDeclaredPageDrawsEachLabelAsATagInItsOwnColour`; mutation-proved twice: the placeholder reverted, and the `color:` line removed from the page). A library-name collision cost one rename: `title: Labels` retyped Board Settings' own heading and `TestRenderingHasNoHardcodedPageHeading` refused it. **A Collection numbers its items (Tahap 3c-3, 2026-10-09)**: `ordered: true` draws an `<ol>` and a position number per item, counted by the renderer so a page writes no number (`web.TestDeclaredPageNumbersTheBoardListsInColumnOrder`; consumer `nav_board_settings`, the board's Lists in column order, in `default`'s Project Management and the library template). The prop is `true`/`false` and not an enum, deliberately: a closed set with one member is a constant in an enum's clothes. Mutation-proved three ways (the renderer ignoring it, the `ir` allow-list without it, the validator accepting `yes`), and the number is `aria-hidden` because `<ol>` already announces position. **A count over a declared Relation is built (Tahap 3c-4, 2026-10-09)**: a `static` node in a records template takes `count: {of: <Relation id>, one, other}` instead of `text:` -- `domain.PageCount`, lowered by `ir.lowerCount`, the number put in the record map by `composition.bindingRecords` under `domain.PageCountRole(<rel id>)` from `Selection.Related`. `of:` is checked by the same lowering that checks a `from:` role (the load-time placeholder record carries a count only for the Relations the bound Dataset declares, so `metadata.PlaceholderResolver` now takes the Workspace's Datasets), not by a second list. The consumer is `nav_board_settings` over `ds_labels_with_cards` (Label Machine, `default` copy and library template; mutation-proved three ways: composition not filling the count, the singular wording ignored, the placeholder not supplying counts). **An author-opted truncation notice is built (Tahap 3c-5, 2026-10-09)**: `complete: true` on a records Collection is the page's claim that its list shows every record the Dataset matches; `ir.Lower` consumes it and puts the runtime-only `truncated: "<limit>"` on the lowered tree only when the claim holds **and** `Selection.Truncated` is true, and the Collection then draws `truncationNotice` (`ir.TestLowerCompleteNamesTheBoundOnlyWhenItBit`; `TestCollectionTruncatedIsAPositiveWholeNumber`). The runtime still does not know whether a `limit:` is a cap or a window, so the claim is the author's: `windowDatasets` in `internal/conformance` names the Datasets known to be windows (`ds_recent_documents`' `limit: 5`) and `TestNoPageClaimsCompletenessOverAWindow` refuses `complete: true` over one -- a list of the judgement, not a derivation; a Dataset-level declaration of what `limit:` means is not built. Consumers are the three `limit: 200` Datasets (`nav_board_settings`, `nav_board_settings`, `nav_board_settings`). **A Collection draws its items as one divided surface (2026-10-09)**: `divided: true` is `true`/`false` like `ordered`, draws a single `surfaceClasses` list whose rows are ruled by `borderClass(ctx, domain.BorderDivider)` instead of one loose tile each -- what the Board Settings mockup (`PM06`) shows and the bespoke screen never did; the same three pages consume it, mutation-proved three ways (the walker ignoring it, the `ir` allow-list without it, the validator accepting `yes`). **Board Settings is the first bespoke screen migrated (2026-10-09, owner-authorized: all metadata but `hanomerch` is experimental)**: `nav_board_settings` is a `layout: split` of two stacks, each a Subheading, a `list_of:` link, a divided Collection and a note; its route moved to `/pages/nav_board_settings`, `BoardSettingsPage`, `composition.BoardSettings`, `GET /board-settings` and the three interim pages were deleted, and `ds_label_usage` went with its last Go reader. The three Dataset ids left `goNamedDatasetsWithNoEngineRequirement` because Go no longer names them -- the loader's binding check is what stands in. Reorder/rename/new stay out (007 §12.3 `actions`). Never write "screen X is declarative" on the
  strength of this. **Installing copies**, so a library change to a bound Dataset reaches no earlier Workspace -- the load
  error on a missing Dataset is what stands in for remembering.
  **Post-primitive gates (2026-10-07):** `TestEveryInstalledPageLowersAndValidates` (every installed `page:` re-checked
  against `ir.Validate`, not only by the loader; since 2026-10-08 also every `rows: records` binding against the owning Machine, by `recordsBindingProblems`, independently of `metadata`'s own check -- the probe showed disabling that check failed only a fixture unit test) and `TestDeclaredPagePipelineHasNoPerScreenBranch` (no nav/Machine/
  Application/Dataset/Field id literal in the six files between YAML and pixels). **The probe is why the first exists and
  what it found:** with the loader's `validatePages` call disabled, only one unit test failed -- no gate over installed
  manifests -- and raising an installed page's `columns` to 9 passed even the new sweep, because `ir.Validate` checked
  property *keys* and never *values*, so a bad `columns`/`gap` loaded and rendered as the default. `ir.layoutPropValues`
  now closes it (read from `domain.Known*`), and `rendering.gridColsFromProp`'s "ir.Validate refuses a bad tree" comment
  is true for the first time. A page's layout values are closed; Component values already were (registered validators).
  `TestInstalledPageDatasetsExistInTheLibraryTemplate` (2026-10-08) closes the copy-model half: the page's Dataset had reached two Workspaces' copies while `metadata/document.yaml` had lost it (removed with the Dashboard, its last Go reader), so a fresh install could not take the page. Mutation-proved; it skips Machines that are not library ones.
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
  not a composable Data Plane**: the reads that remain either correlate two Machines and wait on 007
  §7.5 Relation or legitimately read everything (the classification is in `capabilities.md`'s Dataset row
  and the audit it cites; the count is in `wholeMachineReadRatchet`, not here).
  The second guards a class that became *silent* when it became declarative: a Dataset whose whole
  purpose is showing one identity its own records must declare a `$current_user` predicate. That
  scoping used to be a line in Go where deleting it failed a unit test; deleting it from metadata
  leaves the Dataset loading, the screen rendering, and every viewer seeing everyone's records —
  measured, the whole suite stayed green. A named list (`perViewerDatasets`), because "is this
  per-viewer" is a judgement no scan can make.
  **When declaring `select: records`**: `limit:` is required (007 §7.9), a `where:` value is a literal
  or a context value the runtime resolves -- `$current_user`, `$today[±n]`, `$parameters.<name>`, and `$done` over
  a Machine's `completion:` (§9.2: an unknown `$…` is a load error, on purpose; this line said "only
  `$current_user` or `$parameters`" until 2026-10-11, after `$today` had shipped with K14), and
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
  **40 across 18** the same day once `grid` landed, and **18 across 12** on 2026-10-07 (row 9, split 1, section 8)
  once `columns` reached zero and `section` was re-read. A *new*
  hand-written site fails and the message names the primitive it should have used; a *lower* count fails
  until the entry drops, so a migration is locked in. The next session finds this work from a failing test
  rather than from remembering to read a plan.
  Counted **syntactically** on purpose — "is this really a row" is a judgement, and a gate making it would
  be the ~50%-false-finding shape deleted twice here. What syntax cannot separate is noted per entry:
  `split`'s floor is not 0 (read the entries out of `layoutdebt_test.go`: what is left is `rolematrix.templ`'s
  bordered grid, a genuine one-off, and a one-off is what a primitive is not for).
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
- `TestNoHandWrittenDividedRow` -- a zero-floor gate (2026-10-07): no class set may hold `last:border-b-0` beside `sm:flex-row`; a divided list's row reads `dividedRowClasses(ctx, gap)`. It exists because the reader took three sites out of the `columns` pattern's sight (the pattern reads a literal `class="..."`), so a ratchet falling to zero would otherwise have meant "the shape cannot come back" without anything checking that. Its sanity check counts class sets holding `flex`, not `sm:flex-row`, because after the migration no hand-written site carries the latter -- a check on the very token being forbidden fails the moment the gate succeeds. Mutation-proved in the plain and the spliced form.
- `TestHandWrittenChipsOnlyShrink` -- the second directive ratchet, beside `TestHandWrittenLayoutSitesOnlyShrink`, built **after** 25 chips moved to `statusBadge` (2026-10-05; build, migrate, then gate). It counts a `<span>`/`<code>`/`<li>` that is `rounded` with a `-50`/`-100` tint, `px-` and a small text size, including `templ.KV` conditional classes. Three floors remain with a reason each (uppercase typography, the selected tab, a `<code>`; a fourth, Application Settings' "Not built yet" tag, left 2026-10-11 when it began reading the Theme). **The first pattern allowed any element and matched ~13 files** -- alert paragraphs, buttons, wells -- so it was narrowed before anything was frozen; what it cannot see (`div`/`a` chips, `pl-`/`pr-` pills) is stated in the test. Mutation-proved for both the plain and `templ.KV` forms.
- `TestHandWrittenMutedParagraphsOnlyShrink` -- the third directive ratchet, built after eleven sites moved to `staticText(StaticMessage|StaticNote)` (2026-10-06). It counts a `<p>` whose class *set* holds `m-0`, `text-sm|xs` and `text-slate-500`; **9 remain across 6 files (15 across 9 until 2026-10-11, when Application Settings' six moved to the Theme readers), every one carrying something the Static kinds must not accept** (inline markup, `leading-6`, per-site spacing or width, an escaped quote), each with its reason in the map. **The census that chose the two kinds matched an exact class literal and found 11 sites; the gate's class-set pattern found 15 more** -- the shape of a pattern decides what it counts, which is the same lesson as `row`/`grid`, from the opposite side.
- `TestEveryThemeRoleHasAReaderOrAReason` -- the Theme-side counterpart of "a primitive arrives with its callers" (2026-10-06). Every `Role` constant in `domain/theme.go` (read by AST) must be named as `domain.<Role>` in a hand-written `.templ`/`.go` under `internal/rendering`, comments stripped, or sit in `unreadThemeRoles` with a reason; **none do now** (`WeightBody` and `BorderDivider` were the last two, listed as needing "a divider Component or a table primitive" -- a claim nobody had checked: `borderClass` and `weightClass` already mapped both and had no caller, so ~21 lines needed a call and no primitive was built; `RadiusControl` and `InkBody` left 2026-10-06 when `Button` read them, `BorderControl` the same day when `fieldClasses` did). It exists because seven roles were declarable, documented and validated while nothing read them -- a YAML that loaded and changed no pixel. Shrink-only both ways (a role gaining a reader fails until its entry goes). Mutation-proved four ways, including that a comment naming a role does not count. **It checks that a reader exists, not that it is right.** An empty map stays declared, like any other emptied ratchet.
- `TestHandWrittenCaptionsOnlyShrink` -- the fourth directive ratchet, built after eight `text-2xs` help lines moved to `staticText(StaticCaption)` (2026-10-06). It counts a `<p>` whose class *set* holds `m-0`, `text-2xs` and `text-slate-500`; **11 remain across 6 files, each with its reason in the map** (an apostrophe `{ text }` would escape, a conditional or multi-line run, an inline `<span>`, `mb-2`/`leading-5`, a bordered well). `caption` is the eighth text role and the first one added *because* a ratchet census named it: the plan's 16 exact-literal sites were a count of class strings, and the class-set pattern is what found the 11 floors. Mutation-proved both ways (a new site fails; a migrated one fails until the entry drops).
- `TestHandWrittenButtonsOnlyShrink` -- the fifth directive ratchet, built after 17 `<button>` sites moved to `@button(label, variant, name, value)` (2026-10-06; build, migrate, then gate), and `Button` is the **sixth registered Component** (`registry.validateButton`: `name`/`value` together or not at all, a closed variant set, no `hx-*`/`class`). It freezes two populations: **18** `buttonClasses(` calls outside `controls.templ` (floors, each with a reason -- `hx-*`, `<a>`/`<summary>`/`<label>`, a script hook id, `sig-*`, `data-modal`, per-site spacing; things a Component must not accept, §12.3) and **4** literal `h-9` buttons (`px-3` and a slate-300 outline, not Button's shape). Mutation-proved both ways, and a `//` comment naming `buttonClasses(` is not counted. **Owner decision D8 was taken 2026-10-07: a button's outline reads `border.surface`, not `border.control`.** The mockups lean to slate-200 (5 of 8 bordered buttons) and `border.control` is slate-300, so reading `control` would have darkened 14 buttons; `surface` moved none. `buttonClasses`'s secondary variant, six button-shaped links and the view switch's track read it (`tileClasses`), and three of the 4 literal `h-9` buttons now read `border.control` and `radius.control` (a slate-300 outline is a control's rim, kept as it was) -- **they stay floors, not Button: `px-3` against `px-4`**, so D8 did not unlock a migration into `@button`. The fourth is a red Deactivate. The literal pattern is now read after `joinClassExpressions`, mutation-proved load-bearing (the widening removed, the count falls 4 to 1). The price is stated in `design-system-decisions.md`: a button's outline and an input's are two keys.
- `TestNoHandWrittenTag` -- a zero-floor gate, and `Tag` is the **seventh registered Component** (2026-10-08, slice 3b of the `page:` plan). It was registered over five sites that already existed -- `tagChip(CardTag)` on the board card, calendar, record detail, My Tasks and Board Settings -- so the migration changed a signature (`tagChip(label, color)`: a string and a `domain.TagColor`, which is what `TestRegisteredComponentsStayBounded` needs) and no bytes (`rendering.TestTagChip_reproducesThePreMigrationMarkup`, expected string written by hand). The contract requires `label` and leaves `color` optional; `registry.validateTag` refuses a declared colour outside the palette while the renderer keeps its neutral fallback for data that arrives unknown. **The gate exists because the chip ratchet counts `-50`/`-100` tints and could not see this shape**: a class set holding `rounded-full` beside a `bg-<colour>-<step>/10` fill fails (measured: every `/10` fill in the tree sat in `tagClasses`, none in a class attribute). Mutation-proved by splicing one into `mytasks.templ`; the restore was checked with `grep -c`. **One thing the closed-set gate caught that was worth reading:** `TagSlate` was drawn only by `tagClasses`' `default` arm, so `TestComponentRegistryAndRenderersAgree` could not tell it from a palette entry nothing draws; it is an explicit arm now. A page declares `component: Tag` with written `label`/`color` only -- binding a record's own label to one is not built, and nothing here says a screen is declarative.
- `TestEveryInstalledFormNamesNoRouteOrField` / `TestHandWrittenCreateFormsOnlyShrink` -- the pair installed **after** `Form` (`binding: {dataset, write: create}`, 007 §11.3 write side) shipped and Board Settings used it (T1 → T2, 2026-10-09). The first reads every installed `component: Form` and requires it to declare only `submit` and its binding: no `action`/`href`/`name`/`method`/`id`/`kind`/`options`, no children, and a Dataset whose Machine can be asked (`CreateFormInputs`, re-derived rather than trusting the loader). It does **not** demand `select: records` -- the loader treats the Dataset as a handle to its Machine, and a stricter rule here would be a gate ahead of the primitive. The second is a directive ratchet over `hx-post=…/machines/<id>/records` in hand-written `.templ` (**6 across 4 files**, read the numbers out of the map): each floor is something a bound Form does not carry yet (a file upload, a form inside a record carrying its parent's id -- T3, reference/group options -- T4). Mutation-proved five ways (typed `action:` -- caught by the loader first, so the loader's refusal was disabled to prove the gate bites alone; a new site; a migrated site; a raised count). **Extended for `write: update` (T3a, 2026-10-09):** an update Form must sit inside a records Collection's item template, name no dataset/rows/measure, and its Machine must have `EditFormInputs(nil)` non-empty; `updates == 0` is fatal, so the update half cannot silently measure nothing (mutation-proved by removing every installed update Form). The checkbox exclusion is deliberate, not a gap: `data.ValuesFromForm` reads an unticked box as `false`, so an edit form that asked for booleans would clear them. **`write: delete` (T3b, 2026-10-09) and `write: move` (T5, 2026-10-10)** are `TestEveryInstalledButtonIsADeclaredRecordWrite`, the same shape for `Button`: every installed page Button is a delete or a move inside a records Collection's item template, names no dataset/rows/measure and no `action`/`method`/`href`/`name`/`value`, and sits over a Machine that is not append-only; a delete carries a `confirm`, a move a `direction: up|down`, no `confirm`, and a Dataset with no `where:`/`sort:`; `deletes == 0` and `moves == 0` are both fatal. **`write: transition` (K13, 2026-10-10)** joins it: a `becomes:` that is an option of the Machine's status Field or `$done`/`$reopen` over a `completion:`, no `confirm`, and `transitions == 0` is fatal too; mutation-proved four ways (the same-value and `CheckTransitions` guards in `composition.recordTransition`, the `Allows` filter in `ir`, the option check in `metadata`). The move half was mutation-proved with a `sort:` spliced into the installed Dataset and the loader's own refusal disabled (the gate alone fails). Mutation-proved (a typed `action:` spliced into an installed manifest). The Button contract grew three inputs (`confirm`, `action`, `method`) and **its registered renderer is `requestButton` now, not `button`**, because `TestRegisteredComponentsStayBounded` reads one signature and requires it to name every declared input; `button` stays the four-string submit renderer the seventeen hand-written sites call. **`when:` (K11, 2026-10-10)** is the first of 007 §15.2's `Conditions`: record-scoped, one role and one comparison (`domain.PageCondition`), evaluated by `ir.conditionHolds` per record with `$done`/`$reopen` read from the Machine's `completion:`, the load-time placeholder marked `ShapeOnly` so the nodes a real record would hide are still validated; mutation-proved four ways (negation ignored, `ShapeOnly` ignored, `metadata.conditionIssues` unwired -- caught only once a test walked `bindingIssues` instead of calling the helper -- and the consumer's `when:` removed). Nothing says the viewer or the Application can be tested yet.
- `TestHandWrittenFieldsOnlyShrink` -- the sixth directive ratchet, built after `controlField` (one literal, 24 sites) became `fieldClasses(ctx)` (2026-10-06; build, migrate, then gate). It counts an `<input>`/`<select>`/`<textarea>` whose class set (after `joinClassExpressions`, widened 2026-10-07) holds a literal `border-slate-300`: **4 remain across 2 files, `authshell` and `login`, and every one is pre-auth** -- `currentWorkspace` is installed only inside the authenticated route group (read from `internal/web/router.go`, not probed), so sign in, register, password reset, accept-invite and choose-workspace render with no Workspace on ctx and **no `theme:` block can reach them**. **The earlier reason ("a 15px / `h-10.5` touch target is not a Theme role") was wrong for the three authenticated sites** -- account's name field, the invite-role select and the signature width box -- which kept their sizes as literals and now read `border.control` (and `radius.control` where it moved nothing); they were floors by assumption, not by measurement. **`fieldClasses` is a class reader and deliberately not a registered Component**: an input's attributes are 007 §11.3 Binding, which has no primitive, so a Component for it would accept arbitrary properties (§12.3). It gave `BorderControl` its first reader without deciding D8 for buttons. Mutation-proved: a spliced literal fails with the widening and **passes without it** (so the widening is load-bearing), and a floor raised or lowered fails. Blind spots stated: a class arriving from a Go function (those call `fieldClasses`), and account's read-only email box (slate-200 on slate-50, a disabled look).
- `TestHandWrittenDividersOnlyShrink` -- the seventh directive ratchet (2026-10-06; read, migrate, then gate -- no primitive was needed). It counts a literal `border-slate-100`/`divide-slate-100`/`font-normal` token in any `.templ` but `layout.templ` (where the readers live): **4 remained across 4 files when written (3 on 2026-10-12; read the map)**, none a missing capability (three are a *region* rule rather than a row separator, one is Tailwind's `divide-y` form). **It also found a blind spot in the other ratchets**: they read `class="..."`, and splicing a Theme reader into a literal makes it `class={ "a", reader(ctx), "b" }`, so three hand-written layout sites left `TestHandWrittenLayoutSitesOnlyShrink`'s count without being migrated -- a ratchet falling by an edit to an attribute. The layout one joins class expressions first (`joinClassExpressions`, mutation-proved by removing it). The chip, field, button and paragraph ratchets were not widened because none of their sites was touched; widen the one whose site you splice into.
- `TestHandWrittenThemeTokensOnlyShrink` -- the tenth directive ratchet, and the first that counts **tokens** rather than a shape (2026-10-11; read, migrate, then gate -- no primitive and no new role). A `.templ` may not write a text size (`text-xs`), a text ink (`text-slate-400|500|700|900`), a weight (`font-medium|normal`) or `bg-white` as a literal class when `textClass`/`inkClass`/`weightClass`/`backgroundClass` already map a Theme role to it. **It exists because every ratchet before it is blind to the next shape**: the screen it was written for (Application Settings, restyled to the tokens-v1 boards) wrote its own sizes and inks in plain `<span>`s, which no chip/paragraph/surface count could see, and the two that could see part of it fell *by an edit to an attribute* once the literals became readers. It counts through `joinClassExpressions` (mutation-proved load-bearing: a literal spliced into a `class={ }` expression is missed without it), strips `//` comments, and counts only the **whole, unprefixed** utility -- `sm:text-sm` and `hover:text-slate-700` are state and breakpoint, not roles. **829 tokens across 33 files** when frozen (size 398, ink 299, weight 105, fill 27); read the numbers out of `handWrittenThemeTokens`. **Since 2026-10-12 an arbitrary `text-[Npx]` is not in the size count**: no role names it, so telling a site to "read the Theme" was wrong. It is frozen apart in `handWrittenOffLadderSizes` (26: 23× 15px, plus the 13px monogram and the 21/26px pre-auth brand), with a reason per file that separates pre-auth screens (no Theme reaches them) from authenticated ones (they move if owner decision DS-D13 adds a 15px role). The size count fell by exactly 26. A file absent from the map holds none, so a new screen starts at zero and Application Settings (`appsettings.templ`, `rolematrix.templ`) stays there. Shrink-only both ways, a stale entry fails. **Blind spots, stated in the test:** a class returned by a Go function, tokens with no role (`text-slate-300|600`, `text-blue-*`, `bg-slate-50|100` wells -- `domain.BackgroundRole` records why), and whether the *right* role was chosen. **Its dynamic twin is `rendering.TestApplicationSettingsHub_hasNoTypeInkWeightOrFillTheThemeCannotMove`**: it moves every text, ink, weight and fill role off its default at once and fails if a default-looking class survives in the rendered hub -- which sees a literal arriving through a Go function, the one thing the static count cannot. Mutation-proved (three literals spliced back in); the same test fails if the moved theme never reached the page. **A finding the migration forced:** the mockup sets row labels at 15px and the type ladder has no step between `body` (14) and `subheading` (16), so the hub reads `body` and loses one pixel rather than keeping a `text-[15px]` the Theme cannot move; an author who wants 15px needs a role, which is an owner decision, not a literal.
  **That decision was taken 2026-10-12 (DS-D13):** `domain.RoleItem` is one item's title in a list or on a card, read
  through `textClass` as `text-item`. It is 15px with its own 22px line height and the Design System's `item` style,
  which GS1 checks. Ten `text-[15px]` titles and the hub's row labels read it; the row labels are back at 15px. What
  stays off-ladder is a field's or button's mobile size and the pre-auth screens (`handWrittenOffLadderSizes`).
- `TestHandWrittenSurfacesOnlyShrink` -- the eighth directive ratchet (2026-10-06; find the readers, migrate, then gate -- again no primitive). `surfaceClasses(ctx)` is the surface recipe (`radius.surface`, `border.surface`, `background.raised`) as a class reader; **34 hand-written `rounded-lg border border-slate-200 bg-white` sites in 18 files moved to it**, and `panelLayout`/`sectionLayout`/`metric` stopped repeating the four reads. Before it, declaring `theme.border.surface` recoloured the surfaces a primitive drew and left every other one on slate-200 -- a Theme key that moved some pixels and not others, which no gate could see because each site was individually valid. It counts a class set holding `rounded-lg`, `border` and `border-slate-200` after `joinClassExpressions` (so a state-conditional `templ.KV("border-slate-200", ...)` is seen): **4 remained when written, each a floor with its reason (3 on 2026-10-12; read the map)** (three swap the border by state, one `<dialog>` has no raised fill). **The existing ratchets could not see this population at all**: `section`'s pattern is `<section class=`, and these were `<div>`/`<aside>`/`<fieldset>`/`<details>`/`<ul>`/`<a>`. The 21 `rounded-md` + slate-200 sites are *not* counted -- buttons (D8), a read-only input and nested tiles are three jobs, and sweeping them into `RadiusSurface` would shift each by 2px. Mutation-proved both ways. **A comment of mine leaked a class again** ("a one-pixel outline" emitted `.outline` into `app.css`; found because `make css` showed a one-line diff, not by a gate -- `TestNoClassLivesOnlyInAComment` reads the *committed* CSS).
- `TestHandWrittenTilesOnlyShrink` -- the ninth directive ratchet (2026-10-07; read, migrate, then gate -- no primitive and no new role). `tileClasses(ctx)` is the small box inside a surface: `radius.control` for the corner (the default steps differ from `surface`'s, so reading `surface` would have rounded eleven screens by two more pixels) and `border.surface` for the edge; the four raised tiles also read `background.raised`. **11 sites in 8 files moved.** It counts a class set holding `rounded-md`, `border` and `border-slate-200` after `joinClassExpressions`: **3 remain in 3 files, none a tile or a button** (2026-10-07, after owner decision D8 moved the other seven -- the calendar's three links, `reviewdocument`'s View PDF / Download, the disabled Enable button and the view switch's track -- to `tileClasses`, byte-identical): a read-only email box, a clipped image frame and the edge of a rendered page image. Sorting the 21 by *reading* them is what found that they were never one population. Mutation-proved both ways (a new site fails; a lowered floor fails). Live diff: 36 routes byte-identical, 9 of 11 sites reached (`chooseworkspace` redirects; it is pinned by `TestTiles_followTheThemeAtHandWrittenSites`, which renders it with no Workspace on ctx).
- `TestNoHandWrittenRegionRule` -- a zero-floor gate (2026-10-06): no class set may hold `border-t`/`border-b`/`border-y` beside a literal `border-slate-200`; a rule that bounds a region reads `borderClass(ctx, domain.BorderSurface)`. **Thirteen sites moved** (twelve literals and `tableHeadCell`, now `tableHeadCellClasses(ctx)`), none a floor. **No `region` role was added**: it would default to `surface`'s slate-200 and nothing declares a reason to split them -- the case that earns it is an author wanting a faint box with firm rules. Measured by listing every `border-slate-200` line outside `rounded-md` and reading each. Mutation-proved in both the plain and spliced forms. Blind spots: a rule from a Go function, a variant-prefixed token.
- `TestNoHandWrittenSubheading` -- a zero-floor gate (2026-10-06): no `<h1>`..`<h6>` in `internal/rendering` may carry `text-base`; a section's own title goes through `@staticText(domain.StaticSubheading, ...)`, so `theme.text.subheading` moves all of them. **It reopened a verdict this file's own Theme text claimed: `text-base` "has no consistent role" was true of the 11 sites *as counted* and false once each was read** -- seven are one job, two (a menu label, a monogram) are not headings. The 23 body-sized `<h2>` panel titles are a different job and stay hand-written.
- `TestNoHandWrittenHeadingOrEyebrow` -- the same zero-floor shape (2026-10-06) for the two kinds whose *roles* were the finding: `theme.text.heading` and `theme.text.eyebrow` were declarable, validated, documented in `writing-guide.md`, and **read by no renderer** -- seven declared Theme roles then had no reader (none since 2026-10-06) (`RadiusControl`, `WeightBody`, `BorderControl`, `BorderDivider`, `InkBody`, plus these two until this slice). Measured by grepping `domain.<Const>` in non-test Go/templ with comments stripped. The gate flags a `<h1>`..`<h6>` carrying `text-xl` and a blue `text-3xs` uppercase span without `font-medium`, and (from the same day) the faint `text-slate-400` form of that span, which is `@staticText(domain.StaticOverline, ...)` -- `eyebrow` role, `faint` ink -- and an `<h2 m-0 text-sm font-medium>` panel title, which is `@staticText(domain.StaticPanelHeading, ...)` (body role, emphasis weight; separate from `subheading` pending owner decision D7). The vocabulary gate derives a kind's Go name by title-casing each hyphen-separated word, because it used to capitalise only the first letter and a hyphenated kind could never match. Mutation-proved on one site of each. **The test to ask of any new Theme role: who *reads* it, not who may write it** -- the `ToneWarn` lesson from the other direction.
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
  **Padding (D4–D6) is declared as six roles, not 56 values, and three things changed on measurement.** `px` and `py` came out as the *same* eight-amount set over what a primitive can reach (the two ladders differ in each role's default, not in membership); the all-sides ladder is the smaller one and stops at `five`, because a base `p-6` had no site and the first build emitted exactly that one new class (`app.css` is byte-identical to HEAD now). `panel` and `section` are **both `five` by default since owner decision D6 (2026-10-07; `panel` was `four`)** and stay two roles a Workspace can declare apart. `panelLayout` is reached only by the UI IR walker's `case LayoutPanel` and **no tree builds a panel node** (`grep LayoutPanel`), so equalising moved no rendered screen -- the "one site, 4px" first recommended was wrong by one more step of measurement. The one-offs (`pt-18`, `pb-24`, `pr-11`) are excluded in `domain.PaddingRole`'s comment because each is sized by another element.
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
- `TestNoHandWrittenMarkGood` — a zero-floor gate (2026-10-11): `bg-`/`border-`/`hover:border-emerald-600` is
  written only by `markGoodFill`/`markGoodBorder`/`markGoodHoverBorder` in `layout.templ`. Those readers are the
  Design System artifact's `mark-good`: a plain reader and **not** a Theme role, because the artifact names it outside
  `tone-*` on purpose (no Workspace key moves it). Three sites moved byte-identically, with render output compared
  at HEAD and after. Five emerald-500 "finished" marks on the approval screens are the value the artifact rejected
  for contrast; see the extension below.
  Mutation-proved twice: a class attribute, and a Go string inside `templ.KV`. **Extended 2026-10-12:** the owner moved
  the five approval-screen marks (`approvalinbox` progress bar; `reviewdocument` done step and three "signed"
  markers) from emerald-500 (measured 2.46:1 on white, below WCAG 1.4.11's 3:1) to `mark-good` (3.67:1). The gate now
  refuses `bg-`/`border-emerald-500` too, except the closed `markGoodExempt` (the board's drag-and-drop slot, a
  JavaScript drop target, not a finished mark), which must match its count exactly. Mutation-proved both ways.
- `TestNoHandWrittenCallout` — a zero-floor gate beside `Callout`, the eleventh registered Component (2026-10-12;
  build, migrate, then gate). It is the first registration to follow the governance guide's procedure, and the
  first candidate found by GS7. A message box is `@callout(text, tone)`. The tone decides the Theme's fill and ink
  pair, and also the live-region role (`bad` is `alert`; the others are `status`), so a site no longer chooses
  the role. Six account-screen sites moved. The render-diff changed only the `role` attribute on five sites, plus
  one `text-sm` the parent already set. The gate counts a cornered class set holding a tone hue's `-50` tint and
  the same hue's ink, read after `joinClassExpressions`. Two floors sit in `calloutExempt`:
  - Delete account's blocker list (a title plus items);
  - `truncationNotice` (bordered and caption-sized; folding it is a design decision).

  Blind spots: a pair split between a parent and a child, a pair from a Go function, and a hue outside the five
  tones. Mutation-proved five ways. No installed `page:` uses Callout yet, because no declared screen shows a
  message.
- `TestHandWrittenControlBordersOnlyShrink` — a directive ratchet (2026-10-12, rencana 0.2d; read, migrate, then
  gate, with no new role). A control's rim reads `borderClass(ctx, domain.BorderControl)`. Six rims moved
  byte-identically: Account's three button-shaped controls, the inactive filter chip, and both complete-checkbox
  circles. **Twelve literal `border-slate-300` stay, read by meaning (R2), with a reason per file in
  `handWrittenControlBorders`:**
  - dashed empty states (4);
  - signature markers (3);
  - pre-auth inputs and links (5).

  The plan's "14 sites" was a count of a value. Reading the sites found six controls. Blind spots: a token from a
  Go function (the pending avatar's dashed ring), and variant prefixes. Mutation-proved four ways. Beside it,
  rencana 0.2e moved the approval wizard's Group picker to `fieldClasses`. That was the last approver picker
  carrying the recipe as a literal, and `TestApproverRow_groupPickerFollowsTheThemeLikeItsTwin` pins it, because
  the field ratchet cannot see a class returned by a Go function.
- `TestHandWrittenTonePairsOnlyShrink` — a directive ratchet (2026-10-12, rencana 0.2c). A semantic tone reads
  `toneClass`/`toneFillClass`/`toneInkClass`. Seven sites moved byte-identically: three approver-chip states, the
  pending nav badge, the archived banner, Delete account's blockers, and the truncation notice. It reads **every
  string literal**, so a pair returned from a Go helper is counted too. **Ten stay, by meaning:** a palette
  (Application icons), a diff, a file type, a script string, the stepper's blue "done", and the urgent SLA pill
  (amber-700 against warn's -800). The audit's "±60" was a count of tokens; reading found 17 pairs.
  **Two things this slice taught:**
  - **Never put a Theme reader in a `templ.KV` key beside another reader.** A Workspace may map two tones to one
    palette, the keys then collide, and templ keeps the last value. The done chip drew with no colour. The default
    theme cannot show this, so the byte-diff passed. `TestTonePairs_followTheThemeAtMigratedSites`, under a
    remapped theme, found it. Choose the class in Go (`approverChipTone`).
  - **Moving a literal into a Go string drops a ratchet count without migrating anything.** Check every fall: the
    waiting chip's ink now really reads `InkSecondary`.

  Also, the Callout gate now reads inline message-tone readers as their literals (`toneReadersAsLiterals`).
  Without that, its two floors vanished when their colours moved to the Theme.
  **After the independent review:**
  - The tone-pair gate counts a pair split across literals or `templ.KV`s.
  - It also fails a half migration (one half read through a reader, the other written by hand).
  - The Callout gate counts a tone passed as a variable and the other corner sizes, and it refuses `calloutClasses`
    outside its renderer. That last one is the box without the role.
  - Comments are stripped by `stripTemplComments`, which keeps the `//` in `https://`. That `//` used to swallow
    the class after it on the same line. HTML comments are stripped too.
- `TestNoTwoKVKeysFromOneThemeReader` — a zero-floor gate (2026-10-12). In one class expression, a `templ.KV` key may
  not be able to equal another class part under **any** Theme. templ keys classes on the whole string, and a later
  `false` disables the same string. Each part's possible outputs are computed from the readers' own literal
  `return`s, concatenated through `+`, and any intersection with a KV is a finding. Two shipped sites lost their
  class this way (the approver chip, and Application Settings' current link); both now choose the class in Go.
  **The first version only compared reader names, and the independent review broke it four ways.** "Different
  readers cannot collide" was false: `inkClass(InkFaint)` and `toneInkClass(ToneMuted)` are both `text-slate-400`
  under the default theme. It also missed a plain part beside a KV, and a literal key beside a reader key. Six
  shapes are mutation-proved.
- `TestHandWrittenRadiiOnlyShrink` — a directive ratchet (2026-10-12, rencana 0.2b). A control, surface or pill
  corner reads `radiusClass(ctx, domain.Radius*)`. **57 of 117 corners moved**, and two checks were byte-identical:
  - 106 authenticated pages (the GET sweep plus every per-record route, rendered through the router, with
    volatile ids normalised);
  - 12 component renders for the sites those pages cannot reach.

  The 60 that stay are read by meaning, with a reason per file:
  - circles and bars (geometry: a theme moving `pill` would square them);
  - icon tiles (the artifact's `radius-brand-mark`, no role);
  - `rounded-lg` menu rows beside `rounded-md` ones (a fold is a decision);
  - `rounded`/`rounded-xl` (DS-D17);
  - pre-auth sites and chat bubbles.

  **The slice's second lesson was L13 again, at scale.** Five shape gates (chip, surface, tile, caption, Callout)
  recognise a floor partly by its literal corner, and lost floors the moment corners became readers.
  `joinClassExpressions` therefore now reads `radiusClass(ctx, domain.Radius*)` as its default class
  (`radiusReadersAsLiterals`). Only this ratchet reads the raw form (`joinClassLiterals`). The rewrite then
  surfaced a box the Callout gate had never seen: the leave guard's warning, now a floor with its reason.
  `TestRadii_followTheThemeAtMigratedSites` covers R9. Mutation-proved: a literal put back, a lowered floor, and
  the rewrite removed, which fails three gates.
  **The rewrite is not for every gate.** The independent review showed it blinded `TestHandWrittenLayoutSitesOnlyShrink`,
  whose patterns anchor at the start of the class value: a site opening with `radiusClass` gained a leading corner.
  That gate reads `joinClassLiterals` now. `TestRadiusRewriteMatchesTheDefaultTheme` holds `radiusDefaults` to
  `domain.DefaultTheme()` and `radiusClass`'s arms. The same review completed two parts of the migration:
  - the board card's open popover dropped a stray `open:rounded-md`;
  - the two remaining count badges and both small file and app tiles now read the role too.
- `TestHandWrittenLadderGapsOnlyShrink` — a directive ratchet (2026-10-12, rencana 0.2a). `gap-2` through `gap-5`
  are the Theme's gap ladder (`GapTight`..`GapLoose`). That ladder, unlike `xtight` and `region`, carries no meaning
  beyond size, so **every authenticated site reads `layoutGap`**. 137 moved, the audit's number exactly. Two checks
  were byte-identical: 106 router pages, and 17 component renders for the sites the fixture does not reach. Only
  pre-auth screens keep literals (12 across 4 files). The layout ratchet's counts did not move. That was measured,
  not assumed, because its patterns do not key on the gap literal, so no reader rewrite was needed (unlike radius).
  `TestLadderGaps_followTheThemeAtMigratedSites` covers R9. Mutation-proved both ways.
- `TestEveryThemeCategoryHasAnImplRatchet` — G1 (2026-10-12, rencana 0.4): **every Theme role's default class is
  counted by a gate, or is listed in `themeRolesWithoutImplRatchet` with its reason.**
  - Roles come from the `domain.Known*` sets.
  - Each default is derived from code: the step from `DefaultTheme()`, then the class from the reader's own
    `case … return` arms in layout.templ.
  - A synthetic `<div class="…">` goes through the same counting logic the category's gate uses. The tokens are
    shared constants, not copies.
  - Run it with `-v` for one line per role: 32 are counted and 8 are exempt (background base/sunken, the wells
    decision; tone neutral/muted, grey collisions; the four badge paddings, generic spacing).
  - **The first version checked only that a gate *name* existed.** The independent review satisfied it with
    `"Padding": {"TestAppManifestLoads"}`, and found those eight roles uncounted behind a hard-coded "Padding: 0".
  - The plan's per-file `dsImplDebt` map was not built: it would double-count the ratchets 0.2 had installed
    (001 #8).
  - Coverage closed two categories first:
    - padding: 14 `p-5` on surfaces now read `paddingClass`. `TestNoHandWrittenPanelPadding` (zero floor) and
      `TestPanelAndSectionPaddingMatchTheirElement` hold them; the second exists because panel and section share
      `five`, so no byte-diff can catch a swap.
    - `border.surface`: 8 edges now read `borderClass`. `TestHandWrittenSurfaceEdgesOnlyShrink` keeps one floor, the
      read-only email box, and `TestSurfaceEdges_eachMigratedSiteReadsTheSurfaceRole` pins every edge at its own
      element under the default theme, since the three border roles have different defaults.
  - Only the surface and tile shape gates read `borderClass` as its literal (`borderReadersAsLiterals`); the value
    gates do not.
- `TestEveryUnnamedDesignValueHasAReason` / `TestDesignBucketAndImplRatchetsSplitTheTokens` — G1's `design` bucket
  (2026-10-12, rencana 1.3). The impl ratchets count a role's **default** and say "read the Theme instead". That
  advice is wrong for a value **no role names**, such as `text-white` on a filled mark, `bg-slate-900` (the Design
  System's `primary`), `gap-2.5` or `accent-blue-500`. Such a value is counted here, per value, in
  `unnamedDesignValues`, each with its reason. Read the numbers out of the map (126 tokens, 48 values when written).
  - A new value fails with a different message: register a role through the governance guide's procedure, or add a
    floor with its reason (R1).
  - Families: size, weight, ink, fill, edge (border, divide, ring, outline, accent, ...), gap, and corners outside
    `radiusToken`. Colours include `input.css`'s own `@theme` colours (`bg-brand`), arbitrary hex and `!important`.
  - Left out, each for its reason: padding (roles by job, D4), the corners the radius ratchet counts, `text-[Npx]`
    (`handWrittenOffLadderSizes` is this bucket's size part), and `transparent`/`current`.
  - **One token, one gate.** `classifyClassSet` follows the impl ratchets' own patterns first: anything the tone-pair,
    tone-edge or radius ratchet matches is theirs, even beyond the defaults (`bg-red-100` in a pair, `border-emerald-300`).
  - **A tone's fill or ink written alone** has a role, but the pair ratchet cannot see half a pair. It is held in
    `loneToneHalves` with the impl advice. Before the review, about 16 such sites were counted by nothing.
  - `TestDesignBucketAndImplRatchetsSplitTheTokens` requires every role default, written alone, to land in one
    bucket, and pins the classification with cases. Its first version compared the bucket's regexes with themselves
    and could not fail.
  - Mutation-proved in all seven families, plus a stale entry, a lone half, a pair, and the reviewer's narrowed
    family. Blind spots: variant prefixes, Go strings (74 family tokens), and backticks or a `}` in a class expression.
- `TestKeyboardFocusRuleCoversInteractiveElements` / `TestNoOutlineNoneBesideAFocusRing` — owner decision DS-D14
  (2026-10-12). Keyboard focus is **one base rule** in `static/css/input.css`, as the mockups have it:
  `:where(a[href], button, summary, [tabindex]):focus-visible` draws a 2px `--color-blue-300` outline (the Design
  System's `focus-ring`). Before this, links and buttons drew each browser's own ring. **Measuring it found that no
  field had shown a focus ring since Tailwind v4.** `outline-none` sets `--tw-outline-style: none`, and
  `focus:outline-2` reuses it, so Chromium reported `outline-style: none` on a focused input. `outline-hidden`
  does the same. Fields now write `focus:outline-solid`, and the second gate refuses the trap in class attributes
  and in Go helpers' strings. The first version of that gate missed `fieldClasses` (a Go function), which a mutation
  showed. Both gates are mutation-proved.
  **After the independent review (same day):** the rule's selector covers `input`/`select`/`textarea` too (checkboxes,
  radios and file inputs drew the browser's 1px ring). An `sr-only` input's stand-in draws the ring: the
  notification toggle's track uses `peer-focus-visible:`, and the attachment button uses `has-focus-visible:`.
  Measured in Chromium, all five draw 2px blue-300. The kill gate now reads class sets joined and `[]string{}`
  recipes joined, and refuses any `outline-none`/`outline-hidden` without `focus:outline-solid`, because a utility
  outranks the base rule.
- `TestHandWrittenAccentInksOnlyShrink` — owner decision DS-D10 (2026-10-12). Links and accent text read
  `inkClass(ctx, domain.InkAccent)`, with hover from `accentHoverClass(ctx)`. A Workspace may declare
  `theme.ink.accent` as one of six hues, each at the step GS3 measures above 4.5:1. `domain.InkShadeFits` keeps hues
  on the accent and greys on the other four roles. 42 sites moved, and the 106-page diff showed only `&`→`&amp;`
  inside one arbitrary-variant class, which is the same DOM. Floors:
  - ten links that hover to blue-800, not 700 (folding the two link hovers is a decision not taken);
  - pre-auth sites;
  - the Tag palette's blue;
  - a signature marker.

  **The accent lives in `inkClass`, not a separate reader, and that is the lesson of the slice.** A first version
  had `accentInkClass`. G1 and GS3 asked `inkClass` for the accent and silently measured the grey default:
  "17.84:1" for a link that is really 5.26:1. One reader leaves no road around it. GS3 measures every hue, not only
  the default. Mutation-proved three ways.
- `TestColourRoleClassesComeOnlyFromTheirReaders` and `internal/rendering`'s `TestThemeCSS*` family — owner decision
  DS-D15 stage 2 (2026-10-12): **a colour role is a CSS custom property, and a Workspace may give it a value.** Ink
  came first (3.2a). Since 3.2b **every colour role** draws through one: border (`border-border-surface`), surface
  (`bg-bg-raised`), `hover-wash` (an alias of `bg-base` in `@theme`, so moving the page moves the hover), and each
  tone's fill, ink and edge (`bg-tone-good-bg`, `text-tone-good-ink`, `border-tone-good-border`). Only ink takes a
  colour value so far; the others still choose a shade or palette. Since 3.2c `background` takes one too. A value may
  **tint, not darken**: no darker than the `light` shade (`domain.SurfaceLuminanceFloor`). Contrast is luminance
  alone, so every colour the loader does not measure stays as readable as on that shade: hand-written literals, tone
  inks, `bg-white` fields and mark-good. `TestSurfaceFloorKeepsFixedColoursReadable` checks that on the floor shade.
  The 3.2c review loaded a navy theme with white inks, and every field's text was white on white. Inks are still
  measured, and all three surfaces must sit at least 1.02:1 apart: base is the hover wash, sunken a lane or a
  selected row. Border and tone stay closed, because a decorative edge has no honest floor (the
  default surface edge is about 1.2:1) and a tone is a triple. Verified in Chromium: the hover wash follows a value
  base through its alias. Radius and space follow:
  - `inkClass` answers one class per role (`text-ink-body`, `text-accent`), the same under every theme. The
    property's default sits in `static/css/input.css`'s `@theme` under the Design System's own token names, so a
    page with no Workspace (pre-auth) draws the default. A moved role is an override in the page's `<head>`
    (`rendering.themeCSS`/`themeStyle`), empty for the default theme.
  - `theme.ink.<role>` takes a shade **or** `"#rrggbb"` (not on `faint`). Once a Theme moves any ink or background
    role, **every** ink role, shade or value, is refused below 4.5:1 on the surfaces it is drawn on
    (`domain.InkSurfaces`: `sunken` too, except for `secondary`, whose default is 4.35:1 there;
    `metadata.inkContrastIssues`). Domain's ink and background colour tables are held to `app.css` by
    `TestInkShadeColoursMatchTheCompiledCSS`/`TestBackgroundShadeColoursMatchTheCompiledCSS`.
  - **The independent review found three survivors and two holes, all closed.**
    - `ParseColourValue` survived a G/B swap, because every reference was a grey. It now has chromatic references.
    - A hue arm swapped to another hue passed, because both cleared 4.5:1. `TestEveryAccentHueDrawsItsOwnHue` now
      catches it.
    - Hover was never measured. `TestAccentHoverIsAtLeastAsReadableAsTheAccent` covers the closed steps and the
      free `color-mix`.
    - Closed shades on a moved background, and ink on `sunken`, were not checked.
    - One negative assertion (`text-2xs text-slate-400`) became vacuous.
  - **A byte-diff no longer proves a colour migration.** Tests compare what a class computes to:
    `resolveColourRoles` rewrites a role class through the override and the `@theme` default, and leaves a literal
    alone, so an R9 test still fails on a hand-written colour. GS1 and GS3 follow the `var()` alias in `app.css`;
    GS3 resolves each accent hue under its own Workspace (without that it measured the default blue six times and
    passed, mutation-proved). Both slices were verified as computed style in Chromium against the pre-3.2a pages:
    106 pages, 15,607 elements, 0 differences. 3.2b also hovered one element per distinct hover class set (26 of
    37; the other 11 sit in closed menus): 0 differences.
  - **Reader tests now resolve before they compare.** 3.2b broke 37 rendering tests. They were rewritten
    mechanically: each `buf.String()` after a `Render(ctx, …)`, and each direct reader call, is wrapped in
    `resolveColourRoles` with that ctx. Then every negative assertion on a palette class was swept by hand.
    A negative check on **unresolved** output still catches a hand-written literal, but it is vacuous for the
    reader's own output, which is how `TestMetric_hintAndToneAreOptional` went silent in 3.2a.
  - **Tailwind emits any `--color-*` a scanned file mentions**, so `inkShadeVar`'s literals keep every choosable
    step in the bundle. `TestEveryPaletteVariableTheOverrideCanNameIsCompiled` catches a name the scanner cannot
    read (a runtime-assembled one), not a renamed step.
  - The gate refuses a role class anywhere in `internal/rendering` outside its nine readers, Go strings included,
    under any variant prefix, opacity or `!`, and in **any** utility family (`border-t-`, `caret-`, `shadow-`, `from-`:
    Tailwind makes a colour utility of every family from a `--color-*` property, and the 3.2b review passed seven
    through the first, listed set). `themeCSS` ranges the domain's closed sets, and
    `TestThemeCSSWritesEachRoleAndToneWhenMovedAlone` moves each role and tone alone (the review dropped `neutral` and
    `muted` from a hand-written list and nothing failed). Mutation-proved: a class attribute, a Go string, a renamed reader, and the five
    shapes the review passed through the first regex.
  - Open: in `app.css` the role classes sort before some palette classes, so an element carrying both a role and a
    conflicting literal colour would change which one wins. None was found (the 106-page computed diff is 0), but
    parameters and states the dev DB cannot reach are not covered.
- **Rencana 3.2d, tinta (2026-10-13):** every literal `text-slate-900|700|500` in a `.templ` reads
  `inkClass` (217 sites, pre-auth screens included, since a reader there falls back to the `@theme` default). So
  `TestHandWrittenThemeTokensOnlyShrink`'s ink count is **zero** in every file. The computed-style diff of 106 pages
  showed no change from the migration itself. Two things it taught:
  - **L13 a third time.** The muted-paragraph and caption ratchets recognise their floors partly by `text-slate-500`,
    and both lost every floor. They now read `inkReadersAsLiterals` (held to the code's defaults by
    `TestInkRewriteMatchesTheDefaultTheme`). The muted-paragraph gate also joins class expressions now; it used to read
    raw attributes only.
  - **That widening found sites 1.2b had made.** Faint text moved to `secondary` had become exactly a
    `StaticMessage`/`StaticCaption` shape, and the gate could not see it. Five such sites moved to `staticText`; two stay
    as floors with reasons (a scripted `hidden` note, `px-0.5 leading-4`).

  **Then the rest of the colour, with no new role (owner decisions CP1–CP7, 2026-10-13):** 143 more literals read an
  existing role. Some go through reader *forms*, which are new utilities over existing properties:
  - `inkFillClass` (the primary button is `strong` as a fill);
  - `onFillClass` (its text is the `raised` colour, an inverse pair the loader already measures);
  - `borderFillClass` (1px rules, avatars);
  - `inkHoverClass`, `sunkenHoverClass`, `controlAccentClass`, `accentActiveClass`, `baseTintClass`.

  The computed-style diff of 106 pages and the hover diff were both 0. **What still stays literal:** the PDF "paper"
  layer, the categorical palettes (Tag, app icons, file type; deferred to dark mode), the brand glyph, the focus
  ring and the scrim (right in both modes), a JS toast string, and the sites the owner chose to fold (slice B).
  - **The diff caught what no gate did.** The rewrite script joined two readers without a space
    (`border-ink-strongbg-ink-strong`), and a filter chip and two empty states drew no fill. Every test was green.
    A scan of every rendered class token against `app.css` selectors now finds none missing. That is rencana 3.5's
    G2, still unbuilt.
  - **L13 a fourth time.** The chip, h-9 button and panel-title gates read `colourReadersAsLiterals`, held to the
    code by `TestColourRewriteMatchesTheDefaultTheme`. The field gate does not: it is a value gate.
  - **Slice B (owner decision CP6) folded 14 inconsistencies, and the result is visible.** Measured in Chromium:
    576 chevron SVGs (slate-300 → faint), 79 count-badge fills and 31 danger inks (red-600 → red-700), the danger
    heading (red-400), two warn inks (amber-900), and 12 hover sets. The hover sets are link hovers to blue-700, card
    edges to `accent`/40, the primary hover to `strong`/90, and danger hovers to the bad ink or an underline. New
    forms: `toneInkHoverClass`, `toneInkFillClass`, `inkFillHoverClass`, `accentEdgeClass`/`accentEdgeHoverClass`,
    `accentRingClass`, `inkClassFromSm`. The board's drag-and-drop slot is mark-good in a script now
    (`markGoodScriptUses`), and `markGoodExempt` is empty.
- `TestHandWrittenColoursHaveAReason`: **the closing gate of 3.2d.** Every palette colour still written anywhere in
  `internal/rendering` is counted per file, and each count carries a reason (`handWrittenColours`). That covers a class
  attribute, a templ expression, a Go string and a script string. What remains (since dark mode, 2026-10-13) is the
  focus ring (CP4), the PDF paper layer (CP5, DK9), the brand glyph, and mark-good (its readers and the board's drag
  script). Anything else must draw through a role or a categorical property.
  - It exists because the 3.2d review put `bg-red-600` back into the nav badge's Go string argument, and every other
    gate stayed green. The design bucket reads class attributes only.
  - It counts with a loop, because RE2 has no lookahead and `FindAll` swallowed one of two adjacent tokens.
  - The same review found:
    - `tone.bad: grey-faint` loaded and drew every count badge white on slate-400 (2.63:1). The loader's
      `fillPairIssues` now measures each tone's ink (but `muted`) on raised and base, the primary hover (`strong` at
      90% over raised, composited in sRGB by `domain.Colour.Over`), and `body` on the `border.surface` fill
      (avatars). Their colour tables are held to `app.css`.
    - `TestColourRewriteMatchesTheDefaultTheme` checked only 15 of its 22 entries. It now derives every entry from
      the reader's own return.
- **Dark mode, rencana 3.3 (owner decisions DK1–DK11, 2026-10-13):** `static/css/input.css` redefines every colour
  property under `@media (prefers-color-scheme: dark)`, so no site writes a `dark:` class. It follows the device,
  sign-in screens included. The decision sheet with measured ratios is `https://claude.ai/artifact/QPsrRHHP5HWyQeP6Trsdeo`.
  - `TestEveryColourPropertyHasADarkValue`: every light colour property has a dark one, or sits in `darkFollowsByAlias`
    with its reason (`hover-wash`, an alias of `bg-base`). Each palette step the dark block names must be compiled.
  - `TestDarkColourPairsMeetContrast` is GS3 for the dark block. It covers every ink role on all three surfaces, every
    accent hue, every tone and every tone palette a Workspace may remap to, and the inverse fills (primary button,
    count badge, avatar). It also covers mark-good, the focus ring, the Tag hues over their 10% fill, the app icons, and
    the file-type and draft tiles. The surfaces must stay 1.02:1 apart.
  - `TestThemeCSSCarriesOnlyHueChoicesIntoDark` holds DK11. `themeCSS` writes a light block
    (`@media (prefers-color-scheme:light)`) and a dark block that only carries hue choices (an accent hue, a tone
    remap) at their dark steps. Ink, background and border shades, and free values, stay light-only.
  - `TestDarkDefaultsMatchTheDarkReaders` keeps input.css's dark tone and accent values equal to the dark readers.
  - GS1 compares the artifact's `dark` theme with the dark block. The categorical colours (`tag-*`, `app-*`,
    `file-pdf-*`, `draft-*`) are Design System tokens and properties now, read by `tagClasses`, `appIconClasses` and the
    tile readers, and `TestColourRoleClassesComeOnlyFromTheirReaders` covers them.
  - **Test helpers split the two modes**: `themeVarDefaults` and `compiledColours` read the light side only, while
    `themeVarDarkDefaults` and `compiledDarkColours` apply the dark block. Without that split, a dark value silently
    replaced the light one in every resolved assertion.
  - **The independent review found two real defects, both fixed.**
    - **The PDF page PNG is transparent**, so in dark mode a document's dark text sat on the dark page. The page image
      now carries the paper itself (`paperClass()`, white in both modes), at all three sites.
    - **Text on the paper layer read Theme roles, which turn light in the dark.** A marker's approver name was slate-200
      on a pale marker, 1.2:1. Marks over the page read `paperInkClass` (fixed slate-700/500), and
      `TestPaperLayerReadsNoThemeRole` refuses a role reader in `reviewSignatureBox` and `placementBoxLabel`.
    - The author's audit could not see the first defect: it takes a background from `background-color` and ignores
      images. Paper is a judgement the gate states; no scan derives it.
    - The two `<dialog>`s now take their surface and ink from the Theme. Before, `color-scheme` left them on the
      browser's Canvas colours in dark mode.
    - `TestEveryColourPropertyHasADarkValue` now reads every `--color-*` in input.css, not only the names the readers
      use (a `--color-paper` read from a Go string had passed). `darkSameInBothModes` holds `brand`.
    - `TestNoSiteWritesADarkVariant` (conformance) refuses any `dark:` class; `dark:opacity-50` had passed every gate.
  - Verified in Chromium:
    - light mode is 0 colour differences over 106 pages against HEAD (before the review fixes; after them, only the
      three page images' white backing and the two dialogs' ink move);
    - a contrast audit of every text element in dark mode found only faint and muted text below 4.5:1 (by design;
      higher than in light) and the brand glyph, which is the same in both modes.
- **G2, the closed palette (rencana 3.5, 2026-10-13).** input.css's `@theme` starts with `--color-*: initial` and then
  declares the 62 steps something names (60 hue steps plus white and black), at Tailwind's own values. A colour
  utility outside the palette (`text-cyan-600`) now compiles to nothing. The compiled app.css is the same as before,
  except for the order of the declarations. Three gates hold it:
  - `TestClosedPaletteDeclaresExactlyWhatIsNamed` (conformance) checks both directions. A declared step that nothing
    names must be removed. A step named in `internal/rendering` (as a property or as a utility) or in input.css that is
    not declared fails, because a reader's `var(--color-…)` would resolve to nothing, and no class check can see that.
  - `TestEveryClassTokenIsCompiled` (conformance, static) checks every utility-shaped token in a class attribute or a
    string in `internal/rendering`, `.go` files included, against app.css's selectors. A missing class is reported by
    token and file. It cannot see a class assembled in Go or a bare word.
  - `TestEveryRenderedClassIsCompiled` (`internal/web`, needs `DATABASE_URL`) covers what the static gate cannot. It
    checks every class on 87 served pages: the GET sweep, every per-record case and the pre-auth screens. Classes that
    exist only for a script sit in `classHooks`, closed both ways, with what reads each.

  This catches the failure 3.2d hit, where two readers joined without a space and the element drew without its fill
  while every test stayed green. Measured: no class was missing. Two hook classes that nothing read (`approver-type`,
  `sig-width-label`) were removed. A palette step is not a property with a mode, so the dark-value gate skips steps and
  refuses a dark block that redefines one. Mutation-proved eight ways, including a class assembled in Go, which only
  the rendered gate catches.
  **The independent review found four ways through, all closed:**
  - a Go `/* */` or CSS comment counted as "naming" a step (`stripBlockComments`);
  - a palette colour in a utility family nothing compiles yet (`fill-`, `stroke-`, `caret-`) was not judged;
  - `*:`/`**:`/`@sm:` variants did not parse;
  - classes added by a script attribute (`_="add .x"`, `onclick="…toggle('x')"`) were deleted with `id`/`href`.

  It also found the sweep never draws a placed signature marker. `classHooks` entries carry `reached`, closed both ways.
- **DTCG export, rencana 3.4 (2026-10-13).** `rendering.ExportDesignTokens(ws, appCSS)` writes a Workspace's Theme as
  W3C DTCG 2025.10 tokens:
  - a Resolver Module document (`menata.resolver.json`), which applies a `base` set, then a `mode` modifier with
    `light`/`dark` contexts;
  - `base.tokens.json` for spacing, radius and typography;
  - `light`/`dark.tokens.json` for colour, under the same names.

  Every value is what a page in that Workspace computes: app.css's defaults and dark block, with `themeCSS`'s override
  applied over each mode. So DK11 holds by construction: a hue choice carries into dark, while a free value or a shade
  stays light-only. The token names are the Design System artifact's. **The token-to-reader list is
  `rendering.designTokenClasses`, which GS1 now reads too**; before this slice it lived inside GS1's test body, and a
  second copy for the exporter would have been 001 #8. Use it:
  - `make tokens` regenerates `design-system/dtcg/default/`;
  - `go run ./cmd/tokens -workspace <slug> -out <dir>` exports one Workspace;
  - `design-system/canvas.py` takes a `*.resolver.json` wherever it takes tokens.json, and has a `compare` mode.

  Gates:
  - `TestDesignTokensValidateAgainstTheDTCGSchemas` validates every file, for the default Theme and for a Theme that
    moves every category, against the Community Group's own schemas. The schemas are vendored in
    `internal/rendering/testdata/dtcg` and compiled offline with `santhosh-tekuri/jsonschema`; a deliberately broken
    copy must fail.
  - `TestCommittedDesignTokensAreFresh` checks that the committed export matches a fresh one.
  - `TestDesignTokensMatchTheDesignSystemArtifact` reads the export back through its own resolution, not GS1's, against
    tokens.json in both modes, letter spacing included.
  - `TestDesignTokensFollowAMovedTheme` covers R9, including the free accent's `color-mix` hover, which is evaluated in
    OKLab because DTCG has no mix.

  Mutation-proved eight ways, including the override ignored, the dark block not applied, lightness as a percentage,
  and the mix weights swapped. **Two gates read the new file and were narrowed with care, not silenced.** G2's static
  scan skips Go map *keys* in `designTokenNameFiles` only (a bad class *value* there still fails; mutation-proved).
  `faintInkSites` gained `designtokens.go:designTokenClasses`, which names `ink-faint` but draws nothing. The export does
  not carry the artifact's unmapped tokens (`dsUnmapped`), a style's `uppercase`, or the touch half of
  `control-height-compact`.

- **Component Status and G3, rencana 2.0/2.1 (2026-10-13).** `domain.ComponentContract` carries `Status`
  (`experimental | stable | deprecated`) and `ReplacedBy`. **The words are tied to a measurement, not to opinion:**
  - `stable` means an installed Workspace's `page:` depends on the contract, either by declaring the Component or
    because lowering emits it. `Field` and `Input` are written 0 times and lowered 8, so counting YAML text would have
    marked them unused.
  - `experimental` means no installed manifest depends on it. `Avatar` and `Callout` are drawn by bespoke Go screens
    and by no declared page.
  - `deprecated` needs a `ReplacedBy` naming a registered, live Component.

  Gates (`internal/conformance/componentregistration_test.go`):
  - `TestComponentStatusMatchesInstalledUse` counts lowered Components over every installed page, then holds status to
    use both ways. `componentsWithoutInstalledUse` is closed and carries a reason per entry. The count is over
    *declared pages*, so "no use" is not a verdict on how widely a Component is drawn.
  - `TestEveryRegisteredComponentIsInCapabilities` reads the whole row, not one cell, because the Button row names its
    renderer in prose. That is presence, not accuracy.
  - `TestWritingGuideShowsEachComponentsStatus` holds the status table in writing-guide §12.1a to the registry:
    one row per Component, the contract's word, a deprecated row naming its successor. The comparison is
    `statusTableProblems`, extracted so `TestStatusTableProblems` can feed it a deprecated member; no real Component is
    one, and a branch no real input reaches had not been shown to fail.
  - `registry.CatalogueProblems` checks the contracts' own rules and that a successor is registered and not itself
    deprecated.

  Mutation-proved 14 ways. One mutation did not apply the first time (`gofmt` had realigned the line), and the harness
  now aborts on a no-op. **Nothing at runtime reads Status yet**, and that is deliberate: the day a Component is
  deprecated, `metadata.validatePages` should warn on a new use and name `ReplacedBy`. No input can reach that branch
  today.
  **The independent review measured the values in Chromium**, painting each token's colour to a canvas, light and dark,
  under the default Theme and two moved ones. It found 0 differences, but three mutants that only the snapshot caught,
  and running `make tokens` would have re-blessed each one:
  - sRGB components swapped;
  - the resolver's light/dark refs swapped;
  - a broken OKLab matrix.

  Now the artifact test reads the documents **through** `menata.resolver.json`, an sRGB colour's components must say
  its hex, and four hex fallbacks are pinned to Tailwind's published values. Also fixed:
  - a colour's extension names only the property (a hover class minus `hover:` compiles to nothing);
  - `cmd/tokens` loads tolerantly;
  - `canvas.py compare` checks type styles;
  - `check` warns on a `var()` that nothing defines. It only warns because it has not been run over the live canvas.
- `TestFaintInkSitesHaveAReason` — rencana 1.2b (2026-10-13). The Design System defines `ink-faint` (slate-400, about
  2.6:1) as "hints, placeholders, decoration only" and says it fails WCAG AA for real content. Reading all 98 faint sites
  found most were content, so they moved to `secondary` (slate-500, 4.77:1):
  - emails, roles, SLA lines, document references, timestamps, member counts;
  - empty-state messages, New Application's field labels, section labels (`StaticOverline` included);
  - a Metric's hint, which a page can bind to a computed figure (`hint_figure:`);
  - a not-built setting's explanation and its tag;
  - nine controls (icon buttons need 3:1, a text button 4.5:1).

  Measured in Chromium: 772 elements on 63 of 106 pages changed, every one of them text moving to slate-500, and
  nothing else.
  - **What stays faint is a judgement, so the gate holds a count per function with a reason** (`faintInkSites`, keyed
    `file:function`, in `readPathWriters`' shape). The reasons are a disabled control, ids, em dashes, a chevron,
    ordinals, a pending avatar's initials, a diff's hunk header, and the inference inspector's not-applicable rows.
    The `muted` tone counts as faint, because it draws the same slate-400.
  - **The independent review drove four roads around the first, per-file token count, and all four are closed:**
    - `domain.InkRole("faint")`;
    - an alias in another file;
    - `ToneMuted`;
    - moving faint ink between two functions in one file.

    An `inkClass` role must be a `domain.Ink…` constant at the call, and `domain.InkFaint` may appear only as that
    argument. A swap inside one function still passes; that is inherent to counting.
  - Both former literal floors (`avatarClasses`, `diffLineClass`) take ctx and read the reader. An earlier version
    of this bullet claimed the token ratchet counted them, and nothing did.
  - **Owner decision 2026-10-13: no `secondary` text on `sunken`** (4.35:1 there). The Not built yet tag reads `body`.
    The loader cannot require secondary to pass on sunken without refusing the default Theme, so
    `TestTextOnSunkenMeetsContrast` holds it at the site. It is mutation-proved, and it is blind to hand-written sunken
    surfaces. The same day the active nav became blue-700 on mobile as on desktop.
- `TestHandWrittenToneEdgesOnlyShrink` — owner decision DS-D16 (2026-10-12). A tone is a **triple** now:
  `toneEdgeClass(ctx, domain.Tone*)` gives the edge from the same palette as the fill and ink. No Theme key was added,
  and a Workspace remapping `warn` to red no longer keeps an amber rim. Six edges moved byte-identically (components
  and 106 pages): the archived banner, the truncation notice, the leave guard, the overdue approval card, and two
  danger zones. GS1 now checks `tone-warn-border` and `danger-border`. Floors are the script toast, the Danger and
  Deactivate button recipes (they move with DS-D11), and an unread highlight. The overdue card's edge is chosen in Go
  (`approvalCardEdge`), because two KV keys would collide under a grey `bad` (L12). The shape gates read that helper
  as the surface edge (`stateEdgeHelperCall`), or the card would leave the surface ratchet unmigrated (L13).
  Mutation-proved three ways.
- `TestHandWrittenWellFillsOnlyShrink` — owner decision W1 (2026-10-12): a well follows its colour. A slate-50
  well reads `backgroundClass(ctx, domain.BackgroundBase)` and a slate-100 one `BackgroundSunken`. 13 moved
  byte-identically: header bands, info wells, a file card, an image frame, menu icon wells, and two progress tracks.
  G1 no longer exempts `background.base`/`sunken`. Floors are the same grey doing another job: seven `h-px`
  dividers, chip fills, a selected tab, a step circle, disabled controls, a chat bubble, the pre-auth `<body>`.
  The R9 test moves base and sunken apart, so a track reading the wrong one shows. Mutation-proved three ways.
- **G1, the gap ladder's lower end (owner decision, 2026-10-12)**: `domain.GapMicro` (`gap-1`, 4px) and `GapHair`
  (`gap-0.5`, 2px) extend the ladder downward, as `GapRegion` extended it upward, and like tight..loose they carry
  no meaning beyond size. 105 class sets moved, and the 106 pages were byte-identical.
  `TestHandWrittenLadderGapsOnlyShrink` now counts both, and only pre-auth floors remain. **All three homes moved
  together (R8):**
  - the Design System artifact gained `gap-micro`/`gap-hair` (v43);
  - the repo copy `design-system/tokens.json` is at `1791800557-69fe`, and GS1 checks both tokens;
  - every artboard `:root` was regenerated by `canvas.py` (UI canvas v158), and the Builder canvas's copy was
    updated (v30);
  - the `DefaultTheme` snapshot moved with the recorded decision.
- `TestHandWrittenHoverWashesOnlyShrink` — owner decisions DS-D11 and P1 (2026-10-12). Hover lives in recipes, not
  as a role:
  - `hoverWashClass(ctx)` is the Design System's `hover-wash`, which the artifact defines as an alias of `bg-base`,
    so it reads `background.base`;
  - `toneHoverFillClass(ctx, tone)` gives a danger control its `bad` hover beside its edge and ink.

  38 hovers moved byte-identically, including the secondary and Danger button recipes, the Deactivate button and
  the danger menu row. GS1 now checks `hover-wash`, and `hover:bg-slate-100` (a darker hover) stays an open finding.
  **After the independent review (same day):**
  - A theme whose `background.base` equals `raised` or `sunken` is a load error, because then the hover would draw
    nothing or look like a selected row.
  - The gate's sanity check counts the readers, not the forbidden token, so it can reach zero.
  - Its pattern takes variant prefixes, opacity, `group-hover:` and every tone's `-50`.
  **P1 is the one visible change:** the five `rounded-lg` menu rows read `radius.control` (8→6px). It is the only
  token the 106-page diff showed moving.
- **GS6 is a tool, `design-system/canvas.py`, not a test**, because the canvas lives outside the repo. Its `generate`
  mode rebuilds every artboard's `:root` from `design-system/tokens.json`. Its `check` mode fails on a stale block
  and reports adoption (`var()` references; colour literals on and off the token values). Run it on every token
  change (R8). Proved: a corrupted `--accent` fails it, and regenerating at the same version changes no file.
- `TestDesignSimilarityReport` (GS7) is a **report**: run it with `-v`. It never fails (except on an empty scan). It
  groups hand-written class sets by visual fingerprint and lists groups of three or more sites across two or more
  files. The first reading (2026-10-12) gave 4 groups, all four genuine: the alert family (Callout candidate),
  tinted wells, hover menu rows, and hand-written list header bands. The sample is too small to promote it to a
  gate, and it misses shapes that occur only once or twice per fingerprint.
- `TestRejectedDesignValuesStayOut` (GS2, 2026-10-12) — the Design System artifact now lists what it *rejected* for a
  meaning in a `rejected` token family (shown on its page). Today that is `body-compact` 13px and `mark-good-500`
  (emerald-500 as a finished mark). This gate reads the family from `design-system/tokens.json` and refuses the
  matching Tailwind classes in `.templ`, except the closed `rejectedDesignExempt`: the workspace monogram's 13px
  glyph, and the drag-and-drop slot. Every rejected entry must have a pattern, and stale patterns and exemption
  counts fail. Mutation-proved three ways. The third attempt **did not apply at first** (`gofmt` had realigned the
  line); `grep -c` caught it before a green run could pass for a gate that bites.
- `TestDocumentedDesignSystemCountsMatchTheCode` (GS5, 2026-10-12) — the two fixed-wording count claims in the
  normative and agent docs are held to the code: the Theme category count against `domain.Theme`'s map fields
  (parsed from source), and the registry's contract count against `len(registry.Components)`. Both had gone stale
  (writing-guide said six where there are nine; 007 §40 said seven where there are ten). **Narrow on purpose:** the
  ratchet narratives in this file also held stale floors, but they take too many shapes to match without false
  findings, so they were rewritten as dated numbers that point at the map. Mutation-proved both ways. This
  bullet avoids the exact phrases the gate reads, because a gate's own description sitting inside its data is the
  trap recorded above.
- `TestDesignSystemTokensMatchTheCode` (`internal/rendering`, GS1 of the governance guide, 2026-10-12) — the gate
  *between* sources. `design-system/tokens.json` is a copy of the Design System artifact's file, with the version in
  `design-system/README.md`, and is never edited by hand. Every token and type style is compared, in the artifact's
  own notation (oklch, px, size/line-height/weight), with what the default Theme's reader answers as compiled into
  `app.css`. **Every one of the 64 tokens and 12 styles must be classified:** checked;
  `dsAcceptedNotInCode` (shrink-only: `gap-region`, `control-height-compact`); or `dsUnmapped` with the reason no
  reader answers. An unclassified new token fails, and so does a stale entry. Mutation-proved four ways: a colour
  in the copy, a label line height in the copy, a new token in the copy, and `PaddingPanel` in code. **When the
  artifact changes, copy its `tokens.json` here in the same commit as the matching code.**
- `TestThemeColourPairsMeetContrast` (`internal/rendering`, GS3 of the governance guide, 2026-10-12) — asks the real
  readers for their default classes and resolves each to the `oklch` Tailwind compiled into `app.css`, then holds:
  every tone pair, every ink role on `raised`/`base` at 4.5:1, and `mark-good` at 3:1. `lowContrastByDesign` is
  closed both ways (`ink faint`, `tone muted`, each with a reason). Run with `-v` to see every ratio. The closest
  pass is `ink secondary on base` at **4.55:1**: one step lighter would fail. Mutation-proved three ways (mark-good
  to emerald-500, warn ink to amber-600, an exception deleted). The default Theme only; a Workspace remap is not
  measured.
- `TestDefaultThemeMatchesItsRecordedSnapshot` (`internal/domain`, GS4, 2026-10-12) — `DefaultTheme` is compared
  with `testdata/default_theme.json`. Every Workspace without `theme:` renders it, so a one-line edit restyles the
  product. Passing means doing it on purpose: a DSD entry with its reversal condition, the Design System artifact
  mirrored, then `UPDATE_DEFAULT_THEME=1`. Mutation-proved (`PaddingPanel` back to `four`).
- `TestHandWrittenCompactHeightsOnlyShrink` — beside `controlHeightCompact()` (2026-10-12), the Design System's
  `control-height-compact`. It is 32px **with the artifact's condition built in**: the reader returns
  `h-8 pointer-coarse:h-11`, so a site cannot take the height without keeping 44px on touch. Review's View
  PDF/Download moved. Measured in Chromium: mouse 32→32px; touch emulation **32→44px**, a deliberate mobile change
  the artifact had already decided. The attachment kind tile is not a control and stays. It is a plain reader,
  not a Theme role: Theme has no height category, like `control-height` itself. With it, GS1's
  `dsAcceptedNotInCode` is **empty**. Mutation-proved both ways.
- `TestHandWrittenRegionGapsOnlyShrink` — beside `domain.GapRegion` (2026-10-12), the Design System's `gap-region`
  (24px). **Of three `gap-6` sites, one meant a region** (the pre-auth shell's brand block over its card); it moved
  byte-identically. The two in `rolematrix` are column gutters, so the same value with a different meaning stays
  hand-written (R2). One is frozen here; the other is a `sm:` variant. Mutation-proved both ways. GS1 now checks
  `gap-region`, and the `DefaultTheme` snapshot moved with decision DS-G1 in `design-system-decisions.md`. **GS1
  blind spot found on the way:** it cannot tell that an `accepted` token has since landed in code. `gap-region`
  stayed unchecked until it was moved by hand.
- `TestHandWrittenHalfStepGapsOnlyShrink` — the ratchet beside `domain.GapXtight` (2026-10-11), the Design System's
  `gap-xtight` (6px). Thirteen inline icon/dot+label sites moved to `layoutGap(ctx, domain.GapXtight)`; 15 sweep pages
  compared byte-identical. **Nine `gap-1.5` stay as floors** because they space *components* (vertical stacks, a
  button row), which the artifact puts outside the token's scope. This reverses the 2026-10-03 `row` call ("no
  half-step"), on evidence that call did not have: the artifact's tested finding that 8px breaks the board's lane
  titles. The five sites folded into `GapTight` that day stay folded. Mutation-proved both ways.
- `TestCustomTextSizesDeclareTheirLineHeight` — every custom `--text-*` size in `input.css`'s `@theme` must
  declare `--text-*--line-height` beside it (2026-10-11). Tailwind pairs each built-in size with one; a custom size
  without it compiles to a `font-size`-only utility, and the element inherits its parent's line height. `text-2xs`
  and `text-3xs` shipped that way. Measured in a browser, 416 of 738 such elements across 16 pages rendered an
  11px label at 15.71px instead of the Design System's 16.5px. Both now say 1.5. **A CSS-only change leaves
  every HTML byte identical, so a render-diff cannot see it**; it was measured as computed line heights, old CSS
  against new. Only `/inference` changed height (+87px desktop). Mutation-proved twice: the line deleted, and the
  line left inside a comment.
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
  records the **remainder** (`goNamedDatasetsWithNoEngineRequirement`, read the ids out of the map) rather than gating it,
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

**Two sessions on `ROADMAP.md` `## Planned` at once: split by the files a lane touches, not by priority
number** (2026-10-10). Two sessions rarely collide in logic; they collide in a handful of shared files.
The lane table, the order inside each lane and the reasons are `menata-app-document`'s
`audits/2026-10-10-pembagian-dua-sesi-roadmap.md`; the rules every session follows are these:

- **Lane A (runtime)** owns `internal/metadata`, `data`, `execution`, `authorization`, `aiassist`,
  `cmd/server`, `internal/web/ratelimit.go`, non-design gates, and `metadata/activity.yaml`. **Lane B (UI)**
  owns `internal/rendering`, `ir`, `composition`, screen handlers in `internal/web`, and every Application
  file under `metadata/applications/` with all its Workspace copies. **Only lane B edits a `.templ`**:
  `make generate` rebuilds `static/css/app.css` and `*_templ.go` from every templ file, and two sessions
  doing it produce a generated file no merge can reconcile.
- **Claim before starting** (one line to the other session, `SendMessage`), and when blocked on an owner
  decision take the next item in your own lane, never one from the other.
- **`ROADMAP.md` and `roadmap_test.go` change only at a slice's end**, as their own small commit, after
  `git pull --rebase`; recompute the ratchet numbers from the rebased file instead of keeping yours.
- **A shared file is edited only on the lines your item owns** (`capabilities.md`, `writing-guide.md`,
  `router.go`); a conflict at the tail of `development-history.md` keeps both entries.
- **Restart `menata-app.service` only from the session that just pushed**, and verify the running binary.

**Since 2026-10-11 development runs in one session, and the lane rules above are a fallback, not the
default** (owner direction 2026-10-10 that further work is mostly lane B; session A has an exhausted queue
and is idle). While only one session is active: the lane-A files may be edited by that session, no claim
message is needed, and `make generate` is safe to run. **What stays**: every gate, mutation proof and
re-measurement; `git commit -- <paths>` and never `git add -A`, because the checkout can still be used by
another session at any time; and **deploy**, which still waits for the owner's permission (the permission
classifier refused it once for the working session). The lost second pair of eyes is replaced, for a slice
that adds a gate or a primitive, by a fresh-context agent whose only job is to break the result -- the
independent read of K15 item 3 found five surviving renderer mutations and a `StageWorkspace` defect that
the author's own mutations had not. If a second session starts, it reads
`menata-app-document`'s `audits/2026-10-10-pembagian-dua-sesi-roadmap.md` §10.5 first and the lane rules
above apply again from that moment.

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
