package composition

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

// DeclaredPage builds the UI IR tree of a navigation item's `page:` (007 §15.1: declaration -> UI IR),
// resolving every Binding through the Loader and nothing else.
//
// **Cost is the number of distinct Datasets, not the number of nodes.** A Binding reads through
// `Loader.AggregateDataset`, whose `ListRecords` is memoised per request, so ten bound nodes over one Dataset
// are one read; and a tree with no Binding is no read at all. That is the property the page's query-cost test
// pins (§20: no plane may read everything and trim later).
func DeclaredPage(ctx context.Context, l *Loader, page domain.PageNode) (ir.UINode, error) {
	return ir.Lower(page, func(b domain.PageBinding) ([]ir.Row, error) {
		return bindingRows(ctx, l, b)
	})
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
