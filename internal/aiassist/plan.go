package aiassist

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// DescribeApplication is an installed Application as the model sees it: the same GeneratedApplication
// shape it writes a new one in, so updating is "here it is, return it as it should be".
//
// Only what that shape can express is described. A Permission gated on an actor Field or a Group,
// a Transition performed by a workflow Action, an Event running any Service but log_activity or on a
// schedule, and every block the shape has no key for (datasets, views, constraints, the workflow
// binding) are left out -- not hidden from review, but outside what an update can touch. PlanUpdate
// compares within this projection, and the writer edits only the keys it names, so everything
// outside it is preserved by construction rather than by the model remembering to repeat it.
func DescribeApplication(id string, a ExistingApplicationState) GeneratedApplication {
	g := GeneratedApplication{
		ID: id, Name: a.Name, Description: a.Description, Icon: a.Icon, Color: a.Color,
		Roles: slices.Clone(a.Roles),
	}
	for _, mID := range a.MachineOrder {
		if m := a.Machines[mID]; m != nil {
			g.Machines = append(g.Machines, describeMachine(m))
		}
	}
	for _, n := range a.Navigation {
		item := GeneratedMenuItem{ID: n.ID, Label: n.Label, MachineID: machineOfRoute(n.Route), Parent: n.Parent, VisibleTo: slices.Clone(n.VisibleTo), Badge: n.Badge}
		// A list the assistant wrote is read back as the list, so it can be edited; any other screen stays a screen the model cannot name.
		if machineID, list := listFromExisting(n.ID, n.Page, a.Machines); list != nil {
			item.MachineID, item.List = machineID, list
		}
		g.Navigation = append(g.Navigation, item)
	}
	return g
}

