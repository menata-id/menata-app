package web

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/storage"
)

// TestPerRecordGetRoutes sweeps the twelve authenticated GET routes that take a path parameter --
// the half of the route table nothing exercised until 2026-09-28.
//
// **It exists because of a 404 that ran on menata.app for a day** (fixed in 1ce43fc). Every /review
// link from My Documents returned "not found": a derivation was handed nil, produced an empty Field
// name, and every Document looked like a Document with no steps. That is a real state with a real
// answer -- 404 -- so the screen failed in exactly the shape it is supposed to have. Nothing logged,
// nothing panicked, no test failed. The only signal was three 404s in an access log.
//
// The blind spot was declared, not hidden. TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed parses
// router.go precisely so a new route is covered without anyone remembering, and exists because
// "two routes of fifty-four were checked, which is a kind of coverage that reassures more than it
// protects" -- and then excluded every `{...}` path, with the reason written down: they would need a
// seeded record. So this test seeds them.
//
// **Both directions are asserted, because the bug lived in the gap between them.** A route valid for
// this Machine and record must render; a Machine holding no workflow role must still 404 on the
// role-gated ones. Only the first half would have been satisfied by "make /review always return 200",
// which is a wrong fix that a one-sided sweep would have blessed.
func TestPerRecordGetRoutes(t *testing.T) {
	h, cookie, fx := newPerRecordSweepSetup(t, "perrecord")

	cases := fx.cases(t)
	if len(cases) < minPerRecordCases {
		t.Fatalf("only %d route instances were built (want >= %d) -- the fixture stopped seeding and this sweep is measuring far less than it appears to",
			len(cases), minPerRecordCases)
	}

	// Every discovered per-record route must be claimed by at least one case, so a route added to
	// router.go tomorrow fails here rather than joining the set nothing checks -- which is the exact
	// failure this file exists to end.
	claimed := map[string]bool{}
	for _, c := range cases {
		claimed[c.pattern] = true
	}
	for _, route := range perRecordGetRoutes(t) {
		if !claimed[route] {
			t.Errorf("%s is a per-record GET route no case in this sweep covers -- add one (see fixture.cases). A route nothing exercises is how the /review 404 survived a day in production", route)
		}
	}

	swept := 0
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.url, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		queries, reads, repeated, line := serveAndCountStatus(t, h, req, &c)

		if c.status != c.wantStatus {
			t.Errorf("%s (%s) = %d, want %d\n  %s", c.url, c.why, c.status, c.wantStatus, c.body)
			continue
		}
		if c.wantStatus != http.StatusOK {
			continue // a refusal reads nothing worth measuring
		}
		swept++
		if line == "" {
			t.Errorf("%s produced no read diagnostic -- the middleware that installs it must run for every authenticated route", c.url)
			continue
		}
		if queries != reads {
			t.Errorf("%s: queries=%d reads=%d -- %d statement(s) named nothing\n  %s\n"+
				"  give the query a readLogFrom(ctx).record(...) target",
				c.url, queries, reads, queries-reads, line)
		}
		if repeated > 0 {
			t.Errorf("%s: repeated=%d -- a GET read the same target twice\n  %s\n"+
				"  composition.Loader memoizes within a request; a repeat means something bypassed it",
				c.url, repeated, line)
		}
	}
	t.Logf("swept %d per-record route instances across %d route patterns", swept, len(claimed))
}

// minPerRecordCases is the floor, the same guard minSweptRoutes gives the sibling sweep: a fixture
// that quietly stops seeding must fail rather than pass while measuring nothing.
const minPerRecordCases = 20

// routeCase is one concrete request this sweep makes, plus what it expects and why. The why is
// carried so a failure says which rule was violated rather than only which URL.
type routeCase struct {
	pattern    string // the router.go path this instance came from
	url        string
	wantStatus int
	why        string

	status int
	body   string
}

// serveAndCountStatus serves one request and reports both the status and the read diagnostic.
//
// It does not reuse serveAndCount: that helper t.Fatals on any non-200, which is right for a budget
// measured on one known-good page and exactly wrong here, where a *refusal* is half of what this
// sweep asserts.
func serveAndCountStatus(t *testing.T, h http.Handler, req *http.Request, c *routeCase) (queries, reads, repeated int, line string) {
	t.Helper()
	var buf bytes.Buffer
	previous, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	log.SetOutput(previous)
	log.SetFlags(flags)

	c.status = rec.Code
	if body := strings.TrimSpace(rec.Body.String()); len(body) > 200 {
		c.body = body[:200]
	} else {
		c.body = body
	}
	if line = diagnosticLine(buf.String()); line != "" {
		queries, reads, repeated = parseCounts(line)
	}
	return queries, reads, repeated, line
}

