package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
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

// TestDeclaredPageNamesThePersonARecordBelongsTo: a records template takes `text` from the `person` role, and
// the Field behind it is a reference to a member. The page draws the member's display name, never the stored id,
// and a person who is no longer a member is drawn as nothing rather than as their user id.
func TestDeclaredPageNamesThePersonARecordBelongsTo(t *testing.T) {
	h, cookie, ctx, store, files, ws, actorID := routerSetupFor(t, "declaredperson", "nana-2-workspace")
	seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)

	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc == nil {
		t.Fatal("nana-2-workspace casts no document role")
	}
	titleField, personField := doc.CardFieldFor(domain.CardFieldRoleTitle), doc.CardFieldFor(domain.CardFieldRolePerson)
	if titleField == "" || personField == "" {
		t.Fatalf("the Document Machine projects no title/person role (title=%q person=%q)", titleField, personField)
	}
	const goneID = "usr_no_longer_a_member"
	for title, who := range map[string]string{"Personprobe by a member": actorID, "Personprobe by a stranger": goneID} {
		rec, err := store.CreateRecord(ctx, doc.ID, map[string]any{titleField: title, personField: who})
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, doc.ID, rec.ID) })
	}

	body := getPage(t, h, cookie, "/pages/nav_documents_by_status")
	if !strings.Contains(body, "declaredperson@example.com") {
		t.Error("the member's display name is not on the page")
	}
	if strings.Contains(body, actorID) && strings.Contains(strings.ReplaceAll(body, "/records/", ""), ">"+actorID+"<") {
		t.Error("the member's user id is printed where their name belongs")
	}
	if strings.Contains(body, goneID) {
		t.Error("a user id that names no member was printed")
	}
}

// TestDeclaredPagePastDueListsOnlyWhatIsInReviewAndLate: the "Past due" section is a Dataset whose `where:`
// compares the due date with `$today`. A document in review with an earlier date is listed; the same date on an
// approved one, a future date, today's own date and a document with no date at all are not. The dates are
// built from the clock the handler reads, so the test states the property and not a calendar day.
func TestDeclaredPagePastDueListsOnlyWhatIsInReviewAndLate(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredpastdue", "nana-2-workspace")
	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc == nil || doc.SLAField == "" || doc.StatusField() == "" {
		t.Fatal("nana-2-workspace casts no document role with a due date and a status")
	}
	titleField := doc.CardFieldFor(domain.CardFieldRoleTitle)
	day := func(offset int) string { return time.Now().AddDate(0, 0, offset).Format("2006-01-02") }
	for title, v := range map[string]map[string]any{
		"Pastdue-late-in-review": {doc.SLAField: day(-3), doc.StatusField(): "in_review"},
		"Pastdue-late-approved":  {doc.SLAField: day(-3), doc.StatusField(): "approved"},
		"Pastdue-future":         {doc.SLAField: day(3), doc.StatusField(): "in_review"},
		"Pastdue-today":          {doc.SLAField: day(0), doc.StatusField(): "in_review"},
		"Pastdue-no-date":        {doc.StatusField(): "in_review"},
	} {
		v[titleField] = title
		rec, err := store.CreateRecord(ctx, doc.ID, v)
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, doc.ID, rec.ID) })
	}

	body := getPage(t, h, cookie, "/pages/nav_documents_by_status")
	i := strings.Index(body, "Past due")
	if i < 0 {
		t.Fatal("the page has no Past due section")
	}
	section := body[i:]
	// The recent list above also holds these titles; only what follows the heading is the past-due list.
	if end := strings.Index(section, "Recent documents"); end > 0 {
		t.Fatal("the Past due section must follow the recent list in the page's order")
	}
	if !strings.Contains(section, "Pastdue-late-in-review") {
		t.Error("a document in review with a past due date is not listed")
	}
	for _, not := range []string{"Pastdue-late-approved", "Pastdue-future", "Pastdue-today", "Pastdue-no-date"} {
		if strings.Contains(section, not) {
			t.Errorf("%s is listed as past due", not)
		}
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
		// The first Collection with words is the recent list; a later one (past due) has its own.
		if n.Type == string(domain.ComponentCollection) && n.Props["empty"] != "" && words == "" {
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
	// A Tag's hue is a property since dark mode (DK8); its value is held by rendering's GS1 and dark contrast gates.
	if !strings.Contains(blue, "text-tag-blue") || !strings.Contains(rose, "text-tag-rose") {
		t.Errorf("a label did not draw in its own palette colour:\n%s\n%s", blue, rose)
	}
	if !strings.Contains(odd, "text-tag-slate") {
		t.Errorf("a colour outside the palette must fall back to the neutral chip:\n%s", odd)
	}
	// The colour is a status no Transition names, so each label's update form asks for it (domain.EditFormInputs)
	// as well as the one create form: at least one select per probe label plus the create form's.
	if n := len(regexp.MustCompile(`<select[^>]*name="`+colorField+`"`).FindAllString(body, -1)); n < 4 {
		t.Errorf("%d colour selects, want the create form's plus one per label's update form (>= 4)", n)
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

// TestDeclaredPageTakesItsFilterFromTheQueryString serves `/pages/nav_documents_in_status` end to end: the
// request's `?status=` is the Dataset's `$parameters.status`, so only that status's Documents are listed; a
// request with no status lists none (fail closed, 007 §9.2) and is a 200 rather than a fault; and the value is
// compared as data. The Field and its values are read out of the Dataset's own `where:` and the Machine's
// options, so the test names neither.
func TestDeclaredPageTakesItsFilterFromTheQueryString(t *testing.T) {
	h, cookie, ctx, store, files, ws, actorID := routerSetupFor(t, "declaredparam", "nana-2-workspace")
	seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)

	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc == nil {
		t.Fatal("nana-2-workspace casts no document role")
	}
	var statusField, titleField string
	for _, ds := range doc.Datasets {
		if ds.ID == "ds_documents_in_status" {
			statusField = ds.Where.Comparisons()[0].Field
		}
	}
	for _, cf := range doc.CardFields {
		if cf.Role == domain.CardFieldRoleTitle {
			titleField = cf.Field
		}
	}
	f, ok := doc.FieldByID(statusField)
	if statusField == "" || titleField == "" || !ok || len(f.Options) < 2 {
		t.Fatalf("the Document Machine does not offer a two-valued status filter (status=%q title=%q)", statusField, titleField)
	}
	for i, title := range []string{"Paramprobe in first status", "Paramprobe in second status"} {
		rec, err := store.CreateRecord(ctx, doc.ID, map[string]any{titleField: title, statusField: f.Options[i]})
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, doc.ID, rec.ID) })
	}

	body := getPage(t, h, cookie, "/pages/nav_documents_in_status?status="+url.QueryEscape(f.Options[0]))
	if !strings.Contains(body, "Paramprobe in first status") || strings.Contains(body, "Paramprobe in second status") {
		t.Errorf("?status=%s must list that status's Document and no other", f.Options[0])
	}
	body = getPage(t, h, cookie, "/pages/nav_documents_in_status")
	if strings.Contains(body, "Paramprobe") {
		t.Error("a request naming no status listed Documents; it must list none")
	}
	body = getPage(t, h, cookie, "/pages/nav_documents_in_status?status="+url.QueryEscape("x' OR '1'='1"))
	if strings.Contains(body, "Paramprobe") {
		t.Error("a status that is not one listed Documents")
	}
}

// TestDeclaredPageStatusTilesOpenTheDocumentsInThatStatus: the overview's Metrics link to the filtered page with
// the row's own status as ?status=, and following one lists exactly the Documents in it -- the two ends of
// `$parameters.status` joined through a real request, which neither page's own test can show.
func TestDeclaredPageStatusTilesOpenTheDocumentsInThatStatus(t *testing.T) {
	h, cookie, ctx, store, files, ws, actorID := routerSetupFor(t, "declaredtile", "nana-2-workspace")
	seedRecordForEveryMachine(t, ctx, store, files, ws, actorID)

	doc := ws.MachineInWorkflowRole(domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument, "")
	if doc == nil {
		t.Fatal("nana-2-workspace casts no document role")
	}
	var statusField, titleField string
	for _, ds := range doc.Datasets {
		if ds.ID == "ds_documents_in_status" {
			statusField = ds.Where.Comparisons()[0].Field
		}
	}
	for _, cf := range doc.CardFields {
		if cf.Role == domain.CardFieldRoleTitle {
			titleField = cf.Field
		}
	}
	f, ok := doc.FieldByID(statusField)
	if statusField == "" || titleField == "" || !ok || len(f.Options) < 2 {
		t.Fatalf("the Document Machine does not offer a two-valued status filter (status=%q title=%q)", statusField, titleField)
	}
	for i, title := range []string{"Tileprobe in first status", "Tileprobe in second status"} {
		rec, err := store.CreateRecord(ctx, doc.ID, map[string]any{titleField: title, statusField: f.Options[i]})
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, doc.ID, rec.ID) })
	}

	overview := getPage(t, h, cookie, "/pages/nav_documents_by_status")
	for _, option := range f.Options {
		href := `href="/pages/nav_documents_in_status?status=` + url.QueryEscape(option) + `"`
		if !strings.Contains(overview, href) {
			t.Errorf("no status tile links to %s", href)
		}
	}
	followed := getPage(t, h, cookie, "/pages/nav_documents_in_status?status="+url.QueryEscape(f.Options[1]))
	if !strings.Contains(followed, "Tileprobe in second status") || strings.Contains(followed, "Tileprobe in first status") {
		t.Errorf("following the %q tile must list that status's Document and no other", f.Options[1])
	}
}

