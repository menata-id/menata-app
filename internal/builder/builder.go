package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"menata.app/internal/aiassist"
	"menata.app/internal/installer"
	"menata.app/internal/metadata"
)

// Operation kinds. Each edits one key of the loader structs; capabilityOf names it.
const (
	OpRenameMachine      = "rename_machine"
	OpRenameField        = "rename_field"
	OpSetRequired        = "set_required"
	OpAddOption          = "add_option"
	OpRemoveOption       = "remove_option"
	OpRemoveField        = "remove_field"
	OpSetPermissionRoles = "set_permission_roles"
	OpAddField           = "add_field"
	OpSetDefault         = "set_default"
	OpAddPermission      = "add_permission"
	OpSetEditableBy      = "set_editable_by"
)

// Op is one edit. Machine, Field and Permission are ids (a name is never an identity); Value is a new name or
// option; Flag is the new boolean; Roles the new role set.
type Op struct {
	Kind       string   `json:"kind"`
	Machine    string   `json:"machine,omitempty"`
	Field      string   `json:"field,omitempty"`
	Permission string   `json:"permission,omitempty"`
	Value      string   `json:"value,omitempty"`
	Flag       bool     `json:"flag,omitempty"`
	Roles      []string `json:"roles,omitempty"`
	// Type, Options and Action describe a new thing: add_field takes Field (its id), Value (its name), Type, Flag (required)
	// and Options; add_permission takes Permission (its id), Action and Roles.
	Type    string   `json:"type,omitempty"`
	Options []string `json:"options,omitempty"`
	Action  string   `json:"action,omitempty"`
	// Category, on add_option and add_field, is the colour-meaning (start, active, attention, done, stopped) the new option is drawn in; it is
	// written into the Machine's status_categories. add_field takes it for its Options through Categories (option -> category).
	Category   string            `json:"category,omitempty"`
	Categories map[string]string `json:"categories,omitempty"`
}

// capabilityOf maps each operation to the registry.Capabilities entry of the key it writes.
var capabilityOf = map[string]string{
	OpRenameMachine:      "machineDoc.name",
	OpRenameField:        "fieldDoc.name",
	OpSetRequired:        "fieldDoc.required",
	OpAddOption:          "fieldDoc.options",
	OpRemoveOption:       "fieldDoc.options",
	OpRemoveField:        "machineDoc.fields",
	OpSetPermissionRoles: "permissionDoc.roles",
	OpAddField:           "machineDoc.fields",
	OpSetDefault:         "fieldDoc.default",
	OpAddPermission:      "machineDoc.permissions",
	OpSetEditableBy:      "fieldDoc.editable_by",
}

// Capability is the catalogue id of the key an operation of this kind edits, or "" for an unknown kind.
func Capability(kind string) string { return capabilityOf[kind] }

