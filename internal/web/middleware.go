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

			// membershipFor, not store.GetMembership: resolveIdentity already read this exact row
			// for this request, and GetMembership is four queries, not one. Every admin route
			// was paying for it twice (2026-09-22 sweep: `/workspace-members` at repeated=6,
			// `/workspace-groups` at 5, `/authorization-matrix` at 4 -- all three behind this gate).
			//
			// The four branches below are unchanged, and that is deliberate rather than incidental:
			// this gate **used to fail open** for any identity it could not find (2026-09-21
			// authorization review), so "row absent -> refuse, except the one configured admin
			// identity" is the behaviour, not an implementation detail. Two shape changes to watch:
			// membershipFor reports an absent row as (nil, nil) where GetMembership reports
			// ErrRecordNotFound, so absence is tested on the value; and when the identity was
			// resolved by middleware, a database error has already been logged and degraded to a
			// nil membership there -- which lands on "refuse", the fail-closed direction, instead
			// of on a 500. Losing the 500 is acceptable here precisely because the alternative
			// direction is the one this gate was once wrong in.
			membership, err := membershipFor(ctx, store, workspaceID, userID)
			if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
				serverError(w, err)
				return
			}
			if membership == nil {
				if userID != "" && userID == cfg.AdminUserID {
					next.ServeHTTP(w, req)
					return
				}
				http.Error(w, "forbidden", http.StatusForbidden)
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
		gap := reads.Queries() - reads.Total()
		unnamed := ""
		if gap > 0 {
			unnamed = fmt.Sprintf(" unnamed=%d", gap)
		}
		log.Printf("%squeries=%d reads=%d repeated=%d%s %s [%s]",
			anomalyPrefix(req.Method, reads.Repeated(), gap),
			reads.Queries(), reads.Total(), reads.Repeated(), unnamed, req.URL.Path, strings.Join(parts, ", "))
	})
}

// anomalyMarker is the token a periodic log review greps for.
//
// It exists because the 2026-09-22 review had to read 1853 lines to find what was wrong with
// them. Conformance gates cover the routes someone wrote a fixture for -- two of fifty-four when
// this was added -- so the log is the only thing that observes the rest, and it observes them
// under real traffic rather than under a test's idea of it. Marking the interesting lines is what
// turns the next review from a read into a grep.
const anomalyMarker = "ANOMALY"

// anomalyPrefix marks a request whose own counts say something is wrong with it.
//
// Two conditions, and the asymmetry between them is deliberate:
//
//   - **An unnamed query is always anomalous**, whatever the method. It means a statement was
//     issued by something that did not call record(), so the breakdown beside it is incomplete --
//     precisely the blind spot that made this diagnostic under-report for months.
//
//   - **A repeated read is anomalous only on a safe method.** A GET that reads the same target
//     twice is waste: composition.Loader memoizes within a request and the identity is resolved
//     once on ctx, so there is nothing left that legitimately re-reads. A write is different --
//     composition.Loader's own doc comment forbids one memo from spanning a mutation, so a POST
//     re-reading after its write is the correct behaviour, not a defect. Flagging it would train
//     the reader to ignore the marker, which is the only way a marker like this fails.
func anomalyPrefix(method string, repeated, unnamed int) string {
	var reasons []string
	if unnamed > 0 {
		reasons = append(reasons, "unnamed")
	}
	if repeated > 0 && (method == http.MethodGet || method == http.MethodHead) {
		reasons = append(reasons, "repeated")
	}
	if len(reasons) == 0 {
		return ""
	}
	return anomalyMarker + "(" + strings.Join(reasons, ",") + ") "
}