// perRecordFixture is what the sweep needs to build concrete URLs: one record per installed Machine,
// plus the few extras the role-gated routes need to be *valid* rather than merely well-formed.
type perRecordFixture struct {
	ws      domain.Workspace
	records map[string]string // machine id -> a seeded record id
	actorID string
	// memberID is a *second* member, not the viewer. /workspace-members/{id}/edit reads the target's
	// membership and the viewer's own, and the read diagnostic counts by target *name* with no
	// arguments -- so pointing this at the viewer would report "membership x2" for two reads about
	// two different people and make this sweep assert a bug that is not there.
	memberID string
	groupID  string
	uploaded string // a stored file key, for /uploads/*

	// ctx and store are the Workspace-scoped handles the fixture was seeded through, kept so a test can write
	// one more record (an activity event) the sweep itself does not need.
	ctx   context.Context
	store *data.Store
}

// cases builds every request this sweep makes.
//
// The role-gated routes ask domain.Workspace.MachineInWorkflowRole rather than naming mch_document --
// the same question internal/web.machineForDocument asks, and the only way this test stays true of a
// Workspace that installs an approval Application under different names (CLAUDE.md: a name is never
// an identity the runtime may branch on).
func (f perRecordFixture) cases(t *testing.T) []routeCase {
	t.Helper()
	var out []routeCase
	add := func(pattern, url string, want int, why string) {
		out = append(out, routeCase{pattern: pattern, url: url, wantStatus: want, why: why})
	}

	machineIDs := make([]string, 0, len(f.records))
	for id := range f.records {
		machineIDs = append(machineIDs, id)
	}
	sort.Strings(machineIDs)

	// The generic routes, over *every* installed Machine rather than one: they are the runtime's own
	// per-Machine screens, and "works for the Machine I happened to pick" is the coverage this repo
	// already calls reassuring rather than protective.
	for _, id := range machineIDs {
		rec := f.records[id]
		add("/machines/{machineID}", "/machines/"+id, http.StatusOK, "every installed Machine has a list screen")
		add("/api/machines/{machineID}/records", "/api/machines/"+id+"/records", http.StatusOK, "every installed Machine has a JSON record list")
		add("/machines/{machineID}/records/{id}", "/machines/"+id+"/records/"+rec, http.StatusOK, "every record has a detail page")
		add("/machines/{machineID}/records/{id}/edit", "/machines/"+id+"/records/"+rec+"/edit", http.StatusOK, "every record has an edit form")
	}

	doc := f.ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	step := f.ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleStep, "")
	if doc == nil || step == nil {
		t.Fatal("this Workspace casts no document/step role -- the role-gated half of this sweep would measure nothing")
	}

	docRec, stepRec := f.records[doc.ID], f.records[step.ID]
	add("/machines/{machineID}/records/{id}/review", "/machines/"+doc.ID+"/records/"+docRec+"/review", http.StatusOK,
		"a Document with steps opens its Review screen -- the 404 this sweep was written for")
	add("/machines/{machineID}/records/{id}/review", "/machines/"+step.ID+"/records/"+stepRec+"/review", http.StatusOK,
		"and so does a Step, the route's other arm")
	add("/machines/{machineID}/records/{id}/signature-placement", "/machines/"+doc.ID+"/records/"+docRec+"/signature-placement", http.StatusOK,
		"the Document's signature placement screen")
	add("/machines/{machineID}/records/{id}/pdf-preview", "/machines/"+doc.ID+"/records/"+docRec+"/pdf-preview?page=1", http.StatusOK,
		"page 1 of the Document's own PDF")
	// 200, because the seeded Document holds its status Field's *first* declared option, and
	// mch_document declares `draft` first. That is the happy path for this route, which is the one
	// worth covering -- the business-state refusal on a non-Draft has its own test
	// (TestContinueDocumentWizard, action.CanContinueDraft). Written as a comment rather than left to
	// be rediscovered: an expectation that depends on option order should say so.
	add("/machines/{machineID}/records/{id}/continue-submit", "/machines/"+doc.ID+"/records/"+docRec+"/continue-submit", http.StatusOK,
		"a Draft Document reopens the wizard")

	// The negative half. A Machine the approval engine casts in no role must refuse these, and that
	// refusal is what stops "make /review always return 200" from passing this sweep.
	for _, id := range machineIDs {
		m := machineByID(f.ws, id)
		if m == nil || m.WorkflowRole != "" {
			continue
		}
		add("/machines/{machineID}/records/{id}/review", "/machines/"+id+"/records/"+f.records[id]+"/review", http.StatusNotFound,
			"a Machine in no workflow role has no Review screen")
		add("/machines/{machineID}/records/{id}/signature-placement", "/machines/"+id+"/records/"+f.records[id]+"/signature-placement", http.StatusNotFound,
			"nor a signature placement screen")
		break // one is enough to hold the rule; every Machine would only repeat it
	}

	add("/workspace-members/{userRecordID}/edit", "/workspace-members/"+f.memberID+"/edit", http.StatusOK,
		"a Workspace admin edits *another* member -- see perRecordFixture.memberID")
	if f.groupID != "" {
		add("/workspace-groups/{groupID}", "/workspace-groups/"+f.groupID, http.StatusOK, "a Group's own detail screen")
	}
	if f.uploaded != "" {
		add("/uploads/*", "/uploads/"+f.uploaded, http.StatusOK, "a stored file is served back")
	}
	// /pages/{navID} renders a navigation item's declared `page:`. `default` declares none, so its 200 arm is
	// absent here and held instead by TestDeclaredPageRendersItsBindingAsRows against a Workspace that does;
	// this sweep still owns the refusals, which are the half a "render something for every id" bug would break:
	// an id naming nothing, and an id naming a real navigation item that has no body (a bespoke screen's).
	add("/pages/{navID}", "/pages/nav_does_not_exist", http.StatusNotFound, "an id that names no navigation item is not found")
	pageCase := false
	for _, app := range f.ws.Applications {
		for _, item := range app.AllNavigation {
			if item.Page != nil {
				add("/pages/{navID}", "/pages/"+item.ID, http.StatusOK, "a navigation item declaring `page:` renders its body")
			} else if !pageCase {
				add("/pages/{navID}", "/pages/"+item.ID, http.StatusNotFound, "a navigation item with no `page:` is a bespoke screen, not a page")
				pageCase = true
			}
		}
	}
	// /new-application/{session}/review is the AI assistant's own review screen, and an unknown session
	// is the case this sweep covers. Its happy path is covered by
	// TestShowNewApplicationReview_rendersTheProposalAndRefusesAnotherWorkspaces, which reuses the
	// fixture this comment once claimed did not exist (createGeneratedSession, newapplication_test.go).
	add("/new-application/{session}/review", "/new-application/does-not-exist/review", http.StatusNotFound,
		"an unknown assistant session is not found -- the happy path needs a seeded conversation, still unbuilt")
	return out
}

