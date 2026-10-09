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

// TestDeclaredPageRecordLinkOpensTheRecordsOwnFullPage: each listed title links to the runtime's route for that
// record, and that route -- which the page neither declares nor authorizes -- renders a **whole page** when
// followed (Tahap 0 of 2b: the handler's name was no proof it was not a fragment) and refuses a request with no
// session. The expected route is built from the seeded record's own ids, not read back out of the page.
func TestDeclaredPageRecordLinkOpensTheRecordsOwnFullPage(t *testing.T) {
	h, cookie, ctx, store, files, ws, actorID := routerSetupFor(t, "declaredrecordlink", "nana-2-workspace")
	seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)
	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc == nil {
		t.Fatal("nana-2-workspace casts no document role")
	}
	titleField := doc.CardFieldFor(domain.CardFieldRoleTitle)
	if titleField == "" {
		t.Fatal("the document Machine projects no title role")
	}
	rec, err := store.CreateRecord(ctx, doc.ID, map[string]any{titleField: "Record-link probe"})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, doc.ID, rec.ID) })

	route := "/machines/" + doc.ID + "/records/" + rec.ID
	body := getPage(t, h, cookie, "/pages/nav_documents_by_status")
	want := regexp.MustCompile(`<a href="` + regexp.QuoteMeta(route) + `"[^>]*>Record-link probe</a>`)
	if !want.MatchString(body) {
		t.Errorf("the title does not link to %s", route)
	}

	followed := getPage(t, h, cookie, route)
	if !strings.Contains(strings.ToLower(followed), "<html") || !strings.Contains(followed, "Record-link probe") {
		t.Errorf("following the link did not render a full page for the record")
	}

	anon := httptest.NewRecorder()
	h.ServeHTTP(anon, httptest.NewRequest(http.MethodGet, route, nil))
	if anon.Code == http.StatusOK || strings.Contains(anon.Body.String(), "Record-link probe") {
		t.Errorf("the linked route answered %d with no session, and must refuse (a link is an address, not a grant)", anon.Code)
	}
}

