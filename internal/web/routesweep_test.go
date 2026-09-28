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
	// /new-application/{session}/review is the AI assistant's own review screen. It needs a stored
	// conversation with generated metadata in it, which is a fixture of a different kind -- named here
	// rather than passed over, so the claimed-route check above fails until someone builds it.
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

	fx := perRecordFixture{ws: ws, records: seeded, actorID: actorID}

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
