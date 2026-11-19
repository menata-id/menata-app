package web

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/data"
)

// TestAssistantTurnSaysWhyTheProviderFailed holds the conversation's words and the log line for a failed provider
// call: a 402 is shown as "unavailable, a retry will not help" and logged by name, not shown as a timeout (the
// failure of 2026-10-25, when five sessions got "try sending your message again" for a depleted account).
// Mutation-proved 2026-11-19 (see development-history.md).
func TestAssistantTurnSaysWhyTheProviderFailed(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		inReply string
		notIn   string
		inLog   string
	}{
		{"credits depleted", &aiassist.ProviderError{StatusCode: 402, Status: "402 Payment Required", Body: "{}"}, "no credit left", "try sending your message again", "AI-PROVIDER fault=billing status=402 session=aise_x"},
		{"key refused", &aiassist.ProviderError{StatusCode: 403, Status: "403 Forbidden", Body: "{}"}, "refused the service's key", "try sending your message again", "AI-PROVIDER fault=access status=403 session=aise_x"},
		{"over a limit", &aiassist.ProviderError{StatusCode: 429, Status: "429 Too Many Requests", Body: "{}"}, "usage limit", "will not help", "AI-PROVIDER fault=quota status=429 session=aise_x"},
		{"timeout", context.DeadlineExceeded, "try sending your message again", "will not help", "AI-PROVIDER fault=timeout status=0 session=aise_x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var logged bytes.Buffer
			log.SetOutput(&logged)
			t.Cleanup(func() { log.SetOutput(os.Stderr) })
			reply, err := generateReply(context.Background(), &fakeAIClient{err: c.err}, "prompt", &data.AISession{ID: "aise_x"})
			if err != nil {
				t.Fatalf("a provider failure must become a turn, got %v", err)
			}
			if !strings.Contains(reply.Message, c.inReply) {
				t.Errorf("reply %q lacks %q", reply.Message, c.inReply)
			}
			if strings.Contains(reply.Message, c.notIn) {
				t.Errorf("reply %q must not say %q", reply.Message, c.notIn)
			}
			if !strings.Contains(logged.String(), c.inLog) {
				t.Errorf("log %q lacks %q", logged.String(), c.inLog)
			}
		})
	}
}