// TestDeclaredPageListOfLinkOpensTheMachinesOwnListPage: the page writes `list_of: <Dataset>` and some words, and
// the href must be the runtime's list page of the Machine that Dataset reads. The expected route is built from
// the loaded Dataset's own source, so the property under test is "the Dataset, not the YAML, names the Machine",
// and the route it points at must be one that renders and refuses an anonymous request.
func TestDeclaredPageListOfLinkOpensTheMachinesOwnListPage(t *testing.T) {
	h, cookie, _, _, _, ws, _ := routerSetupFor(t, "declaredlistof", "nana-2-workspace")
	var source string
	for _, m := range ws.Machines {
		for _, ds := range m.Datasets {
			if ds.ID == "ds_recent_documents" {
				source = ds.Source
			}
		}
	}
	if source == "" {
		t.Fatal("nana-2-workspace declares no ds_recent_documents")
	}
	route := domain.MachineListRoute(source)
	body := getPage(t, h, cookie, "/pages/nav_documents_by_status")
	want := regexp.MustCompile(`<a href="` + regexp.QuoteMeta(route) + `"[^>]*>All documents</a>`)
	if !want.MatchString(body) {
		t.Errorf("no link to %s reading %q reached the page", route, "All documents")
	}

	if followed := getPage(t, h, cookie, route); !strings.Contains(strings.ToLower(followed), "<html") {
		t.Error("following the link did not render a full page")
	}
	anon := httptest.NewRecorder()
	h.ServeHTTP(anon, httptest.NewRequest(http.MethodGet, route, nil))
	if anon.Code == http.StatusOK {
		t.Errorf("the linked route answered %d with no session, and must refuse (a link is an address, not a grant)", anon.Code)
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

// TestDeclaredPageSaysItsEmptyWordsOnlyWhenTheListIsEmpty: a fresh Workspace holds no documents, so the
// recent list is empty and must say what the page declared; once a document exists the words must go. The
// expected text is read out of the loaded navigation, so the property under test is "the page's own words
// reach the screen", not a literal.
func TestDeclaredPageSaysItsEmptyWordsOnlyWhenTheListIsEmpty(t *testing.T) {
	h, cookie, ctx, store, files, ws, actorID := routerSetupFor(t, "declaredempty", "nana-2-workspace")

	var words string
	var find func(n domain.PageNode)
	find = func(n domain.PageNode) {
		if n.Type == string(domain.ComponentCollection) && n.Props["empty"] != "" {
			words = n.Props["empty"]
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	for _, app := range ws.Applications {
		for _, item := range app.AllNavigation {
			if item.ID == "nav_documents_by_status" && item.Page != nil {
				find(*item.Page)
			}
		}
	}
	if words == "" {
		t.Fatal("nana-2-workspace's page declares no `empty:` on its Collection")
	}

	if body := getPage(t, h, cookie, "/pages/nav_documents_by_status"); !strings.Contains(body, words) {
		t.Errorf("an empty list did not say %q", words)
	}

	seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)
	if body := getPage(t, h, cookie, "/pages/nav_documents_by_status"); strings.Contains(body, words) {
		t.Errorf("the list has a document and still says %q", words)
	}
}

// TestDeclaredPageDrawsEachLabelAsATagInItsOwnColour: the page writes `from: {label: title, color: color}` and
// no Field, so the chip's words and its colour must be what the Label Machine's Projection says about each
// record. Two labels with different palette entries must draw different chips; a record whose colour is not a
// palette entry (data that arrived unchecked) still draws, in the renderer's neutral colour, rather than
// failing the page. The label Machine and its role fields are read from the loaded Workspace.
func TestDeclaredPageDrawsEachLabelAsATagInItsOwnColour(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredtag", "default")
	var label *domain.Machine
	for _, m := range ws.Machines {
		if m.CardFieldFor(domain.CardFieldRoleColor) != "" {
			label = m
		}
	}
	if label == nil {
		t.Fatal("default installs no Machine projecting a color role")
	}
	titleField, colorField := label.CardFieldFor(domain.CardFieldRoleTitle), label.CardFieldFor(domain.CardFieldRoleColor)
	for name, colour := range map[string]string{"Tag-probe-blue": string(domain.TagBlue), "Tag-probe-rose": string(domain.TagRose), "Tag-probe-odd": "chartreuse"} {
		rec, err := store.CreateRecord(ctx, label.ID, map[string]any{titleField: name, colorField: colour})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, label.ID, rec.ID) })
	}

	body := getPage(t, h, cookie, "/pages/nav_board_settings")
	chip := func(name string) string {
		m := regexp.MustCompile(`<span class="inline-flex[^"]*"><span class="[^"]*" aria-hidden="true"></span>\s*` + name + `\s*</span>`).FindString(body)
		if m == "" {
			t.Fatalf("no Tag reading %q reached the page", name)
		}
		return m
	}
	blue, rose, odd := chip("Tag-probe-blue"), chip("Tag-probe-rose"), chip("Tag-probe-odd")
	if !strings.Contains(blue, "text-blue-600") || !strings.Contains(rose, "text-rose-700") {
		t.Errorf("a label did not draw in its own palette colour:\n%s\n%s", blue, rose)
	}
	if !strings.Contains(odd, "text-slate-600") {
		t.Errorf("a colour outside the palette must fall back to the neutral chip:\n%s", odd)
	}
}

