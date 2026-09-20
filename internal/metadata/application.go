package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

var (
	workspaceIDPattern   = regexp.MustCompile(`^ws_[a-z][a-z0-9_]*$`)
	applicationIDPattern = regexp.MustCompile(`^app_[a-z][a-z0-9_]*$`)
)

// App is a loaded Application manifest: its Workspace, itself, and every Machine it references.
type App struct {
	Workspace   domain.Workspace
	Application domain.Application
	Machines    []*domain.Machine
}

type appDoc struct {
	Workspace struct {
		ID   string `yaml:"id"`
		Name string `yaml:"name"`
	} `yaml:"workspace"`
	Application struct {
		ID         string       `yaml:"id"`
		Name       string       `yaml:"name"`
		Machines   []string     `yaml:"machines"`
		Navigation []navItemDoc `yaml:"navigation"`
		// HiddenNavGroups names navigation: groups (by their group: label) whose items are
		// declared -- so they remain valid destinations, reachable by route and by contextual
		// in-page links -- but must not render in the topbar. Owner request, 2026-09-19: an
		// Application's own screens don't always need a persistent menu entry; this is the
		// metadata-only way to say so, with no runtime change beyond a shorter list handed to the
		// existing topbar renderer (internal/rendering.pageShell already renders whatever list
		// it's given, nothing about it changes here).
		HiddenNavGroups []string `yaml:"hidden_nav_groups"`
	} `yaml:"application"`
}

type navItemDoc struct {
	ID       string `yaml:"id"`
	Label    string `yaml:"label"`
	Route    string `yaml:"route"`
	Group    string `yaml:"group"`
	Priority int    `yaml:"priority"`
	Badge    string `yaml:"badge"`
	HomeCard bool   `yaml:"home_card"`
}

// LoadApplication reads an Application manifest and every Machine file it references (paths
// resolved relative to the manifest's own directory), validating each in turn
// (005-runtime-lifecycle.md Phase 3-4: invalid metadata must not enter execution).
func LoadApplication(path string) (*App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read application manifest %s: %w", path, err)
	}

	var doc appDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse application manifest: %w", err)
	}

	var issues []string
	if !workspaceIDPattern.MatchString(doc.Workspace.ID) {
		issues = append(issues, fmt.Sprintf("workspace id %q must match %s", doc.Workspace.ID, workspaceIDPattern.String()))
	}
	if !applicationIDPattern.MatchString(doc.Application.ID) {
		issues = append(issues, fmt.Sprintf("application id %q must match %s", doc.Application.ID, applicationIDPattern.String()))
	}
	if len(doc.Application.Machines) == 0 {
		issues = append(issues, fmt.Sprintf("application %q: at least one machine is required", doc.Application.ID))
	}
	if len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}

	var navigation []domain.NavigationItem
	for _, n := range doc.Application.Navigation {
		navigation = append(navigation, domain.NavigationItem{
			ID:       n.ID,
			Label:    n.Label,
			Route:    n.Route,
			Group:    n.Group,
			Priority: n.Priority,
			Badge:    n.Badge,
			HomeCard: n.HomeCard,
		})
	}
	if navIssues := validateNavigation(navigation); len(navIssues) > 0 {
		return nil, &ValidationError{Issues: navIssues}
	}

	// primaryNavGroup, homeRoute and allNavigation are all decided from the full declared list,
	// before hidden_nav_groups removes anything -- so hiding a group can never promote a
	// different one into primaryNavGroup's "always open" role, silently blank out homeRoute, or
	// (allNavigation) make a hidden item's own route unreachable by id for a contextual in-page
	// link (see domain.Application.PrimaryNavGroup/HomeRoute/AllNavigation).
	var primaryNavGroup string
	for _, g := range experience.GroupNavigation(navigation) {
		if g.Label != "" {
			primaryNavGroup = g.Label
			break
		}
	}
	homeRoute := domain.HomeCardRoute(navigation)
	allNavigation := navigation

	navigation, hiddenIssues := applyHiddenNavGroups(navigation, doc.Application.HiddenNavGroups)
	if len(hiddenIssues) > 0 {
		return nil, &ValidationError{Issues: hiddenIssues}
	}

	dir := filepath.Dir(path)
	app := &App{
		Workspace: domain.Workspace{ID: doc.Workspace.ID, Name: doc.Workspace.Name},
		Application: domain.Application{
			ID: doc.Application.ID, Name: doc.Application.Name, WorkspaceID: doc.Workspace.ID,
			Navigation: navigation, PrimaryNavGroup: primaryNavGroup, HomeRoute: homeRoute, AllNavigation: allNavigation,
		},
	}
	for _, rel := range doc.Application.Machines {
		m, err := Load(filepath.Join(dir, rel))
		if err != nil {
			return nil, fmt.Errorf("application %q: machine %s: %w", doc.Application.ID, rel, err)
		}
		app.Machines = append(app.Machines, m)
	}

	if err := validateRelationTargets(app.Machines); err != nil {
		return nil, err
	}
	if err := validateConstraintTargets(app.Machines); err != nil {
		return nil, err
	}
	if err := validateDatasetIDsAreUnique(app.Machines); err != nil {
		return nil, err
	}
	if err := validateRollupTargets(app.Machines); err != nil {
		return nil, err
	}
	return app, nil
}

