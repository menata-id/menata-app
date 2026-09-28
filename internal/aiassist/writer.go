package aiassist

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"menata.app/internal/installer"
	"menata.app/internal/metadata"
)

// Write applies a validated GeneratedChange to disk: a brand-new Application into the target
// Workspace's **own** directory, or a surgical append into an already-installed one's own files
// for an extension. Every write is provably additive by construction (see this package's own doc
// comment) -- nothing here ever rewrites an existing value or removes a line.
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
	case KindExtendApplication:
		err = writeExtension(written, workspaceManifestPath, change, resolve)
	default:
		err = fmt.Errorf("unknown change kind %q", change.Kind)
	}
	if err != nil {
		return "", err
	}

	if _, err = metadata.LoadApplication(workspaceManifestPath); err != nil {
		return "", fmt.Errorf("generated metadata was written but does not load, so it has been rolled back: %w", err)
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
	Permissions []permissionDoc `yaml:"permissions,omitempty"`
	Transitions []transitionDoc `yaml:"transitions,omitempty"`
	Events      []eventDoc      `yaml:"events,omitempty"`
}

type fieldDoc struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Required bool     `yaml:"required,omitempty"`
	Options  []string `yaml:"options,omitempty"`
	Machine  string   `yaml:"machine,omitempty"`
}

type permissionDoc struct {
	ID     string   `yaml:"id"`
	Action string   `yaml:"action"`
	Roles  []string `yaml:"roles,omitempty"`
}

type transitionDoc struct {
	ID     string `yaml:"id"`
	Name   string `yaml:"name"`
	Field  string `yaml:"field"`
	From   string `yaml:"from"`
	To     string `yaml:"to"`
	Action string `yaml:"action"`
}

