# Writing Guide: building your own application

This runtime turns a declarative YAML description — a **Machine** — into a working application:
CRUD screens, validation, relations, board/table views, and simple business rules, with no Go
code written per data model. This guide walks through authoring one from scratch.

For the architecture and reasoning behind these choices, see the numbered concept docs
(`001-design-principles.md` through `007-composable-runtime-architecture.md`). For what exists
today in one reference table, see `capabilities.md`. This guide is the practical "how," those are
the "why" and the "what."

The worked example below is a small **Bug Tracker** — a Component (e.g. a subsystem) that owns
Bugs. It isn't part of the shipped application; it's chosen purely to be small and easy to follow.

## 1. A Machine is a YAML file

Every Machine is one file under `metadata/`, referenced from `metadata/app.yaml`'s own
`application.machines` list. The minimal shape:

```yaml
id: mch_component
name: Component
fields:
  - id: fld_name
    name: Name
    type: text
    required: true
```

Add it to `app.yaml`:

```yaml
application:
  machines:
    - component.yaml
    # ... your other machine files
```

That's enough for a working table view with create/edit/delete — no other code changes. IDs
follow a fixed convention: `mch_*` for Machines, `fld_*` for Fields, `cst_*` for Constraints,
`prm_*` for Permissions.

## 2. Field types

Each Field declares a `type:`. What's available today:

| Type | Use it for | Notes |
|---|---|---|
| `text` | Short free text | Single-line only — no long-text/rich-text type yet |
| `number` | A numeric value | |
| `date` | A calendar date | |
| `status` | A closed set of named states | Requires an `options:` list |
| `person` | "Assigned to a user" | See below — this is special |
| `relation` | A reference to another Machine's record | Requires `machine:` naming the target |
| `file` | An uploaded file | Value stored is a storage key, not the raw file |

`boolean` and `money` are also declared field types, but no shipped Machine uses either yet —
they'll work, just without a real example to point to.

A full Bug Machine using several of these:

```yaml
id: mch_bug
name: Bug
fields:
  - id: fld_title
    name: Title
    type: text
    required: true
  - id: fld_status
    name: Status
    type: status
    required: true
    options: [todo, in_progress, done]
    default: todo
  - id: fld_severity
    name: Severity
    type: status
    required: true
    options: [low, medium, high]
    default: medium
  - id: fld_assignee
    name: Assignee
    type: person
  - id: fld_component
    name: Component
    type: relation
    machine: mch_component
  - id: fld_due_date
    name: Due Date
    type: date
```

## 3. Relations and `person`

`relation` is a single-value reference to another Machine — `fld_component` above renders as a
dropdown of real Components and is validated to actually exist (you can't save a Bug pointing at
a Component that doesn't).

`person` is a relation to the built-in user Machine, handled specially: you never write
`machine: mch_user` yourself — the loader sets it automatically. Anywhere you want "assign this
to someone," use `type: person`, not a hand-wired `relation`.

Any Machine that another Machine's `relation`/`person` field points at automatically gains a
**child collection** on its own detail page: a Component's detail view will list every Bug whose
`fld_component` points at it, with no extra declaration.

## 4. Views: table or board

The default is a flat table. To group records into columns instead (e.g. a Kanban-style board by
status):

```yaml
view:
  layout: board
  group_by: fld_status
```

`group_by` can name either a `status` field's own options (fixed columns) or a `relation` field
pointing at another Machine (dynamic, reorderable columns — the real Machine's own records become
board columns).

## 5. Constraints: simple cross-record rules

A Constraint blocks a field transition while a related record matches a condition. The entire
comparison vocabulary is `equals` / `not_equals` against a literal value — deliberately small,
not a general expression language. Example: don't let a Component be archived while it still has
open Bugs.

```yaml
constraints:
  - id: cst_component_archived_no_open_bugs
    on: fld_status
    when_equals: archived
    block_if:
      related_machine: mch_bug
      related_field: fld_component
      condition:
        field: fld_status
        op: not_equals
        value: done
```

Read as: when `fld_status` is being set to `archived`, block the write if any `mch_bug` record
whose `fld_component` points back at this record has a `fld_status` that is `not_equals` `done`.

## 6. Field defaults

`default:` fills a field when a new record leaves it empty — create only, never re-applied on
update, and never overrides a value actually submitted. Always written as a string in YAML, even
for a non-text field (`fld_status` above uses `default: todo`); an invalid default for a `status`
field (not one of its own `options:`) fails at metadata load time, not silently at runtime.

## What this can't do yet

Honest current limits, not a roadmap — some of these may change over time:

- **No field-level permissions.** Access control today is per-Machine and per-Action at best; you
  cannot yet hide or lock one Field from one role while leaving the rest editable.
- **No general Machine-level CRUD permission.** Any authenticated member of a Workspace can edit
  or delete most records in it; only a small number of hardcoded exceptions exist.
- **Reads are whole-Machine.** There's no filtering, projection or pagination pushed to the
  database — a page fetches a Machine's full record set and reduces it in application code. This
  is fine well past ten thousand records in one Machine, and becomes a real cost somewhere between
  ten and fifty thousand.
- **Metadata loads once, at process startup.** Editing a `*.yaml` file requires a restart to take
  effect — there's no hot reload.
- **No metadata versioning.** Removing a Field from a Machine's YAML doesn't migrate or warn about
  existing data already stored under that field's id — it's simply no longer read.
- **Constraints are intentionally limited.** Only `equals`/`not_equals` against a literal value,
  no boolean combinators, no cross-field arithmetic.

If you hit one of these and it matters for what you're building, the numbered concept docs explain
the reasoning behind the current shape and where each of these is headed.
