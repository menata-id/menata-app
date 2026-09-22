package web

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
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
// The number is measured, not chosen, in the same spirit as conformance's maxHandlerLines: /home
// issued seventeen queries before the 2026-09-22 identity work and twelve after, and this ceiling
// leaves an honest page room without leaving room for a re-introduced duplicate read. Raising it
// should be a deliberate act in a commit that says why -- a page quietly costing twice what it
// did is precisely the drift this whole exercise was about.
//
// **It went 12 -> 14 within that same day, and the reason is worth keeping.** The first figure was
// measured against a fixture Workspace carrying Machines and no Applications; against the real
// installed Workspace, /home also counts each Application's summary Machine, which is two more
// queries it genuinely needs (`mch_document count`, `mch_task count`). The budget did not move
// because the page got worse -- it moved because the first measurement was of something thinner
// than production, which is the same mistake in miniature that this whole entry is about.
const maxQueriesPerAuthenticatedPage = 14

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

// minSweptRoutes guards this sweep the way conformance's `checked == 0` guards its own static
// checks: a refactor that changed how routes are registered would leave the AST scan finding
// nothing and the sweep passing while exercising zero routes. The number is below today's count
// so ordinary route churn does not trip it, and far above zero so silence fails.
const minSweptRoutes = 8

// getSweepRatchet grandfathers the routes that already violate the two invariants, each with what
// it violated when the sweep was written (2026-09-22). **The list may only shrink.**
//
// It is a ratchet for the same reason TestRenderingUsesProjectionNotRawValues is one: the gate is
// worth having now, the seven routes below are worth fixing, and holding the gate hostage to
// fixing them first is how a check that would have prevented the next instance ends up not
// existing. Adding an entry is not the way to make this pass -- an entry left behind after its
// route is fixed fails too, so the list cannot quietly stop meaning anything.
//
// **These seven are the answer to "do the two budget tests above cover enough?".** They did not:
// before this sweep, two routes of fifty-four were checked, both of them ones that had just been
// worked on. Every route below has the same disease those two had, and none of it was visible.
//
// The shape is familiar. `unnamed=` is a Store method that never got a record() target (the
// invite and workspace-listing paths). `repeated=` is the admin screens re-reading membership,
// groups and app roles per member or per row -- resolveIdentity fixed the *request-level*
// duplication, and these are the *within-handler* kind it cannot see: a loop over members that
// asks the database about each one separately.
var getSweepRatchet = map[string]string{
	"/authorization-matrix":       "repeated=4 (membership, groups, app roles each read twice)",
	"/create-workspace":           "unnamed=1 (the Workspace-listing query names no target)",
	"/documents/new":              "unnamed=2, repeated=3 (approver picker re-reads groups per row)",
	"/documents/new/approver-row": "unnamed=2, repeated=1 (same picker, rendered as a fragment)",
	"/switch-workspace":           "unnamed=1, repeated=1 (Workspace row read twice, listing unnamed)",
	"/workspace-groups":           "repeated=5 (groups and memberships re-read per group)",
	"/workspace-members":          "unnamed=3, repeated=6 (the worst: per-member membership and group reads)",
}

// TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed sweeps every authenticated GET route that needs no
// path parameter and holds all of them to the two invariants.
//
// WHY A SWEEP AND NOT MORE BUDGETS. The two tests above assert a *number* for one route each,
// which is a threshold someone chose and which will rot; extending that shape to all fifty-four
// routes would produce fifty-four numbers to maintain and an invitation to raise whichever one
// fails. These two properties are different in kind -- they are invariants, with one correct
// value that does not depend on how much data exists:
//
//   - `queries == reads`: every statement named itself, so the breakdown beside it is complete.
//   - `repeated == 0`: nothing was read twice. composition.Loader memoizes within a request and
//     the identity resolves once on ctx, so on a GET there is nothing left that legitimately
//     re-reads. (A write is exempt -- see anomalyPrefix -- which is why this sweep is GET only.)
//
// Routes are discovered by parsing router.go rather than listed here, so a route added tomorrow is
// covered without anyone remembering to add it. That is the failure this sweep exists to prevent:
// before it, two routes of fifty-four were checked, which is a kind of coverage that reassures
// more than it protects.
func TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed(t *testing.T) {
	h, cookie := newRouterTestSetup(t, "getsweep")

	swept, skipped := 0, []string{}
	for _, route := range authenticatedGetRoutes(t) {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})

		rec := httptest.NewRecorder()
		var buf bytes.Buffer
		previous, flags := log.Writer(), log.Flags()
		log.SetOutput(&buf)
		log.SetFlags(0)
		h.ServeHTTP(rec, req)
		log.SetOutput(previous)
		log.SetFlags(flags)

		// A route this fixture cannot reach (no Application installed, no seeded record) tells us
		// nothing about its query shape. Recorded rather than passed over silently: an honest
		// sweep says what it did not cover.
		if rec.Code != http.StatusOK {
			skipped = append(skipped, fmt.Sprintf("%s (%d)", route, rec.Code))
			continue
		}
		line := diagnosticLine(buf.String())
		if line == "" {
			skipped = append(skipped, route+" (no diagnostic)")
			continue
		}
		swept++

		queries, reads, repeated := parseCounts(line)
		clean := queries == reads && repeated == 0
		grandfathered, inRatchet := getSweepRatchet[route]

		if inRatchet {
			if clean {
				t.Errorf("%s is in getSweepRatchet (%q) but now satisfies both invariants -- remove the entry\n"+
					"  the list may only shrink, and an entry left behind is one nobody is checking",
					route, grandfathered)
			}
			continue
		}
		if queries != reads {
			t.Errorf("%s: queries=%d reads=%d -- %d statement(s) named nothing\n  %s\n"+
				"  give the query a readLogFrom(ctx).record(...) target. Adding this route to\n"+
				"  getSweepRatchet is not the way to pass",
				route, queries, reads, queries-reads, line)
		}
		if repeated > 0 {
			t.Errorf("%s: repeated=%d -- a GET read the same target twice\n  %s\n"+
				"  composition.Loader memoizes within a request and the identity resolves once on ctx;\n"+
				"  a repeat on a read path means something bypassed both, usually a per-row query in a loop",
				route, repeated, line)
		}
	}

	for route := range getSweepRatchet {
		if !sweptRoute(route, skipped) {
			continue
		}
		t.Errorf("getSweepRatchet names %s, which this sweep never exercised -- the entry protects nothing", route)
	}

	t.Logf("swept %d GET routes; not exercised by this fixture: %v", swept, skipped)
	if swept < minSweptRoutes {
		t.Fatalf("only %d routes were actually exercised (want >= %d) -- this sweep is measuring far less than it appears to",
			swept, minSweptRoutes)
	}
}

// sweptRoute reports whether route was skipped rather than measured -- a ratchet entry for a
// route this fixture never reaches is protecting nothing and should say so.
func sweptRoute(route string, skipped []string) bool {
	for _, s := range skipped {
		if strings.HasPrefix(s, route+" ") {
			return true
		}
	}
	return false
}

// authenticatedGetRoutes returns the literal paths of every GET route registered inside
// router.go's authenticated group, excluding those taking a path parameter or wildcard (which
// would need a seeded record this fixture does not create) and the badge endpoint (covered above
// with its own budget).
func authenticatedGetRoutes(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "router.go", nil, 0)
	if err != nil {
		t.Fatalf("parse router.go: %v", err)
	}

	var routes []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Get" || len(call.Args) != 2 {
			return true
		}
		// pr and ar are the authenticated group and its admin subgroup; r is the public router,
		// whose routes carry no session and so no identity cost to measure.
		recv, ok := sel.X.(*ast.Ident)
		if !ok || (recv.Name != "pr" && recv.Name != "ar") {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		path := strings.Trim(lit.Value, `"`)
		if strings.ContainsAny(path, "{*") {
			return true
		}
		routes = append(routes, path)
		return true
	})
	sort.Strings(routes)
	return routes
}

// diagnosticLine picks queryDiagnostics' own line out of captured log output.
func diagnosticLine(captured string) string {
	for _, l := range strings.Split(captured, "\n") {
		if queryCountPattern.MatchString(l) {
			return l
		}
	}
	return ""
}

// parseCounts reads the three numbers back out of a diagnostic line.
func parseCounts(line string) (queries, reads, repeated int) {
	m := queryCountPattern.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, 0
	}
	queries, _ = strconv.Atoi(m[1])
	reads, _ = strconv.Atoi(m[2])
	repeated, _ = strconv.Atoi(m[3])
	return queries, reads, repeated
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

	// The *real* installed Workspace, not testWorkspaceFor's Machine-ids-only stand-in: these
	// tests measure the middleware chain, and that chain resolves the current Application and its
	// navigation. A Workspace with no Applications would make several screens panic in
	// labelByID (they render a declared nav label) and would let requireApplicationAccess pass
	// trivially -- measuring a shape production never serves.
	machines, installed := loadRealMachines(t)
	installed.Slug = ws.Slug

	// A real role in each Application that declares a vocabulary, rather than pointing
	// cfg.AdminUserID at this member: the admin bypass would skip a branch every real request
	// takes, and the point here is to measure what a real member's request costs.
	for _, app := range installed.Applications {
		if len(app.Roles) == 0 {
			continue
		}
		if err := store.SetMemberAppRole(ctx, ws.ID, actor.ID, app.ID, app.Roles[0]); err != nil {
			t.Fatalf("SetMemberAppRole(%s): %v", app.ID, err)
		}
	}

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
