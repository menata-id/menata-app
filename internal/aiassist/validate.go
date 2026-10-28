package aiassist

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// applicationIDPattern mirrors internal/metadata's own (unexported) applicationIDPattern exactly
// -- duplicated rather than exported from that package, since internal/metadata's own load-time
// validators for Application-level fields (role vocabulary, icon/color, summary_machine) are
// themselves unexported (loadApplicationFile is package-private). Machine-level validation does
// not have this problem: metadata.Validate is already exported and reused as-is below.
var applicationIDPattern = regexp.MustCompile(`^app_[a-z][a-z0-9_]*$`)

// ExistingState is what a GeneratedChange must be checked against beyond its own internal
// consistency: ids already in use workspace-wide (a generated Machine/Application id must be new),
// and, for an extend_application change, the real current shape of the Application being
// extended. Built by the caller (internal/web's own handler) from the live domain.Workspace the
// request is scoped to -- this package never reads metadata or the database itself.
type ExistingState struct {
	MachineIDs     map[string]bool
	ApplicationIDs map[string]bool
	// Applications holds, for each installed Application id, its own current Roles and the Fields
	// of each Machine it claims -- exactly what validating an extend_application addition needs
	// (does the target option/role already exist, does the target Field exist and is it a status
	// Field) without this package depending on domain.Workspace's own richer shape.
	Applications map[string]ExistingApplicationState
	// NavIDs are every navigation item id declared anywhere in this Workspace. A new one must not
	// reuse one (metadata.validateNavigationIDsAreUnique would refuse the load).
	NavIDs map[string]bool
}

// ExistingApplicationState is the slice of one installed Application's current metadata that
// extend_application validation reads.
type ExistingApplicationState struct {
	Name        string
	Description string
	Icon        string
	Color       string
	Roles       []string
	Machines    map[string]*domain.Machine // keyed by machine id, this Application's own claimed Machines only
	// MachineOrder is the Application's machines: list in declared order, which Machines cannot keep.
	MachineOrder []string
	// Navigation is this Application's own menu, in declared order (domain.Application.AllNavigation).
	Navigation []ExistingNavItem
}

// ExistingNavItem is one navigation item: its id, label and route.
type ExistingNavItem struct {
	ID    string
	Label string
	Route string
	// Parent, VisibleTo and Badge are the item's `parent:`, `visible_to:` and `badge:` as declared.
	Parent    string
	VisibleTo []string
	Badge     string
}

// ExistingStateFrom is the one way to build an ExistingState from a loaded Workspace, used by
// internal/web for validation and by Write for computing an update's plan against the files it is
// about to edit, so the two can never describe the Workspace differently.
func ExistingStateFrom(ws domain.Workspace) ExistingState {
	state := ExistingState{
		MachineIDs:     map[string]bool{},
		ApplicationIDs: map[string]bool{},
		Applications:   map[string]ExistingApplicationState{},
		NavIDs:         map[string]bool{},
	}
	for _, id := range ws.MachineIDs {
		state.MachineIDs[id] = true
	}
	for _, n := range ws.Navigation {
		state.NavIDs[n.ID] = true
	}
	byID := make(map[string]*domain.Machine, len(ws.Machines))
	for _, m := range ws.Machines {
		byID[m.ID] = m
	}
	for _, app := range ws.Applications {
		state.ApplicationIDs[app.ID] = true
		claimed := make(map[string]*domain.Machine, len(app.Machines))
		for _, mID := range app.Machines {
			if m, ok := byID[mID]; ok {
				claimed[mID] = m
			}
		}
		nav := make([]ExistingNavItem, 0, len(app.AllNavigation))
		for _, n := range app.AllNavigation {
			nav = append(nav, ExistingNavItem{ID: n.ID, Label: n.Label, Route: n.Route, Parent: n.Parent, VisibleTo: slices.Clone(n.VisibleTo), Badge: n.Badge})
			state.NavIDs[n.ID] = true
		}
		state.Applications[app.ID] = ExistingApplicationState{
			Name: app.Name, Description: app.Description, Icon: app.Icon, Color: app.Color,
			Roles: app.Roles, Machines: claimed, MachineOrder: app.Machines, Navigation: nav,
		}
	}
	return state
}

