package metadata

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

var (
	// workspaceSlugPattern is the manifest's own key: the slug of the Workspace it installs into
	// (metadata/workspaces/<slug>.yaml). Same shape internal/web.slugify produces when a Workspace
	// is created -- lowercase alphanumerics joined by single hyphens -- so a manifest can only
	// name a Workspace that could actually exist.
	workspaceSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
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

// workspaceDoc is one Workspace's installation manifest: which Workspace it is for, which Machines
// exist there, and which Applications are installed.
//
// Flat rather than nested under a `workspace:` block, which is what it was until 2026-09-22: that
// block held the Workspace's own id and name, and neither belongs in a file any more (see
// domain.Workspace.Slug). What is left is a single scalar naming the target, so a nesting level
// that once carried four keys would now carry one.
type workspaceDoc struct {
	// Workspace is the target Workspace's slug -- see domain.Workspace.Slug.
	Workspace string `yaml:"workspace"`
	// Machines are file paths, resolved relative to this manifest. Workspace-level and unique by
	// id: an Application *selects* from this set by id rather than owning files, because several
	// Applications genuinely share one (mch_user, mch_activity).
	Machines []string `yaml:"machines"`
	// Navigation is the Workspace's own menu. Empty in every manifest today -- the owner removed
	// it on 2026-09-21, since a Workspace's menu is derived from the Applications it contains --
	// but the runtime still reads it first, so an installation that does declare one still wins.
	Navigation []navItemDoc `yaml:"navigation"`
	// Applications are file paths, resolved relative to this manifest. One file per Application:
	// a navigation block alone runs to ~40 lines, so inlining several would bury the manifest's
	// own point, and it keeps one Application's menu changes off another's lines.
	//
	// An Application file names no Workspace of its own: it is a reusable declaration, and several
	// Workspaces may install the same one. Listing it here is what installs it -- and an empty
	// list is a perfectly valid Workspace, which is exactly what one looks like the moment it is
	// created through the UI.
	Applications []string `yaml:"applications"`
	// SuggestedApplications -- see domain.ApplicationSuggestion's own doc comment.
	SuggestedApplications []suggestedApplicationDoc `yaml:"suggested_applications"`
}

// suggestedApplicationDoc is the YAML serialization of a domain.ApplicationSuggestion.
type suggestedApplicationDoc struct {
	Label  string `yaml:"label"`
	Prompt string `yaml:"prompt"`
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
	// Workflow binds this Application to a runtime workflow engine and says which of its own
	// Machines plays each role -- see domain.Workflow. Optional: a plain CRUD Application declares
	// none, which is the normal case.
	Workflow *workflowDoc `yaml:"workflow"`
}

type workflowDoc struct {
	Engine string `yaml:"engine"`
	// Roles maps a role name the engine requires to a Machine id, e.g. `document: mch_document`.
	// A map rather than named keys per role, so an engine's cast is declared by
	// domain.KnownWorkflowEngines and validated against it, instead of every engine's roles
	// needing their own struct field here.
	Roles map[string]string `yaml:"roles"`
}

type navItemDoc struct {
	ID       string `yaml:"id"`
	Label    string `yaml:"label"`
	Route    string `yaml:"route"`
	Group    string `yaml:"group"`
	Priority int    `yaml:"priority"`
	Badge    string `yaml:"badge"`
	HomeCard bool   `yaml:"home_card"`
	// Title/Description -- see domain.NavigationItem. Both optional; Title falls back to Label.
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	// Icon -- see domain.NavigationItem.Icon. Optional.
	Icon string `yaml:"icon"`
	// SettingsHub/SettingsHubMember -- see domain.NavigationItem. Both optional; absent means an
	// ordinary navigation item, unchanged from before either field existed.
	SettingsHub       bool `yaml:"settings_hub"`
	SettingsHubMember bool `yaml:"settings_hub_member"`
}

// Workspaces is every installed Workspace manifest, keyed by the slug it names. A Workspace whose
// slug is absent from this map has no manifest, which is not an error: it has no Applications, no
// Machines, and renders an empty Home -- exactly the state a Workspace is in the moment it is
// created through the UI, before anyone installs anything into it.
type Workspaces map[string]*App

// LoadWorkspaces reads every Workspace manifest in dir, keyed by slug (2026-09-22).
//
// Scanning a directory, rather than reading an index file that lists the manifests, is what makes
// installation a file operation: dropping <slug>.yaml in here installs its Applications into that
// Workspace, and deleting it uninstalls them. An index would mean every install also edited a
// second file, and a Workspace whose manifest existed but went unlisted would be invisible for a
// reason nothing in its own file could explain.
//
// It replaced a single process-wide manifest. Until this, one file was loaded once at startup and
// handed to every request, so every Workspace showed the same Applications no matter which one the
// viewer was in -- a `workspaces` row scoped records and membership, but not what the Workspace
// *was*. That gap is what the owner saw on Dokter Kecil's own Home page: a brand-new, empty
// Workspace showing two Applications it had never installed.
//
// Two manifests naming the same slug is refused rather than resolved by filename order: both
// would claim to be that Workspace's installation, and picking one silently would make the
// Applications a Workspace has depend on how its files happen to sort.
func LoadWorkspaces(dir string) (Workspaces, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read workspace manifests %s: %w", dir, err)
	}

	loaded := Workspaces{}
	from := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml")) {
			continue
		}
		path := filepath.Join(dir, name)
		app, err := LoadApplication(path)
		if err != nil {
			return nil, err
		}
		slug := app.Workspace.Slug
		if earlier, taken := from[slug]; taken {
			return nil, &ValidationError{Issues: []string{fmt.Sprintf(
				"workspace %q is installed by two manifests (%s and %s) -- one Workspace has one installation",
				slug, earlier, path)}}
		}
		from[slug] = path
		loaded[slug] = app
	}
	return loaded, nil
}

