package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/expression"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// Loader resolves the reads one rendered page needs -- a Machine's relation options, its child
// collections, its board columns -- and remembers within a single request what it has already
// read (development-history.md Phase 18 Step 2, closing the duplication Phase 6's fourth test measured).
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
	// files answers an attachment's size; nil leaves sizes blank, which is what a caller that never draws
	// attachments (every one but the record detail) wants.
	files *storage.Store

	listed   map[string][]*data.Record
	listedBy map[string][]*data.Record
	// selected memoizes a `select: records` Dataset's rows, **keyed by Dataset id and never by
	// Machine id**. That distinction is a correctness requirement rather than a naming preference: a
	// selection is a *different set of rows for the same Machine* -- ordered and bounded -- so
	// storing it under machineID would poison `listed` above and hand the next whole-Machine reader
	// ten activity rows where it needs all of them. The bug would be silent and would break a
	// different screen than the one being changed.
	selected map[string][]*data.Record
	// related memoizes a Dataset's relation results, keyed by Dataset id like `selected` above and for
	// the same reason: the children of *this* Dataset's parents are not the children of any other
	// selection over the same Machine. Without it two consumers of one Dataset in one request would
	// each issue the child query, which the GET sweep's repeated==0 invariant forbids.
	related map[string]map[string]map[string][]*data.Record
	// truncated rides with `selected`: a memoised selection has to carry whether it was cut short, or
	// the second consumer in a request sees a complete-looking set.
	truncated map[string]bool
	// record memoizes single-record reads by (Machine, id). Added 2026-09-28 when the per-record
	// route sweep measured one Document read three times in one request: the detail page, the
	// signature-placement embed and the PDF page count each fetched it through *store* rather than
	// through this type, so there was nothing to memoize with.
	record map[string]*data.Record

	// personNames memoizes PersonNames for this request; nil means "not read yet", which is
	// distinguishable from an empty Workspace because MemberNames always returns a non-nil map.
	personNames map[string]string

	// groups and members carry their own "already read" flags rather than relying on a nil
	// check: both legitimately come back nil for a Workspace that has none, and a memo that
	// cannot tell that from "not read yet" would read again every time.
	groups      []data.Group
	groupsRead  bool
	members     []data.Membership
	membersRead bool

	reads  int
	served int
}

// NewLoader returns a Loader for one request.
// WithFiles lets the Loader read the size of a stored upload, for the record detail's attachments list.
func (l *Loader) WithFiles(files *storage.Store) *Loader {
	l.files = files
	return l
}

func NewLoader(store *data.Store, machines map[string]*domain.Machine) *Loader {
	return &Loader{
		store:     store,
		machines:  machines,
		listed:    map[string][]*data.Record{},
		listedBy:  map[string][]*data.Record{},
		selected:  map[string][]*data.Record{},
		related:   map[string]map[string]map[string][]*data.Record{},
		truncated: map[string]bool{},
		record:    map[string]*data.Record{},
	}
}

// Reads is the number of queries actually issued, and Served the number of reads answered from
// what this request had already fetched. Both feed the query diagnostic (Phase 18 Step 3) -- the
// point of surfacing them is that the next forcing condition announces itself in a number rather
// than waiting to be guessed at.
func (l *Loader) Reads() int  { return l.reads }
func (l *Loader) Served() int { return l.served }

// Machine is one of this request's own installed Machines by id, or nil. It exists so a composed
// screen can resolve a record's *declared* display shape (card_fields, 007 §7.6) without the caller
// threading a second argument for a Machine this Loader was already handed.
//
// nil is a real answer: a Workspace that does not install that Machine has none, and every caller
// renders nothing for it rather than failing.
func (l *Loader) Machine(machineID string) *domain.Machine {
	return l.machines[machineID]
}

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

