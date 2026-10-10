package composition

import (
	"fmt"

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

	for _, r := range records {
		var group map[string]float64
		if ds.Dimension != "" {
			key := DisplayString(r.Values[ds.Dimension])
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
			if !filters[ms.ID].Evaluate(r.Values) {
				continue
			}
			v := measureValue(ms, r)
			agg.Total[ms.ID] += v
			if group != nil {
				group[ms.ID] += v
			}
		}
	}
	return agg, nil
}

// resolveMeasureFilters is each Measure's `where:` with its context values replaced by the literals they mean
// for this request, once, before any record is read.
func resolveMeasureFilters(ds domain.Dataset, ctx expression.Context) (map[string]*expression.Predicate, error) {
	out := make(map[string]*expression.Predicate, len(ds.Measures))
	for _, ms := range ds.Measures {
		if ms.Where == nil {
			continue
		}
		resolved := &expression.Predicate{All: make([]expression.Comparison, 0, len(ms.Where.All))}
		for _, c := range ms.Where.All {
			value, ok := ctx.Resolve(c.Value)
			if !ok {
				return nil, fmt.Errorf("composition: dataset %s measure %s filters on %s, which this request cannot resolve", ds.ID, ms.ID, c.Value)
			}
			c.Value = value
			resolved.All = append(resolved.All, c)
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
