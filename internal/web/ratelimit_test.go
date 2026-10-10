package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoginRateLimiter_allowsUpToLimit(t *testing.T) {
	l := newLoginRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("Allow() = false on attempt %d, want true (within limit)", i+1)
		}
	}
	if l.Allow("k") {
		t.Error("Allow() = true on attempt 4, want false (over limit)")
	}
}

func TestLoginRateLimiter_keysAreIndependent(t *testing.T) {
	l := newLoginRateLimiter(1, time.Minute)
	if !l.Allow("a") {
		t.Fatal("Allow(a) first attempt = false, want true")
	}
	if !l.Allow("b") {
		t.Error("Allow(b) first attempt = false, want true -- a different key must not share a's budget")
	}
	if l.Allow("a") {
		t.Error("Allow(a) second attempt = true, want false")
	}
}

func TestLoginRateLimiter_windowExpires(t *testing.T) {
	l := newLoginRateLimiter(1, 10*time.Millisecond)
	if !l.Allow("k") {
		t.Fatal("Allow() first attempt = false, want true")
	}
	if l.Allow("k") {
		t.Fatal("Allow() second attempt (before window expiry) = true, want false")
	}
	time.Sleep(20 * time.Millisecond)
	if !l.Allow("k") {
		t.Error("Allow() after window expiry = false, want true")
	}
}

func TestRateLimitLogin_blocksAfterLimit(t *testing.T) {
	limiter := newLoginRateLimiter(2, time.Minute)
	called := 0
	handler := rateLimitLogin(limiter, nil, func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=a@example.com&password=x"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "1.2.3.4:5555"
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want 200", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=a@example.com&password=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("3rd attempt: status = %d, want 429", rec.Code)
	}
	if called != 2 {
		t.Errorf("wrapped handler called %d times, want 2 (the 3rd should have been blocked before reaching it)", called)
	}
}

func TestRateLimitByAddress_blocksByAddressAlone(t *testing.T) {
	limiter := newLoginRateLimiter(1, time.Minute)
	called := 0
	handler := rateLimitByAddress(limiter, nil, "too many attempts", func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	})

	// Two different attempted emails from the same address -- still only one attempt's worth of
	// budget, since this kind of abuse is "many attempts from one address," not "one email
	// guessed repeatedly."
	first := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("fld_email=a@example.com"))
	first.RemoteAddr = "9.9.9.9:1111"
	rec := httptest.NewRecorder()
	handler(rec, first)
	if rec.Code != http.StatusOK {
		t.Fatalf("1st attempt: status = %d, want 200", rec.Code)
	}

	second := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("fld_email=b@example.com"))
	second.RemoteAddr = "9.9.9.9:2222"
	rec = httptest.NewRecorder()
	handler(rec, second)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("2nd attempt (different email, same address): status = %d, want 429", rec.Code)
	}
	if called != 1 {
		t.Errorf("wrapped handler called %d times, want 1", called)
	}
}

func TestClientAddr_readsForwardedForOnlyFromATrustedPeer(t *testing.T) {
	trusted, err := parseTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, remote, xff, want string
	}{
		{"direct client, no header", "1.2.3.4:5555", "", "1.2.3.4"},
		{"untrusted peer cannot choose its address", "1.2.3.4:5555", "8.8.8.8", "1.2.3.4"},
		{"trusted proxy names the client", "127.0.0.1:5555", "203.0.113.7", "203.0.113.7"},
		{"client-forged left entry is ignored", "127.0.0.1:5555", "6.6.6.6, 203.0.113.7", "203.0.113.7"},
		{"trusted hops are skipped", "127.0.0.1:5555", "203.0.113.7, 10.1.1.1", "203.0.113.7"},
		{"no header from a trusted peer falls back to it", "127.0.0.1:5555", "", "127.0.0.1"},
		{"malformed hop falls back to the peer", "127.0.0.1:5555", "203.0.113.7, junk", "127.0.0.1"},
		{"ipv6 client", "[::1]:5555", "2001:db8::1", "2001:db8::1"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = c.remote
		if c.xff != "" {
			req.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := clientAddr(req, trusted); got != c.want {
			t.Errorf("%s: clientAddr = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRateLimitByAddress_twoClientsBehindOneProxyHaveSeparateBudgets(t *testing.T) {
	trusted, _ := parseTrustedProxies([]string{"127.0.0.1"})
	limiter := newLoginRateLimiter(1, time.Minute)
	handler := rateLimitByAddress(limiter, trusted, "too many", func(w http.ResponseWriter, _ *http.Request) {})
	send := func(client string) int {
		req := httptest.NewRequest(http.MethodPost, "/register", nil)
		req.RemoteAddr = "127.0.0.1:4000"
		req.Header.Set("X-Forwarded-For", client)
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec.Code
	}
	if c := send("203.0.113.1"); c != http.StatusOK {
		t.Fatalf("first client: %d", c)
	}
	if c := send("203.0.113.2"); c != http.StatusOK {
		t.Errorf("second client behind the same proxy: %d, want 200 -- the budget is still global", c)
	}
	if c := send("203.0.113.1"); c != http.StatusTooManyRequests {
		t.Errorf("first client again: %d, want 429", c)
	}
}

func TestParseTrustedProxies_refusesATypo(t *testing.T) {
	if _, err := parseTrustedProxies([]string{"127.0.0.1", "10.0.0.0/33"}); err == nil {
		t.Error("a malformed entry was accepted -- it would silently trust the wrong set")
	}
}
