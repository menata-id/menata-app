package metadata

import (
	"fmt"
	"regexp"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// machineIDPattern, fieldIDPattern, and constraintIDPattern enforce 004-runtime-metadata.md
// "Stable Identity": identity must survive label/presentation/implementation changes, so it is
// validated independently of Name.
var (
	machineIDPattern    = regexp.MustCompile(`^mch_[a-z][a-z0-9_]*$`)
	fieldIDPattern      = regexp.MustCompile(`^fld_[a-z][a-z0-9_]*$`)
	constraintIDPattern = regexp.MustCompile(`^cst_[a-z][a-z0-9_]*$`)
	eventIDPattern      = regexp.MustCompile(`^evt_[a-z][a-z0-9_]*$`)
	permissionIDPattern = regexp.MustCompile(`^prm_[a-z][a-z0-9_]*$`)
	navItemIDPattern    = regexp.MustCompile(`^nav_[a-z][a-z0-9_]*$`)
	datasetIDPattern    = regexp.MustCompile(`^ds_[a-z][a-z0-9_]*$`)
	measureIDPattern    = regexp.MustCompile(`^msr_[a-z][a-z0-9_]*$`)
)

// validateNavigation checks the Application's own navigation: list (ROADMAP.md's "Navigation is
// code, not metadata" gap, closed by making the topbar a projection of this list). Unlike a
// Machine's own declarations, a nav item's Route can't be cross-checked against anything else in
// metadata -- most real routes are bespoke internal/web handlers, not generated from a Machine --
// so this validates shape only: identity, that a route is named, and that Badge (if present) is
// one the runtime actually realizes.
func validateNavigation(items []domain.NavigationItem) []string {
	var issues []string
	seen := make(map[string]bool, len(items))
	homeCardID := ""
	for _, n := range items {
		if !navItemIDPattern.MatchString(n.ID) {
			issues = append(issues, fmt.Sprintf("navigation item id %q must match %s", n.ID, navItemIDPattern.String()))
			continue
		}
		if seen[n.ID] {
			issues = append(issues, fmt.Sprintf("navigation item id %q is declared more than once", n.ID))
		}
		seen[n.ID] = true

		if n.Label == "" {
			issues = append(issues, fmt.Sprintf("navigation item %q: label is required", n.ID))
		}
		if !strings.HasPrefix(n.Route, "/") {
			issues = append(issues, fmt.Sprintf("navigation item %q: route %q must start with \"/\"", n.ID, n.Route))
		}
		if n.Badge != "" && !domain.KnownNavigationBadges[n.Badge] {
			issues = append(issues, fmt.Sprintf("navigation item %q: unknown badge %q", n.ID, n.Badge))
		}
		if n.HomeCard {
			if homeCardID != "" {
				issues = append(issues, fmt.Sprintf("navigation item %q: home_card is already set on %q -- at most one item may claim it", n.ID, homeCardID))
			}
			homeCardID = n.ID
		}
	}
	return issues
}

// applyHiddenNavGroups drops every item whose group: is named in hidden -- the metadata-only way
// to keep a group's destinations declared (still valid routes, still reachable by a contextual
// in-page link, e.g. workspacehome.templ's own "Approval Inbox" / "+ New Approval" links) while
// removing them from the topbar itself. Each hidden name is cross-checked against the groups that
// actually exist in items, the same posture validateNavigation already applies to Badge -- a
// typo here would otherwise silently hide nothing rather than fail loudly.
func applyHiddenNavGroups(items []domain.NavigationItem, hidden []string) ([]domain.NavigationItem, []string) {
	if len(hidden) == 0 {
		return items, nil
	}

	knownGroups := make(map[string]bool)
	for _, n := range items {
		if n.Group != "" {
			knownGroups[n.Group] = true
		}
	}

	var issues []string
	hiddenSet := make(map[string]bool, len(hidden))
	for _, g := range hidden {
		if !knownGroups[g] {
			issues = append(issues, fmt.Sprintf("hidden_nav_groups entry %q does not match any navigation item's group", g))
			continue
		}
		hiddenSet[g] = true
	}
	if len(issues) > 0 {
		return nil, issues
	}

	visible := make([]domain.NavigationItem, 0, len(items))
	for _, n := range items {
		if n.Group != "" && hiddenSet[n.Group] {
			continue
		}
		visible = append(visible, n)
	}
	return visible, nil
}

// ValidationError aggregates every problem found in one metadata document, per 005-runtime-
// lifecycle.md Phase 3: invalid metadata must not enter executable planning, and a metadata
// author should see every problem at once rather than one failure per fix-and-rerun cycle.
type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid metadata:\n  - %s", strings.Join(e.Issues, "\n  - "))
}

