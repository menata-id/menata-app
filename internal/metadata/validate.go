package metadata

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/registry"
)

// machineIDPattern and constraintIDPattern enforce 004-runtime-metadata.md
// "Stable Identity": identity must survive label/presentation/implementation changes, so it is
// validated independently of Name.
var (
	machineIDPattern     = regexp.MustCompile(`^mch_[a-z][a-z0-9_]*$`)
	constraintIDPattern  = regexp.MustCompile(`^cst_[a-z][a-z0-9_]*$`)
	eventIDPattern       = regexp.MustCompile(`^evt_[a-z][a-z0-9_]*$`)
	permissionIDPattern  = regexp.MustCompile(`^prm_[a-z][a-z0-9_]*$`)
	navItemIDPattern     = regexp.MustCompile(`^nav_[a-z][a-z0-9_]*$`)
	datasetIDPattern     = regexp.MustCompile(`^ds_[a-z][a-z0-9_]*$`)
	relationIDPattern    = regexp.MustCompile(`^rel_[a-z][a-z0-9_]*$`)
	measureIDPattern     = regexp.MustCompile(`^msr_[a-z][a-z0-9_]*$`)
	viewIDPattern        = regexp.MustCompile(`^vw_[a-z][a-z0-9_]*$`)
	transitionIDPattern  = regexp.MustCompile(`^trn_[a-z][a-z0-9_]*$`)
	memberBlockIDPattern = regexp.MustCompile(`^blk_[a-z][a-z0-9_]*$`)
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
	settingsHubID := ""
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
		// Same closed-set posture as badge directly above, and for a sharper reason: an icon name
		// nothing draws renders an empty box in the bottom bar rather than no icon at all, so the
		// tab looks broken instead of plain (domain.KnownIcons, internal/rendering.icon).
		if n.Icon != "" && !domain.KnownIcons[n.Icon] {
			issues = append(issues, fmt.Sprintf("navigation item %q: unknown icon %q -- see domain.KnownIcons", n.ID, n.Icon))
		}
		if n.HomeCard {
			if homeCardID != "" {
				issues = append(issues, fmt.Sprintf("navigation item %q: home_card is already set on %q -- at most one item may claim it", n.ID, homeCardID))
			}
			homeCardID = n.ID
		}
		// SettingsHub is at most one per list, the same shape as HomeCard directly above -- it names
		// the one destination an Application's "Settings" link and hub page read.
		if n.SettingsHub {
			if settingsHubID != "" {
				issues = append(issues, fmt.Sprintf("navigation item %q: settings_hub is already set on %q -- at most one item may claim it", n.ID, settingsHubID))
			}
			settingsHubID = n.ID
			if n.SettingsHubMember {
				issues = append(issues, fmt.Sprintf("navigation item %q: settings_hub and settings_hub_member are mutually exclusive -- the hub item is the landing page, not a member of its own hub", n.ID))
			}
		}
	}
	return issues
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
		if !domain.FieldIDPattern.MatchString(f.ID) {
			issues = append(issues, fmt.Sprintf("field id %q must match %s", f.ID, domain.FieldIDPattern.String()))
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
		// A group Field names no Machine and must not pretend to: a Group is a platform row, so a
		// machine: here could never resolve. Rejected rather than ignored, the same rule a View's
		// own group_by follows -- a declaration the runtime silently drops reads as though it were
		// working (see domain.FieldTypeGroup's doc comment for why there is no mch_group).
		if f.Type == domain.FieldTypeGroup && f.RelatedMachine != "" {
			issues = append(issues, fmt.Sprintf("field %q: type group takes no machine: -- a Group is a Workspace platform record, not a Machine (got %q)", f.ID, f.RelatedMachine))
		}
		if f.Default != nil && f.ViolatesOptions(fmt.Sprint(f.Default)) {
			issues = append(issues, fmt.Sprintf("field %q: default %q is not one of its own options %v", f.ID, f.Default, f.Options))
		}
	}

	for _, c := range m.Constraints {
		issues = append(issues, validateConstraint(m, c, fieldsByID)...)
	}

	seenMemberBlocks := make(map[string]bool, len(m.MemberRemovalBlocks))
	for _, b := range m.MemberRemovalBlocks {
		issues = append(issues, validateMemberRemovalBlock(m, b, fieldsByID, seenMemberBlocks)...)
	}

	seenEvents := make(map[string]bool, len(m.Events))
	for _, e := range m.Events {
		issues = append(issues, validateEvent(m, e, fieldsByID, seenEvents)...)
	}

	seenPermissions := make(map[string]bool, len(m.Permissions))
	for _, p := range m.Permissions {
		issues = append(issues, validatePermission(m, p, fieldsByID, seenPermissions)...)
	}

	seenTransitions := make(map[string]bool, len(m.Transitions))
	seenEdges := make(map[string]string, len(m.Transitions))
	for _, t := range m.Transitions {
		issues = append(issues, validateTransition(m, t, fieldsByID, seenTransitions, seenEdges)...)
	}

	issues = append(issues, validateActionEffects(m, fieldsByID)...)

	seenDatasets := make(map[string]bool, len(m.Datasets))
	for _, ds := range m.Datasets {
		issues = append(issues, validateDataset(m, ds, fieldsByID, seenDatasets)...)
	}

	if m.Sequencing != nil {
		issues = append(issues, validateSequencing(m, *m.Sequencing, fieldsByID)...)
	}

	if m.SignaturePlacement != nil {
		issues = append(issues, validateSignaturePlacement(m, *m.SignaturePlacement, fieldsByID)...)
	}
	if m.SignatureStore != nil {
		issues = append(issues, validateSignatureStore(m, *m.SignatureStore, fieldsByID)...)
	}
	if m.FlowTemplate != nil {
		issues = append(issues, validateFlowTemplate(m, *m.FlowTemplate, fieldsByID)...)
	}
	if m.FlowTemplateStep != nil {
		issues = append(issues, validateFlowTemplateStep(m, *m.FlowTemplateStep, fieldsByID)...)
	}

	if m.SLAField != "" {
		f, ok := fieldsByID[m.SLAField]
		if !ok {
			issues = append(issues, fmt.Sprintf("machine %q: sla_field %q is not a field of this machine", m.ID, m.SLAField))
		} else if f.Type != domain.FieldTypeDate {
			issues = append(issues, fmt.Sprintf("machine %q: sla_field %q must be a date field, got %q", m.ID, m.SLAField, f.Type))
		}
	}
	for _, cf := range m.CardFields {
		if _, ok := fieldsByID[cf.Field]; !ok {
			issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q is not a field of this machine", m.ID, cf.Field))
		}
		if !domain.KnownCardFieldRoles[cf.Role] {
			issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q has unknown role %q", m.ID, cf.Field, cf.Role))
		}
	}

	// An append-only Machine that also declares who may edit or delete it is a contradiction the
	// runtime would resolve silently (the refusal wins), leaving a Permission that reads as a
	// grant and can never fire -- the same "declared but unreachable" shape validateTransition
	// refuses. Say one or the other.
	if m.AppendOnly {
		for _, p := range m.Permissions {
			if p.Action == domain.ActionEdit || p.Action == domain.ActionDelete {
				issues = append(issues, fmt.Sprintf("machine %q: append_only is declared, so permission %q (%s) can never be satisfied by anyone -- drop the permission or drop append_only", m.ID, p.ID, p.Action))
			}
		}
	}

	seenViews := make(map[string]bool, len(m.Views))
	for _, v := range m.Views {
		issues = append(issues, validateView(m, v, fieldsByID, seenViews)...)
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateView checks one declared arrangement. Each type states its own requirement, and the
// cards one is the point of this whole step: a cards View whose Machine declares no card_fields
// would render empty cards forever -- exactly the silent state Projection sat in from the day it
// shipped until 2026-09-20, wired end to end with nothing declared for it to project. Requiring
// the declaration is what stops that recurring one View at a time.
func validateView(m *domain.Machine, v domain.View, fieldsByID map[string]domain.Field, seen map[string]bool) []string {
	var issues []string

	if !viewIDPattern.MatchString(v.ID) {
		issues = append(issues, fmt.Sprintf("machine %q: view id %q must match %s", m.ID, v.ID, viewIDPattern.String()))
	}
	if strings.TrimSpace(v.Name) == "" {
		issues = append(issues, fmt.Sprintf("machine %q: view %q must declare a name -- it is what a viewer reads when choosing an arrangement", m.ID, v.ID))
	}
	if seen[v.ID] {
		issues = append(issues, fmt.Sprintf("machine %q: view id %q is declared more than once", m.ID, v.ID))
	}
	seen[v.ID] = true

	if !domain.KnownViewKinds[v.EffectiveType()] {
		issues = append(issues, fmt.Sprintf("machine %q: view %q has unknown type %q", m.ID, v.ID, v.Type))
		return issues
	}

	switch v.EffectiveType() {
	case domain.ViewBoard:
		if _, ok := fieldsByID[v.GroupBy]; !ok {
			issues = append(issues, fmt.Sprintf("machine %q: view %q is a board, so group_by %q must be a field of this machine", m.ID, v.ID, v.GroupBy))
		}
	case domain.ViewCards:
		if len(m.CardFields) == 0 {
			issues = append(issues, fmt.Sprintf("machine %q: view %q renders cards, so this machine must declare card_fields -- a cards view over no projection renders empty cards", m.ID, v.ID))
		}
	case domain.ViewStepper:
		if m.Sequencing == nil {
			issues = append(issues, fmt.Sprintf("machine %q: view %q renders a stepper, so this machine must declare sequencing -- a stepper has no ordering/decision concept to render otherwise", m.ID, v.ID))
		}
	default:
		if v.GroupBy != "" {
			issues = append(issues, fmt.Sprintf("machine %q: view %q is a %s, so group_by %q means nothing here", m.ID, v.ID, v.EffectiveType(), v.GroupBy))
		}
	}

	return issues
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
	} else if onField.ViolatesOptions(c.WhenEquals) {
		issues = append(issues, fmt.Sprintf("constraint %q: when_equals %q is not one of field %q's options %v", c.ID, c.WhenEquals, c.On, onField.Options))
	}
	if c.WhenEquals == "" {
		issues = append(issues, fmt.Sprintf("constraint %q: when_equals is required", c.ID))
	}

	if !machineIDPattern.MatchString(c.BlockIf.RelatedMachine) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.related_machine %q must match %s", c.ID, c.BlockIf.RelatedMachine, machineIDPattern.String()))
	}
	if !domain.FieldIDPattern.MatchString(c.BlockIf.RelatedField) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.related_field %q must match %s", c.ID, c.BlockIf.RelatedField, domain.FieldIDPattern.String()))
	}
	if !domain.FieldIDPattern.MatchString(c.BlockIf.Condition.Field) {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.field %q must match %s", c.ID, c.BlockIf.Condition.Field, domain.FieldIDPattern.String()))
	}
	if !expression.KnownOps[c.BlockIf.Condition.Op] {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.op %q is not a known operator", c.ID, c.BlockIf.Condition.Op))
	}
	if c.BlockIf.Condition.Value == "" {
		issues = append(issues, fmt.Sprintf("constraint %q: block_if.condition.value is required", c.ID))
	}

	return issues
}

