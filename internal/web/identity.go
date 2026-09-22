package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// requestIdentity is everything about *who is asking* that a request resolves once and then
// reads, rather than re-deriving per consumer.
//
// WHY THIS TYPE EXISTS. Identity was being re-read two to four times per request by pieces that
// each looked cheap on their own: currentWorkspace fetched the Workspace row and resolveChrome
// fetched it again; requireApplicationAccess resolved the Actor and discarded it, so each of the
// sixteen handlers calling currentActor paid for it again; resolveChrome and
// viewerWorkspaceContext each called GetMembership, which is not one query but four (the
// membership row, its per-Application roles, and GroupsByMember's two). Measured on 2026-09-22
// through the real router, /home issued **seventeen** queries, eight of them a second
// GetMembership. The diagnostic that should have shown this was itself registered below the
// middleware, so it reported three to seven reads and `repeated=0`.
//
// The shape follows the precedent data.Store's own doc comment records for Workspace scope: a
// per-request fact belongs on ctx, resolved at the one chokepoint every request already passes,
// not in a struct field and not re-derived at each of ~20 handler entry points.
//
// One Membership is the whole of it, and that is the point rather than an economy: domain.Actor's
// four parts (id, group ids, effective roles, Workspace role) are all derivable from a Membership
// that already carries AppRoles and Groups, so resolving the Actor separately through
// ActorMembership was asking the database a question it had just answered.
// EAGER vs LAZY, and why it is split. The Workspace row is read up front because
// currentWorkspace needs it on literally every request. The membership is not, because a great
// many requests never ask who the viewer is beyond their id -- the nav badge endpoint being the
// clearest case, one integer on a third of all requests.
//
// That split is a correction to this change's own first version, which resolved everything
// eagerly and made the badge cost ten queries to render one number. The doc comment on
// resolveChrome used to warn about exactly this ("a middleware would charge every /api/* call and
// every HTMX partial"); it was wrong that the values should therefore be resolved per consumer,
// and right that resolving them for consumers who never look is waste. Lazy-and-memoized is what
// satisfies both halves.
type requestIdentity struct {
	store       *data.Store
	workspaceID string
	userID      string

	// workspace is the Workspace row this request is scoped to; nil if it could not be read.
	// Resolved eagerly, since currentWorkspace asks for it on every request.
	workspace *data.Workspace

	// once guards the membership-derived fields below, which are resolved on first use. A
	// sync.Once rather than a plain flag because a handler is free to fan out across goroutines,
	// and a request-scoped cache that races is a bug the -race build should never have to find.
	once sync.Once
	// membership is this identity's row in that Workspace, or nil for an identity that has none --
	// the shared admin credential's placeholder subject being the one that legitimately does
	// (requireWorkspaceAdmin's own doc comment). Nil is a normal value here, never an error.
	membership *data.Membership
	// actor is the authorization view of the same membership, built by actorFromMembership.
	actor domain.Actor
	// viewerName is what the chrome shows: the identity's full name, falling back to its email,
	// which is at least addressable. Empty when there is no membership to name.
	viewerName string
}

// resolveMembership reads this identity's membership, Actor and display name -- once per request,
// on the first consumer that asks for any of them.
func (i *requestIdentity) resolveMembership(ctx context.Context) {
	i.once.Do(func() {
		if i.userID == "" {
			return
		}
		if m, err := i.store.GetMembership(ctx, i.workspaceID, i.userID); err == nil {
			i.membership = m
		} else if !errors.Is(err, data.ErrRecordNotFound) {
			// Worth a log line and nothing more: see resolveIdentity's own doc comment for why an
			// unreadable membership degrades rather than fails.
			log.Printf("resolve membership for %s: %v", i.userID, err)
		}
		i.actor = actorFromMembership(i.userID, i.membership)
		i.viewerName = viewerNameFor(ctx, i.store, i.membership)
	})
}

// Membership, Actor and ViewerName are the three views of one resolution.
func (i *requestIdentity) Membership(ctx context.Context) *data.Membership {
	i.resolveMembership(ctx)
	return i.membership
}

func (i *requestIdentity) Actor(ctx context.Context) domain.Actor {
	i.resolveMembership(ctx)
	return i.actor
}

func (i *requestIdentity) ViewerName(ctx context.Context) string {
	i.resolveMembership(ctx)
	return i.viewerName
}

