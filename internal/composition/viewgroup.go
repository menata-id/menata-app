package composition

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/rendering"
)

// ViewGroupItem finds the navigation item of app that is one arrangement of a view group and is the screen the request is on:
// by id for a `page:` item, by the Machine's own route for the Board (a Board is a View of its Machine, not a page).
func ViewGroupItem(app domain.Application, machineID, navID string) (domain.NavigationItem, domain.ViewGroup, bool) {
	for _, item := range app.AllNavigation {
		if item.ViewGroup == "" {
			continue
		}
		if (navID != "" && item.ID == navID) || (navID == "" && item.Route == domain.MachineListRoute(machineID)) {
			g, ok := app.ViewGroupByID(item.ViewGroup)
			return item, g, ok
		}
	}
	return domain.NavigationItem{}, domain.ViewGroup{}, false
}

// ViewHeaderFor is the header a view group's screen draws (domain.ViewGroup) and the request parameters its page must be lowered
// with. The scope parameter is checked here, once, for every arrangement: a value that names no record of the scope Machine is
// dropped from the parameters (the screens show everything, as they do with none) rather than filtering to an empty page.
//
// Every number is counted, never typed: the totals from the group's aggregate Dataset (the whole set, scoped by the same
// parameter the screens carry), the scope's size from its Machine. One read of the scope's Dataset serves the menu, the title and the count.
func ViewHeaderFor(ctx context.Context, l *Loader, app domain.Application, item domain.NavigationItem, group domain.ViewGroup, params map[string]string, now time.Time) (*rendering.ViewHeader, map[string]string, error) {
	m := l.Machine(group.Machine)
	if m == nil {
		return nil, params, nil
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		out[k] = v
	}
	var scopeRecords []*data.Record
	var chosen *data.Record
	var scopeCount int
	var cut, pastCut bool
	scope := group.Scope
	var scopeMachine *domain.Machine
	if scope != nil {
		scopeMachine = l.Machine(scope.Machine)
		if scopeMachine == nil {
			return nil, params, nil
		}
		records, err := l.SelectDataset(ctx, scope.Dataset, expression.Context{Today: now.Format("2006-01-02")})
		if err != nil {
			return nil, params, err
		}
		scopeRecords, scopeCount = records, len(records)
		// A list cut at its Dataset's `limit:` is counted whole, and a choice past the cut is looked up on its own: the menu is a
		// window onto the records, not the set of them, so a project beyond it is neither uncounted nor "stale".
		if ds, ok := l.Dataset(scope.Dataset); ok && ds.Limit > 0 && len(records) >= ds.Limit {
			cut = true
			n, err := l.CountSelected(ctx, scope.Dataset, expression.Context{Today: now.Format("2006-01-02")})
			if err != nil {
				return nil, params, err
			}
			scopeCount = max(n, len(records))
		}
		if id := out[scope.Parameter]; id != "" {
			for _, r := range scopeRecords {
				if r.ID == id {
					chosen = r
				}
			}
			if chosen == nil && cut {
				if r, err := l.Record(ctx, scope.Machine, id); err == nil && r != nil {
					chosen, pastCut = r, true
				} else if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
					return nil, params, err
				}
			}
			if chosen == nil {
				delete(out, scope.Parameter)
			}
		}
	}
	agg, err := l.AggregateDataset(ctx, group.Figures.Dataset, expression.Context{Parameters: out, Today: now.Format("2006-01-02")})
	if err != nil {
		return nil, params, err
	}
	total, complete := int(agg.Total[group.Figures.Total]), int(agg.Total[group.Figures.Complete])

	h := &rendering.ViewHeader{Title: item.Heading}
	if h.Title == "" {
		h.Title = item.Label
	}
	var parts []string
	if item.SummaryCounts {
		words := group.Words.TotalOther
		if total == 1 {
			words = group.Words.TotalOne
		}
		parts = append(parts, domain.FillCount(words, total))
		if board, ok := m.BoardView(); ok && item.Route == domain.MachineListRoute(m.ID) {
			if f, ok := m.FieldByID(board.GroupBy); ok {
				parts = append(parts, strings.ReplaceAll(group.Words.Grouped, domain.ViewFieldToken, f.Name))
			}
		}
		parts = append(parts, domain.FillCount(group.Words.Complete, complete))
	} else if item.Description != "" {
		parts = append(parts, item.Description)
	}
	// A Workspace with no project has nothing to switch between and no sentence to say about them.
	if scope != nil && len(scopeRecords) > 0 {
		words := scope.Overall
		if chosen != nil {
			words = scope.Chosen
			h.Title = DisplayString(chosen.Values[scopeMachine.LabelField()])
		}
		parts = append(parts, domain.FillCount(words, scopeCount))
	}
	h.Summary = strings.Join(parts, " · ")

	for _, it := range app.ViewGroupItems(group.ID) {
		h.Tabs = append(h.Tabs, rendering.SegmentTab{Label: it.Label, Href: carryScope(group, out, it.Route), Current: it.ID == item.ID})
	}
	if scope != nil && len(scopeRecords) > 0 {
		menu := &rendering.ViewScopeMenu{Label: scope.Switch, Items: []rendering.ViewScopeItem{{Label: scope.All, Href: item.Route, Current: chosen == nil}}}
		label := scopeMachine.LabelField()
		for _, r := range scopeRecords {
			menu.Items = append(menu.Items, rendering.ViewScopeItem{Label: DisplayString(r.Values[label]), Href: item.Route + "?" + url.Values{scope.Parameter: {r.ID}}.Encode(), Current: chosen != nil && chosen.ID == r.ID})
		}
		if cut && scope.More != "" {
			menu.More = domain.FillCount(scope.More, len(scopeRecords))
		}
		if pastCut {
			menu.Items = append(menu.Items, rendering.ViewScopeItem{Label: DisplayString(chosen.Values[label]), Href: item.Route + "?" + url.Values{scope.Parameter: {chosen.ID}}.Encode(), Current: true})
		}
		h.Scope = menu
	}
	return h, out, nil
}