// Validate checks a parsed Machine against the Domain Plane's structural rules: stable
// identity, known field types, and no duplicate field identity within the Machine.
func Validate(m *domain.Machine) error {
	var issues []string

	if !machineIDPattern.MatchString(m.ID) {
		issues = append(issues, fmt.Sprintf("machine id %q must match %s", m.ID, machineIDPattern.String()))
	}
	if m.Name == "" {
		issues = append(issues, fmt.Sprintf("machine %q: name is required", m.ID))
	}

	seen := make(map[string]bool, len(m.Fields))
	fieldsByID := make(map[string]domain.Field, len(m.Fields))
	for _, f := range m.Fields {
		if !fieldIDPattern.MatchString(f.ID) {
			issues = append(issues, fmt.Sprintf("field id %q must match %s", f.ID, fieldIDPattern.String()))
			continue
		}
		if seen[f.ID] {
			issues = append(issues, fmt.Sprintf("field id %q is declared more than once", f.ID))
		}
		seen[f.ID] = true
		fieldsByID[f.ID] = f

		if !domain.KnownFieldTypes[f.Type] {
			issues = append(issues, fmt.Sprintf("field %q: unknown type %q", f.ID, f.Type))
		}
		if f.Type == domain.FieldTypeStatus && len(f.Options) == 0 {
			issues = append(issues, fmt.Sprintf("field %q: type status requires at least one option", f.ID))
		}
		if f.Type == domain.FieldTypeRelation && !machineIDPattern.MatchString(f.RelatedMachine) {
			issues = append(issues, fmt.Sprintf("field %q: type relation requires a valid target machine id, got %q", f.ID, f.RelatedMachine))
		}
		if f.Default != nil && violatesOptions(f, fmt.Sprint(f.Default)) {
			issues = append(issues, fmt.Sprintf("field %q: default %q is not one of its own options %v", f.ID, f.Default, f.Options))
		}
	}

	for _, c := range m.Constraints {
		issues = append(issues, validateConstraint(m, c, fieldsByID)...)
	}

	seenEvents := make(map[string]bool, len(m.Events))
	for _, e := range m.Events {
		issues = append(issues, validateEvent(m, e, fieldsByID, seenEvents)...)
	}

	seenPermissions := make(map[string]bool, len(m.Permissions))
	for _, p := range m.Permissions {
		issues = append(issues, validatePermission(m, p, fieldsByID, seenPermissions)...)
	}

	seenDatasets := make(map[string]bool, len(m.Datasets))
	for _, ds := range m.Datasets {
		issues = append(issues, validateDataset(m, ds, fieldsByID, seenDatasets)...)
	}

	if m.Sequencing != nil {
		issues = append(issues, validateSequencing(m, *m.Sequencing, fieldsByID)...)
	}

	if m.View.Layout != "" && !domain.KnownLayouts[m.View.Layout] {
		issues = append(issues, fmt.Sprintf("machine %q: view.layout %q is not a known layout", m.ID, m.View.Layout))
	}
	if m.View.EffectiveLayout() == domain.LayoutBoard {
		if _, ok := fieldsByID[m.View.GroupBy]; !ok {
			issues = append(issues, fmt.Sprintf("machine %q: view.group_by %q is not a field of this machine", m.ID, m.View.GroupBy))
		}
	}
	if m.View.SLAField != "" {
		f, ok := fieldsByID[m.View.SLAField]
		if !ok {
			issues = append(issues, fmt.Sprintf("machine %q: view.sla_field %q is not a field of this machine", m.ID, m.View.SLAField))
		} else if f.Type != domain.FieldTypeDate {
			issues = append(issues, fmt.Sprintf("machine %q: view.sla_field %q must be a date field, got %q", m.ID, m.View.SLAField, f.Type))
		}
	}
	for _, cf := range m.View.CardFields {
		if _, ok := fieldsByID[cf.Field]; !ok {
			issues = append(issues, fmt.Sprintf("machine %q: view.card_fields entry %q is not a field of this machine", m.ID, cf.Field))
		}
		if !domain.KnownCardFieldRoles[cf.Role] {
			issues = append(issues, fmt.Sprintf("machine %q: view.card_fields entry %q has unknown role %q", m.ID, cf.Field, cf.Role))
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateConstraint checks one Constraint's own shape: everything a single Machine file can
// verify on its own. Cross-Machine checks (does the related Machine/field actually exist) happen
// once the whole Application is loaded (application.go's validateConstraintTargets), the same
// split already used for relation fields.
func validateConstraint(m *domain.Machine, c domain.Constraint, fieldsByID map[string]domain.Field) []string {
	var issues []string

	if !constraintIDPattern.MatchString(c.ID) {
		issues = append(issues, fmt.Sprintf("constraint id %q must match %s", c.ID, constraintIDPattern.String()))
	}

	onField, onExists := fieldsByID[c.On]
	if !onExists {
		issues = append(issues, fmt.Sprintf("constraint %q: on %q is not a field of machine %q", c.ID, c.On, m.ID))
	} else if violatesOptions(onField, c.WhenEquals) {
		issues = append(issues, fmt.Sprintf("constraint %q: when_equals %q is not one of field %q's options %v", c.ID, c.WhenEquals, c.On, onField.Options))
	}
	if c.WhenEquals == "" {
		issues = append(issues, fmt.Sprintf("constraint %q: when_equals is required", c.ID))
	}

	if !machineIDPattern.MatchString(c.BlockIf.RelatedMachine) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.related_machine %q must match %s", c.ID, c.BlockIf.RelatedMachine, machineIDPattern.String()))
	}
	if !fieldIDPattern.MatchString(c.BlockIf.RelatedField) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.related_field %q must match %s", c.ID, c.BlockIf.RelatedField, fieldIDPattern.String()))
	}
	if !fieldIDPattern.MatchString(c.BlockIf.Condition.Field) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.field %q must match %s", c.ID, c.BlockIf.Condition.Field, fieldIDPattern.String()))
	}
	if !expression.KnownOps[c.BlockIf.Condition.Op] {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.op %q is not a known operator", c.ID, c.BlockIf.Condition.Op))
	}
	if c.BlockIf.Condition.Value == "" {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.value is required", c.ID))
	}

	return issues
}

