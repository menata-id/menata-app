package composition

import (
	"context"

	"menata.app/internal/data"
	"menata.app/internal/domain"
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