// validateMemberRemovalBlock checks one blocks_member_removal entry's own shape -- everything
// checkable from this one Machine file, the same split validateConstraint already takes (no
// cross-Machine pass needed here: actor_field and condition.field are both this Machine's own).
func validateMemberRemovalBlock(m *domain.Machine, b domain.MemberRemovalBlock, fieldsByID map[string]domain.Field, seen map[string]bool) []string {
	var issues []string

	if !memberBlockIDPattern.MatchString(b.ID) {
		issues = append(issues, fmt.Sprintf("member removal block id %q must match %s", b.ID, memberBlockIDPattern.String()))
	}
	if seen[b.ID] {
		issues = append(issues, fmt.Sprintf("member removal block id %q is declared more than once", b.ID))
	}
	seen[b.ID] = true

	actorField, ok := fieldsByID[b.ActorField]
	if !ok {
		issues = append(issues, fmt.Sprintf("member removal block %q: actor_field %q is not a field of machine %q", b.ID, b.ActorField, m.ID))
	} else if actorField.Type != domain.FieldTypePerson {
		issues = append(issues, fmt.Sprintf("member removal block %q: actor_field %q must be type person, got %q", b.ID, b.ActorField, actorField.Type))
	}

	if !domain.FieldIDPattern.MatchString(b.Condition.Field) {
		issues = append(issues, fmt.Sprintf("member removal block %q: condition.field %q must match %s", b.ID, b.Condition.Field, domain.FieldIDPattern.String()))
	} else if conditionField, ok := fieldsByID[b.Condition.Field]; !ok {
		issues = append(issues, fmt.Sprintf("member removal block %q: condition.field %q is not a field of machine %q", b.ID, b.Condition.Field, m.ID))
	} else if conditionField.ViolatesOptions(b.Condition.Value) {
		issues = append(issues, fmt.Sprintf("member removal block %q: condition.value %q is not one of field %q's options %v", b.ID, b.Condition.Value, b.Condition.Field, conditionField.Options))
	}
	if !expression.KnownOps[b.Condition.Op] {
		issues = append(issues, fmt.Sprintf("member removal block %q: condition.op %q is not a known operator", b.ID, b.Condition.Op))
	}
	if b.Reason == "" {
		issues = append(issues, fmt.Sprintf("member removal block %q: reason is required -- it is the sentence a blocked deactivation attempt shows", b.ID))
	}

	return issues
}

