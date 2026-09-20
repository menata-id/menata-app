package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/rendering"
)

// Loader resolves the reads one rendered page needs -- a Machine's relation options, its child
// collections, its board columns -- and remembers within a single request what it has already
// read (ROADMAP.md Phase 18 Step 2, closing the duplication Phase 6's fourth test measured).
//
// It deliberately is not the Composable Execution Planner. It performs exactly one of the CEP's
// jobs, deduplication of shared dependencies (007 §18), because that is the only one a
// measurement has forced. There is no IR and no dependency graph here: every read's arguments are
// known before any read runs, so a flat memo is sufficient and a graph would have nothing to
// order. When that stops being true -- when one fetch's arguments start coming from another
// fetch's result, which happens once a referenced Machine is too large to fetch whole -- the
// graph earns its place and this type is what it grows out of.
//
// A Loader is request-scoped and must not outlive the request or span a write: it caches records,
// so reusing one across a mutation would serve the pre-write state. Callers construct it where
// reads begin, after any write has completed.
type Loader struct {
	store    *data.Store
	machines map[string]*domain.Machine

	listed   map[string][]*data.Record
	listedBy map[string][]*data.Record

	reads  int
	served int
}

// NewLoader returns a Loader for one request.
func NewLoader(store *data.Store, machines map[string]*domain.Machine) *Loader {
	return &Loader{
		store:    store,
		machines: machines,
		listed:   map[string][]*data.Record{},
		listedBy: map[string][]*data.Record{},
	}
}

// Reads is the number of queries actually issued, and Served the number of reads answered from
// what this request had already fetched. Both feed the query diagnostic (Phase 18 Step 3) -- the
// point of surfacing them is that the next forcing condition announces itself in a number rather
// than waiting to be guessed at.
func (l *Loader) Reads() int  { return l.reads }
func (l *Loader) Served() int { return l.served }

// ListRecords returns every record of a Machine, reading it at most once per request.
func (l *Loader) ListRecords(ctx context.Context, machineID string) ([]*data.Record, error) {
	if cached, ok := l.listed[machineID]; ok {
		l.served++
		return cached, nil
	}
	records, err := l.store.ListRecords(ctx, machineID)
	if err != nil {
		return nil, err
	}
	l.reads++
	l.listed[machineID] = records
	return records, nil
}

// ListRecordsBy returns a child collection's records, reading each distinct (Machine, Field,
// value) triple at most once per request.
func (l *Loader) ListRecordsBy(ctx context.Context, machineID, fieldID, value string) ([]*data.Record, error) {
	key := machineID + "\x00" + fieldID + "\x00" + value
	if cached, ok := l.listedBy[key]; ok {
		l.served++
		return cached, nil
	}
	records, err := l.store.ListRecordsBy(ctx, machineID, fieldID, value)
	if err != nil {
		return nil, err
	}
	l.reads++
	l.listedBy[key] = records
	return records, nil
}

// RelationOptions fetches every option a reference field on m could select (Relation or Person,
// per domain.Field.IsReference), keyed by target Machine ID. The target's first Field is used as
// the display label -- a minimal convention until a real Projection/semantic "title" role exists
// (007 §7.6), which isn't forced yet by a case that needs more than one reasonable label field.
func (l *Loader) RelationOptions(ctx context.Context, m *domain.Machine) (rendering.RelationOptions, error) {
	options := rendering.RelationOptions{}
	for _, f := range m.Fields {
		if !f.IsReference() {
			continue
		}
		if _, loaded := options[f.RelatedMachine]; loaded {
			continue
		}
		target, ok := l.machines[f.RelatedMachine]
		if !ok || len(target.Fields) == 0 {
			continue
		}
		labelFieldID := target.Fields[0].ID

		records, err := l.ListRecords(ctx, target.ID)
		if err != nil {
			return nil, err
		}
		list := make([]rendering.RelationOption, 0, len(records))
		for _, r := range records {
			list = append(list, rendering.RelationOption{ID: r.ID, Label: DisplayString(r.Values[labelFieldID])})
		}
		options[f.RelatedMachine] = list
	}
	return options, nil
}

