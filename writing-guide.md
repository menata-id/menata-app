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

**`label` is exactly as much metadata as `route`.** A page rendering its own nav item's title (a
`pageShell(...)` call, an `<h1>`, a card's link text) must read it via `rendering.labelByID(id)`
(`internal/rendering/machine.templ`), never retype the label string, the same discipline
`rendering.routeByID(id)` already holds `route` to (`menata-app/CLAUDE.md`'s "Where a
metadata-derived value belongs"). `internal/conformance.TestRenderingHasNoHardcodedApplicationLabel`
gates every `.templ` file for this — it found nine live violations the first time it ran (every
Page's own title, retyped on a line right next to an already-correct `routeByID` href), which is
the actual failure mode this convention exists to prevent: the two strings are the same
`navigation:` entry, and only one of them was being kept in sync.

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

**What a Permission *can* do today:** gate `decide` (still one real Machine pair only), or gate
`edit`/`delete` on the generic update/delete routes — **for any Machine, not just
`mch_approval_step`** (generalized 2026-09-19, once `edit`/`delete` proved they needed the exact
same record-scoped shape `decide` already had) — record-scoped to a `person`/`relation` field on
that same record:

```yaml
permissions:
  - id: prm_decide_own_step   # mch_approval_step only -- decide is still hardcoded to it, see above
    action: decide
    actor_field: fld_assignee
  - id: prm_edit_own_step     # any Machine: only the record's own fld_assignee may edit/delete it
    action: edit
    actor_field: fld_assignee
  - id: prm_delete_own_step
    action: delete
    actor_field: fld_assignee
```

`actor_field` must name a field on the same Machine that is itself a reference (`person` or
`relation`) — a Permission whose actor can never be resolved would silently protect nothing.
`edit`/`delete` are enforced both server-side (`allowsRecordEdit`/`deleteAllowed`,
`internal/web/record.go`, and the JSON `/api` twins) and client-side (Edit/Delete buttons hide
themselves when `authorization.AllowsAction` would refuse them, the same way `decide`'s own
Approve/Reject buttons already did) — declaring the Permission is enough; no handler code is
needed for a new Machine the way `decide` still requires.

**What every Machine gets for free regardless, with no Permission declared at all:** any
authenticated member of the Workspace can create, edit, and delete its records through the
generic routes — declaring no Permission means unrestricted, not "nobody can." There is still no
per-Field permission (see "What this can't do yet" below), and record creation has no Permission
of its own yet (there is no existing record to match an `actor_field` against at that point).

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

## 10. Events: declarative post-write triggers

An Event fires a runtime Service after a field's value changes on a successful update —
`006-runtime-model.md`'s Behavioral Model chain, `Event → Action → Permission/Constraint →
Service/Data operation → State change`, realized for the first time 2026-09-19 (until then this
shape only existed as hand-written Go, repeated at every call site that needed it). Real shipped
example, `metadata/task.yaml` — log an Activity row whenever a Task's own status changes:

```yaml
events:
  - id: evt_task_status_changed
    on: fld_status
    then:
      service: log_activity
      summary: "\"{fld_title}\" moved from {old} to {new}"
      summary_override_when: done
      summary_override: "\"{fld_title}\" completed"
```

Read as: whenever `fld_status`'s new value differs from its old one, run `log_activity` with
`summary` filled in — unless the new value equals `summary_override_when`, in which case
`summary_override` is used instead. `{old}`/`{new}` are the triggering field's own before/after
values; any other `{field_id}` in braces is that field's current (post-write) value. This
placeholder substitution is deliberately not a general templating language — the same minimalism
`internal/expression`'s `equals`/`not_equals` vocabulary already established for Constraint.

Two things this is *not*: leaving `when_equals` off (the field above doesn't set one) means "any
change fires it" — unlike Constraint, where `when_equals` is required, because a Constraint gates
one specific transition while an Event merely observes one. And `summary_override`/
`summary_override_when` support **at most one** override, not arbitrary per-value branching —
if your wording needs more than "a default, and one exception," that's not expressible here yet.

`service: log_activity` is the one Service this runtime realizes today
(`internal/domain.KnownServices`) — the same closed-set discipline `action:` uses for Permission.
There is no way to declare a new Service purely in YAML, the same limit §8 already describes for
Action.

**A second, mutually exclusive Event shape fires on record creation instead of a field change** —
`on_create: true` in place of `on:`/`when_equals:`. Real shipped example, `metadata/task.yaml`:

```yaml
events:
  - id: evt_task_created
    on_create: true
    then:
      service: log_activity
      summary: "\"{fld_title}\" created"
```

Fires once, unconditionally, the moment the record exists — there's no prior value to compare
against, so `when_equals` is rejected on an `on_create` Event (and `on`/`on_create` are mutually
exclusive: declaring both fails validation). `{old}`/`{new}` are meaningless here too; only
`{field_id}` placeholders make sense in an `on_create` Event's own `summary`. This generalized
what used to be `internal/web`'s own hardcoded `logRecordCreated` (a `switch machine.ID` over
`mch_document`/`mch_task`/`mch_project`) — three real cases already living as one Go switch
statement before this shape existed, not a speculative addition.

One trigger this still deliberately doesn't support, because no second real case has needed it
yet: firing on a schedule/time threshold (the shape SLA-breach detection would actually want,
still hardcoded and read-triggered — see `internal/composition/approval.go`).

## 11. Field defaults

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

## 12. The complete grammar surface

Sections 1–11 teach by example. This section is the index: **every key the parser accepts, and
every value a closed vocabulary allows.** Anything not listed here either fails validation at
startup or is silently ignored — there is no third outcome, and nothing is inferred from a key the
parser doesn't know.

Three properties are worth stating once, because they hold everywhere below: ids are regex-gated
by prefix; closed vocabularies are extended by Go code and a recompile, never by writing a new
value in YAML; and every cross-reference (`fld_*` naming a field, `mch_*` naming a Machine) is
resolved and checked at load time, not at first use.

### 12.1 `app.yaml` — the application manifest

| Key | Value | Notes |
|---|---|---|
| `workspace.id` / `workspace.name` | string | One Workspace per process today |
| `application.id` / `application.name` | string | |
| `application.machines[]` | file names | Load order; each file is one Machine |
| `application.navigation[]` | list of items | The topbar is rendered from this list, never hand-written |
| `application.hidden_nav_groups[]` | group labels | Filtered out after `home_card`/primary-group resolution |

Navigation item keys:

| Key | Value | Notes |
|---|---|---|
| `id` | `nav_*` | Referenced from `.templ` via `routeByID("nav_xxx")` — that's how a page links to a sibling screen without retyping the route |
| `label` | string | |
| `route` | path | Must have a real handler in `internal/web/router.go` (gated by `TestNavigationRoutesAreRegistered`) |
| `group` | string | Presentational label. First group declared stays inline; later ones collapse into a dropdown |
| `priority` | int | |
| `badge` | closed set | Only `approval_inbox_pending` today |
| `home_card` | bool | Marks the Workspace Home card's destination (`Application.HomeRoute`) |

### 12.2 A Machine file

| Key | Value | Notes |
|---|---|---|
| `id` | `mch_*` | Required, regex-gated |
| `name` | string | Required |
| `fields[]` | list | See 12.3 |
| `constraints[]` | list | See 12.4 |
| `events[]` | list | See 12.5 |
| `permissions[]` | list | See 12.6 |
| `view` | block | See 12.7. Omit it entirely and you get a table |

### 12.3 `fields[]`

| Key | Value | Notes |
|---|---|---|
| `id` | `fld_*` | Required, unique within the Machine |
| `name` | string | The label |
| `type` | one of 9 (12.8) | Unknown type fails the load |
| `required` | bool | |
| `options[]` | strings | **Required when `type: status`** |
| `machine` | `mch_*` | **Required when `type: relation`** |
| `default` | string | Always quoted, even for numbers/booleans. Create-only. For a `status` field it must be one of its own `options` |

### 12.4 `constraints[]` — one shape

```yaml
constraints:
  - id: cst_*
    on: fld_*              # the field whose transition is gated
    when_equals: <value>   # gate only the transition to this value
    block_if:
      related_machine: mch_*
      related_field: fld_*     # the field on THAT machine pointing back here
      condition: { field: fld_*, op: equals|not_equals, value: <string> }
```

Reads as: *block setting `on` to `when_equals` while any related record matches `condition`.*

### 12.5 `events[]` — two mutually exclusive shapes

```yaml
events:
  - id: evt_*
    on: fld_*              # shape A: field-change
    when_equals: <value>   #   optional; omit = any change to that field
    then: { service: log_activity, summary: "...", summary_override_when: <value>, summary_override: "..." }

  - id: evt_*
    on_create: true        # shape B: record creation. Never combined with `on`
    then: { service: log_activity, summary: "..." }
```

`summary` placeholders: `{old}`, `{new}`, and any `fld_*` id in braces. Not a templating language —
that is the whole list. At most one override.

### 12.6 `permissions[]` — one shape

```yaml
permissions:
  - id: prm_*
    action: decide|edit|delete   # closed set
    actor_field: fld_*           # must be a person field on this Machine
```

Reads as: *only the person named in `actor_field` on this record may perform `action` on it.* A
Machine declaring no permission for an action leaves it open to any authenticated member.

### 12.7 `view`

| Key | Value | Notes |
|---|---|---|
| `layout` | `table` \| `board` | Default `table` |
| `group_by` | `fld_*` | **Required when `layout: board`** — the field whose values become columns |
| `sla_field` | `fld_*` | Must be a `date` field. Renders as OVERDUE / "N days left" |
| `card_fields[]` | `{field: fld_*, role: ...}` | Projected onto a composed card. Only the Approval Inbox card consumes this today |

### 12.7a `datasets[]` — named numbers over this Machine's records

| Key | Value | Notes |
|---|---|---|
| `id` | `ds_*` | Required, unique within the Machine |
| `dimension` | `fld_*` | Optional grouping axis. Omit it for a grand total with no breakdown |
| `measures[].id` | `msr_*` | Required, unique within the dataset |
| `measures[].aggregate` | `count` \| `sum` | `count` counts records; `sum` adds up a number field |
| `measures[].field` | `fld_*` | **Required for `sum`** (which number field), and **must be absent for `count`** |
| `measures[].where` | `{field, op, value}` | Optional filter, the same comparison shape `constraints[].block_if.condition` uses |

A Dataset says what numbers exist, not how a screen draws them. Everything it names must be a
field of its own Machine — a Dataset spanning two Machines isn't expressible today.

```yaml
# in task.yaml -- "how many Tasks does each assignee have, and how many are still open?"
datasets:
  - id: ds_task_workload
    dimension: fld_assignee
    measures:
      - id: msr_total
        aggregate: count
      - id: msr_active
        aggregate: count
        where: {field: fld_status, op: not_equals, value: done}
```

A screen still decides which measure it displays where (that binding is Go), but the counting,
grouping and filtering are metadata: changing `where.value` above changes Team Capacity's own
numbers with no code edit.

### 12.8 Closed vocabularies — the complete list of values you may write

| Vocabulary | Values | Extended by |
|---|---|---|
| Field types | `text` `number` `boolean` `date` `status` `person` `money` `relation` `file` | `domain.KnownFieldTypes` |
| Layouts | `table` `board` | `domain.KnownLayouts` |
| Card field roles | `title` `person` `money` `status` `date` | `domain.KnownCardFieldRoles` |
| Aggregates | `count` `sum` | `domain.KnownAggregates` |
| Actions | `decide` `edit` `delete` | `domain.KnownActions` |
| Services | `log_activity` | `domain.KnownServices` |
| Comparison operators | `equals` `not_equals` | `expression.KnownOps` |
| Navigation badges | `approval_inbox_pending` | `domain.KnownNavigationBadges` |

Every one of these is a deliberate static seam (007 §14). Writing a value outside the set fails the
load with a named error — it never degrades to "do nothing quietly".

### 12.9 Concept → metadata → code index

The same constructs, mapped to the concept doc that defines them and the code that realizes them.
Useful when you need to know whether something is *specified but unbuilt* (common) or *built but
undocumented* (rare).

| Construct | Concept | Realized by |
|---|---|---|
| Machine | 006 §Machine | `domain.Machine`, `metadata.Parse`/`Validate`, `data.Store`, `rendering/machine.templ` |
| Field + types | 006 §Field | `domain.KnownFieldTypes`, `data.ValidateRecord`, `rendering.fieldInput` |
| Field default | 001 #5 (Convention over Configuration) | `domain.Field.Default`, `data.ApplyDefaults` |
| Relation | 006 §Relation, 007 §7.5 | `domain.Field.RelatedMachine`, `data.ValidateRelations` |
| Child collection | 006 §Relation (reverse) | `domain.FindChildCollections` — derived, not declared |
| Constraint | 006 §Constraint | `domain.Constraint`, `behavior.CheckConstraints` |
| Expression (`op`) | 007 §9 | `internal/expression` — two operators so far |
| Event | 006 §Event, Behavioral Model | `domain.Event`, `behavior.MatchedEvents`/`MatchedCreateEvents`, `web.runEvents`/`runCreateEvents` |
| Service | 006 §Service | `domain.KnownServices`, `web.logActivity` |
| Permission | 006 §Permission, 005 §Security Ordering | `domain.Permission`, `authorization.AllowsAction` |
| Action | 006 §Action | `domain.KnownActions` + `internal/action` — **Go code, not declarable** (§8) |
| View / Layout | 006 §Layout/§View, 007 §12.2 | `domain.View`, `internal/experience`, `composition.loadBoardColumns` |
| SLA badge | 006 §Field + Experience | `experience.EvaluateSLA` |
| Projection (`card_fields`) | 007 §7.6 | `domain.CardField`, `composition.ProjectCardFields`, `rendering.projectedFieldValue` |
| Dataset / Dimension / Measure | 007 §7.2-§7.4 | `domain.Dataset`, `domain.Measure`, `domain.KnownAggregates`, `composition.Aggregate` — `count`/`sum` only |
| Navigation | 006 §Navigation, 004 | `domain.Navigation`, `experience/navigation.go`, `rendering.pageShell` |
| Workspace / Application | 006 §Organizational Model | `domain.Workspace`, `domain.Application`, `metadata.LoadApplication` |

If a concept from 006/007 isn't in this table — Query, Binding, Slot, Component contract, Theme,
API, Workflow/Process — it has **no metadata expression today**. That's
the accurate answer to "can I declare this?", and the next section explains which of those gaps are
deliberate.

## What comes free vs. what's hardcoded today — the honest map

Every item on the left, any new Machine gets automatically, purely from YAML. Every item on the
right is real, working code in this app, but wired to specific Machine ids in Go — declaring
similar-looking metadata for a *different* Machine does not activate it.

| Generic (any Machine, metadata only) | Hardcoded to specific Machines (real Go code required for a new one) |
|---|---|
| CRUD screens + JSON API, table and board views | The `decide` Action, and everything hardcoded to `mch_document`/`mch_approval_step` behind it |
| Relations, `person`, child collections, many-to-many | Document submission wizard |
| Constraints (`equals`/`not_equals` shape) | Signature-coordinate placement screen |
| Events (post-write field-change or record-creation → one Service) | PDF signature compositing |
| Record-scoped `edit`/`delete` Permission (any Machine) | Approval progress stepper UI |
| Field defaults | SLA-breach detection (still read-triggered, not a real Event yet) |
| SLA badges (`view.sla_field`) | — |
| Counting/summing a Machine's own records (`datasets:`) | Composed screens: Approval Inbox, My Tasks, Calendar, Automation, Board Settings. Dashboard, Sprint Dashboard and Team Capacity now get their *numbers* from declared `datasets:`, but their layout, which measure lands in which column, and any list of records they show (Pending, Attention) are still Go |

**The right column is a capability snapshot, not a permanent exemption list.** Each entry existed
because metadata couldn't express it *when it was written* — `view.card_fields` (Projection) and
`Event.OnCreate` both started as a right-column entry and moved left once a real second case
justified generalizing them (`menata-app-document`'s `workflow-behavior-decomposition-criteria.md`
B1-B5, `ui-composition-decomposition-criteria.md` Q1-Q5). Don't read a row here as "this will
always require code" — re-check it against those criteria before assuming it still does,
especially at a `ROADMAP.md` phase close (CLAUDE.md's "Deciding whether a literal is a
metadata-hardcoding violation").

If what you're building is a new data model with CRUD, relations, a board or table view, a
same-shape cross-record rule, a field-change side effect, and defaults — the left column is
genuinely enough, no engineer needed. If it needs a custom multi-step business action, a bespoke
composed page, or something Events/Permissions' one shape each can't express (§§8-10), that is
real feature work today, not a metadata exercise — say so plainly rather than guessing at YAML
that will fail validation or silently do nothing.

## What this can't do yet

Honest current limits, not a roadmap — some of these may change over time:

- **Exactly one Action, hardcoded to one Machine pair.** See §8 — this is the limit most likely
  to matter for a new business process.
- **No field-level permissions.** Access control today is per-Machine and per-Action at best; you
  cannot hide or lock one Field from one role while leaving the rest editable.
- **Events fire on a field change or a record creation — not on a schedule.** See §10.
  SLA-breach detection is still hardcoded, read-triggered Go for exactly that reason.
- **An Event's wording supports at most one override**, not arbitrary per-value branching (§10).
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