type identityKey struct{}

// resolveIdentity resolves this request's Workspace, membership, Actor and viewer name once, and
// puts them on ctx for everything downstream.
//
// It is registered immediately after requireAuth, which is what establishes the Workspace scope it
// reads, and before currentWorkspace/currentApplication/requireApplicationAccess, all three of
// which now consume it rather than querying.
//
// A failure to read any part is deliberately *not* fatal. Every consumer already degraded
// gracefully on a missing membership -- resolveChrome rendered an unnamed viewer,
// viewerWorkspaceContext returned empty strings, requireWorkspaceAdmin has its own explicit
// no-row branch -- and turning those into a 500 here would be a behavioural change smuggled in
// under a query-count change. What lands on ctx is what could be read; consumers keep their
// existing fallbacks.
func resolveIdentity(store *data.Store, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			workspaceID, _ := data.WorkspaceScope(ctx)
			userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

			id := &requestIdentity{store: store, workspaceID: workspaceID, userID: userID}
			if ws, err := store.GetWorkspace(ctx, workspaceID); err == nil {
				id.workspace = ws
			}

			next.ServeHTTP(w, req.WithContext(context.WithValue(ctx, identityKey{}, id)))
		})
	}
}

// identityFrom returns the identity resolved for this request, if resolveIdentity ran.
//
// It reports absence rather than resolving on demand, because the two callers that must cope with
// absence want different fallbacks: a handler mounted directly in a test has no middleware at all
// and re-queries, while a page's chrome renders an anonymous viewer. Hiding that behind a lazy
// resolve would make an un-middlewared request silently cost what this change exists to remove.
func identityFrom(ctx context.Context) (*requestIdentity, bool) {
	id, ok := ctx.Value(identityKey{}).(*requestIdentity)
	return id, ok && id != nil
}

// membershipFor returns the viewer's membership from the identity when one was resolved, and by
// querying when none was. A nil membership with a nil error means "no row", which every caller
// already treats as a normal outcome rather than a failure.
func membershipFor(ctx context.Context, store *data.Store, workspaceID, userID string) (*data.Membership, error) {
	if id, ok := identityFrom(ctx); ok {
		return id.Membership(ctx), nil
	}
	return store.GetMembership(ctx, workspaceID, userID)
}

// currentWorkspaceRow returns this request's Workspace row, from the identity when one was
// resolved and by querying when none was.
func currentWorkspaceRow(ctx context.Context, store *data.Store) (*data.Workspace, bool) {
	if id, ok := identityFrom(ctx); ok {
		return id.workspace, id.workspace != nil
	}
	workspaceID, ok := data.WorkspaceScope(ctx)
	if !ok {
		return nil, false
	}
	row, err := store.GetWorkspace(ctx, workspaceID)
	return row, err == nil && row != nil
}

// actorFromMembership builds the authorization view of one membership.
//
// EffectiveRoles, not a second merge written here: CAP-O07's union of direct and group-granted
// roles has exactly one home (data.EffectiveRoles), and it is already unit tested against the
// cases that rule and board 05 name. A nil membership yields an Actor with an id and nothing
// else, which satisfies no Permission asking for a role -- the fail-closed direction, and the
// same value currentActor produced for an unreadable membership before this existed.
func actorFromMembership(userID string, m *data.Membership) domain.Actor {
	if m == nil {
		return domain.Actor{ID: userID}
	}
	ids := make(map[string]bool, len(m.Groups))
	for _, g := range m.Groups {
		ids[g.ID] = true
	}
	return domain.Actor{
		ID:            userID,
		Groups:        ids,
		Roles:         data.EffectiveRoles(m.AppRoles, m.Groups),
		WorkspaceRole: m.WorkspaceRole,
	}
}

// viewerNameFor resolves the display name behind a membership: the identity's full name, else the
// email it logs in with.
//
// The name comes from the credential rather than from the mch_user record because both name and
// email stopped being Fields on 2026-09-22 (metadata/user.yaml, migration 010) -- they belong to
// whoever owns the login, not to one Workspace's record of them.
func viewerNameFor(ctx context.Context, store *data.Store, m *data.Membership) string {
	if m == nil || m.Email == "" {
		return ""
	}
	if cred, err := store.GetCredential(ctx, m.Email); err == nil && cred.FullName != "" {
		return cred.FullName
	}
	return m.Email
}
