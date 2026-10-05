package db

import (
	"strings"
	"testing"
	"time"
)

func TestFormatPoolLineReportsTheIntervalNotTheLifetime(t *testing.T) {
	prev := poolSample{acquires: 100, empty: 5, canceled: 0, acquireWait: 50 * time.Millisecond}
	cur := poolSample{total: 4, idle: 3, acquired: 1, max: 10, acquires: 120, empty: 5, acquireWait: 60 * time.Millisecond}
	got := formatPoolLine(prev, cur, 31, 20<<20)
	for _, want := range []string{"POOL conns=4/10 idle=3 acquired=1", "acquires=+20", "empty=+0", "canceled=+0", "avg_wait=0.50ms", "goroutines=31", "heap=20.0MB"} {
		if !strings.Contains(got, want) {
			t.Errorf("line lacks %q:\n  %s", want, got)
		}
	}
	if strings.Contains(got, "ANOMALY") {
		t.Errorf("a healthy interval was marked:\n  %s", got)
	}
}

func TestFormatPoolLineMarksOnlyCanceledAcquires(t *testing.T) {
	// A saturated-looking interval (empty acquires, full pool) is not marked: it is expected on a
	// cold pool. A caller giving up on a connection is.
	busy := formatPoolLine(poolSample{}, poolSample{total: 10, max: 10, acquires: 50, empty: 30}, 5, 1<<20)
	if strings.Contains(busy, "ANOMALY") {
		t.Errorf("empty acquires alone were marked:\n  %s", busy)
	}
	gave := formatPoolLine(poolSample{}, poolSample{max: 10, acquires: 1, canceled: 2}, 5, 1<<20)
	if !strings.Contains(gave, "ANOMALY(pool_canceled)") {
		t.Errorf("canceled acquires were not marked:\n  %s", gave)
	}
}

func TestFormatPoolLineSurvivesAnIdleInterval(t *testing.T) {
	// zero acquires must not divide by zero
	if got := formatPoolLine(poolSample{acquires: 7}, poolSample{acquires: 7, max: 10}, 1, 0); !strings.Contains(got, "avg_wait=0.00ms") {
		t.Errorf("idle interval: %s", got)
	}
}
