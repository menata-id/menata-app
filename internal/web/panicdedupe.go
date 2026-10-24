package web

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

// panicRepeatWindow is how long a panic's stack is not printed again once printed.
const panicRepeatWindow = 10 * time.Minute

// ServerErrorLog is the logger to hand http.Server.ErrorLog: it carries no timestamp of its own, and
// each write goes through the standard logger, which stamps it like every other line in app.log.
func ServerErrorLog() *log.Logger {
	return log.New(DedupePanics(stdLogWriter{}), "", 0)
}

type stdLogWriter struct{}

func (stdLogWriter) Write(p []byte) (int, error) {
	log.Print(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// DedupePanics wraps the writer behind http.Server.ErrorLog (ServerErrorLog uses it; a test gives it a buffer). net/http logs a panic in a handler as
// one write: "http: panic serving <addr>: <message>" and then about forty lines of stack. One broken
// route polled by an open tab did that 120 times in a day (2026-09-30) and those traces were a
// quarter of every line that was not a request. The first panic with a given message is written in
// full; the same message again inside panicRepeatWindow is one line saying how many times, which is
// what a review needs once the cause is known. A write that is not a panic passes through untouched.
// The request's own ANOMALY(panic) line is a different writer and is never suppressed.
func DedupePanics(w io.Writer) io.Writer {
	return &panicDedupe{out: w, now: time.Now, seen: map[string]*panicSeen{}}
}

type panicSeen struct {
	first   time.Time
	skipped int
}

type panicDedupe struct {
	out  io.Writer
	now  func() time.Time
	mu   sync.Mutex
	seen map[string]*panicSeen
}

func (d *panicDedupe) Write(p []byte) (int, error) {
	key, ok := panicKey(p)
	if !ok {
		return d.out.Write(p)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	for k, v := range d.seen { // a message that carries an id is a new key each time; do not keep them all
		if now.Sub(v.first) >= panicRepeatWindow {
			delete(d.seen, k)
		}
	}
	if s := d.seen[key]; s != nil && now.Sub(s.first) < panicRepeatWindow {
		s.skipped++
		_, err := fmt.Fprintf(d.out, "http: panic repeated (stack shown at first occurrence, %d suppressed since): %s\n", s.skipped, key)
		return len(p), err
	}
	d.seen[key] = &panicSeen{first: now}
	return d.out.Write(p)
}

// panicKey is the message of a net/http panic report, without the client address that varies.
func panicKey(p []byte) (string, bool) {
	line, _, _ := bytes.Cut(p, []byte("\n"))
	const prefix = "http: panic serving "
	rest, ok := strings.CutPrefix(string(line), prefix)
	if !ok {
		return "", false
	}
	_, msg, ok := strings.Cut(rest, ": ")
	return msg, ok
}
