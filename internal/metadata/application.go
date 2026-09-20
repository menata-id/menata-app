package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

var (
	workspaceIDPattern   = regexp.MustCompile(`^ws_[a-z][a-z0-9_]*$`)
	applicationIDPattern = regexp.MustCompile(`^app_[a-z][a-z0-9_]*$`)
)

// App is a loaded Workspace manifest: the Workspace itself (including every Application declared
// inside it) and every Machine it owns.
//
// Machines are workspace-level and unique by id across the whole Workspace (Fase 3, 2026-09-20),
// which is why they live here rather than under one Application -- see domain.Workspace.
type App struct {
	Workspace domain.Workspace
	Machines  []*domain.Machine
}

type appDoc struct {
	Workspace struct {
		ID         string       `yaml:"id"`
		Name       string       `yaml:"name"`
		Machines   []string     `yaml:"machines"`
		Navigation []navItemDoc `yaml:"navigation"`
	} `yaml:"workspace"`
	// Applications are file paths, resolved relative to this manifest -- the same shape machines:
	// has always had. One file per Application: a navigation block alone runs to ~40 lines, so
	// inlining several would bury the Workspace identity, and it keeps one Application's menu
	// changes off another's lines.
	Applications []string `yaml:"applications"`
}

// applicationDoc is one Application's own file.
type applicationDoc struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Machines []string `yaml:"machines"`
	// Roles is this Application's own role vocabulary -- see domain.Application.Roles. Optional:
	// an Application that declares none offers no role, rather than falling back to another's.
	Roles []string `yaml:"roles"`
	// Description/Icon/Color/SummaryMachine are the Application's card face on Workspace Home --
	// see domain.Application for what each is and why the count is declared rather than summed.
	Description    string `yaml:"description"`
	Icon           string `yaml:"icon"`
	Color          string `yaml:"color"`
	SummaryMachine string `yaml:"summary_machine"`
	// ShowNav defaults to *true* when the key is absent, which is why it is a *bool here: an
	// Application that says nothing about its menu keeps it (ui-sample/nav-metadata.js's own
	// convention -- case19 has no showNav field and keeps both bars). A plain bool would default
	// to false and silently suppress every Application's menu.
	ShowNav    *bool        `yaml:"show_nav"`
	Navigation []navItemDoc `yaml:"navigation"`
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

// LoadApplication reads a Workspace manifest: its own Machine files and navigation, then every
// Application file it references (all paths resolved relative to the manifest's own directory),
// validating each in turn (005-runtime-lifecycle.md Phase 3-4: invalid metadata must not enter
// execution).
//
// The name is kept for its callers' sake; what it loads is a Workspace, of which an Application is
// now one part (Fase 3, 2026-09-20).
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
	if len(doc.Workspace.Machines) == 0 {
		issues = append(issues, fmt.Sprintf("workspace %q: at least one machine is required", doc.Workspace.ID))
	}
	if len(doc.Applications) == 0 {
		issues = append(issues, fmt.Sprintf("workspace %q: at least one application is required", doc.Workspace.ID))
	}
	if len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}

	workspaceNav := toNavigationItems(doc.Workspace.Navigation)
	if navIssues := validateNavigation(workspaceNav); len(navIssues) > 0 {
		return nil, &ValidationError{Issues: navIssues}
	}

	dir := filepath.Dir(path)
	app := &App{
		Workspace: domain.Workspace{
			ID:         doc.Workspace.ID,
			Name:       doc.Workspace.Name,
			Navigation: workspaceNav,
		},
	}

	// Machines first, and once: every Application selects from this one set by id, so they must
	// exist before any Application is resolved against them.
	for _, rel := range doc.Workspace.Machines {
		m, err := Load(filepath.Join(dir, rel))
		if err != nil {
			return nil, fmt.Errorf("workspace %q: machine %s: %w", doc.Workspace.ID, rel, err)
		}
		app.Machines = append(app.Machines, m)
	}
	if err := validateMachineIDsAreUnique(app.Machines); err != nil {
		return nil, err
	}

	for _, rel := range doc.Applications {
		application, err := loadApplicationFile(filepath.Join(dir, rel), doc.Workspace.ID)
		if err != nil {
			return nil, err
		}
		app.Workspace.Applications = append(app.Workspace.Applications, *application)
	}
	if err := validateApplicationClaims(app.Workspace.Applications, app.Machines); err != nil {
		return nil, err
	}
	if err := validateNavigationIDsAreUnique(app.Workspace); err != nil {
		return nil, err
	}

	// The four cross-Machine validators below run over the Workspace's whole Machine set, not one
	// Application's. That is not a widening for convenience: a Machine shared by two Applications
	// (mch_user, mch_activity) has exactly one declaration, so a per-Application scope would ask
	// the same question twice and make dataset-id uniqueness incoherent -- the same declaration
	// living in two scopes at once. See validateDatasetIDsAreUnique's own doc comment.
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
	if err := validateSequencingModes(app.Machines); err != nil {
		return nil, err
	}
	return app, nil
}

