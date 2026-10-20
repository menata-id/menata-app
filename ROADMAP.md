# Menata App Roadmap

What Menata App can do today, what is being built now, and what comes next. It is kept at the feature level on
purpose. Menata App is a runtime: an application is described in metadata (YAML), and the runtime turns that
description into a working product, without per-application code. The target architecture is described in the
concept documents (001–007) at the root of this repository. Design notes and the detailed development history are
kept in a private companion repository.

## Shipped

**Building applications from metadata**
- **Data models from YAML**: a Machine (a kind of record) gets list, board and calendar views, create/edit/delete,
  relations, child collections, checklists, comments and file attachments, with no per-model code.
- **Behaviour from YAML**: permissions per action and per role, status transitions (and who may take each), fields required under a condition, dates derived from a status or from another date, an edge that creates a schedule of records, an edge that waits until its lines add up, limits on a total, records frozen by their parent, what an action writes,
  events (on change, on create, on a schedule) that log activity, notify a person, a role or a group, roll a status up to a parent record, or escalate an overdue record.
- **Declared screens**: a navigation item can declare its whole screen as a tree of layouts, text and components,
  bound to declared data. Lists, metrics, forms that create and edit records, buttons that move, delete or change
  the status of a record, and conditions on what a record shows; a form can choose its fields or be filled from a document an AI reads, and a page can be downloaded as a PDF. The board settings, dashboard, My Tasks and
  calendar screens are drawn this way, and so are the Helpdesk, Accounting, Maintenance and Lending queues.
- **Data definitions**: named datasets with filters, sorting, grouping, counts and relations between models.
- **Status categories**: a model can say which of its statuses are a start, in progress, needing attention, done or
  stopped, the basis for drawing a process map.
- **Strict loading**: a mistake in a metadata file is reported when it loads, not discovered on a screen.

**Workspaces and people**
- Accounts with registration, email verification, login and password reset.
- Workspaces with members, invitations, roles per application, groups, archive and restore.
- Applications are installed into a Workspace as its own copy, so each Workspace can change its applications
  without affecting another. A broken Workspace no longer stops the others.
- Notifications in the app and by email; scheduled reminders.

**Applications built on the runtime**
- **Document Approval**: submission wizard, drafts and revisions, sequential or parallel approval, saved approval
  flows per document type, SLA tracking, an approval inbox, signature placement and signing onto the PDF.
- **Project Management**: boards with lists and labels (each list has a menu to rename, move or delete it, and an Add list tile; a dragged card tilts and shows where it will land, and cards can be moved from the keyboard), a table of tasks in the board's order (completion circle, list, labels, due date, assignee), a calendar by week or month where a task card can be dragged to another day, one header over all three (the project's name, task counts, switch between Board, Table and Calendar, and a Switch project menu that narrows them), My Tasks, dashboard, checklists. **Production Contracts and Instalments & Payments**: contracts, instalments, payment plans with approval and proof, printable purchase orders.

**AI Metadata Assistant**
- Describe an application in plain words; review, edit and install the generated metadata into a Workspace.

**Design system**
- A theme a Workspace can adjust (radius, weight, text size, spacing, colours, padding and more), light and dark
  mode following the device, fifteen shared components, design tokens exported in the W3C format, and contrast and
  touch-target checks on the shipped colours and controls.

**Quality**
- Architecture, accessibility and design-system checks run on every change; security hardening of
  authentication, uploads, sessions and cross-workspace access.

## In progress

- **Bringing the documentation up to date with what has shipped.**

## Planned

In order. Each stage is broken into small, finished steps.

**1. The AI assistant can write everything the runtime can do**
- Updating an application through the assistant keeps everything it does not understand.
- The assistant says clearly what it cannot write yet, and those requests are recorded and reviewed.
- It writes defaults, completion, owner fields, notifications, boards and card layouts, datasets and declared
  screens, including screens per user, filter tiles and work queues.
- From now on, a new metadata capability arrives with the assistant's ability to write it.
- A set of reference applications, written ahead of the runtime, shows which capability to build next.

**2. Every application screen is declared, not coded**
- Workspace-level declared screens, and conditions on who is viewing, so members, groups, settings and
  Application Settings can be declared.
- A table component, reference and group fields in declared forms, child records and uploads in declared forms,
  a single record's own page.
- "Mine" lists (My Tasks, the approval inbox) defined by data instead of code.
- Done means: a new data model appears on every generic screen with no code change.

**3. More business rules in metadata**
- Who may read which records, deny-by-default.
- Multi-level approval for any application.
- A transition from several statuses; date-and-time fields (SLA by the hour); calculated money, numbering, currency.
- Editing a document limited to its submitter; deleting a field without leaving orphaned data.

**4. Remaining mockup screens**
- A mobile bottom bar, an account menu and page, touch drag on the board.
- Account deletion with a grace period, deactivating an application, notification settings per application,
  card actions.

**5. Process map and Application Builder**
- A process map that draws a model's statuses and transitions, then the Application Builder screens on it.

**Alongside**
- A privacy policy and terms page; installable as an app (Play Store ready).
- Scaling the data layer (indexes, pagination, a query planner) when real data volume asks for it.

---

Full build history, architecture decisions, and audit records live in the private
`menata-app-document` repository.
