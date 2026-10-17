package composition

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/registry"
	"menata.app/internal/rendering"
)

// RuntimeProvider supplies the rows of one registry.RuntimeSources entry for one viewer in one request. It is the half of
// the seam that reads the database; registry holds the closed set and what each source promises, and
// TestRuntimeSourcesAndProvidersAgree binds the two.
type RuntimeProvider func(ctx context.Context, l *Loader, viewer domain.Actor) ([]*data.Record, error)

var runtimeProviders = map[string]RuntimeProvider{
	"groups":                  groupsProvider,
	"members":                 membersProvider,
	"invites":                 invitesProvider,
	"settings_applications":   settingsApplicationsProvider,
	"application_members":     applicationMembersProvider,
	"application_suggestions": suggestionsProvider,
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

// WithRuntimeProvider supplies the reader of a source whose registry entry says `Provider: "web"`: a handler passes a
// closure over what only it holds. A source not so marked cannot be given a reader here, so a handler cannot replace
// what composition provides.
func (l *Loader) WithRuntimeProvider(source string, p RuntimeProvider) *Loader {
	if registry.RuntimeSources[source].Provider != "web" {
		return l
	}
	if l.extraProviders == nil {
		l.extraProviders = map[string]RuntimeProvider{}
	}
	l.extraProviders[source] = p
	return l
}

// provider is the reader of a source: composition's own, or the handler's for a "web" source.
func (l *Loader) provider(source string) (RuntimeProvider, bool) {
	if p, ok := runtimeProviders[source]; ok {
		return p, true
	}
	p, ok := l.extraProviders[source]
	return p, ok
}

// WithParams hands the request's query parameters to the runtime sources, which may read only the ones the registry says
// they understand (registry.RuntimeSource.Params): a provider asks for one by name through sourceParam, never for the
// request.
func (l *Loader) WithParams(params map[string]string) *Loader {
	l.runtimeParams = params
	return l
}

// sourceParam is the request's value of a parameter a source declares (registry.RuntimeSource.Params), "" for any other:
// a provider cannot read what its source did not declare.
func (l *Loader) sourceParam(source, name string) string {
	for _, p := range registry.RuntimeSources[source].Params {
		if p == name {
			return l.runtimeParams[name]
		}
	}
	return ""
}

// selectRuntime answers a `select: records` Dataset over a runtime Machine: the provider's rows, filtered by the
// Dataset's `where:`, ordered by its `sort:` and cut at its `limit:`, all in memory, with the same fail-closed reading of
// a sentinel the database path has.
func (l *Loader) selectRuntime(ctx context.Context, ds domain.Dataset, src *domain.Machine, where expression.Context) ([]*data.Record, bool, error) {
	provider, ok := l.provider(src.Runtime)
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
	provider, ok := l.provider(source)
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

// membersProvider is the `members` source: every member of the Workspace, titled by their name (their email when none is
// known), with what a list shows of them in one line: the Workspace role, the role held in each Application that declares
// roles, and the Groups they belong to. `q` narrows by name or email.
func membersProvider(ctx context.Context, l *Loader, _ domain.Actor) ([]*data.Record, error) {
	members, err := l.Members(ctx)
	if err != nil {
		return nil, err
	}
	names, err := l.PersonNames(ctx)
	if err != nil {
		return nil, err
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	apps := rendering.CurrentWorkspace(ctx).Applications
	out := make([]*data.Record, 0, len(members))
	for _, m := range FilterMembers(members, names, l.sourceParam("members", "q")) {
		title, email := names[m.UserRecordID], m.Email
		if strings.TrimSpace(title) == "" || title == email {
			title, email = m.Email, ""
		}
		status := "Active"
		if m.Deactivated {
			status = "Deactivated"
		}
		parts := append([]string{"Workspace " + m.WorkspaceRole}, heldRoles(apps, m.AppRoles)...)
		if len(m.Groups) > 0 {
			var gs []string
			for _, g := range m.Groups {
				gs = append(gs, g.Name)
			}
			parts = append(parts, "Groups: "+strings.Join(gs, ", "))
		}
		out = append(out, &data.Record{ID: m.UserRecordID, MachineID: "mch_member", WorkspaceID: workspaceID,
			Values: map[string]any{"fld_id": m.UserRecordID, "fld_title": title, "fld_email": email, "fld_status": status, "fld_detail": strings.Join(parts, " · "), "fld_effective": effectiveAccessLine(apps, m)}})
	}
	return out, nil
}

// invitesProvider is the `invites` source: the invitations nobody has accepted, titled by the invited email, with the role
// each will become. An invitation holds no membership and no name (only its owner ever states one, on accepting), which is
// why it is a source of its own and not a filtered view of the members.
func invitesProvider(ctx context.Context, l *Loader, _ domain.Actor) ([]*data.Record, error) {
	workspaceID, _ := data.WorkspaceScope(ctx)
	pending, err := l.store.ListPendingInvites(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	l.reads++
	apps := rendering.CurrentWorkspace(ctx).Applications
	out := make([]*data.Record, 0, len(pending))
	for _, inv := range FilterPendingInvites(pending, l.sourceParam("invites", "q")) {
		parts := append([]string{"Workspace " + inv.WorkspaceRole}, heldRoles(apps, inv.AppRoles)...)
		out = append(out, &data.Record{ID: inv.Email, MachineID: "mch_invite", WorkspaceID: workspaceID,
			Values: map[string]any{"fld_id": inv.Email, "fld_title": inv.Email, "fld_status": "Invited", "fld_detail": strings.Join(parts, " · ")}})
	}
	return out, nil
}

// heldRoles says the role held in each Application that declares roles, leaving out the ones with none: a line that
// lists seven "—" says less than a line that lists the one role.
func heldRoles(apps []domain.Application, roles map[string]string) []string {
	var out []string
	for _, app := range apps {
		if role := roles[app.ID]; role != "" && len(app.Roles) > 0 {
			out = append(out, app.Name+": "+role)
		}
	}
	if len(out) == 0 {
		return []string{"no application role"}
	}
	return out
}

// effectiveAccessLine says what a member can actually do once their Groups are counted: the direct roles merged with every
// role their Groups hold (data.EffectiveRoles), one phrase per role, and the Groups beside them. Direct and Group access
// grant identically, so the line shows the merged set and does not pick a winner.
func effectiveAccessLine(apps []domain.Application, m data.Membership) string {
	effective := data.EffectiveRoles(m.AppRoles, m.Groups)
	var parts []string
	for _, app := range apps {
		for _, role := range effective[app.ID] {
			parts = append(parts, app.Name+" · "+role)
		}
	}
	for _, g := range m.Groups {
		parts = append(parts, "+ "+g.Name+" group")
	}
	if len(parts) == 0 {
		return "No application access."
	}
	return strings.Join(parts, ", ")
}

// SettingsApplication finds the Application of this Workspace whose settings hub has the navigation id `hubID`, with the hub
// item, for an Application that declares roles. The hub id is how a members screen names the one Application it is about.
func SettingsApplication(ws domain.Workspace, hubID string) (domain.Application, domain.NavigationItem, bool) {
	for _, app := range ws.Applications {
		if len(app.Roles) == 0 {
			continue
		}
		if hub, ok := SettingsHubOf(app, hubID); ok {
			return app, hub, true
		}
	}
	return domain.Application{}, domain.NavigationItem{}, false
}

// settingsApplicationsProvider is the `settings_applications` source: one record for each Application that declares roles and a
// settings hub, by the hub's navigation id. The record carries the words the Application's members screen says about it: what
// the screen is for, how many of the active members have access (directly or through a Group -- a deactivated member holds a
// role but cannot act on it, and counting them would flatter the number), and the way back to the hub.
func settingsApplicationsProvider(ctx context.Context, l *Loader, _ domain.Actor) ([]*data.Record, error) {
	members, err := l.Members(ctx)
	if err != nil {
		return nil, err
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	var out []*data.Record
	for _, app := range rendering.CurrentWorkspace(ctx).Applications {
		if len(app.Roles) == 0 {
			continue
		}
		for _, item := range app.AllNavigation {
			if !item.SettingsHub {
				continue
			}
			active, covered := 0, 0
			for _, m := range members {
				if m.Deactivated {
					continue
				}
				active++
				if len(data.EffectiveRoles(m.AppRoles, m.Groups)[app.ID]) > 0 {
					covered++
				}
			}
			noun := "members"
			if active == 1 {
				noun = "member"
			}
			out = append(out, &data.Record{ID: item.ID, MachineID: "mch_settings_application", WorkspaceID: workspaceID, Values: map[string]any{
				"fld_id": item.ID, "fld_title": app.Name,
				"fld_intro":    "Who can use " + app.Name + ", and with which role. A role here means nothing in other applications.",
				"fld_coverage": fmt.Sprintf("%d of %d active %s have access to %s.", covered, active, noun, app.Name),
				"fld_back":     "← " + item.Heading, "fld_link": item.Route,
			}})
		}
	}
	return out, nil
}

// applicationMembersProvider is the `application_members` source: every member of the Workspace once for each Application that
// declares roles and a hub, keyed `<hub id>:<user record id>`. The direct role is what the page's select edits; the roles a Group
// grants are shown beside it and not editable here (a Group's role is changed on the Group), so a person who holds "approver"
// both ways is shown it once, as their direct role.
func applicationMembersProvider(ctx context.Context, l *Loader, _ domain.Actor) ([]*data.Record, error) {
	members, err := l.Members(ctx)
	if err != nil {
		return nil, err
	}
	names, err := l.PersonNames(ctx)
	if err != nil {
		return nil, err
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	var out []*data.Record
	for _, app := range rendering.CurrentWorkspace(ctx).Applications {
		if len(app.Roles) == 0 {
			continue
		}
		for _, item := range app.AllNavigation {
			if !item.SettingsHub {
				continue
			}
			for _, m := range members {
				direct := m.AppRoles[app.ID]
				var notes []string
				if m.Deactivated {
					notes = append(notes, "Deactivated")
				}
				for _, role := range data.EffectiveRoles(m.AppRoles, m.Groups)[app.ID] {
					if role != direct {
						notes = append(notes, "+ "+role+" through a Group")
					}
				}
				title, email := names[m.UserRecordID], m.Email
				if strings.TrimSpace(title) == "" || title == email {
					title, email = m.Email, ""
				}
				out = append(out, &data.Record{ID: item.ID + ":" + m.UserRecordID, MachineID: "mch_application_member", WorkspaceID: workspaceID, Values: map[string]any{
					"fld_id": item.ID + ":" + m.UserRecordID, "fld_hub": item.ID, "fld_title": title, "fld_email": email,
					"fld_detail": strings.Join(notes, " · "), "fld_role": direct,
				}})
			}
		}
	}
	return out, nil
}

// sourceRecordRoles is the roles a runtime source's record declares (a library template's `roles:`), read from the source's own
// reader so the Form that offers them and the list that shows the record agree about what it is.
func sourceRecordRoles(ctx context.Context, l *Loader, viewer domain.Actor, source, recordID string) ([]string, error) {
	provider, ok := l.provider(source)
	if !ok {
		return nil, fmt.Errorf("composition: source %q has no provider", source)
	}
	rows, err := provider(ctx, l, viewer)
	if err != nil {
		return nil, err
	}
	var roles []string
	for _, r := range rows {
		if r.ID != recordID {
			continue
		}
		for _, role := range strings.Split(DisplayString(r.Values["fld_roles"]), "\n") {
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	return roles, nil
}

// suggestionsProvider is the `application_suggestions` source: the Workspace's own example prompts for the assistant, in the order
// its manifest lists them, each opening a conversation that starts from that prompt. The Workspace declares them, not the runtime
// (owner instruction, 2026-09-27): a Workspace whose business the mockup's three examples do not fit declares its own, or none.
func suggestionsProvider(ctx context.Context, _ *Loader, _ domain.Actor) ([]*data.Record, error) {
	workspaceID, _ := data.WorkspaceScope(ctx)
	var out []*data.Record
	for i, s := range rendering.CurrentWorkspace(ctx).SuggestedApplications {
		id := "suggestion_" + strconv.Itoa(i)
		out = append(out, &data.Record{ID: id, MachineID: "mch_application_suggestion", WorkspaceID: workspaceID, Values: map[string]any{
			"fld_id": id, "fld_title": s.Label, "fld_link": domain.RuntimeScreenRoute("nav_new_application") + "?idea=" + url.QueryEscape(s.Prompt),
		}})
	}
	return out, nil
}
