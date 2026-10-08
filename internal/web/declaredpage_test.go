package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// TestDeclaredPageRendersItsBindingAsRows serves `/pages/nav_documents_by_status` end to end in a Workspace
// that declares it (nana-2-workspace; `default` declares no page, which is why the per-record sweep can only
// hold the refusals). The page is a Metric bound to ds_document_by_status's Dimension, so what must reach the
// response is **one Metric per declared status, with a count that matches the records**, and not a number any
// file typed.
func TestDeclaredPageRendersItsBindingAsRows(t *testing.T) {
	h, cookie, ctx, store, files, ws, actorID := routerSetupFor(t, "declaredpage", "nana-2-workspace")
	seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)

	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc == nil {
		t.Fatal("nana-2-workspace casts no document role")
	}
	var statusField *domain.Field
	for i := range doc.Fields {
		if doc.Fields[i].Type == domain.FieldTypeStatus {
			statusField = &doc.Fields[i]
		}
	}
	if statusField == nil || len(statusField.Options) < 2 {
		t.Fatal("the document Machine declares no status Field with options")
	}

	want := countByStatus(t, ctx, store, doc.ID, statusField.ID)
	body := getPage(t, h, cookie, "/pages/nav_documents_by_status")

	// The shared Metric draws the value in one div and the label in the next; a declared option with no
	// record must appear too, as an explicit 0.
	for _, option := range statusField.Options {
		pair := regexp.MustCompile(`>` + strconv.Itoa(want[option]) + `</div><div[^>]*>` + regexp.QuoteMeta(option) + `</div>`)
		if !pair.MatchString(body) {
			t.Errorf("no Metric shows %q with a value of %d (the records say %d)", option, want[option], want[option])
		}
	}
	if !strings.Contains(body, "Documents by status") {
		t.Error("the heading comes from the navigation item's title: and is missing")
	}
}

// TestDeclaredPageQueryCostIsFlatInRecordCount: same request, more rows, same cost. A bound Metric expands
// into one node per Dimension value, and the shape that would break this is a per-row read. Query cost is
// the number of distinct Datasets, because the Loader memoises within a request.
func TestDeclaredPageQueryCostIsFlatInRecordCount(t *testing.T) {
	h, cookie, ctx, store, files, ws, actorID := routerSetupFor(t, "declaredcost", "nana-2-workspace")
	seeded := seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)

	measure := func() int {
		req := httptest.NewRequest(http.MethodGet, "/pages/nav_documents_by_status", nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		queries, reads, repeated, line := serveAndCount(t, h, req)
		if queries != reads || repeated != 0 {
			t.Errorf("queries=%d reads=%d repeated=%d -- every statement must name itself and none may repeat\n  %s", queries, reads, repeated, line)
		}
		return queries
	}
	before := measure()

	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	template, err := store.GetRecord(ctx, doc.ID, seeded[doc.ID])
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	for i := 0; i < 5; i++ {
		rec, err := store.CreateRecord(ctx, doc.ID, template.Values)
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}
		id := rec.ID
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, doc.ID, id) })
	}
	if after := measure(); after != before {
		t.Errorf("the page cost %d statements with 1 document and %d with 6 -- a read per row", before, after)
	}
}

// TestDeclaredPageListsRecordsFromTheMachinesProjection: the Collection bound with `rows: records` shows each
// record's title as the Machine's own Projection resolves it, and the newest first. Titles are read out of the
// records through the Machine's `card_fields`, so no Field id appears here either.
func TestDeclaredPageListsRecordsFromTheMachinesProjection(t *testing.T) {
	h, cookie, ctx, store, files, ws, actorID := routerSetupFor(t, "declaredlist", "nana-2-workspace")
	seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)

	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc == nil {
		t.Fatal("nana-2-workspace casts no document role")
	}
	var titleField string
	for _, cf := range doc.CardFields {
		if cf.Role == domain.CardFieldRoleTitle {
			titleField = cf.Field
		}
	}
	if titleField == "" {
		t.Fatal("the document Machine projects no title role")
	}
	rec, err := store.CreateRecord(ctx, doc.ID, map[string]any{titleField: "Zeta recent-list probe"})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, doc.ID, rec.ID) })

	body := getPage(t, h, cookie, "/pages/nav_documents_by_status")
	if !strings.Contains(body, "Zeta recent-list probe") {
		t.Error("the newest document's title, resolved through the Machine's Projection, is not in the list")
	}
	if !strings.Contains(body, "Recent documents") {
		t.Error("the page's declared `static: subheading` did not reach the screen")
	}
}

// TestDeclaredPageLinkComesFromNavigationNotFromTheYAML: the page writes `to: nav_approval_inbox` and nothing
// else, so the href and the words must be what the Application's own navigation declares. Expected values are
// read out of the loaded navigation rather than typed, which is the property under test.
func TestDeclaredPageLinkComesFromNavigationNotFromTheYAML(t *testing.T) {
	h, cookie, _, _, _, ws, _ := routerSetupFor(t, "declaredlink", "nana-2-workspace")
	var route, label string
	for _, app := range ws.Applications {
		for _, item := range app.AllNavigation {
			if item.ID == "nav_approval_inbox" {
				route, label = item.Route, item.Label
			}
		}
	}
	if route == "" || label == "" {
		t.Fatal("nana-2-workspace declares no nav_approval_inbox with a route and a label")
	}
	body := getPage(t, h, cookie, "/pages/nav_documents_by_status")
	want := regexp.MustCompile(`<a href="` + regexp.QuoteMeta(route) + `"[^>]*>` + regexp.QuoteMeta(label) + `</a>`)
	if !want.MatchString(body) {
		t.Errorf("no link to %s reading %q reached the page", route, label)
	}
}

func getPage(t *testing.T, h http.Handler, cookie, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200\n%s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func countByStatus(t *testing.T, ctx context.Context, store *data.Store, machineID, fieldID string) map[string]int {
	t.Helper()
	records, err := store.ListRecords(ctx, machineID)
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	out := map[string]int{}
	for _, r := range records {
		if v, ok := r.Values[fieldID].(string); ok {
			out[v]++
		}
	}
	return out
}