// TestDeclaredPageFormCreatesARecordThroughTheGenericRoute: `component: Form` with `write: create` is the
// write side of a Binding (007 §11.3). The page writes a submit label and a Dataset; the route it posts to, each
// control's `name=`, the Color select's options and its default all come from the Label Machine. Posting it is
// the *existing* create route -- no handler was added for pages -- so what is proved here is that the form the
// page drew is one that route accepts, answers `HX-Refresh` for (a screen other than the Machine's own), and
// refuses when a required Field is missing.
func TestDeclaredPageFormCreatesARecordThroughTheGenericRoute(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredform", "default")
	var label *domain.Machine
	for _, m := range ws.Machines {
		for _, ds := range m.Datasets {
			if ds.ID == "ds_board_labels" {
				label = m
			}
		}
	}
	if label == nil {
		t.Fatal("default installs no Machine providing ds_board_labels")
	}
	nameField := label.CardFieldFor(domain.CardFieldRoleTitle)
	colorField := label.CardFieldFor(domain.CardFieldRoleColor)
	colour, _ := label.FieldByID(colorField)

	body := getPage(t, h, cookie, "/pages/nav_board_settings")
	action := domain.FormRoute(label.ID)
	if !strings.Contains(body, `hx-post="`+action+`"`) {
		t.Fatalf("no form posts to the Label Machine's create route %s", action)
	}
	if !regexp.MustCompile(`<input id="[^"]+" type="text" name="` + nameField + `" value="" required`).MatchString(body) {
		t.Errorf("the required text Field did not become a required text input named by the Field id")
	}
	sel := regexp.MustCompile(`(?s)<select id="[^"]+" name="` + colorField + `" required.*?</select>`).FindString(body)
	if sel == "" {
		t.Fatalf("the status Field did not become a required select")
	}
	for _, opt := range colour.Options {
		if !strings.Contains(sel, `value="`+opt+`"`) {
			t.Errorf("select lacks the Field's option %q", opt)
		}
	}
	if !strings.Contains(sel, `value="`+colour.Default.(string)+`" selected`) {
		t.Errorf("the Field's default %v is not the selected option", colour.Default)
	}

	post := func(vals url.Values) *httptest.ResponseRecorder {
		tok := csrfTokenFor(t, h, "/login")
		req := httptest.NewRequest(http.MethodPost, action, strings.NewReader(vals.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", "http://x/pages/nav_board_settings")
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	const made = "Form-probe-label"
	t.Cleanup(func() {
		recs, _ := store.ListRecords(ctx, label.ID)
		for _, r := range recs {
			if r.Values[nameField] == made {
				_ = store.DeleteRecord(ctx, label.ID, r.ID)
			}
		}
	})
	refused := post(url.Values{nameField: {""}, colorField: {"blue"}})
	if refused.Code != http.StatusUnprocessableEntity {
		t.Errorf("an empty required Field answered %d, want 422", refused.Code)
	}
	ok := post(url.Values{nameField: {made}, colorField: {"blue"}})
	if ok.Code != http.StatusOK || ok.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("a valid form answered %d with HX-Refresh=%q", ok.Code, ok.Header().Get("HX-Refresh"))
	}
	if after := getPage(t, h, cookie, "/pages/nav_board_settings"); !strings.Contains(after, made) {
		t.Error("the created label does not appear on the page after the refresh")
	}
}

// TestDeclaredPageRenameFormPatchesTheRecordItSitsBesideAndStartsAsItsName: `write: update` inside a records
// template. Two Lists are created; each item must carry a form that PATCHes *its own* record's generic route and
// starts as *its own* name, the patch must change that record and only that one, and the route's own refusal
// (an empty required Field) must still hold -- the page declared a form, it did not become the write path.
func TestDeclaredPageRenameFormPatchesTheRecordItSitsBesideAndStartsAsItsName(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredrename", "default")
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
	nameField := list.CardFieldFor(domain.CardFieldRoleTitle)
	mk := func(name string) *data.Record {
		rec, err := store.CreateRecord(ctx, list.ID, map[string]any{nameField: name})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, list.ID, rec.ID) })
		return rec
	}
	first, second := mk("Rename-probe-first"), mk("Rename-probe-second")

	body := getPage(t, h, cookie, "/pages/nav_board_settings")
	for _, rec := range []*data.Record{first, second} {
		route := "/machines/" + list.ID + "/records/" + rec.ID
		re := regexp.MustCompile(`(?s)<form hx-patch="` + regexp.QuoteMeta(route) + `".*?</form>`)
		form := re.FindString(body)
		if form == "" {
			t.Fatalf("no form patches %s", route)
		}
		if !strings.Contains(form, `name="`+nameField+`" value="`+rec.Values[nameField].(string)+`" required`) {
			t.Errorf("the form for %s does not start as its own name:\n%s", rec.ID, form)
		}
		if strings.Contains(form, "hx-post") {
			t.Errorf("an update form must not also post: %s", form)
		}
	}

	patch := func(id string, vals url.Values) *httptest.ResponseRecorder {
		tok := csrfTokenFor(t, h, "/login")
		req := httptest.NewRequest(http.MethodPatch, "/machines/"+list.ID+"/records/"+id, strings.NewReader(vals.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", "http://x/pages/nav_board_settings")
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if refused := patch(first.ID, url.Values{nameField: {""}}); refused.Code != http.StatusUnprocessableEntity {
		t.Errorf("an emptied required Field answered %d, want 422", refused.Code)
	}
	ok := patch(first.ID, url.Values{nameField: {"Rename-probe-renamed"}})
	if ok.Code != http.StatusOK || ok.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("a valid rename answered %d with HX-Refresh=%q", ok.Code, ok.Header().Get("HX-Refresh"))
	}
	after := getPage(t, h, cookie, "/pages/nav_board_settings")
	if !strings.Contains(after, "Rename-probe-renamed") {
		t.Error("the new name does not appear after the refresh")
	}
	if !strings.Contains(after, "Rename-probe-second") {
		t.Error("renaming one list changed another")
	}
}

// TestDeclaredPageDeleteButtonDeletesTheRecordItSitsBesideAndRefreshesThePage: `write: delete` inside a records
// template. Each item carries a button that sends DELETE to *its own* record's generic route, asking the author's
// `confirm:` sentence first; the route (not the page) deletes, and answers `HX-Refresh: true` to a request sent
// from a declared page so the list re-reads. A request from the Machine's own page keeps the empty body the three
// hand-written delete sites rely on.
func TestDeclaredPageDeleteButtonDeletesTheRecordItSitsBesideAndRefreshesThePage(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declareddelete", "default")
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
	nameField := list.CardFieldFor(domain.CardFieldRoleTitle)
	mk := func(name string) *data.Record {
		rec, err := store.CreateRecord(ctx, list.ID, map[string]any{nameField: name})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, list.ID, rec.ID) })
		return rec
	}
	first, second := mk("Delete-probe-first"), mk("Delete-probe-second")

	body := getPage(t, h, cookie, "/pages/nav_board_settings")
	for _, rec := range []*data.Record{first, second} {
		route := "/machines/" + list.ID + "/records/" + rec.ID
		re := regexp.MustCompile(`<button[^>]*hx-delete="` + regexp.QuoteMeta(route) + `"[^>]*>[^<]*</button>`)
		btn := re.FindString(body)
		if btn == "" {
			t.Fatalf("no button deletes %s", route)
		}
		if !strings.Contains(btn, `hx-confirm="Delete this list? This cannot be undone."`) || !strings.Contains(btn, `type="button"`) || !strings.Contains(btn, `hx-swap="none"`) {
			t.Errorf("the delete button is not the confirmed request the page declared: %s", btn)
		}
	}

	del := func(id, from string) *httptest.ResponseRecorder {
		tok := csrfTokenFor(t, h, "/login")
		req := httptest.NewRequest(http.MethodDelete, "/machines/"+list.ID+"/records/"+id, nil)
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", from)
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	ok := del(first.ID, "http://x/pages/nav_board_settings")
	if ok.Code != http.StatusOK || ok.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("a delete from the declared page answered %d with HX-Refresh=%q", ok.Code, ok.Header().Get("HX-Refresh"))
	}
	after := getPage(t, h, cookie, "/pages/nav_board_settings")
	if strings.Contains(after, "Delete-probe-first") {
		t.Error("the deleted list still appears after the refresh")
	}
	if !strings.Contains(after, "Delete-probe-second") {
		t.Error("deleting one list removed another")
	}
	own := del(second.ID, "http://x/machines/"+list.ID)
	if own.Code != http.StatusOK || own.Header().Get("HX-Refresh") != "" || own.Body.Len() != 0 {
		t.Errorf("a delete from the Machine's own page answered %d, HX-Refresh=%q, %d body bytes -- the hand-written sites' contract moved", own.Code, own.Header().Get("HX-Refresh"), own.Body.Len())
	}
}

func TestMoveTarget(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	cases := []struct {
		id, dir, before      string
		wantFound, wantMoves bool
	}{
		{"c", "up", "b", true, true},
		{"a", "up", "", true, false},
		{"b", "down", "d", true, true},
		{"c", "down", "", true, true},
		{"d", "down", "", true, false},
		{"zz", "up", "", false, false},
	}
	for _, tc := range cases {
		before, found, moves := moveTarget(ids, tc.id, tc.dir)
		if before != tc.before || found != tc.wantFound || moves != tc.wantMoves {
			t.Errorf("moveTarget(%s,%s) = %q,%v,%v; want %q,%v,%v", tc.id, tc.dir, before, found, moves, tc.before, tc.wantFound, tc.wantMoves)
		}
	}
}

