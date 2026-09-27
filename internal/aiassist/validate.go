package aiassist

import (
	"fmt"
	"regexp"
	"strings"

	"menata.app/internal/action"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// applicationIDPattern mirrors internal/metadata's own (unexported) applicationIDPattern exactly
// -- duplicated rather than exported from that package, since internal/metadata's own load-time
// validators for Application-level fields (role vocabulary, icon/color, summary_machine) are
// themselves unexported (loadApplicationFile is package-private). Machine-level validation does
// not have this problem: metadata.Validate is already exported and reused as-is below.
var applicationIDPattern = regexp.MustCompile(`^app_[a-z][a-z0-9_]*$`)

// reservedMachineIDs are ids internal/action and internal/composition hardcode as *equality
// checks* against a bare string, not merely ids some Workspace happens to have installed --
// carrying special, code-level meaning process-wide, in every Workspace, regardless of Workspace
// isolation. A generated Machine may never take one, even in a Workspace that has never installed
// Document Approval or Project Management at all.
//
// Found 2026-09-27, the same day Workspace isolation shipped: a generated "Document Tracking"
// Application in the empty "Dokter Kecil" Workspace named its own Machine mch_document -- a
// perfectly reasonable name nobody told the assistant was already special -- and
// internal/web.documentSignaturePlacementView's own gate (`machine.ID != action.DocumentMachineID`)
// treated it as *the* Document Approval Document, not merely *a* document. It then looked up
// mch_approval_step, which this Workspace does not have, and passed a nil *domain.Machine into
// composition.Loader.RelationOptions, which panicked. This is a materially different risk from the
// file-collision existingStateFor/refuseIfExists guard against: before isolation, this id
// collision could never happen at all (ids were deduped process-wide, so a second Workspace's own
// mch_document would have silently *become* the first Workspace's); isolation made it a real,
// distinct Machine for the first time, and every hardcoded id check written under the old
// assumption is a latent version of this same bug.
//
// internal/composition's own Case 19 constants (taskMachineID, projectMachineID) are unexported,
// so their two ids are named here as literals rather than imported -- the same "duplicate a
// curated, reviewed subset" posture prompt.go's own composableSurface already takes for
// capabilities.md, for the identical reason: there is no exported, machine-readable form to read
// from instead. Revisit this list at the same cadence CLAUDE.md already asks for Case 19 itself:
// each new hardcoded Machine-id equality check anywhere in the engine belongs here too.
var reservedMachineIDs = map[string]bool{
	action.DocumentMachineID:     true,
	action.StepMachineID:         true,
	action.SignatureMachineID:    true,
	action.TemplateMachineID:     true,
	action.TemplateStepMachineID: true,
	"mch_task":                   true, // internal/composition/pages.go's own taskMachineID
	"mch_project":                true, // internal/composition/pages.go's own projectMachineID
}

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
}

