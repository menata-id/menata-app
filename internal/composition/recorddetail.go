package composition

import (
	"context"
	"slices"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/expression"
	"menata.app/internal/rendering"
)

// recordActivityDataset is one record's own history (metadata/activity.yaml, Case 19 PM02). Its two
// `$parameters` -- the Machine and the record the page is about -- are what makes it the first Dataset to
// need a request's own values.
const recordActivityDataset = "ds_record_activity"

// RecordExtras resolves what a record's detail page draws beside its Fields, none of it a Field of the
// record itself: the tags its Machine declares (`card_tags:`) and the record's own activity.
//
// Both are optional by construction. A Machine with no `card_tags:` has no chips, and a Workspace whose
// Machines do not include one declaring `ds_record_activity` has no history to show, so the section is
// absent rather than an error -- an activity log is a runtime reference some Workspace may not carry, and
// a detail page that 500s for lacking it would be the outage class CLAUDE.md warns about, for a section
// that is not load-bearing.
func (l *Loader) RecordExtras(ctx context.Context, m *domain.Machine, r *data.Record) (rendering.RecordExtras, error) {
	var out rendering.RecordExtras

	tags, err := l.CardTagsFor(ctx, m, []*data.Record{r})
	if err != nil {
		return out, err
	}
	out.Tags = tags[r.ID]

	if out.Move, err = l.recordMove(ctx, m, r); err != nil {
		return out, err
	}

	if _, ok := l.Dataset(recordActivityDataset); !ok {
		return out, nil
	}
	sel, err := l.SelectRelated(ctx, recordActivityDataset, expression.Context{
		Parameters: map[string]string{"machine": m.ID, "record": r.ID},
	})
	if err != nil {
		return out, err
	}
	names, err := l.PersonNames(ctx)
	if err != nil {
		return out, err
	}
	out.ShowActivity = true
	out.Activity = buildRecordActivity(sel.Records, names)
	out.ActivityTruncation = rendering.Truncation{Limit: sel.Limit, Hit: sel.Truncated}
	return out, nil
}

// buildRecordActivity maps the selected events to what the page draws. The order is the Dataset's own
// (newest first); nothing here sorts, and no clock is read -- a time is the event's own creation time.
func buildRecordActivity(events []*data.Record, names map[string]string) []rendering.ActivityEntry {
	out := make([]rendering.ActivityEntry, 0, len(events))
	for _, e := range events {
		out = append(out, rendering.ActivityEntry{
			Summary: DisplayString(e.Values["fld_summary"]),
			Actor:   names[DisplayString(e.Values["fld_actor"])],
			When:    e.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	return out
}

// recordMove resolves the Move panel: the lists the record can go to and the place it holds in its own.
// Nil for a Machine with no board View, and for a record whose group Field holds no list a value produces
// (the board's synthetic "Other" column), since nothing can be moved *into* that and a panel that opened on
// a blank List would lie about where the card is.
//
// Position is read from the record's own list only -- the cards sharing its group value, in the order a board
// draws them -- not from the whole Machine, so the cost is one bounded statement whatever else exists.
func (l *Loader) recordMove(ctx context.Context, m *domain.Machine, r *data.Record) (*rendering.RecordMove, error) {
	v, ok := m.BoardView()
	if !ok {
		return nil, nil
	}
	columns, err := l.BoardColumns(ctx, m, v)
	if err != nil {
		return nil, err
	}
	if columns == nil {
		columns = experience.GroupRecords(m, v, nil, nil)
	}
	targets := rendering.MoveTargets(m, v, columns)
	current := DisplayString(r.Values[v.GroupBy])
	if len(targets) == 0 || !slices.ContainsFunc(targets, func(t rendering.MoveTarget) bool { return t.Value == current }) {
		return nil, nil
	}
	siblings, err := l.ListRecordsBy(ctx, m.ID, v.GroupBy, current)
	if err != nil {
		return nil, err
	}
	position := 1
	for i, s := range siblings {
		if s.ID == r.ID {
			position = i + 1
		}
	}
	return &rendering.RecordMove{
		CardMove: rendering.CardMove{Field: v.GroupBy, Targets: targets, Current: current, Position: position},
		ViewID:   v.ID,
	}, nil
}

// checklistFor is how a child collection becomes a to-do list instead of a table: its Machine declares
// what "finished" means (`completion:`) and which Field is an item's text (a `title` card field), which
// is everything a checklist needs, so nothing new is declared and a child Machine that lacks either keeps
// its table. Nil for those.
func checklistFor(m *domain.Machine, parentField string, records []*data.Record) *rendering.Checklist {
	text := FieldForRole(m, domain.CardFieldRoleTitle)
	if m.Completion == nil || text == "" {
		return nil
	}
	c := &rendering.Checklist{ParentField: parentField, TextField: text}
	for _, r := range records {
		done := IsComplete(m, r)
		if done {
			c.Done++
		}
		c.Items = append(c.Items, rendering.ChecklistItem{ID: r.ID, Text: DisplayString(r.Values[text]), Complete: *cardComplete(m, r)})
	}
	return c
}
