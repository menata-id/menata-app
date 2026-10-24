package web

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRepeatedPanicPrintsItsStackOnce(t *testing.T) {
	var buf bytes.Buffer
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	d := DedupePanics(&buf).(*panicDedupe)
	d.now = func() time.Time { return now }
	trace := "http: panic serving [::1]:5: rendering: no navigation item with id nav_x\ngoroutine 9 [running]:\nmain.f()\n"

	d.Write([]byte(trace))
	d.Write([]byte(strings.Replace(trace, "[::1]:5", "[::1]:6", 1)))
	d.Write([]byte(strings.Replace(trace, "[::1]:5", "[::1]:7", 1)))
	if got := strings.Count(buf.String(), "goroutine 9"); got != 1 {
		t.Errorf("the stack was printed %d times, want 1:\n%s", got, buf.String())
	}
	if !strings.Contains(buf.String(), "2 suppressed since): rendering: no navigation item with id nav_x") {
		t.Errorf("the repeats are not counted:\n%s", buf.String())
	}

	d.Write([]byte("http: panic serving [::1]:8: another cause\ngoroutine 9 [running]:\n"))
	if got := strings.Count(buf.String(), "goroutine 9"); got != 2 {
		t.Errorf("a different cause must print its own stack; stacks printed = %d", got)
	}

	now = now.Add(panicRepeatWindow)
	d.Write([]byte(trace))
	if got := strings.Count(buf.String(), "goroutine 9"); got != 3 {
		t.Errorf("after the window the same cause prints in full again; stacks printed = %d", got)
	}

	before := buf.Len()
	d.Write([]byte("http: TLS handshake error from x: EOF\n"))
	if !strings.HasSuffix(buf.String()[before:], "EOF\n") {
		t.Errorf("a write that is not a panic must pass through untouched")
	}
}
