package web

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// loginRateLimiter is a simple in-memory sliding-window limiter, keyed by client address +
// attempted email -- real per-user passwords (development-history.md Phase 21) make POST /login worth
// defending against repeated guessing, which the constant-time comparison in
// internal/authorization only ever protected against timing attacks, not volume. No external
// dependency (Redis etc.), matching 007 §4.10's single-binary posture -- a single-process
// in-memory map is correct at this app's current scale.
//
// The client address is read through trustedProxies (clientAddr below), so a reverse proxy in front of
// this app does not collapse every visitor into one budget (capability-lifecycle.md §3b, K03).
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

// trustedProxies is the set of peer addresses allowed to tell this app who the real client is, through
// X-Forwarded-For. Everything else is believed about itself only: a header from an address outside the set
// is a client's own claim, and honouring it would let anyone mint a fresh rate-limit budget per request.
type trustedProxies []netip.Prefix

// parseTrustedProxies reads CIDRs or bare addresses (config.TrustedProxies). An unparseable entry is
// returned as an error rather than skipped: a typo that silently trusts nothing leaves the limiter global
// again, and one that trusts too much is a bypass.
func parseTrustedProxies(entries []string) (trustedProxies, error) {
	var out trustedProxies
	for _, e := range entries {
		if p, err := netip.ParsePrefix(e); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(e)
		if err != nil {
			return nil, &net.ParseError{Type: "trusted proxy address or CIDR", Text: e}
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func (t trustedProxies) contains(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range t {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// clientAddr names the client a request came from, for rate limiting. The TCP peer is the answer unless
// the peer is a trusted proxy; then X-Forwarded-For is read from its right end, skipping hops that are
// themselves trusted, and the first address outside the set is the client. Reading from the right is the
// point: a proxy appends the address it saw, so the leftmost entries are whatever the client chose to send.
// Falls back to the peer when no usable entry exists, and to the raw RemoteAddr when it is malformed --
// more permissive than failing closed, but this is a rate limiter, not an authorization check.
func clientAddr(req *http.Request, trusted trustedProxies) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !trusted.contains(peer) {
		return host
	}
	hops := strings.Split(strings.Join(req.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return host // a malformed hop means the chain is not one a proxy wrote; do not guess past it
		}
		if !trusted.contains(hop) {
			return hop.Unmap().String()
		}
	}
	return host
}

// rateLimitLogin wraps submitLogin, rejecting an attempt once its own (address, email) key has
// been tried too many times within the window -- a wrong-password guess still counts as an
// attempt, since the point is to slow down guessing, not just successful breaches.
func rateLimitLogin(limiter *loginRateLimiter, trusted trustedProxies, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		key := clientAddr(req, trusted) + "|" + normalizeEmail(req.FormValue("username"))
		if !limiter.Allow(key) {
			http.Error(w, "too many login attempts -- try again later", http.StatusTooManyRequests)
			return
		}
		next(w, req)
	}
}

// rateLimitByAddress wraps a handler, keyed by client address alone -- unlike login, abuse of
// registration or forgot-password looks like "many attempts from one address" (each with its own,
// different email) rather than "one email guessed repeatedly," so there is no attempted-email
// worth keying on before the form is even parsed.
func rateLimitByAddress(limiter *loginRateLimiter, trusted trustedProxies, tooManyMsg string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !limiter.Allow(clientAddr(req, trusted)) {
			http.Error(w, tooManyMsg, http.StatusTooManyRequests)
			return
		}
		next(w, req)
	}
}
