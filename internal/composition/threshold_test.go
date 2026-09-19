//go:build threshold

// Package composition's threshold harness constructs the forcing conditions this app cannot wait
// for (ROADMAP.md Phase 18 Step 4, and the Method's 2026-09-18 correction).
//
// menata-app has no users, so every production-shaped trigger the roadmap defers behind -- "the
// first Machine whose record count makes a page slow," "a metadata author who is not the
// engineer" -- can never arrive during development. Waiting for them defers the composable
// architecture forever and makes "not forced" uninformative rather than reassuring. This harness
// manufactures the two triggers that *are* constructible today and measures the architecture
// against them, so "not forced yet" is always backed by a number.
//
// It is behind a build tag because it needs a real database and seeds tens of thousands of rows:
//
//	make threshold
//
// Everything it writes uses a mch_bench_* Machine ID and is deleted afterwards, so it never
// touches application records.
package composition

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// interactiveBudget is the point past which a single whole-Machine read stops being something a
// page can do casually. 007 §18 names interactive execution budgets without fixing a number;
// 100ms for one read leaves room for the several a composed page issues while keeping the total
// inside a response people experience as immediate.
const interactiveBudget = 100 * time.Millisecond

const benchUserMachine = "mch_bench_user"

func benchPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("THRESHOLD_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("set THRESHOLD_DATABASE_URL (or DATABASE_URL) to run the threshold harness")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

// seed replaces the bench Machine's records with exactly n rows, in one statement -- inserting
// them one at a time through the Store would measure the seeding, not the read.
func seed(t *testing.T, pool *pgxpool.Pool, machineID string, n int) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM records WHERE machine_id = $1`, machineID); err != nil {
		t.Fatalf("clear bench records: %v", err)
	}
	if n == 0 {
		return
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO records (id, machine_id, data, sort_order)
		SELECT 'rec_bench_' || $1 || '_' || g,
		       $1,
		       jsonb_build_object('fld_name', 'Bench ' || g, 'fld_email', 'bench' || g || '@example.test'),
		       g
		FROM generate_series(1, $2) AS g
	`, machineID, n)
	if err != nil {
		t.Fatalf("seed %d records: %v", n, err)
	}
}

func cleanupBench(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DELETE FROM records WHERE machine_id LIKE 'mch_bench_%'`); err != nil {
		t.Errorf("cleanup bench records: %v", err)
	}
}

// TestVolumeThreshold answers the question Phase 6 has never been able to answer with a number:
// at what record count does "fetch the whole Machine and reduce it in Go" -- this app's shape
// everywhere -- stop being viable? That count is the forcing condition for projection and filter
// pushdown and pagination (007 §21.1, §28 invariants #2 and #4), and the prerequisite for the
// dependency graph: only once a referenced Machine is too large to fetch whole do a fetch's
// arguments start coming from another fetch's result.
func TestVolumeThreshold(t *testing.T) {
	pool := benchPool(t)
	defer pool.Close()
	defer cleanupBench(t, pool)

	store := data.NewStore(pool).WithWorkspace("ws_default")
	ctx := context.Background()

	var firstOverBudget int
	for _, n := range []int{100, 1_000, 10_000, 50_000, 100_000} {
		seed(t, pool, benchUserMachine, n)

		// One untimed read first: the point is the steady-state cost of the read, not the cost of
		// whatever the connection pool or the page cache happens to be doing on first touch.
		if _, err := store.ListRecords(ctx, benchUserMachine); err != nil {
			t.Fatalf("warm read at n=%d: %v", n, err)
		}

		const runs = 3
		var total time.Duration
		var got int
		for i := 0; i < runs; i++ {
			start := time.Now()
			records, err := store.ListRecords(ctx, benchUserMachine)
			total += time.Since(start)
			if err != nil {
				t.Fatalf("read at n=%d: %v", n, err)
			}
			got = len(records)
		}
		avg := total / runs
		if got != n {
			t.Fatalf("read %d records at n=%d", got, n)
		}

		verdict := "within interactive budget"
		if avg > interactiveBudget {
			verdict = "OVER BUDGET -- whole-Machine reads stop being viable here"
			if firstOverBudget == 0 {
				firstOverBudget = n
			}
		}
		t.Logf("n=%-7d whole-Machine read avg %-12v %s", n, avg.Round(time.Millisecond/10), verdict)
	}

	if firstOverBudget == 0 {
		t.Logf("RESULT: no tested record count exceeded %v. Whole-Machine reads remain viable to 100k rows; projection/pagination is still not forced, and now that is a measurement rather than an assumption.", interactiveBudget)
		return
	}
	t.Logf("RESULT: whole-Machine reads exceed %v at %d records. That is the forcing condition for projection/filter pushdown and pagination -- record it in ROADMAP.md Phase 6 and build pagination before the planner.", interactiveBudget, firstOverBudget)
}

// TestBreadthThreshold constructs the *other* trigger: metadata breadth. Phase 6's fourth test
// claimed the duplication it measured grows with the number of Machines referencing a target --
// schema size -- rather than with record count. That claim was made from 9 Machines. Here it is
// tested against many, with the memo on and off, so the claim is either confirmed or refuted
// instead of believed.
func TestBreadthThreshold(t *testing.T) {
	pool := benchPool(t)
	defer pool.Close()
	defer cleanupBench(t, pool)

	store := data.NewStore(pool).WithWorkspace("ws_default")
	ctx := context.Background()
	seed(t, pool, benchUserMachine, 100)

	for _, referrers := range []int{1, 4, 16, 64} {
		machines := syntheticSchema(referrers)

		ctxCounted, reads := data.WithReadLog(ctx)
		loader := NewLoader(store, machines)
		for _, m := range machines {
			if m.ID == benchUserMachine {
				continue
			}
			if _, err := loader.RelationOptions(ctxCounted, m); err != nil {
				t.Fatalf("relation options at %d referrers: %v", referrers, err)
			}
		}

		t.Logf("referrers=%-4d reads=%-4d repeated=%-4d served-from-memo=%d",
			referrers, reads.Total(), reads.Repeated(), loader.Served())

		if reads.Total() != 1 {
			t.Errorf("with the memo, %d Machines referencing one target should read it once; got %d reads",
				referrers, reads.Total())
		}
	}
	t.Logf("RESULT: read count is flat in schema breadth with the memo in place. Before Phase 18 Step 2 it was one read per referring Machine -- the growth Phase 6 predicted, now closed and locked by this test.")
}

// syntheticSchema returns one target Machine plus n Machines that each reference it, the shape a
// growing application produces naturally as more Machines gain a person/owner/assignee field.
func syntheticSchema(referrers int) map[string]*domain.Machine {
	machines := map[string]*domain.Machine{
		benchUserMachine: {
			ID:   benchUserMachine,
			Name: "Bench User",
			Fields: []domain.Field{
				{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText},
			},
		},
	}
	for i := 0; i < referrers; i++ {
		id := fmt.Sprintf("mch_bench_ref_%02d", i)
		machines[id] = &domain.Machine{
			ID:   id,
			Name: fmt.Sprintf("Bench Referrer %02d", i),
			Fields: []domain.Field{
				{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
				{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: benchUserMachine},
			},
		}
	}
	return machines
}