func machineByID(ws domain.Workspace, id string) *domain.Machine {
	for _, m := range ws.Machines {
		if m.ID == id {
			return m
		}
	}
	return nil
}

// seedRecordForEveryMachine creates one record per installed Machine, in dependency order so a
// required relation always points at something that exists.
//
// Values are built from each Machine's own declared Fields -- required ones only, typed by
// domain.FieldType -- rather than from a hand-written table per Machine. That is deliberate: a
// hand-written table is a second place to update when a Machine gains a required Field, and this
// fixture exists precisely because "someone will remember" already failed once.
func seedRecordForEveryMachine(t *testing.T, ctx context.Context, store *data.Store, files *storage.Store, ws domain.Workspace, actorID string) map[string]string {
	t.Helper()
	seeded := map[string]string{domain.UserMachineID: actorID}

	remaining := append([]*domain.Machine(nil), ws.Machines...)
	for pass := 0; len(remaining) > 0 && pass < len(ws.Machines)+1; pass++ {
		var blocked []*domain.Machine
		for _, m := range remaining {
			if _, done := seeded[m.ID]; done {
				continue
			}
			values, ready := seedValues(t, files, m, seeded, actorID)
			if !ready {
				blocked = append(blocked, m)
				continue
			}
			data.ApplyDefaults(m, values)
			rec, err := store.CreateRecord(ctx, m.ID, values)
			if err != nil {
				t.Fatalf("seed %s: %v (values %v)", m.ID, err, values)
			}
			id := rec.ID
			machineID := m.ID
			t.Cleanup(func() { _ = store.DeleteRecord(ctx, machineID, id) })
			seeded[m.ID] = id
		}
		remaining = blocked
	}
	if len(remaining) > 0 {
		var names []string
		for _, m := range remaining {
			names = append(names, m.ID)
		}
		t.Fatalf("could not seed %v -- a required relation cycle, or a target Machine this Workspace does not install", names)
	}

	linkOptionalReferences(t, ctx, store, ws, seeded, actorID)
	return seeded
}