// validateEvent checks one Event's own shape, mirroring validateConstraint: everything it needs
// is on the Machine itself, so there is no cross-Machine pass the way Constraint's block_if
// needs. Unlike Constraint's when_equals, Event's WhenEquals is optional -- empty means "any
// change fires it," a deliberate difference (Constraint gates a specific transition; Event
// merely observes one). OnCreate and Schedule are the other two, mutually exclusive shapes
// (domain.Event's own doc comment): exactly one of On, OnCreate or Schedule must be set, and
// WhenEquals is meaningless with either of the other two (no prior value exists to compare
// against, or there is no write at all).
func validateEvent(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field, seen map[string]bool) []string {
	var issues []string

	if !eventIDPattern.MatchString(e.ID) {
		issues = append(issues, fmt.Sprintf("event id %q must match %s", e.ID, eventIDPattern.String()))
	}
	if seen[e.ID] {
		issues = append(issues, fmt.Sprintf("event id %q is declared more than once", e.ID))
	}
	seen[e.ID] = true

	shapes := 0
	if e.OnCreate {
		shapes++
	}
	if e.On != "" {
		shapes++
	}
	if e.Schedule != nil {
		shapes++
	}

	switch {
	case shapes > 1:
		issues = append(issues, fmt.Sprintf("event %q: on, on_create and schedule are mutually exclusive -- exactly one may be set", e.ID))
	case shapes == 0:
		issues = append(issues, fmt.Sprintf("event %q: exactly one of on, on_create or schedule is required", e.ID))
	case e.OnCreate && e.WhenEquals != "":
		issues = append(issues, fmt.Sprintf("event %q: when_equals is not meaningful with on_create (no prior value to compare)", e.ID))
	case e.Schedule != nil && e.WhenEquals != "":
		issues = append(issues, fmt.Sprintf("event %q: when_equals is not meaningful with schedule (there is no write to compare a prior value against)", e.ID))
	case e.Schedule != nil:
		issues = append(issues, validateSchedule(m, e, fieldsByID)...)
	case e.On != "":
		onField, onExists := fieldsByID[e.On]
		if !onExists {
			issues = append(issues, fmt.Sprintf("event %q: on %q is not a field of machine %q", e.ID, e.On, m.ID))
		} else if e.WhenEquals != "" && onField.ViolatesOptions(e.WhenEquals) {
			issues = append(issues, fmt.Sprintf("event %q: when_equals %q is not one of field %q's options %v", e.ID, e.WhenEquals, e.On, onField.Options))
		}
	}

	// Each Service owns its own required keys, and since 2026-09-30 it owns them *in one place*:
	// internal/registry.Services maps the name to the contract, so a new Service arrives with its
	// validator attached instead of as a fifth arm here and a sixth somewhere else. An unknown name is
	// a lookup miss, which is the same load error the `default:` arm reported.
	issues = append(issues, registry.ValidateService(m, e, fieldsByID)...)

	return issues
}

// validateSchedule checks a schedule-shaped Event's own block: date_field must be a real "date"
// Field of this Machine, when must be one of the closed vocabulary, and guard_field/guard_equals
// (optional, but only together) must name a real Field and one of its options -- the same shape
// On/WhenEquals validation above already takes for a field-change Event.
func validateSchedule(m *domain.Machine, e domain.Event, fieldsByID map[string]domain.Field) []string {
	var issues []string
	s := *e.Schedule

	dateField, ok := fieldsByID[s.DateField]
	if !ok {
		issues = append(issues, fmt.Sprintf("event %q: schedule.date_field %q is not a field of machine %q", e.ID, s.DateField, m.ID))
	} else if dateField.Type != domain.FieldTypeDate {
		issues = append(issues, fmt.Sprintf("event %q: schedule.date_field %q must be type date, got %q", e.ID, s.DateField, dateField.Type))
	}
	if !domain.KnownScheduleWhens[s.When] {
		issues = append(issues, fmt.Sprintf("event %q: schedule.when %q is not a comparison this runtime knows", e.ID, s.When))
	}
	if (s.GuardField == "") != (s.GuardEquals == "") {
		issues = append(issues, fmt.Sprintf("event %q: schedule.guard_field and schedule.guard_equals must be set together or not at all", e.ID))
	} else if s.GuardField != "" {
		guardField, ok := fieldsByID[s.GuardField]
		if !ok {
			issues = append(issues, fmt.Sprintf("event %q: schedule.guard_field %q is not a field of machine %q", e.ID, s.GuardField, m.ID))
		} else if guardField.ViolatesOptions(s.GuardEquals) {
			issues = append(issues, fmt.Sprintf("event %q: schedule.guard_equals %q is not one of field %q's options %v", e.ID, s.GuardEquals, s.GuardField, guardField.Options))
		}
	}

	return issues
}

// validatePermission checks one Permission's own shape (development-history.md Phase 16). Everything it
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

	// A Permission must gate on *something*. Until Fase 7 that was always actor_field, so its
	// absence was simply a missing field; now a Permission may be role-only (CAP-P01), and the
	// check has to be "at least one arm" rather than "this one arm". A Permission with no arm at
	// all is the dangerous case the emptiness would otherwise hide: authorization.allowsOne reads
	// it as "nothing to check", so it would parse, validate, and protect nothing while looking
	// like a guard.
	if p.WorkspaceRole != "" && !domain.KnownWorkspaceRoles[p.WorkspaceRole] {
		issues = append(issues, fmt.Sprintf("permission %q: workspace_role %q is not a workspace role this runtime realizes -- admin is the only one, and requiring \"member\" would be a rule every member passes and every admin fails", p.ID, p.WorkspaceRole))
	}

	switch {
	case p.ActorField == "" && p.DynamicActor == nil && len(p.Roles) == 0 && p.WorkspaceRole == "":
		issues = append(issues, fmt.Sprintf("permission %q: declares no actor_field, no actor_*_field gate, no roles and no workspace_role -- a permission that gates on nothing protects nothing", p.ID))
	case p.ActorField == "":
		// Role-only, workspace-role-only or dynamic-gate-only: checked above/below instead.
	default:
		actorField, ok := fieldsByID[p.ActorField]
		if !ok {
			issues = append(issues, fmt.Sprintf("permission %q: actor_field %q is not a field of machine %q", p.ID, p.ActorField, m.ID))
		} else if !actorField.IsReference() {
			issues = append(issues, unboundActorField(p.ID, "actor_field", p.ActorField, actorField))
		}
	}

	for i, role := range p.Roles {
		if strings.TrimSpace(role) == "" {
			issues = append(issues, fmt.Sprintf("permission %q: roles entry %d is empty -- omit it rather than declaring a blank role, which no vocabulary contains", p.ID, i))
		}
	}

	issues = append(issues, validateDynamicActor(m, p, fieldsByID)...)

	return issues
}

