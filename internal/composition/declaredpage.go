package composition

import (
	"context"
	"fmt"
	"sort"
	"strconv"

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
// viewerID is the viewing identity's own record id, handed to `$current_user` filters. A Dataset that filters
// on it and receives "" fails closed in `predicatesFor` rather than listing everyone's records.
//
// navigation is the Application's `AllNavigation`, which a `to:` resolves against; a page is a body the
// Application declared, so its links reach that Application's own screens.
func DeclaredPage(ctx context.Context, l *Loader, viewerID string, navigation []domain.NavigationItem, page domain.PageNode) (ir.UINode, error) {
	return ir.Lower(page, ir.Resolver{
		Route: ir.NavigationRoutes(navigation),
		Rows: func(b domain.PageBinding) ([]ir.Row, error) {
			return bindingRows(ctx, l, b)
		},
		Records: func(b domain.PageBinding) ([]map[string]string, error) {
			return bindingRecords(ctx, l, viewerID, b)
		},
	})
}

// bindingRecords is one records Binding: each record of the Dataset, as its Machine's own Projection roles.
//
// The Projection is the Machine's `card_fields` (007 §7.6), so what a page can show of a record is what the
// Machine already declared about its shape, and the page names no Field. `RelationOptions` is deliberately
// empty: the loader refuses a `from:` whose role is a reference Field, because resolving one means reading the
// whole related Machine. Order is the Dataset's declared `sort:`, applied by the database.
func bindingRecords(ctx context.Context, l *Loader, viewerID string, b domain.PageBinding) ([]map[string]string, error) {
	ds, ok := l.Dataset(b.Dataset)
	if !ok {
		return nil, fmt.Errorf("composition: page binding names dataset %q, which no machine declares", b.Dataset)
	}
	src := l.Machine(ds.Source)
	if src == nil {
		return nil, fmt.Errorf("composition: dataset %q reads machine %q, which this Workspace does not install", b.Dataset, ds.Source)
	}
	records, err := l.SelectDataset(ctx, b.Dataset, expression.Context{CurrentUser: viewerID})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]string, 0, len(records))
	for _, r := range records {
		out = append(out, ProjectedByRole(src, r, rendering.RelationOptions{}))
	}
	return out, nil
}

// bindingRows is one Binding's rows: the Dataset's Dimension values, each with the Measure's number.
//
// **The order is a decision, not an accident of a map.** Declared option order first (a status Field lists
// its values in the order its workflow reads), then any value no option names, sorted -- a Go map ranges in a
// different order every run, and 007 §4.6 makes the same input rendering differently a MUST NOT. A declared
// option with no record is a row of 0 rather than an absent one: "no document is rejected" is an answer, and
// a count that vanishes when it reaches zero makes the page's shape depend on the data.
func bindingRows(ctx context.Context, l *Loader, b domain.PageBinding) ([]ir.Row, error) {
	ds, ok := l.Dataset(b.Dataset)
	if !ok {
		return nil, fmt.Errorf("composition: page binding names dataset %q, which no machine declares", b.Dataset)
	}
	if ds.Dimension == "" {
		return nil, fmt.Errorf("composition: dataset %q has no dimension to expand", b.Dataset)
	}
	agg, err := l.AggregateDataset(ctx, b.Dataset)
	if err != nil {
		return nil, err
	}
	var order []string
	seen := map[string]bool{}
	if src := l.Machine(ds.Source); src != nil {
		if f, ok := src.FieldByID(ds.Dimension); ok {
			for _, o := range f.Options {
				if !seen[o] {
					seen[o] = true
					order = append(order, o)
				}
			}
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

func formatMeasure(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
