# Writing Guide: building your own application

This runtime turns a declarative YAML description into a working application: CRUD screens,
validation, relations, board/table views, and a small set of business rules, with no Go code
written per data model. This guide is grounded directly in the runtime's own source
(`internal/domain`, `internal/metadata`) and the real metadata files under `metadata/` — every
example below either is, or is a minimal variant of, something this codebase actually runs today.

For the architecture and reasoning behind these choices, see the numbered concept docs
(`001-design-principles.md` through `007-composable-runtime-architecture.md`). For what exists
today in one reference table, see `capabilities.md`. This guide is the practical "how."

The worked example below is a small **Bug Tracker** — a Component (e.g. a subsystem) that owns
Bugs. It isn't part of the shipped application; it's chosen to be small and easy to follow.

**Read section 8 before you assume a business rule can be expressed in metadata alone.** The
single biggest mistake a first-time (or an AI-driven) metadata author makes with this runtime is
assuming that because CRUD is fully generic, *behavior* is too. It mostly isn't yet — section 8
draws the line precisely, with the source lines that prove it.

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

That's enough for a working table view with create/edit/delete, a JSON API, and a detail page —
no other code changes.

**Identity rules, enforced at load time (`internal/metadata/validate.go`), not a convention you
can bend:**

| Kind | Pattern | Example |
|---|---|---|
| Machine id | `^mch_[a-z][a-z0-9_]*$` | `mch_component` |
| Field id | `^fld_[a-z][a-z0-9_]*$` | `fld_name` |
| Constraint id | `^cst_[a-z][a-z0-9_]*$` | `cst_component_archived_no_open_bugs` |
| Permission id | `^prm_[a-z][a-z0-9_]*$` | `prm_decide_own_step` |
| Navigation item id | `^nav_[a-z][a-z0-9_]*$` | `nav_dashboard` |

A bad id, a duplicate id within its own kind, or any other structural problem below fails
**metadata loading at startup** with every problem listed at once (`ValidationError`) — the
process refuses to serve traffic on invalid metadata rather than failing on first use. There's no
separate "lint" command today; a syntax/validation error surfaces the first time you `make run` or
restart. Read the error text carefully — it names the exact field/id/value that's wrong.

## 2. The application manifest (`app.yaml`)

One manifest per Workspace, naming its Workspace, its one Application, the ordered list of Machine
files to load, and the navigation menu:

```yaml
workspace:
  id: ws_default
  name: Default Workspace

application:
  id: app_task_tracker
  name: Task Tracker
  machines:
    - component.yaml
    - bug.yaml
  navigation:
    - id: nav_home
      label: Home
      route: /home
      priority: 0
    - id: nav_bugs
      label: Bugs
      route: /machines/mch_bug
      group: Bug Tracker
      priority: 1
```

`workspace.id` matches `^ws_[a-z][a-z0-9_]*$`, `application.id` matches `^app_[a-z][a-z0-9_]*$`.
Each entry in `machines:` is a path to a Machine file, resolved relative to `app.yaml`'s own
directory — order here doesn't affect behavior, but keeping it alphabetical or grouped by
Application area helps a human (or an agent) scanning the file.

**Navigation is a real declared list, not free-form:** `id`/`label`/`route` are required
(`route` must start with `/`); `group` is an optional submenu label (a plain string, not a
machine id — items with no `group` render flat, e.g. Home); `priority` orders items within their
group; `badge` is optional and, if present, must be one of the runtime's own known live-count
badges (today, exactly one exists: `approval_inbox_pending` — declaring any other string fails
validation, since a badge nothing renders would silently show nothing). A nav item's `route` is
**not** cross-checked against real routes — most destinations are bespoke `internal/web` handlers,
not generated from a Machine, so a typo in `route` fails silently (a 404 at click time), not at
load time.

**A generic Machine's own page is always reachable at `/machines/{id}`** even with no navigation
entry at all — navigation is a convenience menu, not what makes a Machine's CRUD screens exist.

## 3. Field types

Each Field declares a `type:`. The full closed set the runtime understands
(`internal/domain.KnownFieldTypes`) — declaring anything else fails validation:

| Type | Use it for | Notes |
|---|---|---|
| `text` | Short free text | Single-line only — no long-text/rich-text type yet |
| `number` | A numeric value | |
| `boolean` | True/false | Declared and rendered (a checkbox), but no shipped Machine uses one yet |
| `date` | A calendar date | |
| `status` | A closed set of named states | Requires a non-empty `options:` list |
| `person` | "Assigned to a user" | See §4 — implicitly a relation to the built-in user Machine |
| `money` | A currency value | Declared, but no shipped Machine uses one yet — treat as unproven |
| `relation` | A reference to another Machine's record | Requires `machine:` naming the target |
| `file` | An uploaded file | The stored value is a storage key, not the raw file |

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