// TestDeclaredPageMoveButtonsReorderTheListAndRefreshThePage: `write: move` inside a records template. The first
// list has no "Move up" and the last no "Move down"; the route finds the neighbour itself, so the page names only
// a direction; the answer to a request from a declared page is HX-Refresh, and the order the page redraws is the
// order the board's columns follow (both read the Machine's `sort_order`).
func TestDeclaredPageMoveButtonsReorderTheListAndRefreshThePage(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredmove", "default")
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
	existing, err := store.ListRecords(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	nameField := list.CardFieldFor(domain.CardFieldRoleTitle)
	mk := func(name string) *data.Record {
		rec, err := store.CreateRecord(ctx, list.ID, map[string]any{nameField: name})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, list.ID, rec.ID) })
		return rec
	}
	first, second := mk("Move-probe-first"), mk("Move-probe-second")
	route := func(rec *data.Record, dir string) string {
		return "/machines/" + list.ID + "/records/" + rec.ID + "/move?direction=" + dir
	}

	body := getPage(t, h, cookie, "/pages/nav_board_settings")
	if !strings.Contains(body, `hx-post="`+route(first, "down")+`"`) || !strings.Contains(body, `hx-post="`+route(second, "up")+`"`) {
		t.Fatal("the page draws no Move buttons for the records it lists")
	}
	if strings.Contains(body, `hx-post="`+route(second, "down")+`"`) {
		t.Error("the last list is offered Move down")
	}
	if len(existing) == 0 && strings.Contains(body, `hx-post="`+route(first, "up")+`"`) {
		t.Error("the first list is offered Move up")
	}

	post := func(path string) *httptest.ResponseRecorder {
		tok := csrfTokenFor(t, h, "/login")
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", "http://x/pages/nav_board_settings")
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if got := post(route(second, "up")); got.Code != http.StatusOK || got.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("a move from the declared page answered %d with HX-Refresh=%q", got.Code, got.Header().Get("HX-Refresh"))
	}
	after := getPage(t, h, cookie, "/pages/nav_board_settings")
	if strings.Index(after, "Move-probe-second") > strings.Index(after, "Move-probe-first") {
		t.Error("moving the second list up did not put it before the first")
	}
	// Past the end: nothing moves, and the answer is still a refresh so a stale page redraws.
	if got := post(route(second, "down")); got.Code != http.StatusOK || got.Header().Get("HX-Refresh") != "true" {
		t.Errorf("a move past the end answered %d with HX-Refresh=%q", got.Code, got.Header().Get("HX-Refresh"))
	}
	if got := post(route(first, "sideways")); got.Code != http.StatusBadRequest {
		t.Errorf("a direction that is neither up nor down answered %d, want 400", got.Code)
	}
	if got := post("/machines/" + list.ID + "/records/rec_nope/move?direction=up"); got.Code != http.StatusNotFound {
		t.Errorf("a move of a record that does not exist answered %d, want 404", got.Code)
	}
}

// TestDeclaredPageTotalsCountEachFigureFromItsMeasure: "Task figures" is six Metrics bound with `rows: total`,
// and each figure is a Measure's `where:` over the Task Machine -- open is "not $done", overdue is "not $done
// AND due before $today", completed is "$done". Eight tasks sit on either side of every boundary (a finished one
// that is late, a late open one, one due today, one in the future, one undated, and the week's two edges and a
// finished one due today), so a figure that treated
// "finished" as a literal, dropped the conjunction, or compared the date with <= instead of < shows a number the
// records contradict. The workspace is created for the test, so the figures are exactly these eight.
func TestDeclaredPageTotalsCountEachFigureFromItsMeasure(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredtotals", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil || task.Completion == nil {
		t.Fatal("default installs no mch_task with a completion: block")
	}
	day := func(offset int) string { return time.Now().AddDate(0, 0, offset).Format("2006-01-02") }
	for title, v := range map[string]map[string]any{
		"Fig-done-late":    {"fld_status": task.Completion.Done, "fld_due_date": day(-4)},
		"Fig-open-late":    {"fld_status": "todo", "fld_due_date": day(-2)},
		"Fig-open-today":   {"fld_status": "in_progress", "fld_due_date": day(0)},
		"Fig-open-future":  {"fld_status": "todo", "fld_due_date": day(5)},
		"Fig-open-undated": {"fld_status": "todo"},
		// The week's edges: +7 is inside `$today+7`, +8 is outside, and a finished card due today is neither
		// "due today" nor "due in 7 days".
		"Fig-open-week-end": {"fld_status": "todo", "fld_due_date": day(7)},
		"Fig-open-beyond":   {"fld_status": "todo", "fld_due_date": day(8)},
		"Fig-done-today":    {"fld_status": task.Completion.Done, "fld_due_date": day(0)},
	} {
		v["fld_title"] = title
		rec, err := store.CreateRecord(ctx, task.ID, v)
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}

	body := getPage(t, h, cookie, "/pages/nav_dashboard")
	for label, want := range map[string]int{"All cards": 8, "Open": 6, "Overdue": 1, "Completed": 2, "Due today": 1, "Due in 7 days": 3} {
		pair := regexp.MustCompile(`>` + strconv.Itoa(want) + `</div><div[^>]*>` + regexp.QuoteMeta(label) + `</div>`)
		if !pair.MatchString(body) {
			t.Errorf("no Metric shows %q with a value of %d", label, want)
		}
	}
}

// TestDeclaredPageNamesEachOpenCardGroupByTheProjectAndPersonItBelongsTo: a Dimension over a reference Field
// stores an id. Task figures groups open cards by Project and by assignee, so each tile must carry the Project's
// name and the person's name -- and never the id the record holds. A finished card is not counted, and a card
// pointing at a Project that no longer exists adds no tile.
func TestDeclaredPageNamesEachOpenCardGroupByTheProjectAndPersonItBelongsTo(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declaredbyref", "default")
	var task, project *domain.Machine
	for _, m := range ws.Machines {
		switch m.ID {
		case "mch_task":
			task = m
		case "mch_project":
			project = m
		}
	}
	if task == nil || project == nil || task.Completion == nil {
		t.Fatal("default installs no mch_task/mch_project, or mch_task has no completion: block")
	}
	ids := map[string]string{}
	for _, name := range []string{"Refprobe Zulu", "Refprobe Alpha", "Refprobe Empty"} {
		rec, err := store.CreateRecord(ctx, project.ID, map[string]any{"fld_name": name, "fld_status": "active"})
		if err != nil {
			t.Fatalf("CreateRecord(project): %v", err)
		}
		ids[name] = rec.ID
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, project.ID, rec.ID) })
	}
	for i, v := range []map[string]any{
		{"fld_project": ids["Refprobe Zulu"], "fld_status": "todo", "fld_assignee": actorID},
		{"fld_project": ids["Refprobe Zulu"], "fld_status": "in_progress"},
		{"fld_project": ids["Refprobe Alpha"], "fld_status": "todo"},
		{"fld_project": ids["Refprobe Alpha"], "fld_status": task.Completion.Done},
		{"fld_project": "rec_gone_project", "fld_status": "todo"},
	} {
		v["fld_title"] = "Refprobe card " + strconv.Itoa(i)
		rec, err := store.CreateRecord(ctx, task.ID, v)
		if err != nil {
			t.Fatalf("CreateRecord(task): %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}

	body := getPage(t, h, cookie, "/pages/nav_dashboard")
	// K17: the two groups sit in a `columns` (aligned to the start) of two `section`s, all written in the YAML.
	if !strings.Contains(body, "sm:items-start") || !strings.Contains(body, "sm:flex-row") {
		t.Error("the two open-card groups are not drawn as a columns layout aligned to the start")
	}
	if strings.Count(body, "Tasks by project") != 1 || strings.Count(body, "Open cards per person") != 1 {
		t.Error("want the two group headings once each")
	}
	// K15 stage D2: a person is a Progress bar, open out of everything assigned, named and never an id.
	if !regexp.MustCompile(`role="progressbar" aria-label="declaredbyref@example.com"[^>]*aria-valuetext="1 of 1"`).MatchString(body) {
		t.Error("no Progress bar shows declaredbyref@example.com with 1 of 1")
	}
	// K15 stage D3: every Project is a row with its card count and its open count over two Relations -- a
	// finished card counts toward the first only, and a Project with no cards is a row of zeroes, not missing.
	for label, want := range map[string][2]string{"Refprobe Zulu": {"2 cards", "2 open"}, "Refprobe Alpha": {"2 cards", "1 open"}, "Refprobe Empty": {"0 cards", "0 open"}} {
		row := regexp.MustCompile(regexp.QuoteMeta(label) + `<[\s\S]{0,900}?>` + want[0] + `<[\s\S]{0,300}?>` + want[1] + `<`)
		if !row.MatchString(body) {
			t.Errorf("no project row shows %q with %s and %s", label, want[0], want[1])
		}
	}
	for _, id := range []string{ids["Refprobe Zulu"], ids["Refprobe Alpha"], actorID, "rec_gone_project"} {
		if strings.Contains(body, ">"+id+"<") {
			t.Errorf("the id %q is printed where a name belongs", id)
		}
	}
	if alpha, empty, zulu := strings.Index(body, "Refprobe Alpha"), strings.Index(body, "Refprobe Empty"), strings.Index(body, "Refprobe Zulu"); !(alpha < empty && empty < zulu) {
		t.Error("project rows are not in name order")
	}
}

// TestProjectManagementSettingsHubListsItsDeclaredMemberAndNoAccess: the Application's Settings page is drawn
// from its own navigation (S2.1b). Board Settings is a declared member, so its row links to the screen it
// always was; Project Management declares no roles, so the hub has no Access section, and the page names no
// other Application's words.
func TestProjectManagementSettingsHubListsItsDeclaredMemberAndNoAccess(t *testing.T) {
	h, cookie, _, _, _, _, _ := routerSetupFor(t, "pmsettingshub", "default")
	body := getPage(t, h, cookie, "/settings/nav_pm_settings")
	_, config, found := strings.Cut(body, "Configuration")
	if !found {
		t.Fatal("the hub has no Configuration section")
	}
	for _, want := range []string{`href="/pages/nav_board_settings"`, "Lists &amp; labels"} {
		if !strings.Contains(config, want) {
			t.Errorf("the Configuration section does not contain %q", want)
		}
	}
	if !strings.Contains(body, "Project Management") {
		t.Error("the About section does not name the Application")
	}
	for _, banned := range []string{"Members &amp; roles", "Permissions", "Approval flow"} {
		if strings.Contains(body, banned) {
			t.Errorf("the Project Management hub contains %q, which belongs to another Application's hub", banned)
		}
	}
	if page := getPage(t, h, cookie, "/pages/nav_board_settings"); !strings.Contains(page, `href="/settings/nav_pm_settings"`) {
		t.Error("a Project Management screen does not link to the Application's Settings hub")
	}
}

// TestDeclaredPageNamesEachTaskByTheProjectItSitsIn: a `container` role resolved on a records page. The Task's
// Project is drawn by its own title under the Task's, a Project no listed Task names is not drawn, and a Task whose
// Project no longer exists draws no caption rather than its id. That the read is bounded to the listed ids is held by
// the statement's name in the GET sweep's log (`mch_project by ids`), not by this test.
func TestDeclaredPageNamesEachTaskByTheProjectItSitsIn(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "declaredcontainer", "default")
	mkProject := func(name string) *data.Record {
		rec, err := store.CreateRecord(ctx, "mch_project", map[string]any{"fld_name": name})
		if err != nil {
			t.Fatalf("CreateRecord project %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_project", rec.ID) })
		return rec
	}
	mkTask := func(title, project string) {
		rec, err := store.CreateRecord(ctx, "mch_task", map[string]any{"fld_title": title, "fld_assignee": actorID, "fld_project": project})
		if err != nil {
			t.Fatalf("CreateRecord task %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_task", rec.ID) })
	}
	named, unlisted := mkProject("Container-probe-named"), mkProject("Container-probe-unlisted")
	_ = unlisted
	mkTask("Container-probe-task", named.ID)
	mkTask("Container-probe-orphan", "rec_does_not_exist")
	body := getPage(t, h, cookie, "/pages/nav_my_tasks")
	if !strings.Contains(body, "Container-probe-named") {
		t.Fatalf("the Task is not captioned with its Project's title:\n%s", body)
	}
	if strings.Contains(body, "Container-probe-unlisted") {
		t.Error("a Project no listed Task names appears on the page")
	}
	if strings.Contains(body, "rec_does_not_exist") {
		t.Error("a Task whose Project is gone prints the Project's id")
	}
}

