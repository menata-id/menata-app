package aiassist

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"menata.app/internal/installer"
	"menata.app/internal/metadata"
)

// Write applies a validated GeneratedChange to disk: a brand-new Application into the target
// Workspace's **own** directory, or a surgical append into an already-installed one's own files
// for an extension. A new Application only adds files. An update edits the files it was shown as
// yaml.Node documents: it adds, may change display text and a Permission's roles, drops a menu item,
// and removes a Field only when the owner confirmed it (GeneratedChange.ConfirmedRemovals) -- see
// PlanUpdate for the whole policy, which is the one place that says what an update may do.
//
// workspaceManifestPath is the target Workspace's own metadata/workspaces/<slug>.yaml, and
// everything a new Application brings is written beside it under metadata/workspaces/<slug>/ --
// that Workspace's namespace, nobody else's. Until 2026-09-27 new files landed in the shared
// metadata/ and metadata/applications/ directories instead, which is how a generated Application
// naming its Machine mch_document came to overwrite the real Document Approval one another
// Workspace had installed (ROADMAP.md). metadata/ still holds the *templates* an Application is
// installed from; installing copies them, and a copy lives here.
//
// Every file this function touches is written to a temp file and renamed into place only after a
// strict re-parse (KnownFields) succeeds on the freshly-written bytes -- a bug in this function
// can therefore never leave a half-written or unparseable file for the caller's reload to trip
// over. Returns the new Application's own id (for new_application) so the caller can redirect to
// its HomeRoute once the reload picks it up.
//
// **Nothing survives a failure.** Once every file is in place this loads the whole manifest back
// through metadata.LoadApplication -- the real loader, the same one the caller's reload is about
// to run -- and undoes every write if it does not load (installer.WriteSet.rollback). That ordering is the
// whole point: validation before writing can only ever check the rules someone remembered to
// copy into Validate, and on 2026-09-27 one of them had not been (a Permission naming a role its
// Application does not declare, which only validatePermissionRoles catches, and which is
// unexported). The result was metadata written to disk that then failed to reload: the running
// app kept serving its old route table, but the next process restart would have refused to load
// that Workspace at all. Loading is not a copy of the rules -- it *is* them -- so this cannot
// drift out of step the way a second list of checks always eventually does.
func Write(workspaceManifestPath string, change GeneratedChange, resolve MachineFileResolver) (newAppID string, err error) {
	written := installer.NewWriteSet()
	defer func() {
		if err != nil {
			written.Rollback()
		}
	}()

	switch change.Kind {
	case KindNewApplication:
		newAppID, err = writeNewApplication(written, workspaceManifestPath, change)
	case KindUpdateApplication:
		err = writeUpdate(written, workspaceManifestPath, change, resolve)
	default:
		err = installer.Rejected(fmt.Errorf("unknown change kind %q", change.Kind))
	}
	if err != nil {
		return "", err
	}

	if _, err = metadata.LoadApplication(workspaceManifestPath); err != nil {
		return "", installer.Rejected(fmt.Errorf("generated metadata was written but does not load, so it has been rolled back: %w", err))
	}
	return newAppID, nil
}

// MachineFileResolver answers "which file declares this machine id", relative to the workspace
// manifest's own directory -- needed for extend_application, since domain.Machine carries no
// record of the file it was loaded from. Kept as an interface so this package's own tests can
// supply a fixed map instead of real files; internal/web's handlers use FileMachineResolver below.
type MachineFileResolver interface {
	MachineFile(machineID string) (relativePath string, ok error)
}

// FileMachineResolver is the real MachineFileResolver: reads the target workspace manifest's own
// machines: list and peeks each file's own id: field, exactly mirroring resolveApplicationFile's
// technique below for Applications -- domain.Machine carries no record of its own source file
// either, so both directions need the identical lookup.
type FileMachineResolver struct {
	WorkspaceManifestPath string
}

