package web

import (
	"log"
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// currentActor resolves who is acting on this request: the session identity, the Workspace Groups
// it belongs to (CAP-F24, Fase 6c-1), and its effective Application roles (CAP-P01, Fase 7).
//
// It replaces the bare `actor, _ := authorization.CurrentUserID(...)` every handler used to write.
// That line is now a bug waiting to happen rather than a shortcut: an identity without its Groups
// is a domain.Actor that fails every Group-gated Permission, so a Group-held Approval Step would
// simply show no Approve button and refuse the POST, with nothing anywhere reporting why. Bundling
// the two into one value (see domain.Actor) is what makes "I forgot the groups" impossible to
// write; this function is what makes it cheap not to.
//
// It IS resolved in a middleware and carried on ctx, as of 2026-09-22 -- resolveIdentity. This
// comment used to argue the opposite ("a middleware would charge every /api/* call and every HTMX
// partial for something only permission-checking pages need"), and measuring the real router
// showed the argument had it backwards: sixteen handlers call this function, requireApplicationAccess
// called it as well and discarded the result, and resolveChrome/viewerWorkspaceContext were
// separately reading the same membership. Nothing was being saved by resolving late; the same rows
// were simply being read several times. What the middleware charges every request is *one*
// membership read, which is fewer than the routes it was supposed to be sparing.
//
// The fallback below is for a handler mounted without that middleware -- which in this repo means
// a test mounting one handler on a bare chi router, the shape most of internal/web's own tests use.
//
// Roles and Groups come from one read on purpose. They are answered by the same two tables --
// a Group both gates a CAP-F24 approver_group and grants a CAP-P01 role -- so resolving them
// separately would mean asking the same join twice per request, and worse, would make it possible
// to thread one without the other. That is the failure domain.Actor's own doc comment describes:
// a half-resolved Actor looks forbidden rather than erroring.
//
// Degrades rather than fails, deliberately -- but only in one direction. A membership lookup that
// errors is logged and the Actor comes back with no groups and no roles, so the page still renders
// and the person sees fewer actions than they should; it can never produce *more* access than they
// have. That is the same fail-closed posture domain.Actor's own doc comment describes for a nil
// map, now covering both maps.
func currentActor(req *http.Request, store *data.Store, cfg config.Config) domain.Actor {
	id, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	if id == "" {
		return domain.Actor{}
	}
	ctx := req.Context()
	if resolved, ok := identityFrom(ctx); ok {
		return resolved.Actor(ctx)
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	groups, direct, workspaceRole, err := store.ActorMembership(ctx, workspaceID, id)
	if err != nil {
		log.Printf("resolve membership for actor %s: %v", id, err)
		return domain.Actor{ID: id}
	}
	ids := make(map[string]bool, len(groups))
	for _, g := range groups {
		ids[g.ID] = true
	}
	// EffectiveRoles, not a second merge written here: CAP-O07's union of direct and
	// group-granted roles has exactly one home (data.EffectiveRoles), and it is already unit
	// tested against the cases that rule and board 05 name.
	return domain.Actor{ID: id, Groups: ids, Roles: data.EffectiveRoles(direct, groups), WorkspaceRole: workspaceRole}
}