// TestDeclaredPageNumbersTheBoardListsInColumnOrder: `ordered: true` on a Collection draws an <ol> whose
// position numbers are counted by the renderer. The page writes no number, so the proof is that three Lists
// created in a known order appear in that order and that the numbers across the whole page run 1..N without a
// gap -- whatever else the dev database holds.
func TestDeclaredPageNumbersTheBoardListsInColumnOrder(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredordered", "default")
	var list *domain.Machine
	for _, m := range ws.Machines {
		for _, ds := range m.Datasets {
			if ds.ID == "ds_board_lists" {
				list = m
			}
		}
	}
	if list == nil {
		t.Fatal("default installs no Machine providing ds_board_lists")
	}
	titleField := list.CardFieldFor(domain.CardFieldRoleTitle)
	names := []string{"Ord-probe-first", "Ord-probe-second", "Ord-probe-third"}
	for _, name := range names {
		rec, err := store.CreateRecord(ctx, list.ID, map[string]any{titleField: name})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, list.ID, rec.ID) })
	}

	body := getPage(t, h, cookie, "/pages/nav_board_settings")
	if !strings.Contains(body, `<ol class="m-0 list-none overflow-hidden p-0 rounded-lg`) {
		t.Fatalf("the Collection did not draw an <ol>:\n%s", body)
	}
	nums := regexp.MustCompile(`aria-hidden="true">(\d+)</span>`).FindAllStringSubmatch(body, -1)
	for i, m := range nums {
		if m[1] != strconv.Itoa(i+1) {
			t.Fatalf("position numbers must run 1..N in order; item %d is numbered %s", i+1, m[1])
		}
	}
	last := -1
	for _, name := range names {
		at := strings.Index(body, name)
		if at < 0 {
			t.Fatalf("list %q did not reach the page", name)
		}
		if at < last {
			t.Errorf("list %q appears before an earlier-created one; the order is the Dataset's", name)
		}
		last = at
	}
}

// TestDeclaredPageSaysHowManyCardsCarryEachLabel: `count:` over a declared Relation. Two Labels are created,
// one carried by one card and one by two, and the page must say "used on 1 card" and "used on 2 cards" --
// the singular and the plural both, from one declaration that writes no number.
func TestDeclaredPageSaysHowManyCardsCarryEachLabel(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredcount", "default")
	byID := map[string]*domain.Machine{}
	for _, m := range ws.Machines {
		byID[m.ID] = m
	}
	var label, join *domain.Machine
	for _, m := range ws.Machines {
		for _, ds := range m.Datasets {
			if ds.ID == "ds_labels_with_cards" {
				label = m
				for _, rel := range ds.Relations {
					join = byID[rel.Machine]
				}
			}
		}
	}
	if label == nil || join == nil {
		t.Fatal("default installs no Machine providing ds_labels_with_cards")
	}
	titleField := label.CardFieldFor(domain.CardFieldRoleTitle)
	var viaField string
	for _, f := range join.Fields {
		if f.IsReference() && f.RelatedMachine == label.ID {
			viaField = f.ID
		}
	}
	if viaField == "" {
		t.Fatalf("%s has no reference to %s", join.ID, label.ID)
	}
	for name, cards := range map[string]int{"Count-probe-one": 1, "Count-probe-two": 2} {
		rec, err := store.CreateRecord(ctx, label.ID, map[string]any{titleField: name})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, label.ID, rec.ID) })
		for i := 0; i < cards; i++ {
			pair, err := store.CreateRecord(ctx, join.ID, map[string]any{viaField: rec.ID})
			if err != nil {
				t.Fatalf("CreateRecord pair: %v", err)
			}
			t.Cleanup(func() { _ = store.DeleteRecord(ctx, join.ID, pair.ID) })
		}
	}

	body := getPage(t, h, cookie, "/pages/nav_board_settings")
	after := func(name string) string {
		i := strings.Index(body, name)
		if i < 0 {
			t.Fatalf("label %q did not reach the page", name)
		}
		rest := body[i:]
		if j := strings.Index(rest, "</li>"); j >= 0 {
			rest = rest[:j]
		}
		return rest
	}
	if got := after("Count-probe-one"); !strings.Contains(got, "used on 1 card<") {
		t.Errorf("one card must read the singular wording:\n%s", got)
	}
	if got := after("Count-probe-two"); !strings.Contains(got, "used on 2 cards<") {
		t.Errorf("two cards must read the plural wording:\n%s", got)
	}
}
