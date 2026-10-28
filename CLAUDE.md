# Menata App — agent instructions

> Condensed 2026-10-17. The full narrative behind each rule and gate (what forced it, what was measured, what a
> mutation proved) is `menata-app-document`'s `references/claude-md-full-2026-10-17.md`, a frozen snapshot. Each
> gate's own doc comment in `internal/conformance` (and siblings) is the current, authoritative explanation.

## What this project is

This is **not an ordinary application** — it's a **composable runtime** (README.md: "The runtime application that
turns Menata Runtime Metadata into a running application"). The product is the runtime; any app it renders (Task
Tracker, Document Approval, whatever is in `metadata/workspaces/*.yaml`) is one metadata description it happens to
be interpreting. `007-composable-runtime-architecture.md` is the normative target.

So hand-writing something metadata could express is a **regression** against the product. Ask "can metadata
express this?" before "let me just write the code".

Read `001-design-principles.md` … `007-composable-runtime-architecture.md` before any architectural decision
(a route, a field, a UI element — anything that isn't a pure bugfix). `writing-guide.md` and `capabilities.md`
are the practical/current-state companions.

**Read them as a destination first, and only then as a constraint list.** Reversing that produced three phases
in a row whose outcome was "don't" or "less" (2026-09-30).

| question | the tool for it | where it comes from |
|---|---|---|
| "Should a **new** capability be added?" | admission test (A1–A5), second-case rule, ratchets | `capability-lifecycle.md` §2; 007 §27 (*"Before adding a **new capability**"*) |
| "How do we execute architecture that is **already agreed**?" | sequencing and design; gates **lock** each step afterwards | 001–007 as the normative target |

Most unbuilt work (UI IR, Data IR, the planner, the Experience Plane's primitives — 005 Phase 5, 007 §12/§15/§16)
is the second row. Do not make agreed architecture re-prove itself as a proposal.

- **007 §40's `PROPOSED` is not a veto** — *"A PROPOSED row is not admitted by appearing here"* is an
  anti-overclaim device, not a withheld permission.
- **007 §27 read as a design checklist argues *for* building**: its Q3 (*"Would one new generic primitive unlock
  this and other cases?"*) is what a genuine substrate passes most strongly.
- **The second-case rule governs *semantics*, not *structure*.** It guards against invented meaning (a colour
  role for one Machine), not against adopting vocabulary 007 already closes (`stack`, `row`, `grid` …).
- **Gates preserve what has been achieved; the vision directs the next step.** Order: build the primitive,
  migrate the uses, *then* gate.

Before concluding that something 001–007 directs should not be built yet, read `menata-app-document`'s
`guides/subordinate-mechanisms-must-not-outrank-the-vision.md`. Short form:

1. **Check the mechanism's scope against its own sentence** (§27 is about *new* capabilities; §40 is anti-overclaim).
2. **Look for the imperative word** (`MUST`, `MUST NOT`, *progressively*, *should not require*). A claim that
   contradicts the text is the claim that is wrong.
3. **Ask whether a MUST is in breach right now** (e.g. §12.4: a View MUST NOT be the universal composition
   primitive, while most screens are bespoke Go functions). "No second case yet" is no answer to a live breach.
4. **If the reason to stop is yours, change the reason, not the work.** Ship a smaller, fully verified slice and
   name it. Never dress an operating limit as an architectural finding. Warning sign: *"better not to start than
   to start half"*.

**Receiving a request that aims at a mockup** (a board, a screen, a flow): follow `menata-app-document`'s
`guides/membangun-menuju-mockup.md` first (intake, classify each element a–e, slice vertically, red case from the board),
then the per-item cycle in `guides/pola-pengembangan-per-butir.md`. The target is the mockup; what the runtime or the
AI assistant can do today is a work list, never the bound.

The two principles most often violated by an unreviewed edit:

- **001 #3, Metadata First** — evolution happens by changing Runtime Metadata. If a route, label or nav entry
  exists in a manifest, a page references it, never retypes it.
- **001 #8, Reference over Duplication** — before hardcoding a string that describes application behaviour,
  grep `metadata/` for it.

## One manifest per Workspace

An Application is **installed into** a Workspace. `metadata/workspaces/<slug>.yaml` is one Workspace's
installation (by **slug**; the `ws_...` id is generated). The directory is scanned: a file installs, deleting it
uninstalls; `applications: []` is valid and normal.

- **`metadata/workspaces/` is data, not code, and is git-ignored** except `default` (the worked example). What a gate or test needs from a real Workspace comes from the tracked copies in `metadata/samples/` (`sample-*`), never from a developer's own Workspaces.
- **Creating a Workspace writes its empty manifest** (`installer.EnsureWorkspaceManifest`, without a reload —
  reloading resets `/register`'s rate limiter). A handler writing into a Workspace goes through
  `internal/web.workspaceInstallation`. **Never build a manifest path from `rendering.CurrentWorkspace(ctx).Slug`.**
  Only an `installer.RejectedError` is handed to the AI assistant.
- **Which Workspace a request is in is per-request**: on ctx (`rendering.WithCurrentWorkspace`, set by
  `internal/web.currentWorkspace`); `routeByID`/`labelByID` take `ctx`. No package-level `workspace`.
- **So is which Machine an id means**: resolve from the Workspace on ctx (`machinesFor`/`installedMachines` in
  `internal/web/machine.go`). `web.Deps` holds exactly one Machine, `UserMachine` (`mch_user`), for `/register`,
  `/accept-invite`, `/create-workspace`. Do not add a second.
- **Installing copies.** `metadata/*.yaml` and `metadata/applications/*.yaml` are the read-only **template
  library**; a Workspace's copies live in `metadata/workspaces/<slug>/` (anything generated goes there;
  `aiassist.Write` derives it). Only `mch_user`, `mch_activity`, `mch_notification` stay shared.
- **A name is never an identity the runtime may branch on.** Not a Machine id, not an Application id. Use
  `action.IsDocument`/`action.IsStep`, which read the Application's declared
  `workflow: {engine: document_approval, roles: {...}}` (`domain.Machine.WorkflowEngine`/`WorkflowRole`,
  `registry.KnownWorkflowEngines`). Ask a Machine for its **role**: `internal/web.machineForStep`/
  `machineForDocument`/`approvalMachine(ctx, role)`, `rendering.approvalMachineID(ctx, role)`, all via
  `domain.Workspace.MachineInWorkflowRole` (nil is a real answer for an optional role).
- **What an Action writes is declared** (`actions:` → `domain.ActionEffect`, applied by `action.ApplyEffect`).
  Never `record.Values[action.FieldX] = ...` in a handler. The status move comes from `transitions:`.
- **Where a signature lives is declared** (`signature_placement:`, `signature_store:`; read through
  `action.SignatureFields`/`StoreFields`), and so is a saved approval flow (`flow_template:`,
  `flow_template_step:`; `action.FlowTemplateFields`/`FlowTemplateStepFields`/`FlowTemplateRowFields`).
  **Derive or declare:** derive when the Machine can legitimately answer (a live step's order from `sequencing:`);
  when a derivation returns empty, check whether the Machine *can* answer before forcing it. When no declaration
  answers a question, building the declaration *is* the work.
- **A Workspace's own ids are unique within it** (`metadata.validateMachineIDsAreUnique`; `aiassist.Validate`).
- **Companions**: an Application whose Machines relate to another's declares `companion_of:`; the library lists the named one once and an install brings both in as two Applications (`installer.Listed`, `Plan.Companions`).
- **Switching an Application off** is `inactive: true` in its file (`domain.Application.Inactive`): `currentApplication` 404s everything it owns and `openableApplications` hides it; data stays. A Workspace admin's switch, never part of a proposal.
- **Installing renames colliding Machine/Application ids** (`internal/installer`; no collision ⇒ byte-identical
  copy). A collision on a nav id, nav route or Dataset id is **refused**, not renamed, because Go still names
  those. Installs are load-verified and rolled back on failure.
- **A capability the runtime needs from metadata must be declared**, or the copy model silently diverges: a
  library change never reaches a Workspace that installed earlier. Engines declare their Datasets by role
  (`domain.WorkflowEngineSpec.Datasets`, checked by `metadata.validateWorkflowDatasets`). A broken manifest makes
  only that Workspace unavailable (`metadata.LoadWorkspacesTolerant`, 503, logged `WORKSPACE UNAVAILABLE`); a reload
  that breaks a healthy Workspace is rejected whole. **When you add a metadata key Go names, update the library
  *and* every `metadata/workspaces/<slug>/` copy, and make the loader demand it.**
- **A store method that takes an id takes the Workspace too** — put the Workspace in the `WHERE`
  (`records.workspace_id`, `data.WorkspaceScope`), not only in the caller.

Before adding any `href`, label or button under `internal/rendering/`: check the Application's `navigation:`. If
metadata can't express it yet, say so in a comment — never leave it silently duplicated.

## Where a metadata-derived value belongs

One home: a named, doc-commented field on `domain.Application` (or `domain.Workspace`). Worked example
`HomeRoute`:

1. The Application file declares it (`home_card: true` on a nav item).
2. `internal/metadata.LoadApplication` resolves it at load (before `hidden_nav_groups` filtering, like
   `PrimaryNavGroup`).
3. `cmd/server/main.go` copies it onto `web.Deps`.
4. The handler takes it as an explicit parameter.
5. The `.templ` renders the parameter — never `href="/approval-inbox"`.

`domain.Application`'s fields are the inventory. **Shortcut for a sibling screen:** `rendering.routeByID("nav_xxx")`,
`labelByID`, `titleByID`, `descriptionByID` (they read `AllNavigation`, the pre-filtering list). This applies to an
Application's *own* screens too.

## Deciding whether a literal is a metadata-hardcoding violation

For any literal that looks like application behaviour (route, label, Machine/Field id, option value, badge name):

1. **Declared in `metadata/`?** Reference it through an id-keyed lookup or a `domain.Application`/`Workspace` field.
2. **Metadata can't express it yet?** Allowed only if the site (a) says so in a comment naming the missing
   capability, (b) cites a forward-checkable pointer — a 007 section, a stage of `menata-app-document`'s top-level
   roadmap, or a `development-history.md` phase (never a `ROADMAP.md` phase), and (c) is listed in
   `writing-guide.md`'s "What comes free vs. what's hardcoded today" table.
3. **Needed a second time?** Run it through `menata-app-document`'s decomposition criteria before a third:
   `workflow-behavior-decomposition-criteria.md` (B1–B5) or `ui-composition-decomposition-criteria.md` (Q1–Q5).
4. **Exceptions expire.** At each roadmap stage close, re-check standing exceptions against *current* capability.
   **A deferral is a measurement with an expiry date**: "no primitive yet" and "nothing declares it" are both
   checkable in a minute, and both were wrong more than once here.

## Design reference vs. current code

The mockup canvases are the design target ("a design reference, never current-code
intent"). Match a mockup structurally; don't carry forward extra elements, and fix hardcoded values surfaced by a
port as part of it.

### The design system has three homes

| Question | Where the answer lives |
|---|---|
| What does the runtime render, what can a Workspace override? | **Code**: `internal/domain/theme.go`, readers in `internal/rendering/layout.templ`/`controls.templ`, `registry.Components`, `static/css/input.css` |
| Tokens, type scale, components of the mockups; accepted/rejected proposals | **Design System artifact "Menata App"** `https://claude.ai/artifact/Xi4TYQPxsMQUmL3chScmaS` (`project/README.md`, `project/tokens.json`); repo copy `design-system/tokens.json` (never hand-edited) |
| What should a screen look like? | UI mockups `https://claude.ai/artifact/Wzkc6reCNHBJvtq2DJHsU6`; Application Builder `https://claude.ai/artifact/4tdbksPLQdwogQ19a9rPbq` |
| Why a Theme role was decided that way | `menata-app-document`'s `guides/design-system-decisions.md` |

Governance (rules R1–R9, gates GS1–GS7) is `menata-app-document`'s `guides/menjaga-design-system.md`.

- **Read the Design System artifact's README before proposing a token, a type step, or a mockup component.**
- **Folding, normalising or replacing a value is a decision, not a tidy-up**: test it on the densest surface,
  measure, record it with its reversal condition.
- **R9: test a Theme-reader migration under a moved theme** — the default theme cannot show a collision.
- **Change a token in the artifact, then regenerate the canvas `:root` blocks** (`design-system/canvas.py
  generate`; `check` to verify). Never edit one artboard by hand. Artboards reference tokens (`var(--accent)`).
- **All three homes move together (R8)**: artifact, `design-system/tokens.json` (GS1 holds it to the code), canvases,
  and `make tokens` for the DTCG export.
- Status lists: design-system work in `audits/2026-10-11-rencana-design-system.md` §"STATUS MENYELURUH"; the
  **top-level roadmap** is `audits/2026-10-15-peta-jalan-menuju-aplikasi-lengkap.md` (owner order: docs → AI
  assistant and `page:` → domain → mockups → process map last). **A new metadata capability lands with its
  AI-assistant slot in the same slice** (or a `registry.Capabilities` entry saying why it has none). Do not keep a
  second status list here.

## The component inventory: `capabilities.md`

Before adding a Machine concept, Field type, route or reusable rendering piece, check the matching table in
`capabilities.md`. Add the row in the same change — `internal/conformance` fails when the Machines and Shared
rendering components tables drift. The latter's **Kind** and **YAML** columns are kept by review, not by a test.

## Enforcement, not just prose

`internal/conformance` (and gate tests in `internal/web`, `rendering`, `domain`, `ir`, `registry`) turns 001–007
and `capabilities.md` into failing tests. Run one with `go test ./internal/conformance/... -run <Name>`. **The
test's own doc comment is the authoritative explanation; read it before changing a gate or its map.**

### How gates work here — rules that hold for every family

- **Ratchets are shrink-only both ways.** A higher count fails, a new file fails, and a *lower* count fails until
  the entry is lowered. An emptied ratchet stays declared (it is an ordinary gate now). **Read numbers out of the
  map, never out of prose** — this file deliberately states none.
- **Build the primitive, migrate the uses, then gate.** A gate installed first tests failure classes that do not
  exist yet. A primitive arrives with the uses it replaces, or it is a name with nothing behind it.
- **No gate counts until mutation-proved** — and verify the mutation applied (`grep -c`) before trusting a green
  run; `gofmt` realignment and duplicate patterns have produced no-op mutations. Renderer tests read
  `*_templ.go`, so mutate the generated file or `make generate` first.
- **Counting what is present may be syntactic; concluding something is absent may not.** Class-string counts
  wrongly rejected `row`/`grid` and missed `columns`/`section`; over-claiming a case is the same failure. State
  the measurement method beside any "zero cases" claim.
- **"Two cases" is a claim about sameness** — check what each site would actually need, not a shared symptom.
- **L13: moving a literal into a reader (or a Go string) can drop a shape gate's count without migrating
  anything.** Shape gates read readers as their default literals (`joinClassExpressions`,
  `radiusReadersAsLiterals`, `inkReadersAsLiterals`, `colourReadersAsLiterals`, `typeReadersAsLiterals`); check
  every fall.
- **Never put two Theme readers as `templ.KV` keys in one class expression** — under a remapped theme they collide
  and templ drops a class. Choose the class in Go.
- **Tailwind scans `.templ` as text: a comment naming a utility class emits it** (`TestNoClassLivesOnlyInAComment`).
- **A byte-diff no longer proves a colour migration**: compare computed styles (Chromium), and check that a diff
  can actually reach the code before reporting it as verification.
- **Watch for a gate whose own text sits in the data it reads.**
- A slice that adds a gate or primitive gets a **fresh-context reviewer agent whose job is to break it**.

### Gate index (by family)

- **Plane & handler shape**: `TestPlaneBoundaries`, `TestEveryPackageHasARule`, `TestHandlersStaySmall`.
- **Manifests & routes**: `TestAppManifestLoads`; `TestNavigationRoutesAreRegistered` (every Workspace; a
  `/machines/<id>` route must name a Machine that Workspace installs);
  `TestNoRouteBreaksInAWorkspaceThatDidNotInstallItsApplication`, `TestApplicationOwnedRoutesComeFromNavigation`,
  `TestRequireInstalledApplication`.
- **No hardcoded metadata in rendering/handlers**: `TestRenderingHasNoHardcodedApplicationRoute`/`...Label`/
  `...PageHeading`, `TestHandlersHaveNoHardcodedApplicationRoute`/`...Label`;
  `TestRenderingUsesProjectionNotRawValues` (a `.templ` may not read `Values["fld_..."]`; use `card_fields`).
- **Identity**: `TestNoBareMachineIDIdentityChecks`, `TestNoApplicationIDIdentityChecks`,
  `TestActionDoesNotSwitchOnItsOwnMachineIDs`, `TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames`,
  `TestUnboundMachinesAreNotTheEngines`; coupling ratchets `TestDocumentApprovalCouplingOnlyShrinks`,
  `TestDocumentApprovalFieldCouplingOnlyShrinks` (both empty).
- **Workspace isolation & installs**: `TestInstalledApplicationsAreCopiesNotSharedFiles`,
  `TestTwoWorkspacesCanHoldDifferentMachinesUnderOneID`, `TestCheckDocsMirrorMetadatasOwnKeys` (installer's
  strict-decode mirrors must carry every loader key — **check it whenever you add a key**).
- **Engine Datasets**: `TestEveryCastRoleProvidesItsEngineDatasets`, `TestEveryEngineDatasetIsNamedByComposition`,
  `TestGoNamedDatasetsWithNoEngineRequirement` (closed for the engine's Datasets, open for Case 19's).
- **Inference is inspectable (001 #6)**: `TestInstalledCastsExplainWithoutDefects` (defects are `undeclared` and
  `input unavailable`; `not applicable` is not one), `TestEveryDerivationIsOwedBySomeRole`,
  `TestEveryDerivationAccessorIsExplainedOrExcused` — a new derivation needs its `Derivation*` constant, its role in
  `Answers`, and its branch in `explain.go`. Navigation headings: `TestInstalledNavigationExplainsItsHeadings`,
  `TestRuntimeScreensResolveTheirHeadings`, `TestRendererDoesNotResolveNavigationHeadings`
  (`NavigationItem.Heading`, resolved at load).
- **Fixtures**: `TestMachineFixturesPassProductionValidation` (Normalize first), `TestFixturesMirrorTheRealMachines`
  (add a new rule block to the mirror in the same change).
- **Registries & dispatch**: `TestClosedRegistryMembersAreAcceptedByTheLoader`/`...AreActivatedByMetadata`;
  `TestServiceRegistryAndExecutorsAgree`, `TestRegistryDependsOnDomainOnly`,
  `TestDomainHoldsVocabularyAndRegistryHoldsDispatch` (bool-valued set = vocabulary in `domain`; structured =
  dispatch seam in `registry`, which imports only `domain`).
- **Reads & writes**: `TestGetRoutesDoNotWrite`; `TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed` (cannot see an
  N+1 — use the flat-cost shape of `TestSwitchWorkspaceCostIsFlatInWorkspaceCount`);
  `TestAuthenticatedPageQueryCost`/`TestNavBadgeQueryCost`; `TestPostRoutesRefuseUnauthenticatedAndUnCSRFed`;
  `TestPerRecordGetRoutes` (resolve a role-gated Machine with `MachineInWorkflowRole`);
  `TestWholeMachineReadsOnlyShrink`; `TestPerViewerDatasetsScopeByIdentity` (`viewer_scoped: true` needs a top-level
  `equals $current_user`; it is about what a list shows, not authorization).
  `select: records` needs `limit:`; `where:` values are literals or `$current_user`, `$today[±n]`,
  `$parameters.<name>`, `$done`.
- **Diagnostics**: `TestQueryDiagnosticsRunsBeforeAuth`, `TestPoolInstallsQueryTracer`,
  `TestDiagnosticLineSurvivesAPanicAndMarksIt`, `TestDiagnosticLineCarriesMethodStatusAndRoutePattern`,
  `TestAnomalyPrefixMarksFailedRequests`, `TestSlowStatementIsLoggedByTextAndNeverByArguments`. Log scripts pick
  fields **by name** (`guides/log-review.md`).
- **Case corpus & capability catalogue**: `TestCaseCorpusOnlyAdvances`, `TestAIGrammarGapIsMeasured`
  (`internal/web/testdata/cases/<case>/`; `registry.Capabilities` one row per loader key with `AIWritable` or a
  reason). **New capability pattern: write the case red first, build until it passes, then put real metadata in a
  Workspace.** AI prompt: `TestPromptNamesEveryRegisteredCapability`,
  `aiassist.TestComposableSurfaceRendersEveryFieldType` (naming ≠ truth; read `aiassist.GeneratedEvent`'s shape).
- **`page:` block** (007 §12.4/§15, `writing-guide.md` §12.1a; validated by `metadata.validatePages`, lowered by
  `composition.DeclaredPage`, drawn by `web.showDeclaredPage` → `rendering.DeclaredScreen`):
  `TestBespokeScreensOnlyShrink` (directive ratchet over `templ …Page(`), `TestPageBlockDeferralStaysTrueAndLandsDocumented`,
  `TestEveryInstalledPageLowersAndValidates`, `TestDeclaredPagePipelineHasNoPerScreenBranch`,
  `TestInstalledPageDatasetsExistInTheLibraryTemplate`, `TestEveryInstalledLinkNamesItsDestinationOnce`
  (`static: link` takes `to:` / `list_of:` / `from: {href: record}`, never a typed `href`),
  `TestNoPageClaimsCompletenessOverAWindow`, `TestEveryInstalledFormNamesNoRouteOrField`,
  `TestHandWrittenCreateFormsOnlyShrink`, `TestEveryInstalledButtonIsADeclaredRecordWrite` (delete/move/transition).
  A list screen's vocabulary (2026-10-17): Table `columns: [{field: fld_x}]`, `where:` `contains` + `optional: true`,
  `FilterChips`/`FilterSelect` (dimension-bound), `ActionBar` (lifted into the header), link `variant:`, nav `in_menu: false`,
  **view groups** (Application `view_groups:`, nav `view_group:`/`summary: counts`; a Board/Table/Calendar of one Machine share a declared header and a scope parameter that every Dataset of the group must carry as an `optional: true` comparison),
  a `when: {workspace: applications, is: none}` (Home's empty-Workspace welcome), a calendar's `$month`/`?range=`/`fill: weeks`/`moves: true`, a link's `set:`/`icon:`/`variant: quiet`/`variant: plain`, a panel's `density: compact`, a row's `wrap: nowrap`, a Machine's `plural:`, a view group scope's `new:` (a "New project" form in the Switch menu), 
  a board View's `column_actions: [rename, move, delete]` and `add_column: {to, label, note}` (a relation-grouped board's column menu and "add list" tile), `Popover` `trigger: quiet|pill`, a Dataset `sort:` key's `by:` (the related record's column), a record-template paragraph's `tone:`,
  a Table's children = its row actions. An ordered, divided Collection takes `reorder: true` (2026-10-27: a grip per row, `?to=` on the generic move route) and a Popover `trigger: link`. a navigation item's `badge:` may name a Dataset of its Application (`badge: ds_x`, 2026-10-28: the menu shows the viewer's count, `/api/nav-badge/<nav>`), takes `visible_to: [role]` (2026-10-28: the menu and the route gate read it alike, `web.navItemVisible`) and `parent:` (2026-10-28, one level: drawn under its parent, "Parent › Child" in the More sheet; the loader refuses a second level or a cycle). `IconTile` (2026-10-22) is an Application's face (`icon`/`color`/`size`/`state`, or `from: {icon: icon, color: tint}`); `appIconClasses` is read only by it (`TestNoHandWrittenIconTile`). A `layout: row` takes `tracks:` (one of fill/fit/field/badge per child), `mark: chevron`, and a link `variant: row` makes the row one target (`TestNoHandWrittenRowChevron`). `component: Tree` (2026-10-27) lists a records Dataset under the folders its `title` names, `query:` the parameter that selects a row, `title:leaf` the row's last segment (Workspace files); `component: Frame` (`src`, `query`) draws a page of this host inside a page, and `?builder=1` (admin only) makes that page name its parts with `data-node-path` (the builder's preview). A Collection takes `inline: true`, a link `variant: chip` (the pill `chipClasses` draws for FilterChips too) and, with no variant, `icon:` before its words; a divided Collection takes `density: roomy`, `tracks:` also works on `layout: columns`, `static: divider` is a rule with no text (`TestNoHandWrittenDividerRule`); `IconTile` takes `state: draft|assistant` (the generated-draft indigo) and `variant: row` links read the `item` text role. Edit/delete rules: `set_once`, `locked_when`, `editable_by` (roles that may change a Field; judged against who is asking, `Machine.LockedFieldFor`), `on_parent_delete: restrict`,
  `deletable_unless` -- a button reads the same rule the route enforces (`composition.heldRecords`).
  One record's page: nav `record_of:` (`/pages/<nav>/<id>`, `$parameters.id`, `where: {field: id}`), `field:`/`tone:`/
  `steps:` roles, `Stepper`, `binding: {transition: trn_x}` on a Button or Form, link `pass:`, Form `values:`/`then: record`.
  `compute:` is one closed expression (`domain.ComputeExpr`; `parent:`, `sum_children`/`count_children`): a child's write
  recomputes its parents in the same transaction (`execution.RecomputeParents`), and every load recomputes stored records.
  A record page's own values are `$parameters.record.<field>` (runtime-owned names); `related: rel_x.fld_y` shows a Field of the
  first related record; a Button `write: print` prints the page (`@media print` leaves the chrome and the buttons off the paper); a create Form's `fields:` picks (and orders) the controls it asks for, its `extract:` fills it from a document an AI reads (`composition.ExtractProposal`, `/pages/<nav>/extract` then `/extract/save`, one transaction, nothing written until a person saves); `write: pdf` downloads it as a PDF (`/pages/<nav>[/<id>]/pdf`, `web.showDeclaredPagePDF` draws the page again and `pdf.Printer` prints it; no browser, no button).
  Money is written everywhere in the Workspace's `number_format:` (`international` 1,234.50 or `indonesian` 1.234,50), always two decimals (`domain.FormatMoney`; read from ctx with `composition.NumberFormatOf`/`rendering.CurrentWorkspace(ctx).Number`); never format money in a handler or a `.templ` by hand. **Script is the exception** (`writing-guide.md` "Script is the exception"): the shell's money control (`data-money`, forms only) is the one that formats input; display is always the server's, and a new script exception is added to that list in the same change. A Popover with `density: regular` is a **dialog** (Cancel, a close mark and Escape close it, never a tap outside; its Form's Cancel and submit are pinned in sight), and a transition nothing leads back from takes `confirm:` and is named for what it does (**Cancel contract**, never a bare **Cancel**). A numbered Field (`numbering: {pattern, codes}`, `execution.AssignNumbers`) is a number the runtime writes on create, under a
  lock, per prefix, from a counter that only goes up (a deleted record's number is not reused); never typed, never edited. An inherited Field is `compute: {parent: fld_via.fld_x}` alone (the parent's value, any type; followed down by
  `execution.recomputeChildren`): a child's list shows, searches and filters by what its parent holds.
  Table (2026-10-18): `rows: dimension` (`row:`/`measure:`), `total: sum|count` (added up from the rows shown), `of:`, `related:`; money `currency:`;
  `date:relative`; FilterChips `rows: total` + `counts:` over `if_param:` views.
  Board Settings is the first migrated screen. Never claim "screen X is declarative" without checking it is a `page:`.
- **UI IR & placeholders**: `ir.TestValidateRejectsAllFiveMandatoryFaults`, `TestValidateAcceptsTheHeaderTree`,
  `TestValidateRefusesUnknownTypes`; `TestDeclaredPlaceholdersStayDeclared`, `TestPlaceholderDocsClaimNoContent`
  (`internal/planner` is a placeholder; a placeholder's doc may not claim content). Re-measure before proposing
  `ir.Machine` (see `ir/doc.go`).
- **Layout primitives (007 §12.2)**: `TestLayoutRenderersHaveNoPerScreenBranch`, `TestLayoutVocabularyIsRenderedAndUsed`,
  `TestLayoutClassesStayInRendering`, `TestHandWrittenLayoutSitesOnlyShrink` (directive), `TestNoHandWrittenDividedRow`.
  Measure what templ emits before writing call sites (`class={a, b}` emits a double space for an empty value).
- **Components (007 §12.3/§13/§14)**: `TestComponentRegistryAndRenderersAgree`, `TestRegisteredComponentsStayBounded`
  (reads the renderer's signature), `TestRenderingDoesNotReadTheClock` (`time.Time` as a parameter is the fix),
  `TestPermittedPropsAreTheContractsInputs`, `TestComponentStatusMatchesInstalledUse` (`stable` = an installed `page:`
  depends on it), `TestEveryRegisteredComponentIsInCapabilities`, `TestWritingGuideShowsEachComponentsStatus`.
  Per-Component zero-floor gates: `TestNoHandWrittenTag`, `...Callout`, `...EmptyState`, `...Disclosure`,
  `...SegmentedControl`, `...Popover`; `TestEveryDialogReadsTheShell`, `TestEveryNativeCheckableReadsTheAccent`.
  Buttons: `TestHandWrittenButtonsOnlyShrink`, `TestHandWrittenRawButtonsOnlyShrink`, `TestEveryButtonNamesItsType`,
  `TestQuietDestructiveActionsShareOneLook`. A Component must not accept arbitrary properties (`hx-*`, `class`,
  `popovertarget`…); that is §12.3 `actions`, mostly unbuilt. **Ask what a contract forces that a reviewer would not
  have asked for** (e.g. Avatar's required `label`).
- **Theme (006; nine categories, declarable per Workspace)**: `TestEveryThemeRoleHasAReaderOrAReason`,
  `TestEveryThemeCategoryHasAnImplRatchet`, `TestEveryUnnamedDesignValueHasAReason`,
  `TestDesignBucketAndImplRatchetsSplitTheTokens`. **A new metadata key is not done until `writing-guide.md`,
  `capabilities.md`, 007 §40 and this file mention it** — `writing-guide.md` is the one an author reads.
- **Hand-written design-value ratchets** (each with a reason per floor in its map): chips, muted paragraphs,
  captions, fields, dividers, surfaces, tiles, theme tokens, off-ladder sizes, control borders, tone pairs,
  tone edges, radii, ladder gaps, region gaps, half-step gaps, compact heights, accent inks, hover washes, well
  fills, surface edges, variant tokens, leading, tracking, control heights, colours (`TestHandWrittenColoursHaveAReason`),
  faint ink (`TestFaintInkSitesHaveAReason`). Zero floors: `TestNoHandWrittenRegionRule`, `...Subheading`,
  `...HeadingOrEyebrow`, `...MarkGood`, `...PanelPadding`, `TestNoTwoKVKeysFromOneThemeReader`,
  `TestShadowsAreOnlyTheArtifactsFour`, `TestSVGColoursAreNotHexLiterals`, `TestEveryIconControlHasATouchTarget`.
- **Colour roles, dark mode, palette**: `TestColourRoleClassesComeOnlyFromTheirReaders`,
  `TestNoRolePropertyRedefinedOutsideItsReader`, `TestThemeCSS*`; dark: `TestEveryColourPropertyHasADarkValue`,
  `TestDarkColourPairsMeetContrast`, `TestThemeCSSCarriesOnlyHueChoicesIntoDark`, `TestNoSiteWritesADarkVariant`,
  `TestPaperLayerReadsNoThemeRole`; closed palette: `TestClosedPaletteDeclaresExactlyWhatIsNamed`,
  `TestEveryClassTokenIsCompiled`, `TestEveryRenderedClassIsCompiled` (needs `DATABASE_URL`). Contrast:
  `TestThemeColourPairsMeetContrast` (GS3), `TestLightTagHuesMeetContrast`, `TestTextOnSunkenMeetsContrast` (no
  `secondary` text on `sunken`). Dark mode lives in `input.css`; no site writes `dark:`.
- **Keyboard focus**: `TestKeyboardFocusRuleCoversInteractiveElements`, `TestNoOutlineNoneBesideAFocusRing`
  (fields write `focus:outline-solid`).
- **Cross-source design gates**: `TestDesignSystemTokensMatchTheCode` (GS1), `TestRejectedDesignValuesStayOut`
  (GS2), `TestDefaultThemeMatchesItsRecordedSnapshot` (GS4; change only with a recorded decision and
  `UPDATE_DEFAULT_THEME=1`), `TestDocumentedDesignSystemCountsMatchTheCode` (GS5, reads this file too),
  `TestDesignSimilarityReport` (GS7, report only, `-v`), `TestCustomTextSizesDeclareTheirLineHeight`,
  DTCG export (`TestDesignTokensValidateAgainstTheDTCGSchemas`, `TestCommittedDesignTokensAreFresh`,
  `TestDesignTokensMatchTheDesignSystemArtifact`, `TestDesignTokensFollowAMovedTheme`).
- **Docs**: `TestCapabilitiesMachinesTableMatchesMetadata`, `...ComponentsTableMatchesTempl`,
  `TestClaimMatrixCitesRealArtifacts`, `TestConceptDocsCiteDocumentsThatExist`, `TestRoadmapStaysAReleasePlan`.

If you re-explain the same rule twice in commits, consider whether it should be a conformance test.

## Where a write-up goes: this repo vs. the companion doc repo

| file | holds | shape |
|---|---|---|
| `menata-app/ROADMAP.md` | shipped / in progress / planned | **short, feature-level, public** |
| `menata-app/capabilities.md` | what the runtime can do now | current-state tables |
| `menata-app-document/development-history.md` | phases, forcing conditions, rationale, verification | append-only, private |
| `menata-app-document/audits/` | one-off dated findings | review records |
| `menata-app-document/guides/` | methodology reused ≥2× | standing practice |

A finding's narrative (what forced it, what you measured, what a mutation proved) goes to `development-history.md`.
`ROADMAP.md` gets a plain feature-level line: no commit hashes, gate names, decision codes or private paths. A
paragraph containing a commit hash, "measured", "mutation-proved" or "probe" belongs in the other repo.
`TestRoadmapStaysAReleasePlan` holds volume, not fit. **The same applies to this file**: add a rule here, put its
story in `development-history.md`.

## Working with other agent sessions

Handoff is a dated work list in `menata-app-document/audits/` (each item: where, why, what done means, what blocks
it). **Re-measure every number you inherit** — that is the protocol, not distrust.

- **Never `git add -A`**; use **`git commit -- <paths>`** (the index may hold another session's staged work).
- **Never `git checkout` to restore during mutation testing** — copy to the scratchpad instead. After cp-restore
  mutations, run `go test -race -a <pkg>` before pushing (stale race objects).
- An untracked file that is not yours: don't commit it; ask.
- **`pre-push` runs the whole suite**: if another session's test is red, wait — never `--no-verify`. A duplicate-key
  Postgres failure may be a concurrent-run artifact; re-run it alone.
- **Lanes** (only when two sessions are active; `audits/2026-10-10-pembagian-dua-sesi-roadmap.md` §10.5): lane A
  owns runtime packages, lane B owns `internal/rendering`, `ir`, `composition`, screen handlers and Application
  files; **only lane B edits a `.templ`/runs `make generate`**. `ROADMAP.md`/`roadmap_test.go` change only at a
  slice's end after `git pull --rebase`.
- **Since 2026-10-11 development runs in one session**: lane rules are a fallback. Gates, mutation proofs,
  re-measurement and `git commit -- <paths>` stay. **Deploy waits for the owner's permission.**

## Commands

- `make generate` — regenerate `*_templ.go` and `static/css/app.css` (needed after any `.templ` edit).
- `make build` — generate + `go build ./cmd/server`.
- `make test` — `go test -race ./...` (the pre-push hook runs this).
- `make tokens` — regenerate the DTCG export in `design-system/dtcg/default/`.
- `go test ./...` with `DATABASE_URL` set runs the Postgres-backed tests (they clean up via `t.Cleanup`; safe on the
  shared dev database).

## Working directory is shared

Other sessions may use this checkout concurrently: check `git status`/`git log` before a long refactor.

The dev server is the systemd unit `menata-app.service` (`bin/server`, port 4000, log
`/var/log/menata-app/app.log`). After `make build`, `systemctl restart menata-app` — never `kill`/`nohup` it by
hand. Check `systemctl status menata-app` first. Every app on this host runs under its own unit behind
`caddy.service`.
