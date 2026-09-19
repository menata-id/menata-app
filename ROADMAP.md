# Menata App Roadmap

A high-level view of what menata-app can do today and what's coming next, kept at the feature
level on purpose. It is not the same document as the root-level concept docs (001-007), which
describe the target architecture. The full day-to-day development history -- design rationale,
forcing conditions, verification steps -- is tracked in a private companion repository,
`menata-app-document`.

## Shipped

- **Metadata-driven CRUD** -- define a data model ("Machine") in YAML and get a working table or
  board view, create/edit/delete, relations (one-to-many and many-to-many), nested child
  collections, and file attachments, with no per-model code required.
- **Multi-user accounts** -- registration (creates a workspace), email verification, login,
  password reset, and session-based authentication.
- **Workspaces & members** -- invite teammates, assign per-application roles, a workspace home
  screen.
- **Document Approval** -- submission wizard, sequential or parallel multi-step approval, SLA
  tracking, an approval inbox and dashboard, an activity feed, drag-to-place PDF signatures, and
  automatic signature compositing onto the approved document.
- **Project Management** (core mechanics shipped, still rounding out) -- task boards with labels
  and ordered lists, my-tasks view, calendar, sprint dashboard, team capacity view, activity feed.
- **Grouped navigation**, declared as metadata rather than hardcoded per page.
- **Continuous architecture and code-quality checks** running in CI on every change.
- **Authentication and file-handling hardening**, based on an internal security review --
  invite-acceptance and session handling, upload validation and access checks, security response
  headers, and CSRF protection.

## In progress

- Rounding out Project Management: a project-level workspace overview, richer task detail
  (checklist, comments, attachments), and scoping views to one project at a time.
- Two-level navigation for switching between applications inside a workspace.
- Accessibility and mobile/responsive polish across existing screens.

## Planned

- Installable as a PWA (Progressive Web App) -- add to home screen on a phone and open it like a
  native app, no app-store install required.
- Group-based roles and a visual approval role matrix.
- Permission-aware action visibility -- buttons and actions should follow the current user's
  authorization, not just record/page state: an action the user isn't allowed to perform should be
  hidden, not merely disabled, driven by the same permission decision the backend already
  enforces (today some actions are conditioned on state, e.g. `detail.templ`'s
  `action.CanDeleteApprovalStep`/`CanDeleteDocument`, but not on who the user is).
- Composable workflow/process breakdown -- UI composition already has an answer (the Shared
  rendering components / composition primitives catalogued in `capabilities.md`); Behavior
  composition does not yet -- reusable events, actions, constraints, permissions, and process
  primitives, the third composition dimension in `007-composable-runtime-architecture.md` §1.
  Before designing this, read `001-design-principles.md` through
  `007-composable-runtime-architecture.md` so workflow/process breakdown ends up composable and
  metadata-driven the same way visual composition already is, without regressing runtime
  performance.
- Search, filtering and pagination on record lists.
- Background/scheduled jobs (e.g. SLA-breach notifications that don't depend on someone opening
  the page).
- Expanding beyond the first two applications into the wider portfolio of business cases this
  runtime is designed to support (HR, inventory, point of sale, e-commerce, helpdesk, and more).

---

Full build history, architecture decisions, and audit records live in the private
`menata-app-document` repository.
