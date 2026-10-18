package composition

import (
	"fmt"
	"math"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// Aggregation is one Dataset's computed result: every Measure's value over the whole record set,
// and -- when the Dataset declares a Dimension -- the same Measures broken down by that
// Dimension's value.
//
// Both are returned from one pass because the screens this replaces need both at once: Team
// Capacity shows each member's own active-task count *and* the team's total beside it, from the
// same records. Computing them separately would mean two passes and two chances to disagree.
type Aggregation struct {
	// Total maps Measure id to its value over every record, ignoring the Dimension. Always
	// populated, with an explicit zero for a Measure no record satisfied, so a caller reading a
	// declared Measure never has to distinguish "absent" from "zero".
	Total map[string]float64
	// ByDimension maps a Dimension value to that group's own Measure values. Empty when the
	// Dataset declares no Dimension. A group appears as soon as one record carries its value,
	// even if every Measure filtered that record out -- so "this assignee exists but has nothing
	// open" is a present group of zeroes, not a missing key.
	ByDimension map[string]map[string]float64
	// Types maps a sum Measure's id to the type of the Field it sums, so its figure is written the way that Field is
	// read: a total of money grouped in thousands. A count, or any Measure absent here, is written as a plain number.
	// Aggregate leaves it empty; the Loader, which knows the source Machine, fills it.
	Types map[string]domain.FieldType
	// Currencies maps a sum Measure's id to its Field's currency, so a total of money reads "Rp 966,000,000".
	Currencies map[string]string
	// Number is how the Workspace writes a figure; the Loader fills it, and the zero value is the international format.
	Number domain.NumberFormat
}

// Format writes one of this Aggregation's figures for a page (domain.FormatFigureIn).
func (a Aggregation) Format(measure string, v float64) string {
	return domain.WithCurrency(a.Currencies[measure], domain.FormatFigureIn(a.Number, a.Types[measure], v))
}

// Aggregate evaluates ds over records: 007 §7.2-§7.4 (Dataset, Dimension, Measure) made
// executable, the sibling of ProjectCardFields for §7.6. Pure -- no I/O, no database, no clock: `$today` arrives in
// ctx, injected, so the same records and the same date give the same figure (007 §4.6) --
// so it is unit-tested the same way the rest of internal/composition's build* helpers are.
//
// This is what the decomposition audit counted five hand-written copies of (buildDashboard twice,
// buildCapacity, buildSprint, PersonalTasks): "count records, grouped by some field, optionally
// filtered". Each copy re-implemented the same loop with different field-id literals baked in.
//
// A Measure naming a Field that doesn't exist contributes zero rather than erroring, matching
// ProjectCardFields' own posture for the same situation: internal/metadata.Validate already
// rejects it at load time, so reaching here means metadata changed under a running process, and
// degrading to zero beats failing a whole page render.
//
// A Measure's `where:` that names a context value ctx cannot resolve is an **error**, not a filter that matches
// nothing: a count of "overdue" that quietly became 0 because the date was missing is the silent-empty failure
// 007 §9.2's "must fail closed" exists to prevent.
func Aggregate(ds domain.Dataset, records []*data.Record, ctx expression.Context) (Aggregation, error) {
	filters, err := resolveMeasureFilters(ds, ctx)
	if err != nil {
		return Aggregation{}, err
	}
	agg := Aggregation{
		Total:       make(map[string]float64, len(ds.Measures)),
		ByDimension: make(map[string]map[string]float64),
	}
	for _, ms := range ds.Measures {
		agg.Total[ms.ID] = 0
	}

	// count_distinct keeps the values it has seen, over all records and per group; the counts are taken at the end.
	distinctTotal := map[string]map[string]bool{}
	distinctGroup := map[string]map[string]map[string]bool{}
	for _, r := range records {
		var group map[string]float64
		key := ""
		if ds.Dimension != "" {
			key = DisplayString(r.Values[ds.Dimension])
			group = agg.ByDimension[key]
			if group == nil {
				group = make(map[string]float64, len(ds.Measures))
				for _, ms := range ds.Measures {
					group[ms.ID] = 0
				}
				agg.ByDimension[key] = group
			}
		}
		for _, ms := range ds.Measures {
			if ms.Aggregate == domain.AggregatePercentage || !filters[ms.ID].Evaluate(r.Values) {
				continue
			}
			if ms.Aggregate == domain.AggregateCountDistinct {
				if v := DisplayString(r.Values[ms.Field]); v != "" {
					addDistinct(distinctTotal, ms.ID, v)
					if group != nil {
						if distinctGroup[key] == nil {
							distinctGroup[key] = map[string]map[string]bool{}
						}
						addDistinct(distinctGroup[key], ms.ID, v)
					}
				}
				continue
			}
			v := measureValue(ms, r)
			agg.Total[ms.ID] += v
			if group != nil {
				group[ms.ID] += v
			}
		}
	}
	for id, seen := range distinctTotal {
		agg.Total[id] = float64(len(seen))
	}
	for key, perMeasure := range distinctGroup {
		for id, seen := range perMeasure {
			agg.ByDimension[key][id] = float64(len(seen))
		}
	}
	// Percentages last, once every count and sum they divide is final.
	for _, ms := range ds.Measures {
		if ms.Aggregate != domain.AggregatePercentage {
			continue
		}
		agg.Total[ms.ID] = percentage(agg.Total[ms.Numerator], agg.Total[ms.Denominator])
		for _, group := range agg.ByDimension {
			group[ms.ID] = percentage(group[ms.Numerator], group[ms.Denominator])
		}
	}
	return agg, nil
}

func addDistinct(sets map[string]map[string]bool, measureID, value string) {
	if sets[measureID] == nil {
		sets[measureID] = map[string]bool{}
	}
	sets[measureID][value] = true
}

// percentage is part as a whole percentage of whole, rounded down -- the Dashboard's "18% of all cards" -- and 0
// when whole is 0, since nothing out of nothing is no share rather than a division by zero.
func percentage(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	// The epsilon keeps a share that is whole in decimal whole in binary: 0.29/1*100 is 28.999999999999996, which
	// Floor alone would show as 28.
	return math.Floor(part*100/whole + 1e-9)
}

// resolveMeasureFilters is each Measure's `where:` with its context values replaced by the literals they mean
// for this request, once, before any record is read.
func resolveMeasureFilters(ds domain.Dataset, ctx expression.Context) (map[string]*expression.Predicate, error) {
	out := make(map[string]*expression.Predicate, len(ds.Measures))
	for _, ms := range ds.Measures {
		if ms.Where == nil {
			continue
		}
		resolved := &expression.Predicate{}
		for _, group := range []struct{ from, to *[]expression.Comparison }{
			{&ms.Where.All, &resolved.All}, {&ms.Where.Any, &resolved.Any},
		} {
			for _, c := range *group.from {
				value, ok := ctx.Resolve(c.Value)
				if !ok {
					return nil, fmt.Errorf("composition: dataset %s measure %s filters on %s, which this request cannot resolve", ds.ID, ms.ID, c.Value)
				}
				c.Value = value
				*group.to = append(*group.to, c)
			}
		}
		out[ms.ID] = resolved
	}
	return out, nil
}

// measureValue is one record's contribution to one Measure. A count contributes one record; a sum
// contributes the Field's stored number, and zero for anything else -- a sum Field is validated to
// be a number field, so a non-float64 here means an unset value, which adds nothing.
func measureValue(ms domain.Measure, r *data.Record) float64 {
	switch ms.Aggregate {
	case domain.AggregateCount:
		return 1
	case domain.AggregateSum:
		if n, ok := r.Values[ms.Field].(float64); ok {
			return n
		}
	}
	return 0
}
