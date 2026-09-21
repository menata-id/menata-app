# Case 03 Flow 1 — Document Approval

Owner-supplied design mockup, received 2026-09-20. Twelve desktop boards (1280px wide) and ten
mobile boards (390px), covering the Document Approval flow end to end: sign in → choose workspace
→ workspace home → members/roles admin → approval inbox → submit → place signatures → review and
decide.

Like every other file in `ui-sample/`, this is a design **reference to build toward, never
current-code intent** — none of it is executed and no page here shares code with the real app.
Open it through the running app at `/ui-sample/case-03-flow1/` (or `index.html` directly; unlike
the older mockups, these files have no `/vendor` dependency and render standalone).

Two boards are explicitly marked **TIDAK DIPAKAI** (not used) by the owner and are kept only so the
set stays complete: `11-my-documents-UNUSED.html` and `12-approval-dashboard-UNUSED.html`.

## How it differs from the rest of `ui-sample/`

The older mockups are Tailwind-utility HTML. These are **inline-styled** exports from a design tool
(the `M01`–`M10` files are that tool's mobile artboards), so porting one means reading its computed
values rather than lifting its class list. The palette is also different — slate/`#2563eb` on
`#f8fafc` with the Ubuntu typeface, against the older mockups' own scheme — which is a deliberate
owner decision, not drift.

## What was normalized when unpacking

The file arrived as one 15MB self-contained bundle (`Menata_Runtime___Case_03_Flow1.html`): a
gzip+base64 manifest of 22 nested page bundles, each carrying its own copy of the fonts and of the
design tool's runtime. It was unpacked into one plain HTML file per board, and four things were
rewritten so the result is reviewable and diffable rather than opaque:

1. **Fonts deduplicated.** All 22 pages shipped the same 18 Ubuntu `woff2` files inline; they are
   extracted once to `fonts/` and referenced relatively. This is what takes the directory from
   15MB to under 1MB.
2. **Design-tool runtime dropped.** Each page loaded ~186KB of `x-dc` runtime whose whole job was
   `renderVals() { return {} }` — these boards are static markup, so the `<script>`, the
   `<x-dc>` wrapper element and the trailing `<script type="text/x-dc">` are removed.
3. **Tool-specific attributes restored to real HTML** — `sc-camel-view-box` → `viewBox`,
   `sc-raw-select` → `select`. Without this the SVG icons and every dropdown render wrong once the
   runtime above is gone.
4. **`<helmet>` hoisted into `<head>`**, and a `viewport` meta added. The design tool emitted its
   head content as an in-body portal element that only its runtime relocated.

Nothing inside the boards' own markup — structure, inline styles, copy, ARIA — was changed. The
original bundle is not committed; it is reproducible from this directory, and the unpacking script
is described here rather than kept, since it is single-use.

## Cross-links used by the boards

The boards link to each other by the design tool's own names (`Inbox.dc.html`,
`WorkspaceHome.dc.html`, `Review.dc.html`, …), which do not exist here. Those links are dead on
purpose — they record the *intended* flow, and `index.html` is the way to move between boards.

## `06b-authorization-matrix.html` — a proposed redesign, kept beside board 06

Drawn 2026-09-21 after an authorization review, and **added rather than replacing
`06-approval-role-matrix.html`**, which stays exactly as the owner supplied it.

Board 06 draws a *stage* model: a Document holding `Draft → In Review → Finance OK → Legal OK →
Approved`, each stage's transition gated on a role. This app derives a Document's status from its
Approval Steps and treats *who approves* as per-record data the submitter picks, so under the
model that actually runs the whole Application has exactly two role-gated transitions — and a
matrix of transitions alone is nearly empty however much work goes into the screen. `ROADMAP.md`'s
deferral table carries that as a model decision for the owner, not as screen work.

What 06b changes is the premise, not the styling: a transition is not the only thing a role gates.
It adds the record actions (create/edit/delete) that Permission has always governed and nothing
ever drew, and a Workspace section for the rules that are not an Application's to make. Every row
on it is a real declaration in `metadata/*.yaml`, including the honest blanks — an action nobody
governs is drawn as open, because a matrix that hides what is ungoverned is worse than no matrix.

Its two Workspace rows follow `ui-sample/member-role-detail.html` rather than board 06: that
mockup puts `Admin`/`Member` in a "Workspace role" section captioned *"independent of application
permissions"*, above a separate "Application access" section — so the matrix's role columns are
Application roles only, and board 06's own `ADMIN` and `MEMBER` columns have no place in them.
