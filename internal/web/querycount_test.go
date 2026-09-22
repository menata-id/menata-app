package web

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/storage"
)

// This file measures what one authenticated request actually costs in queries, through the real
// router with its real middleware chain -- the thing the log claimed to report and did not.
//
// It exists because the 2026-09-22 log review could not answer "how many queries is a page?"
// from the log, and answering it by reading the call chain produced a number nobody could check.
// Every other test in this package mounts one handler on a bare chi router, which is right for
// testing that handler and useless here: the cost being measured is mostly middleware, and a bare
// router has none of it.
//
// The number is read out of queryDiagnostics' own log line rather than from an instrumented copy
// of it, so what this asserts is exactly what production prints. If the two ever diverge, this
// test is measuring the wrong thing and should fail rather than reassure.

// maxQueriesPerAuthenticatedPage bounds the queries one simple authenticated page may issue.
//
// The number is measured, not chosen, in the same spirit as conformance's maxHandlerLines: before
// the 2026-09-22 identity work /home cost what it cost, and this ceiling is set just above the
// measured figure so an honest page has room while a re-introduced duplicate read does not. Raising
// it should be a deliberate act in a commit that says why -- a page quietly costing twice what it
// did is precisely the drift this whole exercise was about.
const maxQueriesPerAuthenticatedPage = 12

var queryCountPattern = regexp.MustCompile(`queries=(\d+) reads=(\d+) repeated=(\d+)`)

// TestAuthenticatedPageQueryCost measures /home end to end and fails if it costs more than the
// budget or repeats a read it already made.
func TestAuthenticatedPageQueryCost(t *testing.T) {
	h, cookie := newRouterTestSetup(t, "querycost")

	req := httptest.NewRequest(http.MethodGet, "/home", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})

	queries, reads, repeated, line := serveAndCount(t, h, req)
	t.Logf("/home: %s", line)

	if queries == 0 {
		t.Fatal("/home issued no queries at all -- the diagnostic is not installed, so this test is measuring nothing")
	}
	if queries != reads {
		t.Errorf("/home: queries=%d but reads=%d -- %d statement(s) were issued by something that did not name itself\n"+
			"  the gap is the blind spot data.QueryTracer exists to expose: find the query and give it a\n"+
			"  readLogFrom(ctx).record(...) target, rather than letting the breakdown stay incomplete",
			queries, reads, queries-reads)
	}
	if queries > maxQueriesPerAuthenticatedPage {
		t.Errorf("/home issued %d queries, over the %d-query budget\n"+
			"  a page this expensive is usually re-resolving identity or Workspace that a middleware\n"+
			"  already resolved for this request -- check ctx before adding a cache",
			queries, maxQueriesPerAuthenticatedPage)
	}
	if repeated > 0 {
		t.Errorf("/home repeated %d read(s) it had already made: %s\n"+
			"  a request-scoped value read twice is the duplication composition.Loader and the ctx-carried\n"+
			"  identity both exist to remove", repeated, line)
	}
}

// maxQueriesPerNavBadge bounds the nav badge's own endpoint, which is deliberately far tighter
// than a page's: it renders one integer, it fires on every page carrying it (a third of all
// requests in the 2026-09-22 log), and it used to compose the entire Approval Inbox -- activity
// log, member names, SLA-breach *writes* and all -- to produce that integer.
const maxQueriesPerNavBadge = 8

// TestNavBadgeQueryCost holds the pending-approval badge to a cost proportionate to what it
// renders, and asserts it stays a read.
func TestNavBadgeQueryCost(t *testing.T) {
	h, cookie := newRouterTestSetup(t, "navbadge")

	req := httptest.NewRequest(http.MethodGet, "/api/approval-inbox/pending-count", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})

	queries, _, repeated, line := serveAndCount(t, h, req)
	t.Logf("nav badge: %s", line)

	if queries > maxQueriesPerNavBadge {
		t.Errorf("the nav badge issued %d queries, over the %d-query budget\n"+
			"  it renders a single integer; composing a whole screen to produce one is what this\n"+
			"  budget exists to catch (composition.PendingApprovalCount, not ApprovalInbox)",
			queries, maxQueriesPerNavBadge)
	}
	if repeated > 0 {
		t.Errorf("the nav badge repeated %d read(s): %s", repeated, line)
	}
}

// serveAndCount runs one request through h and returns the counts queryDiagnostics logged for it.
func serveAndCount(t *testing.T, h http.Handler, req *http.Request) (queries, reads, repeated int, line string) {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	flags := log.Flags()
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previous)
		log.SetFlags(flags)
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// A redirect counts as a failure here too: /home redirecting to /login means the session was
	// rejected, and the two or three queries that got that far are not a page's cost.
	if rec.Code != http.StatusOK {
		t.Fatalf("%s returned %d (want 200) -- measuring a redirect or error page's query count would be meaningless\n%s",
			req.URL.Path, rec.Code, buf.String())
	}
	for _, l := range strings.Split(buf.String(), "\n") {
		m := queryCountPattern.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		queries, _ = strconv.Atoi(m[1])
		reads, _ = strconv.Atoi(m[2])
		repeated, _ = strconv.Atoi(m[3])
		return queries, reads, repeated, l
	}
	t.Fatalf("no queries= diagnostic line was logged for %s -- queryDiagnostics is not in the chain\n%s",
		req.URL.Path, buf.String())
	return 0, 0, 0, ""
}

// tracedTestPool is authTestPool with the query tracer production installs, which this file and
// only this file needs: authTestPool builds a bare pgxpool, so a request served through it issues
// untraced queries and queryDiagnostics logs nothing at all. That is harmless for every other
// test in this package -- none of them measures cost -- and fatal here, so the pool is built the
// way cmd/server builds it rather than borrowing one that differs from production in exactly the
// dimension under test. It mirrors db.Connect, which internal/web may not import (plane rule).
func tracedTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run internal/web's query-cost measurement")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.ConnConfig.Tracer = data.NewQueryTracer()
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newRouterTestSetup builds the *real* router over a real Workspace and returns it with a session
// cookie for a member of that Workspace.
func newRouterTestSetup(t *testing.T, name string) (http.Handler, string) {
	t.Helper()
	pool := tracedTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: name + "-secret", SecureCookies: false}
	ctx := context.Background()
	email := name + "@example.com"

	ws, err := store.CreateWorkspace(ctx, name, name)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	actor, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(actor): %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, actor.ID, email, "admin", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	machines := realMachines(t)
	installed := testWorkspaceFor(machines)
	installed.Slug = ws.Slug

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}

	machineList := make([]*domain.Machine, 0, len(machines))
	for _, m := range machines {
		machineList = append(machineList, m)
	}

	h := Routes(Deps{
		Machines:           machines,
		MachineList:        machineList,
		Store:              store,
		Files:              files,
		Cfg:                cfg,
		Workspaces:         map[string]domain.Workspace{ws.Slug: installed},
		DefaultWorkspaceID: ws.ID,
	})
	return h, sessionCookieValueForTest(t, cfg, actor.ID, 0)
}
