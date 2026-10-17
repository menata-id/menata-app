package composition

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/rendering"
)

// RuntimeProvider supplies the rows of one registry.RuntimeSources entry for one viewer in one request. It is the half of
// the seam that reads the database; registry holds the closed set and what each source promises, and
// TestRuntimeSourcesAndProvidersAgree binds the two.
type RuntimeProvider func(ctx context.Context, l *Loader, viewer domain.Actor) ([]*data.Record, error)

var runtimeProviders = map[string]RuntimeProvider{
	"groups": groupsProvider,
}

// RuntimeProviderIDs lists the sources a provider exists for, for the agreement test.
func RuntimeProviderIDs() []string {
	ids := make([]string, 0, len(runtimeProviders))
	for id := range runtimeProviders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// WithRuntime lets this Loader read the runtime's own Machines for this viewer: they are added to the Machines it knows
// (a copy, so the caller's map is untouched) and a Dataset over one is answered by its provider instead of the
// database. Only a runtime page's Loader is given them.
func (l *Loader) WithRuntime(machines []*domain.Machine, viewer domain.Actor) *Loader {
	merged := make(map[string]*domain.Machine, len(l.machines)+len(machines))
	for id, m := range l.machines {
		merged[id] = m
	}
	for _, m := range machines {
		merged[m.ID] = m
	}
	l.machines, l.runtimeViewer, l.hasRuntime = merged, viewer, true
	return l
}

// selectRuntime answers a `select: records` Dataset over a runtime Machine: the provider's rows, filtered by the
// Dataset's `where:`, ordered by its `sort:` and cut at its `limit:`, all in memory, with the same fail-closed reading of
// a sentinel the database path has.
func (l *Loader) selectRuntime(ctx context.Context, ds domain.Dataset, src *domain.Machine, where expression.Context) ([]*data.Record, bool, error) {
	provider, ok := runtimeProviders[src.Runtime]
	if !ok {
		return nil, false, fmt.Errorf("composition: runtime machine %s names source %q, which has no provider", src.ID, src.Runtime)
	}
	if !l.hasRuntime {
		return nil, false, fmt.Errorf("composition: dataset %s reads runtime machine %s, which only a runtime page's loader may", ds.ID, src.ID)
	}
	predicates, err := predicatesFor(ds, where)
	if err != nil {
		return nil, false, err
	}
	rows, err := provider(ctx, l, l.runtimeViewer)
	if err != nil {
		return nil, false, err
	}
	l.reads++
	var kept []*data.Record
	for _, r := range rows {
		if matchesPredicates(r.Values, predicates) {
			kept = append(kept, r)
		}
	}
	sortRuntimeRecords(kept, ds.Sort)
	truncated := false
	if ds.Limit > 0 && len(kept) > ds.Limit {
		kept, truncated = kept[:ds.Limit], true
	}
	return kept, truncated, nil
}

// matchesPredicates is data.FieldPredicate's meaning over a record already in memory: the conjunction of the plain
// predicates, ANDed with the disjunction of the Alternative ones, as the SQL path builds them.
func matchesPredicates(values map[string]any, predicates []data.FieldPredicate) bool {
	anyAlternative, alternativeHeld := false, false
	for _, p := range predicates {
		held := predicateHolds(values, p)
		if p.Alternative {
			anyAlternative = true
			alternativeHeld = alternativeHeld || held
		} else if !held {
			return false
		}
	}
	return !anyAlternative || alternativeHeld
}

func predicateHolds(values map[string]any, p data.FieldPredicate) bool {
	actual := DisplayString(values[p.Field])
	switch {
	case p.Empty:
		return actual == ""
	case p.Order != "":
		op := map[string]expression.Op{"<": expression.OpLessThan, "<=": expression.OpLessOrEqual, ">": expression.OpGreaterThan, ">=": expression.OpGreaterOrEqual}[p.Order]
		return expression.Ordered(op, actual, p.Value)
	case p.Negate:
		return actual != p.Value
	}
	return actual == p.Value
}

// sortRuntimeRecords orders by each key in turn; a number sorts as a number, anything else as text.
func sortRuntimeRecords(rows []*data.Record, keys []domain.SortKey) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, k := range keys {
			a, b := sortValue(rows[i], k.Field), sortValue(rows[j], k.Field)
			if a == b {
				continue
			}
			less := a < b
			if x, errx := strconv.ParseFloat(a, 64); errx == nil {
				if y, erry := strconv.ParseFloat(b, 64); erry == nil {
					less = x < y
				}
			}
			if k.Descending() {
				return !less
			}
			return less
		}
		return false
	})
}

func sortValue(r *data.Record, field string) string {
	switch field {
	case "created_at":
		return r.CreatedAt.Format("2006-01-02T15:04:05.000000000")
	case "updated_at":
		return r.UpdatedAt.Format("2006-01-02T15:04:05.000000000")
	}
	return strings.ToLower(DisplayString(r.Values[field]))
}

// groupsProvider is the `groups` source: every Group of the Workspace, with its member count and the role it grants in
// each Application, as one line ("Helpdesk · agent, Lending · clerk"). Applications are named from the Workspace
// on the request, so a grant to an Application since uninstalled is not shown.
func groupsProvider(ctx context.Context, l *Loader, _ domain.Actor) ([]*data.Record, error) {
	workspaceID, _ := data.WorkspaceScope(ctx)
	// Through the Loader's memo: a page asks several things of the Groups (this source, the roles a Form starts as, the
	// members' own Groups), and each listing re-reads every grant.
	groups, err := l.Groups(ctx)
	if err != nil {
		return nil, err
	}
	apps := rendering.CurrentWorkspace(ctx).Applications
	out := make([]*data.Record, 0, len(groups))
	for _, g := range groups {
		count := strconv.Itoa(g.MemberCount) + " members"
		if g.MemberCount == 1 {
			count = "1 member"
		}
		var grants []string
		for _, app := range apps {
			if role, ok := g.Grants[app.ID]; ok {
				grants = append(grants, app.Name+" · "+role)
			}
		}
		out = append(out, &data.Record{
			ID: g.ID, MachineID: "mch_group", WorkspaceID: workspaceID,
			Values: map[string]any{"fld_id": g.ID, "fld_name": g.Name, "fld_member_count": count, "fld_grants": grantsLine(grants)},
		})
	}
	return out, nil
}

func grantsLine(grants []string) string {
	if len(grants) == 0 {
		return "Grants no role yet"
	}
	return strings.Join(grants, ", ")
}

// RuntimeRecordExists reports whether the runtime source provides a record with this id for this viewer. A runtime
// screen that is one record's page asks it before drawing, so an id the source does not hold is a 404 and not an empty page.
func RuntimeRecordExists(ctx context.Context, l *Loader, viewer domain.Actor, source, id string) (bool, error) {
	provider, ok := runtimeProviders[source]
	if !ok {
		return false, fmt.Errorf("composition: runtime source %q has no provider", source)
	}
	rows, err := provider(ctx, l, viewer)
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.ID == id {
			return true, nil
		}
	}
	return false, nil
}
