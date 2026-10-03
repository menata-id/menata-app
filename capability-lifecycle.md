# Capability Lifecycle Governance

> **How a new capability earns its way into this runtime**: how to test whether it deserves admission,
> what makes it whole, and how the runtime grows to absorb it without bloating the core.
>
> Ported into `menata-app` 2026-10-02 from `menata-runtime`'s own `capability-lifecycle.md` (Study 9,
> 2026-07-04). **Adapted, not copied** — every reference is remapped to an artifact that exists in this
> repository, and where the original named a mechanism this runtime does not have, that is stated rather
> than quietly carried. It is cited by `001`, `002`, `003` and `007` (four citations from §7 alone), which
> is why it had to live here: a normative document citing a file nobody in this repo can open is a pointer
> that does not resolve.
>
> `007-composable-runtime-architecture.md` §14 defers to this document's §4 for the registry-seam
> discipline, and §40 defers to §2 for its PROPOSED/PROVEN admission language.

---

# 1. Lifecycle states

```text
PROPOSED ──admission test──► ADMITTED ──implementation──► INCUBATING ──conformance──► SUPPORTED
    │                            │                             │                          │
    ▼                            ▼                             ▼                          ▼
 REJECTED                  (recorded in                 (built, gates not             (ratchet rule:
 (reason recorded)          ROADMAP.md's                 yet all green)                may never regress;
                            Planned section)                                            deprecate only)
```

- **Proposed** — named by a case or a concept document, not yet admitted.
- **Admitted** — passed §2's admission test; carried in `ROADMAP.md`'s `## Planned`.
- **Incubating** — implemented, with at least one gate still owed. See the note below.
- **Supported** — in `capabilities.md` as Built, with a conformance test naming it. The ratchet rule then
  applies: a count may fall but never rise, and a row is deprecated rather than deleted.
- **Deprecated** — replaced or withdrawn.

**Two states this runtime does not implement the way the original describes, stated rather than carried:**

- **Incubating has no feature flag.** The original activates an incubating capability per workspace so
  schema iteration cannot break stable tenants. Measured 2026-10-02: this repo has **no feature-flag
  mechanism at all**. What it has instead is Workspace isolation — an Application is *installed into* a
  Workspace and copied, so one Workspace can carry a changing Application while others do not
  (`CLAUDE.md`, "One manifest per Workspace"). That is a narrower guarantee: it isolates *which
  Workspaces have the capability*, not *which schema version they interpret*.
- **Deprecated has no cycle.** The original requires flag → warn → sunset. Measured: this repo has no
  deprecation mechanism and no metadata `version:` key, so "old metadata must keep loading" is today a
  review obligation, not an enforced one. See §4's rules 1 and 2.

---

# 2. Admission test — is it worthy?

All five must hold. Any failure leaves the capability Proposed, with the failing criterion recorded.

| # | Criterion | Test, as it applies here |
|---|---|---|
| A1 | **Dual evidence** | Named by ≥2 *independent* sources. The original asks for one case plus one benchmark; **this repo has no benchmark suite**, so the second source is `menata-app-document`'s `case-portfolio.md` (a different case), an audit in `audits/`, or a measured count in this tree. A single enthusiastic source is a hypothesis. **A measured count is evidence *for* presence and never *against* it** — see the asymmetry note below, which exists because this criterion produced four wrong rejections in one session. |
| A2 | **Universality or declared verticality** | Either most platforms have it, or it is explicitly scoped. Never "we might need it someday". |
| A3 | **Single responsibility** | Maps to exactly one plane (Domain / Data / Experience, 004) or one declared cross-cutting area. If it needs two, it is two capabilities. |
| A4 | **Non-composability** | Cannot be built by composing existing supported capabilities. This is the criterion 007 §27's Q1–Q3 ask in more detail, and the one most often answered too fast. |
| A5 | **Business language exists** | A domain expert can say it. If only an engineer can phrase it, it belongs to the runtime's internals, not to metadata. |