// validateTransition checks one declared edge of a Machine's state model (Case 03 Fase 7).
//
// Every check here exists because its absence fails silently rather than loudly. A transition
// naming a Field that is not a status Field, or a value that Field does not declare, can never
// match at write time -- so behavior.CheckTransitions would refuse a move that metadata looks
// like it permits, and the screen reading the same declaration would draw a row for an edge no
// record can take. Both are the "declared but unreachable" shape this package exists to catch at
// load; the Machine would still boot.
//
// seenEdges keys on field+from+to rather than on the id, because TransitionFor resolves an edge
// by those three and returns the first match: two declarations of one edge would make which
// Action governs it depend on declaration order, which is the same ambiguity
// validateApplicationClaims refuses for a Machine claimed twice.
func validateTransition(m *domain.Machine, t domain.Transition, fieldsByID map[string]domain.Field, seen map[string]bool, seenEdges map[string]string) []string {
	var issues []string

	if !transitionIDPattern.MatchString(t.ID) {
		issues = append(issues, fmt.Sprintf("machine %q: transition id %q must match %s", m.ID, t.ID, transitionIDPattern.String()))
	}
	if seen[t.ID] {
		issues = append(issues, fmt.Sprintf("machine %q: transition id %q is declared more than once", m.ID, t.ID))
	}
	seen[t.ID] = true

	if t.Name == "" {
		issues = append(issues, fmt.Sprintf("machine %q: transition %q: name is required -- it is what this move is called in the business, and no screen can derive it from from/to", m.ID, t.ID))
	}

	field, ok := fieldsByID[t.Field]
	switch {
	case !ok:
		issues = append(issues, fmt.Sprintf("machine %q: transition %q: field %q is not a field of this machine", m.ID, t.ID, t.Field))
	case field.Type != domain.FieldTypeStatus:
		issues = append(issues, fmt.Sprintf("machine %q: transition %q: field %q must be a status field, got %q -- a transition moves between declared options", m.ID, t.ID, t.Field, field.Type))
	default:
		for label, value := range map[string]string{"from": t.From, "to": t.To} {
			if !slices.Contains(field.Options, value) {
				issues = append(issues, fmt.Sprintf("machine %q: transition %q: %s %q is not one of %q's own options %v", m.ID, t.ID, label, value, t.Field, field.Options))
			}
		}
		if t.From == t.To {
			issues = append(issues, fmt.Sprintf("machine %q: transition %q: from and to are both %q -- a value that does not change is never a transition (behavior.CheckTransitions skips it), so this edge could never fire", m.ID, t.ID, t.From))
		}
	}

	// An empty action is the declared "the runtime performs this itself" case (domain.Transition.
	// Action), so only a non-empty one is checked against the closed set.
	if t.Action != "" && !domain.KnownActions[t.Action] {
		issues = append(issues, fmt.Sprintf("machine %q: transition %q: action %q is not an action this runtime realizes", m.ID, t.ID, t.Action))
	}

	edge := t.Field + "\x00" + t.From + "\x00" + t.To
	if prev, dup := seenEdges[edge]; dup {
		issues = append(issues, fmt.Sprintf("machine %q: transition %q declares the same %s move %q -> %q as %q -- which action governs it would depend on declaration order", m.ID, t.ID, t.Field, t.From, t.To, prev))
	} else {
		seenEdges[edge] = t.ID
	}

	return issues
}