func (r FileMachineResolver) MachineFile(machineID string) (string, error) {
	manifest, err := os.ReadFile(r.WorkspaceManifestPath)
	if err != nil {
		return "", err
	}
	var doc struct {
		Machines []string `yaml:"machines"`
	}
	if err := yaml.Unmarshal(manifest, &doc); err != nil {
		return "", err
	}
	dir := filepath.Dir(r.WorkspaceManifestPath)
	for _, rel := range doc.Machines {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return "", err
		}
		var idDoc struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(data, &idDoc); err != nil {
			return "", err
		}
		if idDoc.ID == machineID {
			return rel, nil
		}
	}
	return "", fmt.Errorf("no machine file in %s declares id %q", r.WorkspaceManifestPath, machineID)
}

// --- new_application -------------------------------------------------------------------------

// machineDoc/applicationDoc/navItemDoc below mirror internal/metadata's own (unexported) write
// targets field-for-field -- yaml tags copied verbatim from internal/metadata/parse.go and
// application.go, so a file this package writes and a file a person hand-writes are
// indistinguishable to LoadApplication. Duplicated rather than imported because internal/metadata
// exports neither the types nor a serializer (confirmed by direct read: no yaml.Marshal call
// exists anywhere in that package today) -- this is the reverse direction hot-reload-safety.md
// §9.1 already anticipated and internal/metadata never needed until now.
type machineDoc struct {
	ID          string          `yaml:"id"`
	Name        string          `yaml:"name"`
	Fields      []fieldDoc      `yaml:"fields"`
	Completion  *completionDoc  `yaml:"completion,omitempty"`
	CardFields  []cardFieldDoc  `yaml:"card_fields,omitempty"`
	Views       []viewDoc       `yaml:"views,omitempty"`
	Permissions []permissionDoc `yaml:"permissions,omitempty"`
	Transitions []transitionDoc `yaml:"transitions,omitempty"`
	Ensure      []ensureDoc     `yaml:"ensure,omitempty"`
	FrozenWhen  *frozenWhenDoc  `yaml:"frozen_when,omitempty"`
	Events      []eventDoc      `yaml:"events,omitempty"`
	Datasets    []datasetDoc    `yaml:"datasets,omitempty"`
}

type fieldDoc struct {
	ID       string      `yaml:"id"`
	Name     string      `yaml:"name"`
	Type     string      `yaml:"type"`
	Required bool        `yaml:"required,omitempty"`
	Options  []string    `yaml:"options,omitempty"`
	Machine  string      `yaml:"machine,omitempty"`
	Compute  *computeDoc `yaml:"compute,omitempty"`
	Default  string      `yaml:"default,omitempty"`
	Stamp    string      `yaml:"stamp,omitempty"`

	RequiredWhen *requiredWhenDoc `yaml:"required_when,omitempty"`
	DefaultFrom  *defaultFromDoc  `yaml:"default_from,omitempty"`
}

type requiredWhenDoc struct {
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
}

type defaultFromDoc struct {
	Field string         `yaml:"field"`
	Days  map[string]int `yaml:"days"`
}

type frozenWhenDoc struct {
	Via    string `yaml:"via"`
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
}

type ensureDoc struct {
	ID      string   `yaml:"id"`
	SumOf   string   `yaml:"sum_of"`
	Per     string   `yaml:"per"`
	AtLeast *float64 `yaml:"at_least,omitempty"`
	AtMost  *float64 `yaml:"at_most,omitempty"`
	Reason  string   `yaml:"reason,omitempty"`
}

type cardFieldDoc struct {
	Field string `yaml:"field"`
	Role  string `yaml:"role"`
}

type viewDoc struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	GroupBy string `yaml:"group_by,omitempty"`
}

type completionDoc struct {
	Field string `yaml:"field"`
	Done  string `yaml:"done"`
}

type parentActorDoc struct {
	Via   string `yaml:"via"`
	Field string `yaml:"field"`
}