// linkOptionalReferences is a second pass that fills the relation and person Fields a Machine does
// *not* require, once every record exists.
//
// It is separate from the first pass because only *required* relations can drive seeding order --
// an optional one may point at a Machine seeded later, or in a cycle, and blocking on it would make
// the graph unseedable. And it matters more than "more realistic data": an Approval Step's own
// fld_document is optional, so without this the seeded step pointed at no Document, every Document
// had no steps, and /review answered 404 -- the fixture reproducing the very bug this sweep exists
// to catch, which would have made it pass for the wrong reason.
func linkOptionalReferences(t *testing.T, ctx context.Context, store *data.Store, ws domain.Workspace, seeded map[string]string, actorID string) {
	t.Helper()
	for _, m := range ws.Machines {
		id, ok := seeded[m.ID]
		if !ok || id == actorID {
			continue // mch_user's record is the actor, owned by the surrounding fixture
		}
		record, err := store.GetRecord(ctx, m.ID, id)
		if err != nil {
			t.Fatalf("link %s: %v", m.ID, err)
		}
		changed := false
		for _, f := range m.Fields {
			if f.Required {
				continue // already filled by the first pass
			}
			if v, set := record.Values[f.ID]; set && v != nil && v != "" {
				continue
			}
			switch {
			case f.Type == domain.FieldTypePerson:
				record.Values[f.ID], changed = actorID, true
			case f.Type == domain.FieldTypeRelation:
				if target, ok := seeded[f.RelatedMachine]; ok {
					record.Values[f.ID], changed = target, true
				}
			}
		}
		if !changed {
			continue
		}
		if _, err := store.UpdateRecord(ctx, m.ID, id, record.Values); err != nil {
			t.Fatalf("link %s: %v", m.ID, err)
		}
	}
}

// seedValues fills a Machine's required Fields with something its own declaration accepts. ready is
// false when a required relation's target has not been seeded yet, which is what drives the ordering.
func seedValues(t *testing.T, files *storage.Store, m *domain.Machine, seeded map[string]string, actorID string) (map[string]any, bool) {
	t.Helper()
	values := map[string]any{}
	for _, f := range m.Fields {
		if !f.Required {
			continue
		}
		switch f.Type {
		case domain.FieldTypeText:
			values[f.ID] = "route sweep"
		case domain.FieldTypeNumber, domain.FieldTypeMoney:
			values[f.ID] = float64(1)
		case domain.FieldTypeBoolean:
			values[f.ID] = true
		case domain.FieldTypeDate:
			values[f.ID] = "2026-09-28"
		case domain.FieldTypeStatus:
			if len(f.Options) == 0 {
				return nil, false
			}
			values[f.ID] = f.Options[0]
		case domain.FieldTypePerson:
			values[f.ID] = actorID
		case domain.FieldTypeRelation:
			target, ok := seeded[f.RelatedMachine]
			if !ok {
				return nil, false
			}
			values[f.ID] = target
		case domain.FieldTypeFile:
			// A real stored file, not a made-up key: /pdf-preview rasterizes whatever is here, so a
			// placeholder string would turn a rendering bug into a fixture bug. mch_document's
			// required fld_file is why seeding failed until this existed.
			key, err := files.Save(m.ID, f.ID, "sweep.pdf", strings.NewReader(onePagePDF))
			if err != nil {
				t.Fatalf("seed file for %s.%s: %v", m.ID, f.ID, err)
			}
			values[f.ID] = key
		case domain.FieldTypeGroup:
			// A Group is platform data, not a record, and this seeder builds records. A Machine with a
			// *required* group Field would be unseedable here -- the loop above then names it rather
			// than skipping it silently, which is the report we would want.
			return nil, false
		}
	}
	return values, true
}