// TestDeclaredPageTransitionButtonsMoveTheStatusAndAreDrawnOnlyWhereTheMoveIsAnChange: `write: transition` inside a
// records template. Two of the viewer's Tasks, one open and one finished, must each carry only the Button that is a
// change for *them* ("Mark done" on the open one, "Reopen" on the finished one); the Button must send a PATCH of the
// Machine's own status Field to the value its `completion:` names, which the generic route accepts and answers with
// `HX-Refresh`; and after it the same Task is offered the opposite. Another person's Task is not listed at all.
func TestDeclaredPageTransitionButtonsMoveTheStatusAndAreDrawnOnlyWhereTheMoveIsAnChange(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declaredtransition", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil || task.Completion == nil {
		t.Fatal("default installs no mch_task with a completion: block")
	}
	mk := func(title, status string) *data.Record {
		rec, err := store.CreateRecord(ctx, task.ID, map[string]any{"fld_title": title, "fld_status": status, "fld_assignee": actorID})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
		return rec
	}
	open, finished := mk("Move-probe-open", task.ReopenValue()), mk("Move-probe-finished", task.Completion.Done)
	// A third status that is neither the finished value nor the reopen value: `Allows` alone would offer it
	// Reopen (a move to the reopen value is a change), and `when: {role: status, is: $done}` is what stops that.
	middle := ""
	if statusField, ok := task.FieldByID(task.CardFieldFor(domain.CardFieldRoleStatus)); ok {
		for _, o := range statusField.Options {
			if o != task.Completion.Done && o != task.ReopenValue() {
				middle = o
				break
			}
		}
	}
	var inProgress *data.Record
	if middle != "" {
		inProgress = mk("Move-probe-middle", middle)
	}

	// button finds the Button of one record's route that sets the status to value, "" when there is none.
	button := func(body string, rec *data.Record, value string) string {
		route := "/machines/" + task.ID + "/records/" + rec.ID
		re := regexp.MustCompile(`<button[^>]*hx-patch="` + regexp.QuoteMeta(route) + `"[^>]*>[^<]*</button>`)
		for _, b := range re.FindAllString(body, -1) {
			if strings.Contains(b, `&#34;`+task.CardFieldFor(domain.CardFieldRoleStatus)+`&#34;:&#34;`+value+`&#34;`) {
				return b
			}
		}
		return ""
	}
	const page = "/pages/nav_my_tasks"
	body := getPage(t, h, cookie, page)
	markDone := button(body, open, task.Completion.Done)
	if markDone == "" || !strings.Contains(markDone, "Mark done") || !strings.Contains(markDone, `hx-swap="none"`) || !strings.Contains(markDone, `type="button"`) || strings.Contains(markDone, "hx-confirm") {
		t.Fatalf("the open Task is offered no plain Mark done request:\n%s", body)
	}
	if want := `aria-label="Mark done: ` + open.Values[task.CardFieldFor(domain.CardFieldRoleTitle)].(string) + `"`; !strings.Contains(markDone, want) {
		t.Errorf("the Mark done button does not carry its Task's title in its accessible name (%s):\n%s", want, markDone)
	}
	for _, b := range regexp.MustCompile(`<button[^>]*hx-patch=[^>]*>`).FindAllString(body, -1) {
		if !strings.Contains(b, `aria-label="`) {
			t.Errorf("a transition button on the page has no accessible name beyond its bare label: %s", b)
		}
	}
	if button(body, open, task.ReopenValue()) != "" {
		t.Error("an open Task is offered Reopen, which would write the value it already holds")
	}
	if button(body, finished, task.Completion.Done) != "" {
		t.Error("a finished Task is offered Mark done")
	}
	if button(body, finished, task.ReopenValue()) == "" {
		t.Error("a finished Task is offered no Reopen")
	}
	if inProgress != nil {
		if button(body, inProgress, task.Completion.Done) == "" {
			t.Error("an unfinished Task is offered no Mark done")
		}
		if button(body, inProgress, task.ReopenValue()) != "" {
			t.Error("an unfinished, not-yet-open Task is offered Reopen: the `when:` on that Button did not hide it")
		}
	}

	patch := func(rec *data.Record, vals url.Values) *httptest.ResponseRecorder {
		tok := csrfTokenFor(t, h, "/login")
		req := httptest.NewRequest(http.MethodPatch, "/machines/"+task.ID+"/records/"+rec.ID, strings.NewReader(vals.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", "http://x"+page)
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec2 := httptest.NewRecorder()
		h.ServeHTTP(rec2, req)
		return rec2
	}
	statusField := task.CardFieldFor(domain.CardFieldRoleStatus)
	if got := patch(open, url.Values{statusField: {task.Completion.Done}}); got.Code != http.StatusOK || got.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("the Button's request answered %d with HX-Refresh=%q", got.Code, got.Header().Get("HX-Refresh"))
	}
	body = getPage(t, h, cookie, page)
	if button(body, open, task.Completion.Done) != "" || button(body, open, task.ReopenValue()) == "" {
		t.Error("after finishing a Task the page still offers Mark done, or no Reopen")
	}
}