// loadApplicationFile reads and validates one Application's own file.
func loadApplicationFile(path, workspaceID string) (*domain.Application, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read application %s: %w", path, err)
	}
	var doc applicationDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse application %s: %w", path, err)
	}

	var issues []string
	if !applicationIDPattern.MatchString(doc.ID) {
		issues = append(issues, fmt.Sprintf("application id %q must match %s", doc.ID, applicationIDPattern.String()))
	}
	if len(doc.Machines) == 0 {
		issues = append(issues, fmt.Sprintf("application %q: at least one machine is required", doc.ID))
	}
	// Roles are a plain vocabulary, so the only things worth checking are that each word is
	// usable and said once. A blank entry would render an unlabelled option indistinguishable
	// from "no role"; a duplicate would render the same option twice.
	seenRole := make(map[string]bool, len(doc.Roles))
	for _, r := range doc.Roles {
		switch {
		case strings.TrimSpace(r) == "":
			issues = append(issues, fmt.Sprintf("application %q: roles entry is empty -- omit the role rather than declaring a blank one, which is how \"no role\" is already said", doc.ID))
		case seenRole[r]:
			issues = append(issues, fmt.Sprintf("application %q: role %q is declared more than once", doc.ID, r))
		default:
			seenRole[r] = true
		}
	}
	if doc.Color != "" && !domain.KnownApplicationColors[doc.Color] {
		issues = append(issues, fmt.Sprintf("application %q: color %q is not a known application color", doc.ID, doc.Color))
	}
	// summary_machine must be one of this Application's *own* machines. Pointing at another
	// Application's would put that Application's record count on this card -- the same misleading
	// number the declaration exists to prevent, just sourced differently.
	if doc.SummaryMachine != "" && !slices.Contains(doc.Machines, doc.SummaryMachine) {
		issues = append(issues, fmt.Sprintf("application %q: summary_machine %q is not one of this application's own machines -- a card must report its own count, not another application's", doc.ID, doc.SummaryMachine))
	}
	if len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}

	navigation := toNavigationItems(doc.Navigation)
	if navIssues := validateNavigation(navigation); len(navIssues) > 0 {
		return nil, &ValidationError{Issues: navIssues}
	}

	// PrimaryNavGroup, HomeRoute and AllNavigation are all decided from the full declared list,
	// *before* show_nav suppresses anything -- the same freeze-then-filter ordering
	// hidden_nav_groups needed, and load-bearing for the same three reasons: suppressing a menu
	// must never promote a different group into PrimaryNavGroup's "always open" role, never
	// silently blank out HomeRoute, and never (AllNavigation) make a route unreachable by id for
	// a contextual in-page link. internal/rendering's routeByID/labelByID resolve against
	// AllNavigation, and both metadata-hardcoding conformance gates depend on that.
	var primaryNavGroup string
	for _, g := range experience.GroupNavigation(navigation) {
		if g.Label != "" {
			primaryNavGroup = g.Label
			break
		}
	}
	homeRoute := domain.HomeCardRoute(navigation)
	allNavigation := navigation

	// show_nav absent means true -- an Application that says nothing about its menu keeps it.
	showNav := doc.ShowNav == nil || *doc.ShowNav
	if !showNav {
		navigation = nil
	}

	return &domain.Application{
		ID:              doc.ID,
		Name:            doc.Name,
		WorkspaceID:     workspaceID,
		Machines:        doc.Machines,
		Roles:           doc.Roles,
		Description:     doc.Description,
		Icon:            doc.Icon,
		Color:           doc.Color,
		SummaryMachine:  doc.SummaryMachine,
		ShowNav:         showNav,
		Navigation:      navigation,
		PrimaryNavGroup: primaryNavGroup,
		HomeRoute:       homeRoute,
		AllNavigation:   allNavigation,
	}, nil
}

