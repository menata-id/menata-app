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
      '<a href="workspace-home.html" class="mt-1 block rounded-md p-2 text-xs font-medium text-blue-600 hover:bg-slate-50">All applications →</a>'
    );
  }

  function launcherHTML(currentAppId) {
    return (
      '<span class="menata-launcher relative inline-flex shrink-0 items-center align-middle">' +
        '<button type="button" class="menata-launcher-btn flex h-8 w-8 items-center justify-center rounded-md text-slate-500 hover:bg-slate-100" aria-label="Switch application">' +
          '<svg width="16" height="16" viewBox="0 0 18 18" fill="currentColor"><circle cx="3" cy="3" r="1.6"/><circle cx="9" cy="3" r="1.6"/><circle cx="15" cy="3" r="1.6"/><circle cx="3" cy="9" r="1.6"/><circle cx="9" cy="9" r="1.6"/><circle cx="15" cy="9" r="1.6"/><circle cx="3" cy="15" r="1.6"/><circle cx="9" cy="15" r="1.6"/><circle cx="15" cy="15" r="1.6"/></svg>' +
        '</button>' +
        '<div class="menata-launcher-panel absolute left-0 top-9 z-40 hidden w-72 rounded-lg border border-slate-200 bg-white p-2 text-left shadow-lg">' + launcherPanelHTML(currentAppId) + '</div>' +
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

  function wireLaunchers() {
    document.querySelectorAll(".menata-launcher-btn").forEach(function (btn) {
      btn.addEventListener("click", function (e) {
        e.stopPropagation();
        var panel = btn.parentElement.querySelector(".menata-launcher-panel");
        document.querySelectorAll(".menata-launcher-panel").forEach(function (p) { if (p !== panel) p.classList.add("hidden"); });
        panel.classList.toggle("hidden");
      });
    });
    document.addEventListener("click", function () {
      document.querySelectorAll(".menata-launcher-panel").forEach(function (p) { p.classList.add("hidden"); });
    });
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

    var row = header.firstElementChild;
    if (row) {
      row.insertBefore(document.createRange().createContextualFragment(launcherHTML(appId)), row.firstChild);
    }

    if (app) {
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

    wireLaunchers();
  }

  window.MenataNav.mount = mount;
})();
