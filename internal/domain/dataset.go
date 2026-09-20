package domain

import "menata.app/internal/expression"

// AggregateKind is the closed calculation vocabulary a Measure may declare (007 §14's
// static-registry seam, the same posture KnownFieldTypes/KnownActions/KnownCardFieldRoles already
// take). 007 §7.4 names six candidates -- count, sum, avg, min, max, count_distinct -- but only
// the two with a real case in this codebase are realized: every hand-written aggregation the
// decomposition audit found is a count or a sum. The other four are added when a screen actually
// needs one, never inferred from the spec's list, which is the same discipline that kept
// KnownActions at one entry until a second real case reached it.
type AggregateKind string

const (
	AggregateCount AggregateKind = "count"
	AggregateSum   AggregateKind = "sum"
)

// KnownAggregates is the closed set of aggregates a Measure may declare.
var KnownAggregates = map[AggregateKind]bool{
	AggregateCount: true,
	AggregateSum:   true,
}

// Measure is one named calculation over a Dataset's records (007 §7.4 Measure).
//
// Field is required by AggregateSum (which number Field is summed) and must be empty for
// AggregateCount (which counts records, not a Field's values) -- internal/metadata.validateDataset
// enforces both directions.
//
// Where is the optional filter predicate, and it is deliberately the same expression.Comparison a
// Constraint's block_if.condition already uses rather than a second, parallel filter syntax: the
// hand-written aggregations this primitive replaces filter with exactly `status != done`, which is
// expression.OpNotEquals spelled as Go. Reusing the evaluator instead of writing a new one is the
// B3 outcome the workflow-behavior decomposition criteria ask for -- the existing primitive
// already fits, so no new one is built.
type Measure struct {
	ID        string
	Aggregate AggregateKind
	Field     string
	Where     *expression.Comparison
}

// Dataset is a named, reusable semantic data definition over one Machine's own records (007 §7.2
// Dataset): what data is available, without saying how any particular screen renders it.
//
// Declared inside the Machine file that owns its records, the same place view:/events:/
// constraints: already live, so every field it names is checkable against that one file at load
// time. A Dataset spanning two Machines is not expressible today and deliberately so -- the five
// hand-written cases the decomposition audit counted are each single-source, and 007 §7.5 puts
// cross-source joins behind Relation, a separate primitive with no real case yet.
//
// Dimension is the optional grouping axis (007 §7.3 Dimension): a Field id whose value partitions
// the records. Empty means the Dataset produces one grand total rather than a breakdown --
// composition.Aggregate returns both, since a screen showing per-group numbers almost always
// shows the overall number beside them.
type Dataset struct {
	ID        string
	Dimension string
	Measures  []Measure
}
