package web

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/rendering"
)

// showInstallApplication lists the template library and what installing each one into *this* Workspace
// would do (ROADMAP.md "Installing a template into a Workspace that already uses its ids", 2026-09-28).
//
// Every row's renames, added references and refusals come from installer.PlanInstall -- the same
// function the POST runs, so the screen cannot promise something the write then declines. A screen
// computing its own preview would be a second implementation of the rule, free to disagree with the
// one that actually happens.
func showInstallApplication(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		templates, err := installer.Templates(cfg.TemplatePath)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		_, switchHref := viewerWorkspaceContext(ctx, store, currentActor(req, store, cfg).ID)
		render(ctx, w, rendering.InstallApplicationPage(
			installableRows(rendering.CurrentWorkspace(ctx), templates),
			chrome.WorkspaceName, chrome.Viewer(), switchHref))
	}
}

// installableRows turns each template plus this Workspace into one row the page renders. Pure, so the
// interesting half of this screen is testable without a server.
func installableRows(ws domain.Workspace, templates []installer.Template) []rendering.InstallableTemplate {
	rows := make([]rendering.InstallableTemplate, 0, len(templates))
	for _, t := range templates {
		plan := installer.PlanInstall(t, ws)
		row := rendering.InstallableTemplate{
			ApplicationID: t.Application.ID,
			Name:          t.Application.Name,
			Description:   t.Application.Description,
			Icon:          t.Application.Icon,
			Color:         t.Application.Color,
			Roles:         t.Application.Roles,
			MachineNames:  make([]string, 0, len(t.Machines)),
			AddShared:     plan.AddShared,
			Refusals:      plan.Refusals,
		}
		for _, tm := range t.Machines {
			row.MachineNames = append(row.MachineNames, tm.Machine.Name)
		}
		for _, from := range sortedRenameKeys(plan.Renames) {
			row.Renames = append(row.Renames, rendering.InstallRename{From: from, To: plan.Renames[from]})
		}
		rows = append(rows, row)
	}
	return rows
}

// submitInstallApplication installs one template into this Workspace and reloads the route table live
// -- Deps.ReloadMetadata, injected by cmd/server since this package may not import internal/metadata
// itself (TestPlaneBoundaries), exactly as publishNewApplication already does.
//
// Nothing partial survives a failure: installer.Install rolls its own writes back before returning, and
// a failed reload leaves the previous route table serving traffic while the files on disk are correct
// for the next restart.
func submitInstallApplication(store *data.Store, cfg config.Config, reload func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		manifestPath, ws, err := workspaceInstallation(ctx, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		template, err := installer.TemplateByID(cfg.TemplatePath, req.FormValue("template"))
		if err != nil {
			http.Error(w, "no such template", http.StatusNotFound)
			return
		}

		plan := installer.PlanInstall(template, ws)
		if !plan.OK() {
			// The screen already showed these, so reaching here means the Workspace changed underneath
			// the page -- 422 with the reasons, not a generic failure.
			http.Error(w, "this application cannot be installed here: "+strings.Join(plan.Refusals, "; "), http.StatusUnprocessableEntity)
			return
		}
		appID, err := installer.Install(plan, cfg.TemplatePath, manifestPath)
		if err != nil {
			serverError(w, err)
			return
		}
		if reload == nil {
			serverError(w, fmt.Errorf("%s was installed but this process has no reload hook configured -- a restart will pick it up", appID))
			return
		}
		if err := reload(); err != nil {
			serverError(w, fmt.Errorf("%s was installed but the live reload failed -- a process restart will pick it up: %w", appID, err))
			return
		}
		if err := grantInstallerRole(ctx, req, store, cfg, appID, req.FormValue("role")); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, installedHomeRoute(plan))
	}
}

// grantInstallerRole gives the admin who installed an Application a role in it.
//
// Without this they hold none the moment it exists, and requireApplicationAccess answers 403 on the
// very redirect below -- the identical trap the AI publish path fell into on 2026-09-27, which is why
// that path asks the conversation for a publisher_role. Here the install form asks instead, defaulting
// to the Application's first declared role.
//
// Two deliberate skips: an Application declaring no roles has nothing to grant (nothing gates on one
// there), and the shared admin credential holds no membership row at all -- requireApplicationAccess
// exempts it by id, so granting would both fail and be pointless.
func grantInstallerRole(ctx context.Context, req *http.Request, store *data.Store, cfg config.Config, applicationID, role string) error {
	if role == "" {
		return nil
	}
	actor := currentActor(req, store, cfg)
	if actor.ID == "" || actor.ID == cfg.AdminUserID {
		return nil
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	if err := store.SetMemberAppRole(ctx, workspaceID, actor.ID, applicationID, role); err != nil {
		return fmt.Errorf("%s was installed but granting your own role failed -- add it yourself from its Settings: %w", applicationID, err)
	}
	return nil
}

// installedHomeRoute is where a just-installed Application opens: the home_card route the template
// itself declares.
//
// Read from the plan rather than from the reloaded Workspace on purpose. This handler's own ctx carries
// the Workspace as it was *before* the install, and internal/web may not load metadata itself
// (TestPlaneBoundaries) -- but a route is never renamed on install (a collision there is refused
// outright, see installer.PlanInstall), so the template's own declared route is exactly the installed
// copy's route. Workspace Home is the fallback, which is where the new card is anyway.
func installedHomeRoute(plan installer.Plan) string {
	if route := plan.Template.Application.HomeRoute; route != "" {
		return route
	}
	return "/home"
}

func sortedRenameKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