// validateRollupTargets closes the half of a rollup declaration a single Machine file cannot check
// for itself: the Field it writes lives on the *parent* Machine, so its existence, and whether the
// values the rollup sets are among that Field's own options, can only be verified once every
// Machine is loaded -- the same reason validateRelationTargets exists.
//
// Without this, a rollup naming a field the parent doesn't have would write a value nothing reads,
// and a rollup setting a value outside the parent field's options would store a status no screen
// can render -- both silent at load time, both visible only as a page that quietly shows the wrong
// thing.
func validateRollupTargets(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}

	var issues []string
	for _, m := range machines {
		for _, e := range m.Events {
			r := e.Then.Rollup
			if r == nil {
				continue
			}
			parentField, ok := m.FieldByID(r.ParentField)
			if !ok {
				continue // already reported by validateRollup
			}
			parent, ok := byID[parentField.RelatedMachine]
			if !ok {
				issues = append(issues, fmt.Sprintf("machine %q: event %q: then.parent_field %q points at machine %q, which this application does not declare", m.ID, e.ID, r.ParentField, parentField.RelatedMachine))
				continue
			}
			target, ok := parent.FieldByID(r.TargetField)
			if !ok {
				issues = append(issues, fmt.Sprintf("machine %q: event %q: then.target_field %q is not a field of parent machine %q", m.ID, e.ID, r.TargetField, parent.ID))
				continue
			}
			for _, w := range []struct{ key, value string }{{"any.set", r.AnySet}, {"all.set", r.AllSet}, {"default", r.Default}} {
				if w.value == "" {
					continue
				}
				if len(target.Options) > 0 && !contains(target.Options, w.value) {
					issues = append(issues, fmt.Sprintf("machine %q: event %q: then.%s %q is not one of parent field %q's options %v", m.ID, e.ID, w.key, w.value, r.TargetField, target.Options))
				}
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateDatasetIDsAreUnique makes a Dataset id unique across the whole Application, not just
// within its own Machine file -- which a single file cannot check for itself.
//
// This is what makes a Dataset addressable by id alone (composition.Loader.Dataset): a screen
// names ds_task_by_project and the runtime knows which Machine's records that means, because
// exactly one Machine can declare it. Without this the id would be ambiguous and every caller
// would have to keep naming a Machine id alongside it -- which is precisely the hardcoding this
// resolution exists to remove.
func validateDatasetIDsAreUnique(machines []*domain.Machine) error {
	owner := make(map[string]string)
	var issues []string
	for _, m := range machines {
		for _, ds := range m.Datasets {
			if prev, taken := owner[ds.ID]; taken {
				issues = append(issues, fmt.Sprintf("dataset id %q is declared by both machine %q and machine %q -- a dataset id must be unique across the application, since screens resolve it by id alone", ds.ID, prev, m.ID))
				continue
			}
			owner[ds.ID] = m.ID
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateRelationTargets checks that every reference field's target Machine ID (Relation's
// explicit `machine:`, or Person's implicit mch_user) is actually among the Application's own
// Machines -- a single Machine file can't know this on its own, since it only sees its own
// declaration (006-runtime-model.md "Relation": reusable, grounded in existing Machine/reference
// semantics).
func validateRelationTargets(machines []*domain.Machine) error {
	known := make(map[string]bool, len(machines))
	for _, m := range machines {
		known[m.ID] = true
	}

	var issues []string
	for _, m := range machines {
		for _, f := range m.Fields {
			if !f.IsReference() {
				continue
			}
			if !known[f.RelatedMachine] {
				issues = append(issues, fmt.Sprintf("machine %q field %q: relation target %q is not a machine in this application", m.ID, f.ID, f.RelatedMachine))
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateConstraintTargets checks, once every Machine is loaded, that each Constraint's
// block_if actually names a real related Machine and Field -- and that the related field is
// itself a relation pointing back to this Machine, otherwise "related_field" wouldn't identify
// which of the related Machine's records belong to the record being transitioned.
func validateConstraintTargets(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}

	var issues []string
	for _, m := range machines {
		for _, c := range m.Constraints {
			related, ok := byID[c.BlockIf.RelatedMachine]
			if !ok {
				issues = append(issues, fmt.Sprintf("machine %q constraint %q: block_if.related_machine %q is not a machine in this application", m.ID, c.ID, c.BlockIf.RelatedMachine))
				continue
			}

			relatedField, ok := related.FieldByID(c.BlockIf.RelatedField)
			if !ok {
				issues = append(issues, fmt.Sprintf("machine %q constraint %q: block_if.related_field %q is not a field of machine %q", m.ID, c.ID, c.BlockIf.RelatedField, related.ID))
			} else if relatedField.Type != domain.FieldTypeRelation || relatedField.RelatedMachine != m.ID {
				issues = append(issues, fmt.Sprintf("machine %q constraint %q: block_if.related_field %q must be a relation field on %q pointing back to %q", m.ID, c.ID, c.BlockIf.RelatedField, related.ID, m.ID))
			}

			if _, ok := related.FieldByID(c.BlockIf.Condition.Field); !ok {
				issues = append(issues, fmt.Sprintf("machine %q constraint %q: block_if.condition.field %q is not a field of machine %q", m.ID, c.ID, c.BlockIf.Condition.Field, related.ID))
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}