// SelectDataset resolves a `select: records` Dataset into its rows, reading each Dataset at most
// once per request (007 §7.7-§7.9).
//
// It refuses an aggregating Dataset rather than falling back to reading the Machine whole: the two
// modes answer different questions, and a caller that asked for rows and silently received every
// row of the Machine is the unbounded retrieval this method exists to replace.
// Selection is a `select: records` Dataset's result: the records, plus whatever its declared Relations
// attached to them.
//
// Related is a method rather than an exported map because the key shape (relation id, then parent id)
// is this type's business, and because a caller asking for a relation the Dataset does not declare
// should get nothing rather than a nil-map panic.
type Selection struct {
	Records []*data.Record
	// Truncated reports that the Dataset's `limit:` cut the result short -- more records matched than
	// came back.
	//
	// 007 §21.9 requires a composed experience exceeding its budget to "fail clearly or degrade through
	// an explicit runtime policy", and the inverse of "must not silently produce unbounded work" is
	// just as binding: a bound that silently drops rows is the same class of failure. A screen showing
	// 500 of 700 Documents with no indication is indistinguishable from a Workspace that has 500 --
	// and at 13 Documents that is invisible, which is exactly how it would have shipped.
	//
	// **Rendered since 2026-09-30, but only by the lists a bound would lie to** -- see Limit below.
	Truncated bool
	// Limit is the bound that produced Truncated, carried so a screen can name it ("the first 500")
	// without looking the Dataset up a second time (001 #8).
	//
	// **It is also the whole reason Truncated is not rendered everywhere, which measuring found and the
	// plan for this slice had missed.** `limit:` means two different things in the four Datasets that
	// declare it. For `ds_documents_with_steps` (500) and `ds_my_tasks` (200) it is a *safety cap* on a
	// list that means to show everything, so a bound that bites makes the screen lie. For
	// the Dashboard's activity tail and the Activity feed (`ds_recent_activity` at 10 and `ds_activity_feed` at 50, both removed 2026-10-05) it was the list's *meaning* -- "recent" is
	// defined by the bound -- and mch_activity holds 30 records today, so a naive pass of this slice
	// would have shipped a "results were truncated" warning onto the Dashboard's activity tail, where it
	// is simply false.
	//
	// **Nothing declares which kind a limit is**, so the screen decides: a list claiming completeness
	// renders the notice, a list whose bound is its definition does not. That is 2 cases of each, which
	// is past the repetition trigger (CLAUDE.md step 3) -- so the distinction is named here rather than
	// inferred, and a `limit:` that says which kind it is (007 §21.9's "explicit runtime policy") is
	// recorded as the next step rather than invented for this slice. Deriving it from `sort:` or from a
	// Dataset's *name* would be guessing from a coincidence, and a name is never an identity here.
	Limit   int
	related map[string]map[string][]*data.Record
}

// NewSelection builds a Selection from already-correlated parts. It exists for tests, which have to
// express the correlation *somehow* to exercise a builder that no longer performs it -- putting the
// grouping in a fixture is honest; leaving it in production code would mean the migration only moved
// where the reads came from.
func NewSelection(records []*data.Record, relationID string, byParent map[string][]*data.Record) Selection {
	return Selection{
		Records: records,
		related: map[string]map[string][]*data.Record{relationID: byParent},
	}
}

// Related returns one parent's children for one declared relation, or nil.
func (s Selection) Related(relationID, parentID string) []*data.Record {
	return s.related[relationID][parentID]
}

// SelectDataset resolves a `select: records` Dataset into its rows. Kept for callers that declare no
// relations, so the common case reads as a list rather than as a Selection with an unused half.
func (l *Loader) SelectDataset(ctx context.Context, datasetID string, where expression.Context) ([]*data.Record, error) {
	sel, err := l.SelectRelated(ctx, datasetID, where)
	if err != nil {
		return nil, err
	}
	// **This drops Selection.Truncated, and that is the reason My Tasks does not render the notice the
	// approval inbox does (2026-09-30).** ds_my_tasks' limit of 200 is a safety cap in exactly the same
	// sense as ds_documents_with_steps' 500, so the screen has the same claim to make -- it just calls
	// the convenience wrapper, which throws the signal away at the boundary. **A wrapper that discards a
	// diagnostic is how the diagnostic stays unread**, which is the shape TestPoolInstallsQueryTracer
	// exists over: the app works and the log quietly reports less.
	//
	// Not fixed here because fixing it means changing this signature and its two callers, and this slice
	// is the notice, not the plumbing. The second case is on record; the trigger for widening it is a
	// third caller wanting the bound.
	return sel.Records, nil
}