// machineOfRoute is the Machine a /machines/{id} route opens, or "" for any other screen.
func machineOfRoute(route string) string {
	id, ok := strings.CutPrefix(route, "/machines/")
	if !ok || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func describeMachine(m *domain.Machine) GeneratedMachine {
	g := GeneratedMachine{ID: m.ID, Name: m.Name}
	for _, f := range m.Fields {
		gf := GeneratedField{ID: f.ID, Name: f.Name, Type: string(f.Type), Required: f.Required, Options: slices.Clone(f.Options)}
		if f.Type == domain.FieldTypeRelation {
			gf.RelatedMachine = f.RelatedMachine
		}
		if f.Compute != nil {
			gf.Compute = &GeneratedCompute{Op: string(f.Compute.Op), Fields: slices.Clone(f.Compute.Fields)}
		}
		if f.Default != nil {
			gf.Default = fmt.Sprint(f.Default)
		}
		gf.Stamp = string(f.Stamp)
		if f.RequiredWhen != nil {
			gf.RequiredWhen = &GeneratedRequiredWhen{Field: f.RequiredWhen.Field, Equals: f.RequiredWhen.Equals}
		}
		gf.EditableBy = slices.Clone(f.EditableBy)
		if f.ShowWhen != nil {
			gf.ShowWhen = &GeneratedShowWhen{Field: f.ShowWhen.Field, Equals: f.ShowWhen.Equals}
		}
		if f.DefaultFrom != nil {
			df := &GeneratedDefaultFrom{Field: f.DefaultFrom.Field}
			for value, days := range f.DefaultFrom.Days {
				df.Days = append(df.Days, GeneratedDayCount{Value: value, Days: days})
			}
			sort.Slice(df.Days, func(i, j int) bool { return df.Days[i].Value < df.Days[j].Value })
			gf.DefaultFrom = df
		}
		g.Fields = append(g.Fields, gf)
	}
	if m.Completion != nil {
		g.Completion = &GeneratedCompletion{Field: m.Completion.Field, Done: m.Completion.Done}
	}
	if sc := m.StatusCategories; sc != nil {
		gs := &GeneratedStatusCategories{Field: sc.Field}
		for option, c := range sc.Values {
			gs.Values = append(gs.Values, GeneratedStatusCategory{Option: option, Category: string(c)})
		}
		sort.Slice(gs.Values, func(i, j int) bool { return gs.Values[i].Option < gs.Values[j].Option })
		g.StatusCategories = gs
	}
	for _, cf := range m.CardFields {
		if slices.Contains(GeneratableCardRoles, string(cf.Role)) {
			g.CardFields = append(g.CardFields, GeneratedCardField{Field: cf.Field, Role: string(cf.Role)})
		}
	}
	for _, v := range m.Views {
		if slices.Contains(GeneratableViewTypes, string(v.EffectiveType())) {
			g.Views = append(g.Views, GeneratedView{ID: v.ID, Name: v.Name, Type: string(v.EffectiveType()), GroupBy: v.GroupBy})
		}
	}
	for _, p := range m.Permissions {
		if expressiblePermission(p) {
			gp := GeneratedPermission{ID: p.ID, Action: p.Action, Roles: slices.Clone(p.Roles), ActorField: p.ActorField}
			if p.ParentActor != nil {
				gp.ParentActor = &GeneratedParentActor{Via: p.ParentActor.ViaField, Field: p.ParentActor.ActorField}
			}
			g.Permissions = append(g.Permissions, gp)
		}
	}
	for _, t := range m.Transitions {
		if t.Action == domain.ActionEdit {
			g.Transitions = append(g.Transitions, GeneratedTransition{ID: t.ID, Name: t.Name, Field: t.Field, From: t.From, To: t.To, Roles: slices.Clone(t.Roles), ActorField: t.ActorField, Sets: setsList(t.Sets), Balanced: balancedSpec(t.Balanced), Generates: generatesSpec(t.Generates)})
		}
	}
	if m.Frozen != nil {
		g.FrozenWhen = &GeneratedFrozenWhen{Via: m.Frozen.Via, Field: m.Frozen.Field, Equals: m.Frozen.Equals}
	}
	for _, b := range m.Bounds {
		g.Ensure = append(g.Ensure, GeneratedEnsure{ID: b.ID, SumOf: b.SumOf, Per: b.Per, AtLeast: cloneFloat(b.AtLeast), AtMost: cloneFloat(b.AtMost), Reason: b.Reason})
	}
	for _, e := range m.Events {
		if e.Schedule != nil || e.Then.SummaryOverrideWhen != "" {
			continue
		}
		switch e.Then.Name {
		case domain.ServiceLogActivity:
			g.Events = append(g.Events, GeneratedEvent{ID: e.ID, On: e.On, WhenEquals: e.WhenEquals, OnCreate: e.OnCreate, Summary: e.Then.Summary, Disabled: e.Disabled})
		case domain.ServiceSendNotification:
			if e.Then.Notify != nil {
				g.Events = append(g.Events, GeneratedEvent{ID: e.ID, On: e.On, WhenEquals: e.WhenEquals, OnCreate: e.OnCreate, Service: EventServiceSendNotification,
					Summary: e.Then.Summary, RecipientField: e.Then.Notify.RecipientField, RecipientRole: e.Then.Notify.RecipientRole,
					RecipientGroupField: e.Then.Notify.RecipientGroupField, PreferenceKey: e.Then.Notify.PreferenceKey, Disabled: e.Disabled})
			}
		}
	}
	return g
}

func expressiblePermission(p domain.Permission) bool {
	switch p.Action {
	case domain.ActionCreate, domain.ActionEdit, domain.ActionDelete:
		return p.DynamicActor == nil
	}
	return false
}

// Plan is what an update will do, in the order a person reads it, plus what it will not do.
type Plan struct {
	Items    []PlanItem
	Refusals []string
}

// PlanItem is one line of a plan: an added, changed or removed thing, in words.
type PlanItem struct {
	Op   string // "add", "change", "remove"
	What string
	// Confirm is non-empty on a removal the owner must confirm before it is written, and is the key
	// GeneratedChange.ConfirmedRemovals must then carry. A removal that needs none leaves it empty.
	Confirm string
}

// RemovalKey names one field removal for confirmation.
func RemovalKey(machineID, fieldID string) string { return "field:" + machineID + "/" + fieldID }

// Unconfirmed is the confirmation keys this plan asks for that the change does not carry.
func (p Plan) Unconfirmed(confirmed []string) []string {
	var out []string
	for _, it := range p.Items {
		if it.Confirm != "" && !slices.Contains(confirmed, it.Confirm) {
			out = append(out, it.Confirm)
		}
	}
	return out
}

func (p *Plan) add(what string, args ...any) {
	p.Items = append(p.Items, PlanItem{Op: "add", What: fmt.Sprintf(what, args...)})
}
func (p *Plan) change(what string, args ...any) {
	p.Items = append(p.Items, PlanItem{Op: "change", What: fmt.Sprintf(what, args...)})
}
func (p *Plan) remove(what string, args ...any) {
	p.Items = append(p.Items, PlanItem{Op: "remove", What: fmt.Sprintf(what, args...)})
}
func (p *Plan) refuse(what string, args ...any) {
	p.Refusals = append(p.Refusals, fmt.Sprintf(what, args...))
}

// PlanUpdate compares an installed Application (DescribeApplication) with the state the model wants
// it in, and is the whole of the update policy:
//
//   - anything new may be added: a role, a Machine, a Field, an option, a Permission, a Transition,
//     an Event, a menu item;
//   - display text may change (names, labels, the description, icon and color, the menu order), and
//     so may which roles a Permission names and whether a Field is required or computed;
//   - a menu item opening a Machine may be removed -- the Machine and its records stay;
//   - a Field may be removed only with the owner's confirmation (PlanItem.Confirm): the declaration
//     goes, the stored values stay. Everything else listed next may not;
//   - nothing that holds records or grants access may be removed or retyped: a Machine, a declared option, a role, a Permission (removing one opens that action to everyone), a
//     Transition, an Event, a menu item opening a screen the model cannot name again, or a Field's
//     type or relation target. Each is refused with its reason, and a plan with a refusal does not
//     validate, so it goes back to the model rather than to review.
//
// Ids are what the comparison keys on, which is why an update may rename everything and still be
// read correctly: CLAUDE.md's "a name is never an identity", applied to a diff.
func PlanUpdate(current, desired GeneratedApplication) Plan {
	var p Plan
	if desired.ID != current.ID {
		p.refuse("the application id must stay %q (got %q)", current.ID, desired.ID)
	}
	if desired.Name != current.Name {
		p.change("rename the application from %q to %q", current.Name, desired.Name)
	}
	if desired.Description != current.Description {
		p.change("description: %q", desired.Description)
	}
	if desired.Icon != current.Icon {
		p.change("icon: %s", desired.Icon)
	}
	if desired.Color != current.Color {
		p.change("color: %s", desired.Color)
	}
	for _, r := range desired.Roles {
		if !slices.Contains(current.Roles, r) {
			p.add("role %q", r)
		}
	}
	for _, r := range current.Roles {
		if !slices.Contains(desired.Roles, r) {
			p.refuse("role %q cannot be removed: everyone holding it would lose access", r)
		}
	}

	currentMachines := map[string]GeneratedMachine{}
	for _, m := range current.Machines {
		currentMachines[m.ID] = m
	}
	desiredMachines := map[string]bool{}
	for _, m := range desired.Machines {
		desiredMachines[m.ID] = true
		old, exists := currentMachines[m.ID]
		if !exists {
			p.add("machine %q (%s)", m.Name, fieldNames(m))
			continue
		}
		planMachine(&p, old, m)
	}
	for _, m := range current.Machines {
		if !desiredMachines[m.ID] {
			p.refuse("machine %s (%q) cannot be removed: its records would be lost", m.ID, m.Name)
		}
	}

	planMenu(&p, current.Navigation, desired.Navigation)
	return p
}

func fieldNames(m GeneratedMachine) string {
	names := make([]string, 0, len(m.Fields))
	for _, f := range m.Fields {
		names = append(names, f.Name)
	}
	return strings.Join(names, ", ")
}

func planMachine(p *Plan, old, m GeneratedMachine) {
	if m.Name != old.Name {
		p.change("rename machine %s from %q to %q", m.ID, old.Name, m.Name)
	}
	oldFields := map[string]GeneratedField{}
	for _, f := range old.Fields {
		oldFields[f.ID] = f
	}
	seen := map[string]bool{}
	for _, f := range m.Fields {
		seen[f.ID] = true
		of, exists := oldFields[f.ID]
		if !exists {
			p.add("field %q on %q", f.Name, m.Name)
			continue
		}
		planField(p, m, of, f)
	}
	for _, f := range old.Fields {
		if !seen[f.ID] {
			p.Items = append(p.Items, PlanItem{Op: "remove", Confirm: RemovalKey(m.ID, f.ID),
				What: fmt.Sprintf("field %q on %q -- it disappears from every screen; values already stored stay in the records but can no longer be seen or edited", f.Name, m.Name)})
		}
	}

	switch {
	case m.Completion != nil && old.Completion == nil:
		p.add("completion on %q: %q in %s means done", m.Name, m.Completion.Done, m.Completion.Field)
	case !sameCompletion(old.Completion, m.Completion):
		p.refuse("completion on %s cannot be changed or removed: records already finished by it would change meaning", m.ID)
	}

	switch {
	case m.StatusCategories != nil && old.StatusCategories == nil:
		p.add("status colours on %q: each option of %s is given a category", m.Name, m.StatusCategories.Field)
	case m.StatusCategories != nil && old.StatusCategories != nil && categoriesOnlyGrowForNewOptions(old, m):
		p.add("status colours on %q: the new options of %s are given a category", m.Name, m.StatusCategories.Field)
	case !sameStatusCategories(old.StatusCategories, m.StatusCategories):
		p.refuse("status_categories on %s cannot be changed or removed once declared: a status the person has learned to read would change colour", m.ID)
	}

	planCards(p, old, m)
	planViews(p, old, m)

	oldPerms := map[string]GeneratedPermission{}
	for _, x := range old.Permissions {
		oldPerms[x.ID] = x
	}
	seen = map[string]bool{}
	for _, x := range m.Permissions {
		seen[x.ID] = true
		ox, exists := oldPerms[x.ID]
		switch {
		case !exists:
			p.add("permission: %s on %q for %v", x.Action, m.Name, x.Roles)
		case x.Action != ox.Action:
			p.refuse("permission %s cannot change its action from %s to %s -- add a new permission instead", x.ID, ox.Action, x.Action)
		case x.ActorField != ox.ActorField || !sameParentActor(x.ParentActor, ox.ParentActor):
			p.refuse("permission %s cannot change who it is limited to (the record's own person, or a related record's) -- add a new permission instead", x.ID)
		case !sameSet(x.Roles, ox.Roles):
			p.change("permission: %s on %q now for %v (was %v)", x.Action, m.Name, x.Roles, ox.Roles)
		}
	}
	for _, x := range old.Permissions {
		if !seen[x.ID] {
			p.refuse("permission %s cannot be removed: %s on %s would become open to everyone", x.ID, x.Action, m.ID)
		}
	}

	if len(old.Transitions) == 0 && len(m.Transitions) > 0 {
		p.change("the statuses of %q are governed from now on: only the moves declared can change them, and every other change of the status is refused", m.Name)
	}
	oldTrans := map[string]GeneratedTransition{}
	for _, x := range old.Transitions {
		oldTrans[x.ID] = x
	}
	seen = map[string]bool{}
	for _, x := range m.Transitions {
		seen[x.ID] = true
		ox, exists := oldTrans[x.ID]
		switch {
		case !exists:
			p.add("status move %q on %q: %s -> %s", x.Name, m.Name, x.From, x.To)
		case x.Name != ox.Name || x.Field != ox.Field || x.From != ox.From || x.To != ox.To || x.ActorField != ox.ActorField || !sameSet(x.Roles, ox.Roles) || !sameSets(x.Sets, ox.Sets) || !reflect.DeepEqual(x.Balanced, ox.Balanced) || !reflect.DeepEqual(x.Generates, ox.Generates):
			p.refuse("status move %s cannot be changed, only added -- who may take it included, because the file edit only appends", x.ID)
		}
	}
	for _, x := range old.Transitions {
		if !seen[x.ID] {
			p.refuse("status move %s cannot be removed", x.ID)
		}
	}

	switch {
	case old.FrozenWhen == nil && m.FrozenWhen != nil:
		p.add("freeze records of %q while %s is %s", m.Name, m.FrozenWhen.Field, m.FrozenWhen.Equals)
	case old.FrozenWhen != nil && (m.FrozenWhen == nil || *m.FrozenWhen != *old.FrozenWhen):
		p.refuse("the freeze rule of %s cannot be changed or removed, only added -- the file edit only appends and removing it would unfreeze records", m.ID)
	}
	oldEnsure := map[string]GeneratedEnsure{}
	for _, x := range old.Ensure {
		oldEnsure[x.ID] = x
	}
	seen = map[string]bool{}
	for _, x := range m.Ensure {
		seen[x.ID] = true
		ox, exists := oldEnsure[x.ID]
		switch {
		case !exists:
			p.add("limit %q on %q: the total of %s per %s", x.ID, m.Name, x.SumOf, x.Per)
		case !sameEnsure(x, ox):
			p.refuse("limit %s cannot be changed, only added -- the file edit only appends", x.ID)
		}
	}
	for _, x := range old.Ensure {
		if !seen[x.ID] {
			p.refuse("limit %s cannot be removed: writes it refuses would be accepted", x.ID)
		}
	}

	oldEvents := map[string]GeneratedEvent{}
	for _, x := range old.Events {
		oldEvents[x.ID] = x
	}
	seen = map[string]bool{}
	for _, x := range m.Events {
		seen[x.ID] = true
		ox, exists := oldEvents[x.ID]
		switch {
		case !exists:
			p.add("%s", ruleAdded(m.Name, x))
		case x.Disabled != ox.Disabled && withoutSwitch(x) == withoutSwitch(ox):
			if x.Disabled {
				p.change("rule %q on %q is switched off: it will not run until it is switched on again", x.ID, m.Name)
			} else {
				p.change("rule %q on %q is switched on again", x.ID, m.Name)
			}
		case x != ox:
			p.refuse("event %s cannot be changed, only added -- except to switch it off or on", x.ID)
		}
	}
	for _, x := range old.Events {
		if !seen[x.ID] {
			p.refuse("event %s cannot be removed", x.ID)
		}
	}
}

func planField(p *Plan, m GeneratedMachine, of, f GeneratedField) {
	if f.Type != of.Type {
		p.refuse("field %s on %s cannot change type from %s to %s: existing values would not fit", f.ID, m.ID, of.Type, f.Type)
		return
	}
	if f.RelatedMachine != of.RelatedMachine {
		p.refuse("field %s on %s cannot point at %s instead of %s: existing values name records of the old machine", f.ID, m.ID, f.RelatedMachine, of.RelatedMachine)
	}
	if f.Name != of.Name {
		p.change("rename field %q on %q to %q", of.Name, m.Name, f.Name)
	}
	if f.Required != of.Required {
		p.change("field %q on %q is now required: %v", f.Name, m.Name, f.Required)
	}
	for _, o := range f.Options {
		if !slices.Contains(of.Options, o) {
			p.add("option %q on %q", o, f.Name)
		}
	}
	for _, o := range of.Options {
		if !slices.Contains(f.Options, o) {
			p.refuse("option %q on %s cannot be removed: records may hold it", o, f.ID)
		}
	}
	if f.Stamp != of.Stamp {
		p.refuse("field %s on %s cannot gain, lose or change its stamp: records already stored would not match it", f.ID, m.ID)
	}
	if !sameRequiredWhen(f.RequiredWhen, of.RequiredWhen) || !sameDefaultFrom(f.DefaultFrom, of.DefaultFrom) {
		p.refuse("field %s on %s cannot change when it is required or what its date starts as -- the file edit only appends; add a new field instead", f.ID, m.ID)
	}
	switch {
	case sameShowWhen(f.ShowWhen, of.ShowWhen):
	case f.ShowWhen == nil:
		p.change("field %q on %q is now asked for always", f.Name, m.Name)
	default:
		p.change("field %q on %q is now asked for only while %s is %q", f.Name, m.Name, f.ShowWhen.Field, f.ShowWhen.Equals)
	}
	switch {
	case slices.Equal(sortedCopy(f.EditableBy), sortedCopy(of.EditableBy)):
	case len(f.EditableBy) == 0:
		p.refuse("field %s on %s cannot be opened to everyone again: who may change it was a decision about access, and clearing it would grant what it held back", f.ID, m.ID)
	case len(of.EditableBy) == 0:
		p.add("only %s may change field %q on %q", strings.Join(f.EditableBy, ", "), f.Name, m.Name)
	default:
		p.change("who may change field %q on %q is now %s", f.Name, m.Name, strings.Join(f.EditableBy, ", "))
	}
	if f.Default != of.Default {
		p.change("field %q on %q now starts at %q (was %q); records already stored keep their value", f.Name, m.Name, f.Default, of.Default)
	}
	if !sameCompute(of.Compute, f.Compute) {
		if f.Compute == nil {
			p.change("field %q on %q is no longer computed", f.Name, m.Name)
		} else {
			p.change("field %q on %q is computed: %s of %v", f.Name, m.Name, f.Compute.Op, f.Compute.Fields)
		}
	}
}

func planMenu(p *Plan, current, desired []GeneratedMenuItem) {
	byID := map[string]GeneratedMenuItem{}
	for _, n := range current {
		byID[n.ID] = n
	}
	kept := map[string]bool{}
	var keptOrder []string
	for _, n := range desired {
		old, exists := byID[n.ID]
		if (n.ID == "" || !exists) && n.List == nil && n.ByStatus == nil && n.Queue == nil {
			for _, other := range current {
				if other.MachineID == n.MachineID && other.List == nil && len(other.VisibleTo) > 0 {
					p.refuse("menu item %q would open the screens of %s to everyone, and %q restricts them to %s -- an address several entries name is open if any is offered", n.Label, n.MachineID, other.Label, strings.Join(other.VisibleTo, ", "))
				}
			}
		}
		if (n.ID == "" || !exists) && (n.Parent != "" || len(n.VisibleTo) > 0 || n.Badge != "") {
			p.refuse("menu item %q is new, so it cannot also name a parent, who may see it or a count -- add the item, then set those", n.Label)
		}
		if n.ID == "" || !exists {
			if n.ByStatus != nil {
				p.add("menu item %q, a tile per status of %s, and %q for the records in one", n.Label, n.MachineID, n.ByStatus.DetailLabel)
			} else if n.Queue != nil {
				p.add("menu item %q, the work queue of %s: figures, open work in sections, and a button per row for its next step", n.Label, n.MachineID)
			} else if n.List != nil {
				p.add("menu item %q, a list of %s records (%s)", n.Label, n.MachineID, n.List.Scope)
			} else {
				p.add("menu item %q, opening %s", n.Label, n.MachineID)
			}
			continue
		}
		if (n.List != nil && old.List == nil) || n.ByStatus != nil || n.Queue != nil {
			p.refuse("menu item %s cannot be given a list -- add a new menu item with the list instead", n.ID)
		}
		if n.List != nil && old.List != nil && !sameList(n.List, old.List) {
			p.change("the list of menu item %q is changed: %s", n.Label, listChange(old.List, n.List))
		}
		if n.List == nil && old.List != nil {
			p.refuse("menu item %s is a list and cannot stop being one -- remove it and add what you want instead", n.ID)
		}
		kept[n.ID] = true
		keptOrder = append(keptOrder, n.ID)
		if n.MachineID != "" && n.MachineID != old.MachineID {
			p.refuse("menu item %s cannot be pointed somewhere else -- remove it and add a new one", n.ID)
		}
		if n.Label != old.Label {
			p.change("menu item %q is now %q", old.Label, n.Label)
		}
		if n.Parent != old.Parent {
			if n.Parent == "" {
				p.change("menu item %q is a top-level item again", n.Label)
			} else {
				p.change("menu item %q now hangs under %s", n.Label, n.Parent)
			}
		}
		switch {
		case slices.Equal(sortedCopy(n.VisibleTo), sortedCopy(old.VisibleTo)):
		case len(n.VisibleTo) == 0:
			p.refuse("menu item %s cannot be offered to everyone again: who may see it was a decision about access, and clearing it would grant what it held back", n.ID)
		default:
			p.change("menu item %q is offered only to %s", n.Label, strings.Join(n.VisibleTo, ", "))
		}
		if n.Badge != old.Badge {
			if n.Badge == "" {
				p.change("menu item %q no longer shows a count", n.Label)
			} else {
				p.change("menu item %q shows how many records %s selects", n.Label, n.Badge)
			}
		}
	}
	var currentOrder []string
	for _, n := range current {
		if !kept[n.ID] {
			if n.MachineID == "" {
				p.refuse("menu item %s (%q) cannot be removed: it opens a screen that could not be added back", n.ID, n.Label)
			} else {
				p.remove("menu item %q (the %s records stay)", n.Label, n.MachineID)
			}
			continue
		}
		currentOrder = append(currentOrder, n.ID)
	}
	if !slices.Equal(currentOrder, keptOrder) {
		p.change("menu order")
	}
}

// planCards: a card role may be added; changing which field plays a role, or dropping an entry, is refused
// (a card the person has learned to read would change under them, and nothing here asks them first).
func planCards(p *Plan, old, m GeneratedMachine) {
	oldBy := map[string]GeneratedCardField{}
	for _, x := range old.CardFields {
		oldBy[x.Field+"/"+x.Role] = x
	}
	seen := map[string]bool{}
	for _, x := range m.CardFields {
		key := x.Field + "/" + x.Role
		seen[key] = true
		if _, ok := oldBy[key]; !ok {
			p.add("card shows %q as %s on %q", x.Field, x.Role, m.Name)
		}
	}
	for key := range oldBy {
		if !seen[key] {
			p.refuse("card entry %s on %s cannot be removed or re-roled, only added", key, m.ID)
		}
	}
}

// planViews: a view may be added and renamed; retyping, regrouping or removing one is refused, and so is
// putting a new view ahead of the existing ones (the first view is what opens by default).
func planViews(p *Plan, old, m GeneratedMachine) {
	oldBy := map[string]GeneratedView{}
	for _, v := range old.Views {
		oldBy[v.ID] = v
	}
	seen := map[string]bool{}
	for i, v := range m.Views {
		seen[v.ID] = true
		ov, exists := oldBy[v.ID]
		switch {
		case !exists:
			if i < len(old.Views) {
				p.refuse("new view %s on %s must come after the existing views: the first view is what opens by default", v.ID, m.ID)
				continue
			}
			p.add("%s view %q on %q", v.Type, v.Name, m.Name)
		case v.Type != ov.Type || v.GroupBy != ov.GroupBy:
			p.refuse("view %s on %s cannot change its type or grouping -- add a new view instead", v.ID, m.ID)
		case v.Name != ov.Name:
			p.change("view %q on %q is now called %q", ov.Name, m.Name, v.Name)
		}
	}
	removed := 0
	for _, ov := range old.Views {
		if !seen[ov.ID] {
			removed++
			p.change("view %q on %q is removed", ov.Name, m.Name)
		}
	}
	// a view arranges records and holds none, so dropping one loses no data -- but a machine with no
	// view has no screen, so one must remain
	if removed > 0 && len(m.Views) == 0 {
		p.refuse("machine %s needs at least one view: keep one, or add the view that should replace the others", m.ID)
	}
	// the existing views may be reordered: the first is what opens by default, which is often exactly
	// what a person asks for ("only the board")
	var oldOrder, newOrder []string
	for _, ov := range old.Views {
		if seen[ov.ID] {
			oldOrder = append(oldOrder, ov.ID)
		}
	}
	for _, v := range m.Views {
		if _, ok := oldBy[v.ID]; ok {
			newOrder = append(newOrder, v.ID)
		}
	}
	if !slices.Equal(oldOrder, newOrder) {
		p.change("the views on %q are reordered: the first now opens by default", m.Name)
	}
}

// categoriesOnlyGrowForNewOptions says whether m's status categories are old's plus entries for options that did not exist before: nothing a
// person has learned to read changes colour, because an option that was not there had no colour to learn. An entry that changes, or one for an option
// that was already there, is not this and is refused as it always was.
func categoriesOnlyGrowForNewOptions(old, m GeneratedMachine) bool {
	if old.StatusCategories.Field != m.StatusCategories.Field {
		return false
	}
	had := map[string]string{}
	for _, v := range old.StatusCategories.Values {
		had[v.Option] = v.Category
	}
	var oldOptions []string
	for _, f := range old.Fields {
		if f.ID == old.StatusCategories.Field {
			oldOptions = f.Options
		}
	}
	grew := false
	now := map[string]string{}
	for _, v := range m.StatusCategories.Values {
		now[v.Option] = v.Category
		if c, ok := had[v.Option]; ok {
			if c != v.Category {
				return false
			}
			continue
		}
		if slices.Contains(oldOptions, v.Option) {
			return false
		}
		grew = true
	}
	for o := range had {
		if _, ok := now[o]; !ok {
			return false
		}
	}
	return grew
}

func sameStatusCategories(a, b *GeneratedStatusCategories) bool {
	if a == nil || b == nil {
		return a == b
	}
	norm := func(s *GeneratedStatusCategories) map[string]string {
		m := map[string]string{}
		for _, v := range s.Values {
			m[v.Option] = v.Category
		}
		return m
	}
	return a.Field == b.Field && maps.Equal(norm(a), norm(b))
}

func sameCompletion(a, b *GeneratedCompletion) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameParentActor(a, b *GeneratedParentActor) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func sameCompute(a, b *GeneratedCompute) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Op == b.Op && slices.Equal(a.Fields, b.Fields)
}