// validateDynamicActor checks the three Fields a dynamic actor gate names (CAP-F24, Fase 6c-1).
//
// Each one is checked for existence *and* type, because the gate reads them positionally at
// request time and a wrong type fails in a way nobody sees: authorization.allowsOne would compare
// an identity against, say, a date and simply return false, so a mis-declared gate would present
// as "this person may never approve anything" rather than as a metadata error. That is exactly
// the class of silent failure this package exists to turn into a load-time refusal.
//
// The type field must also declare the two values the resolver switches on. Declaring a status
// Field whose options are, say, [person, team] would parse, validate and then never match either
// arm -- falling back to actor_field forever while looking configured.
func validateDynamicActor(m *domain.Machine, p domain.Permission, fieldsByID map[string]domain.Field) []string {
	da := p.DynamicActor
	if da == nil {
		return nil
	}
	var issues []string

	typeField, ok := fieldsByID[da.ActorTypeField]
	switch {
	case da.ActorTypeField == "":
		issues = append(issues, fmt.Sprintf("permission %q: actor_type_field is required when any actor_*_field is declared", p.ID))
	case !ok:
		issues = append(issues, fmt.Sprintf("permission %q: actor_type_field %q is not a field of machine %q", p.ID, da.ActorTypeField, m.ID))
	case typeField.Type != domain.FieldTypeStatus:
		issues = append(issues, fmt.Sprintf("permission %q: actor_type_field %q must be a status field, got %q", p.ID, da.ActorTypeField, typeField.Type))
	default:
		for _, want := range []string{domain.ActorKindUser, domain.ActorKindGroup} {
			if !slices.Contains(typeField.Options, want) {
				issues = append(issues, fmt.Sprintf("permission %q: actor_type_field %q must declare option %q, which is a value the gate resolves; got %v", p.ID, da.ActorTypeField, want, typeField.Options))
			}
		}
	}

	userField, ok := fieldsByID[da.ActorUserField]
	switch {
	case da.ActorUserField == "":
		issues = append(issues, fmt.Sprintf("permission %q: actor_user_field is required when a dynamic actor gate is declared", p.ID))
	case !ok:
		issues = append(issues, fmt.Sprintf("permission %q: actor_user_field %q is not a field of machine %q", p.ID, da.ActorUserField, m.ID))
	case !userField.IsReference():
		issues = append(issues, unboundActorField(p.ID, "actor_user_field", da.ActorUserField, userField))
	}

	groupField, ok := fieldsByID[da.ActorGroupField]
	switch {
	case da.ActorGroupField == "":
		issues = append(issues, fmt.Sprintf("permission %q: actor_group_field is required when a dynamic actor gate is declared", p.ID))
	case !ok:
		issues = append(issues, fmt.Sprintf("permission %q: actor_group_field %q is not a field of machine %q", p.ID, da.ActorGroupField, m.ID))
	case groupField.Type != domain.FieldTypeGroup:
		issues = append(issues, fmt.Sprintf("permission %q: actor_group_field %q must be a group field, got %q", p.ID, da.ActorGroupField, groupField.Type))
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

	// The two modes, and the refusal to mix them. A Dataset answers either "how many, grouped how"
	// or "which rows, in what order"; one trying to be both leaves every consumer asking which half
	// it got, so the mix is a load error rather than a precedence rule.
	if ds.Select != "" && ds.Select != domain.SelectRecords {
		issues = append(issues, fmt.Sprintf("dataset %q: select %q is not a known mode (the only one is %q)", ds.ID, ds.Select, domain.SelectRecords))
	}
	if ds.Select == domain.SelectRecords {
		if len(ds.Measures) > 0 {
			issues = append(issues, fmt.Sprintf("dataset %q: `select: records` returns records, so it declares no measures", ds.ID))
		}
		if ds.Dimension != "" {
			issues = append(issues, fmt.Sprintf("dataset %q: `select: records` returns records, so it declares no dimension -- a dimension groups aggregates", ds.ID))
		}
		// 007 §7.9 puts pagination in the Data Plane because "it determines how much data is
		// retrieved", and asks the runtime to "preserve the current capability that prevents
		// unbounded list retrieval". An unbounded `select: records` is that retrieval.
		if ds.Limit <= 0 {
			issues = append(issues, fmt.Sprintf("dataset %q: `select: records` requires a positive `limit:` -- an unbounded record selection is the unbounded retrieval 007 §7.9 asks the runtime to prevent", ds.ID))
		}
		seenRelations := map[string]bool{}
		for _, rel := range ds.Relations {
			if !relationIDPattern.MatchString(rel.ID) {
				issues = append(issues, fmt.Sprintf("dataset %q: relation id %q must match %s", ds.ID, rel.ID, relationIDPattern.String()))
			}
			if seenRelations[rel.ID] {
				issues = append(issues, fmt.Sprintf("dataset %q: relation id %q is declared more than once", ds.ID, rel.ID))
			}
			seenRelations[rel.ID] = true
			if rel.Machine == "" {
				issues = append(issues, fmt.Sprintf("dataset %q: relation %q declares no `machine:`", ds.ID, rel.ID))
			}
			// Required rather than derived, deliberately -- see domain.Relation's own comment: a child
			// may declare two Fields pointing at one parent, and picking one would depend on declaration
			// order, which 007 §4.6 forbids.
			if rel.Via == "" {
				issues = append(issues, fmt.Sprintf("dataset %q: relation %q declares no `via:` -- name the reference field on %q that points back at %q", ds.ID, rel.ID, rel.Machine, m.ID))
			}
		}
		for _, c := range ds.Where.Comparisons() {
			if _, ok := fieldsByID[c.Field]; !ok {
				issues = append(issues, fmt.Sprintf("dataset %q: where field %q is not a field of machine %q", ds.ID, c.Field, m.ID))
			}
			if !expression.KnownOps[c.Op] {
				issues = append(issues, fmt.Sprintf("dataset %q: where op %q is not a known operator", ds.ID, c.Op))
			}
			// 007 §9.2: "Access outside this context must fail closed." A sentinel the runtime cannot
			// resolve must be refused here, because at runtime it would become a literal -- a filter
			// comparing a Field against the string "$current_usr" matches nothing, renders an empty
			// list, and is indistinguishable from a screen that legitimately has no rows.
			if expression.IsSentinel(c.Value) && !expression.KnownSentinel(c.Value) {
				issues = append(issues, fmt.Sprintf("dataset %q: where value %q is not a runtime context value this runtime resolves (%s, %s<name>) -- an unknown one would be compared as a literal and silently match nothing", ds.ID, c.Value, expression.SentinelCurrentUser, expression.SentinelParameterPrefix))
			}
		}
	} else {
		if ds.Where != nil {
			issues = append(issues, fmt.Sprintf("dataset %q: `where:` selects records; an aggregating Dataset filters per measure instead", ds.ID))
		}
		if len(ds.Relations) > 0 {
			issues = append(issues, fmt.Sprintf("dataset %q: `relations:` attach children to selected records; an aggregate has no rows to attach them to", ds.ID))
		}
		if len(ds.Measures) == 0 {
			issues = append(issues, fmt.Sprintf("dataset %q: at least one measure is required", ds.ID))
		}
		if len(ds.Sort) > 0 {
			issues = append(issues, fmt.Sprintf("dataset %q: `sort:` orders selected records and has no meaning for aggregates -- declare `select: records` or drop it", ds.ID))
		}
		if ds.Limit != 0 {
			issues = append(issues, fmt.Sprintf("dataset %q: `limit:` bounds selected records and has no meaning for aggregates -- declare `select: records` or drop it", ds.ID))
		}
	}

	for _, k := range ds.Sort {
		_, isColumn := domain.SortableColumns[k.Field]
		if _, isField := fieldsByID[k.Field]; !isField && !isColumn {
			issues = append(issues, fmt.Sprintf("dataset %q: sort field %q is neither a field of machine %q nor a record column (%s)", ds.ID, k.Field, m.ID, sortableColumnList()))
		}
		if k.Direction != "" && !domain.KnownSortDirections[k.Direction] {
			issues = append(issues, fmt.Sprintf("dataset %q: sort direction %q is not %q or %q", ds.ID, k.Direction, domain.SortAscending, domain.SortDescending))
		}
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
			} else if whereField.ViolatesOptions(ms.Where.Value) {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.value %q is not one of field %q's options %v", ds.ID, ms.ID, ms.Where.Value, ms.Where.Field, whereField.Options))
			}
			if !expression.KnownOps[ms.Where.Op] {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.op %q is not a known operator", ds.ID, ms.ID, ms.Where.Op))
			}
		}
	}

	return issues
}

