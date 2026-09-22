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

// requireWorkspaceAdmin gates Workspace administration (Members, Groups, the Approval Role
// Matrix) behind a real "admin" WorkspaceRole.
//
// **It used to fail open**, and that was found by the 2026-09-21 authorization review rather than
// by a case: an absent membership row was let through, so the gate passed whenever its own lookup
// found nothing. The intent was narrow and legitimate -- the shared admin credential's
// placeholder identity predates Workspace membership entirely and has no row -- but the
// implementation said "anyone this lookup cannot find", which is a different and much larger set,
// and it is the shape of an authorization gate that admits whatever it fails to identify.
//
// It is now the intent, written literally: that one configured identity, by id, and nobody else.
// Any other identity with no membership row is refused, as it always should have been.
func requireWorkspaceAdmin(store *data.Store, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
			workspaceID, _ := data.WorkspaceScope(ctx)

			membership, err := store.GetMembership(ctx, workspaceID, userID)
			if err != nil {
				if errors.Is(err, data.ErrRecordNotFound) {
					if userID != "" && userID == cfg.AdminUserID {
						next.ServeHTTP(w, req)
						return
					}
					http.Error(w, "forbidden", http.StatusForbidden)
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

// queryDiagnostics reports what each request actually read: how many statements it issued, how
// many of those repeated a target it had already fetched, and the per-target breakdown
// (ROADMAP.md Phase 18 Step 3). Phase 6 needed a throwaway probe inside internal/data to learn
// this; making it permanent is what lets the next forcing condition show up as a number during
// development rather than as a surprise in production -- see the Method's 2026-09-18 correction
// for why this repo can no longer wait for real use to reveal its thresholds.
//
// It logs rather than setting a response header because the count is only final once the handler
// has rendered, by which point the headers are already on the wire.
//
// TWO COUNTS, AND WHY BOTH ARE PRINTED. `queries` is data.QueryTracer's: every statement the pool
// issued, counted by the driver, which no Store method can forget to increment. `reads` is the
// named half, recorded by the Store methods themselves, and it is what the breakdown lists. They
// should agree. **When they don't, the difference is the point** -- it is a statement issued by
// something that did not name itself, which is precisely the state that made this diagnostic
// under-report every authenticated request before 2026-09-22: it was registered below four
// middlewares and counted none of them. Reconciling the two numbers into one would restore
// exactly the blind spot the second number exists to expose, so the gap is printed instead.
func queryDiagnostics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, reads := data.WithReadLog(req.Context())
		next.ServeHTTP(w, req.WithContext(ctx))

		if reads.Queries() == 0 {
			return
		}
		parts := make([]string, 0, 8)
		for _, tc := range reads.Breakdown() {
			if tc.Reads > 1 {
				parts = append(parts, fmt.Sprintf("%s x%d", tc.Target, tc.Reads))
				continue
			}
			parts = append(parts, tc.Target)
		}
		// unnamed is the gap described above, printed only when there is one so a healthy line
		// stays as short as it was.
		unnamed := ""
		if gap := reads.Queries() - reads.Total(); gap > 0 {
			unnamed = fmt.Sprintf(" unnamed=%d", gap)
		}
		log.Printf("queries=%d reads=%d repeated=%d%s %s [%s]",
			reads.Queries(), reads.Total(), reads.Repeated(), unnamed, req.URL.Path, strings.Join(parts, ", "))
	})
}