// carryScope is href with the request's scope choice added, for a link between two screens of the group: the choice is the
// viewer's, so it travels with them. A request with no choice, or a group with no scope, leaves the address as it was.
func carryScope(group domain.ViewGroup, params map[string]string, href string) string {
	if group.Scope == nil || params[group.Scope.Parameter] == "" {
		return href
	}
	base, query, _ := strings.Cut(href, "?")
	q, _ := url.ParseQuery(query)
	if q.Get(group.Scope.Parameter) != "" {
		return href
	}
	q.Set(group.Scope.Parameter, params[group.Scope.Parameter])
	return base + "?" + q.Encode()
}

// ViewCarry is what a `page:` of a view group asks of its links (ir.Resolver.Carry): a link to another screen of the same group
// keeps the scope choice. A link anywhere else (a record, another Application) is left alone.
func ViewCarry(app domain.Application, group domain.ViewGroup, params map[string]string) func(navID, href string) string {
	if group.Scope == nil {
		return nil
	}
	return func(navID, href string) string {
		for _, it := range app.ViewGroupItems(group.ID) {
			if it.ID == navID {
				return carryScope(group, params, href)
			}
		}
		return href
	}
}

// ScopeRecords narrows a group Machine's records to the scope choice -- the Board's half of the declaration the pages carry in
// their Datasets' where:. No choice, no scope, or a value that is not a record id leaves them all.
func ScopeRecords(group domain.ViewGroup, params map[string]string, records []*data.Record) []*data.Record {
	if group.Scope == nil || params[group.Scope.Parameter] == "" {
		return records
	}
	want := params[group.Scope.Parameter]
	out := make([]*data.Record, 0, len(records))
	for _, r := range records {
		if DisplayString(r.Values[group.Scope.Field]) == want {
			out = append(out, r)
		}
	}
	return out
}
