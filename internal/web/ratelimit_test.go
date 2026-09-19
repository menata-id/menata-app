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
	handler := rateLimitLogin(limiter, func(w http.ResponseWriter, _ *http.Request) {
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

func TestRateLimitRegistration_blocksByAddressAlone(t *testing.T) {
	limiter := newLoginRateLimiter(1, time.Minute)
	called := 0
	handler := rateLimitRegistration(limiter, func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	})

	// Two different attempted emails from the same address -- still only one attempt's worth of
	// budget, since registration abuse is "many workspaces from one address," not "one email
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