// Validate checks a GeneratedChange for internal consistency (real domain.Machine/
// domain.Application values built from it, run through metadata.Validate -- the same gate every
// hand-written *.yaml file passes) and for the one thing metadata.Validate cannot see on its own:
// collision with what this Workspace already has. A GeneratedChange that fails here is never
// offered on the review screen (internal/web's own handler re-asks the conversation instead) --
// this is the mechanical half of "confirm until metadata is complete" the kajian asked for.
func Validate(change GeneratedChange, existing ExistingState) error {
	switch change.Kind {
	case KindNewApplication:
		return validateNewApplication(change, existing)
	case KindUpdateApplication:
		return validateUpdateApplication(change, existing)
	default:
		return fmt.Errorf("unknown change kind %q", change.Kind)
	}
}

func validateNewApplication(change GeneratedChange, existing ExistingState) error {
	app := change.Application
	if app == nil {
		return fmt.Errorf("new_application change carries no application")
	}
	var issues []string

	if !applicationIDPattern.MatchString(app.ID) {
		issues = append(issues, fmt.Sprintf("application id %q must match %s", app.ID, applicationIDPattern.String()))
	}
	if existing.ApplicationIDs[app.ID] {
		issues = append(issues, fmt.Sprintf("application id %q is already installed in this workspace", app.ID))
	}
	if strings.TrimSpace(app.Name) == "" {
		issues = append(issues, "application name is required")
	}
	if app.Icon != "" && !domain.KnownIcons[app.Icon] {
		issues = append(issues, fmt.Sprintf("icon %q is not a known icon", app.Icon))
	}
	if app.Color != "" && !domain.KnownApplicationColors[app.Color] {
		issues = append(issues, fmt.Sprintf("color %q is not a known application color", app.Color))
	}
	seenRole := map[string]bool{}
	for _, r := range app.Roles {
		if strings.TrimSpace(r) == "" {
			issues = append(issues, "roles entry is empty")
			continue
		}
		if seenRole[r] {
			issues = append(issues, fmt.Sprintf("role %q is declared more than once", r))
		}
		seenRole[r] = true
	}
	// PublisherRole is what lets the person who just built this Application actually open it
	// (publishNewApplication grants it directly) -- required whenever there is a role vocabulary
	// to hold one from at all. An Application declaring no roles gates nothing
	// (requireApplicationAccess's own len(app.Roles) == 0 check), so nobody needs one there.
	if len(app.Roles) > 0 {
		if strings.TrimSpace(app.PublisherRole) == "" {
			issues = append(issues, "publisher_role is required when the application declares roles -- ask which one the person will hold themselves")
		} else if !seenRole[app.PublisherRole] {
			issues = append(issues, fmt.Sprintf("publisher_role %q must be one of roles %v", app.PublisherRole, app.Roles))
		}
	}
	if len(app.Machines) == 0 {
		issues = append(issues, "an application needs at least one machine")
	}
	issues = append(issues, validateMenu(app)...)

	// Machine ids must be new workspace-wide (Machines are workspace-level and unique by id, per
	// domain.Workspace's own doc comment) and unique within this change too.
	seenMachine := map[string]bool{}
	var built []*domain.Machine
	knownFieldTargets := relationTargets(existing) // a relation may point at a sibling, or at any Machine already here
	for _, m := range app.Machines {
		knownFieldTargets[m.ID] = true
	}
	for _, m := range app.Machines {
		// A Permission's roles must be words this Application actually declares. The real check is
		// internal/metadata.validatePermissionRoles, which is cross-Machine and unexported, so it
		// only runs at load -- Write's own load-verify is what makes a miss here safe rather than
		// destructive (it rolls the whole write back). This copy exists purely so the assistant
		// gets told mid-conversation, where it can still fix it, instead of after publishing.
		for _, p := range m.Permissions {
			for _, r := range p.Roles {
				if !seenRole[r] {
					issues = append(issues, fmt.Sprintf("machine %q permission %q names role %q, which this application does not declare in its roles %v -- nobody could hold it, so the permission would deny everyone while reading as a grant", m.ID, p.ID, r, app.Roles))
				}
			}
		}
		for _, tr := range m.Transitions {
			for _, r := range tr.Roles {
				if !seenRole[r] {
					issues = append(issues, fmt.Sprintf("machine %q step %q names role %q, which this application does not declare in its roles %v -- nobody could hold it, so the step could be taken by no one", m.ID, tr.ID, r, app.Roles))
				}
			}
		}
		issues = append(issues, frozenIssues(m, app.Machines)...)
		for _, ev := range m.Events {
			if ev.RecipientRole != "" && !seenRole[ev.RecipientRole] {
				issues = append(issues, fmt.Sprintf("machine %q event %q tells role %q, which this application does not declare in its roles %v -- nobody holds it, so no one would be told", m.ID, ev.ID, ev.RecipientRole, app.Roles))
			}
		}
		if existing.MachineIDs[m.ID] {
			issues = append(issues, fmt.Sprintf("machine id %q already exists in this workspace", m.ID))
		}
		if seenMachine[m.ID] {
			issues = append(issues, fmt.Sprintf("machine id %q is declared more than once in this application", m.ID))
		}
		seenMachine[m.ID] = true

		domainMachine, fieldIssues := buildDomainMachine(m, knownFieldTargets)
		fieldIssues = append(fieldIssues, notificationIssues(m, existing)...)
		issues = append(issues, fieldIssues...)
		if len(fieldIssues) > 0 {
			continue // metadata.Validate would only pile on the same field-shape complaints
		}
		if err := metadata.Validate(domainMachine); err != nil {
			issues = append(issues, fmt.Sprintf("machine %q: %v", m.ID, err))
		}
		built = append(built, domainMachine)
	}
	// An edge that names another machine (generates, balanced) is judged against every machine it can reach: the ones
	// just built and the ones already installed.
	if len(issues) == 0 {
		for _, a := range existing.Applications {
			for _, em := range a.Machines {
				built = append(built, em)
			}
		}
		if err := metadata.ValidateEdgeRules(built); err != nil {
			issues = append(issues, err.Error())
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// buildDomainMachine constructs a real domain.Machine from a GeneratedMachine, plus any issues
// metadata.Validate has no way to express (a relation field naming a machine that will not exist,
// since that's a Workspace-level constraint, not a per-Machine one). knownTargets names every
// machine id a relation may point at: the ones this change declares and the ones already installed.
func buildDomainMachine(gm GeneratedMachine, knownTargets map[string]bool) (*domain.Machine, []string) {
	var issues []string
	m := &domain.Machine{ID: gm.ID, Name: gm.Name}

	for _, gf := range gm.Fields {
		f := domain.Field{
			ID:       gf.ID,
			Name:     gf.Name,
			Type:     domain.FieldType(gf.Type),
			Required: gf.Required,
			Options:  gf.Options,
		}
		if f.Type == domain.FieldTypeRelation {
			f.RelatedMachine = gf.RelatedMachine
			if !knownTargets[gf.RelatedMachine] {
				issues = append(issues, fmt.Sprintf("machine %q field %q: relation target %q is neither a machine in this change nor one already in this workspace", gm.ID, gf.ID, gf.RelatedMachine))
			}
		}
		if f.Type == domain.FieldTypePerson {
			f.RelatedMachine = domain.UserMachineID
		}
		if gf.Compute != nil {
			f.Compute = &domain.FieldCompute{Op: domain.ComputeOp(gf.Compute.Op), Fields: gf.Compute.Fields}
		}
		if gf.Default != "" {
			// Judged exactly as the loader will judge the same text.
			def, err := metadata.CoerceDefault(f.Type, gf.Default)
			if err != nil {
				issues = append(issues, fmt.Sprintf("machine %q field %q: default %q: %v", gm.ID, gf.ID, gf.Default, err))
			} else {
				f.Default = def
			}
		}
		f.Stamp = domain.FieldStamp(gf.Stamp)
		f.EditableBy = slices.Clone(gf.EditableBy)
		if gf.ShowWhen != nil {
			f.ShowWhen = &domain.FieldCondition{Field: gf.ShowWhen.Field, Equals: gf.ShowWhen.Equals}
		}
		if gf.RequiredWhen != nil {
			f.RequiredWhen = &domain.FieldCondition{Field: gf.RequiredWhen.Field, Equals: gf.RequiredWhen.Equals}
		}
		if gf.DefaultFrom != nil {
			days := make(map[string]int, len(gf.DefaultFrom.Days))
			for _, d := range gf.DefaultFrom.Days {
				if _, dup := days[d.Value]; dup {
					issues = append(issues, fmt.Sprintf("machine %q field %q: default_from names %q more than once", gm.ID, gf.ID, d.Value))
				}
				days[d.Value] = d.Days
			}
			f.DefaultFrom = &domain.FieldDefaultFrom{Field: gf.DefaultFrom.Field, Days: days}
		}
		m.Fields = append(m.Fields, f)
	}
	for _, gp := range gm.Permissions {
		if gp.Action == domain.ActionDecide || gp.Action == domain.ActionRevise {
			issues = append(issues, fmt.Sprintf("machine %q permission %q: action %q is a hardcoded workflow action, not available to a generated machine", gm.ID, gp.ID, gp.Action))
			continue
		}
		perm := domain.Permission{ID: gp.ID, Action: gp.Action, Roles: gp.Roles, ActorField: gp.ActorField}
		if gp.ParentActor != nil {
			perm.ParentActor = &domain.ParentActorGate{ViaField: gp.ParentActor.Via, ActorField: gp.ParentActor.Field}
		}
		m.Permissions = append(m.Permissions, perm)
	}
	for _, cf := range gm.CardFields {
		m.CardFields = append(m.CardFields, domain.CardField{Field: cf.Field, Role: domain.CardFieldRole(cf.Role)})
	}
	if sc := gm.StatusCategories; sc != nil {
		d := &domain.StatusCategories{Field: sc.Field, Values: map[string]domain.StatusCategory{}}
		for _, v := range sc.Values {
			if _, dup := d.Values[v.Option]; dup {
				issues = append(issues, fmt.Sprintf("machine %q status_categories lists option %q more than once", gm.ID, v.Option))
			}
			d.Values[v.Option] = domain.StatusCategory(v.Category)
		}
		m.StatusCategories = d
	}
	for _, gv := range gm.Views {
		m.Views = append(m.Views, domain.View{ID: gv.ID, Name: gv.Name, Type: domain.ViewKind(gv.Type), GroupBy: gv.GroupBy})
	}
	issues = append(issues, viewIssues(gm)...)
	if gm.Completion != nil {
		m.Completion = &domain.Completion{Field: gm.Completion.Field, Done: gm.Completion.Done}
	}
	for _, gt := range gm.Transitions {
		seenSet := map[string]bool{}
		for _, st := range gt.Sets {
			if seenSet[st.Field] {
				issues = append(issues, fmt.Sprintf("machine %q step %q: sets names %q more than once", gm.ID, gt.ID, st.Field))
			}
			seenSet[st.Field] = true
		}
		m.Transitions = append(m.Transitions, domain.Transition{
			ID: gt.ID, Name: gt.Name, Field: gt.Field, From: gt.From, To: gt.To,
			Action: domain.ActionEdit,
			Roles:  slices.Clone(gt.Roles), ActorField: gt.ActorField,
			Sets:     setsOf(gt.Sets),
			Balanced: balancedRule(gt.Balanced), Generates: generateSchedule(gt.Generates),
		})
	}
	if gm.FrozenWhen != nil {
		m.Frozen = &domain.FrozenWhen{Via: gm.FrozenWhen.Via, Field: gm.FrozenWhen.Field, Equals: gm.FrozenWhen.Equals}
	}
	for _, e := range gm.Ensure {
		m.Bounds = append(m.Bounds, domain.AggregateBound{ID: e.ID, SumOf: e.SumOf, Per: e.Per, AtLeast: e.AtLeast, AtMost: e.AtMost, Reason: e.Reason})
	}
	for _, ge := range gm.Events {
		svc := domain.Service{Name: ge.EventService(), Summary: ge.Summary}
		switch ge.EventService() {
		case EventServiceLogActivity:
			if ge.RecipientField != "" || ge.RecipientRole != "" || ge.RecipientGroupField != "" || ge.PreferenceKey != "" {
				issues = append(issues, fmt.Sprintf("machine %q event %q: the recipient and preference_key belong to send_notification, not log_activity", gm.ID, ge.ID))
			}
		case EventServiceSendNotification:
			svc.Notify = &domain.Notify{RecipientField: ge.RecipientField, RecipientRole: ge.RecipientRole, RecipientGroupField: ge.RecipientGroupField, PreferenceKey: ge.PreferenceKey}
			// The loader only asks that the recipient be a field; a notification to anything but a person
			// reaches nobody and says nothing, so the assistant asks for more than the loader does.
			if f := fieldOf(m, ge.RecipientField); f != nil && f.Type != domain.FieldTypePerson {
				issues = append(issues, fmt.Sprintf("machine %q event %q: recipient_field %q must be a person field, got %q -- otherwise the notification reaches nobody", gm.ID, ge.ID, ge.RecipientField, f.Type))
			}
		default:
			issues = append(issues, fmt.Sprintf("machine %q event %q: service %q is not one a generated event may name (log_activity or send_notification)", gm.ID, ge.ID, ge.Service))
		}
		m.Events = append(m.Events, domain.Event{
			ID: ge.ID, On: ge.On, WhenEquals: ge.WhenEquals, OnCreate: ge.OnCreate, Then: svc,
		})
	}
	return m, issues
}

// cardRoleFieldTypes is the field type(s) each card role is generated over. The loader checks only the file,
// container and comment roles; a status shown as a date, or a person that is a number, would render and say
// nothing, so the assistant asks for more than the loader does.
var cardRoleFieldTypes = map[string][]domain.FieldType{
	"title":  {domain.FieldTypeText},
	"status": {domain.FieldTypeStatus},
	"date":   {domain.FieldTypeDate},
	"person": {domain.FieldTypePerson},
	"money":  {domain.FieldTypeMoney, domain.FieldTypeNumber},
}

// viewIssues holds a generated Machine's views and card_fields to what makes them show something: card roles
// over fitting fields, a board grouped by a status field, and a card projection (with a title) before any
// board or cards view, since a card over no projection renders empty cards.
func viewIssues(gm GeneratedMachine) []string {
	var issues []string
	types := map[string]domain.FieldType{}
	for _, f := range gm.Fields {
		types[f.ID] = domain.FieldType(f.Type)
	}
	hasTitle := false
	for _, cf := range gm.CardFields {
		ft, ok := types[cf.Field]
		want, known := cardRoleFieldTypes[cf.Role]
		if !ok || !known {
			continue // the loader names a missing field or an unknown role
		}
		if cf.Role == "title" {
			hasTitle = true
		}
		if !slices.Contains(want, ft) {
			issues = append(issues, fmt.Sprintf("machine %q card_fields: role %q needs a field of type %v, but %q is %q", gm.ID, cf.Role, want, cf.Field, ft))
		}
	}
	needsCards := false
	for _, v := range gm.Views {
		switch v.Type {
		case "board":
			needsCards = true
			if gt, ok := types[v.GroupBy]; ok && gt != domain.FieldTypeStatus {
				issues = append(issues, fmt.Sprintf("machine %q view %q: a board is grouped by a status field, but %q is %q", gm.ID, v.ID, v.GroupBy, gt))
			}
		case "cards":
			needsCards = true
			if v.GroupBy != "" {
				issues = append(issues, fmt.Sprintf("machine %q view %q: group_by belongs to a board, not a %s view", gm.ID, v.ID, v.Type))
			}
		case "table":
			if v.GroupBy != "" {
				issues = append(issues, fmt.Sprintf("machine %q view %q: group_by belongs to a board, not a table", gm.ID, v.ID))
			}
		default:
			issues = append(issues, fmt.Sprintf("machine %q view %q: type %q is not one a generated machine may use (board, cards or table)", gm.ID, v.ID, v.Type))
		}
	}
	if needsCards && !hasTitle {
		issues = append(issues, fmt.Sprintf("machine %q has a board or cards view, so it needs card_fields including a title -- cards over no projection render empty", gm.ID))
	}
	return issues
}

// notificationMachineID is the runtime-level Machine a send_notification writes to. A Workspace created
// through the UI holds it already; one without it cannot take a notifying Application, and this package
// cannot add it (it is a shared runtime Machine, not part of any generated Application).
const notificationMachineID = "mch_notification"

// notificationIssues refuses a notifying Machine in a Workspace that has nowhere for the notice to land.
func notificationIssues(gm GeneratedMachine, existing ExistingState) []string {
	if existing.MachineIDs[notificationMachineID] {
		return nil
	}
	for _, e := range gm.Events {
		if e.EventService() == EventServiceSendNotification {
			return []string{fmt.Sprintf("machine %q event %q sends a notification, but this workspace has no %s to hold it", gm.ID, e.ID, notificationMachineID)}
		}
	}
	return nil
}

// frozenIssues is the cross-Machine half of frozen_when for a target in the same change: the loader checks it only at
// load, after publishing, so the model is told here, while it can still fix it. A target already installed is left to
// the writer's load-verify.
func frozenIssues(gm GeneratedMachine, all []GeneratedMachine) []string {
	f := gm.FrozenWhen
	if f == nil {
		return nil
	}
	var via *GeneratedField
	for i := range gm.Fields {
		if gm.Fields[i].ID == f.Via {
			via = &gm.Fields[i]
		}
	}
	if via == nil || via.Type != "relation" {
		return nil // metadata.Validate reports it per Machine
	}
	for _, other := range all {
		if other.ID != via.RelatedMachine {
			continue
		}
		for _, of := range other.Fields {
			if of.ID != f.Field {
				continue
			}
			switch {
			case of.Type != "status":
				return []string{fmt.Sprintf("machine %q: frozen_when.field %q on %q must be a status field, got %q", gm.ID, f.Field, other.ID, of.Type)}
			case !contains(of.Options, f.Equals):
				return []string{fmt.Sprintf("machine %q: frozen_when.equals %q is not one of %q's options %v -- it would freeze nothing", gm.ID, f.Equals, f.Field, of.Options)}
			}
			return nil
		}
		return []string{fmt.Sprintf("machine %q: frozen_when.field %q is not a field of %q, which %q points at", gm.ID, f.Field, other.ID, f.Via)}
	}
	return nil
}

// setsOf turns a step's list of clock writes into domain.Transition.Sets. A field named twice keeps the last, which the
// loader cannot see, so the duplicate is the validator's to refuse (setsDuplicates).
func setsOf(list []GeneratedSet) map[string]string {
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]string, len(list))
	for _, s := range list {
		out[s.Field] = s.Source
	}
	return out
}

func balancedRule(b *GeneratedBalanced) *domain.BalanceRule {
	if b == nil {
		return nil
	}
	return &domain.BalanceRule{Machine: b.Of, Via: b.Via, Debit: b.Debit, Credit: b.Credit, Reason: b.Reason}
}

func generateSchedule(g *GeneratedGenerates) *domain.GenerateSchedule {
	if g == nil {
		return nil
	}
	return &domain.GenerateSchedule{Machine: g.Machine, CountField: g.CountField, LinkField: g.LinkField, DueField: g.DueField, DueEveryDays: g.DueEveryDays, AmountField: g.AmountField, AmountFrom: g.AmountFrom}
}

func fieldOf(m *domain.Machine, id string) *domain.Field {
	for i := range m.Fields {
		if m.Fields[i].ID == id {
			return &m.Fields[i]
		}
	}
	return nil
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// ValidationError aggregates every issue found, mirroring internal/metadata's own ValidationError
// shape (one list, not fail-fast) -- so a conversation turn can name everything wrong at once
// rather than the model having to guess-and-check one issue at a time.
type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%d issue(s): %s", len(e.Issues), strings.Join(e.Issues, "; "))
}

// validateMenu holds the person's answer to "which of these do you want in the menu": at least one
// entry, each naming a Machine this Application declares, none twice. A Machine left out is allowed
// -- that is the answer, not an omission -- and stays reachable by its own URL.
func validateMenu(app *GeneratedApplication) []string {
	if len(app.Navigation) == 0 {
		return []string{"navigation is required -- ask the person which of this application's machines they want in its menu, in what order, and what each entry should say"}
	}
	own := map[string]bool{}
	for _, m := range app.Machines {
		own[m.ID] = true
	}
	var issues []string
	seen := map[string]bool{}
	for i, item := range app.Navigation {
		if strings.TrimSpace(item.Label) == "" {
			issues = append(issues, fmt.Sprintf("navigation entry %d has no label", i+1))
		}
		if !own[item.MachineID] {
			issues = append(issues, fmt.Sprintf("navigation entry %q opens machine %q, which this application does not declare", item.Label, item.MachineID))
		}
		// A list is a page of its own, so a Machine may have several; only its plain entry is unique.
		if item.List == nil && item.ByStatus == nil && item.Queue == nil {
			if seen[item.MachineID] {
				issues = append(issues, fmt.Sprintf("machine %q has more than one navigation entry", item.MachineID))
			}
			seen[item.MachineID] = true
		}
	}
	return append(issues, validateLists(app)...)
}

// relationTargets is every Machine a generated relation may point at before counting the change's
// own: all of this Workspace's Machines except the runtime's activity and notification logs, which
// no business record refers to. mch_user stays, though a person Field is the usual way to name one.
func relationTargets(existing ExistingState) map[string]bool {
	out := make(map[string]bool, len(existing.MachineIDs))
	for id := range existing.MachineIDs {
		if id != "mch_activity" && id != "mch_notification" {
			out[id] = true
		}
	}
	return out
}
