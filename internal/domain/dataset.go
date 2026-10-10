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
// Where is the optional filter, and it is deliberately the same expression.Predicate a `select: records`
// Dataset's `where:` already is rather than a second, parallel filter syntax: the hand-written
// aggregations this primitive replaces filter with `status != done`, which is expression.OpNotEquals
// spelled as Go, and "overdue" is two of them at once (not finished, and past its date), which is why it
// is a conjunction and not one Comparison (2026-10-10). A measure resolves `$today` and `$done` and no other
// context value: an aggregate has no viewer and no request parameters to resolve them against.
type Measure struct {
	ID        string
	Aggregate AggregateKind
	Field     string
	Where     *expression.Predicate
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
	ID string
	// Source is the Machine id whose records this Dataset counts -- 007 §7.2's `source.machine`,
	// but *derived* rather than declared: a Dataset lives inside the Machine file that owns its
	// records, so writing the id again in YAML would be the duplicated metadata 001 Principle #8
	// rejects. internal/metadata.Parse fills it in from the enclosing Machine.
	//
	// Carrying it here is what lets a screen name a Dataset and nothing else: the Dataset knows
	// where its own records come from, so a composed screen no longer has to also know which
	// Machine to read (composition.Loader.AggregateDataset).
	Source    string
	Dimension string
	Measures  []Measure

	// Select is what this Dataset produces: "" (the default) aggregates its records into Measures,
	// and SelectRecords returns the records themselves (007 §7.7 Filter, §7.8 Sort, §7.9 Pagination).
	//
	// **The two modes are mutually exclusive**, enforced at load: a Dataset with `select: records`
	// declares no measures and no dimension, and one without it declares no sort or limit. They answer
	// different questions -- "how many, grouped how" versus "which rows, in what order" -- and a
	// Dataset trying to be both would leave every consumer asking which half it got.
	Select string
	// Where is the record-selection filter, applied by the database rather than after retrieval
	// (007 §7.7 Filter, §21.2 "Filters should execute in PostgreSQL whenever safe and beneficial").
	// Nil selects everything. Only meaningful with SelectRecords -- an aggregating Dataset filters
	// per Measure, which is a different question and stays where it is.
	Where *expression.Predicate
	// Relations are the associations this Dataset follows (007 §7.5). Only meaningful with
	// SelectRecords: an aggregate has no rows to attach children to.
	Relations []Relation
	// Sort is the declared ordering, applied by the database rather than after retrieval (007 §7.8:
	// "Sort describes logical ordering. Physical execution determines whether an index, database
	// sort, or another strategy is used"). Empty falls back to the Store's own default.
	Sort []SortKey
	// Limit bounds how many records come back, and is **required** when Select is SelectRecords.
	//
	// Required rather than optional because 007 §7.9 puts pagination in the Data Plane for a stated
	// reason -- "it determines how much data is retrieved, not how it is visually presented" -- and
	// asks the runtime to "preserve the current capability that prevents unbounded list retrieval".
	// An unbounded `select: records` is that retrieval. The cost of the choice is real and worth
	// naming: a screen that genuinely wants every matching row has to name a number anyway. That is
	// the safer direction to be wrong in, since a required bound can be loosened later while an
	// optional one has to be imposed on existing declarations.
	Limit int
}

// SelectRecords is the one value Dataset.Select may take besides "". A closed vocabulary rather than
// a bool, so a later mode (007 §7.6's Projection is the obvious candidate) is an added constant
// rather than a second flag contradicting the first.
const SelectRecords = "records"

// SortKey is one declared ordering step.
type SortKey struct {
	// Field is either a Field id on this Dataset's Machine, or one of SortableColumns below.
	Field string
	// Direction is one of KnownSortDirections, or "" for ascending.
	//
	// Carried as the declared string rather than reduced to a bool at parse time, following
	// expression.Op's own precedent: collapsing it early would turn `direction: descending` into a
	// silent ascending sort, and an ordering that quietly does the opposite of what it says is worse
	// than one that refuses to load. The validator sees the raw value and rejects it.
	Direction string
}

const (
	SortAscending  = "asc"
	SortDescending = "desc"
)

// KnownSortDirections is the closed set a `direction:` may name. "" is accepted separately and means
// ascending, so an author writing no direction gets the Store's own default order.
var KnownSortDirections = map[string]bool{SortAscending: true, SortDescending: true}

// Descending reports whether this key sorts downwards.
func (k SortKey) Descending() bool { return k.Direction == SortDescending }

// SortableColumns are the record-level columns a `sort:` may name besides a Field id, mapped to their
// real column names.
//
// They exist because the first real case needs one: mch_activity declares no timestamp Field at all
// and is ordered by the record's own created_at, so without these its Dataset could not be written.
// Closed and validated -- a sort naming neither a Field nor one of these is a load error, never a
// silently ignored ordering, since an ordering that quietly does nothing is indistinguishable from
// one that worked.
var SortableColumns = map[string]string{
	"created_at": "created_at",
	"updated_at": "updated_at",
	"sort_order": "sort_order",
}

// Relation is one declared association a `select: records` Dataset follows -- 007 §7.5, whose two
// sentences settle most of this type's design.
//
// *"Relations should reuse existing Machine reference semantics rather than inventing a second
// relationship identity."* So a Relation names an existing reference **Field**; it does not describe
// the association itself. Validation refuses a Via that is not a reference Field on Machine pointing
// back at the declaring Machine, which is what makes "reuse" checkable rather than aspirational.
//
// *"A relation may be compiled to a join, semi-join, lookup, or other physical strategy."* The runtime
// picks. Today it is a **lookup** -- one bounded query for the parents, one for their children keyed
// by the parents' ids -- because at 13 Documents and 23 Steps two indexed queries beat a join, and
// because §7.5 names lookup as a legitimate compilation rather than a fallback. That choice is
// invisible to metadata and may change (§22).
//
// **Via is required rather than derived**, and that is a deliberate exception to 001 #6.
// domain.FindChildCollections already derives "which Machines point here" for the detail page, so
// deriving was available -- but a child Machine may declare two Fields pointing at the same parent
// (mch_card_label declares two relation Fields), and picking one would depend on declaration order,
// which 007 §4.6 forbids ("Plan construction must not depend on map iteration order, incidental
// database ordering..."). Requiring one line of YAML adds no inference; deriving would add a fourth
// one to explain.
type Relation struct {
	// ID is this association's own stable identity (rel_*), so a consumer asks for it by name rather
	// than by position (004 §Stable Identity).
	ID string
	// Machine is the id of the Machine holding the children. A Machine id names something here rather
	// than claiming an identity, which is the distinction CLAUDE.md draws -- and internal/installer's
	// rewriteIDs is line-based and generic, so a renamed Machine's id is rewritten here without that
	// function needing to know this key exists (its own comment says enumerating keys "would be a list
	// to keep in step with every future key that can hold an id").
	Machine string
	// Via is the reference Field on Machine whose value is the declaring Machine's record id.
	Via string
}
