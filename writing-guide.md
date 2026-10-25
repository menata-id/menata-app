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

Every Machine is one file, referenced from a Workspace manifest's own `machines:` list --
`metadata/*.yaml` holds the reusable templates, and a Workspace's own copies live under
`metadata/workspaces/<slug>/`. The minimal shape:

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
| Transition id | `^trn_[a-z][a-z0-9_]*$` | `trn_step_approve` |
| Navigation item id | `^nav_[a-z][a-z0-9_]*$` | `nav_dashboard` |

A bad id, a duplicate id within its own kind, or any other structural problem below fails
**metadata loading at startup** with every problem listed at once (`ValidationError`) — the
process refuses to serve traffic on invalid metadata rather than failing on first use. There's no
separate "lint" command today; a syntax/validation error surfaces the first time you `make run` or
restart. Read the error text carefully — it names the exact field/id/value that's wrong.

## 2. The manifest: a Workspace file plus one file per Application

A Workspace manifest (`metadata/workspaces/<slug>.yaml`) declares the **Workspace** — the Machines it owns and the navigation that belongs to no
single Application — and then names one file per Application:

```yaml
# Which Workspace this installs into, by slug. Not an id: the `ws_...` id is generated when a
# Workspace is created through the UI, so nobody can write it into a file by hand.
workspace: default

# This Workspace's own Machines. A path under <slug>/ is its own copy, made when an Application
# was installed into it; a path outside is one of the three runtime-level Machines every Workspace
# shares (the user Machine, the activity log, notifications).
machines:
  - default/component.yaml
  - default/bug.yaml
  - ../user.yaml

# Navigation belonging to no single Application.
navigation:
  - id: nav_home
    label: Home
    route: /home
    priority: 0

applications:
  - default/applications/bug-tracker.yaml
```

```yaml
# applications/bug-tracker.yaml
id: app_bug_tracker
name: Bug Tracker
description: Track and triage defects.
icon: bolt
color: amber

# Machines this Application claims, by id -- selected from the Workspace's own set above, never
# file paths. A Machine may be claimed by at most one Application; shared ones (the user Machine,
# an activity log) are claimed by none and stay Workspace-level.
machines:
  - mch_bug
  - mch_component

# The Machine whose record count this Application's card on Workspace Home reports.
summary_machine: mch_bug

# This Application's own role vocabulary. Optional -- declare none and the Application simply
# offers no role select. Roles caption the Members list and are what a Permission's `roles:` arm
# names (see section 8); a member holds them directly or through a Group, and either grants
# identically.
roles:
  - triager
  - reporter

navigation:
  - id: nav_bugs
    label: Bugs
    route: /machines/mch_bug
    priority: 1
    home_card: true
```

**Machines are Workspace-level, declared once *per Workspace*.** Several Applications inside one
Workspace genuinely share a Machine (the user Machine is every Application's identity), so
ownership per Application would load the same file twice and produce two Machine objects with one
id. An Application *selects* from its Workspace's set by id.

**Across Workspaces, the same name is free.** Installing an Application copies it into
`metadata/workspaces/<slug>/`, so name your Machines after your own business without checking what
anyone else called theirs -- two Workspaces may each hold an `mch_document` meaning entirely
different things, and either may edit its own copy without touching the other's. Inside one
Workspace an id is still unique, and the runtime tells same-named Machines apart by which
Application claims them (`domain.Machine.ApplicationID`), never by the id alone.

An Application's `id` matches `^app_[a-z][a-z0-9_]*$`. Each entry in `machines:` is a path to a
Machine file, and each entry in `applications:` a path to an Application file, both resolved
relative to the manifest's own directory — order in `machines:` doesn't affect behavior, but
keeping it alphabetical or grouped by Application area helps a human (or an agent) scanning the file. Order in `applications:` *is* the
order the launcher and Workspace Home list them in.

Two Application-level keys worth knowing before you need them: `show_nav: false` suppresses this
Application's persistent menu chrome without making any of its routes unreachable (they stay
reachable from the launcher and from in-page links), and `home_card: true` on **at most one**
navigation item names where this Application's Workspace Home card links to.

**This section described a single inline `application:` block until 2026-09-21** — the shape that
existed before Applications became a list (2026-09-20). Writing that older shape today fails at
startup with "at least one machine is required / at least one application is required", which
names neither key that actually moved; if you hit that error, this is why.

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

**What that generic page draws (2026-10-17).** The Machine's heading, then its **records** in the selected View, and
below them, for a Workspace admin only, the Machine id and the Fields schema table (plumbing a member does not need).
A `status` cell is a badge, toned from the Machine's `status_categories:` when it declares them (`start`/`active` info,
`attention` warn, `done` good, `stopped` bad) and neutral when it does not, so declaring categories improves every
generic list at no cost. It is still a fallback: an Application whose people work from a queue should declare a `page:`
(section 12.1a) and keep the Machine list as a secondary navigation item. Helpdesk, Accounting, Maintenance and Lending
in Workspace `default` are written that way.

**A create Form does not ask for a governed status that has a default (2026-10-17).** If any `transitions:` edge names the
status Field and the Field declares `default:`, a new record starts at that default and leaves it only along the edges, so
the Form omits the select (a borrower cannot apply for a loan that is already `disbursed`). Without a `default:` the
select stays, since nothing else would give the record a status.

## 3. Field types

Each Field declares a `type:`. The full closed set the runtime understands
(`internal/domain.KnownFieldTypes`) — declaring anything else fails validation:

| Type | Use it for | Notes |
|---|---|---|
| `text` | Short free text | Single-line only |
| `long_text` | Free text that keeps its line breaks | A multi-line input; stored as a plain string. No rich text |
| `number` | A numeric value | |
| `boolean` | True/false | Declared and rendered (a checkbox), but no shipped Machine uses one yet |
| `date` | A calendar date | |
| `status` | A closed set of named states | Requires a non-empty `options:` list |
| `person` | "Assigned to a user" | See §4 — implicitly a relation to the built-in user Machine |
| `money` | A figure (an amount of money) | Stored as a plain decimal number, like `number`, and summed, bounded, computed and ordered the same way. **Written everywhere with two decimals and the Workspace's marks** (`number_format:` in its manifest, §12.1): `180,000,000.00` (international, the default) or `180.000.000,00` (`indonesian`). `currency: IDR` puts its symbol before the figure (`Rp 180,000,000.00`). In a form it is typed with those marks (the shell's script groups the thousands as you type -- see "Script is the exception" below) and is sent and stored as the plain decimal |
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
actually exist in the manifest's own `machines:` list also fails (a single Machine file can't check
this on its own — it's checked once every file is loaded).

### 3.0. A limit on a total: `ensure:` (2026-10-16)

A Machine may bound the total of a number Field over its own records, per group:

```yaml
ensure:
  - id: ens_no_negative_stock      # starts with ens_
    sum_of: fld_delta              # a number or money Field of this Machine, added up
    per: fld_item                  # records sharing this Field's value are totalled together
    at_least: 0                    # and/or at_most; zero is a real bound
    reason: Stok tidak boleh negatif.
```

Every create and edit through the generic routes (the form route and the JSON route) is judged as the total it would leave:
an edit replaces the record's stored self, only the group the write touches is read, and a record naming no group is outside
every bound. `per` is a text, status, person or relation Field (never the summed one, never a date or a number), at least one
bound is required and `at_least` may not exceed `at_most`. A Machine the runtime writes itself or a workflow engine's screens
write is refused, since they would skip the check. A `constraints:` block relates two Machines; this is the one-Machine total
it cannot say. **Atomic:** the check and the write run in one transaction holding a lock on each group the write touches, so two simultaneous
writes to one group queue and the second reads what the first committed (2026-10-16). The assistant writes new limits and cannot change or remove one.

### 3.0c. An edge that creates a schedule of records: `generates:` (2026-10-16)

```yaml
transitions:
  - id: trn_disburse
    name: Disburse
    field: fld_status
    from: approved
    to: disbursed
    action: edit
    roles: [loan officer]
    generates:
      machine: mch_installment     # the Machine whose records are created
      count_field: fld_months      # a number Field of THIS record: how many (a whole number, 1 to 120)
      link_field: fld_loan         # a relation Field of the new Machine pointing back at this record
      due_field: fld_due_date      # a date Field: the i-th row is due i x due_every_days from the day the edge is taken
      due_every_days: 30
      amount_field: fld_amount     # a number Field of the new Machine
      amount_from: fld_amount      # a number Field of THIS record, split evenly; the last row takes the rounding remainder
```

The edge writes the status move and, after it, the rows, each running its own create Events. The target Machine's create
Permission is not asked: the edge is the authority. Everything that could make the schedule unwritable refuses the *move*
before anything is written (a count that is not a whole number, an amount that is not a number, a row the target Machine would
refuse), and an edge never generates twice for one record (409). The loader checks both Machines: the link must be a relation
back to this one, the due Field a date, the amount a number, and the target may declare no other required Field without a
default (the runtime could not fill it). Only an `action: edit` edge may declare it. The assistant writes it too (2026-10-16): on a new machine or added to
an installed one, never changed, and a rule naming another machine is checked against the machines of the change and the ones
already installed before anything is published. The move and its rows are one transaction (2026-10-16): a row that cannot be written
takes the move down with it, and two writers cannot both find no schedule and write one. The rows' create Events run after the commit.

### 3.0d. An edge that waits until its lines add up: `balanced:` (2026-10-16)

```yaml
transitions:
  - id: trn_post_entry
    name: Post
    field: fld_status
    from: draft
    to: posted
    action: edit
    roles: [supervisor]
    balanced:
      of: mch_line            # the Machine whose records are totalled
      via: fld_entry          # a relation Field of that Machine pointing back at THIS record
      debit: fld_debit        # two different number Fields of that Machine
      credit: fld_credit
      reason: Debit and credit of this entry do not balance.
```

Before the status moves, the records of `of` that point at this record are read and their debit and credit totals must be
equal to the cent. Nothing to total (no lines, or only zeros) is refused too, since posting nothing is almost always a
mistake, and the refusal names both totals ("debit 500.00, credit 400.00"). A blank or non-number counts as zero. The check
sits with the other write guards, so the form, single-field and JSON routes all apply it, and only a write that takes this
edge reads anything. The loader checks the edge is an `action: edit` one, the target Machine is a Workspace Machine written
by the generic screens, `via` is a relation back to this Machine, and the two totals are different number Fields.
Pair it with `frozen_when:` on the lines (`via` the entry, `equals` the posted status) and a posted entry stays as it was
balanced. The assistant writes it too (2026-10-16), checked the same way, never changed once written. The posting and any write to its lines
(add, change, remove) take the same lock inside a transaction (2026-10-16), so a line added at that very moment waits for the
posting or is read by it. Stated limit: the lock is per entry and per process database, not a row lock on the lines themselves.

### 3.0c. Computed values: one closed expression (2026-10-18, plan A1)

`compute:` takes either the flat `{op, fields}` (an operation over Fields of the record) or **one operation as the key**
with its operands as a list, nested as deep as the value needs:

```yaml
  - { id: fld_net, name: Net, type: money, compute: { subtract: [fld_gross, fld_vat, fld_wht] } }
  - { id: fld_wht, name: WHT, type: money, compute: { round: [{ multiply: [fld_gross, 0.02] }, 0] } }
  # on the contract, from the instalments pointing at it:
  - { id: fld_paid, name: Paid, type: money,
      compute: { sum_children: { machine: mch_contract_term, via: fld_contract, field: fld_gross,
                                 where: { field: fld_term_status, equals: Paid } } } }
  - { id: fld_outstanding, name: Outstanding, type: money, compute: { subtract: [fld_value, fld_paid] } }
  - { id: fld_terms, name: Instalments, type: number, compute: { count_children: { machine: mch_contract_term, via: fld_contract } } }
```

| Operation | Operands | Gives |
|---|---|---|
| `sum` | one or more figures (an empty one counts 0) | a figure |
| `subtract` | two or more: the first less the rest (the first must hold a value) | a figure |
| `multiply`, `min`, `max` | two or more figures | a figure |
| `divide` | exactly two; nothing when the second is 0 | a figure |
| `round` | a figure and, optionally, a literal number of decimals | a figure |
| `add_days`, `add_months` | a date and a figure | a date |
| `sum_children`, `count_children` | `{machine, via, field, where: {field, equals}}`: the records of `machine` whose relation `via` points here | a figure |
| `join` | `{with: " — ", of: [fld_party, fld_term_label]}`: text or status Fields (or `parent:`) written one after another, empty ones skipped | text (on a `text` Field) |

An operand is a Field id of the record, a number, `{parent: fld_contract.fld_value}` (a Field of the record a relation points
at, which must not be computed itself), or another operation. A computed value may read another computed value of the same
record (it is computed after it); two that read each other are refused, naming the ring. **An aggregate over children is
stored on the parent and recomputed in the same transaction as every write of a child** -- create, edit, delete, a rollup or a
scheduled `set_field` -- so a contract's paid figure never disagrees with its instalments. **When a formula changes, every
stored record is recomputed when the Workspace's metadata loads** (start, reload, publish; owner decision K3), and only a
record whose values change is written. `ensure:` may take its bound from the parent too: `at_most: {parent:
fld_contract.fld_value}` (through the relation it groups by, `per:`).

**An inherited Field (2026-10-18)** is `compute: {parent: fld_contract.fld_category}` and nothing else: the Field takes the
related record's value *as it is*, of whatever type, so it is declared as the Field it takes (the same `type:`, the same
`machine:` for a relation, the same `options:` for a status). It exists so a list of instalments or payment plans can show,
search and filter by what belongs to its contract -- the contract's party, number and category -- with the ordinary
`field:` column, `where:` and FilterSelect, no join:

```yaml
# mch_contract_term
- { id: fld_party,    name: Contracted party, type: text,     compute: { parent: fld_contract.fld_party_name } }
- { id: fld_category, name: Category, type: relation, machine: mch_category, compute: { parent: fld_contract.fld_category } }
# mch_payment_plan: through the instalment, which itself inherits -- two hops
- { id: fld_plan_category, name: Category, type: relation, machine: mch_category, compute: { parent: fld_term.fld_category } }
```

Nobody types it (a typed value is overwritten), it cannot be `required:` or carry a `default:`, and when the contract changes
its instalments follow and, through them, their payment plans (`execution.RecomputeChildren` goes down as well as up -- after a
person's edit, a rollup, a scheduled `set_field`, and a figure the runtime rewrote on a parent; only when a Field the children
read changed). The loader refuses a ring (A.x from B.y while B.y reads A.x) and an inherited Field through a relation to its own
Machine. **Stated limits:** an edit of an instalment and of its contract at the same moment can deadlock on each other's rows
(Postgres aborts one: that request answers 500 and can be repeated); a parent that is deleted leaves its children's inherited
values as they were; a parent edit with thousands of children does one read and one locked write per child that changes.

What it does not do, stated: a value that is not a number (an overflow, a rounding past 10 places) is no value; a figure
stored as text is read as a figure; a month added to the 31st lands on the month's last day, and a count of days or months
must be whole. A record whose computed value changes because a child or a parent changed is **written by the runtime**: its
`locked_when`, `frozen_when` and Events do not judge that write (as they do not judge a rollup). A child written while its
parent is being recomputed waits on the parent's row, so neither write undoes the other. Lowering a contract's value below
what its instalments already add up to is not refused (the bound is judged on a child's write). A Machine may not add up
its own records. Every load recomputes every stored record of a Machine with a computed Field, writing only the ones that
change. The assistant writes the flat shape only (`round`, `sum_children`, `count_children` and `parent:` are written by
hand); an update leaves a tree-shaped compute and a parent bound as they are written.

### 3.0b. A date from a date, and a Field an edge writes from the clock (2026-10-16)

`compute:` gained a second operation. `op: add_days` is a **date** Field computed as a date Field plus a number Field of days:

```yaml
  - id: fld_following_due
    name: Next due
    type: date
    compute: { op: add_days, fields: [fld_last_done, fld_frequency_days] }   # [a date field, a number field], in that order
```

It is derived on every generic write, empty until both operands hold a value, and never an input (like `sum`). And an edge of
`transitions:` may write Fields from the clock when it is taken:

```yaml
transitions:
  - { id: trn_complete, name: Complete, field: fld_status, from: scheduled, to: done, action: edit,
      roles: [technician], sets: { fld_last_done: today } }
```

`today` is the only source (a closed set); the target must be a `date` Field that is not computed or stamped, and only an edge
with `action: edit` may declare it. The write overrides whatever the form carried, and it happens only when the write takes that
edge, so an edit that leaves the status alone cannot rewrite the date. It runs before computed Fields are derived, which is why
the example above gives the next due date from today. The assistant writes both.

### 3.0a. A record frozen by its parent: `frozen_when:` (2026-10-16)

```yaml
frozen_when:
  via: fld_period        # a relation Field of this Machine: the record this one sits under
  field: fld_status      # a status Field of the Machine via points at
  equals: closed         # one of its options
```

While the record `via` points at holds that value, a record of this Machine is not edited, not deleted, and none is created
under it (HTTP 409). Moving a record into a frozen parent is an edit and is refused too, and so is moving one out of it (it is
frozen by where it sits now). A parent that no longer exists freezes nothing. It is checked by the generic create, update and
delete routes, so a Machine a workflow engine or the runtime writes itself is refused at load, and `equals` must be one of the
related status Field's own options (a value it can never hold would freeze nothing). The assistant adds it to a new Machine
and to an existing one without it, and never changes or removes one.

### 3.0b. What may change and what may be deleted: `locked_when`, `set_once`, `on_parent_delete`, `deletable_unless` (2026-10-17)

Some data should be easy to edit and delete row by row; some should rarely change, should not change, or must not be
deleted while something holds (owner decision K1). Four declarations say which, and the write routes enforce them:

```yaml
fields:
  - { id: fld_contract_no, name: Contract no., type: text, set_once: true }        # never changes once it has a value
  - { id: fld_gross, name: Gross, type: money,
      locked_when: { field: fld_term_status, equals: Paid } }                      # stays as it was paid
  - { id: fld_contract, name: Contract, type: relation, machine: mch_contract,
      on_parent_delete: restrict }                                                 # the contract stays while this points at it
deletable_unless:
  - { field: fld_term_status, equals: Paid, reason: A paid instalment is the record of a payment. }
```

- `set_once` and `locked_when` (a status Field of the same Machine and one of its options) are judged against the record **as
  stored**: an update that changes the Field is refused (422) with the reason; one that sends it back unchanged is not. Every
  edit form shows a locked Field as text instead of an input. Not on a computed or stamped Field.
- `on_parent_delete: restrict` (a relation Field only) refuses deleting the record it points at while any record points at
  it (409: "cannot be deleted while 3 Instalment record(s) point at it"). `cascade` and `clear` are not built and are refused.
- `deletable_unless` refuses deleting a record while its status Field holds the value, with the reason given.
- A locked Field a whole-record update leaves out **keeps its stored value**; a figure written another way ("54000.00" for
  54000) is no change. A boolean cannot be locked (an unticked box sends nothing).
- These are rules for what a **person** writes through the generic routes. What the runtime writes itself -- an Event's
  `set_field` or rollup, a workflow engine's own screens -- is not judged by them, as it is not judged by `required_when`.
- `optional: true` is refused inside an `any:` group that also holds a required comparison (a search would widen the list
  past it), and on a parameter the runtime sets as a scope (`application`, `id`, `week`). A Table column may not name a
  `file` Field (its value is a storage key).
- **A button is offered only where the route would take the write**: a page's Delete and Edit read the same rules (and
  `frozen_when:`) for the listed records, in one statement per related Machine, so a contract with instalments shows no
  Delete. The routes check again regardless.

### 3.1. Required only under a condition, and a date derived from a status (2026-10-15)

```yaml
  - id: fld_format
    name: Format
    type: status
    options: [poster, banner_2_1]
  - id: fld_reference_file
    name: Reference file
    type: file
    required_when: { field: fld_format, equals: banner_2_1 }   # refused on any write while this holds and the field is empty
  - id: fld_due
    name: Due date
    type: date
    default_from:                                              # on create, when left empty: today + days
      field: fld_priority
      days: { tinggi: 1, normal: 3, rendah: 7 }
```

`required_when` compares one **status** Field of the same Machine with one of its own options (a value it can never hold
would require nothing, so it is a load error), and cannot sit beside `required:`. `default_from` is for a `date` Field,
reads a status Field's value, and takes only non-negative day counts for options that exist; a value it has no entry for
derives nothing, and a date the submitter chose is kept. It does not combine with `default`, `compute` or `stamp`, and
both are refused on a Machine that a workflow engine's own screens write. The assistant writes both on a NEW field, and
cannot change them on an existing one.

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

The default, with no `views:` block at all, is a flat table. Two ways to get a board instead:

**Fixed columns, grouped by a `status` field's own options:**

```yaml
views:
  - id: vw_bug_status_board
    name: By status
    type: board
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
views:
  - id: vw_bug_board
    name: Board
    type: board
    group_by: fld_list   # a `relation` field on this Machine, pointing at mch_list
```

A Machine declares its arrangements under `views:`, each with an `id` (`vw_...`, unique within the
Machine), a `name` (what a viewer reads when choosing between them) and a `type`. A screen selects
one with `?view=vw_bug_board`; leaving it off renders the first declared; an id the Machine does
not declare is a 404, not a silent fallback. A Machine declaring no `views:` at all renders one
plain table, exactly as before `views:` existed.

`type` must be one of `table`, `board` or `cards` (`internal/domain.KnownViewKinds`). `group_by` is
meaningful only on a `board`, must name a real field on the same Machine, and is *rejected* on any
other type rather than ignored — a declaration the runtime silently drops reads as though it were
working. `cards` requires its Machine to declare `card_fields` (§7.1), since a cards view over no
projection would render empty cards forever.

**What a `board` View draws (2026-10-05, Case 19 PM01).** Not the generic Machine page: a heading, a
"N cards · grouped by <Field name>" summary computed from the records and `group_by`, the View switcher,
a "New <machine name>" button, and one column per group with a count, a card per record and an "Add a
card" composer that pre-fills the column's group. A record with no group lands in an automatic "Other"
column. The heading is the `title:`/`label:` of the navigation item whose route is `/machines/<id>`, so
rename the screen in the Application's `navigation:`, not in Go. A card shows what `card_fields` declares:
`title`, a `date` pill and a `person` avatar (initials) — declare `{ field: fld_assignee, role: person }`
to get the avatar. A Machine that declares `type: table` beside the board gets the segmented Board/Table
switcher; the switcher's look changed for **every** Machine with more than one View.

**A board speaks of its records by the Machine's own name (2026-10-19, slice 3, mockup PM01).** "3 tasks", "Add task", "Task title" and "Tasks with
no list land here." are the Machine's `name:` lower-cased with an s -- nothing is typed; a name that does not take an s says "entrys", so give the Machine
one that does. The column for records with no list is drawn as the board's leftover: dashed, on the page's ground, with no composer and no menu. Columns
are 162px wide, so seven fit a 1180px page. A board that is one screen of a **view group** makes a task started from "New task" or from a column's
composer belong to the project that is showing (the group's scope value is a hidden input / the form's starting value).

**A column menu and an "add list" tile are declared on the View, for a board whose columns are records of another Machine** (a List):

```yaml
views:
  - id: vw_task_board
    name: Board
    type: board
    group_by: fld_list                 # a relation: each column is one record of mch_list
    column_actions: [rename, move, delete]   # the "..." menu of a column; each is the List Machine's own generic record route
    add_column:                        # the dashed tile at the end of the board, for the place lists are made
      to: nav_board_settings           # a navigation item (never a route) of an Application that installs this Machine
      label: Add list
      note: Lists are shared by every project in this workspace. Opens Board settings, where they are renamed, reordered and removed.
```