// TestDeclaredPageFilesEachTaskInItsOwnBucket: the four My Tasks Datasets through the real store. Later is the case
// that needs `any:` and `is_empty` -- a Task that never had a due date (the Field absent) and one due well past the
// window must both land there, and an undated Task that is finished must land only in Completed. Dates sit 3, 30 and
// 60 days from today so the viewer's clock cannot move one across a boundary.
func TestDeclaredPageFilesEachTaskInItsOwnBucket(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declaredbuckets", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil || task.Completion == nil {
		t.Fatal("default installs no mch_task with a completion: block")
	}
	day := func(offset int) string { return time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02") }
	mk := func(title, status, due string) {
		v := map[string]any{"fld_title": title, "fld_status": status, "fld_assignee": actorID}
		if due != "" {
			v["fld_due_date"] = due
		}
		rec, err := store.CreateRecord(ctx, task.ID, v)
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}
	open, done := task.ReopenValue(), task.Completion.Done
	mk("Bucket-probe-overdue", open, day(-30))
	mk("Bucket-probe-soon", open, day(3))
	mk("Bucket-probe-far", open, day(60))
	mk("Bucket-probe-undated", open, "")
	mk("Bucket-probe-finished-undated", done, "")
	mk("Bucket-probe-finished-overdue", done, day(-30))
	// The window's edges: today and today+7 are in Next 7 days, today+8 is Later. A comparison turned from lte to lt
	// (or gte to gt) moves exactly one of these.
	mk("Bucket-probe-today", open, day(0))
	mk("Bucket-probe-day7", open, day(7))
	mk("Bucket-probe-day8", open, day(8))
	mk("Bucket-probe-yesterday", open, day(-1))
	// Another person's Task, in every bucket's shape: the page must list none of them (the assignee predicate runs
	// in each bucket Dataset's own SQL, so a bucket missing it would show here).
	for i, theirs := range []map[string]any{
		{"fld_status": open, "fld_due_date": day(-30)},
		{"fld_status": open, "fld_due_date": day(3)},
		{"fld_status": open},
		{"fld_status": done},
	} {
		theirs["fld_title"], theirs["fld_assignee"] = fmt.Sprintf("Bucket-probe-theirs-%d", i), "usr_someone_else"
		other, err := store.CreateRecord(ctx, task.ID, theirs)
		if err != nil {
			t.Fatalf("CreateRecord theirs: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, other.ID) })
	}

	body := getPage(t, h, cookie, "/pages/nav_my_tasks")
	at := func(heading string) int {
		i := strings.Index(body, ">"+heading+"<")
		if i < 0 {
			t.Fatalf("no %q heading on the page:\n%s", heading, body)
		}
		return i
	}
	bounds := []struct {
		name       string
		start, end int
	}{}
	if strings.Contains(body, "Bucket-probe-theirs-") {
		t.Error("another person's Task is on the viewer's My Tasks page")
	}
	order := []string{"Overdue", "Next 7 days", "Later", "Completed"}
	for i, name := range order {
		end := len(body)
		if i+1 < len(order) {
			end = at(order[i+1])
		}
		bounds = append(bounds, struct {
			name       string
			start, end int
		}{name, at(name), end})
	}
	section := map[string]string{}
	for _, b := range bounds {
		section[b.name] = body[b.start:b.end]
	}
	want := map[string]string{
		"Bucket-probe-overdue":          "Overdue",
		"Bucket-probe-soon":             "Next 7 days",
		"Bucket-probe-far":              "Later",
		"Bucket-probe-undated":          "Later",
		"Bucket-probe-finished-undated": "Completed",
		"Bucket-probe-finished-overdue": "Completed",
		"Bucket-probe-today":            "Next 7 days",
		"Bucket-probe-day7":             "Next 7 days",
		"Bucket-probe-day8":             "Later",
		"Bucket-probe-yesterday":        "Overdue",
	}
	for title, home := range want {
		for name, text := range section {
			if got := strings.Contains(text, title); got != (name == home) {
				t.Errorf("%s: in %q = %v, want it only in %q", title, name, got, home)
			}
		}
	}
}

// TestDeclaredPageOrdersABucketByDateThenByTheBoardsOwnOrder: Overdue and Next 7 days sort by due date, and Tasks
// sharing a date keep the order the Store holds them in (the old screen's stable sort did the same). Without the
// second key the database is free to return same-day Tasks in any order between two requests.
func TestDeclaredPageOrdersABucketByDateThenByTheBoardsOwnOrder(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declaredbucketorder", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil {
		t.Fatal("default installs no mch_task")
	}
	day := func(offset int) string { return time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02") }
	// Created out of date order on purpose: later date first, then two on one earlier date.
	for _, c := range []struct{ title, due string }{
		{"Order-probe-c-late", day(5)}, {"Order-probe-a-same", day(2)}, {"Order-probe-b-same", day(2)}, {"Order-probe-d-same", day(2)},
	} {
		rec, err := store.CreateRecord(ctx, task.ID, map[string]any{"fld_title": c.title, "fld_status": task.ReopenValue(), "fld_assignee": actorID, "fld_due_date": c.due})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", c.title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}
	for i := 0; i < 5; i++ {
		body := getPage(t, h, cookie, "/pages/nav_my_tasks")
		at := func(title string) int { return strings.Index(body, ">"+title+"<") }
		if !(at("Order-probe-a-same") >= 0 && at("Order-probe-a-same") < at("Order-probe-b-same") && at("Order-probe-b-same") < at("Order-probe-d-same") && at("Order-probe-d-same") < at("Order-probe-c-late")) {
			t.Fatalf("request %d: the bucket is not ordered by date, then by the Store's own order (a,b,d same day, then c)", i)
		}
	}
}

// TestDeclaredPageDrawsADatePillPerDatedTask: each dated Task row carries a pill with its short date, red while an
// open Task is past due and green once it is finished, and an undated Task carries none. The pill is a registered
// StatusBadge filled from the derived date roles; nothing on the page says what red means.
func TestDeclaredPageDrawsADatePillPerDatedTask(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declareddatepill", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil || task.Completion == nil {
		t.Fatal("default installs no mch_task with a completion: block")
	}
	past := time.Now().UTC().AddDate(0, 0, -30)
	mk := func(title, status string, due *time.Time) {
		v := map[string]any{"fld_title": title, "fld_status": status, "fld_assignee": actorID}
		if due != nil {
			v["fld_due_date"] = due.Format("2006-01-02")
		}
		rec, err := store.CreateRecord(ctx, task.ID, v)
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}
	mk("Pillprobe-open-late", task.ReopenValue(), &past)
	mk("Pillprobe-done-late", task.Completion.Done, &past)
	mk("Pillprobe-undated", task.ReopenValue(), nil)

	body := getPage(t, h, cookie, "/pages/nav_my_tasks")
	row := func(title string) string {
		i := strings.Index(body, ">"+title+"<")
		if i < 0 {
			t.Fatalf("no row for %q:\n%s", title, body)
		}
		end := strings.Index(body[i:], "</li>")
		if end < 0 {
			t.Fatalf("row for %q has no end", title)
		}
		return body[i : i+end]
	}
	label := past.Format("02 Jan")
	if past.Year() != time.Now().UTC().Year() {
		label = past.Format("02 Jan 2006")
	}
	// The pill's colour is its tone's property since DS-D15 stage 2 (`bg-tone-bad-bg` is red under the default
	// theme); the tone is what this asserts, so a Workspace's palette choice cannot make it pass or fail.
	if r := row("Pillprobe-open-late"); !strings.Contains(r, label) || !strings.Contains(r, "bg-tone-bad-bg") {
		t.Errorf("an open Task past due needs a red pill reading %q:\n%s", label, r)
	}
	if r := row("Pillprobe-done-late"); !strings.Contains(r, label) || !strings.Contains(r, "bg-tone-good-bg") || strings.Contains(r, "bg-tone-bad-bg") {
		t.Errorf("a finished Task needs a green pill reading %q:\n%s", label, r)
	}
	if r := row("Pillprobe-undated"); strings.Contains(r, "bg-tone-bad-bg") || strings.Contains(r, "bg-tone-good-bg") || strings.Contains(r, `<span class="inline-flex`) {
		t.Errorf("an undated Task must carry no pill:\n%s", r)
	}
}

// TestDeclaredPageNamesTheListEachTaskSitsIn: a Task's row carries a badge with the title of the list its board
// groups it under, and a Task in no list carries none. The page names no Field; the group is the Machine's own
// default View.
func TestDeclaredPageNamesTheListEachTaskSitsIn(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declaredlistname", "default")
	var task, list *domain.Machine
	for _, m := range ws.Machines {
		switch m.ID {
		case "mch_task":
			task = m
		case "mch_list":
			list = m
		}
	}
	if task == nil || list == nil {
		t.Fatal("default installs no mch_task / mch_list")
	}
	lrec, err := store.CreateRecord(ctx, list.ID, map[string]any{"fld_name": "Listprobe-Doing"})
	if err != nil {
		t.Fatalf("CreateRecord list: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, list.ID, lrec.ID) })
	mk := func(title string, listID any) {
		v := map[string]any{"fld_title": title, "fld_status": task.ReopenValue(), "fld_assignee": actorID}
		if listID != nil {
			v["fld_list"] = listID
		}
		rec, err := store.CreateRecord(ctx, task.ID, v)
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}
	mk("Listprobe-filed", lrec.ID)
	mk("Listprobe-loose", nil)

	body := getPage(t, h, cookie, "/pages/nav_my_tasks")
	row := func(title string) string {
		i := strings.Index(body, ">"+title+"<")
		if i < 0 {
			t.Fatalf("no row for %q:\n%s", title, body)
		}
		end := strings.Index(body[i:], "</li>")
		return body[i : i+end]
	}
	if r := row("Listprobe-filed"); !strings.Contains(r, ">Listprobe-Doing<") {
		t.Errorf("a Task in a list needs its list's title:\n%s", r)
	}
	if r := row("Listprobe-loose"); strings.Contains(r, `<span class="inline-flex`) {
		t.Errorf("a Task in no list must carry no badge:\n%s", r)
	}
}

// TestDeclaredPageRecentActivityShowsOnlyThisApplicationsNewestTen: the panel reads ds_recent_activity, whose
// `$parameters.application` the runtime sets to the page's own Application. Eleven events of this Application
// show the newest ten, another Application's event never shows, and a query string naming that other
// Application does not change it -- the name is the runtime's, not the visitor's.
func TestDeclaredPageRecentActivityShowsOnlyThisApplicationsNewestTen(t *testing.T) {
	h, cookie, ctx, store, _, _, _ := routerSetupFor(t, "declaredactivity", "default")
	add := func(summary, app string) {
		rec, err := store.CreateRecord(ctx, "mch_activity", map[string]any{
			"fld_machine_id": "mch_task", "fld_record_id": "x", "fld_summary": summary, "fld_application_id": app,
		})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", summary, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_activity", rec.ID) })
		time.Sleep(2 * time.Millisecond) // created_at orders the window; keep the inserts apart
	}
	for i := 1; i <= 11; i++ {
		add(fmt.Sprintf("Pmact-%02d", i), "app_project_management")
	}
	add("Pmact-foreign", "app_document_approval")

	for _, path := range []string{"/pages/nav_dashboard", "/pages/nav_dashboard?application=app_document_approval"} {
		body := getPage(t, h, cookie, path)
		if strings.Contains(body, "Pmact-foreign") {
			t.Errorf("%s: another Application's event is drawn", path)
		}
		if strings.Contains(body, "Pmact-01<") {
			t.Errorf("%s: the eleventh-newest event is drawn; the window is ten", path)
		}
		for _, want := range []string{"Pmact-02<", "Pmact-11<"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: %q missing from the panel", path, want)
			}
		}
		if strings.Index(body, "Pmact-11<") > strings.Index(body, "Pmact-02<") {
			t.Errorf("%s: the newest event is not first", path)
		}
	}
}

// TestDeclaredPageDrawsEachTasksOwnTagsInItsRow: `each: tags` through the real store. A Task with two Labels draws two
// chips in its own row, a Task with none draws none, and one Task's Label never lands on another's row. Titles are
// compared per row so a chip read from the wrong record would show.
func TestDeclaredPageDrawsEachTasksOwnTagsInItsRow(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "declaredtags", "default")
	mk := func(machine string, v map[string]any) string {
		rec, err := store.CreateRecord(ctx, machine, v)
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", machine, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, machine, rec.ID) })
		return rec.ID
	}
	late := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	task := func(title string) string {
		return mk("mch_task", map[string]any{"fld_title": title, "fld_status": "todo", "fld_assignee": actorID, "fld_due_date": late})
	}
	tagged, plain, other := task("Tagprobe-tagged"), task("Tagprobe-plain"), task("Tagprobe-other")
	urgent := mk("mch_label", map[string]any{"fld_name": "Tagprobe-urgent", "fld_color": "rose"})
	backend := mk("mch_label", map[string]any{"fld_name": "Tagprobe-backend", "fld_color": "cyan"})
	for _, j := range [][2]string{{tagged, urgent}, {tagged, backend}, {other, backend}} {
		mk("mch_card_label", map[string]any{"fld_task": j[0], "fld_label": j[1]})
	}
	_ = plain

	body := getPage(t, h, cookie, "/pages/nav_my_tasks")
	rowOf := func(title string) string {
		i := strings.Index(body, ">"+title+"<")
		if i < 0 {
			t.Fatalf("no row for %q:\n%s", title, body)
		}
		end := len(body)
		// The row ends where the next Task's title link starts: the next ">Tagprobe-<" with a closing bracket.
		from := i + len(title) + 2
		if j := strings.Index(body[from:], "</li>"); j >= 0 {
			end = from + j
		}
		for _, next := range []string{"Tagprobe-tagged", "Tagprobe-plain", "Tagprobe-other"} {
			if j := strings.Index(body[from:], ">"+next+"<"); j >= 0 && from+j < end {
				end = from + j
			}
		}
		return body[i:end]
	}
	has := func(row, label string) bool { return strings.Contains(row, label) }
	if r := rowOf("Tagprobe-tagged"); !has(r, "Tagprobe-urgent") || !has(r, "Tagprobe-backend") {
		t.Errorf("the tagged Task's row misses a chip:\n%s", r)
	}
	if r := rowOf("Tagprobe-plain"); strings.Contains(r, "rounded-full border") {
		t.Errorf("a Task with no Label drew a chip:\n%s", r)
	}
	if r := rowOf("Tagprobe-other"); !has(r, "Tagprobe-backend") || has(r, "Tagprobe-urgent") {
		t.Errorf("the other Task's row should carry only backend:\n%s", r)
	}
}