// unboundActorField is the message both actor-field checks share, and it says what is actually wrong.
//
// Both used to read `must reference an identity (a person or relation field), got "person"` -- which
// contradicts itself in one line, since a person field is exactly what is being asked for. The check is
// Field.IsReference(), which reads RelatedMachine, so the real fault is a *missing target*, never the
// type:
//
//   - a `relation` field whose `machine:` is absent -- the only way a hand-written manifest reaches
//     here, and the old wording answered `got "relation"`, equally unhelpful;
//   - a person field built in Go without RelatedMachine -- Parse fills that in for YAML (its own
//     "authors never write `machine: mch_user` by hand"), so only a hand-built *domain.Machine can be
//     in this state. Two test fixtures were, which is how this was found.
//
// So the answer is three branches, not one rewording: a relation field is missing a *target*, a
// non-reference type can never have one and naming the type is the whole answer (which is what the old
// message got right), and a person field here means the Machine skipped normalisation -- unreachable
// through the loader, and the message says so instead of blaming the type.
func unboundActorField(permissionID, key, fieldID string, f domain.Field) string {
	switch f.Type {
	case domain.FieldTypeRelation:
		// The one case a hand-written manifest reaches: the type is right and the target is missing.
		return fmt.Sprintf("permission %q: %s %q is a relation field with no `machine:` target, so it names no identity -- point it at a machine",
			permissionID, key, fieldID)
	case domain.FieldTypePerson:
		// Unreachable through the loader, since Normalize binds every person Field to mch_user. Reaching
		// it means a *domain.Machine was built in Go and validated without being normalised -- so the
		// message says that, rather than blaming a type that is already correct.
		return fmt.Sprintf("permission %q: %s %q is a person field with no related machine -- this Machine was not normalised (metadata.Normalize binds person to %s; the loader does it for you)",
			permissionID, key, fieldID, domain.UserMachineID)
	default:
		// text, number, status and the rest: no target is possible, so naming the type is the answer.
		return fmt.Sprintf("permission %q: %s %q is a %s field, which cannot name an identity -- use a person or relation field",
			permissionID, key, fieldID, f.Type)
	}
}

// validateSignaturePlacement and validateSignatureStore check that each declared Field is this
// Machine's own and can hold what the capability puts in it (Stage D, 2026-09-28).
//
// Everything these two blocks name is a Field of the declaring Machine, so unlike Sequencing there is
// no second half deferred to application.go. What is deliberately *not* checked here is which
// workflow role the Machine was cast in: `signature_placement:` on a Machine playing no role is inert
// rather than wrong, the same way a Machine declaring `sequencing:` outside any engine simply orders
// its own records. Requiring the cast would make load order matter for no gain.
//
// An empty entry is allowed and means "this Machine declares no such Field" -- the readers all handle
// an empty id explicitly (domain.SignaturePlacement's own doc comment). What is refused is a *named*
// Field that does not exist or cannot hold the value, which is the mistake that would otherwise read
// as a feature quietly not working.
func validateSignaturePlacement(m *domain.Machine, s domain.SignaturePlacement, fieldsByID map[string]domain.Field) []string {
	var issues []string
	issues = append(issues, signatureFieldOfType(m, "signature_placement.image_field", s.ImageField, domain.FieldTypeFile, fieldsByID)...)
	// A slice rather than a map, so the report comes out in declaration order every time: map
	// iteration is random, and a validation message set that reshuffles between runs is unreadable.
	for _, pair := range [][2]string{
		{"signature_placement.page_field", s.PageField},
		{"signature_placement.x_field", s.XField},
		{"signature_placement.y_field", s.YField},
		{"signature_placement.width_field", s.WidthField},
	} {
		issues = append(issues, signatureFieldOfType(m, pair[0], pair[1], domain.FieldTypeNumber, fieldsByID)...)
	}
	return issues
}

func validateSignatureStore(m *domain.Machine, s domain.SignatureStore, fieldsByID map[string]domain.Field) []string {
	var issues []string
	issues = append(issues, signatureFieldOfType(m, "signature_store.owner_field", s.OwnerField, domain.FieldTypePerson, fieldsByID)...)
	issues = append(issues, signatureFieldOfType(m, "signature_store.image_field", s.ImageField, domain.FieldTypeFile, fieldsByID)...)
	return issues
}

// signatureFieldOfType is the one check both blocks repeat: a named Field is this Machine's own and
// holds the type the capability needs. An empty id is not an error -- see the doc comment above.
func signatureFieldOfType(m *domain.Machine, key, id string, want domain.FieldType, fieldsByID map[string]domain.Field) []string {
	if id == "" {
		return nil
	}
	f, ok := fieldsByID[id]
	if !ok {
		return []string{fmt.Sprintf("machine %q: %s %q is not a field of this machine", m.ID, key, id)}
	}
	if f.Type != want {
		return []string{fmt.Sprintf("machine %q: %s %q must be a %s field, got %q", m.ID, key, id, want, f.Type)}
	}
	return nil
}

// validateFlowTemplate and validateFlowTemplateStep check that a saved-flow declaration names this
// Machine's own Fields, with types that can hold what the capability puts in them (Stage E2,
// 2026-09-29).
//
// Same posture as the signature blocks: an empty entry means "this Machine declares no such Field" and
// every reader handles that explicitly, while a *named* Field that does not exist or cannot hold the
// value is refused -- the mistake that would otherwise read as a feature quietly not working. Which
// workflow role the Machine was cast in is deliberately not checked here; a block on a Machine playing
// no role is inert rather than wrong, and requiring the cast would make load order matter for nothing.
func validateFlowTemplate(m *domain.Machine, ft domain.FlowTemplate, fieldsByID map[string]domain.Field) []string {
	var issues []string
	// The key and the mode are both closed option sets in the template library, but neither has to be:
	// what matters is that the Field exists, since a saved flow is found by *matching values* of it.
	// So existence only, no type demand -- stated because the neighbours below do demand types.
	issues = append(issues, signatureFieldExists(m, "flow_template.key_field", ft.KeyField, fieldsByID)...)
	issues = append(issues, signatureFieldExists(m, "flow_template.mode_field", ft.ModeField, fieldsByID)...)
	return issues
}

func validateFlowTemplateStep(m *domain.Machine, fs domain.FlowTemplateStep, fieldsByID map[string]domain.Field) []string {
	var issues []string
	issues = append(issues, signatureFieldOfType(m, "flow_template_step.order_field", fs.OrderField, domain.FieldTypeNumber, fieldsByID)...)
	issues = append(issues, signatureFieldOfType(m, "flow_template_step.actor_field", fs.ActorField, domain.FieldTypePerson, fieldsByID)...)
	issues = append(issues, signatureFieldOfType(m, "flow_template_step.actor_group_field", fs.ActorGroupField, domain.FieldTypeGroup, fieldsByID)...)
	// The template link must reference something, and the name and actor-kind Fields only have to
	// exist -- a Machine may spell "which kind of actor" as text or as a closed status set, and this
	// runtime has no reason to prefer one.
	if fs.TemplateField != "" {
		if f, ok := fieldsByID[fs.TemplateField]; !ok {
			issues = append(issues, fmt.Sprintf("machine %q: flow_template_step.template_field %q is not a field of this machine", m.ID, fs.TemplateField))
		} else if !f.IsReference() {
			issues = append(issues, fmt.Sprintf("machine %q: flow_template_step.template_field %q must reference the template machine, got %q", m.ID, fs.TemplateField, f.Type))
		}
	}
	issues = append(issues, signatureFieldExists(m, "flow_template_step.actor_type_field", fs.ActorTypeField, fieldsByID)...)
	return issues
}

