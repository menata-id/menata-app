package web

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// loginRateLimiter is a simple in-memory sliding-window limiter, keyed by client address +
// attempted email -- real per-user passwords (ROADMAP.md Phase 21) make POST /login worth
// defending against repeated guessing, which the constant-time comparison in
// internal/authorization only ever protected against timing attacks, not volume. No external
// dependency (Redis etc.), matching 007 §4.10's single-binary posture -- a single-process
// in-memory map is correct at this app's current scale.
//
// Known limitation, named rather than silently accepted: keying on http.Request.RemoteAddr sees
// a reverse proxy's own address, not the real client's, once this app sits behind one. Reading
// X-Forwarded-For safely (picking the right hop, not trusting a client-supplied header blindly)
// is a real deployment-topology question this app hasn't had to answer yet -- worth revisiting
// the day a reverse proxy is actually in front of it.
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

func newLoginRateLimiter(limit int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{attempts: make(map[string][]time.Time), limit: limit, window: window}
}

// Allow records one attempt for key and reports whether it's within the limit. Attempts outside
// the window are pruned lazily, on that same key's next check, rather than by a background sweep
// -- correct for this app's scale without an extra goroutine to manage.
func (l *loginRateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-l.window)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.attempts[key] = kept
		return false
	}
	l.attempts[key] = append(kept, time.Now())
	return true
}

// clientAddr strips the ephemeral source port from a request's RemoteAddr -- each new TCP
// connection (a new curl process, a browser tab reload) gets its own port, so keying on the raw
// RemoteAddr would give every single request its own unique rate-limit budget, defeating the
// limiter entirely. Falls back to the raw value on the (malformed-input) case SplitHostPort
// itself would error on, which is more permissive than failing closed, but this is a rate limiter
// tightening a soft edge, not an authorization check.
func clientAddr(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}

// rateLimitLogin wraps submitLogin, rejecting an attempt once its own (address, email) key has
// been tried too many times within the window -- a wrong-password guess still counts as an
// attempt, since the point is to slow down guessing, not just successful breaches.
func rateLimitLogin(limiter *loginRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		key := clientAddr(req) + "|" + normalizeEmail(req.FormValue("username"))
		if !limiter.Allow(key) {
			http.Error(w, "too many login attempts -- try again later", http.StatusTooManyRequests)
			return
		}
		next(w, req)
	}
}

// rateLimitRegistration wraps submitRegistration, keyed by client address alone -- unlike login,
// registration abuse looks like "many workspaces created from one address" (each with its own,
// different email) rather than "one email guessed repeatedly," so there is no attempted-email
// worth keying on.
func rateLimitRegistration(limiter *loginRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !limiter.Allow(clientAddr(req)) {
			http.Error(w, "too many registration attempts -- try again later", http.StatusTooManyRequests)
			return
		}
		next(w, req)
	}
}
