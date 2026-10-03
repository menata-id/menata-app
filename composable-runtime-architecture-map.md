# Composable Runtime Architecture Map

> **How this repository's documents relate**: which one wins when two overlap, what each is responsible
> for, and the rules that keep a new composable capability from being recorded in the wrong place.
>
> Ported into `menata-app` 2026-10-02 from `menata-runtime`'s own
> `composable-runtime-architecture-map.md` (488 lines), and **only §2, §12 and §13 of it**. Cited by
> `001` (§109, §272), `002` (§157, §341) and `003` (§129) — four backticked citations that pointed at a
> file nobody in this repo could open, which is what made the port necessary rather than tidy.
>
> **What was deliberately left behind, and why**: §1 Canonical Terminology, §3–§11 (realization pipeline,
> composition model, view lowering, component/data/context contracts, CEP boundary, security ordering) and
> §14–§16 are all covered normatively by `007-composable-runtime-architecture.md` itself. Porting them
> would create a second statement of the same contracts, which is 001 #8 and exactly what §13's own rules
> exist to prevent. §11 "Current Implementation Reality" described `menata-runtime`'s tree.

---

# 1. Source-of-truth hierarchy

When two documents overlap, resolve them in this order:

```text
001 Design Principles
        ↓
002 Architecture
        ↓
003 Runtime Language
        ↓
004 Runtime Metadata
        ↓
005 Runtime Lifecycle
        ↓
006 Runtime Model
        ↓
007 Composable Runtime Architecture
        ↓
Composable Architecture Map (this document)
        ↓
capability-lifecycle.md            (governance: admission, done, NFR gates, seams)
        ↓
capabilities.md                    (evidence: what is Built right now, with its proof)
        ↓
ROADMAP.md                         (sequence: Shipped / In progress / Planned)
        ↓
menata-app-document/               (reasoning: development-history.md, audits/*)
        ↓
internal/, metadata/               (the implementation, which may lag the target)
```

The practical meaning:

1. The numbered documents define stable concepts and architectural constraints.
2. `007` defines the target composable architecture and its normative rules.
3. This map resolves cross-document relationships. **It must not contradict `007`**, and where it looked
   like it might, the section was not ported (see the note above).
4. `capability-lifecycle.md` governs *how* something earns admission; it does not define architecture.
5. `capabilities.md` carries evidence and status. **It does not silently redefine architecture** — a Built
   row describes what exists, and if it conflicts with `007` then one of the two is wrong and that is a
   finding, not a resolution.
6. `ROADMAP.md` defines order, not architecture.
7. `internal/` is the current implementation and **may lag the target**. That is not a defect by itself;
   claiming otherwise is. `007 §40` is where the gap is stated per claim, and
   `conformance.TestClaimMatrixCitesRealArtifacts` holds its citations to artifacts that exist.

**One ordering consequence worth stating, because it was violated.** A document lower in this list may not
be used to *establish* something higher. 007 §40 cited another repository's capability registry as
evidence for its own PROVEN claims — evidence flowing upward from a document that does not exist here, and
the result was a normative document claiming CEL evaluation in a runtime that has none.

---

# 2. Documentation responsibilities

Scoped to this repository's own normative and record-keeping set. **The cross-repository split — which
work goes to `menata-app-document` and which stays here — is defined by `CLAUDE.md`'s "Where a write-up
goes" and by that repo's own README, and is deliberately not restated here** (001 #8); this table is about
the documents in this tree.