// ExistingApplicationState is the slice of one installed Application's current metadata that
// extend_application validation reads.
type ExistingApplicationState struct {
	Roles    []string
	Machines map[string]*domain.Machine // keyed by machine id, this Application's own claimed Machines only
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
	case KindExtendApplication:
		return validateExtendApplication(change, existing)
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

	// Machine ids must be new workspace-wide (Machines are workspace-level and unique by id, per
	// domain.Workspace's own doc comment) and unique within this change too.
	seenMachine := map[string]bool{}
	knownFieldTargets := map[string]bool{} // machine ids declared by this same change -- a relation may point at a sibling
	for _, m := range app.Machines {
		knownFieldTargets[m.ID] = true
	}
	for _, m := range app.Machines {
		if reservedMachineIDs[m.ID] {
			issues = append(issues, fmt.Sprintf("machine id %q is reserved by this runtime's own hardcoded engine and may never be used by a generated machine, in any workspace", m.ID))
		}
		if existing.MachineIDs[m.ID] {
			issues = append(issues, fmt.Sprintf("machine id %q already exists in this workspace", m.ID))
		}
		if seenMachine[m.ID] {
			issues = append(issues, fmt.Sprintf("machine id %q is declared more than once in this application", m.ID))
		}
		seenMachine[m.ID] = true

		domainMachine, fieldIssues := buildDomainMachine(m, knownFieldTargets)
		issues = append(issues, fieldIssues...)
		if len(fieldIssues) > 0 {
			continue // metadata.Validate would only pile on the same field-shape complaints
		}
		if err := metadata.Validate(domainMachine); err != nil {
			issues = append(issues, fmt.Sprintf("machine %q: %v", m.ID, err))
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func validateExtendApplication(change GeneratedChange, existing ExistingState) error {
	target, ok := existing.Applications[change.TargetAppID]
	if !ok {
		return &ValidationError{Issues: []string{fmt.Sprintf("application %q is not installed in this workspace", change.TargetAppID)}}
	}
	if len(change.Additions) == 0 {
		return &ValidationError{Issues: []string{"extend_application change carries no additions"}}
	}

	existingRoles := map[string]bool{}
	for _, r := range target.Roles {
		existingRoles[r] = true
	}

	var issues []string
	for i, add := range change.Additions {
		switch {
		case add.NewOption != "":
			m, ok := target.Machines[add.MachineID]
			if !ok {
				issues = append(issues, fmt.Sprintf("addition %d: machine %q is not part of application %q", i, add.MachineID, change.TargetAppID))
				continue
			}
			f, ok := m.FieldByID(add.FieldID)
			if !ok {
				issues = append(issues, fmt.Sprintf("addition %d: field %q is not on machine %q", i, add.FieldID, add.MachineID))
				continue
			}
			if f.Type != domain.FieldTypeStatus {
				issues = append(issues, fmt.Sprintf("addition %d: field %q is not a status field, so it has no options to add to", i, add.FieldID))
				continue
			}
			if contains(f.Options, add.NewOption) {
				issues = append(issues, fmt.Sprintf("addition %d: %q is already one of field %q's declared options", i, add.NewOption, add.FieldID))
			}
		case add.NewRole != "":
			if strings.TrimSpace(add.NewRole) == "" {
				issues = append(issues, fmt.Sprintf("addition %d: new role is empty", i))
			} else if existingRoles[add.NewRole] {
				issues = append(issues, fmt.Sprintf("addition %d: role %q is already declared on application %q", i, add.NewRole, change.TargetAppID))
			}
		case add.NewNavItem != nil:
			if _, ok := target.Machines[add.NewNavItem.MachineID]; !ok {
				issues = append(issues, fmt.Sprintf("addition %d: nav item points at machine %q, which is not part of application %q", i, add.NewNavItem.MachineID, change.TargetAppID))
			}
			if strings.TrimSpace(add.NewNavItem.ID) == "" || strings.TrimSpace(add.NewNavItem.Label) == "" {
				issues = append(issues, fmt.Sprintf("addition %d: nav item needs an id and a label", i))
			}
		default:
			issues = append(issues, fmt.Sprintf("addition %d: names no actual change (no option, role, or nav item)", i))
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// buildDomainMachine constructs a real domain.Machine from a GeneratedMachine, plus any issues
// metadata.Validate has no way to express (a relation field naming a machine outside this same
// change, since that's a cross-change constraint, not a per-Machine one). knownSiblings names
// every machine id this same GeneratedChange declares.
func buildDomainMachine(gm GeneratedMachine, knownSiblings map[string]bool) (*domain.Machine, []string) {
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
			if !knownSiblings[gf.RelatedMachine] {
				issues = append(issues, fmt.Sprintf("machine %q field %q: relation target %q is not a machine in this same application", gm.ID, gf.ID, gf.RelatedMachine))
			}
		}
		if f.Type == domain.FieldTypePerson {
			f.RelatedMachine = domain.UserMachineID
		}
		m.Fields = append(m.Fields, f)
	}
	for _, gp := range gm.Permissions {
		if gp.Action == domain.ActionDecide || gp.Action == domain.ActionRevise {
			issues = append(issues, fmt.Sprintf("machine %q permission %q: action %q is a hardcoded workflow action, not available to a generated machine", gm.ID, gp.ID, gp.Action))
			continue
		}
		m.Permissions = append(m.Permissions, domain.Permission{ID: gp.ID, Action: gp.Action, Roles: gp.Roles})
	}
	for _, gt := range gm.Transitions {
		m.Transitions = append(m.Transitions, domain.Transition{
			ID: gt.ID, Name: gt.Name, Field: gt.Field, From: gt.From, To: gt.To,
			Action: domain.ActionEdit,
		})
	}
	for _, ge := range gm.Events {
		m.Events = append(m.Events, domain.Event{
			ID: ge.ID, On: ge.On, WhenEquals: ge.WhenEquals, OnCreate: ge.OnCreate,
			Then: domain.Service{Name: domain.ServiceLogActivity, Summary: ge.Summary},
		})
	}
	return m, issues
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
