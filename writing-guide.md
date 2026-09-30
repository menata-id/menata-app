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
actually exist in the manifest's own `machines:` list also fails (a single Machine file can't check
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

## 7.1. Card fields — what a record shows when it renders as a card

```yaml
card_fields:
  - { field: fld_document_type, role: status }
  - { field: fld_status, role: status }
  - { field: fld_due_date, role: date }
```

Each entry names one of this Machine's own Fields plus the semantic **role** it renders as —
`title`, `person`, `money`, `status` or `date` (`internal/domain.KnownCardFieldRoles`). The
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

| Key | Value | Notes |
|---|---|---|
| `workspace` | slug string | Which Workspace this manifest installs into. A slug, not an id: the `ws_...` id is generated when the Workspace is created through the UI. The nested `workspace:` block this key replaced (carrying `id`/`name`/`machines`/`navigation`) stopped loading on 2026-09-22 |
| `machines[]` | file paths | Load order; each file is one Machine, and this Workspace's own. A path under `<slug>/` is its own copy; a path outside is one of the three runtime-level Machines every Workspace shares |
| `navigation[]` | list of items | Destinations belonging to no single Application (Home, All Machines, Members, Groups) |
| `applications[]` | file paths | One file per Application; declaration order is launcher/Workspace Home order |
| `suggested_applications[]` | list of `{label, prompt}` | Workspace Home's "Add an application" chips (2026-09-27); both required. Display text and a conversation-starter prompt for `/new-application?idea=`, not an identifier anything else references — a Workspace with none declared simply shows no chips |

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

`engine` is a closed set (`domain.KnownWorkflowEngines`) with one member today,
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
| `priority` | int | |
| `badge` | closed set | Only `approval_inbox_pending` today |
| `home_card` | bool | Marks the Workspace Home card's destination (`Application.HomeRoute`) |
| `icon` | closed set | Drawn on the mobile bottom bar. Same vocabulary as an Application's own `icon` — see below |

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
      guard_field: fld_*          # optional, must be set together with guard_equals
      guard_equals: <value>
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
      default: in_review              # everything else, including no children yet