// newPerRecordSweepSetup is routerSetupParts plus the records the per-record routes need. See
// newRouterTestSetup for why the real installed Workspace is used rather than a stand-in.
func newPerRecordSweepSetup(t *testing.T, name string) (http.Handler, string, perRecordFixture) {
	t.Helper()
	h, cookie, ctx, store, files, ws, actorID := routerSetupParts(t, name)

	seeded := seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)

	fx := perRecordFixture{ws: ws, records: seeded, actorID: actorID, ctx: ctx, store: store}

	// The extras the role-gated routes need to be *valid*: a Document with a real PDF (pdf-preview
	// rasterizes it) and at least one Approval Step pointing at it (the Review screen opens on one).
	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc != nil {
		if key, err := files.Save(doc.ID, "fld_file", "sweep.pdf", strings.NewReader(onePagePDF)); err == nil {
			record, err := store.GetRecord(ctx, doc.ID, seeded[doc.ID])
			if err == nil {
				record.Values["fld_file"] = key
				if _, err := store.UpdateRecord(ctx, doc.ID, record.ID, record.Values); err != nil {
					t.Fatalf("attach seed PDF: %v", err)
				}
			}
			fx.uploaded = key
		}
	}

	// The Workspace row id comes off ctx rather than from another return value: it is already there,
	// put by the same scope every request carries (data.WithWorkspaceScope).
	workspaceID, _ := data.WorkspaceScope(ctx)

	// A second member, so the member-edit route is exercised as an admin editing *someone else* --
	// the real use of that screen, and the only way its read count means anything (see memberID).
	other, err := store.CreateRecord(ctx, domain.UserMachineID, map[string]any{})
	if err != nil {
		t.Fatalf("seed second member record: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, domain.UserMachineID, other.ID) })
	otherEmail := name + "-other@example.com"
	if err := store.AddMember(ctx, workspaceID, other.ID, otherEmail, "member", ""); err != nil {
		t.Fatalf("add second member: %v", err)
	}
	fx.memberID = other.ID
	if group, err := store.CreateGroup(ctx, workspaceID, "Sweep Group"); err == nil {
		fx.groupID = group.ID
		t.Cleanup(func() { _ = store.DeleteGroup(ctx, workspaceID, group.ID) })
	}
	return h, cookie, fx
}

// onePagePDF is the smallest thing internal/pdf will open -- enough for /pdf-preview to rasterize a
// page. Inline rather than a testdata file so this fixture has no second place to go wrong.
const onePagePDF = "%PDF-1.4\n" +
	"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
	"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
	"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>endobj\n" +
	"trailer<</Root 1 0 R>>\n"

// TestPostRoutesRefuseUnauthenticatedAndUnCSRFed is the write side's structural sweep: every POST in
// router.go's authenticated groups must refuse a request carrying no session, and refuse one carrying
// no CSRF token.
//
// **It is the third population this repo has measured rather than guessed at**, after the fixture
// mirrors and the per-record GET routes -- and the write side had never been measured at all. 14 of
// 29 POST routes are named by no test, and one of them (`revise`) is a declared-Action write path
// edited by the very commit that broke /review, by the same kind of edit, with nothing proving it.
//
// **What this sweep does not do, said here so a green run is not misread.** Refusing correctly is not
// behaviour coverage: these routes stay untested *as writes* after this test passes. What it holds is
// two things that fail **invisibly at runtime** -- the same reason TestQueryDiagnosticsRunsBeforeAuth
// exists:
//
//   - a route registered on the public router instead of pr/ar would serve anonymous writes, and look
//     completely normal to anyone signed in;
//   - csrfProtect is global today (router.go's own r.Use), so the CSRF half passes everywhere. Its
//     value is a future refactor that moves that middleware failing here rather than in production.
//
// Behaviour coverage for the three routes where a silent no-op matters most is separate and named:
// revise (revise_test.go), group role grants, and the password change.
func TestPostRoutesRefuseUnauthenticatedAndUnCSRFed(t *testing.T) {
	h, cookie, fx := newPerRecordSweepSetup(t, "postsweep")

	routes := postRoutesByGroup(t)
	if len(routes) < minSweptPostRoutes {
		t.Fatalf("found %d POST routes (want >= %d) -- the parse stopped seeing them and this sweep is measuring nothing",
			len(routes), minSweptPostRoutes)
	}

	paths := make([]string, 0, len(routes))
	for path := range routes {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, route := range paths {
		if routes[route] == "r" {
			switch {
			case preIdentityPostRoutes[route]:
				// Nothing to assert about identity: there is none yet by definition.
			case pendingIdentityPostRoutes[route]:
				// These run on the half-identity login creates before a Workspace is chosen, so the
				// claim "this is safe outside pr" is checkable: without that cookie they must send the
				// caller to /login rather than act. Asserting it is what makes this an allowlist with
				// a guarantee behind it instead of a way to be excused from the sweep.
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, fx.concrete(route), nil)
				token := csrfTokenFor(t, h, route)
				req.Header.Set("X-CSRF-Token", token.value)
				req.AddCookie(token.cookie)
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
					t.Errorf("POST %s with no pending identity = %d %q, want 303 to /login -- it is outside the authenticated group on the promise that it checks the pending identity itself",
						route, rec.Code, rec.Header().Get("Location"))
				}
			default:
				t.Errorf("POST %s is registered on the public router but is in neither declared set -- an authenticated write served anonymously looks completely normal to anyone signed in. Move it to pr/ar, or add it to one of the sets with a reason", route)
			}
			continue
		}
		url := fx.concrete(route)

		// No session at all. A CSRF token is supplied so the refusal can only be about identity --
		// otherwise csrfProtect would answer first and this would prove nothing about the route group.
		token := csrfTokenFor(t, h, route)
		anon := httptest.NewRequest(http.MethodPost, url, nil)
		anon.Header.Set("X-CSRF-Token", token.value)
		anon.AddCookie(token.cookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, anon)
		if rec.Code == http.StatusOK || rec.Code == http.StatusNoContent {
			t.Errorf("POST %s with no session = %d -- an authenticated route must refuse an anonymous write. Is it registered on the public router instead of pr/ar?",
				route, rec.Code)
		}

		// A real session, no CSRF token.
		noToken := httptest.NewRequest(http.MethodPost, url, nil)
		noToken.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, noToken)
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST %s with a session but no CSRF token = %d, want 403 -- csrfProtect must cover every write",
				route, rec.Code)
		}
	}
	// A stale allowlist entry protects nothing and hides that the route it named is gone -- the same
	// terms every ratchet in this repo carries.
	for _, set := range []map[string]bool{preIdentityPostRoutes, pendingIdentityPostRoutes} {
		for route := range set {
			if _, exists := routes[route]; !exists {
				t.Errorf("%s is allowlisted as a public POST but router.go registers no such route -- remove the entry", route)
			}
		}
	}
	t.Logf("swept %d POST routes for the two refusal invariants", len(routes))
}

