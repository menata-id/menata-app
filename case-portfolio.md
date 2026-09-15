# Case Portfolio

The 21 business scenarios `menata-runtime`'s own discovery phase used to prove what a Menata
Runtime needs to support (that repo's `case-portfolio.md` and `roadmap.md`, Studies 1-34). This
is the long-term scope `menata-app` should eventually be able to realize as real applications --
copied and condensed here (2026-09-15) from `app/web/static/ui-sample/case.html`'s own case data,
not hand-summarized, so the descriptions match the design mockups exactly.

**Only Case 3 and Case 19 are the current priority** (owner decision, 2026-09-15) -- the two
trial applications this repo is meant to actually build first. The other 19 are recorded here as
known future scope, not scheduled work; see `ROADMAP.md` for the phased build order and its own
"only build what's forced" method, which applies to which case gets tackled next just as much as
to which architecture piece gets built next.

## All 21 cases

| # | Case | Domain | What it does |
|---|------|--------|---------------|
| 1 | Design Request | Creative services workflow | Requester submits a design request; a Designer accepts or rejects it, starts work, and completes it. Banner 2:1 requests require an attachment. |
| 2 | Leave Request | HR · employee leave approval | An employee submits leave; a Manager approves or rejects; the employee may cancel before approval. |
| **3** | **Document Approval** | **Multi-approver document workflow** | **A submitted PDF follows a sequential or parallel approval flow. Approvers may be sourced from users or groups; signatures can be positioned on the PDF; the flow can be saved by document type.** |
| 4 | Maintenance Reminder | Facility · recurring equipment maintenance | Equipment maintenance tasks recur on schedules. A task becomes Due, escalates when overdue, and on completion records the completion date and advances the next due date by frequency. |
| 5 | Inventory / Stock Movement | Warehouse · multi-UoM stock control | Goods move in and out of a warehouse. Confirming a movement creates an immutable ledger entry, normalizes quantity by unit, updates stock on hand and rejects a movement that would make stock negative. |
| 6 | Petty Cash Ledger | Small cash box · imprest fund | A fixed imprest fund is controlled by one Custodian, every voucher reduces the current balance, an independent Auditor reconciles the period and a closed period is frozen. |
| 7 | Customer Complaint | Customer service · ad-hoc case management | Complaints can be investigated in no fixed order, have priority-driven SLA, escalate on breach and can be reopened after resolution. |
| 8 | Payment Confirmation | Payments · webhook ingestion and reconciliation | A payment provider sends a webhook. Duplicate delivery must return success without double-applying; matched payments update the Invoice exactly once, while unmatched payments wait for manual reconciliation. |
| 9 | Accounting | Small-org bookkeeping · monthly close | Accountants create journal entries with debit/credit lines, a Supervisor posts them, closed periods are immutable and Trial Balance groups totals by account. |
| 10 | Organization Composite | One organization running multiple applications together | An organization runs multiple applications in one workspace; one employee crosses HR, Facility, Approval and Safety in a single morning. |
| 11 | Social App | Instagram-like social application | Members post photos, follow other members, like and comment, and see a feed from people they follow. |
| 12 | Community Site | Groups, events and participation | Members join Groups, Groups host Events, members participate and earn points that can unlock badges. |
| 13 | Blog / One-Page Site | Public blog | Anyone can read published posts and submit comments without login; only an authenticated Author writes and moderates. |
| 14 | Lending Services | Loan application and repayment schedule | A borrower applies for a loan. A Loan Officer approves it; disbursement generates the full monthly repayment schedule; repayments update installments and overdue installments are flagged. |
| 15 | E-commerce | Product browsing, cart and checkout | Customers browse products, edit a cart freely and Checkout converts the cart into a real Order; payment reuses the Payment machine. |
| 16 | Point of Sale | Cashier checkout | A cashier rings up line items and takes payment in one motion, with no cart lingering and no webhook delay. |
| 17 | Helpdesk | Internal IT support | Internal support tickets use the same discretionary, SLA-timed and reopenable shape as Customer Complaint. |
| 18 | HR Operations | Employee master data | Adds the Employee master that Leave, Helpdesk and other apps reference. |
| **19** | **Project Management** | **Trello-like project boards** | **Boards contain Lists and Lists contain Cards. Users reorder Lists and Cards freely and move Cards between Lists.** |
| 20 | Hospital System | Appointments and clinical records | Patients are scheduled for Appointments. Each visit produces a Medical Record whose sensitive notes are visible only to the treating clinician. |
| 21 | E-learning | Course, lessons and certificates | Students enroll in courses, progress through sequentially unlocked lessons and receive a rendered certificate once complete. |

---

## Case 3 — Document Approval (priority)

**Roles:** Submitter · Approver · System. **Flow:** Draft → In Review → Approved / Rejected.

Design mockups: [`ui-sample/`](ui-sample/) (copied from `menata-runtime`).

| # | Screen | Type | What it shows | Mockup |
|---|--------|------|----------------|--------|
| 1 | Submit document | Wizard / Form | Document metadata, PDF attachment, Sequential/Parallel mode, per-step User/Group assignee, step reordering, optional saved flow | [`document-submit.html`](ui-sample/document-submit.html) |
| 2 | Signature positions | Coordinate editor | PDF preview with a marker per approval step | [`document-signature-placement.html`](ui-sample/document-signature-placement.html) |
| 3 | Approval inbox | Worklist + Detail | Pending approvals, SLA chips, approval progress, PDF preview, Approve/Reject action bar | [`document-approval.html`](ui-sample/document-approval.html) |
| 4 | Approval dashboard | Composed page | Summary KPI tiles + pending-document list + recent-activity feed | [`approval-dashboard.html`](ui-sample/approval-dashboard.html) |

Runtime concepts this case exercises against the concept docs (001-007): a Machine with a
multi-step Constraint/Permission-gated status flow (approval steps as ordered related records,
not a single status field); Permission sourced from either a User or a Group; file
upload/storage; a composed Dashboard page combining multiple Datasets (Summary/Pending/Activity)
-- a second real candidate for testing ROADMAP.md's Phase 6 forcing condition, since this
combines three different aggregate shapes on one page, not just two whole-machine lists like the
current `/dashboard` does.

## Case 19 — Project Management (priority)

**Roles:** implicit (workspace members). **Flow:** Boards → Lists → Cards, freely reordered and
moved between Lists.

Design mockups: [`ui-sample/`](ui-sample/) (copied from `menata-runtime`, current 11-screen
expanded set per that repo's `case-19.html`, 2026-09-13).

| # | Screen | What it shows | Mockup |
|---|--------|----------------|--------|
| 1 | Project Workspace | Workspace-level project discovery, project cards, progress, recent activity | [`project-workspace.html`](ui-sample/project-workspace.html) |
| 2 | Board Workspace | Lists, cards, labels, members, checklist progress, drag/drop | [`project-board.html`](ui-sample/project-board.html) |
| 3 | Task Detail | Description, checklist, activity, comments, attachments, metadata | [`project-card.html`](ui-sample/project-card.html) |
| 4 | Timeline / Roadmap | Workstreams, date-range bars, milestones | [`project-timeline.html`](ui-sample/project-timeline.html) |
| 5 | Calendar | Due dates, scheduled work, milestones | [`project-calendar.html`](ui-sample/project-calendar.html) |
| 6 | Sprint Dashboard / Insights | KPIs, burndown, workload, attention-needed projections | [`project-dashboard.html`](ui-sample/project-dashboard.html) |
| 7 | Team Capacity | Members, allocation, active ownership | [`project-team.html`](ui-sample/project-team.html) |
| 8 | My Tasks | User-centric task queue projected across projects | [`project-my-tasks.html`](ui-sample/project-my-tasks.html) |
| 9 | Project Activity | Cross-project event feed: actor, object, action, time | [`project-activity.html`](ui-sample/project-activity.html) |
| 10 | Workflow Automation | Trigger → condition → action metadata | [`project-automation.html`](ui-sample/project-automation.html) |
| 11 | Board Settings | Board identity, ordered statuses, reusable labels | [`project-settings.html`](ui-sample/project-settings.html) |
| — | UI Component Breakdown | Reusable-component inventory across the screens above | [`case-19-component-breakdown.html`](ui-sample/case-19-component-breakdown.html) |

Where this repo already stands against it (2026-09-15): screen 2 (Board) and part of screen 6
(status grouping) are the working `mch_task`/`mch_project` Relation + board Layout already built
in Phases 3-5. Labels, multiple Members per card, checklist, comments/attachments, timeline,
calendar, my-tasks, activity feed, automation, and board settings are all still unbuilt here --
real future scope, not yet attempted.