// validateEvent checks one Event's own shape, mirroring validateConstraint: everything it needs
// is on the Machine itself, so there is no cross-Machine pass the way Constraint's block_if
// needs. Unlike Constraint's when_equals, Event's WhenEquals is optional -- empty means "any
// change fires it," a deliberate difference (Constraint gates a specific transition; Event
// merely observes one). OnCreate is a second, mutually exclusive shape (domain.Event's own doc
// comment): exactly one of On or OnCreate must be set, and WhenEquals is meaningless with
// OnCreate (no prior value exists to compare against).
func validateEvent(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field, seen map[string]bool) []string {
	var issues []string

	if !eventIDPattern.MatchString(e.ID) {
		issues = append(issues, fmt.Sprintf("event id %q must match %s", e.ID, eventIDPattern.String()))
	}
	if seen[e.ID] {
		issues = append(issues, fmt.Sprintf("event id %q is declared more than once", e.ID))
	}
	seen[e.ID] = true

	switch {
	case e.OnCreate && e.On != "":
		issues = append(issues, fmt.Sprintf("event %q: on_create and on must not both be set", e.ID))
	case e.OnCreate && e.WhenEquals != "":
		issues = append(issues, fmt.Sprintf("event %q: when_equals is not meaningful with on_create (no prior value to compare)", e.ID))
	case !e.OnCreate && e.On == "":
		issues = append(issues, fmt.Sprintf("event %q: exactly one of on or on_create is required", e.ID))
	case !e.OnCreate:
		onField, onExists := fieldsByID[e.On]
		if !onExists {
			issues = append(issues, fmt.Sprintf("event %q: on %q is not a field of machine %q", e.ID, e.On, m.ID))
		} else if e.WhenEquals != "" && violatesOptions(onField, e.WhenEquals) {
			issues = append(issues, fmt.Sprintf("event %q: when_equals %q is not one of field %q's options %v", e.ID, e.WhenEquals, e.On, onField.Options))
		}
	}

	// Each Service owns its own required keys: summary is log_activity's message, and means
	// nothing to a rollup, which writes a field rather than a sentence.
	switch e.Then.Name {
	case domain.ServiceLogActivity:
		if e.Then.Summary == "" {
			issues = append(issues, fmt.Sprintf("event %q: then.summary is required", e.ID))
		}
		if (e.Then.SummaryOverrideWhen == "") != (e.Then.SummaryOverride == "") {
			issues = append(issues, fmt.Sprintf("event %q: then.summary_override_when and then.summary_override must be set together or not at all", e.ID))
		}
	case domain.ServiceRollupParentStatus:
		issues = append(issues, validateRollup(m, e, fieldsByID)...)
	default:
		issues = append(issues, fmt.Sprintf("event %q: then.service %q is not a service this runtime realizes", e.ID, e.Then.Name))
	}

	return issues
}

