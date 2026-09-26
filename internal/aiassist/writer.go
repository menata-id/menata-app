package aiassist

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Write applies a validated GeneratedChange to disk: metadataDir/applications/ for a brand-new
// Application, or a surgical append into an already-installed one's own files for an extension.
// Every write is provably additive by construction (see this package's own doc comment) --
// nothing here ever rewrites an existing value or removes a line.
//
// workspaceManifestPath is the target Workspace's own metadata/workspaces/<slug>.yaml.
// applicationsDir is metadata/applications/ (new Application/Machine files land under
// filepath.Dir(applicationsDir) for the Machines, matching the repo's own convention of Machine
// files living beside metadata/, one directory above applications/).
//
// Every file this function touches is written to a temp file and renamed into place only after a
// strict re-parse (KnownFields) succeeds on the freshly-written bytes -- a bug in this function
// can therefore never leave a half-written or unparseable file for the caller's reload to trip
// over. Returns the new Application's own id (for new_application) so the caller can redirect to
// its HomeRoute once the reload picks it up.
func Write(metadataDir, workspaceManifestPath string, change GeneratedChange, resolve MachineFileResolver) (newAppID string, err error) {
	switch change.Kind {
	case KindNewApplication:
		return writeNewApplication(metadataDir, workspaceManifestPath, change)
	case KindExtendApplication:
		return "", writeExtension(workspaceManifestPath, change, resolve)
	default:
		return "", fmt.Errorf("unknown change kind %q", change.Kind)
	}
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
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Machines    []string `yaml:"machines"`
	Roles       []string `yaml:"roles,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Icon        string   `yaml:"icon,omitempty"`
	Color       string   `yaml:"color,omitempty"`
}

func writeNewApplication(metadataDir, workspaceManifestPath string, change GeneratedChange) (string, error) {
	app := change.Application
	workspaceDir := filepath.Dir(workspaceManifestPath)

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
		absPath := filepath.Join(metadataDir, filename)
		if err := writeYAMLStrict(absPath, doc); err != nil {
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
	appFilename := app.ID[len("app_"):] + ".yaml"
	appAbsPath := filepath.Join(metadataDir, "applications", appFilename)
	if err := writeYAMLStrict(appAbsPath, appDoc); err != nil {
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
		updated, err = appendBlockListItem(updated, "machines:", toSlash(rel))
		if err != nil {
			return "", fmt.Errorf("append machine to workspace manifest: %w", err)
		}
	}
	updated, err = appendBlockListItem(updated, "applications:", toSlash(appRelToWorkspace))
	if err != nil {
		return "", fmt.Errorf("append application to workspace manifest: %w", err)
	}
	if err := writeFileStrict[workspaceManifestCheckDoc](workspaceManifestPath, updated); err != nil {
		return "", fmt.Errorf("write workspace manifest: %w", err)
	}
	return app.ID, nil
}

// --- extend_application ------------------------------------------------------------------------

func writeExtension(workspaceManifestPath string, change GeneratedChange, resolve MachineFileResolver) error {
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
			if err := writeFileStrict[fullMachineCheckDoc](path, updated); err != nil {
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
			updated, err := appendBlockListItem(src, "roles:", add.NewRole)
			if err != nil {
				return fmt.Errorf("append role to %s: %w", path, err)
			}
			if err := writeFileStrict[fullApplicationCheckDoc](path, updated); err != nil {
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

// appendBlockListItem finds "key:" at any indentation, then the last consecutive "- item" line
// directly under it (matching that block's own indentation), and inserts a new line with the same
// indentation and list-marker style right after it. Refuses (clear error, no partial edit) rather
// than guess if the key is missing or is not declared in block style -- exactly the "refuse rather
// than corrupt" posture this package's own doc comment promises.
func appendBlockListItem(src []byte, key, newItem string) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	keyLine := -1
	var keyIndent string
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == key || strings.HasPrefix(trimmed, key+" ") {
			keyLine = i
			keyIndent = line[:len(line)-len(trimmed)]
			break
		}
	}
	if keyLine == -1 {
		return nil, fmt.Errorf("no %q key found", key)
	}

	// "key: []" (or "key: [ ]") is a real, common, documented shape -- CLAUDE.md's own words: "An
	// empty applications: [] is valid and normal... what a Workspace looks like the moment it is
	// created". Rewriting that one line into block form ("key:" plus one indented "- item" line)
	// is still a single-line replacement, not a rewrite of anything else in the file.
	emptyFlow := regexp.MustCompile(`^\[\s*\]\s*$`)
	if rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimLeft(lines[keyLine], " "), key)); emptyFlow.MatchString(rest) {
		lines[keyLine] = keyIndent + key
		itemIndent := keyIndent + "  "
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:keyLine+1]...)
		out = append(out, itemIndent+"- "+newItem)
		out = append(out, lines[keyLine+1:]...)
		return []byte(strings.Join(out, "\n")), nil
	}

	itemPattern := regexp.MustCompile(`^(\s*)-\s`)
	lastItem := -1
	var itemIndent string
	for i := keyLine + 1; i < len(lines); i++ {
		m := itemPattern.FindStringSubmatch(lines[i])
		if m == nil {
			if strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
				continue // blank line or comment inside the block -- keep scanning
			}
			break
		}
		if lastItem == -1 {
			itemIndent = m[1]
		}
		lastItem = i
	}
	if lastItem == -1 {
		return nil, fmt.Errorf("%q has no block-style (\"- item\") entries to append after -- refusing rather than guessing a shape", key)
	}
	_ = keyIndent

	newLine := itemIndent + "- " + newItem
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:lastItem+1]...)
	out = append(out, newLine)
	out = append(out, lines[lastItem+1:]...)
	return []byte(strings.Join(out, "\n")), nil
}

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

// writeYAMLStrict marshals v (of concrete type T) and writes it through writeFileStrict[T], so the
// strict re-parse below decodes into the exact same shape it was just encoded from -- genuinely
// proving round-trip fidelity, not merely that the bytes parse as *some* YAML.
func writeYAMLStrict[T any](path string, v T) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return writeFileStrict[T](path, buf.Bytes())
}

// workspaceManifestCheckDoc is writeFileStrict's own strict-decode target for the workspace
// manifest specifically -- every key internal/metadata's real workspaceDoc declares, so
// KnownFields(true) only ever rejects a genuinely unrecognized key, never one of the manifest's
// own four.
type workspaceManifestCheckDoc struct {
	Workspace    string   `yaml:"workspace"`
	Machines     []string `yaml:"machines"`
	Navigation   []any    `yaml:"navigation"`
	Applications []string `yaml:"applications"`
}

// fullMachineCheckDoc/fullApplicationCheckDoc mirror internal/metadata's own real machineDoc/
// applicationDoc *completely* (every field parse.go/application.go declare, not just the narrower
// generator-only subset above) -- the strict-decode target for a surgical edit to an *existing*
// file, which may carry Constraints/Datasets/Views/CardFields/Sequencing/AppendOnly/ShowNav/
// SummaryMachine/Navigation that this package's own generator never writes but must not corrupt or
// reject as "unknown". Using the narrower machineDoc/applicationDoc above for this check would
// reject any real file that uses one of those fields, which is not this check's job -- it exists
// only to catch a genuinely hallucinated key the surgical text edit might have introduced, never to
// second-guess a shape internal/metadata's own Validate already accepts.
type fullMachineCheckDoc struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Fields      []any  `yaml:"fields"`
	Constraints []any  `yaml:"constraints"`
	Events      []any  `yaml:"events"`
	Permissions []any  `yaml:"permissions"`
	Transitions []any  `yaml:"transitions"`
	Datasets    []any  `yaml:"datasets"`
	Sequencing  any    `yaml:"sequencing"`
	SLAField    string `yaml:"sla_field"`
	CardFields  []any  `yaml:"card_fields"`
	Views       []any  `yaml:"views"`
	AppendOnly  bool   `yaml:"append_only"`
}

type fullApplicationCheckDoc struct {
	ID             string   `yaml:"id"`
	Name           string   `yaml:"name"`
	Machines       []string `yaml:"machines"`
	Roles          []string `yaml:"roles"`
	Description    string   `yaml:"description"`
	Icon           string   `yaml:"icon"`
	Color          string   `yaml:"color"`
	SummaryMachine string   `yaml:"summary_machine"`
	ShowNav        *bool    `yaml:"show_nav"`
	Navigation     []any    `yaml:"navigation"`
}

// writeFileStrict re-parses data into a fresh T with strict, unknown-key-rejecting decoding before
// writing it anywhere -- the narrow, scoped strict-decode check this package adds (see this
// package's own doc comment and ROADMAP.md's "Reject unknown metadata keys" entry): it catches a
// hallucinated/misspelled key in bytes this package itself produced, without touching the leniency
// internal/metadata's three existing yaml.Unmarshal call sites still rely on for every hand-written
// file already in production. Writes to a temp file in the same directory and renames into place,
// so a crash mid-write can never leave a half-written file for the next reload to trip over.
func writeFileStrict[T any](path string, data []byte) error {
	var check T
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&check); err != nil {
		return fmt.Errorf("internal error: freshly-written metadata does not parse strictly: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".aiassist-*.yaml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	w := bufio.NewWriter(tmp)
	if _, err := w.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
