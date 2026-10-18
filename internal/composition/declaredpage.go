package composition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
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
	"menata.app/internal/experience"
	"menata.app/internal/expression"
	"menata.app/internal/ir"
	"menata.app/internal/registry"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
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
	return DeclaredPageFor(ctx, l, viewer, params, now, navigation, "", page)
}

// DeclaredPageFor is DeclaredPage for the page that belongs to navigation item pageID, which is how a NavList marks the
// row for the screen the viewer is on.
func DeclaredPageFor(ctx context.Context, l *Loader, viewer domain.Actor, params map[string]string, now time.Time, navigation []domain.NavigationItem, pageID string, page domain.PageNode) (ir.UINode, error) {
	return ir.Lower(page, ir.Resolver{
		PageID: pageID,
		Route:  ir.NavigationRoutes(navigation),
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
			return bindingForm(ctx, l, viewer, params, pageID, b)
		},
		Action: func(b domain.PageBinding, recordID string) (ir.FormSpec, error) {
			return bindingAction(ctx, l, viewer, b, recordID)
		},
		Navigation: navigationSection,
		PDF:        func() bool { return l.pdf },
		Query:      func(name string) string { return params[name] },
		Params:     func() map[string]string { return params },
		TransitionAll: func(b domain.PageBinding, becomes string) (ir.AllTransition, error) {
			return bindingTransitionAll(ctx, l, viewer, params, now, b, becomes)
		},
		Viewer: func(name string) string {
			if name == domain.PageViewerWorkspaceRole {
				return viewer.WorkspaceRole
			}
			return ""
		},
		Parameter: func(name string) string {
			if name == domain.PageParameterWeek {
				return strconv.Itoa(expression.RequestWeek(params))
			}
			return ""
		},
		Step: func(name string, by int) string {
			if name != domain.PageParameterWeek {
				return ""
			}
			return weekStep(params, by)
		},
		Window: func(datasetID string) (string, error) {
			return datasetWindowWords(l, datasetID, expression.Context{Parameters: params, Today: now.Format("2006-01-02")})
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
func bindingForm(ctx context.Context, l *Loader, viewer domain.Actor, params map[string]string, pageID string, b domain.PageBinding) (ir.FormSpec, error) {
	ds, ok := l.Dataset(b.Dataset)
	if !ok {
		return ir.FormSpec{}, fmt.Errorf("composition: page binding names dataset %q, which no machine declares", b.Dataset)
	}
	src := l.Machine(ds.Source)
	if src == nil {
		return ir.FormSpec{}, fmt.Errorf("composition: dataset %q names machine %q, which this Workspace does not install", b.Dataset, ds.Source)
	}
	inputs, _ := src.CreateFormInputs()
	inputs = domain.SelectInputs(inputs, b.FormFields)
	if err := fillChoices(ctx, l, src, inputs); err != nil {
		return ir.FormSpec{}, err
	}
	// `values:` from the request: a Field the request names starts as that value and is drawn fixed. One the request
	// does not send stays an ordinary control.
	ctxValues := expression.Context{CurrentUser: viewer.ID, Parameters: params}
	for i, in := range inputs {
		if declared, ok := b.Values[in.FieldID]; ok {
			if v, ok := ctxValues.Resolve(declared); ok && v != "" {
				inputs[i].Default, inputs[i].Fixed = v, true
			}
			continue
		}
		// `starts:` begins a Field with the request's value and leaves it a control: what a Copy link carries over.
		if declared, ok := b.Starts[in.FieldID]; ok {
			if v, ok := ctxValues.Resolve(declared); ok && v != "" {
				inputs[i].Default = v
			}
		}
	}
	spec := ir.FormSpec{
		Route:     domain.FormRoute(src.ID),
		Permitted: authorization.AllowsAction(src, domain.ActionCreate, nil, viewer),
		Inputs:    inputs,
	}
	if b.Extract != nil {
		if err := extractSpec(ctx, l, viewer, params, pageID, *b.Extract, src, &spec); err != nil {
			return ir.FormSpec{}, err
		}
	}
	return spec, nil
}

// extractSpec adds to a create Form what its `extract:` brings: the reader that sends a document, and -- when the page was
// opened with a proposal this viewer asked for on this page and has not saved -- the form filled from it, its proposed rows,
// the document's name, and a submit that goes to the page's own save route instead of the Machine's. A proposal that is
// not found, not this viewer's, not this page's or already saved is simply not used: the form stays the ordinary one.
func extractSpec(ctx context.Context, l *Loader, viewer domain.Actor, params map[string]string, pageID string, x domain.FormExtract, src *domain.Machine, spec *ir.FormSpec) error {
	if pageID == "" {
		return nil
	}
	if l.reader && spec.Permitted {
		label := x.File
		if f, ok := src.FieldByID(x.File); ok {
			label = f.Name
		}
		spec.Reader = &ir.FormReader{Route: "/pages/" + pageID + domain.ExtractRouteSuffix, Read: x.Read, Label: label}
	}
	id := params[domain.ExtractParam]
	workspaceID, scoped := data.WorkspaceScope(ctx)
	if id == "" || !scoped {
		return nil
	}
	rec, err := l.store.GetExtraction(ctx, workspaceID, id)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if rec.UserID != viewer.ID || rec.NavID != pageID || rec.Status != data.ExtractionProposed {
		return nil
	}
	var p domain.ExtractionProposal
	if err := json.Unmarshal(rec.Proposal, &p); err != nil {
		return nil
	}
	rows, err := applyProposal(ctx, l, x, spec.Inputs, p)
	if err != nil {
		return err
	}
	spec.Rows, spec.Carries = rows, domain.ExtractParam+"="+rec.ID
	spec.Route = "/pages/" + pageID + domain.ExtractSaveSuffix
	label := x.File
	if f, ok := src.FieldByID(x.File); ok {
		label = f.Name
	}
	spec.Document = label + ": " + rec.FileName
	if l.files != nil {
		if size := l.files.Size(rec.FileKey); size > 0 {
			spec.Document += " · " + humanSize(size)
		}
	}
	return nil
}

// fillChoices gives each reference select of a form its records: the related Machine's records by their labels, and the
// Workspace's members by name -- read once per related Machine through the picker's own reader (RelationOptions), so a
// page's form offers what the generic create form offers. Nothing is read for a form with no reference Field.
func fillChoices(ctx context.Context, l *Loader, src *domain.Machine, inputs []domain.FormInput) error {
	needs := false
	for _, in := range inputs {
		needs = needs || in.Reference != ""
	}
	if !needs {
		return nil
	}
	options, err := l.RelationOptions(ctx, src)
	if err != nil {
		return err
	}
	for i, in := range inputs {
		if in.Reference == "" {
			continue
		}
		for _, o := range options[in.Reference] {
			inputs[i].Choices = append(inputs[i].Choices, domain.FormChoice{Value: o.ID, Label: o.Label})
		}
	}
	return nil
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
		if strings.HasPrefix(c.Value, expression.SentinelParameterPrefix) && !c.Optional && c.IfParam == "" {
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
	people, err := recordRelations(ctx, l, src, records, b.Fields...)
	if err != nil {
		return ir.RecordSet{}, err
	}
	// The tags of exactly the listed records, in two statements however many there are (CardTagsFor), and none at
	// all when the Machine declares no `card_tags:`.
	tags, err := l.CardTagsFor(ctx, src, records)
	if err != nil {
		return ir.RecordSet{}, err
	}
	held, err := heldRecords(ctx, l, src, ds, sel)
	if err != nil {
		return ir.RecordSet{}, err
	}
	// An edge a declared Constraint would refuse is not offered either: the records a Constraint reads, once per page.
	var constraintRecords map[string][]*data.Record
	if len(src.Constraints) > 0 && len(src.Transitions) > 0 && len(records) > 0 {
		if constraintRecords, err = l.ConstraintRelatedRecords(ctx, src); err != nil {
			return ir.RecordSet{}, err
		}
	}
	out := make([]map[string]string, 0, len(records))
	tagLists := make([][]map[string]string, 0, len(records))
	edits := make([]ir.FormSpec, 0, len(records))
	deletes := make([]ir.RecordAction, 0, len(records))
	moves := make([]ir.RecordMove, 0, len(records))
	transitions := make([]ir.RecordTransition, 0, len(records))
	for i, r := range records {
		item := ProjectedByRole(src, r, people)
		item[domain.PageRecordRole] = recordRouteOf(src, r.ID)
		fillDerivedDate(item, src, r, now)
		fillDerivedStatus(item, src, r)
		fillFieldColumns(item, src, r, people, b.Fields, l.files)
		fillCreatedAndLink(item, src, r)
		item[domain.PageRecordIDRole] = r.ID
		fillDerivedGroup(item, src, r, people)
		edit, del := recordEditForm(src, r, viewer), recordDeleteAction(src, r, viewer)
		move, transition := recordMove(src, r, viewer, i > 0, i < len(records)-1 || sel.Truncated), recordTransition(src, r, viewer)
		// What the routes would refuse for a reason outside the record itself is not offered either: a record frozen by
		// its parent is not written at all, and one other records still point at (`on_parent_delete: restrict`) is not
		// deleted.
		if held[r.ID].frozen {
			edit.Permitted, del.Permitted, move = false, false, ir.RecordMove{Route: move.Route}
			transition.Allows = func(string) (bool, bool) { return false, true }
			for id, e := range transition.Edges {
				e.Allowed = false
				transition.Edges[id] = e
			}
		}
		if held[r.ID].pointedAt {
			del.Permitted = false
		}
		if err := offerEdges(ctx, l, src, r, &transition, constraintRecords); err != nil {
			return ir.RecordSet{}, err
		}
		for id, e := range transition.Edges {
			item[domain.PageBlockedRoleOf(id)] = e.Blocked
		}
		edits = append(edits, edit)
		deletes = append(deletes, del)
		moves = append(moves, move)
		transitions = append(transitions, transition)
		for _, rel := range ds.Relations {
			children := sel.Related(rel.ID, r.ID)
			item[domain.PageCountRole(rel.ID)] = strconv.Itoa(len(children))
			// The first record the Relation found, for a Table's `related:` column: its title, and its status in its tone.
			item[domain.PageRelatedRole(rel.ID)] = ""
			// A Field of that first record, for a `related: rel_x.fld_y` column.
			for _, rf := range b.RelatedFields {
				if relation, field := domain.SplitRelatedColumn(rf); relation == rel.ID {
					item[domain.PageRelatedFieldRole(relation, field)] = ""
					if child := l.Machine(rel.Machine); child != nil && len(children) > 0 {
						if f, ok := child.FieldByID(field); ok {
							item[domain.PageRelatedFieldRole(relation, field)] = projectedFieldDisplay(f, children[0].Values[field], nil)
						}
					}
				}
			}
			if child := l.Machine(rel.Machine); child != nil && len(children) > 0 {
				// Named as a record is named everywhere (its title role, else its first Field); its status, when it declares one.
				item[domain.PageRelatedRole(rel.ID)] = DisplayString(children[0].Values[child.LabelField()])
				statusField := child.CardFieldFor(domain.CardFieldRoleStatus)
				if statusField == "" {
					for _, f := range child.Fields {
						if f.Type == domain.FieldTypeStatus {
							statusField = f.ID
							break
						}
					}
				}
				if st := DisplayString(children[0].Values[statusField]); statusField != "" && st != "" {
					item[domain.PageRelatedStatusRole(rel.ID)] = st
					item[domain.PageRelatedToneRole(rel.ID)] = string(child.StatusTone(statusField, st))
				}
			}
		}
		out = append(out, item)
		var list []map[string]string
		for _, t := range tags[r.ID] {
			list = append(list, map[string]string{string(domain.CardFieldRoleTitle): t.Label, string(domain.CardFieldRoleColor): string(t.Color)})
		}
		tagLists = append(tagLists, list)
	}
	set := ir.RecordSet{Records: out, Tags: tagLists, Edits: edits, Deletes: deletes, Moves: moves, Transitions: transitions, Truncated: sel.Truncated, Limit: sel.Limit}
	if sel.Truncated && b.CountCut {
		// A list cut by its bound says what it was cut from ("the first 100 of 340", owner decision K2). One count,
		// and only when the bound bit.
		if set.Total, err = l.CountSelected(ctx, b.Dataset, where); err != nil {
			return ir.RecordSet{}, err
		}
	}
	if b.Group == domain.PageGroupDate {
		if set.Buckets, set.BucketKeys, err = dateBuckets(ds, src, records, where); err != nil {
			return ir.RecordSet{}, err
		}
	}
	if src.Completion != nil {
		set.Done, set.Reopen = src.Completion.Done, src.ReopenValue()
	}
	return set, nil
}

// weekStep is the `?week=` value a link `by` weeks from the request's own should carry: the offset moved and
// clamped to the same bound the Dataset's `$week` reads, and "" for this week, which is the plain route rather
// than `?week=0` -- the form every other link to the page already has.
func weekStep(params map[string]string, by int) string {
	// by is clamped first: the sum of two ints near the extremes wraps, and a wrapped sum clamps to the wrong end.
	by = max(-expression.MaxWeekOffset, min(expression.MaxWeekOffset, by))
	next := max(-expression.MaxWeekOffset, min(expression.MaxWeekOffset, expression.RequestWeek(params)+by))
	if next == 0 {
		return ""
	}
	return strconv.Itoa(next)
}

// datasetWindowWords is `window:`: the Dataset's window as calendarRange says it ("12 \u2013 18 Oct 2026").
func datasetWindowWords(l *Loader, datasetID string, where expression.Context) (string, error) {
	ds, ok := l.Dataset(datasetID)
	if !ok {
		return "", fmt.Errorf("composition: window names dataset %q, which no machine declares", datasetID)
	}
	src := l.Machine(ds.Source)
	if src == nil {
		return "", fmt.Errorf("composition: dataset %q reads machine %q, which this Workspace does not install", datasetID, ds.Source)
	}
	return windowWords(ds, src, where)
}

func windowWords(ds domain.Dataset, src *domain.Machine, where expression.Context) (string, error) {
	lo, hi, err := dateWindow(ds, src, where)
	if err != nil {
		return "", err
	}
	return calendarRange(lo, hi), nil
}

// dateWindow reads a Dataset's window: the day its `gte` and the day its `lte` bound on the Machine's `date` card
// role, resolved against the request. `group: date` draws one bucket for each day between them and `window:` says
// them in words, so both read here and cannot disagree about which days the Dataset holds.
func dateWindow(ds domain.Dataset, src *domain.Machine, where expression.Context) (lo, hi time.Time, err error) {
	field := src.CardFieldFor(domain.CardFieldRoleDate)
	for _, c := range ds.Where.All {
		if c.Field != field || (c.Op != expression.OpGreaterOrEqual && c.Op != expression.OpLessOrEqual) {
			continue
		}
		v, ok := where.Resolve(c.Value)
		if !ok {
			return lo, hi, fmt.Errorf("composition: dataset %q: the %s bound %q resolves to nothing", ds.ID, c.Op, c.Value)
		}
		day, perr := time.Parse("2006-01-02", v)
		if perr != nil {
			return lo, hi, fmt.Errorf("composition: dataset %q: the %s bound %q resolved to %q, which is not a date", ds.ID, c.Op, c.Value, v)
		}
		if c.Op == expression.OpGreaterOrEqual {
			lo = day
		} else {
			hi = day
		}
	}
	if lo.IsZero() || hi.IsZero() || hi.Before(lo) {
		return lo, hi, fmt.Errorf("composition: dataset %q: window %s..%s is not a range of days", ds.ID, lo.Format("2006-01-02"), hi.Format("2006-01-02"))
	}
	if days := int(hi.Sub(lo).Hours()/24) + 1; days > domain.PageGroupMaxBuckets {
		return lo, hi, fmt.Errorf("composition: dataset %q: window of %d days exceeds the %d a group draws", ds.ID, days, domain.PageGroupMaxBuckets)
	}
	return lo, hi, nil
}

// dateBuckets is `group: date`: one bucket per day from the Dataset's own `gte` to its own `lte` on the Machine's
// `date` card role, and for each record the day it falls on. The window is **read from the Dataset's `where:`**,
// resolved the way the database resolves it (`$week`, `$today+n`, a written date), so the days drawn and the
// records selected cannot disagree. metadata.groupBindingIssues guarantees exactly one of each bound at load;
// what is checked again here is what only a request can break -- a bound that resolves to no date, an inverted
// window, one longer than domain.PageGroupMaxBuckets days. A record whose date is not a bucket is an error and
// not a skipped row, since the Dataset's own filter is what put it here.
func dateBuckets(ds domain.Dataset, src *domain.Machine, records []*data.Record, where expression.Context) ([]ir.Bucket, []string, error) {
	field := src.CardFieldFor(domain.CardFieldRoleDate)
	lo, hi, err := dateWindow(ds, src, where)
	if err != nil {
		return nil, nil, err
	}
	var buckets []ir.Bucket
	for d := lo; !d.After(hi); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		buckets = append(buckets, ir.Bucket{Key: key, Label: d.Format("Mon 02 Jan"), Current: where.Today != "" && key == where.Today})
	}
	keys := make([]string, len(records))
	for i, r := range records {
		keys[i] = DisplayString(r.Values[field])
	}
	return buckets, keys, nil
}

// fillDerivedGroup puts the name of the group the record sits in under `group:label`: the title of the record its
// default View's group Field points at, or the stored option itself when that Field is not a reference. "" when
// the Machine's default View groups by nothing or the record has no value, which leaves a node reading it undrawn.
func fillDerivedGroup(item map[string]string, src *domain.Machine, r *data.Record, relations rendering.RelationOptions) {
	f, ok := src.FieldByID(src.DefaultView().GroupBy)
	if !ok {
		return
	}
	value := DisplayString(r.Values[f.ID])
	if f.IsReference() {
		value = rendering.RelationLabel(relations, f.RelatedMachine, value)
	}
	item[domain.PageGroupLabelRole] = value
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
func recordRelations(ctx context.Context, l *Loader, src *domain.Machine, records []*data.Record, columns ...string) (rendering.RelationOptions, error) {
	people, containers := map[string]string{}, map[string]string{}
	// A Table column that names a reference Field (PageColumn.Field) is resolved the same bounded way: a person from
	// the memoized member names, any other relation by the ids the listed records name, in one statement per Machine.
	for _, id := range columns {
		f, ok := src.FieldByID(id)
		if !ok || !f.IsReference() {
			continue
		}
		if f.RelatedMachine == domain.UserMachineID {
			people[f.ID] = f.RelatedMachine
		} else {
			containers[f.ID] = f.RelatedMachine
		}
	}
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
	// The Field the default View groups by is read the same way: the group's name is the related record's title.
	if g, ok := src.FieldByID(src.DefaultView().GroupBy); ok && g.IsReference() && g.RelatedMachine != domain.UserMachineID {
		containers[g.ID] = g.RelatedMachine
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
	editable := !src.AppendOnly && authorization.AllowsAction(src, domain.ActionEdit, r.Values, viewer)
	t.Edges = make(map[string]ir.EdgeOffer, len(src.Transitions))
	for _, e := range src.Transitions {
		offer := ir.EdgeOffer{Name: e.Name, Field: e.Field, To: e.To, Inputs: src.TransitionInputs(e)}
		next := map[string]any{e.Field: e.To}
		offer.Allowed = editable && e.Action == domain.ActionEdit && DisplayString(r.Values[e.Field]) == e.From &&
			behavior.CheckTransitions(src, domain.ActionEdit, r.Values, next) == nil && authorization.AllowsTransition(src, e, r.Values, viewer)
		t.Edges[e.ID] = offer
	}
	f, ok := src.FieldByID(field)
	if !ok {
		return t
	}
	current := DisplayString(r.Values[field])
	t.Allows = func(target string) (bool, bool) {
		if !slices.Contains(f.Options, target) {
			return false, false
		}
		if !editable || target == current {
			return false, true
		}
		next := map[string]any{field: target}
		if behavior.CheckTransitions(src, domain.ActionEdit, r.Values, next) != nil {
			return false, true
		}
		for _, e := range behavior.MovedTransitions(src, r.Values, next) {
			if !authorization.AllowsTransition(src, e, r.Values, viewer) {
				return false, true
			}
		}
		return true, true
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
		rows = append(rows, ir.Row{Label: k, Value: agg.Format(b.Measure, agg.ByDimension[k][b.Measure]), Measures: rowMeasures(agg, k), Numbers: rowNumbers(agg, k)})
	}
	return rows, nil
}

// referenceRows is a Dimension over a reference Field (a Project, a person): its stored value is a record id, so
// each row is named by that record's title -- the label a picker offers for the same record (`optionsOf`) -- and
// keeps the id as its Key. Ordered by title, then id, since an id carries no order worth showing and §4.6 forbids
// leaving it to map iteration. A value naming no record that exists (one deleted since) has no title to show and is
// left out, as an unset one is, rather than drawn as an opaque id.
func referenceRows(ctx context.Context, l *Loader, dim domain.Field, agg Aggregation, measure string) ([]ir.Row, error) {
	options, optionRecords, _, err := l.optionRecords(ctx, dim.RelatedMachine)
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
	// The related records themselves, for a Table's `row:<field>` columns (a category's type): the read optionsOf made,
	// served again from the Loader's memo.
	related := map[string]*data.Record{}
	target := l.Machine(dim.RelatedMachine)
	for _, r := range optionRecords {
		related[r.ID] = r
	}
	rows := make([]ir.Row, 0, len(keys))
	for _, k := range keys {
		row := ir.Row{Label: titles[k], Key: k, Value: agg.Format(measure, agg.ByDimension[k][measure]), Measures: rowMeasures(agg, k), Numbers: rowNumbers(agg, k)}
		if r := related[k]; r != nil && target != nil {
			row.Attrs = map[string]string{}
			for _, f := range target.Fields {
				if !f.IsReference() {
					row.Attrs[f.ID] = projectedFieldDisplay(f, r.Values[f.ID], nil)
				}
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// rowNumbers is every Measure of one Dimension value as a bare number, what a fraction's bar reads.
func rowNumbers(agg Aggregation, key string) map[string]string {
	out := make(map[string]string, len(agg.Total))
	for id := range agg.Total {
		out[id] = strconv.FormatFloat(agg.ByDimension[key][id], 'f', -1, 64)
	}
	return out
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
	return agg.Format(b.Measure, v), nil
}

// aggregateContext is what a Measure's `where:` may resolve: the request's date and nothing else. An aggregate
// has no viewer and no request parameters, and metadata refuses a Measure that names either.
func aggregateContext(now time.Time) expression.Context {
	return expression.Context{Today: now.Format("2006-01-02")}
}

// rowMeasures is every declared Measure's figure for one Dimension value -- the Measures are agg.Total's keys,
// which always holds each one -- with a Measure the group has no value for as 0, as the Total's own zero is.
func rowMeasures(agg Aggregation, key string) map[string]string {
	out := make(map[string]string, len(agg.Total))
	for id := range agg.Total {
		out[id] = agg.Format(id, agg.ByDimension[key][id])
	}
	return out
}

// fillCreatedAndLink puts two things every record can say in its map. The creation time is a column, not a Field, so a
// page reads it as the derived role domain.PageCreatedLabelRole. A Machine's `link` role holds a route as plain text, and
// what a page may be handed is only a route inside the app (domain.SafeInAppPath): anything else is cut to "" here, so a
// stored value can never become a link that leaves the app, whatever wrote it.
func fillCreatedAndLink(item map[string]string, src *domain.Machine, r *data.Record) {
	item[domain.PageCreatedLabelRole] = r.CreatedAt.Format("2 Jan 2006 15:04")
	if _, has := item[string(domain.CardFieldRoleLink)]; has && !domain.SafeInAppPath(item[string(domain.CardFieldRoleLink)]) {
		item[string(domain.CardFieldRoleLink)] = ""
	}
}

// fillDerivedDate puts the two derived date roles (domain.PageDerivedRole) in a record's map, for a Machine that
// declares a `date` card_fields role. experience.ResolveCardDate does the rule -- the short date, bad once past,
// good once the record is complete -- with the request's `now` and not the clock (007 §4.6). A record with no date
// holds both as "", which is how a node reading them is left undrawn (ir.omittedForWantOfDate).
func fillDerivedDate(item map[string]string, src *domain.Machine, r *data.Record, now time.Time) {
	fieldID := src.CardFieldFor(domain.CardFieldRoleDate)
	if fieldID == "" {
		return
	}
	d := experience.ResolveCardDate(r.Values[fieldID], IsComplete(src, r), now)
	item[domain.PageDateLabelRole], item[domain.PageDateToneRole] = d.Label, string(d.Tone)
	item[domain.PageDateRelativeRole] = relativeDay(DisplayString(r.Values[fieldID]), d.Done, now)
	if !d.Present {
		item[domain.PageDateLabelRole], item[domain.PageDateToneRole], item[domain.PageDateRelativeRole] = "", "", ""
	}
}

// offerEdges finishes what a record's edges offer once its Machine's own state model has spoken (recordTransition): an
// edge a declared Constraint refuses for this record is not offered, and an edge's inputs leave out what the record
// already holds (a proof uploaded with the plan is not asked again) and offer a relation's records.
func offerEdges(ctx context.Context, l *Loader, src *domain.Machine, r *data.Record, t *ir.RecordTransition, constraintRecords map[string][]*data.Record) error {
	for id, e := range t.Edges {
		if e.Allowed && constraintRecords != nil {
			next := maps.Clone(r.Values)
			next[e.Field] = e.To
			if err := behavior.CheckConstraints(src, r.ID, next, constraintRecords); err != nil {
				e.Allowed = false
				// The edge is held back for its record: its Constraint's own words say why, for a screen that wants to.
				var held *behavior.ConstraintError
				if errors.As(err, &held) {
					e.Blocked = strings.Join(held.Reasons, " ")
				}
			}
		}
		var inputs []domain.FormInput
		for _, in := range e.Inputs {
			if DisplayString(r.Values[in.FieldID]) == "" {
				inputs = append(inputs, in)
			}
		}
		if e.Allowed {
			if err := fillChoices(ctx, l, src, inputs); err != nil {
				return err
			}
		}
		e.Inputs = inputs
		t.Edges[id] = e
	}
	return nil
}

// held is what keeps a listed record from being written beyond its own values: frozen by its parent (`frozen_when:`),
// or pointed at by records that restrict deleting it (`on_parent_delete: restrict`).
type held struct{ frozen, pointedAt bool }

// heldRecords answers held for the listed records in one statement per parent Machine and per restricting child
// Machine, however many records are listed -- the same answers frozenReason and deleteAllowed give one record at the
// routes, so a page offers no Edit or Delete the route would refuse. Nothing is read for a Machine that declares
// neither.
func heldRecords(ctx context.Context, l *Loader, src *domain.Machine, ds domain.Dataset, sel Selection) (map[string]held, error) {
	if cached, ok := l.held[ds.ID]; ok {
		l.served++
		return cached, nil
	}
	out, err := readHeldRecords(ctx, l, src, ds, sel)
	if err == nil {
		l.held[ds.ID] = out
	}
	return out, err
}

func readHeldRecords(ctx context.Context, l *Loader, src *domain.Machine, ds domain.Dataset, sel Selection) (map[string]held, error) {
	records := sel.Records
	out := map[string]held{}
	if len(records) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
	}
	if f := src.Frozen; f != nil {
		if via, ok := src.FieldByID(f.Via); ok {
			var parentIDs []string
			for _, r := range records {
				if id := DisplayString(r.Values[f.Via]); id != "" {
					parentIDs = append(parentIDs, id)
				}
			}
			parents, err := l.RecordsByIDs(ctx, via.RelatedMachine, parentIDs)
			if err != nil {
				return nil, err
			}
			frozenParent := map[string]bool{}
			for _, p := range parents {
				frozenParent[p.ID] = DisplayString(p.Values[f.Field]) == f.Equals
			}
			for _, r := range records {
				if frozenParent[DisplayString(r.Values[f.Via])] {
					h := out[r.ID]
					h.frozen = true
					out[r.ID] = h
				}
			}
		}
	}
	machines := make([]*domain.Machine, 0, len(l.machines))
	for _, id := range slices.Sorted(maps.Keys(l.machines)) {
		machines = append(machines, l.machines[id])
	}
	for _, link := range domain.RestrictingChildren(machines, src.ID) {
		// A Relation the Dataset already declares over the same child and Field has read them: no second statement.
		var children []*data.Record
		declared := false
		for _, rel := range ds.Relations {
			if rel.Machine == link.Machine.ID && rel.Via == link.Field && rel.Where == nil {
				declared = true
				for _, id := range ids {
					children = append(children, sel.Related(rel.ID, id)...)
				}
			}
		}
		if !declared {
			var err error
			if children, err = l.store.ListRecordsByAny(ctx, link.Machine.ID, ds.ID+" restrict", link.Field, ids); err != nil {
				return nil, err
			}
		}
		for _, c := range children {
			parent := DisplayString(c.Values[link.Field])
			h := out[parent]
			h.pointedAt = true
			out[parent] = h
		}
	}
	return out, nil
}

// fillDerivedStatus is PageStatusToneRole: the tone of the record's `status` role, from the Machine's own
// `status_categories:`, so a page's badge and the generic list's cell cannot disagree about an option.
func fillDerivedStatus(item map[string]string, src *domain.Machine, r *data.Record) {
	fieldID := src.CardFieldFor(domain.CardFieldRoleStatus)
	if fieldID == "" {
		return
	}
	item[domain.PageStatusToneRole] = ""
	if v := DisplayString(r.Values[fieldID]); v != "" {
		item[domain.PageStatusToneRole] = string(src.StatusTone(fieldID, v))
	}
}

// fillFieldColumns is the Fields a Table's columns name by id (PageColumn.Field): each as the record's own page shows
// it, and a status Field's tone beside it. The loader has checked that every id is a Field of src.
func fillFieldColumns(item map[string]string, src *domain.Machine, r *data.Record, relations rendering.RelationOptions, fields []string, files *storage.Store) {
	for _, id := range fields {
		f, ok := src.FieldByID(id)
		if !ok {
			continue
		}
		if f.Type == domain.FieldTypeFile {
			// A file's value is a storage key: a page reads its name, its size and the address that serves it.
			// Only a key the storage itself would serve (inside its root): a client-written string is no address.
			if key := DisplayString(r.Values[id]); key != "" && files != nil && storageKey(files, key) {
				name := storage.DisplayName(key)
				if size := humanSize(files.Size(key)); size != "" {
					name += " · " + size
				}
				item[domain.PageFileRole(id)], item[domain.PageFileHrefRole(id)] = name, "/uploads/"+key
			} else {
				item[domain.PageFileRole(id)], item[domain.PageFileHrefRole(id)] = "", ""
			}
			continue
		}
		item[domain.PageFieldRole(id)] = projectedFieldDisplay(f, r.Values[id], relations)
		item[domain.PageFieldValueRole(id)] = DisplayString(r.Values[id])
		if n, ok := domain.FigureOf(r.Values[id]); ok && f.Type.IsNumeric() {
			item[domain.PageRowNumberRole(id)] = strconv.FormatFloat(n, 'f', -1, 64)
		}
		if f.Type != domain.FieldTypeStatus {
			continue
		}
		v := DisplayString(r.Values[id])
		// A status with no value yet (a PO whose contract has none) has no tone to speak of: neutral, so a template reading
		// `tone:` is drawn rather than failing the page for want of the key.
		item[domain.PageFieldToneRole(id)] = string(domain.ToneNeutral)
		if v != "" {
			item[domain.PageFieldToneRole(id)] = string(src.StatusTone(id, v))
		}
		item[domain.PageFieldStepsRole(id)] = statusSteps(src, f, v)
	}
}

// statusSteps is a status Field's options as a Stepper draws them (domain.PageFieldStepsRole): done before the current
// one, current, todo after; an option the Machine categorises `stopped` (Cancelled) is a way out, not a step, and is drawn
// only when it is where the record is.
func statusSteps(m *domain.Machine, f domain.Field, current string) string {
	var lines []string
	// A record that left by a way out (Cancelled) did not pass the steps written before it: none is drawn done.
	reached := slices.Contains(f.Options, current)
	if c, ok := m.OptionCategory(f.ID, current); ok && c == domain.StatusStopped {
		reached = false
	}
	for _, o := range f.Options {
		if c, ok := m.OptionCategory(f.ID, o); ok && c == domain.StatusStopped && o != current {
			continue
		}
		state := "todo"
		switch {
		case o == current:
			state, reached = "current", false
		case reached:
			state = "done"
		}
		lines = append(lines, o+"\t"+state)
	}
	return strings.Join(lines, "\n")
}

// relativeDay is a stored day said against now's (domain.PageDateRelativeRole): "12 days late", "1 day late",
// "today", "tomorrow", "in 5 days"; a finished record's day is said as its date, since lateness no longer applies.
func relativeDay(stored string, done bool, now time.Time) string {
	due, err := time.Parse(domain.DateLayout, stored)
	if err != nil {
		return ""
	}
	if done {
		return domain.FormatDay(stored)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	days := int(due.Sub(today).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow"
	case days == -1:
		return "1 day late"
	case days < 0:
		return strconv.Itoa(-days) + " days late"
	}
	return "in " + strconv.Itoa(days) + " days"
}

// bindingTransitionAll is one bulk transition Binding: the route, and how many of the Dataset's records this viewer
// could move to the status `becomes:` names. The count is courtesy (the route judges each record again), and it is what
// keeps a "mark all as read" from being drawn when everything already is.
func bindingTransitionAll(ctx context.Context, l *Loader, viewer domain.Actor, params map[string]string, now time.Time, b domain.PageBinding, becomes string) (ir.AllTransition, error) {
	src, target, records, err := AllTransitionRecords(ctx, l, viewer, params, now, b.Dataset, becomes)
	if err != nil {
		return ir.AllTransition{}, err
	}
	n := 0
	for _, r := range records {
		if allowed, _ := recordTransition(src, r, viewer).Allows(target); allowed {
			n++
		}
	}
	return ir.AllTransition{Route: domain.TransitionAllRoute(src.ID, b.Dataset, target), Count: n}, nil
}

// AllTransitionRecords is what a bulk transition acts on: the Machine, the status `becomes` stands for (a `$done`
// or `$reopen` is read from the Machine's `completion:`), and the records the Dataset selects for this viewer, in the
// Dataset's own order. The page's Button and the route that serves it both call this, so what is drawn and what is
// moved are selected by one function. A Dataset filtering on a parameter the request did not send selects nothing.
func AllTransitionRecords(ctx context.Context, l *Loader, viewer domain.Actor, params map[string]string, now time.Time, datasetID, becomes string) (*domain.Machine, string, []*data.Record, error) {
	ds, ok := l.Dataset(datasetID)
	if !ok {
		return nil, "", nil, fmt.Errorf("composition: bulk transition names dataset %q, which no machine declares", datasetID)
	}
	src := l.Machine(ds.Source)
	if src == nil {
		return nil, "", nil, fmt.Errorf("composition: dataset %q reads machine %q, which this Workspace does not install", datasetID, ds.Source)
	}
	if ds.Select != domain.SelectRecords {
		return nil, "", nil, fmt.Errorf("composition: dataset %q selects no records", datasetID)
	}
	target := becomes
	switch becomes {
	case domain.PageTransitionDone:
		if src.Completion == nil {
			return nil, "", nil, fmt.Errorf("composition: %s needs machine %q to declare a `completion:`", becomes, src.ID)
		}
		target = src.Completion.Done
	case domain.PageTransitionReopen:
		target = src.ReopenValue()
	}
	field, ok := src.FieldByID(src.CardFieldFor(domain.CardFieldRoleStatus))
	if !ok || !slices.Contains(field.Options, target) {
		return nil, "", nil, fmt.Errorf("composition: %q is not an option of machine %q's status Field", target, src.ID)
	}
	where := expression.Context{CurrentUser: viewer.ID, Parameters: params, Today: now.Format("2006-01-02")}
	for _, c := range ds.Where.Comparisons() {
		if strings.HasPrefix(c.Value, expression.SentinelParameterPrefix) && !c.Optional && c.IfParam == "" {
			if _, ok := where.Resolve(c.Value); !ok {
				return src, target, nil, nil
			}
		}
	}
	records, err := l.SelectDataset(ctx, datasetID, where)
	if err != nil {
		return nil, "", nil, err
	}
	return src, target, records, nil
}

// bindingAction is one runtime-action Binding: what a Form for it asks, and whether this viewer may perform it. The
// permission is courtesy (the route asks again): an admin-only action is not drawn for a member, so a page does not
// offer what the route would refuse. The same split the Workspace menu makes: anyone who is not a plain member counts as an
// admin here, because the shared admin credential has no role and is let through the gate.
//
// The dynamic inputs (registry.ActionInputMembers, ActionInputAppRoles) are decided here, for the record the Form sits
// beside: the Workspace's members with the record's own ticked, and a select per Application that declares roles with the
// record's role there chosen. The page writes neither an option nor a name.
func bindingAction(ctx context.Context, l *Loader, viewer domain.Actor, b domain.PageBinding, recordID string) (ir.FormSpec, error) {
	a, ok := registry.RuntimeActions[b.Action]
	if !ok {
		return ir.FormSpec{}, fmt.Errorf("composition: page binding names action %q, which is not an operation the runtime performs", b.Action)
	}
	spec := ir.FormSpec{Permitted: !a.Admin || viewer.WorkspaceRole != domain.WorkspaceRoleMember}
	if !spec.Permitted {
		return spec, nil
	}
	for _, in := range a.Inputs {
		switch in.Kind {
		case registry.ActionInputMembers:
			members, err := l.Members(ctx)
			if err != nil {
				return ir.FormSpec{}, err
			}
			workspaceID, _ := data.WorkspaceScope(ctx)
			ticked, err := l.store.GroupMemberIDs(ctx, workspaceID, recordID)
			if err != nil {
				return ir.FormSpec{}, err
			}
			var options []string
			for _, m := range members {
				options = append(options, m.UserRecordID+"="+m.Email)
			}
			spec.Inputs = append(spec.Inputs, domain.FormInput{FieldID: in.Name, Label: in.Label, Kind: domain.InputMultiSelect, Options: options, Default: strings.Join(ticked, domain.InputOptionsSep)})
		case registry.ActionInputAppRoles:
			held, err := recordAppRoles(ctx, l, a.Source, recordID)
			if err != nil {
				return ir.FormSpec{}, err
			}
			for _, app := range rendering.CurrentWorkspace(ctx).Applications {
				if len(app.Roles) == 0 {
					continue
				}
				spec.Inputs = append(spec.Inputs, domain.FormInput{FieldID: domain.AppRoleField(app.ID), Label: app.Name, Kind: domain.InputSelect, Options: app.Roles, Default: held[app.ID]})
			}
		case registry.ActionInputRecordText:
			text, err := sourceRecordText(ctx, l, viewer, a.Source, recordID, in.Field)
			if err != nil {
				return ir.FormSpec{}, err
			}
			spec.Inputs = append(spec.Inputs, domain.FormInput{FieldID: in.Name, Label: in.Label, Kind: domain.InputCode, Required: in.Required, Default: text})
		case registry.ActionInputTemplateRoles:
			roles, err := sourceRecordRoles(ctx, l, viewer, a.Source, recordID)
			if err != nil {
				return ir.FormSpec{}, err
			}
			if len(roles) > 0 {
				spec.Inputs = append(spec.Inputs, domain.FormInput{FieldID: in.Name, Label: in.Label, Kind: domain.InputSelect, Options: roles, Default: roles[0]})
			}
		case registry.ActionInputMemberAppRole:
			hubID, userID, _ := strings.Cut(recordID, ":")
			app, _, found := SettingsApplication(rendering.CurrentWorkspace(ctx), hubID)
			if !found {
				return ir.FormSpec{}, fmt.Errorf("composition: %q names no Application settings hub of this Workspace", hubID)
			}
			members, err := l.Members(ctx)
			if err != nil {
				return ir.FormSpec{}, err
			}
			direct := ""
			for _, m := range members {
				if m.UserRecordID == userID {
					direct = m.AppRoles[app.ID]
				}
			}
			spec.Inputs = append(spec.Inputs, domain.FormInput{FieldID: in.Name, Label: in.Label, Kind: domain.InputSelect, Options: app.Roles, Default: direct})
		case registry.ActionInputWorkspaceRole:
			members, err := l.Members(ctx)
			if err != nil {
				return ir.FormSpec{}, err
			}
			current := ""
			for _, m := range members {
				if m.UserRecordID == recordID {
					current = m.WorkspaceRole
				}
			}
			spec.Inputs = append(spec.Inputs, domain.FormInput{FieldID: in.Name, Label: in.Label, Kind: domain.InputSelect, Options: []string{domain.WorkspaceRoleAdmin, domain.WorkspaceRoleMember}, Default: current, Required: in.Required})
		default:
			spec.Inputs = append(spec.Inputs, domain.FormInput{FieldID: in.Name, Label: in.Label, Kind: domain.InputText, Required: in.Required})
		}
	}
	return spec, nil
}

// navigationSection answers a NavList's section from the runtime's own screens: the entries are those that declare
// themselves part of it, grouped in the order their first row is declared and ordered inside a group by Priority (ties
// keep declaration order). The label and the way back are the settings hub's own, never retyped.
func navigationSection(section string) (ir.NavSection, error) {
	if section != domain.NavSectionWorkspaceSettings {
		return ir.NavSection{}, fmt.Errorf("composition: navigation section %q is not declared", section)
	}
	var hub domain.NavigationItem
	groupOrder := []string{}
	byGroup := map[string][]domain.NavigationItem{}
	for _, s := range domain.RuntimeScreens {
		if s.ID == domain.NavSectionHub[section] {
			hub = s
		}
		if s.SettingsSection != section {
			continue
		}
		if _, seen := byGroup[s.SettingsGroup]; !seen {
			groupOrder = append(groupOrder, s.SettingsGroup)
		}
		byGroup[s.SettingsGroup] = append(byGroup[s.SettingsGroup], s)
	}
	out := ir.NavSection{Label: hub.Label, Home: hub.Route}
	for _, g := range groupOrder {
		items := byGroup[g]
		sort.SliceStable(items, func(i, j int) bool { return items[i].Priority < items[j].Priority })
		for _, it := range items {
			out.Entries = append(out.Entries, ir.NavEntry{ID: it.ID, Group: g, Label: it.Heading, Description: it.Description, Href: it.Route, Placeholder: it.Placeholder, SidebarOnly: it.SidebarOnly})
		}
	}
	return out, nil
}

// recordRouteOf is the screen a listed record opens: the generic record page for a stored Machine, the source's own
// screen for a runtime one (a runtime record has no generic page).
func recordRouteOf(src *domain.Machine, id string) string {
	if src.Runtime != "" {
		return strings.ReplaceAll(registry.RuntimeSources[src.Runtime].RecordRoute, "{id}", url.PathEscape(id))
	}
	if src.RecordPage != "" {
		return src.RecordPage + "/" + url.PathEscape(id)
	}
	return RecordRoute(src.ID, id)
}

// recordAppRoles is the role the record already holds in each Application, the state a role select starts as: a Group's
// grants for a Group, a member's direct assignment for a member. Nothing for any other source.
func recordAppRoles(ctx context.Context, l *Loader, source, recordID string) (map[string]string, error) {
	switch source {
	case "groups":
		groups, err := l.Groups(ctx)
		if err != nil {
			return nil, err
		}
		for _, g := range groups {
			if g.ID == recordID {
				return g.Grants, nil
			}
		}
	case "members":
		members, err := l.Members(ctx)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if m.UserRecordID == recordID {
				return m.AppRoles, nil
			}
		}
	}
	return nil, nil
}

// storageKey says key is one the file store resolves to a path inside its root.
func storageKey(files *storage.Store, key string) bool {
	_, err := files.Path(key)
	return err == nil
}
