//go:build threshold

package composition

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
)

// TestListCostThreshold measures what a list screen costs at 1,000, 5,000 and 50,000 rows -- the numbers the BRD for
// flexible presentation (menata-app-document audits/2026-11-19-brd-penyajian-data-fleksibel-ala-views.md, F0) needs
// before any pager, viewer-chosen sort or SQL aggregation is built. TestVolumeThreshold answered only "how long does
// reading the whole Machine take"; this asks the questions the new controls would put: a filter, a sort by a Field, a
// text search, a count, a deep page, and an aggregate, each through the same Store statements a screen uses (or, where
// the Store has no such statement yet -- OFFSET, GROUP BY -- the SQL it would send, written out here).
//
// **Run it against a scratch schema, not the live tables**: it seeds tens of thousands of rows and creates and drops
// two expression indexes. `make threshold`'s own rule applies (cleanupBench removes `mch_bench_%` rows and
// reindexes), and the schema form is THRESHOLD_DATABASE_URL=<url>&options=-csearch_path%3Dbench with a `bench.records`
// made by `create table bench.records (like public.records including all)`.
//
// It reports medians; it asserts nothing, because the point is the number, and a gate is chosen from it afterwards.
const listBenchMachine = "mch_bench_list"

func seedList(t *testing.T, pool *pgxpool.Pool, n int) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM records WHERE machine_id = $1`, listBenchMachine); err != nil {
		t.Fatalf("clear: %v", err)
	}
	// Titles are md5 hashes so that ordering by title is not the order of creation (a sort that happens to match the
	// insert order would be measured too kindly). Five statuses; amounts spread over 0..99,999; a due date over a year.
	_, err := pool.Exec(ctx, `
		INSERT INTO records (id, machine_id, data, sort_order)
		SELECT 'rec_bench_list_' || g, $1,
		       jsonb_build_object(
		         'fld_title',  'T' || md5(g::text),
		         'fld_status', (ARRAY['draft','review','approved','paid','closed'])[1 + g % 5],
		         'fld_amount', ((g * 37) % 100000)::text,
		         'fld_due',    to_char(date '2026-01-01' + (g % 365), 'YYYY-MM-DD'),
		         'fld_notes',  repeat('lorem ipsum ', 12)),
		       g
		FROM generate_series(1, $2) AS g`, listBenchMachine, n)
	if err != nil {
		t.Fatalf("seed %d: %v", n, err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE records`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