// LoadApplicationFile reads and validates one Application file on its own, outside any Workspace --
// what the template library holds (metadata/applications/*.yaml) before anything installs it.
//
// It is loadApplicationFile with no Workspace slug, exported for internal/installer, which has to
// know what a template declares (its id, its card face, the Machines it claims, the workflow it
// binds) before it can plan a copy. Exported rather than letting that package parse the file itself:
// an Application's shape is declared once, here, and a second reader would be the two-lists drift
// this repo has already paid for.
//
// Every check loadApplicationFile makes runs, and every check it cannot make still cannot be made --
// summary_machine is verified against this Application's own machines:, but whether those Machines
// exist at all is a Workspace-level question (validateApplicationClaims), which is exactly what
// installer's own load-verify after the copy is for.
func LoadApplicationFile(path string) (*domain.Application, error) {
	return loadApplicationFile(path, "")
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

	var doc workspaceDoc
	if err := decodeStrict(data, &doc, "a workspace manifest"); err != nil {
		return nil, fmt.Errorf("parse workspace manifest %s: %w", path, err)
	}

	var issues []string
	if !workspaceSlugPattern.MatchString(doc.Workspace) {
		issues = append(issues, fmt.Sprintf("workspace slug %q must match %s", doc.Workspace, workspaceSlugPattern.String()))
	}
	if len(doc.Machines) == 0 {
		issues = append(issues, fmt.Sprintf("workspace %q: at least one machine is required", doc.Workspace))
	}
	// No "at least one application" check, deliberately, and this is the change that makes a
	// Workspace installable rather than born fully formed: a Workspace with nothing installed is
	// the normal state of one that was just created, not a broken manifest.
	for i, s := range doc.SuggestedApplications {
		if strings.TrimSpace(s.Label) == "" {
			issues = append(issues, fmt.Sprintf("workspace %q: suggested_applications[%d]: label is required", doc.Workspace, i))
		}
		if strings.TrimSpace(s.Prompt) == "" {
			issues = append(issues, fmt.Sprintf("workspace %q: suggested_applications[%d]: prompt is required", doc.Workspace, i))
		}
	}
	if len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}

	workspaceNav := toNavigationItems(doc.Navigation)
	if navIssues := validateNavigation(workspaceNav); len(navIssues) > 0 {
		return nil, &ValidationError{Issues: navIssues}
	}

	dir := filepath.Dir(path)
	suggestions := make([]domain.ApplicationSuggestion, 0, len(doc.SuggestedApplications))
	for _, s := range doc.SuggestedApplications {
		suggestions = append(suggestions, domain.ApplicationSuggestion{Label: s.Label, Prompt: s.Prompt})
	}
	app := &App{
		Workspace: domain.Workspace{
			Slug:                  doc.Workspace,
			Navigation:            workspaceNav,
			SuggestedApplications: suggestions,
		},
	}

	// Machines first, and once: every Application selects from this one set by id, so they must
	// exist before any Application is resolved against them.
	for _, rel := range doc.Machines {
		m, err := Load(filepath.Join(dir, rel))
		if err != nil {
			return nil, fmt.Errorf("workspace %q: machine %s: %w", doc.Workspace, rel, err)
		}
		app.Machines = append(app.Machines, m)
		app.Workspace.MachineIDs = append(app.Workspace.MachineIDs, m.ID)
	}
	if err := validateMachineIDsAreUnique(app.Machines); err != nil {
		return nil, err
	}

	for _, rel := range doc.Applications {
		application, err := loadApplicationFile(filepath.Join(dir, rel), doc.Workspace)
		if err != nil {
			return nil, err
		}
		app.Workspace.Applications = append(app.Workspace.Applications, *application)
	}
	if err := validateApplicationClaims(app.Workspace.Applications, app.Machines); err != nil {
		return nil, err
	}
	// Stamping comes straight after the claim check, and before any validator that reads it: a
	// Machine's ApplicationID is only unambiguous once "claimed by at most one Application" has
	// been established.
	stampApplicationIDs(app.Workspace.Applications, app.Machines)
	stampWorkflowRoles(app.Workspace.Applications, app.Machines)
	// After stamping, because it asks each cast Machine a question -- and before the Dataset validators
	// below, so "the engine needs this Dataset" is reported ahead of "this Dataset is malformed".
	if err := validateWorkflowDatasets(app.Workspace.Applications, app.Machines); err != nil {
		return nil, err
	}
	if err := validatePermissionRoles(app.Workspace.Applications, app.Machines); err != nil {
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
	if err := validateDatasetRelations(app.Machines); err != nil {
		return nil, err
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
	if err := validateCompositeTargets(app.Machines); err != nil {
		return nil, err
	}
	if err := validateSequencingModes(app.Machines); err != nil {
		return nil, err
	}

	// The Workspace carries its own loaded Machines, not only their ids: what a Machine id means
	// is a per-Workspace fact (domain.Workspace.Machines). Assigned here, once everything above
	// has finished building and stamping them, so the two can never be out of step.
	app.Workspace.Machines = app.Machines
	return app, nil
}

// loadApplicationFile reads and validates one Application's own file.
func loadApplicationFile(path, workspaceSlug string) (*domain.Application, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read application %s: %w", path, err)
	}
	var doc applicationDoc
	if err := decodeStrict(data, &doc, "an application file"); err != nil {
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
	if doc.Icon != "" && !domain.KnownIcons[doc.Icon] {
		issues = append(issues, fmt.Sprintf("application %q: icon %q is not a known icon -- see domain.KnownIcons", doc.ID, doc.Icon))
	}
	// summary_machine must be one of this Application's *own* machines. Pointing at another
	// Application's would put that Application's record count on this card -- the same misleading
	// number the declaration exists to prevent, just sourced differently.
	if doc.SummaryMachine != "" && !slices.Contains(doc.Machines, doc.SummaryMachine) {
		issues = append(issues, fmt.Sprintf("application %q: summary_machine %q is not one of this application's own machines -- a card must report its own count, not another application's", doc.ID, doc.SummaryMachine))
	}
	issues = append(issues, validateWorkflowBinding(doc)...)
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

	var workflow *domain.Workflow
	if doc.Workflow != nil {
		roles := make(map[string]string, len(doc.Workflow.Roles))
		for role, machineID := range doc.Workflow.Roles {
			roles[role] = machineID
		}
		workflow = &domain.Workflow{Engine: doc.Workflow.Engine, Roles: roles}
	}

	return &domain.Application{
		ID:              doc.ID,
		Name:            doc.Name,
		WorkspaceSlug:   workspaceSlug,
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
		Workflow:        workflow,
	}, nil
}

// validateWorkflowBinding closes every way a `workflow:` block could name something the runtime
// cannot realize. Each of these failures is silent at runtime if it loads, which is why they are
// all load-time errors (005 Phase 3: invalid metadata must not enter execution):
//
//   - an unknown engine would bind to nothing, and the Application would render its screens while
//     no approval mechanics ever engaged;
//   - a missing *required* role would leave the engine half-cast, failing on the first request
//     rather than at load (an optional one is a legitimate smaller installation -- see
//     domain.WorkflowEngineSpec for where that distinction is drawn);
//   - an unknown role name is almost always a misspelled real one, which without this check reports
//     as "missing" *and* leaves the typo unexplained;
//   - a role naming a Machine this Application does not claim would hand the engine records
//     belonging to another Application, past the claim boundary Machine.ApplicationID establishes;
//   - two roles naming one Machine would make two of the engine's predicates true for it, and every
//     caller that branches on them would take whichever branch it happens to test first;
//   - half a saved approval flow (a template Machine with no template-step Machine, or the reverse)
//     is the one pair whose halves are useless apart -- CAP-V28 writes both records or neither, so a
//     one-sided cast would be a feature that silently stores nothing.
func validateWorkflowBinding(doc applicationDoc) []string {
	w := doc.Workflow
	if w == nil {
		return nil
	}

	var issues []string
	spec, known := domain.KnownWorkflowEngines[w.Engine]
	if !known {
		return []string{fmt.Sprintf("application %q: workflow.engine %q is not an engine this runtime realizes -- see domain.KnownWorkflowEngines", doc.ID, w.Engine)}
	}
	for _, role := range spec.Required {
		if strings.TrimSpace(w.Roles[role]) == "" {
			issues = append(issues, fmt.Sprintf("application %q: workflow engine %q requires a machine for role %q -- an engine cannot run against a partial cast", doc.ID, w.Engine, role))
		}
	}
	byMachine := make(map[string]string, len(w.Roles))
	for _, role := range slices.Sorted(maps.Keys(w.Roles)) {
		machineID := w.Roles[role]
		if !slices.Contains(spec.Roles(), role) {
			issues = append(issues, fmt.Sprintf("application %q: workflow engine %q declares no role %q -- its roles are %v", doc.ID, w.Engine, role, spec.Roles()))
			continue
		}
		if !slices.Contains(doc.Machines, machineID) {
			issues = append(issues, fmt.Sprintf("application %q: workflow role %q names machine %q, which is not one of this application's own machines -- an engine may only act on the records its application claims", doc.ID, role, machineID))
		}
		if earlier, taken := byMachine[machineID]; taken {
			issues = append(issues, fmt.Sprintf("application %q: machine %q is declared for both workflow roles %q and %q -- one Machine plays one role", doc.ID, machineID, earlier, role))
			continue
		}
		byMachine[machineID] = role
	}
	if template, step := w.Roles[domain.WorkflowRoleFlowTemplate] != "", w.Roles[domain.WorkflowRoleFlowTemplateStep] != ""; template != step {
		issues = append(issues, fmt.Sprintf("application %q: workflow roles %q and %q are optional but inseparable -- a saved approval flow is a template *and* its steps, so casting one without the other declares a feature that stores nothing", doc.ID, domain.WorkflowRoleFlowTemplate, domain.WorkflowRoleFlowTemplateStep))
	}
	return issues
}

// validateWorkflowDatasets closes the gap that shipped a 500 to two Workspaces for a day: an engine
// needs a Dataset to select through, the id is named from Go, and nothing made the Machine cast in that
// role declare it.
//
// Tahap A (2026-09-29) replaced three hand-written Document-to-Step index loops with one declared
// Relation, `ds_documents_with_steps`, and added it to the template library plus `default`'s own copy.
// An install *copies*, so that never reached `hanomerch` or `dokter-kecil`, both of which cast the
// engine's roles -- and composition.selectRecords answers a missing Dataset with an error, so every
// approval screen in both returned 500 unconditionally. Nothing was red: the handler was registered,
// the YAML was valid, and the requirement existed only inside a Go constant.
//
// So this runs where the *Machines* are visible, after stampWorkflowRoles, rather than inside
// validateWorkflowBinding, which sees only one applicationDoc. It asks the engine what each role owes
// (domain.WorkflowEngineSpec.Datasets) and asks the Machine cast in that role whether it declares it --
// so a Workspace whose Machines were renamed on install is checked by *role*, never by file or id. That
// distinction is not theoretical: dokter-kecil's `document` role is `mch_document_approval`, while its
// own unrelated `mch_document` is the unbound Machine that has panicked two pages before.
//
// A load error rather than a per-request one, the same reason a missing Required role is: an engine that
// cannot select its own records cannot run, and refusing to start beats serving a screen that throws.
func validateWorkflowDatasets(applications []domain.Application, machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}
	var issues []string
	for _, app := range applications {
		if app.Workflow == nil {
			continue
		}
		spec, known := domain.KnownWorkflowEngines[app.Workflow.Engine]
		if !known {
			continue // validateWorkflowBinding already reported this
		}
		for _, role := range slices.Sorted(maps.Keys(spec.Datasets)) {
			machineID := app.Workflow.MachineForRole(role)
			if machineID == "" {
				continue // an uncast optional role owes nothing
			}
			m := byID[machineID]
			if m == nil {
				continue // validateWorkflowBinding already reported this
			}
			for _, want := range spec.DatasetsFor(role) {
				if !slices.ContainsFunc(m.Datasets, func(ds domain.Dataset) bool { return ds.ID == want }) {
					issues = append(issues, fmt.Sprintf("application %q: workflow engine %q selects through dataset %q, which machine %q (its %q role) does not declare -- the engine's own screens would fail at request time, not here. Copy the dataset from the template library's own machine file",
						app.ID, app.Workflow.Engine, want, machineID, role))
				}
			}
		}
	}
	if len(issues) > 0 {
		return fmt.Errorf("%s", strings.Join(issues, "; "))
	}
	return nil
}

