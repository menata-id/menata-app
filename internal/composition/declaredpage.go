package composition

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/behavior"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/ir"
	"menata.app/internal/rendering"
)

// DeclaredPage builds the UI IR tree of a navigation item's `page:` (007 §15.1: declaration -> UI IR),
// resolving every Binding through the Loader and nothing else.
//
// **Cost is the number of distinct Datasets, not the number of nodes.** A Binding reads through
// `Loader.AggregateDataset`, whose `ListRecords` is memoised per request, so ten bound nodes over one Dataset
// are one read; and a tree with no Binding is no read at all. That is the property the page's query-cost test
// pins (§20: no plane may read everything and trim later). A records Binding is bounded the same way a
// `select: records` Dataset always is, by the `limit:` it is required to declare (§7.9), and **reaches the
// database once per Dataset however many nodes name it**.
//
// viewer is the acting identity. Its ID is handed to `$current_user` filters (a Dataset that filters on it and
// receives "" fails closed in `predicatesFor` rather than listing everyone's records), and the whole Actor
// answers a Form's `create` Permission: a form the viewer may not submit is not drawn.
//
// params are the request's query values (007 §9.2 `parameters`). A page declares none of them: the names are the
// `$parameters.<name>` its bound Datasets filter on, so what a request may supply is whatever the Datasets
// already say and nothing a page author could mistype. A name no bound Dataset reads is never looked at.
//
// now is the request's clock, handed in because 007 §4.6 makes the same input give the same page: it is
// the `$today` a Dataset's `where:` may compare a date against, and the only place a page can learn the date.
//
// navigation is the Application's `AllNavigation`, which a `to:` resolves against; a page is a body the
// Application declared, so its links reach that Application's own screens.
func DeclaredPage(ctx context.Context, l *Loader, viewer domain.Actor, params map[string]string, now time.Time, navigation []domain.NavigationItem, page domain.PageNode) (ir.UINode, error) {
	return ir.Lower(page, ir.Resolver{
		Route: ir.NavigationRoutes(navigation),
		Source: func(datasetID string) (string, bool) {
			ds, ok := l.Dataset(datasetID)
			return ds.Source, ok
		},
		Rows: func(b domain.PageBinding) ([]ir.Row, error) {
			return bindingRows(ctx, l, now, b)
		},
		Total: func(b domain.PageBinding) (string, error) {
			return bindingTotal(ctx, l, now, b)
		},
		Records: func(b domain.PageBinding) (ir.RecordSet, error) {
			return bindingRecords(ctx, l, viewer, params, now, b)
		},
		Form: func(b domain.PageBinding) (ir.FormSpec, error) {
			return bindingForm(l, viewer, b)
		},
	})
}

// bindingForm is one write Binding: the route a Machine is created through and the controls its Fields ask for.
// Nothing here is read from the database -- a form is the Machine's shape, not its records -- so it costs no query
// however many forms a page holds.
//
// `Permitted` is the viewer's `create` Permission evaluated against no record (there is none yet), so a
// Permission that depends on a record's own Field answers false and the form is hidden: fail closed. The create
// route evaluates the real values and refuses on its own, which is why this is courtesy and not the enforcement.
func bindingForm(l *Loader, viewer domain.Actor, b domain.PageBinding) (ir.FormSpec, error) {
	ds, ok := l.Dataset(b.Dataset)
	if !ok {
		return ir.FormSpec{}, fmt.Errorf("composition: page binding names dataset %q, which no machine declares", b.Dataset)
	}
	src := l.Machine(ds.Source)
	if src == nil {
		return ir.FormSpec{}, fmt.Errorf("composition: dataset %q names machine %q, which this Workspace does not install", b.Dataset, ds.Source)
	}
	inputs, _ := src.CreateFormInputs()
	return ir.FormSpec{
		Route:     domain.FormRoute(src.ID),
		Permitted: authorization.AllowsAction(src, domain.ActionCreate, nil, viewer),
		Inputs:    inputs,
	}, nil
}