// TestDeclaredPageHidesAnEmptyBucketAndCountsTheOthers: a section with no Tasks is not on the page -- neither its
// heading nor an empty list -- and one with Tasks carries its heading, a count of the rows under it and its hint.
// A viewer with nothing assigned gets the page's own words in place of four hidden sections, and not a layout with
// no children. Both halves are asserted, because "hides when empty" and "always hides" look the same on a page
// whose viewer happens to have nothing.
func TestDeclaredPageHidesAnEmptyBucketAndCountsTheOthers(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declaredhideempty", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil || task.Completion == nil {
		t.Fatal("default installs no mch_task with a completion: block")
	}
	quiet := getPage(t, h, cookie, "/pages/nav_my_tasks")
	if !strings.Contains(quiet, "Nothing is assigned to you.") {
		t.Errorf("a viewer with no Tasks was not told so:\n%s", quiet)
	}
	// E-1 (2026-10-14): a layout's `empty:` is the whole page being quiet, so it draws the EmptyState card, not bare words.
	if i := strings.Index(quiet, "Nothing is assigned to you."); i >= 0 {
		open := strings.LastIndex(quiet[:i], "<")
		if !strings.Contains(quiet[open:i], "border-dashed") {
			t.Errorf("the quiet page's words are not inside the EmptyState card: %q", quiet[open:i])
		}
	}
	for _, heading := range []string{"Overdue", "Next 7 days", "Later", "Completed"} {
		if strings.Contains(quiet, ">"+heading+"<") {
			t.Errorf("the %q section is on a page with no Tasks", heading)
		}
	}

	late := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	for _, title := range []string{"Hideprobe-late-a", "Hideprobe-late-b"} {
		rec, err := store.CreateRecord(ctx, task.ID, map[string]any{"fld_title": title, "fld_status": task.ReopenValue(), "fld_assignee": actorID, "fld_due_date": late})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}
	busy := getPage(t, h, cookie, "/pages/nav_my_tasks")
	i := strings.Index(busy, ">Overdue<")
	if i < 0 {
		t.Fatalf("no Overdue section for a viewer with overdue Tasks:\n%s", busy)
	}
	head := busy[i:]
	if j := strings.Index(head, "Hideprobe-late-a"); j < 0 || !strings.Contains(head[:j], ">2<") {
		t.Errorf("the Overdue heading is not followed by a count of 2 before its first row:\n%s", head)
	}
	if !strings.Contains(head, "needs a new date or a tick") {
		t.Error("the Overdue hint is missing")
	}
	for _, heading := range []string{"Next 7 days", "Later", "Completed"} {
		if strings.Contains(busy, ">"+heading+"<") {
			t.Errorf("the empty %q section is on the page", heading)
		}
	}
	if strings.Contains(busy, "Nothing is assigned to you.") {
		t.Error("the quiet-page words are drawn beside real Tasks")
	}
}

// TestDeclaredPageWordsTheTileHintsAroundDerivedMeasures (K15 D1): "All cards" says how many Projects its cards
// sit in (count_distinct over fld_project) and "Completed" says its share of all cards (a percentage Measure),
// both numbers coming from ds_task_figures and none typed in the YAML.
func TestDeclaredPageWordsTheTileHintsAroundDerivedMeasures(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredhints", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil || task.Completion == nil {
		t.Fatal("default installs no mch_task with a completion: block")
	}
	for i, v := range []map[string]any{
		{"fld_project": "rec_hint_p1", "fld_status": task.Completion.Done},
		{"fld_project": "rec_hint_p1", "fld_status": "todo"},
		{"fld_project": "rec_hint_p2", "fld_status": "todo"},
	} {
		v["fld_title"] = "Hint card " + strconv.Itoa(i)
		rec, err := store.CreateRecord(ctx, task.ID, v)
		if err != nil {
			t.Fatalf("CreateRecord: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}
	body := getPage(t, h, cookie, "/pages/nav_dashboard")
	for _, want := range []string{">In 2 projects<", ">33% of all cards<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not say %q", want)
		}
	}
}

// TestDeclaredPageListsOnlyTheTasksDueInTheWeekInView: `$week` and `$week+6` bound a Dataset to Monday..Sunday of
// the week `?week=N` selects, N whole weeks from this one. The edges are the point -- the Sunday that closes the
// window belongs to it and the Monday after does not, and a Sunday must not read as the start of the next week
// (Go counts it as day 0) -- and so is that every assignee's Task is listed, since a week is not a personal bucket.
func TestDeclaredPageListsOnlyTheTasksDueInTheWeekInView(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declaredweek", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil {
		t.Fatal("default installs no mch_task")
	}
	today := time.Now() // the handler reads the local clock, so this does too
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	day := func(offset int) string { return monday.AddDate(0, 0, offset).Format("2006-01-02") }
	for _, c := range []struct {
		title, due, assignee string
	}{
		{"Week-probe-prev-sunday", day(-1), actorID},
		{"Week-probe-monday", day(0), actorID},
		{"Week-probe-wednesday-theirs", day(2), "usr_somebody_else"},
		{"Week-probe-sunday", day(6), actorID},
		{"Week-probe-next-monday", day(7), actorID},
		{"Week-probe-next-sunday", day(13), actorID},
		{"Week-probe-after-next-monday", day(14), actorID},
		{"Week-probe-undated", "", actorID},
	} {
		v := map[string]any{"fld_title": c.title, "fld_status": task.ReopenValue(), "fld_assignee": c.assignee}
		if c.due != "" {
			v["fld_due_date"] = c.due
		}
		rec, err := store.CreateRecord(ctx, task.ID, v)
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", c.title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"", []string{"Week-probe-monday", "Week-probe-wednesday-theirs", "Week-probe-sunday"}},
		{"?week=0", []string{"Week-probe-monday", "Week-probe-wednesday-theirs", "Week-probe-sunday"}},
		{"?week=1", []string{"Week-probe-next-monday", "Week-probe-next-sunday"}},
		{"?week=-1", []string{"Week-probe-prev-sunday"}},
		{"?week=abc", []string{"Week-probe-monday", "Week-probe-wednesday-theirs", "Week-probe-sunday"}},
	} {
		body := getPage(t, h, cookie, "/pages/nav_calendar"+c.query)
		listed := map[string]bool{}
		for _, m := range regexp.MustCompile(`>(Week-probe-[a-z-]+)<`).FindAllStringSubmatch(body, -1) {
			listed[m[1]] = true
		}
		if len(listed) != len(c.want) {
			t.Errorf("%q lists %v, want exactly %v", c.query, listed, c.want)
		}
		for _, w := range c.want {
			if !listed[w] {
				t.Errorf("%q does not list %s (listed %v)", c.query, w, listed)
			}
		}
	}
	// The Dataset sorts by due date, so Monday precedes Wednesday precedes Sunday on the page.
	body := getPage(t, h, cookie, "/pages/nav_calendar")
	at := func(title string) int { return strings.Index(body, ">"+title+"<") }
	if !(at("Week-probe-monday") >= 0 && at("Week-probe-monday") < at("Week-probe-wednesday-theirs") && at("Week-probe-wednesday-theirs") < at("Week-probe-sunday")) {
		t.Error("the week's tasks are not listed in due-date order")
	}
}

// TestDeclaredPageGroupsTheWeekIntoSevenDays: the grouped Collection draws every day of the week in view once, in
// order, a task under the day it is due and no other, and a day with nothing says so in the page's own words.
// Placement is the point -- the count and the order of headings alone would pass a page that listed every task
// under Monday -- so each probe is located between its own day's heading and the next one.
func TestDeclaredPageGroupsTheWeekIntoSevenDays(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredgroup", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil {
		t.Fatal("default installs no mch_task")
	}
	today := time.Now()
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	for title, offset := range map[string]int{"Group-probe-mon": 0, "Group-probe-wed-a": 2, "Group-probe-wed-b": 2, "Group-probe-sun": 6} {
		rec, err := store.CreateRecord(ctx, task.ID, map[string]any{
			"fld_title": title, "fld_status": task.ReopenValue(), "fld_due_date": monday.AddDate(0, 0, offset).Format("2006-01-02"),
		})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
	}
	body := getPage(t, h, cookie, "/pages/nav_calendar")
	head := func(offset int) int {
		// A day's heading is its weekday over its date (two nodes); the date is what places it.
		at := strings.Index(body, ">"+monday.AddDate(0, 0, offset).Format("02 Jan")+"<")
		if at < 0 {
			t.Fatalf("no heading for day %d of the week", offset)
		}
		return at
	}
	for d := 1; d < 7; d++ {
		if head(d) <= head(d-1) {
			t.Errorf("day %d is not after day %d", d, d-1)
		}
	}
	for title, want := range map[string]int{"Group-probe-mon": 0, "Group-probe-wed-a": 2, "Group-probe-wed-b": 2, "Group-probe-sun": 6} {
		at := strings.Index(body, ">"+title+"<")
		upper := len(body)
		if want < 6 {
			upper = head(want + 1)
		}
		if at < head(want) || at > upper {
			t.Errorf("%s is not under its own day's heading", title)
		}
	}
	// Tuesday, Thursday, Friday and Saturday hold nothing of ours, but the shared database may hold other
	// people's tasks, so the check is on the page's own words appearing at all rather than on a count.
	if !strings.Contains(body, "—") {
		t.Error("an empty day does not say so")
	}
}

// TestDeclaredPageSetsTodayApartInTheWeekInView: the section of today's heading, and only it, is announced as the
// current date; a week that does not hold today has none. Located by the heading inside the section, since a
// count alone would pass a page that marked the wrong day.
func TestDeclaredPageSetsTodayApartInTheWeekInView(t *testing.T) {
	h, cookie, _, _, _, _, _ := routerSetupFor(t, "declaredtoday", "default")
	todayHeading := ">" + time.Now().Format("02 Jan") + "<"
	body := getPage(t, h, cookie, "/pages/nav_calendar")
	if n := strings.Count(body, `aria-current="date"`); n != 1 {
		t.Fatalf("this week marks %d days as current, want exactly 1", n)
	}
	mark, head := strings.Index(body, `aria-current="date"`), strings.Index(body, todayHeading)
	if head < mark || strings.Contains(body[mark:head], "<section") {
		t.Errorf("the marked section is not the one holding today's heading (mark %d, heading %d)", mark, head)
	}
	if next := getPage(t, h, cookie, "/pages/nav_calendar?week=1"); strings.Contains(next, `aria-current="date"`) {
		t.Error("next week marks a day as current")
	}
}