func toNavigationItems(docs []navItemDoc) []domain.NavigationItem {
	var items []domain.NavigationItem
	for _, n := range docs {
		items = append(items, domain.NavigationItem{
			ID:                n.ID,
			Label:             n.Label,
			Route:             n.Route,
			Group:             n.Group,
			Priority:          n.Priority,
			Badge:             n.Badge,
			HomeCard:          n.HomeCard,
			Icon:              n.Icon,
			Title:             n.Title,
			Description:       n.Description,
			SettingsHub:       n.SettingsHub,
			SettingsHubMember: n.SettingsHubMember,
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

// validateCompositeTargets is composite_signed_document's cross-Machine half, the exact shape
// validateRollupTargets already has and for the identical reason: source_field and target_field live on
// the *parent*, so whether they exist -- and whether they can hold a file at all -- can only be checked
// once every Machine is loaded.
//
// Without it, a composite naming a Field the parent does not have would run, produce a correct PDF, and
// store its key under a name nothing reads: the signed document would exist on disk and be unreachable
// from every screen, with no error anywhere.
func validateCompositeTargets(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}

	var issues []string
	for _, m := range machines {
		for _, e := range m.Events {
			if e.Then.Composite == nil {
				continue
			}
			c := *e.Then.Composite
			parentField, ok := m.FieldByID(c.ParentField)
			if !ok {
				continue // already reported by validateComposite
			}
			parent, ok := byID[parentField.RelatedMachine]
			if !ok {
				issues = append(issues, fmt.Sprintf("machine %q: event %q: then.parent_field %q points at machine %q, which this workspace does not declare", m.ID, e.ID, c.ParentField, parentField.RelatedMachine))
				continue
			}
			for _, f := range []struct{ key, value string }{{"source_field", c.SourceField}, {"target_field", c.TargetField}} {
				target, ok := parent.FieldByID(f.value)
				if !ok {
					issues = append(issues, fmt.Sprintf("machine %q: event %q: then.%s %q is not a field of parent machine %q", m.ID, e.ID, f.key, f.value, parent.ID))
					continue
				}
				if target.Type != domain.FieldTypeFile {
					issues = append(issues, fmt.Sprintf("machine %q: event %q: then.%s %q is a %q field on %q -- compositing reads and writes an uploaded document, so both must be file fields", m.ID, e.ID, f.key, f.value, target.Type, parent.ID))
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
// validateDatasetRelations is the Workspace-level half of a Dataset's `relations:` check: whether the
// Machine it names exists, and whether `via:` really is a reference Field on that Machine pointing back
// at the declaring one.
//
// It lives here rather than in validateDataset for the same reason validateRelationTargets below does:
// a Machine file is validated alone and cannot see its siblings, while this question is about two
// Machines. And it is the check that makes 007 §7.5's "reuse existing Machine reference semantics
// rather than inventing a second relationship identity" enforceable -- without it, `relations:` would
// be free to describe an association no Field expresses, which is precisely a second identity.
func validateDatasetRelations(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}

	var issues []string
	for _, m := range machines {
		for _, ds := range m.Datasets {
			for _, rel := range ds.Relations {
				child, ok := byID[rel.Machine]
				if !ok {
					issues = append(issues, fmt.Sprintf("machine %q dataset %q relation %q: machine %q is not installed in this workspace", m.ID, ds.ID, rel.ID, rel.Machine))
					continue
				}
				f, ok := child.FieldByID(rel.Via)
				if !ok {
					issues = append(issues, fmt.Sprintf("machine %q dataset %q relation %q: %q has no field %q", m.ID, ds.ID, rel.ID, rel.Machine, rel.Via))
					continue
				}
				if !f.IsReference() {
					issues = append(issues, fmt.Sprintf("machine %q dataset %q relation %q: %s.%s is a %s field, not a reference -- a relation reuses an existing reference field (007 §7.5) rather than describing an association of its own", m.ID, ds.ID, rel.ID, rel.Machine, rel.Via, f.Type))
					continue
				}
				if f.RelatedMachine != m.ID {
					issues = append(issues, fmt.Sprintf("machine %q dataset %q relation %q: %s.%s references %q, not %q -- a relation follows a field that points back at the declaring machine", m.ID, ds.ID, rel.ID, rel.Machine, rel.Via, f.RelatedMachine, m.ID))
				}
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

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