// Kinds lists the operation kinds in declaration order.
func Kinds() []string {
	out := make([]string, 0, len(capabilityOf))
	for k := range capabilityOf {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Apply returns the Application as it should be once ops are applied to current, in order. It edits a copy.
// An id that names nothing is an error; whether the result is allowed is PlanUpdate's question, not this one's.
func Apply(current aiassist.GeneratedApplication, ops []Op) (aiassist.GeneratedApplication, error) {
	app := clone(current)
	for i, op := range ops {
		if err := apply(&app, op); err != nil {
			return aiassist.GeneratedApplication{}, fmt.Errorf("operation %d (%s): %w", i+1, op.Kind, err)
		}
	}
	return app, nil
}

func apply(app *aiassist.GeneratedApplication, op Op) error {
	m := machineByID(app, op.Machine)
	if m == nil {
		return fmt.Errorf("machine %q is not in application %s", op.Machine, app.ID)
	}
	switch op.Kind {
	case OpRenameMachine:
		m.Name = op.Value
	case OpSetPermissionRoles:
		for i := range m.Permissions {
			if m.Permissions[i].ID == op.Permission {
				m.Permissions[i].Roles = slices.Clone(op.Roles)
				return nil
			}
		}
		return fmt.Errorf("permission %q is not on machine %s", op.Permission, m.ID)
	case OpAddPermission:
		if slices.ContainsFunc(m.Permissions, func(x aiassist.GeneratedPermission) bool { return x.ID == op.Permission }) {
			return fmt.Errorf("permission %q already exists on machine %s", op.Permission, m.ID)
		}
		m.Permissions = append(m.Permissions, aiassist.GeneratedPermission{ID: op.Permission, Action: op.Action, Roles: slices.Clone(op.Roles)})
	case OpAddField:
		if slices.ContainsFunc(m.Fields, func(f aiassist.GeneratedField) bool { return f.ID == op.Field }) {
			return fmt.Errorf("field %q already exists on machine %s", op.Field, m.ID)
		}
		m.Fields = append(m.Fields, aiassist.GeneratedField{ID: op.Field, Name: op.Value, Type: op.Type, Required: op.Flag, Options: slices.Clone(op.Options)})
		nf := &m.Fields[len(m.Fields)-1]
		for _, o := range nf.Options {
			if c := op.Categories[o]; c != "" {
				if err := categorise(m, nf, o, c); err != nil {
					return err
				}
			}
		}
	case OpRemoveField:
		i := slices.IndexFunc(m.Fields, func(f aiassist.GeneratedField) bool { return f.ID == op.Field })
		if i < 0 {
			return fmt.Errorf("field %q is not on machine %s", op.Field, m.ID)
		}
		m.Fields = slices.Delete(m.Fields, i, i+1)
	case OpRenameField, OpSetRequired, OpSetDefault, OpAddOption, OpRemoveOption, OpSetEditableBy:
		i := slices.IndexFunc(m.Fields, func(f aiassist.GeneratedField) bool { return f.ID == op.Field })
		if i < 0 {
			return fmt.Errorf("field %q is not on machine %s", op.Field, m.ID)
		}
		f := &m.Fields[i]
		switch op.Kind {
		case OpRenameField:
			f.Name = op.Value
		case OpSetRequired:
			f.Required = op.Flag
		case OpSetDefault:
			f.Default = op.Value
		case OpSetEditableBy:
			f.EditableBy = slices.Clone(op.Roles)
		case OpAddOption:
			if slices.Contains(f.Options, op.Value) {
				return fmt.Errorf("option %q already exists on %s", op.Value, f.ID)
			}
			f.Options = append(f.Options, op.Value)
			if op.Category != "" {
				if err := categorise(m, f, op.Value, op.Category); err != nil {
					return err
				}
			}
		case OpRemoveOption:
			j := slices.Index(f.Options, op.Value)
			if j < 0 {
				return fmt.Errorf("option %q is not on %s", op.Value, f.ID)
			}
			f.Options = slices.Delete(f.Options, j, j+1)
			// The colour goes with the choice, so a choice added and removed in one draft leaves nothing behind (an installed one is refused by the plan).
			if sc := m.StatusCategories; sc != nil && sc.Field == f.ID {
				sc.Values = slices.DeleteFunc(sc.Values, func(v aiassist.GeneratedStatusCategory) bool { return v.Option == op.Value })
			}
		}
	default:
		return fmt.Errorf("unknown operation kind %q", op.Kind)
	}
	return nil
}

// categorise gives an option of a status Field the category it is drawn in, in the Machine's status_categories. A Machine tracks one status that way
// (the one its status_categories names, or the first it is given), so a second status cannot be coloured here.
func categorise(m *aiassist.GeneratedMachine, f *aiassist.GeneratedField, option, category string) error {
	if f.Type != "status" {
		return fmt.Errorf("%s is not a status, so its options have no colour", f.ID)
	}
	if m.StatusCategories == nil {
		m.StatusCategories = &aiassist.GeneratedStatusCategories{Field: f.ID}
	}
	if m.StatusCategories.Field != f.ID {
		return fmt.Errorf("machine %s colours the options of %s, not of %s", m.ID, m.StatusCategories.Field, f.ID)
	}
	m.StatusCategories.Values = append(m.StatusCategories.Values, aiassist.GeneratedStatusCategory{Option: option, Category: category})
	return nil
}

func machineByID(app *aiassist.GeneratedApplication, id string) *aiassist.GeneratedMachine {
	for i := range app.Machines {
		if app.Machines[i].ID == id {
			return &app.Machines[i]
		}
	}
	return nil
}

// clone copies the parts Apply edits so the caller's value is never touched.
func clone(a aiassist.GeneratedApplication) aiassist.GeneratedApplication {
	a.Machines = slices.Clone(a.Machines)
	for i := range a.Machines {
		m := &a.Machines[i]
		m.Fields = slices.Clone(m.Fields)
		for j := range m.Fields {
			m.Fields[j].Options = slices.Clone(m.Fields[j].Options)
		}
		m.Permissions = slices.Clone(m.Permissions)
		if sc := m.StatusCategories; sc != nil {
			m.StatusCategories = &aiassist.GeneratedStatusCategories{Field: sc.Field, Values: slices.Clone(sc.Values)}
		}
	}
	return a
}

// Plan is what a list of operations would do, before anything is written.
type Plan struct {
	// Change is the whole desired state the operations produce, in the shape the writer takes.
	Change aiassist.GeneratedChange
	// Plan is PlanUpdate's account: what is added, changed, removed (waiting on the owner) or refused.
	Plan aiassist.Plan
	// Files is the writer's own output diffed with the live files (the Code tab).
	Files []aiassist.FileChange
	// Version names the installation the plan was made against (Version); Commit refuses a plan whose version is no longer the live one.
	Version string
}

// Review applies ops to the installed Application and returns the plan and the diff. It writes nothing. A refused
// plan returns the Plan with an installer.RejectedError; a refusal is a finding for the person, not a fault.
func Review(workspaceManifestPath, appID string, ops []Op) (Plan, error) {
	loaded, err := metadata.LoadApplication(workspaceManifestPath)
	if err != nil {
		return Plan{}, fmt.Errorf("load workspace: %w", err)
	}
	state := aiassist.ExistingStateFrom(loaded.Workspace)
	cur, ok := state.Applications[appID]
	if !ok {
		return Plan{}, installer.Rejected(fmt.Errorf("application %q is not installed in this workspace", appID))
	}
	current := aiassist.DescribeApplication(appID, cur)
	version, err := Version(workspaceManifestPath)
	if err != nil {
		return Plan{}, err
	}
	desired, err := Apply(current, ops)
	if err != nil {
		return Plan{}, installer.Rejected(err)
	}
	out := Plan{
		Version: version,
		Change:  aiassist.GeneratedChange{Kind: aiassist.KindUpdateApplication, TargetAppID: appID, Application: &desired},
		Plan:    aiassist.PlanUpdate(current, desired),
	}
	if err := aiassist.Validate(out.Change, state); err != nil {
		return out, installer.Rejected(err)
	}
	if out.Files, err = aiassist.Preview(workspaceManifestPath, out.Change); err != nil {
		return out, err
	}
	return out, nil
}

// ErrStale is a plan made against an installation that has since changed (another admin saved, or the assistant published).
// The person sees the plan again against what is there now; the edit is not applied blind.
var ErrStale = errors.New("the application changed since this plan was made; review it again")

// lock serialises Commit per Workspace manifest, so the version check and the write are one step within the process.
var lock sync.Map

// Commit writes the plan Review made for the same ops, if the installation is still the one it was made against
// (version) and every removal the plan asks for is among confirmed. It is the single write path: aiassist.Write, the
// same writer a published conversation uses, which load-verifies and rolls back. Snapshot and reload belong to the caller.
func Commit(workspaceManifestPath, appID string, ops []Op, version string, confirmed []string) error {
	mu, _ := lock.LoadOrStore(workspaceManifestPath, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	live, err := Version(workspaceManifestPath)
	if err != nil {
		return err
	}
	if live != version {
		return ErrStale
	}
	plan, err := Review(workspaceManifestPath, appID, ops)
	if err != nil {
		return err
	}
	if open := plan.Plan.Unconfirmed(confirmed); len(open) > 0 {
		return installer.Rejected(fmt.Errorf("this update removes something the owner has not confirmed: %s", strings.Join(open, ", ")))
	}
	plan.Change.ConfirmedRemovals = confirmed
	_, err = aiassist.Write(workspaceManifestPath, plan.Change, aiassist.FileMachineResolver{WorkspaceManifestPath: workspaceManifestPath})
	return err
}

// Staged is the Application as installed with ops applied and nothing else -- no validation, no write -- so a form can start from, and be compared with,
// what the admin's draft already says rather than what is installed.
func Staged(workspaceManifestPath, appID string, ops []Op) (aiassist.GeneratedApplication, error) {
	loaded, err := metadata.LoadApplication(workspaceManifestPath)
	if err != nil {
		return aiassist.GeneratedApplication{}, fmt.Errorf("load workspace: %w", err)
	}
	cur, ok := aiassist.ExistingStateFrom(loaded.Workspace).Applications[appID]
	if !ok {
		return aiassist.GeneratedApplication{}, installer.Rejected(fmt.Errorf("application %q is not installed in this workspace", appID))
	}
	desired, err := Apply(aiassist.DescribeApplication(appID, cur), ops)
	if err != nil {
		return aiassist.GeneratedApplication{}, installer.Rejected(err)
	}
	return desired, nil
}

// AppVersion names the state of one Application as the builder edits it: a hash of its description (its Machines, Fields, options, permissions, menu), not of the
// Workspace's files, so a draft is stale when *its Application* changed under it and not whenever anything in the Workspace was published.
func AppVersion(workspaceManifestPath, appID string) (string, error) {
	loaded, err := metadata.LoadApplication(workspaceManifestPath)
	if err != nil {
		return "", fmt.Errorf("load workspace: %w", err)
	}
	cur, ok := aiassist.ExistingStateFrom(loaded.Workspace).Applications[appID]
	if !ok {
		return "", installer.Rejected(fmt.Errorf("application %q is not installed in this workspace", appID))
	}
	raw, err := json.Marshal(aiassist.DescribeApplication(appID, cur))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