// bindingRecords is one records Binding: each record of the Dataset, as its Machine's own Projection roles,
// with the Dataset's bound when it bit (`Selection.Truncated`, which a page may then say -- `complete: true`).
//
// The Projection is the Machine's `card_fields` (007 §7.6), so what a page can show of a record is what the
// Machine already declared about its shape, and the page names no Field. The only references it resolves are a
// person and a container (`recordRelations`): the loader refuses a `from:` whose role is any other reference
// Field, because resolving one means reading the whole related Machine. Order is the Dataset's declared `sort:`, applied by the database.
func bindingRecords(ctx context.Context, l *Loader, viewer domain.Actor, params map[string]string, now time.Time, b domain.PageBinding) (ir.RecordSet, error) {
	viewerID := viewer.ID
	ds, ok := l.Dataset(b.Dataset)
	if !ok {
		return ir.RecordSet{}, fmt.Errorf("composition: page binding names dataset %q, which no machine declares", b.Dataset)
	}
	src := l.Machine(ds.Source)
	if src == nil {
		return ir.RecordSet{}, fmt.Errorf("composition: dataset %q reads machine %q, which this Workspace does not install", b.Dataset, ds.Source)
	}
	// A Dataset filtering on a parameter this request did not send lists nothing and reads nothing: fail closed
	// (007 §9.2) without an error, because an absent query value is an ordinary request and not a fault. The
	// page's `empty:` words are what the viewer sees, so an author writes them for this case too.
	where := expression.Context{CurrentUser: viewerID, Parameters: params, Today: now.Format("2006-01-02")}
	for _, c := range ds.Where.Comparisons() {
		if strings.HasPrefix(c.Value, expression.SentinelParameterPrefix) {
			if _, ok := where.Resolve(c.Value); !ok {
				return ir.RecordSet{}, nil
			}
		}
	}
	// SelectRelated, not SelectDataset: the Relations the Dataset declares (007 §7.5) are how a record's child
	// count is known, and the lookup is one bounded query per Relation however many records there are.
	sel, err := l.SelectRelated(ctx, b.Dataset, where)
	if err != nil {
		return ir.RecordSet{}, err
	}
	records := sel.Records
	people, err := recordRelations(ctx, l, src, records)
	if err != nil {
		return ir.RecordSet{}, err
	}
	out := make([]map[string]string, 0, len(records))
	edits := make([]ir.FormSpec, 0, len(records))
	deletes := make([]ir.RecordAction, 0, len(records))
	moves := make([]ir.RecordMove, 0, len(records))
	transitions := make([]ir.RecordTransition, 0, len(records))
	for i, r := range records {
		item := ProjectedByRole(src, r, people)
		item[domain.PageRecordRole] = RecordRoute(src.ID, r.ID)
		edits = append(edits, recordEditForm(src, r, viewer))
		deletes = append(deletes, recordDeleteAction(src, r, viewer))
		moves = append(moves, recordMove(src, r, viewer, i > 0, i < len(records)-1 || sel.Truncated))
		transitions = append(transitions, recordTransition(src, r, viewer))
		for _, rel := range ds.Relations {
			item[domain.PageCountRole(rel.ID)] = strconv.Itoa(len(sel.Related(rel.ID, r.ID)))
		}
		out = append(out, item)
	}
	set := ir.RecordSet{Records: out, Edits: edits, Deletes: deletes, Moves: moves, Transitions: transitions, Truncated: sel.Truncated, Limit: sel.Limit}
	if src.Completion != nil {
		set.Done, set.Reopen = src.Completion.Done, src.ReopenValue()
	}
	return set, nil
}

// recordRelations is what the Projection resolves a reference through for the listed records: the display name of
// every person they name in a `person` Field, and the title of every record they name in a `container` Field.
//
// People ask the Loader's memoized PersonNames (one read of the Workspace's members however many records there
// are, and none at all when the Machine declares no such Field) and keep only those actually listed, so the options
// grow with the page and not the Workspace. A container asks Loader.RelatedLabels for exactly the ids listed, one
// statement per related Machine. Neither reads a whole Machine.
//
// A reference to someone or something that no longer exists gets an option with an empty label, not none: an
// absent option makes RelationLabel fall back to the raw id, and an id is not something a page should print.
func recordRelations(ctx context.Context, l *Loader, src *domain.Machine, records []*data.Record) (rendering.RelationOptions, error) {
	people, containers := map[string]string{}, map[string]string{}
	for _, cf := range src.CardFields {
		f, ok := src.FieldByID(cf.Field)
		if !ok || !f.IsReference() {
			continue
		}
		switch {
		case f.RelatedMachine == domain.UserMachineID:
			people[f.ID] = f.RelatedMachine
		case cf.Role == domain.CardFieldRoleContainer:
			containers[f.ID] = f.RelatedMachine
		}
	}
	out := rendering.RelationOptions{}
	if len(people) > 0 {
		names, err := l.PersonNames(ctx)
		if err != nil {
			return nil, err
		}
		out[domain.UserMachineID] = listedOptions(records, people, names)
	}
	targets := map[string]bool{}
	for _, machineID := range containers {
		targets[machineID] = true
	}
	for _, machineID := range slices.Sorted(maps.Keys(targets)) {
		fields := map[string]string{}
		var ids []string
		for fieldID, target := range containers {
			if target != machineID {
				continue
			}
			fields[fieldID] = target
			for _, r := range records {
				if id := DisplayString(r.Values[fieldID]); id != "" {
					ids = append(ids, id)
				}
			}
		}
		labels, err := l.RelatedLabels(ctx, machineID, ids)
		if err != nil {
			return nil, err
		}
		out[machineID] = listedOptions(records, fields, labels)
	}
	return out, nil
}

