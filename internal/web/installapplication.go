package web

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/installer"
)

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
