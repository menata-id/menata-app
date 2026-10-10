package composition

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/ir"
)

// testToday is the fixed clock every page in this file is composed at.
var testToday = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

// declaredPageLoader seeds `counts` records per status into a throwaway Machine, so a Binding has something
// to count. Statuses absent from `counts` get no record at all, which is the case row ordering has to handle.
func declaredPageLoader(t *testing.T, suffix string, counts map[string]int) (*Loader, context.Context) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run the declared-page integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	machineID := "mch_declared_page_" + suffix
	store := data.NewStore(pool)
	ctx := data.WithWorkspaceScope(context.Background(), "ws_declared_page_test")
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM records WHERE machine_id = $1`, machineID); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	for status, n := range counts {
		for range n {
			if _, err := store.CreateRecord(ctx, machineID, map[string]any{"fld_status": status}); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
	}
	m := &domain.Machine{
		ID:     machineID,
		Fields: []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"draft", "in_review", "approved", "rejected"}}},
		Datasets: []domain.Dataset{{
			ID: "ds_by_status", Source: machineID, Dimension: "fld_status",
			Measures: []domain.Measure{{ID: "msr_total", Aggregate: domain.AggregateCount}},
		}},
	}
	return NewLoader(store, map[string]*domain.Machine{machineID: m}), ctx
}

func boundMetricGrid(n int) domain.PageNode {
	root := domain.PageNode{Kind: "layout", Type: "grid", Props: map[string]string{"gap": "default"}}
	for range n {
		root.Children = append(root.Children, domain.PageNode{
			Kind: "component", Type: "Metric",
			Binding: &domain.PageBinding{Dataset: "ds_by_status", Measure: "msr_total", Rows: domain.PageRowsDimension},
		})
	}
	return root
}

func declaredRowsOf(tree ir.UINode) (out [][2]string) {
	for _, c := range tree.Children {
		out = append(out, [2]string{c.Props["label"], c.Props["value"]})
	}
	return out
}

// Declared option order, a declared option with no record as an explicit 0, and a value no option names last.
func TestDeclaredPage_rowsFollowOptionOrderAndKeepEmptyOptions(t *testing.T) {
	l, ctx := declaredPageLoader(t, "order", map[string]int{"approved": 2, "draft": 1, "legacy": 3})
	tree, err := DeclaredPage(ctx, l, domain.Actor{}, nil, testToday, nil, boundMetricGrid(1))
	if err != nil {
		t.Fatalf("DeclaredPage: %v", err)
	}
	want := [][2]string{{"draft", "1"}, {"in_review", "0"}, {"approved", "2"}, {"rejected", "0"}, {"legacy", "3"}}
	if got := declaredRowsOf(tree); !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

// A map ranges differently every run (007 §4.6), so one call passing proves little: 40 fresh loaders.
func TestDeclaredPage_rowOrderIsDeterministic(t *testing.T) {
	l, ctx := declaredPageLoader(t, "determinism", map[string]int{"zeta": 1, "alpha": 1, "mid": 1, "approved": 1})
	first, err := DeclaredPage(ctx, l, domain.Actor{}, nil, testToday, nil, boundMetricGrid(1))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 40 {
		l2 := NewLoader(l.store, l.machines)
		again, err := DeclaredPage(ctx, l2, domain.Actor{}, nil, testToday, nil, boundMetricGrid(1))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(declaredRowsOf(first), declaredRowsOf(again)) {
			t.Fatalf("render %d differs: %v vs %v", i, declaredRowsOf(first), declaredRowsOf(again))
		}
	}
}

// Cost is the number of distinct Datasets, never the number of nodes: ten bound nodes over one Dataset are one
// read, and an unbound tree is none.
func TestDeclaredPage_queryCostIsFlatInNodeCount(t *testing.T) {
	l1, ctx1 := declaredPageLoader(t, "cost1", map[string]int{"draft": 1})
	if _, err := DeclaredPage(ctx1, l1, domain.Actor{}, nil, testToday, nil, boundMetricGrid(1)); err != nil {
		t.Fatal(err)
	}
	l10, ctx10 := declaredPageLoader(t, "cost10", map[string]int{"draft": 1})
	if _, err := DeclaredPage(ctx10, l10, domain.Actor{}, nil, testToday, nil, boundMetricGrid(10)); err != nil {
		t.Fatal(err)
	}
	if l1.Reads() != 1 || l10.Reads() != 1 {
		t.Errorf("reads = %d for one bound node and %d for ten; want 1 and 1", l1.Reads(), l10.Reads())
	}
	lNone, ctxNone := declaredPageLoader(t, "cost0", nil)
	static := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}}
	if _, err := DeclaredPage(ctxNone, lNone, domain.Actor{}, nil, testToday, nil, static); err != nil {
		t.Fatal(err)
	}
	if lNone.Reads() != 0 {
		t.Errorf("an unbound page read %d times; want 0", lNone.Reads())
	}
}

// recordListLoader seeds one record per title into a throwaway Machine that projects `title` and `status`, and
// declares a `select: records` Dataset over it ordered by title with the given limit.
func recordListLoader(t *testing.T, suffix string, titles []string, limit int) (*Loader, context.Context) {
	t.Helper()
	l, ctx := declaredPageLoader(t, suffix, nil)
	machineID := "mch_declared_page_" + suffix
	for _, title := range titles {
		if _, err := l.store.CreateRecord(ctx, machineID, map[string]any{"fld_title": title, "fld_status": "draft"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := &domain.Machine{
		ID: machineID,
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"draft", "approved"}},
		},
		CardFields: []domain.CardField{
			{Field: "fld_title", Role: domain.CardFieldRoleTitle},
			{Field: "fld_status", Role: domain.CardFieldRoleStatus},
		},
		Datasets: []domain.Dataset{{
			ID: "ds_recent", Source: machineID, Select: domain.SelectRecords, Limit: limit,
			Sort: []domain.SortKey{{Field: "fld_title"}},
		}},
	}
	return NewLoader(l.store, map[string]*domain.Machine{machineID: m}), ctx
}

func recordList() domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{
		Kind: "component", Type: "Collection", Props: map[string]string{"gap": "tight"},
		Binding: &domain.PageBinding{Dataset: "ds_recent", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{Kind: "layout", Type: "row", Children: []domain.PageNode{
			{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}},
			{Kind: "component", Type: "StatusBadge", Props: map[string]string{"tone": "info"}, From: map[string]string{"label": "status"}},
		}}},
	}}}
}

func itemTexts(tree ir.UINode) (out []string) {
	for _, item := range tree.Children[0].Children {
		out = append(out, item.Children[0].Props["text"]+"/"+item.Children[1].Props["label"])
	}
	return out
}

// Items follow the Dataset's own `sort:` (applied by the database), carry the Machine's Projection and not a
// Field id, and the limit bounds the list.
func TestDeclaredPage_recordsFollowTheDatasetSortAndLimit(t *testing.T) {
	l, ctx := recordListLoader(t, "reclist", []string{"charlie", "alpha", "bravo", "delta"}, 3)
	tree, err := DeclaredPage(ctx, l, domain.Actor{}, nil, testToday, nil, recordList())
	if err != nil {
		t.Fatalf("DeclaredPage: %v", err)
	}
	want := []string{"alpha/draft", "bravo/draft", "charlie/draft"}
	if got := itemTexts(tree); !reflect.DeepEqual(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
}

// 007 §20: the page reads the Dataset once however many records it holds, and once however many nodes name it.
func TestDeclaredPage_recordsQueryCostIsFlatInRecordCount(t *testing.T) {
	few, ctxFew := recordListLoader(t, "reccost1", []string{"a"}, 50)
	if _, err := DeclaredPage(ctxFew, few, domain.Actor{}, nil, testToday, nil, recordList()); err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 30)
	for i := range titles {
		titles[i] = string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	many, ctxMany := recordListLoader(t, "reccost30", titles, 50)
	if _, err := DeclaredPage(ctxMany, many, domain.Actor{}, nil, testToday, nil, recordList()); err != nil {
		t.Fatal(err)
	}
	if few.Reads() != many.Reads() || many.Reads() != 1 {
		t.Errorf("reads = %d for one record and %d for thirty; want 1 and 1", few.Reads(), many.Reads())
	}
}

// A role the Machine does not project is a fault of the declaration surfaced by Lower, not a blank cell.
func TestDeclaredPage_recordsRefuseARoleTheMachineDoesNotProject(t *testing.T) {
	l, ctx := recordListLoader(t, "recrole", []string{"a"}, 5)
	page := recordList()
	page.Children[0].Children[0].Children[0].From = map[string]string{"text": "money"}
	if _, err := DeclaredPage(ctx, l, domain.Actor{}, nil, testToday, nil, page); err == nil {
		t.Fatal("DeclaredPage accepted a from: role the Machine does not project")
	}
}

// A per-viewer Dataset fails closed for a page that cannot say who is viewing, rather than listing everyone's.
func TestDeclaredPage_recordsOnAPerViewerDatasetNeedTheViewer(t *testing.T) {
	l, ctx := recordListLoader(t, "recviewer", []string{"a", "b"}, 5)
	ds := &l.machines["mch_declared_page_recviewer"].Datasets[0]
	ds.Where = &expression.Predicate{All: []expression.Comparison{{Field: "fld_title", Op: expression.OpEquals, Value: expression.SentinelCurrentUser}}}
	if _, err := DeclaredPage(ctx, l, domain.Actor{}, nil, testToday, nil, recordList()); err == nil {
		t.Fatal("a $current_user Dataset answered for an unknown viewer")
	}
	tree, err := DeclaredPage(ctx, NewLoader(l.store, l.machines), domain.Actor{ID: "a"}, nil, testToday, nil, recordList())
	if err != nil {
		t.Fatal(err)
	}
	if got := itemTexts(tree); !reflect.DeepEqual(got, []string{"a/draft"}) {
		t.Errorf("items = %v, want only the viewer's", got)
	}
}

// A page that claims its list is complete is told by the Data Plane whether the Dataset's own `limit:` bit,
// through the real loader and a real database: four records under a limit of three is cut, under five is not.
func TestDeclaredPage_completeNamesTheDatasetsBoundOnlyWhenItBit(t *testing.T) {
	for name, c := range map[string]struct {
		limit    int
		complete string
		want     string
	}{
		"cut and claimed":     {3, "true", "3"},
		"not cut and claimed": {5, "true", ""},
		"cut, no claim":       {3, "", ""},
	} {
		l, ctx := recordListLoader(t, "complete"+strings.ReplaceAll(name, " ", ""), []string{"charlie", "alpha", "bravo", "delta"}, c.limit)
		page := recordList()
		if c.complete != "" {
			page.Children[0].Props["complete"] = c.complete
		}
		tree, err := DeclaredPage(ctx, l, domain.Actor{}, nil, testToday, nil, page)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := tree.Children[0].Props["truncated"]; got != c.want {
			t.Errorf("%s: truncated = %q, want %q", name, got, c.want)
		}
	}
}

// A Dataset filtering on `$parameters.<name>` takes the value from the request's query, fails closed without
// it (no records AND no read -- an absent value is an ordinary request, not a fault, and not "everything"),
// and compares the supplied value as data: a value that is not a status lists nothing.
func TestDeclaredPage_recordsFilterOnARequestParameterAndFailClosedWithout(t *testing.T) {
	l, ctx := recordListLoader(t, "recparam", []string{"alpha", "bravo"}, 5)
	if _, err := l.store.CreateRecord(ctx, "mch_declared_page_recparam", map[string]any{"fld_title": "charlie", "fld_status": "approved"}); err != nil {
		t.Fatal(err)
	}
	ds := &l.machines["mch_declared_page_recparam"].Datasets[0]
	ds.Where = &expression.Predicate{All: []expression.Comparison{{Field: "fld_status", Op: expression.OpEquals, Value: expression.SentinelParameterPrefix + "status"}}}

	for name, c := range map[string]struct {
		params map[string]string
		want   []string
		reads  int
	}{
		"sent":            {map[string]string{"status": "approved"}, []string{"charlie/approved"}, 1},
		"sent, no match":  {map[string]string{"status": "x' OR '1'='1"}, nil, 1},
		"absent":          {nil, nil, 0},
		"empty":           {map[string]string{"status": ""}, nil, 0},
		"other name only": {map[string]string{"state": "approved"}, nil, 0},
	} {
		fresh := NewLoader(l.store, l.machines)
		tree, err := DeclaredPage(ctx, fresh, domain.Actor{}, c.params, testToday, nil, recordList())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := itemTexts(tree); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: items = %v, want %v", name, got, c.want)
		}
		if fresh.Reads() != c.reads {
			t.Errorf("%s: reads = %d, want %d", name, fresh.Reads(), c.reads)
		}
	}
}

// An `update` Form's permission is answered per record, from the record already in hand: a Permission naming an
// actor Field lets the owner of one record edit it and not the next, and an append-only Machine is offered no form
// at all. The route and the starting values are the record's own.
func TestRecordEditFormIsPerRecordAndRefusesAppendOnly(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_note",
		Fields: []domain.Field{
			{ID: "fld_title", Type: domain.FieldTypeText, Required: true},
			{ID: "fld_owner", Type: domain.FieldTypePerson},
		},
		Permissions: []domain.Permission{{ID: "perm_edit", Action: domain.ActionEdit, ActorField: "fld_owner"}},
	}
	mine := &data.Record{ID: "rec_mine", Values: map[string]any{"fld_title": "Mine", "fld_owner": "usr_me"}}
	theirs := &data.Record{ID: "rec_theirs", Values: map[string]any{"fld_title": "Theirs", "fld_owner": "usr_other"}}
	viewer := domain.Actor{ID: "usr_me"}

	a, b := recordEditForm(m, mine, viewer), recordEditForm(m, theirs, viewer)
	if !a.Permitted || b.Permitted {
		t.Errorf("Permitted = %v / %v, want true for the viewer's own record and false for another's", a.Permitted, b.Permitted)
	}
	if a.Route != "/machines/mch_note/records/rec_mine" || a.Method != domain.FormMethodPatch {
		t.Errorf("route/method = %q %q", a.Route, a.Method)
	}
	if len(a.Inputs) == 0 || a.Inputs[0].Default != "Mine" {
		t.Errorf("inputs do not start as the record's own values: %+v", a.Inputs)
	}

	m.AppendOnly = true
	if recordEditForm(m, mine, viewer).Permitted {
		t.Error("an append-only Machine was offered an edit form")
	}
}

// A `delete` Button's permission is answered per record from the record in hand, by the delete route's own two
// checks: the Machine's `delete` Permission against the record's values and `action.CanDelete`'s business-state
// rule. An append-only Machine is offered none. The route is the record's own generic one.
func TestRecordDeleteActionIsPerRecordAndRefusesAppendOnly(t *testing.T) {
	m := &domain.Machine{
		ID:          "mch_note",
		Fields:      []domain.Field{{ID: "fld_title", Type: domain.FieldTypeText}, {ID: "fld_owner", Type: domain.FieldTypePerson}},
		Permissions: []domain.Permission{{ID: "perm_delete", Action: domain.ActionDelete, ActorField: "fld_owner"}},
	}
	mine := &data.Record{ID: "rec_mine", Values: map[string]any{"fld_owner": "usr_me"}}
	theirs := &data.Record{ID: "rec_theirs", Values: map[string]any{"fld_owner": "usr_other"}}
	viewer := domain.Actor{ID: "usr_me"}

	a, b := recordDeleteAction(m, mine, viewer), recordDeleteAction(m, theirs, viewer)
	if !a.Permitted || b.Permitted {
		t.Errorf("Permitted = %v / %v, want true for the viewer's own record and false for another's", a.Permitted, b.Permitted)
	}
	if a.Route != "/machines/mch_note/records/rec_mine" {
		t.Errorf("route = %q", a.Route)
	}
	m.AppendOnly = true
	if recordDeleteAction(m, mine, viewer).Permitted {
		t.Error("an append-only Machine was offered a delete")
	}
}

// A `move` Button is offered per record: never on the end the list has nothing past, only to a viewer who may edit
// *that* record, and never for an append-only Machine.
func TestRecordMoveIsPerRecordAndRefusesAppendOnly(t *testing.T) {
	m := &domain.Machine{
		ID:          "mch_note",
		Fields:      []domain.Field{{ID: "fld_title", Type: domain.FieldTypeText}, {ID: "fld_owner", Type: domain.FieldTypePerson}},
		Permissions: []domain.Permission{{ID: "perm_edit", Action: domain.ActionEdit, ActorField: "fld_owner"}},
	}
	mine := &data.Record{ID: "rec_mine", Values: map[string]any{"fld_owner": "usr_me"}}
	theirs := &data.Record{ID: "rec_theirs", Values: map[string]any{"fld_owner": "usr_other"}}
	viewer := domain.Actor{ID: "usr_me"}

	a := recordMove(m, mine, viewer, true, true)
	if !a.Up || !a.Down || a.Route != "/machines/mch_note/records/rec_mine" {
		t.Errorf("own record in the middle: %+v", a)
	}
	if b := recordMove(m, theirs, viewer, true, true); b.Up || b.Down {
		t.Errorf("another's record was offered a move: %+v", b)
	}
	if c := recordMove(m, mine, viewer, false, true); c.Up || !c.Down {
		t.Errorf("first record: %+v, want no Up", c)
	}
	if d := recordMove(m, mine, viewer, true, false); !d.Up || d.Down {
		t.Errorf("last record: %+v, want no Down", d)
	}
	m.AppendOnly = true
	if e := recordMove(m, mine, viewer, true, true); e.Up || e.Down {
		t.Errorf("an append-only Machine was offered a move: %+v", e)
	}
}

func totalMetric(measure, label, tone string) domain.PageNode {
	props := map[string]string{"label": label}
	if tone != "" {
		props["tone"] = tone
	}
	return domain.PageNode{Kind: "component", Type: "Metric", Props: props,
		Binding: &domain.PageBinding{Dataset: "ds_by_status", Measure: measure, Rows: domain.PageRowsTotal}}
}

// A total is the Measure over every record, filtered by the Measure's own conjunction; a zero draws no tone.
func TestDeclaredPage_totalsFollowTheMeasureAndDropToneOnZero(t *testing.T) {
	l, ctx := declaredPageLoader(t, "totals", map[string]int{"draft": 2, "in_review": 1, "approved": 3})
	m := l.machines["mch_declared_page_totals"]
	m.Datasets[0].Measures = append(m.Datasets[0].Measures,
		domain.Measure{ID: "msr_open", Aggregate: domain.AggregateCount, Where: &expression.Predicate{All: []expression.Comparison{
			{Field: "fld_status", Op: expression.OpNotEquals, Value: "approved"},
			{Field: "fld_status", Op: expression.OpNotEquals, Value: "rejected"},
		}}},
		domain.Measure{ID: "msr_rejected", Aggregate: domain.AggregateCount, Where: &expression.Predicate{All: []expression.Comparison{
			{Field: "fld_status", Op: expression.OpEquals, Value: "rejected"},
		}}})
	root := domain.PageNode{Kind: "layout", Type: "grid", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{
		totalMetric("msr_total", "All", ""),
		totalMetric("msr_open", "Open", "warn"),
		totalMetric("msr_rejected", "Rejected", "bad"),
	}}
	tree, err := DeclaredPage(ctx, l, domain.Actor{}, nil, testToday, nil, root)
	if err != nil {
		t.Fatalf("DeclaredPage: %v", err)
	}
	type fig struct{ label, value, tone string }
	var got []fig
	for _, c := range tree.Children {
		got = append(got, fig{c.Props["label"], c.Props["value"], c.Props["tone"]})
	}
	want := []fig{{"All", "6", ""}, {"Open", "3", "warn"}, {"Rejected", "0", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("figures = %v, want %v", got, want)
	}
}

// `$today` in a Measure is the request's injected date: the same records answer differently on a different day.
func TestDeclaredPage_aTotalResolvesTodayFromTheInjectedClock(t *testing.T) {
	l, ctx := declaredPageLoader(t, "totals_today", map[string]int{"draft": 1})
	m := l.machines["mch_declared_page_totals_today"]
	m.Fields = append(m.Fields, domain.Field{ID: "fld_due", Type: domain.FieldTypeDate})
	m.Datasets[0].Measures = append(m.Datasets[0].Measures, domain.Measure{ID: "msr_past", Aggregate: domain.AggregateCount,
		Where: &expression.Predicate{All: []expression.Comparison{{Field: "fld_due", Op: expression.OpLessThan, Value: expression.SentinelToday}}}})
	store := l.store
	if _, err := store.CreateRecord(ctx, m.ID, map[string]any{"fld_status": "draft", "fld_due": "2026-10-05"}); err != nil {
		t.Fatal(err)
	}
	root := domain.PageNode{Kind: "layout", Type: "grid", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{totalMetric("msr_past", "Past", "")}}
	value := func(now time.Time) string {
		tree, err := DeclaredPage(ctx, NewLoader(l.store, l.machines), domain.Actor{}, nil, now, nil, root)
		if err != nil {
			t.Fatal(err)
		}
		return tree.Children[0].Props["value"]
	}
	if got := value(testToday); got != "1" {
		t.Errorf("on %s the past-due count = %s, want 1", testToday.Format("2006-01-02"), got)
	}
	if got := value(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)); got != "0" {
		t.Errorf("a week earlier the past-due count = %s, want 0", got)
	}
}

// A `transition` Button is offered per record: only toward an option of the status Field, never toward the value the
// record already holds, never where the Machine's declared edges refuse the move, only to a viewer who may edit
// *that* record, and never for an append-only Machine. The Field is the Machine's `status` role and `$done` is its
// `completion:`, so the page names neither.
func TestRecordTransitionIsPerRecordAndFollowsTheStateModel(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_note",
		Fields: []domain.Field{
			{ID: "fld_title", Type: domain.FieldTypeText},
			{ID: "fld_owner", Type: domain.FieldTypePerson},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "doing", "done"}, Default: "todo"},
		},
		CardFields:  []domain.CardField{{Field: "fld_status", Role: domain.CardFieldRoleStatus}},
		Completion:  &domain.Completion{Field: "fld_status", Done: "done"},
		Permissions: []domain.Permission{{ID: "perm_edit", Action: domain.ActionEdit, ActorField: "fld_owner"}},
		Transitions: []domain.Transition{
			{ID: "t1", Field: "fld_status", From: "todo", To: "doing", Action: domain.ActionEdit},
			{ID: "t2", Field: "fld_status", From: "doing", To: "done", Action: domain.ActionEdit},
		},
	}
	todo := &data.Record{ID: "rec_a", Values: map[string]any{"fld_owner": "usr_me", "fld_status": "todo"}}
	doing := &data.Record{ID: "rec_b", Values: map[string]any{"fld_owner": "usr_me", "fld_status": "doing"}}
	theirs := &data.Record{ID: "rec_c", Values: map[string]any{"fld_owner": "usr_other", "fld_status": "todo"}}
	viewer := domain.Actor{ID: "usr_me"}

	a := recordTransition(m, todo, viewer)
	if a.Route != "/machines/mch_note/records/rec_a" || a.Field != "fld_status" || a.Done != "done" || a.Reopen != "todo" {
		t.Errorf("answer = %+v", a)
	}
	check := func(label string, tr interface{ Allows(string) (bool, bool) }, target string, wantAllowed, wantKnown bool) {
	}
	_ = check
	cases := []struct {
		name           string
		rec            *data.Record
		target         string
		allowed, known bool
	}{
		{"declared edge", todo, "doing", true, true},
		{"the value it already holds", todo, "todo", false, true},
		{"an edge the state model does not declare", todo, "done", false, true},
		{"the next edge", doing, "done", true, true},
		{"not an option", todo, "blocked", false, false},
		{"another's record", theirs, "doing", false, true},
	}
	for _, tc := range cases {
		got, known := recordTransition(m, tc.rec, viewer).Allows(tc.target)
		if got != tc.allowed || known != tc.known {
			t.Errorf("%s: Allows(%q) = %v, %v; want %v, %v", tc.name, tc.target, got, known, tc.allowed, tc.known)
		}
	}
	m.AppendOnly = true
	if got, _ := recordTransition(m, doing, viewer).Allows("done"); got {
		t.Error("an append-only Machine was offered a transition")
	}
}
