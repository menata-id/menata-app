package main

import (
	"net/http"
	"testing"
	"time"
)

// TestServerTimeoutsOutlastTheAIClient: every timeout is set (K20), and WriteTimeout stays above the 60s
// Gemini client timeout in internal/aiassist, so bounding the server cannot cut the assistant off mid-answer.
func TestServerTimeoutsOutlastTheAIClient(t *testing.T) {
	s := newServer(":0", http.NotFoundHandler())
	for name, d := range map[string]time.Duration{
		"ReadHeaderTimeout": s.ReadHeaderTimeout, "ReadTimeout": s.ReadTimeout,
		"WriteTimeout": s.WriteTimeout, "IdleTimeout": s.IdleTimeout,
	} {
		if d <= 0 {
			t.Errorf("%s is unset -- http.Server waits forever on that stage", name)
		}
	}
	const geminiClientTimeout = 60 * time.Second // internal/aiassist.NewClientFromConfig
	if s.WriteTimeout <= geminiClientTimeout {
		t.Errorf("WriteTimeout %v does not outlast the %v Gemini client timeout", s.WriteTimeout, geminiClientTimeout)
	}
}