// keepDeclared gives a desired Machine back what the assistant's shapes cannot carry and it handed back unchanged: a
// computed value declared as a tree (`{subtract: [...]}`, `sum_children`), which it is shown as its operation with no
// fields, and an `ensure:` bound taken from a parent. The file edit leaves both as written (they compare equal to what
// was described); without this the update would be judged against a Machine that lost them, and refused (the review of
// step 3 found every update of Production Contracts refused).
func keepDeclared(desired, current *domain.Machine) {
	if desired == nil || current == nil {
		return
	}
	for i, f := range desired.Fields {
		was, ok := current.FieldByID(f.ID)
		if !ok || was.Compute == nil || was.Compute.Expr == nil || f.Compute == nil {
			continue
		}
		if f.Compute.Op == was.Compute.Op && len(f.Compute.Fields) == 0 {
			desired.Fields[i].Compute = was.Compute
		}
	}
	for i, b := range desired.Bounds {
		for _, was := range current.Bounds {
			if was.ID == b.ID && was.AtMostParent != nil && b.AtMost == nil {
				desired.Bounds[i].AtMostParent = was.AtMostParent
			}
		}
	}
}

// validateUpdateApplication checks an update the way a new Application is checked -- every desired
// Machine through the real metadata.Validate, every Permission's roles against the desired roles,
// the menu against the desired Machines -- and then holds it to PlanUpdate's policy.
func validateUpdateApplication(change GeneratedChange, existing ExistingState) error {
	current, ok := existing.Applications[change.TargetAppID]
	if !ok {
		return &ValidationError{Issues: []string{fmt.Sprintf("application %q is not installed in this workspace", change.TargetAppID)}}
	}
	desired := change.Application
	if desired == nil {
		return &ValidationError{Issues: []string{"update_application carries no application -- return the whole application as it should be"}}
	}
	var issues []string
	if strings.TrimSpace(desired.Name) == "" {
		issues = append(issues, "application name is required")
	}
	if desired.Icon != "" && !domain.KnownIcons[desired.Icon] {
		issues = append(issues, fmt.Sprintf("icon %q is not a known icon", desired.Icon))
	}
	if desired.Color != "" && !domain.KnownApplicationColors[desired.Color] {
		issues = append(issues, fmt.Sprintf("color %q is not a known application color", desired.Color))
	}

	targets := relationTargets(existing)
	roles := map[string]bool{}
	for _, r := range desired.Roles {
		roles[r] = true
	}
	for _, m := range desired.Machines {
		targets[m.ID] = true
	}
	own := map[string]bool{}
	for _, m := range desired.Machines {
		if own[m.ID] {
			issues = append(issues, fmt.Sprintf("machine id %q is declared more than once", m.ID))
		}
		own[m.ID] = true
		if _, installed := current.Machines[m.ID]; !installed && existing.MachineIDs[m.ID] {
			issues = append(issues, fmt.Sprintf("machine id %q already exists in this workspace", m.ID))
		}
		issues = append(issues, frozenIssues(m, desired.Machines)...)
		for _, ev := range m.Events {
			if ev.RecipientRole != "" && !roles[ev.RecipientRole] {
				issues = append(issues, fmt.Sprintf("machine %q event %q tells role %q, which the application does not declare in %v", m.ID, ev.ID, ev.RecipientRole, desired.Roles))
			}
		}
		for _, perm := range m.Permissions {
			for _, r := range perm.Roles {
				if !roles[r] {
					issues = append(issues, fmt.Sprintf("machine %q permission %q names role %q, which the application does not declare in %v", m.ID, perm.ID, r, desired.Roles))
				}
			}
		}
		for _, tr := range m.Transitions {
			for _, r := range tr.Roles {
				if !roles[r] {
					issues = append(issues, fmt.Sprintf("machine %q step %q names role %q, which the application does not declare in %v", m.ID, tr.ID, r, desired.Roles))
				}
			}
		}
		dm, fieldIssues := buildDomainMachine(m, targets)
		keepDeclared(dm, current.Machines[m.ID])
		fieldIssues = append(fieldIssues, notificationIssues(m, existing)...)
		issues = append(issues, fieldIssues...)
		if len(fieldIssues) == 0 {
			if err := metadata.Validate(dm); err != nil {
				issues = append(issues, fmt.Sprintf("machine %q: %v", m.ID, err))
			}
		}
	}
	currentNav := map[string]bool{}
	for _, n := range current.Navigation {
		currentNav[n.ID] = true
	}
	for _, n := range desired.Navigation {
		if strings.TrimSpace(n.Label) == "" {
			issues = append(issues, "every menu item needs a label")
		}
		if (n.ID == "" || !currentNav[n.ID]) && !own[n.MachineID] {
			issues = append(issues, fmt.Sprintf("new menu item %q must open one of this application's machines, got %q", n.Label, n.MachineID))
		}
	}

	// A list that is new or changed is held to what a list needs of its Machine; a list left as it was is not judged again, so an old one a person wrote
	// by hand cannot stand in the way of an unrelated edit.
	described := DescribeApplication(change.TargetAppID, current)
	oldLists := map[string]*GeneratedList{}
	for _, n := range described.Navigation {
		oldLists[n.ID] = n.List
	}
	judged := *desired
	judged.Navigation = nil
	for _, n := range desired.Navigation {
		if n.List != nil && (n.ID == "" || !sameList(oldLists[n.ID], n.List)) {
			judged.Navigation = append(judged.Navigation, n)
		}
	}
	issues = append(issues, validateLists(&judged)...)

	plan := PlanUpdate(described, *desired)
	issues = append(issues, plan.Refusals...)
	if len(plan.Items) == 0 && len(plan.Refusals) == 0 {
		issues = append(issues, "the returned application is identical to the installed one -- nothing would change. If the change the person asked for cannot be made, say so plainly in your message and why; do not tell them it is done")
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func sameRequiredWhen(a, b *GeneratedRequiredWhen) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameDefaultFrom(a, b *GeneratedDefaultFrom) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Field != b.Field || len(a.Days) != len(b.Days) {
		return false
	}
	have := map[GeneratedDayCount]bool{}
	for _, d := range a.Days {
		have[d] = true
	}
	for _, d := range b.Days {
		if !have[d] {
			return false
		}
	}
	return true
}

func cloneFloat(f *float64) *float64 {
	if f == nil {
		return nil
	}
	v := *f
	return &v
}

func sameFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameEnsure(a, b GeneratedEnsure) bool {
	return a.SumOf == b.SumOf && a.Per == b.Per && a.Reason == b.Reason && sameFloat(a.AtLeast, b.AtLeast) && sameFloat(a.AtMost, b.AtMost)
}

func balancedSpec(b *domain.BalanceRule) *GeneratedBalanced {
	if b == nil {
		return nil
	}
	return &GeneratedBalanced{Of: b.Machine, Via: b.Via, Debit: b.Debit, Credit: b.Credit, Reason: b.Reason}
}

func generatesSpec(g *domain.GenerateSchedule) *GeneratedGenerates {
	if g == nil {
		return nil
	}
	return &GeneratedGenerates{Machine: g.Machine, CountField: g.CountField, LinkField: g.LinkField, DueField: g.DueField, DueEveryDays: g.DueEveryDays, AmountField: g.AmountField, AmountFrom: g.AmountFrom}
}

func setsList(m map[string]string) []GeneratedSet {
	if len(m) == 0 {
		return nil
	}
	out := make([]GeneratedSet, 0, len(m))
	for f, src := range m {
		out = append(out, GeneratedSet{Field: f, Source: src})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

func sameSets(a, b []GeneratedSet) bool {
	am, bm := setsOf(a), setsOf(b)
	if len(am) != len(bm) {
		return false
	}
	for f, s := range am {
		if bm[f] != s {
			return false
		}
	}
	return true
}

func sortedCopy(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}

func sameShowWhen(a, b *GeneratedShowWhen) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// listChange says in a line what differs between two versions of a list, for the plan.
func listChange(old, now *GeneratedList) string {
	var what []string
	if !slices.Equal(old.Columns, now.Columns) {
		what = append(what, "columns "+strings.Join(now.Columns, ", "))
	}
	if old.SortField != now.SortField || old.Direction != now.Direction {
		what = append(what, "ordered by "+now.SortField+" "+now.Direction)
	}
	if old.Scope != now.Scope || old.PersonField != now.PersonField {
		what = append(what, "shows "+now.Scope+" records")
	}
	if old.Limit != now.Limit {
		what = append(what, "at most "+strconv.Itoa(now.Limit))
	}
	if old.FilterField != now.FilterField {
		what = append(what, "chips by "+now.FilterField)
	}
	if !slices.Equal(old.SearchFields, now.SearchFields) {
		what = append(what, "search in "+strings.Join(now.SearchFields, ", "))
	}
	if old.AddLabel != now.AddLabel {
		what = append(what, "add button "+now.AddLabel)
	}
	if old.Density != now.Density {
		what = append(what, "rows "+now.Density)
	}
	if old.GroupBy != now.GroupBy {
		what = append(what, "grouped by "+now.GroupBy)
	}
	if old.RowTone != now.RowTone {
		what = append(what, "rows toned by "+now.RowTone)
	}
	return strings.Join(what, "; ")
}

// sameList compares two lists as a person would: an empty list of columns or search fields is the same as none, so a list echoed back with `[]` where it was read with nothing
// is not a change.
func sameList(a, b *GeneratedList) bool {
	if a == nil || b == nil {
		return a == b
	}
	x, y := *a, *b
	for _, l := range []*GeneratedList{&x, &y} {
		if len(l.Columns) == 0 {
			l.Columns = nil
		}
		if len(l.SearchFields) == 0 {
			l.SearchFields = nil
		}
		if l.SortField == "" {
			l.SortField = "created_at"
		}
		if l.Direction == "" {
			l.Direction = "desc"
		}
		if l.Density == string(domain.ListRegular) {
			l.Density = ""
		}
	}
	return reflect.DeepEqual(x, y)
}

// withoutSwitch is a rule with its switch left as it was, to tell a rule that was only switched from one that was changed.
func withoutSwitch(e GeneratedEvent) GeneratedEvent {
	e.Disabled = false
	return e
}

// ruleAdded says a new rule in a line for the plan: when it runs and what it does.
func ruleAdded(machine string, e GeneratedEvent) string {
	when := "a field changes (" + e.On + ")"
	switch {
	case e.Schedule != nil:
		when = e.Schedule.DateField + " is " + e.Schedule.When
	case e.OnCreate:
		when = "a record is created"
	case e.WhenEquals != "":
		when = e.On + " becomes " + e.WhenEquals
	}
	switch e.EventService() {
	case EventServiceSendNotification:
		who := e.RecipientField
		if e.RecipientRole != "" {
			who = "everyone who is " + e.RecipientRole
		} else if e.RecipientGroupField != "" {
			who = "the group in " + e.RecipientGroupField
		}
		return fmt.Sprintf("rule on %q: when %s, tell %s: %s", machine, when, who, e.Summary)
	case EventServiceSetField:
		return fmt.Sprintf("rule on %q: when %s, set %s to %s", machine, when, e.SetField, e.SetValue)
	}
	return fmt.Sprintf("rule on %q: when %s, write an activity entry: %s", machine, when, e.Summary)
}