type computeDoc struct {
	Op     string   `yaml:"op"`
	Fields []string `yaml:"fields,flow"`
}

type permissionDoc struct {
	ID          string          `yaml:"id"`
	Action      string          `yaml:"action"`
	Roles       []string        `yaml:"roles,omitempty"`
	ActorField  string          `yaml:"actor_field,omitempty"`
	ParentActor *parentActorDoc `yaml:"parent_actor,omitempty"`
}

type transitionDoc struct {
	ID         string            `yaml:"id"`
	Name       string            `yaml:"name"`
	Field      string            `yaml:"field"`
	From       string            `yaml:"from"`
	To         string            `yaml:"to"`
	Action     string            `yaml:"action"`
	Roles      []string          `yaml:"roles,omitempty"`
	ActorField string            `yaml:"actor_field,omitempty"`
	Sets       map[string]string `yaml:"sets,omitempty"`
}

type eventDoc struct {
	ID         string `yaml:"id"`
	On         string `yaml:"on,omitempty"`
	WhenEquals string `yaml:"when_equals,omitempty"`
	OnCreate   bool   `yaml:"on_create,omitempty"`
	Then       struct {
		Service             string `yaml:"service"`
		Summary             string `yaml:"summary"`
		RecipientField      string `yaml:"recipient_field,omitempty"`
		RecipientRole       string `yaml:"recipient_role,omitempty"`
		RecipientGroupField string `yaml:"recipient_group_field,omitempty"`
		PreferenceKey       string `yaml:"preference_key,omitempty"`
	} `yaml:"then"`
}

type applicationDoc struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Machines    []string     `yaml:"machines"`
	Roles       []string     `yaml:"roles,omitempty"`
	Description string       `yaml:"description,omitempty"`
	Icon        string       `yaml:"icon,omitempty"`
	Color       string       `yaml:"color,omitempty"`
	Navigation  []navItemDoc `yaml:"navigation,omitempty"`
}

// navItemDoc mirrors internal/metadata's own (unexported) navItemDoc field-for-field, the same
// reasoning machineDoc/applicationDoc above already carry -- only the fields writeNewApplication
// ever sets are here, not the full shape a hand-written file may use.
type navItemDoc struct {
	ID       string `yaml:"id"`
	Label    string `yaml:"label"`
	Route    string `yaml:"route"`
	HomeCard bool   `yaml:"home_card"`
	// Page is a menu entry's declared screen (a list, W5-a); its route is then /pages/<id>.
	Page *pageNodeDoc `yaml:"page,omitempty"`
}

