package aiassist

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"menata.app/internal/installer"
	"menata.app/internal/metadata"
)

// writeUpdate applies an update_application change: it reloads the Workspace from the files it is
// about to edit, recomputes the plan against them (a plan computed at review time may be stale),
// refuses if the plan does, and then edits only the keys the generated shape names. Files are edited
// as yaml.Node documents, which keep comments, list styles and every key this package does not know
// -- that is what preserves the blocks the model never saw.
func writeUpdate(written *installer.WriteSet, workspaceManifestPath string, change GeneratedChange, resolve MachineFileResolver) error {
	loaded, err := metadata.LoadApplication(workspaceManifestPath)
	if err != nil {
		return fmt.Errorf("load workspace before updating: %w", err)
	}
	existing := ExistingStateFrom(loaded.Workspace)
	cur, ok := existing.Applications[change.TargetAppID]
	if !ok || change.Application == nil {
		return installer.Rejected(fmt.Errorf("application %q is not installed in this workspace", change.TargetAppID))
	}
	current := DescribeApplication(change.TargetAppID, cur)
	desired := *change.Application
	if plan := PlanUpdate(current, desired); len(plan.Refusals) > 0 {
		return installer.Rejected(fmt.Errorf("the update is refused: %s", strings.Join(plan.Refusals, "; ")))
	}

	workspaceDir := filepath.Dir(workspaceManifestPath)
	ownDir, err := installer.WorkspaceOwnDir(workspaceManifestPath)
	if err != nil {
		return err
	}
	appRel, err := resolveApplicationFile(workspaceManifestPath, change.TargetAppID)
	if err != nil {
		return err
	}
	appPath := filepath.Join(workspaceDir, appRel)
	app, err := readYAMLDoc(appPath)
	if err != nil {
		return err
	}

	setOrDelete(app, "name", desired.Name, current.Name)
	setOrDelete(app, "description", desired.Description, current.Description)
	setOrDelete(app, "icon", desired.Icon, current.Icon)
	setOrDelete(app, "color", desired.Color, current.Color)
	for _, r := range desired.Roles {
		if !slices.Contains(current.Roles, r) {
			appendScalar(ensureSeq(app, "roles"), r)
		}
	}

	currentMachines := map[string]GeneratedMachine{}
	for _, m := range current.Machines {
		currentMachines[m.ID] = m
	}
	manifest, err := os.ReadFile(workspaceManifestPath)
	if err != nil {
		return fmt.Errorf("read workspace manifest: %w", err)
	}
	manifestChanged := false
	for _, m := range desired.Machines {
		old, exists := currentMachines[m.ID]
		if !exists {
			path := filepath.Join(ownDir, strings.TrimPrefix(m.ID, "mch_")+".yaml")
			if err := installer.RefuseIfExists(path, "machine "+m.ID); err != nil {
				return installer.Rejected(err)
			}
			if err := written.Note(path); err != nil {
				return err
			}
			if err := installer.WriteYAMLStrict(path, machineDocFor(m)); err != nil {
				return fmt.Errorf("write machine %s: %w", m.ID, err)
			}
			rel, err := filepath.Rel(workspaceDir, path)
			if err != nil {
				return err
			}
			if manifest, err = installer.AppendBlockListItem(manifest, "machines:", toSlash(rel)); err != nil {
				return fmt.Errorf("append machine to workspace manifest: %w", err)
			}
			manifestChanged = true
			appendScalar(ensureSeq(app, "machines"), m.ID)
			continue
		}
		if reflect.DeepEqual(old, m) {
			continue
		}
		if err := updateMachineFile(written, workspaceDir, resolve, old, m); err != nil {
			return err
		}
	}

	if err := rebuildMenu(ensureSeq(app, "navigation"), desired, existing.NavIDs); err != nil {
		return installer.Rejected(err)
	}

	if err := writeYAMLDoc[installer.FullApplicationCheckDoc](written, appPath, app); err != nil {
		return err
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

// updateMachineFile edits one installed Machine's file toward its desired state. PlanUpdate has
// already refused every removal and retyping, so only additions and the permitted changes remain.
func updateMachineFile(written *installer.WriteSet, workspaceDir string, resolve MachineFileResolver, old, m GeneratedMachine) error {
	rel, err := resolve.MachineFile(m.ID)
	if err != nil {
		return fmt.Errorf("resolve file for machine %s: %w", m.ID, err)
	}
	path := filepath.Join(workspaceDir, rel)
	doc, err := readYAMLDoc(path)
	if err != nil {
		return err
	}
	setOrDelete(doc, "name", m.Name, old.Name)

	oldFields := map[string]GeneratedField{}
	for _, f := range old.Fields {
		oldFields[f.ID] = f
	}
	fields := ensureSeq(doc, "fields")
	for _, f := range m.Fields {
		of, exists := oldFields[f.ID]
		if !exists {
			if err := appendEncoded(fields, machineDocFor(GeneratedMachine{Fields: []GeneratedField{f}}).Fields[0]); err != nil {
				return err
			}
			continue
		}
		node := navItemByID(fields, f.ID)
		if node == nil {
			return fmt.Errorf("field %s is not declared in %s", f.ID, rel)
		}
		setOrDelete(node, "name", f.Name, of.Name)
		if f.Required != of.Required {
			setBool(node, "required", f.Required)
		}
		for _, o := range f.Options {
			if !slices.Contains(of.Options, o) {
				appendScalar(ensureSeq(node, "options"), o)
			}
		}
		if !sameCompute(of.Compute, f.Compute) {
			deleteKey(node, "compute")
			if f.Compute != nil {
				if err := setEncoded(node, "compute", computeDoc{Op: f.Compute.Op, Fields: f.Compute.Fields}); err != nil {
					return err
				}
			}
		}
	}

	oldPerms := map[string]GeneratedPermission{}
	for _, x := range old.Permissions {
		oldPerms[x.ID] = x
	}
	for _, x := range m.Permissions {
		ox, exists := oldPerms[x.ID]
		switch {
		case !exists:
			if err := appendEncoded(ensureSeq(doc, "permissions"), permissionDoc{ID: x.ID, Action: x.Action, Roles: x.Roles}); err != nil {
				return err
			}
		case !sameSet(x.Roles, ox.Roles):
			node := navItemByID(ensureSeq(doc, "permissions"), x.ID)
			if node == nil {
				return fmt.Errorf("permission %s is not declared in %s", x.ID, rel)
			}
			roles := ensureSeq(node, "roles")
			roles.Content = nil
			for _, r := range x.Roles {
				appendScalar(roles, r)
			}
		}
	}
	oldTrans := map[string]bool{}
	for _, x := range old.Transitions {
		oldTrans[x.ID] = true
	}
	for _, x := range m.Transitions {
		if !oldTrans[x.ID] {
			if err := appendEncoded(ensureSeq(doc, "transitions"), transitionDoc{ID: x.ID, Name: x.Name, Field: x.Field, From: x.From, To: x.To, Action: "edit"}); err != nil {
				return err
			}
		}
	}
	oldEvents := map[string]bool{}
	for _, x := range old.Events {
		oldEvents[x.ID] = true
	}
	for _, x := range m.Events {
		if !oldEvents[x.ID] {
			if err := appendEncoded(ensureSeq(doc, "events"), machineDocFor(GeneratedMachine{Events: []GeneratedEvent{x}}).Events[0]); err != nil {
				return err
			}
		}
	}
	return writeYAMLDoc[installer.FullMachineCheckDoc](written, path, doc)
}

// rebuildMenu makes the navigation sequence the desired menu: an existing item keeps its node (so
// its route, badge, group, icon and anything else declared stay), takes the new label and moves to
// its new position; a new item is a new node opening its Machine; an item left out is dropped. In a
// menu ordered by priority (domain.NavigationItem.Priority orders ahead of position) every item is
// renumbered, or the new order would not show.
func rebuildMenu(seq *yaml.Node, desired GeneratedApplication, takenIDs map[string]bool) error {
	byID := map[string]*yaml.Node{}
	usesPriority := false
	for _, item := range seq.Content {
		if v := mapValue(item, "id"); v != nil {
			byID[v.Value] = item
		}
		if mapValue(item, "priority") != nil {
			usesPriority = true
		}
	}
	appName := strings.TrimPrefix(desired.ID, "app_")
	var rebuilt []*yaml.Node
	for _, n := range desired.Navigation {
		if node, ok := byID[n.ID]; ok && n.ID != "" {
			setScalar(node, "label", n.Label)
			rebuilt = append(rebuilt, node)
			delete(byID, n.ID)
			continue
		}
		id := n.ID
		if id == "" || takenIDs[id] {
			id = uniqueNavID("nav_"+appName+"_"+strings.TrimPrefix(n.MachineID, "mch_"), takenIDs)
		}
		takenIDs[id] = true
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setScalar(node, "id", id)
		setScalar(node, "label", n.Label)
		setScalar(node, "route", "/machines/"+n.MachineID)
		rebuilt = append(rebuilt, node)
	}
	if len(rebuilt) > 0 && !anyHomeCard(rebuilt) {
		setBool(rebuilt[0], "home_card", true)
	}
	if usesPriority {
		for i, item := range rebuilt {
			setScalar(item, "priority", strconv.Itoa(i+1))
			mapValue(item, "priority").Tag = "!!int"
		}
	}
	seq.Content = rebuilt
	seq.Style = 0
	return nil
}

// anyHomeCard: the item the Home card opens may have been removed from the menu; the first item then
// takes over, since an Application whose card leads nowhere links back to /home.
func anyHomeCard(items []*yaml.Node) bool {
	for _, item := range items {
		if v := mapValue(item, "home_card"); v != nil && v.Value == "true" {
			return true
		}
	}
	return false
}

func uniqueNavID(base string, taken map[string]bool) string {
	id := base
	for i := 2; taken[id]; i++ {
		id = base + "_" + strconv.Itoa(i)
	}
	return id
}

func setOrDelete(m *yaml.Node, key, desired, current string) {
	if desired == current {
		return
	}
	if desired == "" {
		deleteKey(m, key)
		return
	}
	setScalar(m, key, desired)
}

func setBool(m *yaml.Node, key string, v bool) {
	setScalar(m, key, strconv.FormatBool(v))
	mapValue(m, key).Tag = "!!bool"
}

func deleteKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

func encodeNode(v any) (*yaml.Node, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	return &n, nil
}

func appendEncoded(seq *yaml.Node, v any) error {
	n, err := encodeNode(v)
	if err != nil {
		return err
	}
	seq.Content = append(seq.Content, n)
	return nil
}

func setEncoded(m *yaml.Node, key string, v any) error {
	n, err := encodeNode(v)
	if err != nil {
		return err
	}
	m.Content = append(m.Content, scalar(key), n)
	return nil
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