Validation you'll actually hit while authoring: a `status` field with an empty `options:` list
fails to load; a `relation` field whose `machine:` isn't a valid-looking Machine id fails to load;
once the whole Application loads, a `relation`/`person` field whose target Machine doesn't
actually exist in `app.yaml`'s own `machines:` list also fails (a single Machine file can't check
this on its own — it's checked once every file is loaded).

## 4. Relations, `person`, and child collections

`relation` is a single-value reference to another Machine — `fld_component` above renders as a
dropdown of real Components and is validated to actually exist (you can't save a Bug pointing at
a Component that doesn't exist).

`person` is a relation to the built-in user Machine, handled specially by the parser
(`internal/metadata.Parse`): you never write `machine: mch_user` yourself — it's set
automatically the moment a field's `type` is `person`. Anywhere you want "assign this to
someone," use `type: person`, not a hand-wired `relation` to `mch_user`.

Any Machine that another Machine's `relation`/`person` field points at automatically gains a
**child collection** on its own detail page: a Component's detail view lists every Bug whose
`fld_component` points at it, with no extra declaration anywhere.

## 5. Many-to-many: a join Machine, not a new mechanism

There is no `type: multi_relation` or array-of-relations field. Many-to-many is a small,
ordinary Machine with two `relation` fields, one to each side — the real shipped example:

```yaml
id: mch_card_label
name: Card Label
fields:
  - id: fld_task
    name: Task
    type: relation
    machine: mch_task
  - id: fld_label
    name: Label
    type: relation
    machine: mch_label
```

Nothing else is needed. This join Machine shows up as a child collection on **both** `mch_task`'s
and `mch_label`'s own detail pages automatically, by the same rule as §4 — many-to-many is table
stakes composed from Relation and Child Collection, not a third primitive.

## 6. Views: table or board

The default, with no `view:` block at all, is a flat table. Two ways to get a board instead:

**Fixed columns, grouped by a `status` field's own options:**

```yaml
view:
  layout: board
  group_by: fld_status
```

**Dynamic, reorderable columns, grouped by a `relation` to another Machine** (the real shipped
shape — Task's board groups by its own List, and Lists are ordinary records someone can rename or
reorder, not a hardcoded enum):

```yaml
# mch_list -- an ordinary Machine, its own records ARE the board's columns
id: mch_list
name: List
fields:
  - id: fld_name
    name: Name
    type: text
    required: true
```

```yaml
# mch_bug -- boards by relation, not by status
view:
  layout: board
  group_by: fld_list   # a `relation` field on this Machine, pointing at mch_list
```

`view.layout` must be `table` or `board` (the only two `internal/domain.KnownLayouts`);
`view.group_by` must name a real field on the same Machine, checked at load time.

## 7. SLA badges

A `date` field can be flagged for SLA rendering — it then shows as `OVERDUE` or "N day(s) left"
instead of a plain date, anywhere that field is displayed:

```yaml
view:
  layout: table
  sla_field: fld_due_date
```

`sla_field` must name a real field on the same Machine, and that field must be `type: date` —
validated the same way as `group_by`. This is fully generic: any Machine can declare it, not just
the one that happens to ship with it today.

## 8. Actions and Permissions — read this before you assume something works

This is the section most likely to trip up an agent generating metadata for a new business
process, because CRUD's genuine flexibility makes it tempting to assume behavior is equally
open-ended. It is not, and the runtime's own source says so explicitly.

**There is exactly one Action the runtime realizes today: `decide`** (Approve/Reject on an
Approval Step). `internal/domain.KnownActions` is a closed map containing only that one entry.
Declaring a Permission with any other `action:` value fails metadata validation outright
("action %q is not an action this runtime realizes"). There is no way to declare a brand-new
Action — "Submit," "Publish," "Cancel," anything — purely in YAML. Doing that requires real Go
code: a new entry in `internal/domain.KnownActions`, a new handler, and whatever business logic
that Action performs. This is the one place "no per-model code required" (the very first line of
this guide) does not hold.

**And `decide` itself is hardcoded to one specific pair of Machines**, by explicit design, not
oversight — `internal/action/decide.go`'s own comment: *"This is hardcoded to
mch_document/mch_approval_step's own field ids, not a generic [mechanism]."* The
`POST /machines/{machineID}/records/{id}/decide` route exists for any Machine id path-wise, but
the handler checks the Machine id and does nothing for any Machine other than
`mch_approval_step`. Declaring `permissions: - action: decide` on your own new Machine does not
give it an approval workflow — it declares a Permission the runtime cannot actually invoke for
that Machine.

**What a Permission *can* do today:** gate the one real Action, for the one real Machine pair,
record-scoped to a `person`/`relation` field on that same record:

```yaml
# The real shipped shape, unchanged -- do not adapt this to a new Machine expecting it to work
permissions:
  - id: prm_decide_own_step
    action: decide
    actor_field: fld_assignee
```

`actor_field` must name a field on the same Machine that is itself a reference (`person` or
`relation`) — a Permission whose actor can never be resolved would silently protect nothing.

**What every Machine gets for free regardless, with no Permission declared at all:** any
authenticated member of the Workspace can create, edit, and delete its records through the
generic routes. There is no per-Field or general per-Machine CRUD permission mechanism yet (see
"What this can't do yet" below) — if your business process needs "only the owner can edit this,"
that is not expressible in metadata today.

## 9. Constraints: simple cross-record rules

A Constraint blocks a field transition while a related record matches a condition. The entire
comparison vocabulary is `equals` / `not_equals` against a literal value
(`internal/expression.KnownOps`) — deliberately small, not a general expression language, and
there is no `and`/`or` combinator. Real shipped example: don't let a Project become `done` while
it still has open Tasks.

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

Validated at two levels: within one file (`on` names a real field; if that field is `status`
typed, `when_equals` must be one of its own declared `options:`), and once the whole Application
is loaded (`block_if.related_machine` must be a real Machine; `block_if.related_field` must be a
field on *that* Machine, and it must itself be a `relation` pointing back at *this* Machine — a
condition can only identify related records through a real reverse link, never a guess).

## 10. Field defaults

`default:` fills a field when a new record leaves it empty — create only, never re-applied on
update, and never overrides a value actually submitted:

```yaml
- id: fld_status
  type: status
  options: [todo, in_progress, done]
  default: todo
```

Always written as a YAML string, even for a `number` or `boolean` field (e.g. `default: "5"`,
`default: "true"`) — the loader coerces it to the field's real storage type. An invalid default
(not parseable as that type, or not one of a `status` field's own `options:`) fails at metadata
load time, not silently at runtime.

## What comes free vs. what's hardcoded today — the honest map

Every item on the left, any new Machine gets automatically, purely from YAML. Every item on the
right is real, working code in this app, but wired to specific Machine ids in Go — declaring
similar-looking metadata for a *different* Machine does not activate it.

| Generic (any Machine, metadata only) | Hardcoded to specific Machines (real Go code required for a new one) |
|---|---|
| CRUD screens + JSON API, table and board views | The `decide` Action, and everything permission-gated behind it |
| Relations, `person`, child collections, many-to-many | Document submission wizard |
| Constraints (`equals`/`not_equals` shape) | Signature-coordinate placement screen |
| Field defaults | PDF signature compositing |
| SLA badges (`view.sla_field`) | Approval progress stepper UI |
| Workspace scoping, session-auth gating | Composed screens: Dashboard, Approval Inbox, My Tasks, Sprint Dashboard, Calendar, Team Capacity, Automation, Board Settings |

If what you're building is a new data model with CRUD, relations, a board or table view, a
same-shape cross-record rule, and defaults — the left column is genuinely enough, no engineer
needed. If it needs a custom multi-step business action, a bespoke composed page, or per-Field/
per-Machine permission beyond the one shape in §8, that is real feature work today, not a metadata
exercise — say so plainly rather than guessing at YAML that will fail validation or silently do
nothing.

## What this can't do yet

Honest current limits, not a roadmap — some of these may change over time:

- **Exactly one Action, hardcoded to one Machine pair.** See §8 — this is the limit most likely
  to matter for a new business process.
- **No field-level permissions.** Access control today is per-Machine and per-Action at best; you
  cannot hide or lock one Field from one role while leaving the rest editable.
- **No general Machine-level CRUD permission.** Any authenticated member of a Workspace can edit
  or delete most records in it; only one narrow hardcoded exception exists (blocking deletion of
  a decided Approval Step or a Document with any decision on it).
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
- **One Workspace runs one fixed Application today.** A different Workspace needing a genuinely
  different set of Machines isn't supported yet — every Workspace shares the same metadata.

If you hit one of these and it matters for what you're building, the numbered concept docs explain
the reasoning behind the current shape and where each of these is headed.