func toNavigationItems(docs []navItemDoc) []domain.NavigationItem {
	var items []domain.NavigationItem
	for _, n := range docs {
		items = append(items, domain.NavigationItem{
			ID:       n.ID,
			Label:    n.Label,
			Route:    n.Route,
			Group:    n.Group,
			Priority: n.Priority,
			Badge:    n.Badge,
			HomeCard: n.HomeCard,
		})
	}
	return items
}

// validateSequencingModes closes the cross-Machine half of a sequencing declaration: mode_field
// lives on the parent Machine, and sequential_value must be a value that Field can actually hold.
// A typo in either would mean ordering silently never applies -- every step permanently unlocked,
// with no error anywhere, which is the failure this runtime least wants to be quiet about.
func validateSequencingModes(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}

	var issues []string
	for _, m := range machines {
		s := m.Sequencing
		if s == nil {
			continue
		}
		parentField, ok := m.FieldByID(s.ParentField)
		if !ok {
			continue // already reported by validateSequencing
		}
		parent, ok := byID[parentField.RelatedMachine]
		if !ok {
			issues = append(issues, fmt.Sprintf("machine %q: sequencing.parent_field %q points at machine %q, which this workspace does not declare", m.ID, s.ParentField, parentField.RelatedMachine))
			continue
		}
		modeField, ok := parent.FieldByID(s.ModeField)
		if !ok {
			issues = append(issues, fmt.Sprintf("machine %q: sequencing.mode_field %q is not a field of parent machine %q", m.ID, s.ModeField, parent.ID))
			continue
		}
		if len(modeField.Options) > 0 && !contains(modeField.Options, s.SequentialValue) {
			issues = append(issues, fmt.Sprintf("machine %q: sequencing.sequential_value %q is not one of parent field %q's options %v", m.ID, s.SequentialValue, s.ModeField, modeField.Options))
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
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
				issues = append(issues, fmt.Sprintf("machine %q: event %q: then.parent_field %q points at machine %q, which this workspace does not declare", m.ID, e.ID, r.ParentField, parentField.RelatedMachine))
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

// validateDatasetIDsAreUnique makes a Dataset id unique across every Machine loaded here, not
// just within its own Machine file -- which a single file cannot check for itself.
//
// This is what makes a Dataset addressable by id alone (composition.Loader.Dataset): a screen
// names ds_task_by_project and the runtime knows which Machine's records that means, because
// exactly one Machine can declare it. Without this the id would be ambiguous and every caller
// would have to keep naming a Machine id alongside it -- which is precisely the hardcoding this
// resolution exists to remove.
//
// Scope, decided 2026-09-20 while planning multi-Application support and **realized the same day**
// when Fase 3 made `applications:` a list: that uniqueness is **workspace-wide, not
// per-Application**. This validator and the three named below now genuinely receive the
// Workspace's whole Machine set (LoadApplication), where before the distinction was invisible
// because exactly one Application existed.
//
// The reasoning, so it isn't re-derived: Machines are shared between Applications (mch_user
// certainly, mch_activity likely), which makes per-Application scoping incoherent -- a shared
// Machine's own Dataset would live in two scopes at once, and per-Application ownership would
// force loading the same file twice and leave cross-Application relations either illegal or
// unchecked. So Machines stay workspace-level, loaded once and unique by id across the workspace,
// and an Application selects which of them it exposes plus its own navigation. The same scope
// applies to validateRelationTargets, validateRollupTargets and validateSequencingModes, all of
// which take the same flat Machine set for the same reason.
func validateDatasetIDsAreUnique(machines []*domain.Machine) error {
	owner := make(map[string]string)
	var issues []string
	for _, m := range machines {
		for _, ds := range m.Datasets {
			if prev, taken := owner[ds.ID]; taken {
				issues = append(issues, fmt.Sprintf("dataset id %q is declared by both machine %q and machine %q -- a dataset id must be unique across the workspace, since screens resolve it by id alone", ds.ID, prev, m.ID))
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