// SelectRelated is SelectDataset plus the children its Relations declare (007 §7.5).
func (l *Loader) SelectRelated(ctx context.Context, datasetID string, where expression.Context) (Selection, error) {
	records, truncated, err := l.selectRecords(ctx, datasetID, where)
	if err != nil {
		return Selection{}, err
	}
	ds, _ := l.Dataset(datasetID)
	if len(ds.Relations) == 0 {
		return Selection{Records: records, Truncated: truncated, Limit: ds.Limit}, nil
	}
	if cached, ok := l.related[datasetID]; ok {
		l.served++
		return Selection{Records: records, Truncated: truncated, Limit: ds.Limit, related: cached}, nil
	}

	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
	}

	sel := Selection{Records: records, Truncated: truncated, Limit: ds.Limit, related: map[string]map[string][]*data.Record{}}
	for _, rel := range ds.Relations {
		children, err := l.store.ListRecordsByAny(ctx, rel.Machine, ds.ID, rel.Via, ids)
		if err != nil {
			return Selection{}, err
		}
		l.reads++
		byParent := make(map[string][]*data.Record, len(records))
		for _, c := range children {
			byParent[DisplayString(c.Values[rel.Via])] = append(byParent[DisplayString(c.Values[rel.Via])], c)
		}
		sel.related[rel.ID] = byParent
	}
	l.related[datasetID] = sel.related
	return sel, nil
}

func (l *Loader) selectRecords(ctx context.Context, datasetID string, where expression.Context) ([]*data.Record, bool, error) {
	ds, ok := l.Dataset(datasetID)
	if !ok {
		return nil, false, fmt.Errorf("composition: no machine declares dataset %s", datasetID)
	}
	if ds.Select != domain.SelectRecords {
		return nil, false, fmt.Errorf("composition: dataset %s aggregates; SelectDataset needs `select: records`", datasetID)
	}
	if cached, ok := l.selected[datasetID]; ok {
		l.served++
		return cached, l.truncated[datasetID], nil
	}

	sort, err := sortKeysFor(l.Machine(ds.Source), ds.Sort)
	if err != nil {
		return nil, false, err
	}
	predicates, err := predicatesFor(ds, where)
	if err != nil {
		return nil, false, err
	}
	records, truncated, err := l.store.ListRecordsSelect(ctx, ds.Source, ds.ID, predicates, sort, ds.Limit)
	if err != nil {
		return nil, false, err
	}
	l.reads++
	l.selected[datasetID] = records
	// Memoised beside the records: a second consumer in the same request must be told the same truth
	// about truncation, not silently handed a complete-looking set.
	l.truncated[datasetID] = truncated
	return records, truncated, nil
}

// predicatesFor resolves a declared `where:` into the literals internal/data compares against.
//
// **An unresolvable sentinel is an error, not an empty filter.** `$current_user` with no viewer
// cannot mean "everyone" -- a personal worklist silently listing every record is a data exposure,
// not a degraded screen -- and it cannot mean `= ”` either, which would match records whose Field
// is absent. 007 §9.2's "must fail closed" is about the load-time check; this is the same rule at
// the one moment the value is actually needed.
func predicatesFor(ds domain.Dataset, ctx expression.Context) ([]data.FieldPredicate, error) {
	comparisons := ds.Where.Comparisons()
	out := make([]data.FieldPredicate, 0, len(comparisons))
	for _, c := range comparisons {
		value, ok := ctx.Resolve(c.Value)
		if !ok {
			return nil, fmt.Errorf("composition: dataset %s filters on %s, which this request cannot resolve", ds.ID, c.Value)
		}
		out = append(out, data.FieldPredicate{
			Field:  c.Field,
			Negate: c.Op == expression.OpNotEquals,
			Value:  value,
		})
	}
	return out, nil
}

