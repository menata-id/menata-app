package web

import (
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
func requireAuth(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !authorization.IsAuthenticated(req, cfg.SessionSecret) {
				if req.Header.Get("HX-Request") == "true" || strings.HasPrefix(req.URL.Path, "/api/") {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				http.Redirect(w, req, "/login", http.StatusSeeOther)
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