type eventDoc struct {
	ID         string `yaml:"id"`
	On         string `yaml:"on,omitempty"`
	WhenEquals string `yaml:"when_equals,omitempty"`
	OnCreate   bool   `yaml:"on_create,omitempty"`
	Then       struct {
		Service string `yaml:"service"`
		Summary string `yaml:"summary"`
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
}

func writeNewApplication(written *installer.WriteSet, workspaceManifestPath string, change GeneratedChange) (string, error) {
	app := change.Application
	workspaceDir := filepath.Dir(workspaceManifestPath)
	// This Workspace's own namespace: metadata/workspaces/<slug>/, named from its manifest rather
	// than passed in, so the two can never disagree about which Workspace is being written to.
	ownDir := filepath.Join(workspaceDir, strings.TrimSuffix(filepath.Base(workspaceManifestPath), filepath.Ext(workspaceManifestPath)))

	var machineRelPaths []string
	for _, m := range app.Machines {
		doc := machineDoc{ID: m.ID, Name: m.Name}
		for _, f := range m.Fields {
			doc.Fields = append(doc.Fields, fieldDoc{
				ID: f.ID, Name: f.Name, Type: f.Type, Required: f.Required,
				Options: f.Options, Machine: f.RelatedMachine,
			})
		}
		for _, p := range m.Permissions {
			doc.Permissions = append(doc.Permissions, permissionDoc{ID: p.ID, Action: p.Action, Roles: p.Roles})
		}
		for _, t := range m.Transitions {
			doc.Transitions = append(doc.Transitions, transitionDoc{ID: t.ID, Name: t.Name, Field: t.Field, From: t.From, To: t.To, Action: "edit"})
		}
		for _, e := range m.Events {
			var ed eventDoc
			ed.ID, ed.On, ed.WhenEquals, ed.OnCreate = e.ID, e.On, e.WhenEquals, e.OnCreate
			ed.Then.Service, ed.Then.Summary = "log_activity", e.Summary
			doc.Events = append(doc.Events, ed)
		}

		filename := m.ID[len("mch_"):] + ".yaml"
		absPath := filepath.Join(ownDir, filename)
		if err := installer.RefuseIfExists(absPath, "machine "+m.ID); err != nil {
			return "", err
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
	// A generated Application otherwise declares no navigation: at all, and domain.Workspace.
	// HomeRoute (see its own doc comment) is resolved from a home_card: true item or is empty --
	// so its Workspace Home card had no destination and linked back to /home, found 2026-09-27
	// alongside the missing publisher_role in the same conversation. One item, pointing at the
	// first Machine's own generic list page (/machines/{id}, the one route this package can always
	// name regardless of what the Application declares -- the same reasoning GeneratedNavItem's own
	// doc comment already gives for extend_application), is enough to make the card go somewhere.
	appDoc.Navigation = []navItemDoc{{
		ID: "nav_" + app.ID[len("app_"):], Label: app.Name,
		Route: "/machines/" + app.Machines[0].ID, HomeCard: true,
	}}
	appFilename := app.ID[len("app_"):] + ".yaml"
	appAbsPath := filepath.Join(ownDir, "applications", appFilename)
	if err := installer.RefuseIfExists(appAbsPath, "application "+app.ID); err != nil {
		return "", err
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

func writeExtension(written *installer.WriteSet, workspaceManifestPath string, change GeneratedChange, resolve MachineFileResolver) error {
	workspaceDir := filepath.Dir(workspaceManifestPath)

	for _, add := range change.Additions {
		switch {
		case add.NewOption != "":
			rel, ok := resolve.MachineFile(add.MachineID)
			if ok != nil {
				return fmt.Errorf("resolve file for machine %s: %w", add.MachineID, ok)
			}
			path := filepath.Join(workspaceDir, rel)
			src, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read machine file %s: %w", path, err)
			}
			updated, err := appendFlowListItemNearAnchor(src, "id: "+add.FieldID, "options:", add.NewOption)
			if err != nil {
				return fmt.Errorf("append option to %s: %w", path, err)
			}
			if err := written.Note(path); err != nil {
				return err
			}
			if err := installer.WriteFileStrict[installer.FullMachineCheckDoc](path, updated); err != nil {
				return err
			}
		case add.NewRole != "":
			rel, err := resolveApplicationFile(workspaceManifestPath, change.TargetAppID)
			if err != nil {
				return err
			}
			path := filepath.Join(workspaceDir, rel)
			src, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read application file %s: %w", path, err)
			}
			updated, err := installer.AppendBlockListItem(src, "roles:", add.NewRole)
			if err != nil {
				return fmt.Errorf("append role to %s: %w", path, err)
			}
			if err := written.Note(path); err != nil {
				return err
			}
			if err := installer.WriteFileStrict[installer.FullApplicationCheckDoc](path, updated); err != nil {
				return err
			}
		case add.NewNavItem != nil:
			return fmt.Errorf("generated navigation additions are not implemented yet -- named here rather than silently accepted")
		}
	}
	return nil
}

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

// --- surgical YAML text edits -------------------------------------------------------------------

// appendFlowListItemNearAnchor finds anchor (e.g. "id: fld_status") to scope the search, then the
// next "key: [...]" line after it (e.g. "options: [...]"), and inserts newItem before the closing
// bracket. Scoped to the anchor rather than the first match in the whole file, since a Machine
// file can declare the same key (options:) on more than one Field.
func appendFlowListItemNearAnchor(src []byte, anchor, key, newItem string) ([]byte, error) {
	text := string(src)
	anchorIdx := strings.Index(text, anchor)
	if anchorIdx == -1 {
		return nil, fmt.Errorf("anchor %q not found", anchor)
	}
	rest := text[anchorIdx:]

	flowPattern := regexp.MustCompile(`(?m)^(\s*` + regexp.QuoteMeta(key) + `\s*\[)([^\]]*)(\])`)
	loc := flowPattern.FindStringSubmatchIndex(rest)
	if loc == nil {
		return nil, fmt.Errorf("no flow-style %q list found after %q -- refusing rather than guessing a shape", key, anchor)
	}
	existingItems := rest[loc[4]:loc[5]]
	replacement := rest[loc[2]:loc[3]] + strings.TrimRight(existingItems, " ") + ", " + newItem + rest[loc[6]:loc[7]]
	newRest := rest[:loc[0]] + replacement + rest[loc[1]:]
	return []byte(text[:anchorIdx] + newRest), nil
}

func toSlash(p string) string {
	return filepath.ToSlash(p)
}