func writeNewApplication(written *installer.WriteSet, workspaceManifestPath string, change GeneratedChange) (string, error) {
	app := change.Application
	workspaceDir := filepath.Dir(workspaceManifestPath)
	// This Workspace's own namespace: metadata/workspaces/<slug>/, named from its manifest rather
	// than passed in, so the two can never disagree about which Workspace is being written to.
	ownDir, err := installer.WorkspaceOwnDir(workspaceManifestPath)
	if err != nil {
		return "", err
	}

	menu := expandMenu(app.Navigation)
	navIDs := assignNavIDs(app.ID, menu, nil, map[string]bool{}, true)
	lists := listedItems(menu, navIDs, nil)

	var machineRelPaths []string
	for _, m := range app.Machines {
		doc := machineDocFor(m)
		for _, l := range lists {
			if l.Item.MachineID == m.ID {
				doc.Datasets = append(doc.Datasets, datasetFor(l))
			}
		}

		filename := m.ID[len("mch_"):] + ".yaml"
		absPath := filepath.Join(ownDir, filename)
		if err := installer.RefuseIfExists(absPath, "machine "+m.ID); err != nil {
			return "", installer.Rejected(err)
		}
		if err := written.Note(absPath); err != nil {
			return "", err
		}
		if err := installer.WriteYAMLStrict(absPath, doc); err != nil {
			return "", fmt.Errorf("write machine %s: %w", m.ID, err)
		}
		relToWorkspace, err := filepath.Rel(workspaceDir, absPath)
		if err != nil {
			return "", err
		}
		machineRelPaths = append(machineRelPaths, relToWorkspace)
	}

	appDoc := applicationDoc{
		ID: app.ID, Name: app.Name, Description: app.Description, Icon: app.Icon, Color: app.Color,
		Roles: app.Roles,
	}
	for _, m := range app.Machines {
		appDoc.Machines = append(appDoc.Machines, m.ID)
	}
	// The menu is the person's answer (GeneratedApplication.Navigation), written as it was given.
	// The first entry carries home_card: true, which is what domain.Workspace.HomeRoute resolves
	// from; without one the Workspace Home card linked back to /home (2026-09-27). Ids are not the
	// person's concern: the first keeps the id a generated Application has always had, the rest are
	// qualified by the Application's own id, which is unique in the Workspace.
	machinesByID := map[string]GeneratedMachine{}
	for _, m := range app.Machines {
		machinesByID[m.ID] = m
	}
	for i, item := range menu {
		nav := navItemDoc{ID: navIDs[i], Label: item.Label, Route: "/machines/" + item.MachineID, HomeCard: i == 0}
		if l, ok := listFor(lists, navIDs[i]); ok {
			page := pageFor(l, machinesByID[item.MachineID])
			nav.Route, nav.Page = "/pages/"+navIDs[i], &page
		}
		appDoc.Navigation = append(appDoc.Navigation, nav)
	}
	appFilename := app.ID[len("app_"):] + ".yaml"
	appAbsPath := filepath.Join(ownDir, "applications", appFilename)
	if err := installer.RefuseIfExists(appAbsPath, "application "+app.ID); err != nil {
		return "", installer.Rejected(err)
	}
	if err := written.Note(appAbsPath); err != nil {
		return "", err
	}
	if err := installer.WriteYAMLStrict(appAbsPath, appDoc); err != nil {
		return "", fmt.Errorf("write application %s: %w", app.ID, err)
	}
	appRelToWorkspace, err := filepath.Rel(workspaceDir, appAbsPath)
	if err != nil {
		return "", err
	}

	manifest, err := os.ReadFile(workspaceManifestPath)
	if err != nil {
		return "", fmt.Errorf("read workspace manifest: %w", err)
	}
	updated := manifest
	for _, rel := range machineRelPaths {
		updated, err = installer.AppendBlockListItem(updated, "machines:", toSlash(rel))
		if err != nil {
			return "", fmt.Errorf("append machine to workspace manifest: %w", err)
		}
	}
	updated, err = installer.AppendBlockListItem(updated, "applications:", toSlash(appRelToWorkspace))
	if err != nil {
		return "", fmt.Errorf("append application to workspace manifest: %w", err)
	}
	if err := written.Note(workspaceManifestPath); err != nil {
		return "", err
	}
	if err := installer.WriteFileStrict[installer.WorkspaceManifestCheckDoc](workspaceManifestPath, updated); err != nil {
		return "", fmt.Errorf("write workspace manifest: %w", err)
	}
	return app.ID, nil
}

// --- extend_application ------------------------------------------------------------------------

// resolveApplicationFile finds the target Application's own file path by reading the workspace
// manifest's applications: list and peeking each file's own id: -- the same technique
// MachineFileResolver uses for Machines, needed here too since domain.Application carries no
// record of its own source file either.
func resolveApplicationFile(workspaceManifestPath, targetAppID string) (string, error) {
	manifest, err := os.ReadFile(workspaceManifestPath)
	if err != nil {
		return "", err
	}
	var doc struct {
		Applications []string `yaml:"applications"`
	}
	if err := yaml.Unmarshal(manifest, &doc); err != nil {
		return "", err
	}
	dir := filepath.Dir(workspaceManifestPath)
	for _, rel := range doc.Applications {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return "", err
		}
		var idDoc struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(data, &idDoc); err != nil {
			return "", err
		}
		if idDoc.ID == targetAppID {
			return rel, nil
		}
	}
	return "", fmt.Errorf("no application file in %s declares id %q", workspaceManifestPath, targetAppID)
}