func TestDeclaredPageWeekToolbarFollowsTheWeekInView(t *testing.T) {
	h, cookie, _, _, _, _, _ := routerSetupFor(t, "declaredtoolbar", "default")
	this := getPage(t, h, cookie, "/pages/nav_calendar")
	for _, want := range []string{`href="/pages/nav_calendar?week=1"`, `href="/pages/nav_calendar?week=-1"`} {
		if !strings.Contains(this, want) {
			t.Errorf("this week lacks %s", want)
		}
	}
	// The navigation names the page "This week" too, so the toolbar's link is the count that differs between weeks.
	const backLink = ">This week<"
	navOnly := strings.Count(this, backLink)
	if strings.Contains(this, "week=0") {
		t.Error("a link carries ?week=0 instead of the plain route")
	}
	monday := time.Now()
	for monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, -1)
	}
	if want := monday.Format("02"); !strings.Contains(this, want+" – ") && !strings.Contains(this, want+" "+monday.Format("Jan")+" – ") {
		t.Errorf("this week does not say it starts on %s", monday.Format("02 Jan"))
	}
	third := getPage(t, h, cookie, "/pages/nav_calendar?week=3")
	if got := strings.Count(third, backLink); got != navOnly+1 {
		t.Errorf("week 3 has %d \"This week\" links, this week had %d: want exactly one more, the toolbar's way back", got, navOnly)
	}
	for _, want := range []string{`href="/pages/nav_calendar?week=4"`, `href="/pages/nav_calendar?week=2"`, `href="/pages/nav_calendar"`} {
		if !strings.Contains(third, want) {
			t.Errorf("week 3 lacks %s", want)
		}
	}
	if back := getPage(t, h, cookie, "/pages/nav_calendar?week=1"); strings.Contains(back, "week=0") || !strings.Contains(back, `href="/pages/nav_calendar"`) {
		t.Error("week 1's way back to this week is not the plain route")
	}
}

// TestDeclaredCalendarCardIsACircleATitleAndWhoHoldsIt: a card on the Calendar is a completion circle that finishes an open task and
// reopens a finished one (pressed for the finished one), the title in the link's ink while open and the quiet ink once finished, and
// the assignee as an avatar of initials named for them. Located per record route, since a count of circles would pass a page that
// offered the wrong state on the wrong task.
func TestDeclaredCalendarCardIsACircleATitleAndWhoHoldsIt(t *testing.T) {
	h, cookie, ctx, store, _, ws, actorID := routerSetupFor(t, "declaredcal", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil || task.Completion == nil {
		t.Fatal("default installs no mch_task with a completion: block")
	}
	due := time.Now().Format("2006-01-02")
	mk := func(title, status string) *data.Record {
		rec, err := store.CreateRecord(ctx, task.ID, map[string]any{"fld_title": title, "fld_status": status, "fld_assignee": actorID, "fld_due_date": due})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
		return rec
	}
	open, finished := mk("Cal-probe-open", task.ReopenValue()), mk("Cal-probe-finished", task.Completion.Done)
	body := getPage(t, h, cookie, "/pages/nav_calendar")

	route := func(rec *data.Record) string { return "/machines/" + task.ID + "/records/" + rec.ID }
	circle := func(rec *data.Record, title string, pressed bool) bool {
		re := regexp.MustCompile(`<form[^>]*hx-patch="` + regexp.QuoteMeta(route(rec)) + `"[^>]*>.*?<button[^>]*aria-pressed="` + fmt.Sprint(pressed) + `"[^>]*aria-label="Mark \x{201c}` + title + `\x{201d} (?:not )?complete"`)
		return re.MatchString(body)
	}
	if !circle(open, "Cal-probe-open", false) || circle(open, "Cal-probe-open", true) {
		t.Error("the open task is not offered exactly an unpressed circle")
	}
	if !circle(finished, "Cal-probe-finished", true) || circle(finished, "Cal-probe-finished", false) {
		t.Error("the finished task is not offered exactly a pressed circle")
	}
	if strings.Contains(body, ">Mark done<") || strings.Contains(body, ">Reopen<") {
		t.Error("a card still carries a Mark done / Reopen text button")
	}
	// The title: strong ink while open (a plain link), quiet once finished.
	if !strings.Contains(body, `href="`+route(open)+`" class="text-ink-strong`) || !strings.Contains(body, `href="`+route(finished)+`" class="text-ink-secondary`) {
		t.Error("the open title is not in the link's ink, or the finished title is not in the quiet ink")
	}
	// Both cards are draggable by whoever may edit them: the patch route and the day they are dropped on are on the page.
	if n := strings.Count(body, `data-move-url="`+route(open)+`"`) + strings.Count(body, `data-move-url="`+route(finished)+`"`); n != 2 {
		t.Errorf("%d of our two cards carry a move route, want both", n)
	}
	if !strings.Contains(body, `data-drop-value="`+due+`"`) {
		t.Error("today is not a day a card can be dropped on")
	}
	if n := strings.Count(body, "declaredcal@example.com"); n < 2 {
		t.Errorf("the assignee is named on %d cards, want both of ours", n)
	}
}

// TestDeclaredPageDrawsAnInstalledTable: the Helpdesk's All tickets page declares a Table over a records Dataset. Three
// tickets created out of due order come back as table rows soonest due first, the title is a link to the record's own
// page, the status is a badge, and the header names each column.
func TestDeclaredPageDrawsAnInstalledTable(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "declaredtable", "default")
	var ticket *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_helpdesk_ticket" {
			ticket = m
		}
	}
	if ticket == nil {
		t.Fatal("default installs no Helpdesk ticket Machine")
	}
	for title, due := range map[string]string{"Table-probe-late": "2031-03-01", "Table-probe-soon": "2031-01-01", "Table-probe-mid <b>x</b>": "2031-02-01"} {
		rec, err := store.CreateRecord(ctx, ticket.ID, map[string]any{"fld_title": title, "fld_category": "software", "fld_priority": "normal", "fld_status": "new", "fld_due": due})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, ticket.ID, rec.ID) })
	}
	body := getPage(t, h, cookie, "/pages/nav_helpdesk_table")
	for _, want := range []string{`aria-label="All tickets"`, ">Ticket</th>", ">Status</th>", ">Due</th>", ">Agent</th>", `href="/machines/mch_helpdesk_ticket/records/rec_`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %s", want)
		}
	}
	soon, mid, late := strings.Index(body, "Table-probe-soon"), strings.Index(body, "Table-probe-mid"), strings.Index(body, "Table-probe-late")
	if soon < 0 || mid < 0 || late < 0 || !(soon < mid && mid < late) {
		t.Errorf("rows are not soonest due first: soon=%d mid=%d late=%d", soon, mid, late)
	}
	if strings.Contains(body, "<b>x</b>") {
		t.Error("a ticket title reached the page unescaped")
	}
}

// TestWorkspacePageIsDrawnForAnAdminAndRefusedForAMember: the Workspace's own page (`navigation:` on the manifest) shows
// figures from three Applications and a table, is offered in the Workspace menu to an admin, and answers a plain member with
// the refusal a Workspace screen gives, because no Application role gates it.
func TestWorkspacePageIsDrawnForAnAdminAndRefusedForAMember(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "workspacepage", "default")
	var ticket *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_helpdesk_ticket" {
			ticket = m
		}
	}
	if ticket == nil {
		t.Fatal("default installs no Helpdesk ticket Machine")
	}
	for _, status := range []string{"new", "resolved"} {
		rec, err := store.CreateRecord(ctx, ticket.ID, map[string]any{"fld_title": "WsPage-probe-" + status, "fld_category": "software", "fld_priority": "normal", "fld_status": status, "fld_due": "2031-01-01"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, ticket.ID, rec.ID) })
	}
	body := getPage(t, h, cookie, "/pages/nav_workspace_overview")
	for _, want := range []string{"Open tickets", "Overdue maintenance", "Loans waiting", "Loans out", `aria-label="Tickets, soonest due first"`, "WsPage-probe-new", "WsPage-probe-resolved"} {
		if !strings.Contains(body, want) {
			t.Errorf("the overview does not contain %q", want)
		}
	}
	// A link on the page reaches an Application's own page: the page resolves `to:` against the whole Workspace.
	if !strings.Contains(body, `href="/pages/nav_helpdesk_table"`) {
		t.Error("the overview's link to the Helpdesk's table did not resolve to its route")
	}
	// (The Workspace's name used to open a menu that listed its declared pages; it is a link to Home now, so a declared page is
	// reached by its route and the shell offers none -- owner decision, 2026-10-22.)
	// A plain member is refused the page.
	wsID, _ := data.WorkspaceScope(ctx)
	member, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "workspacepage_member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(context.Background(), wsID, member.ID, "workspacepage_member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	memberCookie := sessionCookieValueForTest(t, config.Config{SessionSecret: "workspacepage-secret"}, member.ID, 0)
	mreq := httptest.NewRequest(http.MethodGet, "/pages/nav_workspace_overview", nil)
	mreq.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: memberCookie})
	mrec := httptest.NewRecorder()
	h.ServeHTTP(mrec, mreq)
	if mrec.Code != http.StatusForbidden {
		t.Errorf("a member opening the Workspace page = %d, want 403", mrec.Code)
	}
	if strings.Contains(mrec.Body.String(), "Open tickets") {
		t.Error("a member was shown the figures that the 403 refuses")
	}
	if home := getPage(t, h, memberCookie, "/home"); strings.Contains(home, `href="/pages/nav_workspace_overview"`) {
		t.Error("the Workspace menu offers a plain member a page that answers 403")
	}
	// A page that is no Workspace's and no Application's is a 404, not an empty shell.
	req := httptest.NewRequest(http.MethodGet, "/pages/nav_nothing", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	if rec := httptest.NewRecorder(); func() int { h.ServeHTTP(rec, req); return rec.Code }() != http.StatusNotFound {
		t.Error("an unknown page id did not answer 404")
	}
}