```

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
| `card_fields[]` | `{field: fld_*, role: title\|person\|money\|status\|date}` | Projection (007 §7.6). Consumed by any `cards` View (`mch_document`'s `vw_document_cards`), and by `internal/composition` for any composed screen that renders a record outside a View — My Tasks, the Calendar week, the Sprint dashboard's Attention list and the Dashboard's Project/pending-Document rows all read their title/status/date from here since 2026-09-28, which is what took them out of `internal/conformance`'s projection ratchet |

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
| Services | `log_activity` `rollup_parent_status` `send_notification` | `domain.KnownServices` |
| Schedule triggers | `overdue` | `domain.KnownScheduleWhens` |
| Notification preference keys | `assigned` `decided` `sla_breach` | `domain.KnownNotificationPreferenceKeys` |
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

## What comes free vs. what's hardcoded today — the honest map

Every item on the left, any new Machine gets automatically, purely from YAML. Every item on the
right is real, working code in this app, but wired to specific Machine ids in Go — declaring
similar-looking metadata for a *different* Machine does not activate it.

| Generic (any Machine, metadata only) | Hardcoded to specific Machines (real Go code required for a new one) |
|---|---|
| CRUD screens + JSON API, table and board views | The `decide` Action itself — though its two cross-record rules (step ordering, Document status rollup) are now declared, not hardcoded, *which decisions are legal at all* moved left in Fase 7 (`transitions:`), **which Machines it acts on** moved left on 2026-09-28 (`workflow:`), and **which Fields it writes** moved left the same day (`actions:`, §12.6c — with the status move itself derived from the Transition that names the Action, so it is declared once rather than twice). What is still Go is the *Fields it reads*: the signed-PDF compositing filters steps by `fld_decision`, and composed screens project Fields by id |
| Relations, `person`, child collections, many-to-many | Document submission wizard |
| Constraints (`equals`/`not_equals` shape) | The signature-placement *screen* — its canvas, drag maths and layout are bespoke Go/JS. Its four Field bindings are not: since Stage D they come from `signature_placement:` (§12.6d), read and written alike |
| Events (field-change, record-creation, or schedule/time-passing → one Service), and since 2026-09-28 the **signature shape** a Machine holds (`signature_placement:`/`signature_store:`, §12.6d) — where a box sits, which Field takes the captured image, where a reusable one is kept | The PDF compositing *itself* (`internal/action/composite.go` manipulates binary PDF bytes; 002 keeps that runtime-owned, and rewriting it as metadata is not a target). What is no longer here: *which* Fields it reads |
| Role-based Permission (`roles:` on any Permission, any Machine an Application claims — CAP-P01, Fase 7) and a **declared transition model** (`transitions:`, any Machine's own status Fields), both enforced by the generic routes with no per-Machine code | **A rule that reads a field on a *related* record.** Every Permission arm reads the record being acted on, or the actor — none reaches a parent. Two real cases are Go for exactly this reason: who may place a Document's signature boxes (`composition.MayPlaceSignature` — its *submitter*, read off the parent Document, or the step's own approver) and who may add Approval Steps to a Document. Forward-checkable pointer: `ROADMAP.md`'s deferral table, "Only a Document's own submitter may add Approval Steps to it" |
| Record-scoped `edit`/`delete` Permission (any Machine), and a **per-record User-or-Group actor gate** on any of them (CAP-F24) — declared on `decide`, `edit` and `delete` alike since 6c-3 | The Review Document screen (Fase 6b) — its Approve/Reject bar, signature canvas and placement panel are all `mch_approval_step`-shaped. What moved *left* with it: the generic record-detail page no longer special-cases deciding, and no longer runs a signature lookup for every Machine. (The Approval progress stepper UI itself moved left too, 2026-09-26 — see the Declared Views row) |
| Field defaults | **A file Field's own preview** — `detail.templ` decides whether to draw a PDF thumbnail by naming `action.FieldDocumentFile`, the last Field-id literal `documentApprovalFieldCoupling` still counts (1 of 1, 2026-09-30). **A props decision, not a missing primitive**: the Field *is* derivable from the compositing Event's `source_field`, but it lives on the step Machine and the page's signature is already at ten parameters — and it would answer a different question, since "which file gets a preview" is presentation while "which file the signing composites onto" is workflow. They name the same Field today and need not. What is actually missing is a *presentation* declaration for "preview this file" (a `card_fields` role, or a Field-level flag); forward-checkable at 007 §7.6 Projection and §12.3 Component boundedness. Trigger: a second Machine wanting a file previewed |
| — | **Conditional required** — "this Field is required only when a sibling Field holds a given value". `Constraint` has one shape (block a transition while a *related Machine* has a matching record), which cannot condition on a sibling Field of the same record. The one real case is `mch_approval_step`: `fld_assignee` is required when `fld_approver_type` is User and meaningless when it is Group, so the Field is declared optional and `internal/web`'s `parseStepInputs` enforces the pairing (Fase 6c-2). Upstream states the same rule as two conditional Constraints, so the shape is known — it is this runtime's Constraint that has to grow |
| SLA badges (`sla_field`) | **A Page's own primary action** — the Approval Inbox's "+ New Document" button, and (2026-09-26) the Workspace menu's own "New application" link into the AI Metadata Assistant (Flow 2 gap study Tahap 8). `navigation:` declares menu *destinations*; it cannot say "this screen has a call to action pointing at that screen", nor can it declare a Workspace-level action that exists whether zero or ten Applications are installed. `nav_new_approval` was deleted on 2026-09-21 (owner instruction) precisely because declaring a submit form as a menu item was the wrong shape for it, which left the route and the label as literals in `approvalinbox.templ`/`documentsubmit.templ` and `appshell.templ`, and an entry each in `internal/conformance`'s `applicationSubScreens`. Forward-checkable pointer: 007 §12.3's `ActionBar` component, still unbuilt |
| Declared Views (`views:` — `table`/`board`/`cards`/`stepper`) | A View composing *other* Views, rather than one Machine's own records, has one real case built (2026-09-26): `stepper` (`domain.ViewStepper`, menata-runtime's CAP-V20) renders a parent's own children as a sequential done/current/waiting progress indicator — `mch_approval_step`'s `vw_step_progress`, composed by `internal/rendering/detail.templ` via `domain.Machine.StepperView`, not selected by `?view=`. What's still hardcoded is the Review Document screen's own orchestration around it (Approve/Reject bar, signature canvas/placement) — see the row above |
| Counting/summing a Machine's own records (`datasets:`), and — since 2026-09-28 — **which Field a composed screen shows in a row** (`card_fields:`, §7.1): My Tasks, the Calendar, the Sprint Attention list and the Dashboard's Project/pending-Document rows resolve title/status/date through Projection, and Team Capacity reads its weekly number out of `ds_user_capacity`'s own `field:` | Composed screens: Approval Inbox, Automation, Board Settings. What stays Go on the four migrated screens is their *layout* — which measure lands in which column, how rows are bucketed (overdue/today/upcoming), and the temporal predicate that selects the Pending and Attention lists, which `where:` cannot express. Board Settings also still reads `fld_color` raw: no `card_fields` role describes a colour token (`internal/conformance`'s `projectionRatchet`, now one entry) |
| Notifications (`send_notification`, in-app + email, `domain.Notify`) — a declared Event writes an `mch_notification` record and, if the recipient's own preference allows it, emails them | **A notification's own link target.** `internal/execution.notificationLinkFor` special-cases `mch_approval_step` → its `/review` route rather than the generic `/machines/{id}/records/{id}` detail page, which is explicitly "POC scaffolding no real approver should land on" (`detailBackLink`'s own reasoning). Every other Machine (`mch_document` today) gets the generic detail route. Forward-checkable pointer: a second notification-emitting Machine needing its own non-generic destination is the trigger to turn this into a declared Field rather than a per-Machine-id branch |

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
  `continue-submit` shares the generic `edit` Action and has no Action of its own to declare effects on;
  and a Document's file Field is named in one composed screen (`detail.templ`, to decide whether to draw
  a page thumbnail). The approver row's three `name=` attributes are **no longer** among them: since
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
- **Metadata loads once, at process startup.** Editing a `*.yaml` file requires a restart to take
  effect — there's no hot reload.
- **A Workspace cannot install a template whose ids it already uses.** Installing an Application
  copies its metadata into the Workspace's own directory, and two Machines cannot share an id
  inside one Workspace — so if your Workspace already has an `mch_document`, it cannot also take a
  template that declares one, and nothing offers to rename either. This is the one collision the
  isolation model does *not* relax: the same name in a **different** Workspace is always fine. The
  AI Metadata Assistant avoids the problem when it generates (it is told every id already taken in
  your Workspace), but installing a hand-written template gets no such help. Planned:
  `ROADMAP.md`, "Installing a template into a Workspace that already uses its ids".
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
