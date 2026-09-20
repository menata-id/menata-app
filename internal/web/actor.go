package web

import (
	"log"
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// currentActor resolves who is acting on this request: the session identity, plus the Workspace
// Groups it belongs to (CAP-F24, Fase 6c-1).
//
// It replaces the bare `actor, _ := authorization.CurrentUserID(...)` every handler used to write.
// That line is now a bug waiting to happen rather than a shortcut: an identity without its Groups
// is a domain.Actor that fails every Group-gated Permission, so a Group-held Approval Step would
// simply show no Approve button and refuse the POST, with nothing anywhere reporting why. Bundling
// the two into one value (see domain.Actor) is what makes "I forgot the groups" impossible to
// write; this function is what makes it cheap not to.
//
// The group read is one indexed query returning ids only (data.GroupIDsForMember) and is skipped
// entirely for an unidentified caller. It is resolved per request rather than in requireAuth and
// carried on ctx, for the same reason resolveChrome gives for not doing that: a middleware would
// charge every /api/* call and every HTMX partial for something only permission-checking pages
// need.
//
// Degrades rather than fails, deliberately -- but only in one direction. A Groups lookup that
// errors is logged and the Actor comes back with no groups, so the page still renders and the
// person sees fewer actions than they should; it can never produce *more* access than they have.
// That is the same fail-closed posture domain.Actor's own doc comment describes for a nil map.
func currentActor(req *http.Request, store *data.Store, cfg config.Config) domain.Actor {
	id, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	if id == "" {
		return domain.Actor{}
	}
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	groups, err := store.GroupIDsForMember(ctx, workspaceID, id)
	if err != nil {
		log.Printf("resolve groups for actor %s: %v", id, err)
		return domain.Actor{ID: id}
	}
	return domain.Actor{ID: id, Groups: groups}
}
