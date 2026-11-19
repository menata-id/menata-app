package aiassist

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// roundTripFunc lets a test answer the Gemini client's request itself, so no call leaves the machine.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func answering(status int, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// TestProviderFailuresAreToldApart holds what the person at the screen is told for each way the provider can fail.
// The failure that forced it (2026-10-25): credits depleted, answered 402, and shown as "try sending your message
// again" -- five times for one request. Real Gemini clients, answered by a fake transport, so the whole path from the
// provider's status to the words is exercised and nothing is called.
// Mutation-proved 2026-11-19 (see development-history.md).
func TestProviderFailuresAreToldApart(t *testing.T) {
	cases := []struct {
		name   string
		client GeminiClient
		fault  Fault
		status int
		retry  bool // the words invite sending the message again
	}{
		{"credits depleted", GeminiClient{APIKey: "k", HTTPClient: answering(402, `{"error":{"code":402}}`)}, FaultBilling, 402, false},
		{"key refused", GeminiClient{APIKey: "k", HTTPClient: answering(403, `{}`)}, FaultAccess, 403, false},
		{"key missing", GeminiClient{APIKey: "k", HTTPClient: answering(401, `{}`)}, FaultAccess, 401, false},
		{"over a limit", GeminiClient{APIKey: "k", HTTPClient: answering(429, `{}`)}, FaultQuota, 429, true},
		{"provider server error", GeminiClient{APIKey: "k", HTTPClient: answering(503, `{}`)}, FaultTimeout, 503, true},
		{"a status not named", GeminiClient{APIKey: "k", HTTPClient: answering(400, `{}`)}, FaultOther, 400, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.client.Generate(context.Background(), "p", nil)
			if err == nil {
				t.Fatal("no error")
			}
			if got := FaultOf(err); got != c.fault {
				t.Errorf("fault = %q, want %q (%v)", got, c.fault, err)
			}
			if got := FaultStatus(err); got != c.status {
				t.Errorf("status = %d, want %d", got, c.status)
			}
			words := FaultMessage(FaultOf(err))
			if invites := strings.Contains(words, "send"); invites != c.retry && !strings.Contains(words, "will not help") {
				t.Errorf("words %q: invite a retry = %v, want %v", words, invites, c.retry)
			}
			if !c.retry && !strings.Contains(words, "will not help") {
				t.Errorf("a cause a retry cannot cure must say so: %q", words)
			}
			if c.retry && strings.Contains(words, "will not help") {
				t.Errorf("a retry-able cause must not say a retry is useless: %q", words)
			}
			// The error's own text is what the Errorf it replaced said, so logs read the same.
			if !strings.HasPrefix(err.Error(), "gemini returned ") {
				t.Errorf("error text changed: %q", err.Error())
			}
		})
	}
}

func TestProviderTimeoutsAndOddErrorsAreClassified(t *testing.T) {
	deadline := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}
	if _, err := (GeminiClient{APIKey: "k", HTTPClient: deadline}).Generate(context.Background(), "p", nil); FaultOf(err) != FaultTimeout || FaultStatus(err) != 0 {
		t.Errorf("deadline: fault %q status %d (%v)", FaultOf(err), FaultStatus(err), err)
	}
	net := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, timeoutErr{} }), Timeout: time.Second}
	if _, err := (GeminiClient{APIKey: "k", HTTPClient: net}).Generate(context.Background(), "p", nil); FaultOf(err) != FaultTimeout {
		t.Errorf("network timeout: fault %q (%v)", FaultOf(err), err)
	}
	if got := FaultOf(errors.New("gemini returned no candidates")); got != FaultOther {
		t.Errorf("an unusable answer: %q", got)
	}
	// An unknown fault still gets words, and they are the retry sentence.
	if FaultMessage(Fault("nonsense")) != FaultMessage(FaultTimeout) {
		t.Error("an unknown fault must read as the retry sentence")
	}
}