// sortKeysFor turns a declared `sort:` into the column expressions internal/data takes.
//
// The resolution lives here, in Composition, because internal/data does not know what a Field is --
// and because this is where a Field id becomes a JSONB path, which is the one place the two
// vocabularies meet. Every id reaching this point was validated at load against the Machine's own
// fields or domain.SortableColumns, so the fragments it builds are never user input.
func sortKeysFor(m *domain.Machine, keys []domain.SortKey) ([]data.SortKey, error) {
	out := make([]data.SortKey, 0, len(keys))
	for _, k := range keys {
		column, isColumn := domain.SortableColumns[k.Field]
		if !isColumn {
			if m == nil {
				return nil, fmt.Errorf("composition: sort field %q needs its machine, which this Workspace does not install", k.Field)
			}
			if _, ok := m.FieldByID(k.Field); !ok {
				return nil, fmt.Errorf("composition: sort field %q is not a field of machine %s", k.Field, m.ID)
			}
			column = "data->>'" + k.Field + "'"
		}
		out = append(out, data.SortKey{Column: column, Descending: k.Descending()})
	}
	return out, nil
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

// Record returns one record, reading each (Machine, id) at most once per request.
//
// It is the single-record counterpart of ListRecords/ListRecordsBy, and it exists for the same
// reason they do: a screen that shows a record and then composes something *about* that record
// (its child collections, its signature placement, its page count) asked the store for it again
// each time. The per-record route sweep measured a Document read three times on one Review request
// and twice on one detail request.
//
// The same request-scoped contract applies as to the rest of this type: a Loader must not span a
// write, because a memo would then serve the pre-write state.
func (l *Loader) Record(ctx context.Context, machineID, id string) (*data.Record, error) {
	key := machineID + "\x00" + id
	if cached, ok := l.record[key]; ok {
		l.served++
		return cached, nil
	}
	record, err := l.store.GetRecord(ctx, machineID, id)
	if err != nil {
		return nil, err
	}
	l.reads++
	l.record[key] = record
	return record, nil
}

// PersonNames maps every mch_user record id in this request's Workspace to the display name of
// the identity behind it. It is the Loader's single answer to "who is this person", and the only
// one there is: a name stopped being a Field on mch_user on 2026-09-22 (metadata/user.yaml,
// migration 010), so nothing can read one off a record any more.
//
// Memoized like every other read here -- the Approval Inbox, the activity feed and the approver
// picker each want it within one request, which is exactly the duplication this type exists to
// collapse.
func (l *Loader) PersonNames(ctx context.Context) (map[string]string, error) {
	if l.personNames != nil {
		l.served++
		return l.personNames, nil
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	names, err := l.store.MemberNames(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	l.reads++
	l.personNames = names
	return names, nil
}

// Groups memoizes this Workspace's Groups for the request.
//
// It closes the one gap in this type: of the four Store reads reached from here, this was the
// only one without a memo, and /documents/new paid for it -- the approver picker asked for the
// Group options while the members read separately reached ListGroups through GroupsByMember, so
// the same list came back twice. A bool flag rather than a nil check, unlike PersonNames: a
// Workspace with no Groups legitimately returns nil, and "read it and found nothing" must not be
// mistaken for "not read yet".
func (l *Loader) Groups(ctx context.Context) ([]data.Group, error) {
	if l.groupsRead {
		l.served++
		return l.groups, nil
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	groups, err := l.store.ListGroups(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	l.reads++
	l.groups, l.groupsRead = groups, true
	return groups, nil
}

// Members memoizes this Workspace's members, with their per-Application roles and Groups.
//
// It reuses Groups above rather than letting the Store read them again (data.ListMembersFrom),
// which is the whole reason it is here instead of the handler calling store.ListMembers.
func (l *Loader) Members(ctx context.Context) ([]data.Membership, error) {
	if l.membersRead {
		l.served++
		return l.members, nil
	}
	groups, err := l.Groups(ctx)
	if err != nil {
		return nil, err
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	members, err := l.store.ListMembersFrom(ctx, workspaceID, groups)
	if err != nil {
		return nil, err
	}
	l.reads++
	l.members, l.membersRead = members, true
	return members, nil
}

// RelationOptions fetches every option a reference field on m could select (Relation or Person,
// per domain.Field.IsReference), keyed by target Machine ID. The target's first Field is used as
// the display label -- a minimal convention until a real Projection/semantic "title" role exists
// (007 §7.6), which isn't forced yet by a case that needs more than one reasonable label field.
//
// mch_user is the one target that does not work that way, and cannot: the runtime's own identity
// Machine declares no name Field to be "first" (its name lives on the identity), so its options
// are labelled through PersonNames instead. That is not a special case smuggled in -- mch_user is
// the Machine domain.UserMachineID already names as the runtime's, and labelling a person is the
// runtime's job in a way that labelling a Project is not.
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
		if !ok {
			continue
		}

		records, err := l.ListRecords(ctx, target.ID)
		if err != nil {
			return nil, err
		}
		list := make([]rendering.RelationOption, 0, len(records))
		if target.ID == domain.UserMachineID {
			names, err := l.PersonNames(ctx)
			if err != nil {
				return nil, err
			}
			for _, r := range records {
				list = append(list, rendering.RelationOption{ID: r.ID, Label: names[r.ID]})
			}
			options[f.RelatedMachine] = list
			continue
		}
		if len(target.Fields) == 0 {
			continue
		}
		labelFieldID := target.Fields[0].ID
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
	groups, err := l.Groups(ctx)
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
// another Machine whose reference field names this one (development-history.md Phase 9) -- fetching each
// collection's records and the relation options its own rows need to render.
//
// Each section's relation options are resolved through the same Loader, which is what removes the
// duplication Phase 6 measured: before this, every section resolved its own options from scratch,
// so a Machine referenced by four sections was fetched four times.
func (l *Loader) ChildSections(ctx context.Context, m *domain.Machine, recordID string) ([]rendering.ChildSection, error) {
	var sections []rendering.ChildSection
	for _, cc := range domain.FindChildCollections(l.machineSlice(), m.ID) {
		// A Machine's declared tags are drawn as chips by RecordExtras; the join rows themselves are not a
		// section a reader wants (a Task's "Card Label" table of two opaque ids).
		if m.CardTags != nil && cc.Machine.ID == m.CardTags.Machine {
			continue
		}
		records, err := l.ListRecordsBy(ctx, cc.Machine.ID, cc.Field.ID, recordID)
		if err != nil {
			return nil, err
		}
		relations, err := l.RelationOptions(ctx, cc.Machine)
		if err != nil {
			return nil, err
		}
		sections = append(sections, rendering.ChildSection{Machine: cc.Machine, Records: records, Relations: relations, Checklist: checklistFor(cc.Machine, cc.Field.ID, records), Attachments: attachmentsFor(cc.Machine, cc.Field.ID, records, l.files)})
	}
	return sections, nil
}

// BoardColumns resolves board columns for m when its board Layout groups by a reference field
// (development-history.md Phase 10's ordered Lists, e.g. mch_list) -- fetching those real records is I/O
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

// CardTags resolves m's `card_tags:` for the records a board is about to draw: the join records that
// point at them, then the few tag records those name -- two bounded statements whatever the board's size,
// never one per card and never every record of the tag Machine. A tag's name and colour come from the tag
// Machine's own `title` and `color` card_fields, so a Machine declaring nothing here costs nothing and
// returns nil.
//
// A tag whose colour is missing or outside the palette (a record written before the Field was constrained)
// is drawn in the neutral entry rather than dropped: losing the chip would hide that the card is tagged.
func (l *Loader) CardTags(ctx context.Context, m *domain.Machine, v domain.View, records []*data.Record) (map[string][]rendering.CardTag, error) {
	if v.EffectiveType() != domain.ViewBoard {
		return nil, nil
	}
	return l.CardTagsFor(ctx, m, records)
}

// CardTagsFor is CardTags for a screen that is not a board View but draws the same chips on the same
// records (My Tasks): the declaration belongs to the Machine, and which screen asks does not change what a
// card is tagged with.
func (l *Loader) CardTagsFor(ctx context.Context, m *domain.Machine, records []*data.Record) (map[string][]rendering.CardTag, error) {
	ct := m.CardTags
	if ct == nil || len(records) == 0 {
		return nil, nil
	}
	join, ok := l.machines[ct.Machine]
	if !ok {
		return nil, nil
	}
	tagField, ok := join.FieldByID(ct.Tag)
	if !ok {
		return nil, nil
	}
	tagMachine, ok := l.machines[tagField.RelatedMachine]
	if !ok {
		return nil, nil
	}

	parentIDs := make([]string, 0, len(records))
	for _, r := range records {
		parentIDs = append(parentIDs, r.ID)
	}
	joins, err := l.store.ListRecordsByAny(ctx, join.ID, "card_tags", ct.Via, parentIDs)
	if err != nil {
		return nil, err
	}
	l.reads++
	tagIDs := make([]string, 0, len(joins))
	seen := map[string]bool{}
	for _, j := range joins {
		if id := DisplayString(j.Values[ct.Tag]); id != "" && !seen[id] {
			seen[id] = true
			tagIDs = append(tagIDs, id)
		}
	}
	tagRecords, err := l.store.ListRecordsByIDs(ctx, tagMachine.ID, tagIDs)
	if err != nil {
		return nil, err
	}
	l.reads++

	titleField, colorField := FieldForRole(tagMachine, domain.CardFieldRoleTitle), FieldForRole(tagMachine, domain.CardFieldRoleColor)
	byID := make(map[string]rendering.CardTag, len(tagRecords))
	for _, t := range tagRecords {
		color := domain.TagSlate
		if c := DisplayString(t.Values[colorField]); domain.IsTagColor(c) {
			color = domain.TagColor(c)
		}
		byID[t.ID] = rendering.CardTag{Label: DisplayString(t.Values[titleField]), Color: color}
	}
	out := make(map[string][]rendering.CardTag, len(records))
	for _, j := range joins {
		if tag, ok := byID[DisplayString(j.Values[ct.Tag])]; ok {
			parent := DisplayString(j.Values[ct.Via])
			out[parent] = append(out[parent], tag)
		}
	}
	return out, nil
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

// ApproverOptions narrows the person picker on the submit wizard to people who could actually
// decide a step -- those holding a role the step Machine's own `decide` Permission requires
// (upstream's CAP-F05, "a query-time filter, no new metadata concept").
//
// It exists because the roles decision of 2026-09-21 gave the unfiltered picker a way to fail
// silently: a submitter could name anyone in the Workspace, so they could build a chain whose
// steps are assigned to a real person who holds no `approver` role -- or no role in Document
// Approval at all, and therefore cannot even open its screens. The step is created, looks normal,
// and is decidable by nobody. Nothing errors, at submit time or after.
//
// The eligible set is derived, never declared twice: the required roles come from the same
// Permissions authorization.AllowsAction enforces (requiredRoles, shared with the Authorization
// Matrix), and a member's own roles come from data.EffectiveRoles, so a role held through a Group
// qualifies exactly as a direct one does. A Machine whose decide Permission names no role at all
// narrows nothing -- the picker goes back to everyone, which is what "no rule declared" means
// everywhere else (001 Principle #6).
//
// Deliberately scoped to the mch_user list and to `decide`. The Group half of the picker needs no
// narrowing (a Group's members are checked at the moment someone acts, not at submission -- board
// 09's own note), and narrowing every reference field on every screen by whatever Permission
// happens to mention it would be a general rule invented from one case.
func ApproverOptions(all rendering.RelationOptions, stepMachine *domain.Machine, members []data.Membership) rendering.RelationOptions {
	if stepMachine == nil {
		return all
	}
	required, restricted := requiredRoles(stepMachine, domain.ActionDecide)
	if !restricted {
		return all
	}
	eligible := make(map[string]bool, len(members))
	for _, m := range members {
		for _, role := range data.EffectiveRoles(m.AppRoles, m.Groups)[stepMachine.ApplicationID] {
			if required[role] {
				eligible[m.UserRecordID] = true
				break
			}
		}
	}

	narrowed := make(rendering.RelationOptions, len(all))
	for machineID, list := range all {
		if machineID != domain.UserMachineID {
			narrowed[machineID] = list
			continue
		}
		kept := make([]rendering.RelationOption, 0, len(list))
		for _, opt := range list {
			if eligible[opt.ID] {
				kept = append(kept, opt)
			}
		}
		narrowed[machineID] = kept
	}
	return narrowed
}
