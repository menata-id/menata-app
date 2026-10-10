package aiassist

import (
	"fmt"
	"slices"
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
		g.Navigation = append(g.Navigation, GeneratedMenuItem{ID: n.ID, Label: n.Label, MachineID: machineOfRoute(n.Route)})
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
		g.Fields = append(g.Fields, gf)
	}
	for _, p := range m.Permissions {
		if expressiblePermission(p) {
			g.Permissions = append(g.Permissions, GeneratedPermission{ID: p.ID, Action: p.Action, Roles: slices.Clone(p.Roles)})
		}
	}
	for _, t := range m.Transitions {
		if t.Action == domain.ActionEdit {
			g.Transitions = append(g.Transitions, GeneratedTransition{ID: t.ID, Name: t.Name, Field: t.Field, From: t.From, To: t.To})
		}
	}
	for _, e := range m.Events {
		if e.Schedule == nil && e.Then.Name == domain.ServiceLogActivity && e.Then.SummaryOverrideWhen == "" {
			g.Events = append(g.Events, GeneratedEvent{ID: e.ID, On: e.On, WhenEquals: e.WhenEquals, OnCreate: e.OnCreate, Summary: e.Then.Summary})
		}
	}
	return g
}

func expressiblePermission(p domain.Permission) bool {
	switch p.Action {
	case domain.ActionCreate, domain.ActionEdit, domain.ActionDelete:
		return p.ActorField == "" && p.DynamicActor == nil && p.ParentActor == nil
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
//   - nothing that holds records or grants access may be removed or retyped: a Machine, a Field, a
//     declared option, a role, a Permission (removing one opens that action to everyone), a
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
			p.refuse("field %s (%q) on %s cannot be removed: its values would be lost", f.ID, f.Name, m.ID)
		}
	}

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
		case !sameSet(x.Roles, ox.Roles):
			p.change("permission: %s on %q now for %v (was %v)", x.Action, m.Name, x.Roles, ox.Roles)
		}
	}
	for _, x := range old.Permissions {
		if !seen[x.ID] {
			p.refuse("permission %s cannot be removed: %s on %s would become open to everyone", x.ID, x.Action, m.ID)
		}
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
		case x != ox:
			p.refuse("status move %s cannot be changed, only added", x.ID)
		}
	}
	for _, x := range old.Transitions {
		if !seen[x.ID] {
			p.refuse("status move %s cannot be removed", x.ID)
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
			p.add("activity entry on %q: %s", m.Name, x.Summary)
		case x != ox:
			p.refuse("event %s cannot be changed, only added", x.ID)
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
		if n.ID == "" || !exists {
			p.add("menu item %q, opening %s", n.Label, n.MachineID)
			continue
		}
		kept[n.ID] = true
		keptOrder = append(keptOrder, n.ID)
		if n.MachineID != "" && n.MachineID != old.MachineID {
			p.refuse("menu item %s cannot be pointed somewhere else -- remove it and add a new one", n.ID)
		}
		if n.Label != old.Label {
			p.change("menu item %q is now %q", old.Label, n.Label)
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
		for _, perm := range m.Permissions {
			for _, r := range perm.Roles {
				if !roles[r] {
					issues = append(issues, fmt.Sprintf("machine %q permission %q names role %q, which the application does not declare in %v", m.ID, perm.ID, r, desired.Roles))
				}
			}
		}
		dm, fieldIssues := buildDomainMachine(m, targets)
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

	plan := PlanUpdate(DescribeApplication(change.TargetAppID, current), *desired)
	issues = append(issues, plan.Refusals...)
	if len(plan.Items) == 0 && len(plan.Refusals) == 0 {
		issues = append(issues, "the returned application is identical to the installed one -- nothing would change")
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}
