# Menata App — agent instructions

## What this project is

This is **not an ordinary application** — it's a **composable runtime** (README.md: "The runtime
application that turns Menata Runtime Metadata into a running application"). The product is the
runtime itself; any one app it renders (Task Tracker, Document Approval, whatever's in
`metadata/app.yaml` today) is just one metadata description it happens to be interpreting right
now. `007-composable-runtime-architecture.md` is the normative target for this.

That distinction changes what "just add a button/field/route" means here. In an ordinary app,
hand-writing a button is the whole job. In this repo, hand-writing something that metadata could
express instead is a regression against the actual product — it's exactly the kind of change that
works today and quietly breaks the runtime's own premise (`metadata/app.yaml`, no per-model code)
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
  navigation entry already exists in `metadata/app.yaml`, a page must reference it, not retype it
  as a second literal.
- **001 Principle #8, Reference over Duplication** — "Duplicated metadata should be avoided
  whenever possible." Before hardcoding a string that looks like it describes application
  behavior (a route, a label, a count), grep `metadata/*.yaml` for it first.

Concretely, before adding any hardcoded `href`, label, or button to a page under
`internal/rendering/`: check `metadata/app.yaml`'s `application.navigation` list first. If the
same route/label is already declared there, that's a signal the value belongs in metadata (or
should be read from the `Navigation` the handler already has, e.g. `internal/web/router.go`'s
`d.Navigation`), not typed again in the `.templ`. If you add it anyway because metadata can't
express it yet, say so in a comment — don't leave it silently duplicated.

## Design reference vs. current code

`ui-sample/*.html` is the design target (`internal/web/router.go`'s own comment: "a design
reference, never current-code intent"). When a page under `internal/rendering/` is meant to match
one of these mockups, match it structurally — don't add elements the mockup doesn't have (e.g. an
extra CTA button on a card) just because older code already had one. If porting a mockup surfaces
a pre-existing hardcoded value that should have come from metadata, fix it as part of the port
rather than carrying it forward unreviewed.

## Enforcement, not just prose

`internal/conformance` holds executable tests for architectural obligations 001-007 state in
prose (plane boundaries, handler size). Prose in a doc gets skimmed; a failing `go test` doesn't.
If you find yourself re-explaining the same architectural rule in a PR/commit twice, consider
whether it can become a conformance test instead.

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
