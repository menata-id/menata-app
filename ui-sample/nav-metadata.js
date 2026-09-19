// Single source of truth for the two-level navigation applied across every ui-sample mockup:
//   1. Menu Lintas Aplikasi -- which Applications exist in this Workspace (the 9-dot launcher).
//   2. Menu Aplikasi yang sedang berjalan -- the current Application's own ordered menu.
// See navigation.html for the original spec and ROADMAP.md Phase 21's research note for why this
// shape (an ordered {label, route, icon, priority} list per Application, no per-item metadata
// needed at the cross-Application level) is the one worth previewing as a real metadata schema.
// Every mockup includes this same file and nav-chrome.js instead of hand-copying this data, so
// there is exactly one nav list per Application to keep in sync, not one per file.
//
// Each item's `icon` is a single Unicode glyph, chosen per item's meaning (not by array position --
// nav-chrome.js and navigation.html previously each kept their own local icons-by-index array for
// the mobile bottom bar, and those two arrays had already drifted out of sync with each other. Both
// now read `item.icon` from here instead, so there is one place to keep in sync, not two). These are
// placeholders standing in for a real SVG icon set -- swapping to one is a separate, not-yet-scoped
// piece of work, not done here.
window.MenataNav = {
  workspace: "Acme Procurement",
  apps: {
    case3: {
      name: "Document Approval",
      color: "blue",
      icon: "✓",
      summary: "Case 3 · submit, review, approve",
      home: "document-approval.html",
      nav: [
        { id: "dashboard", label: "Dashboard", route: "/dashboard", mockup: "approval-dashboard.html", priority: 1, icon: "◆" },
        { id: "inbox", label: "Approval Inbox", route: "/approval-inbox", mockup: "document-approval.html", priority: 1, badge: 2, icon: "✓" },
        { id: "new", label: "New Approval", route: "/documents/new", mockup: "document-submit.html", priority: 2, icon: "＋" },
        { id: "signatures", label: "Signature Positions", route: "/machines/mch_document/records/{id}/signature-placement", mockup: "document-signature-placement.html", priority: 3, icon: "✎" }
      ]
    },
    case19: {
      name: "Project Management",
      color: "emerald",
      icon: "▤",
      summary: "Case 19 · boards, sprints, team",
      home: "project-board.html",
      nav: [
        { id: "workspace", label: "Project Workspace", route: null, mockup: "project-workspace.html", priority: 1, icon: "▤" },
        { id: "board", label: "Board", route: "/machines/mch_task", mockup: "project-board.html", priority: 1, badge: 5, icon: "☰" },
        { id: "my-tasks", label: "My Tasks", route: "/my-tasks", mockup: "project-my-tasks.html", priority: 1, icon: "☑" },
        { id: "calendar", label: "Calendar", route: "/calendar", mockup: "project-calendar.html", priority: 2, icon: "◷" },
        { id: "sprint", label: "Sprint", route: "/sprint", mockup: "project-dashboard.html", priority: 2, icon: "▣" },
        { id: "timeline", label: "Timeline", route: null, mockup: "project-timeline.html", priority: 3, icon: "▬" },
        { id: "team", label: "Team Capacity", route: "/team-capacity", mockup: "project-team.html", priority: 3, icon: "◍" },
        { id: "activity", label: "Activity", route: "/activity", mockup: "project-activity.html", priority: 3, icon: "●" },
        { id: "automation", label: "Automation", route: "/automation", mockup: "project-automation.html", priority: 4, icon: "⚙" },
        { id: "settings", label: "Board Settings", route: "/board-settings", mockup: "project-settings.html", priority: 4, icon: "⚏" }
      ]
    }
  }
};