// listedOptions is one option per distinct id the records name in any of the given Fields, labelled from labels
// (empty when the id has none), in the order the records list them.
func listedOptions(records []*data.Record, fields map[string]string, labels map[string]string) []rendering.RelationOption {
	seen := map[string]bool{}
	var options []rendering.RelationOption
	for _, r := range records {
		for _, fieldID := range slices.Sorted(maps.Keys(fields)) {
			id := DisplayString(r.Values[fieldID])
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			options = append(options, rendering.RelationOption{ID: id, Label: labels[id]})
		}
	}
	return options
}

// recordEditForm is what an `update` Form inside a records template needs of one record: the route that patches
// it, the controls starting as its current values, and whether this viewer may edit *it*.
//
// `Permitted` evaluates the Machine's `edit` Permission against this record's own values, so a Permission that
// reads a Field of the record answers per record. It is a courtesy -- the patch route runs the same check, the
// state model, the write guards and the Events -- and an append-only Machine is never offered a form, because
// its route refuses every write. It costs no query: the record is already in hand.
func recordEditForm(src *domain.Machine, r *data.Record, viewer domain.Actor) ir.FormSpec {
	return ir.FormSpec{
		Route:     RecordRoute(src.ID, r.ID),
		Method:    domain.FormMethodPatch,
		Permitted: !src.AppendOnly && authorization.AllowsAction(src, domain.ActionEdit, r.Values, viewer),
		Inputs:    src.EditFormInputs(r.Values),
	}
}

// recordDeleteAction is what a `delete` Button inside a records template needs of one record: the route that
// deletes it and whether this viewer may. `Permitted` is the delete route's own two checks evaluated in hand --
// the Machine's `delete` Permission against this record's values, and `action.CanDelete`'s business-state rule --
// and an append-only Machine is never offered one. It is a courtesy, as recordEditForm's is: the route runs the
// same checks again, and for a Document, whose rule also reads its Approval Steps, the route is the stricter of
// the two (this passes none, which CanDelete answers from the status alone).
func recordDeleteAction(src *domain.Machine, r *data.Record, viewer domain.Actor) ir.RecordAction {
	deletable, _ := action.CanDelete(src, r.Values, nil)
	return ir.RecordAction{
		Route:     RecordRoute(src.ID, r.ID),
		Permitted: !src.AppendOnly && deletable && authorization.AllowsAction(src, domain.ActionDelete, r.Values, viewer),
	}
}

// recordMove is what a `move` Button inside a records template needs of one record. `hasPrev`/`hasNext` say
// whether the list has a record on that side; a list cut short by its `limit:` always has a next, because the
// records past the cut are still in the Machine's order. Moving is an edit of the record's place, so it asks the
// `edit` Permission, and an append-only Machine is never offered it. A courtesy as the others: the move route
// asks again and computes the neighbour itself.
func recordMove(src *domain.Machine, r *data.Record, viewer domain.Actor, hasPrev, hasNext bool) ir.RecordMove {
	permitted := !src.AppendOnly && authorization.AllowsAction(src, domain.ActionEdit, r.Values, viewer)
	return ir.RecordMove{Route: RecordRoute(src.ID, r.ID), Up: permitted && hasPrev, Down: permitted && hasNext}
}

// recordTransition is what a `transition` Button inside a records template needs of one record: the route that
// patches it, the Field it sets (the Machine's `status` Projection role, so a page names no Field), the two values
// `$done` and `$reopen` stand for (the Machine's `completion:`), and, for any target, whether moving *this* record
// there is a change worth offering. A target equal to the current value is not; one the state model's declared edges
// refuse (`behavior.CheckTransitions` as the `edit` route calls it) is not; and neither is one this viewer's `edit`
// Permission, read against this record's own values, would refuse, nor any on an append-only Machine. A courtesy, as
// the others: the patch route runs every one of these checks again.
func recordTransition(src *domain.Machine, r *data.Record, viewer domain.Actor) ir.RecordTransition {
	field := src.CardFieldFor(domain.CardFieldRoleStatus)
	t := ir.RecordTransition{Route: RecordRoute(src.ID, r.ID), Field: field, Reopen: src.ReopenValue()}
	if src.Completion != nil {
		t.Done = src.Completion.Done
	}
	f, ok := src.FieldByID(field)
	if !ok {
		return t
	}
	editable := !src.AppendOnly && authorization.AllowsAction(src, domain.ActionEdit, r.Values, viewer)
	current := DisplayString(r.Values[field])
	t.Allows = func(target string) (bool, bool) {
		if !slices.Contains(f.Options, target) {
			return false, false
		}
		if !editable || target == current {
			return false, true
		}
		return behavior.CheckTransitions(src, domain.ActionEdit, r.Values, map[string]any{field: target}) == nil, true
	}
	return t
}

