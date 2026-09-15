# menata-app

The runtime application that turns Menata Runtime Metadata into a running application.

This repository starts from the design concepts below (carried over from the private
`menata-runtime` design-history repository) and builds the real implementation from a clean
slate — no code is ported in from prior prototypes.

## Concepts (read in order)

1. [001-design-principles.md](001-design-principles.md)
2. [002-architecture.md](002-architecture.md)
3. [003-runtime-language.md](003-runtime-language.md)
4. [004-runtime-metadata.md](004-runtime-metadata.md)
5. [005-runtime-lifecycle.md](005-runtime-lifecycle.md)
6. [006-runtime-model.md](006-runtime-model.md)
7. [007-composable-runtime-architecture.md](007-composable-runtime-architecture.md)

## Build order

See [ROADMAP.md](ROADMAP.md) for the phased build plan — what gets implemented when, and the
forcing condition that justifies each phase.

## Trial applications

Two priority target applications — Case 3 (Document Approval) and Case 19 (Project Management),
out of a larger 21-case portfolio. See [case-portfolio.md](case-portfolio.md) for all 21 and
both priority cases' full screen breakdowns, and [ui-sample/](ui-sample/) for their design
mockups.

## Tech stack

Go 1.25 + PostgreSQL + `templ` (server-rendered HTML) + `chi` + `pgx` + `goose` — a single
binary, matching 007-composable-runtime-architecture.md §4.10's "modest-server, server-rendered"
constraint. No client-side framework.

**Client-side interactivity is minimized deliberately, in this order of preference:**

1. **HTMX** (2.0.4) first — partial page swaps over real HTTP requests, no client state to manage.
2. **Hyperscript** (0.9.93) when HTMX's request/response model genuinely isn't enough (e.g. drag
   handles, local UI toggles) — inline, declarative, still no build step.
3. **Vanilla JS** only as a named exception, when neither of the above can express the
   interaction (e.g. Phase 14's signature-coordinate drag editor may need this) — kept small,
   inline or a single `<script>`, no bundler, no framework, and the exception's reason stated in
   a comment next to the code.

Both `<script>` tags load from `pageShell` (`internal/rendering/machine.templ`) already.

## Naming

`menata-app` is short for **Menata Runtime App** — the deployed application produced by running
Menata Runtime, hosted at [menata.app](https://menata.app). Throughout the concept docs below,
"Menata Runtime" names the engine/system itself (parsing, compiling, executing Runtime Metadata);
"Menata App" names this repository — the product that ships that runtime as a running application.

## Relationship to menata-runtime

`menata-runtime` (private) holds the capability-discovery history that produced these concepts:
seven parallel prototypes, benchmarks, and the capability registry that proved out what a Menata
Runtime needs to support. That discovery phase is closed. This repository (`menata-app`) is where
active development happens going forward.
