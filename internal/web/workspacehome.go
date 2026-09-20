package web

import (
	"errors"
	"net/http"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showWorkspaceHome is the landing page after login (ROADMAP.md Phase 21 Step 5): the signed-in
// identity's Workspace, one card per Application declared in it, and a "Your access" summary. A
// membership row is expected to be missing for the shared admin credential's placeholder identity
// (it predates real Workspace membership entirely); that degrades to a blank role rather than an
// error.
//
// One card per Application since Fase 3 (2026-09-20), where it used to be exactly one: with
// `applications:` a real list, rendering a single card would be showing stale data, not a
// deferred port. What is still deferred to Fase 3c is the *board's* card face (icon, description,
// record count) and the per-Application role rows -- see ROADMAP.md's deferral table.
func showWorkspaceHome(machines map[string]*domain.Machine, store *data.Store, ws domain.Workspace, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		workspaceID, _ := data.WorkspaceScope(ctx)

		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		membership, err := store.GetMembership(ctx, workspaceID, userID)
		if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
			serverError(w, err)
			return
		}
		if membership == nil {
			membership = &data.Membership{}
		}

		inbox, err := composition.ApprovalInbox(ctx, composition.NewLoader(store, machines), userID, time.Now(), machines[action.StepMachineID])
		if err != nil {
			serverError(w, err)
			return
		}

		switchHref := ""
		if membership.Email != "" {
			if choices, err := loadWorkspaceChoices(ctx, store, membership.Email); err == nil && len(choices) > 1 {
				switchHref = "/switch-workspace"
			}
		}

		render(ctx, w, rendering.WorkspaceHomePage(
			chrome.WorkspaceName, membership.WorkspaceRole, chrome.UserInitials, switchHref,
			applicationCards(ws, membership, len(inbox.Pending)),
		))
	}
}

// applicationCards builds one card per Application in the Workspace.
//
// Each card's link is that Application's own declared HomeRoute (its `home_card: true` navigation
// item), never a literal here or in workspacehome.templ -- the route/label conformance gates hold
// both to that. An Application declaring no home_card falls back to "/home", which is always a
// valid destination, rather than to a guessed Application route.
//
// PendingCount is shown only on the Application that actually declares the pending-approval badge
// (domain.NavBadgeApprovalInboxPending). That is what keeps the count honest with several
// Applications: it is Document Approval's number, and putting it on a Project Management card
// would be inventing a meaning for it. An Application declaring no badge simply shows no count.
//
// Role comes from that Application's own entry in membership.AppRoles (Fase 3b), so each card
// shows the role held *there* rather than repeating one workspace-wide value. An Application the
// member holds no role in shows "—".
func applicationCards(ws domain.Workspace, membership *data.Membership, pending int) []rendering.ApplicationCard {
	cards := make([]rendering.ApplicationCard, 0, len(ws.Applications))
	for _, app := range ws.Applications {
		route := app.HomeRoute
		if route == "" {
			route = "/home"
		}
		card := rendering.ApplicationCard{
			Name:      app.Name,
			Initials:  composition.Initials(app.Name),
			Role:      membership.AppRoles[app.ID],
			HomeRoute: route,
		}
		for _, item := range app.AllNavigation {
			if item.Badge == domain.NavBadgeApprovalInboxPending {
				card.PendingCount = pending
				card.ShowPending = true
				break
			}
		}
		cards = append(cards, card)
	}
	return cards
}
