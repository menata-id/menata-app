// Renders the two-level navigation (nav-metadata.js's data) into any ui-sample mockup, merged
// into that page's own existing <header> rather than stacking a second bar above it:
//   - the 9-dot launcher goes to the left of whatever brand/breadcrumb text the header already
//     shows, and any literal "Menata Runtime" text in that header is replaced with the active
//     Workspace's own name (nav-metadata.js's workspace field) -- the header already answers
//     "which Workspace", it should say so, not repeat the product name.
//   - the current Application's own desktop menu becomes a second row inside that same <header>
//     (a border-t below the first row, no new element outside it).
//   - the mobile bottom bar is still a separate bar, but only because the spec places it there on
//     purpose ("untuk mobile ada di bagian bawah berupa bottom bar").
//   - the header's own avatar (previously a static "NS" span) becomes an Account menu button, per
//     an owner-supplied mockup pair added 2026-09-21 (M11/M12 mobile, 13/14 desktop). Both this
//     panel and the launcher's now open as a bottom sheet on mobile and a dropdown on desktop --
//     the mockups gave both menus the same responsive shape, so wiring them one way (wireMenus(),
//     sheetPanelClasses()) keeps that in sync rather than teaching the launcher and the Account
//     menu two slightly different behaviors.
//
// Usage, at the end of <body>:
//   <script>MenataNav.mount("case3", "inbox");</script>          -- full chrome: launcher + this
//                                                                    Application's own menu
//   <script>MenataNav.mount();</script>                          -- launcher only, no Application
//                                                                    open yet (Workspace-level
//                                                                    screens: Workspace Home,
//                                                                    Workspace Members, etc.)
(function () {
  "use strict";

  function launcherPanelHTML(currentAppId) {
    var data = window.MenataNav;
    var rows = Object.keys(data.apps).map(function (appId) {
      var app = data.apps[appId];
      var isCurrent = appId === currentAppId;
      return (
        '<a href="' + app.home + '" class="flex w-full items-start gap-3 rounded-md p-2 text-left hover:bg-slate-50">' +
          '<span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-' + app.color + '-50 text-' + app.color + '-700">' + (isCurrent ? "✓" : app.icon) + '</span>' +
          '<span><span class="block text-sm font-medium">' + app.name + '</span><span class="block text-xs text-slate-500">' + app.summary + '</span></span>' +
        '</a>'
      );
    }).join("");
    return (
      '<p class="px-2 pb-2 pt-1 text-xs font-medium uppercase tracking-wide text-slate-400">Menata · ' + data.workspace + '</p>' +
      rows +
      '<a href="choose-workspace.html" class="mt-1 block rounded-md p-2 text-xs font-medium text-blue-600 hover:bg-slate-50">All Workspaces →</a>'
    );
  }

  // The Account menu's own content (13/14/M11/M12's owner-supplied mockup pair): the signed-in
  // user, their role badge in the active Workspace, Profile/Notifications/Security (account-
  // profile.html / account-notifications.html / account-security.html, added 2026-09-21 -- see
  // ui-sample/README.md's own section on them; there is still no `domain` model or real route
  // behind any of the three, only a design reference), the Workspace quick-switch list, and Sign
  // out. Reads nav-metadata.js's currentUser/workspaces instead of hardcoding any of this a
  // second time.
  function accountPanelHTML() {
    var data = window.MenataNav;
    var user = data.currentUser;
    var current = data.workspaces.filter(function (w) { return w.current; })[0];
    var wsRows = data.workspaces.map(function (w) {
      return (
        '<a href="' + (w.current ? "workspace-home.html" : "choose-workspace.html") + '" class="flex items-center gap-2.5 rounded-md px-3 py-2 text-left hover:bg-slate-50">' +
          '<span class="flex w-3.5 shrink-0 justify-center text-xs text-blue-600">' + (w.current ? "✓" : "") + '</span>' +
          '<span class="flex-1 text-sm">' + w.name + '</span>' +
          '<span class="text-xs text-slate-400">' + w.role + '</span>' +
        '</a>'
      );
    }).join("");
    return (
      '<div class="flex items-center gap-3 px-3 pb-3 pt-2">' +
        '<span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-blue-100 text-sm font-semibold text-blue-700">' + user.initials + '</span>' +
        '<span class="min-w-0"><span class="block truncate text-sm font-medium">' + user.name + '</span><span class="block truncate text-xs text-slate-400">' + user.email + '</span></span>' +
      '</div>' +
      (current ? '<div class="mx-3 mb-2 flex items-center gap-2 rounded-md bg-slate-50 px-2.5 py-2 text-xs text-slate-600"><span>' + current.name + '</span><span class="rounded-full bg-blue-50 px-2 py-0.5 text-[10px] font-medium text-blue-700">' + current.role + '</span></div>' : "") +
      '<div class="my-1.5 h-px bg-slate-100"></div>' +
      '<a href="account-profile.html" class="block rounded-md px-3 py-2 text-sm hover:bg-slate-50">Profile</a>' +
      '<a href="account-notifications.html" class="block rounded-md px-3 py-2 text-sm hover:bg-slate-50">Notifications</a>' +
      '<a href="account-security.html" class="block rounded-md px-3 py-2 text-sm hover:bg-slate-50">Security</a>' +
      '<div class="my-1.5 h-px bg-slate-100"></div>' +
      '<p class="px-3 pb-1 pt-1 text-[10px] font-medium uppercase tracking-wide text-slate-400">Workspaces</p>' +
      wsRows +
      '<a href="choose-workspace.html" class="mt-0.5 block rounded-md px-3 py-2 text-xs font-medium text-blue-600 hover:bg-slate-50">Switch workspace →</a>' +
      '<div class="my-1.5 h-px bg-slate-100"></div>' +
      '<a href="login.html" class="block rounded-md px-3 py-2 text-sm hover:bg-slate-50">Sign out</a>'
    );
  }

  // Shared responsive shape for every panel this file opens: a bottom sheet (full width, anchored
  // to the viewport bottom, dismissed by its own backdrop) below the `sm` breakpoint, the
  // pre-existing anchored dropdown at `sm` and up -- the owner-supplied mockup pair (M11/M12
  // mobile, 13/14 desktop) gives the launcher and the Account menu the same two shapes, so both
  // read this one function rather than each hand-rolling its own breakpoint behavior.
  function sheetPanelClasses(desktopAlign) {
    return (
      "menata-panel fixed inset-x-0 bottom-0 z-40 hidden max-h-[80vh] overflow-y-auto rounded-t-2xl " +
      "border-t border-slate-200 bg-white p-2 pb-6 shadow-xl text-left " +
      "sm:absolute sm:inset-x-auto sm:bottom-auto sm:top-9 sm:max-h-none sm:w-72 sm:overflow-visible " +
      "sm:rounded-lg sm:border sm:shadow-lg sm:" + desktopAlign + "-0"
    );
  }

  // A bottom sheet needs its own backdrop (mobile only, `sm:hidden`) to dim the page behind it and
  // give the user a large, obvious tap target to dismiss it -- the desktop dropdown needs neither.
  function panelBackdropHTML() {
    return '<div class="menata-panel-backdrop fixed inset-0 z-30 hidden bg-slate-900/40 sm:hidden"></div>';
  }

  // The drag-handle bar every bottom sheet in the mockups shows (`sm:hidden`, absent on desktop
  // where the same panel renders as a plain dropdown with no such affordance).
  function sheetHandleHTML() {
    return '<div class="mx-auto mb-1 h-1 w-10 rounded-full bg-slate-200 sm:hidden"></div>';
  }

  function launcherHTML(currentAppId) {
    return (
      '<span class="menata-menu menata-launcher relative inline-flex shrink-0 items-center align-middle">' +
        panelBackdropHTML() +
        '<button type="button" class="menata-menu-btn menata-launcher-btn flex h-8 w-8 items-center justify-center rounded-md text-slate-500 hover:bg-slate-100" aria-haspopup="true" aria-expanded="false" aria-label="Switch application">' +
          '<svg width="16" height="16" viewBox="0 0 18 18" fill="currentColor"><circle cx="3" cy="3" r="1.6"/><circle cx="9" cy="3" r="1.6"/><circle cx="15" cy="3" r="1.6"/><circle cx="3" cy="9" r="1.6"/><circle cx="9" cy="9" r="1.6"/><circle cx="15" cy="9" r="1.6"/><circle cx="3" cy="15" r="1.6"/><circle cx="9" cy="15" r="1.6"/><circle cx="15" cy="15" r="1.6"/></svg>' +
        '</button>' +
        '<div class="menata-launcher-panel ' + sheetPanelClasses("left") + '" role="menu" aria-label="Applications">' + sheetHandleHTML() + launcherPanelHTML(currentAppId) + '</div>' +
      '</span>'
    );
  }

  // Replaces the header's own static avatar span (e.g. `<span class="... rounded-full
  // bg-slate-200 ...">NS</span>`) with a real Account menu button + panel -- see wireAccountMenu().
  function accountHTML() {
    var user = window.MenataNav.currentUser;
    return (
      '<span class="menata-menu menata-account relative inline-flex shrink-0 items-center align-middle">' +
        panelBackdropHTML() +
        '<button type="button" class="menata-menu-btn menata-account-btn flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-200 text-xs font-semibold hover:bg-slate-300" aria-haspopup="true" aria-expanded="false" aria-label="Account menu">' + user.initials + '</button>' +
        '<div class="menata-account-panel ' + sheetPanelClasses("right") + '" role="menu" aria-label="Account">' + sheetHandleHTML() + accountPanelHTML() + '</div>' +
      '</span>'
    );
  }

  function itemLabelHTML(item) {
    return item.label +
      (item.badge ? ' <span class="rounded-full bg-red-100 px-1.5 text-[10px] font-semibold text-red-700">' + item.badge + '</span>' : '') +
      (!item.route ? ' <span class="text-[10px] text-slate-300">(planned)</span>' : '');
  }

  function appNavHTML(app, activeId) {
    return app.nav.map(function (item) {
      var isActive = item.id === activeId;
      var cls = "flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm " + (
        !item.mockup ? "cursor-default text-slate-300"
          : isActive ? "bg-" + app.color + "-50 font-medium text-" + app.color + "-700"
          : "text-slate-600 hover:bg-slate-50"
      );
      return '<a href="' + (item.mockup || "#") + '" class="' + cls + '"' + (isActive ? ' aria-current="page"' : '') + '>' + itemLabelHTML(item) + '</a>';
    }).join("");
  }

  function bottomNavHTML(app, activeId) {
    var top4 = app.nav.slice().sort(function (a, b) { return a.priority - b.priority; }).slice(0, 4);
    return top4.map(function (item) {
      var isActive = item.id === activeId;
      var tag = item.mockup ? "a" : "div";
      var cls = "flex flex-col items-center gap-0.5 py-2 text-[11px] " + (isActive ? "font-medium text-" + app.color + "-700" : "text-slate-400");
      var hrefAttr = item.mockup ? ' href="' + item.mockup + '"' : '';
      return '<' + tag + hrefAttr + ' class="' + cls + '"' + (isActive ? ' aria-current="page"' : '') + '><span>' + item.icon + '</span><span>' + item.label + '</span></' + tag + '>';
    }).join("");
  }

  // Keyboard-focus visibility: none of these mockups' buttons/links define their own focus ring, so
  // a keyboard user loses track of focus everywhere except login.html's text inputs. Injected once
  // per page here rather than hand-added per file, since every Case 3/Case 19/Workspace-level mockup
  // already loads this script. login.html and choose-workspace.html (no nav chrome) carry the same
  // rule inline in their own <head>, since they never call mount().
  function injectFocusVisibleStyle() {
    if (document.getElementById("menataFocusVisible")) return;
    var style = document.createElement("style");
    style.id = "menataFocusVisible";
    style.textContent = ":focus-visible{outline:2px solid #2563eb;outline-offset:2px}";
    document.head.appendChild(style);
  }

  // Closes every open menu (launcher and/or Account) at once -- used when opening a different one,
  // clicking a bottom-sheet's own backdrop, clicking anywhere outside, or pressing Escape.
  function closeAllMenus() {
    document.querySelectorAll(".menata-panel").forEach(function (p) { p.classList.add("hidden"); });
    document.querySelectorAll(".menata-panel-backdrop").forEach(function (b) { b.classList.add("hidden"); });
    document.querySelectorAll(".menata-menu-btn").forEach(function (b) { b.setAttribute("aria-expanded", "false"); });
  }

  // Wires every menu this file renders (launcher, Account) the same way: click its button to
  // toggle its own panel + backdrop shut/open (closing any other open menu first), click the
  // backdrop, click outside, or press Escape to dismiss. One shared function rather than a
  // launcher-only one plus a near-duplicate for the Account menu, since both now open the same way.
  function wireMenus() {
    document.querySelectorAll(".menata-menu-btn").forEach(function (btn) {
      btn.addEventListener("click", function (e) {
        e.stopPropagation();
        var wrapper = btn.closest(".menata-menu");
        var panel = wrapper.querySelector(".menata-panel");
        var isOpen = !panel.classList.contains("hidden");
        closeAllMenus();
        if (!isOpen) {
          panel.classList.remove("hidden");
          var backdrop = wrapper.querySelector(".menata-panel-backdrop");
          if (backdrop) backdrop.classList.remove("hidden");
          btn.setAttribute("aria-expanded", "true");
        }
      });
    });
    document.addEventListener("click", closeAllMenus);
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape") closeAllMenus();
    });
  }

  // Finds the header's own pre-existing avatar span (every mockup that has one uses this same
  // `rounded-full bg-slate-200` shape -- see ui-sample/README.md) and swaps it for a real Account
  // menu button + panel. Scoped to this page's own <header> so it can't match one of the unrelated
  // per-record avatars (assignees, approvers, ...) the same class combination is used for
  // elsewhere on several of these pages (e.g. approval-dashboard.html's approver list). A page with
  // no such avatar (several Case 19 screens have none in their header) is left untouched -- adding
  // one would be new page content, not wiring up a click on something that already exists.
  function wireAccountMenu(header) {
    var avatar = header.querySelector("span.rounded-full.bg-slate-200");
    if (!avatar) return;
    avatar.replaceWith(document.createRange().createContextualFragment(accountHTML()));
  }

  // Replaces the generic product name with the active Workspace's own name, wherever the header
  // still says it -- a Workspace-scoped screen should say which Workspace, not just the product.
  function renameWorkspaceBrand(root, workspaceName) {
    var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
    var node;
    while ((node = walker.nextNode())) {
      if (node.data.indexOf("Menata Runtime") !== -1) {
        node.data = node.data.replace("Menata Runtime", workspaceName);
      }
    }
  }

  function mount(appId, activeId) {
    var data = window.MenataNav;
    var app = appId ? data.apps[appId] : null;
    var header = document.querySelector("header");
    if (!header) return;

    injectFocusVisibleStyle();
    renameWorkspaceBrand(header, data.workspace);

    // The launcher sits directly beside the breadcrumb/title text, not spread across the row by
    // the row's own justify-between -- both are wrapped in one flex item with a small gap, so
    // justify-between still only splits two things apart: this left-hand brand group, and
    // whatever the page's own header already has after it (tabs, avatar, ...).
    var row = header.firstElementChild;
    if (row && row.firstElementChild) {
      var crumb = row.firstElementChild;
      var brandGroup = document.createElement("div");
      brandGroup.className = "flex items-center gap-2";
      row.insertBefore(brandGroup, crumb);
      brandGroup.appendChild(document.createRange().createContextualFragment(launcherHTML(appId)));
      brandGroup.appendChild(crumb);
    }

    wireAccountMenu(header);

    // showNav: false (nav-metadata.js, per-Application) means this Application declares no menu
    // of its own -- neither the desktop top-bar row nor the mobile bottom bar is rendered, only
    // the (always-uniform, never metadata-driven) launcher inserted above. Default true when the
    // field is absent, so every Application that doesn't opt out keeps both bars unchanged.
    if (app && app.showNav !== false) {
      // Match this header's own max-w-* container so the new menu row lines up with the row
      // above it instead of spanning full width while the header's own content stays centered.
      var maxWMatch = row && row.className.match(/max-w-\[[^\]]+\]|max-w-\S+/);
      var maxWClass = maxWMatch ? maxWMatch[0] : "max-w-6xl";

      var appNav = document.createElement("nav");
      appNav.id = "menataAppNav";
      appNav.className = "hidden border-t border-slate-200 px-4 py-2 text-sm sm:block sm:px-6";
      appNav.innerHTML = '<div class="mx-auto flex flex-wrap items-center gap-1 ' + maxWClass + '">' + appNavHTML(app, activeId) + '</div>';
      header.appendChild(appNav);

      var bottom = document.createElement("nav");
      bottom.id = "menataBottomBar";
      bottom.className = "sticky bottom-0 z-30 grid border-t border-slate-200 bg-white sm:hidden";
      bottom.style.gridTemplateColumns = "repeat(4, 1fr)";
      bottom.innerHTML = bottomNavHTML(app, activeId);
      document.body.appendChild(bottom);
    }

    wireMenus();
  }

  window.MenataNav.mount = mount;
})();