**What this test does *not* govern, and getting that wrong cost three phases of progress** (2026-09-30,
recorded in `menata-app-document`'s `audits/2026-09-30-kajian-arah-pengembangan-dan-protokol-kolaborasi-agen.md`):
it governs **new capabilities**. It does not govern executing architecture `001`–`007` already agreed.
007 §27 says so in its own first line — *"Before adding a **new** capability"* — and `007 §40`'s
`PROPOSED` is an anti-overclaim device (*"A PROPOSED row is not admitted by appearing here"*), not
permission withheld. UI IR, Data IR and the Composable Execution Planner were named here as agreed
architecture awaiting sequencing, not proposals awaiting evidence. **UI IR shipped on 2026-10-03**
(`ir.UINode`, §15.3's five rejections, two consumers) and the record of what delayed it is instructive: it was
held back one commit by my own remaining context dressed as the architectural claim "Stage 3 cannot be split
honestly", which one word in 007 §24 refutes (*"progressively* lower them to generic primitives"). Data IR and
the planner remain correctly deferred — §34 marks the planner PROPOSED and says naming a boundary admits no
capability, so building its input is still the wrong order.

**The asymmetry A1 depends on, added 2026-10-03 after it failed four times.** A grep over this tree is sound
evidence that something *is* used and worthless as evidence that it is *not*. The absence of a class string is
not the absence of a meaning, so a count may admit a capability and may never reject one. Four rejections were
made this way and all four were wrong: `row` and `grid` ("not primitives in this corpus" — class strings
counted as shapes); `columns` ("zero measured uses" — nine, three inside Document Approval); `section`
("overlaps `panel`" — 34 sites, the largest population in the corpus); `tabs` ("zero" — three tabs running in
production). Each was disproved by *reading a screen*, which a grep structurally cannot do.

So before A1 is used to leave something Proposed:

1. open `case-portfolio.md` — 21 written use cases, 105 lines, and the source I never opened while rejecting;
2. decompose one **running** application and read its screens, not its class attributes;
3. check whether established framework vocabulary (Bootstrap, Material, Vue/Svelte slots, CSS Grid) has a name
   for it — if it does, this corpus probably has the shape under different classes;
4. if you still conclude absence, **state the measurement method beside the claim** so the next reader judges
   the method rather than trusting the number.

The full failure record, including the three further instances where a subordinate mechanism was allowed to
outrank `001`–`007`, is `menata-app-document`'s
`guides/subordinate-mechanisms-must-not-outrank-the-vision.md`.

---

# 3. Definition of done — is it whole?

A capability is a **column through every layer**, not a feature in one. Each layer is implemented or
*explicitly deferred with a reason* — silence is not a decision.

| Layer | Deliverable in this repo | Worked example: declared Relation (007 §7.5, shipped 2026-09-29) |
|---|---|---|
| 1. Concept | The normative docs say what it is | 007 §7.5, 004's Data Plane |
| 2. Metadata shape | A declarable block, and a doc comment saying where its value comes from | `relations:` on a Dataset |
| 3. Loader | `internal/metadata` parses and validates it; a dangling reference is a **load-time error** | `validateDatasetRelations`, `validateRelationTargets` |
| 4. Domain model | `internal/domain` carries it as a named, doc-commented field | `domain.Relation`, `Dataset.Relations` |
| 5. Engine | `internal/action` / `behavior` / `execution` / `data` act on it | `data.ListRecordsByAny`, `composition.SelectRelated` |
| 6. Composition | `internal/composition` resolves the shape a Page renders | `Selection.Related` |
| 7. Experience | `internal/rendering` renders a shape already decided | the approval screens' own cards |
| 8. Conformance | A test in `internal/conformance` (or the owning package), negative case included | `TestEveryCastRoleProvidesItsEngineDatasets` |
| 9. Record | A row in `capabilities.md`, and `ROADMAP.md`'s `## Shipped` if it is user-visible | both |

Layers 6 and 7 are this repo's own split of the original's single "UI" layer, because
`internal/composition` resolving a shape and `internal/rendering` rendering it is a boundary
`TestRenderingUsesProjectionNotRawValues` enforces (007 §4.4, §7.6).

---

# 3b. NFR gates — is it safe, fast, and sound?

Layers 1–9 prove a capability *works*. Three further gates prove it is safe, fast and architecturally
sound. **The original evaluates these against an `nfr-standards.md` that does not exist here**, so each is
remapped to the mechanism this repo actually has — and where it has none, that is the finding.

| Gate | What stands in for the standard here | Evidence required |
|---|---|---|
| **Security** | No threat-profile document. What exists: `menata-app-document`'s `audits/2026-09-19-security-audit.md`, and the structural sweeps — `TestPostRoutesRefuseUnauthenticatedAndUnCSRFed` (every POST refuses no-session and 403s no-CSRF), `TestGetRoutesDoNotWrite`, and `data.WorkspaceScope` in every statement | A negative conformance test (the forbidden action is refused), or an explicit waiver with its reason in `capabilities.md`. **A sweep is middleware wiring, not behaviour coverage** — that distinction is in `TestPostRoutesRefuseUnauthenticatedAndUnCSRFed`'s own comment and must not be blurred in a waiver |
| **Performance** | No P1–P5 budget classes and no load-test matrix. What exists: per-route query budgets (`web.TestAuthenticatedPageQueryCost`, `TestNavBadgeQueryCost`), the two sweep invariants (`queries == reads`, `repeated == 0`), `make threshold` for volume, and 007 §33's fan-out questions | Either a measurement, or a declared budget with the measurement deferred *and its reason*. **Neither sweep invariant can see an N+1** — their own comment says so; a per-row loop repeats nothing when there is one row. `TestSwitchWorkspaceCostIsFlatInWorkspaceCount` is the shape that catches those |
| **Architecture** | `TestPlaneBoundaries` + `TestEveryPackageHasARule`, and §4's seam discipline below | The capability lives behind its seam, and the gate is mutation-proved. **A gate not yet shown to bite is not a gate** — this repo has found four whose own text sat inside the data they read, each passing when it should have failed |

A capability may be **Incubating** without passing these; it may not reach **Supported** until each is
satisfied or explicitly waived. Silence is not a decision.

---

# 4. Extension architecture — how the runtime grows

**Small core, registries at every seam.** Each engine exposes a resolution point; capabilities plug in
rather than patch the core. 007 §14 defers to this section, and quotes it back as "a compile-time registry
seam for field/action/view types".

**Read the original's own status note before trusting the table below**, because it is the most useful
sentence in the document: at ~90 capabilities, `menata-runtime` still dispatched field/action/view types
through ordinary `switch` statements, and *"the predicted migration triggers fired without the migration
happening."* A seam named and not built is worse than a switch, because the name suggests otherwise.

| Seam | State in this repo (measured 2026-10-02) |
|---|---|
| **Services** | **Built.** `internal/registry.Services` maps each name to its contract and validator; `internal/execution` holds the executors; `TestServiceRegistryAndExecutorsAgree` binds the two. Replaced one validating `switch` and three dispatching ones |
| **Workflow engines** | **Built.** `registry.KnownWorkflowEngines` with each role's cast and the derivations it owes |
| Field types | **A `bool`-valued closed set plus three plane-local switches**, and 007 §14 permits exactly that ("a Go map **or compiler-checked switch**"), objecting only to switches "scattered across handlers". Measured: the three are in `metadata/parse.go`, `data/validate.go`, `rendering/controls.templ` — none is a handler, each has a `default`. `domain.KnownFieldTypes` carries what a type *is* (`FieldTypeSpec`), which is a Domain fact, not dispatch |
| Actions | `domain.KnownActions`, still the two-list shape a drift gate guards (`TestClosedRegistryMembersAreAcceptedByTheLoader`) |
| View types | `domain.KnownViewKinds`, four members, renderer-side dispatch |
| Constraint operators | `internal/expression`'s closed `equals`/`not_equals`, no registry |
| Event sources | three trigger shapes on `domain.Event` (field-change, create, schedule), no registry |

**The rule that decides which side a closed set belongs on**, enforced by
`TestDomainHoldsVocabularyAndRegistryHoldsDispatch`: a set whose value only says *which strings are legal*
is vocabulary and belongs in `internal/domain`; one read to decide *what to run* is a dispatch seam and
belongs in `internal/registry`. `internal/registry` may import `internal/domain` and nothing else, because
`internal/metadata` (forbidden from `internal/data`) and `internal/execution` (forbidden from
`internal/metadata`) must both import it — which is why a `Service` carries its validator and never its
executor.

## Four rules that keep this honest — and three of them this repo does not yet have

Measured 2026-10-02, stated plainly because a ported rule that does not apply is worse than an absent one:

1. **Versioned metadata schema.** ❌ **Absent.** No metadata file declares `version:` and the loader
   applies no per-version interpretation. Every manifest is interpreted by whatever binary reads it.
2. **Backward compatibility.** ❌ **Absent as a mechanism.** No deprecation cycle exists (flag → warn →
   sunset), so "old metadata MUST keep loading" is a review obligation today. What partly substitutes:
   `internal/installer` load-verifies a write through the real loader and rolls back, and
   `TestCheckDocsMirrorMetadatasOwnKeys` stops a write rejecting a file the loader accepts.
3. **Unknown = explicit.** ✅ **Present.** An unrecognized type, action, service or engine is a load-time
   error, never a silent skip — `metadata/validate.go` reads the closed registries directly, and
   `registry.ValidateService` reports an unknown service by name.
4. **Incubation flags.** ❌ **Absent.** See §1: Workspace isolation substitutes partially and guarantees
   something narrower.

Rules 1, 2 and 4 are therefore **named gaps, not inherited guarantees.** Anything that claims this runtime
has long-term metadata compatibility is claiming more than the code does.

---

# 5. Proposal template

```markdown
## Capability Proposal: <name>
- Plane: <Domain | Data | Experience> (004), or the declared cross-cutting area
- Evidence (≥2 independent): <case ref + audit ref or measured count in this tree>
- Business language: <how a domain expert says it>
- Non-composability: <why existing primitives cannot express it — answer 007 §27 Q1–Q3>
- Universality or declared vertical scope: <refs, or the declared scope>
- Sketch per layer (1–9 of §3): <one line each; "deferred: <reason>" allowed>
- NFR sketch (§3b): <security / performance / architecture, or waiver + reason>
- Fan-out (007 §33): <DAG nodes added, what can be shared, worst-case physical fan-out, budget>
- Conformance sketch: <which test would prove it, negative case included, and how it will be mutated>
```

Admission is recorded in `ROADMAP.md`'s `## Planned`; rejection is recorded too, with its reason — in
`menata-app-document`'s `audits/` when the reasoning is long. A rejected proposal returning with new
evidence restarts at A1, not from memory. Two worked rejections: the `internal/ir` refactor
(`internal/ir/doc.go` carries the full measurement) and a static fixture-discovery gate deleted for a
~50% false-finding rate.

---

# 6. Relationship to the other artifacts

```text
menata-app-document/case-portfolio.md   → evidence (business terrain)
menata-app-document/audits/*            → evidence (measured findings, dated)
        │
        ▼  admission test (§2)
menata-app/ROADMAP.md  ## Planned       → admitted, feature-level
        │
        ▼  definition of done (§3) + NFR gates (§3b) + extension architecture (§4)
implementation + internal/conformance
        │
        ▼  ratchet
menata-app/capabilities.md              → single source of record, Built rows with their proof
menata-app/ROADMAP.md  ## Shipped       → what a reader of the product sees
menata-app-document/development-history.md → why, measured, and how it was verified
```

`menata-app-document`'s own README defines which document holds what, and `CLAUDE.md`'s "Where a write-up
goes" carries the operational form. This section is the lifecycle view of the same split and deliberately
does not restate it (001 #8).
