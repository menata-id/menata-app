# UI Sample

Static HTML design mockups, copied from `menata-runtime`'s (private) `app/web/static/ui-sample/`
on 2026-09-15. These are design **references** to build toward, never current-code intent — none
of this HTML is executed, and no page here shares any code with the real app. The running app does
serve this directory as-is at `/ui-sample/*` (a plain file server, no rendering logic), so a built
page can be compared against the design it was built toward; opening a file directly in a browser
works just as well.

Each file was originally self-contained (Tailwind via CDN, no relative asset dependencies), so it
rendered standalone even outside its original directory. That is no longer true of every file as
of the two-level navigation below (owner decision, 2026-09-19): every mockup that carries the nav
chrome now also needs `nav-metadata.js` and `nav-chrome.js` alongside it to render correctly --
which, per the section below, is every file in the Case 3 and Case 19 lists plus four of the six
platform-level screens. Only `login.html` and `choose-workspace.html` (no Workspace or Application
chosen yet) and `index.html` (this directory's own index, never had a nav concept) stay dependency-free.

## Menu Navigasi — two-level navigation, applied across every mockup

`navigation.html` (added 2026-09-19) specced two levels, owner-requested reference for the
platform-onboarding plan in `ROADMAP.md`:
1. Menu Lintas Aplikasi - di pojok kiri atas, dengan icon titik 9, jika diklik akan membuka menu untuk pilih ke aplikasi mana yang akan dibuka
2. Menu Aplikasi yang sedang berjalan - untuk desktop ada di atas, kalau untuk di mobile, ada di bagian bawah berupa bottom bar menu, dengan 3-4 icon.

It first shipped only as `navigation.html`'s own hub page, specifically to avoid touching the
other mockups (see git history for that reasoning). Owner decision (2026-09-19) reversed that:
the nav concept needed to be exercised on every real screen it will eventually replace, not
demonstrated once in isolation. What changed:

- **`nav-metadata.js`** — the single source of truth: an ordered `{label, route, mockup, priority,
  badge}` list per Application (Case 3, Case 19), plus the Workspace's own Application list for
  the cross-Application launcher. One list per Application, not one per file -- see
  `ROADMAP.md` Phase 21's research note for why this exact shape was chosen as a preview of a real
  declared navigation schema.
- **`nav-chrome.js`** — merges that data into each mockup's own existing `<header>`, rather than
  stacking a second bar above it (owner correction, 2026-09-19, after the first pass added a
  separate sticky top bar): the 9-dot launcher is inserted to the left of whatever brand/breadcrumb
  text that header already shows, and the current Application's own desktop menu becomes a second
  row inside that same `<header>` (a `border-t` below the first row, not a new element outside it).
  Only the mobile bottom bar (top 4 items by `priority`) is genuinely a separate bar, because the
  spec places it there on purpose ("untuk mobile ada di bagian bawah berupa bottom bar"). Call
  `MenataNav.mount(appId, activeId)` for a screen inside an Application, or `MenataNav.mount()`
  with no arguments for a Workspace-level screen (launcher only, no Application open yet).
- Any literal "Menata Runtime" text already in that header is replaced with the active Workspace's
  own name (`nav-metadata.js`'s `workspace` field, "Acme Procurement" today) -- the header already
  answers "which Workspace", it should say so instead of repeating the product name next to it.
- Every Case 3 and Case 19 screen, plus the Workspace-level screens that already assume a signed-in
  identity (`workspace-home.html`, `workspace-members.html`, `member-role-detail.html`,
  `approval-role-matrix.html`), now loads both scripts and calls `MenataNav.mount(...)`. `login.html`
  and `choose-workspace.html` are deliberately excluded -- they exist *before* a Workspace or
  Application is chosen, so neither level of this nav has anything to show yet.
- Each mockup's own pre-existing local header/breadcrumb (e.g. "← Back to board", a page title) was
  left as-is -- that is page-local chrome, a different concern from the platform-level nav this
  change adds. Two exceptions, both genuinely dead or duplicate chrome rather than something worth
  preserving: `document-approval.html`'s own mobile bottom bar (a literal duplicate of the new one),
  and `approval-dashboard.html`'s `hx-boost` tab row (linked to `worklist.html`/`process-map.html`/
  `groups.html`, none of which exist in this trimmed copy).
- The amber "UI Sample — ..." disclaimer banner that eight of these mockups carried (a leftover
  from `menata-runtime`'s own fuller page set) was removed -- it duplicated what this README and
  `index.html`'s own banner already say about every file here, and crowded the header it sat above.
- `navigation.html` itself now loads `nav-metadata.js` too (dropping its own inline copy of the
  same data) and remains the fuller, annotated version of the same nav -- explaining `route`,
  `badge`, and the "planned" (no real route yet) treatment for `mockup`s.

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

## Style-consistency and a11y fixes (2026-09-19 audit)

An audit of all 24 files found and fixed, directly in the mockups:

- **Container-width bugs, not just style**: `project-calendar.html`, `project-dashboard.html`,
  `project-timeline.html` had their two-row header at `max-w-[1500px]` (copied from
  `project-board.html`) but `<main>` still at `max-w-7xl` -- content edges didn't line up with the
  header above them. `project-my-tasks.html` had the same mismatch (`7xl` header, `6xl` main). Fixed
  by matching each file's `<main>` to its own header.
- **`login.html`'s primary button** was the only `bg-blue-600` CTA in the whole directory; every other
  primary action uses `bg-slate-900`. Fixed to match.
- **Category tags vs. status colors collided**: `rose`/`amber`/`emerald`/`blue` are used for
  status/urgency everywhere (`document-approval.html`, `project-board.html`'s "On track"/"Due today",
  etc.), but `project-board.html` and `project-card.html` also used `blue`/`amber`/`emerald` for the
  unrelated category tags Design/Planning/Frontend -- a category label could misread as an urgency
  signal. Recolored to a disjoint palette: Design→`indigo`, Planning→`sky`, Frontend→`teal`,
  UX→`fuchsia`, QA→`cyan` (Research stayed `violet`, already collision-free).
- **Bottom-nav icon drift**: `nav-chrome.js` and `navigation.html` each kept their own local
  `icons`-by-array-index list for the mobile bottom bar, and the two had already drifted (4 vs. 5
  entries). `nav-metadata.js` now carries an explicit `icon` per nav item; both renderers read that
  instead of a local array, so there's one place left to keep in sync.
- **Accessibility**: `document-submit.html`'s labels now have matching `for`/`id`; icon-only buttons
  (list `•••` menus, step `▲`/`▼`/`✕`) got `aria-label` plus enough padding to approach a 44px touch
  target; a shared `:focus-visible` outline rule (previously present only on `login.html`'s text
  inputs) is now injected by `nav-chrome.js` for every mockup that calls `MenataNav.mount()`, and
  added inline to the three that don't (`login.html`, `choose-workspace.html`, `navigation.html`).
- **`project-board.html` touch gap**: added `scroll-snap` so the horizontal list scroll lands cleanly
  per column on touch, and an explicit note in the page's own "Interaction model" callout that
  HTML5 `draggable` (used for card drag today) has no real touch equivalent -- a long-press gesture
  is still undesigned, not silently assumed to work.

Not done in this pass, on purpose: card-padding standardization (`p-3`/`p-4`/`p-5`/`p-6` variation
doesn't reduce to a clear rule without inventing one), a full SVG icon set (the Unicode glyphs above
are still placeholders), and a mobile-specific Kanban layout (kept as the existing horizontal-scroll
pattern, now touch-snapped, not redesigned).

## Design tokens (reference for later, not applied to these mockups)

An audit of all 24 files (2026-09-19) found real style drift -- inconsistent container widths, a
primary-CTA color that only `login.html` disagreed on, and category tags reusing the same hues as
status colors. Those were fixed directly in the mockups. Dark mode was flagged in the same audit but
deliberately **not** added here -- owner decision (2026-09-19): document the intended light/dark
color-role mapping as a reference table, don't touch 24 files' worth of markup for a mode nothing
renders yet. Whoever builds a real dark theme (mockup or `internal/rendering`) starts from this table
rather than re-deriving it:

| Role | Light (in use today) | Dark (proposed) |
|---|---|---|
| Page background | `bg-slate-50` | `bg-slate-950` |
| Card/surface background | `bg-white` | `bg-slate-900` |
| Border | `border-slate-200` | `border-slate-800` |
| Primary text | `text-slate-900` | `text-slate-100` |
| Secondary/muted text | `text-slate-500` | `text-slate-400` |
| Primary CTA (buttons like "Sign in", "Approve") | `bg-slate-900 text-white` | `bg-white text-slate-900` (inverted, same convention GitHub's dark theme uses for its primary button) |
| Status: danger/overdue (`rose`) | `bg-rose-50 text-rose-700` | `bg-rose-950 text-rose-300` |
| Status: warning/due-soon (`amber`) | `bg-amber-50 text-amber-700` | `bg-amber-950 text-amber-300` |
| Status: success/on-track (`emerald`) | `bg-emerald-50 text-emerald-700` | `bg-emerald-950 text-emerald-300` |
| Status: info/active (`blue`) | `bg-blue-50 text-blue-700` | `bg-blue-950 text-blue-300` |
| Category tags (`indigo`/`sky`/`teal`/`fuchsia`/`cyan`/`violet` -- kept separate from the 4 status hues above so a category label can't be misread as an urgency signal) | `bg-{hue}-50 text-{hue}-700` | `bg-{hue}-950 text-{hue}-300` |

The dark column follows Tailwind's own documented pattern for its palette (light mode: `-50` background
with `-700` text; dark mode: `-950` background with `-300` text, chosen for WCAG-contrast reasons, not
just inverted lightness) -- not a new convention invented for this file.

## What's deliberately not copied

The other ~19 cases' own mockups, and the navigational chrome (`index.html`, `case.html`,
per-case redirect stubs) that ties everything back into `menata-runtime`'s own case-browsing UI.
See `case-portfolio.md` (root of this repo) for all 21 cases' descriptions -- only Case 3 and
Case 19 are the current priority (owner decision, 2026-09-15), so only their mockups are here.
