package web

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
)

// requireAuth gates every route in its group behind a valid session cookie
// (internal/authorization, ROADMAP.md Phase 2). An HTMX/API request gets a plain 401 so the
// client can react; a full-page navigation is redirected to /login.
//
// Beyond the cookie's own signature, this also checks its generation against
// data.Store.CurrentSessionGeneration (security audit 2026-09-19, M2): logout and a successful
// password reset each bump the affected subject's generation, so a cookie issued before that bump
// is rejected here even though its HMAC still verifies -- this is the one enforcement point for
// that revocation, the same way this is already the one place Workspace scope gets resolved. A
// stale generation is treated identically to "not authenticated" (redirect/401), not a separate
// error, so it can't be used to distinguish "this cookie used to be valid" from "never was".
//
// Once a session names a real identity, this is also the one place a request's Workspace is
// resolved and put on ctx (ROADMAP.md Phase 21 Step 4) -- 007 §20's own ordering: scope is
// established before any retrieval, not trimmed after. defaultWorkspaceID is the fallback for a
// session whose subject is not a real mch_user record id at all -- exactly the shared admin
// credential's placeholder subject (config.AdminUserID) before it is bootstrapped to a real one
// (Phase 7) -- so that path keeps working exactly as it did before this Step existed.
func requireAuth(store *data.Store, defaultWorkspaceID string, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			unauthorized := func() {
				if req.Header.Get("HX-Request") == "true" || strings.HasPrefix(req.URL.Path, "/api/") {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				http.Redirect(w, req, "/login", http.StatusSeeOther)
			}

			userID, cookieGeneration, ok := authorization.CurrentSession(req, cfg.SessionSecret)
			if !ok {
				unauthorized()
				return
			}
			currentGeneration, err := store.CurrentSessionGeneration(req.Context(), userID)
			if err != nil {
				serverError(w, err)
				return
			}
			if cookieGeneration != currentGeneration {
				unauthorized()
				return
			}

			workspaceID, err := store.ResolveUserWorkspace(req.Context(), userID)
			if err != nil {
				if !errors.Is(err, data.ErrRecordNotFound) {
					serverError(w, err)
					return
				}
				workspaceID = defaultWorkspaceID
			}
			ctx := data.WithWorkspaceScope(req.Context(), workspaceID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

// requireWorkspaceAdmin gates Workspace Members (ROADMAP.md Phase 21 Step 6) behind a real
// "admin" WorkspaceRole. The shared admin credential's placeholder identity has no membership row
// at all (it predates real Workspace membership entirely) -- rather than locking it out of a
// screen it could always reach before this phase existed, an absent membership is let through,
// the same fallback posture requireAuth already takes for that same identity.
func requireWorkspaceAdmin(store *data.Store, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
			workspaceID, _ := data.WorkspaceScope(ctx)

			membership, err := store.GetMembership(ctx, workspaceID, userID)
			if err != nil {
				if errors.Is(err, data.ErrRecordNotFound) {
					next.ServeHTTP(w, req)
					return
				}
				serverError(w, err)
				return
			}
			if membership.WorkspaceRole != "admin" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// queryDiagnostics reports what each request actually read: how many queries it issued, how many
// of those repeated a target it had already fetched, and the per-target breakdown (ROADMAP.md
// Phase 18 Step 3). Phase 6 needed a throwaway probe inside internal/data to learn this; making
// it permanent is what lets the next forcing condition show up as a number during development
// rather than as a surprise in production -- see the Method's 2026-09-18 correction for why this
// repo can no longer wait for real use to reveal its thresholds.
//
// It logs rather than setting a response header because the count is only final once the handler
// has rendered, by which point the headers are already on the wire.
func queryDiagnostics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, reads := data.WithReadLog(req.Context())
		next.ServeHTTP(w, req.WithContext(ctx))

		if reads.Total() == 0 {
			return
		}
		parts := make([]string, 0, 4)
		for _, tc := range reads.Breakdown() {
			if tc.Reads > 1 {
				parts = append(parts, fmt.Sprintf("%s x%d", tc.Target, tc.Reads))
				continue
			}
			parts = append(parts, tc.Target)
		}
		log.Printf("reads=%d repeated=%d %s [%s]", reads.Total(), reads.Repeated(), req.URL.Path, strings.Join(parts, ", "))
	})
}