// median runs f once untimed, then runs times and returns the median and the slowest.
func median(t *testing.T, runs int, f func() error) (med, worst time.Duration) {
	t.Helper()
	if err := f(); err != nil {
		t.Fatalf("warm: %v", err)
	}
	ds := make([]time.Duration, runs)
	for i := range ds {
		start := time.Now()
		if err := f(); err != nil {
			t.Fatalf("run: %v", err)
		}
		ds[i] = time.Since(start)
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return ds[runs/2], ds[runs-1]
}

func TestListCostThreshold(t *testing.T) {
	pool := benchPool(t)
	defer pool.Close()
	// This test creates and drops real indexes and seeds 50,000 rows, so it refuses the live tables: `make threshold`
	// runs every Threshold test against DATABASE_URL, and a CREATE INDEX on the live `records` blocks its writes.
	var schema string
	if err := pool.QueryRow(context.Background(), `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current schema: %v", err)
	}
	if schema != "bench" {
		t.Skipf("current schema is %q: run this against a scratch schema (see the comment above), never the live tables", schema)
	}
	defer cleanupBench(t, pool)
	store := data.NewStore(pool)
	ctx := data.WithWorkspaceScope(context.Background(), "ws_default")
	const page = 25
	const runs = 9

	report := func(n int, name string, f func() error) {
		med, worst := median(t, runs, f)
		verdict := ""
		if med > interactiveBudget {
			verdict = "  OVER 100ms"
		}
		t.Logf("n=%-6d %-58s median %-9v worst %-9v%s", n, name, med.Round(10*time.Microsecond), worst.Round(10*time.Microsecond), verdict)
	}
	status := []data.FieldPredicate{{Field: "fld_status", Value: "review"}}
	statusAndAmount := []data.FieldPredicate{{Field: "fld_status", Value: "review"}, {Field: "fld_amount", Value: "50000", Order: ">=", Numeric: true}}
	search := []data.FieldPredicate{{Field: "fld_title", Value: "ab", Contains: true}}
	rare := []data.FieldPredicate{{Field: "fld_amount", Value: "37"}}                     // a handful of rows match: the scan cannot stop early
	noMatch := []data.FieldPredicate{{Field: "fld_title", Value: "zzzz", Contains: true}} // nothing matches: a full scan
	byTitle := []data.SortKey{{Column: "data->>'fld_title'"}}
	byAmount := []data.SortKey{{Column: "(data->>'fld_amount')::numeric", Descending: true}}

	rawRows := func(q string, args ...any) func() error {
		return func() error {
			rows, err := pool.Query(ctx, q, args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
			}
			return rows.Err()
		}
	}
	scope := `workspace_id = 'ws_default' AND machine_id = '` + listBenchMachine + `'`

	for _, n := range []int{1_000, 5_000, 50_000} {
		seedList(t, pool, n)
		deep := n - page
		t.Logf("---- %d rows ----", n)
		report(n, "whole-Machine read (the pattern aggregation uses)", func() error { _, err := store.ListRecords(ctx, listBenchMachine); return err })
		report(n, "page 1, default order, limit 25", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", nil, nil, page)
			return err
		})
		report(n, "filter status (20%), limit 25", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", status, nil, page)
			return err
		})
		report(n, "filter status + amount>=50000, sort amount desc, limit 25", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", statusAndAmount, byAmount, page)
			return err
		})
		report(n, "sort by title (text, unindexed), limit 25", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", nil, byTitle, page)
			return err
		})
		report(n, "search title contains 'ab', limit 25", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", search, nil, page)
			return err
		})
		report(n, "filter rare value (few match), limit 25", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", rare, nil, page)
			return err
		})
		report(n, "search title contains 'zzzz' (no match, full scan)", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", noMatch, nil, page)
			return err
		})
		report(n, "count of filter status (pager total)", func() error {
			_, err := store.CountRecordsSelect(ctx, listBenchMachine, "ds_bench", status)
			return err
		})
		report(n, "OFFSET deep page, default order (SQL written out)", rawRows(
			`SELECT id, data FROM records WHERE `+scope+` ORDER BY sort_order ASC, created_at ASC OFFSET $1 LIMIT $2`, deep, page))
		report(n, "OFFSET deep page, sort by title (SQL written out)", rawRows(
			`SELECT id, data FROM records WHERE `+scope+` ORDER BY data->>'fld_title' ASC, id OFFSET $1 LIMIT $2`, deep, page))
		report(n, "SQL aggregate: count and sum by status (GROUP BY)", rawRows(
			`SELECT data->>'fld_status', count(*), sum((data->>'fld_amount')::numeric) FROM records WHERE `+scope+` GROUP BY 1`))
		report(n, "Go aggregate: whole read, then count and sum by status", func() error {
			rs, err := store.ListRecords(ctx, listBenchMachine)
			if err != nil {
				return err
			}
			counts, sums := map[string]int{}, map[string]float64{}
			for _, r := range rs {
				s, _ := r.Values["fld_status"].(string)
				counts[s]++
				if a, ok := r.Values["fld_amount"].(string); ok {
					f, _ := strconv.ParseFloat(a, 64)
					sums[s] += f
				}
			}
			return nil
		})

		// What an expression index would buy, measured and then dropped: nothing shipped here.
		for _, ix := range []struct{ name, expr string }{
			{"bench_ix_status", `(data->>'fld_status')`},
			{"bench_ix_title", `(data->>'fld_title')`},
			{"bench_ix_amount", `(((data->>'fld_amount'))::numeric)`},
		} {
			if _, err := pool.Exec(ctx, fmt.Sprintf(`CREATE INDEX %s ON records (workspace_id, machine_id, %s)`, ix.name, ix.expr)); err != nil {
				t.Fatalf("index %s: %v", ix.name, err)
			}
		}
		if _, err := pool.Exec(ctx, `ANALYZE records`); err != nil {
			t.Fatalf("analyze: %v", err)
		}
		t.Logf("---- %d rows, with expression indexes on status, title, amount ----", n)
		report(n, "filter rare value (few match) [indexed on amount only]", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", rare, nil, page)
			return err
		})
		report(n, "filter status (20%), limit 25 [indexed]", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", status, nil, page)
			return err
		})
		report(n, "sort by title, limit 25 [indexed]", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", nil, byTitle, page)
			return err
		})
		report(n, "filter status + amount>=50000, sort amount desc [indexed]", func() error {
			_, _, err := store.ListRecordsSelect(ctx, listBenchMachine, "ds_bench", statusAndAmount, byAmount, page)
			return err
		})
		report(n, "OFFSET deep page, sort by title [indexed]", rawRows(
			`SELECT id, data FROM records WHERE `+scope+` ORDER BY data->>'fld_title' ASC, id OFFSET $1 LIMIT $2`, deep, page))
		for _, name := range []string{"bench_ix_status", "bench_ix_title", "bench_ix_amount"} {
			if _, err := pool.Exec(ctx, `DROP INDEX `+name); err != nil {
				t.Errorf("drop %s: %v", name, err)
			}
		}
	}
}