// signatureFieldExists is signatureFieldOfType without the type demand, for the entries whose Field
// may legitimately be spelled more than one way.
func signatureFieldExists(m *domain.Machine, key, id string, fieldsByID map[string]domain.Field) []string {
	if id == "" {
		return nil
	}
	if _, ok := fieldsByID[id]; !ok {
		return []string{fmt.Sprintf("machine %q: %s %q is not a field of this machine", m.ID, key, id)}
	}
	return nil
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
	} else if stateField.ViolatesOptions(s.OpenValue) {
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

func contains(options []string, v string) bool {
	for _, o := range options {
		if o == v {
			return true
		}
	}
	return false
}

// validateMachineIDsAreUnique guards the invariant that makes Workspace-level Machine loading
// safe: one id, one Machine object (Fase 3, 2026-09-20). Before this, Machines were loaded per
// Application and a duplicate could not arise; now app.yaml lists files once and every Application
// selects from the result, so listing the same file twice -- or two files declaring the same id --
// would silently give half the runtime one object and half the other.
func validateMachineIDsAreUnique(machines []*domain.Machine) error {
	seen := make(map[string]bool, len(machines))
	var issues []string
	for _, m := range machines {
		if seen[m.ID] {
			issues = append(issues, fmt.Sprintf("machine id %q is declared more than once -- machine ids are unique across the workspace, since every application selects from one loaded set", m.ID))
			continue
		}
		seen[m.ID] = true
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateApplicationClaims checks every Application's `machines:` selection against the
// Workspace's own set, and enforces that no Machine is claimed by two Applications.
//
// The second half is what keeps domain.Workspace.ApplicationForMachine unambiguous, which is how
// the runtime answers "which Application is this request in" for the many routes no navigation
// item names (/machines/{id}/records/{id}, /decide, /signature-placement, ...). Two claimants
// would make that answer depend on declaration order. A Machine genuinely shared between
// Applications (mch_user, mch_activity) is therefore claimed by *none* and stays Workspace-level;
// routes concerning it fall back to navigation, which is the designed path, not a gap.
func validateApplicationClaims(applications []domain.Application, machines []*domain.Machine) error {
	known := make(map[string]bool, len(machines))
	for _, m := range machines {
		known[m.ID] = true
	}

	var issues []string
	claimedBy := make(map[string]string)
	seenApp := make(map[string]bool, len(applications))
	for _, app := range applications {
		if seenApp[app.ID] {
			issues = append(issues, fmt.Sprintf("application id %q is declared more than once", app.ID))
		}
		seenApp[app.ID] = true

		for _, id := range app.Machines {
			if !known[id] {
				issues = append(issues, fmt.Sprintf("application %q: machines entry %q is not a machine this workspace declares", app.ID, id))
				continue
			}
			if prev, ok := claimedBy[id]; ok {
				issues = append(issues, fmt.Sprintf("machine %q is claimed by both application %q and application %q -- a machine belongs to at most one application, or which application a /machines/%s/... route is in would depend on declaration order; a genuinely shared machine should be claimed by neither", id, prev, app.ID, id))
				continue
			}
			claimedBy[id] = app.ID
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateNavigationIDsAreUnique checks navigation ids across the whole Workspace -- its own list
// plus every Application's.
//
// validateNavigation already enforces uniqueness *within* one list. Workspace-wide is the scope
// that matters now: internal/rendering's routeByID/labelByID resolve an id against the union of
// every declared list, so a duplicate id in two Applications would make those lookups return
// whichever was loaded first, silently pointing a page at another Application's screen.
func validateNavigationIDsAreUnique(ws domain.Workspace) error {
	declaredIn := make(map[string]string)
	var issues []string

	record := func(items []domain.NavigationItem, where string) {
		for _, item := range items {
			if prev, ok := declaredIn[item.ID]; ok {
				issues = append(issues, fmt.Sprintf("navigation id %q is declared by both %s and %s -- ids are resolved workspace-wide by routeByID/labelByID, so a duplicate would silently resolve to whichever loaded first", item.ID, prev, where))
				continue
			}
			declaredIn[item.ID] = where
		}
	}

	// The runtime's own screens are seeded first so an Application cannot take one of their ids.
	// They are no longer declared in metadata (see metadata/workspaces/*.yaml), but rendering.routeByID
	// still resolves them, so a redeclared id would silently shadow -- or be shadowed by -- a real
	// runtime screen depending only on slice order.
	record(domain.RuntimeScreens, "the runtime itself (domain.RuntimeScreens)")
	record(ws.Navigation, fmt.Sprintf("workspace %q", ws.Slug))
	for _, app := range ws.Applications {
		record(app.AllNavigation, fmt.Sprintf("application %q", app.ID))
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateActionEffects closes every way an `actions:` block could declare a write the runtime cannot
// perform, or can perform and should not (domain.ActionEffect; Stage B, 2026-09-28). Each of these is
// silent at runtime if it loads, which is why they are all load-time errors (005 Phase 3):
//
//   - an Action outside KnownActions names no route, so the effect would never run;
//   - two effects for one Action would make "what does decide write here" depend on declaration order;
//   - a Field this Machine does not declare would write a value nothing reads -- and, because record
//     values are JSONB, would not fail anywhere;
//   - neither `from:` nor `value:`, or both, is a write with no value or two;
//   - a `from:` outside KnownWriteSources would resolve to nothing and store an empty string;
//   - a `value:` outside the Field's own declared options would store a status no screen can render,
//     the same check validateTransition already applies to an edge's own From/To;
//   - an Action naming edges on two different Fields, which would make the moved Field ambiguous
//     (domain.Machine.ActionField returns "" rather than guessing, and this is what keeps that
//     unreachable).
//
// It deliberately does *not* require an effect to exist for every permitted Action: `edit` and `delete`
// write only what was submitted, and a Machine declaring no effect at all is the normal case.
func validateActionEffects(m *domain.Machine, fieldsByID map[string]domain.Field) []string {
	var issues []string
	seen := map[string]bool{}
	for _, e := range m.ActionEffects {
		if !domain.KnownActions[e.Action] {
			issues = append(issues, fmt.Sprintf("machine %q: actions entry names unknown action %q", m.ID, e.Action))
			continue
		}
		if seen[e.Action] {
			issues = append(issues, fmt.Sprintf("machine %q: action %q declares its writes twice -- one effect per action, or what it writes depends on declaration order", m.ID, e.Action))
			continue
		}
		seen[e.Action] = true
		if len(e.Writes) == 0 {
			issues = append(issues, fmt.Sprintf("machine %q: action %q declares no writes -- omit the entry rather than declaring an effect that does nothing", m.ID, e.Action))
		}
		for _, wr := range e.Writes {
			field, ok := fieldsByID[wr.Field]
			if !ok {
				issues = append(issues, fmt.Sprintf("machine %q: action %q writes field %q, which this machine does not declare", m.ID, e.Action, wr.Field))
				continue
			}
			switch {
			case wr.From == "" && wr.Value == "":
				issues = append(issues, fmt.Sprintf("machine %q: action %q writes field %q with neither from: nor value:", m.ID, e.Action, wr.Field))
			case wr.From != "" && wr.Value != "":
				issues = append(issues, fmt.Sprintf("machine %q: action %q writes field %q with both from: %q and value: %q -- say one", m.ID, e.Action, wr.Field, wr.From, wr.Value))
			case wr.From != "" && !domain.KnownWriteSources[wr.From]:
				issues = append(issues, fmt.Sprintf("machine %q: action %q writes field %q from %q, which is not a source this runtime resolves -- see domain.KnownWriteSources", m.ID, e.Action, wr.Field, wr.From))
			case wr.Value != "" && len(field.Options) > 0 && !contains(field.Options, wr.Value):
				issues = append(issues, fmt.Sprintf("machine %q: action %q writes field %q the value %q, which is not one of its own options %v", m.ID, e.Action, wr.Field, wr.Value, field.Options))
			}
		}
	}

	// The derivation half: an Action whose declared edges disagree about which Field they move leaves
	// the runtime with no answer, so it is refused here rather than resolved arbitrarily.
	fields := map[string]map[string]bool{}
	for _, t := range m.Transitions {
		if t.Action == "" {
			continue
		}
		if fields[t.Action] == nil {
			fields[t.Action] = map[string]bool{}
		}
		fields[t.Action][t.Field] = true
	}
	for actionName, moved := range fields {
		if len(moved) > 1 {
			issues = append(issues, fmt.Sprintf("machine %q: action %q is declared on transitions moving %d different fields -- which field it moves has to be unambiguous", m.ID, actionName, len(moved)))
		}
	}
	return issues
}

// stampApplicationIDs fills domain.Machine.ApplicationID from each Application's own `machines:`
// selection -- the claim is declared once, there, and this is the index over it, not a second
// declaration (001 Principle #8).
//
// It runs after validateApplicationClaims, never before: that check is what guarantees no Machine
// is claimed twice, and stamping an ambiguous claim would silently pick whichever Application
// loaded first -- which for a role-bearing Permission means resolving its role words against the
// wrong vocabulary and denying the right people. A Machine no Application claims (mch_user,
// mch_activity) keeps "", which Actor.HasRole reads as "no vocabulary here".
func stampApplicationIDs(applications []domain.Application, machines []*domain.Machine) {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}
	for _, app := range applications {
		for _, id := range app.Machines {
			if m, ok := byID[id]; ok {
				m.ApplicationID = app.ID
			}
		}
	}
}

// stampWorkflowRoles fills domain.Machine.WorkflowEngine/WorkflowRole from each Application's own
// `workflow:` block, the same derive-don't-retype relationship stampApplicationIDs has with
// `machines:` (001 Principle #8): the binding is declared once, on the Application, and this is the
// per-Machine index over it.
//
// It runs after validateWorkflowBinding has already established that every role names a Machine
// this Application claims and that no Machine holds two roles, so there is nothing ambiguous left
// to resolve here. A Machine whose Application binds no engine, or binds one without naming it,
// keeps both fields empty -- which is what action.IsDocument/IsStep read as "not mine".
func stampWorkflowRoles(applications []domain.Application, machines []*domain.Machine) {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}
	for _, app := range applications {
		if app.Workflow == nil {
			continue
		}
		for role, machineID := range app.Workflow.Roles {
			if m, ok := byID[machineID]; ok {
				m.WorkflowEngine = app.Workflow.Engine
				m.WorkflowRole = role
			}
		}
	}
}

// validatePermissionRoles is CAP-P01's cross-Machine half: a Permission's `roles:` name words
// from the vocabulary of the Application that claims its Machine, and that Application has to
// both exist and declare them.
//
// Cross-Machine, and therefore here rather than in Validate, for the same reason
// validateSequencingModes is: the two halves of the fact live in different files. The role word
// is written on the Machine; the vocabulary that gives it meaning is written on the Application.
//
// Both failures it catches are silent ones. A role nobody declares can never be held, so the
// Permission denies everyone forever while reading like a grant -- the same "protects everything"
// shape a Permission with no arm at all would have. And a role-bearing Permission on an
// *unclaimed* Machine has no vocabulary to resolve against at all (Actor.HasRole returns false for
// an empty Application id), so it denies everyone for a different reason and with even less to
// show for it. Neither would fail a single existing validator.
func validatePermissionRoles(applications []domain.Application, machines []*domain.Machine) error {
	vocabulary := make(map[string]map[string]bool, len(applications))
	for _, app := range applications {
		words := make(map[string]bool, len(app.Roles))
		for _, r := range app.Roles {
			words[r] = true
		}
		vocabulary[app.ID] = words
	}

	var issues []string
	for _, m := range machines {
		for _, p := range m.Permissions {
			if len(p.Roles) == 0 {
				continue
			}
			if m.ApplicationID == "" {
				issues = append(issues, fmt.Sprintf(
					"machine %q: permission %q names roles %v, but no application claims this machine -- a role word only means something inside one application's own vocabulary, so this permission could never be satisfied by anyone",
					m.ID, p.ID, p.Roles))
				continue
			}
			for _, role := range p.Roles {
				if !vocabulary[m.ApplicationID][role] {
					issues = append(issues, fmt.Sprintf(
						"machine %q: permission %q names role %q, which application %q does not declare in its roles: vocabulary -- nobody can hold it, so this permission would deny everyone while reading as a grant",
						m.ID, p.ID, role, m.ApplicationID))
				}
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// sortableColumnList renders domain.SortableColumns for an error message, sorted so the text is
// stable rather than map-ordered (007 §4.6 applies to diagnostics too: a message that reshuffles
// between runs is one nobody can diff).
func sortableColumnList() string {
	names := make([]string, 0, len(domain.SortableColumns))
	for name := range domain.SortableColumns {
		names = append(names, name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}