// preIdentityPostRoutes and pendingIdentityPostRoutes are the two closed sets of writes that
// legitimately sit outside the authenticated group. Everything else there is a finding.
//
// Closed allowlists rather than a filter, for the same reason internal/conformance's
// runtimeLevelRoutes is one: the interesting failure is a route arriving here that nobody decided
// should be public, and a filter cannot notice that. A stale entry fails too (below), the same terms
// this repo's ratchets carry.
//
// **The split is a real distinction the sweep surfaced rather than one written in advance.** The first
// allowlist lumped them together as "pre-session"; /choose-workspace is not -- it runs after login, on
// the pending-email half-identity, and verifies it itself (authorization.PendingEmail). Writing them
// as one set would have recorded something false about two of them.
var preIdentityPostRoutes = map[string]bool{
	"/login":               true, // creates the session
	"/register":            true, // creates the identity
	"/resend-verification": true, // the identity exists but cannot sign in yet
	"/accept-invite":       true, // the invitation *is* the credential
	"/forgot-password":     true,
	"/reset-password":      true,
}

// pendingIdentityPostRoutes run between "signed in" and "Workspace chosen". Each is asserted to
// refuse without that pending identity, above.
var pendingIdentityPostRoutes = map[string]bool{
	"/choose-workspace":         true,
	"/choose-workspace/restore": true,
}

// minSweptPostRoutes is the floor, matching minSweptRoutes and minPerRecordCases: a parse that stops
// finding routes must fail rather than pass quietly.
const minSweptPostRoutes = 25