func toSlash(p string) string {
	return filepath.ToSlash(p)
}

// machineDocFor is the file a generated Machine is written as, whether it arrives with a new
// Application or is added to an installed one.
func machineDocFor(m GeneratedMachine) machineDoc {
	doc := machineDoc{ID: m.ID, Name: m.Name}
	for _, f := range m.Fields {
		fd := fieldDoc{
			ID: f.ID, Name: f.Name, Type: f.Type, Required: f.Required,
			Options: f.Options, Machine: f.RelatedMachine, Default: f.Default, Stamp: f.Stamp,
		}
		if f.Compute != nil {
			fd.Compute = &computeDoc{Op: f.Compute.Op, Fields: f.Compute.Fields}
		}
		if f.RequiredWhen != nil {
			fd.RequiredWhen = &requiredWhenDoc{Field: f.RequiredWhen.Field, Equals: f.RequiredWhen.Equals}
		}
		if f.DefaultFrom != nil {
			days := make(map[string]int, len(f.DefaultFrom.Days))
			for _, d := range f.DefaultFrom.Days {
				days[d.Value] = d.Days
			}
			fd.DefaultFrom = &defaultFromDoc{Field: f.DefaultFrom.Field, Days: days}
		}
		doc.Fields = append(doc.Fields, fd)
	}
	if m.Completion != nil {
		doc.Completion = &completionDoc{Field: m.Completion.Field, Done: m.Completion.Done}
	}
	for _, cf := range m.CardFields {
		doc.CardFields = append(doc.CardFields, cardFieldDoc{Field: cf.Field, Role: cf.Role})
	}
	for _, v := range m.Views {
		doc.Views = append(doc.Views, viewDoc{ID: v.ID, Name: v.Name, Type: v.Type, GroupBy: v.GroupBy})
	}
	for _, p := range m.Permissions {
		pd := permissionDoc{ID: p.ID, Action: p.Action, Roles: p.Roles, ActorField: p.ActorField}
		if p.ParentActor != nil {
			pd.ParentActor = &parentActorDoc{Via: p.ParentActor.Via, Field: p.ParentActor.Field}
		}
		doc.Permissions = append(doc.Permissions, pd)
	}
	for _, t := range m.Transitions {
		doc.Transitions = append(doc.Transitions, transitionDoc{ID: t.ID, Name: t.Name, Field: t.Field, From: t.From, To: t.To, Action: "edit", Roles: t.Roles, ActorField: t.ActorField, Sets: setsOf(t.Sets)})
	}
	if m.FrozenWhen != nil {
		doc.FrozenWhen = &frozenWhenDoc{Via: m.FrozenWhen.Via, Field: m.FrozenWhen.Field, Equals: m.FrozenWhen.Equals}
	}
	for _, e := range m.Ensure {
		doc.Ensure = append(doc.Ensure, ensureDoc{ID: e.ID, SumOf: e.SumOf, Per: e.Per, AtLeast: e.AtLeast, AtMost: e.AtMost, Reason: e.Reason})
	}
	for _, e := range m.Events {
		var ed eventDoc
		ed.ID, ed.On, ed.WhenEquals, ed.OnCreate = e.ID, e.On, e.WhenEquals, e.OnCreate
		ed.Then.Service, ed.Then.Summary = e.EventService(), e.Summary
		ed.Then.RecipientField, ed.Then.PreferenceKey = e.RecipientField, e.PreferenceKey
		ed.Then.RecipientRole, ed.Then.RecipientGroupField = e.RecipientRole, e.RecipientGroupField
		doc.Events = append(doc.Events, ed)
	}
	return doc
}
