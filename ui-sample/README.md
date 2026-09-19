# UI Sample

Static HTML design mockups, copied from `menata-runtime`'s (private) `app/web/static/ui-sample/`
on 2026-09-15. These are design **references** to build toward, never current-code intent — none
of this HTML is executed, and no page here shares any code with the real app. The running app does
serve this directory as-is at `/ui-sample/*` (a plain file server, no rendering logic), so a built
page can be compared against the design it was built toward; opening a file directly in a browser
works just as well.

Each file is self-contained (Tailwind via CDN, no relative asset dependencies), so it renders
standalone even outside its original directory.

## What's here and why

**Menu Navigasi** — `navigation.html` (added 2026-09-19, owner-requested reference for the
platform-onboarding plan in `ROADMAP.md`)
1. Menu Lintas Aplikasi - di pojok kiri atas, dengan icon titik 9, jika diklik akan membuka menu untuk pilih ke aplikasi mana yang akan dibuka
2. Menu Aplikasi yang sedang berjalan - untuk desktop ada di atas, kalau untuk di mobile, ada di bagian bawah berupa bottom bar menu, dengan 3-4 icon.

Unlike every other file here, `navigation.html` is not copied from `menata-runtime` -- it applies
this spec to this app's own two real cases (Case 3 "Document Approval", Case 19 "Project
Management") using their actual `internal/web/router.go` routes, not a generic example. Still a
reference only: its own inline JS exists to demonstrate the app-switching behavior for review, not
as an implementation to copy.

**Case 3 (Document Approval) — all 4 screens**, per `case-portfolio.md`'s own screen list:

- `document-submit.html` — Submit document (Wizard/Form)
- `document-signature-placement.html` — Signature positions (Coordinate editor)
- `document-approval.html` — Approval inbox (Worklist + Detail)
- `approval-dashboard.html` — Approval dashboard (Composed page)

**Case 19 (Project Management) — the current 11-screen expanded set**, per that repo's own
`case-19.html` (dated 2026-09-13, superseding its original 3-screen scope):

- `project-workspace.html` — Project Workspace
- `project-board.html` — Board Workspace (Kanban)
- `project-card.html` — Task Detail
- `project-timeline.html` — Timeline / Roadmap
- `project-calendar.html` — Calendar
- `project-dashboard.html` — Sprint Dashboard / Insights
- `project-team.html` — Team Capacity
- `project-my-tasks.html` — My Tasks
- `project-activity.html` — Project Activity
- `project-automation.html` — Workflow Automation
- `project-settings.html` — Board Settings
- `case-19-component-breakdown.html` — a reusable-component inventory across the above screens

**Platform-level (not case-specific)** — generic screens every application needs, useful as
design reference regardless of which case is being built:

- `login.html`, `choose-workspace.html`, `workspace-home.html`, `workspace-members.html`,
  `member-role-detail.html`, `approval-role-matrix.html`

## What's deliberately not copied

The other ~19 cases' own mockups, and the navigational chrome (`index.html`, `case.html`,
per-case redirect stubs) that ties everything back into `menata-runtime`'s own case-browsing UI.
See `case-portfolio.md` (root of this repo) for all 21 cases' descriptions -- only Case 3 and
Case 19 are the current priority (owner decision, 2026-09-15), so only their mockups are here.