| Document | Responsible for | Should not become |
|---|---|---|
| `001-design-principles.md` | enduring philosophy, the four fundamental beliefs | an implementation plan |
| `002-architecture.md` | conceptual layers and the runtime boundary | a component schema |
| `003-runtime-language.md` | language semantics and declarative realization | Go or SQL design |
| `004-runtime-metadata.md` | metadata artifact semantics, the three composition planes | a physical execution plan |
| `005-runtime-lifecycle.md` | the metadata lifecycle and live evolution | a renderer specification |
| `006-runtime-model.md` | logical runtime concepts, Navigation and Theme | a database schema |
| `007-composable-runtime-architecture.md` | the normative composable target, and §40's per-claim status | a status report of every implementation detail |
| this map | cross-document precedence and responsibilities | a second statement of `007`'s contracts |
| `capability-lifecycle.md` | admission, definition of done, NFR gates, extension seams | architectural vocabulary |
| `capabilities.md` | what is Built now, each row citing its proof | a source of architectural truth |
| `writing-guide.md` | how to write metadata, and what is hardcoded today | normative architecture |
| `ROADMAP.md` | Shipped / In progress / Planned, at feature grain | a development diary (it was one; 3,279 lines moved out on 2026-09-30) |
| `CLAUDE.md` | operational rules for working in this repo, and the gate index | a substitute for reading 001–007 |

---

# 3. Consistency rules for a new composable capability

1. Define or reuse the **semantic primitive** first. 007 §27 Q1–Q3 is the checklist; reusing beats adding.
2. Run `capability-lifecycle.md` §2's admission test if it is a **new capability**. If it is architecture
   `001`–`007` already settled, it needs **sequencing, not evidence** — conflating the two stalled three
   phases of work on 2026-09-30.
3. State its status honestly in `007 §40`: **PROVEN** (cited to a file, symbol or test *in this tree*),
   **PARTIAL**, or **PROPOSED**. A PROPOSED row admits nothing and forbids nothing.
4. Define its **lowering target** — which existing primitive it compiles into (007 §25, §26). A
   capability with no lowering target is a second execution engine.
5. Define its **data dependencies and context requirements**, and keep context inside 007 §9.2's allowed
   set, failing closed.
6. Define its **security boundary**, and establish scope before retrieval (007 §20). Never query-then-trim.
7. Define its **physical implications**: 007 §33's fan-out questions, and a budget that prevents unbounded
   work.
8. Add a **conformance proof**, or record why it is deferred. **A gate not shown to bite is not a gate** —
   mutate it, and verify the mutation applied before trusting a green run.
   **And say which kind of gate it is**, because the two have opposite failure modes. A gate that *preserves*
   an achievement fails loudly when something regresses, and silence means safety. A gate that *directs* work
   — `conformance.TestHandWrittenLayoutSitesOnlyShrink` is the only one here — fails when the debt grows, and
   silence means nothing: if its patterns miss a shape, it under-reports and the next session trusts the
   number. That happened. It reported 13 remaining layout sites against a real population of **51**, because
   its three patterns were written by searching for class strings. A directive gate must be re-derived from
   the vocabulary it tracks whenever that vocabulary grows, not maintained by hand.
9. Record it in `capabilities.md` with the test that proves it, and in `ROADMAP.md` at feature grain.
10. Put the reasoning, the measurements and what could not be verified in
    `menata-app-document/development-history.md`, not in `ROADMAP.md`.
11. **Do not create a new View type to bypass a composition boundary** — 007 §12.4 states this
    normatively and §24 gives the Specialization Rule. This rule is a pointer, not a second statement.
12. **Do not introduce a mini-language** where the shared Expression model can express the requirement
    safely (007 §9, and `internal/expression`'s deliberately bounded shape).
13. **A measurement may establish presence, never absence.** Counting what is there with a regex is sound;
    concluding "this has no case" with the same regex is not, because the absence of a class string is not the
    absence of a meaning. Four capabilities were rejected this way in one session and all four were wrong
    (`row`, `grid`, `columns`, `section` — see `capability-lifecycle.md` §2's asymmetry note). To establish
    absence, read the running screens and `menata-app-document`'s `case-portfolio.md`, and state your method
    beside the claim.
14. **Re-measure every number you inherit.** Six carried figures failed on measurement in a single
    session; a deferral reading "no primitive exists" is grep-checkable in a minute. A stale measurement
    reads exactly like a settled decision.