// RecordRoute is the runtime's generic route to one record (`GET /machines/{machineID}/records/{id}`, which
// renders the full detail page unless the request is htmx). It is the runtime's own, like `/home`, and not an
// Application's, so it is assembled here and not read from `navigation:`. Opening it still goes through that
// route's own authorization: a link is an address, not a grant.
func RecordRoute(machineID, recordID string) string {
	return "/machines/" + machineID + "/records/" + recordID
}

// bindingRows is one Binding's rows: the Dataset's Dimension values, each with the Measure's number.
//
// **The order is a decision, not an accident of a map.** Declared option order first (a status Field lists
// its values in the order its workflow reads), then any value no option names, sorted -- a Go map ranges in a
// different order every run, and 007 §4.6 makes the same input rendering differently a MUST NOT. A declared
// option with no record is a row of 0 rather than an absent one: "no document is rejected" is an answer, and
// a count that vanishes when it reaches zero makes the page's shape depend on the data.
func bindingRows(ctx context.Context, l *Loader, now time.Time, b domain.PageBinding) ([]ir.Row, error) {
	ds, ok := l.Dataset(b.Dataset)
	if !ok {
		return nil, fmt.Errorf("composition: page binding names dataset %q, which no machine declares", b.Dataset)
	}
	if ds.Dimension == "" {
		return nil, fmt.Errorf("composition: dataset %q has no dimension to expand", b.Dataset)
	}
	agg, err := l.AggregateDataset(ctx, b.Dataset, aggregateContext(now))
	if err != nil {
		return nil, err
	}
	var dim domain.Field
	if src := l.Machine(ds.Source); src != nil {
		dim, _ = src.FieldByID(ds.Dimension)
	}
	if dim.IsReference() {
		return referenceRows(ctx, l, dim, agg, b.Measure)
	}
	var order []string
	seen := map[string]bool{}
	for _, o := range dim.Options {
		if !seen[o] {
			seen[o] = true
			order = append(order, o)
		}
	}
	var rest []string
	for k := range agg.ByDimension {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	order = append(order, rest...)

	rows := make([]ir.Row, 0, len(order))
	for _, k := range order {
		if k == "" {
			continue // records with the Dimension unset have no label to show
		}
		rows = append(rows, ir.Row{Label: k, Value: formatMeasure(agg.ByDimension[k][b.Measure])})
	}
	return rows, nil
}

// referenceRows is a Dimension over a reference Field (a Project, a person): its stored value is a record id, so
// each row is named by that record's title -- the label a picker offers for the same record (`optionsOf`) -- and
// keeps the id as its Key. Ordered by title, then id, since an id carries no order worth showing and §4.6 forbids
// leaving it to map iteration. A value naming no record that exists (one deleted since) has no title to show and is
// left out, as an unset one is, rather than drawn as an opaque id.
func referenceRows(ctx context.Context, l *Loader, dim domain.Field, agg Aggregation, measure string) ([]ir.Row, error) {
	options, _, err := l.optionsOf(ctx, dim.RelatedMachine)
	if err != nil {
		return nil, err
	}
	titles := make(map[string]string, len(options))
	for _, o := range options {
		titles[o.ID] = o.Label
	}
	var keys []string
	for k := range agg.ByDimension {
		if titles[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if titles[keys[i]] != titles[keys[j]] {
			return titles[keys[i]] < titles[keys[j]]
		}
		return keys[i] < keys[j]
	})
	rows := make([]ir.Row, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, ir.Row{Label: titles[k], Key: k, Value: formatMeasure(agg.ByDimension[k][measure])})
	}
	return rows, nil
}

// bindingTotal is one `rows: total` Binding: the Measure's figure over every record the Dataset reads.
func bindingTotal(ctx context.Context, l *Loader, now time.Time, b domain.PageBinding) (string, error) {
	agg, err := l.AggregateDataset(ctx, b.Dataset, aggregateContext(now))
	if err != nil {
		return "", err
	}
	v, ok := agg.Total[b.Measure]
	if !ok {
		return "", fmt.Errorf("composition: dataset %q declares no measure %q", b.Dataset, b.Measure)
	}
	return formatMeasure(v), nil
}

// aggregateContext is what a Measure's `where:` may resolve: the request's date and nothing else. An aggregate
// has no viewer and no request parameters, and metadata refuses a Measure that names either.
func aggregateContext(now time.Time) expression.Context {
	return expression.Context{Today: now.Format("2006-01-02")}
}

func formatMeasure(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