`rename` writes the List's label Field (the `edit` Permission), `move` moves it one place earlier or later in its Machine's order -- which *is* the order of
the columns -- (`edit`; the first column has no "Move left", the last no "Move right"), `delete` removes it after a question that says what becomes of what is in the list ("Delete this list? Its 3 tasks move to Other. This cannot be undone."; where
the board's relation declares `on_parent_delete: restrict`: "It can only be deleted while no tasks are in it."). Each is offered only where the List Machine's own
Permissions let the viewer -- resolved the way the routes resolve them, parents included -- and the menu asks the record routes to answer with a refresh
(`X-Answer-With-Refresh`, put on the menu by `hx-headers`); every other caller keeps the empty answer its own `hx-target` acts on. A refusal's reason is the
shell's one notice. A name of only spaces is refused by the Data Plane like an empty one (a required text), and a long one wraps in its column.
The loader refuses `column_actions`/`add_column` on a board whose `group_by` is not a relation, an action it does not know, and an `add_column.to` that is not
a navigation item of the Application. **The AI assistant has no slot for either key** (`registry.Capabilities` says why: a generated board groups by a status
Field's options, which are not records).

**A calendar of weeks and months (2026-10-19, mockup PM05).** The window is `$week`..`$week+6` or, new, `$month`..`$month_end` (the first and last
day of the month `?month=N` selects, N whole months from the one holding today; no offsets, a month's length is not a number of days); `?range=week|month`
chooses the grain and a request that sends neither, or nonsense, is the week. A page tests it with `when: {parameter: range, is: month}` (also `week` and
`month` as whole numbers), steps it with `step: {parameter: month, by: 1}` and, so a step or a "This month" link stays a month, `set: {range: month}` on the
link (the one parameter a link sets, from a closed set). `component: SegmentedControl` with `query: range` is the Week | Month switch (its state is the
request's; the other parameters, the project among them, stay in each link). A grouped Collection takes `binding: {group: date, fill: weeks}` to draw
whole Monday-to-Sunday weeks around the window (the added days are quiet, show their date alone -- they were not asked what is due -- take no drop, and are not drawn on a phone) and `moves: true` to make each card the viewer may
edit draggable to another day -- a drop, or Space then the arrows then Space on its title, writes the record's own date Field through the generic patch
route (a named script exception, below; after a keyboard move the page reloads and the moved card has focus again). A date Field holds a day, `YYYY-MM-DD`: the Data Plane refuses any other value on a create, an update or a drag (an update judges only a date it *changes*: a record that already holds a value from before the rule can still have its other Fields edited, and the date cleared or corrected), and a record whose stored date is no day of the window (a value from before that check) is left out of the days and counted in one notice under the grid, which also says when the Dataset's `limit:` cut the window and from which day cards may be missing. Its heading template reads `bucket:label`, or the parts `bucket:day` ("Mon"), `bucket:date` ("05 Oct"),
`bucket:number` ("5"). A `static: link` may be `variant: quiet` (the secondary ink, picked for a finished record with `when:`), `variant: plain` (2026-10-20: the strong ink, the accent only on hover -- what a list of records is read for, a Table's first column or a calendar card's title; a column of blue links reads as a column of actions), `variant: chip` (2026-10-22: the bordered pill of a filter chip, for suggestions that each start something; best inside a Collection with `inline: true`), `icon:` with no `variant:` (the mark -- `settings` today, plus the two chevrons -- before the link's words, as the gear of "Workspace settings"; with a button variant the mark still replaces the words), `variant: row` (2026-10-22: `plain`'s ink, and the link's hit area is the whole row around it -- the nearest positioned ancestor, which a divided Collection's item is -- so a list of things that each open is one target a row and what else the row holds is not a second one; Workspace Home's Applications), a `layout: row` may declare `tracks:` (2026-10-22: one word for each child, in order -- `fill` takes what is left and may shrink, `fit` is as wide as what it holds, `field` is a fixed column for one short labelled value, `badge` a fixed column whose content sits at its end; the two fixed ones are fixed from `sm` up; on a phone declare `wrap: nowrap` -- the row stays one line and the badge keeps its own width; `columns` takes `tracks:` too, so a name and its `field` set in `columns` sit side by side from `sm` up and stack on a phone, the role under the name -- so the rows of a list share their columns; a child a `when:` hides still holds its column, so the next one does not slide left, and a count that is not the children's is a load error) and `mark: chevron` (the disclosure chevron after its last child: the row opens something), and may be `wrap: nowrap` (its children stay on one line, so a circle keeps its place beside a title that wraps within its own column -- the calendar card), a `layout: panel` may be `density: compact` (the small 10px-by-8px card a calendar day and a board column hold, the meta size for its words; the board's own card reads the same recipe) or, as a button, carry
`icon: chevron-left|chevron-right` in place of its words (the words stay its accessible name). The properties lowering alone produces -- a section's
`outside`, `dropField`, `dropValue` and a stack's `move` -- are never written.

**Moving a card from the keyboard (2026-10-19, mockup PM01b):** Space on a focused card lifts it, the arrow keys choose the list and the place, Space drops it and
Esc puts it back; the board's status line says where it is going. It ends in the very PATCH a drop sends, so there is one contract. It is part of the board's
drag script in `machine.templ` (see "Script is the exception").
**A mouse drag is drawn as PM01b shows it (2026-10-19):** the card under the pointer is a tilted copy with a shadow (the browser draws a picture of the
element, so the script hands it a copy that has both), the place it came from collapses to a dashed "moving…" gap, the list it is over shows its count going
up ("4 → 5"), is ringed, and gets the green bar where the card will land, and the board's status line says "Moving “…” from A to B, as the first card.
Release to drop, Esc cancels." -- the same line the keyboard move writes. Nothing is moved in the page until the PATCH answers.

**Moving a card needs nothing declared.** Anyone who may edit a record can drag its card to another column or
place in the same one, or use the pencil's *Move to list* / *Position*; both write the `group_by` Field and the
card's order (`sort_order`). Nothing can be moved into the automatic "Other" column — it has no value to write.
The same move is offered as a **Move…** panel in the ⋯ menu of the record's own page, with nothing more to declare:
any Machine that has a `board` View gets it, and it writes through that View. That page also draws a breadcrumb
(the Machine's heading, linking to the board, then the card's list) from the same View.

The menu's **Copy…** is just as undeclared: any Machine with a `title` card role, a `create` Permission the actor
passes, and that is neither `append_only` nor cast in a workflow role gets it. It names the copy (default
"<title> (copy)"), writes it through the same create pipeline as `POST /machines/{id}/records` (defaults, computed
Fields, stamps, the Permission, validation, `on_create` events) and opens it. Computed, stamped and `file` Fields are
not carried over, and neither are child records (checklist, comments, attachments).

Write the `name` yourself; don't expect the type to supply it. `table`/`cards` is the runtime
engine's own vocabulary, and a string the user reads is the Application author's to write — the
same separation `name:` already has on a Machine and on a Field.

## 7. SLA badges

A `date` field can be flagged for SLA rendering — it then shows as `OVERDUE` or "N day(s) left"
instead of a plain date, anywhere that field is displayed:

```yaml
sla_field: fld_due_date
```

`sla_field` must name a real field on the same Machine, and that field must be `type: date` —
validated the same way as `group_by`. This is fully generic: any Machine can declare it, not just
the one that happens to ship with it today.

Note where it sits: at the top level of the Machine, **not** inside a View. It describes how this
Machine's records look *wherever* they appear, and the record detail page selects no View at all —
putting it on one arrangement would have forced an arbitrary pick. The same reasoning applies to
`card_fields` below.

## 7.05. Completion — which status value means "finished"

```yaml
completion: { field: fld_status, done: done }
```

`field` must be a `status` Field of this Machine and `done` one of its `options:` — both checked at
load. Nothing else in a Machine says which status is terminal (the last option is a convention, and
a Machine may list `archived` after `done`), so a screen that wants to draw a record as finished reads
this. Today that is a board card's date pill (grey ahead, red once past, green with a check when
finished — a finished card is never overdue) and its **completion circle**: hovering a card shows a
circle that writes `done` into `field`, and on a finished card writes it back to the Field's `default:`
(its first option when it declares none), so give the Field a `default` that means "not started". The
circle and the quick-edit pencil appear only for someone the Machine's `edit` Permission allows. Optional: a Machine that declares none simply has no
finished records. Like `sla_field`, it sits at the top level of the Machine, not inside a View.

## 7.06. Card tags — the chips on a board card

```yaml
# on mch_task
card_tags: { machine: mch_card_label, via: fld_task, tag: fld_label }
```

`machine` is the join Machine (one record per task–label pairing), `via` the reference Field on it that
points back at this Machine, and `tag` the reference Field that names the label. All three are checked at
load, across Machines, so a `via` that does not point back or a tag Machine with no name fails to start
instead of drawing a board with no chips.

What a chip looks like is declared **on the tag Machine**, once, in its own `card_fields`:

```yaml
# on mch_label
card_fields:
  - { field: fld_name, role: title }
  - { field: fld_color, role: color }
```

A `color` Field must be a `status` Field whose `options:` are all in the closed palette
(`blue, purple, amber, slate, emerald, cyan, rose`) — you choose a name, never a colour value, so a label
reads the same on every board. Only a `board` View draws chips; the optional block costs nothing when absent.

## 7.1. Card fields — what a record shows when it renders as a card

```yaml
card_fields:
  - { field: fld_document_type, role: status }
  - { field: fld_status, role: status }
  - { field: fld_due_date, role: date }
```

Each entry names one of this Machine's own Fields plus the semantic **role** it renders as —
`title`, `person`, `money`, `status`, `date`, `color`, `file`, `comment` or `container` (`internal/domain.KnownCardFieldRoles`; `container` on a `relation` Field naming the record this one sits inside, a Task's Project, drawn by that record's own title; `color` only on a tag Machine, see 7.06; `file` on a `file` Field of a child Machine, which draws it as an attachments list; `comment` on a `long_text` Field of a child Machine that also has a `stamp: current_user` person Field, which draws the Machine as the record's "Comments and activity" feed — copy `metadata/comment.yaml`). The
renderer picks markup by role, not by the Field's storage type, so the same declaration works
whether the underlying Field is plain text or a relation.

This is what a `type: cards` View renders, and what `internal/composition.ProjectCardFields`
resolves for a composed card. Changing a row here changes what every card shows with no Go touched
— which is the whole point, and the reason a cards View is refused on a Machine that declares none.

## 8. Actions and Permissions — read this before you assume something works

This is the section most likely to trip up an agent generating metadata for a new business
process, because CRUD's genuine flexibility makes it tempting to assume behavior is equally
open-ended. It is not, and the runtime's own source says so explicitly.

**The runtime realizes exactly four Actions: `decide`, `create`, `edit` and `delete`.**
`internal/domain.KnownActions` is a closed map holding those four and nothing else. Declaring a
Permission with any other `action:` value fails metadata validation outright ("action %q is not an
action this runtime realizes"). There is no way to declare a brand-new Action — "Submit,"
"Publish," "Cancel," anything — purely in YAML. Doing that requires real Go code: a new entry in
`internal/domain.KnownActions`, a new handler, and whatever business logic that Action performs.
This is the one place "no per-model code required" (the very first line of this guide) does not
hold.

Of those four, **only `decide` is bound to specific Machines**; `create`, `edit` and `delete`
gate the generic create/update/delete routes and work on any Machine (see "What a Permission
*can* do today" below). This paragraph read "exactly one Action" until 2026-09-21 — wrong since
`edit`/`delete` were generalized on 2026-09-19, and contradicted three paragraphs further down in
this same section. `create` was added later the same day: it had been left out because a
record-scoped rule needs a record and creation has none, which stopped being the whole story once
a Permission could gate on a role, and had meanwhile left creation as the one write path in this
runtime with no authorization check of any kind on any Machine.

**And `decide` itself is hardcoded to one specific pair of Machines**, by explicit design, not
oversight — `internal/action/decide.go`'s own comment: *"This is hardcoded to
mch_document/mch_approval_step's own field ids, not a generic [mechanism]."* The
`POST /machines/{machineID}/records/{id}/decide` route exists for any Machine id path-wise, but
the handler does nothing for any Machine other than Document Approval's own Approval Step.
Declaring `permissions: - action: decide` on your own new Machine does not give it an approval
workflow — it declares a Permission the runtime cannot actually invoke for that Machine.

**Naming your own Machine `mch_approval_step` does not borrow that engine either**, and since
2026-09-27 it does not break anything by trying: the handler asks `action.IsStep`, which checks
which *Application* claims the Machine (`domain.Machine.ApplicationID`) as well as its id, so
another Workspace's own Machine of the same name is simply not Document Approval's. It used to
compare the bare id, which was sound only while ids were unique across the whole process — see
CLAUDE.md's "One manifest per Workspace" and
`internal/conformance.TestNoBareMachineIDIdentityChecks`.

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

**A Permission can also gate on an Application role** (`roles:`, added 2026-09-21). The acting
person must hold at least one of the named roles in the Application that claims this Machine:

```yaml
permissions:
  - id: prm_decide_own_step
    action: decide
    roles: [approver, submitter]   # hold EITHER role...
    actor_field: fld_assignee      # ...AND be the person this record names
```

Three rules worth knowing before you write one:

- **Roles within one Permission are alternatives; Permissions on one Action are requirements.**
  The example above passes for someone holding `approver` *or* `submitter`. Splitting them into
  two Permissions would mean holding *both*, which is almost never what you want.
- **The words come from that Application's own `roles:` vocabulary** (§2), not a Workspace-wide
  one. A role the claiming Application does not declare fails the load — it could never be held,
  so the Permission would deny everyone while reading like a grant. So does a `roles:` on a
  Machine no Application claims (`mch_user`, `mch_activity`): there is no vocabulary to resolve
  it against.
- **A role held through a Group counts identically to a direct one.** What is checked is the
  member's *effective* roles — their direct assignment plus every role any Group they belong to
  holds there — so granting a role to a Group grants it to everyone in it, with no Document
  touched.

`roles:` alone, with no `actor_field`, is a valid Permission: "anyone holding this role may, on
any record." What is *not* valid is a Permission that names no arm at all — it would gate on
nothing.

**`create` is the one Action where `actor_field` reads differently, and deliberately so.** At
creation there is no stored record, so what gets checked is what the request is *submitting* —
which makes `actor_field` mean "the value you submit for this field must be you":

```yaml
permissions:
  - id: prm_create_own_signature
    action: create
    actor_field: fld_owner      # you may only create a signature that names YOU as its owner
```

That is a rule about the record you may write, not one you may reach, and it is what stops a
record being created in somebody else's name. Before it existed, any member could create a
signature record naming anyone as owner — and `fld_owner` is what selects whose signature gets
composited onto an approved PDF.

**`parent_actor` is the arm that reads a record this one points at** (K21). "Only this Document's submitter
may add a step to it" is a rule about the *parent*, declared on the child:

```yaml
permissions:
  - id: prm_create_step
    action: create
    roles: [approver, submitter]
    parent_actor:
      via: fld_document            # a relation field of THIS Machine; its value is the parent's id
      field: fld_submitted_by      # a person field on the Machine `via` points at
```

The acting person must be the value of `field` on that parent, in addition to every other arm of the same
Permission (it is a requirement like `roles` and `actor_field`, never an alternative to them). At `create` the
parent id is read from what the request submits. Both names are checked at load: `via` must be a relation of
this Machine and `field` a `person` Field of its target. It can only *restrict* — a screen that has not been
taught to read the parent (a hand-written button, a `.templ`) refuses rather than allows, so a missing
button is the symptom of a call site not resolving parents, not of a rule failing open. The generic
create, edit and delete routes resolve parents; the approval engine's own `decide`/`revise` do not yet.

**A Permission can also require a *Workspace* role**, which is a different namespace from an
Application's `roles:`:

```yaml
permissions:
  - id: prm_edit_user_is_admin
    action: edit
    workspace_role: admin       # administering the Workspace, not a role any Application declares
```

`admin` is the only value (`member` would be a rule every member passes and every admin fails).
It is a separate key rather than a word inside `roles:` because the two namespaces genuinely
overlap: an Application may declare `admin` in its own vocabulary — `ui-sample/member-role-detail.
html` shows exactly that for HR — and a single list would make `roles: [admin, approver]` read as
one alternation across two different meanings.

**Holding `admin` is not a bypass.** It satisfies a Permission that *asks* for it and changes
nothing about any Permission that does not. This is a decision, held by
`internal/conformance.TestWorkspaceAdminIsNotAPermissionBypass` because it is invisible in the
code — there is no bypass branch to read, so nothing would otherwise mark its absence as
deliberate. An administrator who can approve a document they are not an approver of is the audit
hole an approval flow exists to close.

**What every Machine gets for free regardless, with no Permission declared at all:** any
authenticated member of the Workspace can create, edit, and delete its records through the
generic routes — declaring no Permission means unrestricted, not "nobody can." There is still no
per-Field permission (see "What this can't do yet" below), and record creation has no Permission
of its own yet (there is no existing record to match an `actor_field` against at that point).

## 7.2. Declaring `roles:` also closes the Application

An Application that declares a `roles:` vocabulary (§2) is **gated on holding one of them**:
someone with no role there cannot open a single one of its screens, reading included. An
Application that declares none stays fully open, because requiring a role nobody can hold would
deny everyone.

That is not a separate key — it falls out of the vocabulary existing
(`internal/web.requireApplicationAccess`). It is worth knowing before you add `roles:` to an
Application that did not have them: the moment you do, every existing member without an assigned
role loses access to it.

It is coarse on purpose. There is no `read` Action in this runtime, so what can be said is "in or
out of this Application", never "may see Documents but not Approval Steps". The finer shape
(upstream's per-role `can_read`) is in `ROADMAP.md`, unbuilt.

## 8.0. `append_only`: a Machine whose records are never changed

```yaml
append_only: true
```

A Machine-level key, not a Permission: every update and delete refuses, for everyone, including a
Workspace admin. Records can still be created — that is what the runtime itself does for an audit
trail.

It is a Machine property rather than a Permission because a Permission answers "which actor may",
and here the answer is that there is no such actor. Saying it as a Permission would mean naming a
role nobody can hold, which is a rule that reads as a grant and denies everyone — a shape this
runtime refuses at load elsewhere. Declaring `append_only` alongside an `edit` or `delete`
Permission is therefore a load-time error: one of the two can never fire.

`mch_activity` is the whole of it today. Until 2026-09-21 it declared nothing, which in this
runtime means unrestricted — so any member could edit the summary of an approval they did not
make, or delete the row recording it, through the ordinary CRUD screens.

## 8.1. Transitions: which moves of a status Field exist, and who performs them

A `status` Field lists its legal *values* (§3). `transitions:` lists the legal *moves* between
them, and which Action is allowed to make each one:

```yaml
transitions:
  - id: trn_step_approve
    name: Approve            # what this move is called in the business -- required
    field: fld_decision      # a status Field on this Machine
    from: pending            # both must be options that Field declares
    to: approved
    action: decide           # one of decide/edit/delete, or omitted (see below)
```

**Opt-in, per Field.** A status Field that no transition mentions still moves freely — declaring
the state model of one Field does not freeze the others. But once a Field *is* mentioned, every
move of it must be declared, or the write is refused (`422`).

**Omitting `action:` means "the runtime performs this itself."** That is not the same as leaving
the edge out. `mch_document`'s `fld_status` is derived — an Event rolls it up from the Document's
own Approval Steps — so all six of its edges declare no action, which is what refuses a person
setting the status by hand through the ordinary edit form while leaving the rollup free to write
it. Leaving those edges undeclared instead would make the rollup's own result look like an
undeclared move.

**Naming a different Action than the route performing the write refuses it.** `action: decide`
above is why an Approval Step's decision cannot be changed through the generic edit form: the
edit route realizes `edit`, and this edge is reserved for `decide`. Before transitions existed
this was one hand-written Go rule naming this exact Machine and Field.

**Who may take an edge: `roles:` and `actor_field:` (2026-10-15).** An edge performed by `edit` may say who takes it,
using the same two arms a Permission has:

```yaml
transitions:
  - { id: trn_approve, name: Setujui, field: fld_status, from: diajukan, to: disetujui, action: edit,
      roles: [manajer], actor_field: fld_manager }
  - { id: trn_cancel, name: Batalkan, field: fld_status, from: diajukan, to: dibatalkan, action: edit,
      actor_field: fld_employee }
```

`roles:` are Application roles, any one of which suffices and each of which the Application must declare.
`actor_field:` is a person Field of this Machine: only the person the record names in it may take the edge (a record
whose field is empty can be taken by no one on that edge). Both together must both hold. A Permission on `edit` still
decides who may edit the record at all; the edge's arms narrow that for one move, so a manager can edit a leave
request without being able to cancel it. The key is refused on an edge whose `action:` is not `edit`, because only the
generic update route enforces it. A page's `transition` Button is drawn only for a viewer who passes it, and the
assistant can write both keys on a new status move (it cannot change them on an existing move).

**Nothing may leave a value with no outgoing edge.** The two edges above both start at `pending`
and none leaves `approved` or `rejected`, so a decision is final — and the Review screen reads
the same declaration to decide whether to draw the Approve/Reject bar at all, rather than
comparing against the literal `pending`.

The **Authorization Matrix** (`/authorization-matrix`, Workspace admins) draws every one of these
declarations in two sections — what the Workspace decides, then one block per Application — so
"what may this role do here" is a page rather than a walk through every Machine's `permissions:`
block. It renders the
same declarations the server enforces and has no permission model of its own. **Read it after
writing a `permissions:` block**: an action nobody governs and an action everybody is granted look
identical in a YAML file and are labelled differently there.

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

A placeholder for a `relation` Field reads as the related record's own title — its Machine's
`card_fields:` `title` role — not its id, so `on: fld_list` with `moved from {old} to {new}` logs
"moved from Backlog to Pre-production" (`mch_task`'s `evt_task_list_changed`). A record that no longer
resolves, or a Machine with no `title` role, falls back to the stored id; an unset relation reads "no value".

Two things this is *not*: leaving `when_equals` off (the field above doesn't set one) means "any
change fires it" — unlike Constraint, where `when_equals` is required, because a Constraint gates
one specific transition while an Event merely observes one. And `summary_override`/
`summary_override_when` support **at most one** override, not arbitrary per-value branching —
if your wording needs more than "a default, and one exception," that's not expressible here yet.

`service: log_activity` is the one Service this runtime realizes today
(`internal/registry.Services`) — the same closed-set discipline `action:` uses for Permission.
There is no way to declare a new Service purely in YAML, the same limit §8 already describes for
Action.

An event is stamped with the Application that claims its Machine (`fld_application_id` on
`mch_activity`), so a feed can declare
`where: { field: fld_application_id, op: equals, value: $parameters.application }` and show only the
Application the request is in. A Machine no Application claims logs without one, and such an event
appears in no feed.

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

A third shape fires on the passage of time rather than a write — `schedule:` in place of
`on`/`on_create`, evaluated periodically (`internal/execution.RunScheduledEvents`, on a ticker,
not inside a request) against every record of the declaring Machine:

```yaml
events:
  - id: evt_document_overdue_notify
    schedule:
      date_field: fld_due_date
      when: overdue
      guard_field: fld_status
      guard_equals: in_review
    then:
      service: send_notification
      recipient_field: fld_submitted_by
      preference_key: sla_breach
      summary: "Your submitted document is overdue for approval"
```

`date_field` must be a real `date` Field; `when` is closed vocabulary (only `overdue` exists
today); `guard_field`/`guard_equals` are optional but must be set together, and generalize
`on`/`when_equals`'s own field-equality shape rather than inventing a second one — "overdue AND
still in_review" is declarative, not a special case for one Machine. This closed the shape's own
long-standing note that it was "the one deferred shape, waiting on their own second real case"
(the SLA-breach reminder, replacing `internal/composition`'s own read-triggered
`logSLABreaches`, 2026-09-27).

**The other polarity, and writing a Field on a schedule (2026-10-16).** `guard_not_equals` is the guard
turned round ("overdue and NOT resolved"); declare one of `guard_equals`/`guard_not_equals`. The
`set_field` Service writes one option into a status Field of the record when a schedule fires —
escalation:

```yaml
  - id: evt_ticket_overdue_log            # the scheduler fires once per record; this line is its marker
    schedule: { date_field: fld_due, when: overdue, guard_field: fld_status, guard_not_equals: resolved }
    then: { service: log_activity, summary: "Ticket past its SLA: {fld_title}" }
  - id: evt_ticket_escalate
    schedule: { date_field: fld_due, when: overdue, guard_field: fld_status, guard_not_equals: resolved }
    then: { service: set_field, target_field: fld_priority, value: urgent }
```

`set_field` is valid only on a `schedule:` Event (on a write it would re-enter that write's own events),
its `target_field` must be a status Field and `value` one of its options. A record already holding the
value is left alone; a move the Machine's `transitions:` refuse is skipped (the runtime performs it, so
it needs an edge declared with no `action:`). After the write, the record's field-change Events run, so a
notification declared on that Field fires as for a person's edit. Keep `{fld_*}` placeholders in the
marker's summary to Fields `set_field` does not write, or the marker changes and the Event fires again.

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

**A key that is not in these tables is refused at load, since 2026-09-28.** A misspelled or retired one
(`view:` where `views:` is meant, `sla_filed:` for `sla_field:`) used to load without complaint and the
capability simply never appeared -- the one failure shape that looked exactly like success. Now it fails
with the key, the kind of document, and the line number. So these tables are the vocabulary, not a
summary of it: if you need a key that is not here, the runtime cannot read it, and adding it is Go work.

Sections 1–11 teach by example. This section is the index: **every key the parser accepts, and
every value a closed vocabulary allows.** Anything not listed here either fails validation at
startup or is silently ignored — there is no third outcome, and nothing is inferred from a key the
parser doesn't know.

Three properties are worth stating once, because they hold everywhere below: ids are regex-gated
by prefix; closed vocabularies are extended by Go code and a recompile, never by writing a new
value in YAML; and every cross-reference (`fld_*` naming a field, `mch_*` naming a Machine) is
resolved and checked at load time, not at first use.

### 12.1 `metadata/workspaces/<slug>.yaml` — one installation manifest per Workspace, and one file per Application

**`metadata/workspaces/<slug>.yaml` — a Workspace's installation manifest**

Since 2026-09-22 there is one per Workspace, named by slug, discovered by scanning the directory:
dropping a file in *installs* its Applications into that Workspace. `applications: []` is valid.
Creating a Workspace through the UI writes this file for you, with the three shared Machines and
`applications: []` (2026-09-30); a Workspace missing one is given it on its first install or publish.

| Key | Value | Notes |
|---|---|---|
| `workspace` | slug string | Which Workspace this manifest installs into. A slug, not an id: the `ws_...` id is generated when the Workspace is created through the UI. The nested `workspace:` block this key replaced (carrying `id`/`name`/`machines`/`navigation`) stopped loading on 2026-09-22 |
| `machines[]` | file paths | Load order; each file is one Machine, and this Workspace's own. A path under `<slug>/` is its own copy; a path outside is one of the three runtime-level Machines every Workspace shares |
| `navigation[]` | list of items | Destinations belonging to no single Application (Home, All Machines, Members, Groups) |
| `applications[]` | file paths | One file per Application; declaration order is launcher/Workspace Home order |
| `suggested_applications[]` | list of `{label, prompt}` | Workspace Home's "Add an application" chips (2026-09-27); both required. Display text and a conversation-starter prompt for `/new-application?idea=`, not an identifier anything else references — a Workspace with none declared simply shows no chips |
| `number_format` | `international` \| `indonesian` | How **every** screen of this Workspace writes a figure (2026-10-18): the mark between thousands and the mark before the decimals. `international` is `1,234.50` and is what a manifest that says nothing uses; `indonesian` is `1.234,50`. One choice for the whole Workspace, not per Application or per Field: a figure reads the same wherever it is met. Applies to money in tables, totals, metrics, a record's page, a printed PDF, the money control of a form, and the figures a document reader proposes. An unknown name is refused when the manifest loads. A plain `number` Field is not affected (it is written as stored). Not writable by the AI assistant yet |

**One Application file** (`applications/<name>.yaml`)

| Key | Value | Notes |
|---|---|---|
| `id` / `name` | `app_*` / string | |
| `description` / `icon` / `color` | string / closed set / closed set | The Application's card face on Workspace Home. `icon` ∈ `domain.KnownIcons` (see below); `color` ∈ `blue`, `emerald`, `amber`, `slate` (`domain.KnownApplicationColors`) |
| `summary_machine` | `mch_*` | Whose record count the card reports; must be a Machine this Application claims |
| `show_nav` | bool | `false` suppresses this Application's menu chrome only — every route stays reachable, and the launcher never reads it to decide *whether* an Application appears |
| `machines[]` | `mch_*` ids | Selected from the Workspace's set, never file paths. At most one Application may claim a given Machine |
| `roles[]` | strings | This Application's own role vocabulary; optional. Captions the Members list — not yet an authorization input |
| `navigation[]` | list of items | This Application's own menu |
| `view_groups[]` | list | Optional (2026-10-19). Screens that are **arrangements of one Machine's records** and share one header -- see **View groups** below |
| `workflow` | `{engine, roles{}}` | Optional — which runtime workflow engine this Application runs on, and which of its own Machines plays each role in it (2026-09-28). See below |

**Installing one of these instead of writing it** (2026-09-28): `metadata/applications/*.yaml` is a
template library, and `/install-application` (workspace admins only) copies one into a Workspace. A
Machine id or Application id the Workspace already uses is **renamed** on the way in
(`mch_document` → `mch_document_approval`); a navigation id, a route or a Dataset id that collides is
**refused**, because Go still names those. With no collision the copy is byte-identical to the
template, so `diff`ing your Workspace's file against the library shows exactly what you have changed
since. Each copy carries a header saying where it came from and what was renamed — the comments below
that header still name the template's original ids.

**`workflow:` — binding a runtime engine to your own Machines**

```yaml
workflow:
  engine: document_approval
  roles:
    document: mch_document
    step: mch_approval_step
```

`engine` is a closed set (`registry.KnownWorkflowEngines`) with one member today,
`document_approval`: the sequential/parallel multi-step approval engine in `internal/action`. Its
required roles are `document` (what is being approved) and `step` (one approval decision each). Both
are required — an engine cannot run against a partial cast — and each must name a Machine **this**
Application claims in its own `machines:` above. One Machine may hold only one role. Every one of
those is a load-time error.

What the binding buys you: the engine engages because you *declared* it, not because anything is
named a particular way. Before 2026-09-28 `internal/action` matched the literals
`app_document_approval` + `mch_document` / `mch_approval_step`, so an approval Application had to
carry those exact three ids or the engine silently never woke — the screens rendered and nothing
approved. With the binding, `app_persetujuan` over `mch_surat`/`mch_langkah` works identically
(`internal/conformance.TestWorkflowEngineEngagesUnderAnyApplicationAndMachineNames` is that exact
case).

The cast can be wider than the two required roles. `signature` (a person's own reusable signature
image), `flow_template` and `flow_template_step` (a saved default approval flow per document type) are
**optional**: cast them and the feature exists, leave them out and the engine runs without it — an
approver draws a one-time signature per decision, and no default flow is offered. `flow_template` and
`flow_template_step` are optional *together*; casting one without the other fails at load, since a
template with no steps stores nothing.

What the binding does **not** buy you, as of 2026-09-28: an Action now declares what it *writes*
(`actions:`, §12.6c below), so a bound Machine may name those Fields anything — but the engine still
*reads* several by id (`fld_decision` when it decides which steps are approved for the signed PDF,
`fld_sequence`, `fld_assignee`). So a Machine you bind should still carry that vocabulary for the
reading half to work. This is why the AI assistant is deliberately not told about `workflow:` yet: it
could produce an Application that binds correctly, writes correctly, and still shows nothing on the
screens that read those Fields.

One consequence worth knowing if you install two approval Applications in one Workspace: the submit
wizard's own routes (`/documents/new`, `POST /documents`) are named by no navigation item, so nothing
tells them which Application they belong to, and they answer 404 rather than guessing. Every screen
*inside* an Application resolves its own Machines correctly, and so does the Workspace-level chrome
(Home's pending count and the navigation badge sum across every approval Application installed).

There is no `application:` singular block and no `hidden_nav_groups:` any more — the first became
`applications:`, the second became per-Application `show_nav:` (2026-09-20).

Navigation item keys:

| Key | Value | Notes |
|---|---|---|
| `id` | `nav_*` | Referenced from `.templ` via `routeByID("nav_xxx")` — that's how a page links to a sibling screen without retyping the route |
| `label` | string | What a **menu** says: the header strip and the mobile bottom bar. Short enough for a four-column bar |
| `title` | string | What the **screen** says about itself — its own `<h1>`. Optional; falls back to `label`. Render with `titleByID("nav_xxx")`, never retyped |
| `description` | string | The sentence under that heading. Optional; no subtitle renders when absent. Render with `descriptionByID("nav_xxx")` |
| `route` | path | Must have a real handler in `internal/web/router.go` (gated by `TestNavigationRoutesAreRegistered`) |
| `group` | string | Presentational label. First group declared stays inline; later ones collapse into a dropdown |
| `priority` | int | Orders the menu. The first four (`rendering.appMenuTabLimit`) are tabs on desktop; the rest collapse behind a "More" dropdown (hardcoded count -- no per-item overflow declaration exists yet, 007 §12.2). An Application with four or fewer items never shows it |
| `badge` | closed set | Only `approval_inbox_pending` today |
| `home_card` | bool | Marks the Workspace Home card's destination (`Application.HomeRoute`) |
| `icon` | closed set | Drawn on the mobile bottom bar. Same vocabulary as an Application's own `icon` — see below |
| `page` | node tree | Optional (2026-10-07): declares the screen's **body**, so no Go renders it. The `route` must then be `/pages/<this id>`. See **`page:` — declaring a screen's body** below |
| `back_to` | navigation item id | Optional (2026-10-18), on a `record_of` page only: the screen of the same Application a person lands on when they delete the record from its own page (a `write: delete` Button). Without it they land on the Machine's own list. A delete from any other screen stays on it |
| `record_of` | Machine id | Optional (2026-10-17). Makes the page **one record's page**: it opens at `/pages/<id>/<record id>`, reads the record as `$parameters.id`, draws its own heading, is no menu entry, and every link to a record of that Machine (a Table's `href: record`) opens it. See **One record's page** below |
| `in_menu` | bool | Optional (2026-10-17), default `true`. `false` keeps the item out of the Application's menu while its route stays valid and `to:` still resolves it: a screen another screen opens, such as the "New contract" form behind a list's action |
| `view_group` | `view_groups[].id` | Optional (2026-10-19). Makes the screen one arrangement of that group: it draws the group's header (title, summary, the Board / Table / Calendar switch, the scope control) **instead of its own heading**, and keeps the group's scope choice on its links to the group's other screens. Items of a group are listed in the switch in the order they are declared |
| `summary` | `counts` | Optional (2026-10-19), only beside `view_group`. Words the group's header with its counts ("11 tasks · grouped by List · 2 complete") instead of the item's own `description`, for the arrangements that show every record |

**Icon names** (`domain.KnownIcons`, drawn by `internal/rendering/icons.templ`). A name, never a
glyph or a path: metadata says *which* icon, the runtime says how it is drawn, so the whole set is
restyled in one file and every icon on a screen keeps the same stroke weight. An unknown name fails
at load.

| Declarable | |
|---|---|
| `check` `board` `inbox` `file-text` `check-square` | |
| `dashboard` `calendar` `timer` `bar-chart` `list` | |
| `bolt` `settings` `user-check` | |

`home`, `grid`, `more`, `chevron-right` and `chevron-down`/`switch` are drawn too, but they are the
runtime's own chrome (`appShell`) and no manifest names them. Adding an icon means adding a `case` to
`icons.templ` *and* a name to `KnownIcons` — `TestKnownIconsAreAllDrawn` fails if only one of
the two happens.

This replaced `icon: "▣"`, a single literal character, on 2026-09-24. A character could never
carry a consistent stroke weight across a set, because it belongs to whichever font happened to
have that codepoint — which is why the convention's own declaration comment had always called
it a placeholder.

**View groups (`view_groups:`, 2026-10-19, Case 19 mockups PM01/PM05/PM08).** A Board, a Table and a Calendar of the same tasks are three
screens, not three Views of one page: the Board is the Machine's `board` View, the others are `page:` blocks. What they share is a
**header**, and that is declared once on the Application and named by each screen's `view_group:`:

```yaml
view_groups:
  - id: vg_tasks
    machine: mch_task
    figures: { dataset: ds_task_group_figures, total: msr_all, complete: msr_completed }   # an aggregate Dataset of the Machine
    summary:
      total: { one: "{n} task", other: "{n} tasks" }
      complete: "{n} complete"
      grouped: "grouped by {field}"          # said only on the screen that is the Machine's board
    scope:                                    # optional: which one of a larger thing these records are of
      machine: mch_project
      field: fld_project                      # the group Machine's relation to it
      dataset: ds_projects_with_tasks         # a records Dataset of the scope Machine: the menu's choices
      parameter: project                      # ?project=<record id>
      switch: Switch project
      all: All projects
      chosen: "one of {n} projects in this workspace"
      chosen_one: "the only project in this workspace"   # optional: said for a count of one; absent, `chosen` serves one too
      overall: "{n} projects in this workspace"
      overall_one: "{n} project in this workspace"       # optional, likewise (2026-10-20: "1 projects" was the review's finding)
      more: "Showing the first {n} projects"   # optional: said under a menu cut at the Dataset's `limit:`
      new: New project                         # optional: the form that makes one more (2026-10-19), for a viewer who may create the scope Machine
navigation:
  - { id: nav_board, route: /machines/mch_task, view_group: vg_tasks, summary: counts, ... }
  - { id: nav_task_table, view_group: vg_tasks, summary: counts, in_menu: false, page: ... }
```

**`scope.new:` (2026-10-19)** puts a "New project" form under the choices in the menu: every Field the scope Machine can be given, posted to
its own create route, which answers a form sent from another screen with a refresh, so the new project is then one of the choices. It is drawn only for a
viewer who may create the Machine, and a Workspace with **no project yet** shows it alone as the pill (otherwise nothing on these screens could make the
first). The mockups draw only "Switch project"; making one was asked for by the owner (2026-10-20). The same shape for a settings page is a **Popover over a
`Form`** (`component: Popover` with `density: regular`, its child `component: Form` with `fields:` and `binding: {dataset, write: create}`): Board settings
has "New list" and "New label" beside their headings, as mockup PM06 draws "+ New list" and "+ New label".

The header reads the runtime's own numbers -- the figures come from the aggregate Dataset (the whole set, so a list's `limit:` never
understates them), the project count from the scope's Dataset -- and no screen writes a count or a name. A request with **no** scope
parameter shows every record (tasks that name no project stay visible) and the title is the screen's own heading; with one, the title is
that record's name and every screen shows only its records. **The scope is carried by declaration, not by luck**: every Dataset a
`page:` of the group reads over the group's Machine -- a records Dataset's `where:`, and **each counted Measure's `where:` of an aggregate one** -- and
the two Measures the summary counts, must carry
`{field: fld_project, op: equals, value: $parameters.project, optional: true}` **at the top level (`all:`), never inside `any:`** (there it would widen the
result: "done anywhere OR in this project", and the count would disagree with the rows); the loader refuses a group where one does not, so choosing
a project cannot change one screen and leave another unfiltered. The scope's own Dataset is read by the header with **no viewer and no request
parameter**, so the loader also refuses one that is `viewer_scoped`, filters on `$current_user`, `$week` or a non-optional `$parameters.*` (it would be a 500
on every screen of the group, found by the first request). A scope list cut at its Dataset's `limit:` is said to be cut (`scope.more`), is still counted
whole, and a project past the cut is a real choice, not a stale one. The Board reads the same declaration. A parameter naming no record of the
scope is dropped (the screens show everything), not turned into an empty filter. Links between the group's screens (the switch, and a
`to:` link on a page, week links included) keep the choice; any other link does not. A Workspace with no project has no pill and no
sentence about projects. **The AI assistant has no slot for `view_groups`** (it writes no `page:` either; `UnwritableByTheAssistant`'s
`page` entry says so).

`optional: true` on an **aggregate Measure's** `where:` is new with this (a count follows the same request parameter a list does); it is
valid over a non-reserved `$parameters.<name>` only.

### 12.1b `settings_hub` / `settings_hub_member` — an Application's Settings page

Mark one navigation item `settings_hub: true` and the runtime draws that Application's Settings page for it.
The route is `/settings/<the item's own id>`, which one generic handler serves the way `/pages/<id>` is served;
a route of that shape naming an item that is not a hub is a load-time-checked 404 (`TestNavigationRoutesAreRegistered`).

```yaml
- id: nav_tally_settings
  label: Settings
  title: Tally settings              # the page heading
  description: Applies to Tally only.
  route: /settings/nav_tally_settings
  icon: settings
  settings_hub: true
- id: nav_tally_units               # one row under "Configuration"
  label: Units
  description: What a count is measured in.
  route: /machines/mch_unit        # any real destination of this Application
  settings_hub_member: true
```

What the page draws is derived, never written into a template:

| section | present when | reads |
|---|---|---|
| Access (Members & roles for a Workspace admin, Permissions) | the Application declares `roles:` | `roles:`, the viewer's Workspace role; Members & roles links to `<hub route>/members` |
| Configuration | at least one `settings_hub_member` item | each item's `label`, `description`, `route`, in declaration order |
| About | always | the Application's name, description, machine and role counts |

**Members & roles** is the hub's own sub-address, `/settings/<hub id>/members`, not a navigation item: it lists
every Workspace member with this Application's role in a select (`roles:` is the vocabulary) and saves one row
at a time to `POST /settings/<hub id>/members/<user record id>`. It reads and writes the same `app_roles` row the
Workspace's Edit member screen does, so a role set in either place is the role shown in the other. A Workspace
admin only (403 otherwise); 404 for a hub whose Application declares no `roles:`. A member whose role comes
through a Group is annotated, and the select shows only the *direct* role. Nothing to write in YAML.

A member is a row; it needs no code. There is no Permissions item to declare: its page is the runtime's own
role matrix and is reached as `?section=permissions` on the hub. An Application with no `roles:` has no Access
section and no matrix. The page does not say when an Application was installed or from which template -- nothing
stores either. `TestSettingsHubNamesNoApplication` keeps the files that draw it free of any one Application's words.

### 12.1a `page:` — declaring a screen's body

A navigation item may carry a `page:` block. The runtime then renders `/pages/<nav id>` with one generic
handler: **no Go function, no `.templ`, no per-screen route** (007 §12.4, §15.1). The header (`<h1>`,
subtitle; no eyebrow since 2026-10-18 -- the application bar already names the Application) is **not written in `page:`**; it comes from the item's own `title:`/`description:`
(`label:` when there is no title), so a heading is never stated twice (001 #8).

A node is a mapping with **exactly one** of three discriminators, whose value is the type:

| Key | Value | Meaning |
|---|---|---|
| `layout:` | `columns`, `grid`, `panel`, `row`, `section`, `split`, `stack` | Arranges `children:`; a property's value must be one of its closed set (an unknown `columns: 9` or `gap: huge` is a load error, not a silent default). Properties: `columns` `align`, `gap`, `tracks`; `grid` `columns`, `gap`, `mobile`; `panel` `density`; `row` `align`, `gap`, `justify`, `mark`, `tracks`, `wrap`; `section` `anchor`, `current`, `dropField`, `dropValue`, `gap`, `outside`; `split` `aside`, `gap`, `side`; `stack` `anchor`, `gap`, `move` |
| `static:` | `eyebrow`, `heading`, `paragraph`, `subheading`, `overline`, `panel-heading`, `caption`, `message`, `note`, `code`, `divider` | Text that is the same for everyone (`code` keeps its line breaks and spacing: a verdict or a file's content). Property: `text`. Each kind is a text role the Theme can restyle (`theme.text.*`), so a section title is `subheading`, not a `heading` made smaller. The tenth, `link`, takes `to: <navigation item id>` instead of text-and-route: see the next paragraph. `divider` (2026-10-22) is a thin rule that sets one region off from the next: it has **no text and no properties** (words on it are a load error), only a `when:` if the region it separates is not drawn for everyone |
| `component:` | a registered Component (`Metric`, `StatusBadge`, `Avatar`, `Button`, `Tag`, `Callout`, `EmptyState`, ...) | Drawn from its contract; properties are that Component's declared inputs |

`columns` puts different content side by side from the small breakpoint up and stacks it on a phone (`align`: `start`, `center` or `end`, the cross axis); `section` is a bordered surface that stacks its children with a `gap`. A `StatusBadge` may carry `size: compact` for a dense row. The Layout row above is checked against `ir.PermittedProps` by `conformance.TestGuideLayoutTableIsTheAllowedPropsList`, so it cannot fall behind the list that decides what loads. Worked example: `nav_dashboard`' two open-card groups, a `columns` of two `section`s.

**A Workspace's own pages (2026-10-16).** A page does not have to belong to an Application. The Workspace manifest's own
`navigation:` takes items with a `page:` exactly as an Application's does: the route is still `/pages/<id>`, the heading
still comes from `title:`/`description:`, and the bindings may read any Dataset of the Workspace, so one page can put
figures from three Applications side by side (`default.yaml`'s Overview). Two things differ. A `to:` link on it resolves
against the whole Workspace (its own pages, the runtime's screens and every installed Application's menu), where an
Application's page resolves against its own menu. And **who opens it is the item's `open_to:`** (2026-10-17): absent, or
`admin`, is Workspace admins only, as before; `member` admits any member of the Workspace. **The shell no longer offers a Workspace page (2026-10-22):** the Workspace's name is a link to Home and the menu it used to open is gone, so a Workspace page is reached by a `to:` link from another page or by its route; nothing lists it. A person with no membership in the Workspace is refused either way (403, the answer a
Workspace screen gives), and so is an unknown page id for anyone who is not an admin. No Application role gates a Workspace page,
so `open_to: member` is the author's statement that what the page reads is for every member: bind it to a Dataset that is the
viewer's own (`viewer_scoped: true`, as `default.yaml`'s My work does) or to figures you are content for every member to see; who
may read which record is not built (D5). `open_to:` is valid only on a Workspace page, never on an Application's item, and takes
`admin` or `member` and nothing else. A node's `when: {viewer: ...}` (below) decides what a viewer sees inside a page they
may open; `open_to:` decides who may open it. The loader checks
a Workspace page by the same rules as an Application's, so a dataset nobody declares, a link to a missing item or a route that
is not `/pages/<id>` is a load error.

```yaml
# metadata/workspaces/<slug>.yaml
navigation:
  - id: nav_workspace_overview
    label: Overview
    route: /pages/nav_workspace_overview
    icon: dashboard
    page:
      layout: stack
      children:
        - component: Metric
          label: Open tickets
          binding: { dataset: ds_helpdesk_figures, measure: msr_open, rows: total }
```

`children:` is a list of nodes. The root must be a `layout:`. Any key a node's type does not declare is a
**load error**, not an ignored typo (`ir.Validate`, 007 §15.3), and the same sweep refuses a cycle, a tree
deeper than `ir.MaxDepth`, and a Component input its own validator rejects.

**How far to rely on a Component (its Status).** Every registered Component carries one, and it is a measurement, not a
mood. **`stable`**: an installed Workspace's `page:` depends on the contract, either by declaring the Component or because
lowering emits it for one that does, so changing an input would break a manifest that exists. **`experimental`**: no
installed manifest depends on it yet, so its contract may still change; the first page to use it is the moment that stops
being free, and the Status is promoted then. **`deprecated`**: a successor exists, named in the last column. Nothing
refuses an `experimental` or a `deprecated` Component today: it loads and renders, and the Status tells you what you are
building on. `conformance.TestWritingGuideShowsEachComponentsStatus` holds this table to the registry and
`TestComponentStatusMatchesInstalledUse` holds the words to the installed manifests, so neither can go stale quietly.

| Component | Status | What that rests on |
|---|---|---|
| `Button` | stable | Declared by installed pages (Board Settings, Calendar, My Tasks) |
| `Collection` | stable | Declared by six installed pages (Board Settings, Calendar, Dashboard, My Tasks, both document lists) |
| `Field` | stable | Never written: every `Form` lowers to one around each control, so changing it changes every installed Form |
| `Form` | stable | Declared by an installed page (Board Settings) |
| `Input` | stable | Never written: emitted by a `Form`'s lowering, one per control, so it is `Form`'s contract; `label` names a control that sits under a column header (a hidden label) |
| `Metric` | stable | Declared by installed pages (Dashboard, Documents by status) |
| `Progress` | stable | Declared by an installed page (Dashboard) |
| `StatusBadge` | stable | Declared by five installed pages (Calendar, Dashboard, both document lists, My Tasks) |
| `Tag` | stable | Declared by installed pages (Board Settings, Calendar, My Tasks) |
| `Avatar` | stable | Declared by installed pages (Project Management's Table: the assignee, `from: {initials: "person:initials", label: person}`) |
| `Callout` | stable | Declared by the runtime's own page for the template library (a template that cannot be installed here, and why) |
| `Diff` | stable | Declared by the runtime's own page for replacing a Workspace file (what applying the reviewed draft would change) |
| `Disclosure` | stable | Declared by the runtime's own page for Groups ("+ New group"); Members still draws it in Go ("+ Invite member") |
| `SegmentedControl` | stable | The Calendar of the Project Management Application declares one (Week \| Month); the record list's view switch draws it in Go |
| `Table` | stable | Declared by an installed page (Helpdesk's All tickets in Workspace `default`) |
| `NavList` | stable | Declared by the runtime's own page for Groups, installed in every Workspace |
| `SearchBox` | stable | Declared by the runtime's own page for Members, installed in every Workspace |
| `Stepper` | stable | Declared by installed pages (Dokter Kecil's contract and payment plan pages) |
| `Toggle` | stable | Declared by installed pages (Project Management's Table: the completion circle) |
| `IconTile` | stable | Declared by the runtime's own page for Workspace Home (an Application's face at the head of its row); the application menu, the More sheet and the launcher draw it from Go |
| `ActionBar` | stable | Declared by installed pages (Dokter Kecil's Contracts: "+ New contract") |
| `FilterChips` | stable | Declared by installed pages (Dokter Kecil's Contracts, Instalments and Payment plans) |
| `FilterSelect` | stable | Declared by installed pages (Dokter Kecil's Contracts, Instalments and Payment plans) |
| `Popover` | stable | `trigger: pill` (2026-10-19) is its words in a small outlined pill with a chevron after them (the shared header's "Switch project"); `trigger: quiet` (2026-10-19) is a three-dot mark with no outline, for the menu of a thing that is already a surface (a board column's "List actions"; it names the menu with `title:`). Declared by installed pages (Dokter Kecil's Contracts: a row's Edit opens the contract's form in place); the record page's "..." menu and the board header's "New <machine>" form draw it in Go. **The panel is `position: fixed` (2026-10-19)** so the scrolling box of a table (a row's Edit) cannot cut it; it sits where it would have been, under the trigger and flush with the side `align:` names, and below `sm` (a phone) it gets offsets of its own: a gutter in from each side, above the bottom bar, scrolling inside when it is tall. It does not follow the trigger if the page is scrolled while it is open. **Two shapes by `density` (2026-10-19):** `compact` (a menu of actions) is a small panel under its trigger; `regular` (a form to fill in: an Edit, a "New ...") is a **dialog** -- a centred card with the page dimmed behind it from `sm` up, a sheet above the bottom bar on a phone, a title and a close mark on top, a scrolling body, and the Form's own **Cancel and submit pinned under it** so they are always in sight. It closes only on purpose (Cancel, the close mark, Escape) and never by a tap outside it, which would throw a half-filled form away. **Only one is open at a time (2026-10-25):** opening a Popover closes the others, except the menu it was opened from; you declare nothing for it, and it needs a current browser (Chrome 120 / Safari 17.2, no fallback) |
| `EmptyState` | stable | A layout's `empty:` lowers to it (My Tasks in two Workspaces); the Approval Inbox and Groups screens draw it in Go, and a `Collection`'s `empty:` stays a plain message |

**`binding:` — the only way a number gets into a page.** A `component: Metric` may carry
`binding: {dataset: ds_x, measure: msr_y, rows: dimension}`. It then stands for **one Metric per value of
that Dataset's Dimension**: `label` is the value, `value` is the Measure. Rules, all checked at load:

- the Dataset must be an **aggregate** one (no `select: records`) that some Machine in the Workspace
  declares, with that Measure and a `dimension:`;
- `label` and `value` may not also be written on the node — a typed figure beside a bound one is the
  number the author typed, which is what `binding:` exists to avoid;
- rows follow the Dimension's declared option order, then any unseen values sorted; a declared option with
  no record shows an explicit `0`;
- when the Dimension is a **reference Field** (a `relation` to another Machine, or a `person`), the stored value is
  an id, so each tile is named by the related record's title -- a Project's name, a member's display name -- and
  ordered by that title; a value naming no record that exists, and a record with the Dimension unset, get no tile
  rather than an id as a label. A link (`to:`/`param:`) still carries the stored id, which is what the destination
  compares. Worked example: `ds_open_by_assignee` on `nav_dashboard`;
- there is no expression and no path (007 §9.2): a Dataset, a Measure and a mode;
- a bound Metric may **link each of its rows** with `to: <navigation item id>` and `param: <name>` together:
  every tile becomes `<route>?<name>=<the row's own value>`, the value escaped as data. The destination must be
  a page of the same Application that binds a Dataset filtering on `$parameters.<name>` -- checked at load,
  because a link carrying `?status=` to a page that reads no such parameter would draw, work, and filter
  nothing. `href` is never written (a route is declared once); `to:`/`param:` on any other node is a load error.
  Worked example: `nav_documents_by_status`'s tiles open `nav_documents_in_status`.

Only `Metric` (`rows: dimension` or `rows: total`) and `Collection` (`rows: records` or `rows: dimension`) are
bindable (`domain.BindableComponents`). Cost: one query per distinct Dataset, however many nodes bind it.

**A Collection over a Dimension's values (2026-10-11).** When each value needs more than one figure -- a bar of
open cards out of everything assigned -- bind a `Collection` with `{dataset: ds_x, rows: dimension}` and **no
measure**. Its one child is cloned per value, and `from:` reads the row: `row:label` is the value's display (an
option, or a related record's title for a reference Dimension), and `measure:<id>` is any Measure the Dataset
declares (a mistyped id is a load error). A row is a group of records, not a record, so the template may hold no
binding, `count:`, `each:`, `when:` or `from: {href: record}`. `Progress` (a label, a `value`, a `max`) draws a
bar with the two numbers in words beside it. Worked example, `nav_dashboard`'s "Open cards per person":

```yaml
- id: ds_open_by_assignee          # metadata/task.yaml
  dimension: fld_assignee
  measures:
    - { id: msr_open, aggregate: count, where: { field: fld_status, op: not_equals, value: $done } }
    - { id: msr_all, aggregate: count }

- component: Collection            # the page
  gap: tight
  divided: true
  empty: No cards are assigned yet.
  binding: { dataset: ds_open_by_assignee, rows: dimension }
  children:
    - component: Progress
      from: { label: "row:label", value: "measure:msr_open", max: "measure:msr_all" }
```

**`rows: total` — one figure over the whole Dataset.** A `component: Metric` bound
`{dataset: ds_x, measure: msr_y, rows: total}` is a single Metric holding that Measure's figure, for an
aggregate Dataset. It reads the Measure's whole-Dataset total, which a Dimension does not change, so the Dataset
needs none (`ds_task_figures` has none). There is no
Dimension value to name the figure, so **the node writes its own `label:`** (a load error if it does not, and
`value:` is still never written), and it takes no `to:`/`param:` (there are no rows to link). `tone:` is a signal
and is drawn only while the figure is not `0` -- "Overdue: 0" is not a problem, so it is not red. Worked example,
four figures of the Task Machine (`nav_dashboard`):

```yaml
# metadata/task.yaml -- the Measures
- id: ds_task_figures
  measures:
    - { id: msr_all, aggregate: count }
    - { id: msr_open, aggregate: count, where: { field: fld_status, op: not_equals, value: $done } }
    - id: msr_overdue
      aggregate: count
      where:
        all:
          - { field: fld_status, op: not_equals, value: $done }
          - { field: fld_due_date, op: lt, value: $today }

# the Application -- the page
- component: Metric
  label: Overdue
  tone: bad
  binding: { dataset: ds_task_figures, measure: msr_overdue, rows: total }
```

**Two derived figures (2026-10-11): `count_distinct` and `percentage`.** `aggregate: count_distinct` with a
`field:` counts the distinct non-empty values that Field holds (how many Projects the cards sit in). `aggregate:
percentage` with `numerator:` and `denominator:` names two other Measures of the same Dataset -- neither itself a
percentage -- and is the first as a whole percentage of the second, rounded down, and `0` when the denominator is
`0`; it takes no `field:` and no `where:` of its own. A total Metric can word its hint around one of them with
`hint_figure: {of: <measure>, one: ..., other: ...}` -- `{n}` is the figure, `one` is optional, and a written
`hint:` beside it is a load error:

```yaml
- { id: msr_projects, aggregate: count_distinct, field: fld_project }
- { id: msr_completed_share, aggregate: percentage, numerator: msr_completed, denominator: msr_all }

- component: Metric
  label: All cards
  hint_figure: { of: msr_projects, one: "In {n} project", other: "In {n} projects" }
  binding: { dataset: ds_task_figures, measure: msr_all, rows: total }
```

A Measure's `where:` takes the same one-level `all:` conjunction a records Dataset does, and two sentinels:
`$today` (the request's date, against a date Field; `$today+7` / `$today-30` are whole days from it, so "the next
week" is `gte $today` and `lte $today+7` rather than a date typed into the file) and **`$done`** -- whatever the Machine's own `completion:`
block names as finished, so "finished" is declared once, there, and never retyped as `done` or `closed`. `$done`
is accepted only as `equals`/`not_equals` against the completion Field itself, and a Machine with no `completion:`
refuses it at load. `$current_user` and `$parameters.<name>` are refused in a Measure: an aggregate has no viewer
and no request, so they would be compared as literal text and count nothing.

**`any:` and `is_empty` — "one of these", and "has no value" (K15, 2026-10-10).** A `where:` (a records Dataset's or a
Measure's) may carry an `any:` list beside `all:`: the record is listed when **every** `all:` comparison holds **and**,
when `any:` is declared, **at least one** `any:` comparison does. It is one OR-group and not a boolean tree -- there is
no nesting, no `not`, and `any:` alone is valid. `op: is_empty` holds when the Field was never set or holds nothing, and
takes **no** `value:` (one is a load error). It is the only way to say "undated" or "unassigned": `equals` with an
empty value cannot, because an absent Field is not the empty string. An ordered operator (`lt`, `gt`, ...) is false for
an empty Field, so "overdue" needs no `is_empty` guard, and "later" -- undated, or due past the window -- is
`any: [{field: fld_due_date, op: is_empty}, {field: fld_due_date, op: gt, value: $today+7}]` beside the `all:` that
names whose and whether open (the four `ds_my_tasks_*` Datasets in `task.yaml`).

**`contains` and `optional: true` — a list people search and filter (2026-10-17).** `op: contains` holds when a text Field
holds the words, ignoring case (`reza` finds "Reza Anggara"); it is for `text`/`long_text` Fields, and a `%` or `_` in the
words is matched as itself, never as a pattern. A comparison on a records Dataset whose value is a `$parameters.<name>` may
say **`optional: true`**: when the request does not send the parameter, or sends it empty, the comparison is left out --
so the "All" chip and an empty search box list everything. Without `optional`, an absent parameter still lists nothing
(fail closed). The usual shape is one optional `equals` per filter and one `any:` of optional `contains` for the search:

```yaml
- id: ds_contracts
  select: records
  where:
    all:
      - { field: fld_contract_kind, op: equals, value: $parameters.kind, optional: true }
      - { field: fld_category, op: equals, value: $parameters.category, optional: true }
    any:
      - { field: fld_contract_no, op: contains, value: $parameters.q, optional: true }
      - { field: fld_party_name, op: contains, value: $parameters.q, optional: true }
  limit: 100
```

`optional` on a Measure, or on a value that is not a `$parameters.<name>`, is a load error.

**`rows: records` — a list of a Machine's records.** A `component: Collection` may carry
`binding: {dataset: ds_x, rows: records}` and **exactly one child**, the item template. The template is cloned once
per record, in the Dataset's own `sort:` order, and a node inside it fills a property from the record with
`from: {property: role}`, where `role` is a Projection role (`title`, `status`, `person`, `money`, `date`, `container`) the
Machine declares in `card_fields:` -- so the page names no Field, and what a list shows of a record is what its
Machine already said about its shape. Rules, all checked at load:

- the Dataset must be `select: records` (so it already carries the `limit:` 007 §7.9 requires -- **the limit is
  the list's meaning, "the latest five", and no truncation notice is drawn unless the page writes `complete: true`**), and may filter on `$current_user`
  and on `$parameters.<name>`. **A list of the viewer's own records declares it: `viewer_scoped: true` on the Dataset**
  (2026-10-15), and the loader then refuses the Dataset unless `where:` carries a top-level `equals $current_user` on a
  person field -- so deleting the predicate stops being silent. It says what the list shows; it does not hide the
  records elsewhere (the Machine's own page lists them all, and a read permission per role does not exist yet).
  A page's request parameters are its **query string**, and a page declares none of
  them: the names are whatever its bound Datasets' `where:` reads (`value: $parameters.status` is
  `/pages/<id>?status=...`). A request that sends no such value lists **nothing and reads nothing** (007 §9.2,
  fail closed -- an absent value is an ordinary request, not "everyone"), so write `empty:` words that serve both
  that and a value matching nothing; the value is compared as data;
- a `where:` may also order a **number or date** Field with `lt`/`lte`/`gt`/`gte`, and `value: $today` is the request's
  date (`YYYY-MM-DD`, from the clock the handler reads and injected, never read by the evaluator). `$today` is valid
  only against a `date` Field and only in a records Dataset's `where:` -- a `Measure.Where` refuses every
  `$`-value. A record with no value for the Field, or an empty one, satisfies none of the four; a number Field
  compares as a number ("10" is above "9"). **`$week`** is the Monday of the week the request's `?week=N` selects
  (N whole weeks from this one; absent or not a whole number means this week, and N is clamped to 520), and `$week+6`
  is that week's Sunday -- so "the week in view" is `gte $week` and `lte $week+6`, with no date typed into the file.
  Like `$today` it is a date and only a date, and it is valid only in a records Dataset's `where:` (a Measure or a
  Relation has no request parameters and refuses it). The parameter name `week` is the runtime's own
  (`expression.WeekParameter`), the one request parameter a *Dataset* reads by a name the runtime chose rather than
  an author. Example, "past due": `fld_due_date lt $today` beside
  `fld_status equals in_review`, which `nav_documents_by_status` draws under a "Past due" subheading;
- `from:` is valid only inside a records-bound Collection's template, a property cannot be both written and
  `from:`, and a records binding does not nest inside a template;
- a role whose Field is a **reference** to another Machine is refused: resolving it reads the whole related
  Machine (007 §20), which a list would repeat per page view. Ask for a role backed by a plain Field. **Two
  references are resolved. A `container` Field** (a `relation` Field declared `{ field: fld_project, role: container }`)
  draws the related record's own title -- its Machine's `title` role, or its first Field when it declares none --
  read for only the records the list shows, in one statement by id (`composition.Loader.RelatedLabels`), so it does
  not grow with the Workspace; a record that no longer exists draws nothing, never its id. `from: {text: container}`
  on a `static` node is the whole declaration (`nav_my_tasks` captions each Task with its Project). **The other is a
  `person` Field** (the one that points at `mch_user`). A page draws the member's display
  name (their email until they set one), through the same per-request member list the runtime's own screens use
  -- one query however many records the list shows -- and draws **nothing** for someone who is no longer a member,
  never their user id. `{ field: fld_submitted_by, role: person }` in `card_fields:` and `from: {text: person}` on
  a `static` node is the whole declaration. Adding the role also gives that Machine's generic cards an avatar for it;
- a role the Machine does not declare is a load error, not a blank cell.

A records-bound Collection may also carry `empty: <words>`: what the page says when the Dataset lists no
records (the words are drawn as a `message`, **in place of** the list, and never beside a non-empty one). The
Component only decides *when*; the words are the page's. Without `empty:` an empty list draws nothing, as
before. `empty:` is a load error anywhere but a Collection bound with `rows: records` -- an unbound Collection
or a `rows: dimension` Metric cannot produce nothing. There is no *automatic* truncation notice, on purpose: a
Dataset's `limit:` is a safety cap in one Dataset and a window ("the latest five") in another, nothing declares
which, and a "showing the first N" line over a window is false. A page that knows its list is a cap opts in with
`complete: true` (below).

**A section that is hidden when it is empty, with a heading and a count (K15, 2026-10-10).** A records-bound
Collection may carry `heading: <words>` (drawn as a panel heading, then a small badge with the number of rows
under it) and `hint: <words>` (a note after the count; a `hint:` without a `heading:` is a load error). Add
`hide_empty: true` and a Collection whose Dataset lists nothing is **not drawn at all** -- no heading over an empty
list. `hide_empty:` and `empty:` answer the same question in opposite ways, so writing both is a load error, and
`hide_empty:` (like `empty:`) is valid only on a Collection bound with `rows: records`; its value is `true` or
`false`. Because a page whose every section is hidden would be a layout with nothing in it, **a layout may carry
`empty: <words>`**: drawn as a `message` only when every one of its children was left out, and never beside
real content. A layout with no children cannot carry it. The count is the rows drawn; a list cut at its Dataset's
`limit:` says so through `complete: true`, not here. A layout whose children are **all** `hide_empty` Collections must write `empty:` (a load error otherwise -- load lowers with one placeholder record, where nothing is empty, so the bare case would first fail on a viewer's quiet day). `hide_empty` and the count read the rows the Dataset returned; a `when:` that hides every item of a non-empty list still leaves the heading with its count. The worked page is `nav_my_tasks` (Overdue / Next 7 days /
Later / Completed) in Project Management.

**A list grouped by day (K15 Calendar stage 2, 2026-10-11).** A records Collection whose binding adds `group: date`
draws one **section per day** of the Dataset's own window -- a `Mon 05 Oct` heading, then that day's records --
and draws a day with nothing in it too, with the Collection's `empty:` words. The sections are siblings in
the parent, so the layout around the Collection is yours (`layout: grid` with `columns: 7` and `mobile: 1` is a week;
a `stack` is an agenda). The Collection holds **exactly two children**: the heading template (read
`from: {text: "bucket:label"}`, the day's name -- a role of that template alone, never of a record) and the item
template cloned for each record. Three things are checked at load and refused with their reason: the Machine
declares a `date` role in `card_fields:`; and the Dataset closes its window on that Field with an `all:` holding
exactly one `gte` and one `lte` (`$week`/`$week+6` is a week; the days drawn are *read from* these two, so the page
cannot draw days its records are not selected from). The length of the window is checked **per request**, since
`$week` and a `$parameters.x` bound only have a value then: more than 62 days is an error, and so is a bound that
resolves to no date. A bound that names a parameter the request did not send draws the list empty, with the
Collection's `empty:` words, as an ungrouped list does. `complete:` and `hide_empty:` are refused -- an empty day is drawn on
purpose, and a window is complete by construction, so **when the Dataset's `limit:` cut it short the page says so by
itself**, once, above the first day. The heading template may not carry a record link, `count:`, `each:` or `when:`. Worked page: `nav_calendar` in Project Management.
The section holding the request's own date (today, from the same `$today` the Dataset resolves) is set apart without anything being written: it is announced as `aria-current="date"` and takes the `info` tone's fill instead of the raised background, so a `theme: tones:` declaration moves it. A page cannot write `current:` itself -- it cannot know which day it is read on.

**Week links (K15 Calendar stage 4, 2026-10-11).** Three keys let a page step through weeks without a route of its own, all for the one parameter the runtime names -- `week`, the `?week=N` that `$week` reads (a closed set of one):

```yaml
- static: link
  to: nav_calendar
  text: Next week
  step: {parameter: week, by: 1}      # the request's own week plus one, never the default's
- static: caption
  window: ds_tasks_due_week            # the words for that Dataset's window: "12 - 18 Oct 2026"
- static: link
  to: nav_calendar
  text: This week
  when: {parameter: week, is_not: "0"} # drawn only away from this week
```

`step:` is valid only on a `static: link` that has `to:`, and `by:` is a whole number other than zero. The link's address is the destination's route plus `?week=<offset>`, and a step that lands on this week links to the **plain route**, not to `?week=0`. It is checked at load that the destination page binds a Dataset reading `$week` -- a link moving a parameter nothing reads would draw and change nothing. `window:` takes a Dataset id and writes the text itself (so `text:`/`from:` beside it are refused): the Dataset must close a `date` window the way `group: date` requires, and the words are read from the same two bounds the day sections are drawn from, so the two cannot disagree. `when: {parameter: week, is: N}` / `is_not: N` is the first **request-scoped** condition: it needs no record and is valid on any node except inside a dimension row's or a day heading's template (which stand for a group, and refuse every `when:`), `N` is compared as a whole number of weeks, and `role:` and `parameter:` together are refused. A node it would hide is still validated at load.

**`static: link` — a link to one of the Application's own screens.** Write `to:` with a navigation item id and
nothing else:

```yaml
- static: link
  to: nav_approval_inbox
```

The route is read from that item's `route:` and the words default to its `label:`, so renaming either in
`navigation:` moves every page that links to it. Rules, all load errors:

- `to:` is required on a `link` and valid on nothing else;
- `href:` is not a property a page may write -- a route is declared once, in `navigation:` (001 #3, #8);
- the id must be an item in **the same Application's** `navigation:`, including one in a hidden group (the list
  is read before `hidden_nav_groups` filtering, as `rendering.routeByID` reads it);
- `text:` may be written when the link should say something other than the item's label.

**A link to a record — `from: {href: record}`.** Inside the item template of a records-bound `Collection`, a
link takes its words from a Projection role and its address from the one reserved word `record`:

```yaml
- static: link
  from:
    text: title
    href: record
```

`record` is the runtime's own route to that record (`/machines/<Machine>/records/<id>`, the full detail page),
so the page types no route and no Machine id. It is a reserved word and **not** a `card_fields` role: a role says
what a Machine declares about its records, and every Machine would otherwise have to declare a "link" to be
listed on a page. Rules, all load errors:

- `href` may be taken from `record` only, never from a Projection role, so a page cannot turn a text Field into
  an address;
- `record` is valid for nothing but a link's `href` (not as `text:`, not on another node);
- a link has one destination: `to:`, `list_of:` or `from: {href: record}`, never two of them and never a typed `href:`;
- a record link has no menu label to default its text to, so `text` must come from a role (or be written).

**A link to a Machine's list — `list_of:`.** A Machine no navigation item points at still has the runtime's own list
page (`/machines/<id>`, where its records are listed and created). Name it by a Dataset that reads the Machine:

```yaml
- static: link
  list_of: ds_recent_documents
  text: All documents
```

The Dataset is the declared handle on the Machine, so the page types no route and no Machine id. Rules, all load
errors: the Dataset must exist in **this Workspace**; `text:` is required (the Machine has no menu label to default
to, which is why this form exists beside `to:`); `list_of:` is valid on a `link` and nothing else, never beside `to:`
or `from: {href: record}`, and never with a typed `href:`.

A link is an address, not a grant: the page lists only the records its Dataset selects (`$current_user` fails
closed), and following the link still goes through that route's own authorization. Not available: a link to a
record's sub-screen (`/review`, `/edit`), which would need a second reserved word and a case that asks for it.

```yaml
- component: Collection
  gap: tight
  binding: {dataset: ds_recent_documents, rows: records}
  empty: No documents yet.
  children:
    - layout: row
      children:
        - static: paragraph
          from: {text: title}
        - component: StatusBadge
          tone: info
          from: {label: status}
```

```yaml
- id: nav_documents_by_status
  label: By Status
  title: Documents by status
  description: How many documents sit in each status.
  route: /pages/nav_documents_by_status
  page:
    layout: stack
    gap: default
    children:
      - layout: grid
        gap: default
        mobile: 2
        columns: 4
        children:
          - component: Metric
            binding: {dataset: ds_document_by_status, measure: msr_total, rows: dimension}
```

**`component: Tag`** (2026-10-08) draws a name as a pill with a colour dot: `label` is required, `color` is
optional and, when written, must be one of the palette (`blue`, `purple`, `amber`, `slate`, `emerald`, `cyan`,
`rose`). A colour outside it is a load error; there is no hex value and no class. The dot is decoration, so the
label is always what names the tag. It takes written text, or -- inside a records-bound `Collection` -- a
record's own label and colour (below); the boards, calendar and record detail keep drawing theirs from their
Machines.

```yaml
- component: Tag
  label: Urgent
  color: amber
```

**`component: Callout`** (2026-10-12) draws one message in a box tinted by its tone: `text` and `tone` are both
required, and `tone` is one of `info`, `good`, `bad`, `warn`. `neutral` and `muted` are label tones, not message
tones, so they are load errors. The tone also decides the live-region role: `bad` is `role="alert"`, and the other
three are `role="status"`. A page never chooses the role, and never writes a class or a title. The fill, ink, corner
and size follow the Workspace's `theme:` (`tone`, `radius.control`, `text.body`). The account screens use it in Go.
No installed page declares one yet, because no declared screen shows a message today.

```yaml
- component: Callout
  text: Nothing waits for your review today.
  tone: info
```

**`component: EmptyState`** (2026-10-13) draws what a list says when it holds nothing: one message in a dashed box. `text`
is its only input, it is required, and a blank one is a load error (a dashed box with no words says nothing). A page never
writes a class, an icon, a title or an action. The corner, edge, fill, size and ink follow the Workspace's `theme:`
(`radius.surface`, `border.control`, `background.raised`, `text.body`, `ink.secondary`). The Approval Inbox and Groups
screens use it in Go. **It is `stable` since owner decision E-1 (2026-10-14): a layout's `empty:` (what a page says when
everything it holds was hidden, My Tasks) lowers to this card**, because that is a whole region of the page. **A `Collection`'s
`empty:` is not what draws it** (E-2, rule: no): it stays a plain `static: message`, since Calendar uses `empty: "—"` as a per-day-cell
marker where a card would be wrong. A page cannot show an EmptyState *only when a list is empty*: it draws whenever it is
declared, and `when:` tests a record's own role or the request's `week` parameter, never whether a Dataset lists anything.
Declare a `component: EmptyState` yourself only for a message that is always true of the page.

```yaml
- component: EmptyState
  text: Reports build overnight, so there is nothing to show before the first run.
```

**`component: Disclosure`** (2026-10-14) is a panel that opens on a click to show what is under its `summary`. `summary` is
required and a blank one is a load error; the `body` slot holds the children, which may be any node (a `Form`, text, a
list). It is a native `<details>`, so it opens and closes with no script and no request, and a page writes no `open`,
`class`, `id` or `hx-*`. The surface, the summary's size, weight and ink follow the Workspace's `theme:`. **It is
`experimental`**: Groups and Members draw it in Go ("+ New group", "+ Invite member"), and no installed page declares one.
It declares `toggle`, the first Component action that is not a write; popovers and menus (native `popovertarget`, no script) are the next candidate, and modal dialogs (which need `showModal()`
script) wait on a declaration for that; none is covered.

```yaml
- component: Disclosure
  summary: + New label
  children:
    - component: Form
      # ... a bound Form, as under "Form" above
```

**`component: IconTile`** is one icon on an Application's tint, in a small square: the face of an Application in a list. `icon` is a
name of the closed icon set (the same names a navigation item's `icon:` takes, plus the chrome's: `sparkle`, `settings`, ...) and is
required; `color` is `blue`, `emerald`, `amber` or `slate` (slate when left out); `size` is required and one of `compact` (24px, in a
menu row), `inline` (32px, a card in the launcher), `sheet` (36px) and `lead` (40px, a list row that is the screen's subject); `state:
draft` draws the dashed outline of something nobody has published and `state: assistant` the assistant's own pale-indigo tile (a tile in a state takes no `color`). It is decoration and carries no name: draw the Application's name
beside it. A bad name is a load error, and so is any property that is not one of those four. Both `icon` and `color` may be read from a
record with `from: {icon: icon, color: tint}` (the `icon` and `tint` card roles, which a runtime Machine such as Workspace Home's
`mch_home_application` fills from the Application's own `icon:` and `color:`).

```yaml
- component: IconTile
  size: lead
  from: { icon: icon, color: tint }
```

**`component: Avatar`** is a person's initials in a circle. All four inputs are required: `initials` (one to three letters you
write, never derived), `label` (the person's name or email, which becomes the `aria-label` and the hover `title`, because two
letters are not a name), `size` (`inline` beside other content in a row, `lead` to head a detail screen, `compact` in the footer
of a dense card) and `presence` (`present`, or `pending` for someone who has only been invited, drawn with a dashed ring). A blank
label is a load error, and so is any property that is not one of the four. The circle's fill, ink and size follow the Workspace's
`theme:`. **It is `experimental`**: the members, assignee and app-shell screens draw it in Go, and no installed page declares one.

```yaml
- component: Avatar
  initials: AP
  label: Ana Putri
  size: inline
  presence: present
```

**`component: Popover`** (2026-10-15) is a floating panel that opens under a trigger and holds a short form or a short list of actions.
`label` is the trigger's words and is required; a trigger that is a glyph ("…") needs a `title`, its accessible name. Three closed
inputs choose what the hand-drawn ones differed in: `trigger` (`secondary`, an outlined button, or `filled`, the strong ink as a fill),
`align` (`start` opens toward the right from the trigger's left edge, `end` toward the left from its right edge) and `density`
(`compact` for a menu of rows, `regular` for a form). The `body` slot holds the children. It is a native `<details>`: it opens and
closes with no script and no request, and a page writes no `open`, `class` or `hx-*`. **`experimental`**: the record page's menu and the
board header's "New" form draw it in Go, and no installed page declares one. It is not the app shell's menus, a card that expands in place,
or a modal dialog.

```yaml
- component: Popover
  label: Actions
  trigger: secondary
  align: start
  density: compact
  children:
    - static: message
      text: Nothing to do here yet.
```

**`component: Table`** (2026-10-16) is a records table: a header row from the columns you declare and one row per record.

```yaml
- component: Table
  label: All tickets          # the table's accessible name: required
  empty: No tickets yet.      # said in place of a header over nothing
  columns:
    - { header: Ticket, from: title, href: record }   # the cell links to the record's own page
    - { header: Status, from: status }                # a status role is drawn as a badge
    - { header: Due, from: date, align: end }         # align: start (default) or end
  binding: { dataset: ds_tickets, rows: records }
```

Each column shows exactly one of two things. `from:` is a role the bound Dataset's Machine projects with `card_fields:` (`title`,
`status`, `date`, `person`, ...) or a derived role (`created:label`, `date:label`, `group:label`); the loader refuses a role the
Machine does not declare or cannot derive, the way it does for a Collection's item. **`field:` (2026-10-17) names one Field of the
Machine by id**, so a list shows as many columns as the screen needs (a contract's number, party, category and value side by side):

```yaml
  columns:
    - { header: Contract no., field: fld_contract_no, href: record }
    - { header: Category, field: fld_category }           # a relation: the related record's label, never its id
    - { header: Contract value, field: fld_value, align: end }   # money grouped: 180,000,000
    - { header: Payment, field: fld_payment_status }       # a status: a badge toned from status_categories
```

**A Table that reads like the mockups (2026-10-18, plan U-02/A2).**

```yaml
- component: Table                       # a Dimension's rows: one per category
  label: Contracts by category
  columns:
    - { header: Category, from: "row:label", total_label: Total }
    - { header: Kind, from: "row:fld_cat_kind" }                    # a Field of the category record itself
    - { header: Instalments, from: "measure:msr_terms_paid", of: msr_terms, total: sum }
    - { header: Contract value, from: "measure:msr_value", align: end, total: sum }
  binding: { dataset: ds_contracts_by_category, rows: dimension }
- component: Table                       # records, with a total row
  label: Contracts
  columns:
    - { header: Contract no., field: fld_contract_no, href: record, total: count, total_label: contracts }
    - { header: Instalments, field: fld_terms_paid, of: fld_terms }       # "1/3" with a bar
    - { header: Payment plan, related: rel_term_plans }                    # the first plan: its name and status
```

- **`rows: dimension`** on a Table: columns read `row:label`, `measure:<id>`, and (for a Dimension over a relation)
  `row:<field>`, a Field of the related record.
- **`of:`** draws a fraction with a bar ("1/3"): beside `field:` it names the Field it is over, beside `from: measure:<a>` the
  Measure `b`.
- **`total: sum`** or **`total: count`** puts a column in the total row, **added up from the rows shown** -- so a search or a
  chip narrows the total with the list, and a Dimension's total is its rows'. `sum` writes the figure as the cells do ("Rp",
  grouping); beside `of:` it adds both ("9/36"). `total_label:` words it (alone, it is a label: "Total"). A list its
  Dataset's bound cut has no total row.
- **`related: rel_x`** shows the first record a Dataset's Relation found for the row (named by its title, else its first
  Field of its own words, never an id) and its status in its tone.
- **`cell:`** (2026-10-19, mockup PM08) makes a column's cell a **template drawn once per record**, the way a Collection's item is,
  instead of one value the column names. Anything a record template may hold goes in it -- a Tag per label with `each: tags`, an
  Avatar, a StatusBadge toned by a derived role, the completion `Toggle` -- under the same grammar (`from:`, `each:`, `when:`), so
  there is no second vocabulary for cells. A column shows exactly one of `from:`, `field:`, `related:` or `cell:`.
  **`empty:`** (beside `cell:`) is what the cell says when its template draws nothing for a record (no label, no date, nobody
  assigned: "—", "no list"); several nodes (three labels) lie in a row. **`hide_header: true`** keeps the header's words for a screen
  reader and draws none of them (the circle's column). Not on a Dimension's rows.

  ```yaml
  - component: Table                       # a Task Table: circle, title, list, labels, due date, assignee
    label: Tasks
    framed: true
    binding: { dataset: ds_task_table, rows: records }
    columns:
      - header: Complete
        hide_header: true
        cell: { component: Toggle, from: { describes: title }, binding: { write: transition } }
      - { header: Task, from: title, href: record }
      - header: Label
        empty: "—"
        cell: { component: Tag, each: tags, from: { label: title, color: color } }
      - header: Due
        empty: "—"
        cell: { component: StatusBadge, from: { label: "date:label", tone: "date:tone" } }
      - header: Assignee
        empty: "—"
        cell: { component: Avatar, size: compact, presence: present, from: { initials: "person:initials", label: person } }
  ```

- **`component: Toggle`** is a record's completion circle: a ring while open, a filled check once finished, `aria-pressed` for the
  state. It takes only `from: {describes: <role>}` (the record's name, its accessible name) and `binding: {write: transition}`;
  which Field it writes and the value are the Machine's `completion:`, written the *other* way from where the record stands, so one
  declaration serves both directions. It is not drawn for a record or a viewer the move is not allowed on, and needs a Machine that
  declares `completion:` **whose Field is the Machine's `status` Field** (the circle reads its state from one and writes the other, so
  two Fields are refused at load). A record with no name is announced as "Mark complete" rather than with an empty pair of marks.
  A checklist row and a board card draw the same circle. **The AI assistant has no slot for a Table's `cell:`, `empty:`,
  `hide_header:` or the Toggle** -- its `page:` is a fixed set of shapes (`UnwritableByTheAssistant`'s `page` entry names that), so
  these are written by hand until the assistant grows page vocabulary.
- **`person:initials`** is a derived role (beside `date:label`, `group:label`): the initials of the record's `person`, empty when
  nobody is named, and a node that reads an empty derived role is not drawn -- so an unassigned task draws no circle of "?".
- **A paragraph may take a tone** (2026-10-19): `static: paragraph` with `from: {text: "date:label", tone: "date:tone"}` is the date's words in
  that tone's ink with no badge around them (the Table's Due: an open task past its day in red, a finished one in green). A tone that
  is none of the known ones is the neutral ink, as a badge's is.
- **A Dataset may sort by the related record's column** (2026-10-19): `sort: [{field: fld_list, by: sort_order}]` orders the tasks by their
  List's own position, which no id order can say (the Table lists in the board's order); `field:` must be a reference Field and `by:` one of the
  record columns a plain sort may name (`sort_order`, `created_at`, `updated_at`). Add `{field: fld_list, by: created_at}` after it so lists
  with the same position keep the order they were made in, as the board's columns do. A record whose reference names nothing -- or names a record of another Workspace -- sorts last, ascending or descending (`NULLS LAST`). **Every declared `sort:` ends on the record's id** (2026-10-20), so rows that tie on every key come back in one order and a `limit:` cuts in the same place each time.
- **`under: fld_x`** beside `field:` draws a second line in the cell, smaller: another Field of the same record
  (`{header: Instalment, field: fld_term_title, under: fld_contract_ref}` -- K02a's "Dimas Prayoga — Week 2" over
  "CTR/K/031"). Nothing in it leaves the cell as it was. **`from: "row:number"`** shows the row's place, 1 first (K01d's
  "#").
- A row action may read a Field by id in `when:` -- `when: {role: "field:fld_plans", is: "0"}` draws "Create plan" only
  where an instalment has none.
- **Money and days:** a `money` Field may declare **`currency: IDR`** ("Rp 180,000,000"; also USD, EUR, GBP, SGD, MYR, AUD,
  JPY), and every date reads "02 Nov 2026". The derived role **`date:relative`** says the date against today ("12 days late",
  "today", "in 5 days").
- **Chips over named views:** a FilterChips bound with `rows: total` lists `options:` (`overdue=Overdue` lines) and counts
  each with `counts: {all: msr_unpaid, overdue: msr_overdue}`; the list's Dataset says what each view selects with
  **`if_param: view=overdue`** on a comparison (applied only when the request's `view` is `overdue`). A conditional comparison
sits in `all:`, compares a literal or a `$today` day, and never the viewer, a request parameter or the record's id (each would
turn a scope off when the view is not chosen); it does not count as a `viewer_scoped:` scope. A Measure that counts a view is
written beside it by hand: keep the two the same.

**A Table's `children:` are its row actions** (2026-10-17): a template drawn once per record in a last, unnamed column --
an Edit (`Popover` holding a `Form` with `binding: {write: update}`), a Delete (`Button` with `binding: {write: delete}`),
a transition -- each drawn only where that record's own rules let this viewer, exactly as inside a Collection's item:

```yaml
  children:
    - component: Popover
      label: Edit
      title: Edit contract
      trigger: secondary
      align: end
      density: regular
      children:
        - component: Form
          submit: Save
          binding: { write: update }
    - component: Button
      label: Delete
      variant: danger
      confirm: Delete this contract?
      binding: { write: delete }
```

The cell is the Field's value as the record's own page shows it. A relation or person Field is resolved for the listed records only,
in one statement per related Machine. A Field the Machine does not have, and a `group` Field, are refused at load. **A status cell,
by role or by Field, takes the tone `status_categories:` gives its option** (neutral without), the same tone the generic list and the
record's own page draw; `from: {label: status, tone: "status:tone"}` gives a Collection's `StatusBadge` that tone too. `href: record` is the only address a
cell may take and makes it a link to the record's own page. There are up to twelve columns. The cells are produced for you, so a Table's cells are produced from its columns
(its `children:` are its row actions, below), and `headers` and `aligns` are never written. A Table is bound with `rows: records`
only: a table of several Measures per Dimension (the Dashboard's shape) is not built and is refused by name. It reads `limit:` as a
window and claims nothing about completeness -- unless it says **`complete: true`** (2026-10-17, as a Collection may): then a list
the bound cut says so under its rows, with the count it was cut from ("Showing the first 100 of 340"), which costs one count only
when the bound bit. **`framed: true`** draws the table on a surface, the bordered card the mockups put a list in. **`stable`**: the
Helpdesk's All tickets page declares one.

**One record's page (`record_of:`, 2026-10-17, plan B1).** A navigation item with `record_of: mch_contract` and a `page:`
is a contract's own page (mockup K01d). The record is `$parameters.id`, which a Dataset selects with the record's own id,
and its children by their relation:

```yaml
- id: ds_contract_one
  select: records
  where: { field: id, op: equals, value: $parameters.id }   # `id` is the record's own id; equals only
  limit: 1
- id: ds_contract_terms
  select: records
  where: { field: fld_contract, op: equals, value: $parameters.id }
  limit: 100
```

A Collection over the first draws the record; inside its item template:

- `from: {text: "field:fld_x"}` reads **any Field by id** (as a Table's `field:` column does), `tone:fld_x` a status
  Field's tone and `steps:fld_x` its steps;
- **`component: Stepper`** with `from: {steps: "steps:fld_status"}` draws the lifecycle (Draft — Active — Closed): steps
  passed done, the current one marked, a `stopped` option shown only when the record is there;
- a Button with `binding: {write: transition, transition: trn_contract_activate}` **takes that declared edge** on its own
  Field, drawn only where the record stands at the edge's `from` and the viewer's roles allow it; its label defaults to the
  edge's name;
- **A move that cannot be taken back asks first.** `confirm: Cancel this contract? A cancelled contract cannot be reopened.` on a transition Button is the sentence a person reads before the request is sent. It is for an edge nothing leads out of (Cancelled, Closed); one that its opposite undoes (Activate) asks nothing, and the loader's gate refuses a question there. Name such an edge for what it does -- **Cancel contract**, never **Cancel**, which a dialog's own Cancel (dismiss) would be mistaken for (a contract was cancelled that way, 2026-10-19).
- a **Form** with `binding: {write: transition, transition: trn_plan_settle}` takes the edge **with what it asks**: the
  Fields its `to` makes required (`required_when:`), a `file` among them as a file input, in one submit (K02d: proof, then
  Paid);
- a `static: link` with **`pass: {contract: "record:id"}`** carries the record to another page's request
  (`?contract=<id>`): "+ Add instalment".

**Why an edge is not offered (2026-10-18, K01d's Status panel).** A Constraint may say **`reason:`** -- "A contract cannot be closed
while any instalment is not yet Paid." -- and a record template reads it as the derived role **`blocked:trn_contract_close`**:
the reason while a Constraint holds that edge back for this record (it stands at the edge's `from`, the viewer's roles allow
it), and nothing otherwise, so a `Callout` reading it (`from: {text: "blocked:trn_contract_close"}`) is drawn only while there is
something to say. The loader refuses a `blocked:` role naming a transition the Machine does not declare, or one no Constraint
holds back with a `reason:`.

An edge is offered only where the route would take it: the record stands at its `from`, the viewer's roles allow it,
and no declared Constraint refuses it ("Close" is not drawn while an instalment is unpaid). An edge whose target makes a
Field required is a **Form**'s, never a Button's (refused at load), and the Form does not ask again for what the record
already holds (a proof uploaded with the plan). The record id in `where: {field: id}` comes from a request parameter only.

**Master and detail on one page (K02d).** A list whose row action is a link back to *its own page* with
`pass: {selected: "record:id"}`, and a Collection under it over a Dataset `where: {field: id, op: equals, value:
$parameters.selected}` (`limit: 1`): the list alone shows no detail, and picking a row opens that record's panel below -- its
fields and its edge Form ("Mark as paid", the proof uploaded in the same submit) -- while a pick that is no record opens
nothing. No new key: the pattern is the row action, `pass:` and the request-parameter Dataset.

**A record page's own values (`$parameters.record.<field>`, 2026-10-18, K02e).** On a `record_of:` page the runtime sets the record's
scalar Fields as request parameters named `record.<field id>` (a file or group Field is left out), and a `select: records` Dataset
reads them like any parameter: `where: {field: fld_contract, op: equals, value: $parameters.record.fld_contract}` is "the records of
this PO's contract" -- the instalments a purchase order lists. The runtime owns the `record.` names: a query string never supplies one
(it is dropped), and no search box, chip or `pass:` may name one. A Field the record holds no value for sets none, so the list is empty.

**A Field of the first related record (`related: rel_x.fld_y`).** Beside `related: rel_x` (the first record's name and status), a
Table column may name one Field of it, shown as words: `{header: Planned, related: "rel_po_term_plans.fld_plan_date"}` -- the planned
date of the payment plan an instalment has, empty where it has none. The Relation is the Dataset's; the Field is the related
Machine's and is not a file, group or reference.

**Print (`write: print`).** `{component: Button, label: Print, variant: secondary, binding: {write: print}}` opens the browser's print
dialog for the page. It names no Dataset and sends nothing. What is printed is the page: the header, the application menu and every
button are left off the paper (`@media print` in `static/css/input.css`), so a page "shaped for printing" -- a purchase order -- is its
content. A status Field with no value yet has a neutral `tone:`, so a template reading it is drawn. A link drawn as a button (a row's Edit,
"Copy") is a control too and stays off the paper.

**A page as a PDF file (`write: pdf`, 2026-10-18).** `{component: Button, label: Download PDF, variant: secondary, binding: {write: pdf}}`
downloads the page it is on as a PDF: the runtime draws the same page again for the same viewer -- the same address with `/pdf`
added (`/pages/<nav>/pdf`, `/pages/<nav>/<id>/pdf`), the same query string, so a filtered list prints filtered -- and prints it with a
headless browser (`internal/pdf.Printer`), with the same `@media print` rules as the Print button and an A4 `@page`. Who may open the
page is who may download it; a refusal of the page (403, 404) is the refusal of its PDF. The file is named after the page's first
`<h1>` ("PO/2026/0087" becomes `PO-2026-0087.pdf`), else `page.pdf`. It names no Dataset and takes no `confirm`/`action`. **A host
with no browser draws no button and answers 404 on the route**: set `PDF_BROWSER` to chrome-headless-shell or Chromium, or have one on
`PATH` (`chrome-headless-shell`, `chromium`, `chromium-browser`, `google-chrome`); a path that does not exist is logged at start-up.
Two prints run at a time, each limited to 30 seconds. This is any page's output, not the purchase order's: the second render target
007 §12 and the audit's B5 call for, fed by the page the viewer already has.

**A form filled from a document (`extract:`, 2026-10-18, K01c).** A create Form can be filled from a PDF or an image that an AI
reads, a person checks, and only then is anything written. It is the same declaration whatever the document is -- a contract, an
invoice, a receipt -- because the questions come from the Fields of the Machine:

```yaml
- component: Form
  submit: Save contract
  binding: { dataset: ds_contracts, write: create }
  extract:
    file: fld_doc                 # the file Field the document is kept in once saved
    read: Read document           # the words on the button that sends the document
    review: needs review          # the words beside a value the reader inferred or could not fit
    instruction: A production contract between a film house and a cast member...   # what the document is (optional)
    fields: [fld_party_name, fld_contract_kind, fld_category, fld_value, fld_start_date]
    rows:                         # child records the document holds several of
      - label: Instalments
        machine: mch_contract_term
        via: fld_contract         # the relation Field of the rows that points at the record being created
        fields: [fld_term_label, fld_due_date, fld_gross, fld_wht]
```

What happens: the page draws a small reader (a file control and the `read:` button) above the form. Sending a document posts it to
`/pages/<nav>/extract`, the reader (Gemini, `GEMINI_API_KEY`) answers a value per listed Field with **where it read it** ("p. 3 ·
art. 5(1)") and whether it **inferred** rather than quoted, and the page opens again with `?proposal=<id>`: the ordinary form, each
control started with the proposed value, the page it was read from under it, the `review:` words beside an inferred or unfit value,
and the proposed rows as one Table of controls per kind -- a heading, one header row (`#`, each Field, `Keep`), a row each with its source under the control, and a total row that adds up the number columns from what was proposed (a tick in `Keep` = kept). A required control the reader left empty carries a `required: fill in` flag beside its label, and every required control is marked `*`. **Nothing is written until Save**, which posts to
`/pages/<nav>/extract/save` and writes the record and every kept row in **one transaction**, each through the Machine's own create
rules (Permissions, validation, numbering, constraints), then runs their on_create Events. A row the Machine refuses refuses the whole
save. The document saved is the one the proposal was read from, never one the request names. A proposal belongs to the person who
asked, on the page that asked, and is saved once.

How a value is held to its control (`domain.Fit`): a number is a number (a reader that wrote "180,000,000" is read as 180000000),
a date is a date (read as `2026-11-02`), a choice is one of the Field's options -- or, for a relation, the label of one of the
related records -- and what does not fit is left empty and flagged, never put in a control that would refuse it. Proposable
Fields are text, long text, number, money, date, status and relation; not person, file, group or boolean. The loader refuses a
`file:` that is not a file Field, a Field the form does not ask for, a `via` that does not point back at the Machine being created, and
a required Field of the rows that the declaration leaves out (a row that could never be saved). One Form per page declares it, on an
Application's page. No `GEMINI_API_KEY` means no reader is drawn and the form stays the ordinary one. Every reading writes an
`AI-EXTRACT` line and a `document_extractions` row, the ledger a usage quota would count; no cap exists yet (menata-app-document C2).
A due date that a document gives only as a period can be asked for as the period's last day, in the `instruction:` (it comes back marked for review); "14 days after signing" is not worked out. A document that yields no value at all (not the kind the form is for) is refused with a message and makes no proposal (log `AI-EXTRACT-EMPTY`). A document reads for a few seconds (3-4 s for seven pages measured against the real model).
**What leaves the host:** the document, the Fields' names and types, the instruction, and -- for a relation Field -- the labels of
the related records (up to 150 of them; a longer list is not sent, and the answer is matched to a record by its label afterwards).
A Workspace may send 60 documents an hour (`MAX_READINGS_PER_HOUR`; a failed attempt counts); the next is refused with 429.

**A link into another Application (`in:, 2026-10-18, K01d's "Create payment plan").** `{static: link, to: nav_new_plan,
in: app_instalments_payments, text: Create payment plan}` points at a screen of *another* Application of the same Workspace.
Applications install and remove on their own, so the link is drawn only while that Application is installed here and this viewer
may open it -- otherwise it is simply not there, never a 403. The loader refuses `in:` naming the page's own Application, and,
when the other Application is installed, a screen it does not have or a record page (which needs a record's id). An Application installed under a **renamed id** (the installer renames a colliding Application id) is not found by an `in:`
written for the original id: the link is not drawn, silently -- write the id the Workspace holds. Everything else
about a link (`text:`, `variant:`) is as for an own link; `pass:`/`step:`/`list_of:` do not go with it.

A create Form may take **`values: {fld_contract: $parameters.contract}`**: a Field the request names starts as that value
and is drawn as its words in a hidden input, not offered again. **`then: record`** (a create Form only) opens the record it made
(its record page when it has one). **`starts: {fld_party_name: $parameters.fld_party_name}`** (2026-10-18) begins a Field with
the request's value but leaves it a control the person may change; with a link's **`pass:`** reading the record's stored value
(**`value:fld_x`**: a relation's record id, a money Field's bare number -- where `field:` is the words a person reads) it is
**Copy**:

```yaml
- static: link                    # on the contract's page
  to: nav_new_contract
  text: Copy
  pass: { fld_party_name: "value:fld_party_name", fld_category: "value:fld_category", fld_value: "value:fld_value" }
# nav_new_contract's Form
starts: { fld_party_name: $parameters.fld_party_name, fld_category: $parameters.fld_category, fld_value: $parameters.fld_value }
```

**Which controls a create Form asks for: `fields:` (2026-10-18).** Without it a Form asks for every Field its Machine allows a form to
ask for -- not a computed, stamped, numbered, file or group Field, not a governed status that has a default, and **not a Field a
transition writes when it is taken** (`sets:`: an approval date, a payment date -- the runtime's, never typed). `fields: [fld_term,
fld_plan_amount, fld_plan_date]` on the Form narrows that to the named controls, **in that order**. The loader refuses a Field the
form would not ask for anyway, one listed twice, a required Field with no default that the list leaves out (no form could be
submitted), a `values:`/`starts:` naming a Field that is not listed, and an `extract:` proposing one. An ordinary Form that says
nothing is unchanged.

**The same key narrows an edit Form** (`write: update` inside a records template, 2026-10-19): `fields: [fld_party_name, fld_value]` is
what a person may edit there, in that order. An edit form already leaves out computed, stamped, numbered, file, group, boolean,
reference and governed-status Fields; `fields:` also leaves out one a *rollup* writes (a contract's payment status, which the
instalments keep) or one a person should not touch. A Field it leaves out needs no default -- the record holds its value. The loader refuses
an unknown Field and one listed twice. Dokter Kecil's two "Edit contract" forms say it.

A Field is fixed (`values:`) or started (`starts:`), not both. A file Field is read through **`file:fld_doc`** (its name with its
size, "ctr-c-026.pdf · 1.4 MB") and **`filehref:fld_doc`** (the address that serves it), the only roles of a file Field:
`{static: link, from: {text: "file:fld_doc", href: "filehref:fld_doc"}}` draws the Document of mockup K01d, and nothing when
the record holds no file. `carries` and `multipart` are lowered and never written. A record page answers 404 for an id that is not a record of its Machine in this Workspace, and the
Application's roles gate it as they gate the list. A status Field may declare **`categories:`** (the same five as
`status_categories:`) when it is not the Machine's lifecycle Field, so a second status (a contract's payment) has tones too.

**`component: ActionBar`** (2026-10-17) is a page's primary actions, named as a group. Put it at the root of the page and it
is drawn in the header row, opposite the heading; its links may take `variant: primary` (or `secondary`) to look like a
button while staying a link:

```yaml
- component: ActionBar
  label: Contract actions          # the group's accessible name: required
  children:
    - static: link
      to: nav_new_contract         # a navigation item, usually one with in_menu: false
      text: + New contract
      variant: primary
```

**`component: FilterChips`** and **`component: FilterSelect`** (2026-10-17) are a list's filters: chips that count what
each would list ("All 9 · Cast 3 · Crew 3"), and a drop-down for a Dimension with too many values to be chips ("All
categories"). Both are bound to an aggregate Dataset's Dimension, so a choice exists for what the data holds and its count
is the database's; both set one query parameter, which the list's Dataset reads with `optional: true` (above):

```yaml
- layout: row
  gap: tight
  children:
    - component: FilterChips
      label: Contract type            # the group's accessible name: required
      query: kind                     # the parameter each chip sets
      all: All                        # the chip that clears it, counting every record
      binding: { dataset: ds_contracts_by_kind, measure: msr_contracts_by_kind, rows: dimension }
    - component: FilterChips          # a second row of one chip: a subset of a Dimension, in your order
      label: Payment
      query: payment
      options: Overdue=Overdue
      binding: { dataset: ds_contracts_by_payment, measure: msr_contracts_by_payment, rows: dimension }
    - component: SearchBox
      label: Search contracts
      query: q
      placeholder: Search name or number
    - component: FilterSelect
      label: Category
      query: category
      all: All categories             # required: a filter you cannot clear is a trap
      binding: { dataset: ds_contracts_by_category, measure: msr_contracts_by_category, rows: dimension }
```

**Every filter keeps the others**: a chip's link, the drop-down's form and the search box carry the request's other
parameters, so choosing "Crew" keeps the words in the search box and a search keeps the chip. A relation Dimension is
named by the related records' titles and sends the id. `chips`, `choices`, `selected` and `keep` are produced from the
binding and the request and are never written.

**`component: SegmentedControl`** (2026-10-14) is a short row of mutually exclusive choices of the page itself, the selected one
marked. `label` names the group for assistive technology, `query` is the query parameter the choice travels in,
`options` lists `value=Label` pairs (two or more, values unique, no spaces or URL punctuation in a value). **Which one is
showing is the request's, never written** (2026-10-19): the choice the request carried under `query` is the selected one (the first
option when it carried none, or something that is no option), and the request's other parameters stay in every link, so choosing
Month leaves the project and the window as they were. A choice is a link to the page's own route with `?query=value`, so you
never type a route. **`stable`**: the Calendar declares one. It is the navigation shape only; a choice held in the page and
switched by script (the Builder mockup's `radiogroup`) needs `actions` and is not covered.

```yaml
- component: SegmentedControl
  label: Range
  query: range
  options: |-
    week=Week
    month=Month
```

**A Tag taken from each record (3c-2, 2026-10-08).** As the one template child of a `Collection` bound with
`rows: records`, a Tag takes both inputs from the Machine's `card_fields`: `from: {label: title, color: color}`.
The `color` role is a `status` Field whose options are the palette, so it is the one role a Tag's `color:` may
come from -- `from: {color: title}` is a load error (a title is not a palette entry), and `from: {label: color}`
loads, since `label` accepts any text. The page names no Field and no colour. A record whose stored colour is
not in the palette (data written before the Field was a `status`) still draws, in the neutral slate chip.

```yaml
- component: Collection
  gap: tight
  empty: No labels yet.
  binding: {dataset: ds_board_labels, rows: records}
  children:
    - component: Tag
      from: {label: title, color: color}
```

**A numbered list (3c-3, 2026-10-09).** A `Collection` takes `ordered: true` (or `false`; anything else is a
load error). It draws an `<ol>` and a position number before each item -- `1`, `2`, `3` -- counted by the
renderer in the order the Dataset lists its records, so the page writes no number and the numbers can never
disagree with the order. Use it when position is the content, as with a board's columns; a plain list stays a
`<ul>`. The number is hidden from assistive technology because `<ol>` already announces each item's position.

```yaml
- component: Collection
  gap: tight
  ordered: true
  empty: No lists yet.
  binding: {dataset: ds_board_lists, rows: records}
  children:
    - static: link
      from: {text: title, href: record}
```

**A form that creates a record (T1, 2026-10-09; 007 §11.3 write side).** A `Form` with
`binding: {dataset: <id>, write: create}` asks for one new record of the Machine that Dataset reads, and submits it
through the runtime's own generic create route (`POST /machines/<id>/records`) -- the same route a Machine's list
page uses, so defaults, computed Fields, stamps, the `create` Permission, validation and `on_create` events all run
exactly as there. The page names **neither the route nor a single Field**: the controls are the Machine's own
askable Fields (text, long text, number, date, checkbox, select with its declared options), in declared order, each
with its label and a `required` mark taken from the Field.

```yaml
- component: Form
  submit: Add label            # the button's words; required
  binding:
    dataset: ds_board_labels   # must be installed in this Workspace; the Machine it reads is the one written
    write: create
```

Things a Form refuses, at load: `write:` with `rows:`/`measure:`, a Dataset that is not installed, `children:` (the
fields are derived, never written), a typed `action:` or `component: Input`, and a Machine whose **required** Field
is one a form cannot ask yet (a reference to another Machine, a group). An *optional* Field of that kind is simply
left out. **Who sees it:** a viewer without the Machine's `create` Permission gets no Form at all (nothing is hidden
by CSS), and the route still refuses a forged post. **When a post is refused** (validation 422, permission 403) the
shell's shared error notice says so and the form keeps what was typed; a success refreshes the page, so the new
record appears in the list beside it. Not built: reference Fields (T4).
**Installing copies**, so add the Form to your Workspace's own copy of the Application.

**A form that edits the record beside it (T3a, 2026-10-09).** Inside the item template of a records `Collection`,
a `Form` with `binding: {write: update}` and **no dataset** edits *that item's* record: it starts as the record's
own values and PATCHes the runtime's generic route (`PATCH /machines/<id>/records/<rid>`), so the state model, write
guards, `edit` Permission and events run exactly as on the record's own page. The page names neither the route nor a
Field; the controls are the Machine's own editable Fields, in declared order.

```yaml
- component: Collection
  binding: {dataset: ds_board_lists, rows: records}
  children:
    - layout: stack
      children:
        - static: link
          from: {text: title, href: record}
        - component: Form
          submit: Rename
          binding: {write: update}     # no dataset: the record is the Collection's current one
```

Refused at load: `update` outside a records Collection's item template (there is no record to edit), `update` with a
`dataset:` (it would be a second Collection, a join), and a Machine with nothing an edit form can ask for. **What an
edit form leaves out, and why:** computed, stamped, file, group and reference Fields, a **status that `transitions:`
names** (it moves only along those edges, through the Action that names one), and **checkboxes** --
an unticked box sends nothing, and the route reads "nothing" as `false`, so a form that did not mean to touch a
checkbox would clear it. A status **no** Transition names is an ordinary choice and is asked for as a select of its
options -- a Label's colour is the worked example (since 2026-10-11). The route changes only the Fields the request names. **Who sees it** is decided per record
(`edit` Permission evaluated against that record's values; an append-only Machine is never offered one), the route
still enforces.

**A button that deletes the record beside it (T3b, 2026-10-09).** In the same place, a `Button` with
`binding: {write: delete}` and a written `confirm:` sends `DELETE /machines/<id>/records/<rid>` for *that item's*
record, after the browser has asked the `confirm` sentence. State what will happen in it: a delete with no
consequence named is a load error.

```yaml
        - component: Button
          label: Delete
          variant: secondary
          confirm: Delete this list? This cannot be undone.
          binding: {write: delete}     # no dataset, like update
```

Refused at load: `delete` outside a records Collection's item template, `delete` with a `dataset:`, a Machine that
is append-only, no `confirm`, and a typed `action`, `method`, `name` or `value` (the route and the verb are the
runtime's). **Who sees it** is decided per record (the Machine's `delete` Permission against that record's values,
plus the business-state rule the generic route applies -- an approved Document, a decided step -- so a record that
route would refuse gets no button); the route still enforces, and a refusal shows through the shell's error notice.
A successful delete refreshes the page, so the list is re-read without the record. It is the same route the record's own
page uses, so whatever deleting does to related records happens here too; word `confirm` for that.

**A button that moves the record beside it (T5, 2026-10-10).** A `Button` with `binding: {write: move}` and
`direction: up` or `direction: down` sends `POST /machines/<id>/records/<rid>/move?direction=<d>` for *that item's*
record. The route finds the neighbour itself, so the page names a direction and nothing else; it asks nothing (a
move is undone by its opposite), so `confirm` is refused.

```yaml
        - component: Button
          label: Move up
          variant: secondary
          direction: up
          binding: {write: move}       # no dataset, like update and delete
```

Refused at load: `move` outside a records Collection's item template, with a `dataset:`, over an append-only
Machine, a `direction` that is not `up`/`down`, a typed `action`, `method`, `name`, `value` or `confirm`, and **a
Collection whose Dataset declares a `where:` or a `sort:`** -- a move changes the Machine's own order
(`sort_order`), and a filtered or re-sorted list shows a neighbour the move would not step over. **Who sees it** is
per record (the `edit` Permission against that record's values), and **there is no Up on the first record and no
Down on the last** -- a button that cannot do anything is not drawn (when `limit:` truncates the list, the last
shown record keeps Down). A successful move refreshes the page; whatever else reads the Machine's order (a board's
columns) follows.

**A button that moves the record's status (K13, 2026-10-10).** A `Button` with `binding: {write: transition}` and
`becomes: <target>` sends `PATCH /machines/<id>/records/<rid>` for *that item's* record, writing the Machine's
**status Field** (its `card_fields` entry with role `status`) with `<target>`. `becomes` is an option of that Field,
or `$done` / `$reopen`, which read the Machine's own `completion:` (the finished value, and the Field's default or
first option), so the page names neither a Field nor a literal status. The route is the generic edit route, so its
Permission, `transitions:` edges and events are the only rule; a successful move refreshes the page. It asks nothing,
so `confirm` is refused.

```yaml
        - component: Button
          label: Mark done
          variant: secondary
          becomes: $done
          binding: {write: transition}   # no dataset, like update, delete and move
```

A list repeats one label per row, so a screen reader listing the buttons would hear "Mark done" once per task. Add
`from: {describes: title}` (any projected role) and the button's accessible name becomes `Mark done: <the task's
title>` (`aria-label`). It is the only property a transition Button may take from its record, and
`conformance.TestEveryInstalledButtonIsADeclaredRecordWrite` requires it of every installed one. (`delete` and `move`
Buttons do not take it yet; the renderer would draw it, the lowering has not been opened for them.)

```yaml
        - component: Button
          label: Mark done
          variant: secondary
          becomes: $done
          from: {describes: title}
          binding: {write: transition}
```

Refused at load: `transition` outside a records Collection's item template, with a `dataset:`, over an append-only
Machine, over a Machine with no `status` role over a Field that has options, a `becomes` that is not an option, a
`$done`/`$reopen` over a Machine with no `completion:`, and a typed `action`, `method`, `name`, `value` or `confirm`.
**Who sees it** is per record: a button is drawn only where the move is a change the viewer may make -- not toward
the value the record already holds, not where the Machine's declared edges refuse it, not where the viewer's `edit`
Permission does not reach that record. (A Machine with no `transitions:` edges would offer "Reopen" on every status
but the reopen value; say when a node is visible with `when:`, next.)

**A node that is drawn only for some records (K11, 2026-10-10).** Any node inside a records Collection's item template
may carry `when: {role: <card_fields role>, is: <value>}` or `is_not: <value>` -- exactly one of the two. The node, and
everything under it, is drawn for the records the test holds for and not for the others. The operand is a literal or
`$done` / `$reopen`, the same two words a transition's `becomes:` takes, read from the Machine's `completion:`.

```yaml
        - component: Button
          label: Reopen
          becomes: $reopen
          when: {role: status, is: $done}     # only on finished tasks
          binding: {write: transition}
```

Refused at load: `when:` anywhere but inside a records Collection's item template, a role the Machine does not project, a
literal that is not an option of the role's Field (when it has options), `$done`/`$reopen` over a Machine with no
`completion:` or over a role other than the completion Field, and both `is` and `is_not` or neither. **What it is not:** one
role and one comparison -- no `and`/`or`, no comparing two roles. It hides a node; it is not a permission --
a Button's own rule (Permission, `transitions:`) still decides whether it is drawn for that viewer.

**A page the runtime owns (2026-10-17).** The screens in `domain.RuntimeScreens` (Home, Members, Groups, Notifications...) exist in every Workspace and belong to the runtime, so no manifest lists them. A screen can still have its body declared: a file in `internal/metadata/runtimepages/<nav id>.yaml` names the screen and gives `open_to:` and `page:` (nothing else: its route, label and heading are the screen's, declared once in Go). It is written exactly like any other page, and a Workspace gets it when it declares every Dataset the page binds (the shared notification Machine brings `ds_my_notifications`; a manifest without it has no such page and the route is 404). You do not write one for an Application. Three things it uses that any page may:

- **`from: {href: link}`** on a link in a records template takes the destination from the Machine's `card_fields` role `link` (a text Field holding a route). The runtime hands over only a route inside the app; anything else (`https://...`, `//host`) makes the link plain words.
- **`from: {text: "created:label"}`** is when the record was created, for any Machine.
- **`binding: {dataset: ds_x, write: transition, all: true}`** on a `Button` with `becomes: <option>` moves every record of the Dataset the viewer may move, outside any template. It is drawn only while something would move, and the route applies the same rules as moving each record alone.

**A runtime page may also perform the runtime's own operations and draw its settings navigation (W3, 2026-10-17).** Three more things only the runtime's own pages use, or that exist for them:

- **`binding: {write: action, action: create_group}`** on a `Form` (it asks what the action asks) or on a `Button` inside a Collection over a runtime source's Machine (`action: delete_group`, with a `confirm:` sentence). Nothing else about the request is written; the closed set of actions is `registry.RuntimeActions`. A Workspace's or an Application's page that names one is a load error.
- **`component: NavList`** with `section: workspace_settings` and `variant: sidebar|page`: the grouped list of the settings screens, one marked as where the viewer is. You write neither the rows nor the label: they come from the runtime screens that declare themselves part of the section. Put the sidebar in `layout: split` with `side: start` and `aside: compact` (220px from `sm` up, not drawn on a phone).
- **`anchor: <word>`** on a `layout: stack` or `section` makes it the target of a `#fragment` link (drawn as one wrapping element holding the id); the word is lowercase letters, digits and hyphens, starting with a letter.
- **`component: SearchBox`** with `label`, `query` (the key; `q` for the members and invitations) and `placeholder`: a GET form that sends its words back to the page; the runtime source the page reads narrows from the key, and the box keeps the words the request carried. It never narrows by itself. With `to: <navigation item id>` it sends its words to that screen instead (a typed `action` is refused), and `submit: <label>` adds a button (a box with a button takes the width that is left; a box alone keeps its fixed width): Home's "Describe the application you need" box is one, sending `idea` to the new-application screen.
- **`from: {text: subtitle}`, `from: {text: detail}` and `from: {text: summary}`**: two plain lines and a longer sentence a Machine's `card_fields` may give a record under its title.

**A runtime page for one record (W4, 2026-10-17).** A runtime screen that is one record's page names its `RecordSource` in `domain.RuntimeScreens` and is served at a route carrying the record's id; the page reads it as `$parameters.id` in a Dataset (`where: { field: fld_id, op: equals, value: $parameters.id }, limit: 1`), and an id the source does not hold is a 404. Put the whole body in a `Collection` with `block: true` (each item one full-width column, not a run of inline parts), and a record action (`delete_group`) or a record Form (`set_group_members`, `set_group_roles`) inside it is about that record. An action's input of kind `members` or `app_roles` is a checklist or a role select per Application that the runtime fills for that record; the page writes neither.

**A node drawn only for some viewers (P-b, 2026-10-16).** A node anywhere on a page, not only inside a template, may carry
`when: {viewer: workspace_role, is: admin}` (or `is: member`): it, and everything under it, is drawn for a viewer whose
Workspace role is that one. Three rules, all checked at load. It takes `is:` and **never `is_not:`**: a viewer the runtime
cannot place (no membership row) holds no role, so a negation would show them what only a role should see; the condition
fails closed. The only fact a page may ask is `workspace_role`, and the only values are `admin` and `member`: an Application
role is not offered (a Workspace page has none to ask). And a node it hides is **not lowered at all**, so a Dataset it binds is
never read for a viewer who would not see it. It is the same kind of condition as `when: {parameter: week, ...}` -- a statement
about the request -- and, like it, it is not a permission: it decides what is drawn, not what a viewer may open or write (a Workspace page's `open_to:` decides who opens it).
Used by the Helpdesk's All tickets page, which links to the Workspace overview for admins only.

```yaml
        - static: link
          to: nav_workspace_overview
          when: { viewer: workspace_role, is: admin }
```

An Application's page may link (`to:`) to its own menu and to the Workspace's own pages, never into another Application.

**One node per tag of the record (`each: tags`, K15, 2026-10-10).** Inside a records Collection's item template,
a node may carry `each: tags` and is then drawn once for every tag the current record owns -- the Machine's
`card_tags:` (the join Machine's Labels) -- or not at all when it has none. The node takes `from:` as usual, but the
roles are the **tag's**, not the record's: `title` (the Label's name) and `color`.

```yaml
- component: Collection
  binding: {dataset: ds_my_tasks_overdue, rows: records}
  children:
    - layout: row
      gap: tight
      children:
        - component: Tag
          each: tags
          from: {label: title, color: color}
```

Refused at load: `each:` outside a records template, a value other than `tags`, a Machine with no `card_tags:`, a
`from:` role other than `title`/`color`; refused at lowering: an `each:` node with no `from:` (it would repeat
identically), one that also carries `binding:`, and one inside another `each:`. A node under `each:` holds no write of its own, so a
Button there cannot act on the parent record. The tags of every listed record are read in two statements, however many records.

**A date pill from the record's date (K15, 2026-10-10).** Inside a records Collection's item template, a `StatusBadge`
may read two **derived** roles, `"date:label"` and `"date:tone"`, instead of projecting a Field: the short date ("2 Jan",
with the year when it is not this year's) and the tone the runtime works out from it -- `bad` once an open record is past due,
`good` once the record is finished (the Machine's `completion:`), `neutral` otherwise. The page writes none of that rule.

```yaml
- component: StatusBadge
  from:
    label: "date:label"
    tone: "date:tone"
```

The Machine must declare a `date` role in `card_fields:` (a load error otherwise), and the colon-quoted names are reserved
-- they are never `card_fields` roles. **A record with no date draws no pill:** these two roles are the only ones whose
empty value leaves the node out; an empty `title` or `container` still draws its empty string. The day the rule is judged
on is the request's, not the renderer's clock (007 §4.6). Not built: a pill for a different Field than the `date` role,
and any wording such as "3 days left" (the pill shows the date only).

**The list a record sits in (`"group:label"`, K15, 2026-10-10).** The third derived role is the name of the group the
record falls under in its Machine's *default View* (`views:`, first entry, `group_by:`): the title of the record a
relation points at (the List a Task is in), or the stored option itself when the Field is not a reference. It is read the
same way as the date roles, and like them a record with no group leaves the node undrawn.

```yaml
- component: StatusBadge
  tone: neutral          # written, because a badge requires one and a group has no tone of its own
  from:
    label: "group:label"
```

Refused at load when the Machine's default View groups by nothing. The titles are read in one statement per related
Machine, however many records are listed.

**A count with plural text (3c-4, 2026-10-09).** A `static` node inside a records Collection's item template
may take `count:` instead of `text:`: the number of children a Relation (007 §7.5) found for *that record*,
with the author's own words around it.

```yaml
# metadata/label.yaml -- the Dataset declares the Relation; the page only names it
datasets:
  - id: ds_labels_with_cards
    select: records
    limit: 200
    relations:
      - {id: rel_label_cards, machine: mch_card_label, via: fld_label}

# the page
- component: Collection
  binding: {dataset: ds_labels_with_cards, rows: records}
  children:
    - static: caption
      count:
        of: rel_label_cards          # a Relation of the Dataset the Collection is bound to
        one: used on {n} card        # optional; serves exactly 1
        other: used on {n} cards     # required; serves 0 and every number above 1; must carry {n}
```

The page writes the words and never the number, so `text:` (written or `from:`) beside `count:` is a load error,
as is `count:` outside a records template, on a non-`static` node, or with an `of:` the bound Dataset does not
declare. `{n}` is the one substitution; there is no expression and no arithmetic (007 §9.2). Two wordings are
what English and Indonesian need -- a language with more plural categories is a capability no one has asked for.
**Cost:** the Relation is a lookup (one bounded query for the children of every listed record), and it loads the
child rows to count them; that strategy is invisible to metadata and may become a `COUNT ... GROUP BY` later.
**Installing copies**, so the Dataset and its Relation must be in your Workspace's own Machine copy.

**A Relation can count only some children: `where:` (K15 Dashboard D3, 2026-10-11).** A Relation may carry the same
one-level `all:` / `any:` conjunction a records Dataset does, over the **child** Machine's Fields, so "open cards"
is a count beside "all cards" and a parent with none still has a row (a Dimension over the children cannot list it):

```yaml
# metadata/project.yaml
datasets:
  - id: ds_projects_with_tasks
    select: records
    limit: 200
    relations:
      - {id: rel_project_tasks, machine: mch_task, via: fld_project}
      - id: rel_project_open_tasks
        machine: mch_task
        via: fld_project
        where: {field: fld_status, op: not_equals, value: $done}
```

Two context values resolve: `$done` (the child Machine's own `completion:` value -- never type the status) and
`$today` (the request's date). Anything else (`$current_user`, `$parameters.*`) is a load error, because a Relation is
read once per page with no viewer to answer it. A field the child lacks, an option it does not declare, or `$done` on a
child with no `completion:` is a load error too. Relations over the same child Machine and `via:` share one read.
`$today` resolves only on a declared `page:` (the one reader that supplies the request's date); a Dataset also read by
the approval engine's or My Tasks' own screens must not use it in a Relation `where:` -- those callers pass no date, and
the read fails with an error rather than keeping the wrong children.
**Installing copies**: the Dataset lives in the *parent's* Machine file, so every Workspace that installed the page needs it there.

**A truncation notice, only when you claim it (3c-5, 2026-10-09).** A Dataset's `limit:` is a safety cap in
one Dataset and a window in another, and nothing on the Dataset says which -- so the runtime never guesses. A
records `Collection` over a Dataset whose `limit:` is only a cap may say so itself:

```yaml
- component: Collection
  complete: true            # "this list shows every record its Dataset matches"
  binding: {dataset: ds_board_labels, rows: records}
```

`complete: true` is **your claim**, not a measurement. When the claim holds and the Dataset's bound actually cut
the list, the Collection draws a notice after it ("showing the first 200") and the number is the Dataset's own
`limit:`; when nothing was cut, nothing is drawn. Never write `complete: true` over a window (`ds_recent_documents`'
`limit: 5` is "the latest five" -- a notice there would say the list is incomplete when it is exactly what was
asked for); `internal/conformance`'s `windowDatasets` names the Datasets known to be windows and
`TestNoPageClaimsCompletenessOverAWindow` refuses the combination. `complete:` takes `true` or `false`, is a load
error on any node but a Collection bound with `rows: records`, and `truncated:` is not a key you can write -- it
is what the runtime puts on the lowered tree when the claim and the cut both hold. **Installing copies**, so the
Dataset's `limit:` in your Workspace's own Machine copy is the number the notice shows.

**One surface, rows ruled between (2026-10-09).** A `Collection` takes `divided: true` (or `false`; anything else
is a load error). Instead of one loose tile per item it draws **one** bordered surface whose items are separated by
a thin rule -- the look of a settings list. It combines with `ordered: true` (the position number sits in the row's
first column) and takes its colours from the Theme (`border.surface`, `border.divider`), so a page names no class.

```yaml
- component: Collection
  ordered: true
  divided: true
  binding: {dataset: ds_board_lists, rows: records}
```

**Roomier rows (2026-10-22).** A divided `Collection` takes `density: roomy` (or `regular`, the default; it needs `divided: true`): the row's
padding is 20px by 14px instead of 16px by 12px, for a list whose rows carry an icon tile and two lines -- Workspace Home's Applications. The
mockups' Applications and Members lists share the 20px sides; the word is the one `layout: panel` and `Popover` already use.

**A run of chips (2026-10-22).** A `Collection` takes `inline: true`: its items sit side by side and wrap, one inline part each, instead of
stacking -- a row of suggestions. It is not also `ordered`, `divided` or `block` (a load error). With `variant: chip` on a `static: link` each
item is the same bordered pill a filter chip is, so a suggestion and a filter are one look:

```yaml
- component: Collection
  gap: tight
  inline: true
  binding: {dataset: ds_runtime_application_suggestions, rows: records}
  children:
    - static: link
      variant: chip
      from: {text: title, href: link}
```

**What it is not.** It does not migrate an existing screen: a bespoke Go screen stays bespoke until its
own route is replaced, and the screens declared this way are new ones. Writes (`Form`,
`Button` with `write:`), request-parameter filters (`$parameters`) and ordered comparisons (`$today`) are
declared in the sections that follow, each on its own terms. **Installing copies**, so a Dataset a `page:` binds must exist in the Machine your
Workspace's own copy declares; the loader refuses the Workspace otherwise.

### 12.2 A Machine file

| Key | Value | Notes |
|---|---|---|
| `id` | `mch_*` | Required, regex-gated |
| `name` | string | Required |
| `fields[]` | list | See 12.3 |
| `constraints[]` | list | See 12.4 |
| `blocks_member_removal[]` | list | See 12.4a |
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
| `compute` | `{op, fields[]}` | Makes a `number` field computed (2026-09-30): never an input, recalculated on every save through the generic create/edit routes. `op` ∈ `domain.KnownComputeOps` (`sum`); `fields` are number fields of the same machine that are not computed themselves. Cannot be `required` or have a `default`. Refused at load on a machine a workflow engine casts, or on `mch_user`/`mch_activity`/`mch_notification`, because those are written by Go routes that do not compute |
| `numbering` | `{pattern, codes}` | A number the runtime assigns (2026-10-18, plan A4) to a `text` Field when the record is created: `CTR/C/026`, `PP-0146`, `PO/2026/0087`. `pattern:` is literal text, `{fld_x}` (a text or status Field of the record: its value, or its `codes:` letters -- `{Cast: C, Crew: K, Item: I}`), `{year}` and exactly one `{seq:N}` last: the counter, N digits wide (1-9), one past the larger of the last number handed out and the highest still used **under the same text before it** -- so `CTR/{fld_contract_kind}/{seq:3}` counts each kind separately, and `PP-{seq:4}` counts every plan. Nobody types it: no form asks for it, a client's value is overwritten on create, an edit leaves it (a change is refused like `set_once`), and a copy gets its own. Two creates at once queue on the Field's lock and never write one number; a number is never handed out again: the counter (`number_counters`) only goes up, so deleting a record does not free its number. Not with `compute`/`stamp`/`default`; refused where Go writes the Machine itself. The loader refuses an unknown Field, a figure Field, a computed one, a missing counter, a counter that is not last |
| `stamp` | `current_user` | Makes a `person` field server-written (2026-10-05): when a record is created through the generic create routes the field is set to the acting user, whatever the request said; it is never an input, and an edit cannot change it. Cannot have a `default` or be `compute`d. Refused at load on a machine a workflow engine casts, or on `mch_user`/`mch_activity`/`mch_notification`. Needed by the `comment` card role, which is how a comment says who wrote it without trusting the client |

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

### 12.4a `blocks_member_removal[]` — the same shape, gating a person's removal instead of a field

```yaml
blocks_member_removal:
  - id: blk_*
    actor_field: fld_*         # a `person`-type field on THIS machine
    condition: { field: fld_*, op: equals|not_equals, value: <string> }
    reason: "..."               # required; shown to the admin attempting the deactivation
```

Reads as: *block deactivating `actor_field`'s person while any record of this Machine matches
`condition`* — `internal/composition.BlockingReasonsForMemberRemoval` sweeps every Machine
declaring this key before `submitDeactivateMember` (`internal/web/workspacemembers.go`) commits,
the general primitive behind the Flow 2 canvas re-audit's member-deactivation gap (2026-09-27; see
`metadata/approval_step.yaml`'s own `blk_step_pending` for the real declaration — a person cannot be
deactivated while assigned a still-pending Approval Step). Declared once so far, but general by
construction, not by a second-real-case generalization (CLAUDE.md's own decomposition-criteria
rule, deliberately overridden here on owner instruction): any Machine with a `person`-type field can
declare its own block using the same `condition` vocabulary `constraints[]` already uses.

### 12.5 `events[]` — three mutually exclusive shapes

```yaml
events:
  - id: evt_*
    on: fld_*              # shape A: field-change
    when_equals: <value>   #   optional; omit = any change to that field
    then: { service: log_activity, summary: "...", summary_override_when: <value>, summary_override: "..." }

  - id: evt_*
    on_create: true        # shape B: record creation. Never combined with `on`
    then: { service: log_activity, summary: "..." }

  - id: evt_*
    schedule:                # shape C: the passage of time, not a write. Never combined with on/on_create
      date_field: fld_*        # a real date Field
      when: overdue             # closed vocabulary; only value today
      guard_field: fld_*          # optional, must be set together with guard_equals or guard_not_equals
      guard_equals: <value>       # or guard_not_equals: <value>, never both
    then: { service: send_notification, recipient_field: fld_*, preference_key: <value>, summary: "..." }
```

`summary` placeholders: `{old}`, `{new}`, and any `fld_*` id in braces. Not a templating language —
that is the whole list. At most one override. `{old}`/`{new}` are meaningless on `on_create` or
`schedule` (no prior value to compare against); only `{field_id}` placeholders make sense there.
`schedule:` is evaluated periodically (`internal/execution.RunScheduledEvents`, on a ticker, never
inside a request) against every record of the declaring Machine — see `metadata/document.yaml`'s
own `evt_document_overdue_notify` (§10).

**The third Service, `send_notification`** (`domain.Notify`) — writes an in-app `mch_notification`
record and, if the recipient's own preference allows it, an email. Usable from any of the three
Event shapes above, not just `schedule:`:

```yaml
then:
  service: send_notification
  recipient_field: fld_*        # a Field on THIS record — no cross-record resolution
  preference_key: assigned      # one of KnownNotificationPreferenceKeys (12.8)
  summary: "..."
```

**Telling a role or a Group (2026-10-16).** Name exactly one recipient: `recipient_field` (the person a person Field
holds), `recipient_role` (everyone who holds that role of the Application that claims the Machine), or
`recipient_group_field` (every member of the Group a `group` Field of the record holds):

```yaml
then: { service: send_notification, recipient_role: koordinator desain, preference_key: assigned, summary: "Request baru: {fld_title}" }
```

A role holder is anyone who has it directly or through a Group, counted once; a deactivated member and a member of another
Workspace are not told. The role must be one the Application declares in `roles:` (a load error otherwise, as for a
Permission's), and a Group Field must be of type `group`. Each person's own preference still decides whether the email follows;
the in-app notice is always written. To tell two different audiences, write two Events. The assistant writes all three
recipient shapes.

**The other Service, `rollup_parent_status`** — a child's own field change decides its parent's
status. Declared on the child, because that is where the relation to the parent already is:

```yaml
events:
  - id: evt_*
    on: fld_decision                  # the field whose change triggers it, read on every sibling
    then:
      service: rollup_parent_status
      parent_field: fld_document      # a relation/person field on THIS machine
      target_field: fld_status        # a field on the PARENT machine
      any: { value: rejected, set: rejected }   # one child holding it decides immediately
      all: { value: approved, set: approved }   # only when every child holds it
      default: in_review              # everything else, including no children yet (optional)
```

`default` is optional. Without it, a change neither `any` nor `all` decides leaves the parent's Field as it is. Use that when
something else also writes the Field: a payment plan that is only submitted must not undo the Overdue a schedule put on its
instalment, so that rollup declares only `any: { value: Paid, set: Paid }`.

**The fourth Service, `composite_signed_document`** (`domain.Composite`, 2026-09-28) — burns every
approved step's signature image, plus the approval-status banner, onto the parent document's PDF and
stores the result. Declared on the child for the same reason a rollup is, and it reuses the same two
keys:

```yaml
events:
  - id: evt_*
    on: fld_decision
    when_equals: approved
    then:
      service: composite_signed_document
      parent_field: fld_document      # a relation/person field on THIS machine
      source_field: fld_file          # a file field on the PARENT — the original, composited onto
      target_field: fld_signed_file   # a file field on the PARENT — where the result goes
```

Source and target must both be `file` Fields on the parent, and must differ: compositing always starts
from the original, so writing the result back over it would make each run composite onto the previous
output. It recomposites from scratch on every approval, which is why firing on each one is correct
rather than merely tolerable.

What this declares is *which records and Fields*, never how. The PDF work itself is runtime-owned
(002). It used to add an unstated requirement — a step's signature image and placement were read by
their own ids, so a Machine pointed at this Service still had to name its Fields the way the template
library does. Since §12.6d's `signature_placement:` that requirement is a declaration, and the Service
reads whatever the step Machine says.

Priority is part of the rule: `any` is checked first, so a single rejection decides the parent
without waiting for the remaining children. Declaring only one of `any`/`all` is fine; declaring
neither leaves the parent permanently at `default`, which fails validation. `set` and `default`
must be options of the *parent's* `target_field`, and `any.value`/`all.value` options of `on`.

### 12.6 `permissions[]` — two arms, combinable

```yaml
permissions:
  - id: prm_*
    action: decide|create|edit|delete   # closed set
    roles: [role, role]                 # any ONE of this Application's declared roles
    workspace_role: admin               # the Workspace namespace; admin is the only value
    actor_field: fld_*                  # a person/relation field on this Machine
```

Reads as: *someone holding one of `roles`, and the Workspace role if one is named, who is also
the person named in `actor_field` on this record, may perform `action` on it.* Every arm is
optional; **all of them may not be** — a Permission gating on nothing fails the load rather than
silently protecting nothing. On `action: create` the values checked are the ones being submitted,
so `actor_field` there reads "the record must name you" (§8). A Machine declaring no
permission for an action leaves it open to any authenticated member.

`roles` names words from the vocabulary of the Application that claims this Machine (§2); a word
that Application does not declare, or a `roles:` on a Machine no Application claims, fails the
load. What is matched is the member's *effective* roles — direct assignment ∪ every role any
Group they belong to holds there.

Several Permissions on one action are **requirements** (all must pass); several roles within one
Permission are **alternatives** (hold any one).

### 12.6b `transitions[]` — a status Field's declared state model

```yaml
transitions:
  - id: trn_*
    name: Approve          # required -- the business name, no screen can derive it
    field: fld_*           # a status field on this Machine
    from: pending          # both must be options that field declares, and differ
    to: approved
    action: decide|edit|delete   # omit for a move the runtime performs itself
```

Machine-level keys not shown above that belong to the same question: `append_only: true` (§8.0).

Opt-in per Field: a status Field no transition names still moves freely. Once one names it, any
move that is not a declared edge — or is declared for a different `action:` than the route
performing the write — is refused with `422`. Declaring the same `field`/`from`/`to` twice fails
the load, since which action governed it would depend on declaration order.

### 12.6c `actions[]` — what a named Action writes

```yaml
actions:
  - action: decide
    writes:
      - field: fld_decided_by_name
        from: actor_name
```

| Key | Value | Notes |
|---|---|---|
| `action` | one of `decide`, `create`, `edit`, `delete`, `revise` | At most one entry per Action per Machine |
| `writes[].field` | `fld_*` | Must be a Field this Machine declares |
| `writes[].from` | `submitted` \| `actor` \| `actor_name` | The value the request carried, the acting identity's record id, or their display name **as it stands now** (a snapshot — `fld_decided_by_name` is printed onto a signed PDF) |
| `writes[].value` | string | A literal instead of a source. Mutually exclusive with `from`; validated against the Field's own `options:` |

**The status move is not declared here.** A `transitions:` edge naming this Action already says which
Field it moves and to what, and the runtime reads it from there — so an Action that moves a status
needs no `writes:` entry for it, and the route accepts exactly the values those edges name. Declare a
literal `value:` only for a Field whose state model deliberately declares no person-performed edge
(`mch_document`'s `fld_status`, derived from its Approval Steps).

There is no `now` source. Nothing in this repo writes a Field from the clock yet; the day a Machine
declares such a Field is the day it earns one.

### 12.7 `views[]`, plus the two Machine-level keys that are *not* per-View

`views[]` — zero or more arrangements of this Machine's own records, each addressable by id
(`?view=vw_...`). No block at all means one implicit table.

| Key | Value | Notes |
|---|---|---|
| `id` | `vw_*` | Required, unique within the Machine |
| `name` | text | Required, and deliberately **not** derived from `type` — the viewer reads it |
| `type` | `table` \| `board` \| `cards` | Closed set (`domain.KnownViewKinds`). `cards` requires `card_fields` below |
| `group_by` | `fld_*` | **Required when `type: board`**, rejected on any other type |

`sla_field` and `card_fields[]` are **Machine-level, beside `fields:` — not inside a View**. They
describe how this Machine's records look *wherever* they appear, and the record detail page selects
no View at all, so putting them on one arrangement would have forced an arbitrary choice:

| Key | Value | Notes |
|---|---|---|
| `sla_field` | `fld_*` | Must be a `date` field. Renders as OVERDUE / "N days left" |
| `card_tags` | `{machine, via, tag}` | `machine` an installed join Machine; `via` a reference on it pointing back at this Machine; `tag` a reference on it naming the tag. The tag Machine needs `card_fields` with `title` and (for colour) `color` roles. Read by a board card's chips | Also drawn as chips under a record's own heading on its detail page. A Workspace that carries `mch_activity` and its `ds_record_activity` Dataset also gets a **History** section there (events naming this Machine and record, newest first, at most 50) — nothing to declare per Machine
| `plural` | text, a word or two on one line (at most 40 characters) | What a board says of several of this Machine's records: `plural: tasks` -> "3 tasks", "Its 3 tasks move to Other". **Declared, never guessed** (an `s` makes "categorys"); a Machine that declares none is spoken of as "items". Singular is the Machine's name, lower-cased |
| `completion` | `{field: fld_*, done: <option>}` | `field` must be a `status` field and `done` one of its options. Read by a board card's date pill (`composition.IsComplete`). A Machine that is a *child* of another (a relation to it) and declares this plus a `title` card field is drawn on the parent's detail page as a **checklist** — "2 of 4", a completion circle per item, an add-item input — rather than a table (`mch_checklist_item`); declare nothing else |
| `status_categories` | `{field: fld_*, values: {<option>: start\|active\|attention\|done\|stopped}}` | `field` must be a `status` field; each key one of its options; each value one of the five categories. Every state a `transitions:` edge on that Field starts or ends in must be categorised, and where `completion:` is declared on the same Field its `done` option is the one marked `done`. Every status badge takes its tone from it (the list, a page's Table, the record's own page); the process map will read it too. Categories are business words, not colours |
| `card_fields[]` | `{field: fld_*, role: title\|person\|money\|status\|date\|color\|file\|comment\|container}` | Projection (007 §7.6). Consumed by any `cards` View (`mch_document`'s `vw_document_cards`), and by `internal/composition` for any composed screen that renders a record outside a View — My Tasks, the Calendar week and the Dashboard's Project rows all read their title/status/date from here since 2026-09-28, which is what took them out of `internal/conformance`'s projection ratchet |

**There is no singular `view:` block any more.** It was one anonymous arrangement per Machine until
2026-09-20; a second arrangement of the same records could not be expressed at all. If you write
`view:` today, nothing validates it and nothing reads it — the loader accepts the file and the
Machine simply has no declared View (see "What this can't do yet": unknown keys are ignored, not
rejected).

### 12.6a `sequencing` — records acted on in order

One optional block per Machine. Declaring it means a record is locked while a sibling earlier in
the order is still open — but only when the parent record's mode says so:

```yaml
sequencing:
  parent_field: fld_document      # a relation/person field on THIS machine; siblings share it
  mode_field: fld_mode            # on the PARENT machine
  sequential_value: sequential    # the one mode value that turns ordering on
  order_field: fld_sequence       # a number field; lower acts first
  state_field: fld_decision       # where a sibling's progress is read
  open_value: pending             # a sibling holding this, earlier in order, is what locks
```

Ordering is opt-in: a Machine with no `sequencing:` block never locks anything. A record whose
parent's `mode_field` holds any other value isn't locked either, which is how the same two Machines
serve both a sequential and a parallel process without a second declaration.

### 12.6d `signature_placement` / `signature_store` — where a signature lives

Two optional blocks per Machine, added 2026-09-28 (Stage D). They are what let an approval
Application cast *any* Machine in the engine's `step` and `signature` roles (§12.1 `workflow:`) and
still have its signatures composited: before them, the five Field ids below were strings inside
`internal/action`, so a Machine whose Fields were named anything else placed no boxes and stamped
nothing, silently.

On the Machine cast in the `step` role:

```yaml
signature_placement:
  image_field: fld_signature_image   # file: the one-time image captured at Approve time
  page_field:  fld_signature_page    # number: 1-based page; no page means no placement at all
  x_field:     fld_signature_x       # number: the box's centre, % of the page, origin top-left
  y_field:     fld_signature_y       # number: Y grows down
  width_field: fld_signature_width   # number: % of page width; height comes from the image
```

On the Machine cast in the optional `signature` role — a person's own reusable image:

```yaml
signature_store:
  owner_field: fld_owner   # person: whose signature this is
  image_field: fld_image   # file
```

Each entry is optional and each named Field must exist on the *declaring* Machine with the right
type; the loader refuses a name that does not, or a coordinate Field that is not a `number`. An
omitted entry means "this Machine declares no such Field", and every reader handles that explicitly
rather than falling back to the template library's own names — a Machine declaring no
`signature_placement:` has no placement to read and nowhere to capture into, which is the honest
answer rather than a Machine that appears to work against the wrong Fields.

**These blocks are inert without a role.** Declaring them on a Machine no Application casts is legal
and does nothing, the same way `sequencing:` on a Machine outside any engine simply orders its own
records. What makes them run is the Application's `workflow:` binding.

### 12.6e `flow_template` / `flow_template_step` — a remembered approval flow

Two optional blocks per Machine, added 2026-09-29 (Stage E2), for the pair an approval Application
casts in its `flow_template` and `flow_template_step` roles (§12.1). They are what lets a saved
default flow work over Machines named anything: before them `internal/web` named all eight Field ids
itself, so the roles resolved and the writes went to Fields the Machine might not have.

```yaml
# on the Machine cast in the `flow_template` role
flow_template:
  key_field: fld_document_type   # one saved flow per distinct value of this Field
  mode_field: fld_mode           # the approval mode the saved flow remembers

# on the Machine cast in `flow_template_step`
flow_template_step:
  template_field: fld_template          # relation back to the flow this row belongs to
  order_field: fld_sequence             # number; lower runs first
  actor_field: fld_assignee             # person
  actor_type_field: fld_approver_type   # which kind of actor this row chose
  actor_group_field: fld_approver_group # group
```

Each named Field must exist on the *declaring* Machine, and the order, actor and group entries must be
`number`, `person` and `group`; `template_field` must be a reference. The key, mode, name and
actor-kind entries only have to exist — a Machine may legitimately spell those as text or as a closed
status set. An omitted entry means "this Machine declares no such Field", and a Machine cast in the
role while declaring *no* shape is **refused** when a flow is saved rather than guessed at.

**Why this is declared when a live step's equivalent is derived.** `sequencing:` says which Field
orders an Approval Step's siblings and the `decide` Permission says which holds its actor — so for a
live step the answer already exists and is read, never restated (§12.6a, §12.6). A template row has
neither block, correctly: nothing decides a template. Deriving from them was tried and returns *every
id empty*, which would write the rows under `""`. Six keys that look like a duplicate of the live
step's are two different Machines each answering for itself.

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

You don't write a `source:`: a Dataset lives inside the Machine file whose records it counts, so
the runtime fills that in (007 §7.2's `source.machine`, derived rather than declared, since
repeating the id here would be duplicated metadata). That's also why a dataset `id` must be
unique across the whole application, not just within its file — screens name a Dataset and
nothing else, and the runtime knows which Machine's records that means.

```yaml
# "how many Tasks does each assignee have, and how many are still open?"
datasets:
  - id: ds_task_by_assignee
    dimension: fld_assignee
    measures:
      - id: msr_total
        aggregate: count
      - id: msr_active
        aggregate: count
        where: {field: fld_status, op: not_equals, value: done}
```

A screen still decides which measure it displays where (that binding is Go), but the counting,
grouping and filtering are metadata: changing `where.value` above changes the figure a screen
shows for it with no code edit.

### 12.8 Closed vocabularies — the complete list of values you may write

| Vocabulary | Values | Extended by |
|---|---|---|
| Field types | `text` `long_text` `number` `boolean` `date` `status` `person` `money` `relation` `file` | `domain.KnownFieldTypes` |
| Layouts | `table` `board` | `domain.KnownLayouts` |
| Card field roles | `title` `person` `money` `status` `date` `color` `file` `comment` `container` | `domain.KnownCardFieldRoles` |
| Aggregates | `count` `sum` | `domain.KnownAggregates` |
| Actions | `decide` `edit` `delete` | `domain.KnownActions` |
| Services | `log_activity` `rollup_parent_status` `send_notification` | `registry.Services` |
| Schedule triggers | `overdue` | `domain.KnownScheduleWhens` |
| Notification preference keys | `assigned` `decided` `sla_breach` | `domain.KnownNotificationPreferenceKeys` |
| Comparison operators | `equals` `not_equals` `is_empty` `lt` `lte` `gt` `gte` `contains` | `expression.KnownOps` |
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
| Service | 006 §Service | `registry.Services`, `web.logActivity` |
| Permission | 006 §Permission, 005 §Security Ordering | `domain.Permission`, `authorization.AllowsAction` |
| Action | 006 §Action | `domain.KnownActions` + `internal/action` — **Go code, not declarable** (§8) |
| Sequencing (ordered activation) | 006 §Constraint (adjacent), CAP-A07 | `domain.Sequencing`, `behavior.CanAct`, `metadata.validateSequencing` |
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

### Script is the exception, and this is the list (2026-10-18)

The runtime draws pages on the server. A page, a Component or an author's YAML **never carries script**: no Component takes an
event handler or an `hx-*` property (§12.3), and `page:` has no way to write one. The shell loads two libraries (htmx, which
sends a form without leaving the page, and hyperscript) and each screen that needs behaviour a server cannot give carries a short
script of its own. Each exception is named here, with why script is the only way and what keeps it small. **A new exception
is added to this list in the same change, or it is not allowed.**

| Where | What it does | Why script is the only way | What bounds it |
|---|---|---|---|
| The shell (`appshell.templ`): the refused-write notice | Shows the server's refusal (a 422, a 403) in one place for every htmx request | htmx swaps nothing in for a 4xx/5xx | Reads only the response text; changes no value |
| The shell: the held-back form notice | Says "N required fields are still empty" and scrolls to the first one when the browser holds a form back | A held-back form sends no request, so no response can say why | Reads `:invalid`; changes no value |
| **The shell: closing a dialog** (2026-10-19) | **A dialog (a Popover that holds a form) closes on its Cancel, its close mark (`data-popover-close`) and on Escape** | A `<details>` cannot be closed by a button inside it, nor by a key, without script | Closes only `details[data-popover]`; never on a tap outside (a half-filled form would lose its typing); changes no value |
| **The shell: one popover at a time** (2026-10-25) | **Opening a Popover closes every other open one, except the menu it came from and what opened from it** | Popovers nest (a dialog inside a menu), so a shared `<details name=>` would close the parent; only script can tell parent from sibling | Closes only `details[data-popover]`; changes no value |
| **The shell: one row editor at a time** (2026-10-25) | **When a table row's inline editor is drawn, every other open row editor is put back as the row it was (its own Cancel is pressed)** | Each editor is a separate htmx swap that knows nothing of the others; a person on a phone ended with two rows in edit mode | Reads `tr[data-row-editor]` and clicks its `[data-row-cancel]`; changes no value (an unsaved edit in the other row is dropped, as with a popover) |
| **The shell: the money control** (2026-10-18) | **Groups the thousands of a money control as it is typed, stops at two decimals, pads to two decimals when the control is left, and sends the plain decimal** | A `number` control cannot show marks at all, and a `text` control cannot group digits as they are typed without script. The server only ever sees the plain decimal, so it cannot do this for the person typing | **Forms only, and only a `data-money` control** (a money Field of a Form). It reads the Workspace's marks from the control's `data-thousands`/`data-decimal` (the server wrote them from `number_format:`); it names no mark itself. It changes nothing but that control's text, and replaces the value in the request with the plain decimal. Without script the control is a text control holding the plain decimal, and the server accepts it as it always did. Display is never done by script: every table, total, metric and page is written by the server |
| `leaveguard`, `signatureplacement`, `reviewdocument`, `machine.templ` (board drag, **keyboard move**, quick edit) | Their own screens' interaction (a "leave this page?" guard, placing a signature on a PDF, dragging a card -- or lifting it with Space, moving it with the arrows, dropping it with Space and cancelling with Esc, reporting in the board's status line) | Pointer and focus behaviour; a gesture has no declarative form | Each in its own template, none reads a Workspace's data; the move ends in the same PATCH a drop sends |
| **`declaredscreen.templ` (a calendar card moved to another day, 2026-10-19)** | **A grouped list that declares `moves: true` draws each card the viewer may edit as draggable, and each day as a drop target: dropping a card on another day -- or Space on its title, the arrows to choose the day (left and right a day, up and down a week), Space to drop, Esc to cancel -- sends one PATCH of the record's own date Field through the generic patch route, whose answer to a declared page is a refresh** | A drag has no declarative form (§12.3's `actions` carries a click, not a gesture); the keyboard path is the same gesture another way | One delegated listener, on a page that has such a card and no other (`treeMovesCards`); writes only the date Field the binding names, only to a day of the window (a day added to fill a week is no drop target), and only for a record the viewer may edit; moves nothing in the DOM; its status line is a live region on the page before anything is lifted |

What this means for an author: you write `type: money` and `number_format:`, never a script. If a screen needs behaviour that none of the
rows above gives, say so as a gap; do not work around it with a property that smuggles a handler in.

## What comes free vs. what's hardcoded today — the honest map

Every item on the left, any new Machine gets automatically, purely from YAML. Every item on the
right is real, working code in this app, but wired to specific Machine ids in Go — declaring
similar-looking metadata for a *different* Machine does not activate it.

> **Nine Theme token categories are declarable, and they are the first things in this app a Workspace can
> restyle from YAML.** A Workspace manifest may carry:
>
> ```yaml
> theme:
>   radius:
>     control: small    # inputs, buttons, menu rows, small boxes (default: small -> rounded-md)
>     surface: large    # panels, sections     (default: large  -> rounded-lg)
>     pill:    full     # badges, chips        (default: full   -> rounded-full; circles such as avatars stay round)
>   weight:
>     body:     normal  # text explicitly unemphasised  (table column headings and group headers)  (default: normal)
>     emphasis: medium  # headings, labels, values      (default: medium)
>   text:
>     display: huge     # a tile's headline number   (default: huge)
>     heading: large    # a screen's own title        (default: large)
>     subheading: medium  # one section's title inside a screen (default: medium)
>     body:    normal   # prose and row content      (default: normal)
>     meta:    small    # dates, counts, names       (default: small)
>     label:   tiny     # badge and pill text        (default: tiny)
>     eyebrow: micro    # the uppercase line above a title (default: micro)
>     caption: tiny     # the help line under a panel title or control (default: tiny)
>     item:    normal-plus  # one item's title in a list or on a card: an Application, a member, a document (default: normal-plus, 15px)
>   gap:
>     hair:        half   # the smallest step: tightly stacked lines, a name over its email (default: half)
>     micro:       one    # a title over its subtitle, a label beside its value (default: one)
>     xtight:      one-half  # an icon or dot with its label inside a compact component -- never between components (default: one-half)
>     tight:       two    # (default: two)
>     default:     three  # (default: three)
>     comfortable: four   # (default: four)
>     loose:       five   # (default: five)
>     region:      six    # the space between a screen's large regions -- a header from its content (default: six)
>   border:
>     surface: soft     # panels, sections, cards, table wrappers, popovers (default: soft)
>     control: defined  # an input's own box  (default: defined)
>     divider: faint    # row separators in lists and tables  (default: faint)
>   tone:
>     neutral: grey        # a badge with no particular meaning  (default: grey)
>     info:    blue        # informational                       (default: blue)
>     good:    green       # success / approved                  (default: green)
>     bad:     red         # failure / overdue                   (default: red)
>     warn:    amber       # needs attention                     (default: amber)
>     muted:   grey-faint  # de-emphasised                       (default: grey-faint)
>   ink:
>     strong:    darkest  # titles and the figures people scan for   (default: darkest)
>     body:      dark     # text people read or click  (default: dark; read by the secondary Button)
>     secondary: medium   # captions, descriptions, meta lines       (default: medium)
>     faint:     light    # hints, placeholders, decoration          (default: light)
>     accent:    blue     # links and accent text, hover included; a HUE: blue, indigo, violet, emerald,
>                         #   rose or cyan (default: blue). A grey role may not take a hue, nor accent a grey.
>                         #   Or a colour value, "#be123c" -- see below.
>   background:
>     raised: lightest    # cards, panels, the header and bottom bar   (default: lightest)
>     base:   lighter     # the page behind everything                (default: lighter)
>     sunken: light       # a board lane or recessed well             (default: light)
>   padding:
>     panel:           four      # the bordered panel                  (default: four)
>     section:         five      # a titled, stacked section           (default: five)
>     badge-x:         two-half  # a badge's side room                 (default: two-half)
>     badge-y:         one       # a badge's top and bottom room       (default: one)
>     badge-compact-x: two       # the compact badge, sides            (default: two)
>     badge-compact-y: half      # the compact badge, top and bottom   (default: half)
> ```
>
> `tone:` maps each semantic tone to a **palette**, and a palette is a background *and* text colour as one
> pair -- you cannot declare one without the other, so a badge never becomes unreadable. Palettes:
> `grey`, `grey-faint`, `blue`, `green`, `red`, `amber`. Like `gap:` it needs no role of its own, because the
> tone *is* the role.
>
> `ink:` is text colour: four roles, four shades (`darkest`, `dark`, `medium`, `light`). Keep `faint` for text
> that may be missed -- the default `light` shade is about 2.6:1 on white, below WCAG AA, so mapping real
> content to it makes that content hard to read. Only the runtime's shared components (a Metric, a static
> paragraph) follow `ink:`; text a screen draws by hand keeps its own colour.
>
> **An ink role may also take a colour value** instead of a shade -- your brand colour, written `"#rrggbb"` (quoted,
> because YAML reads `#` as a comment): `accent: "#be123c"`, `body: "#1f2937"`. Every role takes one except `faint`.
> The runtime checks contrast when it loads the Workspace, for shades and values alike, as soon as your `theme:` moves
> any `ink` or `background` role: text must read at **4.5:1** on the surfaces it sits on (`strong`, `body` and
> `accent` on `raised`, `base` and `sunken`; `secondary` on `raised` and `base`). A pair that does not is a load
> error naming the ratio, so pick a darker ink or a lighter background. So `secondary: light` is refused (2.6:1),
> and so is a darker `base: light` unless you also darken `secondary`. The
> accent's hover is the same colour, slightly darker. A value applies to the screens a Workspace renders; the sign-in
> pages, which belong to no Workspace, keep the default colours.
>
> `background:` is the colour of light surfaces: three roles, three shades (`lightest` is white, `lighter` and
> `light` step toward grey), or a quoted colour value (`base: "#fefcf8"`) for a tinted surface. A value is checked
> when the Workspace loads. It may tint a surface but not darken it past `light`, because the runtime draws colours of
> its own on surfaces (a field's white box, the green check, red and amber status text) and those are only guaranteed
> readable down to that shade. Your `ink:` colours must read at 4.5:1 on
> the surfaces they sit on. And the three surfaces must stay visibly apart: the page colour is also what a row turns
> when you hover it, and `sunken` is a lane or a selected row. Dark surfaces (the primary button, a menu) are not part of it. Like `ink:`, it
> reaches what the runtime's own components draw -- panels and sections, the header and page, a board lane --
> and not a card a screen draws by hand.
>
> **Dark mode.** When a person's device is set to dark, every screen draws its dark palette, sign-in pages included;
> there is nothing to declare. What your `theme:` does there depends on the kind of choice. A **hue** you picked
> carries over: `ink: {accent: violet}` keeps violet links in the dark (at a lighter step that reads on a dark
> surface), and `tone: {warn: red}` keeps red warnings. A **shade** or a **colour value** for `ink`, `background` or
> `border` applies to light mode only, because it was checked against light surfaces; in dark mode those roles take the
> dark palette. A brand accent written as `"#rrggbb"` is a value, so dark mode draws the default blue there.
>
> `padding:` is the room inside a surface, and **a role names a place, not a number of sides**: `panel` and
> `section` pad all four, a badge pads its two axes separately (`badge-x`, `badge-y`). Amounts, smallest
> first: `half`, `one`, `one-half`, `two`, `two-half`, `three`, `four`, `five` -- except that `panel` and `section`
> start at `two`, since nothing smaller exists on a surface. An amount off a role's ladder is a load error.
> `panel` and `section` are both `five` by default (owner decision D6, 2026-10-07; `panel` was `four`); they stay two roles,
> so declaring them apart is how a Workspace gives a panel less room than a section. Only the runtime's own panels, sections and badges follow
> `padding:` -- the ~400 other padding sites on hand-written screens do not.
>
> `gap:` is the one category shaped differently: it maps a **spacing step straight to an amount**, because a
> caller already says `tight` rather than a raw class — there is no role to invent. Amounts, smallest first:
> `half`, `one`, `one-half`, `two`, `three`, `four`, `five`, `six`.
>
> The eight text scales, smallest first: `micro` (10px), `tiny` (11), `small` (12), `normal` (14), `normal-plus` (15),
> `medium` (16), `large` (20), `huge` (24). `normal-plus` arrived with `item` (2026-10-12); 13px was rejected as a step
> and stays out (the Design System records why).
>
> **`border.control` and `radius.control` move every field a signed-in person types into, and no field on a pre-auth screen.** Sign in, register, forgot/reset password, accept-invite and choose-workspace render before a Workspace exists on the request, so a `theme:` block cannot reach them -- they always draw the default theme (2026-10-07; read from the router, so a screen moving behind the login gate would start following the theme). Account's name field, the invite-role select and the signature width box read the theme; four fields on the sign-in and register kit are the ones that cannot.
>
> **`border.surface` moves every large-radius bordered surface, and only those.** Since 2026-10-06 that is the 34 hand-written cards, asides, fieldsets, table wrappers and popovers as well as the primitives' own (`surfaceClasses`). It moves the eleven small boxes inside a surface (a selectable option, a list item, a card in a column) too, through `tileClasses`, which reads `radius.control` for the corner and `border.surface` for the edge (2026-10-07) -- so a Workspace that rounds its controls rounds its tiles with them, and a tile cannot yet differ from a control: that is the case that would earn a `tile` radius role. It also moves a button's outline -- the secondary Button, the button-shaped links and the view switch's track (owner decision D8, 2026-10-07: a button is clicked and an input is typed into, so the two are separate keys, and a Workspace wanting one line declares `border.surface` and `border.control` the same). It does **not** move a dashed empty state, a read-only email box, an image frame, or the four surfaces whose border changes with state (an overdue card, today's calendar column, an unread notification, the signature dialog) -- those keep slate-200 until a role exists for them. It **does** move the thirteen rules that bound a region (a card's header and footer rule, a table's head, the page's top bar and bottom nav; 2026-10-06), so a faint box and firm rules are not yet expressible: that is the case that would earn a `region` role.
>
> **There is no `border.danger`.** A danger zone's red edge (the Remove-from-workspace and Danger-zone panels) is a fixed `red-200`, as are the danger Button, the overdue approval card and the Deactivate button: five jobs, no key (owner decision D9, 2026-10-07). Red belongs to the tone palettes, not to the `border.*` shade ladder; if a Workspace ever needs to recolour them, the capability is an edge on the `red` tone palette, which does not exist yet.
>
> **Every key in this block changes pixels now.** `weight.body` and `border.divider` did not until 2026-10-06, when their readers were found to exist already (`weightClass`, `borderClass`) and every list row, table cell and reset-weight column heading was made to call them; neither needed a primitive. `radius.control` and `ink.body` started working on 2026-10-06, when the Button Component (007 §12.3) became their first reader, and `border.control` the same day, when every input began reading it through `fieldClasses`. **`border.control` does not move buttons**: a button's outline is slate-200 in the mockups and `border.control` is slate-300, so reading it there would shift fourteen buttons (owner decision D8). `conformance.TestEveryThemeRoleHasAReaderOrAReason` holds this list, which is empty: a key declared without a reader fails until it gets one.
>
> Every role, step and palette is a closed set: an unknown value is a **load error**, not a silent
> default, so a typo cannot ship as a missing style. Omitting the block, or any role in it, inherits the
> default — which is exactly what the app rendered before Theme existed, so adding the key changes nothing
> until you change a value. All nine token categories are declarable; shadow is deliberately not a tenth (six uses in total). The inventory and the order they arrived in:
> `menata-app-document/audits/2026-10-04-inventaris-token-design-system.md`.

> **A screen's own layout is on neither side yet, and that is worth stating so nothing here is
> misread.** As of 2026-10-03 the Experience Plane has six of 007 §12.2's Layout primitives, three
> §12.3 Components with declared contracts, and UI IR (`ir.UINode`) with two consumers — so a screen's
> composition is now *data* rather than templ structure in those two places. **But no YAML declares a
> layout.** The tree is built in Go, which means this capability has moved from "hardcoded per screen"
> to "one generic vocabulary, still reached from Go" — a real step and not the left column. What the
> left column requires is §15.1's pipeline running from metadata, which is unbuilt. Forward pointer:
> 007 §12.4's normative rule (a View MUST NOT be the universal composition primitive) and
> `conformance.TestHandWrittenLayoutSitesOnlyShrink`, which carries the sites still hand-written (read the count out of the map; each remaining one is a floor with its reason).

| Generic (any Machine, metadata only) | Hardcoded to specific Machines (real Go code required for a new one) |
|---|---|
| CRUD screens + JSON API, table and board views | The `decide` Action itself — though its two cross-record rules (step ordering, Document status rollup) are now declared, not hardcoded, *which decisions are legal at all* moved left in Fase 7 (`transitions:`), **which Machines it acts on** moved left on 2026-09-28 (`workflow:`), and **which Fields it writes** moved left the same day (`actions:`, §12.6c — with the status move itself derived from the Transition that names the Action, so it is declared once rather than twice). What is still Go is the *Fields it reads*: the signed-PDF compositing filters steps by `fld_decision`, and composed screens project Fields by id |
| Relations, `person`, child collections, many-to-many | Document submission wizard |
| Constraints (`equals`/`not_equals` shape) | The signature-placement *screen* — its canvas, drag maths and layout are bespoke Go/JS. Its four Field bindings are not: since Stage D they come from `signature_placement:` (§12.6d), read and written alike |
| Events (field-change, record-creation, or schedule/time-passing → one Service), and since 2026-09-28 the **signature shape** a Machine holds (`signature_placement:`/`signature_store:`, §12.6d) — where a box sits, which Field takes the captured image, where a reusable one is kept | The PDF compositing *itself* (`internal/action/composite.go` manipulates binary PDF bytes; 002 keeps that runtime-owned, and rewriting it as metadata is not a target). What is no longer here: *which* Fields it reads |
| Role-based Permission (`roles:` on any Permission, any Machine an Application claims — CAP-P01, Fase 7) and a **declared transition model** (`transitions:`, any Machine's own status Fields), both enforced by the generic routes with no per-Machine code | **A rule that reads a field on a *related* record.** Every Permission arm reads the record being acted on, or the actor — none reaches a parent. Two real cases are Go for exactly this reason: who may place a Document's signature boxes (`composition.MayPlaceSignature` — its *submitter*, read off the parent Document, or the step's own approver) and who may add Approval Steps to a Document. Forward-checkable pointer: `ROADMAP.md`'s deferral table, "Only a Document's own submitter may add Approval Steps to it" |
| Record-scoped `edit`/`delete` Permission (any Machine), and a **per-record User-or-Group actor gate** on any of them (CAP-F24) — declared on `decide`, `edit` and `delete` alike since 6c-3 | The Review Document screen (Fase 6b) — its Approve/Reject bar, signature canvas and placement panel are all `mch_approval_step`-shaped. What moved *left* with it: the generic record-detail page no longer special-cases deciding, and no longer runs a signature lookup for every Machine. (The Approval progress stepper UI itself moved left too, 2026-09-26 — see the Declared Views row) |
| — | **Conditional required** — "this Field is required only when a sibling Field holds a given value". `Constraint` has one shape (block a transition while a *related Machine* has a matching record), which cannot condition on a sibling Field of the same record. The one real case is `mch_approval_step`: `fld_assignee` is required when `fld_approver_type` is User and meaningless when it is Group, so the Field is declared optional and `internal/web`'s `parseStepInputs` enforces the pairing (Fase 6c-2). Upstream states the same rule as two conditional Constraints, so the shape is known — it is this runtime's Constraint that has to grow |
| SLA badges (`sla_field`) | **A Page's own primary action** — the Approval Inbox's "+ New Document" button, and (2026-09-26) the launcher's own "New application" link (it began in the Workspace menu, gone since 2026-10-22) into the AI Metadata Assistant (Flow 2 gap study Tahap 8). `navigation:` declares menu *destinations*; it cannot say "this screen has a call to action pointing at that screen", nor can it declare a Workspace-level action that exists whether zero or ten Applications are installed. `nav_new_approval` was deleted on 2026-09-21 (owner instruction) precisely because declaring a submit form as a menu item was the wrong shape for it, which left the route and the label as literals in `approvalinbox.templ`/`documentsubmit.templ` and `appshell.templ`, and an entry each in `internal/conformance`'s `applicationSubScreens`. Forward-checkable pointer: 007 §12.3's `ActionBar` component, still unbuilt |
| Declared Views (`views:` — `table`/`board`/`cards`/`stepper`) | A View composing *other* Views, rather than one Machine's own records, has one real case built (2026-09-26): `stepper` (`domain.ViewStepper`, menata-runtime's CAP-V20) renders a parent's own children as a sequential done/current/waiting progress indicator — `mch_approval_step`'s `vw_step_progress`, composed by `internal/rendering/detail.templ` via `domain.Machine.StepperView`, not selected by `?view=`. What's still hardcoded is the Review Document screen's own orchestration around it (Approve/Reject bar, signature canvas/placement) — see the row above |
| Counting/summing a Machine's own records (`datasets:`), and — since 2026-09-28 — **which Field a composed screen shows in a row** (`card_fields:`, §7.1): My Tasks and the Calendar resolve title/status/date through Projection | Composed screens: Approval Inbox. What stays Go on the migrated screens is their *layout* — which measure lands in which column and how rows are bucketed (overdue/today/upcoming), a temporal predicate that `where:` cannot express. Board Settings reads a Label's name and colour through `mch_label`'s `title`/`color` card_fields and its three Datasets, so `projectionRatchet` is empty |
| Notifications (`send_notification`, in-app + email, `domain.Notify`) — a declared Event writes an `mch_notification` record and, if the recipient's own preference allows it, emails them | **A notification's own link target.** `internal/execution.notificationLinkFor` special-cases `mch_approval_step` → its `/review` route rather than the generic `/machines/{id}/records/{id}` detail page, which is explicitly "POC scaffolding no real approver should land on" (`detailBackLink`'s own reasoning). Every other Machine (`mch_document` today) gets the generic detail route. Forward-checkable pointer: a second notification-emitting Machine needing its own non-generic destination is the trigger to turn this into a declared Field rather than a per-Machine-id branch |
| Money figures: summed, bounded, computed, ordered (`type: money`) | **How a money figure is written**: the international convention, `,` between thousands and `.` before at most two decimals, no currency symbol (`domain.FormatFigure`; the owner's choice, 2026-10-17, and the same `.` a number input reads). No Workspace or Field can declare another locale or a currency yet. Forward pointer: top-level roadmap (`menata-app-document` audits/2026-10-15-peta-jalan-menuju-aplikasi-lengkap.md) Tahap 3, domain capabilities; gap U-08 of audits/2026-10-17-kajian-gap-mockup-21-case |
| — | **The ⋯ menu's Share item** — a clipboard write of the page's own address, inline in `detail.templ`. A copy-to-clipboard gesture has no declarative form: it is 007 §12.3's `actions` slot, not built, the same class as the board's drag script. Listed per `CLAUDE.md`'s exception rule; revisit when `actions` lands |

**A Page's own layout is the next row to move, and the plan for it is written** (2026-09-30):
`menata-app-document`'s `audits/2026-09-30-kajian-ui-ir-untuk-apa-dan-bagaimana-merealisasikannya.md`
carries the staged plan, the farthest capability 001-007 authorises, and its boundary. Today a Page's
arrangement is Go: 33 of the 38 navigation items installed across Workspaces (nine distinct in the
library; measured 2026-10-07) point at a bespoke route, which 007 §12.4 forbids normatively. What will move is the *arrangement* (Layout §12.2, Static Content §12.6, then bounded
Components §12.3) -- **not** styling, which stays Theme (006) and must not arrive through composition,
and not the eight identity/lifecycle screens, which 001 #9 keeps in the runtime.

**One measured gap this surfaced, worth writing here because a metadata author hits it directly:** a
navigation item may declare `title:` and `description:` as well as `label:`, and **18 of 38 declare
neither**. Where they are absent the screen's own subtitle is a literal in its `.templ` --
`activity.templ`'s "A cross-project feed of business events." is one -- and
`TestRenderingHasNoHardcodedPageHeading` cannot see it, because that gate matches a *declared* string
verbatim and there is nothing declared to match. So the page header is already almost entirely
derivable (heading from `label:`/`title:`, subtitle from
`description:`); what is missing is the declaration, not the mechanism. Declaring `description:` on a
nav item is the cheapest real step toward the row above.

**The right column is a capability snapshot, not a permanent exemption list.** Each entry existed
because metadata couldn't express it *when it was written* — `card_fields` (Projection) and
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

- **Three Actions, and the approval engine no longer names any Field.** `decide`, `edit`, `delete`.
  Since 2026-09-28 `decide` runs for whichever Machines an Application casts in the engine's
  `document` and `step` roles (`workflow:`, §12.1), under any names; writes whichever Fields that
  Machine declares (`actions:`, §12.6c); derives the ones it reads from declarations that already
  existed (the decide `transitions:` edges give the decision Field and the still-open value, the
  relation gives the parent, `sequencing:` gives the order, the Permission gives the actor); and,
  since `signature_placement:`/`signature_store:` (§12.6d), places and composites signatures through
  a declaration too. A Machine you bind therefore does **not** have to carry Document Approval's own
  vocabulary — `composition.TestBuildInbox_overAMachineThatNamesItsFieldsDifferently` and
  `web.TestDecideStep_writesTheFieldsTheMachineDeclares` are the assertions, not the claim.
  What is still Go is narrower and worth naming exactly: the **submit wizard** reads the *Document's*
  own Fields by id (`fld_title`, `fld_document_type`, `fld_file` in `internal/web/document.go`), because
  `continue-submit` shares the generic `edit` Action and has no Action of its own to declare effects on.
  The page thumbnail in `detail.templ` stopped naming a Field on 2026-10-07: it reads the compositing Event's
  `source_field`, the same declaration `/pdf-preview` renders. The approver row's three `name=` attributes are **no longer** among them: since
  2026-09-29 `documentsubmit.templ` renders `rendering.StepFields`, resolved once in `readWizardOptions`
  from the same `action.DeclaredFields` the form's *read* side always used -- so the two ends of that
  form read one declaration instead of agreeing because both happened to type `fld_assignee`. See §8.

- **No field-level permissions.** Access control today is per-Machine and per-Action at best; you
  cannot hide or lock one Field from one role while leaving the rest editable. A `roles:` arm
  (§8) gates a whole Action, not a Field within it.
- **A role grants the same thing everywhere in its Application.** There is no scope depth
  ("their own records", "their unit and below") — a role either satisfies a Permission or does
  not, and narrowing *which records* is still `actor_field`'s job alone. Upstream carries the
  scope-depth shape as a proposed, unbuilt capability, so this is a known gap rather than an
  open design question.
- **A transition cannot carry a condition or a side effect.** `transitions:` declares which moves
  exist and which Action performs each (§8.1) — not "only when this other Field is set", not
  "and then notify". Those remain `constraints:` and `events:`, declared separately against the
  same Fields.
- **A schedule Event's `when` supports exactly one comparison** (`overdue`) — no `due_today`, no
  arbitrary offsets. See §10.
- **A schedule Event's recipient reads a Field on its own record only**, same as any
  `send_notification` Event (§10, `domain.Notify`'s own doc comment) — the SLA-breach reminder
  notifies a Document's submitter, not whichever step is currently pending, because reaching the
  latter needs cross-record resolution no case has forced yet.
- **An Event's wording supports at most one override**, not arbitrary per-value branching (§10).
- **Reads are whole-Machine.** There's no filtering, projection or pagination pushed to the
  database — a page fetches a Machine's full record set and reduces it in application code. This
  is fine well past ten thousand records in one Machine, and becomes a real cost somewhere between
  ten and fifty thousand.
- **A hand edit takes effect on a reload, not on save.** A Workspace is re-read when an Application is
  installed or published, when a snapshot is restored, and when a workspace admin presses Reload in
  Workspace Settings (`POST /reload-workspace`); the other Workspaces are not touched, and a manifest that
  does not load changes nothing live and names its file. A process restart re-reads everything.
- **Installing a template renames a colliding Machine or Application id; other collisions are refused.**
  Installing copies the template into the Workspace's own directory, and two Machines cannot share an id
  inside one Workspace, so `mch_document` becomes `mch_document_approval` when the Workspace already has
  one (a Workspace with no collision gets a byte-identical copy). A collision on a navigation id, a
  navigation route or a Dataset id is refused with its reason instead, because Go still names those. The
  same name in a **different** Workspace is always fine. `/install-application` is workspace-admin only,
  and the install is load-verified and rolled back if the result does not load.
- **No metadata versioning.** Removing a Field from a Machine's YAML doesn't migrate or warn about
  existing data already stored under that field's id — it's simply no longer read.
- **Constraints are intentionally limited.** Only `equals`/`not_equals` against a literal value,
  no boolean combinators, no cross-field arithmetic.
- **~~Every Workspace shares one manifest.~~ ~~The set of Machines and Applications is
  process-wide.~~** Both closed, and the pair is worth reading together because the first fix made
  the second one's absence visible. 2026-09-22: one manifest per Workspace, keyed by slug, and a
  Workspace may run several Applications. 2026-09-27: a Workspace's Machines are its *own* —
  installing an Application copies it into `metadata/workspaces/<slug>/` rather than pointing at a
  shared file, so two Workspaces may each hold an `mch_document` meaning different things and
  either may diverge without touching the other. Only `mch_user`, `mch_activity` and
  `mch_notification` stay shared. This bullet read "one Workspace runs one fixed Application" until
  2026-09-21, which stopped being true at Fase 3a when `applications:` became a list.

If you hit one of these and it matters for what you're building, the numbered concept docs explain
the reasoning behind the current shape and where each of these is headed.
