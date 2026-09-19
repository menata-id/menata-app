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
	permissionIDPattern = regexp.MustCompile(`^prm_[a-z][a-z0-9_]*$`)
	navItemIDPattern    = regexp.MustCompile(`^nav_[a-z][a-z0-9_]*$`)
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
		if f.Type == domain.FieldTypeStatus && f.Default != nil && !contains(f.Options, f.Default.(string)) {
			issues = append(issues, fmt.Sprintf("field %q: default %q is not one of its own options %v", f.ID, f.Default, f.Options))
		}
	}

	for _, c := range m.Constraints {
		issues = append(issues, validateConstraint(m, c, fieldsByID)...)
	}

	seenPermissions := make(map[string]bool, len(m.Permissions))
	for _, p := range m.Permissions {
		issues = append(issues, validatePermission(m, p, fieldsByID, seenPermissions)...)
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
	} else if onField.Type == domain.FieldTypeStatus && !contains(onField.Options, c.WhenEquals) {
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

func contains(options []string, v string) bool {
	for _, o := range options {
		if o == v {
			return true
		}
	}
	return false
}
