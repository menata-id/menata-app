package metadata

import (
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

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
	boundIDPattern       = regexp.MustCompile(`^ens_[a-z][a-z0-9_]*$`)
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

// validateNavigation checks the Application's own navigation: list (development-history.md's "Navigation is
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
		if n.Badge != "" && !domain.KnownNavigationBadges[n.Badge] && !strings.HasPrefix(n.Badge, domain.NavBadgeDatasetPrefix) {
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
	// A second level is one level: a parent is another item of this list that has no parent itself (which also rules out a cycle), and a hub is not nested.
	byID := make(map[string]domain.NavigationItem, len(items))
	for _, n := range items {
		byID[n.ID] = n
	}
	for _, n := range items {
		if n.Parent == "" {
			continue
		}
		parent, ok := byID[n.Parent]
		switch {
		case n.Parent == n.ID:
			issues = append(issues, fmt.Sprintf("navigation item %q: parent names the item itself", n.ID))
		case !ok:
			issues = append(issues, fmt.Sprintf("navigation item %q: parent %q is not an item of this list", n.ID, n.Parent))
		case parent.Parent != "":
			issues = append(issues, fmt.Sprintf("navigation item %q: parent %q is itself under %q -- the menu has one level under an item, no more", n.ID, n.Parent, parent.Parent))
		case n.SettingsHub || n.SettingsHubMember || parent.SettingsHub || parent.SettingsHubMember:
			issues = append(issues, fmt.Sprintf("navigation item %q: a settings hub and its members belong to the Settings link and are not nested in the menu", n.ID))
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

	issues = append(issues, runtimeMachineIssues(m)...)

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

		if _, known := domain.KnownFieldTypes[f.Type]; !known {
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

	for _, f := range m.Fields {
		issues = append(issues, validateCompute(f, fieldsByID)...)
		issues = append(issues, validateNumbering(f, fieldsByID)...)
		issues = append(issues, validateStamp(f)...)
		issues = append(issues, validateRequiredWhen(f, fieldsByID)...)
		issues = append(issues, validateShowWhen(f, fieldsByID)...)
		issues = append(issues, validateLocks(f, fieldsByID)...)
		issues = append(issues, validateFieldCategories(m, f)...)
		if f.Currency != "" {
			if f.Type != domain.FieldTypeMoney {
				issues = append(issues, fmt.Sprintf("field %q: currency: belongs to a money field, and this is %q", f.ID, f.Type))
			} else if _, ok := domain.KnownCurrencies[f.Currency]; !ok {
				issues = append(issues, fmt.Sprintf("field %q: currency %q is not one this runtime writes (%v)", f.ID, f.Currency, slices.Sorted(maps.Keys(domain.KnownCurrencies))))
			}
		}
		issues = append(issues, validateDefaultFrom(f, fieldsByID)...)
	}
	issues = append(issues, validateDeleteGuards(m, fieldsByID)...)
	issues = append(issues, validateComputeCycles(m)...)

	if m.Frozen != nil {
		via, ok := fieldsByID[m.Frozen.Via]
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("machine %q: frozen_when.via %q is not a field of this machine", m.ID, m.Frozen.Via))
		case via.Type != domain.FieldTypeRelation:
			issues = append(issues, fmt.Sprintf("machine %q: frozen_when.via %q must be a relation field (the record this one sits under), got %q", m.ID, m.Frozen.Via, via.Type))
		}
		if m.Frozen.Field == "" || m.Frozen.Equals == "" {
			issues = append(issues, fmt.Sprintf("machine %q: frozen_when needs field and equals -- what the related record must hold", m.ID))
		}
	}
	seenBound := map[string]bool{}
	for _, b := range m.Bounds {
		issues = append(issues, validateBound(m, b, fieldsByID, seenBound)...)
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
	if m.Plural != "" && (strings.ContainsAny(m.Plural, "\n\r") || utf8.RuneCountInString(m.Plural) > maxPluralRunes) {
		issues = append(issues, fmt.Sprintf("machine %q: plural %q is a word or two on one line, at most %d characters", m.ID, m.Plural, maxPluralRunes))
	}
	if m.Completion != nil {
		f, ok := fieldsByID[m.Completion.Field]
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("machine %q: completion field %q is not a field of this machine", m.ID, m.Completion.Field))
		case f.Type != domain.FieldTypeStatus:
			issues = append(issues, fmt.Sprintf("machine %q: completion field %q must be a status field, got %q", m.ID, m.Completion.Field, f.Type))
		case !slices.Contains(f.Options, m.Completion.Done):
			issues = append(issues, fmt.Sprintf("machine %q: completion done %q is not one of %q's options %v", m.ID, m.Completion.Done, f.ID, f.Options))
		}
	}
	issues = append(issues, validateStatusCategories(m, fieldsByID)...)
	for _, cf := range m.CardFields {
		if _, ok := fieldsByID[cf.Field]; !ok {
			issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q is not a field of this machine", m.ID, cf.Field))
		}
		if !domain.KnownCardFieldRoles[cf.Role] {
			issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q has unknown role %q", m.ID, cf.Field, cf.Role))
		}
		if f, ok := fieldsByID[cf.Field]; ok && cf.Role == domain.CardFieldRoleFile && f.Type != domain.FieldTypeFile {
			issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q has the file role, so it must be a file field, got %q", m.ID, cf.Field, f.Type))
		}
		if f, ok := fieldsByID[cf.Field]; ok && cf.Role == domain.CardFieldRoleContainer && f.Type != domain.FieldTypeRelation {
			issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q has the container role, so it must be a relation field, got %q", m.ID, cf.Field, f.Type))
		}
		if f, ok := fieldsByID[cf.Field]; ok && cf.Role == domain.CardFieldRoleLink && f.Type != domain.FieldTypeText {
			issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q has the link role, so it must be a text field holding a route, got %q", m.ID, cf.Field, f.Type))
		}
		if f, ok := fieldsByID[cf.Field]; ok && cf.Role == domain.CardFieldRoleComment {
			if f.Type != domain.FieldTypeLongText {
				issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q has the comment role, so it must be a long_text field, got %q", m.ID, cf.Field, f.Type))
			}
			if m.StampedAuthorField() == "" {
				issues = append(issues, fmt.Sprintf("machine %q: card_fields entry %q has the comment role, so the machine needs a person field declaring `stamp: current_user` to say who wrote it", m.ID, cf.Field))
			}
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
	if strings.ContainsAny(v.Name, "\r\n") {
		// The view switch is a SegmentedControl, whose options are one `value=Label` per line: a name holding a line
		// break would split into a second, empty choice.
		issues = append(issues, fmt.Sprintf("machine %q: view %q has a line break in its name -- a view name is one line", m.ID, v.ID))
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
		group, ok := fieldsByID[v.GroupBy]
		if !ok {
			issues = append(issues, fmt.Sprintf("machine %q: view %q is a board, so group_by %q must be a field of this machine", m.ID, v.ID, v.GroupBy))
		}
		if (len(v.ColumnActions) > 0 || v.AddColumn != nil) && ok && !group.IsReference() {
			issues = append(issues, fmt.Sprintf("machine %q: view %q: column_actions and add_column are for a board whose columns are records of another Machine, and group_by %q is not a relation", m.ID, v.ID, v.GroupBy))
		}
		seenAction := map[domain.ColumnAction]bool{}
		for _, a := range v.ColumnActions {
			switch {
			case !domain.KnownColumnActions[a]:
				issues = append(issues, fmt.Sprintf("machine %q: view %q: column action %q is not rename, move or delete", m.ID, v.ID, a))
			case seenAction[a]:
				issues = append(issues, fmt.Sprintf("machine %q: view %q: column action %q is declared twice", m.ID, v.ID, a))
			}
			seenAction[a] = true
		}
		if a := v.AddColumn; a != nil && (strings.TrimSpace(a.To) == "" || strings.TrimSpace(a.Label) == "" || strings.ContainsAny(a.Label+a.Note, "\r\n")) {
			issues = append(issues, fmt.Sprintf("machine %q: view %q: add_column names `to:` (a navigation item) and a one-line `label:`, and a one-line `note:` when it has one", m.ID, v.ID))
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
	guardValue, guardKey := s.GuardEquals, "guard_equals"
	if s.GuardNotEquals != "" {
		guardValue, guardKey = s.GuardNotEquals, "guard_not_equals"
	}
	switch {
	case s.GuardEquals != "" && s.GuardNotEquals != "":
		issues = append(issues, fmt.Sprintf("event %q: schedule.guard_equals and schedule.guard_not_equals are two polarities of one guard -- declare one", e.ID))
	case (s.GuardField == "") != (guardValue == ""):
		issues = append(issues, fmt.Sprintf("event %q: schedule.guard_field and schedule.guard_equals (or guard_not_equals) must be set together or not at all", e.ID))
	case s.GuardField != "":
		guardField, ok := fieldsByID[s.GuardField]
		if !ok {
			issues = append(issues, fmt.Sprintf("event %q: schedule.guard_field %q is not a field of machine %q", e.ID, s.GuardField, m.ID))
		} else if guardField.ViolatesOptions(guardValue) {
			issues = append(issues, fmt.Sprintf("event %q: schedule.%s %q is not one of field %q's options %v", e.ID, guardKey, guardValue, s.GuardField, guardField.Options))
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
	case p.ActorField == "" && p.DynamicActor == nil && p.ParentActor == nil && len(p.Roles) == 0 && p.WorkspaceRole == "":
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
	issues = append(issues, validateParentActorVia(m, p, fieldsByID)...)

	return issues
}

// validateTransitionActor checks the two keys that say who may take an edge. They are enforced by the generic
// update route, which is the `edit` Action's, so an edge any other Action (or no Action) performs may not declare
// them: a key nothing reads would look like a guard and protect nothing. Role words are checked against the
// Application's vocabulary later, with the Permissions' (validatePermissionRoles).
func validateTransitionActor(m *domain.Machine, t domain.Transition, fieldsByID map[string]domain.Field) []string {
	var issues []string
	if (len(t.Roles) > 0 || t.ActorField != "") && t.Action != domain.ActionEdit {
		issues = append(issues, fmt.Sprintf("machine %q: transition %q: roles and actor_field gate the generic update route, so only an edge with action %q may declare them, not %q", m.ID, t.ID, domain.ActionEdit, t.Action))
	}
	for i, role := range t.Roles {
		if strings.TrimSpace(role) == "" {
			issues = append(issues, fmt.Sprintf("machine %q: transition %q: roles entry %d is empty -- omit it rather than declaring a blank role, which no vocabulary contains", m.ID, t.ID, i))
		}
	}
	if len(t.Sets) > 0 && t.Action != domain.ActionEdit {
		issues = append(issues, fmt.Sprintf("machine %q: transition %q: sets is written by the generic update route, so only an edge with action %q may declare it, not %q", m.ID, t.ID, domain.ActionEdit, t.Action))
	}
	for fieldID, source := range t.Sets {
		f, ok := fieldsByID[fieldID]
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("machine %q: transition %q: sets names %q, which is not a field of this machine", m.ID, t.ID, fieldID))
		case !domain.KnownTransitionSources[source]:
			issues = append(issues, fmt.Sprintf("machine %q: transition %q: sets %q from %q, which is not a source this runtime knows (%v)", m.ID, t.ID, fieldID, source, sortedTransitionSources()))
		case source == domain.TransitionSourceToday && f.Type != domain.FieldTypeDate:
			issues = append(issues, fmt.Sprintf("machine %q: transition %q: sets %q from today, but it is a %s field, not a date", m.ID, t.ID, fieldID, f.Type))
		case f.Compute != nil || f.Stamp != "":
			issues = append(issues, fmt.Sprintf("machine %q: transition %q: sets %q, which the runtime already writes another way (compute or stamp)", m.ID, t.ID, fieldID))
		}
	}
	if t.ActorField != "" {
		f, ok := fieldsByID[t.ActorField]
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("machine %q: transition %q: actor_field %q is not a field of this machine", m.ID, t.ID, t.ActorField))
		case !f.IsReference():
			issues = append(issues, unboundActorField(t.ID, "actor_field", t.ActorField, f))
		}
	}
	return issues
}

func sortedTransitionSources() []string {
	out := make([]string, 0, len(domain.KnownTransitionSources))
	for s := range domain.KnownTransitionSources {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
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

	issues = append(issues, validateTransitionActor(m, t, fieldsByID)...)

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
// viewerScopedIssues holds a Dataset that declares `viewer_scoped: true` to what it says: it selects records, and
// it carries a top-level `equals $current_user` comparison on a person Field. Only a top-level `all:` equality
// scopes -- a sentinel inside `any:` widens the result to everyone's records and `not_equals` inverts it, both
// of which satisfied the earlier named-list gate. Nothing here says the opposite (a Dataset with the predicate
// and no flag is allowed): the flag is the author's claim, and the loader's job is to make the claim true.
func viewerScopedIssues(m *domain.Machine, ds domain.Dataset, fieldsByID map[string]domain.Field) []string {
	if !ds.ViewerScoped {
		return nil
	}
	if ds.Select != domain.SelectRecords {
		return []string{fmt.Sprintf("dataset %q: `viewer_scoped: true` says which records a list shows; an aggregating Dataset shows none -- declare `select: records` or drop it", ds.ID)}
	}
	if ds.Where != nil {
		for _, c := range ds.Where.All {
			if c.Value != expression.SentinelCurrentUser || c.Op != expression.OpEquals || c.IfParam != "" || c.Optional {
				continue
			}
			f, ok := fieldsByID[c.Field]
			if ok && f.Type == domain.FieldTypePerson {
				return nil
			}
			if ok {
				return []string{fmt.Sprintf("dataset %q: `viewer_scoped: true` compares %s with %s, but %q is %q -- the viewer is a person, so the field must be a person field", ds.ID, expression.SentinelCurrentUser, c.Field, c.Field, f.Type)}
			}
		}
	}
	return []string{fmt.Sprintf("dataset %q: `viewer_scoped: true` but no top-level `%s` comparison in `where:` -- without it the list shows everyone's records (a sentinel inside `any:` widens it and `not_equals` inverts it, so neither counts)", ds.ID, expression.SentinelCurrentUser)}
}

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
		if msg := optionalAnyIssue(ds.Where); msg != "" {
			issues = append(issues, fmt.Sprintf("dataset %q: %s", ds.ID, msg))
		}
		if ds.Where != nil {
			for _, c := range ds.Where.Any {
				if c.IfParam != "" {
					issues = append(issues, fmt.Sprintf("dataset %q: where any: holds a conditional comparison (if_param), which would drop the group's alternatives and widen the list; a named view's comparisons sit in all:", ds.ID))
					break
				}
			}
		}
		for _, c := range ds.Where.Comparisons() {
			if c.Field == domain.RecordIDField {
				if c.Op != expression.OpEquals || c.Optional {
					issues = append(issues, fmt.Sprintf("dataset %q: where field %q is the record's own id, which a Dataset compares with equals only and never leaves out", ds.ID, c.Field))
				}
				// The id comes from the request (a record page's $parameters.id): a literal would name one record of one
				// Workspace, and a mistyped sentinel would answer 500 on every visit.
				if !expression.IsParameter(c.Value) {
					issues = append(issues, fmt.Sprintf("dataset %q: where field %q compares with a request parameter ($parameters.<name>), not %q", ds.ID, c.Field, c.Value))
				}
				continue
			}
			if _, ok := fieldsByID[c.Field]; !ok {
				issues = append(issues, fmt.Sprintf("dataset %q: where field %q is not a field of machine %q", ds.ID, c.Field, m.ID))
			}
			if !expression.KnownOps[c.Op] {
				issues = append(issues, fmt.Sprintf("dataset %q: where op %q is not a known operator", ds.ID, c.Op))
			}
			if f, ok := fieldsByID[c.Field]; ok {
				for _, msg := range orderedComparisonIssues(f, c) {
					issues = append(issues, fmt.Sprintf("dataset %q: where %s", ds.ID, msg))
				}
			}
			if msg := emptyComparisonIssue(c); msg != "" {
				issues = append(issues, fmt.Sprintf("dataset %q: where %s", ds.ID, msg))
			}
			if c.IfParam != "" {
				name, value, ok := strings.Cut(c.IfParam, "=")
				if !ok || name == "" || value == "" || domain.ParamReserved(name) || strings.ContainsAny(name+value, " &?#") {
					issues = append(issues, fmt.Sprintf("dataset %q: where if_param %q is <parameter>=<value>, a plain word the runtime does not own", ds.ID, c.IfParam))
				}
				// A named view narrows a list: its comparisons are literals about the record (a status, a date window,
				// a count). One about the viewer or the request would turn off a scope when the view is not chosen (the
				// review of step 4 turned a My Tasks list into everyone's), and one on the record's id is a record page's.
				if expression.IsSentinel(c.Value) && !expression.IsToday(c.Value) || c.Field == domain.RecordIDField || c.Optional {
					issues = append(issues, fmt.Sprintf("dataset %q: where field %q is conditional (if_param), so it compares a literal or a $today day -- never the viewer, a request parameter, or the record's id", ds.ID, c.Field))
				}
			}
			if c.Optional && !expression.IsParameter(c.Value) {
				issues = append(issues, fmt.Sprintf("dataset %q: where field %q is optional, which means \"left out when the request does not send it\" -- so its value must be a %s<name>, not %q", ds.ID, c.Field, expression.SentinelParameterPrefix, c.Value))
			}
			// A parameter the runtime sets itself (the Application a page is drawn in, a record page's id, the week) is a
			// scope, not a filter a person chooses: left out, it would list every Application's rows.
			if name, ok := strings.CutPrefix(c.Value, expression.SentinelParameterPrefix); c.Optional && ok && domain.ParamReserved(name) {
				issues = append(issues, fmt.Sprintf("dataset %q: where field %q is optional on %s, which the runtime sets as a scope -- a scope is never left out", ds.ID, c.Field, c.Value))
			}
			if f, ok := fieldsByID[c.Field]; ok {
				if msg := containsIssue(f, c); msg != "" {
					issues = append(issues, fmt.Sprintf("dataset %q: where %s", ds.ID, msg))
				}
			}
			// 007 §9.2: "Access outside this context must fail closed." A sentinel the runtime cannot
			// resolve must be refused here, because at runtime it would become a literal -- a filter
			// comparing a Field against the string "$current_usr" matches nothing, renders an empty
			// list, and is indistinguishable from a screen that legitimately has no rows.
			if c.Value == expression.SentinelDone {
				issues = append(issues, fmt.Sprintf("dataset %q: where %s", ds.ID, doneIssue(m, c)))
			} else if expression.IsSentinel(c.Value) && !expression.KnownSentinel(c.Value) {
				issues = append(issues, fmt.Sprintf("dataset %q: where value %q is not a runtime context value this runtime resolves (%s, %s[+-n], %s[+-n], %s, %s, %s<name>) -- an unknown one would be compared as a literal and silently match nothing", ds.ID, c.Value, expression.SentinelCurrentUser, expression.SentinelToday, expression.SentinelWeek, expression.SentinelMonth, expression.SentinelMonthEnd, expression.SentinelParameterPrefix))
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

	issues = append(issues, viewerScopedIssues(m, ds, fieldsByID)...)

	for _, k := range ds.Sort {
		_, isColumn := domain.SortableColumns[k.Field]
		if _, isField := fieldsByID[k.Field]; !isField && !isColumn {
			issues = append(issues, fmt.Sprintf("dataset %q: sort field %q is neither a field of machine %q nor a record column (%s)", ds.ID, k.Field, m.ID, sortableColumnList()))
		}
		if k.By != "" {
			if f, isField := fieldsByID[k.Field]; !isField || !f.IsReference() {
				issues = append(issues, fmt.Sprintf("dataset %q: sort by: %q orders by a column of the related record, so field: %q must be a reference Field (a relation or a person)", ds.ID, k.By, k.Field))
			} else if _, ok := domain.SortableColumns[k.By]; !ok {
				issues = append(issues, fmt.Sprintf("dataset %q: sort by: %q is not a record column (%s)", ds.ID, k.By, sortableColumnList()))
			}
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
			} else if !f.Type.IsNumeric() {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q sums field %q, which must be a number or money field, got %q", ds.ID, ms.ID, ms.Field, f.Type))
			}
		case domain.AggregateCount:
			if ms.Field != "" {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q is a count, which counts records, so field %q must not be set", ds.ID, ms.ID, ms.Field))
			}
		case domain.AggregateCountDistinct:
			if _, ok := fieldsByID[ms.Field]; !ok {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q counts the distinct values of a field, so field %q must be a field of machine %q", ds.ID, ms.ID, ms.Field, m.ID))
			}
		case domain.AggregatePercentage:
			issues = append(issues, percentageIssues(ds, ms)...)
		}
		if ms.Aggregate != domain.AggregatePercentage && (ms.Numerator != "" || ms.Denominator != "") {
			issues = append(issues, fmt.Sprintf("dataset %q: measure %q: numerator/denominator belong to a percentage, and this is a %s", ds.ID, ms.ID, ms.Aggregate))
		}

		for _, c := range ms.Where.Comparisons() {
			whereField, ok := fieldsByID[c.Field]
			if !ok {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.field %q is not a field of machine %q", ds.ID, ms.ID, c.Field, m.ID))
			} else if c.Op != expression.OpIsEmpty && !expression.IsSentinel(c.Value) && whereField.ViolatesOptions(c.Value) {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.value %q is not one of field %q's options %v", ds.ID, ms.ID, c.Value, c.Field, whereField.Options))
			}
			if !expression.KnownOps[c.Op] {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.op %q is not a known operator", ds.ID, ms.ID, c.Op))
			}
			if msg := emptyComparisonIssue(c); msg != "" {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where %s", ds.ID, ms.ID, msg))
			}
			if c.IfParam != "" {
				issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where field %q is if_param, which names a view of a list and an aggregate has none", ds.ID, ms.ID, c.Field))
			}
			// An optional comparison is how a count follows the same request parameter a list does (a view group's summary
			// counts the tasks of the project the Board shows): left out when the request does not send it.
			if c.Optional {
				if !expression.IsParameter(c.Value) {
					issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where field %q is optional, so its value must be a %s<name>, not %q", ds.ID, ms.ID, c.Field, expression.SentinelParameterPrefix, c.Value))
				} else if name := strings.TrimPrefix(c.Value, expression.SentinelParameterPrefix); domain.ParamReserved(name) {
					issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where field %q is optional on %s, which the runtime sets as a scope -- a scope is never left out", ds.ID, ms.ID, c.Field, c.Value))
				}
			}
			if ok {
				if msg := containsIssue(whereField, c); msg != "" {
					issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where %s", ds.ID, ms.ID, msg))
				}
			}
			if ok {
				switch {
				case c.Value == expression.SentinelDone:
					issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where %s", ds.ID, ms.ID, doneIssue(m, c)))
				case expression.IsSentinel(c.Value) && !expression.IsToday(c.Value) && !(c.Optional && expression.IsParameter(c.Value)):
					issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where.value %q names a runtime context value an aggregate cannot resolve -- it has no viewer, and a request parameter only in an `optional: true` comparison (left out when the request sends none), otherwise only %s and %s", ds.ID, ms.ID, c.Value, expression.SentinelToday, expression.SentinelDone))
				default:
					for _, msg := range orderedComparisonIssues(whereField, c) {
						issues = append(issues, fmt.Sprintf("dataset %q: measure %q: where %s", ds.ID, ms.ID, msg))
					}
				}
			}
		}
	}

	return issues
}

// percentageIssues is what a derived percentage asks: two Measures of its own Dataset, neither derived itself (so
// evaluation is one pass, and bounded -- 007 §7.4), and nothing of its own to count or filter.
func percentageIssues(ds domain.Dataset, ms domain.Measure) []string {
	var issues []string
	if ms.Field != "" || ms.Where != nil {
		issues = append(issues, fmt.Sprintf("dataset %q: measure %q is a percentage of two other measures, so it takes no field: and no where: of its own", ds.ID, ms.ID))
	}
	for role, id := range map[string]string{"numerator": ms.Numerator, "denominator": ms.Denominator} {
		var found *domain.Measure
		for i := range ds.Measures {
			if ds.Measures[i].ID == id {
				found = &ds.Measures[i]
			}
		}
		switch {
		case id == "":
			issues = append(issues, fmt.Sprintf("dataset %q: measure %q is a percentage and names no %s:", ds.ID, ms.ID, role))
		case found == nil:
			issues = append(issues, fmt.Sprintf("dataset %q: measure %q: %s %q is not a measure of this dataset", ds.ID, ms.ID, role, id))
		case found.Aggregate == domain.AggregatePercentage:
			issues = append(issues, fmt.Sprintf("dataset %q: measure %q: %s %q is itself a percentage -- a percentage is derived from counted or summed measures only", ds.ID, ms.ID, role, id))
		}
	}
	sort.Strings(issues)
	return issues
}

// doneIssue explains why a `$done` survived Normalize, which stamps every one it can answer: this Machine
// declares no `completion:`, or the comparison is on some other Field, or its operator is not an equality.
// Reaching here means the value would otherwise be compared as the literal string "$done" and match nothing.
func doneIssue(m *domain.Machine, c expression.Comparison) string {
	switch {
	case m.Completion == nil:
		return fmt.Sprintf("value %s means \"the finished value\", but machine %q declares no `completion:` to read it from", expression.SentinelDone, m.ID)
	case c.Field != m.Completion.Field:
		return fmt.Sprintf("value %s is the finished value of field %q (the machine's `completion:`), not of %q", expression.SentinelDone, m.Completion.Field, c.Field)
	default:
		return fmt.Sprintf("value %s is a state, so only `equals` and `not_equals` can ask about it, not %q", expression.SentinelDone, c.Op)
	}
}

// orderedComparisonIssues checks what an ordered operator (`lt`/`lte`/`gt`/`gte`) is asked to order. It
// speaks only of the Field and the value, so each caller prefixes where the comparison was found.
//
// Only a number or a date has an order worth declaring: a status orders by the sequence its options happen
// to be written in, and text by an alphabet nobody chose. The value must then be one the Field could hold --
// a literal date for a date, a number for a number -- or a context value (`$today` for a date only, a
// `$parameters.<name>` for either, which cannot be checked until a request supplies it). `$today` on a Field
// that is not a date is refused whatever the operator, since a date compared against a status is never meant.
// emptyComparisonIssue refuses a value beside `is_empty`: the operator asks whether the Field has one, so a value
// is either a mistake for `equals` or a comparison that silently ignores half of what it says.
func emptyComparisonIssue(c expression.Comparison) string {
	if c.Op == expression.OpIsEmpty && c.Value != "" {
		return fmt.Sprintf("op %q asks whether field %q has a value, so it takes none (got %q)", c.Op, c.Field, c.Value)
	}
	return ""
}

// optionalAnyIssue refuses an `any:` group that mixes optional and required comparisons. An optional one left out
// narrows nothing, but sent, it is OR-ed with the others -- so `any: [{owner equals $current_user}, {title contains
// $parameters.q, optional}]` would list other people's records the moment someone searched (the review of step 1). In
// one group, either every comparison is optional (a search over several Fields) or none is.
func optionalAnyIssue(p *expression.Predicate) string {
	if p == nil || len(p.Any) == 0 {
		return ""
	}
	optional := 0
	for _, c := range p.Any {
		if c.Optional {
			optional++
		}
	}
	if optional > 0 && optional < len(p.Any) {
		return "where any: mixes optional and required comparisons -- searching would widen the list past the required ones; make every one optional (a search over several Fields) or none"
	}
	return ""
}

// containsIssue is what `contains` asks: words to look for, in a Field that holds text.
func containsIssue(f domain.Field, c expression.Comparison) string {
	if c.Op != expression.OpContains {
		return ""
	}
	switch {
	case f.Type != domain.FieldTypeText && f.Type != domain.FieldTypeLongText:
		return fmt.Sprintf("op %q looks for words in text, and field %q is type %q", c.Op, f.ID, f.Type)
	case c.Value == "":
		return fmt.Sprintf("op %q on field %q has no words to look for", c.Op, f.ID)
	}
	return ""
}

func orderedComparisonIssues(f domain.Field, c expression.Comparison) []string {
	var issues []string
	if expression.IsDate(c.Value) && f.Type != domain.FieldTypeDate {
		issues = append(issues, fmt.Sprintf("value %s is a date, but field %q is type %q", c.Value, f.ID, f.Type))
	}
	if !expression.IsOrdered(c.Op) {
		return issues
	}
	if !f.Type.IsNumeric() && f.Type != domain.FieldTypeDate {
		return append(issues, fmt.Sprintf("op %q orders a number or a date, and field %q is type %q", c.Op, f.ID, f.Type))
	}
	if expression.IsSentinel(c.Value) {
		return issues
	}
	switch f.Type {
	case domain.FieldTypeNumber, domain.FieldTypeMoney:
		if !expression.IsNumber(c.Value) {
			issues = append(issues, fmt.Sprintf("value %q is not a number, and field %q is one", c.Value, f.ID))
		}
	case domain.FieldTypeDate:
		if _, err := time.Parse("2006-01-02", c.Value); err != nil {
			issues = append(issues, fmt.Sprintf("value %q is not a YYYY-MM-DD date, and field %q is one", c.Value, f.ID))
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
// maxPluralRunes bounds a Machine's `plural:` -- a word or two, not a sentence a board would put in its summary.
const maxPluralRunes = 40

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
	for _, app := range applications {
		for _, n := range app.AllNavigation {
			for _, role := range n.VisibleTo {
				if !vocabulary[app.ID][role] {
					issues = append(issues, fmt.Sprintf("application %q: navigation item %q visible_to names role %q, which the application does not declare in its roles: vocabulary -- nobody can hold it, so nobody would be offered the item", app.ID, n.ID, role))
				}
			}
		}
	}
	// A counting badge names a Dataset of the Application's own Machines that lists records (a count of an aggregate has no meaning).
	for _, app := range applications {
		for _, n := range app.AllNavigation {
			if !strings.HasPrefix(n.Badge, domain.NavBadgeDatasetPrefix) || domain.KnownNavigationBadges[n.Badge] {
				continue
			}
			found := false
			for _, m := range machines {
				if m.ApplicationID != app.ID {
					continue
				}
				for _, ds := range m.Datasets {
					found = found || (ds.ID == n.Badge && ds.Select == domain.SelectRecords)
				}
			}
			if !found {
				issues = append(issues, fmt.Sprintf("application %q: navigation item %q badge %q is not a Dataset of the application's Machines that selects records, so there is nothing to count", app.ID, n.ID, n.Badge))
			}
		}
	}
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
	for _, m := range machines {
		editable := map[string]bool{}
		for _, f := range m.Fields {
			if len(f.EditableBy) > 0 {
				editable[f.ID] = true
			}
		}
		for _, t := range m.Transitions {
			for field := range t.Sets {
				if editable[field] {
					issues = append(issues, fmt.Sprintf("machine %q: transition %q sets field %q, which names who may change it (editable_by) -- the edge would be refused for everyone who lacks that role; let the edge write a field anyone may change", m.ID, t.ID, field))
				}
			}
		}
		for _, f := range m.Fields {
			for _, role := range f.EditableBy {
				switch {
				case m.ApplicationID == "":
					issues = append(issues, fmt.Sprintf("machine %q: field %q names roles %v in editable_by, but no application claims this machine -- a role word only means something inside one application's own vocabulary", m.ID, f.ID, f.EditableBy))
				case !vocabulary[m.ApplicationID][role]:
					issues = append(issues, fmt.Sprintf("machine %q: field %q editable_by names role %q, which application %q does not declare in its roles: vocabulary -- nobody can hold it, so nobody could change this field", m.ID, f.ID, role, m.ApplicationID))
				}
			}
		}
	}
	for _, m := range machines {
		for _, e := range m.Events {
			role := ""
			if e.Then.Notify != nil {
				role = e.Then.Notify.RecipientRole
			}
			switch {
			case role == "":
			case m.ApplicationID == "":
				issues = append(issues, fmt.Sprintf("machine %q: event %q tells role %q, but no application claims this machine -- a role word only means something inside one application's own vocabulary", m.ID, e.ID, role))
			case !vocabulary[m.ApplicationID][role]:
				issues = append(issues, fmt.Sprintf("machine %q: event %q tells role %q, which application %q does not declare in its roles: vocabulary -- nobody holds it, so no one would be told", m.ID, e.ID, role, m.ApplicationID))
			}
		}
		for _, t := range m.Transitions {
			for _, role := range t.Roles {
				switch {
				case m.ApplicationID == "":
					issues = append(issues, fmt.Sprintf("machine %q: transition %q names roles %v, but no application claims this machine -- a role word only means something inside one application's own vocabulary", m.ID, t.ID, t.Roles))
				case !vocabulary[m.ApplicationID][role]:
					issues = append(issues, fmt.Sprintf("machine %q: transition %q names role %q, which application %q does not declare in its roles: vocabulary -- nobody can hold it, so this edge would be taken by no one", m.ID, t.ID, role, m.ApplicationID))
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

// validateBound holds an aggregate bound to what its check assumes: an id, a number Field to add up (a computed one is
// fine, it is evaluated before the check), a Field of this Machine to group by (not the summed one), and at least one
// bound in the right order. A bound that can never be violated reads as a rule and protects nothing, so an empty
// range is refused.
func validateBound(m *domain.Machine, b domain.AggregateBound, fieldsByID map[string]domain.Field, seen map[string]bool) []string {
	var issues []string
	if !boundIDPattern.MatchString(b.ID) {
		issues = append(issues, fmt.Sprintf("machine %q: ensure id %q must match %s", m.ID, b.ID, boundIDPattern.String()))
	}
	if seen[b.ID] {
		issues = append(issues, fmt.Sprintf("machine %q: ensure id %q is declared more than once", m.ID, b.ID))
	}
	seen[b.ID] = true
	sum, ok := fieldsByID[b.SumOf]
	switch {
	case !ok:
		issues = append(issues, fmt.Sprintf("machine %q: ensure %q: sum_of %q is not a field of this machine", m.ID, b.ID, b.SumOf))
	case !sum.Type.IsNumeric():
		issues = append(issues, fmt.Sprintf("machine %q: ensure %q: sum_of %q must be a number or money field, got %q", m.ID, b.ID, b.SumOf, sum.Type))
	}
	switch per, ok := fieldsByID[b.Per]; {
	case !ok:
		issues = append(issues, fmt.Sprintf("machine %q: ensure %q: per %q is not a field of this machine", m.ID, b.ID, b.Per))
	case b.Per == b.SumOf:
		issues = append(issues, fmt.Sprintf("machine %q: ensure %q: per and sum_of name the same field", m.ID, b.ID))
	case per.Type.IsNumeric() || per.Type == domain.FieldTypeDate || per.Type == domain.FieldTypeFile || per.Type == domain.FieldTypeBoolean:
		issues = append(issues, fmt.Sprintf("machine %q: ensure %q: per %q must group records by a text, status, person or relation field, got %q", m.ID, b.ID, b.Per, per.Type))
	}
	if p := b.AtMostParent; p != nil {
		if per, ok := fieldsByID[b.Per]; p.Via != b.Per || !ok || per.Type != domain.FieldTypeRelation {
			issues = append(issues, fmt.Sprintf("machine %q: ensure %q: at_most: parent: goes through %q, and it must be the relation the bound groups by (per: %q)", m.ID, b.ID, p.Via, b.Per))
		}
	}
	switch {
	case b.AtLeast == nil && b.AtMost == nil && b.AtMostParent == nil:
		issues = append(issues, fmt.Sprintf("machine %q: ensure %q: declares neither at_least nor at_most, so it bounds nothing", m.ID, b.ID))
	case b.AtLeast != nil && b.AtMost != nil && *b.AtLeast > *b.AtMost:
		issues = append(issues, fmt.Sprintf("machine %q: ensure %q: at_least %v is above at_most %v, so no total could satisfy it", m.ID, b.ID, *b.AtLeast, *b.AtMost))
	}
	return issues
}

// validateFieldCategories holds a status Field's own `categories:` to its options and the five categories, and refuses
// it on the Field the Machine's `status_categories:` already categorises (one statement per Field).
func validateFieldCategories(m *domain.Machine, f domain.Field) []string {
	if len(f.Categories) == 0 {
		return nil
	}
	var issues []string
	switch {
	case f.Type != domain.FieldTypeStatus:
		return []string{fmt.Sprintf("field %q: categories: belong to a status field, and this is %q", f.ID, f.Type)}
	case m.StatusCategories != nil && m.StatusCategories.Field == f.ID:
		return []string{fmt.Sprintf("field %q: categories: and the machine's status_categories: both categorise it -- say it once", f.ID)}
	}
	for _, option := range slices.Sorted(maps.Keys(f.Categories)) {
		if !slices.Contains(f.Options, option) {
			issues = append(issues, fmt.Sprintf("field %q: categories: %q is not one of its options %v", f.ID, option, f.Options))
		}
		if !domain.KnownStatusCategories[f.Categories[option]] {
			issues = append(issues, fmt.Sprintf("field %q: categories: %q has unknown category %q (want start, active, attention, done or stopped)", f.ID, option, f.Categories[option]))
		}
	}
	return issues
}

// validateLocks holds `locked_when`, `set_once` and `on_parent_delete` to what their enforcement assumes: a lock on a
// Field a person writes, compared against a status Field's own option, and a parent rule on a relation only.
func validateLocks(f domain.Field, fieldsByID map[string]domain.Field) []string {
	var issues []string
	if (f.LockedWhen != nil || f.SetOnce) && (f.Compute != nil || f.Stamp != "") {
		issues = append(issues, fmt.Sprintf("field %q: a computed or stamped field is written by the runtime, so locked_when/set_once would lock nothing a person writes", f.ID))
	}
	// A checkbox sends nothing when unticked, so a form cannot send a stored false back unchanged: the record could never
	// be saved again.
	if len(f.EditableBy) > 0 && (f.Compute != nil || f.Stamp != "" || f.Numbering != nil) {
		issues = append(issues, fmt.Sprintf("field %q: a computed, stamped or numbered field is written by the runtime, so editable_by would restrict nothing a person writes", f.ID))
	}
	if (f.LockedWhen != nil || f.SetOnce) && f.Type == domain.FieldTypeBoolean {
		issues = append(issues, fmt.Sprintf("field %q: a boolean cannot be locked -- an unticked box sends nothing, so an edit could never carry it back unchanged", f.ID))
	}
	if lw := f.LockedWhen; lw != nil {
		other, ok := fieldsByID[lw.Field]
		switch {
		case lw.Field == f.ID:
			issues = append(issues, fmt.Sprintf("field %q: locked_when names itself -- use set_once for a value that never changes", f.ID))
		case !ok:
			issues = append(issues, fmt.Sprintf("field %q: locked_when.field %q is not a field of this machine", f.ID, lw.Field))
		case other.Type != domain.FieldTypeStatus:
			issues = append(issues, fmt.Sprintf("field %q: locked_when.field %q must be a status field, got %q", f.ID, lw.Field, other.Type))
		case !slices.Contains(other.Options, lw.Equals):
			issues = append(issues, fmt.Sprintf("field %q: locked_when.equals %q is not one of %q's options %v -- it would lock nothing", f.ID, lw.Equals, lw.Field, other.Options))
		}
	}
	switch {
	case f.OnParentDelete == "":
	case f.Type != domain.FieldTypeRelation:
		issues = append(issues, fmt.Sprintf("field %q: on_parent_delete belongs to a relation field, and this is %q", f.ID, f.Type))
	case f.OnParentDelete != domain.ParentDeleteRestrict:
		issues = append(issues, fmt.Sprintf("field %q: on_parent_delete %q is not built -- the one rule today is %q", f.ID, f.OnParentDelete, domain.ParentDeleteRestrict))
	}
	return issues
}

// validateDeleteGuards holds `deletable_unless:` to a status Field of the Machine and one of its options.
func validateDeleteGuards(m *domain.Machine, fieldsByID map[string]domain.Field) []string {
	var issues []string
	for _, g := range m.DeletableUnless {
		f, ok := fieldsByID[g.Field]
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("deletable_unless.field %q is not a field of this machine", g.Field))
		case f.Type != domain.FieldTypeStatus:
			issues = append(issues, fmt.Sprintf("deletable_unless.field %q must be a status field, got %q", g.Field, f.Type))
		case !slices.Contains(f.Options, g.Equals):
			issues = append(issues, fmt.Sprintf("deletable_unless.equals %q is not one of %q's options %v -- it would refuse nothing", g.Equals, g.Field, f.Options))
		}
	}
	return issues
}

// validateRequiredWhen holds a conditionally required Field to what its enforcement assumes: another status
// Field of this Machine to compare, one of that Field's own options to compare with (a value it can never hold
// would never require anything, which reads as a rule), and no second statement of "required" beside it.
func validateRequiredWhen(f domain.Field, fieldsByID map[string]domain.Field) []string {
	rw := f.RequiredWhen
	if rw == nil {
		return nil
	}
	var issues []string
	if f.Required {
		issues = append(issues, fmt.Sprintf("field %q: required and required_when together -- it is either always required or only while %q holds a value", f.ID, rw.Field))
	}
	if f.Compute != nil || f.Stamp != "" {
		issues = append(issues, fmt.Sprintf("field %q: a computed or stamped field is written by the runtime, so required_when could never ask a person for it", f.ID))
	}
	other, ok := fieldsByID[rw.Field]
	switch {
	case rw.Field == f.ID:
		issues = append(issues, fmt.Sprintf("field %q: required_when names itself", f.ID))
	case !ok:
		issues = append(issues, fmt.Sprintf("field %q: required_when.field %q is not a field of this machine", f.ID, rw.Field))
	case other.Type != domain.FieldTypeStatus:
		issues = append(issues, fmt.Sprintf("field %q: required_when.field %q must be a status field, got %q -- a comparison needs a closed set of values", f.ID, rw.Field, other.Type))
	case !slices.Contains(other.Options, rw.Equals):
		issues = append(issues, fmt.Sprintf("field %q: required_when.equals %q is not one of %q's own options %v -- it would never be required", f.ID, rw.Equals, rw.Field, other.Options))
	}
	return issues
}

// validateShowWhen holds a conditionally shown Field to what its form assumes: another status Field of this Machine to watch and one of its own options (a value it
// can never hold would hide the Field forever), a Field a person writes, and no required_when beside it (one says when it is asked, the other when it is demanded).
func validateShowWhen(f domain.Field, fieldsByID map[string]domain.Field) []string {
	sw := f.ShowWhen
	if sw == nil {
		return nil
	}
	var issues []string
	if f.RequiredWhen != nil {
		issues = append(issues, fmt.Sprintf("field %q: show_when and required_when together -- a Field is asked for while %q holds a value and is then required if it is `required`", f.ID, sw.Field))
	}
	if f.Compute != nil || f.Stamp != "" || f.Numbering != nil {
		issues = append(issues, fmt.Sprintf("field %q: a computed, stamped or numbered field is written by the runtime, so show_when would hide nothing a person is asked", f.ID))
	}
	other, ok := fieldsByID[sw.Field]
	switch {
	case sw.Field == f.ID:
		issues = append(issues, fmt.Sprintf("field %q: show_when names itself", f.ID))
	case !ok:
		issues = append(issues, fmt.Sprintf("field %q: show_when.field %q is not a field of this machine", f.ID, sw.Field))
	case other.Type != domain.FieldTypeStatus:
		issues = append(issues, fmt.Sprintf("field %q: show_when.field %q must be a status field, got %q -- a comparison needs a closed set of values", f.ID, sw.Field, other.Type))
	case !slices.Contains(other.Options, sw.Equals):
		issues = append(issues, fmt.Sprintf("field %q: show_when.equals %q is not one of %q's own options %v -- it would never be shown", f.ID, sw.Equals, sw.Field, other.Options))
	case other.ShowWhen != nil:
		issues = append(issues, fmt.Sprintf("field %q: show_when watches %q, which is itself shown conditionally -- one step, no chains", f.ID, sw.Field))
	}
	return issues
}

// validateDefaultFrom holds a derived default to its shape: a date Field, a status Field of this Machine to read,
// a non-negative day count for options that exist, at least one entry, and no second default beside it.
func validateDefaultFrom(f domain.Field, fieldsByID map[string]domain.Field) []string {
	df := f.DefaultFrom
	if df == nil {
		return nil
	}
	var issues []string
	if f.Type != domain.FieldTypeDate {
		issues = append(issues, fmt.Sprintf("field %q: default_from is only valid on a date field, got type %q", f.ID, f.Type))
	}
	if f.Default != nil || f.Compute != nil || f.Stamp != "" {
		issues = append(issues, fmt.Sprintf("field %q: default_from beside a default, compute or stamp -- a field has one source for its value", f.ID))
	}
	other, ok := fieldsByID[df.Field]
	switch {
	case !ok:
		issues = append(issues, fmt.Sprintf("field %q: default_from.field %q is not a field of this machine", f.ID, df.Field))
	case other.Type != domain.FieldTypeStatus:
		issues = append(issues, fmt.Sprintf("field %q: default_from.field %q must be a status field, got %q", f.ID, df.Field, other.Type))
	default:
		for value, days := range df.Days {
			if !slices.Contains(other.Options, value) {
				issues = append(issues, fmt.Sprintf("field %q: default_from.days names %q, which is not one of %q's own options %v", f.ID, value, df.Field, other.Options))
			}
			if days < 0 {
				issues = append(issues, fmt.Sprintf("field %q: default_from.days[%q] is %d -- a due date is not in the past", f.ID, value, days))
			}
		}
	}
	if len(df.Days) == 0 {
		issues = append(issues, fmt.Sprintf("field %q: default_from.days is empty -- it would derive nothing", f.ID))
	}
	return issues
}

// validateStamp holds a stamped Field to what its application assumes: a known source, a person Field to
// hold the user it names, and no default (the stamp is always what gets stored).
func validateStamp(f domain.Field) []string {
	if f.Stamp == "" {
		return nil
	}
	var issues []string
	if !domain.KnownFieldStamps[f.Stamp] {
		issues = append(issues, fmt.Sprintf("field %q: stamp %q is not one of %v", f.ID, f.Stamp, sortedFieldStamps()))
	}
	if f.Stamp == domain.FieldStampCurrentUser && f.Type != domain.FieldTypePerson {
		issues = append(issues, fmt.Sprintf("field %q: stamp: current_user is only valid on a person field, got type %q", f.ID, f.Type))
	}
	if f.Default != nil {
		issues = append(issues, fmt.Sprintf("field %q: a stamped field cannot declare a default -- its value is always the stamp", f.ID))
	}
	if f.Compute != nil {
		issues = append(issues, fmt.Sprintf("field %q: a field cannot be both computed and stamped", f.ID))
	}
	return issues
}

func sortedFieldStamps() []string {
	names := make([]string, 0, len(domain.KnownFieldStamps))
	for s := range domain.KnownFieldStamps {
		names = append(names, string(s))
	}
	slices.Sort(names)
	return names
}

// validateCompute holds a computed Field to the shape its evaluation assumes: a number, one known
// operation, operands that are number (or money) Fields of this Machine and not computed themselves (so the
// order of evaluation cannot matter and no cycle can exist), and none of the declarations that
// would contradict "the runtime writes this, a person does not" -- required, or a default.
func validateNumbering(f domain.Field, fieldsByID map[string]domain.Field) []string {
	if f.Numbering == nil {
		return nil
	}
	var issues []string
	prefix, _, err := domain.ParseNumbering(f.Numbering.Pattern)
	if err != nil {
		return []string{fmt.Sprintf("field %q: %v", f.ID, err)}
	}
	switch {
	case f.Type != domain.FieldTypeText:
		issues = append(issues, fmt.Sprintf("field %q: numbering writes text, and this field is %s", f.ID, f.Type))
	case f.Compute != nil || f.Stamp != "" || f.Default != nil || f.DefaultFrom != nil:
		issues = append(issues, fmt.Sprintf("field %q: a numbered field is written by the runtime once, so it takes no compute, stamp or default", f.ID))
	case f.RequiredWhen != nil:
		issues = append(issues, fmt.Sprintf("field %q: a numbered field always has its number, so it takes no required_when", f.ID))
	}
	for _, p := range prefix {
		if p.Field == "" {
			continue
		}
		operand, ok := fieldsByID[p.Field]
		switch {
		case p.Field == f.ID:
			issues = append(issues, fmt.Sprintf("field %q: numbering names itself", f.ID))
		case !ok:
			issues = append(issues, fmt.Sprintf("field %q: numbering names %q, which this machine does not declare", f.ID, p.Field))
		case operand.Type != domain.FieldTypeText && operand.Type != domain.FieldTypeStatus:
			issues = append(issues, fmt.Sprintf("field %q: numbering reads %q, a %s field; the number is written from text or status words", f.ID, p.Field, operand.Type))
		case operand.Compute != nil:
			issues = append(issues, fmt.Sprintf("field %q: numbering reads %q, which is computed after the number is written", f.ID, p.Field))
		}
	}
	for value, code := range f.Numbering.Codes {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(code) == "" || strings.ContainsAny(code, "{}") {
			issues = append(issues, fmt.Sprintf("field %q: numbering codes map a value to letters, and %q -> %q is not that", f.ID, value, code))
		}
	}
	return issues
}

func validateCompute(f domain.Field, fieldsByID map[string]domain.Field) []string {
	if f.Compute == nil {
		return nil
	}
	var issues []string
	tree := f.Compute.Tree()
	if tree.Inherited() {
		return validateInherited(f, tree, fieldsByID)
	}
	if tree.Op == domain.ComputeJoin {
		return validateJoin(f, tree, fieldsByID)
	}
	if !domain.KnownComputeOps[tree.Op] {
		issues = append(issues, fmt.Sprintf("field %q: compute op %q is not one of %v", f.ID, tree.Op, sortedComputeOps()))
		return issues
	}
	if f.Compute.Expr == nil && len(f.Compute.Fields) == 0 {
		issues = append(issues, fmt.Sprintf("field %q: compute: needs at least one field to compute from", f.ID))
	}
	if f.Required {
		issues = append(issues, fmt.Sprintf("field %q: a computed field cannot be required -- nobody enters it", f.ID))
	}
	if f.Default != nil {
		issues = append(issues, fmt.Sprintf("field %q: a computed field cannot declare a default -- its value is always derived", f.ID))
	}
	// What the value is: a figure (number or money) or a date. The Field must hold what its expression gives.
	kind, exprIssues := computeKind(f, tree, fieldsByID)
	issues = append(issues, exprIssues...)
	switch {
	case kind == "date" && f.Type != domain.FieldTypeDate:
		issues = append(issues, fmt.Sprintf("field %q: compute %s is only valid on a date field, got type %q", f.ID, tree.Op, f.Type))
	case kind == "figure" && !f.Type.IsNumeric():
		issues = append(issues, fmt.Sprintf("field %q: compute %s is only valid on a number or money field, got type %q", f.ID, tree.Op, f.Type))
	}
	if f.Compute.Expr == nil {
		seen := map[string]bool{}
		for _, id := range f.Compute.Fields {
			if seen[id] {
				issues = append(issues, fmt.Sprintf("field %q: compute: names %q more than once", f.ID, id))
			}
			seen[id] = true
		}
	}
	return issues
}

// validateInherited holds an inherited Field (`compute: {parent: fld_contract.fld_category}`: the value is the related
// record's own, whatever its type) to what such a Field is: written by the runtime, reached through a relation of this
// Machine. That the parent's Field exists and is of the same type is validateComputeTargets' question.
// validateJoin holds a joined Field to what it is: a text Field written by the runtime from the words of text or status
// Fields of its own record (never itself) or of a related record's (`parent:`, whose type validateComputeTargets reads).
func validateJoin(f domain.Field, tree domain.ComputeExpr, fieldsByID map[string]domain.Field) []string {
	var issues []string
	if f.Type != domain.FieldTypeText {
		issues = append(issues, fmt.Sprintf("field %q: compute join is only valid on a text field, got type %q", f.ID, f.Type))
	}
	if f.Required {
		issues = append(issues, fmt.Sprintf("field %q: a computed field cannot be required -- nobody enters it", f.ID))
	}
	if f.Default != nil {
		issues = append(issues, fmt.Sprintf("field %q: a computed field cannot declare a default -- its value is always derived", f.ID))
	}
	if len(tree.Args) == 0 {
		issues = append(issues, fmt.Sprintf("field %q: compute join: needs at least one operand in of:", f.ID))
	}
	for _, a := range tree.Args {
		switch {
		case a.Parent != nil:
			if via, ok := fieldsByID[a.Parent.Via]; !ok || via.Type != domain.FieldTypeRelation {
				issues = append(issues, fmt.Sprintf("field %q: compute: parent: %s.%s goes through %q, which is not a relation field of this machine", f.ID, a.Parent.Via, a.Parent.Field, a.Parent.Via))
			}
		case a.Op == "" && a.Field != "":
			operand, ok := fieldsByID[a.Field]
			switch {
			case a.Field == f.ID:
				issues = append(issues, fmt.Sprintf("field %q: compute: names itself", f.ID))
			case !ok:
				issues = append(issues, fmt.Sprintf("field %q: compute: names %q, which this machine does not declare", f.ID, a.Field))
			case operand.Type != domain.FieldTypeText && operand.Type != domain.FieldTypeStatus:
				issues = append(issues, fmt.Sprintf("field %q: compute join: operand %q must be a text or status field, got type %q", f.ID, a.Field, operand.Type))
			}
		default:
			issues = append(issues, fmt.Sprintf("field %q: compute join: an operand is a Field id or parent:, not a number or an operation", f.ID))
		}
	}
	return issues
}

// describeField is a Field's type as an issue says it: "relation to mch_category", "status [A B]".
func describeField(f domain.Field) string {
	switch {
	case f.RelatedMachine != "":
		return fmt.Sprintf("a %s to %s", f.Type, f.RelatedMachine)
	case len(f.Options) > 0:
		return fmt.Sprintf("a %s %v", f.Type, f.Options)
	}
	return "a " + string(f.Type)
}

func validateInherited(f domain.Field, tree domain.ComputeExpr, fieldsByID map[string]domain.Field) []string {
	var issues []string
	if f.Required {
		issues = append(issues, fmt.Sprintf("field %q: an inherited field cannot be required -- nobody enters it", f.ID))
	}
	if f.Default != nil {
		issues = append(issues, fmt.Sprintf("field %q: an inherited field cannot declare a default -- its value is always the related record's", f.ID))
	}
	via, ok := fieldsByID[tree.Parent.Via]
	if !ok || via.Type != domain.FieldTypeRelation {
		issues = append(issues, fmt.Sprintf("field %q: compute: parent: %s.%s goes through %q, which is not a relation field of this machine", f.ID, tree.Parent.Via, tree.Parent.Field, tree.Parent.Via))
	}
	if tree.Parent.Via == f.ID {
		issues = append(issues, fmt.Sprintf("field %q: compute: names itself", f.ID))
	}
	return issues
}

// computeKind is what an expression gives -- "figure" or "date" -- and what is wrong with it: each operation's operands
// and their number, each Field it reads (one of this Machine's, never the Field itself), each relation a parent: goes
// through. What a parent: or an aggregate reads on another Machine is validateComputeTargets' question.
func computeKind(f domain.Field, e domain.ComputeExpr, fieldsByID map[string]domain.Field) (string, []string) {
	var issues []string
	switch {
	case e.Number != nil:
		return "figure", nil
	case e.Parent != nil:
		via, ok := fieldsByID[e.Parent.Via]
		if !ok || via.Type != domain.FieldTypeRelation {
			return "figure", []string{fmt.Sprintf("field %q: compute: parent: %s.%s goes through %q, which is not a relation field of this machine", f.ID, e.Parent.Via, e.Parent.Field, e.Parent.Via)}
		}
		return "figure", nil
	case e.Op == "" && e.Field != "":
		operand, ok := fieldsByID[e.Field]
		switch {
		case e.Field == f.ID:
			return "", []string{fmt.Sprintf("field %q: compute: names itself", f.ID)}
		case !ok:
			return "", []string{fmt.Sprintf("field %q: compute: names %q, which this machine does not declare", f.ID, e.Field)}
		case operand.Type.IsNumeric():
			return "figure", nil
		case operand.Type == domain.FieldTypeDate:
			return "date", nil
		}
		return "", []string{fmt.Sprintf("field %q: compute: operand %q must be a number, money or date field, got type %q", f.ID, e.Field, operand.Type)}
	case e.Op == domain.ComputeSumChildren || e.Op == domain.ComputeCountChildren:
		if e.Children == nil || e.Children.Machine == "" || e.Children.Via == "" || (e.Op == domain.ComputeSumChildren && e.Children.Field == "") {
			return "figure", []string{fmt.Sprintf("field %q: compute: %s names the machine, the relation (via) that points here and, for a sum, the field", f.ID, e.Op)}
		}
		return "figure", nil
	}
	kinds := make([]string, len(e.Args))
	for i, a := range e.Args {
		k, sub := computeKind(f, a, fieldsByID)
		kinds[i], issues = k, append(issues, sub...)
	}
	want := func(i int, k string) {
		if kinds[i] != "" && kinds[i] != k {
			issues = append(issues, fmt.Sprintf("field %q: compute %s: operand %d must be a %s, got a %s", f.ID, e.Op, i+1, k, kinds[i]))
		}
	}
	arity := func(lo, hi int) bool {
		if len(e.Args) < lo || (hi > 0 && len(e.Args) > hi) {
			issues = append(issues, fmt.Sprintf("field %q: compute %s takes %s operands, got %d", f.ID, e.Op, arityWords(lo, hi), len(e.Args)))
			return false
		}
		return true
	}
	switch e.Op {
	case domain.ComputeAddDays, domain.ComputeAddMonths:
		if arity(2, 2) {
			want(0, "date")
			want(1, "figure")
			if e.Op == domain.ComputeAddDays && e.Args[1].Op == "" && e.Args[1].Field != "" && fieldsByID[e.Args[1].Field].Type != domain.FieldTypeNumber {
				issues = append(issues, fmt.Sprintf("field %q: compute: operand %q must be a number field, got type %q", f.ID, e.Args[1].Field, fieldsByID[e.Args[1].Field].Type))
			}
		}
		return "date", issues
	case domain.ComputeSum:
		arity(1, 0)
	case domain.ComputeSubtract, domain.ComputeMultiply, domain.ComputeMin, domain.ComputeMax:
		arity(2, 0)
	case domain.ComputeDivide:
		arity(2, 2)
	case domain.ComputeRound:
		if arity(1, 2) && len(e.Args) == 2 {
			if d := e.Args[1].Number; d == nil || *d != math.Trunc(*d) || *d < 0 || *d > 10 {
				issues = append(issues, fmt.Sprintf("field %q: compute round: its second operand is the number of decimals, a whole number from 0 to 10", f.ID))
			}
		}
	}
	for i := range e.Args {
		want(i, "figure")
	}
	return "figure", issues
}

func arityWords(lo, hi int) string {
	switch {
	case hi == 0:
		return fmt.Sprintf("at least %d", lo)
	case lo == hi:
		return fmt.Sprintf("exactly %d", lo)
	}
	return fmt.Sprintf("%d to %d", lo, hi)
}

// validateComputeCycles refuses computed Fields that read each other in a ring: each is computed after what it reads
// (Machine.ComputeOrder), which a ring has no order for.
func validateComputeCycles(m *domain.Machine) []string {
	reads := map[string][]string{}
	for _, f := range m.Fields {
		if f.Compute == nil {
			continue
		}
		f.Compute.Tree().Leaves(func(l domain.ComputeExpr) {
			if l.Op == "" && l.Field != "" {
				reads[f.ID] = append(reads[f.ID], l.Field)
			}
		})
	}
	state := map[string]int{}
	var issues []string
	var visit func(id string, path []string)
	visit = func(id string, path []string) {
		switch state[id] {
		case 1:
			issues = append(issues, fmt.Sprintf("computed fields read each other in a ring: %s", strings.Join(append(path, id), " -> ")))
			return
		case 2:
			return
		}
		state[id] = 1
		for _, next := range reads[id] {
			if _, computed := reads[next]; computed {
				visit(next, append(path, id))
			}
		}
		state[id] = 2
	}
	for _, f := range m.Fields {
		if f.Compute != nil && state[f.ID] == 0 {
			visit(f.ID, nil)
		}
	}
	return issues
}

// validateComputeTargets holds what a computed value reads on another Machine: a parent: Field that exists on the Machine
// its relation points at and is not computed itself (a parent's own computed value would go stale under the child), and
// an aggregate over a Machine whose relation (via) points at this one, summing a figure, filtered by one of a status
// Field's options -- a Machine every write of which recomputes this one, so one written by the runtime is refused.
func validateComputeTargets(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}
	var issues []string
	for _, m := range machines {
		for _, f := range m.Fields {
			if f.Compute == nil {
				continue
			}
			f.Compute.Tree().Leaves(func(l domain.ComputeExpr) {
				where := fmt.Sprintf("machine %q: field %q: compute", m.ID, f.ID)
				if p := l.Parent; p != nil {
					via, _ := m.FieldByID(p.Via)
					parent := byID[via.RelatedMachine]
					target, ok := domain.Field{}, false
					if parent != nil {
						target, ok = parent.FieldByID(p.Field)
					}
					inherited := f.Compute.Tree().Inherited()
					switch {
					case inherited && via.RelatedMachine == m.ID:
						// A Machine reading its own records through a self-relation is a tree the cascade follows only a few
						// levels, and a ring when the records point at each other.
						issues = append(issues, fmt.Sprintf("%s: parent: %s.%s goes through %q, a relation to this same machine -- an inherited field takes another machine's value", where, p.Via, p.Field, p.Via))
					case !ok:
						issues = append(issues, fmt.Sprintf("%s: parent: %s.%s -- %q has no field %q", where, p.Via, p.Field, via.RelatedMachine, p.Field))
					case inherited:
						// The Field takes the related record's value as it is, so it is that Field's own type (a relation
						// to the same Machine, a status with the same options): what is stored is then what is read.
						if target.Type != f.Type || target.RelatedMachine != f.RelatedMachine || !slices.Equal(target.Options, f.Options) {
							issues = append(issues, fmt.Sprintf("%s: parent: %s.%s is %s, and this field is %s: an inherited field is declared as the field it takes", where, p.Via, p.Field, describeField(target), describeField(f)))
						}
					case f.Compute.Tree().Op == domain.ComputeJoin:
						if target.Type != domain.FieldTypeText && target.Type != domain.FieldTypeStatus {
							issues = append(issues, fmt.Sprintf("%s: parent: %s.%s is a %s field, and a join writes words from text or status fields", where, p.Via, p.Field, target.Type))
						}
					case !target.Type.IsNumeric():
						issues = append(issues, fmt.Sprintf("%s: parent: %s.%s is a %s field, and a computed value reads a figure", where, p.Via, p.Field, target.Type))
					case target.Compute != nil:
						issues = append(issues, fmt.Sprintf("%s: parent: %s.%s is itself computed, so this value would go stale when it changes", where, p.Via, p.Field))
					}
				}
				if a := l.Children; a != nil {
					child := byID[a.Machine]
					if a.Machine == m.ID {
						issues = append(issues, fmt.Sprintf("%s: %s: a machine adding up its own records would grow on every recompute; aggregate another machine", where, l.Op))
						return
					}
					if child == nil {
						issues = append(issues, fmt.Sprintf("%s: %s: machine %q is not installed in this workspace", where, l.Op, a.Machine))
						return
					}
					via, ok := child.FieldByID(a.Via)
					if !ok || via.Type != domain.FieldTypeRelation || via.RelatedMachine != m.ID {
						issues = append(issues, fmt.Sprintf("%s: %s: %s.%s is not a relation pointing at %q", where, l.Op, a.Machine, a.Via, m.ID))
					}
					if l.Op == domain.ComputeSumChildren {
						if sum, ok := child.FieldByID(a.Field); !ok || !sum.Type.IsNumeric() {
							issues = append(issues, fmt.Sprintf("%s: sum_children: %s.%s is not a number or money field", where, a.Machine, a.Field))
						}
					}
					if w := a.Where; w != nil {
						if st, ok := child.FieldByID(w.Field); !ok || st.Type != domain.FieldTypeStatus || !slices.Contains(st.Options, w.Equals) {
							issues = append(issues, fmt.Sprintf("%s: %s: where %s equals %q is not a status field of %q holding that option", where, l.Op, w.Field, w.Equals, a.Machine))
						}
					}
					if !child.GenericallyWritten() {
						issues = append(issues, fmt.Sprintf("%s: %s: %q is written by the runtime itself, which would not recompute %q", where, l.Op, a.Machine, m.ID))
					}
				}
			})
		}
	}
	issues = append(issues, inheritedRingIssues(machines, byID)...)
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// inheritedRingIssues refuses computed Fields that read each other across Machines in a ring (A.x from B.y while B.y reads
// A.x): each is recomputed after what it reads, which a ring has no order for, and the cascade would only stop at its depth
// limit. Only a `parent:` read of another computed Field is an edge -- reading a stored one ends the chain.
func inheritedRingIssues(machines []*domain.Machine, byID map[string]*domain.Machine) []string {
	type node struct{ machine, field string }
	edges := map[node][]node{}
	for _, m := range machines {
		for _, f := range m.Fields {
			if f.Compute == nil {
				continue
			}
			from := node{m.ID, f.ID}
			f.Compute.Tree().Leaves(func(l domain.ComputeExpr) {
				if l.Parent == nil {
					return
				}
				via, _ := m.FieldByID(l.Parent.Via)
				parent := byID[via.RelatedMachine]
				if parent == nil {
					return
				}
				if target, ok := parent.FieldByID(l.Parent.Field); ok && target.Compute != nil {
					edges[from] = append(edges[from], node{parent.ID, target.ID})
				}
			})
		}
	}
	var issues []string
	state := map[node]int{}
	var visit func(n node, path []node)
	visit = func(n node, path []node) {
		state[n] = 1
		for _, next := range edges[n] {
			switch state[next] {
			case 1:
				var ring []string
				for _, p := range append(slices.Clone(path), n, next) {
					ring = append(ring, p.machine+"."+p.field)
				}
				issues = append(issues, fmt.Sprintf("computed fields read each other in a ring across machines: %s", strings.Join(ring, " -> ")))
			case 0:
				visit(next, append(slices.Clone(path), n))
			}
		}
		state[n] = 2
	}
	keys := make([]node, 0, len(edges))
	for n := range edges {
		keys = append(keys, n)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].machine+"."+keys[i].field < keys[j].machine+"."+keys[j].field })
	for _, n := range keys {
		if state[n] == 0 {
			visit(n, nil)
		}
	}
	return issues
}

func sortedComputeOps() []string {
	out := make([]string, 0, len(domain.KnownComputeOps))
	for op := range domain.KnownComputeOps {
		out = append(out, string(op))
	}
	sort.Strings(out)
	return out
}

// validateComputedFieldsAreGenericallyWritten refuses a computed Field on a Machine that Go writes
// through its own routes. A computed value is filled in by the generic create/edit routes
// (internal/web's record.go and api.go); a workflow engine's screens (the approval inbox, the submit
// wizard) and the runtime's own writers (a membership's mch_user record, activity, notifications)
// write their Machines directly and would leave the value stale. Refusing here keeps "every write of
// this Machine computes it" true by construction, instead of true until someone adds a route.
func validateComputedFieldsAreGenericallyWritten(machines []*domain.Machine) error {
	return validateFieldsAreGenericallyWritten(machines, "computed", func(f domain.Field) bool { return f.Compute != nil })
}

// validateStampedFieldsAreGenericallyWritten is the same refusal for a stamped Field: the acting user is
// written by the generic create routes (record.go, api.go), and the engines' own screens and the runtime's
// own writers would leave it empty.
// validateBoundsAreGenericallyWritten refuses `ensure:` on a Machine that Go writes through its own routes: the bound is
// checked by the generic create and update routes, and an engine's screens or the runtime's own writers would skip it.
func validateBoundsAreGenericallyWritten(machines []*domain.Machine) error {
	var issues []string
	for _, m := range machines {
		if len(m.Bounds) == 0 || m.GenericallyWritten() {
			continue
		}
		issues = append(issues, fmt.Sprintf("machine %q declares ensure: but is written by the runtime or a workflow engine's own screens, which do not check it", m.ID))
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateGenerates holds an edge that generates a schedule of records to what the generation assumes, against both
// Machines: the edge is performed by `edit` (the route that generates), this Machine holds the count and the amount,
// the target Machine points back at it, carries a date and a number to fill, and has no other Field the runtime could not
// fill (a required Field with no default would make every generated row fail, and the move with it).
func validateGenerates(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}
	var issues []string
	for _, m := range machines {
		for _, t := range m.Transitions {
			g := t.Generates
			if g == nil {
				continue
			}
			bad := func(format string, a ...any) {
				issues = append(issues, fmt.Sprintf("machine %q: transition %q: generates: ", m.ID, t.ID)+fmt.Sprintf(format, a...))
			}
			if t.Action != domain.ActionEdit {
				bad("only an edge with action %q may generate, not %q", domain.ActionEdit, t.Action)
			}
			if g.DueEveryDays < 1 {
				bad("due_every_days is %d; it must be at least 1", g.DueEveryDays)
			}
			for key, id := range map[string]string{"count_field": g.CountField, "amount_from": g.AmountFrom} {
				if f, ok := m.FieldByID(id); !ok || !f.Type.IsNumeric() || (key == "count_field" && f.Type != domain.FieldTypeNumber) {
					bad("%s %q must be a number field of this machine", key, id)
				}
			}
			target, ok := byID[g.Machine]
			switch {
			case !ok:
				bad("machine %q is not a machine of this workspace", g.Machine)
				continue
			case g.Machine == m.ID:
				bad("a machine cannot generate records of itself")
				continue
			case !target.GenericallyWritten():
				bad("machine %q is written by the runtime or an engine's own screens", g.Machine)
				continue
			}
			named := map[string]bool{g.LinkField: true, g.DueField: true, g.AmountField: true}
			if len(named) != 3 {
				bad("link_field, due_field and amount_field must be three different fields")
			}
			if f, ok := target.FieldByID(g.LinkField); !ok || f.Type != domain.FieldTypeRelation || f.RelatedMachine != m.ID {
				bad("link_field %q must be a relation field of %s pointing back at %s", g.LinkField, target.ID, m.ID)
			}
			if f, ok := target.FieldByID(g.DueField); !ok || f.Type != domain.FieldTypeDate {
				bad("due_field %q must be a date field of %s", g.DueField, target.ID)
			}
			if f, ok := target.FieldByID(g.AmountField); !ok || !f.Type.IsNumeric() {
				bad("amount_field %q must be a number or money field of %s", g.AmountField, target.ID)
			}
			for _, f := range target.Fields {
				if f.Required && !named[f.ID] && f.Default == nil && f.Compute == nil && f.Stamp == "" {
					bad("%s requires field %q, which the runtime cannot fill for a generated row -- give it a default or make it optional", target.ID, f.ID)
				}
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// ValidateEdgeRules runs the two cross-Machine checks on edges that name another Machine (`generates:`, `balanced:`)
// over machines, the way the loader does at load. The assistant calls it on the Machines it just built together with the
// ones already installed, so it is told while it can still fix the answer instead of after a publish is rolled back.
func ValidateEdgeRules(machines []*domain.Machine) error {
	if err := validateGenerates(machines); err != nil {
		return err
	}
	return validateBalanced(machines)
}

// validateBalanced holds an edge that must balance to what the check reads: the edge is performed by `edit`, the Machine
// it totals is another Machine of this Workspace written by the generic screens, `via` is a relation on it pointing back
// at this Machine, and the debit and credit are two different number Fields of it.
func validateBalanced(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}
	var issues []string
	for _, m := range machines {
		for _, t := range m.Transitions {
			b := t.Balanced
			if b == nil {
				continue
			}
			bad := func(format string, a ...any) {
				issues = append(issues, fmt.Sprintf("machine %q: transition %q: balanced: ", m.ID, t.ID)+fmt.Sprintf(format, a...))
			}
			if t.Action != domain.ActionEdit {
				bad("only an edge with action %q may balance, not %q", domain.ActionEdit, t.Action)
			}
			target, ok := byID[b.Machine]
			switch {
			case !ok:
				bad("machine %q is not a machine of this workspace", b.Machine)
				continue
			case b.Machine == m.ID:
				bad("a machine cannot balance against itself")
				continue
			case !target.GenericallyWritten():
				bad("machine %q is written by the runtime or an engine's own screens", b.Machine)
				continue
			}
			if f, ok := target.FieldByID(b.Via); !ok || f.Type != domain.FieldTypeRelation || f.RelatedMachine != m.ID {
				bad("via %q must be a relation field of %s pointing back at %s", b.Via, target.ID, m.ID)
			}
			if b.Debit == b.Credit {
				bad("debit and credit must be two different fields")
			}
			for key, id := range map[string]string{"debit": b.Debit, "credit": b.Credit} {
				if f, ok := target.FieldByID(id); !ok || !f.Type.IsNumeric() {
					bad("%s %q must be a number or money field of %s", key, id, target.ID)
				}
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateFrozenWhen is the cross-Machine half of frozen_when: the Field it reads must exist on the Machine its `via`
// points at, be a status Field, and the value be one of its options (a value it can never hold would freeze nothing).
func validateFrozenWhen(machines []*domain.Machine) error {
	byID := make(map[string]*domain.Machine, len(machines))
	for _, m := range machines {
		byID[m.ID] = m
	}
	var issues []string
	for _, m := range machines {
		if m.Frozen == nil {
			continue
		}
		if !m.GenericallyWritten() {
			issues = append(issues, fmt.Sprintf("machine %q declares frozen_when but is written by the runtime or a workflow engine's own screens, which do not check it", m.ID))
			continue
		}
		via, ok := m.FieldByID(m.Frozen.Via)
		if !ok || via.Type != domain.FieldTypeRelation {
			continue // reported per Machine
		}
		parent, ok := byID[via.RelatedMachine]
		if !ok {
			continue // reported by validateRelationTargets
		}
		f, ok := parent.FieldByID(m.Frozen.Field)
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("machine %q: frozen_when.field %q is not a field of %q, which %q points at", m.ID, m.Frozen.Field, parent.ID, m.Frozen.Via))
		case f.Type != domain.FieldTypeStatus:
			issues = append(issues, fmt.Sprintf("machine %q: frozen_when.field %q on %q must be a status field, got %q", m.ID, m.Frozen.Field, parent.ID, f.Type))
		case !slices.Contains(f.Options, m.Frozen.Equals):
			issues = append(issues, fmt.Sprintf("machine %q: frozen_when.equals %q is not one of %q's options %v -- it would freeze nothing", m.ID, m.Frozen.Equals, m.Frozen.Field, f.Options))
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func validateDerivedFieldsAreGenericallyWritten(machines []*domain.Machine) error {
	return validateFieldsAreGenericallyWritten(machines, "conditionally required or derived", func(f domain.Field) bool { return f.RequiredWhen != nil || f.DefaultFrom != nil || f.ShowWhen != nil })
}

func validateNumberedFieldsAreGenericallyWritten(machines []*domain.Machine) error {
	return validateFieldsAreGenericallyWritten(machines, "numbered", func(f domain.Field) bool { return f.Numbering != nil })
}

func validateStampedFieldsAreGenericallyWritten(machines []*domain.Machine) error {
	return validateFieldsAreGenericallyWritten(machines, "stamped", func(f domain.Field) bool { return f.Stamp != "" })
}

func validateFieldsAreGenericallyWritten(machines []*domain.Machine, kind string, has func(domain.Field) bool) error {
	var issues []string
	for _, m := range machines {
		var found []string
		for _, f := range m.Fields {
			if has(f) {
				found = append(found, f.ID)
			}
		}
		if len(found) == 0 {
			continue
		}
		switch {
		case m.GenericallyWritten():
		case m.WorkflowRole != "":
			issues = append(issues, fmt.Sprintf("machine %q declares %s field(s) %v, but it plays the %q role in a workflow engine whose own screens write it without applying them", m.ID, kind, found, m.WorkflowRole))
		default:
			issues = append(issues, fmt.Sprintf("machine %q declares %s field(s) %v, but it is a runtime-level machine the runtime writes itself", m.ID, kind, found))
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// validateParentActorVia checks the half of a parent_actor gate this Machine can answer alone: `via` is a
// relation Field of the Machine declaring the Permission. Whether the Field named on the *parent* exists is a
// cross-Machine question (validateParentActors). Both are load errors for the reason the dynamic gate's are: a
// gate reading a Field that is not there compares the actor against nothing and refuses everyone, which looks
// exactly like a rule working.
func validateParentActorVia(m *domain.Machine, p domain.Permission, fieldsByID map[string]domain.Field) []string {
	pa := p.ParentActor
	if pa == nil {
		return nil
	}
	var issues []string
	if pa.ViaField == "" || pa.ActorField == "" {
		return []string{fmt.Sprintf("permission %q: parent_actor needs both via and field", p.ID)}
	}
	via, ok := fieldsByID[pa.ViaField]
	switch {
	case !ok:
		issues = append(issues, fmt.Sprintf("permission %q: parent_actor.via %q is not a field of machine %q", p.ID, pa.ViaField, m.ID))
	case via.Type != domain.FieldTypeRelation:
		issues = append(issues, fmt.Sprintf("permission %q: parent_actor.via %q must be a relation field pointing at the parent Machine, got %q", p.ID, pa.ViaField, via.Type))
	}
	return issues
}

// validateStatusCategories checks a Machine's `status_categories:` (K-15): the Field is a status Field of this
// Machine, every key is one of its options, every value is one of the five closed categories, and the block is
// not empty. Where the Machine also declares `completion:` on the same Field, the option it calls finished must
// be the one categorised `done` -- "finished" is stated once, and two statements may not disagree (001 #8).
func validateStatusCategories(m *domain.Machine, fieldsByID map[string]domain.Field) []string {
	sc := m.StatusCategories
	if sc == nil {
		return nil
	}
	f, ok := fieldsByID[sc.Field]
	switch {
	case !ok:
		return []string{fmt.Sprintf("machine %q: status_categories field %q is not a field of this machine", m.ID, sc.Field)}
	case f.Type != domain.FieldTypeStatus:
		return []string{fmt.Sprintf("machine %q: status_categories field %q must be a status field, got %q", m.ID, sc.Field, f.Type)}
	case len(sc.Values) == 0:
		return []string{fmt.Sprintf("machine %q: status_categories declares no values", m.ID)}
	}
	var issues []string
	options := make([]string, 0, len(sc.Values))
	for option := range sc.Values {
		options = append(options, option)
	}
	slices.Sort(options)
	for _, option := range options {
		category := sc.Values[option]
		if !slices.Contains(f.Options, option) {
			issues = append(issues, fmt.Sprintf("machine %q: status_categories value %q is not one of %q's options %v", m.ID, option, f.ID, f.Options))
		}
		if !domain.KnownStatusCategories[category] {
			issues = append(issues, fmt.Sprintf("machine %q: status_categories value %q has unknown category %q (want start, active, attention, done or stopped)", m.ID, option, category))
		}
	}
	// A state a transition on this Field starts or ends in must be categorised: a map draws every edge, and an
	// endpoint with no category would be drawn in a colour nobody chose.
	for _, tr := range m.Transitions {
		if tr.Field != sc.Field {
			continue
		}
		for _, end := range []string{tr.From, tr.To} {
			if _, ok := sc.Values[end]; !ok && end != "" {
				issues = append(issues, fmt.Sprintf("machine %q: transition %q reaches %q, which status_categories does not categorise", m.ID, tr.ID, end))
			}
		}
	}
	if m.Completion != nil && m.Completion.Field == sc.Field {
		if got, ok := sc.Values[m.Completion.Done]; !ok || got != domain.StatusDone {
			issues = append(issues, fmt.Sprintf("machine %q: completion done %q must be categorised done in status_categories", m.ID, m.Completion.Done))
		}
		for _, option := range options {
			if sc.Values[option] == domain.StatusDone && option != m.Completion.Done {
				issues = append(issues, fmt.Sprintf("machine %q: status_categories marks %q done, but completion says %q is the finished value", m.ID, option, m.Completion.Done))
			}
		}
	}
	return issues
}

// runtimeMachineIssues holds what a Machine declaring `runtime:` promises: a source the registry knows, every Field the
// source fills, and nothing that would make it writable or reactive -- it is read-only, so a Permission, an edge or an
// Event on it would read as behaviour the runtime never performs.
func runtimeMachineIssues(m *domain.Machine) []string {
	if m.Runtime == "" {
		return nil
	}
	src, ok := registry.RuntimeSources[m.Runtime]
	if !ok {
		return []string{fmt.Sprintf("machine %q: runtime: %q is not a source the runtime provides (%s)", m.ID, m.Runtime, strings.Join(registry.RuntimeSourceIDs(), ", "))}
	}
	var issues []string
	for _, id := range src.Provides {
		if _, ok := m.FieldByID(id); !ok {
			issues = append(issues, fmt.Sprintf("machine %q: runtime source %q fills %s, which this machine does not declare", m.ID, m.Runtime, id))
		}
	}
	if blocks := m.DeclaredRuleBlocks(); len(blocks) > 0 {
		issues = append(issues, fmt.Sprintf("machine %q: a runtime machine is read-only and declares no rule block, and this one declares %s", m.ID, strings.Join(blocks, ", ")))
	}
	return issues
}
