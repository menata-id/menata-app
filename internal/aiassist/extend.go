package aiassist

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/metadata"
)

// --- validation ----------------------------------------------------------------------------------

// validateExtendApplication checks every addition against the target Application as it is now plus
// whatever this same change adds before it: a navigation item may open a Machine the change adds, a
// new Machine's Permission may name a role the change adds, and a reorder must list the items the
// change adds. Additions are therefore read as one change, not a list of independent ones.
func validateExtendApplication(change GeneratedChange, existing ExistingState) error {
	target, ok := existing.Applications[change.TargetAppID]
	if !ok {
		return &ValidationError{Issues: []string{fmt.Sprintf("application %q is not installed in this workspace", change.TargetAppID)}}
	}
	if len(change.Additions) == 0 {
		return &ValidationError{Issues: []string{"extend_application change carries no additions -- put every change you describe into \"additions\""}}
	}

	roles := map[string]bool{}
	for _, r := range target.Roles {
		roles[r] = true
	}
	machines := map[string]bool{}
	for id := range target.Machines {
		machines[id] = true
	}
	navIDs := map[string]bool{}
	var navOrder []string
	for _, n := range target.Navigation {
		navIDs[n.ID] = true
		navOrder = append(navOrder, n.ID)
	}
	relationTo := relationTargets(existing)
	for _, add := range change.Additions {
		if add.NewRole != "" {
			roles[add.NewRole] = true
		}
		if add.NewMachine != nil {
			machines[add.NewMachine.ID] = true
			relationTo[add.NewMachine.ID] = true
		}
		if add.NewNavItem != nil {
			navOrder = append(navOrder, add.NewNavItem.ID)
		}
	}

	var issues []string
	addedMachine := map[string]bool{}
	addedNav := map[string]bool{}
	for i, add := range change.Additions {
		n := i + 1
		switch {
		case add.NewOption != "":
			issues = append(issues, validateNewOption(n, add, target, change.TargetAppID)...)
		case add.NewRole != "":
			if strings.TrimSpace(add.NewRole) == "" {
				issues = append(issues, fmt.Sprintf("addition %d: new role is empty", n))
			} else if contains(target.Roles, add.NewRole) {
				issues = append(issues, fmt.Sprintf("addition %d: role %q is already declared on application %q", n, add.NewRole, change.TargetAppID))
			}
		case add.NewMachine != nil:
			gm := *add.NewMachine
			if existing.MachineIDs[gm.ID] {
				issues = append(issues, fmt.Sprintf("addition %d: machine id %q already exists in this workspace", n, gm.ID))
			}
			if addedMachine[gm.ID] {
				issues = append(issues, fmt.Sprintf("addition %d: machine id %q is added twice", n, gm.ID))
			}
			addedMachine[gm.ID] = true
			for _, p := range gm.Permissions {
				for _, r := range p.Roles {
					if !roles[r] {
						issues = append(issues, fmt.Sprintf("addition %d: machine %q permission %q names role %q, which application %q does not declare -- add the role in this same change, or use one of %v", n, gm.ID, p.ID, r, change.TargetAppID, target.Roles))
					}
				}
			}
			m, fieldIssues := buildDomainMachine(gm, relationTo)
			issues = append(issues, fieldIssues...)
			if len(fieldIssues) == 0 {
				if err := metadata.Validate(m); err != nil {
					issues = append(issues, fmt.Sprintf("addition %d: machine %q: %v", n, gm.ID, err))
				}
			}
		case add.NewNavItem != nil:
			item := *add.NewNavItem
			if !machines[item.MachineID] {
				issues = append(issues, fmt.Sprintf("addition %d: menu item opens machine %q, which is not part of application %q", n, item.MachineID, change.TargetAppID))
			}
			if !strings.HasPrefix(item.ID, "nav_") || strings.TrimSpace(item.Label) == "" {
				issues = append(issues, fmt.Sprintf("addition %d: a menu item needs an id starting with nav_ and a label", n))
			}
			if existing.NavIDs[item.ID] || addedNav[item.ID] {
				issues = append(issues, fmt.Sprintf("addition %d: menu item id %q is already used in this workspace", n, item.ID))
			}
			if item.Icon != "" && !domain.KnownIcons[item.Icon] {
				issues = append(issues, fmt.Sprintf("addition %d: icon %q is not a known icon", n, item.Icon))
			}
			addedNav[item.ID] = true
		case add.RenameApplication != "":
			if strings.TrimSpace(add.RenameApplication) == target.Name {
				issues = append(issues, fmt.Sprintf("addition %d: application %q is already called %q", n, change.TargetAppID, target.Name))
			}
		case add.RelabelNavItem != nil:
			r := *add.RelabelNavItem
			if !navIDs[r.NavID] && !addedNav[r.NavID] {
				issues = append(issues, fmt.Sprintf("addition %d: menu item %q is not in application %q's menu (its items are %v)", n, r.NavID, change.TargetAppID, navOrder))
			}
			if strings.TrimSpace(r.Label) == "" {
				issues = append(issues, fmt.Sprintf("addition %d: menu item %q needs a label", n, r.NavID))
			}
		case len(add.ReorderNavigation) > 0:
			if !samePermutation(add.ReorderNavigation, navOrder) {
				issues = append(issues, fmt.Sprintf("addition %d: a new menu order must list every menu item exactly once -- %v, in any order -- got %v", n, navOrder, add.ReorderNavigation))
			}
		default:
			issues = append(issues, fmt.Sprintf("addition %d: names no actual change", n))
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func validateNewOption(n int, add MetadataAddition, target ExistingApplicationState, appID string) []string {
	m, ok := target.Machines[add.MachineID]
	if !ok {
		return []string{fmt.Sprintf("addition %d: machine %q is not part of application %q", n, add.MachineID, appID)}
	}
	f, ok := m.FieldByID(add.FieldID)
	switch {
	case !ok:
		return []string{fmt.Sprintf("addition %d: field %q is not on machine %q", n, add.FieldID, add.MachineID)}
	case f.Type != domain.FieldTypeStatus:
		return []string{fmt.Sprintf("addition %d: field %q is not a status field, so it has no options to add to", n, add.FieldID)}
	case contains(f.Options, add.NewOption):
		return []string{fmt.Sprintf("addition %d: %q is already one of field %q's declared options", n, add.NewOption, add.FieldID)}
	}
	return nil
}

func samePermutation(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	count := map[string]int{}
	for _, id := range want {
		count[id]++
	}
	for _, id := range got {
		count[id]--
		if count[id] < 0 {
			return false
		}
	}
	return true
}

// --- writing -------------------------------------------------------------------------------------

// writeExtension applies an extend_application change. Files are edited as YAML documents
// (yaml.Node, which keeps comments and each list's own style) rather than as text: the text edits
// this replaced could only append to a flow-style options list, which is not the style this package
// writes, so adding an option to a generated Machine failed; and a navigation item was validated and
// then never written at all.
func writeExtension(written *installer.WriteSet, workspaceManifestPath string, change GeneratedChange, resolve MachineFileResolver) error {
	workspaceDir := filepath.Dir(workspaceManifestPath)
	ownDir, err := installer.WorkspaceOwnDir(workspaceManifestPath)
	if err != nil {
		return err
	}
	// The Application file is read only if an addition edits it: adding an option touches a Machine
	// file alone.
	var app *yaml.Node
	var appPath, appRel string
	appDoc := func() (*yaml.Node, error) {
		if app != nil {
			return app, nil
		}
		rel, err := resolveApplicationFile(workspaceManifestPath, change.TargetAppID)
		if err != nil {
			return nil, err
		}
		appRel, appPath = rel, filepath.Join(workspaceDir, rel)
		app, err = readYAMLDoc(appPath)
		return app, err
	}
	manifest, err := os.ReadFile(workspaceManifestPath)
	if err != nil {
		return fmt.Errorf("read workspace manifest: %w", err)
	}
	manifestChanged := false

	for _, add := range change.Additions {
		if add.NewOption != "" {
			if err := addOption(written, workspaceDir, resolve, add); err != nil {
				return err
			}
			continue
		}
		app, err := appDoc()
		if err != nil {
			return err
		}
		switch {
		case add.NewRole != "":
			appendScalar(ensureSeq(app, "roles"), add.NewRole)
		case add.NewMachine != nil:
			path := filepath.Join(ownDir, strings.TrimPrefix(add.NewMachine.ID, "mch_")+".yaml")
			if err := installer.RefuseIfExists(path, "machine "+add.NewMachine.ID); err != nil {
				return installer.Rejected(err)
			}
			if err := written.Note(path); err != nil {
				return err
			}
			if err := installer.WriteYAMLStrict(path, machineDocFor(*add.NewMachine)); err != nil {
				return fmt.Errorf("write machine %s: %w", add.NewMachine.ID, err)
			}
			rel, err := filepath.Rel(workspaceDir, path)
			if err != nil {
				return err
			}
			if manifest, err = installer.AppendBlockListItem(manifest, "machines:", toSlash(rel)); err != nil {
				return fmt.Errorf("append machine to workspace manifest: %w", err)
			}
			manifestChanged = true
			appendScalar(ensureSeq(app, "machines"), add.NewMachine.ID)
		case add.NewNavItem != nil:
			appendNavItem(ensureSeq(app, "navigation"), *add.NewNavItem)
		case add.RenameApplication != "":
			setScalar(app, "name", strings.TrimSpace(add.RenameApplication))
		case add.RelabelNavItem != nil:
			item := navItemByID(ensureSeq(app, "navigation"), add.RelabelNavItem.NavID)
			if item == nil {
				return installer.Rejected(fmt.Errorf("menu item %q is not in %s", add.RelabelNavItem.NavID, appRel))
			}
			setScalar(item, "label", add.RelabelNavItem.Label)
		case len(add.ReorderNavigation) > 0:
			if err := reorderNav(ensureSeq(app, "navigation"), add.ReorderNavigation); err != nil {
				return installer.Rejected(err)
			}
		}
	}

	if app != nil {
		if err := writeYAMLDoc[installer.FullApplicationCheckDoc](written, appPath, app); err != nil {
			return err
		}
	}
	if manifestChanged {
		if err := written.Note(workspaceManifestPath); err != nil {
			return err
		}
		if err := installer.WriteFileStrict[installer.WorkspaceManifestCheckDoc](workspaceManifestPath, manifest); err != nil {
			return fmt.Errorf("write workspace manifest: %w", err)
		}
	}
	return nil
}

func addOption(written *installer.WriteSet, workspaceDir string, resolve MachineFileResolver, add MetadataAddition) error {
	rel, err := resolve.MachineFile(add.MachineID)
	if err != nil {
		return fmt.Errorf("resolve file for machine %s: %w", add.MachineID, err)
	}
	path := filepath.Join(workspaceDir, rel)
	doc, err := readYAMLDoc(path)
	if err != nil {
		return err
	}
	field := navItemByID(ensureSeq(doc, "fields"), add.FieldID)
	if field == nil {
		return installer.Rejected(fmt.Errorf("field %s is not declared in %s", add.FieldID, rel))
	}
	appendScalar(ensureSeq(field, "options"), add.NewOption)
	return writeYAMLDoc[installer.FullMachineCheckDoc](written, path, doc)
}

// --- yaml.Node helpers ---------------------------------------------------------------------------

// readYAMLDoc returns the top-level mapping of a YAML file.
func readYAMLDoc(path string) (*yaml.Node, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s is not a YAML mapping", path)
	}
	return doc.Content[0], nil
}

func writeYAMLDoc[T any](written *installer.WriteSet, path string, root *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := written.Note(path); err != nil {
		return err
	}
	return installer.WriteFileStrict[T](path, buf.Bytes())
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// ensureSeq returns m's sequence under key, creating an empty block sequence when it is absent or
// an empty flow one ("key: []").
func ensureSeq(m *yaml.Node, key string) *yaml.Node {
	if v := mapValue(m, key); v != nil {
		if v.Kind == yaml.SequenceNode {
			if len(v.Content) == 0 {
				v.Style = 0
			}
			return v
		}
		v.Kind, v.Tag, v.Value, v.Style = yaml.SequenceNode, "!!seq", "", 0
		return v
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, seq)
	return seq
}

func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

func appendScalar(seq *yaml.Node, v string) { seq.Content = append(seq.Content, scalar(v)) }

func setScalar(m *yaml.Node, key, v string) {
	if existing := mapValue(m, key); existing != nil {
		existing.Kind, existing.Tag, existing.Value, existing.Style = yaml.ScalarNode, "!!str", v, 0
		return
	}
	m.Content = append(m.Content, scalar(key), scalar(v))
}

// navItemByID finds the mapping in seq whose id: is id -- a navigation item, or a Field.
func navItemByID(seq *yaml.Node, id string) *yaml.Node {
	for _, item := range seq.Content {
		if item.Kind == yaml.MappingNode {
			if v := mapValue(item, "id"); v != nil && v.Value == id {
				return item
			}
		}
	}
	return nil
}

func appendNavItem(seq *yaml.Node, item GeneratedNavItem) {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setScalar(m, "id", item.ID)
	setScalar(m, "label", item.Label)
	setScalar(m, "route", "/machines/"+item.MachineID)
	if item.Title != "" {
		setScalar(m, "title", item.Title)
	}
	if item.Description != "" {
		setScalar(m, "description", item.Description)
	}
	if item.Icon != "" {
		setScalar(m, "icon", item.Icon)
	}
	// In a menu that orders by priority, an item without one sorts first (priority 0), so a new item
	// would land at the top instead of the end. It takes the next number instead.
	highest, usesPriority := 0, false
	for _, existing := range seq.Content {
		if v := mapValue(existing, "priority"); v != nil {
			usesPriority = true
			if n, err := strconv.Atoi(v.Value); err == nil && n > highest {
				highest = n
			}
		}
	}
	if usesPriority {
		setScalar(m, "priority", strconv.Itoa(highest+1))
		mapValue(m, "priority").Tag = "!!int"
	}
	seq.Content = append(seq.Content, m)
}

func reorderNav(seq *yaml.Node, order []string) error {
	byID := map[string]*yaml.Node{}
	for _, item := range seq.Content {
		if v := mapValue(item, "id"); v != nil {
			byID[v.Value] = item
		}
	}
	if len(byID) != len(seq.Content) || len(order) != len(seq.Content) {
		return fmt.Errorf("the new menu order lists %d items but the menu has %d", len(order), len(seq.Content))
	}
	reordered := make([]*yaml.Node, 0, len(order))
	for _, id := range order {
		item, ok := byID[id]
		if !ok {
			return fmt.Errorf("the new menu order names %q, which is not a menu item", id)
		}
		reordered = append(reordered, item)
		delete(byID, id)
	}
	seq.Content = reordered
	// Priority orders items ahead of their declared position (domain.NavigationItem.Priority), so in
	// a menu that uses it, moving entries alone would change nothing on screen. Renumbering keeps
	// the order the person asked for.
	usesPriority := false
	for _, item := range reordered {
		if mapValue(item, "priority") != nil {
			usesPriority = true
		}
	}
	if usesPriority {
		for i, item := range reordered {
			setScalar(item, "priority", strconv.Itoa(i+1))
			mapValue(item, "priority").Tag = "!!int"
		}
	}
	return nil
}