// validateRollup checks everything a rollup declaration can be checked against from inside its own
// Machine file: the parent reference it writes through, and that the child values it watches for
// are really values the watched Field can hold. The other half -- that target_field exists on the
// *parent* Machine and that set/default are among its options -- needs both Machines loaded, so it
// lives in application.go beside validateRelationTargets.
func validateRollup(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field) []string {
	var issues []string

	if e.Then.Rollup == nil {
		return append(issues, fmt.Sprintf("event %q: then.service %q requires parent_field/target_field", e.ID, e.Then.Name))
	}
	r := *e.Then.Rollup

	parentField, ok := fieldsByID[r.ParentField]
	if !ok {
		issues = append(issues, fmt.Sprintf("event %q: then.parent_field %q is not a field of machine %q", e.ID, r.ParentField, m.ID))
	} else if !parentField.IsReference() {
		issues = append(issues, fmt.Sprintf("event %q: then.parent_field %q must reference the parent machine (a relation or person field), got %q", e.ID, r.ParentField, parentField.Type))
	}
	if !fieldIDPattern.MatchString(r.TargetField) {
		issues = append(issues, fmt.Sprintf("event %q: then.target_field %q must match %s", e.ID, r.TargetField, fieldIDPattern.String()))
	}
	if r.Default == "" {
		issues = append(issues, fmt.Sprintf("event %q: then.default is required -- it is what the parent becomes when neither rule fires, including when it has no children yet", e.ID))
	}
	if r.AnyValue == "" && r.AllValue == "" {
		issues = append(issues, fmt.Sprintf("event %q: a rollup declaring neither any nor all can only ever write then.default", e.ID))
	}

	// The values watched for are values of the Event's own on: field -- that is the field whose
	// change triggers this rollup, and whose value every sibling is then read for.
	if onField, onExists := fieldsByID[e.On]; onExists {
		for _, v := range []struct{ key, value string }{{"any.value", r.AnyValue}, {"all.value", r.AllValue}} {
			if v.value != "" && violatesOptions(onField, v.value) {
				issues = append(issues, fmt.Sprintf("event %q: then.%s %q is not one of field %q's options %v", e.ID, v.key, v.value, e.On, onField.Options))
			}
		}
	}

	return issues
}