// GroupOptions fetches the Workspace Groups a `group` field on m could select (CAP-F24, Fase
// 6c-1) -- the group-side counterpart of RelationOptions above.
//
// It reads nothing at all for a Machine declaring no group field, which is every Machine but
// mch_approval_step today: the picker is the only consumer, so a Machine that cannot render one
// should not pay a query for it. That check is the whole reason this is a method on Loader rather
// than a bare store call in each handler.
//
// Unlike RelationOptions there is no per-target keying and no label-field convention to apply: a
// Group is not a Machine, so every group field in the Workspace draws from one list and the label
// is the Group's own name column. See domain.FieldTypeGroup for why no mch_group exists.
func (l *Loader) GroupOptions(ctx context.Context, m *domain.Machine) (rendering.GroupOptions, error) {
	needed := false
	for _, f := range m.Fields {
		if f.Type == domain.FieldTypeGroup {
			needed = true
			break
		}
	}
	if !needed {
		return nil, nil
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	groups, err := l.store.ListGroups(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	options := make(rendering.GroupOptions, 0, len(groups))
	for _, g := range groups {
		options = append(options, rendering.RelationOption{ID: g.ID, Label: g.Name})
	}
	return options, nil
}

// ChildSections resolves every child collection pointing at (m, recordID) -- every record of
// another Machine whose reference field names this one (ROADMAP.md Phase 9) -- fetching each
// collection's records and the relation options its own rows need to render.
//
// Each section's relation options are resolved through the same Loader, which is what removes the
// duplication Phase 6 measured: before this, every section resolved its own options from scratch,
// so a Machine referenced by four sections was fetched four times.
func (l *Loader) ChildSections(ctx context.Context, m *domain.Machine, recordID string) ([]rendering.ChildSection, error) {
	var sections []rendering.ChildSection
	for _, cc := range domain.FindChildCollections(l.machineSlice(), m.ID) {
		records, err := l.ListRecordsBy(ctx, cc.Machine.ID, cc.Field.ID, recordID)
		if err != nil {
			return nil, err
		}
		relations, err := l.RelationOptions(ctx, cc.Machine)
		if err != nil {
			return nil, err
		}
		sections = append(sections, rendering.ChildSection{Machine: cc.Machine, Records: records, Relations: relations})
	}
	return sections, nil
}

// BoardColumns resolves board columns for m when its board Layout groups by a reference field
// (ROADMAP.md Phase 10's ordered Lists, e.g. mch_list) -- fetching those real records is I/O
// experience.GroupRecords doesn't perform itself. Returns nil (not an error) when m isn't a
// board, or groups by an ordinary status field instead: GroupRecords computes its own columns
// from that Field's Options in that case, unchanged since Phase 5.
func (l *Loader) BoardColumns(ctx context.Context, m *domain.Machine, v domain.View) ([]experience.Column, error) {
	if v.EffectiveType() != domain.ViewBoard {
		return nil, nil
	}
	groupField, ok := m.FieldByID(v.GroupBy)
	if !ok || !groupField.IsReference() {
		return nil, nil
	}
	listMachine, ok := l.machines[groupField.RelatedMachine]
	if !ok || len(listMachine.Fields) == 0 {
		return nil, nil
	}

	records, err := l.ListRecords(ctx, listMachine.ID)
	if err != nil {
		return nil, err
	}
	labelFieldID := listMachine.Fields[0].ID
	columns := make([]experience.Column, 0, len(records))
	for _, r := range records {
		columns = append(columns, experience.Column{ID: r.ID, Label: DisplayString(r.Values[labelFieldID])})
	}
	return columns, nil
}

// ConstraintRelatedRecords fetches every record of each Constraint's related Machine, keyed by
// that Machine's ID, for behavior.CheckConstraints to evaluate against.
func (l *Loader) ConstraintRelatedRecords(ctx context.Context, m *domain.Machine) (map[string][]*data.Record, error) {
	related := map[string][]*data.Record{}
	for _, c := range m.Constraints {
		if _, loaded := related[c.BlockIf.RelatedMachine]; loaded {
			continue
		}
		records, err := l.ListRecords(ctx, c.BlockIf.RelatedMachine)
		if err != nil {
			return nil, err
		}
		related[c.BlockIf.RelatedMachine] = records
	}
	return related, nil
}

// machineSlice returns the Machines in a stable order. Sorting is not cosmetic: Go randomizes map
// iteration, so without it FindChildCollections returns a detail page's child sections in a
// different order on every request, and the same record renders its sections shuffled on reload.
// That defect predates this package -- main.go's own machineSlice had it too -- and was found by
// Phase 18 Step 2's before/after HTML comparison, which is exactly what a rendered-output check
// exists to catch.
//
// Ordering by ID is deterministic but arbitrary; manifest declaration order would read better and
// is what a real section-order metadata concept would give (004 §Navigation Metadata, still code
// rather than metadata -- see ROADMAP.md's conformance gaps). Not built here: the Loader is handed
// a map, and the actual defect is the instability, not the choice of order.
func (l *Loader) machineSlice() []*domain.Machine {
	list := make([]*domain.Machine, 0, len(l.machines))
	for _, m := range l.machines {
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// Dataset resolves one Dataset (007 §7.2) by its id alone -- no Machine id, because the Dataset
// already carries its own Source and internal/metadata guarantees the id is unique across the
// Application (validateDatasetIDsAreUnique). A screen that only needs numbers therefore names one
// thing, not two.
//
// Reported missing rather than returning a zero Dataset, because a composed screen naming a
// Dataset metadata no longer declares would otherwise render a page of silent zeroes -- the same
// "fail where it's cheap to find" reasoning rendering.routeByID states for an unknown navigation
// id, routed through the error return this layer already has instead of a panic.
func (l *Loader) Dataset(datasetID string) (domain.Dataset, bool) {
	for _, m := range l.machineSlice() {
		if ds, ok := m.DatasetByID(datasetID); ok {
			return ds, true
		}
	}
	return domain.Dataset{}, false
}

// AggregateDataset is the whole read: resolve the Dataset, read its own Source Machine's records,
// and evaluate it. A screen composing numbers calls this and never names a Machine at all -- the
// Dataset knows where its records live, which is the point of Source existing.
//
// Records come through ListRecords, so a screen that also needs the same Machine's records for
// something a Dataset can't express (a list of rows, not a count) pays for one read, not two:
// the Loader's own per-request memo serves the second caller.
func (l *Loader) AggregateDataset(ctx context.Context, datasetID string) (Aggregation, error) {
	ds, ok := l.Dataset(datasetID)
	if !ok {
		return Aggregation{}, fmt.Errorf("composition: no machine declares dataset %s", datasetID)
	}
	records, err := l.ListRecords(ctx, ds.Source)
	if err != nil {
		return Aggregation{}, err
	}
	return Aggregate(ds, records), nil
}

// DisplayString renders a stored field value as text. Records hold JSONB-decoded values, so a
// value is a string for most Field types and a decoded number/bool/slice for the rest.
func DisplayString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
