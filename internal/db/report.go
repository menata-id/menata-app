package db

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// poolSample is the part of pgxpool.Stat a report reads, copied out because Stat has no public
// constructor and a test cannot build one -- and because the formatting is the part worth testing.
type poolSample struct {
	total, idle, acquired, max int32
	acquires, empty, canceled  int64
	acquireWait                time.Duration
}

func sampleOf(s *pgxpool.Stat) poolSample {
	return poolSample{
		total: s.TotalConns(), idle: s.IdleConns(), acquired: s.AcquiredConns(), max: s.MaxConns(),
		acquires: s.AcquireCount(), empty: s.EmptyAcquireCount(), canceled: s.CanceledAcquireCount(),
		acquireWait: s.AcquireDuration(),
	}
}

// ReportPool logs one POOL line every interval until ctx ends: what a per-request line cannot say,
// whether the pool itself is the bottleneck, plus the two runtime numbers (goroutines, heap) that
// show a leak growing between requests. It is the only periodic line in app.log.
//
// Gauges (conns, idle, acquired) are the instant of the tick; the counters (acquires, empty, wait)
// are the change since the previous tick, because pgxpool's own are cumulative since start and a
// cumulative number cannot show that the last five minutes were worse than the five before. The
// first line therefore covers everything since start.
func ReportPool(ctx context.Context, pool *pgxpool.Pool, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	var prev poolSample
	quiet := 0 // consecutive ticks whose line was left out
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cur := sampleOf(pool.Stat())
			if poolLineDue(prev, cur, quiet) {
				var mem runtime.MemStats
				runtime.ReadMemStats(&mem)
				log.Print(formatPoolLine(prev, cur, runtime.NumGoroutine(), mem.HeapAlloc))
				quiet = 0
			} else {
				quiet++
			}
			prev = cur
		}
	}
}

// quietPoolTicks is how many consecutive idle ticks are left unprinted before one is printed anyway.
// Six at five minutes is a line every half hour on a host nobody is using: the heap and goroutine
// gauges still show a leak growing, at a sixth of the lines.
const quietPoolTicks = 6

// poolLineDue reports whether this tick prints. An interval in which the pool was not asked for a
// connection (no acquires) and no caller gave up on one says only what the previous line said, so it
// is left out until quietPoolTicks of them have passed. A canceled acquire always prints, since that
// line carries the ANOMALY marker; so does the first tick, which covers everything since start.
func poolLineDue(prev, cur poolSample, quiet int) bool {
	if cur.acquires != prev.acquires || cur.canceled != prev.canceled {
		return true
	}
	return quiet >= quietPoolTicks-1 || prev == (poolSample{})
}

// formatPoolLine renders one report. `empty` is acquires that found no idle connection and had to
// wait for or open one: it is expected to be nonzero while a cold pool grows, and means saturation
// only when it stays nonzero with conns equal to max. `canceled` is the one that is marked: a
// caller gave up waiting for a connection, which no healthy pool does.
func formatPoolLine(prev, cur poolSample, goroutines int, heapBytes uint64) string {
	acquires := cur.acquires - prev.acquires
	wait := cur.acquireWait - prev.acquireWait
	var avg time.Duration
	if acquires > 0 {
		avg = wait / time.Duration(acquires)
	}
	canceled := cur.canceled - prev.canceled
	marker := ""
	if canceled > 0 {
		marker = "ANOMALY(pool_canceled) "
	}
	return fmt.Sprintf("%sPOOL conns=%d/%d idle=%d acquired=%d acquires=+%d empty=+%d canceled=+%d avg_wait=%.2fms goroutines=%d heap=%.1fMB",
		marker, cur.total, cur.max, cur.idle, cur.acquired, acquires, cur.empty-prev.empty, canceled,
		float64(avg)/float64(time.Millisecond), goroutines, float64(heapBytes)/(1<<20))
}