// validatePermission checks one Permission's own shape (ROADMAP.md Phase 16). Everything it
// needs is on the Machine itself, so unlike a Constraint there is no second, cross-Machine pass:
// the Action must be one the runtime actually realizes, and actor_field must be a reference Field
// on this same Machine -- a Permission whose actor can never be resolved would silently protect
// nothing, which is worse than no Permission at all.
func validatePermission(m *domain.Machine, p domain.Permission, fieldsByID map[string]domain.Field, seen map[string]bool) []string {
	var issues []string

	if !permissionIDPattern.MatchString(p.ID) {
		issues = append(issues, fmt.Sprintf("permission id %q must match %s", p.ID, permissionIDPattern.String()))
	}
	if seen[p.ID] {
		issues = append(issues, fmt.Sprintf("permission id %q is declared more than once", p.ID))
	}
	seen[p.ID] = true

	if !domain.KnownActions[p.Action] {
		issues = append(issues, fmt.Sprintf("permission %q: action %q is not an action this runtime realizes", p.ID, p.Action))
	}

	actorField, ok := fieldsByID[p.ActorField]
	if !ok {
		issues = append(issues, fmt.Sprintf("permission %q: actor_field %q is not a field of machine %q", p.ID, p.ActorField, m.ID))
	} else if !actorField.IsReference() {
		issues = append(issues, fmt.Sprintf("permission %q: actor_field %q must reference an identity (a person or relation field), got %q", p.ID, p.ActorField, actorField.Type))
	}

	return issues
}

// validateDataset checks one Dataset's own shape, mirroring validateConstraint/validateEvent:
// everything it names lives on this same Machine, so there is no cross-Machine pass (a Dataset
// spanning sources is not expressible today -- domain.Dataset's own doc comment says why).
//
// The two aggregate-specific rules are enforced in both directions on purpose: sum without a
// field has nothing to add up, and count *with* a field reads as though the field changed what is
// counted when it does not. Both would be silent nonsense at render time -- an empty number on a
// page -- rather than an error, which is exactly the class of metadata typo Validate exists to
// turn into a startup failure (005 Phase 3-4).
func validateDataset(m *domain.Machine, ds domain.Dataset, fieldsByID map[string]domain.Field, seen map[string]bool) []string {
	var issues []string

	if !datasetIDPattern.MatchString(ds.ID) {
		issues = append(issues, fmt.Sprintf("dataset id %q must match %s", ds.ID, datasetIDPattern.String()))
	}
	if seen[ds.ID] {
		issues = append(issues, fmt.Sprintf("dataset id %q is declared more than once", ds.ID))
	}
	seen[ds.ID] = true

	if ds.Dimension != "" {
		if _, ok := fieldsByID[ds.Dimension]; !ok {
			issues = append(issues, fmt.Sprintf("dataset %q: dimension %q is not a field of machine %q", ds.ID, ds.Dimension, m.ID))
		}
	}

	if len(ds.Measures) == 0 {
		issues = append(issues, fmt.Sprintf("dataset %q: at least one measure is required", ds.ID))
	}

	seenMeasures := make(map[string]bool, len(ds.Measures))
	for _, ms := range ds.Measures {
		if !measureIDPattern.MatchString(ms.ID) {
			issues = append(issues, fmt.Sprintf("dataset %q: measure id %q must match %s", ds.ID, ms.ID, measureIDPattern.String()))
		}
		if seenMeasures[ms.ID] {
			issues = append(issues, fmt.Sprintf("dataset %q: measure id %q is declared more than once", ds.ID, ms.ID))
		}
		seenMeasures[ms.ID] = true

		if !domain.KnownAggregates[ms.Aggregate] {
			issues = append(issues, fmt.Sprintf("dataset %q: measure %q has unknown aggregate %q", ds.ID, ms.ID, ms.Aggregate))
		}

		switch ms.Aggregate {
		case domain.AggregateSum:
			f, ok := fieldsByID[ms.Field]
			if !ok {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q is a sum, so field %q must be a field of machine %q", ds.ID, ms.ID, ms.Field, m.ID))
			} else if f.Type != domain.FieldTypeNumber {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q sums field %q, which must be a number field, got %q", ds.ID, ms.ID, ms.Field, f.Type))
			}
		case domain.AggregateCount:
			if ms.Field != "" {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q is a count, which counts records, so field %q must not be set", ds.ID, ms.ID, ms.Field))
			}
		}

		if ms.Where != nil {
			whereField, ok := fieldsByID[ms.Where.Field]
			if !ok {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.field %q is not a field of machine %q", ds.ID, ms.ID, ms.Where.Field, m.ID))
			} else if violatesOptions(whereField, ms.Where.Value) {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.value %q is not one of field %q's options %v", ds.ID, ms.ID, ms.Where.Value, ms.Where.Field, whereField.Options))
			}
			if !expression.KnownOps[ms.Where.Op] {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.op %q is not a known operator", ds.ID, ms.ID, ms.Where.Op))
			}
		}
	}

	return issues
}