// concrete substitutes a seeded id for every path parameter, so a `{...}` POST is addressable rather
// than skipped -- the exclusion that left the GET routes unswept for months.
//
// The values only have to make the router match and the handler get far enough to refuse; this sweep
// never asserts a successful write, which is what lets one substitution table serve every route.
func (f perRecordFixture) concrete(route string) string {
	doc := f.ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	url := route
	if doc != nil {
		url = strings.ReplaceAll(url, "{machineID}", doc.ID)
		url = strings.ReplaceAll(url, "{id}", f.records[doc.ID])
	}
	url = strings.ReplaceAll(url, "{userRecordID}", f.memberID)
	url = strings.ReplaceAll(url, "{groupID}", f.groupID)
	url = strings.ReplaceAll(url, "{session}", "no-such-session")
	url = strings.ReplaceAll(url, "{slug}", "no-such-workspace")
	return url
}

// csrfTokenFor gets a real CSRF cookie/token pair out of the app itself, by making the GET any page
// makes. Minting one in the test would be testing this test's copy of the scheme rather than the
// app's.
type csrfPair struct {
	value  string
	cookie *http.Cookie
}

func csrfTokenFor(t *testing.T, h http.Handler, path string) csrfPair {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	for _, c := range rec.Result().Cookies() {
		if c.Name == authorization.CSRFCookieName {
			return csrfPair{value: c.Value, cookie: c}
		}
	}
	t.Fatalf("no CSRF cookie was issued for %s -- csrfProtect is not in the chain", path)
	return csrfPair{}
}

// TestPDFPreviewIsCachedByContentAndRevalidates is K19 through the real router: the second request for a page
// is answered from the cache (same bytes, same ETag), and a revalidating client gets 304 with no body.
func TestPDFPreviewIsCachedByContentAndRevalidates(t *testing.T) {
	h, cookie, fx := newPerRecordSweepSetup(t, "pdfcache")
	doc := fx.ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc == nil {
		t.Fatal("this Workspace casts no document role")
	}
	url := "/machines/" + doc.ID + "/records/" + fx.records[doc.ID] + "/pdf-preview?page=1"
	get := func(etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	first := get("")
	if first.Code != http.StatusOK {
		t.Fatalf("first preview = %d", first.Code)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("preview carries no ETag")
	}
	second := get("")
	if second.Code != http.StatusOK || second.Header().Get("ETag") != etag || !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Errorf("second preview differs from the first: %d, etag %q vs %q", second.Code, second.Header().Get("ETag"), etag)
	}
	revalidated := get(etag)
	if revalidated.Code != http.StatusNotModified || revalidated.Body.Len() != 0 {
		t.Errorf("revalidation = %d with %d body bytes, want 304 and none", revalidated.Code, revalidated.Body.Len())
	}
	if stale := get(`"not-the-etag"`); stale.Code != http.StatusOK {
		t.Errorf("a mismatched If-None-Match = %d, want a full 200", stale.Code)
	}
}

// TestReviewShowsTheSubmitterFromTheDocumentsFirstEvent is K18's behaviour: the Review screen reads the
// Document's earliest logged event through ds_record_first_events, not the whole activity log, and still
// names who submitted it.
func TestReviewShowsTheSubmitterFromTheDocumentsFirstEvent(t *testing.T) {
	h, cookie, fx := newPerRecordSweepSetup(t, "reviewsubmitter")
	doc := fx.ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	step := fx.ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleStep, "")
	if doc == nil || step == nil {
		t.Fatal("this Workspace casts no document/step role")
	}
	review := func() string {
		req := httptest.NewRequest(http.MethodGet, "/machines/"+step.ID+"/records/"+fx.records[step.ID]+"/review", nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("review = %d", rec.Code)
		}
		return rec.Body.String()
	}
	if strings.Contains(review(), "submitted by") {
		t.Fatal("the Document has no logged event yet but the screen names a submitter")
	}
	if _, err := fx.store.CreateRecord(fx.ctx, "mch_activity", map[string]any{
		"fld_machine_id": doc.ID, "fld_record_id": fx.records[doc.ID], "fld_summary": "submitted", "fld_actor": fx.actorID,
	}); err != nil {
		t.Fatalf("log the submission: %v", err)
	}
	if !strings.Contains(review(), "submitted by") {
		t.Error("the Review screen does not name the submitter after the Document's first event was logged")
	}
}