// TestAWorkspacePageOpenToMembersIsOpenedByAMemberAndOnlyByAMember: `open_to: member` on the Workspace's My work page admits
// a plain member, who sees their own tickets and not another reporter's, and is offered the page in the Workspace menu; a
// person with no membership in the Workspace is still refused, and the Overview, which says nothing, stays with the admins.
func TestAWorkspacePageOpenToMembersIsOpenedByAMemberAndOnlyByAMember(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "mywork", "default")
	var ticket *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_helpdesk_ticket" {
			ticket = m
		}
	}
	if ticket == nil {
		t.Fatal("default installs no Helpdesk ticket Machine")
	}
	wsID, _ := data.WorkspaceScope(ctx)
	person := func(email string, member bool) (id, sessionCookie string) {
		u, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		if member {
			if err := store.AddMember(context.Background(), wsID, u.ID, email, "member", ""); err != nil {
				t.Fatal(err)
			}
		}
		return u.ID, sessionCookieValueForTest(t, config.Config{SessionSecret: "mywork-secret"}, u.ID, 0)
	}
	memberID, memberCookie := person("mywork_member@example.com", true)
	otherID, _ := person("mywork_other@example.com", true)
	_, strangerCookie := person("mywork_stranger@example.com", false)
	for who, title := range map[string]string{memberID: "MyWork-mine", otherID: "MyWork-theirs"} {
		rec, err := store.CreateRecord(ctx, ticket.ID, map[string]any{"fld_title": title, "fld_category": "software", "fld_priority": "normal", "fld_status": "new", "fld_due": "2031-01-01", "fld_reporter": who})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, ticket.ID, rec.ID) })
	}
	get := func(path, ck string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: ck})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	code, body := get("/pages/nav_workspace_my_work", memberCookie)
	if code != http.StatusOK {
		t.Fatalf("a member opening My work = %d, want 200", code)
	}
	if !strings.Contains(body, "MyWork-mine") || strings.Contains(body, "MyWork-theirs") {
		t.Error("My work did not show the member their own ticket and only theirs")
	}
	if code, _ := get("/pages/nav_workspace_overview", memberCookie); code != http.StatusForbidden {
		t.Errorf("a member opening the admin-only Overview = %d, want 403", code)
	}
	if code, _ := get("/pages/nav_workspace_my_work", strangerCookie); code != http.StatusForbidden {
		t.Errorf("a person with no membership opening My work = %d, want 403", code)
	}
	if code, _ := get("/pages/nav_workspace_my_work", cookie); code != http.StatusOK {
		t.Errorf("an admin opening My work = %d, want 200", code)
	}
}

// TestAViewerConditionShowsALinkOnlyToAnAdmin: the Helpdesk's All tickets page links to the Workspace overview only for a
// Workspace admin, though a member holding a Helpdesk role opens the same page and sees the rest of it.
func TestAViewerConditionShowsALinkOnlyToAnAdmin(t *testing.T) {
	h, adminCookie, ctx, store, _, _, _ := routerSetupFor(t, "viewercond", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	member, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "viewercond_member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(context.Background(), wsID, member.ID, "viewercond_member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMemberAppRole(context.Background(), wsID, member.ID, "app_helpdesk", "reporter"); err != nil {
		t.Fatal(err)
	}
	memberCookie := sessionCookieValueForTest(t, config.Config{SessionSecret: "viewercond-secret"}, member.ID, 0)
	rec, err := store.CreateRecord(ctx, "mch_helpdesk_ticket", map[string]any{"fld_title": "ViewerCond-probe", "fld_category": "software", "fld_priority": "normal", "fld_status": "new", "fld_due": "2031-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_helpdesk_ticket", rec.ID) })
	const link = `href="/pages/nav_workspace_overview"`
	admin := getPage(t, h, adminCookie, "/pages/nav_helpdesk_table")
	if !strings.Contains(admin, link) || !strings.Contains(admin, "Workspace overview") {
		t.Error("an admin was not offered the Workspace overview")
	}
	plain := getPage(t, h, memberCookie, "/pages/nav_helpdesk_table")
	if strings.Contains(plain, link) || strings.Contains(plain, "Workspace overview") {
		t.Error("a member was offered a link to a page that answers them 403")
	}
	if !strings.Contains(plain, `aria-label="All tickets"`) {
		t.Error("the rest of the page was hidden from the member too")
	}
}

// Deleting a task from its own page returns to the board of the project it belonged to, not to the board of every project: the view
// group's scope (`vg_tasks`) names the Field that holds the project and the parameter the board reads it from, so the way back is read
// from that declaration. A task with no project returns to the plain board.
func TestDeletingATaskFromItsPageReturnsToItsProjectsBoard(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "deletebackscope", "default")
	var task, project *domain.Machine
	for _, m := range ws.Machines {
		switch m.ID {
		case "mch_task":
			task = m
		case "mch_project":
			project = m
		}
	}
	if task == nil || project == nil {
		t.Fatal("default installs no mch_task / mch_project")
	}
	proj, err := store.CreateRecord(ctx, project.ID, map[string]any{"fld_name": "Back-probe", "fld_status": project.Fields[1].Options[0]})
	if err != nil {
		t.Skipf("cannot make a project here: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, project.ID, proj.ID) })
	mk := func(title string, values map[string]any) *data.Record {
		values["fld_title"] = title
		values["fld_status"] = task.ReopenValue()
		rec, err := store.CreateRecord(ctx, task.ID, values)
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
		return rec
	}
	scoped, loose := mk("Back-probe-scoped", map[string]any{"fld_project": proj.ID}), mk("Back-probe-loose", map[string]any{})
	del := func(id string) string {
		tok := csrfTokenFor(t, h, "/login")
		req := httptest.NewRequest(http.MethodDelete, "/machines/"+task.ID+"/records/"+id, nil)
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "record-detail")
		req.Header.Set("HX-Current-URL", "http://x/machines/"+task.ID+"/records/"+id)
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Header().Get("HX-Redirect")
	}
	if got, want := del(scoped.ID), "/machines/"+task.ID+"?project="+proj.ID; got != want {
		t.Errorf("a task of a project went back to %q, want %q", got, want)
	}
	if got, want := del(loose.ID), "/machines/"+task.ID; got != want {
		t.Errorf("a task with no project went back to %q, want %q", got, want)
	}
}

// A record's page remembers the screen it was opened from: arriving from a list screen of its Machine (the board, the table, the
// calendar, whatever project or week they showed) puts that address on the page as `back`, the back link and a delete both return
// to it, and an address that is not one of this Workspace's screens (another site, a protocol-relative one, the record itself) is ignored.
func TestARecordPageReturnsToTheScreenItWasOpenedFrom(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "recordback", "default")
	var task *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_task" {
			task = m
		}
	}
	if task == nil {
		t.Fatal("default installs no mch_task")
	}
	mk := func(title string) *data.Record {
		rec, err := store.CreateRecord(ctx, task.ID, map[string]any{"fld_title": title, "fld_status": task.ReopenValue()})
		if err != nil {
			t.Fatalf("CreateRecord %s: %v", title, err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, task.ID, rec.ID) })
		return rec
	}
	rec := mk("Origin-probe")
	page := "/machines/" + task.ID + "/records/" + rec.ID
	get := func(target, referer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	from := "/pages/nav_task_table?project=prj_x"
	opened := get(page, "http://example.com"+from)
	if opened.Code != http.StatusSeeOther || opened.Header().Get("Location") != page+"?back="+url.QueryEscape(from) {
		t.Fatalf("opening a record from the table answered %d, Location %q", opened.Code, opened.Header().Get("Location"))
	}
	for _, bad := range []string{"http://example.com//evil.example/a", "http://example.com/machines/" + task.ID + "/records/" + rec.ID, "http://example.com/settings/files", ""} {
		if w := get(page, bad); w.Code != http.StatusOK {
			t.Errorf("a Referer of %q was taken for an origin: %d", bad, w.Code)
		}
	}
	if body := get(page+"?back="+url.QueryEscape(from), "").Body.String(); !strings.Contains(body, `href="`+from+`"`) {
		t.Error("the page does not link back to the screen it was opened from")
	}
	if body := get(page+"?back="+url.QueryEscape("//evil.example/x"), "").Body.String(); strings.Contains(body, "evil.example") {
		t.Error("an address that is not a screen of this Workspace was linked")
	}
	del := func(current string) string {
		tok := csrfTokenFor(t, h, "/login")
		req := httptest.NewRequest(http.MethodDelete, page, nil)
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "record-detail")
		req.Header.Set("HX-Current-URL", current)
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Header().Get("HX-Redirect")
	}
	gone := mk("Origin-probe-2")
	page = "/machines/" + task.ID + "/records/" + gone.ID
	if got := del("http://example.com" + page + "?back=" + url.QueryEscape("//evil.example/x")); got != "/machines/"+task.ID {
		t.Errorf("a delete followed a foreign address: %q", got)
	}
	page = "/machines/" + task.ID + "/records/" + mk("Origin-probe-3").ID
	if got := del("http://example.com" + page + "?back=" + url.QueryEscape(from)); got != from {
		t.Errorf("a delete went to %q, want the screen it was opened from, %q", got, from)
	}
}

// A project's name on the Dashboard opens that project's board: the row's id goes where the Switch project menu puts it.
func TestDashboardProjectOpensItsBoard(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "dashproject", "default")
	var project *domain.Machine
	for _, m := range ws.Machines {
		if m.ID == "mch_project" {
			project = m
		}
	}
	if project == nil {
		t.Fatal("default installs no mch_project")
	}
	proj, err := store.CreateRecord(ctx, project.ID, map[string]any{"fld_name": "Dash-probe", "fld_status": project.Fields[1].Options[0]})
	if err != nil {
		t.Skipf("cannot make a project here: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, project.ID, proj.ID) })
	body := getPage(t, h, cookie, "/pages/nav_dashboard")
	if !regexp.MustCompile(`<a href="/machines/mch_task\?project=` + proj.ID + `"[^>]*>Dash-probe</a>`).MatchString(body) {
		t.Error("the project's name on the Dashboard is not a link to its board")
	}
}