// validateSequencing checks the half of a sequencing declaration this Machine can verify alone:
// the Fields it orders and reads state from are its own, the value that means "still open" is one
// that Field can actually hold, and the parent reference exists. mode_field and sequential_value
// live on the *parent* Machine, so they are checked in application.go once every Machine is
// loaded -- the same split validateRollup/validateRollupTargets already uses.
func validateSequencing(m *domain.Machine, s domain.Sequencing, fieldsByID map[string]domain.Field) []string {
	var issues []string

	orderField, ok := fieldsByID[s.OrderField]
	if !ok {
		issues = append(issues, fmt.Sprintf("machine %q: sequencing.order_field %q is not a field of this machine", m.ID, s.OrderField))
	} else if orderField.Type != domain.FieldTypeNumber {
		issues = append(issues, fmt.Sprintf("machine %q: sequencing.order_field %q must be a number field to order siblings by, got %q", m.ID, s.OrderField, orderField.Type))
	}

	stateField, ok := fieldsByID[s.StateField]
	if !ok {
		issues = append(issues, fmt.Sprintf("machine %q: sequencing.state_field %q is not a field of this machine", m.ID, s.StateField))
	} else if violatesOptions(stateField, s.OpenValue) {
		issues = append(issues, fmt.Sprintf("machine %q: sequencing.open_value %q is not one of field %q's options %v", m.ID, s.OpenValue, s.StateField, stateField.Options))
	}
	if s.OpenValue == "" {
		issues = append(issues, fmt.Sprintf("machine %q: sequencing.open_value is required -- without it nothing would ever count as still open, and ordering would never lock anything", m.ID))
	}

	parentField, ok := fieldsByID[s.ParentField]
	if !ok {
		issues = append(issues, fmt.Sprintf("machine %q: sequencing.parent_field %q is not a field of this machine", m.ID, s.ParentField))
	} else if !parentField.IsReference() {
		issues = append(issues, fmt.Sprintf("machine %q: sequencing.parent_field %q must reference the machine whose children are ordered together, got %q", m.ID, s.ParentField, parentField.Type))
	}

	if s.SequentialValue == "" {
		issues = append(issues, fmt.Sprintf("machine %q: sequencing.sequential_value is required -- it is the one mode value that turns ordering on", m.ID))
	}

	return issues
}

// violatesOptions reports whether f constrains its values to a declared option list and value is
// not one of them.
//
// The question is deliberately asked of the *property* (does this Field declare options?) rather
// than of the type (is this Field a status?). Those happen to be the same set today only because
// validateField below requires a status to declare options and nothing else declares any -- a
// coincidence held up by one rule, not a property of the model. Branching on the type would make
// `status` a fused name meaning "text that has options", the way a `string_cap_header` type would
// fuse a string with its capitalisation instead of letting capitalisation be a property of
// string. domain.Field.IsReference() already draws this line correctly for relation/person, and
// its own doc comment says why: so a future reference-shaped type doesn't need adding in five
// places at once. This is the same fix for options.
func violatesOptions(f domain.Field, value string) bool {
	return len(f.Options) > 0 && !contains(f.Options, value)
}

func contains(options []string, v string) bool {
	for _, o := range options {
		if o == v {
			return true
		}
	}
	return false
}
